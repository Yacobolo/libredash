package cli

import (
	"context"
	"time"

	"github.com/Yacobolo/libredash/internal/manageddata/control"
	manageddatahttp "github.com/Yacobolo/libredash/internal/manageddata/http"
	"github.com/Yacobolo/libredash/internal/manageddata/s3multipart"
)

type managedDataMaintenance struct {
	uploads   *control.Service
	multipart *s3multipart.Service
	uploadTTL time.Duration
}

func (m managedDataMaintenance) ExpireUploads(ctx context.Context) (control.ExpireResult, error) {
	result, err := m.uploads.ExpireUploads(ctx)
	if err != nil || m.multipart == nil {
		return result, err
	}
	_, err = m.multipart.RecoverOrphaned(ctx, time.Now().UTC().Add(-m.uploadTTL), 100)
	return result, err
}

type managedDataMultipartHTTP struct{ service *s3multipart.Service }

func (a managedDataMultipartHTTP) Create(ctx context.Context, request manageddatahttp.MultipartCreateRequest) (manageddatahttp.MultipartUpload, error) {
	result, err := a.service.Create(ctx, s3multipart.CreateRequest{
		Project: request.Project, Connection: request.Connection, UploadSessionID: request.UploadSessionID,
		Path: request.File.Path, IdempotencyKey: request.IdempotencyKey,
	})
	return multipartUploadToHTTP(result), err
}

func (a managedDataMultipartHTTP) SignPart(ctx context.Context, request manageddatahttp.MultipartSignPartRequest) (manageddatahttp.MultipartSignedPart, error) {
	result, err := a.service.SignPart(ctx, s3multipart.SignPartRequest{
		Project: request.Project, Connection: request.Connection, UploadSessionID: request.UploadSessionID,
		MultipartUploadID: request.MultipartUploadID, PartNumber: request.PartNumber, Size: request.Size, SHA256: request.SHA256,
	})
	headers := make([]manageddatahttp.HTTPHeader, len(result.Headers))
	for i, header := range result.Headers {
		headers[i] = manageddatahttp.HTTPHeader{Name: header.Name, Value: header.Value}
	}
	return manageddatahttp.MultipartSignedPart{
		UploadSessionID: request.UploadSessionID, MultipartUploadID: request.MultipartUploadID,
		PartNumber: result.PartNumber, URL: result.URL, Headers: headers, ExpiresAt: result.ExpiresAt,
	}, err
}

func (a managedDataMultipartHTTP) Complete(ctx context.Context, request manageddatahttp.MultipartCompleteRequest) (manageddatahttp.MultipartUpload, error) {
	parts := make([]s3multipart.CompletedPart, len(request.Parts))
	for i, part := range request.Parts {
		parts[i] = s3multipart.CompletedPart{PartNumber: part.PartNumber, ETag: part.ETag, SHA256: part.SHA256}
	}
	result, err := a.service.Complete(ctx, s3multipart.CompleteRequest{
		Project: request.Project, Connection: request.Connection, UploadSessionID: request.UploadSessionID,
		MultipartUploadID: request.MultipartUploadID, IdempotencyKey: request.IdempotencyKey, Parts: parts,
	})
	return multipartUploadToHTTP(result), err
}

func (a managedDataMultipartHTTP) Abort(ctx context.Context, request manageddatahttp.MultipartRequest) (manageddatahttp.MultipartUpload, error) {
	result, err := a.service.Abort(ctx, s3multipart.AbortRequest{
		Project: request.Project, Connection: request.Connection, UploadSessionID: request.UploadSessionID,
		MultipartUploadID: request.MultipartUploadID, IdempotencyKey: request.IdempotencyKey,
	})
	return multipartUploadToHTTP(result), err
}

func multipartUploadToHTTP(result s3multipart.UploadResult) manageddatahttp.MultipartUpload {
	return manageddatahttp.MultipartUpload{
		ID: result.ID, UploadSessionID: result.UploadSessionID, File: result.File,
		Status: manageddatahttp.MultipartStatus(result.Status), Existing: result.Existing,
		CreatedAt: result.CreatedAt, ExpiresAt: result.ExpiresAt,
	}
}
