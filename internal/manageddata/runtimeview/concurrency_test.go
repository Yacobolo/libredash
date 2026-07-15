package runtimeview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/Yacobolo/libredash/internal/manageddata/storage"
)

func TestConcurrentMaterializeStreamsRevisionOnlyOnce(t *testing.T) {
	manifest, blobs := manifestAndBlobs(map[string][]byte{"data.csv": []byte("shared")})
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

	const callers = 16
	results := make(chan manageddata.RevisionLease, callers)
	errorsFound := make(chan error, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			view, materializeErr := cache.MaterializeRevision(context.Background(), manifest.RevisionID(), manifest)
			results <- view
			errorsFound <- materializeErr
		}()
	}
	<-started
	close(release)
	wait.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	var path string
	for view := range results {
		if path == "" {
			path = view.Root()
		}
		if view.Root() != path {
			t.Fatalf("concurrent view path = %q, want %q", view.Root(), path)
		}
		if err := view.Release(); err != nil {
			t.Fatal(err)
		}
	}
	if got := store.openCount(); got != len(manifest.Files) {
		t.Fatalf("blob store Open() calls = %d, want %d", got, len(manifest.Files))
	}
}

func TestNewCleansAbandonedStagingButSkipsLockedRevision(t *testing.T) {
	root := t.TempDir()
	store := newMemoryStore(map[string][]byte{})
	cache, err := New(root, store)
	if err != nil {
		t.Fatal(err)
	}
	abandonedDigest := strings.Repeat("a", 64)
	activeDigest := strings.Repeat("b", 64)
	abandoned := filepath.Join(cache.staging, abandonedDigest+".abandoned")
	active := filepath.Join(cache.staging, activeDigest+".active")
	for _, directory := range []string{abandoned, active} {
		if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(directory, 0o500); err != nil {
			t.Fatal(err)
		}
	}
	held, err := acquireFileLock(t.Context(), filepath.Join(cache.locks, activeDigest+".lock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(abandoned); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abandoned staging still exists: %v", err)
	}
	if _, err := os.Lstat(active); err != nil {
		t.Fatalf("active staging was removed: %v", err)
	}
	if err := held.release(); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(active); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released staging still exists: %v", err)
	}
}

func TestRevisionLockCoordinatesAcrossProcessesAndHonorsContext(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "revision.lock")
	readyPath := filepath.Join(root, "ready")
	releasePath := filepath.Join(root, "release")
	command := exec.Command(os.Args[0], "-test.run=^TestRevisionLockHelperProcess$")
	command.Env = append(os.Environ(),
		"LIBREDASH_RUNTIMEVIEW_HELPER_LOCK="+lockPath,
		"LIBREDASH_RUNTIMEVIEW_HELPER_READY="+readyPath,
		"LIBREDASH_RUNTIMEVIEW_HELPER_RELEASE="+releasePath,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(releasePath, []byte("release"), 0o600)
		_ = command.Process.Kill()
	})
	helperDone := make(chan error, 1)
	go func() { helperDone <- command.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not acquire the process lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if lock, err := acquireFileLock(ctx, lockPath); !errors.Is(err, context.DeadlineExceeded) {
		if err == nil {
			_ = lock.release()
		}
		t.Fatalf("contended acquireFileLock() error = %v, want %v", err, context.DeadlineExceeded)
	}
	if err := os.WriteFile(releasePath, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-helperDone; err != nil {
		t.Fatal(err)
	}
	lock, err := acquireFileLock(t.Context(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
}

func TestRevisionLockHelperProcess(t *testing.T) {
	lockPath := os.Getenv("LIBREDASH_RUNTIMEVIEW_HELPER_LOCK")
	if lockPath == "" {
		return
	}
	held, err := acquireFileLock(t.Context(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	if err := os.WriteFile(os.Getenv("LIBREDASH_RUNTIMEVIEW_HELPER_READY"), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(os.Getenv("LIBREDASH_RUNTIMEVIEW_HELPER_RELEASE")); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for release")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDeleteRevisionIsExplicitIdempotentAndValidatesID(t *testing.T) {
	manifest, blobs := manifestAndBlobs(map[string][]byte{"data.csv": []byte("delete")})
	cache, err := New(t.TempDir(), newMemoryStore(blobs))
	if err != nil {
		t.Fatal(err)
	}
	view, err := cache.MaterializeRevision(t.Context(), manifest.RevisionID(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := view.Release(); err != nil {
		t.Fatal(err)
	}
	if err := cache.DeleteRevision(t.Context(), manifest.RevisionID()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(view.Root()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted revision still exists: %v", err)
	}
	if err := cache.DeleteRevision(t.Context(), manifest.RevisionID()); err != nil {
		t.Fatalf("idempotent DeleteRevision() error = %v", err)
	}
	if err := cache.DeleteRevision(t.Context(), "../revision"); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("unsafe DeleteRevision() error = %v, want %v", err, storage.ErrInvalid)
	}
}
