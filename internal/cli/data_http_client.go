package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
)

type managedDataCLIClient struct {
	http   *http.Client
	target string
	token  string
}

func newManagedDataCLIClient(client *http.Client, target, token string) *managedDataCLIClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &managedDataCLIClient{http: client, target: strings.TrimRight(target, "/"), token: token}
}

func (c *managedDataCLIClient) createUploadSession(ctx context.Context, project, connection, key string, body apigenapi.ManagedDataUploadSessionCreateRequest) (apigenapi.ManagedDataUploadSessionResponse, error) {
	var response apigenapi.ManagedDataUploadSessionResponse
	err := c.json(ctx, http.MethodPost, "createManagedDataUploadSession", map[string]string{"project": project, "connection": connection}, nil, key, body, &response)
	return response, err
}

func (c *managedDataCLIClient) finalizeUploadSession(ctx context.Context, project, connection, uploadID, key string) (apigenapi.ManagedDataUploadSessionResponse, error) {
	var response apigenapi.ManagedDataUploadSessionResponse
	err := c.json(ctx, http.MethodPost, "finalizeManagedDataUploadSession", managedDataUploadPath(project, connection, uploadID), nil, key, nil, &response)
	return response, err
}

func (c *managedDataCLIClient) abortUploadSession(ctx context.Context, project, connection, uploadID, key string) {
	var response apigenapi.ManagedDataUploadSessionResponse
	_ = c.json(ctx, http.MethodPost, "abortManagedDataUploadSession", managedDataUploadPath(project, connection, uploadID), nil, key, nil, &response)
}

func (c *managedDataCLIClient) createMultipart(ctx context.Context, project, connection, uploadID, key, logicalPath string) (apigenapi.ManagedDataS3MultipartUploadResponse, error) {
	var response apigenapi.ManagedDataS3MultipartUploadResponse
	err := c.json(ctx, http.MethodPost, "createManagedDataS3MultipartUpload", managedDataUploadPath(project, connection, uploadID), nil, key, apigenapi.ManagedDataS3MultipartCreateRequest{Path: logicalPath}, &response)
	return response, err
}

func (c *managedDataCLIClient) signMultipartPart(ctx context.Context, project, connection, uploadID, multipartID string, partNumber int32, body apigenapi.ManagedDataS3MultipartSignPartRequest) (apigenapi.ManagedDataS3MultipartSignedPartResponse, error) {
	params := managedDataMultipartPath(project, connection, uploadID, multipartID)
	params["partNumber"] = strconv.FormatInt(int64(partNumber), 10)
	var response apigenapi.ManagedDataS3MultipartSignedPartResponse
	err := c.json(ctx, http.MethodPost, "signManagedDataS3MultipartPart", params, nil, "", body, &response)
	return response, err
}

func (c *managedDataCLIClient) completeMultipart(ctx context.Context, project, connection, uploadID, multipartID, key string, body apigenapi.ManagedDataS3MultipartCompleteRequest) (apigenapi.ManagedDataS3MultipartUploadResponse, error) {
	var response apigenapi.ManagedDataS3MultipartUploadResponse
	err := c.json(ctx, http.MethodPost, "completeManagedDataS3MultipartUpload", managedDataMultipartPath(project, connection, uploadID, multipartID), nil, key, body, &response)
	return response, err
}

func (c *managedDataCLIClient) abortMultipart(ctx context.Context, project, connection, uploadID, multipartID, key string) {
	var response apigenapi.ManagedDataS3MultipartUploadResponse
	_ = c.json(ctx, http.MethodPost, "abortManagedDataS3MultipartUpload", managedDataMultipartPath(project, connection, uploadID, multipartID), nil, key, nil, &response)
}

func (c *managedDataCLIClient) listRevisions(ctx context.Context, project, connection string, query url.Values) (apigenapi.ManagedDataRevisionListResponse, error) {
	var response apigenapi.ManagedDataRevisionListResponse
	err := c.json(ctx, http.MethodGet, "listManagedDataRevisions", map[string]string{"project": project, "connection": connection}, query, "", nil, &response)
	return response, err
}

func (c *managedDataCLIClient) currentRevision(ctx context.Context, project, connection, environment string) (apigenapi.ManagedDataEnvironmentRevisionResponse, error) {
	var response apigenapi.ManagedDataEnvironmentRevisionResponse
	err := c.json(ctx, http.MethodGet, "getManagedDataEnvironmentRevision", map[string]string{"project": project, "connection": connection, "environment": environment}, nil, "", nil, &response)
	return response, err
}

func (c *managedDataCLIClient) createProjectDeployment(ctx context.Context, project, key string, body apigenapi.ProjectDeploymentCreateRequest) (apigenapi.ProjectDeploymentResponse, error) {
	var response apigenapi.ProjectDeploymentResponse
	err := c.json(ctx, http.MethodPost, "createProjectDeployment", map[string]string{"project": project}, nil, key, body, &response)
	return response, err
}

func (c *managedDataCLIClient) activateProjectDeployment(ctx context.Context, project, deploymentID, key string) (apigenapi.ProjectDeploymentResponse, error) {
	var response apigenapi.ProjectDeploymentResponse
	err := c.json(ctx, http.MethodPost, "activateProjectDeployment", map[string]string{"project": project, "deployment": deploymentID}, nil, key, nil, &response)
	return response, err
}

func (c *managedDataCLIClient) json(ctx context.Context, method, operation string, pathParams map[string]string, query url.Values, idempotencyKey string, body, out any) error {
	endpoint, err := apiOperationURL(c.target, operation, pathParams, query)
	if err != nil {
		return fmt.Errorf("build managed-data request: %w", err)
	}
	return c.jsonEndpoint(ctx, method, endpoint, operation, idempotencyKey, body, out)
}

func (c *managedDataCLIClient) jsonEndpoint(ctx context.Context, method, endpoint, operation, idempotencyKey string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode managed-data request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("build managed-data request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("operation %s could not reach the server", operation)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return fmt.Errorf("operation %s failed with HTTP %d", operation, response.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode operation %s response: %w", operation, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("decode operation %s response: trailing data", operation)
	}
	return nil
}

func managedDataUploadPath(project, connection, uploadID string) map[string]string {
	return map[string]string{"project": project, "connection": connection, "uploadSession": uploadID}
}

func managedDataMultipartPath(project, connection, uploadID, multipartID string) map[string]string {
	params := managedDataUploadPath(project, connection, uploadID)
	params["multipartUpload"] = multipartID
	return params
}
