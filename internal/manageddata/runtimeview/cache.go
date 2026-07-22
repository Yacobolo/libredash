// Package runtimeview materializes managed-data manifests as immutable local trees.
package runtimeview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata"
	"github.com/Yacobolo/leapview/internal/manageddata/storage"
)

const (
	privateDirectory  = os.FileMode(0o700)
	readOnlyDirectory = os.FileMode(0o500)
	stagingFile       = os.FileMode(0o600)
	readOnlyFile      = os.FileMode(0o400)
)

// Cache owns local execution copies of immutable managed-data revisions.
type Cache struct {
	root      string
	revisions string
	staging   string
	locks     string
	access    string
	blobs     storage.BlobStore
	now       func() time.Time
}

// New initializes a private runtime-view cache rooted at root.
func New(root string, blobs storage.BlobStore) (*Cache, error) {
	if root == "" {
		return nil, fmt.Errorf("%w: runtime view root is required", storage.ErrInvalid)
	}
	if blobs == nil {
		return nil, fmt.Errorf("%w: blob store is required", storage.ErrInvalid)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime view root: %w", err)
	}
	cache := &Cache{
		root:      absoluteRoot,
		revisions: filepath.Join(absoluteRoot, "revisions"),
		staging:   filepath.Join(absoluteRoot, "staging"),
		locks:     filepath.Join(absoluteRoot, "locks"),
		access:    filepath.Join(absoluteRoot, "access"),
		blobs:     blobs,
		now:       time.Now,
	}
	for _, directory := range []string{cache.root, cache.revisions, cache.staging, cache.locks, cache.access} {
		if err := ensurePrivateDirectory(directory); err != nil {
			return nil, err
		}
	}
	if err := cache.cleanupAbandonedStaging(); err != nil {
		return nil, err
	}
	return cache, nil
}

// MaterializeRevision returns a lease for the verified local root. The
// revision ID must be the canonical digest of manifest.
func (c *Cache) MaterializeRevision(ctx context.Context, revisionID string, manifest manageddata.Manifest) (manageddata.RevisionLease, error) {
	if err := validateMaterialization(revisionID, manifest); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	digest := revisionDigest(revisionID)
	lock, err := acquireFileLock(ctx, filepath.Join(c.locks, digest+".lock"))
	if err != nil {
		return nil, fmt.Errorf("lock runtime revision %s: %w", revisionID, err)
	}
	defer lock.release()

	if err := c.cleanupStaging(digest); err != nil {
		return nil, err
	}
	finalContainer := c.revisionContainerPath(revisionID)
	finalPath := c.revisionPath(revisionID)
	if _, err := os.Lstat(finalContainer); err == nil {
		if err := verifyRevisionContainer(ctx, finalContainer, finalPath, manifest); err != nil {
			return nil, err
		}
		return c.acquireRevisionLease(ctx, revisionID, finalPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect runtime revision %s: %w", revisionID, err)
	}

	temporary, err := os.MkdirTemp(c.staging, digest+".")
	if err != nil {
		return nil, fmt.Errorf("create runtime revision staging directory: %w", err)
	}
	if err := os.Chmod(temporary, privateDirectory); err != nil {
		_ = removeTree(temporary)
		return nil, fmt.Errorf("secure runtime revision staging directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = removeTree(temporary)
		}
	}()

	temporaryView := filepath.Join(temporary, "data")
	if err := os.Mkdir(temporaryView, privateDirectory); err != nil {
		return nil, fmt.Errorf("create runtime revision data root: %w", err)
	}
	files := append([]manageddata.File(nil), manifest.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		if err := c.materializeFile(ctx, temporaryView, file); err != nil {
			return nil, err
		}
	}
	if err := makeTreeReadOnly(temporaryView); err != nil {
		return nil, fmt.Errorf("make runtime revision read-only: %w", err)
	}
	if err := syncDirectory(temporary); err != nil {
		return nil, fmt.Errorf("sync runtime revision staging envelope: %w", err)
	}
	if err := os.Rename(temporary, finalContainer); err != nil {
		if _, statErr := os.Lstat(finalContainer); statErr == nil {
			if verifyErr := verifyRevisionContainer(ctx, finalContainer, finalPath, manifest); verifyErr != nil {
				return nil, verifyErr
			}
			return c.acquireRevisionLease(ctx, revisionID, finalPath)
		}
		return nil, fmt.Errorf("publish runtime revision %s: %w", revisionID, err)
	}
	cleanup = false
	if err := syncDirectory(c.revisions); err != nil {
		return nil, fmt.Errorf("sync published runtime revision: %w", err)
	}
	return c.acquireRevisionLease(ctx, revisionID, finalPath)
}

// EvictionCandidate identifies the exact access generation observed by a
// bounded cache scan.
type EvictionCandidate struct {
	RevisionID string
	LastUsed   time.Time
	Token      string
}

type accessRecord struct {
	RevisionID string    `json:"revision_id"`
	LastUsed   time.Time `json:"last_used"`
	Token      string    `json:"token"`
}

type revisionLease struct {
	root string
	lock *fileLock
	once sync.Once
	err  error
}

func (l *revisionLease) Root() string { return l.root }

func (l *revisionLease) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() { l.err = l.lock.release() })
	return l.err
}

func (c *Cache) acquireRevisionLease(ctx context.Context, revisionID, root string) (manageddata.RevisionLease, error) {
	if err := c.writeAccessRecord(revisionID); err != nil {
		return nil, err
	}
	lease, err := acquireSharedFileLock(ctx, c.lifetimeLockPath(revisionID))
	if err != nil {
		return nil, fmt.Errorf("lease runtime revision %s: %w", revisionID, err)
	}
	return &revisionLease{root: root, lock: lease}, nil
}

func (c *Cache) writeAccessRecord(revisionID string) error {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generate runtime revision access token: %w", err)
	}
	record := accessRecord{RevisionID: revisionID, LastUsed: c.now().UTC(), Token: hex.EncodeToString(tokenBytes)}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode runtime revision access record: %w", err)
	}
	temporary, err := os.CreateTemp(c.staging, revisionDigest(revisionID)+".access.*")
	if err != nil {
		return fmt.Errorf("create runtime revision access record: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(stagingFile); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, c.accessRecordPath(revisionID)); err != nil {
		return fmt.Errorf("publish runtime revision access record: %w", err)
	}
	if err := syncDirectory(c.access); err != nil {
		return fmt.Errorf("sync runtime revision access record: %w", err)
	}
	return nil
}

// ListEvictionCandidates returns at most limit idle generations ordered from
// least recently used to most recently used.
func (c *Cache) ListEvictionCandidates(ctx context.Context, cutoff time.Time, limit int) ([]EvictionCandidate, error) {
	if cutoff.IsZero() || limit <= 0 || limit > 10_000 {
		return nil, fmt.Errorf("%w: invalid runtime cache eviction bounds", storage.ErrInvalid)
	}
	directory, err := os.Open(c.revisions)
	if err != nil {
		return nil, fmt.Errorf("open runtime revision directory: %w", err)
	}
	defer directory.Close()
	candidates := make([]EvictionCandidate, 0, limit)
	for {
		entries, readErr := directory.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if storage.ValidateSHA256(entry.Name()) != nil || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("%w: runtime cache contains a noncanonical revision", storage.ErrIntegrity)
			}
			candidate, candidateErr := c.readEvictionCandidate("sha256:" + entry.Name())
			if candidateErr != nil {
				return nil, candidateErr
			}
			if candidate.LastUsed.After(cutoff) {
				continue
			}
			candidates = append(candidates, candidate)
			sort.Slice(candidates, func(i, j int) bool {
				if !candidates[i].LastUsed.Equal(candidates[j].LastUsed) {
					return candidates[i].LastUsed.Before(candidates[j].LastUsed)
				}
				return candidates[i].RevisionID < candidates[j].RevisionID
			})
			if len(candidates) > limit {
				candidates = candidates[:limit]
			}
		}
		if errors.Is(readErr, io.EOF) {
			return candidates, nil
		}
		if readErr != nil {
			return nil, fmt.Errorf("scan runtime revision directory: %w", readErr)
		}
	}
}

func (c *Cache) readEvictionCandidate(revisionID string) (EvictionCandidate, error) {
	file, err := os.Open(c.accessRecordPath(revisionID))
	if errors.Is(err, os.ErrNotExist) {
		info, statErr := os.Lstat(c.revisionContainerPath(revisionID))
		if statErr != nil {
			return EvictionCandidate{}, statErr
		}
		return EvictionCandidate{RevisionID: revisionID, LastUsed: info.ModTime().UTC(), Token: fmt.Sprintf("legacy:%d", info.ModTime().UnixNano())}, nil
	}
	if err != nil {
		return EvictionCandidate{}, fmt.Errorf("open runtime revision access record: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() {
		return EvictionCandidate{}, fmt.Errorf("%w: runtime revision access record is not regular", storage.ErrIntegrity)
	}
	pathInfo, err := os.Lstat(c.accessRecordPath(revisionID))
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedInfo, pathInfo) {
		return EvictionCandidate{}, fmt.Errorf("%w: runtime revision access record is unstable", storage.ErrIntegrity)
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var record accessRecord
	if err := decoder.Decode(&record); err != nil {
		return EvictionCandidate{}, fmt.Errorf("%w: invalid runtime revision access record", storage.ErrIntegrity)
	}
	if err := requireJSONEnd(decoder); err != nil || record.RevisionID != revisionID || record.Token == "" || record.LastUsed.IsZero() {
		return EvictionCandidate{}, fmt.Errorf("%w: invalid runtime revision access record", storage.ErrIntegrity)
	}
	return EvictionCandidate(record), nil
}

// DeleteIfIdle atomically proves that a candidate is unchanged and has no
// active materializer or runtime lease before deleting it.
func (c *Cache) DeleteIfIdle(ctx context.Context, candidate EvictionCandidate) (bool, error) {
	if err := validateRevisionID(candidate.RevisionID); err != nil || candidate.Token == "" || candidate.LastUsed.IsZero() {
		return false, fmt.Errorf("%w: invalid runtime cache eviction candidate", storage.ErrInvalid)
	}
	mutation, acquired, err := tryAcquireFileLock(filepath.Join(c.locks, revisionDigest(candidate.RevisionID)+".lock"))
	if err != nil || !acquired {
		return false, err
	}
	defer mutation.release()
	lifetime, acquired, err := tryAcquireFileLock(c.lifetimeLockPath(candidate.RevisionID))
	if err != nil || !acquired {
		return false, err
	}
	defer lifetime.release()
	current, err := c.readEvictionCandidate(candidate.RevisionID)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if current != candidate {
		return false, nil
	}
	if err := removeTree(c.revisionContainerPath(candidate.RevisionID)); err != nil {
		return false, fmt.Errorf("delete idle runtime revision: %w", err)
	}
	if err := os.Remove(c.accessRecordPath(candidate.RevisionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("delete runtime revision access record: %w", err)
	}
	if err := syncDirectory(c.revisions); err != nil {
		return false, err
	}
	if err := syncDirectory(c.access); err != nil {
		return false, err
	}
	return true, nil
}

func (c *Cache) lifetimeLockPath(revisionID string) string {
	return filepath.Join(c.locks, revisionDigest(revisionID)+".lease")
}

func (c *Cache) accessRecordPath(revisionID string) string {
	return filepath.Join(c.access, revisionDigest(revisionID)+".json")
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

// DeleteRevision removes a local revision cache entry after waiting for all
// active revision leases to drain.
func (c *Cache) DeleteRevision(ctx context.Context, revisionID string) error {
	if err := validateRevisionID(revisionID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	digest := revisionDigest(revisionID)
	lock, err := acquireFileLock(ctx, filepath.Join(c.locks, digest+".lock"))
	if err != nil {
		return fmt.Errorf("lock runtime revision %s for deletion: %w", revisionID, err)
	}
	defer lock.release()
	lifetime, err := acquireFileLock(ctx, c.lifetimeLockPath(revisionID))
	if err != nil {
		return fmt.Errorf("wait for runtime revision %s leases: %w", revisionID, err)
	}
	defer lifetime.release()
	if err := c.cleanupStaging(digest); err != nil {
		return err
	}
	if err := removeTree(c.revisionContainerPath(revisionID)); err != nil {
		return fmt.Errorf("delete runtime revision %s: %w", revisionID, err)
	}
	if err := os.Remove(c.accessRecordPath(revisionID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete runtime revision access record: %w", err)
	}
	if err := syncDirectory(c.revisions); err != nil {
		return fmt.Errorf("sync deleted runtime revision: %w", err)
	}
	if err := syncDirectory(c.access); err != nil {
		return fmt.Errorf("sync deleted runtime revision access record: %w", err)
	}
	return nil
}

func (c *Cache) materializeFile(ctx context.Context, root string, expected manageddata.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	destination := filepath.Join(root, filepath.FromSlash(expected.Path))
	if err := os.MkdirAll(filepath.Dir(destination), privateDirectory); err != nil {
		return fmt.Errorf("create directory for runtime file %q: %w", expected.Path, err)
	}
	source, err := c.blobs.Open(ctx, expected.SHA256)
	if err != nil {
		return fmt.Errorf("open managed blob for %q: %w", expected.Path, err)
	}
	if source == nil {
		return fmt.Errorf("%w: blob store returned a nil stream for %q", storage.ErrBackend, expected.Path)
	}
	destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, stagingFile)
	if err != nil {
		_ = source.Close()
		return fmt.Errorf("create runtime file %q: %w", expected.Path, err)
	}

	hash := sha256.New()
	reader := io.Reader(&contextReader{ctx: ctx, reader: source})
	if expected.Size < int64(^uint64(0)>>1) {
		reader = io.LimitReader(reader, expected.Size+1)
	}
	written, copyErr := io.Copy(io.MultiWriter(destinationFile, hash), reader)
	sourceCloseErr := source.Close()
	syncErr := destinationFile.Sync()
	chmodErr := destinationFile.Chmod(readOnlyFile)
	destinationCloseErr := destinationFile.Close()
	if copyErr != nil {
		return fmt.Errorf("stream managed blob for %q: %w", expected.Path, copyErr)
	}
	if sourceCloseErr != nil {
		return fmt.Errorf("close managed blob stream for %q: %w", expected.Path, sourceCloseErr)
	}
	if written != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return fmt.Errorf("%w: managed blob for %q does not match manifest size and SHA-256", storage.ErrIntegrity, expected.Path)
	}
	if syncErr != nil {
		return fmt.Errorf("sync runtime file %q: %w", expected.Path, syncErr)
	}
	if chmodErr != nil {
		return fmt.Errorf("make runtime file %q read-only: %w", expected.Path, chmodErr)
	}
	if destinationCloseErr != nil {
		return fmt.Errorf("close runtime file %q: %w", expected.Path, destinationCloseErr)
	}
	return nil
}

func (c *Cache) cleanupStaging(digest string) error {
	entries, err := os.ReadDir(c.staging)
	if err != nil {
		return fmt.Errorf("inspect runtime revision staging: %w", err)
	}
	prefix := digest + "."
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		if err := removeTree(filepath.Join(c.staging, entry.Name())); err != nil {
			return fmt.Errorf("clean abandoned runtime revision staging: %w", err)
		}
	}
	return nil
}

func (c *Cache) cleanupAbandonedStaging() error {
	entries, err := os.ReadDir(c.staging)
	if err != nil {
		return fmt.Errorf("inspect abandoned runtime revision staging: %w", err)
	}
	digests := make(map[string]struct{})
	for _, entry := range entries {
		name := entry.Name()
		if len(name) > sha256.Size*2 && name[sha256.Size*2] == '.' && storage.ValidateSHA256(name[:sha256.Size*2]) == nil {
			digests[name[:sha256.Size*2]] = struct{}{}
			continue
		}
		if err := removeTree(filepath.Join(c.staging, name)); err != nil {
			return fmt.Errorf("clean malformed runtime revision staging %q: %w", name, err)
		}
	}
	for digest := range digests {
		lock, acquired, err := tryAcquireFileLock(filepath.Join(c.locks, digest+".lock"))
		if err != nil {
			return fmt.Errorf("probe runtime revision staging lock: %w", err)
		}
		if !acquired {
			continue
		}
		cleanupErr := c.cleanupStaging(digest)
		releaseErr := lock.release()
		if cleanupErr != nil {
			return cleanupErr
		}
		if releaseErr != nil {
			return fmt.Errorf("release runtime revision staging lock: %w", releaseErr)
		}
	}
	return nil
}

func (c *Cache) revisionPath(revisionID string) string {
	return filepath.Join(c.revisionContainerPath(revisionID), "data")
}

func (c *Cache) revisionContainerPath(revisionID string) string {
	return filepath.Join(c.revisions, revisionDigest(revisionID))
}

func validateMaterialization(revisionID string, manifest manageddata.Manifest) error {
	if err := validateRevisionID(revisionID); err != nil {
		return err
	}
	if err := manifest.Validate(manageddata.Limits{}); err != nil {
		return fmt.Errorf("%w: invalid managed-data manifest: %v", storage.ErrInvalid, err)
	}
	if revisionID != manifest.RevisionID() {
		return fmt.Errorf("%w: revision ID does not match canonical manifest digest", storage.ErrInvalid)
	}
	paths := make(map[string]struct{}, len(manifest.Files))
	for _, file := range manifest.Files {
		localPath := filepath.FromSlash(file.Path)
		if strings.IndexByte(file.Path, 0) >= 0 || !filepath.IsLocal(localPath) || filepath.VolumeName(localPath) != "" {
			return fmt.Errorf("%w: manifest path %q is not a safe local path", storage.ErrInvalid, file.Path)
		}
		paths[file.Path] = struct{}{}
	}
	for logicalPath := range paths {
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(logicalPath))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			if _, collision := paths[parent]; collision {
				return fmt.Errorf("%w: manifest path %q is both a file and a directory", storage.ErrInvalid, parent)
			}
		}
	}
	return nil
}

func validateRevisionID(revisionID string) error {
	const prefix = "sha256:"
	if !strings.HasPrefix(revisionID, prefix) {
		return fmt.Errorf("%w: revision ID must use the sha256 scheme", storage.ErrInvalid)
	}
	if err := storage.ValidateSHA256(strings.TrimPrefix(revisionID, prefix)); err != nil {
		return fmt.Errorf("%w: revision ID must contain a canonical SHA-256 digest", storage.ErrInvalid)
	}
	return nil
}

func revisionDigest(revisionID string) string {
	return strings.TrimPrefix(revisionID, "sha256:")
}

func verifyRevision(ctx context.Context, root string, manifest manageddata.Manifest) error {
	expectedFiles := make(map[string]manageddata.File, len(manifest.Files))
	expectedDirectories := map[string]struct{}{".": {}}
	for _, file := range manifest.Files {
		expectedFiles[file.Path] = file
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(file.Path))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			expectedDirectories[parent] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(expectedFiles))
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		logicalPath := filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: runtime revision path %q is a symlink", storage.ErrIntegrity, logicalPath)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if _, ok := expectedDirectories[logicalPath]; !ok {
				return fmt.Errorf("%w: runtime revision contains unexpected directory %q", storage.ErrIntegrity, logicalPath)
			}
			if info.Mode().Perm()&0o222 != 0 || info.Mode().Perm()&0o500 != 0o500 {
				return fmt.Errorf("%w: runtime revision directory %q is not read-only and accessible", storage.ErrIntegrity, logicalPath)
			}
			return nil
		}
		expected, ok := expectedFiles[logicalPath]
		if !ok {
			return fmt.Errorf("%w: runtime revision contains unexpected file %q", storage.ErrIntegrity, logicalPath)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: runtime revision path %q is not a regular file", storage.ErrIntegrity, logicalPath)
		}
		if info.Mode().Perm()&0o222 != 0 || info.Mode().Perm()&0o400 == 0 {
			return fmt.Errorf("%w: runtime revision file %q is not read-only and readable", storage.ErrIntegrity, logicalPath)
		}
		if info.Size() != expected.Size {
			return fmt.Errorf("%w: runtime revision file %q has the wrong size", storage.ErrIntegrity, logicalPath)
		}
		file, err := os.Open(current)
		if err != nil {
			return err
		}
		openedInfo, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return statErr
		}
		if !os.SameFile(info, openedInfo) {
			_ = file.Close()
			return fmt.Errorf("%w: runtime revision file %q changed during verification", storage.ErrIntegrity, logicalPath)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, &contextReader{ctx: ctx, reader: file})
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
			return fmt.Errorf("%w: runtime revision file %q has the wrong SHA-256", storage.ErrIntegrity, logicalPath)
		}
		seen[logicalPath] = struct{}{}
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, storage.ErrIntegrity) {
			return err
		}
		return fmt.Errorf("%w: verify runtime revision: %v", storage.ErrIntegrity, err)
	}
	if len(seen) != len(expectedFiles) {
		return fmt.Errorf("%w: runtime revision is missing manifest files", storage.ErrIntegrity)
	}
	return nil
}

func verifyRevisionContainer(ctx context.Context, container, root string, manifest manageddata.Manifest) error {
	info, err := os.Lstat(container)
	if err != nil {
		return fmt.Errorf("%w: inspect runtime revision envelope: %v", storage.ErrIntegrity, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: runtime revision envelope is not a real directory", storage.ErrIntegrity)
	}
	if info.Mode().Perm() != privateDirectory {
		return fmt.Errorf("%w: runtime revision envelope is not private", storage.ErrIntegrity)
	}
	entries, err := os.ReadDir(container)
	if err != nil {
		return fmt.Errorf("%w: inspect runtime revision envelope: %v", storage.ErrIntegrity, err)
	}
	if len(entries) != 1 || entries[0].Name() != "data" || !entries[0].IsDir() || entries[0].Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: runtime revision envelope must contain only its data root", storage.ErrIntegrity)
	}
	return verifyRevision(ctx, root, manifest)
}

func ensurePrivateDirectory(directory string) error {
	if err := os.MkdirAll(directory, privateDirectory); err != nil {
		return fmt.Errorf("create runtime view directory %q: %w", directory, err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect runtime view directory %q: %w", directory, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: runtime view path %q is not a real directory", storage.ErrInvalid, directory)
	}
	if err := os.Chmod(directory, privateDirectory); err != nil {
		return fmt.Errorf("secure runtime view directory %q: %w", directory, err)
	}
	return nil
}

func makeTreeReadOnly(root string) error {
	var directories []string
	if err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse symlink in runtime revision staging at %q", current)
		}
		if entry.IsDir() {
			directories = append(directories, current)
			return nil
		}
		if err := os.Chmod(current, readOnlyFile); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
		if err := os.Chmod(directory, readOnlyDirectory); err != nil {
			return err
		}
	}
	return nil
}

func removeTree(root string) error {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return os.Remove(root)
	}
	if err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(current, privateDirectory)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

var _ manageddata.RevisionMaterializer = (*Cache)(nil)

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
