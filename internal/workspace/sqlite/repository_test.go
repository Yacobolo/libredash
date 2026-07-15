package sqlite_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/platform"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestRepositoryEnsureRejectsBlankWorkspaceID(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	err = workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: " \t\n", Title: "Fallback"})
	if err == nil || !strings.Contains(err.Error(), "workspace id is required") {
		t.Fatalf("Ensure(blank) error = %v, want workspace id required", err)
	}

	workspaces, err := workspaceRepo.List(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(workspaces) != 0 {
		t.Fatalf("workspaces = %#v, want none", workspaces)
	}
}

func TestRepositoryActiveServingStateGraphUsesLogicalAssetIDs(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	deploymentRepo := servingstatesqlite.NewRepository(store.SQLDB())
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := deploymentRepo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test"})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}

	model := mustAsset(t, workspace.AssetTypeSemanticModel, "olist", "", created.ID)
	dashboard := mustAsset(t, workspace.AssetTypeDashboard, "sales", model.ID, created.ID)
	validation := servingstate.Validation{
		Digest:            "digest",
		ManifestJSON:      "{}",
		ProjectID:         "project",
		ProjectDigest:     "sha256:" + strings.Repeat("a", 64),
		ProjectWorkspaces: []string{"test"},
		Graph: workspace.AssetGraph{
			Assets: []workspace.Asset{model, dashboard},
			Edges: []workspace.AssetEdge{
				workspace.NewAssetEdge("test", workspace.ServingStateID(created.ID), dashboard.ID, model.ID, workspace.AssetEdgeUsesSemanticModel),
			},
		},
	}
	if _, err := deploymentRepo.SaveValidated(ctx, created.ID, validation, servingstate.Artifact{ID: "artifact", ServingStateID: created.ID, WorkspaceID: "test", Environment: servingstate.DefaultEnvironment, Digest: "digest", Format: "tar.gz", Path: "artifact.tar.gz", ManifestJSON: "{}"}); err != nil {
		t.Fatalf("save validated: %v", err)
	}
	if _, err := deploymentRepo.Activate(ctx, "test", servingstate.DefaultEnvironment, created.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}

	got, ok, err := workspaceRepo.ActiveServingStateGraph(ctx, "test", string(servingstate.DefaultEnvironment))
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

func TestRepositoryAssetVersionsAreDistinctPublishedConfigVersions(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	deploymentRepo := servingstatesqlite.NewRepository(store.SQLDB())
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: "other", Title: "Other"}); err != nil {
		t.Fatalf("ensure other workspace: %v", err)
	}

	first := seedVersionDeployment(t, ctx, deploymentRepo, versionSeed{WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, Source: servingstate.SourcePublish, VersionKey: "v1"})
	duplicate := seedVersionDeployment(t, ctx, deploymentRepo, versionSeed{WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, Source: servingstate.SourcePublish, VersionKey: "v1"})
	current := seedVersionDeployment(t, ctx, deploymentRepo, versionSeed{WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, Source: servingstate.SourcePublish, VersionKey: "v2"})
	refreshCopy := seedVersionDeployment(t, ctx, deploymentRepo, versionSeed{WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, Source: servingstate.SourceRefresh, VersionKey: "v2"})
	_ = seedVersionDeployment(t, ctx, deploymentRepo, versionSeed{WorkspaceID: "test", Environment: "prod", Status: servingstate.StatusActive, Source: servingstate.SourcePublish, VersionKey: "prod"})
	_ = seedVersionDeployment(t, ctx, deploymentRepo, versionSeed{WorkspaceID: "other", Environment: "dev", Status: servingstate.StatusActive, Source: servingstate.SourcePublish, VersionKey: "other"})
	failed := seedVersionDeployment(t, ctx, deploymentRepo, versionSeed{WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusFailed, Source: servingstate.SourcePublish, VersionKey: "failed"})
	pending, err := deploymentRepo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", Environment: "dev", CreatedBy: "tester", Source: servingstate.SourcePublish})
	if err != nil {
		t.Fatalf("create pending deployment: %v", err)
	}
	_ = pending
	_ = failed
	_ = first
	_ = duplicate
	_ = refreshCopy

	versions, err := workspaceRepo.AssetVersions(ctx, "test", "dev", workspace.NewAssetID(workspace.AssetTypeDashboard, "sales"))
	if err != nil {
		t.Fatalf("asset versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions len = %d, want 2 distinct published config versions: %#v", len(versions), versions)
	}
	hashes := map[string]int{}
	seen := map[workspace.ServingStateID]string{}
	for _, version := range versions {
		if version.WorkspaceID != "test" || version.Environment != "dev" || version.AssetID != "dashboard:sales" {
			t.Fatalf("unexpected version scope: %#v", version)
		}
		hashes[version.ContentHash]++
		seen[version.ServingStateID] = version.Status
	}
	for hash, count := range hashes {
		if count != 1 {
			t.Fatalf("content hash %s appeared %d times, want deduped versions: %#v", hash, count, versions)
		}
	}
	_ = current
	if _, ok := seen[workspace.ServingStateID(refreshCopy.ID)]; ok {
		t.Fatalf("refresh serving state included as asset config version: %#v", seen)
	}
	if _, ok := seen[workspace.ServingStateID(failed.ID)]; ok {
		t.Fatalf("failed deployment included: %#v", seen)
	}
	if _, ok := seen[workspace.ServingStateID(pending.ID)]; ok {
		t.Fatalf("pending deployment included: %#v", seen)
	}
}

type versionSeed struct {
	WorkspaceID servingstate.WorkspaceID
	Environment servingstate.Environment
	Status      servingstate.Status
	Source      servingstate.Source
	VersionKey  string
}

func seedVersionDeployment(t *testing.T, ctx context.Context, repo *servingstatesqlite.Repository, seed versionSeed) servingstate.State {
	t.Helper()
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: seed.WorkspaceID, Environment: seed.Environment, CreatedBy: "tester", Source: seed.Source})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	assetWorkspaceID := workspace.WorkspaceID(seed.WorkspaceID)
	asset := mustAssetForWorkspaceWithVersion(t, assetWorkspaceID, workspace.AssetTypeDashboard, "sales", "", created.ID, seed.VersionKey)
	validation := servingstate.Validation{
		Digest:            "digest-" + string(created.ID),
		ManifestJSON:      "{}",
		ProjectID:         "project",
		ProjectDigest:     "sha256:" + strings.Repeat("a", 64),
		ProjectWorkspaces: []string{string(seed.WorkspaceID)},
		Graph:             workspace.AssetGraph{Assets: []workspace.Asset{asset}},
	}
	artifact := servingstate.Artifact{ID: "artifact_" + string(created.ID), ServingStateID: created.ID, WorkspaceID: seed.WorkspaceID, Environment: seed.Environment, Digest: validation.Digest, Format: "tar.gz", Path: "artifact.tar.gz", ManifestJSON: "{}"}
	if _, err := repo.SaveValidated(ctx, created.ID, validation, artifact); err != nil {
		t.Fatalf("save validated deployment: %v", err)
	}
	switch seed.Status {
	case servingstate.StatusActive:
		active, err := repo.Activate(ctx, seed.WorkspaceID, seed.Environment, created.ID)
		if err != nil {
			t.Fatalf("activate deployment: %v", err)
		}
		return active
	case servingstate.StatusValidated:
		validated, err := repo.ByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("get validated deployment: %v", err)
		}
		return validated
	case servingstate.StatusFailed:
		if err := repo.MarkFailed(ctx, created.ID, errVersionFailure{}); err != nil {
			t.Fatalf("mark failed: %v", err)
		}
		failed, err := repo.ByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("get failed deployment: %v", err)
		}
		return failed
	default:
		t.Fatalf("unsupported final status %q", seed.Status)
		return servingstate.State{}
	}
}

type errVersionFailure struct{}

func (errVersionFailure) Error() string { return "failed" }

func mustAsset(t *testing.T, typ workspace.AssetType, key string, parent workspace.AssetID, servingStateID servingstate.ID) workspace.Asset {
	t.Helper()
	return mustAssetForWorkspace(t, "test", typ, key, parent, servingStateID)
}

func mustAssetForWorkspace(t *testing.T, workspaceID workspace.WorkspaceID, typ workspace.AssetType, key string, parent workspace.AssetID, servingStateID servingstate.ID) workspace.Asset {
	return mustAssetForWorkspaceWithVersion(t, workspaceID, typ, key, parent, servingStateID, key)
}

func mustAssetForWorkspaceWithVersion(t *testing.T, workspaceID workspace.WorkspaceID, typ workspace.AssetType, key string, parent workspace.AssetID, servingStateID servingstate.ID, versionKey string) workspace.Asset {
	t.Helper()
	asset, err := workspace.NewAssetWithSourceFile(workspaceID, workspace.ServingStateID(servingStateID), typ, key, parent, key, "", "testdata/"+string(typ)+"-"+key+".yaml", string(typ)+".v1", map[string]any{"key": key, "version": versionKey})
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
