package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

type RegistryOptions struct {
	Repo             ServingStateRepository
	WorkspaceIDs     []servingstate.WorkspaceID
	Environment      servingstate.Environment
	Factory          RuntimeFactory
	ManagedData      ManagedDataResolver
	OnDrained        func(servingstate.ID, int64)
	Logger           *slog.Logger
	OnCleanupFailure func(CleanupFailure)
}

type Registry struct {
	mu               sync.RWMutex
	prepareMu        sync.Mutex
	cutoverMu        sync.RWMutex
	repo             ServingStateRepository
	environment      servingstate.Environment
	factory          RuntimeFactory
	managedData      ManagedDataResolver
	onDrained        func(servingstate.ID, int64)
	logger           *slog.Logger
	onCleanupFailure func(CleanupFailure)
	managers         map[servingstate.WorkspaceID]*Manager
}

type RegistryPrepared struct {
	registry    *Registry
	workspaceID servingstate.WorkspaceID
	manager     *Manager
	prepared    servingstate.PreparedRuntime
}

// PreparedSet contains candidate runtimes that must become visible as one rollout.
type PreparedSet struct {
	mu        sync.Mutex
	registry  *Registry
	items     []*RegistryPrepared
	committed bool
	consumed  bool
}

// PreparedSnapshot identifies durable snapshot metadata produced while a
// serving-state candidate is still private.
type PreparedSnapshot struct {
	WorkspaceID        servingstate.WorkspaceID
	ServingStateID     servingstate.ID
	DuckLakeSnapshotID int64
}

// Snapshots returns candidate snapshot metadata in deterministic workspace
// order. Callers may persist these values before the atomic activation step.
func (p *PreparedSet) Snapshots() []PreparedSnapshot {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]PreparedSnapshot, 0, len(p.items))
	for _, item := range p.items {
		if item == nil {
			continue
		}
		prepared, ok := item.prepared.(*Prepared)
		if !ok || prepared == nil {
			continue
		}
		out = append(out, PreparedSnapshot{
			WorkspaceID:        item.workspaceID,
			ServingStateID:     prepared.servingStateID,
			DuckLakeSnapshotID: prepared.snapshotID,
		})
	}
	return out
}

// ServingStateCandidate binds an unpublished runtime preparation to an
// explicit managed-data resolution. The resolution is committed separately.
type ServingStateCandidate struct {
	ServingStateID string
	ManagedData    ManagedDataResolution
}

func (p *PreparedSet) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var first error
	for _, item := range p.items {
		if err := item.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (p *RegistryPrepared) Close() error {
	if p == nil || p.prepared == nil {
		return nil
	}
	return p.prepared.Close()
}

type WorkspaceProvider struct {
	registry    *Registry
	workspaceID servingstate.WorkspaceID
}

func NewRegistryWithFactory(options RegistryOptions) *Registry {
	registry := &Registry{
		repo:             options.Repo,
		environment:      servingstate.NormalizeEnvironment(options.Environment),
		factory:          options.Factory,
		managedData:      options.ManagedData,
		onDrained:        options.OnDrained,
		logger:           options.Logger,
		onCleanupFailure: options.OnCleanupFailure,
		managers:         map[servingstate.WorkspaceID]*Manager{},
	}
	for _, workspaceID := range options.WorkspaceIDs {
		registry.managerForWorkspace(workspaceID)
	}
	return registry
}

func (r *Registry) Reload(ctx context.Context) error {
	for _, workspaceID := range r.workspaceIDs() {
		manager := r.managerForWorkspace(workspaceID)
		r.prepareMu.Lock()
		err := manager.ReloadBeforePrepare(ctx, r.closePreparedRuntimes)
		r.prepareMu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) PrepareServingState(ctx context.Context, servingStateID string) (servingstate.PreparedRuntime, error) {
	current, err := r.repo.ByID(ctx, servingstate.ID(servingStateID))
	if err != nil {
		return nil, err
	}
	if servingstate.NormalizeEnvironment(current.Environment) != r.environment {
		return nil, fmt.Errorf("serving state %s environment = %q, want %q", servingStateID, current.Environment, r.environment)
	}
	manager := r.managerForWorkspace(current.WorkspaceID)
	r.prepareMu.Lock()
	if err := r.closePreparedRuntimes(); err != nil {
		r.prepareMu.Unlock()
		return nil, err
	}
	prepared, err := manager.PrepareServingState(ctx, servingStateID)
	r.prepareMu.Unlock()
	if err != nil {
		return nil, err
	}
	return &RegistryPrepared{registry: r, workspaceID: current.WorkspaceID, manager: manager, prepared: prepared}, nil
}

// PrepareServingStates prepares one candidate per workspace without exposing a
// partial rollout. Preparation is serialized because DuckLake has one writer.
func (r *Registry) PrepareServingStates(ctx context.Context, servingStateIDs []string) (*PreparedSet, error) {
	candidates := make([]servingStateCandidate, 0, len(servingStateIDs))
	for _, servingStateID := range servingStateIDs {
		candidates = append(candidates, servingStateCandidate{servingStateID: servingStateID})
	}
	return r.prepareServingStateCandidates(ctx, candidates)
}

// PrepareServingStateCandidates prepares runtime generations against data
// that is not durable yet. ActivatePreparedSet must persist the corresponding
// bindings in its activation callback before exposing these runtimes.
func (r *Registry) PrepareServingStateCandidates(ctx context.Context, inputs []ServingStateCandidate) (*PreparedSet, error) {
	candidates := make([]servingStateCandidate, 0, len(inputs))
	for _, input := range inputs {
		resolution := input.ManagedData
		candidates = append(candidates, servingStateCandidate{servingStateID: input.ServingStateID, managedData: &resolution})
	}
	return r.prepareServingStateCandidates(ctx, candidates)
}

type servingStateCandidate struct {
	servingStateID string
	managedData    *ManagedDataResolution
}

func (r *Registry) prepareServingStateCandidates(ctx context.Context, inputs []servingStateCandidate) (_ *PreparedSet, resultErr error) {
	defer func() {
		for _, input := range inputs {
			if input.managedData == nil || input.managedData.Lifetime == nil {
				continue
			}
			resultErr = errors.Join(resultErr, releaseManagedDataLifetime(input.managedData.Lifetime))
			input.managedData.Lifetime = nil
		}
	}()
	type candidate struct {
		state       servingstate.State
		artifact    servingstate.Artifact
		managedData *ManagedDataResolution
	}
	candidates := make([]candidate, 0, len(inputs))
	workspaces := make(map[servingstate.WorkspaceID]struct{}, len(inputs))
	for _, input := range inputs {
		current, err := r.repo.ByID(ctx, servingstate.ID(input.servingStateID))
		if err != nil {
			return nil, err
		}
		if servingstate.NormalizeEnvironment(current.Environment) != r.environment {
			return nil, fmt.Errorf("serving state %s environment = %q, want %q", input.servingStateID, current.Environment, r.environment)
		}
		if _, duplicate := workspaces[current.WorkspaceID]; duplicate {
			return nil, fmt.Errorf("multiple serving states supplied for workspace %s", current.WorkspaceID)
		}
		workspaces[current.WorkspaceID] = struct{}{}
		artifact, err := r.repo.ArtifactByServingState(ctx, current.ID)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate{state: current, artifact: artifact, managedData: input.managedData})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].state.WorkspaceID < candidates[j].state.WorkspaceID })

	r.prepareMu.Lock()
	defer r.prepareMu.Unlock()
	set := &PreparedSet{registry: r, items: make([]*RegistryPrepared, 0, len(candidates))}
	for _, candidate := range candidates {
		manager := r.managerForWorkspace(candidate.state.WorkspaceID)
		var prepared *Prepared
		var err error
		if candidate.managedData == nil {
			prepared, err = manager.prepare(ctx, candidate.state, candidate.artifact)
		} else {
			prepared, err = manager.prepareResolved(ctx, candidate.state, candidate.artifact, *candidate.managedData)
			candidate.managedData.Lifetime = nil
		}
		if err != nil {
			_ = set.Close()
			return nil, err
		}
		set.items = append(set.items, &RegistryPrepared{registry: r, workspaceID: candidate.state.WorkspaceID, manager: manager, prepared: prepared})
	}
	return set, nil
}

// ActivatePrepared serializes a single-workspace durable pointer
// update with its in-memory runtime swap.
func (r *Registry) ActivatePrepared(candidate servingstate.PreparedRuntime, activate func() error) error {
	if activate == nil {
		return fmt.Errorf("metadata activation is required")
	}
	prepared, err := r.sealPrepared(candidate)
	if err != nil {
		return err
	}
	r.cutoverMu.Lock()
	if err := activate(); err != nil {
		r.cutoverMu.Unlock()
		return errors.Join(err, prepared.abort())
	}
	retired := prepared.publish()
	r.cutoverMu.Unlock()
	prepared.manager.cleanupRetired(retired)
	return nil
}

// ActivatePreparedSet serializes the durable pointer transaction with the
// in-memory swap, so requests cannot observe a mixture of rollout revisions.
func (r *Registry) ActivatePreparedSet(set *PreparedSet, activate func() error) error {
	if set == nil || set.registry != r {
		return fmt.Errorf("prepared set belongs to a different host")
	}
	set.mu.Lock()
	defer set.mu.Unlock()
	if set.committed {
		return fmt.Errorf("prepared set is already committed")
	}
	if set.consumed {
		return fmt.Errorf("prepared set is already consumed")
	}
	if activate == nil {
		return fmt.Errorf("metadata activation is required")
	}
	workspaces := make(map[servingstate.WorkspaceID]struct{}, len(set.items))
	for _, item := range set.items {
		if item == nil || item.registry != r || item.manager == nil || item.prepared == nil {
			return fmt.Errorf("prepared runtime is nil")
		}
		if _, duplicate := workspaces[item.workspaceID]; duplicate {
			return fmt.Errorf("multiple prepared runtimes supplied for workspace %s", item.workspaceID)
		}
		workspaces[item.workspaceID] = struct{}{}
	}
	batch := make([]*sealedPrepared, 0, len(set.items))
	for _, item := range set.items {
		sealed, err := r.sealRegistryPrepared(item)
		if err != nil {
			return errors.Join(err, abortSealed(batch))
		}
		batch = append(batch, sealed)
	}
	set.consumed = true

	r.cutoverMu.Lock()
	if err := activate(); err != nil {
		r.cutoverMu.Unlock()
		return errors.Join(err, abortSealed(batch))
	}
	retired := make([]struct {
		manager *Manager
		runtime *managedRuntime
	}, 0, len(batch))
	for _, item := range batch {
		retired = append(retired, struct {
			manager *Manager
			runtime *managedRuntime
		}{manager: item.manager, runtime: item.publish()})
	}
	set.committed = true
	r.cutoverMu.Unlock()
	for _, item := range retired {
		item.manager.cleanupRetired(item.runtime)
	}
	return nil
}

func (r *Registry) sealPrepared(candidate servingstate.PreparedRuntime) (*sealedPrepared, error) {
	prepared, ok := candidate.(*RegistryPrepared)
	if !ok || prepared == nil {
		return nil, fmt.Errorf("prepared runtime belongs to a different host")
	}
	return r.sealRegistryPrepared(prepared)
}

func (r *Registry) sealRegistryPrepared(prepared *RegistryPrepared) (*sealedPrepared, error) {
	if prepared == nil || prepared.registry != r || prepared.manager == nil || prepared.prepared == nil {
		return nil, fmt.Errorf("prepared runtime belongs to a different host")
	}
	return prepared.manager.sealPrepared(prepared.prepared)
}

func abortSealed(items []*sealedPrepared) error {
	errs := make([]error, 0, len(items))
	for _, item := range items {
		if err := item.abort(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Registry) Close() error {
	var first error
	for _, workspaceID := range r.workspaceIDs() {
		if err := r.managerForWorkspace(workspaceID).Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (r *Registry) AcquireForWorkspace(ctx context.Context, workspaceID servingstate.WorkspaceID) (Lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.cutoverMu.RLock()
	defer r.cutoverMu.RUnlock()
	r.mu.RLock()
	manager := r.managers[workspaceID]
	r.mu.RUnlock()
	if manager == nil {
		return nil, fmt.Errorf("no active LeapView serving state")
	}
	return manager.Acquire()
}

func (r *Registry) ProviderForWorkspace(workspaceID servingstate.WorkspaceID) *WorkspaceProvider {
	r.managerForWorkspace(workspaceID)
	return &WorkspaceProvider{registry: r, workspaceID: workspaceID}
}

func (p *WorkspaceProvider) Acquire(ctx context.Context) (Lease, error) {
	if p == nil || p.registry == nil {
		return nil, fmt.Errorf("runtime provider is not configured")
	}
	return p.registry.AcquireForWorkspace(ctx, p.workspaceID)
}

func (r *Registry) LeasedSnapshots() []int64 {
	r.mu.RLock()
	managers := make([]*Manager, 0, len(r.managers))
	for _, manager := range r.managers {
		managers = append(managers, manager)
	}
	r.mu.RUnlock()
	snapshots := map[int64]struct{}{}
	for _, manager := range managers {
		for _, snapshotID := range manager.LeasedSnapshots() {
			snapshots[snapshotID] = struct{}{}
		}
	}
	return snapshotKeys(snapshots)
}

func (r *Registry) managerForWorkspace(workspaceID servingstate.WorkspaceID) *Manager {
	r.mu.Lock()
	defer r.mu.Unlock()
	if manager := r.managers[workspaceID]; manager != nil {
		return manager
	}
	manager := NewManagerWithFactory(ManagerOptions{
		Repo:             r.repo,
		WorkspaceID:      workspaceID,
		Environment:      r.environment,
		Factory:          r.factory,
		ManagedData:      r.managedData,
		OnDrained:        r.onDrained,
		Logger:           r.logger,
		OnCleanupFailure: r.onCleanupFailure,
	})
	r.managers[workspaceID] = manager
	return manager
}

func (r *Registry) workspaceIDs() []servingstate.WorkspaceID {
	r.mu.RLock()
	ids := make([]servingstate.WorkspaceID, 0, len(r.managers))
	for id := range r.managers {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (r *Registry) closePreparedRuntimes() error {
	r.mu.RLock()
	managers := make([]*Manager, 0, len(r.managers))
	for _, manager := range r.managers {
		managers = append(managers, manager)
	}
	r.mu.RUnlock()
	var first error
	for _, manager := range managers {
		if err := manager.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
