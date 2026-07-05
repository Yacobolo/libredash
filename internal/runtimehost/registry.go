package runtimehost

import (
	"context"
	"fmt"
	"sort"
	"sync"

	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

type RegistryOptions struct {
	Repo         ServingStateRepository
	WorkspaceIDs []servingstate.WorkspaceID
	Environment  servingstate.Environment
	DataDir      string
	Factory      RuntimeFactory
	OnDrained    func(servingstate.ID, int64)
}

type Registry struct {
	mu          sync.RWMutex
	repo        ServingStateRepository
	environment servingstate.Environment
	dataDir     string
	factory     RuntimeFactory
	onDrained   func(servingstate.ID, int64)
	managers    map[servingstate.WorkspaceID]*Manager
}

type RegistryPrepared struct {
	workspaceID servingstate.WorkspaceID
	manager     *Manager
	prepared    servingstate.PreparedRuntime
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
		dataDir:     options.DataDir,
		factory:     options.Factory,
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
		if err := manager.Reload(ctx); err != nil {
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
	prepared, err := manager.PrepareServingState(ctx, servingStateID)
	if err != nil {
		return nil, err
	}
	return &RegistryPrepared{workspaceID: current.WorkspaceID, manager: manager, prepared: prepared}, nil
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
	r.mu.RLock()
	manager := r.managers[workspaceID]
	r.mu.RUnlock()
	if manager == nil {
		return nil, fmt.Errorf("no active LibreDash serving state")
	}
	if err := manager.Reload(ctx); err != nil {
		return nil, err
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
		DataDir:     r.dataDir,
		Factory:     r.factory,
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
