// Package http exposes managed-data control operations over the generated API.
package http

import (
	"context"
	"errors"
	stdhttp "net/http"

	"github.com/Yacobolo/leapview/internal/manageddata/control"
	"github.com/Yacobolo/leapview/internal/manageddata/s3multipart"
)

var (
	ErrInvalid  = control.ErrInvalid
	ErrNotFound = control.ErrNotFound
	ErrConflict = control.ErrConflict
	ErrTooLarge = errors.New("managed-data request is too large")
	ErrBackend  = control.ErrBackend
)

type Principal struct {
	ID string
}

type RevisionMetadata = control.RevisionMetadata
type Repository = control.MetadataRepository

type UploadCoordinator interface {
	BeginUpload(context.Context, control.BeginUploadRequest) (control.UploadResult, error)
	RecoverUpload(context.Context, control.UploadRequest) (control.UploadResult, error)
	FinalizeUpload(context.Context, control.UploadRequest) (control.FinalizeResult, error)
	BeginFinalizeUpload(context.Context, control.UploadRequest) (control.UploadResult, error)
	CompleteFinalizeUpload(context.Context, control.UploadRequest) (control.FinalizeResult, error)
	AbortUpload(context.Context, control.UploadRequest) (control.UploadResult, error)
}

type Options struct {
	Repository          Repository
	Uploads             UploadCoordinator
	Multipart           s3multipart.Coordinator
	CurrentPrincipal    func(*stdhttp.Request) (Principal, bool)
	MaxJSONBodyBytes    int64
	Environment         string
	EnqueueFinalize     func(context.Context, control.UploadRequest) error
	RecordUploadCreated func(context.Context, control.UploadResult) error
}

type Handler struct {
	options Options
}

func NewHandler(options Options) *Handler {
	if options.MaxJSONBodyBytes <= 0 {
		options.MaxJSONBodyBytes = 16 << 20
	}
	if options.Environment == "" {
		options.Environment = "dev"
	}
	return &Handler{options: options}
}
