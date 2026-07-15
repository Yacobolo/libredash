// Package tus adapts tusd's filesystem engine to managed-data upload contracts.
package tus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Yacobolo/libredash/internal/manageddata/storage"
	"github.com/tus/tusd/v2/pkg/filelocker"
	"github.com/tus/tusd/v2/pkg/filestore"
	"github.com/tus/tusd/v2/pkg/handler"
)

const digestMetadataKey = "sha256"

type Engine struct {
	store  filestore.FileStore
	locker filelocker.FileLocker
	blobs  storage.BlobStore
}

type HTTPConfig struct {
	BasePath string
	MaxSize  int64
}

func New(root string, blobs storage.BlobStore) (*Engine, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: tus upload root is required", storage.ErrInvalid)
	}
	if blobs == nil {
		return nil, fmt.Errorf("%w: blob store is required", storage.ErrInvalid)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("initialize tus upload root: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, fmt.Errorf("set tus upload root permissions: %w", err)
	}
	store := filestore.New(root)
	store.DirModePerm = 0o700
	store.FileModePerm = 0o600
	return &Engine{store: store, locker: filelocker.New(root), blobs: blobs}, nil
}

func (e *Engine) Create(ctx context.Context, request storage.CreateUpload) (storage.Upload, error) {
	if request.Size < 0 {
		return storage.Upload{}, fmt.Errorf("%w: upload size must not be negative", storage.ErrInvalid)
	}
	id := request.ID
	if id == "" {
		var err error
		id, err = newUploadID()
		if err != nil {
			return storage.Upload{}, fmt.Errorf("generate tus upload ID: %w", err)
		}
	}
	if err := validateUploadID(id); err != nil {
		return storage.Upload{}, err
	}
	var result storage.Upload
	err := e.withLock(ctx, id, func() error {
		existing, err := e.store.GetUpload(ctx, id)
		if err == nil {
			info, err := existing.GetInfo(ctx)
			if err != nil {
				return err
			}
			if info.Size != request.Size || !equalMetadata(info.MetaData, request.Metadata) {
				return fmt.Errorf("%w: upload ID already exists with different properties", storage.ErrInvalid)
			}
			result = uploadFromInfo(info)
			return nil
		}
		if !errors.Is(err, handler.ErrNotFound) {
			return err
		}
		upload, err := e.store.NewUpload(ctx, handler.FileInfo{ID: id, Size: request.Size, MetaData: cloneMetadata(request.Metadata)})
		if err != nil {
			return err
		}
		info, err := upload.GetInfo(ctx)
		if err != nil {
			return err
		}
		result = uploadFromInfo(info)
		return nil
	})
	if err != nil {
		return storage.Upload{}, mapError("create tus upload", err)
	}
	return result, nil
}

func (e *Engine) Resume(ctx context.Context, uploadID string) (storage.Upload, error) {
	if err := validateUploadID(uploadID); err != nil {
		return storage.Upload{}, err
	}
	var result storage.Upload
	err := e.withLock(ctx, uploadID, func() error {
		upload, err := e.store.GetUpload(ctx, uploadID)
		if err != nil {
			return err
		}
		info, err := upload.GetInfo(ctx)
		if err != nil {
			return err
		}
		result = uploadFromInfo(info)
		return nil
	})
	if err != nil {
		return storage.Upload{}, mapError("resume tus upload", err)
	}
	return result, nil
}

func (e *Engine) WriteChunk(ctx context.Context, uploadID string, offset int64, content io.Reader) (storage.Upload, error) {
	if err := validateUploadID(uploadID); err != nil {
		return storage.Upload{}, err
	}
	if offset < 0 || content == nil {
		return storage.Upload{}, fmt.Errorf("%w: upload offset and content are required", storage.ErrInvalid)
	}
	var result storage.Upload
	err := e.withLock(ctx, uploadID, func() error {
		upload, err := e.store.GetUpload(ctx, uploadID)
		if err != nil {
			return err
		}
		info, err := upload.GetInfo(ctx)
		if err != nil {
			return err
		}
		if info.Offset != offset {
			return fmt.Errorf("%w: expected offset %d, received %d", storage.ErrOffset, info.Offset, offset)
		}
		remaining := info.Size - info.Offset
		limited := &io.LimitedReader{R: &contextReader{ctx: ctx, reader: content}, N: remaining}
		written, err := upload.WriteChunk(ctx, offset, limited)
		if err != nil {
			return err
		}
		if limited.N == 0 {
			var extra [1]byte
			if count, readErr := content.Read(extra[:]); count > 0 || readErr != nil && !errors.Is(readErr, io.EOF) {
				return fmt.Errorf("%w: upload chunk exceeds declared size", storage.ErrIntegrity)
			}
		}
		info.Offset += written
		result = uploadFromInfo(info)
		return nil
	})
	if err != nil {
		return storage.Upload{}, mapError("write tus upload chunk", err)
	}
	return result, nil
}

func (e *Engine) Finalize(ctx context.Context, uploadID string, expected storage.Blob) (storage.Blob, error) {
	if err := validateUploadID(uploadID); err != nil {
		return storage.Blob{}, err
	}
	if err := storage.ValidateBlob(expected); err != nil {
		return storage.Blob{}, err
	}
	var result storage.Blob
	err := e.withLock(ctx, uploadID, func() error {
		var err error
		result, err = e.finalizeUnlocked(ctx, uploadID, expected)
		return err
	})
	if err != nil {
		return storage.Blob{}, mapError("finalize tus upload", err)
	}
	return result, nil
}

func (e *Engine) Abort(ctx context.Context, uploadID string) error {
	if err := validateUploadID(uploadID); err != nil {
		return err
	}
	if _, err := e.store.GetUpload(ctx, uploadID); errors.Is(err, handler.ErrNotFound) {
		return nil
	} else if err != nil {
		return mapError("load tus upload for abort", err)
	}
	err := e.withLock(ctx, uploadID, func() error {
		upload, err := e.store.GetUpload(ctx, uploadID)
		if errors.Is(err, handler.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return e.store.AsTerminatableUpload(upload).Terminate(ctx)
	})
	return mapError("abort tus upload", err)
}

// HTTPHandler composes the tus v1 protocol over this engine without exposing tusd types.
func (e *Engine) HTTPHandler(config HTTPConfig) (http.Handler, error) {
	composer := handler.NewStoreComposer()
	e.store.UseIn(composer)
	e.locker.UseIn(composer)
	tusHandler, err := handler.NewHandler(handler.Config{
		StoreComposer:        composer,
		BasePath:             config.BasePath,
		MaxSize:              config.MaxSize,
		DisableDownload:      true,
		DisableConcatenation: true,
		PreUploadCreateCallback: func(event handler.HookEvent) (handler.HTTPResponse, handler.FileInfoChanges, error) {
			digest := event.Upload.MetaData[digestMetadataKey]
			if err := storage.ValidateSHA256(digest); err != nil {
				return handler.HTTPResponse{}, handler.FileInfoChanges{}, handler.NewError("ERR_INVALID_SHA256", "valid sha256 upload metadata is required", http.StatusBadRequest)
			}
			return handler.HTTPResponse{}, handler.FileInfoChanges{}, nil
		},
		PreFinishResponseCallback: func(event handler.HookEvent) (handler.HTTPResponse, error) {
			expected := storage.Blob{SHA256: event.Upload.MetaData[digestMetadataKey], Size: event.Upload.Size}
			if _, err := e.finalizeUnlocked(event.Context, event.Upload.ID, expected); err != nil {
				return handler.HTTPResponse{}, handler.NewError("ERR_BLOB_FINALIZE", "uploaded content failed managed-data verification", http.StatusUnprocessableEntity)
			}
			return handler.HTTPResponse{}, nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("compose tus HTTP handler: %w", err)
	}
	routePrefix := strings.TrimSuffix(config.BasePath, "/")
	if parsed, parseErr := url.Parse(config.BasePath); parseErr == nil && parsed.IsAbs() {
		routePrefix = strings.TrimSuffix(parsed.Path, "/")
	}
	if routePrefix == "" {
		return tusHandler, nil
	}
	return http.StripPrefix(routePrefix, tusHandler), nil
}

func (e *Engine) finalizeUnlocked(ctx context.Context, uploadID string, expected storage.Blob) (storage.Blob, error) {
	upload, err := e.store.GetUpload(ctx, uploadID)
	if err != nil {
		return storage.Blob{}, err
	}
	info, err := upload.GetInfo(ctx)
	if err != nil {
		return storage.Blob{}, err
	}
	if info.Offset != info.Size || info.Size != expected.Size {
		return storage.Blob{}, fmt.Errorf("%w: upload is incomplete", storage.ErrIntegrity)
	}
	reader, err := upload.GetReader(ctx)
	if err != nil {
		return storage.Blob{}, err
	}
	blob, putErr := e.blobs.Put(ctx, expected, reader)
	closeErr := reader.Close()
	if putErr != nil {
		return storage.Blob{}, putErr
	}
	if closeErr != nil {
		return storage.Blob{}, closeErr
	}
	if err := upload.FinishUpload(ctx); err != nil {
		return storage.Blob{}, err
	}
	return blob, nil
}

func (e *Engine) withLock(ctx context.Context, uploadID string, operation func() error) error {
	lock, err := e.locker.NewLock(uploadID)
	if err != nil {
		return err
	}
	if err := lock.Lock(ctx, func() {}); err != nil {
		return err
	}
	operationErr := operation()
	unlockErr := lock.Unlock()
	if operationErr != nil {
		return operationErr
	}
	return unlockErr
}

func validateUploadID(value string) error {
	if value == "" || len(value) > 200 || value == "." || value == ".." {
		return fmt.Errorf("%w: upload ID is invalid", storage.ErrInvalid)
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("._-", character) {
			continue
		}
		return fmt.Errorf("%w: upload ID is invalid", storage.ErrInvalid)
	}
	return nil
}

func newUploadID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func uploadFromInfo(info handler.FileInfo) storage.Upload {
	return storage.Upload{ID: info.ID, Size: info.Size, Offset: info.Offset, Metadata: cloneMetadata(info.MetaData)}
}

func cloneMetadata(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func equalMetadata(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func mapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, handler.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", storage.ErrNotFound, operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
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

var _ storage.ResumableUploadEngine = (*Engine)(nil)
