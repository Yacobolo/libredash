package storage

import (
	"context"
	"io"
)

// Upload describes resumable staging state without exposing an upload engine.
type Upload struct {
	ID       string
	Size     int64
	Offset   int64
	Metadata map[string]string
}

type CreateUpload struct {
	ID       string
	Size     int64
	Metadata map[string]string
}

// ResumableUploadEngine stages content before verified blob finalization.
type ResumableUploadEngine interface {
	Create(ctx context.Context, request CreateUpload) (Upload, error)
	Resume(ctx context.Context, uploadID string) (Upload, error)
	WriteChunk(ctx context.Context, uploadID string, offset int64, content io.Reader) (Upload, error)
	Finalize(ctx context.Context, uploadID string, expected Blob) (Blob, error)
	Abort(ctx context.Context, uploadID string) error
}
