package deployment

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/runtimehost"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

type Repository interface {
	CreateDeployment(context.Context, CreateInput) (Deployment, error)
	DeploymentByID(context.Context, string) (Deployment, error)
	ActivateDeployment(context.Context, string) (Deployment, error)
	CancelDeployment(context.Context, string) (Deployment, error)
	FailDeployment(context.Context, string, error) error
}

func (s *Service) Cancel(ctx context.Context, scope Scope) (Deployment, error) {
	row, err := s.Get(ctx, scope)
	if err != nil {
		return Deployment{}, err
	}
	if row.Status != StatusPending {
		return Deployment{}, fmt.Errorf("%w: deployment is %s", ErrConflict, row.Status)
	}
	return s.repository.CancelDeployment(ctx, row.ID)
}

type ServingStateRepository interface {
	RecordDuckLakeSnapshot(context.Context, servingstate.ID, int64) error
}

type ManagedDataResolver interface {
	ResolveManagedData(context.Context, servingstate.ID) (runtimehost.ManagedDataResolution, error)
}

type Prepared interface {
	Snapshots() []runtimehost.PreparedSnapshot
	Close() error
}

type Runtime interface {
	Prepare(context.Context, []runtimehost.ServingStateCandidate) (Prepared, error)
	Commit(Prepared, func() error) error
}

type registryRuntime struct{ registry *runtimehost.Registry }
type registryPrepared struct{ set *runtimehost.PreparedSet }

func NewRegistryRuntime(registry *runtimehost.Registry) (Runtime, error) {
	if registry == nil {
		return nil, fmt.Errorf("runtime registry is required")
	}
	return registryRuntime{registry: registry}, nil
}

func (r registryRuntime) Prepare(ctx context.Context, candidates []runtimehost.ServingStateCandidate) (Prepared, error) {
	set, err := r.registry.PrepareServingStateCandidates(ctx, candidates)
	if err != nil {
		return nil, err
	}
	return registryPrepared{set: set}, nil
}

func (r registryRuntime) Commit(prepared Prepared, activate func() error) error {
	value, ok := prepared.(registryPrepared)
	if !ok || value.set == nil {
		return fmt.Errorf("prepared runtimes belong to a different deployment coordinator")
	}
	return r.registry.CommitPreparedSet(value.set, activate)
}

func (p registryPrepared) Snapshots() []runtimehost.PreparedSnapshot { return p.set.Snapshots() }
func (p registryPrepared) Close() error                              { return p.set.Close() }

type Service struct {
	repository Repository
	states     ServingStateRepository
	runtime    Runtime
	resolver   ManagedDataResolver
}

func New(repository Repository, states ServingStateRepository, runtime Runtime, resolver ManagedDataResolver) (*Service, error) {
	if repository == nil || states == nil || runtime == nil || resolver == nil {
		return nil, fmt.Errorf("deployment repository, serving-state repository, runtime, and managed-data resolver are required")
	}
	return &Service{repository: repository, states: states, runtime: runtime, resolver: resolver}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Deployment, error) {
	if err := validateCreate(input); err != nil {
		return Deployment{}, err
	}
	input.ID = strings.TrimSpace(input.ID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.Environment = strings.TrimSpace(input.Environment)
	input.RequestDigest = strings.TrimSpace(input.RequestDigest)
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	input.Targets = append([]TargetInput(nil), input.Targets...)
	sort.Slice(input.Targets, func(i, j int) bool { return input.Targets[i].WorkspaceID < input.Targets[j].WorkspaceID })
	return s.repository.CreateDeployment(ctx, input)
}

func (s *Service) Get(ctx context.Context, scope Scope) (Deployment, error) {
	projectID := strings.TrimSpace(scope.ProjectID)
	deploymentID := strings.TrimSpace(scope.DeploymentID)
	if projectID == "" || deploymentID == "" {
		return Deployment{}, fmt.Errorf("project and deployment id are required")
	}
	row, err := s.repository.DeploymentByID(ctx, deploymentID)
	if err != nil {
		return Deployment{}, err
	}
	if row.ID != deploymentID || row.ProjectID != projectID {
		return Deployment{}, ErrNotFound
	}
	return row, nil
}

func (s *Service) Activate(ctx context.Context, scope Scope) (Deployment, error) {
	row, err := s.Get(ctx, scope)
	if err != nil {
		return Deployment{}, err
	}
	if row.Status == StatusActive {
		return row, nil
	}
	if row.Status != StatusPending {
		return Deployment{}, fmt.Errorf("%w: deployment is %s", ErrConflict, row.Status)
	}

	targets := append([]Target(nil), row.Targets...)
	sort.Slice(targets, func(i, j int) bool { return targets[i].WorkspaceID < targets[j].WorkspaceID })
	candidates := make([]runtimehost.ServingStateCandidate, 0, len(targets))
	for _, target := range targets {
		resolution, resolveErr := s.resolver.ResolveManagedData(ctx, servingstate.ID(target.ServingStateID))
		if resolveErr != nil {
			_ = s.repository.FailDeployment(ctx, row.ID, resolveErr)
			return Deployment{}, resolveErr
		}
		candidates = append(candidates, runtimehost.ServingStateCandidate{ServingStateID: target.ServingStateID, ManagedData: resolution})
	}

	prepared, err := s.runtime.Prepare(ctx, candidates)
	if err != nil {
		_ = s.repository.FailDeployment(ctx, row.ID, err)
		return Deployment{}, err
	}
	defer prepared.Close()
	for _, snapshot := range prepared.Snapshots() {
		if snapshot.DuckLakeSnapshotID <= 0 {
			continue
		}
		if err := s.states.RecordDuckLakeSnapshot(ctx, snapshot.ServingStateID, snapshot.DuckLakeSnapshotID); err != nil {
			_ = s.repository.FailDeployment(ctx, row.ID, err)
			return Deployment{}, err
		}
	}

	var activated Deployment
	err = s.runtime.Commit(prepared, func() error {
		var activateErr error
		activated, activateErr = s.repository.ActivateDeployment(ctx, row.ID)
		return activateErr
	})
	if err != nil {
		return Deployment{}, err
	}
	return activated, nil
}
