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

func TestRepositoryReplaceActiveDeploymentGraph(t *testing.T) {
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
	initial := deployment.Validation{
		Digest:       "digest",
		ManifestJSON: "{}",
		Assets: []deployment.Asset{
			{ID: "asset_dashboard", WorkspaceID: "test", DeploymentID: created.ID, Type: "dashboard", Key: "sales", ContentJSON: "{}", ContentHash: "old"},
			{ID: "asset_model", WorkspaceID: "test", DeploymentID: created.ID, Type: "semantic_model", Key: "olist", ContentJSON: "{}", ContentHash: "old"},
		},
		Edges: []deployment.AssetEdge{
			{ID: "edge_dashboard_model", WorkspaceID: "test", DeploymentID: created.ID, FromAssetID: "asset_dashboard", ToAssetID: "asset_model", Type: "uses_semantic_model"},
		},
	}
	if _, err := deploymentRepo.SaveValidated(ctx, created.ID, initial, deployment.Artifact{ID: "artifact", DeploymentID: created.ID, WorkspaceID: "test", Digest: "digest", Format: "tar.gz", Path: "artifact.tar.gz", ManifestJSON: "{}"}); err != nil {
		t.Fatalf("save validated: %v", err)
	}
	if _, err := deploymentRepo.Activate(ctx, "test", created.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}

	replacement := workspace.AssetGraph{
		Assets: []workspace.Asset{
			mustAsset(t, "asset_catalog", "catalog", "test", "", created.ID),
			mustAsset(t, "asset_semantic_table", "semantic_table", "olist.orders", "", created.ID),
			mustAsset(t, "asset_page_item", "page_item", "sales.overview.item_1", "", created.ID),
		},
		Edges: []workspace.AssetEdge{
			workspace.NewAssetEdge("test", workspace.DeploymentID(created.ID), "asset_page_item", "asset_semantic_table", "uses_table"),
		},
	}
	if err := workspaceRepo.ReplaceActiveDeploymentGraph(ctx, "test", replacement); err != nil {
		t.Fatalf("replace graph: %v", err)
	}

	got, ok, err := workspaceRepo.ActiveDeploymentGraph(ctx, "test")
	if err != nil {
		t.Fatalf("active graph: %v", err)
	}
	if !ok {
		t.Fatal("active graph ok = false, want true")
	}
	if len(got.Assets) != len(replacement.Assets) || len(got.Edges) != len(replacement.Edges) {
		t.Fatalf("graph sizes assets/edges = %d/%d, want %d/%d", len(got.Assets), len(got.Edges), len(replacement.Assets), len(replacement.Edges))
	}
	for _, asset := range got.Assets {
		if asset.ContentHash != "new" {
			t.Fatalf("asset %s content hash = %q, want replacement graph only", asset.ID, asset.ContentHash)
		}
		if asset.ContentVersion != workspace.CurrentAssetContentVersion {
			t.Fatalf("asset %s content version = %d, want %d", asset.ID, asset.ContentVersion, workspace.CurrentAssetContentVersion)
		}
	}
	after, err := deploymentRepo.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("deployment after replace: %v", err)
	}
	if after.Status != deployment.StatusActive {
		t.Fatalf("deployment status = %q, want active", after.Status)
	}
}

func mustAsset(t *testing.T, id workspace.AssetID, typ workspace.AssetType, key string, parent workspace.AssetID, deploymentID deployment.ID) workspace.Asset {
	t.Helper()
	return workspace.Asset{
		ID:             id,
		WorkspaceID:    "test",
		DeploymentID:   workspace.DeploymentID(deploymentID),
		Type:           typ,
		Key:            key,
		ParentID:       parent,
		Title:          key,
		ContentJSON:    "{}",
		ContentHash:    "new",
		ContentVersion: workspace.CurrentAssetContentVersion,
	}
}
