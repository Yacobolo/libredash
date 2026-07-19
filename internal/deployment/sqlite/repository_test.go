package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/deployment"
	refreshpipelinesqlite "github.com/Yacobolo/libredash/internal/refreshpipeline/sqlite"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestCreateDeploymentSnapshotsCompleteTargetsAndManagedPointers(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	insertWorkspaceCandidate(t, ctx, db, "support", "support_old", "support_new", "prod")
	insertReadyRevision(t, ctx, db, "orders", "project", "orders", "orders_v2")
	insertBinding(t, ctx, db, "sales_new", "orders", "orders_v2", "prod")
	insertBinding(t, ctx, db, "support_new", "orders", "orders_v2", "prod")
	setCandidateProjectMetadata(t, ctx, db, []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}, {WorkspaceID: "support", ServingStateID: "support_new"}})

	created, err := repository.CreateDeployment(ctx, deployment.CreateInput{
		ID: "deployment_1", ProjectID: "project", Environment: "prod", RequestDigest: "sha256:request", CreatedBy: "principal",
		Targets: []deployment.TargetInput{{WorkspaceID: "support", ServingStateID: "support_new"}, {WorkspaceID: "sales", ServingStateID: "sales_new"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != deployment.StatusPending || len(created.Targets) != 2 || len(created.Connections) != 1 {
		t.Fatalf("created deployment = %#v", created)
	}
	if created.Targets[0].WorkspaceID != "sales" || created.Targets[0].PriorServingStateID != "sales_old" {
		t.Fatalf("targets = %#v", created.Targets)
	}
	connection := created.Connections[0]
	if connection.CollectionID != "orders" || connection.RevisionID != "orders_v2" || connection.PriorRevisionID != "" || connection.PriorGeneration != 0 {
		t.Fatalf("connection pointer = %#v", connection)
	}

	replayed, err := repository.CreateDeployment(ctx, deployment.CreateInput{
		ID: "deployment_1", ProjectID: "project", Environment: "prod", RequestDigest: "sha256:request", CreatedBy: "principal",
		Targets: []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}, {WorkspaceID: "support", ServingStateID: "support_new"}},
	})
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("idempotent replay = %#v, err = %v", replayed, err)
	}
	_, err = repository.CreateDeployment(ctx, deployment.CreateInput{
		ID: "deployment_1", ProjectID: "project", Environment: "prod", RequestDigest: "sha256:different", CreatedBy: "principal",
		Targets: []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}, {WorkspaceID: "support", ServingStateID: "support_new"}},
	})
	if !errors.Is(err, deployment.ErrConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
}

func TestCreateDeploymentRejectsIncompleteOrMixedProjectTargets(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *sql.DB)
	}{
		{name: "incomplete target set"},
		{name: "mixed project source", mutate: func(ctx context.Context, db *sql.DB) {
			if _, err := db.ExecContext(ctx, `UPDATE serving_states SET project_digest = ? WHERE id = 'support_new'`, "sha256:"+strings.Repeat("b", 64)); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, db, repository := testRepository(t)
			insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
			insertWorkspaceCandidate(t, ctx, db, "support", "support_old", "support_new", "prod")
			targets := []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}, {WorkspaceID: "support", ServingStateID: "support_new"}}
			setCandidateProjectMetadata(t, ctx, db, targets)
			if test.mutate != nil {
				test.mutate(ctx, db)
			}
			requestTargets := targets
			if test.name == "incomplete target set" {
				requestTargets = targets[:1]
			}
			_, err := repository.CreateDeployment(ctx, deployment.CreateInput{
				ID: "deployment_invalid", ProjectID: "project", Environment: "prod", RequestDigest: "sha256:request", CreatedBy: "principal", Targets: requestTargets,
			})
			if !errors.Is(err, deployment.ErrConflict) {
				t.Fatalf("CreateDeployment() error = %v, want conflict", err)
			}
		})
	}
}

func TestActivateDeploymentAtomicallyAppliesArtifactAccessPolicy(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	targets := []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}}
	setCandidateProjectMetadata(t, ctx, db, targets)
	policy, err := json.Marshal(workspace.AccessPolicy{
		Groups: map[string]workspace.WorkspaceGroup{"analysts": {Name: "Analysts", Members: []workspace.WorkspaceGroupMember{{Email: "analyst@example.com"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE serving_states SET access_policy_json = ? WHERE id = 'sales_new'`, string(policy)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO assets (snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key, payload_schema, content_hash) VALUES ('asset:sales_new', 'dashboard:sales', 'sales', 'sales_new', 'dashboard', 'sales.executive', 'v1', 'hash')`); err != nil {
		t.Fatal(err)
	}
	created := createDeployment(t, ctx, repository, "deployment_access", targets)
	if _, err := repository.ActivateDeployment(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	var groupCount, objectCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM groups WHERE workspace_id = 'sales' AND name = 'Analysts'`).Scan(&groupCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM securable_objects WHERE id = 'dashboard:sales:executive' AND parent_id = 'workspace:sales'`).Scan(&objectCount); err != nil {
		t.Fatal(err)
	}
	if groupCount != 1 || objectCount != 1 {
		t.Fatalf("activated access state: groups=%d dashboard_objects=%d", groupCount, objectCount)
	}
}

func TestActivateDeploymentPersistsPublishSemanticModelDataVersion(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	targets := []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}}
	setCandidateProjectMetadata(t, ctx, db, targets)
	if _, err := db.ExecContext(ctx, `UPDATE serving_states SET ducklake_snapshot_id = 42 WHERE id = 'sales_new'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO assets (snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key, payload_schema, content_hash) VALUES ('asset:model', 'semantic_model:sales.sales', 'sales', 'sales_new', 'semantic_model', 'sales.sales', 'semantic_model.v1', 'hash')`); err != nil {
		t.Fatal(err)
	}
	created := createDeployment(t, ctx, repository, "deployment_data_version", targets)
	if _, err := repository.ActivateDeployment(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	var modelID, servingStateID, source string
	var snapshotID int64
	if err := db.QueryRowContext(ctx, `SELECT semantic_model_id, snapshot_id, serving_state_id, source FROM semantic_model_data_versions WHERE workspace_id = 'sales' AND environment = 'prod'`).Scan(&modelID, &snapshotID, &servingStateID, &source); err != nil {
		t.Fatal(err)
	}
	if modelID != "sales" || snapshotID != 42 || servingStateID != "sales_new" || source != "publish" {
		t.Fatalf("data version = model=%q snapshot=%d state=%q source=%q", modelID, snapshotID, servingStateID, source)
	}
	version, ok, err := refreshpipelinesqlite.NewRepository(db).DataVersion(ctx, "sales", "prod", "sales")
	if err != nil {
		t.Fatalf("load publish data version: %v", err)
	}
	if !ok || version.RefreshedAt.IsZero() || time.Since(version.RefreshedAt) > time.Minute {
		t.Fatalf("publish data version = %#v, found=%v", version, ok)
	}
}

func TestActivateDeploymentRemovesDataVersionsForDeletedSemanticModels(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	targets := []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}}
	setCandidateProjectMetadata(t, ctx, db, targets)
	if _, err := db.ExecContext(ctx, `UPDATE serving_states SET ducklake_snapshot_id = 42 WHERE id = 'sales_new'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO assets (snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key, payload_schema, content_hash) VALUES ('asset:model', 'semantic_model:sales.sales', 'sales', 'sales_new', 'semantic_model', 'sales.sales', 'semantic_model.v1', 'hash')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO semantic_model_data_versions (workspace_id, environment, semantic_model_id, snapshot_id, serving_state_id, refreshed_at, source) VALUES ('sales', 'prod', 'removed', 7, 'sales_old', '2026-07-18T06:00:00Z', 'publish')`); err != nil {
		t.Fatal(err)
	}
	created := createDeployment(t, ctx, repository, "deployment_removes_model_version", targets)
	if _, err := repository.ActivateDeployment(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_model_data_versions WHERE workspace_id = 'sales' AND environment = 'prod' AND semantic_model_id = 'removed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("removed semantic-model data versions = %d, want 0", count)
	}
}

func TestActivateDeploymentRollsBackOnInvalidAccessPolicy(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	targets := []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}}
	created := createDeployment(t, ctx, repository, "deployment_invalid_access", targets)
	if _, err := db.ExecContext(ctx, `UPDATE serving_states SET access_policy_json = '{"unknown":true}' WHERE id = 'sales_new'`); err != nil {
		t.Fatal(err)
	}

	if _, err := repository.ActivateDeployment(ctx, created.ID); !errors.Is(err, deployment.ErrConflict) {
		t.Fatalf("ActivateDeployment() error = %v, want conflict", err)
	}
	assertActiveState(t, ctx, db, "sales", "prod", "sales_old")
	assertStateStatus(t, ctx, db, "sales_new", "validated")
}

func TestActivateDeploymentAtomicallyUpdatesAllWorkspaceAndManagedPointers(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	insertWorkspaceCandidate(t, ctx, db, "support", "support_old", "support_new", "prod")
	insertReadyRevision(t, ctx, db, "orders", "project", "orders", "orders_v2")
	insertReadyRevision(t, ctx, db, "tickets", "project", "tickets", "tickets_v3")
	insertBinding(t, ctx, db, "sales_new", "orders", "orders_v2", "prod")
	insertBinding(t, ctx, db, "support_new", "orders", "orders_v2", "prod")
	insertBinding(t, ctx, db, "support_new", "tickets", "tickets_v3", "prod")
	created := createDeployment(t, ctx, repository, "deployment_1", []deployment.TargetInput{
		{WorkspaceID: "sales", ServingStateID: "sales_new"},
		{WorkspaceID: "support", ServingStateID: "support_new"},
	})

	active, err := repository.ActivateDeployment(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if active.Status != deployment.StatusActive || len(active.Connections) != 2 {
		t.Fatalf("active deployment = %#v", active)
	}
	assertActiveState(t, ctx, db, "sales", "prod", "sales_new")
	assertActiveState(t, ctx, db, "support", "prod", "support_new")
	assertStateStatus(t, ctx, db, "sales_old", "draining")
	assertStateStatus(t, ctx, db, "support_old", "draining")
	assertPointer(t, ctx, db, "orders", "prod", "orders_v2", created.ID, 1)
	assertPointer(t, ctx, db, "tickets", "prod", "tickets_v3", created.ID, 1)

	replayed, err := repository.ActivateDeployment(ctx, created.ID)
	if err != nil || replayed.Status != deployment.StatusActive {
		t.Fatalf("activation replay = %#v, err = %v", replayed, err)
	}
}

func TestActivateDeploymentRollsBackOnWorkspacePointerConflict(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	insertWorkspaceCandidate(t, ctx, db, "support", "support_old", "support_new", "prod")
	insertReadyRevision(t, ctx, db, "orders", "project", "orders", "orders_v2")
	insertBinding(t, ctx, db, "sales_new", "orders", "orders_v2", "prod")
	insertBinding(t, ctx, db, "support_new", "orders", "orders_v2", "prod")
	created := createDeployment(t, ctx, repository, "deployment_1", []deployment.TargetInput{
		{WorkspaceID: "sales", ServingStateID: "sales_new"},
		{WorkspaceID: "support", ServingStateID: "support_new"},
	})
	setActiveState(t, ctx, db, "support", "prod", "support_new")

	_, err := repository.ActivateDeployment(ctx, created.ID)
	if !errors.Is(err, deployment.ErrConflict) {
		t.Fatalf("activation error = %v", err)
	}
	assertActiveState(t, ctx, db, "sales", "prod", "sales_old")
	assertNoPointer(t, ctx, db, "orders", "prod")
}

func TestActivateDeploymentRollsBackWhenCandidateBindingsChange(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	insertReadyRevision(t, ctx, db, "orders", "project", "orders", "orders_v2")
	insertReadyRevision(t, ctx, db, "orders_v3_collection", "project", "orders_v3_connection", "orders_v3")
	insertBinding(t, ctx, db, "sales_new", "orders", "orders_v2", "prod")
	created := createDeployment(t, ctx, repository, "deployment_1", []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}})
	if _, err := db.ExecContext(ctx, `UPDATE managed_data_serving_state_bindings SET revision_id = 'orders_v3' WHERE serving_state_id = 'sales_new' AND collection_id = 'orders'`); err == nil {
		t.Fatal("cross-collection binding unexpectedly accepted")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM managed_data_serving_state_bindings WHERE serving_state_id = 'sales_new'`); err != nil {
		t.Fatal(err)
	}

	_, err := repository.ActivateDeployment(ctx, created.ID)
	if !errors.Is(err, deployment.ErrConflict) {
		t.Fatalf("activation error = %v", err)
	}
	assertActiveState(t, ctx, db, "sales", "prod", "sales_old")
	assertNoPointer(t, ctx, db, "orders", "prod")
}

func TestActivateDeploymentRollsBackOnManagedPointerConflict(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	insertReadyRevision(t, ctx, db, "orders", "project", "orders", "orders_v1")
	insertSecondReadyRevision(t, ctx, db, "orders", "orders_v2")
	insertBinding(t, ctx, db, "sales_new", "orders", "orders_v2", "prod")
	if _, err := db.ExecContext(ctx, `INSERT INTO project_deployments (id, project_id, environment, request_digest, status, activated_at) VALUES ('deployment_seed', 'project', 'prod', 'sha256:seed', 'active', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO managed_data_environment_pointers (collection_id, environment, revision_id, deployment_id, generation) VALUES ('orders', 'prod', 'orders_v1', 'deployment_seed', 1)`); err != nil {
		t.Fatal(err)
	}
	created := createDeployment(t, ctx, repository, "deployment_1", []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}})
	if _, err := db.ExecContext(ctx, `UPDATE managed_data_environment_pointers SET generation = 2 WHERE collection_id = 'orders' AND environment = 'prod'`); err != nil {
		t.Fatal(err)
	}

	_, err := repository.ActivateDeployment(ctx, created.ID)
	if !errors.Is(err, deployment.ErrConflict) {
		t.Fatalf("activation error = %v", err)
	}
	assertActiveState(t, ctx, db, "sales", "prod", "sales_old")
	assertPointer(t, ctx, db, "orders", "prod", "orders_v1", "deployment_seed", 2)
}

func TestDeploymentWithoutManagedConnectionsActivates(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	created := createDeployment(t, ctx, repository, "deployment_empty", []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}})
	if len(created.Connections) != 0 {
		t.Fatalf("connections = %#v", created.Connections)
	}
	if _, err := repository.ActivateDeployment(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	assertActiveState(t, ctx, db, "sales", "prod", "sales_new")
}

func TestCancelDeploymentOnlyTransitionsPendingDeployment(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	created := createDeployment(t, ctx, repository, "deployment_cancel", []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}})
	cancelled, err := repository.CancelDeployment(ctx, created.ID)
	if err != nil || cancelled.Status != deployment.StatusCancelled {
		t.Fatalf("cancelled = %#v, error = %v", cancelled, err)
	}
	if _, err := repository.ActivateDeployment(ctx, created.ID); !errors.Is(err, deployment.ErrConflict) {
		t.Fatalf("activation after cancellation error = %v", err)
	}
}

func TestCreateDeploymentRejectsServingStateFromAnotherProject(t *testing.T) {
	ctx, db, repository := testRepository(t)
	insertWorkspaceCandidate(t, ctx, db, "sales", "sales_old", "sales_new", "prod")
	if _, err := db.ExecContext(ctx, `UPDATE serving_states SET project_id = 'other-project' WHERE id = 'sales_new'`); err != nil {
		t.Fatal(err)
	}

	_, err := repository.CreateDeployment(ctx, deployment.CreateInput{
		ID: "deployment_wrong_project", ProjectID: "project", Environment: "prod", RequestDigest: "sha256:request", CreatedBy: "principal",
		Targets: []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: "sales_new"}},
	})
	if !errors.Is(err, deployment.ErrConflict) {
		t.Fatalf("create deployment error = %v, want conflict", err)
	}
}

func testRepository(t *testing.T) (context.Context, *sql.DB, *Repository) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "libredash.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, db, "../../platform/migrations"); err != nil {
		_ = db.Close()
		t.Fatalf("migrate platform store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return ctx, db, NewRepository(db)
}

func insertWorkspaceCandidate(t *testing.T, ctx context.Context, db *sql.DB, workspaceID, oldID, candidateID, environment string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO workspaces (id, title) VALUES (?, ?)`, workspaceID, workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO serving_states (id, workspace_id, project_id, environment, status, source) VALUES (?, ?, 'project', ?, 'active', 'publish'), (?, ?, 'project', ?, 'validated', 'publish')`, oldID, workspaceID, environment, candidateID, workspaceID, environment); err != nil {
		t.Fatal(err)
	}
	setActiveState(t, ctx, db, workspaceID, environment, oldID)
}

func insertReadyRevision(t *testing.T, ctx context.Context, db *sql.DB, collectionID, projectID, connectionName, revisionID string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO managed_data_collections (id, project_id, connection_name, name) VALUES (?, ?, ?, ?)`, collectionID, projectID, connectionName, connectionName); err != nil {
		t.Fatal(err)
	}
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := db.ExecContext(ctx, `INSERT INTO managed_data_revisions (id, collection_id, sequence, digest, status, manifest_json, file_count, size_bytes, ready_at) VALUES (?, ?, 1, ?, 'ready', '{"files":[]}', 0, 0, CURRENT_TIMESTAMP)`, revisionID, collectionID, digest); err != nil {
		t.Fatal(err)
	}
}

func insertSecondReadyRevision(t *testing.T, ctx context.Context, db *sql.DB, collectionID, revisionID string) {
	t.Helper()
	digest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := db.ExecContext(ctx, `INSERT INTO managed_data_revisions (id, collection_id, sequence, digest, status, manifest_json, file_count, size_bytes, ready_at) VALUES (?, ?, 2, ?, 'ready', '{"files":[]}', 0, 0, CURRENT_TIMESTAMP)`, revisionID, collectionID, digest); err != nil {
		t.Fatal(err)
	}
}

func insertBinding(t *testing.T, ctx context.Context, db *sql.DB, stateID, collectionID, revisionID, environment string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO managed_data_serving_state_bindings (serving_state_id, collection_id, revision_id, environment) VALUES (?, ?, ?, ?)`, stateID, collectionID, revisionID, environment); err != nil {
		t.Fatal(err)
	}
}

func createDeployment(t *testing.T, ctx context.Context, repository *Repository, id string, targets []deployment.TargetInput) deployment.Deployment {
	t.Helper()
	setCandidateProjectMetadata(t, ctx, repository.db, targets)
	created, err := repository.CreateDeployment(ctx, deployment.CreateInput{ID: id, ProjectID: "project", Environment: "prod", RequestDigest: "sha256:" + id, Targets: targets, CreatedBy: "principal"})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func setCandidateProjectMetadata(t *testing.T, ctx context.Context, db *sql.DB, targets []deployment.TargetInput) {
	t.Helper()
	workspaces := make([]string, 0, len(targets))
	for _, target := range targets {
		workspaces = append(workspaces, target.WorkspaceID)
	}
	sort.Strings(workspaces)
	encoded, err := json.Marshal(workspaces)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if _, err := db.ExecContext(ctx, `UPDATE serving_states SET project_digest = ?, project_workspaces_json = ? WHERE id = ?`, "sha256:"+strings.Repeat("a", 64), string(encoded), target.ServingStateID); err != nil {
			t.Fatal(err)
		}
	}
}

func setActiveState(t *testing.T, ctx context.Context, db *sql.DB, workspaceID, environment, stateID string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO workspace_active_serving_states (workspace_id, environment, serving_state_id) VALUES (?, ?, ?) ON CONFLICT(workspace_id, environment) DO UPDATE SET serving_state_id = excluded.serving_state_id`, workspaceID, environment, stateID); err != nil {
		t.Fatal(err)
	}
}

func assertActiveState(t *testing.T, ctx context.Context, db *sql.DB, workspaceID, environment, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(ctx, `SELECT serving_state_id FROM workspace_active_serving_states WHERE workspace_id = ? AND environment = ?`, workspaceID, environment).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("active state for %s = %q, want %q", workspaceID, got, want)
	}
}

func assertStateStatus(t *testing.T, ctx context.Context, db *sql.DB, stateID, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(ctx, `SELECT status FROM serving_states WHERE id = ?`, stateID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("state %s status = %q, want %q", stateID, got, want)
	}
}

func assertPointer(t *testing.T, ctx context.Context, db *sql.DB, collectionID, environment, revisionID, deploymentID string, generation int64) {
	t.Helper()
	var gotRevision, gotDeployment string
	var gotGeneration int64
	if err := db.QueryRowContext(ctx, `SELECT revision_id, deployment_id, generation FROM managed_data_environment_pointers WHERE collection_id = ? AND environment = ?`, collectionID, environment).Scan(&gotRevision, &gotDeployment, &gotGeneration); err != nil {
		t.Fatal(err)
	}
	if gotRevision != revisionID || gotDeployment != deploymentID || gotGeneration != generation {
		t.Fatalf("pointer = (%q, %q, %d)", gotRevision, gotDeployment, gotGeneration)
	}
}

func assertNoPointer(t *testing.T, ctx context.Context, db *sql.DB, collectionID, environment string) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM managed_data_environment_pointers WHERE collection_id = ? AND environment = ?`, collectionID, environment).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("pointer count = %d, want 0", count)
	}
}
