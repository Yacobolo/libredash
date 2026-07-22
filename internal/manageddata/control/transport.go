package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata/storage"
)

type Protocol string

const (
	ProtocolAlreadyPresent Protocol = "already_present"
	ProtocolTus            Protocol = "tus"
	ProtocolS3Multipart    Protocol = "s3_multipart"
)

type TransportRequest struct {
	UploadID  string
	SHA256    string
	Size      int64
	Paths     []string
	ExpiresAt time.Time
}

type Transport interface {
	Backend() string
	Describe(context.Context, TransportRequest) (TransportDescription, error)
	Abort(context.Context, TransportRequest) error
}

type TransportDescription struct {
	Protocol    Protocol
	Tus         *TusDescription
	S3Multipart *S3MultipartDescription
}

type TusDescription struct {
	Endpoint  string
	UploadID  string
	Offset    int64
	ExpiresAt string
	Metadata  map[string]string
}

type S3MultipartDescription struct {
	CreateEndpoint  string
	MinimumPartSize int64
	MaximumPartSize int64
	MaximumParts    int32
}

type tusTransport struct {
	backend  string
	endpoint string
	engine   storage.ResumableUploadEngine
}

func NewTusTransport(backend, endpoint string, engine storage.ResumableUploadEngine) (Transport, error) {
	backend = strings.TrimSpace(backend)
	endpoint = strings.TrimSpace(endpoint)
	if backend == "" || !safeEndpoint(endpoint) || engine == nil {
		return nil, fmt.Errorf("%w: tus backend, endpoint, and upload engine are required", ErrInvalid)
	}
	return &tusTransport{backend: backend, endpoint: endpoint, engine: engine}, nil
}

func (t *tusTransport) Backend() string { return t.backend }

func (t *tusTransport) Describe(ctx context.Context, request TransportRequest) (TransportDescription, error) {
	if err := validateTransportRequest(request); err != nil {
		return TransportDescription{}, err
	}
	uploadID := transportUploadID(request.UploadID, request.SHA256)
	metadata := map[string]string{
		"sha256":     request.SHA256,
		"size":       strconv.FormatInt(request.Size, 10),
		"session_id": request.UploadID,
	}
	upload, err := t.engine.Resume(ctx, uploadID)
	if errors.Is(err, storage.ErrNotFound) {
		upload, err = t.engine.Create(ctx, storage.CreateUpload{ID: uploadID, Size: request.Size, Metadata: metadata})
	}
	if err != nil {
		return TransportDescription{}, err
	}
	if upload.ID != uploadID || upload.Size != request.Size || upload.Offset < 0 || upload.Offset > request.Size {
		return TransportDescription{}, fmt.Errorf("%w: tus staging state does not match the requested blob", ErrIntegrity)
	}
	for key, value := range metadata {
		if upload.Metadata[key] != value {
			return TransportDescription{}, fmt.Errorf("%w: tus staging metadata does not match the requested blob", ErrIntegrity)
		}
	}
	return TransportDescription{
		Protocol: ProtocolTus,
		Tus: &TusDescription{
			Endpoint: t.endpoint, UploadID: uploadID, Offset: upload.Offset,
			ExpiresAt: request.ExpiresAt.UTC().Format(time.RFC3339Nano), Metadata: metadata,
		},
	}, nil
}

func (t *tusTransport) Abort(ctx context.Context, request TransportRequest) error {
	if err := validateTransportRequest(request); err != nil {
		return err
	}
	return t.engine.Abort(ctx, transportUploadID(request.UploadID, request.SHA256))
}

type s3MultipartTransport struct {
	backend    string
	capability S3MultipartDescription
}

func NewS3MultipartTransport(backend string, capability S3MultipartDescription) (Transport, error) {
	backend = strings.TrimSpace(backend)
	capability.CreateEndpoint = strings.TrimSpace(capability.CreateEndpoint)
	if backend == "" || !safeEndpoint(capability.CreateEndpoint) || capability.MinimumPartSize <= 0 ||
		capability.MaximumPartSize < capability.MinimumPartSize || capability.MaximumParts < 1 || capability.MaximumParts > 10_000 {
		return nil, fmt.Errorf("%w: invalid S3 multipart transport capability", ErrInvalid)
	}
	return &s3MultipartTransport{backend: backend, capability: capability}, nil
}

func (t *s3MultipartTransport) Backend() string { return t.backend }

func (t *s3MultipartTransport) Describe(_ context.Context, request TransportRequest) (TransportDescription, error) {
	if err := validateTransportRequest(request); err != nil {
		return TransportDescription{}, err
	}
	capability := t.capability
	return TransportDescription{Protocol: ProtocolS3Multipart, S3Multipart: &capability}, nil
}

func (t *s3MultipartTransport) Abort(context.Context, TransportRequest) error { return nil }

func validateTransportRequest(request TransportRequest) error {
	if strings.TrimSpace(request.UploadID) == "" {
		return fmt.Errorf("%w: upload id is required", ErrInvalid)
	}
	if err := storage.ValidateBlob(storage.Blob{SHA256: request.SHA256, Size: request.Size}); err != nil {
		return fmt.Errorf("%w: invalid transport blob", ErrInvalid)
	}
	return nil
}

func transportUploadID(sessionID, digest string) string {
	sum := sha256.Sum256([]byte(sessionID + "\x00" + digest))
	return "tus_" + hex.EncodeToString(sum[:])
}

func safeEndpoint(value string) bool {
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.IsAbs() || strings.HasPrefix(parsed.Path, "/")
}
