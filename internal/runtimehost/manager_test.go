package runtimehost

import (
	"context"
	"errors"
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment"
)

func TestManagerReloadIgnoresMissingActiveDeployment(t *testing.T) {
	manager := NewManagerWithFactory(&fakeRepo{activeErr: deployment.ErrNotFound}, "test", "/data", &fakeFactory{})

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
}

func TestManagerPrepareCommitSwapsRuntimeAndClosesOld(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", Digest: "digest"},
	}
	factory := &fakeFactory{}
	manager := NewManagerWithFactory(repo, "test", "/data", factory)

	prepared, err := manager.PrepareDeployment(ctx, "dep_1")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := manager.CommitPrepared(prepared); err != nil {
		t.Fatalf("commit: %v", err)
	}
	active, err := manager.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if active == nil {
		t.Fatal("active runtime is nil")
	}

	second, err := manager.PrepareDeployment(ctx, "dep_1")
	if err != nil {
		t.Fatalf("prepare second: %v", err)
	}
	if err := manager.CommitPrepared(second); err != nil {
		t.Fatalf("commit second: %v", err)
	}
	if factory.prepareCalls != 1 {
		t.Fatalf("factory calls = %d, want no-change reuse", factory.prepareCalls)
	}
}

func TestManagerRejectsPreparedFromDifferentHost(t *testing.T) {
	manager := NewManagerWithFactory(&fakeRepo{}, "test", "/data", &fakeFactory{})
	if err := manager.CommitPrepared(fakePrepared{}); err == nil {
		t.Fatal("expected wrong prepared runtime error")
	}
}

func TestManagerCloseClearsActiveRuntime(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", Digest: "digest"},
	}
	manager := NewManagerWithFactory(repo, "test", "/data", &fakeFactory{})
	prepared, err := manager.PrepareDeployment(ctx, "dep_1")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := manager.CommitPrepared(prepared); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := manager.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := manager.Active(); err == nil {
		t.Fatal("expected no active runtime after close")
	}
}

type fakeRepo struct {
	deployment deployment.Deployment
	artifact   deployment.Artifact
	activeErr  error
}

func (r *fakeRepo) ActiveArtifact(context.Context, deployment.WorkspaceID, deployment.Environment) (deployment.Deployment, deployment.Artifact, error) {
	if r.activeErr != nil {
		return deployment.Deployment{}, deployment.Artifact{}, r.activeErr
	}
	return r.deployment, r.artifact, nil
}

func (r *fakeRepo) ByID(context.Context, deployment.ID) (deployment.Deployment, error) {
	if r.deployment.ID == "" {
		return deployment.Deployment{}, deployment.ErrNotFound
	}
	return r.deployment, nil
}

func (r *fakeRepo) ArtifactByDeployment(context.Context, deployment.ID) (deployment.Artifact, error) {
	if r.artifact.Digest == "" {
		return deployment.Artifact{}, deployment.ErrNotFound
	}
	return r.artifact, nil
}

type fakeFactory struct {
	prepareCalls int
	err          error
}

func (f *fakeFactory) Prepare(context.Context, RuntimeInput) (Runtime, error) {
	f.prepareCalls++
	if f.err != nil {
		return nil, f.err
	}
	return &fakeRuntime{}, nil
}

type fakeRuntime struct {
	closed bool
}

func (r *fakeRuntime) Close() error {
	r.closed = true
	return nil
}

type fakePrepared struct{}

func (fakePrepared) Close() error { return errors.New("unused") }
