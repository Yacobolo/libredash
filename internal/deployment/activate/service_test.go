package activate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentfs "github.com/Yacobolo/libredash/internal/deployment/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestServiceActivatesPreparedRuntime(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Status: deployment.StatusValidated},
	}
	runtime := &fakeRuntime{}
	service := NewService(repo, runtime)

	activated, err := service.Activate(ctx, "dep_1")
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if activated.Status != deployment.StatusActive {
		t.Fatalf("status = %q, want active", activated.Status)
	}
	if runtime.prepareID != "dep_1" || runtime.commitCalls != 1 {
		t.Fatalf("runtime prepare=%q commits=%d, want dep_1/1", runtime.prepareID, runtime.commitCalls)
	}
}

func TestServiceReconcilesAccessPolicyFromValidatedArtifact(t *testing.T) {
	ctx := context.Background()
	deploymentID := deployment.ID("dep_access")
	projectPath := filepath.Join("..", "..", "..", "dashboards", deploymentfs.ProjectFile)
	var bundle bytes.Buffer
	if _, _, err := deploymentfs.PackProject(projectPath, "sales", deploymentID, &bundle); err != nil {
		t.Fatalf("PackProject() error = %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "artifact.tar.gz")
	if err := os.WriteFile(artifactPath, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: deploymentID, WorkspaceID: "sales", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: deploymentID, WorkspaceID: "sales", Path: artifactPath},
	}
	reconciler := &fakeAccessReconciler{}
	service := NewServiceWithAccess(repo, &fakeRuntime{}, repo, reconciler)

	if _, err := service.Activate(ctx, deploymentID); err != nil {
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
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Status: deployment.StatusValidated},
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
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Status: deployment.StatusPending},
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
	deployment              deployment.Deployment
	artifact                deployment.Artifact
	activateErr             error
	activateCalls           int
	activateWithPolicyCalls int
	policy                  workspace.AccessPolicy
}

func (r *fakeRepo) ByID(context.Context, deployment.ID) (deployment.Deployment, error) {
	return r.deployment, nil
}

func (r *fakeRepo) Activate(context.Context, deployment.WorkspaceID, deployment.Environment, deployment.ID) (deployment.Deployment, error) {
	r.activateCalls++
	if r.activateErr != nil {
		return deployment.Deployment{}, r.activateErr
	}
	r.deployment.Status = deployment.StatusActive
	return r.deployment, nil
}

func (r *fakeRepo) ActivateWithWorkspacePolicy(_ context.Context, _ deployment.WorkspaceID, _ deployment.Environment, _ deployment.ID, policy workspace.AccessPolicy) (deployment.Deployment, error) {
	r.activateWithPolicyCalls++
	r.policy = policy
	if r.activateErr != nil {
		return deployment.Deployment{}, r.activateErr
	}
	r.deployment.Status = deployment.StatusActive
	return r.deployment, nil
}

func (r *fakeRepo) ArtifactByDeployment(context.Context, deployment.ID) (deployment.Artifact, error) {
	return r.artifact, nil
}

type fakeRuntime struct {
	prepareID   string
	prepareErr  error
	commitCalls int
}

func (r *fakeRuntime) PrepareDeployment(_ context.Context, deploymentID string) (deployment.PreparedRuntime, error) {
	r.prepareID = deploymentID
	if r.prepareErr != nil {
		return nil, r.prepareErr
	}
	return fakePrepared{}, nil
}

func (r *fakeRuntime) CommitPrepared(deployment.PreparedRuntime) error {
	r.commitCalls++
	return nil
}

type fakePrepared struct{}

func (fakePrepared) Close() error { return nil }

type fakeAccessReconciler struct {
	workspaceID string
	policy      workspace.AccessPolicy
}

func (r *fakeAccessReconciler) ReconcileWorkspacePolicy(_ context.Context, workspaceID string, policy workspace.AccessPolicy) error {
	r.workspaceID = workspaceID
	r.policy = policy
	return nil
}
