package mapasset

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

const ImmutableCacheControl = "public, max-age=31536000, immutable"

var (
	ErrPublicationObjectNotFound = errors.New("map asset publication object not found")
	ErrPublicationConflict       = errors.New("map asset publication conflict")
)

// PublicationObject is the immutable metadata contract shared by filesystem
// verification and managed object-store publishers.
type PublicationObject struct {
	Key          string
	Digest       string
	Size         int64
	ContentType  string
	CacheControl string
}

type PublicationStore interface {
	Stat(context.Context, string) (PublicationObject, error)
	Put(context.Context, PublicationObject, io.Reader) error
}

type PublicationSummary struct {
	Uploaded int
	Reused   int
	Bytes    int64
}

// PublishInstalled publishes the complete compiled package inventory. Local
// and remote objects are both verified; a conflicting immutable destination
// is never overwritten.
func PublishInstalled(ctx context.Context, root string, store PublicationStore) (PublicationSummary, error) {
	if store == nil {
		return PublicationSummary{}, fmt.Errorf("map asset publication store is required")
	}
	if err := VerifyInstalled(root); err != nil {
		return PublicationSummary{}, err
	}
	return publishFiles(ctx, root, ExpectedFiles(), store)
}

func publishFiles(ctx context.Context, root string, files []File, store PublicationStore) (PublicationSummary, error) {
	if store == nil {
		return PublicationSummary{}, fmt.Errorf("map asset publication store is required")
	}
	var summary PublicationSummary
	for _, expected := range files {
		if err := ctx.Err(); err != nil {
			return PublicationSummary{}, err
		}
		name := filepath.Join(root, filepath.FromSlash(expected.Path))
		file, err := os.Open(name)
		if err != nil {
			return PublicationSummary{}, fmt.Errorf("open map asset %s for publication: %w", expected.Path, err)
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return PublicationSummary{}, fmt.Errorf("stat map asset %s for publication: %w", expected.Path, err)
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			file.Close()
			return PublicationSummary{}, fmt.Errorf("hash map asset %s for publication: %w", expected.Path, err)
		}
		actual := fmt.Sprintf("%x", hash.Sum(nil))
		if actual != expected.Digest {
			file.Close()
			return PublicationSummary{}, fmt.Errorf("map asset %s digest mismatch: got %s", expected.Path, actual)
		}
		object := PublicationObject{
			Key: expected.Path, Digest: expected.Digest, Size: info.Size(),
			ContentType: ContentType(expected.Path), CacheControl: ImmutableCacheControl,
		}
		remote, statErr := store.Stat(ctx, object.Key)
		switch {
		case statErr == nil:
			file.Close()
			if !publicationObjectsMatch(remote, object) {
				return PublicationSummary{}, publicationConflict(object, remote)
			}
			summary.Reused++
			summary.Bytes += object.Size
			continue
		case !errors.Is(statErr, ErrPublicationObjectNotFound):
			file.Close()
			return PublicationSummary{}, fmt.Errorf("stat published map asset %q: %w", object.Key, statErr)
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			return PublicationSummary{}, fmt.Errorf("rewind map asset %s: %w", expected.Path, err)
		}
		putErr := store.Put(ctx, object, file)
		closeErr := file.Close()
		if putErr != nil {
			return PublicationSummary{}, fmt.Errorf("publish map asset %q: %w", object.Key, putErr)
		}
		if closeErr != nil {
			return PublicationSummary{}, fmt.Errorf("close map asset %q: %w", object.Key, closeErr)
		}
		remote, err = store.Stat(ctx, object.Key)
		if err != nil {
			return PublicationSummary{}, fmt.Errorf("verify published map asset %q: %w", object.Key, err)
		}
		if !publicationObjectsMatch(remote, object) {
			return PublicationSummary{}, publicationConflict(object, remote)
		}
		summary.Uploaded++
		summary.Bytes += object.Size
	}
	return summary, nil
}

func publicationObjectsMatch(actual, expected PublicationObject) bool {
	return actual.Digest == expected.Digest && actual.Size == expected.Size && actual.ContentType == expected.ContentType && actual.CacheControl == expected.CacheControl
}

func publicationConflict(expected, actual PublicationObject) error {
	return fmt.Errorf("%w: object %q has digest %q, size %d, content type %q, and cache control %q; want %q, %d, %q, and %q", ErrPublicationConflict, expected.Key, actual.Digest, actual.Size, actual.ContentType, actual.CacheControl, expected.Digest, expected.Size, expected.ContentType, expected.CacheControl)
}

// ContentType returns the canonical HTTP representation metadata for an
// immutable package path. Serving and object publication share this mapping.
func ContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pmtiles":
		return "application/vnd.pmtiles"
	case ".pbf":
		return "application/x-protobuf"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	default:
		if value := mime.TypeByExtension(filepath.Ext(path)); value != "" {
			return value
		}
		return "application/octet-stream"
	}
}
