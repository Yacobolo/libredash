package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

type RegistryOptions struct {
	Repo         ServingStateRepository
	WorkspaceIDs []servingstate.WorkspaceID
	Environment  servingstate.Environment
	Factory      RuntimeFactory
	ManagedData  ManagedDataResolver
	OnDrained    func(servingstate.ID, int64)
}

type Registry struct {
	mu          sync.RWMutex
	prepareMu   sync.Mutex
	cutoverMu   sync.RWMutex
	repo        ServingStateRepository
	environment servingstate.Environment
	factory     RuntimeFactory
	managedData ManagedDataResolver
	onDrained   func(servingstate.ID, int64)
	managers    map[servingstate.WorkspaceID]*Manager
}

type RegistryPrepared struct {
	workspaceID servingstate.WorkspaceID
	manager     *Manager
	prepared    servingstate.PreparedRuntime
}

// PreparedSet contains candidate runtimes that must become visible as one rollout.
type PreparedSet struct {
	registry  *Registry
	items     []*RegistryPrepared
	committed bool
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
		repo:        options.Repo,
		environment: servingstate.NormalizeEnvironment(options.Environment),
		factory:     options.Factory,
		managedData: options.ManagedData,
		onDrained:   options.OnDrained,
		managers:    map[servingstate.WorkspaceID]*Manager{},
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
	return &RegistryPrepared{workspaceID: current.WorkspaceID, manager: manager, prepared: prepared}, nil
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
// that is not durable yet. CommitPreparedSet must persist the corresponding
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
		set.items = append(set.items, &RegistryPrepared{workspaceID: candidate.state.WorkspaceID, manager: manager, prepared: prepared})
	}
	return set, nil
}

func (r *Registry) CommitPrepared(candidate servingstate.PreparedRuntime) error {
	prepared, ok := candidate.(*RegistryPrepared)
	if !ok {
		return fmt.Errorf("prepared runtime belongs to a different host")
	}
	if prepared == nil || prepared.manager == nil || prepared.prepared == nil {
		return fmt.Errorf("prepared runtime is nil")
	}
	return prepared.manager.CommitPrepared(prepared.prepared)
}

// CommitPreparedWithActivation serializes a single-workspace durable pointer
// update with its in-memory runtime swap.
func (r *Registry) CommitPreparedWithActivation(candidate servingstate.PreparedRuntime, activate func() error) error {
	prepared, ok := candidate.(*RegistryPrepared)
	if !ok {
		return fmt.Errorf("prepared runtime belongs to a different host")
	}
	if prepared == nil || prepared.manager == nil || prepared.prepared == nil {
		return fmt.Errorf("prepared runtime is nil")
	}
	if activate == nil {
		return fmt.Errorf("metadata activation is required")
	}
	r.cutoverMu.Lock()
	defer r.cutoverMu.Unlock()
	if err := activate(); err != nil {
		return err
	}
	return prepared.manager.CommitPrepared(prepared.prepared)
}

// CommitPreparedSet serializes the durable pointer transaction with the
// in-memory swap, so requests cannot observe a mixture of rollout revisions.
func (r *Registry) CommitPreparedSet(set *PreparedSet, activate func() error) error {
	if set == nil || set.registry != r {
		return fmt.Errorf("prepared set belongs to a different host")
	}
	if set.committed {
		return fmt.Errorf("prepared set is already committed")
	}
	if activate == nil {
		return fmt.Errorf("metadata activation is required")
	}
	for _, item := range set.items {
		if item == nil || item.manager == nil || item.prepared == nil {
			return fmt.Errorf("prepared runtime is nil")
		}
		if _, ok := item.prepared.(*Prepared); !ok {
			return fmt.Errorf("prepared runtime belongs to a different host")
		}
	}

	r.cutoverMu.Lock()
	defer r.cutoverMu.Unlock()
	if err := activate(); err != nil {
		return err
	}
	for _, item := range set.items {
		if err := item.manager.CommitPrepared(item.prepared); err != nil {
			return err
		}
	}
	set.committed = true
	return nil
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

func (r *Registry) ActiveForWorkspace(ctx context.Context, workspaceID servingstate.WorkspaceID) (Runtime, error) {
	lease, err := r.AcquireForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	runtime := lease.Runtime()
	lease.Release()
	return runtime, nil
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

func (p *WorkspaceProvider) Active(ctx context.Context) (Runtime, error) {
	if p == nil || p.registry == nil {
		return nil, fmt.Errorf("runtime provider is not configured")
	}
	return p.registry.ActiveForWorkspace(ctx, p.workspaceID)
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
		Repo:        r.repo,
		WorkspaceID: workspaceID,
		Environment: r.environment,
		Factory:     r.factory,
		ManagedData: r.managedData,
		OnDrained:   r.onDrained,
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
