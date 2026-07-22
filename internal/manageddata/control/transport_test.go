package control_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata/control"
	"github.com/Yacobolo/leapview/internal/manageddata/storage"
)

func TestTusTransportDescribesResumableStateAndAborts(t *testing.T) {
	engine := &fakeUploadEngine{uploads: make(map[string]storage.Upload)}
	transport, err := control.NewTusTransport("local", "/upload-protocols/tus", engine)
	if err != nil {
		t.Fatal(err)
	}
	request := control.TransportRequest{
		UploadID: "upload-1", SHA256: digestA, Size: 9, Paths: []string{"orders.csv"},
		ExpiresAt: time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC),
	}
	first, err := transport.Describe(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Protocol != control.ProtocolTus || first.Tus == nil || first.Tus.Offset != 0 || first.Tus.Metadata["sha256"] != digestA {
		t.Fatalf("first description = %#v", first)
	}
	staged := engine.uploads[first.Tus.UploadID]
	staged.Offset = 4
	engine.uploads[first.Tus.UploadID] = staged
	resumed, err := transport.Describe(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Tus.Offset != 4 || engine.createCalls != 1 {
		t.Fatalf("resumed description = %#v, create calls = %d", resumed, engine.createCalls)
	}
	if err := transport.Abort(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if _, ok := engine.uploads[first.Tus.UploadID]; ok {
		t.Fatal("tus staging upload still exists after abort")
	}
}

func TestS3MultipartTransportExposesCapabilityWithoutSDKTypes(t *testing.T) {
	transport, err := control.NewS3MultipartTransport("s3", control.S3MultipartDescription{
		CreateEndpoint:  "/api/v1/projects/p/connections/c/upload-sessions/u/s3-multipart-uploads",
		MinimumPartSize: 5 << 20,
		MaximumPartSize: 5 << 30,
		MaximumParts:    10_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	description, err := transport.Describe(t.Context(), control.TransportRequest{UploadID: "upload-1", SHA256: digestA, Size: 9})
	if err != nil {
		t.Fatal(err)
	}
	if description.Protocol != control.ProtocolS3Multipart || description.S3Multipart == nil || description.Tus != nil {
		t.Fatalf("description = %#v", description)
	}
	if err := transport.Abort(t.Context(), control.TransportRequest{}); err != nil {
		t.Fatalf("S3 capability abort = %v", err)
	}
}

func TestTransportDescriptionsRejectEndpointsThatCouldExposeSecrets(t *testing.T) {
	engine := &fakeUploadEngine{uploads: make(map[string]storage.Upload)}
	if _, err := control.NewTusTransport("local", "https://access:secret@example.test/tus", engine); !errors.Is(err, control.ErrInvalid) {
		t.Fatalf("credentialed tus endpoint error = %v, want ErrInvalid", err)
	}
	if _, err := control.NewS3MultipartTransport("s3", control.S3MultipartDescription{
		CreateEndpoint: "/multipart?token=secret", MinimumPartSize: 1, MaximumPartSize: 1, MaximumParts: 1,
	}); !errors.Is(err, control.ErrInvalid) {
		t.Fatalf("tokenized multipart endpoint error = %v, want ErrInvalid", err)
	}
}

type fakeUploadEngine struct {
	uploads     map[string]storage.Upload
	createCalls int
}

func (e *fakeUploadEngine) Create(_ context.Context, request storage.CreateUpload) (storage.Upload, error) {
	e.createCalls++
	if upload, ok := e.uploads[request.ID]; ok {
		return upload, nil
	}
	upload := storage.Upload{ID: request.ID, Size: request.Size, Metadata: request.Metadata}
	e.uploads[request.ID] = upload
	return upload, nil
}

func (e *fakeUploadEngine) Resume(_ context.Context, uploadID string) (storage.Upload, error) {
	upload, ok := e.uploads[uploadID]
	if !ok {
		return storage.Upload{}, storage.ErrNotFound
	}
	return upload, nil
}

func (e *fakeUploadEngine) WriteChunk(context.Context, string, int64, io.Reader) (storage.Upload, error) {
	panic("unexpected WriteChunk")
}

func (e *fakeUploadEngine) Finalize(context.Context, string, storage.Blob) (storage.Blob, error) {
	panic("unexpected Finalize")
}

func (e *fakeUploadEngine) Abort(_ context.Context, uploadID string) error {
	if _, ok := e.uploads[uploadID]; !ok {
		return errors.New("not found")
	}
	delete(e.uploads, uploadID)
	return nil
}
