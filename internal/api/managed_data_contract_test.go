package api_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apigen "github.com/Yacobolo/libredash/internal/api/gen"
	cligen "github.com/Yacobolo/libredash/internal/cli/gen"
)

func TestManagedDataAPIContractIsProjectGlobalAndComplete(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	paths := openAPIMap(t, spec, "paths")

	operations := map[string]struct {
		method      string
		operationID string
	}{
		"/api/v1/projects/{project}/data-connections/{connection}/environments/{environment}/revision":                                                            {"get", "getManagedDataEnvironmentRevision"},
		"/api/v1/projects/{project}/data-connections/{connection}/revisions":                                                                                      {"get", "listManagedDataRevisions"},
		"/api/v1/projects/{project}/data-connections/{connection}/revisions/{revision}":                                                                           {"get", "getManagedDataRevision"},
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions":                                                                                {"post", "createManagedDataUploadSession"},
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}":                                                                {"get", "getManagedDataUploadSession"},
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/abort":                                                          {"post", "abortManagedDataUploadSession"},
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/finalize":                                                       {"post", "finalizeManagedDataUploadSession"},
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/s3-multipart-uploads":                                           {"post", "createManagedDataS3MultipartUpload"},
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/s3-multipart-uploads/{multipartUpload}/parts/{partNumber}/sign": {"post", "signManagedDataS3MultipartPart"},
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/s3-multipart-uploads/{multipartUpload}/complete":                {"post", "completeManagedDataS3MultipartUpload"},
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/s3-multipart-uploads/{multipartUpload}/abort":                   {"post", "abortManagedDataS3MultipartUpload"},
	}

	for path, want := range operations {
		operation := openAPIOperation(t, paths, path, want.method)
		if got := operation["operationId"]; got != want.operationID {
			t.Fatalf("%s %s operationId = %#v, want %q", want.method, path, got, want.operationID)
		}
		if strings.Contains(path, "/workspaces/") {
			t.Fatalf("managed-data control route is workspace-scoped: %s", path)
		}
		for _, parameter := range []string{"project", "connection"} {
			if !operationHasParameter(operation, "path", parameter) {
				t.Fatalf("%s %s missing %s path parameter", want.method, path, parameter)
			}
		}
	}

	for _, tc := range []struct {
		path   string
		status string
	}{
		{"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions", "201"},
		{"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/finalize", "202"},
		{"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/s3-multipart-uploads", "201"},
	} {
		operation := openAPIOperation(t, paths, tc.path, "post")
		responses := openAPIMap(t, operation, "responses")
		if _, ok := responses[tc.status]; !ok {
			t.Fatalf("POST %s missing success response %s", tc.path, tc.status)
		}
	}

	for _, path := range []string{"/api/v1/projects/{project}/data-connections/{connection}/revisions"} {
		operation := openAPIOperation(t, paths, path, "get")
		for _, parameter := range []string{"limit", "pageToken"} {
			if !operationHasParameter(operation, "query", parameter) {
				t.Fatalf("GET %s missing pagination parameter %s", path, parameter)
			}
		}
	}

	for _, path := range []string{
		"/api/v1/workspaces/{workspace}/data-connections/{connection}/revisions",
		"/api/v1/projects/{project}/workspaces/{workspace}/data-connections/{connection}/revisions",
		"/api/v1/projects/{project}/data-connections/{connection}/tus",
		"/api/v1/projects/{project}/data-connections/{connection}/rollouts",
	} {
		if _, exists := paths[path]; exists {
			t.Fatalf("managed-data API must not expose forbidden route %s", path)
		}
	}
}

func TestManagedDataAPIMutationsDeclareIdempotency(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	paths := openAPIMap(t, spec, "paths")

	for _, path := range []string{
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions",
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/abort",
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/finalize",
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/s3-multipart-uploads",
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/s3-multipart-uploads/{multipartUpload}/complete",
		"/api/v1/projects/{project}/data-connections/{connection}/upload-sessions/{uploadSession}/s3-multipart-uploads/{multipartUpload}/abort",
	} {
		operation := openAPIOperation(t, paths, path, "post")
		if !operationHasParameter(operation, "header", "Idempotency-Key") {
			t.Fatalf("POST %s missing Idempotency-Key header", path)
		}
	}
}

func TestManagedDataAPIModelsAreBoundedAndBackendNeutral(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	components := openAPIMap(t, spec, "components")
	schemas := openAPIMap(t, components, "schemas")

	revisionID := schemaProperty(t, openAPISchema(t, schemas, "ManagedDataRevisionResponse"), "id")
	revisionDescription, _ := revisionID["description"].(string)
	if revisionID["minLength"] != float64(71) || revisionID["maxLength"] != float64(71) || !strings.Contains(revisionDescription, "64 lowercase hexadecimal") {
		t.Fatalf("revision identity contract = %#v", revisionID)
	}
	fileHash := schemaProperty(t, openAPISchema(t, schemas, "ManagedDataFileMetadata"), "sha256")
	fileHashDescription, _ := fileHash["description"].(string)
	if fileHash["minLength"] != float64(64) || fileHash["maxLength"] != float64(64) || !strings.Contains(fileHashDescription, "lowercase 64-character hexadecimal") {
		t.Fatalf("file sha256 contract = %#v", fileHash)
	}

	manifest := openAPISchema(t, schemas, "ManagedDataManifest")
	files := schemaProperty(t, manifest, "files")
	if files["type"] != "array" {
		t.Fatalf("manifest files schema = %#v", files)
	}
	parts := schemaProperty(t, openAPISchema(t, schemas, "ManagedDataS3MultipartCompleteRequest"), "parts")
	if parts["type"] != "array" {
		t.Fatalf("multipart parts schema = %#v", parts)
	}
	for schemaName, itemRef := range map[string]string{
		"ManagedDataRevisionListResponse": "#/components/schemas/ManagedDataRevisionSummaryResponse",
	} {
		listItems := schemaProperty(t, openAPISchema(t, schemas, schemaName), "items")
		itemSchema := openAPIMap(t, listItems, "items")
		if itemSchema["$ref"] != itemRef {
			t.Fatalf("%s item schema = %#v, want %s", schemaName, itemSchema, itemRef)
		}
	}

	typespec, err := os.ReadFile(filepath.Join("..", "..", "api", "typespec", "managed_data.tsp"))
	if err != nil {
		t.Fatalf("read managed-data TypeSpec: %v", err)
	}
	for _, bounds := range []string{
		"@minItems(1)\n  @maxItems(10000)\n  files:",
		"@minItems(1)\n  @maxItems(10000)\n  parts:",
	} {
		if !strings.Contains(string(typespec), bounds) {
			t.Fatalf("managed-data TypeSpec missing array bounds %q", bounds)
		}
	}

	protocol := openAPISchema(t, schemas, "ManagedDataUploadProtocol")
	assertEnum(t, protocol, "tus", "s3_multipart", "already_present")
	assertEnum(t, openAPISchema(t, schemas, "ManagedDataRevisionStatus"), "available")
	assertEnum(t, openAPISchema(t, schemas, "ManagedDataUploadSessionStatus"), "open", "finalizing", "completed", "aborted", "failed", "expired")
	assertEnum(t, openAPISchema(t, schemas, "ManagedDataFileUploadStatus"), "pending", "uploading", "uploaded", "verified", "skipped", "failed")
	assertEnum(t, openAPISchema(t, schemas, "ManagedDataS3MultipartStatus"), "open", "completed", "aborted")

	negotiation := openAPISchema(t, schemas, "ManagedDataUploadNegotiation")
	for _, property := range []string{"protocol", "tus", "s3Multipart"} {
		_ = schemaProperty(t, negotiation, property)
	}
	encoded := strings.ToLower(string(mustJSON(t, negotiation)))
	for _, forbidden := range []string{"accesskeyid", "secretaccesskey", "sessiontoken", "password"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("upload negotiation exposes credential field %q", forbidden)
		}
	}
}

func TestManagedDataAPIDoesNotGenerateHighLevelCLICommands(t *testing.T) {
	managedOperationIDs := map[string]bool{}
	for operationID := range apigen.GetAPIGenOperationContracts() {
		if strings.Contains(operationID, "ManagedData") {
			managedOperationIDs[operationID] = true
		}
	}
	if len(managedOperationIDs) != 11 {
		t.Fatalf("managed-data generated operations = %d, want 11", len(managedOperationIDs))
	}
	for _, command := range cligen.APIGeneratedCommandSpecs {
		if managedOperationIDs[command.OperationID] {
			t.Fatalf("managed-data operation %s unexpectedly generated high-level CLI command %v", command.OperationID, command.Command)
		}
	}
}

func managedDataOpenAPISpec(t *testing.T) map[string]any {
	t.Helper()
	spec, err := apigen.GetEmbeddedOpenAPISpec()
	if err != nil {
		t.Fatalf("embedded openapi: %v", err)
	}
	return spec
}

func openAPIMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI %s missing or not an object: %#v", key, parent[key])
	}
	return value
}

func openAPIOperation(t *testing.T, paths map[string]any, path, method string) map[string]any {
	t.Helper()
	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI path missing: %s", path)
	}
	operation, ok := pathItem[method].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI operation missing: %s %s", method, path)
	}
	return operation
}

func openAPISchema(t *testing.T, schemas map[string]any, name string) map[string]any {
	t.Helper()
	schema, ok := schemas[name].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI schema missing: %s", name)
	}
	return schema
}

func schemaProperty(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()
	properties := openAPIMap(t, schema, "properties")
	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI schema property missing: %s", name)
	}
	return property
}

func operationHasParameter(operation map[string]any, location, name string) bool {
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter, _ := raw.(map[string]any)
		if parameter["in"] == location && parameter["name"] == name {
			return true
		}
	}
	return false
}

func assertEnum(t *testing.T, schema map[string]any, values ...string) {
	t.Helper()
	raw, ok := schema["enum"].([]any)
	if !ok {
		t.Fatalf("schema enum missing: %#v", schema)
	}
	if len(raw) != len(values) {
		t.Fatalf("enum = %#v, want %v", raw, values)
	}
	for index, value := range values {
		if raw[index] != value {
			t.Fatalf("enum = %#v, want %v", raw, values)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	return encoded
}
