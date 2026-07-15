package binding

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentsqlite "github.com/Yacobolo/libredash/internal/deployment/sqlite"
	"github.com/Yacobolo/libredash/internal/manageddata"
	manageddatasqlite "github.com/Yacobolo/libredash/internal/manageddata/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestBinderPinsRevisionAfterEnvironmentPointerChanges(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "sales", Title: "Sales"}); err != nil {
		t.Fatal(err)
	}
	servingStates := servingstatesqlite.NewRepository(store.SQLDB())
	candidate, err := servingStates.Create(ctx, servingstate.CreateInput{WorkspaceID: "sales", Environment: "prod"})
	if err != nil {
		t.Fatal(err)
	}

	repository := manageddatasqlite.NewRepository(store.SQLDB())
	collection, err := repository.CreateCollection(ctx, manageddata.CreateCollectionInput{
		ID: "orders", ProjectID: "project-a", ConnectionName: "orders", Name: "Orders",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstRevision := createReadyRevision(t, ctx, repository, collection.ID, "orders-v1.csv", "a")
	firstTarget := createValidatedState(t, ctx, store, servingStates, "sales", "prod")
	activateRevision(t, ctx, store, repository, collection.ID, firstRevision.ID, firstTarget.ID)

	validation := servingstate.Validation{
		ProjectID:            "project-a",
		ManagedDataRevisions: map[string]string{"orders": firstRevision.Digest},
	}
	secondRevision := createReadyRevision(t, ctx, repository, collection.ID, "orders-v2.csv", "b")
	secondTarget := createValidatedState(t, ctx, store, servingStates, "sales", "prod")
	activateRevision(t, ctx, store, repository, collection.ID, secondRevision.ID, secondTarget.ID)
	binder, err := New(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := binder.AfterArtifactValidation(ctx, candidate, validation); err != nil {
		t.Fatalf("pin artifact revision: %v", err)
	}
	bindings, err := repository.ListServingStateBindings(ctx, string(candidate.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].RevisionID != firstRevision.ID {
		t.Fatalf("later deployment mutated pinned publish bindings: %#v", bindings)
	}
}

func createReadyRevision(t *testing.T, ctx context.Context, repository *manageddatasqlite.Repository, collectionID, path, digestCharacter string) manageddata.Revision {
	t.Helper()
	manifest := manageddata.Manifest{Files: []manageddata.File{{
		Path: path, Size: 1, SHA256: strings.Repeat(digestCharacter, 64),
	}}}
	session, err := repository.CreateUploadSession(ctx, manageddata.CreateUploadSessionInput{
		CollectionID: collectionID, Manifest: manifest, StorageBackend: "local",
		StagingPrefix: "staging/" + path, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := repository.CompleteUpload(ctx, manageddata.CompleteUploadInput{
		SessionID: session.ID,
		Files:     []manageddata.StoredFile{{File: manifest.Files[0], StorageKey: "objects/" + digestCharacter}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func createValidatedState(t *testing.T, ctx context.Context, store *platform.Store, repository *servingstatesqlite.Repository, workspaceID string, environment servingstate.Environment) servingstate.State {
	t.Helper()
	state, err := repository.Create(ctx, servingstate.CreateInput{WorkspaceID: servingstate.WorkspaceID(workspaceID), Environment: environment})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, `UPDATE serving_states SET status = 'validated', project_id = 'project-a', project_digest = ?, project_workspaces_json = ? WHERE id = ?`, "sha256:"+strings.Repeat("a", 64), `["`+workspaceID+`"]`, state.ID); err != nil {
		t.Fatal(err)
	}
	state.Status = servingstate.StatusValidated
	return state
}

func activateRevision(t *testing.T, ctx context.Context, store *platform.Store, repository *manageddatasqlite.Repository, collectionID, revisionID string, targetID servingstate.ID) {
	t.Helper()
	if err := repository.ReplaceServingStateBindings(ctx, string(targetID), []manageddata.ServingStateBinding{{
		ServingStateID: string(targetID), CollectionID: collectionID, RevisionID: revisionID, Environment: "prod",
	}}); err != nil {
		t.Fatal(err)
	}
	deployments := deploymentsqlite.NewRepository(store.SQLDB())
	created, err := deployments.CreateDeployment(ctx, deployment.CreateInput{
		ID: "deployment-" + revisionID, ProjectID: "project-a", Environment: "prod", RequestDigest: "sha256:" + strings.Repeat("d", 64),
		Targets: []deployment.TargetInput{{WorkspaceID: "sales", ServingStateID: string(targetID)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deployments.ActivateDeployment(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}
