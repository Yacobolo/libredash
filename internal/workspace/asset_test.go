package workspace

import (
	"strings"
	"testing"
)

func TestNewAssetRequiresAllowedPayloadSchema(t *testing.T) {
	if _, err := NewAsset("test", "dep", AssetTypeDashboard, "sales", "", "Sales", "", "", map[string]any{}); err == nil {
		t.Fatal("NewAsset() error = nil, want missing payload schema failure")
	}
	if _, err := NewAsset("test", "dep", AssetTypeDashboard, "sales", "", "Sales", "", "visual.v1", map[string]any{}); err == nil || !strings.Contains(err.Error(), "want \"dashboard.v1\"") {
		t.Fatalf("NewAsset() error = %v, want unexpected payload schema failure", err)
	}
	if _, err := NewAsset("test", "dep", AssetType("unknown"), "sales", "", "Sales", "", "unknown.v1", map[string]any{}); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("NewAsset() error = %v, want unregistered payload schema failure", err)
	}
	if _, err := NewAsset("test", "dep", AssetTypeDashboard, "sales", "", "Sales", "", "dashboard.v1", map[string]any{}); err != nil {
		t.Fatalf("NewAsset() error = %v, want allowed payload schema", err)
	}
}

func TestNewAssetContentHashIgnoresSourceFile(t *testing.T) {
	first, err := NewAssetWithSourceFile("test", "dep", AssetTypeDashboard, "sales", "", "Sales", "", "dashboards/a.yaml", "dashboard.v1", map[string]any{"key": "sales"})
	if err != nil {
		t.Fatalf("first asset: %v", err)
	}
	second, err := NewAssetWithSourceFile("test", "dep", AssetTypeDashboard, "sales", "", "Sales", "", "dashboards/moved.yaml", "dashboard.v1", map[string]any{"key": "sales"})
	if err != nil {
		t.Fatalf("second asset: %v", err)
	}
	if first.ContentHash != second.ContentHash {
		t.Fatalf("content hashes differ for sourceFile-only change: %s != %s", first.ContentHash, second.ContentHash)
	}
}

func TestValidateAssetGraphForDeploymentRejectsInvalidGraph(t *testing.T) {
	workspaceID := WorkspaceID("test")
	deploymentID := DeploymentID("dep")
	dashboard := mustTestAsset(workspaceID, deploymentID, AssetTypeDashboard, "sales", "")
	model := mustTestAsset(workspaceID, deploymentID, AssetTypeSemanticModel, "olist", "")

	tests := []struct {
		name   string
		mutate func(*AssetGraph)
	}{
		{
			name: "missing source file",
			mutate: func(graph *AssetGraph) {
				graph.Assets[1].SourceFile = ""
			},
		},
		{
			name: "generated child missing source file",
			mutate: func(graph *AssetGraph) {
				field, err := NewAsset(workspaceID, deploymentID, AssetTypeField, "sales.status", model.ID, "status", "", "field.v1", map[string]any{"key": "status"})
				if err != nil {
					panic(err)
				}
				graph.Assets = append(graph.Assets, field)
			},
		},
		{
			name: "duplicate logical asset",
			mutate: func(graph *AssetGraph) {
				graph.Assets = append(graph.Assets, graph.Assets[0])
			},
		},
		{
			name: "missing parent",
			mutate: func(graph *AssetGraph) {
				graph.Assets[0].ParentID = "dashboard:missing"
			},
		},
		{
			name: "duplicate edge",
			mutate: func(graph *AssetGraph) {
				graph.Edges = append(graph.Edges, graph.Edges[0])
			},
		},
		{
			name: "dangling from edge",
			mutate: func(graph *AssetGraph) {
				graph.Edges[0] = NewAssetEdge(workspaceID, deploymentID, "dashboard:missing", model.ID, AssetEdgeUsesSemanticModel)
			},
		},
		{
			name: "dangling to edge",
			mutate: func(graph *AssetGraph) {
				graph.Edges[0] = NewAssetEdge(workspaceID, deploymentID, dashboard.ID, "semantic_model:missing", AssetEdgeUsesSemanticModel)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := AssetGraph{
				Assets: []Asset{dashboard, model},
				Edges:  []AssetEdge{NewAssetEdge(workspaceID, deploymentID, dashboard.ID, model.ID, AssetEdgeUsesSemanticModel)},
			}
			tt.mutate(&graph)
			if err := ValidateAssetGraphForDeployment(graph, workspaceID, deploymentID); err == nil {
				t.Fatal("ValidateAssetGraphForDeployment() error = nil, want invalid graph failure")
			}
		})
	}
}

func TestValidateAssetGraphForDeploymentAcceptsValidGraph(t *testing.T) {
	workspaceID := WorkspaceID("test")
	deploymentID := DeploymentID("dep")
	dashboard := mustTestAsset(workspaceID, deploymentID, AssetTypeDashboard, "sales", "")
	model := mustTestAsset(workspaceID, deploymentID, AssetTypeSemanticModel, "olist", dashboard.ID)
	graph := AssetGraph{
		Assets: []Asset{dashboard, model},
		Edges:  []AssetEdge{NewAssetEdge(workspaceID, deploymentID, dashboard.ID, model.ID, AssetEdgeUsesSemanticModel)},
	}
	if err := ValidateAssetGraphForDeployment(graph, workspaceID, deploymentID); err != nil {
		t.Fatalf("ValidateAssetGraphForDeployment() error = %v, want valid graph", err)
	}
}

func mustTestAsset(workspaceID WorkspaceID, deploymentID DeploymentID, typ AssetType, key string, parent AssetID) Asset {
	asset, err := NewAssetWithSourceFile(workspaceID, deploymentID, typ, key, parent, key, "", "testdata/"+string(typ)+"-"+key+".yaml", string(typ)+".v1", map[string]any{"key": key})
	if err != nil {
		panic(err)
	}
	return asset
}
