package deployment

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Yacobolo/leapview/internal/runtimehost"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

func TestActivateResolvesPersistedBindingsPreparesAndAtomicallyCommits(t *testing.T) {
	repo := &fakeRepository{deployment: Deployment{
		ID: "deployment_1", ProjectID: "project", Environment: "prod", Status: StatusPending,
		Targets: []Target{
			{DeploymentID: "deployment_1", WorkspaceID: "support", ServingStateID: "support_2", PriorServingStateID: "support_1", Status: TargetStatusPending},
			{DeploymentID: "deployment_1", WorkspaceID: "sales", ServingStateID: "sales_2", PriorServingStateID: "sales_1", Status: TargetStatusPending},
		},
	}}
	resolver := &fakeResolver{resolutions: map[servingstate.ID]runtimehost.ManagedDataResolution{
		"sales_2":   {RevisionID: "sha256:sales", Roots: map[string]string{"orders": "/cache/orders"}},
		"support_2": {RevisionID: "sha256:support", Roots: map[string]string{}},
	}}
	runtime := &fakeRuntime{prepared: &fakePrepared{snapshots: []runtimehost.PreparedSnapshot{
		{WorkspaceID: "sales", ServingStateID: "sales_2", DuckLakeSnapshotID: 41},
		{WorkspaceID: "support", ServingStateID: "support_2", DuckLakeSnapshotID: 42},
	}}}
	states := &fakeServingStates{}
	service := mustService(t, repo, states, runtime, resolver)

	got, err := service.Activate(context.Background(), Scope{ProjectID: "project", DeploymentID: "deployment_1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusActive {
		t.Fatalf("status = %q", got.Status)
	}
	wantCandidates := []runtimehost.ServingStateCandidate{
		{ServingStateID: "sales_2", ManagedData: resolver.resolutions["sales_2"]},
		{ServingStateID: "support_2", ManagedData: resolver.resolutions["support_2"]},
	}
	if !reflect.DeepEqual(runtime.candidates, wantCandidates) {
		t.Fatalf("candidates = %#v, want %#v", runtime.candidates, wantCandidates)
	}
	if !reflect.DeepEqual(states.recorded, map[servingstate.ID]int64{"sales_2": 41, "support_2": 42}) {
		t.Fatalf("recorded snapshots = %#v", states.recorded)
	}
	if repo.activateCalls != 1 || !runtime.committed {
		t.Fatalf("activate calls = %d, runtime committed = %v", repo.activateCalls, runtime.committed)
	}
}

func TestActivateSupportsDeploymentWithoutManagedConnections(t *testing.T) {
	repo := &fakeRepository{deployment: Deployment{
		ID: "deployment_empty", ProjectID: "project", Environment: "prod", Status: StatusPending,
		Targets: []Target{{DeploymentID: "deployment_empty", WorkspaceID: "sales", ServingStateID: "sales_2", Status: TargetStatusPending}},
	}}
	resolver := &fakeResolver{resolutions: map[servingstate.ID]runtimehost.ManagedDataResolution{
		"sales_2": {Roots: map[string]string{}},
	}}
	runtime := &fakeRuntime{prepared: &fakePrepared{}}
	service := mustService(t, repo, &fakeServingStates{}, runtime, resolver)

	if _, err := service.Activate(context.Background(), Scope{ProjectID: "project", DeploymentID: "deployment_empty"}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.candidates) != 1 || len(runtime.candidates[0].ManagedData.Roots) != 0 {
		t.Fatalf("candidates = %#v", runtime.candidates)
	}
}

func TestActivatePreparationFailureLeavesDeploymentPending(t *testing.T) {
	wantErr := errors.New("duckdb preparation failed")
	repo := &fakeRepository{deployment: Deployment{
		ID: "deployment_1", ProjectID: "project", Environment: "prod", Status: StatusPending,
		Targets: []Target{{DeploymentID: "deployment_1", WorkspaceID: "sales", ServingStateID: "sales_2", Status: TargetStatusPending}},
	}}
	runtime := &fakeRuntime{prepareErr: wantErr}
	service := mustService(t, repo, &fakeServingStates{}, runtime, &fakeResolver{resolutions: map[servingstate.ID]runtimehost.ManagedDataResolution{"sales_2": {Roots: map[string]string{}}}})

	_, err := service.Activate(context.Background(), Scope{ProjectID: "project", DeploymentID: "deployment_1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if repo.activateCalls != 0 || runtime.committed {
		t.Fatalf("activate calls = %d, runtime committed = %v", repo.activateCalls, runtime.committed)
	}
	if repo.failedID != "deployment_1" {
		t.Fatalf("failed deployment = %q", repo.failedID)
	}
}

func TestActivateIsIdempotentAfterSuccess(t *testing.T) {
	repo := &fakeRepository{deployment: Deployment{ID: "deployment_1", ProjectID: "project", Environment: "prod", Status: StatusActive}}
	runtime := &fakeRuntime{}
	service := mustService(t, repo, &fakeServingStates{}, runtime, &fakeResolver{})

	got, err := service.Activate(context.Background(), Scope{ProjectID: "project", DeploymentID: "deployment_1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusActive || runtime.prepareCalls != 0 || repo.activateCalls != 0 {
		t.Fatalf("deployment = %#v, prepare calls = %d, activate calls = %d", got, runtime.prepareCalls, repo.activateCalls)
	}
}

func mustService(t *testing.T, repo Repository, states ServingStateRepository, runtime Runtime, resolver ManagedDataResolver) *Service {
	t.Helper()
	service, err := New(repo, states, runtime, resolver)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type fakeRepository struct {
	deployment    Deployment
	activateCalls int
	failedID      string
}

func (r *fakeRepository) CreateDeployment(context.Context, CreateInput) (Deployment, error) {
	return r.deployment, nil
}
func (r *fakeRepository) DeploymentByID(context.Context, string) (Deployment, error) {
	return r.deployment, nil
}
func (r *fakeRepository) ActivateDeployment(context.Context, string) (Deployment, error) {
	r.activateCalls++
	r.deployment.Status = StatusActive
	return r.deployment, nil
}

func (r *fakeRepository) CancelDeployment(context.Context, string) (Deployment, error) {
	r.deployment.Status = StatusCancelled
	return r.deployment, nil
}
func (r *fakeRepository) FailDeployment(_ context.Context, id string, _ error) error {
	r.failedID = id
	return nil
}

type fakeResolver struct {
	resolutions map[servingstate.ID]runtimehost.ManagedDataResolution
	err         error
}

func (r *fakeResolver) ResolveManagedData(_ context.Context, id servingstate.ID) (runtimehost.ManagedDataResolution, error) {
	return r.resolutions[id], r.err
}

type fakePrepared struct {
	snapshots []runtimehost.PreparedSnapshot
}

func (p *fakePrepared) Snapshots() []runtimehost.PreparedSnapshot {
	return append([]runtimehost.PreparedSnapshot(nil), p.snapshots...)
}
func (p *fakePrepared) Close() error { return nil }

type fakeRuntime struct {
	prepared     Prepared
	prepareErr   error
	prepareCalls int
	candidates   []runtimehost.ServingStateCandidate
	committed    bool
}

func (r *fakeRuntime) Prepare(_ context.Context, candidates []runtimehost.ServingStateCandidate) (Prepared, error) {
	r.prepareCalls++
	r.candidates = append([]runtimehost.ServingStateCandidate(nil), candidates...)
	return r.prepared, r.prepareErr
}
func (r *fakeRuntime) Commit(_ Prepared, activate func() error) error {
	if err := activate(); err != nil {
		return err
	}
	r.committed = true
	return nil
}

type fakeServingStates struct {
	recorded map[servingstate.ID]int64
}

func (s *fakeServingStates) RecordDuckLakeSnapshot(_ context.Context, id servingstate.ID, snapshotID int64) error {
	if s.recorded == nil {
		s.recorded = map[servingstate.ID]int64{}
	}
	s.recorded[id] = snapshotID
	return nil
}
