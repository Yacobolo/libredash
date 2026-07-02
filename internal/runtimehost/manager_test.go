package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment"
)

func TestManagerReloadIgnoresMissingActiveDeployment(t *testing.T) {
	manager := NewManagerWithFactory(ManagerOptions{Repo: &fakeRepo{activeErr: deployment.ErrNotFound}, WorkspaceID: "test", Environment: "dev", DataDir: "/data", Factory: &fakeFactory{}})

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
}

func TestManagerReloadUsesConfiguredEnvironment(t *testing.T) {
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_prod", WorkspaceID: "test", Environment: "prod", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_prod", Environment: "prod", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "prod", DataDir: "/data", Factory: &fakeFactory{}})

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if repo.activeEnvironment != "prod" {
		t.Fatalf("active environment = %q, want prod", repo.activeEnvironment)
	}
}

func TestManagerPrepareCommitSwapsRuntimeAndClosesOld(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", Digest: "digest"},
	}
	factory := &fakeFactory{}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", DataDir: "/data", Factory: factory})

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

func TestManagerPreparedRuntimeExposesDuckLakeSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		DataDir:     "/data",
		Factory:     &fakeFactory{snapshotID: 42},
	})

	prepared, err := manager.PrepareDeployment(ctx, "dep_1")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	snapshot, ok := prepared.(interface{ DuckLakeSnapshotID() int64 })
	if !ok {
		t.Fatalf("prepared runtime does not expose DuckLakeSnapshotID")
	}
	if snapshot.DuckLakeSnapshotID() != 42 {
		t.Fatalf("snapshot = %d, want 42", snapshot.DuckLakeSnapshotID())
	}
}

func TestManagerReloadRoutesWhenOnlyActiveDeploymentPointerChanges(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: deployment.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "same-digest"},
	}
	factory := &fakeFactory{}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", DataDir: "/data", Factory: factory})

	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("first reload: %v", err)
	}
	active, err := manager.Active()
	if err != nil {
		t.Fatalf("first active: %v", err)
	}
	if got := active.(RuntimeSnapshot).DuckLakeSnapshotID(); got != 11 {
		t.Fatalf("first active snapshot = %d, want 11", got)
	}

	repo.deployment = deployment.Deployment{ID: "dep_2", WorkspaceID: "test", Environment: "dev", Status: deployment.StatusActive, DuckLakeSnapshotID: 22}
	repo.artifact = deployment.Artifact{DeploymentID: "dep_2", WorkspaceID: "test", Environment: "dev", Digest: "same-digest"}
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	active, err = manager.Active()
	if err != nil {
		t.Fatalf("second active: %v", err)
	}
	if got := active.(RuntimeSnapshot).DuckLakeSnapshotID(); got != 22 {
		t.Fatalf("second active snapshot = %d, want 22", got)
	}
	if factory.prepareCalls != 2 {
		t.Fatalf("prepare calls = %d, want reload to prepare both deployment pointers", factory.prepareCalls)
	}
}

func TestManagerRejectsPreparedFromDifferentHost(t *testing.T) {
	manager := NewManagerWithFactory(ManagerOptions{Repo: &fakeRepo{}, WorkspaceID: "test", Environment: "dev", DataDir: "/data", Factory: &fakeFactory{}})
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
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", DataDir: "/data", Factory: &fakeFactory{}})
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

func TestRegistryReloadLoadsConfiguredEnvironmentForEachWorkspace(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.active["sales/prod"] = registryDeploymentArtifact{
		deployment: deployment.Deployment{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"},
	}
	repo.active["operations/prod"] = registryDeploymentArtifact{
		deployment: deployment.Deployment{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"},
	}
	repo.active["sales/dev"] = registryDeploymentArtifact{
		deployment: deployment.Deployment{ID: "dep_sales_dev", WorkspaceID: "sales", Environment: "dev", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_sales_dev", WorkspaceID: "sales", Environment: "dev", Digest: "sales-dev"},
	}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []deployment.WorkspaceID{"sales", "operations", "empty"},
		Environment:  "prod",
		DataDir:      "/data",
		Factory:      factory,
	})

	if err := registry.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := repo.activeCalls; !equalStrings(got, []string{"empty/prod", "operations/prod", "sales/prod"}) {
		t.Fatalf("active calls = %#v, want configured prod workspaces only", got)
	}
	if _, err := registry.ActiveForWorkspace("sales"); err != nil {
		t.Fatalf("sales active: %v", err)
	}
	if _, err := registry.ActiveForWorkspace("operations"); err != nil {
		t.Fatalf("operations active: %v", err)
	}
	if _, err := registry.ActiveForWorkspace("empty"); err == nil {
		t.Fatal("empty workspace active error = nil, want no active deployment")
	}
	if got := factory.inputs; !equalStrings(got, []string{"operations/prod/dep_ops_prod", "sales/prod/dep_sales_prod"}) {
		t.Fatalf("factory inputs = %#v, want only active prod deployments", got)
	}
}

func TestRegistryPrepareCommitRoutesDeploymentByWorkspace(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.deployments["dep_ops_prod"] = deployment.Deployment{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: deployment.StatusValidated}
	repo.artifacts["dep_ops_prod"] = deployment.Artifact{DeploymentID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []deployment.WorkspaceID{"sales"},
		Environment:  "prod",
		DataDir:      "/data",
		Factory:      factory,
	})

	prepared, err := registry.PrepareDeployment(context.Background(), "dep_ops_prod")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := registry.CommitPrepared(prepared); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := registry.ActiveForWorkspace("operations"); err != nil {
		t.Fatalf("operations active after commit: %v", err)
	}
	if _, err := registry.ActiveForWorkspace("sales"); err == nil {
		t.Fatal("sales active error = nil, want only operations runtime committed")
	}
	if got := factory.inputs; !equalStrings(got, []string{"operations/prod/dep_ops_prod"}) {
		t.Fatalf("factory inputs = %#v, want operations only", got)
	}
}

func TestRegistryRejectsPreparedDeploymentFromDifferentEnvironment(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.deployments["dep_ops_dev"] = deployment.Deployment{ID: "dep_ops_dev", WorkspaceID: "operations", Environment: "dev", Status: deployment.StatusValidated}
	repo.artifacts["dep_ops_dev"] = deployment.Artifact{DeploymentID: "dep_ops_dev", WorkspaceID: "operations", Environment: "dev", Digest: "ops-dev"}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []deployment.WorkspaceID{"operations"},
		Environment:  "prod",
		DataDir:      "/data",
		Factory:      &recordingRegistryFactory{},
	})

	if _, err := registry.PrepareDeployment(context.Background(), "dep_ops_dev"); err == nil {
		t.Fatal("prepare error = nil, want environment mismatch")
	}
}

func TestRegistryCloseClosesEveryActiveWorkspaceRuntime(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.active["sales/prod"] = registryDeploymentArtifact{
		deployment: deployment.Deployment{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"},
	}
	repo.active["operations/prod"] = registryDeploymentArtifact{
		deployment: deployment.Deployment{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: deployment.StatusValidated},
		artifact:   deployment.Artifact{DeploymentID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"},
	}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []deployment.WorkspaceID{"sales", "operations"},
		Environment:  "prod",
		DataDir:      "/data",
		Factory:      factory,
	})
	if err := registry.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if err := registry.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	for _, runtime := range factory.runtimes {
		if !runtime.closed {
			t.Fatalf("runtime %#v was not closed", runtime)
		}
	}
}

type fakeRepo struct {
	deployment        deployment.Deployment
	artifact          deployment.Artifact
	activeErr         error
	activeEnvironment deployment.Environment
}

func (r *fakeRepo) ActiveArtifact(_ context.Context, _ deployment.WorkspaceID, environment deployment.Environment) (deployment.Deployment, deployment.Artifact, error) {
	r.activeEnvironment = environment
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
	snapshotID   int64
}

func (f *fakeFactory) Prepare(_ context.Context, input RuntimeInput) (Runtime, error) {
	f.prepareCalls++
	if f.err != nil {
		return nil, f.err
	}
	if input.Deployment.DuckLakeSnapshotID > 0 {
		return &fakeRuntime{snapshotID: input.Deployment.DuckLakeSnapshotID}, nil
	}
	return &fakeRuntime{snapshotID: f.snapshotID}, nil
}

type fakeRuntime struct {
	closed     bool
	snapshotID int64
}

func (r *fakeRuntime) Close() error {
	r.closed = true
	return nil
}

func (r *fakeRuntime) DuckLakeSnapshotID() int64 {
	return r.snapshotID
}

type fakePrepared struct{}

func (fakePrepared) Close() error { return errors.New("unused") }

type registryDeploymentArtifact struct {
	deployment deployment.Deployment
	artifact   deployment.Artifact
}

type fakeRegistryRepo struct {
	active      map[string]registryDeploymentArtifact
	deployments map[deployment.ID]deployment.Deployment
	artifacts   map[deployment.ID]deployment.Artifact
	activeCalls []string
}

func newFakeRegistryRepo() *fakeRegistryRepo {
	return &fakeRegistryRepo{
		active:      map[string]registryDeploymentArtifact{},
		deployments: map[deployment.ID]deployment.Deployment{},
		artifacts:   map[deployment.ID]deployment.Artifact{},
	}
}

func (r *fakeRegistryRepo) ActiveArtifact(_ context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment) (deployment.Deployment, deployment.Artifact, error) {
	key := string(workspaceID) + "/" + string(environment)
	r.activeCalls = append(r.activeCalls, key)
	current, ok := r.active[key]
	if !ok {
		return deployment.Deployment{}, deployment.Artifact{}, deployment.ErrNotFound
	}
	return current.deployment, current.artifact, nil
}

func (r *fakeRegistryRepo) ByID(_ context.Context, id deployment.ID) (deployment.Deployment, error) {
	current, ok := r.deployments[id]
	if !ok {
		return deployment.Deployment{}, deployment.ErrNotFound
	}
	return current, nil
}

func (r *fakeRegistryRepo) ArtifactByDeployment(_ context.Context, id deployment.ID) (deployment.Artifact, error) {
	artifact, ok := r.artifacts[id]
	if !ok {
		return deployment.Artifact{}, deployment.ErrNotFound
	}
	return artifact, nil
}

type recordingRegistryFactory struct {
	inputs   []string
	runtimes []*recordingRuntime
}

func (f *recordingRegistryFactory) Prepare(_ context.Context, input RuntimeInput) (Runtime, error) {
	f.inputs = append(f.inputs, fmt.Sprintf("%s/%s/%s", input.Deployment.WorkspaceID, input.Deployment.Environment, input.Deployment.ID))
	runtime := &recordingRuntime{}
	f.runtimes = append(f.runtimes, runtime)
	return runtime, nil
}

type recordingRuntime struct {
	closed bool
}

func (r *recordingRuntime) Close() error {
	r.closed = true
	return nil
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
