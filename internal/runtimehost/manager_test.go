package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/deployment"
)

func TestManagerReloadIgnoresMissingActiveDeployment(t *testing.T) {
	manager := NewManagerWithFactory(ManagerOptions{Repo: &fakeRepo{activeErr: deployment.ErrNotFound}, WorkspaceID: "test", Environment: "dev", DataDir: "/data", Factory: &fakeFactory{}})

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
}

func TestManagerReloadClearsStaleRuntimeWhenActiveDeploymentMissing(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: deployment.StatusActive},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", DataDir: "/data", Factory: &fakeFactory{}})
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload active: %v", err)
	}
	repo.activeErr = deployment.ErrNotFound
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload missing active: %v", err)
	}
	if _, err := manager.Active(); err == nil {
		t.Fatal("active runtime survived missing active deployment")
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

func TestManagerKeepsOldRuntimeOpenUntilLeaseRelease(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: deployment.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest-1"},
	}
	var drained []int64
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		DataDir:     "/data",
		Factory:     &fakeFactory{},
		OnDrained: func(_ deployment.ID, snapshotID int64) {
			drained = append(drained, snapshotID)
		},
	})
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload first: %v", err)
	}
	oldLease, err := manager.Acquire()
	if err != nil {
		t.Fatalf("acquire old: %v", err)
	}
	oldRuntime := oldLease.Runtime().(*fakeRuntime)

	repo.deployment = deployment.Deployment{ID: "dep_2", WorkspaceID: "test", Environment: "dev", Status: deployment.StatusActive, DuckLakeSnapshotID: 22}
	repo.artifact = deployment.Artifact{DeploymentID: "dep_2", WorkspaceID: "test", Environment: "dev", Digest: "digest-2"}
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload second: %v", err)
	}
	if oldRuntime.closed {
		t.Fatal("old runtime closed while lease was still active")
	}
	newLease, err := manager.Acquire()
	if err != nil {
		t.Fatalf("acquire new: %v", err)
	}
	if got := newLease.DuckLakeSnapshotID(); got != 22 {
		t.Fatalf("new lease snapshot = %d, want 22", got)
	}
	newLease.Release()
	if got := oldLease.DuckLakeSnapshotID(); got != 11 {
		t.Fatalf("old lease snapshot = %d, want 11", got)
	}
	if got := manager.LeasedSnapshots(); !equalInt64s(got, []int64{11}) {
		t.Fatalf("leased snapshots = %#v, want old snapshot only", got)
	}

	oldLease.Release()
	if !oldRuntime.closed {
		t.Fatal("old runtime was not closed after final lease release")
	}
	if !equalInt64s(drained, []int64{11}) {
		t.Fatalf("drained snapshots = %#v, want [11]", drained)
	}
}

func TestManagerPersistsSnapshotLeaseOnAcquireAndRelease(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: deployment.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		DataDir:     "/data",
		Factory:     &fakeFactory{},
		LeaseTTL:    time.Minute,
		LeaseOwner:  "test-owner",
	})
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	lease, err := manager.Acquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if len(repo.createdLeases) != 1 {
		t.Fatalf("created leases = %#v, want one", repo.createdLeases)
	}
	created := repo.createdLeases[0]
	if created.WorkspaceID != "test" || created.Environment != "dev" || created.DeploymentID != "dep_1" || created.DuckLakeSnapshotID != 11 || created.OwnerID != "test-owner" {
		t.Fatalf("created lease = %#v", created)
	}
	lease.Release()
	if got := repo.releasedLeases; len(got) != 1 || got[0] != "lease_1" {
		t.Fatalf("released leases = %#v, want [lease_1]", got)
	}
	lease.Release()
	if got := repo.releasedLeases; len(got) != 1 {
		t.Fatalf("released leases after second release = %#v, want one release", got)
	}
}

func TestManagerRetriesPersistentLeaseRelease(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment:        deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: deployment.StatusActive, DuckLakeSnapshotID: 42},
		artifact:          deployment.Artifact{DeploymentID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
		releaseFailures:   2,
		releaseFailureErr: errors.New("database is locked"),
	}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		DataDir:     "/data",
		Factory:     &fakeFactory{},
	})
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	lease, err := manager.Acquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	lease.Release()

	if got := len(repo.releasedLeases); got != 3 {
		t.Fatalf("release attempts = %d, want retry until success", got)
	}
}

func TestManagerCloseDefersRuntimeCloseUntilLeaseRelease(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: deployment.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", DataDir: "/data", Factory: &fakeFactory{}})
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	lease, err := manager.Acquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	runtime := lease.Runtime().(*fakeRuntime)
	if err := manager.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if runtime.closed {
		t.Fatal("runtime closed while close waited on active lease")
	}
	if _, err := manager.Acquire(); err == nil {
		t.Fatal("acquire after close error = nil")
	}
	lease.Release()
	if !runtime.closed {
		t.Fatal("runtime was not closed after leased close release")
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

func TestManagerReloadBackfillsMissingDeploymentSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: deployment.StatusActive},
		artifact:   deployment.Artifact{DeploymentID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		DataDir:     "/data",
		Factory:     &fakeFactory{snapshotID: 42},
	})

	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if repo.recordedDeploymentID != "dep_1" || repo.recordedSnapshotID != 42 {
		t.Fatalf("recorded snapshot = (%s, %d), want (dep_1, 42)", repo.recordedDeploymentID, repo.recordedSnapshotID)
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

func TestManagerReloadRoutesWhenOnlyDuckLakeSnapshotPointerChanges(t *testing.T) {
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
	repo.deployment.DuckLakeSnapshotID = 22
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	active, err := manager.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if got := active.(RuntimeSnapshot).DuckLakeSnapshotID(); got != 22 {
		t.Fatalf("active snapshot = %d, want 22", got)
	}
	if factory.prepareCalls != 2 {
		t.Fatalf("prepare calls = %d, want snapshot pointer change to reload runtime", factory.prepareCalls)
	}
}

func TestManagerReloadReusesRuntimeWhenDeploymentDigestAndSnapshotMatch(t *testing.T) {
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
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	if factory.prepareCalls != 1 {
		t.Fatalf("prepare calls = %d, want matching pointer to reuse runtime", factory.prepareCalls)
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
	if _, err := registry.ActiveForWorkspace(context.Background(), "sales"); err != nil {
		t.Fatalf("sales active: %v", err)
	}
	if _, err := registry.ActiveForWorkspace(context.Background(), "operations"); err != nil {
		t.Fatalf("operations active: %v", err)
	}
	if _, err := registry.ActiveForWorkspace(context.Background(), "empty"); err == nil {
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
	repo.active["operations/prod"] = registryDeploymentArtifact{
		deployment: deployment.Deployment{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: deployment.StatusActive},
		artifact:   deployment.Artifact{DeploymentID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"},
	}
	if _, err := registry.ActiveForWorkspace(context.Background(), "operations"); err != nil {
		t.Fatalf("operations active after commit: %v", err)
	}
	if _, err := registry.ActiveForWorkspace(context.Background(), "sales"); err == nil {
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
	deployment           deployment.Deployment
	artifact             deployment.Artifact
	activeErr            error
	activeEnvironment    deployment.Environment
	recordedDeploymentID deployment.ID
	recordedSnapshotID   int64
	createdLeases        []deployment.SnapshotLeaseInput
	releasedLeases       []string
	extendedLeases       []string
	releaseFailures      int
	releaseFailureErr    error
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

func (r *fakeRepo) RecordDuckLakeSnapshot(_ context.Context, deploymentID deployment.ID, snapshotID int64) error {
	r.recordedDeploymentID = deploymentID
	r.recordedSnapshotID = snapshotID
	r.deployment.DuckLakeSnapshotID = snapshotID
	return nil
}

func (r *fakeRepo) CreateQuerySnapshotLease(_ context.Context, input deployment.SnapshotLeaseInput) (string, error) {
	r.createdLeases = append(r.createdLeases, input)
	return fmt.Sprintf("lease_%d", len(r.createdLeases)), nil
}

func (r *fakeRepo) ReleaseQuerySnapshotLease(_ context.Context, id string) error {
	r.releasedLeases = append(r.releasedLeases, id)
	if r.releaseFailures > 0 {
		r.releaseFailures--
		if r.releaseFailureErr != nil {
			return r.releaseFailureErr
		}
		return errors.New("release failed")
	}
	return nil
}

func (r *fakeRepo) ExtendQuerySnapshotLease(_ context.Context, id string, _ time.Time) error {
	r.extendedLeases = append(r.extendedLeases, id)
	return nil
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

func (r *fakeRegistryRepo) RecordDuckLakeSnapshot(_ context.Context, deploymentID deployment.ID, snapshotID int64) error {
	current := r.deployments[deploymentID]
	current.DuckLakeSnapshotID = snapshotID
	r.deployments[deploymentID] = current
	for key, pair := range r.active {
		if pair.deployment.ID == deploymentID {
			pair.deployment.DuckLakeSnapshotID = snapshotID
			r.active[key] = pair
		}
	}
	return nil
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

func equalInt64s(got, want []int64) bool {
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
