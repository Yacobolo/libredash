package runtimeview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/Yacobolo/libredash/internal/manageddata/storage"
)

func TestMaterializePublishesImmutableVerifiedRevision(t *testing.T) {
	bodyByPath := map[string][]byte{
		"customers.csv":          []byte("customer_id\n1\n"),
		"orders/2026/orders.csv": []byte("order_id,total\n1,42\n"),
	}
	manifest, blobs := manifestAndBlobs(bodyByPath)
	store := newMemoryStore(blobs)
	root := filepath.Join(t.TempDir(), "runtime")
	cache, err := New(root, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.DeleteRevision(context.Background(), manifest.RevisionID()) })

	view, err := cache.Materialize(t.Context(), manifest.RevisionID(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if view.ID != manifest.RevisionID() {
		t.Fatalf("view ID = %q, want %q", view.ID, manifest.RevisionID())
	}
	if view.Path != filepath.Join(root, "revisions", strings.TrimPrefix(manifest.RevisionID(), "sha256:"), "data") {
		t.Fatalf("view path = %q", view.Path)
	}
	for logicalPath, want := range bodyByPath {
		got, readErr := os.ReadFile(filepath.Join(view.Path, filepath.FromSlash(logicalPath)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s content = %q, want %q", logicalPath, got, want)
		}
	}
	assertPermissions(t, root, 0o700)
	assertPermissions(t, view.Path, 0o500)
	assertPermissions(t, filepath.Join(view.Path, "customers.csv"), 0o400)
	assertPermissions(t, filepath.Join(view.Path, "orders"), 0o500)
	assertPermissions(t, filepath.Join(view.Path, "orders", "2026"), 0o500)

	before := store.openCount()
	reused, err := cache.Materialize(t.Context(), manifest.RevisionID(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if reused != view {
		t.Fatalf("reused view = %#v, want %#v", reused, view)
	}
	if got := store.openCount(); got != before {
		t.Fatalf("idempotent reuse opened blob store %d additional times", got-before)
	}
}

func TestMaterializeRequiresCanonicalMatchingRevisionIDAndSafePaths(t *testing.T) {
	body := []byte("orders")
	digest := digestOf(body)
	valid := manageddata.Manifest{Files: []manageddata.File{{Path: "orders.csv", Size: int64(len(body)), SHA256: digest}}}
	tests := []struct {
		name       string
		revisionID string
		manifest   manageddata.Manifest
	}{
		{name: "missing scheme", revisionID: strings.TrimPrefix(valid.RevisionID(), "sha256:"), manifest: valid},
		{name: "uppercase", revisionID: "sha256:" + strings.ToUpper(strings.TrimPrefix(valid.RevisionID(), "sha256:")), manifest: valid},
		{name: "different canonical manifest", revisionID: "sha256:" + strings.Repeat("a", 64), manifest: valid},
		{name: "traversal", revisionID: valid.RevisionID(), manifest: manageddata.Manifest{Files: []manageddata.File{{Path: "../orders.csv", Size: int64(len(body)), SHA256: digest}}}},
		{name: "file directory collision", manifest: manageddata.Manifest{Files: []manageddata.File{
			{Path: "orders", Size: int64(len(body)), SHA256: digest},
			{Path: "orders/2026.csv", Size: int64(len(body)), SHA256: digest},
		}}},
	}
	for index := range tests {
		test := &tests[index]
		if test.revisionID == "" {
			test.revisionID = test.manifest.RevisionID()
		}
		t.Run(test.name, func(t *testing.T) {
			cache, err := New(t.TempDir(), newMemoryStore(map[string][]byte{digest: body}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := cache.Materialize(t.Context(), test.revisionID, test.manifest); !errors.Is(err, storage.ErrInvalid) {
				t.Fatalf("Materialize() error = %v, want %v", err, storage.ErrInvalid)
			}
		})
	}
}

func TestMaterializeRejectsBlobSizeAndDigestMismatchWithoutPublishing(t *testing.T) {
	wantBody := []byte("expected")
	digest := digestOf(wantBody)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "size", body: append(append([]byte(nil), wantBody...), '!')},
		{name: "digest", body: []byte("differed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := manageddata.Manifest{Files: []manageddata.File{{Path: "data.csv", Size: int64(len(wantBody)), SHA256: digest}}}
			root := t.TempDir()
			cache, err := New(root, newMemoryStore(map[string][]byte{digest: test.body}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = cache.Materialize(t.Context(), manifest.RevisionID(), manifest)
			if !errors.Is(err, storage.ErrIntegrity) {
				t.Fatalf("Materialize() error = %v, want %v", err, storage.ErrIntegrity)
			}
			if _, statErr := os.Lstat(cache.revisionPath(manifest.RevisionID())); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("partial revision is visible: %v", statErr)
			}
			entries, readErr := os.ReadDir(cache.staging)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("staging contains %d abandoned entries", len(entries))
			}
		})
	}
}

func TestMaterializeDoesNotExposeRevisionBeforeCompleteReadOnlyPublication(t *testing.T) {
	body := []byte("streamed")
	manifest, blobs := manifestAndBlobs(map[string][]byte{"data.csv": body})
	digest := manifest.Files[0].SHA256
	started := make(chan struct{})
	release := make(chan struct{})
	store := newMemoryStore(blobs)
	store.open = func(content []byte) io.ReadCloser {
		return &blockingReadCloser{Reader: bytes.NewReader(content), started: started, release: release}
	}
	cache, err := New(t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.DeleteRevision(context.Background(), manifest.RevisionID()) })
	result := make(chan error, 1)
	go func() {
		_, materializeErr := cache.Materialize(t.Context(), manifest.RevisionID(), manifest)
		result <- materializeErr
	}()
	<-started
	if _, err := os.Lstat(cache.revisionPath(manifest.RevisionID())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("revision became visible while blob %s was streaming: %v", digest, err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	assertPermissions(t, cache.revisionPath(manifest.RevisionID()), 0o500)
}

func TestMaterializeRejectsCorruptOrSymlinkedExistingView(t *testing.T) {
	body := []byte("original")
	manifest, blobs := manifestAndBlobs(map[string][]byte{"nested/data.csv": body})
	store := newMemoryStore(blobs)
	cache, err := New(t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	view, err := cache.Materialize(t.Context(), manifest.RevisionID(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(view.Path, "nested", "data.csv")
	if err := os.Chmod(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := store.openCount()
	if _, err := cache.Materialize(t.Context(), manifest.RevisionID(), manifest); !errors.Is(err, storage.ErrIntegrity) {
		t.Fatalf("corrupt existing Materialize() error = %v, want %v", err, storage.ErrIntegrity)
	}
	if got := store.openCount(); got != before {
		t.Fatal("corrupt existing view was silently rebuilt from blob storage")
	}

	if err := cache.DeleteRevision(t.Context(), manifest.RevisionID()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(view.Path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(view.Path, "nested")); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Materialize(t.Context(), manifest.RevisionID(), manifest); !errors.Is(err, storage.ErrIntegrity) {
		t.Fatalf("symlinked existing Materialize() error = %v, want %v", err, storage.ErrIntegrity)
	}
}

type memoryStore struct {
	mu    sync.Mutex
	blobs map[string][]byte
	opens int
	open  func([]byte) io.ReadCloser
}

func newMemoryStore(blobs map[string][]byte) *memoryStore {
	return &memoryStore{blobs: blobs, open: func(content []byte) io.ReadCloser {
		return io.NopCloser(bytes.NewReader(content))
	}}
}

func (s *memoryStore) Put(context.Context, storage.Blob, io.Reader) (storage.Blob, error) {
	panic("unexpected Put")
}

func (s *memoryStore) Stat(context.Context, string) (storage.Blob, error) {
	panic("unexpected Stat")
}

func (s *memoryStore) Open(_ context.Context, digest string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, ok := s.blobs[digest]
	if !ok {
		return nil, storage.ErrNotFound
	}
	s.opens++
	return s.open(append([]byte(nil), body...)), nil
}

func (s *memoryStore) DeleteUnreachable(context.Context, []string) error {
	panic("unexpected DeleteUnreachable")
}

func (s *memoryStore) openCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opens
}

type blockingReadCloser struct {
	*bytes.Reader
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingReadCloser) Read(buffer []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return r.Reader.Read(buffer)
}

func (*blockingReadCloser) Close() error { return nil }

func manifestAndBlobs(bodyByPath map[string][]byte) (manageddata.Manifest, map[string][]byte) {
	manifest := manageddata.Manifest{Files: make([]manageddata.File, 0, len(bodyByPath))}
	blobs := make(map[string][]byte, len(bodyByPath))
	for logicalPath, body := range bodyByPath {
		digest := digestOf(body)
		manifest.Files = append(manifest.Files, manageddata.File{Path: logicalPath, Size: int64(len(body)), SHA256: digest})
		blobs[digest] = body
	}
	return manifest, blobs
}

func digestOf(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func assertPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s permissions = %o, want %o", path, got, want)
	}
}
