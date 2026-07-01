package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
)

func TestDeployPrintsPlanAndRequiresApprovalBeforeMutation(t *testing.T) {
	var mutations atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/sales/active-deployment/graph":
			writeCLIJSON(t, w, activeGraphResponse(nil, nil))
		default:
			mutations.Add(1)
			t.Fatalf("deploy mutated server before approval: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	var err error
	output := captureStdout(t, func() {
		err = runDeploy(context.Background(), &rootOptions{
			target:      server.URL,
			token:       "token",
			workspaceID: "sales",
			catalog:     filepath.Join("..", "..", "dashboards", "libredash.yaml"),
		})
	})
	if err == nil || !strings.Contains(err.Error(), "auto-approve") {
		t.Fatalf("runDeploy() error = %v, want approval error", err)
	}
	if mutations.Load() != 0 {
		t.Fatalf("mutations = %d, want 0", mutations.Load())
	}
	for _, want := range []string{"project libredash-showcase", "workspace sales", "changes +"} {
		if !strings.Contains(output, want) {
			t.Fatalf("deploy output missing plan text %q:\n%s", want, output)
		}
	}
}

func TestDeployAutoApproveActivatesAfterPlan(t *testing.T) {
	var sequence []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sequence = append(sequence, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/sales/active-deployment/graph":
			writeCLIJSON(t, w, activeGraphResponse(nil, nil))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/sales/deployments":
			writeCLIJSON(t, w, map[string]any{"id": "dep_1", "workspaceId": "sales", "environment": "dev", "status": "pending"})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/workspaces/sales/deployments/dep_1/artifact":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/sales/deployments/dep_1/validate":
			writeCLIJSON(t, w, map[string]any{"id": "dep_1", "workspaceId": "sales", "environment": "dev", "status": "validated", "digest": "sha256:remote"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/sales/deployments/dep_1/activate":
			writeCLIJSON(t, w, map[string]any{"id": "dep_1", "workspaceId": "sales", "environment": "dev", "status": "active", "digest": "sha256:remote"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		if err := runDeploy(context.Background(), &rootOptions{
			target:      server.URL,
			token:       "token",
			workspaceID: "sales",
			catalog:     filepath.Join("..", "..", "dashboards", "libredash.yaml"),
			autoApprove: true,
		}); err != nil {
			t.Fatalf("runDeploy() error = %v", err)
		}
	})
	if !strings.Contains(output, "workspace sales") || !strings.Contains(output, "deployed sales deployment=dep_1 environment=dev") {
		t.Fatalf("deploy output missing plan or final status:\n%s", output)
	}
	wantPrefix := []string{
		"GET /api/v1/workspaces/sales/active-deployment/graph",
		"POST /api/v1/workspaces/sales/deployments",
	}
	for i, want := range wantPrefix {
		if len(sequence) <= i || sequence[i] != want {
			t.Fatalf("sequence = %#v, want prefix %#v", sequence, wantPrefix)
		}
	}
}

func TestDeployProjectDeploysAllWorkspacesInDeterministicOrder(t *testing.T) {
	var sequence []string
	deployments := map[string]string{
		"operations": "dep_operations",
		"sales":      "dep_sales",
		"visuals":    "dep_visuals",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sequence = append(sequence, r.Method+" "+r.URL.Path)
		workspaceID := workspaceIDFromAPIPath(r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/active-deployment/graph"):
			writeCLIJSON(t, w, activeGraphResponse(nil, nil))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/deployments"):
			writeCLIJSON(t, w, map[string]any{"id": deployments[workspaceID], "workspaceId": workspaceID, "environment": "dev", "status": "pending"})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/deployments/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/validate"):
			writeCLIJSON(t, w, map[string]any{"id": deployments[workspaceID], "workspaceId": workspaceID, "environment": "dev", "status": "validated", "digest": "sha256:remote"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/activate"):
			writeCLIJSON(t, w, map[string]any{"id": deployments[workspaceID], "workspaceId": workspaceID, "environment": "dev", "status": "active", "digest": "sha256:remote"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		if err := runDeploy(context.Background(), &rootOptions{
			target:      server.URL,
			token:       "token",
			catalog:     filepath.Join("..", "..", "dashboards", "libredash.yaml"),
			autoApprove: true,
		}); err != nil {
			t.Fatalf("runDeploy() error = %v", err)
		}
	})

	wantOrder := []string{
		"GET /api/v1/workspaces/operations/active-deployment/graph",
		"GET /api/v1/workspaces/sales/active-deployment/graph",
		"GET /api/v1/workspaces/visuals/active-deployment/graph",
		"POST /api/v1/workspaces/operations/deployments",
		"POST /api/v1/workspaces/sales/deployments",
		"POST /api/v1/workspaces/visuals/deployments",
	}
	assertSequenceContainsInOrder(t, sequence, wantOrder)
	for _, want := range []string{"deployed operations", "deployed sales", "deployed visuals"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDeployProjectSkipsUnchangedWorkspaces(t *testing.T) {
	graphs := compileProjectGraphsForDeployTest(t)
	var mutations atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspaceID := workspaceIDFromAPIPath(r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/active-deployment/graph"):
			writeCLIJSON(t, w, activeGraphResponse(graphs[workspaceID].Assets, graphs[workspaceID].Edges))
		default:
			mutations.Add(1)
			t.Fatalf("unchanged deploy should not mutate server: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		if err := runDeploy(context.Background(), &rootOptions{
			target:      server.URL,
			token:       "token",
			catalog:     filepath.Join("..", "..", "dashboards", "libredash.yaml"),
			autoApprove: true,
		}); err != nil {
			t.Fatalf("runDeploy() error = %v", err)
		}
	})

	if mutations.Load() != 0 {
		t.Fatalf("mutations = %d, want 0", mutations.Load())
	}
	for _, want := range []string{"skipped operations", "skipped sales", "skipped visuals"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDeployProjectWorkspaceFlagFiltersProject(t *testing.T) {
	var sequence []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sequence = append(sequence, r.Method+" "+r.URL.Path)
		switch {
		case strings.Contains(r.URL.Path, "/workspaces/operations/"), strings.Contains(r.URL.Path, "/workspaces/visuals/"):
			t.Fatalf("workspace filter leaked request: %s %s", r.Method, r.URL.Path)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/active-deployment/graph"):
			writeCLIJSON(t, w, activeGraphResponse(nil, nil))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/deployments"):
			writeCLIJSON(t, w, map[string]any{"id": "dep_sales", "workspaceId": "sales", "environment": "dev", "status": "pending"})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/deployments/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/validate"):
			writeCLIJSON(t, w, map[string]any{"id": "dep_sales", "workspaceId": "sales", "environment": "dev", "status": "validated", "digest": "sha256:remote"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/activate"):
			writeCLIJSON(t, w, map[string]any{"id": "dep_sales", "workspaceId": "sales", "environment": "dev", "status": "active", "digest": "sha256:remote"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		if err := runDeploy(context.Background(), &rootOptions{
			target:      server.URL,
			token:       "token",
			workspaceID: "sales",
			catalog:     filepath.Join("..", "..", "dashboards", "libredash.yaml"),
			autoApprove: true,
		}); err != nil {
			t.Fatalf("runDeploy() error = %v", err)
		}
	})

	if !strings.Contains(output, "deployed sales") {
		t.Fatalf("output missing sales deploy:\n%s", output)
	}
	for _, request := range sequence {
		if !strings.Contains(request, "/workspaces/sales/") {
			t.Fatalf("request = %q, want only sales requests; sequence=%#v", request, sequence)
		}
	}
}

func TestDeployProjectReportsMixedResults(t *testing.T) {
	graphs := compileProjectGraphsForDeployTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspaceID := workspaceIDFromAPIPath(r.URL.Path)
		switch {
		case workspaceID == "operations" && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/active-deployment/graph"):
			writeCLIJSON(t, w, activeGraphResponse(graphs[workspaceID].Assets, graphs[workspaceID].Edges))
		case workspaceID == "sales" && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/active-deployment/graph"):
			writeCLIJSON(t, w, activeGraphResponse(nil, nil))
		case workspaceID == "visuals" && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/active-deployment/graph"):
			http.Error(w, "boom", http.StatusInternalServerError)
		case workspaceID == "sales" && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/deployments"):
			writeCLIJSON(t, w, map[string]any{"id": "dep_sales", "workspaceId": "sales", "environment": "dev", "status": "pending"})
		case workspaceID == "sales" && r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/deployments/"):
			w.WriteHeader(http.StatusNoContent)
		case workspaceID == "sales" && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/validate"):
			writeCLIJSON(t, w, map[string]any{"id": "dep_sales", "workspaceId": "sales", "environment": "dev", "status": "validated", "digest": "sha256:remote"})
		case workspaceID == "sales" && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/activate"):
			writeCLIJSON(t, w, map[string]any{"id": "dep_sales", "workspaceId": "sales", "environment": "dev", "status": "active", "digest": "sha256:remote"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	var err error
	output := captureStdout(t, func() {
		err = runDeploy(context.Background(), &rootOptions{
			target:      server.URL,
			token:       "token",
			catalog:     filepath.Join("..", "..", "dashboards", "libredash.yaml"),
			autoApprove: true,
		})
	})

	if err == nil || !strings.Contains(err.Error(), "visuals") {
		t.Fatalf("runDeploy() error = %v, want visuals failure", err)
	}
	for _, want := range []string{"skipped operations", "deployed sales", "failed visuals"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func workspaceIDFromAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "workspaces" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func assertSequenceContainsInOrder(t *testing.T, sequence, want []string) {
	t.Helper()
	offset := 0
	for _, request := range sequence {
		if offset < len(want) && request == want[offset] {
			offset++
		}
	}
	if offset != len(want) {
		t.Fatalf("sequence = %#v, want in order %#v", sequence, want)
	}
}

func compileProjectGraphsForDeployTest(t *testing.T) map[string]workspace.AssetGraph {
	t.Helper()
	compiled, err := workspacecompiler.CompileProject(filepath.Join("..", "..", "dashboards", "libredash.yaml"), workspacecompiler.Options{DeploymentID: "plan"})
	if err != nil {
		t.Fatalf("compile project: %v", err)
	}
	graphs := map[string]workspace.AssetGraph{}
	for id, compiledWorkspace := range compiled.Workspaces {
		graphs[id] = compiledWorkspace.Workspace.Graph
	}
	return graphs
}

func activeGraphResponse(assets []workspace.Asset, edges []workspace.AssetEdge) api.WorkspaceAssetGraphResponse {
	return api.WorkspaceAssetGraphResponse{
		Assets: assetGraphResponses(assets),
		Edges:  assetEdgeResponses(edges),
	}
}

func assetGraphResponses(assets []workspace.Asset) []api.AssetGraphAssetResponse {
	out := make([]api.AssetGraphAssetResponse, 0, len(assets))
	for _, asset := range assets {
		payload := map[string]any{}
		if asset.PayloadJSON != "" {
			_ = json.Unmarshal([]byte(asset.PayloadJSON), &payload)
		}
		out = append(out, api.AssetGraphAssetResponse{
			ID:            string(asset.ID),
			SnapshotID:    string(asset.SnapshotID),
			WorkspaceID:   string(asset.WorkspaceID),
			DeploymentID:  string(asset.DeploymentID),
			Type:          string(asset.Type),
			Key:           asset.Key,
			ParentID:      string(asset.ParentID),
			Title:         asset.Title,
			Description:   asset.Description,
			SourceFile:    asset.SourceFile,
			PayloadSchema: asset.PayloadSchema,
			Payload:       payload,
			ContentHash:   asset.ContentHash,
		})
	}
	return out
}

func assetEdgeResponses(edges []workspace.AssetEdge) []api.AssetEdgeResponse {
	out := make([]api.AssetEdgeResponse, 0, len(edges))
	for _, edge := range edges {
		out = append(out, api.AssetEdgeResponse{
			ID:           string(edge.ID),
			WorkspaceID:  string(edge.WorkspaceID),
			DeploymentID: string(edge.DeploymentID),
			FromAssetID:  string(edge.FromAssetID),
			ToAssetID:    string(edge.ToAssetID),
			Type:         string(edge.Type),
		})
	}
	return out
}
