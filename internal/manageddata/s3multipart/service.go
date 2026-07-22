package s3multipart

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata"
	"github.com/Yacobolo/leapview/internal/manageddata/control"
	"github.com/Yacobolo/leapview/internal/manageddata/storage"
)

const (
	integrityTerminalError = "completed object failed integrity verification"
	sqliteTimestampLayout  = "2006-01-02 15:04:05.000000000"
)

type Service struct {
	repo       Repository
	store      MultipartStore
	backend    string
	signExpiry time.Duration
	now        func() time.Time
}

var _ Coordinator = (*Service)(nil)

func New(repo Repository, store MultipartStore, config Config) (*Service, error) {
	if repo == nil || store == nil {
		return nil, fmt.Errorf("%w: multipart repository and store are required", control.ErrInvalid)
	}
	if err := validateIdentity("storage backend", config.Backend, 128); err != nil {
		return nil, err
	}
	expiry := config.SignExpiry
	if expiry == 0 {
		expiry = defaultSignExpiry
	}
	if expiry < time.Minute || expiry > 24*time.Hour {
		return nil, fmt.Errorf("%w: signing expiry must be between one minute and 24 hours", control.ErrInvalid)
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, store: store, backend: config.Backend, signExpiry: expiry, now: clock}, nil
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (UploadResult, error) {
	session, manifest, err := s.scopedSession(ctx, request.Project, request.Connection, request.UploadSessionID)
	if err != nil {
		return UploadResult{}, err
	}
	if err := validateIdempotencyKey(request.IdempotencyKey); err != nil {
		return UploadResult{}, err
	}
	file, err := manifestFile(manifest, request.Path)
	if err != nil {
		return UploadResult{}, err
	}
	if file.Size == 0 || file.Size > MaximumObjectSize {
		return UploadResult{}, fmt.Errorf("%w: file size is outside S3 multipart limits", control.ErrInvalid)
	}
	identity := identityHash("create", session.ID, request.IdempotencyKey)
	id := "multipart_" + identity
	if existing, lookupErr := s.repo.S3MultipartUploadByID(ctx, id); lookupErr == nil {
		if !sameCreateIdentity(existing, session.ID, file, identity) {
			return UploadResult{}, control.ErrConflict
		}
		if existing.Status == manageddata.S3MultipartStatusCompleted || existing.Status == manageddata.S3MultipartStatusAborted {
			return resultFor(existing, session, file)
		}
	} else if !errors.Is(lookupErr, manageddata.ErrNotFound) {
		return UploadResult{}, repositoryError(lookupErr)
	}
	if err := requireOpenSession(session, s.now()); err != nil {
		return UploadResult{}, err
	}
	upload, err := s.repo.CreateS3MultipartUpload(ctx, manageddata.CreateS3MultipartUploadInput{
		ID: id, UploadSessionID: session.ID, LogicalPath: file.Path, SHA256: file.SHA256,
		SizeBytes: file.Size, IdempotencyIdentity: identity,
	})
	if err != nil {
		return UploadResult{}, repositoryError(err)
	}
	switch upload.Status {
	case manageddata.S3MultipartStatusOpen, manageddata.S3MultipartStatusCompleted:
		return resultFor(upload, session, file)
	case manageddata.S3MultipartStatusCreating:
	default:
		return UploadResult{}, fmt.Errorf("%w: multipart upload is %s", control.ErrConflict, upload.Status)
	}

	provider, err := s.store.CreateMultipart(ctx, storage.Blob{SHA256: file.SHA256, Size: file.Size})
	if err != nil {
		return UploadResult{}, storageError(err)
	}
	initialized, initErr := s.repo.InitializeS3MultipartUpload(ctx, manageddata.InitializeS3MultipartUploadInput{
		ID: upload.ID, ObjectKey: provider.Key, ProviderUploadID: provider.UploadID, Existing: provider.Existing,
	})
	if initErr == nil {
		return resultFor(initialized, session, file)
	}
	if !provider.Existing {
		_ = s.store.AbortMultipart(ctx, provider)
	}
	current, lookupErr := s.repo.S3MultipartUploadByID(ctx, upload.ID)
	if lookupErr == nil && (current.Status == manageddata.S3MultipartStatusOpen || current.Status == manageddata.S3MultipartStatusCompleted) {
		return resultFor(current, session, file)
	}
	return UploadResult{}, repositoryError(initErr)
}

func (s *Service) SignPart(ctx context.Context, request SignPartRequest) (SignedPartResult, error) {
	session, upload, file, err := s.scopedUpload(ctx, request.Project, request.Connection, request.UploadSessionID, request.MultipartUploadID)
	if err != nil {
		return SignedPartResult{}, err
	}
	if err := requireOpenSession(session, s.now()); err != nil {
		return SignedPartResult{}, err
	}
	if upload.Status != manageddata.S3MultipartStatusOpen {
		return SignedPartResult{}, fmt.Errorf("%w: multipart upload is %s", control.ErrConflict, upload.Status)
	}
	if request.PartNumber < 1 || request.PartNumber > MaximumParts || request.Size <= 0 || request.Size > MaximumPartSize || request.Size > file.Size {
		return SignedPartResult{}, fmt.Errorf("%w: part number or size is outside S3 multipart limits", control.ErrInvalid)
	}
	if request.SHA256 != "" {
		if err := validateDigest(request.SHA256); err != nil {
			return SignedPartResult{}, err
		}
	}
	part, err := s.repo.ReserveS3MultipartPart(ctx, manageddata.S3MultipartPart{
		MultipartUploadID: upload.ID, PartNumber: request.PartNumber, SizeBytes: request.Size, SHA256: request.SHA256,
	})
	if err != nil {
		return SignedPartResult{}, repositoryError(err)
	}
	signed, err := s.store.SignPart(ctx, providerUpload(upload), storage.MultipartPartRequest{Number: part.PartNumber, Size: part.SizeBytes, SHA256: part.SHA256})
	if err != nil {
		return SignedPartResult{}, storageError(err)
	}
	if signed.Number != part.PartNumber || !safeProviderValue(signed.URL, 8192) {
		return SignedPartResult{}, control.ErrBackend
	}
	headers, err := responseHeaders(signed.Headers)
	if err != nil {
		return SignedPartResult{}, err
	}
	return SignedPartResult{
		UploadSessionID: request.UploadSessionID, MultipartUploadID: request.MultipartUploadID,
		PartNumber: signed.Number, URL: signed.URL, Headers: headers,
		ExpiresAt: s.now().UTC().Add(s.signExpiry).Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) Complete(ctx context.Context, request CompleteRequest) (UploadResult, error) {
	session, upload, file, err := s.scopedUpload(ctx, request.Project, request.Connection, request.UploadSessionID, request.MultipartUploadID)
	if err != nil {
		return UploadResult{}, err
	}
	if err := validateIdempotencyKey(request.IdempotencyKey); err != nil {
		return UploadResult{}, err
	}
	ordered, requestHash, err := canonicalCompletedParts(request.Parts)
	if err != nil {
		return UploadResult{}, err
	}
	if upload.Status != manageddata.S3MultipartStatusCompleted {
		if err := requireOpenSession(session, s.now()); err != nil {
			return UploadResult{}, err
		}
		reserved, listErr := s.repo.ListS3MultipartParts(ctx, upload.ID)
		if listErr != nil {
			return UploadResult{}, repositoryError(listErr)
		}
		if err := validateCompletionShape(file.Size, reserved, ordered); err != nil {
			return UploadResult{}, err
		}
	}
	claim, err := s.repo.BeginS3MultipartCompletion(ctx, manageddata.BeginS3MultipartCompletionInput{
		ID: upload.ID, IdempotencyIdentity: identityHash("complete", upload.ID, request.IdempotencyKey), RequestHash: requestHash,
	})
	if err != nil {
		return UploadResult{}, repositoryError(err)
	}
	if !claim.Execute {
		return resultFor(claim.Upload, session, file)
	}
	providerParts := make([]storage.CompletedMultipartPart, len(ordered))
	for index, part := range ordered {
		providerParts[index] = storage.CompletedMultipartPart{Number: part.PartNumber, ETag: part.ETag, SHA256: part.SHA256}
	}
	blob, err := s.store.CompleteMultipart(ctx, providerUpload(claim.Upload), providerParts)
	if err != nil {
		if errors.Is(err, storage.ErrIntegrity) {
			_, _ = s.repo.FailS3MultipartUpload(ctx, upload.ID, integrityTerminalError)
			return UploadResult{}, control.ErrIntegrity
		}
		return UploadResult{}, storageError(err)
	}
	if blob.SHA256 != file.SHA256 || blob.Size != file.Size {
		_, _ = s.repo.FailS3MultipartUpload(ctx, upload.ID, integrityTerminalError)
		return UploadResult{}, control.ErrIntegrity
	}
	completed, err := s.repo.FinishS3MultipartCompletion(ctx, upload.ID)
	if err != nil {
		return UploadResult{}, repositoryError(err)
	}
	return resultFor(completed, session, file)
}

func (s *Service) Abort(ctx context.Context, request AbortRequest) (UploadResult, error) {
	session, upload, file, err := s.scopedUpload(ctx, request.Project, request.Connection, request.UploadSessionID, request.MultipartUploadID)
	if err != nil {
		return UploadResult{}, err
	}
	if err := validateIdempotencyKey(request.IdempotencyKey); err != nil {
		return UploadResult{}, err
	}
	claim, err := s.repo.BeginS3MultipartAbort(ctx, manageddata.BeginS3MultipartAbortInput{
		ID: upload.ID, IdempotencyIdentity: identityHash("abort", upload.ID, request.IdempotencyKey),
	})
	if err != nil {
		return UploadResult{}, repositoryError(err)
	}
	if claim.Execute && claim.Upload.ProviderUploadID != "" {
		if err := s.store.AbortMultipart(ctx, providerUpload(claim.Upload)); err != nil {
			return UploadResult{}, storageError(err)
		}
	}
	if claim.Execute {
		claim.Upload, err = s.repo.FinishS3MultipartAbort(ctx, upload.ID)
		if err != nil {
			return UploadResult{}, repositoryError(err)
		}
	}
	return resultFor(claim.Upload, session, file)
}

func (s *Service) RecoverOrphaned(ctx context.Context, before time.Time, limit int64) (RecoveryResult, error) {
	if ctx == nil || before.IsZero() {
		return RecoveryResult{}, fmt.Errorf("%w: context and recovery cutoff are required", control.ErrInvalid)
	}
	uploads, err := s.repo.ListRecoverableS3MultipartUploads(ctx, before, limit)
	if err != nil {
		return RecoveryResult{}, repositoryError(err)
	}
	result := RecoveryResult{}
	for _, upload := range uploads {
		if upload.Status != manageddata.S3MultipartStatusAborting {
			claim, claimErr := s.repo.BeginS3MultipartAbort(ctx, manageddata.BeginS3MultipartAbortInput{
				ID: upload.ID, IdempotencyIdentity: identityHash("recovery", upload.ID, upload.ID),
			})
			if claimErr != nil {
				return result, repositoryError(claimErr)
			}
			upload = claim.Upload
		}
		if err := s.store.AbortMultipart(ctx, providerUpload(upload)); err != nil {
			return result, storageError(err)
		}
		if _, err := s.repo.FinishS3MultipartAbort(ctx, upload.ID); err != nil {
			return result, repositoryError(err)
		}
		result.Aborted++
	}
	return result, nil
}

func (s *Service) scopedSession(ctx context.Context, project, connection, sessionID string) (manageddata.UploadSession, manageddata.Manifest, error) {
	if ctx == nil {
		return manageddata.UploadSession{}, manageddata.Manifest{}, fmt.Errorf("%w: context is required", control.ErrInvalid)
	}
	if err := validateScopeValue("project", project); err != nil {
		return manageddata.UploadSession{}, manageddata.Manifest{}, err
	}
	if err := validateScopeValue("connection", connection); err != nil {
		return manageddata.UploadSession{}, manageddata.Manifest{}, err
	}
	if err := validateIdentity("upload session id", sessionID, 160); err != nil {
		return manageddata.UploadSession{}, manageddata.Manifest{}, err
	}
	collection, err := s.repo.CollectionByProjectConnection(ctx, project, connection)
	if err != nil {
		return manageddata.UploadSession{}, manageddata.Manifest{}, repositoryError(err)
	}
	if collection.Status != manageddata.CollectionStatusActive {
		return manageddata.UploadSession{}, manageddata.Manifest{}, control.ErrConflict
	}
	session, err := s.repo.UploadSessionByID(ctx, sessionID)
	if err != nil {
		return manageddata.UploadSession{}, manageddata.Manifest{}, repositoryError(err)
	}
	if session.CollectionID != collection.ID {
		return manageddata.UploadSession{}, manageddata.Manifest{}, control.ErrNotFound
	}
	if session.StorageBackend != s.backend {
		return manageddata.UploadSession{}, manageddata.Manifest{}, fmt.Errorf("%w: upload session uses another storage backend", control.ErrConflict)
	}
	manifest, err := strictManifest(session.ManifestJSON)
	if err != nil {
		return manageddata.UploadSession{}, manageddata.Manifest{}, err
	}
	return session, manifest, nil
}

func (s *Service) scopedUpload(ctx context.Context, project, connection, sessionID, multipartID string) (manageddata.UploadSession, manageddata.S3MultipartUpload, manageddata.File, error) {
	session, manifest, err := s.scopedSession(ctx, project, connection, sessionID)
	if err != nil {
		return manageddata.UploadSession{}, manageddata.S3MultipartUpload{}, manageddata.File{}, err
	}
	if err := validateIdentity("multipart upload id", multipartID, 160); err != nil {
		return manageddata.UploadSession{}, manageddata.S3MultipartUpload{}, manageddata.File{}, err
	}
	upload, err := s.repo.S3MultipartUploadByID(ctx, multipartID)
	if err != nil {
		return manageddata.UploadSession{}, manageddata.S3MultipartUpload{}, manageddata.File{}, repositoryError(err)
	}
	if upload.UploadSessionID != session.ID {
		return manageddata.UploadSession{}, manageddata.S3MultipartUpload{}, manageddata.File{}, control.ErrNotFound
	}
	file, err := manifestFile(manifest, upload.LogicalPath)
	if err != nil {
		return manageddata.UploadSession{}, manageddata.S3MultipartUpload{}, manageddata.File{}, control.ErrIntegrity
	}
	if file.SHA256 != upload.SHA256 || file.Size != upload.SizeBytes {
		return manageddata.UploadSession{}, manageddata.S3MultipartUpload{}, manageddata.File{}, control.ErrIntegrity
	}
	return session, upload, file, nil
}

func strictManifest(value string) (manageddata.Manifest, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var manifest manageddata.Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return manageddata.Manifest{}, control.ErrIntegrity
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return manageddata.Manifest{}, control.ErrIntegrity
	}
	canonical, err := manifest.CanonicalJSON()
	if err != nil || !bytes.Equal(canonical, []byte(value)) {
		return manageddata.Manifest{}, control.ErrIntegrity
	}
	return manifest, nil
}

func sameCreateIdentity(upload manageddata.S3MultipartUpload, sessionID string, file manageddata.File, identity string) bool {
	return upload.UploadSessionID == sessionID && upload.LogicalPath == file.Path && upload.SHA256 == file.SHA256 &&
		upload.SizeBytes == file.Size && upload.IdempotencyIdentity == identity
}

func manifestFile(manifest manageddata.Manifest, path string) (manageddata.File, error) {
	if path == "" || path != strings.TrimSpace(path) {
		return manageddata.File{}, fmt.Errorf("%w: canonical manifest path is required", control.ErrInvalid)
	}
	for _, file := range manifest.Files {
		if file.Path == path {
			return file, nil
		}
	}
	return manageddata.File{}, control.ErrNotFound
}

func requireOpenSession(session manageddata.UploadSession, now time.Time) error {
	if session.Status != manageddata.UploadStatusOpen {
		return fmt.Errorf("%w: upload session is %s", control.ErrConflict, session.Status)
	}
	expiresAt, err := time.Parse(sqliteTimestampLayout, session.ExpiresAt)
	if err != nil {
		return control.ErrIntegrity
	}
	if !now.UTC().Before(expiresAt) {
		return control.ErrExpired
	}
	return nil
}

func canonicalCompletedParts(parts []CompletedPart) ([]CompletedPart, string, error) {
	if len(parts) == 0 || len(parts) > int(MaximumParts) {
		return nil, "", fmt.Errorf("%w: completion requires between 1 and %d parts", control.ErrInvalid, MaximumParts)
	}
	ordered := append([]CompletedPart(nil), parts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].PartNumber < ordered[j].PartNumber })
	for index, part := range ordered {
		if part.PartNumber < 1 || part.PartNumber > MaximumParts || !safeProviderValue(part.ETag, 1024) {
			return nil, "", fmt.Errorf("%w: completed part is invalid", control.ErrInvalid)
		}
		if index > 0 && ordered[index-1].PartNumber == part.PartNumber {
			return nil, "", fmt.Errorf("%w: completed part numbers must be unique", control.ErrInvalid)
		}
		if part.SHA256 != "" {
			if err := validateDigest(part.SHA256); err != nil {
				return nil, "", err
			}
		}
	}
	encoded, _ := json.Marshal(ordered)
	sum := sha256.Sum256(encoded)
	return ordered, hex.EncodeToString(sum[:]), nil
}

func validateCompletionShape(size int64, reserved []manageddata.S3MultipartPart, completed []CompletedPart) error {
	byNumber := make(map[int32]manageddata.S3MultipartPart, len(reserved))
	for _, part := range reserved {
		byNumber[part.PartNumber] = part
	}
	var total int64
	for index, part := range completed {
		reservation, exists := byNumber[part.PartNumber]
		if !exists || reservation.SHA256 != "" && reservation.SHA256 != part.SHA256 {
			return fmt.Errorf("%w: completed part does not match its signed request", control.ErrInvalid)
		}
		if index < len(completed)-1 && reservation.SizeBytes < MinimumPartSize {
			return fmt.Errorf("%w: every non-final S3 part must be at least %d bytes", control.ErrInvalid, MinimumPartSize)
		}
		if total > size-reservation.SizeBytes {
			return fmt.Errorf("%w: completed part sizes do not match the file", control.ErrInvalid)
		}
		total += reservation.SizeBytes
	}
	if total != size {
		return fmt.Errorf("%w: completed part sizes do not match the file", control.ErrInvalid)
	}
	return nil
}

func responseHeaders(headers map[string][]string) ([]Header, error) {
	names := make([]string, 0, len(headers))
	count := 0
	for name, values := range headers {
		if !safeProviderValue(name, 256) || len(values) == 0 {
			return nil, control.ErrBackend
		}
		names = append(names, name)
		count += len(values)
	}
	if count > 32 {
		return nil, control.ErrBackend
	}
	sort.Strings(names)
	result := make([]Header, 0, count)
	for _, name := range names {
		for _, value := range headers[name] {
			if !safeProviderValue(value, 8192) {
				return nil, control.ErrBackend
			}
			result = append(result, Header{Name: name, Value: value})
		}
	}
	return result, nil
}

func resultFor(upload manageddata.S3MultipartUpload, session manageddata.UploadSession, file manageddata.File) (UploadResult, error) {
	var status Status
	switch upload.Status {
	case manageddata.S3MultipartStatusOpen:
		status = StatusOpen
	case manageddata.S3MultipartStatusCompleted:
		status = StatusCompleted
	case manageddata.S3MultipartStatusAborted:
		status = StatusAborted
	default:
		return UploadResult{}, fmt.Errorf("%w: multipart upload transition is incomplete", control.ErrConflict)
	}
	return UploadResult{ID: upload.ID, UploadSessionID: upload.UploadSessionID, File: file, Status: status, Existing: upload.Existing, CreatedAt: upload.CreatedAt, ExpiresAt: session.ExpiresAt}, nil
}

func providerUpload(upload manageddata.S3MultipartUpload) storage.MultipartUpload {
	return storage.MultipartUpload{UploadID: upload.ProviderUploadID, SHA256: upload.SHA256, Size: upload.SizeBytes, Key: upload.ObjectKey, Existing: upload.Existing}
}

func validateScopeValue(name, value string) error {
	if len(value) < 1 || len(value) > 128 || value != strings.TrimSpace(value) {
		return fmt.Errorf("%w: %s is not canonical", control.ErrInvalid, name)
	}
	for index, char := range value {
		if char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || index > 0 && (char == '.' || char == '_' || char == '-') {
			continue
		}
		return fmt.Errorf("%w: %s is not canonical", control.ErrInvalid, name)
	}
	return nil
}

func validateIdentity(name, value string, max int) error {
	if value == "" || len(value) > max || value != strings.TrimSpace(value) || !safeProviderValue(value, max) {
		return fmt.Errorf("%w: %s is invalid", control.ErrInvalid, name)
	}
	return nil
}

func validateIdempotencyKey(value string) error {
	return validateIdentity("idempotency key", value, 255)
}

func validateDigest(value string) error {
	if err := storage.ValidateSHA256(value); err != nil {
		return fmt.Errorf("%w: SHA-256 is invalid", control.ErrInvalid)
	}
	return nil
}

func safeProviderValue(value string, max int) bool {
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

func identityHash(operation string, values ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(operation))
	for _, value := range values {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func repositoryError(err error) error {
	switch {
	case errors.Is(err, manageddata.ErrNotFound):
		return control.ErrNotFound
	case errors.Is(err, manageddata.ErrConflict):
		return control.ErrConflict
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return control.ErrInternal
	}
}

func storageError(err error) error {
	switch {
	case errors.Is(err, storage.ErrInvalid):
		return control.ErrInvalid
	case errors.Is(err, storage.ErrNotFound):
		return control.ErrNotFound
	case errors.Is(err, storage.ErrIntegrity):
		return control.ErrIntegrity
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return control.ErrBackend
	}
}
