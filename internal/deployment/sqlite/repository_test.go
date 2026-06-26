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

	validation := validationGraph(created.ID)
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

	validation := validationGraph(created.ID)
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
	if _, err := repo.SaveValidated(ctx, created.ID, validationGraph(created.ID), artifact(created.ID, "test")); err != nil {
		t.Fatalf("first save validated: %v", err)
	}

	replacement := validationGraph(created.ID)
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
	wantEdgeID := workspace.NewAssetEdgeID(workspace.DeploymentID(created.ID), replacement.Graph.Edges[0].FromAssetID, replacement.Graph.Edges[0].ToAssetID, replacement.Graph.Edges[0].Type)
	if len(graph.Edges) != 1 || graph.Edges[0].ID != wantEdgeID {
		t.Fatalf("edges after replacement = %#v, want only %q", graph.Edges, wantEdgeID)
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
	validation := validationGraph(created.ID)
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
	if _, err := repo.SaveValidated(ctx, first.ID, validationGraph(first.ID), artifact(first.ID, "test")); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, second.ID, validationGraph(second.ID), artifact(second.ID, "test")); err != nil {
		t.Fatalf("save second: %v", err)
	}
}

func TestRepositorySaveValidatedRejectsMismatchedAssetGraph(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*deployment.Validation)
	}{
		{
			name: "asset workspace",
			mutate: func(validation *deployment.Validation) {
				validation.Graph.Assets[0].WorkspaceID = "other"
			},
		},
		{
			name: "asset deployment",
			mutate: func(validation *deployment.Validation) {
				validation.Graph.Assets[0].DeploymentID = "other"
			},
		},
		{
			name: "asset snapshot",
			mutate: func(validation *deployment.Validation) {
				validation.Graph.Assets[0].SnapshotID = "asset_wrong"
			},
		},
		{
			name: "asset parent",
			mutate: func(validation *deployment.Validation) {
				validation.Graph.Assets[0].ParentID = "dashboard:missing"
			},
		},
		{
			name: "edge workspace",
			mutate: func(validation *deployment.Validation) {
				validation.Graph.Edges[0].WorkspaceID = "other"
			},
		},
		{
			name: "edge deployment",
			mutate: func(validation *deployment.Validation) {
				validation.Graph.Edges[0].DeploymentID = "other"
			},
		},
		{
			name: "edge id",
			mutate: func(validation *deployment.Validation) {
				validation.Graph.Edges[0].ID = "edge_wrong"
			},
		},
		{
			name: "edge from",
			mutate: func(validation *deployment.Validation) {
				edge := validation.Graph.Edges[0]
				validation.Graph.Edges[0] = workspace.NewAssetEdge(edge.WorkspaceID, edge.DeploymentID, "dashboard:missing", edge.ToAssetID, edge.Type)
			},
		},
		{
			name: "edge to",
			mutate: func(validation *deployment.Validation) {
				edge := validation.Graph.Edges[0]
				validation.Graph.Edges[0] = workspace.NewAssetEdge(edge.WorkspaceID, edge.DeploymentID, edge.FromAssetID, "semantic_model:missing", edge.Type)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store, repo := openRepo(t, ctx)
			if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
				t.Fatalf("ensure workspace: %v", err)
			}
			created, err := repo.Create(ctx, deployment.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			validation := validationGraph(created.ID)
			tt.mutate(&validation)
			if _, err := repo.SaveValidated(ctx, created.ID, validation, artifact(created.ID, "test")); err == nil {
				t.Fatal("expected mismatched graph error")
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
		})
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

func validationGraph(deploymentID deployment.ID) deployment.Validation {
	workspaceID := workspace.WorkspaceID("test")
	assetA := mustTestAsset(workspaceID, workspace.DeploymentID(deploymentID), workspace.AssetTypeDashboard, "a", "")
	assetB := mustTestAsset(workspaceID, workspace.DeploymentID(deploymentID), workspace.AssetTypeSemanticModel, "b", "")
	return deployment.Validation{
		Digest:       "digest",
		ManifestJSON: "{}",
		Graph: workspace.AssetGraph{
			Assets: []workspace.Asset{assetA, assetB},
			Edges: []workspace.AssetEdge{
				workspace.NewAssetEdge(workspaceID, workspace.DeploymentID(deploymentID), assetA.ID, assetB.ID, workspace.AssetEdgeUsesSemanticModel),
				workspace.NewAssetEdge(workspaceID, workspace.DeploymentID(deploymentID), assetB.ID, assetA.ID, workspace.AssetEdgeContains),
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
