package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
	accesssqlite "github.com/Yacobolo/leapview/internal/access/sqlite"
	"github.com/Yacobolo/leapview/internal/dashboard/publication"
	"github.com/Yacobolo/leapview/internal/platform"
	"github.com/Yacobolo/leapview/internal/workspace"
)

func TestReconcilePreservesPublicIDAcrossCutoverRemovalAndReAdd(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, t.TempDir()+"/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db := store.SQLDB()
	seedPublicationWorkspace(t, db)
	repo := NewRepository(db)

	reconcilePublications(t, ctx, db, publication.ReconcileInput{
		ProjectID: "site", WorkspaceID: "visuals", ServingStateID: "state_1", ActorID: "owner",
		Publications: map[string]workspace.DashboardPublication{"website": testCompiledPublication("digest-1")},
	})
	var principalKind, principalName string
	if err := db.QueryRowContext(ctx, `SELECT kind, display_name FROM principals WHERE id = ?`, access.DashboardPublicationSubjectID("visuals", "website")).Scan(&principalKind, &principalName); err != nil {
		t.Fatalf("publication principal: %v", err)
	}
	if principalKind != string(access.PrincipalKindDashboardPublication) || principalName != "website" {
		t.Fatalf("publication principal = kind %q name %q", principalKind, principalName)
	}
	if err := access.PersistAuditEvent(ctx, accesssqlite.NewRepository(db), access.AuditEventInput{
		WorkspaceID: "visuals", PrincipalID: access.DashboardPublicationSubjectID("visuals", "website"),
		Action: "data_query.executed", TargetType: "dashboard_publication", TargetID: "website", Status: "success",
	}); err != nil {
		t.Fatalf("publication audit event: %v", err)
	}
	first := mustGetPublication(t, repo, ctx, "visuals", "website")
	if len(first.PublicID) != 32 || first.Status() != publication.StatusActive {
		t.Fatalf("first = %#v, status=%s", first, first.Status())
	}

	reconcilePublications(t, ctx, db, publication.ReconcileInput{
		ProjectID: "site", WorkspaceID: "visuals", ServingStateID: "state_2", ActorID: "owner",
		Publications: map[string]workspace.DashboardPublication{"website": testCompiledPublication("digest-2")},
	})
	second := mustGetPublication(t, repo, ctx, "visuals", "website")
	if second.PublicID != first.PublicID || second.ServingStateID != "state_2" || second.ConfigurationDigest != "digest-2" {
		t.Fatalf("second = %#v", second)
	}

	reconcilePublications(t, ctx, db, publication.ReconcileInput{ProjectID: "site", WorkspaceID: "visuals", ServingStateID: "state_3", ActorID: "owner", Publications: map[string]workspace.DashboardPublication{}})
	disabled := mustGetPublication(t, repo, ctx, "visuals", "website")
	if disabled.Status() != publication.StatusUnconfigured || disabled.PublicID != first.PublicID || disabled.DisabledAt == "" {
		t.Fatalf("disabled = %#v, status=%s", disabled, disabled.Status())
	}

	reconcilePublications(t, ctx, db, publication.ReconcileInput{
		ProjectID: "site", WorkspaceID: "visuals", ServingStateID: "state_4", ActorID: "owner",
		Publications: map[string]workspace.DashboardPublication{"website": testCompiledPublication("digest-4")},
	})
	readded := mustGetPublication(t, repo, ctx, "visuals", "website")
	if readded.PublicID != first.PublicID || readded.Status() != publication.StatusActive {
		t.Fatalf("readded = %#v, status=%s", readded, readded.Status())
	}
}

func TestSuspensionAlwaysWinsUntilExplicitResumeAndRotationInvalidatesOldID(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, t.TempDir()+"/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db := store.SQLDB()
	seedPublicationWorkspace(t, db)
	repo := NewRepository(db)
	input := publication.ReconcileInput{ProjectID: "site", WorkspaceID: "visuals", ServingStateID: "state_1", ActorID: "owner", Publications: map[string]workspace.DashboardPublication{"website": testCompiledPublication("digest-1")}}
	reconcilePublications(t, ctx, db, input)
	active := mustGetPublication(t, repo, ctx, "visuals", "website")

	suspended, err := repo.Suspend(ctx, "visuals", "website", "admin")
	suspended = mustPublication(t, suspended, err)
	if suspended.Status() != publication.StatusSuspended {
		t.Fatalf("suspended status = %s", suspended.Status())
	}
	input.ServingStateID = "state_2"
	reconcilePublications(t, ctx, db, input)
	stillSuspended := mustGetPublication(t, repo, ctx, "visuals", "website")
	if stillSuspended.Status() != publication.StatusSuspended || stillSuspended.ServingStateID != "state_2" {
		t.Fatalf("after cutover = %#v, status=%s", stillSuspended, stillSuspended.Status())
	}
	resumed, err := repo.Resume(ctx, "visuals", "website", "admin")
	resumed = mustPublication(t, resumed, err)
	if resumed.Status() != publication.StatusActive {
		t.Fatalf("resumed status = %s", resumed.Status())
	}

	rotated, err := repo.Rotate(ctx, "visuals", "website", "admin")
	rotated = mustPublication(t, rotated, err)
	if rotated.PublicID == active.PublicID || len(rotated.PublicID) != 32 {
		t.Fatalf("rotated public id = %q, prior %q", rotated.PublicID, active.PublicID)
	}
	if _, err := repo.GetByPublicID(ctx, active.PublicID); err != publication.ErrNotFound {
		t.Fatalf("old public id error = %v", err)
	}
	got, err := repo.GetByPublicID(ctx, rotated.PublicID)
	if got = mustPublication(t, got, err); got.Name != "website" {
		t.Fatalf("rotated lookup = %#v", got)
	}
}

func TestResumeFailsWhilePublicationIsAbsentFromConfiguration(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, t.TempDir()+"/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db := store.SQLDB()
	seedPublicationWorkspace(t, db)
	repo := NewRepository(db)
	reconcilePublications(t, ctx, db, publication.ReconcileInput{ProjectID: "site", WorkspaceID: "visuals", ServingStateID: "state_1", Publications: map[string]workspace.DashboardPublication{"website": testCompiledPublication("digest")}})
	_, _ = repo.Suspend(ctx, "visuals", "website", "admin")
	reconcilePublications(t, ctx, db, publication.ReconcileInput{ProjectID: "site", WorkspaceID: "visuals", ServingStateID: "state_2", Publications: map[string]workspace.DashboardPublication{}})
	if _, err := repo.Resume(ctx, "visuals", "website", "admin"); err != publication.ErrConflict {
		t.Fatalf("Resume() error = %v", err)
	}
}

func TestReconcileDisablesPublicationsFromThePreviouslyActiveProject(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, t.TempDir()+"/platform.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db := store.SQLDB()
	seedPublicationWorkspace(t, db)
	if _, err := db.Exec(`INSERT INTO serving_states (id, workspace_id, project_id, environment, status, source) VALUES ('state_other', 'visuals', 'other-site', 'prod', 'validated', 'publish')`); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	reconcilePublications(t, ctx, db, publication.ReconcileInput{
		ProjectID: "site", WorkspaceID: "visuals", ServingStateID: "state_1",
		Publications: map[string]workspace.DashboardPublication{"website": testCompiledPublication("site-digest")},
	})
	prior := mustGetPublication(t, repo, ctx, "visuals", "website")

	reconcilePublications(t, ctx, db, publication.ReconcileInput{
		ProjectID: "other-site", WorkspaceID: "visuals", ServingStateID: "state_other",
		Publications: map[string]workspace.DashboardPublication{"website": testCompiledPublication("other-digest")},
	})
	current := mustGetPublication(t, repo, ctx, "visuals", "website")
	if current.ProjectID != "other-site" || current.Status() != publication.StatusActive {
		t.Fatalf("current publication = %#v status=%s", current, current.Status())
	}
	if _, err := repo.GetByPublicID(ctx, prior.PublicID); !errors.Is(err, publication.ErrNotFound) {
		t.Fatalf("prior project public id error = %v, want not found", err)
	}
	var configured int
	if err := db.QueryRowContext(ctx, `SELECT configured FROM dashboard_publications WHERE project_id = 'site' AND workspace_id = 'visuals' AND name = 'website'`).Scan(&configured); err != nil {
		t.Fatal(err)
	}
	if configured != 0 {
		t.Fatalf("prior project configured = %d, want 0", configured)
	}
}

func seedPublicationWorkspace(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		`INSERT INTO workspaces (id, title) VALUES ('visuals', 'Visuals')`,
		`INSERT INTO serving_states (id, workspace_id, project_id, environment, status, source) VALUES ('state_1', 'visuals', 'site', 'prod', 'validated', 'publish')`,
		`INSERT INTO serving_states (id, workspace_id, project_id, environment, status, source) VALUES ('state_2', 'visuals', 'site', 'prod', 'validated', 'publish')`,
		`INSERT INTO serving_states (id, workspace_id, project_id, environment, status, source) VALUES ('state_3', 'visuals', 'site', 'prod', 'validated', 'publish')`,
		`INSERT INTO serving_states (id, workspace_id, project_id, environment, status, source) VALUES ('state_4', 'visuals', 'site', 'prod', 'validated', 'publish')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func reconcilePublications(t *testing.T, ctx context.Context, db *sql.DB, input publication.ReconcileInput) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReconcileTx(ctx, tx, input); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func testCompiledPublication(digest string) workspace.DashboardPublication {
	return workspace.DashboardPublication{Name: "website", Dashboard: "visual-showcase", DefaultPage: "overview", AllowedOrigins: []string{"https://leapview.dev"}, DependencyAssetIDs: []string{"dashboard:visuals.visual-showcase"}, ConfigurationDigest: digest}
}

func mustPublication(t *testing.T, row publication.Publication, err error) publication.Publication {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func mustGetPublication(t *testing.T, repo *Repository, ctx context.Context, workspaceID, name string) publication.Publication {
	t.Helper()
	row, err := repo.Get(ctx, workspaceID, name)
	return mustPublication(t, row, err)
}
