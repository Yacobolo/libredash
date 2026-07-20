// Package storagetest provides backend conformance tests for managed-data storage.
package storagetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/Yacobolo/leapview/internal/manageddata/storage"
)

type Factory func(t *testing.T) storage.BlobStore

func BlobStoreConformance(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("put stat and open", func(t *testing.T) {
		store := factory(t)
		body := []byte("managed data")
		expected := expectedBlob(body)

		first, err := store.Put(t.Context(), expected, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.Put(t.Context(), expected, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if first != second || first.URI == "" {
			t.Fatalf("idempotent Put() = %#v, %#v", first, second)
		}

		stat, err := store.Stat(t.Context(), expected.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if stat != first {
			t.Fatalf("Stat() = %#v, want %#v", stat, first)
		}
		reader, err := store.Open(t.Context(), expected.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		got, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read = %v, close = %v", readErr, closeErr)
		}
		if !bytes.Equal(got, body) {
			t.Fatalf("Open() = %q, want %q", got, body)
		}
	})

	t.Run("server verifies size and hash", func(t *testing.T) {
		store := factory(t)
		body := []byte("verified")
		expected := expectedBlob(body)
		expected.Size++
		if _, err := store.Put(t.Context(), expected, bytes.NewReader(body)); !errors.Is(err, storage.ErrIntegrity) {
			t.Fatalf("size mismatch error = %v", err)
		}

		expected = expectedBlob(body)
		expected.SHA256 = digest([]byte("different"))
		if _, err := store.Put(t.Context(), expected, bytes.NewReader(body)); !errors.Is(err, storage.ErrIntegrity) {
			t.Fatalf("hash mismatch error = %v", err)
		}
	})

	t.Run("deletes only explicit unreachable blobs", func(t *testing.T) {
		store := factory(t)
		inventory, ok := store.(storage.BlobInventory)
		if !ok {
			t.Fatal("BlobStore does not implement BlobInventory")
		}
		keepBody := []byte("keep")
		deleteBody := []byte("delete")
		keep := expectedBlob(keepBody)
		unreachable := expectedBlob(deleteBody)
		mustPut(t, store, keep, keepBody)
		mustPut(t, store, unreachable, deleteBody)

		if err := inventory.DeleteBlobs(t.Context(), []string{unreachable.SHA256, digest([]byte("missing"))}); err != nil {
			t.Fatal(err)
		}
		if err := inventory.DeleteBlobs(t.Context(), []string{unreachable.SHA256}); err != nil {
			t.Fatalf("idempotent DeleteBlobs() = %v", err)
		}
		if _, err := store.Stat(t.Context(), unreachable.SHA256); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("deleted Stat() error = %v", err)
		}
		if _, err := store.Stat(t.Context(), keep.SHA256); err != nil {
			t.Fatalf("unlisted blob was deleted: %v", err)
		}
	})

	t.Run("validates digests before backend access", func(t *testing.T) {
		store := factory(t)
		inventory, ok := store.(storage.BlobInventory)
		if !ok {
			t.Fatal("BlobStore does not implement BlobInventory")
		}
		if _, err := store.Stat(context.Background(), "../escape"); !errors.Is(err, storage.ErrInvalid) {
			t.Fatalf("Stat() error = %v", err)
		}
		if err := inventory.DeleteBlobs(context.Background(), []string{"../escape"}); !errors.Is(err, storage.ErrInvalid) {
			t.Fatalf("DeleteBlobs() error = %v", err)
		}
	})
}

func expectedBlob(body []byte) storage.Blob {
	return storage.Blob{SHA256: digest(body), Size: int64(len(body))}
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func mustPut(t *testing.T, store storage.BlobStore, blob storage.Blob, body []byte) {
	t.Helper()
	if _, err := store.Put(t.Context(), blob, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
}
