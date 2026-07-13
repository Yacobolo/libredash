package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
)

func TestAPIGenUsesTypeSpecV040(t *testing.T) {
	root := projectRoot(t)
	manifest, err := os.ReadFile(filepath.Join(root, "api", "apigen.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestText := string(manifest)
	if !strings.Contains(manifestText, "typespec_dir: typespec") {
		t.Fatalf("manifest should use TypeSpec source, got:\n%s", manifestText)
	}
	if strings.Contains(manifestText, "cue_dir:") {
		t.Fatalf("manifest should not use cue_dir after APIGen v0.3.0 migration")
	}

	taskfile, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatalf("read Taskfile.yml: %v", err)
	}
	taskText := string(taskfile)
	for _, want := range []string{
		"github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.4.0 typespec-compile",
		"github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.4.0 all",
	} {
		if !strings.Contains(taskText, want) {
			t.Fatalf("Taskfile.yml missing generation command %q", want)
		}
	}
	for _, forbidden := range []string{"cue-compile", "apigen@v0.2.0", "apigen@v0.3.0", "apigen@v0.3.2", "apigen@v0.3.3"} {
		if strings.Contains(taskText, forbidden) {
			t.Fatalf("Taskfile.yml should not contain %q after APIGen v0.4.0 migration", forbidden)
		}
	}

	ir, err := os.ReadFile(filepath.Join(root, "api", "gen", "json-ir.json"))
	if err != nil {
		t.Fatalf("read APIGen IR: %v", err)
	}
	var irDoc map[string]any
	if err := json.Unmarshal(ir, &irDoc); err != nil {
		t.Fatalf("decode APIGen IR: %v", err)
	}
	if got := irDoc["schema_version"]; got != "v3" {
		t.Fatalf("APIGen IR schema_version = %#v, want v3", got)
	}
}

func TestAPIGenRoutesCoverHeadlessAPINotUITransports(t *testing.T) {
	spec, err := apigenapi.GetEmbeddedOpenAPISpec()
	if err != nil {
		t.Fatalf("embedded openapi: %v", err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("openapi paths missing: %#v", spec["paths"])
	}

	for _, path := range []string{
		"/api/v1/me",
		"/api/v1/me/effective-privileges",
		"/api/v1/me/api-tokens",
		"/api/v1/me/api-tokens/{token}",
		"/api/v1/me/sessions",
		"/api/v1/me/sessions/{session}",
		"/api/v1/principals",
		"/api/v1/principals/{principal}",
		"/api/v1/workspaces",
		"/api/v1/workspaces/{workspace}/search",
		"/api/v1/workspaces/{workspace}/assets",
		"/api/v1/workspaces/{workspace}/asset-edges",
		"/api/v1/workspaces/{workspace}/dashboards",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/components",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/visuals/{visual}",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/visuals/{visual}/data",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/tables/{table}/data",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}/options",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/query",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/tables/{table}/query",
		"/api/v1/workspaces/{workspace}/semantic-models",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/fields",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/query",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/preview",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/query/explain",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/preview/explain",
		"/api/v1/workspaces/{workspace}/publishes",
		"/api/v1/workspaces/{workspace}/publishes/{publish}",
		"/api/v1/workspaces/{workspace}/publishes/{publish}/artifact",
		"/api/v1/workspaces/{workspace}/publishes/{publish}/validate",
		"/api/v1/workspaces/{workspace}/publishes/{publish}/activate",
		"/api/v1/workspaces/{workspace}/refresh-runs",
		"/api/v1/workspaces/{workspace}/refresh-runs/{run}",
		"/api/v1/workspaces/{workspace}/agent/conversations",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/messages",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/turns",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}/events",
		"/api/v1/admin/agent/config",
		"/api/v1/principals",
		"/api/v1/principals/{principal}",
		"/api/v1/principals/{principal}/password-reset",
		"/api/v1/workspaces/{workspace}/roles",
		"/api/v1/workspaces/{workspace}/groups",
		"/api/v1/workspaces/{workspace}/groups/{group}",
		"/api/v1/workspaces/{workspace}/groups/{group}/members",
		"/api/v1/workspaces/{workspace}/groups/{group}/members/{principal}",
		"/api/v1/workspaces/{workspace}/role-bindings",
		"/api/v1/workspaces/{workspace}/role-bindings/{binding}",
		"/api/v1/workspaces/{workspace}/audit-events",
	} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("generated OpenAPI missing path %s", path)
		}
	}

	for _, path := range []string{"/api/workspaces", "/api/publishes", "/api/v1/workspaces/{workspace}/publishes/{publish}/rollback", "/updates", "/commands/select", "/workspaces/{workspace}/chat/updates", "/dashboards/{dashboard}"} {
		if _, ok := paths[path]; ok {
			t.Fatalf("generated OpenAPI should not include UI transport path %s", path)
		}
	}
	if _, ok := paths["/api/v1/me/permissions"]; ok {
		t.Fatal("generated OpenAPI still includes removed /api/v1/me/permissions path")
	}
}

func TestAPIGenOperationAuthCoverage(t *testing.T) {
	contracts := apigenapi.GetAPIGenOperationContracts()
	if len(contracts) == 0 {
		t.Fatal("no generated operation contracts")
	}
	for operationID, contract := range contracts {
		if contract.AuthzMode != "privilege" || !contract.Protected {
			t.Fatalf("%s auth contract = mode %q protected %t, want privilege/protected", operationID, contract.AuthzMode, contract.Protected)
		}
		if _, ok := apigenOperationPrivileges[operationID]; !ok {
			t.Fatalf("%s missing app privilege mapping", operationID)
		}
	}
	for operationID := range apigenOperationPrivileges {
		if _, ok := contracts[operationID]; !ok {
			t.Fatalf("%s has app privilege mapping but no generated contract", operationID)
		}
	}
	if got := apigenOperationPrivileges["uploadPublishArtifact"]; got != access.PrivilegeDeploy {
		t.Fatalf("uploadPublishArtifact privilege = %q, want %q", got, access.PrivilegeDeploy)
	}
}

func TestAPIGenOperationObjectResolverCoverage(t *testing.T) {
	contracts := apigenapi.GetAPIGenOperationContracts()
	objectScopedOperations := []string{
		"getWorkspaceAsset",
		"getWorkspaceAssetLineage",
		"listWorkspaceAssetEdges",
		"getDashboard",
		"listDashboardComponents",
		"getDashboardVisual",
		"queryDashboardPage",
		"queryDashboardVisualData",
		"queryDashboardTable",
		"queryDashboardTableData",
		"listDashboardFilterOptions",
		"getSemanticModel",
		"listSemanticDatasets",
		"getSemanticDataset",
		"listSemanticFields",
		"querySemanticDataset",
		"previewSemanticDataset",
		"explainSemanticQuery",
		"explainSemanticPreview",
		"getAgentConversation",
		"updateAgentConversation",
		"archiveAgentConversation",
		"listAgentMessages",
		"createAgentTurn",
		"listAgentRuns",
		"getAgentRun",
		"listAgentEvents",
	}
	for _, operationID := range objectScopedOperations {
		if _, ok := contracts[operationID]; !ok {
			t.Fatalf("%s missing generated contract", operationID)
		}
		if _, ok := apigenOperationPrivileges[operationID]; !ok {
			t.Fatalf("%s missing privilege mapping", operationID)
		}
		if apigenOperationObjectResolvers[operationID] == nil {
			t.Fatalf("%s missing exact object resolver", operationID)
		}
	}
	for operationID := range apigenOperationObjectResolvers {
		if _, ok := contracts[operationID]; !ok {
			t.Fatalf("%s has object resolver but no generated contract", operationID)
		}
		if _, ok := apigenOperationPrivileges[operationID]; !ok {
			t.Fatalf("%s has object resolver but no privilege mapping", operationID)
		}
	}
	for _, operationID := range []string{
		"listWorkspaceAssets",
		"listDashboards",
		"listSemanticModels",
		"createAgentConversation",
		"listAgentConversations",
		"createPublish",
		"listPublishes",
		"createRefreshRun",
		"listRefreshRuns",
	} {
		if apigenOperationObjectResolvers[operationID] != nil {
			t.Fatalf("%s should stay workspace-scoped and not use an exact object resolver", operationID)
		}
	}
}

func TestAPIGenOperationExtensions(t *testing.T) {
	contracts := apigenapi.GetAPIGenOperationContracts()
	toolContracts := apigenapi.GetAPIGenToolContracts()
	toolsByOperation := make(map[string]string, len(toolContracts))
	for name, tool := range toolContracts {
		toolsByOperation[tool.OperationID] = name
	}
	agentTools := map[string]string{
		"getPublish":                 "get_publish",
		"getDashboard":               "describe_dashboard",
		"getDashboardVisual":         "describe_dashboard_visual",
		"getWorkspaceAsset":          "describe_asset",
		"getWorkspaceAssetLineage":   "asset_lineage",
		"getSemanticModel":           "describe_model",
		"getSemanticDataset":         "describe_semantic_dataset",
		"getRefreshRun":              "get_refresh_run",
		"listDashboardComponents":    "list_dashboard_components",
		"listDashboards":             "list_dashboards",
		"listPublishes":              "list_publishes",
		"listDashboardFilterOptions": "list_dashboard_filter_options",
		"listRefreshRuns":            "list_refresh_runs",
		"listSemanticDatasets":       "list_semantic_datasets",
		"listSemanticFields":         "list_semantic_fields",
		"listSemanticModels":         "list_semantic_models",
		"listWorkspaceAssetEdges":    "list_workspace_asset_edges",
		"listWorkspaceAssets":        "list_assets",
		"listWorkspaces":             "list_workspaces",
		"searchWorkspace":            "search_workspace",
		"queryDashboardTableData":    "query_dashboard_table_data",
		"queryDashboardVisualData":   "query_dashboard_visual_data",
		"queryDashboardPage":         "query_dashboard_page",
		"queryDashboardTable":        "query_table",
		"querySemanticDataset":       "query_semantic_dataset",
		"previewSemanticDataset":     "preview_semantic_dataset",
		"explainSemanticQuery":       "explain_semantic_query",
		"explainSemanticPreview":     "explain_semantic_preview",
	}
	for operationID, contract := range contracts {
		authz, ok := contract.Extensions["x-authz"].(map[string]any)
		if !ok {
			t.Fatalf("%s missing generated x-authz extension: %#v", operationID, contract.Extensions["x-authz"])
		}
		if got := authz["mode"]; got != "privilege" {
			t.Fatalf("%s x-authz mode = %#v, want privilege", operationID, got)
		}
		if got := authz["privilege"]; got != string(apigenOperationPrivileges[operationID]) {
			t.Fatalf("%s x-authz privilege = %#v, want %q", operationID, got, apigenOperationPrivileges[operationID])
		}
		if wantName, ok := agentTools[operationID]; ok {
			if got := toolsByOperation[operationID]; got != wantName {
				t.Fatalf("%s generated tool name = %q, want %q", operationID, got, wantName)
			}
		} else if name := toolsByOperation[operationID]; name != "" {
			t.Fatalf("%s should not have generated tool %q", operationID, name)
		}
		if _, hasLegacy := contract.Extensions["x-agent"]; hasLegacy {
			t.Fatalf("%s retained legacy x-agent metadata", operationID)
		}
		if _, ok := contract.Extensions["x-libredash-dispatch"]; ok {
			t.Fatalf("%s should not have raw-body dispatch extension", operationID)
		}
	}
}

func TestAPIGenUploadArtifactUsesNativeOctetStreamBody(t *testing.T) {
	spec, err := apigenapi.GetEmbeddedOpenAPISpec()
	if err != nil {
		t.Fatalf("embedded openapi: %v", err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("openapi paths missing: %#v", spec["paths"])
	}
	operation := mustOpenAPIOperation(t, paths, "/api/v1/workspaces/{workspace}/publishes/{publish}/artifact", "put")
	if _, ok := operation["x-libredash-dispatch"]; ok {
		t.Fatalf("upload operation should not use x-libredash-dispatch: %#v", operation["x-libredash-dispatch"])
	}
	requestBody, _ := operation["requestBody"].(map[string]any)
	content, _ := requestBody["content"].(map[string]any)
	octetStream, ok := content["application/octet-stream"].(map[string]any)
	if !ok {
		t.Fatalf("upload operation missing application/octet-stream request body: %#v", requestBody)
	}
	schema, _ := octetStream["schema"].(map[string]any)
	if schema == nil {
		t.Fatalf("upload operation missing application/octet-stream schema: %#v", octetStream)
	}
	if got := schema["type"]; got != "string" {
		t.Fatalf("upload operation schema type = %#v, want string", got)
	}
	if got := schema["format"]; got != "binary" {
		t.Fatalf("upload operation schema format = %#v, want binary", got)
	}

	root := projectRoot(t)
	ir, err := os.ReadFile(filepath.Join(root, "api", "gen", "json-ir.json"))
	if err != nil {
		t.Fatalf("read APIGen IR: %v", err)
	}
	var irDoc struct {
		Endpoints []struct {
			OperationID string `json:"operation_id"`
			RequestBody *struct {
				Contents []struct {
					ContentType string `json:"content_type"`
					BodyKind    string `json:"body_kind"`
				} `json:"contents"`
			} `json:"request_body"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(ir, &irDoc); err != nil {
		t.Fatalf("decode APIGen IR: %v", err)
	}
	for _, endpoint := range irDoc.Endpoints {
		if endpoint.OperationID != "uploadPublishArtifact" {
			continue
		}
		if endpoint.RequestBody == nil || len(endpoint.RequestBody.Contents) != 1 {
			t.Fatalf("upload IR request body = %#v", endpoint.RequestBody)
		}
		content := endpoint.RequestBody.Contents[0]
		if content.ContentType != "application/octet-stream" || content.BodyKind != "binary" {
			t.Fatalf("upload IR content = %#v, want application/octet-stream binary", content)
		}
		var generatedBody apigenapi.GenUploadPublishArtifactBody
		_ = []byte(generatedBody)
		return
	}
	t.Fatal("uploadPublishArtifact missing from APIGen IR")
}

func TestAPIGenListOperationsUseStandardEnvelope(t *testing.T) {
	spec, err := apigenapi.GetEmbeddedOpenAPISpec()
	if err != nil {
		t.Fatalf("embedded openapi: %v", err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("openapi paths missing: %#v", spec["paths"])
	}
	components, _ := spec["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	for _, tc := range []struct {
		path   string
		method string
	}{
		{"/api/v1/workspaces", "get"},
		{"/api/v1/workspaces/{workspace}/search", "get"},
		{"/api/v1/workspaces/{workspace}/assets", "get"},
		{"/api/v1/workspaces/{workspace}/asset-edges", "get"},
		{"/api/v1/workspaces/{workspace}/dashboards", "get"},
		{"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/components", "get"},
		{"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}/options", "post"},
		{"/api/v1/workspaces/{workspace}/semantic-models", "get"},
		{"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets", "get"},
		{"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/fields", "get"},
		{"/api/v1/workspaces/{workspace}/publishes", "get"},
		{"/api/v1/workspaces/{workspace}/refresh-runs", "get"},
		{"/api/v1/workspaces/{workspace}/agent/conversations", "get"},
	} {
		operation := mustOpenAPIOperation(t, paths, tc.path, tc.method)
		for _, want := range []string{"limit", "pageToken"} {
			if !openAPIOperationHasQueryParam(operation, want) {
				t.Fatalf("%s %s missing query param %s", tc.method, tc.path, want)
			}
		}
		schemaName := responseSchemaName(operation, "200")
		if schemaName == "" {
			t.Fatalf("%s %s missing 200 response schema", tc.method, tc.path)
		}
		schema, _ := schemas[schemaName].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		if _, ok := properties["items"]; !ok {
			t.Fatalf("%s %s schema %s missing items property: %#v", tc.method, tc.path, schemaName, properties)
		}
		if _, ok := properties["page"]; !ok {
			t.Fatalf("%s %s schema %s missing page property: %#v", tc.method, tc.path, schemaName, properties)
		}
		if _, ok := properties["dashboards"]; ok {
			t.Fatalf("%s %s schema %s has legacy dashboards property", tc.method, tc.path, schemaName)
		}
		if _, ok := properties["models"]; ok {
			t.Fatalf("%s %s schema %s has legacy models property", tc.method, tc.path, schemaName)
		}
	}
}

func mustOpenAPIOperation(t *testing.T, paths map[string]any, path, method string) map[string]any {
	t.Helper()
	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("path %s missing", path)
	}
	operation, ok := pathItem[method].(map[string]any)
	if !ok {
		t.Fatalf("%s operation missing for %s", method, path)
	}
	return operation
}

func openAPIOperationHasQueryParam(operation map[string]any, name string) bool {
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter, _ := raw.(map[string]any)
		if parameter["name"] == name && parameter["in"] == "query" {
			return true
		}
	}
	return false
}

func responseSchemaName(operation map[string]any, status string) string {
	responses, _ := operation["responses"].(map[string]any)
	response, _ := responses[status].(map[string]any)
	content, _ := response["content"].(map[string]any)
	jsonContent, _ := content["application/json"].(map[string]any)
	schema, _ := jsonContent["schema"].(map[string]any)
	ref, _ := schema["$ref"].(string)
	return strings.TrimPrefix(ref, "#/components/schemas/")
}

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("could not find project root from %s", dir)
		}
		dir = next
	}
}
