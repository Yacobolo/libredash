// Package runtimeview materializes managed-data manifests as immutable local trees.
package runtimeview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/Yacobolo/libredash/internal/manageddata/storage"
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
	blobs     storage.BlobStore
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
		blobs:     blobs,
	}
	for _, directory := range []string{cache.root, cache.revisions, cache.staging, cache.locks} {
		if err := ensurePrivateDirectory(directory); err != nil {
			return nil, err
		}
	}
	if err := cache.cleanupAbandonedStaging(); err != nil {
		return nil, err
	}
	return cache, nil
}

// Materialize returns the verified local root for revisionID. The revision ID
// must be the canonical digest of manifest.
func (c *Cache) Materialize(ctx context.Context, revisionID string, manifest manageddata.Manifest) (storage.RevisionView, error) {
	if err := validateMaterialization(revisionID, manifest); err != nil {
		return storage.RevisionView{}, err
	}
	if err := ctx.Err(); err != nil {
		return storage.RevisionView{}, err
	}
	digest := revisionDigest(revisionID)
	lock, err := acquireFileLock(ctx, filepath.Join(c.locks, digest+".lock"))
	if err != nil {
		return storage.RevisionView{}, fmt.Errorf("lock runtime revision %s: %w", revisionID, err)
	}
	defer lock.release()

	if err := c.cleanupStaging(digest); err != nil {
		return storage.RevisionView{}, err
	}
	finalContainer := c.revisionContainerPath(revisionID)
	finalPath := c.revisionPath(revisionID)
	if _, err := os.Lstat(finalContainer); err == nil {
		if err := verifyRevisionContainer(ctx, finalContainer, finalPath, manifest); err != nil {
			return storage.RevisionView{}, err
		}
		return storage.RevisionView{ID: revisionID, Path: finalPath}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return storage.RevisionView{}, fmt.Errorf("inspect runtime revision %s: %w", revisionID, err)
	}

	temporary, err := os.MkdirTemp(c.staging, digest+".")
	if err != nil {
		return storage.RevisionView{}, fmt.Errorf("create runtime revision staging directory: %w", err)
	}
	if err := os.Chmod(temporary, privateDirectory); err != nil {
		_ = removeTree(temporary)
		return storage.RevisionView{}, fmt.Errorf("secure runtime revision staging directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = removeTree(temporary)
		}
	}()

	temporaryView := filepath.Join(temporary, "data")
	if err := os.Mkdir(temporaryView, privateDirectory); err != nil {
		return storage.RevisionView{}, fmt.Errorf("create runtime revision data root: %w", err)
	}
	files := append([]manageddata.File(nil), manifest.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		if err := c.materializeFile(ctx, temporaryView, file); err != nil {
			return storage.RevisionView{}, err
		}
	}
	if err := makeTreeReadOnly(temporaryView); err != nil {
		return storage.RevisionView{}, fmt.Errorf("make runtime revision read-only: %w", err)
	}
	if err := syncDirectory(temporary); err != nil {
		return storage.RevisionView{}, fmt.Errorf("sync runtime revision staging envelope: %w", err)
	}
	if err := os.Rename(temporary, finalContainer); err != nil {
		if _, statErr := os.Lstat(finalContainer); statErr == nil {
			if verifyErr := verifyRevisionContainer(ctx, finalContainer, finalPath, manifest); verifyErr != nil {
				return storage.RevisionView{}, verifyErr
			}
			return storage.RevisionView{ID: revisionID, Path: finalPath}, nil
		}
		return storage.RevisionView{}, fmt.Errorf("publish runtime revision %s: %w", revisionID, err)
	}
	cleanup = false
	if err := syncDirectory(c.revisions); err != nil {
		return storage.RevisionView{}, fmt.Errorf("sync published runtime revision: %w", err)
	}
	return storage.RevisionView{ID: revisionID, Path: finalPath}, nil
}

// DeleteRevision removes a local revision cache entry. Callers must first
// prove that no query or other consumer is using the returned revision root.
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
	if err := c.cleanupStaging(digest); err != nil {
		return err
	}
	if err := removeTree(c.revisionContainerPath(revisionID)); err != nil {
		return fmt.Errorf("delete runtime revision %s: %w", revisionID, err)
	}
	if err := syncDirectory(c.revisions); err != nil {
		return fmt.Errorf("sync deleted runtime revision: %w", err)
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
