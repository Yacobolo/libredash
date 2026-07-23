package mapasset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishFilesIsImmutableAndIdempotent(t *testing.T) {
	root := t.TempDir()
	content := []byte("pinned map data")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	file := File{Path: "maps/archives/" + digest + "/basemap.pmtiles", Digest: digest}
	name := filepath.Join(root, filepath.FromSlash(file.Path))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, content, 0o644); err != nil {
		t.Fatal(err)
	}
	store := &memoryPublicationStore{objects: map[string]publishedMemoryObject{}}

	first, err := publishFiles(context.Background(), root, []File{file}, store)
	if err != nil {
		t.Fatal(err)
	}
	second, err := publishFiles(context.Background(), root, []File{file}, store)
	if err != nil {
		t.Fatal(err)
	}
	if first.Uploaded != 1 || first.Reused != 0 || second.Uploaded != 0 || second.Reused != 1 || store.puts != 1 {
		t.Fatalf("summaries = %#v then %#v, puts = %d", first, second, store.puts)
	}
	object := store.objects[file.Path]
	if !bytes.Equal(object.body, content) || object.spec.Digest != digest || object.spec.CacheControl != ImmutableCacheControl || object.spec.ContentType != "application/vnd.pmtiles" {
		t.Fatalf("published object = %#v", object)
	}
}

func TestPublishFilesRejectsConflictingRemoteObject(t *testing.T) {
	root := t.TempDir()
	content := []byte("expected")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	file := File{Path: "maps/styles/" + digest + "/style.json", Digest: digest}
	name := filepath.Join(root, filepath.FromSlash(file.Path))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, content, 0o644); err != nil {
		t.Fatal(err)
	}
	store := &memoryPublicationStore{objects: map[string]publishedMemoryObject{
		file.Path: {spec: PublicationObject{Key: file.Path, Digest: "forged", Size: int64(len(content))}},
	}}

	_, err := publishFiles(context.Background(), root, []File{file}, store)
	if err == nil || !errors.Is(err, ErrPublicationConflict) {
		t.Fatalf("publishFiles() error = %v, want publication conflict", err)
	}
	if store.puts != 0 {
		t.Fatalf("conflicting immutable object was overwritten")
	}
}

func TestPublishFilesRejectsRemoteObjectWithMutableOrIncorrectHTTPMetadata(t *testing.T) {
	root := t.TempDir()
	content := []byte("expected")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	file := File{Path: "maps/styles/" + digest + "/style.json", Digest: digest}
	name := filepath.Join(root, filepath.FromSlash(file.Path))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, content, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, remote := range []PublicationObject{
		{Key: file.Path, Digest: digest, Size: int64(len(content)), ContentType: "text/plain", CacheControl: ImmutableCacheControl},
		{Key: file.Path, Digest: digest, Size: int64(len(content)), ContentType: "application/json", CacheControl: "no-cache"},
	} {
		store := &memoryPublicationStore{objects: map[string]publishedMemoryObject{
			file.Path: {spec: remote},
		}}
		if _, err := publishFiles(context.Background(), root, []File{file}, store); err == nil || !errors.Is(err, ErrPublicationConflict) {
			t.Fatalf("publishFiles() error = %v, want publication conflict for metadata %#v", err, remote)
		}
		if store.puts != 0 {
			t.Fatal("conflicting immutable metadata was overwritten")
		}
	}
}

type publishedMemoryObject struct {
	spec PublicationObject
	body []byte
}

type memoryPublicationStore struct {
	objects map[string]publishedMemoryObject
	puts    int
}

func (s *memoryPublicationStore) Stat(_ context.Context, key string) (PublicationObject, error) {
	object, ok := s.objects[key]
	if !ok {
		return PublicationObject{}, ErrPublicationObjectNotFound
	}
	return object.spec, nil
}

func (s *memoryPublicationStore) Put(_ context.Context, object PublicationObject, body io.Reader) error {
	content, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.puts++
	s.objects[object.Key] = publishedMemoryObject{spec: object, body: content}
	return nil
}
