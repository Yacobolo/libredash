package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentsqlite "github.com/Yacobolo/libredash/internal/deployment/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestRepositoryActiveDeploymentGraphUsesLogicalAssetIDs(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	deploymentRepo := deploymentsqlite.NewRepository(store.SQLDB())
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := deploymentRepo.Create(ctx, deployment.CreateInput{WorkspaceID: "test"})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}

	model := mustAsset(t, workspace.AssetTypeSemanticModel, "olist", "", created.ID)
	dashboard := mustAsset(t, workspace.AssetTypeDashboard, "sales", model.ID, created.ID)
	validation := deployment.Validation{
		Digest:       "digest",
		ManifestJSON: "{}",
		Graph: workspace.AssetGraph{
			Assets: []workspace.Asset{model, dashboard},
			Edges: []workspace.AssetEdge{
				workspace.NewAssetEdge("test", workspace.DeploymentID(created.ID), dashboard.ID, model.ID, workspace.AssetEdgeUsesSemanticModel),
			},
		},
	}
	if _, err := deploymentRepo.SaveValidated(ctx, created.ID, validation, deployment.Artifact{ID: "artifact", DeploymentID: created.ID, WorkspaceID: "test", Environment: deployment.DefaultEnvironment, Digest: "digest", Format: "tar.gz", Path: "artifact.tar.gz", ManifestJSON: "{}"}); err != nil {
		t.Fatalf("save validated: %v", err)
	}
	if _, err := deploymentRepo.Activate(ctx, "test", deployment.DefaultEnvironment, created.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}

	got, ok, err := workspaceRepo.ActiveDeploymentGraph(ctx, "test", string(deployment.DefaultEnvironment))
	if err != nil {
		t.Fatalf("active graph: %v", err)
	}
	if !ok {
		t.Fatal("active graph ok = false, want true")
	}
	gotDashboard := assetByID(got, dashboard.ID)
	if gotDashboard.ID != "dashboard:sales" {
		t.Fatalf("dashboard logical id = %q, want dashboard:sales", gotDashboard.ID)
	}
	if gotDashboard.SnapshotID == "" {
		t.Fatal("dashboard snapshot id is blank")
	}
	if gotDashboard.ParentID != model.ID {
		t.Fatalf("dashboard parent = %q, want %q", gotDashboard.ParentID, model.ID)
	}
	if gotDashboard.PayloadSchema != "dashboard.v1" {
		t.Fatalf("dashboard payload schema = %q, want dashboard.v1", gotDashboard.PayloadSchema)
	}
	if gotDashboard.PayloadJSON == "" {
		t.Fatal("dashboard payload json is blank")
	}
	if len(got.Edges) != 1 {
		t.Fatalf("edge count = %d, want 1", len(got.Edges))
	}
	if got.Edges[0].FromAssetID != dashboard.ID || got.Edges[0].ToAssetID != model.ID {
		t.Fatalf("edge endpoints = %q -> %q, want logical ids %q -> %q", got.Edges[0].FromAssetID, got.Edges[0].ToAssetID, dashboard.ID, model.ID)
	}
}

func mustAsset(t *testing.T, typ workspace.AssetType, key string, parent workspace.AssetID, deploymentID deployment.ID) workspace.Asset {
	t.Helper()
	asset, err := workspace.NewAssetWithSourceFile("test", workspace.DeploymentID(deploymentID), typ, key, parent, key, "", "testdata/"+string(typ)+"-"+key+".yaml", string(typ)+".v1", map[string]any{"key": key})
	if err != nil {
		t.Fatalf("new asset: %v", err)
	}
	return asset
}

func assetByID(graph workspace.AssetGraph, id workspace.AssetID) workspace.Asset {
	for _, asset := range graph.Assets {
		if asset.ID == id {
			return asset
		}
	}
	return workspace.Asset{}
}
