package s3multipart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata"
	"github.com/Yacobolo/leapview/internal/manageddata/control"
	"github.com/Yacobolo/leapview/internal/manageddata/sqlite"
	"github.com/Yacobolo/leapview/internal/manageddata/storage"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

const (
	minPart = int64(5 * 1024 * 1024)
	nowText = "2026-07-14T10:00:00Z"
)

func TestCoordinatorCreateSignCompleteIsStrictAndRetrySafe(t *testing.T) {
	ctx, repo, session := coordinatorFixture(t, []manageddata.File{
		{Path: "large.csv", Size: minPart + 3, SHA256: strings.Repeat("a", 64)},
		{Path: "other.csv", Size: 1, SHA256: strings.Repeat("b", 64)},
	})
	provider := &fakeMultipartStore{}
	service := newTestService(t, repo, provider)
	create := CreateRequest{Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID, Path: "large.csv", IdempotencyKey: "create-1"}

	upload, err := service.Create(ctx, create)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if upload.Status != StatusOpen || upload.File.Path != "large.csv" || upload.Existing {
		t.Fatalf("upload = %#v", upload)
	}
	retry, err := service.Create(ctx, create)
	if err != nil || retry != upload || provider.createCalls != 1 {
		t.Fatalf("create retry = %#v, calls = %d, err = %v", retry, provider.createCalls, err)
	}
	create.Path = "other.csv"
	if _, err := service.Create(ctx, create); !errors.Is(err, control.ErrConflict) {
		t.Fatalf("conflicting create error = %v, want conflict", err)
	}

	if _, err := service.SignPart(ctx, SignPartRequest{
		Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID,
		MultipartUploadID: upload.ID, PartNumber: 1, Size: minPart, SHA256: strings.Repeat("c", 64),
	}); err != nil {
		t.Fatalf("sign first part: %v", err)
	}
	signed, err := service.SignPart(ctx, SignPartRequest{
		Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID,
		MultipartUploadID: upload.ID, PartNumber: 2, Size: 3,
	})
	if err != nil {
		t.Fatalf("sign final part: %v", err)
	}
	if signed.UploadSessionID != session.ID || signed.MultipartUploadID != upload.ID || signed.URL == "" || signed.ExpiresAt != "2026-07-14T10:15:00Z" || len(signed.Headers) != 1 {
		t.Fatalf("signed part = %#v", signed)
	}
	if _, err := service.SignPart(ctx, SignPartRequest{
		Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID,
		MultipartUploadID: upload.ID, PartNumber: 2, Size: 4,
	}); !errors.Is(err, control.ErrConflict) {
		t.Fatalf("conflicting sign error = %v, want conflict", err)
	}

	complete := CompleteRequest{
		Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID,
		MultipartUploadID: upload.ID, IdempotencyKey: "complete-1",
		Parts: []CompletedPart{{PartNumber: 2, ETag: "etag-2"}, {PartNumber: 1, ETag: "etag-1", SHA256: strings.Repeat("c", 64)}},
	}
	completed, err := service.Complete(ctx, complete)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Status != StatusCompleted || provider.completeCalls != 1 || provider.completedParts[0].Number != 1 {
		t.Fatalf("completed = %#v, provider = %#v", completed, provider)
	}
	retry, err = service.Complete(ctx, complete)
	if err != nil || retry != completed || provider.completeCalls != 1 {
		t.Fatalf("complete retry = %#v, calls = %d, err = %v", retry, provider.completeCalls, err)
	}
}

func TestCoordinatorRejectsWrongScopeManifestAndMultipartShape(t *testing.T) {
	ctx, repo, session := coordinatorFixture(t, []manageddata.File{{Path: "data.csv", Size: minPart + 1, SHA256: strings.Repeat("a", 64)}})
	service := newTestService(t, repo, &fakeMultipartStore{})

	for _, request := range []CreateRequest{
		{Project: "project-b", Connection: "warehouse", UploadSessionID: session.ID, Path: "data.csv", IdempotencyKey: "key"},
		{Project: " project-a", Connection: "warehouse", UploadSessionID: session.ID, Path: "data.csv", IdempotencyKey: "key"},
		{Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID, Path: "missing.csv", IdempotencyKey: "key"},
	} {
		if _, err := service.Create(ctx, request); err == nil {
			t.Fatalf("Create(%#v) unexpectedly succeeded", request)
		}
	}

	upload, err := service.Create(ctx, CreateRequest{Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID, Path: "data.csv", IdempotencyKey: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignPart(ctx, SignPartRequest{Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID, MultipartUploadID: upload.ID, PartNumber: 1, Size: minPart + 2}); !errors.Is(err, control.ErrInvalid) {
		t.Fatalf("oversized part error = %v, want invalid", err)
	}
	if _, err := service.SignPart(ctx, SignPartRequest{Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID, MultipartUploadID: upload.ID, PartNumber: 1, Size: 1}); err != nil {
		t.Fatal(err)
	}
	_, err = service.Complete(ctx, CompleteRequest{
		Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID,
		MultipartUploadID: upload.ID, IdempotencyKey: "complete", Parts: []CompletedPart{{PartNumber: 1, ETag: "etag"}},
	})
	if !errors.Is(err, control.ErrInvalid) {
		t.Fatalf("incomplete shape error = %v, want invalid", err)
	}
}

func TestCoordinatorSanitizesIntegrityFailureAndRecoversProviderUpload(t *testing.T) {
	ctx, repo, session := coordinatorFixture(t, []manageddata.File{{Path: "data.csv", Size: 1, SHA256: strings.Repeat("a", 64)}})
	provider := &fakeMultipartStore{completeErr: fmt.Errorf("raw backend secret: %w", storage.ErrIntegrity)}
	service := newTestService(t, repo, provider)
	upload, err := service.Create(ctx, CreateRequest{Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID, Path: "data.csv", IdempotencyKey: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignPart(ctx, SignPartRequest{Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID, MultipartUploadID: upload.ID, PartNumber: 1, Size: 1}); err != nil {
		t.Fatal(err)
	}
	_, err = service.Complete(ctx, CompleteRequest{
		Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID,
		MultipartUploadID: upload.ID, IdempotencyKey: "complete", Parts: []CompletedPart{{PartNumber: 1, ETag: "etag"}},
	})
	if !errors.Is(err, control.ErrIntegrity) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("completion error = %v", err)
	}
	stored, err := repo.S3MultipartUploadByID(ctx, upload.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != manageddata.S3MultipartStatusFailed || stored.Error != integrityTerminalError || strings.Contains(stored.Error, "secret") {
		t.Fatalf("stored failure = %#v", stored)
	}

	recovery, err := service.RecoverOrphaned(ctx, time.Now().Add(time.Hour), 10)
	if err != nil || recovery.Aborted != 1 || provider.abortCalls != 1 {
		t.Fatalf("recovery = %#v, abort calls = %d, err = %v", recovery, provider.abortCalls, err)
	}
}

func TestCoordinatorRecoversOpenMultipartAfterParentAbort(t *testing.T) {
	ctx, repo, session := coordinatorFixture(t, []manageddata.File{{Path: "data.csv", Size: 1, SHA256: strings.Repeat("a", 64)}})
	provider := &fakeMultipartStore{}
	service := newTestService(t, repo, provider)
	upload, err := service.Create(ctx, CreateRequest{Project: "project-a", Connection: "warehouse", UploadSessionID: session.ID, Path: "data.csv", IdempotencyKey: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AbortUploadSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}

	recovery, err := service.RecoverOrphaned(ctx, time.Now().Add(time.Hour), 10)
	if err != nil || recovery.Aborted != 1 || provider.abortCalls != 1 {
		t.Fatalf("recovery = %#v, abort calls = %d, err = %v", recovery, provider.abortCalls, err)
	}
	stored, err := repo.S3MultipartUploadByID(ctx, upload.ID)
	if err != nil || stored.Status != manageddata.S3MultipartStatusAborted {
		t.Fatalf("stored upload = %#v, err = %v", stored, err)
	}
}

type fakeMultipartStore struct {
	createCalls    int
	completeCalls  int
	abortCalls     int
	completeErr    error
	completedParts []storage.CompletedMultipartPart
}

func (f *fakeMultipartStore) CreateMultipart(_ context.Context, blob storage.Blob) (storage.MultipartUpload, error) {
	f.createCalls++
	return storage.MultipartUpload{UploadID: "provider-1", SHA256: blob.SHA256, Size: blob.Size, Key: "blobs/" + blob.SHA256}, nil
}

func (f *fakeMultipartStore) SignPart(_ context.Context, _ storage.MultipartUpload, part storage.MultipartPartRequest) (storage.SignedMultipartPart, error) {
	return storage.SignedMultipartPart{Number: part.Number, URL: "https://s3.example/upload?signature=transient", Headers: http.Header{"X-Checksum": []string{"value"}}}, nil
}

func (f *fakeMultipartStore) CompleteMultipart(_ context.Context, upload storage.MultipartUpload, parts []storage.CompletedMultipartPart) (storage.Blob, error) {
	f.completeCalls++
	f.completedParts = append([]storage.CompletedMultipartPart(nil), parts...)
	if f.completeErr != nil {
		return storage.Blob{}, f.completeErr
	}
	return storage.Blob{SHA256: upload.SHA256, Size: upload.Size, URI: "s3://bucket/" + upload.Key}, nil
}

func (f *fakeMultipartStore) AbortMultipart(context.Context, storage.MultipartUpload) error {
	f.abortCalls++
	return nil
}

func coordinatorFixture(t *testing.T, files []manageddata.File) (context.Context, *sqlite.Repository, manageddata.UploadSession) {
	t.Helper()
	ctx := context.Background()
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "leapview.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, database, "../../platform/migrations"); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(database)
	collection, err := repo.CreateCollection(ctx, manageddata.CreateCollectionInput{ID: "collection-a", ProjectID: "project-a", ConnectionName: "warehouse", Name: "Warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := repo.CreateUploadSession(ctx, manageddata.CreateUploadSessionInput{
		ID: "upload-a", CollectionID: collection.ID, Manifest: manageddata.Manifest{Files: files}, StorageBackend: "s3",
		StagingPrefix: "uploads/upload-a", ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, repo, session
}

func newTestService(t *testing.T, repo *sqlite.Repository, provider MultipartStore) *Service {
	t.Helper()
	now, _ := time.Parse(time.RFC3339, nowText)
	service, err := New(repo, provider, Config{Backend: "s3", Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
