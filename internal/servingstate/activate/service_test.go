package activate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestServiceActivatesPreparedRuntime(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Status: servingstate.StatusValidated},
	}
	runtime := &fakeRuntime{}
	service := NewService(repo, runtime)

	activated, err := service.Activate(ctx, "dep_1")
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if activated.Status != servingstate.StatusActive {
		t.Fatalf("status = %q, want active", activated.Status)
	}
	if runtime.prepareID != "dep_1" || runtime.commitCalls != 1 {
		t.Fatalf("runtime prepare=%q commits=%d, want dep_1/1", runtime.prepareID, runtime.commitCalls)
	}
}

func TestServiceRecordsPreparedDuckLakeSnapshotBeforeActivation(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Status: servingstate.StatusValidated},
	}
	runtime := &fakeRuntime{snapshotID: 42}
	service := NewService(repo, runtime)

	activated, err := service.Activate(ctx, "dep_1")
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if activated.DuckLakeSnapshotID != 42 {
		t.Fatalf("activated snapshot = %d, want 42", activated.DuckLakeSnapshotID)
	}
	if repo.snapshotServingStateID != "dep_1" || repo.snapshotID != 42 {
		t.Fatalf("recorded snapshot = %s/%d, want dep_1/42", repo.snapshotServingStateID, repo.snapshotID)
	}
	if repo.snapshotRecordOrder == 0 || repo.activateOrder == 0 || repo.snapshotRecordOrder > repo.activateOrder {
		t.Fatalf("record/activate order = %d/%d, want record before activate", repo.snapshotRecordOrder, repo.activateOrder)
	}
}

func TestServiceReconcilesAccessPolicyFromValidatedArtifact(t *testing.T) {
	ctx := context.Background()
	servingStateID := servingstate.ID("dep_access")
	projectPath := filepath.Join("..", "..", "..", "dashboards", servingstatefs.ProjectFile)
	var bundle bytes.Buffer
	if _, _, err := servingstatefs.PackProject(projectPath, "sales", servingStateID, &bundle); err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(artifactPath, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepo{
		deployment: servingstate.State{ID: servingStateID, WorkspaceID: "sales", Status: servingstate.StatusValidated},
		artifact:   servingstate.Artifact{ServingStateID: servingStateID, WorkspaceID: "sales", Path: artifactPath},
	}
	reconciler := &fakeAccessReconciler{}
	service := NewServiceWithAccess(repo, &fakeRuntime{}, repo, reconciler)

	if _, err := service.Activate(ctx, servingStateID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if _, ok := repo.policy.Groups["analysts"]; !ok {
		t.Fatalf("transaction policy groups = %#v, want analysts", repo.policy.Groups)
	}
	if _, ok := repo.policy.RoleBindings["analysts-viewer"]; !ok {
		t.Fatalf("transaction policy role bindings = %#v, want analysts-viewer", repo.policy.RoleBindings)
	}
	if repo.activateWithPolicyCalls != 1 {
		t.Fatalf("activate with policy calls = %d, want 1", repo.activateWithPolicyCalls)
	}
}

func TestServicePrepareFailureLeavesDeploymentUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Status: servingstate.StatusValidated},
	}
	runtime := &fakeRuntime{prepareErr: errors.New("load failed")}
	service := NewService(repo, runtime)

	if _, err := service.Activate(ctx, "dep_1"); err == nil {
		t.Fatal("expected prepare error")
	}
	if repo.activateCalls != 0 {
		t.Fatalf("activate calls = %d, want 0", repo.activateCalls)
	}
}

func TestServiceRejectsInvalidStatusBeforePrepare(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Status: servingstate.StatusPending},
	}
	runtime := &fakeRuntime{}
	service := NewService(repo, runtime)

	if _, err := service.Activate(ctx, "dep_1"); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("error = %v, want ErrInvalidStatus", err)
	}
	if runtime.prepareID != "" {
		t.Fatalf("prepared invalid deployment %q", runtime.prepareID)
	}
}

type fakeRepo struct {
	deployment              servingstate.State
	artifact                servingstate.Artifact
	activateErr             error
	activateCalls           int
	activateWithPolicyCalls int
	policy                  workspace.AccessPolicy
	snapshotServingStateID  servingstate.ID
	snapshotID              int64
	snapshotRecordOrder     int
	activateOrder           int
	order                   int
}

func (r *fakeRepo) ByID(context.Context, servingstate.ID) (servingstate.State, error) {
	return r.deployment, nil
}

func (r *fakeRepo) Activate(context.Context, servingstate.WorkspaceID, servingstate.Environment, servingstate.ID) (servingstate.State, error) {
	r.activateCalls++
	r.order++
	r.activateOrder = r.order
	if r.activateErr != nil {
		return servingstate.State{}, r.activateErr
	}
	r.deployment.Status = servingstate.StatusActive
	return r.deployment, nil
}

func (r *fakeRepo) ActivateWithWorkspacePolicy(_ context.Context, _ servingstate.WorkspaceID, _ servingstate.Environment, _ servingstate.ID, policy workspace.AccessPolicy) (servingstate.State, error) {
	r.activateWithPolicyCalls++
	r.order++
	r.activateOrder = r.order
	r.policy = policy
	if r.activateErr != nil {
		return servingstate.State{}, r.activateErr
	}
	r.deployment.Status = servingstate.StatusActive
	return r.deployment, nil
}

func (r *fakeRepo) RecordDuckLakeSnapshot(_ context.Context, servingStateID servingstate.ID, snapshotID int64) error {
	r.order++
	r.snapshotRecordOrder = r.order
	r.snapshotServingStateID = servingStateID
	r.snapshotID = snapshotID
	r.deployment.DuckLakeSnapshotID = snapshotID
	return nil
}

func (r *fakeRepo) ArtifactByServingState(context.Context, servingstate.ID) (servingstate.Artifact, error) {
	return r.artifact, nil
}

type fakeRuntime struct {
	prepareID   string
	prepareErr  error
	commitCalls int
	snapshotID  int64
}

func (r *fakeRuntime) PrepareServingState(_ context.Context, servingStateID string) (servingstate.PreparedRuntime, error) {
	r.prepareID = servingStateID
	if r.prepareErr != nil {
		return nil, r.prepareErr
	}
	return fakePrepared{snapshotID: r.snapshotID}, nil
}

func (r *fakeRuntime) CommitPrepared(servingstate.PreparedRuntime) error {
	r.commitCalls++
	return nil
}

type fakePrepared struct {
	snapshotID int64
}

func (fakePrepared) Close() error { return nil }

func (p fakePrepared) DuckLakeSnapshotID() int64 { return p.snapshotID }

type fakeAccessReconciler struct {
	workspaceID string
	policy      workspace.AccessPolicy
}

func (r *fakeAccessReconciler) ReconcileWorkspacePolicy(_ context.Context, workspaceID string, policy workspace.AccessPolicy) error {
	r.workspaceID = workspaceID
	r.policy = policy
	return nil
}
