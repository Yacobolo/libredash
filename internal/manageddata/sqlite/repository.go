package sqlite

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/manageddata"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
)

type Repository struct {
	db *sql.DB
	q  *platformdb.Queries
}

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db, q: platformdb.New(db)} }

func (r *Repository) CreateCollection(ctx context.Context, input manageddata.CreateCollectionInput) (manageddata.Collection, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.ConnectionName = strings.TrimSpace(input.ConnectionName)
	input.Name = strings.TrimSpace(input.Name)
	if input.ID != "" {
		if err := manageddata.ValidateCollectionID(input.ID); err != nil {
			return manageddata.Collection{}, err
		}
	}
	if err := validateIdentityPart("project id", input.ProjectID); err != nil {
		return manageddata.Collection{}, err
	}
	if err := validateIdentityPart("connection name", input.ConnectionName); err != nil {
		return manageddata.Collection{}, err
	}
	if input.Name == "" {
		input.Name = input.ConnectionName
	}
	if existing, err := r.CollectionByProjectConnection(ctx, input.ProjectID, input.ConnectionName); err == nil {
		return idempotentCollection(existing, input)
	} else if !errors.Is(err, manageddata.ErrNotFound) {
		return manageddata.Collection{}, err
	}
	var err error
	if input.ID == "" {
		input.ID, err = newID("collection")
		if err != nil {
			return manageddata.Collection{}, err
		}
	}
	err = r.q.CreateManagedDataCollection(ctx, platformdb.CreateManagedDataCollectionParams{
		ID: input.ID, ProjectID: input.ProjectID, ConnectionName: input.ConnectionName,
		Name: input.Name, Description: strings.TrimSpace(input.Description), CreatedBy: strings.TrimSpace(input.CreatedBy),
	})
	if err != nil {
		if existing, lookupErr := r.CollectionByProjectConnection(ctx, input.ProjectID, input.ConnectionName); lookupErr == nil {
			return idempotentCollection(existing, input)
		}
		return manageddata.Collection{}, mapError(err)
	}
	return r.CollectionByID(ctx, input.ID)
}

func (r *Repository) CollectionByProjectConnection(ctx context.Context, projectID, connectionName string) (manageddata.Collection, error) {
	row, err := r.q.GetManagedDataCollectionByProjectConnection(ctx, platformdb.GetManagedDataCollectionByProjectConnectionParams{
		ProjectID: strings.TrimSpace(projectID), ConnectionName: strings.TrimSpace(connectionName),
	})
	if err != nil {
		return manageddata.Collection{}, mapError(err)
	}
	return mapCollection(row), nil
}

func (r *Repository) CollectionByID(ctx context.Context, id string) (manageddata.Collection, error) {
	row, err := r.q.GetManagedDataCollection(ctx, strings.TrimSpace(id))
	if err != nil {
		return manageddata.Collection{}, mapError(err)
	}
	return mapCollection(row), nil
}

func (r *Repository) ListCollections(ctx context.Context, includeArchived bool) ([]manageddata.Collection, error) {
	var rows []platformdb.ManagedDataCollection
	var err error
	if includeArchived {
		rows, err = r.q.ListAllManagedDataCollections(ctx)
	} else {
		rows, err = r.q.ListActiveManagedDataCollections(ctx)
	}
	if err != nil {
		return nil, err
	}
	out := make([]manageddata.Collection, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapCollection(row))
	}
	return out, nil
}

func (r *Repository) ArchiveCollection(ctx context.Context, id string) error {
	result, err := r.q.ArchiveManagedDataCollection(ctx, strings.TrimSpace(id))
	return expectOne(result, err, "collection is not active")
}

func (r *Repository) CreateUploadSession(ctx context.Context, input manageddata.CreateUploadSessionInput) (manageddata.UploadSession, error) {
	input.CollectionID = strings.TrimSpace(input.CollectionID)
	input.BaseRevisionID = strings.TrimSpace(input.BaseRevisionID)
	input.StorageBackend = strings.TrimSpace(input.StorageBackend)
	input.StagingPrefix = strings.TrimSpace(input.StagingPrefix)
	if input.CollectionID == "" {
		return manageddata.UploadSession{}, fmt.Errorf("collection id is required")
	}
	if input.StorageBackend == "" {
		return manageddata.UploadSession{}, fmt.Errorf("storage backend is required")
	}
	if input.StagingPrefix == "" {
		return manageddata.UploadSession{}, fmt.Errorf("staging prefix is required")
	}
	if input.ExpiresAt.IsZero() || !input.ExpiresAt.After(time.Now()) {
		return manageddata.UploadSession{}, fmt.Errorf("upload session expiry must be in the future")
	}
	manifestJSON, err := input.Manifest.CanonicalJSON()
	if err != nil {
		return manageddata.UploadSession{}, err
	}
	fileCount, sizeBytes := manifestTotals(input.Manifest)
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id, err = newID("upload")
		if err != nil {
			return manageddata.UploadSession{}, err
		}
	}
	err = r.q.CreateManagedDataUploadSession(ctx, platformdb.CreateManagedDataUploadSessionParams{
		ID: id, CollectionID: input.CollectionID, BaseRevisionID: nullable(input.BaseRevisionID), ManifestJson: string(manifestJSON),
		ExpectedFileCount: fileCount, ExpectedSizeBytes: sizeBytes, StorageBackend: input.StorageBackend,
		StagingPrefix: input.StagingPrefix, CreatedBy: strings.TrimSpace(input.CreatedBy), ExpiresAt: timestamp(input.ExpiresAt),
	})
	if err != nil {
		return manageddata.UploadSession{}, mapError(err)
	}
	return r.UploadSessionByID(ctx, id)
}

func (r *Repository) UploadSessionByID(ctx context.Context, id string) (manageddata.UploadSession, error) {
	row, err := r.q.GetManagedDataUploadSession(ctx, strings.TrimSpace(id))
	if err != nil {
		return manageddata.UploadSession{}, mapError(err)
	}
	return mapUploadSession(row), nil
}

func (r *Repository) ListUploadSessions(ctx context.Context, collectionID string) ([]manageddata.UploadSession, error) {
	rows, err := r.q.ListManagedDataUploadSessions(ctx, strings.TrimSpace(collectionID))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]manageddata.UploadSession, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapUploadSession(row))
	}
	return out, nil
}

func (r *Repository) UpdateUploadProgress(ctx context.Context, id string, progress manageddata.UploadProgress) error {
	if progress.UploadedFileCount < 0 || progress.UploadedSizeBytes < 0 {
		return fmt.Errorf("upload progress cannot be negative")
	}
	result, err := r.q.UpdateManagedDataUploadProgress(ctx, platformdb.UpdateManagedDataUploadProgressParams{
		UploadedFileCount: progress.UploadedFileCount, UploadedSizeBytes: progress.UploadedSizeBytes, ID: strings.TrimSpace(id),
		ExpectedFileCount: progress.UploadedFileCount, ExpectedSizeBytes: progress.UploadedSizeBytes,
	})
	return expectOne(result, err, "upload session is not open or progress exceeds its manifest")
}

func (r *Repository) BeginUploadFinalization(ctx context.Context, id string) (manageddata.UploadSession, error) {
	id = strings.TrimSpace(id)
	result, err := r.q.BeginManagedDataUploadFinalization(ctx, id)
	if err := expectOne(result, err, "upload session changed while beginning finalization"); err != nil {
		return manageddata.UploadSession{}, err
	}
	row, err := r.q.GetManagedDataUploadSession(ctx, id)
	return mapUploadSession(row), mapError(err)
}

func (r *Repository) FailUploadFinalization(ctx context.Context, id, message string) (manageddata.UploadSession, error) {
	id, message = strings.TrimSpace(id), strings.TrimSpace(message)
	if message == "" {
		message = "upload finalization failed"
	}
	result, err := r.q.FailManagedDataUploadFinalization(ctx, platformdb.FailManagedDataUploadFinalizationParams{Error: message, ID: id})
	if err := expectOne(result, err, "upload session changed while failing finalization"); err != nil {
		return manageddata.UploadSession{}, err
	}
	row, err := r.q.GetManagedDataUploadSession(ctx, id)
	return mapUploadSession(row), mapError(err)
}

func (r *Repository) AbortUploadSession(ctx context.Context, id string) error {
	result, err := r.q.AbortManagedDataUploadSession(ctx, strings.TrimSpace(id))
	return expectOne(result, err, "upload session is not open")
}

func (r *Repository) ExpireUploadSessions(ctx context.Context, now time.Time) (int64, error) {
	if now.IsZero() {
		now = time.Now()
	}
	result, err := r.q.ExpireManagedDataUploadSessions(ctx, timestamp(now))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *Repository) CreateS3MultipartUpload(ctx context.Context, input manageddata.CreateS3MultipartUploadInput) (manageddata.S3MultipartUpload, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.UploadSessionID = strings.TrimSpace(input.UploadSessionID)
	input.LogicalPath = strings.TrimSpace(input.LogicalPath)
	input.SHA256 = strings.TrimSpace(input.SHA256)
	input.IdempotencyIdentity = strings.TrimSpace(input.IdempotencyIdentity)
	if input.ID == "" || input.UploadSessionID == "" || input.LogicalPath == "" {
		return manageddata.S3MultipartUpload{}, fmt.Errorf("multipart upload id, session id, and logical path are required")
	}
	if err := validateHexIdentity("multipart SHA-256", input.SHA256); err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	if input.SizeBytes < 0 {
		return manageddata.S3MultipartUpload{}, fmt.Errorf("multipart size cannot be negative")
	}
	if err := validateHexIdentity("multipart idempotency identity", input.IdempotencyIdentity); err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	err := r.q.CreateManagedDataS3MultipartUpload(ctx, platformdb.CreateManagedDataS3MultipartUploadParams{
		ID: input.ID, UploadSessionID: input.UploadSessionID, LogicalPath: input.LogicalPath, Sha256: input.SHA256,
		SizeBytes: input.SizeBytes, IdempotencyIdentity: input.IdempotencyIdentity,
	})
	if err == nil {
		return r.S3MultipartUploadByID(ctx, input.ID)
	}
	if row, lookupErr := r.q.GetManagedDataS3MultipartUpload(ctx, input.ID); lookupErr == nil {
		return idempotentS3MultipartUpload(mapS3MultipartUpload(row), input)
	}
	if _, lookupErr := r.q.GetManagedDataS3MultipartUploadByIdentity(ctx, platformdb.GetManagedDataS3MultipartUploadByIdentityParams{
		UploadSessionID: input.UploadSessionID, IdempotencyIdentity: input.IdempotencyIdentity,
	}); lookupErr == nil {
		return manageddata.S3MultipartUpload{}, fmt.Errorf("%w: multipart idempotency identity is already in use", manageddata.ErrConflict)
	}
	return manageddata.S3MultipartUpload{}, mapError(err)
}

func (r *Repository) S3MultipartUploadByID(ctx context.Context, id string) (manageddata.S3MultipartUpload, error) {
	row, err := r.q.GetManagedDataS3MultipartUpload(ctx, strings.TrimSpace(id))
	if err != nil {
		return manageddata.S3MultipartUpload{}, mapError(err)
	}
	return mapS3MultipartUpload(row), nil
}

func (r *Repository) InitializeS3MultipartUpload(ctx context.Context, input manageddata.InitializeS3MultipartUploadInput) (manageddata.S3MultipartUpload, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.ObjectKey = strings.TrimSpace(input.ObjectKey)
	input.ProviderUploadID = strings.TrimSpace(input.ProviderUploadID)
	if input.ID == "" || input.ObjectKey == "" || !safeMetadata(input.ObjectKey, 2048) {
		return manageddata.S3MultipartUpload{}, fmt.Errorf("multipart upload id and safe object key are required")
	}
	if input.Existing && input.ProviderUploadID != "" || !input.Existing && (input.ProviderUploadID == "" || !safeMetadata(input.ProviderUploadID, 2048)) {
		return manageddata.S3MultipartUpload{}, fmt.Errorf("multipart provider upload id does not match existing state")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	row, err := q.GetManagedDataS3MultipartUpload(ctx, input.ID)
	if err != nil {
		return manageddata.S3MultipartUpload{}, mapError(err)
	}
	current := mapS3MultipartUpload(row)
	if current.Status != manageddata.S3MultipartStatusCreating {
		if sameS3MultipartInitialization(current, input) {
			return current, nil
		}
		return manageddata.S3MultipartUpload{}, fmt.Errorf("%w: multipart upload is %s", manageddata.ErrConflict, current.Status)
	}
	var result sql.Result
	if input.Existing {
		result, err = q.InitializeExistingManagedDataS3MultipartUpload(ctx, platformdb.InitializeExistingManagedDataS3MultipartUploadParams{ObjectKey: input.ObjectKey, ID: input.ID})
	} else {
		result, err = q.InitializeManagedDataS3MultipartUpload(ctx, platformdb.InitializeManagedDataS3MultipartUploadParams{ObjectKey: input.ObjectKey, ProviderUploadID: input.ProviderUploadID, ID: input.ID})
	}
	if err := expectOne(result, err, "multipart upload changed while initializing"); err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	row, err = q.GetManagedDataS3MultipartUpload(ctx, input.ID)
	if err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	if err := tx.Commit(); err != nil {
		return manageddata.S3MultipartUpload{}, mapError(err)
	}
	return mapS3MultipartUpload(row), nil
}

func (r *Repository) ReserveS3MultipartPart(ctx context.Context, part manageddata.S3MultipartPart) (manageddata.S3MultipartPart, error) {
	part.MultipartUploadID = strings.TrimSpace(part.MultipartUploadID)
	part.SHA256 = strings.TrimSpace(part.SHA256)
	if part.MultipartUploadID == "" || part.PartNumber < 1 || part.PartNumber > 10_000 || part.SizeBytes <= 0 {
		return manageddata.S3MultipartPart{}, fmt.Errorf("invalid multipart part reservation")
	}
	if part.SHA256 != "" {
		if err := validateHexIdentity("multipart part SHA-256", part.SHA256); err != nil {
			return manageddata.S3MultipartPart{}, err
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return manageddata.S3MultipartPart{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	uploadRow, err := q.GetManagedDataS3MultipartUpload(ctx, part.MultipartUploadID)
	if err != nil {
		return manageddata.S3MultipartPart{}, mapError(err)
	}
	if uploadRow.Status != string(manageddata.S3MultipartStatusOpen) {
		return manageddata.S3MultipartPart{}, fmt.Errorf("%w: multipart upload is %s", manageddata.ErrConflict, uploadRow.Status)
	}
	existing, err := q.GetManagedDataS3MultipartPart(ctx, platformdb.GetManagedDataS3MultipartPartParams{
		MultipartUploadID: part.MultipartUploadID, PartNumber: int64(part.PartNumber),
	})
	if err == nil {
		mapped := mapS3MultipartPart(existing)
		if mapped == part {
			return mapped, nil
		}
		return manageddata.S3MultipartPart{}, fmt.Errorf("%w: multipart part number was reused with different metadata", manageddata.ErrConflict)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return manageddata.S3MultipartPart{}, err
	}
	total, err := q.SumManagedDataS3MultipartPartSizes(ctx, part.MultipartUploadID)
	if err != nil {
		return manageddata.S3MultipartPart{}, err
	}
	if total > uploadRow.SizeBytes-part.SizeBytes {
		return manageddata.S3MultipartPart{}, fmt.Errorf("%w: multipart part reservations exceed blob size", manageddata.ErrConflict)
	}
	if err := q.CreateManagedDataS3MultipartPart(ctx, platformdb.CreateManagedDataS3MultipartPartParams{
		MultipartUploadID: part.MultipartUploadID, PartNumber: int64(part.PartNumber), SizeBytes: part.SizeBytes, Sha256: part.SHA256,
	}); err != nil {
		return manageddata.S3MultipartPart{}, mapError(err)
	}
	if err := tx.Commit(); err != nil {
		return manageddata.S3MultipartPart{}, mapError(err)
	}
	return part, nil
}

func (r *Repository) ListS3MultipartParts(ctx context.Context, id string) ([]manageddata.S3MultipartPart, error) {
	rows, err := r.q.ListManagedDataS3MultipartParts(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	parts := make([]manageddata.S3MultipartPart, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, mapS3MultipartPart(row))
	}
	return parts, nil
}

func (r *Repository) BeginS3MultipartCompletion(ctx context.Context, input manageddata.BeginS3MultipartCompletionInput) (manageddata.S3MultipartCompletion, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.IdempotencyIdentity = strings.TrimSpace(input.IdempotencyIdentity)
	input.RequestHash = strings.TrimSpace(input.RequestHash)
	if input.ID == "" {
		return manageddata.S3MultipartCompletion{}, fmt.Errorf("multipart upload id is required")
	}
	if err := validateHexIdentity("completion idempotency identity", input.IdempotencyIdentity); err != nil {
		return manageddata.S3MultipartCompletion{}, err
	}
	if err := validateHexIdentity("completion request hash", input.RequestHash); err != nil {
		return manageddata.S3MultipartCompletion{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return manageddata.S3MultipartCompletion{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	row, err := q.GetManagedDataS3MultipartUpload(ctx, input.ID)
	if err != nil {
		return manageddata.S3MultipartCompletion{}, mapError(err)
	}
	upload := mapS3MultipartUpload(row)
	execute := true
	switch upload.Status {
	case manageddata.S3MultipartStatusOpen:
		result, updateErr := q.BeginManagedDataS3MultipartCompletion(ctx, platformdb.BeginManagedDataS3MultipartCompletionParams{
			CompletionIdentity: input.IdempotencyIdentity, CompletionRequestHash: input.RequestHash, ID: input.ID,
		})
		if err := expectOne(result, updateErr, "multipart upload changed while beginning completion"); err != nil {
			return manageddata.S3MultipartCompletion{}, err
		}
		row, err = q.GetManagedDataS3MultipartUpload(ctx, input.ID)
		if err != nil {
			return manageddata.S3MultipartCompletion{}, err
		}
		upload = mapS3MultipartUpload(row)
	case manageddata.S3MultipartStatusCompleting:
		if upload.CompletionIdentity != input.IdempotencyIdentity || upload.CompletionRequestHash != input.RequestHash {
			return manageddata.S3MultipartCompletion{}, fmt.Errorf("%w: multipart completion identity conflicts", manageddata.ErrConflict)
		}
	case manageddata.S3MultipartStatusCompleted:
		if upload.CompletionIdentity != input.IdempotencyIdentity || upload.CompletionRequestHash != input.RequestHash {
			return manageddata.S3MultipartCompletion{}, fmt.Errorf("%w: multipart completion identity conflicts", manageddata.ErrConflict)
		}
		execute = false
	default:
		return manageddata.S3MultipartCompletion{}, fmt.Errorf("%w: multipart upload is %s", manageddata.ErrConflict, upload.Status)
	}
	partRows, err := q.ListManagedDataS3MultipartParts(ctx, input.ID)
	if err != nil {
		return manageddata.S3MultipartCompletion{}, err
	}
	parts := make([]manageddata.S3MultipartPart, 0, len(partRows))
	for _, part := range partRows {
		parts = append(parts, mapS3MultipartPart(part))
	}
	if err := tx.Commit(); err != nil {
		return manageddata.S3MultipartCompletion{}, mapError(err)
	}
	return manageddata.S3MultipartCompletion{Upload: upload, Parts: parts, Execute: execute}, nil
}

func (r *Repository) FinishS3MultipartCompletion(ctx context.Context, id string) (manageddata.S3MultipartUpload, error) {
	return r.finishS3Multipart(ctx, strings.TrimSpace(id), manageddata.S3MultipartStatusCompleting, manageddata.S3MultipartStatusCompleted)
}

func (r *Repository) BeginS3MultipartAbort(ctx context.Context, input manageddata.BeginS3MultipartAbortInput) (manageddata.S3MultipartAbort, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.IdempotencyIdentity = strings.TrimSpace(input.IdempotencyIdentity)
	if input.ID == "" {
		return manageddata.S3MultipartAbort{}, fmt.Errorf("multipart upload id is required")
	}
	if err := validateHexIdentity("abort idempotency identity", input.IdempotencyIdentity); err != nil {
		return manageddata.S3MultipartAbort{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return manageddata.S3MultipartAbort{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	row, err := q.GetManagedDataS3MultipartUpload(ctx, input.ID)
	if err != nil {
		return manageddata.S3MultipartAbort{}, mapError(err)
	}
	upload := mapS3MultipartUpload(row)
	execute := true
	switch upload.Status {
	case manageddata.S3MultipartStatusCreating, manageddata.S3MultipartStatusOpen, manageddata.S3MultipartStatusFailed:
		result, updateErr := q.BeginManagedDataS3MultipartAbort(ctx, platformdb.BeginManagedDataS3MultipartAbortParams{AbortIdentity: input.IdempotencyIdentity, ID: input.ID})
		if err := expectOne(result, updateErr, "multipart upload changed while beginning abort"); err != nil {
			return manageddata.S3MultipartAbort{}, err
		}
		row, err = q.GetManagedDataS3MultipartUpload(ctx, input.ID)
		if err != nil {
			return manageddata.S3MultipartAbort{}, err
		}
		upload = mapS3MultipartUpload(row)
	case manageddata.S3MultipartStatusAborting:
		if upload.AbortIdentity != input.IdempotencyIdentity {
			return manageddata.S3MultipartAbort{}, fmt.Errorf("%w: multipart abort identity conflicts", manageddata.ErrConflict)
		}
	case manageddata.S3MultipartStatusAborted:
		if upload.AbortIdentity != input.IdempotencyIdentity {
			return manageddata.S3MultipartAbort{}, fmt.Errorf("%w: multipart abort identity conflicts", manageddata.ErrConflict)
		}
		execute = false
	default:
		return manageddata.S3MultipartAbort{}, fmt.Errorf("%w: multipart upload is %s", manageddata.ErrConflict, upload.Status)
	}
	if err := tx.Commit(); err != nil {
		return manageddata.S3MultipartAbort{}, mapError(err)
	}
	return manageddata.S3MultipartAbort{Upload: upload, Execute: execute}, nil
}

func (r *Repository) FinishS3MultipartAbort(ctx context.Context, id string) (manageddata.S3MultipartUpload, error) {
	return r.finishS3Multipart(ctx, strings.TrimSpace(id), manageddata.S3MultipartStatusAborting, manageddata.S3MultipartStatusAborted)
}

func (r *Repository) FailS3MultipartUpload(ctx context.Context, id, message string) (manageddata.S3MultipartUpload, error) {
	id = strings.TrimSpace(id)
	message = strings.TrimSpace(message)
	if id == "" || message == "" || !safeMetadata(message, 2048) {
		return manageddata.S3MultipartUpload{}, fmt.Errorf("multipart upload id and safe terminal error are required")
	}
	current, err := r.S3MultipartUploadByID(ctx, id)
	if err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	if current.Status == manageddata.S3MultipartStatusFailed {
		if current.Error == message {
			return current, nil
		}
		return manageddata.S3MultipartUpload{}, fmt.Errorf("%w: multipart terminal error conflicts", manageddata.ErrConflict)
	}
	result, err := r.q.FailManagedDataS3MultipartUpload(ctx, platformdb.FailManagedDataS3MultipartUploadParams{Error: message, ID: id})
	if err := expectOne(result, err, "multipart upload cannot fail from its current state"); err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	return r.S3MultipartUploadByID(ctx, id)
}

func (r *Repository) ListRecoverableS3MultipartUploads(ctx context.Context, before time.Time, limit int64) ([]manageddata.S3MultipartUpload, error) {
	if before.IsZero() {
		before = time.Now()
	}
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("multipart recovery limit must be between 1 and 1000")
	}
	rows, err := r.q.ListRecoverableManagedDataS3MultipartUploads(ctx, platformdb.ListRecoverableManagedDataS3MultipartUploadsParams{
		UpdatedCutoff: timestamp(before), ExpiryCutoff: timestamp(before), RowLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	uploads := make([]manageddata.S3MultipartUpload, 0, len(rows))
	for _, row := range rows {
		uploads = append(uploads, mapS3MultipartUpload(row))
	}
	return uploads, nil
}

func (r *Repository) finishS3Multipart(ctx context.Context, id string, from, to manageddata.S3MultipartStatus) (manageddata.S3MultipartUpload, error) {
	if id == "" {
		return manageddata.S3MultipartUpload{}, fmt.Errorf("multipart upload id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	row, err := q.GetManagedDataS3MultipartUpload(ctx, id)
	if err != nil {
		return manageddata.S3MultipartUpload{}, mapError(err)
	}
	current := mapS3MultipartUpload(row)
	if current.Status == to {
		return current, nil
	}
	if current.Status != from {
		return manageddata.S3MultipartUpload{}, fmt.Errorf("%w: multipart upload is %s", manageddata.ErrConflict, current.Status)
	}
	var result sql.Result
	if to == manageddata.S3MultipartStatusCompleted {
		result, err = q.FinishManagedDataS3MultipartCompletion(ctx, id)
	} else {
		result, err = q.FinishManagedDataS3MultipartAbort(ctx, id)
	}
	if err := expectOne(result, err, "multipart upload changed while finishing transition"); err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	row, err = q.GetManagedDataS3MultipartUpload(ctx, id)
	if err != nil {
		return manageddata.S3MultipartUpload{}, err
	}
	if err := tx.Commit(); err != nil {
		return manageddata.S3MultipartUpload{}, mapError(err)
	}
	return mapS3MultipartUpload(row), nil
}

func (r *Repository) CompleteUpload(ctx context.Context, input manageddata.CompleteUploadInput) (manageddata.Revision, error) {
	input.SessionID = strings.TrimSpace(input.SessionID)
	if input.SessionID == "" {
		return manageddata.Revision{}, fmt.Errorf("upload session id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return manageddata.Revision{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	result, err := q.MarkManagedDataUploadCommitting(ctx, input.SessionID)
	if err != nil {
		return manageddata.Revision{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return manageddata.Revision{}, err
	}
	if affected != 1 {
		session, getErr := q.GetManagedDataUploadSession(ctx, input.SessionID)
		if getErr != nil {
			return manageddata.Revision{}, mapError(getErr)
		}
		if session.Status == string(manageddata.UploadStatusComplete) && session.RevisionID.Valid {
			row, getErr := q.GetManagedDataRevision(ctx, session.RevisionID.String)
			return mapRevision(row), mapError(getErr)
		}
		return manageddata.Revision{}, fmt.Errorf("%w: upload session is %s or expired", manageddata.ErrConflict, session.Status)
	}
	session, err := q.GetManagedDataUploadSession(ctx, input.SessionID)
	if err != nil {
		return manageddata.Revision{}, err
	}
	manifest, err := decodeManifest(session.ManifestJson)
	if err != nil {
		return manageddata.Revision{}, err
	}
	if err := validateStoredFiles(manifest, input.Files); err != nil {
		return manageddata.Revision{}, err
	}
	sequence, err := q.NextManagedDataRevisionSequence(ctx, session.CollectionID)
	if err != nil {
		return manageddata.Revision{}, err
	}
	revisionID := strings.TrimSpace(input.RevisionID)
	if revisionID == "" {
		revisionID, err = newID("revision")
		if err != nil {
			return manageddata.Revision{}, err
		}
	}
	if err := q.CreateReadyManagedDataRevision(ctx, platformdb.CreateReadyManagedDataRevisionParams{
		ID: revisionID, CollectionID: session.CollectionID, Sequence: sequence, Digest: manifest.RevisionID(),
		ManifestJson: session.ManifestJson, FileCount: session.ExpectedFileCount, SizeBytes: session.ExpectedSizeBytes, CreatedBy: session.CreatedBy,
	}); err != nil {
		return manageddata.Revision{}, mapError(err)
	}
	for _, file := range sortedStoredFiles(input.Files) {
		if err := q.CreateManagedDataRevisionFile(ctx, platformdb.CreateManagedDataRevisionFileParams{
			RevisionID: revisionID, LogicalPath: file.Path, SizeBytes: file.Size, Sha256: file.SHA256,
			StorageKey: file.StorageKey, MediaType: strings.TrimSpace(file.MediaType), Etag: strings.TrimSpace(file.ETag),
		}); err != nil {
			return manageddata.Revision{}, mapError(err)
		}
	}
	result, err = q.CompleteManagedDataUploadSession(ctx, platformdb.CompleteManagedDataUploadSessionParams{RevisionID: nullable(revisionID), ID: input.SessionID})
	if err != nil {
		return manageddata.Revision{}, err
	}
	if err := requireOne(result, "upload session changed while committing"); err != nil {
		return manageddata.Revision{}, err
	}
	if err := tx.Commit(); err != nil {
		return manageddata.Revision{}, mapError(err)
	}
	return r.RevisionByID(ctx, revisionID)
}

func (r *Repository) RevisionByID(ctx context.Context, id string) (manageddata.Revision, error) {
	row, err := r.q.GetManagedDataRevision(ctx, strings.TrimSpace(id))
	if err != nil {
		return manageddata.Revision{}, mapError(err)
	}
	return mapRevision(row), nil
}

func (r *Repository) ListRevisions(ctx context.Context, collectionID string) ([]manageddata.Revision, error) {
	rows, err := r.q.ListManagedDataRevisions(ctx, strings.TrimSpace(collectionID))
	if err != nil {
		return nil, err
	}
	out := make([]manageddata.Revision, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapRevision(row))
	}
	return out, nil
}

func (r *Repository) UploadSessionIDByRevisionID(ctx context.Context, revisionID string) (string, error) {
	id, err := r.q.GetManagedDataUploadSessionIDByRevision(ctx, nullable(strings.TrimSpace(revisionID)))
	if err != nil {
		return "", mapError(err)
	}
	return id, nil
}

func (r *Repository) ListRevisionFiles(ctx context.Context, revisionID string) ([]manageddata.RevisionFile, error) {
	rows, err := r.q.ListManagedDataRevisionFiles(ctx, strings.TrimSpace(revisionID))
	if err != nil {
		return nil, err
	}
	out := make([]manageddata.RevisionFile, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapRevisionFile(row))
	}
	return out, nil
}

func (r *Repository) EnvironmentPointer(ctx context.Context, collectionID string, environment manageddata.Environment) (manageddata.EnvironmentPointer, error) {
	normalized, err := manageddata.NormalizeEnvironment(string(environment))
	if err != nil {
		return manageddata.EnvironmentPointer{}, err
	}
	row, err := r.q.GetManagedDataEnvironmentPointer(ctx, platformdb.GetManagedDataEnvironmentPointerParams{CollectionID: strings.TrimSpace(collectionID), Environment: string(normalized)})
	if err != nil {
		return manageddata.EnvironmentPointer{}, mapError(err)
	}
	return mapEnvironmentPointer(row), nil
}

func (r *Repository) ReplaceServingStateBindings(ctx context.Context, servingStateID string, bindings []manageddata.ServingStateBinding) error {
	servingStateID = strings.TrimSpace(servingStateID)
	if servingStateID == "" {
		return fmt.Errorf("serving state id is required")
	}
	normalized := make([]manageddata.ServingStateBinding, 0, len(bindings))
	seen := map[string]struct{}{}
	for _, binding := range bindings {
		binding.CollectionID = strings.TrimSpace(binding.CollectionID)
		binding.RevisionID = strings.TrimSpace(binding.RevisionID)
		if binding.CollectionID == "" || binding.RevisionID == "" {
			return fmt.Errorf("binding collection and revision ids are required")
		}
		if _, exists := seen[binding.CollectionID]; exists {
			return fmt.Errorf("duplicate binding for collection %q", binding.CollectionID)
		}
		seen[binding.CollectionID] = struct{}{}
		environment, err := manageddata.NormalizeEnvironment(string(binding.Environment))
		if err != nil {
			return err
		}
		binding.Environment = environment
		binding.ServingStateID = servingStateID
		normalized = append(normalized, binding)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].CollectionID < normalized[j].CollectionID })
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	if err := q.DeleteManagedDataServingStateBindings(ctx, servingStateID); err != nil {
		return err
	}
	for _, binding := range normalized {
		if err := q.CreateManagedDataServingStateBinding(ctx, platformdb.CreateManagedDataServingStateBindingParams{
			ServingStateID: servingStateID, CollectionID: binding.CollectionID, RevisionID: binding.RevisionID, Environment: string(binding.Environment),
		}); err != nil {
			return mapError(err)
		}
	}
	return tx.Commit()
}

func (r *Repository) ListServingStateBindings(ctx context.Context, servingStateID string) ([]manageddata.ServingStateBinding, error) {
	rows, err := r.q.ListManagedDataServingStateBindings(ctx, strings.TrimSpace(servingStateID))
	if err != nil {
		return nil, err
	}
	out := make([]manageddata.ServingStateBinding, 0, len(rows))
	for _, row := range rows {
		out = append(out, manageddata.ServingStateBinding{ServingStateID: row.ServingStateID, CollectionID: row.CollectionID, RevisionID: row.RevisionID, Environment: manageddata.Environment(row.Environment), BoundAt: row.BoundAt})
	}
	return out, nil
}

func validateStoredFiles(manifest manageddata.Manifest, files []manageddata.StoredFile) error {
	if len(files) != len(manifest.Files) {
		return fmt.Errorf("stored file count %d does not match manifest count %d", len(files), len(manifest.Files))
	}
	actual := manageddata.Manifest{Files: make([]manageddata.File, 0, len(files))}
	for _, file := range files {
		if strings.TrimSpace(file.StorageKey) == "" {
			return fmt.Errorf("stored file %q has no storage key", file.Path)
		}
		actual.Files = append(actual.Files, file.File)
	}
	wantJSON, err := manifest.CanonicalJSON()
	if err != nil {
		return err
	}
	actualJSON, err := actual.CanonicalJSON()
	if err != nil {
		return err
	}
	if !bytes.Equal(wantJSON, actualJSON) {
		return fmt.Errorf("stored files do not match upload manifest")
	}
	return nil
}

func decodeManifest(value string) (manageddata.Manifest, error) {
	var manifest manageddata.Manifest
	if err := json.Unmarshal([]byte(value), &manifest); err != nil {
		return manageddata.Manifest{}, fmt.Errorf("decode upload manifest: %w", err)
	}
	if err := manifest.Validate(manageddata.Limits{}); err != nil {
		return manageddata.Manifest{}, err
	}
	return manifest, nil
}

func manifestTotals(manifest manageddata.Manifest) (int64, int64) {
	var size int64
	for _, file := range manifest.Files {
		size += file.Size
	}
	return int64(len(manifest.Files)), size
}

func sortedStoredFiles(files []manageddata.StoredFile) []manageddata.StoredFile {
	out := append([]manageddata.StoredFile(nil), files...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func mapCollection(row platformdb.ManagedDataCollection) manageddata.Collection {
	return manageddata.Collection{ID: row.ID, ProjectID: row.ProjectID, ConnectionName: row.ConnectionName, Name: row.Name, Description: row.Description, Status: manageddata.CollectionStatus(row.Status), CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ArchivedAt: row.ArchivedAt.String}
}

func mapRevision(row platformdb.ManagedDataRevision) manageddata.Revision {
	return manageddata.Revision{ID: row.ID, CollectionID: row.CollectionID, Sequence: row.Sequence, Digest: row.Digest, Status: manageddata.RevisionStatus(row.Status), ManifestJSON: row.ManifestJson, FileCount: row.FileCount, SizeBytes: row.SizeBytes, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, ReadyAt: row.ReadyAt.String, Error: row.Error}
}

func mapRevisionFile(row platformdb.ManagedDataRevisionFile) manageddata.RevisionFile {
	return manageddata.RevisionFile{RevisionID: row.RevisionID, StoredFile: manageddata.StoredFile{File: manageddata.File{Path: row.LogicalPath, Size: row.SizeBytes, SHA256: row.Sha256}, StorageKey: row.StorageKey, MediaType: row.MediaType, ETag: row.Etag}, CreatedAt: row.CreatedAt}
}

func mapUploadSession(row platformdb.ManagedDataUploadSession) manageddata.UploadSession {
	return manageddata.UploadSession{ID: row.ID, CollectionID: row.CollectionID, BaseRevisionID: row.BaseRevisionID.String, RevisionID: row.RevisionID.String, Status: manageddata.UploadStatus(row.Status), ManifestJSON: row.ManifestJson, ExpectedFileCount: row.ExpectedFileCount, ExpectedSizeBytes: row.ExpectedSizeBytes, UploadedFileCount: row.UploadedFileCount, UploadedSizeBytes: row.UploadedSizeBytes, StorageBackend: row.StorageBackend, StagingPrefix: row.StagingPrefix, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ExpiresAt: row.ExpiresAt, CompletedAt: row.CompletedAt.String, Error: row.Error}
}

func mapS3MultipartUpload(row platformdb.ManagedDataS3MultipartUpload) manageddata.S3MultipartUpload {
	return manageddata.S3MultipartUpload{
		ID: row.ID, UploadSessionID: row.UploadSessionID, LogicalPath: row.LogicalPath, SHA256: row.Sha256, SizeBytes: row.SizeBytes,
		ObjectKey: row.ObjectKey, ProviderUploadID: row.ProviderUploadID, Status: manageddata.S3MultipartStatus(row.Status),
		Existing: row.Existing == 1, IdempotencyIdentity: row.IdempotencyIdentity,
		CompletionIdentity: row.CompletionIdentity, CompletionRequestHash: row.CompletionRequestHash,
		AbortIdentity: row.AbortIdentity, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		CompletedAt: row.CompletedAt.String, AbortedAt: row.AbortedAt.String, Error: row.Error,
	}
}

func mapS3MultipartPart(row platformdb.ManagedDataS3MultipartPart) manageddata.S3MultipartPart {
	return manageddata.S3MultipartPart{MultipartUploadID: row.MultipartUploadID, PartNumber: int32(row.PartNumber), SizeBytes: row.SizeBytes, SHA256: row.Sha256}
}

func idempotentS3MultipartUpload(existing manageddata.S3MultipartUpload, input manageddata.CreateS3MultipartUploadInput) (manageddata.S3MultipartUpload, error) {
	if existing.ID != input.ID || existing.UploadSessionID != input.UploadSessionID || existing.LogicalPath != input.LogicalPath || existing.SHA256 != input.SHA256 ||
		existing.SizeBytes != input.SizeBytes || existing.IdempotencyIdentity != input.IdempotencyIdentity {
		return manageddata.S3MultipartUpload{}, fmt.Errorf("%w: multipart identity was reused with different metadata", manageddata.ErrConflict)
	}
	return existing, nil
}

func sameS3MultipartInitialization(existing manageddata.S3MultipartUpload, input manageddata.InitializeS3MultipartUploadInput) bool {
	if existing.ObjectKey != input.ObjectKey || existing.Existing != input.Existing {
		return false
	}
	if input.Existing {
		return existing.Status == manageddata.S3MultipartStatusCompleted && existing.ProviderUploadID == ""
	}
	return existing.ProviderUploadID == input.ProviderUploadID && existing.Status != manageddata.S3MultipartStatusCreating
}

func validateHexIdentity(name, value string) error {
	if len(value) != 64 || strings.ToLower(value) != value {
		return fmt.Errorf("%s must be 64 lowercase hexadecimal characters", name)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%s must be 64 lowercase hexadecimal characters", name)
	}
	return nil
}

func safeMetadata(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func validateIdentityPart(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > 255 {
		return fmt.Errorf("%s exceeds 255 characters", name)
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}

func idempotentCollection(existing manageddata.Collection, input manageddata.CreateCollectionInput) (manageddata.Collection, error) {
	if input.ID != "" && existing.ID != input.ID || existing.Name != input.Name || existing.Description != strings.TrimSpace(input.Description) {
		return manageddata.Collection{}, fmt.Errorf("%w: collection %q/%q already exists with different identity or metadata", manageddata.ErrConflict, input.ProjectID, input.ConnectionName)
	}
	return existing, nil
}

func mapEnvironmentPointer(row platformdb.ManagedDataEnvironmentPointer) manageddata.EnvironmentPointer {
	return manageddata.EnvironmentPointer{CollectionID: row.CollectionID, Environment: manageddata.Environment(row.Environment), RevisionID: row.RevisionID, DeploymentID: row.DeploymentID, Generation: row.Generation, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt}
}

func timestamp(value time.Time) string { return value.UTC().Format("2006-01-02 15:04:05.000000000") }

func nullable(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func newID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

func expectOne(result sql.Result, err error, message string) error {
	if err != nil {
		return mapError(err)
	}
	return requireOne(result, message)
}

func requireOne(result sql.Result, message string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("%w: %s", manageddata.ErrConflict, message)
	}
	return nil
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return manageddata.ErrNotFound
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "foreign key constraint") || strings.Contains(message, "constraint failed") {
		return fmt.Errorf("%w: %v", manageddata.ErrConflict, err)
	}
	return err
}

var _ manageddata.Repository = (*Repository)(nil)
