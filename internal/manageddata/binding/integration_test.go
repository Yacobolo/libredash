package binding

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/manageddata"
	manageddatasqlite "github.com/Yacobolo/libredash/internal/manageddata/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestBinderPinsCompiledArtifactToCurrentSQLiteRevision(t *testing.T) {
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
	activateRevision(t, ctx, repository, collection.ID, firstRevision.ID, firstTarget.ID, manageddata.PointerExpectation{})

	validation := validateManagedProjectArtifact(t, candidate)
	binder, err := New(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := binder.AfterArtifactValidation(ctx, candidate, validation); err != nil {
		t.Fatalf("pin current revision: %v", err)
	}
	bindings, err := repository.ListServingStateBindings(ctx, string(candidate.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].CollectionID != collection.ID || bindings[0].RevisionID != firstRevision.ID || bindings[0].Environment != "prod" {
		t.Fatalf("bindings = %#v, want first revision", bindings)
	}

	secondRevision := createReadyRevision(t, ctx, repository, collection.ID, "orders-v2.csv", "b")
	secondTarget := createValidatedState(t, ctx, store, servingStates, "sales", "prod")
	activateRevision(t, ctx, repository, collection.ID, secondRevision.ID, secondTarget.ID, manageddata.PointerExpectation{
		RevisionID: firstRevision.ID,
		Generation: 1,
	})
	bindings, err = repository.ListServingStateBindings(ctx, string(candidate.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].RevisionID != firstRevision.ID {
		t.Fatalf("later rollout mutated pinned publish bindings: %#v", bindings)
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
	if _, err := store.SQLDB().ExecContext(ctx, `UPDATE serving_states SET status = 'validated' WHERE id = ?`, state.ID); err != nil {
		t.Fatal(err)
	}
	state.Status = servingstate.StatusValidated
	return state
}

func activateRevision(t *testing.T, ctx context.Context, repository *manageddatasqlite.Repository, collectionID, revisionID string, targetID servingstate.ID, expectation manageddata.PointerExpectation) {
	t.Helper()
	rollout, err := repository.CreateRollout(ctx, manageddata.CreateRolloutInput{
		CollectionID: collectionID,
		Environment:  "prod",
		RevisionID:   revisionID,
		Targets: []manageddata.RolloutTargetInput{{
			WorkspaceID: "sales", ServingStateID: string(targetID),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ActivateRollout(ctx, rollout.ID, expectation); err != nil {
		t.Fatal(err)
	}
}

func validateManagedProjectArtifact(t *testing.T, candidate servingstate.State) servingstate.Validation {
	t.Helper()
	projectPath := writeManagedProject(t)
	var bundle bytes.Buffer
	if _, _, err := servingstatefs.PackProjectAgainstGraphForEnvironment(projectPath, "sales", "prod", candidate.ID, workspace.AssetGraph{}, &bundle); err != nil {
		t.Fatalf("pack project: %v", err)
	}
	path := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	validation, err := (servingstatefs.Validator{}).ValidateArtifact(path, "sales", "prod", candidate.ID)
	if err != nil {
		t.Fatalf("validate artifact: %v", err)
	}
	t.Cleanup(func() { _ = (servingstatefs.Validator{}).Cleanup(validation) })
	return validation
}

func writeManagedProject(t *testing.T) string {
	t.Helper()
	files := map[string]string{
		"libredash.yaml": `
apiVersion: libredash.dev/v1
kind: Project
metadata:
  name: project-a
spec:
  connections:
    include: [connections/*.yaml]
  sources:
    include: [sources/*.yaml]
  workspaces:
    include: [workspaces/*/workspace.yaml]
`,
		"connections/orders.yaml": `
apiVersion: libredash.dev/v1
kind: Connection
metadata:
  name: orders
spec:
  kind: managed
  credentials:
    provider: none
`,
		"sources/orders.orders.yaml": `
apiVersion: libredash.dev/v1
kind: Source
metadata:
  name: orders.orders
spec:
  connection: orders
  path: orders.csv
  format: csv
  fields:
    order_id:
      type: string
`,
		"workspaces/sales/workspace.yaml": `
apiVersion: libredash.dev/v1
kind: Workspace
metadata:
  name: sales
spec:
  uses:
    sources: [orders.orders]
  models:
    include: [models/*.yaml]
  semanticModels:
    include: [semantic-models/*.yaml]
  dashboards:
    include: []
  access:
    include: []
  agentPolicy:
    include: []
`,
		"workspaces/sales/models/orders.yaml": `
apiVersion: libredash.dev/v1
kind: ModelTable
metadata:
  workspace: sales
  name: orders
spec:
  source: orders.orders
  primaryKey: order_id
  fields:
    order_id:
      label: Order ID
`,
		"workspaces/sales/semantic-models/sales.yaml": `
apiVersion: libredash.dev/v1
kind: SemanticModel
metadata:
  workspace: sales
  name: sales
spec:
  baseTable: orders
  tables: [orders]
  measures: {}
`,
	}
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "libredash.yaml")
}
