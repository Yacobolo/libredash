package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestRepositorySaveValidatedCommitsDeploymentGraph(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	validation := validationGraph(created.ID)
	artifact := artifact(created.ID, "test")
	saved, err := repo.SaveValidated(ctx, created.ID, validation, artifact)
	if err != nil {
		t.Fatalf("save validated: %v", err)
	}
	if saved.Status != servingstate.StatusValidated || saved.Digest != "digest" {
		t.Fatalf("saved = %#v, want validated digest", saved)
	}
	gotArtifact, err := repo.ArtifactByServingState(ctx, created.ID)
	if err != nil {
		t.Fatalf("artifact: %v", err)
	}
	if gotArtifact.Path != "artifact.tar.gz" {
		t.Fatalf("artifact path = %q, want artifact.tar.gz", gotArtifact.Path)
	}
	if gotArtifact.DataRoot != ".data/test" {
		t.Fatalf("artifact data root = %q, want .data/test", gotArtifact.DataRoot)
	}
}

func TestRepositorySaveValidatedRollsBackOnDuplicateEdge(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
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
	if after.Status != servingstate.StatusPending {
		t.Fatalf("status = %q, want pending rollback", after.Status)
	}
	if _, err := repo.ArtifactByServingState(ctx, created.ID); !errors.Is(err, servingstate.ErrNotFound) {
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
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
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
	if _, err := repo.Activate(ctx, "test", servingstate.DefaultEnvironment, created.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	graph, ok, err := workspaceRepo.ActiveServingStateGraph(ctx, "test", string(servingstate.DefaultEnvironment))
	if err != nil {
		t.Fatalf("active graph: %v", err)
	}
	if !ok {
		t.Fatal("active graph ok = false")
	}
	wantEdgeID := workspace.NewAssetEdgeID(workspace.ServingStateID(created.ID), replacement.Graph.Edges[0].FromAssetID, replacement.Graph.Edges[0].ToAssetID, replacement.Graph.Edges[0].Type)
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
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
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
	if after.Status != servingstate.StatusPending {
		t.Fatalf("status = %q, want pending rollback", after.Status)
	}
	if _, err := repo.ArtifactByServingState(ctx, created.ID); !errors.Is(err, servingstate.ErrNotFound) {
		t.Fatalf("artifact error = %v, want ErrNotFound", err)
	}
}

func TestRepositorySaveValidatedAllowsSameLogicalAssetsAcrossDeployments(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	first, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
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

func TestRepositoryTracksActiveDeploymentsPerEnvironment(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	dev, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", Environment: "dev", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create dev: %v", err)
	}
	prod, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", Environment: "prod", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create prod: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, dev.ID, validationGraph(dev.ID), artifact(dev.ID, "test")); err != nil {
		t.Fatalf("save dev: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, prod.ID, validationGraph(prod.ID), artifactForEnvironment(prod.ID, "test", "prod")); err != nil {
		t.Fatalf("save prod: %v", err)
	}
	if _, err := repo.Activate(ctx, "test", "dev", dev.ID); err != nil {
		t.Fatalf("activate dev: %v", err)
	}
	if _, err := repo.Activate(ctx, "test", "prod", prod.ID); err != nil {
		t.Fatalf("activate prod: %v", err)
	}
	activeDev, _, err := repo.ActiveArtifact(ctx, "test", "dev")
	if err != nil {
		t.Fatalf("active dev: %v", err)
	}
	activeProd, _, err := repo.ActiveArtifact(ctx, "test", "prod")
	if err != nil {
		t.Fatalf("active prod: %v", err)
	}
	if activeDev.ID != dev.ID || activeProd.ID != prod.ID {
		t.Fatalf("active dev/prod = %s/%s, want %s/%s", activeDev.ID, activeProd.ID, dev.ID, prod.ID)
	}
	devRows, err := repo.List(ctx, "test", "dev")
	if err != nil {
		t.Fatalf("list dev: %v", err)
	}
	if len(devRows) != 1 || devRows[0].ID != dev.ID {
		t.Fatalf("dev serving_states = %#v, want only %s", devRows, dev.ID)
	}
}

func TestRepositoryCreateDefaultsToPublishSource(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Source != servingstate.SourcePublish {
		t.Fatalf("source = %q, want publish", created.Source)
	}
	refresh, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester", Source: servingstate.SourceRefresh})
	if err != nil {
		t.Fatalf("create refresh: %v", err)
	}
	if refresh.Source != servingstate.SourceRefresh {
		t.Fatalf("refresh source = %q, want refresh", refresh.Source)
	}
}

func TestRepositoryRecordsDuckLakeSnapshot(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.RecordDuckLakeSnapshot(ctx, created.ID, 42); err != nil {
		t.Fatalf("record snapshot: %v", err)
	}
	got, err := repo.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.DuckLakeSnapshotID != 42 {
		t.Fatalf("snapshot = %d, want 42", got.DuckLakeSnapshotID)
	}
	if _, err := repo.SaveValidated(ctx, created.ID, validationGraph(created.ID), artifact(created.ID, "test")); err != nil {
		t.Fatalf("save validated: %v", err)
	}
	if _, err := repo.Activate(ctx, "test", servingstate.DefaultEnvironment, created.ID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	active, _, err := repo.ActiveArtifact(ctx, "test", servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("active artifact: %v", err)
	}
	if active.DuckLakeSnapshotID != 42 {
		t.Fatalf("active snapshot = %d, want 42", active.DuckLakeSnapshotID)
	}
}

func TestRepositoryListsReferencedDuckLakeSnapshots(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	first, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	third, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create third: %v", err)
	}
	prod, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", Environment: "prod", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create prod: %v", err)
	}
	if err := repo.RecordDuckLakeSnapshot(ctx, first.ID, 7); err != nil {
		t.Fatalf("record first: %v", err)
	}
	if err := repo.RecordDuckLakeSnapshot(ctx, second.ID, 9); err != nil {
		t.Fatalf("record second: %v", err)
	}
	if err := repo.RecordDuckLakeSnapshot(ctx, third.ID, 7); err != nil {
		t.Fatalf("record third: %v", err)
	}
	if err := repo.RecordDuckLakeSnapshot(ctx, prod.ID, 11); err != nil {
		t.Fatalf("record prod: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, "UPDATE serving_states SET status = ? WHERE id = ?", string(servingstate.StatusActive), string(first.ID)); err != nil {
		t.Fatalf("mark first active: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, "UPDATE serving_states SET status = ? WHERE id = ?", string(servingstate.StatusDraining), string(third.ID)); err != nil {
		t.Fatalf("mark third draining: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, "UPDATE serving_states SET status = ? WHERE id = ?", string(servingstate.StatusInactive), string(prod.ID)); err != nil {
		t.Fatalf("mark prod inactive: %v", err)
	}

	got, err := repo.ReferencedDuckLakeSnapshots(ctx)
	if err != nil {
		t.Fatalf("referenced snapshots: %v", err)
	}
	want := []int64{7}
	if len(got) != len(want) {
		t.Fatalf("referenced snapshots = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("referenced snapshots = %#v, want %#v", got, want)
		}
	}
}

func TestRepositoryPersistsQuerySnapshotLeaseLifecycle(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, created.ID, validationGraph(created.ID), artifact(created.ID, "test")); err != nil {
		t.Fatalf("save validated: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, "UPDATE serving_states SET status = ?, ducklake_snapshot_id = ? WHERE id = ?", string(servingstate.StatusDraining), int64(42), string(created.ID)); err != nil {
		t.Fatalf("mark deployment draining: %v", err)
	}

	leaseID, err := repo.CreateQuerySnapshotLease(ctx, servingstate.SnapshotLeaseInput{
		WorkspaceID:        "test",
		Environment:        servingstate.DefaultEnvironment,
		ServingStateID:     created.ID,
		DuckLakeSnapshotID: 42,
		OwnerID:            "test",
		ExpiresAt:          time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create lease: %v", err)
	}
	leased, err := repo.LeasedDuckLakeSnapshots(ctx)
	if err != nil {
		t.Fatalf("leased snapshots: %v", err)
	}
	if len(leased) != 1 || leased[0] != 42 {
		t.Fatalf("leased snapshots = %#v, want [42]", leased)
	}
	if err := repo.ReleaseQuerySnapshotLease(ctx, leaseID); err != nil {
		t.Fatalf("release lease: %v", err)
	}
	leased, err = repo.LeasedDuckLakeSnapshots(ctx)
	if err != nil {
		t.Fatalf("leased snapshots after release: %v", err)
	}
	if len(leased) != 0 {
		t.Fatalf("leased snapshots after release = %#v, want empty", leased)
	}
}

func TestRepositoryActivationMarksPreviousActiveDeploymentDraining(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	first, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, first.ID, validationGraph(first.ID), artifact(first.ID, "test")); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, second.ID, validationGraph(second.ID), artifact(second.ID, "test")); err != nil {
		t.Fatalf("save second: %v", err)
	}
	if _, err := repo.Activate(ctx, "test", servingstate.DefaultEnvironment, first.ID); err != nil {
		t.Fatalf("activate first: %v", err)
	}
	if _, err := repo.Activate(ctx, "test", servingstate.DefaultEnvironment, second.ID); err != nil {
		t.Fatalf("activate second: %v", err)
	}
	requireDeploymentStatus(t, ctx, repo, first.ID, servingstate.StatusDraining)
	requireDeploymentStatus(t, ctx, repo, second.ID, servingstate.StatusActive)
	firstAfter, err := repo.ByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("first after: %v", err)
	}
	if firstAfter.SupersededAt == "" {
		t.Fatalf("first superseded_at = empty, want set")
	}
}

func TestRepositoryReconcileRetentionDeletesDrainingDeploymentsWithoutGrace(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	first, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, first.ID, validationGraph(first.ID), artifact(first.ID, "test")); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, second.ID, validationGraph(second.ID), artifact(second.ID, "test")); err != nil {
		t.Fatalf("save second: %v", err)
	}
	if _, err := repo.Activate(ctx, "test", servingstate.DefaultEnvironment, first.ID); err != nil {
		t.Fatalf("activate first: %v", err)
	}
	if _, err := repo.Activate(ctx, "test", servingstate.DefaultEnvironment, second.ID); err != nil {
		t.Fatalf("activate second: %v", err)
	}
	requireDeploymentStatus(t, ctx, repo, first.ID, servingstate.StatusDraining)
	if err := repo.ReconcileRetention(ctx, time.Now()); err != nil {
		t.Fatalf("reconcile retention: %v", err)
	}
	requireDeploymentStatus(t, ctx, repo, first.ID, servingstate.StatusDeleted)
	requireDeploymentStatus(t, ctx, repo, second.ID, servingstate.StatusActive)
}

func TestRepositorySaveValidatedRejectsMismatchedArtifactEnvironment(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", Environment: "prod", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	wrongArtifact := artifact(created.ID, "test")
	wrongArtifact.Environment = "dev"

	if _, err := repo.SaveValidated(ctx, created.ID, validationGraph(created.ID), wrongArtifact); err == nil {
		t.Fatal("SaveValidated() error = nil, want environment mismatch")
	}
	after, err := repo.ByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after rollback: %v", err)
	}
	if after.Status != servingstate.StatusPending {
		t.Fatalf("status = %q, want pending rollback", after.Status)
	}
	if _, err := repo.ArtifactByServingState(ctx, created.ID); !errors.Is(err, servingstate.ErrNotFound) {
		t.Fatalf("artifact error = %v, want ErrNotFound", err)
	}
}

func TestRepositoryActivateWithWorkspacePolicyIsAtomic(t *testing.T) {
	ctx := context.Background()
	store, repo := openRepo(t, ctx)
	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	first, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, first.ID, validationGraph(first.ID), artifact(first.ID, "test")); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if _, err := repo.SaveValidated(ctx, second.ID, validationGraph(second.ID), artifact(second.ID, "test")); err != nil {
		t.Fatalf("save second: %v", err)
	}
	initial := workspace.AccessPolicy{
		Groups: map[string]workspace.WorkspaceGroup{
			"analysts": {Name: "analysts", Members: []workspace.WorkspaceGroupMember{{Email: "analyst@example.com"}}},
		},
		RoleBindings: map[string]workspace.WorkspaceRoleBinding{
			"analysts-viewer": {Role: "viewer", Subject: workspace.WorkspaceRoleBindingSubject{Kind: "group", Group: "analysts"}},
		},
	}
	if _, err := repo.ActivateWithWorkspacePolicy(ctx, "test", servingstate.DefaultEnvironment, first.ID, initial); err != nil {
		t.Fatalf("activate first: %v", err)
	}

	invalid := workspace.AccessPolicy{
		RoleBindings: map[string]workspace.WorkspaceRoleBinding{
			"missing-viewer": {Role: "viewer", Subject: workspace.WorkspaceRoleBindingSubject{Kind: "group", Group: "missing"}},
		},
	}
	if _, err := repo.ActivateWithWorkspacePolicy(ctx, "test", servingstate.DefaultEnvironment, second.ID, invalid); err == nil {
		t.Fatal("ActivateWithWorkspacePolicy() error = nil, want atomic policy failure")
	}

	active, _, err := repo.ActiveArtifact(ctx, "test", servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("active artifact: %v", err)
	}
	if active.ID != first.ID {
		t.Fatalf("active deployment = %s, want %s", active.ID, first.ID)
	}
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	groups, err := accessRepo.ListGroups(ctx, "test")
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "analysts" {
		t.Fatalf("groups after failed activation = %#v, want original analysts group", groups)
	}
	bindings, err := accessRepo.ListRoleBindings(ctx, "test")
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Role != "viewer" {
		t.Fatalf("bindings after failed activation = %#v, want original viewer binding", bindings)
	}
}

func TestRepositorySaveValidatedRejectsMismatchedAssetGraph(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*servingstate.Validation)
	}{
		{
			name: "asset workspace",
			mutate: func(validation *servingstate.Validation) {
				validation.Graph.Assets[0].WorkspaceID = "other"
			},
		},
		{
			name: "asset deployment",
			mutate: func(validation *servingstate.Validation) {
				validation.Graph.Assets[0].ServingStateID = "other"
			},
		},
		{
			name: "asset snapshot",
			mutate: func(validation *servingstate.Validation) {
				validation.Graph.Assets[0].SnapshotID = "asset_wrong"
			},
		},
		{
			name: "asset parent",
			mutate: func(validation *servingstate.Validation) {
				validation.Graph.Assets[0].ParentID = "dashboard:missing"
			},
		},
		{
			name: "edge workspace",
			mutate: func(validation *servingstate.Validation) {
				validation.Graph.Edges[0].WorkspaceID = "other"
			},
		},
		{
			name: "edge deployment",
			mutate: func(validation *servingstate.Validation) {
				validation.Graph.Edges[0].ServingStateID = "other"
			},
		},
		{
			name: "edge id",
			mutate: func(validation *servingstate.Validation) {
				validation.Graph.Edges[0].ID = "edge_wrong"
			},
		},
		{
			name: "edge from",
			mutate: func(validation *servingstate.Validation) {
				edge := validation.Graph.Edges[0]
				validation.Graph.Edges[0] = workspace.NewAssetEdge(edge.WorkspaceID, edge.ServingStateID, "dashboard:missing", edge.ToAssetID, edge.Type)
			},
		},
		{
			name: "edge to",
			mutate: func(validation *servingstate.Validation) {
				edge := validation.Graph.Edges[0]
				validation.Graph.Edges[0] = workspace.NewAssetEdge(edge.WorkspaceID, edge.ServingStateID, edge.FromAssetID, "semantic_model:missing", edge.Type)
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
			created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
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
			if after.Status != servingstate.StatusPending {
				t.Fatalf("status = %q, want pending rollback", after.Status)
			}
			if _, err := repo.ArtifactByServingState(ctx, created.ID); !errors.Is(err, servingstate.ErrNotFound) {
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

func requireDeploymentStatus(t *testing.T, ctx context.Context, repo *Repository, id servingstate.ID, want servingstate.Status) {
	t.Helper()
	got, err := repo.ByID(ctx, id)
	if err != nil {
		t.Fatalf("deployment %s: %v", id, err)
	}
	if got.Status != want {
		t.Fatalf("deployment %s status = %q, want %q", id, got.Status, want)
	}
}

func validationGraph(servingStateID servingstate.ID) servingstate.Validation {
	workspaceID := workspace.WorkspaceID("test")
	assetA := mustTestAsset(workspaceID, workspace.ServingStateID(servingStateID), workspace.AssetTypeDashboard, "a", "")
	assetB := mustTestAsset(workspaceID, workspace.ServingStateID(servingStateID), workspace.AssetTypeSemanticModel, "b", "")
	return servingstate.Validation{
		Digest:       "digest",
		ManifestJSON: "{}",
		Graph: workspace.AssetGraph{
			Assets: []workspace.Asset{assetA, assetB},
			Edges: []workspace.AssetEdge{
				workspace.NewAssetEdge(workspaceID, workspace.ServingStateID(servingStateID), assetA.ID, assetB.ID, workspace.AssetEdgeUsesSemanticModel),
				workspace.NewAssetEdge(workspaceID, workspace.ServingStateID(servingStateID), assetB.ID, assetA.ID, workspace.AssetEdgeContains),
			},
		},
	}
}

func mustTestAsset(workspaceID workspace.WorkspaceID, servingStateID workspace.ServingStateID, typ workspace.AssetType, key string, parent workspace.AssetID) workspace.Asset {
	asset, err := workspace.NewAssetWithSourceFile(workspaceID, servingStateID, typ, key, parent, key, "", "testdata/"+string(typ)+"-"+key+".yaml", string(typ)+".v1", map[string]any{"key": key})
	if err != nil {
		panic(err)
	}
	return asset
}

func artifact(servingStateID servingstate.ID, workspaceID servingstate.WorkspaceID) servingstate.Artifact {
	return artifactForEnvironment(servingStateID, workspaceID, servingstate.DefaultEnvironment)
}

func artifactForEnvironment(servingStateID servingstate.ID, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) servingstate.Artifact {
	return servingstate.Artifact{
		ID:             "artifact_" + string(servingStateID),
		ServingStateID: servingStateID,
		WorkspaceID:    workspaceID,
		Environment:    environment,
		Digest:         "digest",
		Format:         "tar.gz",
		Path:           "artifact.tar.gz",
		DataRoot:       ".data/" + string(workspaceID),
		ManifestJSON:   "{}",
	}
}
