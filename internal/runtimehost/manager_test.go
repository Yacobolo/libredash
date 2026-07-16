package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

func TestManagerReloadIgnoresMissingActiveDeployment(t *testing.T) {
	manager := NewManagerWithFactory(ManagerOptions{Repo: &fakeRepo{activeErr: servingstate.ErrNotFound}, WorkspaceID: "test", Environment: "dev", Factory: &fakeFactory{}})

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
}

func TestManagerReloadClearsStaleRuntimeWhenActiveDeploymentMissing(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: &fakeFactory{}})
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload active: %v", err)
	}
	repo.activeErr = servingstate.ErrNotFound
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload missing active: %v", err)
	}
	if _, err := manager.Active(); err == nil {
		t.Fatal("active runtime survived missing active deployment")
	}
}

func TestManagerReloadUsesConfiguredEnvironment(t *testing.T) {
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_prod", WorkspaceID: "test", Environment: "prod", Status: servingstate.StatusValidated},
		artifact:   servingstate.Artifact{ServingStateID: "dep_prod", Environment: "prod", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "prod", Factory: &fakeFactory{}})

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
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Status: servingstate.StatusValidated},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", Digest: "digest"},
	}
	factory := &fakeFactory{}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: factory})

	prepared, err := manager.PrepareServingState(ctx, "dep_1")
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

	second, err := manager.PrepareServingState(ctx, "dep_1")
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

func TestManagerPassesManagedDataResolutionToRuntimeFactory(t *testing.T) {
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "prod", Status: servingstate.StatusValidated},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "prod", Digest: "digest"},
	}
	resolver := fakeManagedDataResolver{resolution: ManagedDataResolution{
		RevisionID: "sha256:" + strings.Repeat("a", 64),
		Roots:      map[string]string{"olist": "/managed/olist/revision"},
	}}
	factory := &fakeFactory{}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "prod", Factory: factory, ManagedData: resolver})

	prepared, err := manager.PrepareServingState(context.Background(), "dep_1")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if factory.input.ManagedData.RevisionID != resolver.resolution.RevisionID {
		t.Fatalf("revision = %q", factory.input.ManagedData.RevisionID)
	}
	if got := factory.input.ManagedData.Roots["olist"]; got != "/managed/olist/revision" {
		t.Fatalf("olist root = %q", got)
	}
	if factory.input.ManagedData.Lifetime != nil {
		t.Fatal("runtime factory received the manager-owned managed-data lifetime")
	}
}

func TestManagerReloadResolvesManagedDataWhenArtifactChanges(t *testing.T) {
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "prod", Status: servingstate.StatusActive},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "prod", Digest: "digest"},
	}
	resolver := &fakeManagedDataResolver{resolution: ManagedDataResolution{RevisionID: "sha256:" + strings.Repeat("a", 64)}}
	factory := &fakeFactory{}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "prod", Factory: factory, ManagedData: resolver})

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	resolver.resolution = ManagedDataResolution{RevisionID: "sha256:" + strings.Repeat("b", 64)}
	repo.artifact.Digest = "digest-2"
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if factory.prepareCalls != 2 {
		t.Fatalf("factory calls = %d, want reload for artifact change", factory.prepareCalls)
	}
	if got := factory.input.ManagedData.RevisionID; got != resolver.resolution.RevisionID {
		t.Fatalf("prepared managed-data revision = %q, want %q", got, resolver.resolution.RevisionID)
	}
}

func TestManagerReloadDoesNotResolveManagedDataForUnchangedArtifact(t *testing.T) {
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "prod", Status: servingstate.StatusActive},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "prod", Digest: "digest"},
	}
	resolver := &countingManagedDataResolver{resolution: ManagedDataResolution{
		RevisionID: "sha256:" + strings.Repeat("a", 64),
	}}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "prod",
		Factory:     &fakeFactory{},
		ManagedData: resolver,
	})

	if err := manager.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 1 {
		t.Fatalf("managed-data resolutions = %d, want one resolution for unchanged artifact", resolver.calls)
	}
}

func TestManagerKeepsOldRuntimeOpenUntilLeaseRelease(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest-1"},
	}
	var drained []int64
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		Factory:     &fakeFactory{},
		OnDrained: func(_ servingstate.ID, snapshotID int64) {
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

	repo.deployment = servingstate.State{ID: "dep_2", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 22}
	repo.artifact = servingstate.Artifact{ServingStateID: "dep_2", WorkspaceID: "test", Environment: "dev", Digest: "digest-2"}
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
	if got := manager.LeasedSnapshots(); !equalInt64s(got, []int64{11, 22}) {
		t.Fatalf("leased snapshots = %#v, want active and draining generations", got)
	}

	oldLease.Release()
	if !oldRuntime.closed {
		t.Fatal("old runtime was not closed after final lease release")
	}
	if !equalInt64s(drained, []int64{11}) {
		t.Fatalf("drained snapshots = %#v, want [11]", drained)
	}
}

func TestManagerKeepsManagedDataLifetimeUntilRuntimeDrains(t *testing.T) {
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest-1"},
	}
	lifetime := &fakeManagedDataLifetime{}
	resolver := &fakeManagedDataResolver{resolution: ManagedDataResolution{
		RevisionID: "sha256:" + strings.Repeat("a", 64),
		Roots:      map[string]string{"warehouse": "/runtime/revision"},
		Lifetime:   lifetime,
	}}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: &fakeFactory{}, ManagedData: resolver,
	})
	if err := manager.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	queryLease, err := manager.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	runtime := queryLease.Runtime().(*fakeRuntime)
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if lifetime.releases != 0 {
		t.Fatal("managed-data lifetime released while a query still held the runtime")
	}
	queryLease.Release()
	if !runtime.closed {
		t.Fatal("runtime was not closed after its final query lease")
	}
	if lifetime.releases != 1 {
		t.Fatalf("managed-data lifetime releases = %d, want 1", lifetime.releases)
	}
}

func TestPreparedReleasesManagedDataLifetimeOnFailureAndAbandonment(t *testing.T) {
	state := servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusValidated}
	artifact := servingstate.Artifact{ServingStateID: state.ID, WorkspaceID: state.WorkspaceID, Environment: state.Environment, Digest: "digest"}
	repo := &fakeRepo{deployment: state, artifact: artifact}

	failureLifetime := &fakeManagedDataLifetime{}
	failing := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: &fakeFactory{err: errors.New("prepare failed")}})
	_, err := failing.prepareResolved(t.Context(), state, artifact, ManagedDataResolution{Lifetime: failureLifetime})
	if err == nil {
		t.Fatal("prepare unexpectedly succeeded")
	}
	if failureLifetime.releases != 1 {
		t.Fatalf("failed preparation releases = %d, want 1", failureLifetime.releases)
	}

	abandonedLifetime := &fakeManagedDataLifetime{}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: &fakeFactory{}})
	prepared, err := manager.prepareResolved(t.Context(), state, artifact, ManagedDataResolution{Lifetime: abandonedLifetime})
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if abandonedLifetime.releases != 1 {
		t.Fatalf("abandoned preparation releases = %d, want 1", abandonedLifetime.releases)
	}
}

func TestManagerPreparationFailsClosedWhenGenerationLeaseCannotPersist(t *testing.T) {
	state := servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusValidated}
	artifact := servingstate.Artifact{ServingStateID: state.ID, WorkspaceID: state.WorkspaceID, Environment: state.Environment, Digest: "digest"}
	repo := &fakeRepo{deployment: state, artifact: artifact, createLeaseErr: errors.New("lease unavailable")}
	lifetime := &fakeManagedDataLifetime{}
	factory := &fakeFactory{snapshotID: 42}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: factory,
		ManagedData: fakeManagedDataResolver{resolution: ManagedDataResolution{Lifetime: lifetime}},
	})

	if _, err := manager.PrepareServingState(t.Context(), "dep_1"); err == nil {
		t.Fatal("prepare error = nil, want durable generation lease failure")
	}
	if factory.runtime == nil || !factory.runtime.closed {
		t.Fatalf("prepared runtime = %#v, want closed after lease failure", factory.runtime)
	}
	if lifetime.releases != 1 {
		t.Fatalf("managed-data lifetime releases = %d, want 1", lifetime.releases)
	}
}

func TestManagerPersistsOneSnapshotLeaseForRuntimeGeneration(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		Factory:     &fakeFactory{},
		LeaseTTL:    time.Minute,
		LeaseOwner:  "test-owner",
	})
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(repo.createdLeases) != 1 {
		t.Fatalf("created leases = %#v, want one", repo.createdLeases)
	}
	created := repo.createdLeases[0]
	if created.WorkspaceID != "test" || created.Environment != "dev" || created.ServingStateID != "dep_1" || created.DuckLakeSnapshotID != 11 || created.OwnerID != "test-owner" {
		t.Fatalf("created lease = %#v", created)
	}
	for range 3 {
		lease, err := manager.Acquire()
		if err != nil {
			t.Fatalf("acquire: %v", err)
		}
		lease.Release()
	}
	if len(repo.createdLeases) != 1 || len(repo.releasedLeases) != 0 {
		t.Fatalf("request acquisitions changed durable leases: created=%d released=%d", len(repo.createdLeases), len(repo.releasedLeases))
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := repo.releasedLeases; len(got) != 1 || got[0] != "lease_1" {
		t.Fatalf("released leases = %#v, want [lease_1] after generation close", got)
	}
}

func TestRenewSnapshotLeaseRetriesTransientFailure(t *testing.T) {
	repo := &fakeRepo{extendFailures: 2, extendFailureErr: errors.New("database busy")}
	err := renewSnapshotLease(t.Context(), repo, "lease_1", time.Now().Add(time.Minute), 3, time.Millisecond)
	if err != nil {
		t.Fatalf("renew snapshot lease: %v", err)
	}
	if got := len(repo.extendedLeases); got != 3 {
		t.Fatalf("extension attempts = %d, want 3", got)
	}
}

func TestRenewSnapshotLeaseReportsPersistentFailure(t *testing.T) {
	repo := &fakeRepo{extendFailures: 3, extendFailureErr: errors.New("database unavailable")}
	err := renewSnapshotLease(t.Context(), repo, "lease_1", time.Now().Add(time.Minute), 3, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("renew snapshot lease error = %v, want database failure", err)
	}
	if got := len(repo.extendedLeases); got != 3 {
		t.Fatalf("extension attempts = %d, want 3", got)
	}
}

func TestManagerTracksLeaseRenewalFailuresPerGeneration(t *testing.T) {
	manager := NewManagerWithFactory(ManagerOptions{})
	manager.setLeaseRenewalError("lease_1", errors.New("first failed"))
	manager.setLeaseRenewalError("lease_2", errors.New("second failed"))
	if err := manager.LeaseRenewalError(); err == nil || !strings.Contains(err.Error(), "first failed") || !strings.Contains(err.Error(), "second failed") {
		t.Fatalf("lease renewal error = %v, want both generation failures", err)
	}
	manager.setLeaseRenewalError("lease_1", nil)
	if err := manager.LeaseRenewalError(); err == nil || strings.Contains(err.Error(), "first failed") || !strings.Contains(err.Error(), "second failed") {
		t.Fatalf("lease renewal error after recovery = %v, want only second failure", err)
	}
}

func TestManagerRetiredGenerationKeepsSnapshotLeaseUntilReadersDrain(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest-1"},
	}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: &fakeFactory{}})
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("first reload: %v", err)
	}
	reader, err := manager.Acquire()
	if err != nil {
		t.Fatalf("acquire old generation: %v", err)
	}

	repo.deployment = servingstate.State{ID: "dep_2", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 22}
	repo.artifact = servingstate.Artifact{ServingStateID: "dep_2", WorkspaceID: "test", Environment: "dev", Digest: "digest-2"}
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	if len(repo.createdLeases) != 2 || len(repo.releasedLeases) != 0 {
		t.Fatalf("leases after cutover: created=%d released=%d, want 2 and 0", len(repo.createdLeases), len(repo.releasedLeases))
	}

	reader.Release()
	if got := repo.releasedLeases; len(got) != 1 || got[0] != "lease_1" {
		t.Fatalf("released leases after old reader drained = %#v, want [lease_1]", got)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("close current generation: %v", err)
	}
	if got := repo.releasedLeases; len(got) != 2 || got[1] != "lease_2" {
		t.Fatalf("released leases after close = %#v, want [lease_1 lease_2]", got)
	}
}

func TestManagerRetriesPersistentLeaseRelease(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment:        servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 42},
		artifact:          servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
		releaseFailures:   2,
		releaseFailureErr: errors.New("database is locked"),
	}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		Factory:     &fakeFactory{},
	})
	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if got := len(repo.releasedLeases); got != 3 {
		t.Fatalf("release attempts = %d, want retry until success", got)
	}
}

func TestManagerCloseDefersRuntimeCloseUntilLeaseRelease(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: &fakeFactory{}})
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
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Status: servingstate.StatusValidated},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		Factory:     &fakeFactory{snapshotID: 42},
	})

	prepared, err := manager.PrepareServingState(ctx, "dep_1")
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
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:        repo,
		WorkspaceID: "test",
		Environment: "dev",
		Factory:     &fakeFactory{snapshotID: 42},
	})

	if err := manager.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if repo.recordedServingStateID != "dep_1" || repo.recordedSnapshotID != 42 {
		t.Fatalf("recorded snapshot = (%s, %d), want (dep_1, 42)", repo.recordedServingStateID, repo.recordedSnapshotID)
	}
}

func TestManagerReloadRoutesWhenOnlyActiveDeploymentPointerChanges(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "same-digest"},
	}
	factory := &fakeFactory{}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: factory})

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

	repo.deployment = servingstate.State{ID: "dep_2", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 22}
	repo.artifact = servingstate.Artifact{ServingStateID: "dep_2", WorkspaceID: "test", Environment: "dev", Digest: "same-digest"}
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
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "same-digest"},
	}
	factory := &fakeFactory{}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: factory})

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
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Environment: "dev", Status: servingstate.StatusActive, DuckLakeSnapshotID: 11},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", WorkspaceID: "test", Environment: "dev", Digest: "same-digest"},
	}
	factory := &fakeFactory{}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: factory})

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
	manager := NewManagerWithFactory(ManagerOptions{Repo: &fakeRepo{}, WorkspaceID: "test", Environment: "dev", Factory: &fakeFactory{}})
	if err := manager.CommitPrepared(fakePrepared{}); err == nil {
		t.Fatal("expected wrong prepared runtime error")
	}
}

func TestManagerCloseClearsActiveRuntime(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Status: servingstate.StatusValidated},
		artifact:   servingstate.Artifact{ServingStateID: "dep_1", Digest: "digest"},
	}
	manager := NewManagerWithFactory(ManagerOptions{Repo: repo, WorkspaceID: "test", Environment: "dev", Factory: &fakeFactory{}})
	prepared, err := manager.PrepareServingState(ctx, "dep_1")
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
		deployment: servingstate.State{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusValidated, DuckLakeSnapshotID: 9},
		artifact:   servingstate.Artifact{ServingStateID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"},
	}
	repo.active["operations/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: servingstate.StatusValidated, DuckLakeSnapshotID: 7},
		artifact:   servingstate.Artifact{ServingStateID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"},
	}
	repo.active["sales/dev"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_sales_dev", WorkspaceID: "sales", Environment: "dev", Status: servingstate.StatusValidated, DuckLakeSnapshotID: 3},
		artifact:   servingstate.Artifact{ServingStateID: "dep_sales_dev", WorkspaceID: "sales", Environment: "dev", Digest: "sales-dev"},
	}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []servingstate.WorkspaceID{"sales", "operations", "empty"},
		Environment:  "prod",
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
	repo.deployments["dep_ops_prod"] = servingstate.State{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: servingstate.StatusValidated}
	repo.artifacts["dep_ops_prod"] = servingstate.Artifact{ServingStateID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []servingstate.WorkspaceID{"sales"},
		Environment:  "prod",
		Factory:      factory,
	})

	prepared, err := registry.PrepareServingState(context.Background(), "dep_ops_prod")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := registry.CommitPrepared(prepared); err != nil {
		t.Fatalf("commit: %v", err)
	}
	repo.active["operations/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: servingstate.StatusActive},
		artifact:   servingstate.Artifact{ServingStateID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"},
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

func TestRegistryPreparedSetActivatesAndCommitsEveryWorkspaceTogether(t *testing.T) {
	repo := newFakeRegistryRepo()
	for _, workspaceID := range []servingstate.WorkspaceID{"operations", "sales"} {
		activeID := servingstate.ID("dep_" + string(workspaceID) + "_active")
		repo.active[string(workspaceID)+"/prod"] = registryDeploymentArtifact{
			deployment: servingstate.State{ID: activeID, WorkspaceID: workspaceID, Environment: "prod", Status: servingstate.StatusActive, DuckLakeSnapshotID: 1},
			artifact:   servingstate.Artifact{ServingStateID: activeID, WorkspaceID: workspaceID, Environment: "prod", Digest: string(workspaceID) + "-active"},
		}
		id := servingstate.ID("dep_" + string(workspaceID) + "_prod")
		repo.deployments[id] = servingstate.State{ID: id, WorkspaceID: workspaceID, Environment: "prod", Status: servingstate.StatusValidated}
		repo.artifacts[id] = servingstate.Artifact{ServingStateID: id, WorkspaceID: workspaceID, Environment: "prod", Digest: string(workspaceID) + "-prod"}
	}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []servingstate.WorkspaceID{"operations", "sales"},
		Environment:  "prod",
		Factory:      factory,
	})
	if err := registry.Reload(context.Background()); err != nil {
		t.Fatalf("reload active generation: %v", err)
	}

	prepared, err := registry.PrepareServingStates(context.Background(), []string{"dep_sales_prod", "dep_operations_prod"})
	if err != nil {
		t.Fatalf("prepare set: %v", err)
	}
	defer prepared.Close()
	if len(factory.runtimes) != 4 || factory.runtimes[0].closed || factory.runtimes[1].closed {
		t.Fatalf("active generation was not retained during prepare: %#v", factory.runtimes)
	}
	if runtime, err := registry.managerForWorkspace("operations").Active(); err != nil || runtime != factory.runtimes[0] {
		t.Fatalf("operations active during prepare = %#v, %v", runtime, err)
	}
	activated := false
	err = registry.CommitPreparedSet(prepared, func() error {
		activated = true
		for _, workspaceID := range []servingstate.WorkspaceID{"operations", "sales"} {
			id := servingstate.ID("dep_" + string(workspaceID) + "_prod")
			repo.active[string(workspaceID)+"/prod"] = registryDeploymentArtifact{deployment: repo.deployments[id], artifact: repo.artifacts[id]}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("commit set: %v", err)
	}
	if !activated {
		t.Fatal("metadata activation was not called")
	}
	for _, workspaceID := range []servingstate.WorkspaceID{"operations", "sales"} {
		if _, err := registry.ActiveForWorkspace(context.Background(), workspaceID); err != nil {
			t.Fatalf("%s active: %v", workspaceID, err)
		}
	}
	if got := factory.inputs; !equalStrings(got, []string{"operations/prod/dep_operations_active", "sales/prod/dep_sales_active", "operations/prod/dep_operations_prod", "sales/prod/dep_sales_prod"}) {
		t.Fatalf("factory inputs = %#v", got)
	}
}

func TestRegistryPreparedSetDoesNotCommitWhenMetadataActivationFails(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.deployments["dep_sales_prod"] = servingstate.State{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusValidated}
	repo.artifacts["dep_sales_prod"] = servingstate.Artifact{ServingStateID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{Repo: repo, WorkspaceIDs: []servingstate.WorkspaceID{"sales"}, Environment: "prod", Factory: factory})

	prepared, err := registry.PrepareServingStates(context.Background(), []string{"dep_sales_prod"})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	wantErr := errors.New("activation failed")
	if err := registry.CommitPreparedSet(prepared, func() error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("commit error = %v, want %v", err, wantErr)
	}
	if _, err := registry.managerForWorkspace("sales").Active(); err == nil {
		t.Fatal("runtime committed after metadata activation failed")
	}
}

func TestRegistryPreparesExplicitManagedDataCandidateBeforeActivation(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.deployments["dep_sales_prod"] = servingstate.State{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusValidated}
	repo.artifacts["dep_sales_prod"] = servingstate.Artifact{ServingStateID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"}
	resolver := &fakeManagedDataResolver{resolution: ManagedDataResolution{RevisionID: "sha256:" + strings.Repeat("a", 64)}}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{Repo: repo, WorkspaceIDs: []servingstate.WorkspaceID{"sales"}, Environment: "prod", Factory: factory, ManagedData: resolver})
	candidate := ManagedDataResolution{RevisionID: "sha256:" + strings.Repeat("b", 64), Roots: map[string]string{"warehouse": "/cache/revision-b"}}

	prepared, err := registry.PrepareServingStateCandidates(context.Background(), []ServingStateCandidate{{ServingStateID: "dep_sales_prod", ManagedData: candidate}})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if len(factory.managedData) != 1 || factory.managedData[0].RevisionID != candidate.RevisionID {
		t.Fatalf("prepared managed data = %#v, want candidate revision", factory.managedData)
	}
	if got := factory.managedData[0].Roots["warehouse"]; got != "/cache/revision-b" {
		t.Fatalf("prepared warehouse root = %q", got)
	}
}

func TestRegistryPreparationFailureReleasesUnprocessedManagedDataLifetimes(t *testing.T) {
	repo := newFakeRegistryRepo()
	for _, workspaceID := range []servingstate.WorkspaceID{"operations", "sales"} {
		stateID := servingstate.ID("dep_" + string(workspaceID) + "_prod")
		repo.deployments[stateID] = servingstate.State{ID: stateID, WorkspaceID: workspaceID, Environment: "prod", Status: servingstate.StatusValidated}
		repo.artifacts[stateID] = servingstate.Artifact{ServingStateID: stateID, WorkspaceID: workspaceID, Environment: "prod", Digest: string(workspaceID)}
	}
	first := &fakeManagedDataLifetime{}
	second := &fakeManagedDataLifetime{}
	registry := NewRegistryWithFactory(RegistryOptions{Repo: repo, Environment: "prod", Factory: &fakeFactory{err: errors.New("prepare failed")}})
	_, err := registry.PrepareServingStateCandidates(t.Context(), []ServingStateCandidate{
		{ServingStateID: "dep_operations_prod", ManagedData: ManagedDataResolution{RevisionID: "sha256:" + strings.Repeat("a", 64), Lifetime: first}},
		{ServingStateID: "dep_sales_prod", ManagedData: ManagedDataResolution{RevisionID: "sha256:" + strings.Repeat("b", 64), Lifetime: second}},
	})
	if err == nil {
		t.Fatal("preparation unexpectedly succeeded")
	}
	if first.releases != 1 || second.releases != 1 {
		t.Fatalf("managed-data releases = (%d, %d), want (1, 1)", first.releases, second.releases)
	}
}

func TestPreparedSetReportsCandidateSnapshots(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.deployments["dep_sales_prod"] = servingstate.State{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusValidated}
	repo.artifacts["dep_sales_prod"] = servingstate.Artifact{ServingStateID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"}
	registry := NewRegistryWithFactory(RegistryOptions{Repo: repo, Environment: "prod", Factory: &fakeFactory{snapshotID: 42}})

	prepared, err := registry.PrepareServingStates(context.Background(), []string{"dep_sales_prod"})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	want := []PreparedSnapshot{{WorkspaceID: "sales", ServingStateID: "dep_sales_prod", DuckLakeSnapshotID: 42}}
	if got := prepared.Snapshots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshots = %#v, want %#v", got, want)
	}
}

func TestRegistryServesActiveGenerationWhilePreparedSetIsBuilding(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.active["sales/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_sales_active", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusActive, DuckLakeSnapshotID: 1},
		artifact:   servingstate.Artifact{ServingStateID: "dep_sales_active", WorkspaceID: "sales", Environment: "prod", Digest: "active"},
	}
	repo.deployments["dep_sales_next"] = servingstate.State{ID: "dep_sales_next", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusValidated}
	repo.artifacts["dep_sales_next"] = servingstate.Artifact{ServingStateID: "dep_sales_next", WorkspaceID: "sales", Environment: "prod", Digest: "next"}
	factory := &blockingRegistryFactory{started: make(chan struct{}), release: make(chan struct{})}
	registry := NewRegistryWithFactory(RegistryOptions{Repo: repo, WorkspaceIDs: []servingstate.WorkspaceID{"sales"}, Environment: "prod", Factory: factory})
	if err := registry.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}

	preparedResult := make(chan *PreparedSet, 1)
	errResult := make(chan error, 1)
	go func() {
		prepared, err := registry.PrepareServingStates(context.Background(), []string{"dep_sales_next"})
		preparedResult <- prepared
		errResult <- err
	}()
	<-factory.started

	leaseResult := make(chan Lease, 1)
	leaseErr := make(chan error, 1)
	go func() {
		lease, err := registry.AcquireForWorkspace(context.Background(), "sales")
		leaseResult <- lease
		leaseErr <- err
	}()
	select {
	case err := <-leaseErr:
		if err != nil {
			t.Fatalf("acquire active generation: %v", err)
		}
		lease := <-leaseResult
		if lease.ServingStateID() != "dep_sales_active" {
			t.Fatalf("serving state = %q", lease.ServingStateID())
		}
		lease.Release()
	case <-time.After(500 * time.Millisecond):
		t.Fatal("active acquisition blocked behind candidate materialization")
	}

	close(factory.release)
	prepared := <-preparedResult
	if err := <-errResult; err != nil {
		t.Fatal(err)
	}
	if prepared != nil {
		defer prepared.Close()
	}
}

func TestRegistryRejectsPreparedDeploymentFromDifferentEnvironment(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.deployments["dep_ops_dev"] = servingstate.State{ID: "dep_ops_dev", WorkspaceID: "operations", Environment: "dev", Status: servingstate.StatusValidated}
	repo.artifacts["dep_ops_dev"] = servingstate.Artifact{ServingStateID: "dep_ops_dev", WorkspaceID: "operations", Environment: "dev", Digest: "ops-dev"}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []servingstate.WorkspaceID{"operations"},
		Environment:  "prod",
		Factory:      &recordingRegistryFactory{},
	})

	if _, err := registry.PrepareServingState(context.Background(), "dep_ops_dev"); err == nil {
		t.Fatal("prepare error = nil, want environment mismatch")
	}
}

func TestRegistrySerializesRuntimePrepareAcrossWorkspaces(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.deployments["dep_ops_prod"] = servingstate.State{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: servingstate.StatusValidated}
	repo.artifacts["dep_ops_prod"] = servingstate.Artifact{ServingStateID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"}
	repo.deployments["dep_sales_prod"] = servingstate.State{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusValidated}
	repo.artifacts["dep_sales_prod"] = servingstate.Artifact{ServingStateID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"}
	factory := &overlapDetectingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []servingstate.WorkspaceID{"operations", "sales"},
		Environment:  "prod",
		Factory:      factory,
	})

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, id := range []string{"dep_ops_prod", "dep_sales_prod"} {
		wg.Add(1)
		go func(servingStateID string) {
			defer wg.Done()
			<-start
			prepared, err := registry.PrepareServingState(context.Background(), servingStateID)
			if err != nil {
				errs <- err
				return
			}
			errs <- prepared.Close()
		}(id)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("prepare: %v", err)
		}
	}
	if factory.overlapped.Load() {
		t.Fatal("runtime prepares overlapped across workspaces")
	}
}

func TestRegistryPrepareServingStateClosesLoadedRuntimesBeforePrepare(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.active["operations/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: servingstate.StatusActive, DuckLakeSnapshotID: 7},
		artifact:   servingstate.Artifact{ServingStateID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"},
	}
	repo.active["sales/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusActive, DuckLakeSnapshotID: 9},
		artifact:   servingstate.Artifact{ServingStateID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"},
	}
	repo.deployments["dep_visuals_prod"] = servingstate.State{ID: "dep_visuals_prod", WorkspaceID: "visuals", Environment: "prod", Status: servingstate.StatusValidated}
	repo.artifacts["dep_visuals_prod"] = servingstate.Artifact{ServingStateID: "dep_visuals_prod", WorkspaceID: "visuals", Environment: "prod", Digest: "visuals-prod"}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []servingstate.WorkspaceID{"operations", "sales"},
		Environment:  "prod",
		Factory:      factory,
	})
	if err := registry.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(factory.runtimes) != 2 || factory.runtimes[0].closed || factory.runtimes[1].closed {
		t.Fatalf("loaded runtimes = %#v, want two open runtimes", factory.runtimes)
	}

	prepared, err := registry.PrepareServingState(context.Background(), "dep_visuals_prod")
	if err != nil {
		t.Fatalf("prepare visuals: %v", err)
	}
	defer prepared.Close()
	if !factory.runtimes[0].closed || !factory.runtimes[1].closed {
		t.Fatalf("previous active runtimes were not closed before prepare: %#v", factory.runtimes)
	}
	if len(factory.runtimes) != 3 || factory.runtimes[2].closed {
		t.Fatalf("prepared runtime = %#v, want new open runtime", factory.runtimes)
	}
}

func TestRegistryAcquireForWorkspaceDoesNotDiscoverRepositoryChanges(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.active["operations/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: servingstate.StatusActive, DuckLakeSnapshotID: 7},
		artifact:   servingstate.Artifact{ServingStateID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"},
	}
	repo.active["sales/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusActive, DuckLakeSnapshotID: 9},
		artifact:   servingstate.Artifact{ServingStateID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"},
	}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []servingstate.WorkspaceID{"operations", "sales", "visuals"},
		Environment:  "prod",
		Factory:      factory,
	})
	if err := registry.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(factory.runtimes) != 2 || factory.runtimes[0].closed || factory.runtimes[1].closed {
		t.Fatalf("loaded runtimes = %#v, want two open runtimes", factory.runtimes)
	}
	repo.active["visuals/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_visuals_prod", WorkspaceID: "visuals", Environment: "prod", Status: servingstate.StatusActive},
		artifact:   servingstate.Artifact{ServingStateID: "dep_visuals_prod", WorkspaceID: "visuals", Environment: "prod", Digest: "visuals-prod"},
	}

	activeCalls := len(repo.activeCalls)
	if _, err := registry.AcquireForWorkspace(context.Background(), "visuals"); err == nil {
		t.Fatal("acquire visuals error = nil, want explicit activation requirement")
	}
	if len(repo.activeCalls) != activeCalls {
		t.Fatalf("repository active calls = %d, want unchanged %d", len(repo.activeCalls), activeCalls)
	}
	if len(factory.runtimes) != 2 || factory.runtimes[0].closed || factory.runtimes[1].closed {
		t.Fatalf("acquisition mutated loaded runtimes: %#v", factory.runtimes)
	}
}

func TestRegistryAcquireForWorkspaceIsMemoryOnly(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.active["operations/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: servingstate.StatusActive, DuckLakeSnapshotID: 7},
		artifact:   servingstate.Artifact{ServingStateID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"},
	}
	repo.active["sales/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusActive, DuckLakeSnapshotID: 9},
		artifact:   servingstate.Artifact{ServingStateID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"},
	}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []servingstate.WorkspaceID{"operations", "sales"},
		Environment:  "prod",
		Factory:      factory,
	})
	if err := registry.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	activeCalls := len(repo.activeCalls)

	lease, err := registry.AcquireForWorkspace(context.Background(), "sales")
	if err != nil {
		t.Fatalf("acquire sales: %v", err)
	}
	lease.Release()
	if len(repo.activeCalls) != activeCalls {
		t.Fatalf("repository active calls = %d, want unchanged %d", len(repo.activeCalls), activeCalls)
	}
	if len(factory.runtimes) != 2 {
		t.Fatalf("runtime count = %d, want no new prepare", len(factory.runtimes))
	}
	if factory.runtimes[0].closed || factory.runtimes[1].closed {
		t.Fatalf("no-change reload closed runtimes: %#v", factory.runtimes)
	}
}

func TestRegistryCloseClosesEveryActiveWorkspaceRuntime(t *testing.T) {
	repo := newFakeRegistryRepo()
	repo.active["sales/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Status: servingstate.StatusValidated},
		artifact:   servingstate.Artifact{ServingStateID: "dep_sales_prod", WorkspaceID: "sales", Environment: "prod", Digest: "sales-prod"},
	}
	repo.active["operations/prod"] = registryDeploymentArtifact{
		deployment: servingstate.State{ID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Status: servingstate.StatusValidated},
		artifact:   servingstate.Artifact{ServingStateID: "dep_ops_prod", WorkspaceID: "operations", Environment: "prod", Digest: "ops-prod"},
	}
	factory := &recordingRegistryFactory{}
	registry := NewRegistryWithFactory(RegistryOptions{
		Repo:         repo,
		WorkspaceIDs: []servingstate.WorkspaceID{"sales", "operations"},
		Environment:  "prod",
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
	deployment             servingstate.State
	artifact               servingstate.Artifact
	activeErr              error
	activeEnvironment      servingstate.Environment
	recordedServingStateID servingstate.ID
	recordedSnapshotID     int64
	createdLeases          []servingstate.SnapshotLeaseInput
	releasedLeases         []string
	extendedLeases         []string
	extendFailures         int
	extendFailureErr       error
	releaseFailures        int
	releaseFailureErr      error
	createLeaseErr         error
}

func (r *fakeRepo) ActiveArtifact(_ context.Context, _ servingstate.WorkspaceID, environment servingstate.Environment) (servingstate.State, servingstate.Artifact, error) {
	r.activeEnvironment = environment
	if r.activeErr != nil {
		return servingstate.State{}, servingstate.Artifact{}, r.activeErr
	}
	return r.deployment, r.artifact, nil
}

func (r *fakeRepo) ByID(context.Context, servingstate.ID) (servingstate.State, error) {
	if r.deployment.ID == "" {
		return servingstate.State{}, servingstate.ErrNotFound
	}
	return r.deployment, nil
}

func (r *fakeRepo) ArtifactByServingState(context.Context, servingstate.ID) (servingstate.Artifact, error) {
	if r.artifact.Digest == "" {
		return servingstate.Artifact{}, servingstate.ErrNotFound
	}
	return r.artifact, nil
}

func (r *fakeRepo) RecordDuckLakeSnapshot(_ context.Context, servingStateID servingstate.ID, snapshotID int64) error {
	r.recordedServingStateID = servingStateID
	r.recordedSnapshotID = snapshotID
	r.deployment.DuckLakeSnapshotID = snapshotID
	return nil
}

func (r *fakeRepo) CreateQuerySnapshotLease(_ context.Context, input servingstate.SnapshotLeaseInput) (string, error) {
	if r.createLeaseErr != nil {
		return "", r.createLeaseErr
	}
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
	if r.extendFailures > 0 {
		r.extendFailures--
		if r.extendFailureErr != nil {
			return r.extendFailureErr
		}
		return errors.New("extension failed")
	}
	return nil
}

type fakeFactory struct {
	prepareCalls int
	err          error
	snapshotID   int64
	input        RuntimeInput
	runtime      *fakeRuntime
}

func (f *fakeFactory) Prepare(_ context.Context, input RuntimeInput) (Runtime, error) {
	f.prepareCalls++
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	snapshotID := f.snapshotID
	if input.State.DuckLakeSnapshotID > 0 {
		snapshotID = input.State.DuckLakeSnapshotID
	}
	f.runtime = &fakeRuntime{snapshotID: snapshotID}
	return f.runtime, nil
}

type fakeManagedDataResolver struct {
	resolution ManagedDataResolution
	err        error
}

type countingManagedDataResolver struct {
	resolution ManagedDataResolution
	err        error
	calls      int
}

func (r *countingManagedDataResolver) ResolveManagedData(context.Context, servingstate.ID) (ManagedDataResolution, error) {
	r.calls++
	return r.resolution, r.err
}

type fakeManagedDataLifetime struct {
	releases int
}

func (l *fakeManagedDataLifetime) Release() error {
	l.releases++
	return nil
}

func (r fakeManagedDataResolver) ResolveManagedData(context.Context, servingstate.ID) (ManagedDataResolution, error) {
	return r.resolution, r.err
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
	deployment servingstate.State
	artifact   servingstate.Artifact
}

type fakeRegistryRepo struct {
	active      map[string]registryDeploymentArtifact
	deployments map[servingstate.ID]servingstate.State
	artifacts   map[servingstate.ID]servingstate.Artifact
	activeCalls []string
}

func newFakeRegistryRepo() *fakeRegistryRepo {
	return &fakeRegistryRepo{
		active:      map[string]registryDeploymentArtifact{},
		deployments: map[servingstate.ID]servingstate.State{},
		artifacts:   map[servingstate.ID]servingstate.Artifact{},
	}
}

func (r *fakeRegistryRepo) ActiveArtifact(_ context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) (servingstate.State, servingstate.Artifact, error) {
	key := string(workspaceID) + "/" + string(environment)
	r.activeCalls = append(r.activeCalls, key)
	current, ok := r.active[key]
	if !ok {
		return servingstate.State{}, servingstate.Artifact{}, servingstate.ErrNotFound
	}
	return current.deployment, current.artifact, nil
}

func (r *fakeRegistryRepo) ByID(_ context.Context, id servingstate.ID) (servingstate.State, error) {
	current, ok := r.deployments[id]
	if !ok {
		return servingstate.State{}, servingstate.ErrNotFound
	}
	return current, nil
}

func (r *fakeRegistryRepo) ArtifactByServingState(_ context.Context, id servingstate.ID) (servingstate.Artifact, error) {
	artifact, ok := r.artifacts[id]
	if !ok {
		return servingstate.Artifact{}, servingstate.ErrNotFound
	}
	return artifact, nil
}

func (r *fakeRegistryRepo) RecordDuckLakeSnapshot(_ context.Context, servingStateID servingstate.ID, snapshotID int64) error {
	current := r.deployments[servingStateID]
	current.DuckLakeSnapshotID = snapshotID
	r.deployments[servingStateID] = current
	for key, pair := range r.active {
		if pair.deployment.ID == servingStateID {
			pair.deployment.DuckLakeSnapshotID = snapshotID
			r.active[key] = pair
		}
	}
	return nil
}

type recordingRegistryFactory struct {
	inputs      []string
	managedData []ManagedDataResolution
	runtimes    []*recordingRuntime
}

func (f *recordingRegistryFactory) Prepare(_ context.Context, input RuntimeInput) (Runtime, error) {
	f.inputs = append(f.inputs, fmt.Sprintf("%s/%s/%s", input.State.WorkspaceID, input.State.Environment, input.State.ID))
	f.managedData = append(f.managedData, input.ManagedData)
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

type overlapDetectingRegistryFactory struct {
	active     atomic.Int32
	overlapped atomic.Bool
}

type blockingRegistryFactory struct {
	started chan struct{}
	release chan struct{}
}

func (f *blockingRegistryFactory) Prepare(_ context.Context, input RuntimeInput) (Runtime, error) {
	if input.State.ID == "dep_sales_next" {
		close(f.started)
		<-f.release
	}
	return &recordingRuntime{}, nil
}

func (f *overlapDetectingRegistryFactory) Prepare(context.Context, RuntimeInput) (Runtime, error) {
	active := f.active.Add(1)
	if active > 1 {
		f.overlapped.Store(true)
	}
	time.Sleep(25 * time.Millisecond)
	f.active.Add(-1)
	return &recordingRuntime{}, nil
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
