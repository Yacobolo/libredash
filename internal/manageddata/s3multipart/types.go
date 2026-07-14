// Package s3multipart coordinates durable S3 multipart uploads for managed data.
package s3multipart

import (
	"context"
	"time"

	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/Yacobolo/libredash/internal/manageddata/storage"
	storages3 "github.com/Yacobolo/libredash/internal/manageddata/storage/s3"
)

const (
	MinimumPartSize   int64 = 5 * 1024 * 1024
	MaximumPartSize   int64 = 5 * 1024 * 1024 * 1024
	MaximumParts      int32 = 10_000
	MaximumObjectSize int64 = 5 * 1024 * 1024 * 1024 * 1024
)

const defaultSignExpiry = 15 * time.Minute

type Repository interface {
	CollectionByProjectConnection(context.Context, string, string) (manageddata.Collection, error)
	UploadSessionByID(context.Context, string) (manageddata.UploadSession, error)
	CreateS3MultipartUpload(context.Context, manageddata.CreateS3MultipartUploadInput) (manageddata.S3MultipartUpload, error)
	S3MultipartUploadByID(context.Context, string) (manageddata.S3MultipartUpload, error)
	InitializeS3MultipartUpload(context.Context, manageddata.InitializeS3MultipartUploadInput) (manageddata.S3MultipartUpload, error)
	ReserveS3MultipartPart(context.Context, manageddata.S3MultipartPart) (manageddata.S3MultipartPart, error)
	ListS3MultipartParts(context.Context, string) ([]manageddata.S3MultipartPart, error)
	BeginS3MultipartCompletion(context.Context, manageddata.BeginS3MultipartCompletionInput) (manageddata.S3MultipartCompletion, error)
	FinishS3MultipartCompletion(context.Context, string) (manageddata.S3MultipartUpload, error)
	BeginS3MultipartAbort(context.Context, manageddata.BeginS3MultipartAbortInput) (manageddata.S3MultipartAbort, error)
	FinishS3MultipartAbort(context.Context, string) (manageddata.S3MultipartUpload, error)
	FailS3MultipartUpload(context.Context, string, string) (manageddata.S3MultipartUpload, error)
	ListRecoverableS3MultipartUploads(context.Context, time.Time, int64) ([]manageddata.S3MultipartUpload, error)
}

var _ Repository = (manageddata.Repository)(nil)

// MultipartStore is the SDK-free subset of the S3 Store used by the coordinator.
type MultipartStore interface {
	CreateMultipart(context.Context, storage.Blob) (storages3.MultipartUpload, error)
	SignPart(context.Context, storages3.MultipartUpload, storages3.PartRequest) (storages3.SignedPart, error)
	CompleteMultipart(context.Context, storages3.MultipartUpload, []storages3.CompletedPart) (storage.Blob, error)
	AbortMultipart(context.Context, storages3.MultipartUpload) error
}

var _ MultipartStore = (*storages3.Store)(nil)

type Config struct {
	Backend    string
	SignExpiry time.Duration
	Clock      func() time.Time
}

type CreateRequest struct {
	Project         string
	Connection      string
	UploadSessionID string
	Path            string
	IdempotencyKey  string
}

type SignPartRequest struct {
	Project           string
	Connection        string
	UploadSessionID   string
	MultipartUploadID string
	PartNumber        int32
	Size              int64
	SHA256            string
}

type CompletedPart struct {
	PartNumber int32
	ETag       string
	SHA256     string
}

type CompleteRequest struct {
	Project           string
	Connection        string
	UploadSessionID   string
	MultipartUploadID string
	IdempotencyKey    string
	Parts             []CompletedPart
}

type AbortRequest struct {
	Project           string
	Connection        string
	UploadSessionID   string
	MultipartUploadID string
	IdempotencyKey    string
}

type Status string

const (
	StatusOpen      Status = "open"
	StatusCompleted Status = "completed"
	StatusAborted   Status = "aborted"
)

type UploadResult struct {
	ID              string
	UploadSessionID string
	File            manageddata.File
	Status          Status
	Existing        bool
	CreatedAt       string
	ExpiresAt       string
}

type Header struct {
	Name  string
	Value string
}

type SignedPartResult struct {
	PartNumber int32
	URL        string
	Headers    []Header
	ExpiresAt  string
}

type RecoveryResult struct {
	Aborted int64
}
