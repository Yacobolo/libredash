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

func TestRepositoryAssetVersionsAreEnvironmentScopedSuccessfulDeployments(t *testing.T) {
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
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: "other", Title: "Other"}); err != nil {
		t.Fatalf("ensure other workspace: %v", err)
	}

	inactive := seedVersionDeployment(t, ctx, deploymentRepo, "test", "dev", deployment.StatusActive)
	current := seedVersionDeployment(t, ctx, deploymentRepo, "test", "dev", deployment.StatusActive)
	validated := seedVersionDeployment(t, ctx, deploymentRepo, "test", "dev", deployment.StatusValidated)
	_ = seedVersionDeployment(t, ctx, deploymentRepo, "test", "prod", deployment.StatusActive)
	_ = seedVersionDeployment(t, ctx, deploymentRepo, "other", "dev", deployment.StatusActive)
	failed := seedVersionDeployment(t, ctx, deploymentRepo, "test", "dev", deployment.StatusFailed)
	pending, err := deploymentRepo.Create(ctx, deployment.CreateInput{WorkspaceID: "test", Environment: "dev", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create pending deployment: %v", err)
	}
	_ = pending
	_ = failed
	_ = inactive

	versions, err := workspaceRepo.AssetVersions(ctx, "test", "dev", workspace.NewAssetID(workspace.AssetTypeDashboard, "sales"))
	if err != nil {
		t.Fatalf("asset versions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("versions len = %d, want 3 successful dev versions: %#v", len(versions), versions)
	}
	seen := map[workspace.DeploymentID]string{}
	for _, version := range versions {
		if version.WorkspaceID != "test" || version.Environment != "dev" || version.AssetID != "dashboard:sales" {
			t.Fatalf("unexpected version scope: %#v", version)
		}
		seen[version.DeploymentID] = version.Status
	}
	if seen[workspace.DeploymentID(current.ID)] != string(deployment.StatusActive) {
		t.Fatalf("current deployment missing or not active: %#v", seen)
	}
	if seen[workspace.DeploymentID(validated.ID)] != string(deployment.StatusValidated) {
		t.Fatalf("validated deployment missing: %#v", seen)
	}
	if _, ok := seen[workspace.DeploymentID(failed.ID)]; ok {
		t.Fatalf("failed deployment included: %#v", seen)
	}
	if _, ok := seen[workspace.DeploymentID(pending.ID)]; ok {
		t.Fatalf("pending deployment included: %#v", seen)
	}
}

func seedVersionDeployment(t *testing.T, ctx context.Context, repo *deploymentsqlite.Repository, workspaceID deployment.WorkspaceID, environment deployment.Environment, finalStatus deployment.Status) deployment.Deployment {
	t.Helper()
	created, err := repo.Create(ctx, deployment.CreateInput{WorkspaceID: workspaceID, Environment: environment, CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	assetWorkspaceID := workspace.WorkspaceID(workspaceID)
	asset := mustAssetForWorkspace(t, assetWorkspaceID, workspace.AssetTypeDashboard, "sales", "", created.ID)
	validation := deployment.Validation{
		Digest:       "digest-" + string(created.ID),
		ManifestJSON: "{}",
		Graph:        workspace.AssetGraph{Assets: []workspace.Asset{asset}},
	}
	artifact := deployment.Artifact{ID: "artifact_" + string(created.ID), DeploymentID: created.ID, WorkspaceID: workspaceID, Environment: environment, Digest: validation.Digest, Format: "tar.gz", Path: "artifact.tar.gz", ManifestJSON: "{}"}
	if _, err := repo.SaveValidated(ctx, created.ID, validation, artifact); err != nil {
		t.Fatalf("save validated deployment: %v", err)
	}
	switch finalStatus {
	case deployment.StatusActive:
		active, err := repo.Activate(ctx, workspaceID, environment, created.ID)
		if err != nil {
			t.Fatalf("activate deployment: %v", err)
		}
		return active
	case deployment.StatusValidated:
		validated, err := repo.ByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("get validated deployment: %v", err)
		}
		return validated
	case deployment.StatusFailed:
		if err := repo.MarkFailed(ctx, created.ID, errVersionFailure{}); err != nil {
			t.Fatalf("mark failed: %v", err)
		}
		failed, err := repo.ByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("get failed deployment: %v", err)
		}
		return failed
	default:
		t.Fatalf("unsupported final status %q", finalStatus)
		return deployment.Deployment{}
	}
}

type errVersionFailure struct{}

func (errVersionFailure) Error() string { return "failed" }

func mustAsset(t *testing.T, typ workspace.AssetType, key string, parent workspace.AssetID, deploymentID deployment.ID) workspace.Asset {
	t.Helper()
	return mustAssetForWorkspace(t, "test", typ, key, parent, deploymentID)
}

func mustAssetForWorkspace(t *testing.T, workspaceID workspace.WorkspaceID, typ workspace.AssetType, key string, parent workspace.AssetID, deploymentID deployment.ID) workspace.Asset {
	t.Helper()
	asset, err := workspace.NewAssetWithSourceFile(workspaceID, workspace.DeploymentID(deploymentID), typ, key, parent, key, "", "testdata/"+string(typ)+"-"+key+".yaml", string(typ)+".v1", map[string]any{"key": key})
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
