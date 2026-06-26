package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestRepositorySaveValidatedCommitsDeploymentGraph(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, deployment.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	validation := validationGraph(created.ID, "edge_1", "edge_2")
	artifact := artifact(created.ID, "test")
	saved, err := repo.SaveValidated(ctx, created.ID, validation, artifact)
	if err != nil {
		t.Fatalf("save validated: %v", err)
	}
	if saved.Status != deployment.StatusValidated || saved.Digest != "digest" {
		t.Fatalf("saved = %#v, want validated digest", saved)
	}
	gotArtifact, err := repo.ArtifactByDeployment(ctx, created.ID)
	if err != nil {
		t.Fatalf("artifact: %v", err)
	}
	if gotArtifact.Path != "artifact.tar.gz" {
		t.Fatalf("artifact path = %q, want artifact.tar.gz", gotArtifact.Path)
	}
}

func TestRepositorySaveValidatedRollsBackOnDuplicateEdge(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, deployment.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	validation := validationGraph(created.ID, "edge_1", "edge_2")
	validation.Graph.Edges[1].FromAssetID = validation.Graph.Edges[0].FromAssetID
	validation.Graph.Edges[1].ToAssetID = validation.Graph.Edges[0].ToAssetID
	validation.Graph.Edges[1].Type = validation.Graph.Edges[0].Type
	if _, err := repo.SaveValidated(ctx, created.ID, validation, artifact(created.ID, "test")); err == nil {
		t.Fatal("expected duplicate edge error")
	}

	after, err := repo.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after rollback: %v", err)
	}
	if after.Status != deployment.StatusPending {
		t.Fatalf("status = %q, want pending rollback", after.Status)
	}
	if _, err := repo.ArtifactByDeployment(ctx, created.ID); !errors.Is(err, deployment.ErrNotFound) {
		t.Fatalf("artifact error = %v, want ErrNotFound", err)
	}
}

func TestRepositorySaveValidatedReplacesDeploymentGraph(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, deployment.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, created.ID, validationGraph(created.ID, "edge_1", "edge_2"), artifact(created.ID, "test")); err != nil {
		t.Fatalf("first save validated: %v", err)
	}

	replacement := validationGraph(created.ID, "edge_1", "edge_3")
	replacement.Digest = "replacement"
	replacement.Graph.Edges = replacement.Graph.Edges[:1]
	if _, err := repo.SaveValidated(ctx, created.ID, replacement, artifact(created.ID, "test")); err != nil {
		t.Fatalf("replacement save validated: %v", err)
	}
	if _, err := repo.Activate(ctx, "test", created.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	graph, ok, err := workspaceRepo.ActiveDeploymentGraph(ctx, "test")
	if err != nil {
		t.Fatalf("active graph: %v", err)
	}
	if !ok {
		t.Fatal("active graph ok = false")
	}
	if len(graph.Edges) != 1 || graph.Edges[0].ID != "edge_1" {
		t.Fatalf("edges after replacement = %#v, want only edge_1", graph.Edges)
	}
}

func TestRepositorySaveValidatedRollsBackOnDuplicateLogicalAsset(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, deployment.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	validation := validationGraph(created.ID, "edge_1", "edge_2")
	validation.Graph.Assets = append(validation.Graph.Assets, validation.Graph.Assets[0])
	if _, err := repo.SaveValidated(ctx, created.ID, validation, artifact(created.ID, "test")); err == nil {
		t.Fatal("expected duplicate logical asset error")
	}
	after, err := repo.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after rollback: %v", err)
	}
	if after.Status != deployment.StatusPending {
		t.Fatalf("status = %q, want pending rollback", after.Status)
	}
	if _, err := repo.ArtifactByDeployment(ctx, created.ID); !errors.Is(err, deployment.ErrNotFound) {
		t.Fatalf("artifact error = %v, want ErrNotFound", err)
	}
}

func TestRepositorySaveValidatedAllowsSameLogicalAssetsAcrossDeployments(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	first, err := repo.Create(ctx, deployment.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := repo.Create(ctx, deployment.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, first.ID, validationGraph(first.ID, "edge_first_1", "edge_first_2"), artifact(first.ID, "test")); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, second.ID, validationGraph(second.ID, "edge_second_1", "edge_second_2"), artifact(second.ID, "test")); err != nil {
		t.Fatalf("save second: %v", err)
	}
}

func openRepo(t *testing.T, ctx context.Context) (*platform.Store, *Repository) {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, NewRepository(store.SQLDB())
}

func validationGraph(deploymentID deployment.ID, edgeID1, edgeID2 string) deployment.Validation {
	workspaceID := workspace.WorkspaceID("test")
	assetA := mustTestAsset(workspaceID, workspace.DeploymentID(deploymentID), workspace.AssetTypeDashboard, "a", "")
	assetB := mustTestAsset(workspaceID, workspace.DeploymentID(deploymentID), workspace.AssetTypeSemanticModel, "b", "")
	return deployment.Validation{
		Digest:       "digest",
		ManifestJSON: "{}",
		Graph: workspace.AssetGraph{
			Assets: []workspace.Asset{assetA, assetB},
			Edges: []workspace.AssetEdge{
				{ID: workspace.AssetEdgeID(edgeID1), WorkspaceID: workspaceID, DeploymentID: workspace.DeploymentID(deploymentID), FromAssetID: assetA.ID, ToAssetID: assetB.ID, Type: workspace.AssetEdgeUsesSemanticModel},
				{ID: workspace.AssetEdgeID(edgeID2), WorkspaceID: workspaceID, DeploymentID: workspace.DeploymentID(deploymentID), FromAssetID: assetB.ID, ToAssetID: assetA.ID, Type: workspace.AssetEdgeContains},
			},
		},
	}
}

func mustTestAsset(workspaceID workspace.WorkspaceID, deploymentID workspace.DeploymentID, typ workspace.AssetType, key string, parent workspace.AssetID) workspace.Asset {
	asset, err := workspace.NewAsset(workspaceID, deploymentID, typ, key, parent, key, "", string(typ)+".v1", map[string]any{"key": key})
	if err != nil {
		panic(err)
	}
	return asset
}

func artifact(deploymentID deployment.ID, workspaceID deployment.WorkspaceID) deployment.Artifact {
	return deployment.Artifact{
		ID:           "artifact_" + string(deploymentID),
		DeploymentID: deploymentID,
		WorkspaceID:  workspaceID,
		Digest:       "digest",
		Format:       "tar.gz",
		Path:         "artifact.tar.gz",
		ManifestJSON: "{}",
	}
}
