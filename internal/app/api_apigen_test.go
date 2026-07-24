package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	"github.com/Yacobolo/leapview/internal/workspace"
)

func TestServingSnapshotIsOwnedByServer(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{
		Store: testStore(t),
		WorkspaceRepo: apiSnapshotWorkspaceRepository{summary: workspace.Summary{
			ID: "sales", ActiveServingStateID: "state-current",
		}},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/sales/semantic-models/orders/query", nil)
	request.Header.Set("X-Serving-Snapshot", "state-attacker-controlled")

	apiGenAdapter{server: server}.setServingSnapshot(request, "sales")

	if got := request.Header.Get("X-Serving-Snapshot"); got != "state-current" {
		t.Fatalf("serving snapshot = %q, want server-owned state-current", got)
	}
}

type apiSnapshotWorkspaceRepository struct{ summary workspace.Summary }

func (r apiSnapshotWorkspaceRepository) Ensure(context.Context, workspace.EnsureInput) error {
	return nil
}
func (r apiSnapshotWorkspaceRepository) ByID(context.Context, workspace.WorkspaceID) (workspace.Summary, error) {
	return r.summary, nil
}
func (r apiSnapshotWorkspaceRepository) List(context.Context) ([]workspace.Summary, error) {
	return []workspace.Summary{r.summary}, nil
}
func (r apiSnapshotWorkspaceRepository) ActiveServingStateGraph(context.Context, workspace.WorkspaceID, string) (workspace.AssetGraph, bool, error) {
	return workspace.AssetGraph{}, false, nil
}
func (r apiSnapshotWorkspaceRepository) AssetVersions(context.Context, workspace.WorkspaceID, string, workspace.AssetID) ([]workspace.AssetVersion, error) {
	return nil, nil
}

func TestAPIGenUsesTypeSpecV065(t *testing.T) {
	root := projectRoot(t)
	manifest, err := os.ReadFile(filepath.Join(root, "api", "apigen.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestText := string(manifest)
	for _, source := range []string{"typespec_entrypoint: typespec/main.tsp", "typespec_entrypoint: signals/main.tsp", "typespec_entrypoint: visualization/main.tsp"} {
		if !strings.Contains(manifestText, source) {
			t.Fatalf("manifest should select shared-root TypeSpec source %q, got:\n%s", source, manifestText)
		}
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
		"- task: api:generate\n      - task: ui-signals:generate\n      - task: schema:generate",
		"schema:generate:\n    desc: Generate JSON Schema artifacts for LeapView YAML contracts\n    deps:\n      - db:generate\n      - config:generate\n      - api:generate\n      - ui-signals:generate",
	} {
		if !strings.Contains(taskText, want) {
			t.Fatalf("Taskfile.yml does not enforce generated-model ordering %q", want)
		}
	}
	for _, want := range []string{
		"github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.6.5 typespec-compile",
		"github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.6.5 all",
	} {
		if !strings.Contains(taskText, want) {
			t.Fatalf("Taskfile.yml missing generation command %q", want)
		}
	}
	for _, forbidden := range []string{"cue-compile", "apigen@v0.2.0", "apigen@v0.3.0", "apigen@v0.3.2", "apigen@v0.3.3", "apigen@v0.4.0", "apigen@v0.5.0", "apigen@v0.5.1", "apigen@v0.5.2", "apigen@v0.5.3", "apigen@v0.6.0", "apigen@v0.6.1", "apigen@v0.6.2", "apigen@v0.6.3", "apigen@v0.6.4", "apigenpostprocess"} {
		if strings.Contains(taskText, forbidden) {
			t.Fatalf("Taskfile.yml should not contain %q after APIGen v0.6.5 migration", forbidden)
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
	if got := irDoc["schema_version"]; got != "v4" {
		t.Fatalf("APIGen IR schema_version = %#v, want v4", got)
	}

	if _, err := os.Stat(filepath.Join(root, "internal", "tools", "apigenpostprocess")); !os.IsNotExist(err) {
		t.Fatalf("APIGen v0.6.5 should not require a postprocessor, stat error = %v", err)
	}
	for path, forbidden := range map[string]string{
		filepath.Join(root, "api", "typespec", "bi.tsp"):                        "toolbelt#34",
		filepath.Join(root, "internal", "agent", "tools", "apigen_provider.go"): "projectUnionToolResult",
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("APIGen v0.6.5 superseded workaround %q in %s", forbidden, path)
		}
	}
}

func TestAPIGenOwnsUISignalContracts(t *testing.T) {
	root := projectRoot(t)

	manifest, err := os.ReadFile(filepath.Join(root, "api", "apigen.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestText := string(manifest)
	for _, want := range []string{
		"name: ui-signals",
		"kind: contracts",
		"typespec_dir: .",
		"typespec_entrypoint: signals/main.tsp",
		"go_models_out: ../internal/ui/signals/models.gen.go",
		"ts_out: ../web/generated/signals/index.ts",
	} {
		if !strings.Contains(manifestText, want) {
			t.Fatalf("APIGen manifest missing UI signal contract setting %q", want)
		}
	}
	if strings.Contains(manifestText, "json_schema_out: ../schemas/signals/ui-signals.schema.json") {
		t.Fatal("APIGen manifest should not generate an unused UI signal JSON Schema")
	}

	taskfile, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatalf("read Taskfile.yml: %v", err)
	}
	taskText := string(taskfile)
	for _, want := range []string{
		"typespec-compile -manifest api/apigen.yaml -target ui-signals",
		"all -manifest api/apigen.yaml -target ui-signals",
	} {
		if !strings.Contains(taskText, want) {
			t.Fatalf("Taskfile.yml missing UI signal generation command %q", want)
		}
	}
	if strings.Contains(taskText, "go run ./internal/tools/uisignalsgen") {
		t.Fatal("Taskfile.yml still uses the Go reflection UI signal generator")
	}
	if strings.Contains(taskText, "schemas/signals/ui-signals.schema.json") {
		t.Fatal("Taskfile.yml should not track an unused UI signal JSON Schema")
	}

	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignore), "internal/ui/signals/models.gen.go") {
		t.Fatal("generated Go UI signal models should be ignored build output")
	}

	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	workflowText := string(workflow)
	if !strings.Contains(workflowText, "internal/ui/signals/models.gen.go") {
		t.Fatal("CI generated-assets artifact does not include Go UI signal models")
	}
	if strings.Contains(workflowText, "schemas/signals/") {
		t.Fatal("CI should not upload an unused UI signal JSON Schema")
	}

	typespec, err := os.ReadFile(filepath.Join(root, "api", "signals", "main.tsp"))
	if err != nil {
		t.Fatalf("read UI signal TypeSpec source: %v", err)
	}
	typespecText := string(typespec)
	for _, want := range []string{"@apigen.`package`", "@apigen.contract", "@apigen.`metadata`"} {
		if !strings.Contains(typespecText, want) {
			t.Fatalf("UI signal TypeSpec source missing %q", want)
		}
	}

	generatedGo, err := os.ReadFile(filepath.Join(root, "internal", "ui", "signals", "models.gen.go"))
	if err != nil {
		t.Fatalf("read generated Go UI signal models: %v", err)
	}
	if !strings.Contains(string(generatedGo), "Code generated by apigen data-contract Go emitter") {
		t.Fatal("UI signal Go models were not generated by APIGen data contracts")
	}

	if _, err := os.Stat(filepath.Join(root, "internal", "tools", "uisignalsgen")); !os.IsNotExist(err) {
		t.Fatalf("legacy UI signal reflection generator still exists: %v", err)
	}

	ir, err := os.ReadFile(filepath.Join(root, "api", "gen", "ui-signals-ir.json"))
	if err != nil {
		t.Fatalf("read UI signal contract IR: %v", err)
	}
	var irDoc struct {
		SchemaVersion string `json:"schema_version"`
		Schemas       map[string]struct {
			Namespace string `json:"namespace"`
		} `json:"schemas"`
		Contracts []struct {
			Name       string         `json:"name"`
			Kind       string         `json:"kind"`
			Extensions map[string]any `json:"extensions"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(ir, &irDoc); err != nil {
		t.Fatalf("decode UI signal contract IR: %v", err)
	}
	if irDoc.SchemaVersion != "v4" {
		t.Fatalf("UI signal IR schema_version = %q, want v4", irDoc.SchemaVersion)
	}
	if len(irDoc.Contracts) != 83 {
		t.Fatalf("UI signal IR contracts = %d, want 83", len(irDoc.Contracts))
	}
	foundEnvelopeMetadata := false
	foundImportedVisualizationRoot := false
	foundDashboardVisualizationSignal := false
	for _, contract := range irDoc.Contracts {
		if contract.Name == "DashboardEnvelope" && contract.Kind == "ui-envelope" && contract.Extensions["x-leapview-contract-role"] == "envelope" {
			foundEnvelopeMetadata = true
		}
		if contract.Name == "VisualizationEnvelope" {
			foundImportedVisualizationRoot = true
		}
		if contract.Name == "DashboardVisualizationSignal" && contract.Kind == "ui-signal" {
			foundDashboardVisualizationSignal = true
		}
	}
	if !foundEnvelopeMetadata {
		t.Fatal("DashboardEnvelope contract metadata was not preserved in IR")
	}
	if foundImportedVisualizationRoot {
		t.Fatal("UI signal contract roots must not duplicate imported visualization contracts")
	}
	if schema, ok := irDoc.Schemas["VisualizationEnvelope"]; !ok || schema.Namespace != "LeapViewVisualization" {
		t.Fatalf("UI signals do not retain the canonical visualization schema ownership: %#v", schema)
	}
	if !foundDashboardVisualizationSignal {
		t.Fatal("UI signals do not emit the dashboard visualization transport")
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
		"/api/v1/search",
		"/api/v1/workspaces",
		"/api/v1/workspaces/{workspace}",
		"/api/v1/workspaces/{workspace}/assets",
		"/api/v1/workspaces/{workspace}/asset-edges",
		"/api/v1/workspaces/{workspace}/dashboards",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/visuals/{visual}",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/visuals/{visual}/query",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}/values",
		"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/query",
		"/api/v1/workspaces/{workspace}/semantic-models",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/fields",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/preview",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/preview/explain",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/relationships",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/query",
		"/api/v1/workspaces/{workspace}/semantic-models/{model}/query/explain",
		"/api/v1/projects/{project}/releases",
		"/api/v1/projects/{project}/releases/{release}/workspaces/{workspace}/artifact",
		"/api/v1/projects/{project}/releases/{release}/finalize",
		"/api/v1/projects/{project}/deployments",
		"/api/v1/workspaces/{workspace}/refresh-runs",
		"/api/v1/workspaces/{workspace}/refresh-runs/{run}",
		"/api/v1/agent/conversations",
		"/api/v1/agent/conversations/{conversation}",
		"/api/v1/agent/conversations/{conversation}/messages",
		"/api/v1/agent/conversations/{conversation}/runs",
		"/api/v1/agent/conversations/{conversation}/runs/{run}",
		"/api/v1/agent/conversations/{conversation}/runs/{run}/events",
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

	for _, path := range []string{"/api/workspaces", "/api/publishes", "/api/v1/workspaces/{workspace}/publishes", "/api/v1/workspaces/{workspace}/publishes/{publish}", "/api/v1/admin/agent/config", "/updates", "/commands/select", "/workspaces/{workspace}/chat/updates", "/dashboards/{dashboard}"} {
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
		if !contract.Protected {
			t.Fatalf("%s auth contract is not protected", operationID)
		}
		if operationID == "getInstance" {
			if contract.AuthzMode != "authenticated" {
				t.Fatalf("getInstance auth mode = %q, want authenticated", contract.AuthzMode)
			}
			if _, ok := apigenOperationPrivilege(operationID); ok {
				t.Fatal("getInstance must not require a privilege mapping")
			}
			continue
		}
		if contract.AuthzMode != "privilege" {
			t.Fatalf("%s auth contract mode = %q, want privilege", operationID, contract.AuthzMode)
		}
		if _, ok := apigenOperationPrivilege(operationID); !ok {
			t.Fatalf("%s missing generated privilege metadata", operationID)
		}
	}
	if got, _ := apigenOperationPrivilege("uploadReleaseArtifact"); got != access.PrivilegeDeploy {
		t.Fatalf("uploadReleaseArtifact privilege = %q, want %q", got, access.PrivilegeDeploy)
	}
	if _, ok := apigenOperationPrivilege("unknownOperation"); ok {
		t.Fatal("unknown operation unexpectedly resolved a privilege")
	}
}

func TestAPIGenOperationObjectResolverCoverage(t *testing.T) {
	contracts := apigenapi.GetAPIGenOperationContracts()
	objectScoped := 0
	for operationID, contract := range contracts {
		if isGlobalAgentOperation(operationID) {
			if _, hasScope := contract.Extensions[apiGenObjectScopeExtension]; hasScope {
				t.Fatalf("%s global agent operation retains object-scope metadata", operationID)
			}
			continue
		}
		expectedScope, ambiguous := apigenObjectScopeForPath(contract.Path)
		if ambiguous {
			t.Fatalf("%s path %q selects multiple object scopes", operationID, contract.Path)
		}
		resolver, ok := apigenObjectResolverForContract(contract)
		if !ok {
			t.Fatalf("%s has invalid object-scope metadata for %q", operationID, contract.Path)
		}
		if expectedScope == "" {
			if resolver != nil {
				t.Fatalf("%s should stay workspace-scoped", operationID)
			}
			continue
		}
		objectScoped++
		if got := contract.Extensions[apiGenObjectScopeExtension]; got != expectedScope {
			t.Fatalf("%s object scope = %#v, want %q", operationID, got, expectedScope)
		}
		if resolver == nil {
			t.Fatalf("%s scope %q has no exact-object resolver", operationID, expectedScope)
		}
	}
	if objectScoped == 0 {
		t.Fatal("no exact-object API operations found")
	}
}

func TestAPIGenObjectResolverRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name         string
		contract     apigenapi.GenOperationContract
		wantOK       bool
		wantResolver bool
	}{
		{
			name:     "workspace scoped",
			contract: apigenapi.GenOperationContract{OperationID: "listDashboards", Path: "/api/v1/workspaces/{workspace}/dashboards", Extensions: map[string]any{}},
			wantOK:   true,
		},
		{
			name:         "supported exact scope",
			contract:     apigenapi.GenOperationContract{OperationID: "getDashboard", Path: "/api/v1/workspaces/{workspace}/dashboards/{dashboard}", Extensions: map[string]any{apiGenObjectScopeExtension: "dashboard"}},
			wantOK:       true,
			wantResolver: true,
		},
		{
			name:     "missing exact scope",
			contract: apigenapi.GenOperationContract{OperationID: "getDashboard", Path: "/api/v1/workspaces/{workspace}/dashboards/{dashboard}", Extensions: map[string]any{}},
		},
		{
			name:     "wrong exact scope",
			contract: apigenapi.GenOperationContract{OperationID: "getDashboard", Path: "/api/v1/workspaces/{workspace}/dashboards/{dashboard}", Extensions: map[string]any{apiGenObjectScopeExtension: "semantic-model"}},
		},
		{
			name:     "unknown exact scope",
			contract: apigenapi.GenOperationContract{OperationID: "getDashboard", Path: "/api/v1/workspaces/{workspace}/dashboards/{dashboard}", Extensions: map[string]any{apiGenObjectScopeExtension: "tenant"}},
		},
		{
			name:     "malformed exact scope",
			contract: apigenapi.GenOperationContract{OperationID: "getDashboard", Path: "/api/v1/workspaces/{workspace}/dashboards/{dashboard}", Extensions: map[string]any{apiGenObjectScopeExtension: map[string]any{"kind": "dashboard"}}},
		},
		{
			name:     "unexpected exact scope",
			contract: apigenapi.GenOperationContract{OperationID: "listDashboards", Path: "/api/v1/workspaces/{workspace}/dashboards", Extensions: map[string]any{apiGenObjectScopeExtension: "dashboard"}},
		},
		{
			name:     "ambiguous exact scope",
			contract: apigenapi.GenOperationContract{OperationID: "ambiguous", Path: "/api/v1/workspaces/{workspace}/dashboards/{dashboard}/semantic-models/{model}", Extensions: map[string]any{apiGenObjectScopeExtension: "dashboard"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver, ok := apigenObjectResolverForContract(test.contract)
			if ok != test.wantOK {
				t.Fatalf("ok = %t, want %t", ok, test.wantOK)
			}
			if got := resolver != nil; got != test.wantResolver {
				t.Fatalf("has resolver = %t, want %t", got, test.wantResolver)
			}
		})
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
		"getDashboard":              "describe_dashboard",
		"getDashboardVisual":        "describe_dashboard_visual",
		"getWorkspaceAsset":         "describe_asset",
		"getWorkspaceAssetLineage":  "asset_lineage",
		"getSemanticModel":          "describe_model",
		"listSemanticModelFields":   "list_semantic_model_fields",
		"querySemanticModel":        "query_semantic_model",
		"explainSemanticModelQuery": "explain_semantic_model_query",
		"getSemanticDataset":        "describe_semantic_dataset",
		"getRefreshRun":             "get_refresh_run",
		"getDashboardPage":          "describe_dashboard_page",
		"listDashboards":            "list_dashboards",
		"listDashboardFilterValues": "list_dashboard_filter_values",
		"listRefreshRuns":           "list_refresh_runs",
		"listSemanticDatasets":      "list_semantic_datasets",
		"listSemanticFields":        "list_semantic_fields",
		"listSemanticModels":        "list_semantic_models",
		"listWorkspaceAssetEdges":   "list_workspace_asset_edges",
		"listWorkspaceAssets":       "list_assets",
		"listWorkspaces":            "list_workspaces",
		"search":                    "search",
		"queryDashboardVisualData":  "query_dashboard_visual",
		"queryDashboardPage":        "query_dashboard_page",
		"previewSemanticDataset":    "preview_semantic_dataset",
		"explainSemanticPreview":    "explain_semantic_preview",
	}
	for operationID, contract := range contracts {
		authz, ok := contract.Extensions["x-authz"].(map[string]any)
		if !ok {
			t.Fatalf("%s missing generated x-authz extension: %#v", operationID, contract.Extensions["x-authz"])
		}
		if operationID == "getInstance" {
			if got := authz["mode"]; got != "authenticated" {
				t.Fatalf("getInstance x-authz mode = %#v, want authenticated", got)
			}
			continue
		}
		if got := authz["mode"]; got != "privilege" {
			t.Fatalf("%s x-authz mode = %#v, want privilege", operationID, got)
		}
		privilege, ok := apigenOperationPrivilege(operationID)
		if !ok {
			t.Fatalf("%s missing generated privilege metadata", operationID)
		}
		if got := authz["privilege"]; got != string(privilege) {
			t.Fatalf("%s x-authz privilege = %#v, want %q", operationID, got, privilege)
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
		if _, ok := contract.Extensions["x-leapview-dispatch"]; ok {
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
	operation := mustOpenAPIOperation(t, paths, "/api/v1/projects/{project}/releases/{release}/workspaces/{workspace}/artifact", "put")
	if _, ok := operation["x-leapview-dispatch"]; ok {
		t.Fatalf("upload operation should not use x-leapview-dispatch: %#v", operation["x-leapview-dispatch"])
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
		if endpoint.OperationID != "uploadReleaseArtifact" {
			continue
		}
		if endpoint.RequestBody == nil || len(endpoint.RequestBody.Contents) != 1 {
			t.Fatalf("upload IR request body = %#v", endpoint.RequestBody)
		}
		content := endpoint.RequestBody.Contents[0]
		if content.ContentType != "application/octet-stream" || content.BodyKind != "binary" {
			t.Fatalf("upload IR content = %#v, want application/octet-stream binary", content)
		}
		var generatedBody apigenapi.GenUploadReleaseArtifactBody
		_ = []byte(generatedBody)
		return
	}
	t.Fatal("uploadReleaseArtifact missing from APIGen IR")
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
		{"/api/v1/search", "get"},
		{"/api/v1/workspaces/{workspace}/assets", "get"},
		{"/api/v1/workspaces/{workspace}/asset-edges", "get"},
		{"/api/v1/workspaces/{workspace}/dashboards", "get"},
		{"/api/v1/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}/values", "post"},
		{"/api/v1/workspaces/{workspace}/semantic-models", "get"},
		{"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets", "get"},
		{"/api/v1/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/fields", "get"},
		{"/api/v1/workspaces/{workspace}/refresh-runs", "get"},
		{"/api/v1/agent/conversations", "get"},
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
