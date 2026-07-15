package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestCollectionIdentityUsesProjectAndConnection(t *testing.T) {
	ctx, _, repo := testRepository(t)
	first, err := repo.CreateCollection(ctx, manageddata.CreateCollectionInput{ID: "orders-a", ProjectID: "project-a", ConnectionName: "warehouse", Name: "Orders A"})
	if err != nil {
		t.Fatalf("create first collection: %v", err)
	}
	second, err := repo.CreateCollection(ctx, manageddata.CreateCollectionInput{ID: "orders-b", ProjectID: "project-b", ConnectionName: "warehouse", Name: "Orders B"})
	if err != nil {
		t.Fatalf("same connection name in another project: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("collections share ID %q", first.ID)
	}

	got, err := repo.CollectionByProjectConnection(ctx, "project-a", "warehouse")
	if err != nil {
		t.Fatalf("lookup collection: %v", err)
	}
	if got.ID != first.ID || got.ProjectID != "project-a" || got.ConnectionName != "warehouse" {
		t.Fatalf("lookup = %#v", got)
	}

	retry, err := repo.CreateCollection(ctx, manageddata.CreateCollectionInput{ID: first.ID, ProjectID: "project-a", ConnectionName: "warehouse", Name: "Orders A"})
	if err != nil {
		t.Fatalf("idempotent create retry: %v", err)
	}
	if retry.ID != first.ID {
		t.Fatalf("retry ID = %q, want %q", retry.ID, first.ID)
	}
	_, err = repo.CreateCollection(ctx, manageddata.CreateCollectionInput{ID: "different", ProjectID: "project-a", ConnectionName: "warehouse", Name: "Conflicting"})
	if !errors.Is(err, manageddata.ErrConflict) {
		t.Fatalf("conflicting project+connection error = %v, want conflict", err)
	}
}

func TestCompleteUploadCreatesImmutableRevisionAtomically(t *testing.T) {
	ctx, store, repo := testRepository(t)
	collection := createCollection(t, ctx, repo, "customers", "project-a", "customers")
	manifest := manageddata.Manifest{Files: []manageddata.File{
		{Path: "customers.csv", Size: 12, SHA256: strings.Repeat("a", 64)},
		{Path: "regions.csv", Size: 7, SHA256: strings.Repeat("b", 64)},
	}}
	session, err := repo.CreateUploadSession(ctx, manageddata.CreateUploadSessionInput{
		ID: "upload-1", CollectionID: collection.ID, Manifest: manifest, StorageBackend: "local",
		StagingPrefix: "sessions/upload-1", CreatedBy: "principal-1", ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	revision, err := repo.CompleteUpload(ctx, manageddata.CompleteUploadInput{SessionID: session.ID, Files: []manageddata.StoredFile{
		{File: manifest.Files[1], StorageKey: "objects/b", MediaType: "text/csv", ETag: "etag-b"},
		{File: manifest.Files[0], StorageKey: "objects/a", MediaType: "text/csv", ETag: "etag-a"},
	}})
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	if revision.Digest != manifest.RevisionID() || revision.Status != manageddata.RevisionStatusReady || revision.Sequence != 1 {
		t.Fatalf("revision = %#v", revision)
	}
	files, err := repo.ListRevisionFiles(ctx, revision.ID)
	if err != nil {
		t.Fatalf("list revision files: %v", err)
	}
	if len(files) != 2 || files[0].Path != "customers.csv" || files[1].Path != "regions.csv" {
		t.Fatalf("files = %#v", files)
	}
	completed, err := repo.UploadSessionByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if completed.Status != manageddata.UploadStatusComplete || completed.RevisionID != revision.ID {
		t.Fatalf("completed session = %#v", completed)
	}
	if _, err := store.ExecContext(ctx, `UPDATE managed_data_revisions SET digest = ? WHERE id = ?`, "sha256:"+strings.Repeat("f", 64), revision.ID); err == nil {
		t.Fatal("ready revision metadata was mutable")
	}
}

func TestCompleteUploadRollsBackWhenStoredFilesDoNotMatchManifest(t *testing.T) {
	ctx, store, repo := testRepository(t)
	collection := createCollection(t, ctx, repo, "orders", "project-a", "orders")
	manifest := manageddata.Manifest{Files: []manageddata.File{{Path: "orders.csv", Size: 4, SHA256: strings.Repeat("a", 64)}}}
	session, err := repo.CreateUploadSession(ctx, manageddata.CreateUploadSessionInput{ID: "upload-bad", CollectionID: collection.ID, Manifest: manifest, StorageBackend: "local", StagingPrefix: "staging/upload-bad", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CompleteUpload(ctx, manageddata.CompleteUploadInput{SessionID: session.ID, Files: []manageddata.StoredFile{{File: manageddata.File{Path: "orders.csv", Size: 5, SHA256: strings.Repeat("b", 64)}, StorageKey: "objects/bad"}}})
	if err == nil {
		t.Fatal("CompleteUpload() unexpectedly succeeded")
	}
	var revisionCount int
	if err := store.QueryRowContext(ctx, `SELECT count(*) FROM managed_data_revisions`).Scan(&revisionCount); err != nil {
		t.Fatal(err)
	}
	if revisionCount != 0 {
		t.Fatalf("revision count = %d, want 0", revisionCount)
	}
	got, err := repo.UploadSessionByID(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != manageddata.UploadStatusOpen {
		t.Fatalf("session status = %q, want open", got.Status)
	}
}

func TestServingStateBindingsAllowMultipleCollections(t *testing.T) {
	ctx, store, repo := testRepository(t)
	firstCollection, firstRevision := readyRevision(t, ctx, repo, "inventory", "project-a", "inventory", "inventory.csv", "c")
	secondCollection, secondRevision := readyRevision(t, ctx, repo, "prices", "project-a", "prices", "prices.csv", "d")
	insertWorkspaceState(t, ctx, store, "workspace-1", "state-1", "prod", "validated")
	bindings := []manageddata.ServingStateBinding{
		{CollectionID: firstCollection.ID, RevisionID: firstRevision.ID, Environment: "prod"},
		{CollectionID: secondCollection.ID, RevisionID: secondRevision.ID, Environment: "prod"},
	}
	if err := repo.ReplaceServingStateBindings(ctx, "state-1", bindings); err != nil {
		t.Fatalf("replace bindings: %v", err)
	}
	got, err := repo.ListServingStateBindings(ctx, "state-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].CollectionID != firstCollection.ID || got[1].CollectionID != secondCollection.ID {
		t.Fatalf("bindings = %#v", got)
	}
}

func testRepository(t *testing.T) (context.Context, *sql.DB, *Repository) {
	t.Helper()
	ctx := context.Background()
	store, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "libredash.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	store.SetMaxOpenConns(1)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, store, "../../platform/migrations"); err != nil {
		_ = store.Close()
		t.Fatalf("migrate platform store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return ctx, store, NewRepository(store)
}

func createCollection(t *testing.T, ctx context.Context, repo *Repository, id, projectID, connectionName string) manageddata.Collection {
	t.Helper()
	collection, err := repo.CreateCollection(ctx, manageddata.CreateCollectionInput{ID: id, ProjectID: projectID, ConnectionName: connectionName, Name: connectionName})
	if err != nil {
		t.Fatal(err)
	}
	return collection
}

func readyRevision(t *testing.T, ctx context.Context, repo *Repository, id, projectID, connectionName, path, digestChar string) (manageddata.Collection, manageddata.Revision) {
	t.Helper()
	collection := createCollection(t, ctx, repo, id, projectID, connectionName)
	manifest := manageddata.Manifest{Files: []manageddata.File{{Path: path, Size: 1, SHA256: strings.Repeat(digestChar, 64)}}}
	session, err := repo.CreateUploadSession(ctx, manageddata.CreateUploadSessionInput{CollectionID: collection.ID, Manifest: manifest, StorageBackend: "local", StagingPrefix: "staging/" + path, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := repo.CompleteUpload(ctx, manageddata.CompleteUploadInput{SessionID: session.ID, Files: []manageddata.StoredFile{{File: manifest.Files[0], StorageKey: "objects/" + digestChar}}})
	if err != nil {
		t.Fatal(err)
	}
	return collection, revision
}

func insertWorkspaceState(t *testing.T, ctx context.Context, db *sql.DB, workspaceID, stateID, environment, status string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO workspaces (id, title) VALUES (?, ?)`, workspaceID, workspaceID); err != nil {
		t.Fatal(err)
	}
	insertServingState(t, ctx, db, workspaceID, stateID, environment, status)
}

func insertServingState(t *testing.T, ctx context.Context, db *sql.DB, workspaceID, stateID, environment, status string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO serving_states (id, workspace_id, environment, status, source) VALUES (?, ?, ?, ?, 'publish')`, stateID, workspaceID, environment, status); err != nil {
		t.Fatal(err)
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
		t.Fatalf("active serving state for %s = %q, want %q", workspaceID, got, want)
	}
}

func assertServingStateStatus(t *testing.T, ctx context.Context, db *sql.DB, stateID, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(ctx, `SELECT status FROM serving_states WHERE id = ?`, stateID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("serving state %s status = %q, want %q", stateID, got, want)
	}
}
