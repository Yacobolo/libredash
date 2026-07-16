package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/release"
)

func TestReleaseLifecycleIsIdempotentAndImmutable(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewRepository(store.SQLDB())
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO workspaces (id, title, description) VALUES ('sales', 'Sales', '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQLDB().ExecContext(t.Context(), `INSERT INTO serving_states (id, workspace_id, project_id, environment, status, created_by) VALUES ('state_1', 'sales', 'commerce', 'dev', 'pending', 'principal'), ('state_2', 'sales', 'commerce', 'dev', 'pending', 'principal')`); err != nil {
		t.Fatal(err)
	}
	input := release.CreateInput{
		ID: "rel_1", ProjectID: "commerce", ProjectDigest: "sha256:project", RequestDigest: "sha256:request",
		IdempotencyKey: "release-1", CreatedBy: "principal",
		Workspaces:  []release.WorkspaceManifest{{WorkspaceID: "sales", ArtifactDigest: "sha256:artifact"}},
		Connections: []release.ConnectionPin{{ConnectionID: "warehouse", RevisionID: "sha256:revision"}},
	}
	created, err := repo.Create(t.Context(), input)
	if err != nil || created.Status != release.StatusDraft {
		t.Fatalf("Create() = %#v, %v", created, err)
	}
	replayed, err := repo.Create(t.Context(), input)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("replay = %#v, %v", replayed, err)
	}
	conflict := input
	conflict.RequestDigest = "sha256:different"
	if _, err := repo.Create(t.Context(), conflict); !errors.Is(err, release.ErrConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}

	if err := repo.AssignArtifactTarget(t.Context(), created.ProjectID, created.ID, "sales", "state_1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordArtifact(t.Context(), release.Artifact{ReleaseID: created.ID, WorkspaceID: "sales", ExpectedDigest: "sha256:artifact", ServingStateID: "state_1", SizeBytes: 42}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.BeginFinalization(t.Context(), created.ProjectID, created.ID); err != nil {
		t.Fatal(err)
	}
	if resumed, err := repo.BeginFinalization(t.Context(), created.ProjectID, created.ID); err != nil || resumed.Status != release.StatusValidating {
		t.Fatalf("resume BeginFinalization() = %#v, %v", resumed, err)
	}
	ready, err := repo.CompleteFinalization(t.Context(), created.ProjectID, created.ID, map[string]string{"sales": "sha256:artifact"})
	if err != nil || ready.Status != release.StatusReady || ready.FinalizedAt == "" {
		t.Fatalf("CompleteFinalization() = %#v, %v", ready, err)
	}
	if err := repo.RecordArtifact(t.Context(), release.Artifact{ReleaseID: created.ID, WorkspaceID: "sales", ExpectedDigest: "sha256:artifact", ServingStateID: "state_2", SizeBytes: 1}); !errors.Is(err, release.ErrImmutable) {
		t.Fatalf("post-finalize artifact error = %v", err)
	}
}

func TestReleaseFinalizationRejectsMissingOrMismatchedArtifacts(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewRepository(store.SQLDB())
	created, err := repo.Create(t.Context(), release.CreateInput{
		ID: "rel_2", ProjectID: "commerce", ProjectDigest: "sha256:project", RequestDigest: "sha256:req-2", IdempotencyKey: "release-2", CreatedBy: "principal",
		Workspaces: []release.WorkspaceManifest{{WorkspaceID: "sales", ArtifactDigest: "sha256:expected"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.BeginFinalization(t.Context(), created.ProjectID, created.ID); !errors.Is(err, release.ErrIncomplete) {
		t.Fatalf("missing artifact error = %v", err)
	}
}
