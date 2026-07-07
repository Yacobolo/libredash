package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

type ServingStateRepository interface {
	ActiveArtifact(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) (servingstate.State, servingstate.Artifact, error)
	ByID(ctx context.Context, id servingstate.ID) (servingstate.State, error)
	ArtifactByServingState(ctx context.Context, servingStateID servingstate.ID) (servingstate.Artifact, error)
	RecordDuckLakeSnapshot(ctx context.Context, servingStateID servingstate.ID, snapshotID int64) error
}

type Runtime interface {
	Close() error
}

type RuntimeSnapshot interface {
	DuckLakeSnapshotID() int64
}

type Lease interface {
	Runtime() Runtime
	ServingStateID() servingstate.ID
	DuckLakeSnapshotID() int64
	Release()
}

type RuntimeFactory interface {
	Prepare(ctx context.Context, input RuntimeInput) (Runtime, error)
}

type SnapshotLeaseRepository interface {
	CreateQuerySnapshotLease(ctx context.Context, input servingstate.SnapshotLeaseInput) (string, error)
	ReleaseQuerySnapshotLease(ctx context.Context, id string) error
	ExtendQuerySnapshotLease(ctx context.Context, id string, expiresAt time.Time) error
}

type RuntimeInput struct {
	State      servingstate.State
	Artifact   servingstate.Artifact
	DataDir    string
	DuckDBDir  string
	RuntimeDir string
}

type Manager struct {
	mu                   sync.RWMutex
	repo                 ServingStateRepository
	workspaceID          servingstate.WorkspaceID
	environment          servingstate.Environment
	dataDir              string
	factory              RuntimeFactory
	onDrained            func(servingstate.ID, int64)
	leaseTTL             time.Duration
	leaseOwner           string
	activeServingStateID servingstate.ID
	activeDigest         string
	activeSnapshotID     int64
	current              *managedRuntime
	retired              []*managedRuntime
}

type ManagerOptions struct {
	Repo        ServingStateRepository
	WorkspaceID servingstate.WorkspaceID
	Environment servingstate.Environment
	DataDir     string
	Factory     RuntimeFactory
	OnDrained   func(servingstate.ID, int64)
	LeaseTTL    time.Duration
	LeaseOwner  string
}

type Prepared struct {
	servingStateID servingstate.ID
	digest         string
	runtime        Runtime
	noChange       bool
	snapshotID     int64
}

func (p *Prepared) Close() error {
	if p == nil || p.runtime == nil {
		return nil
	}
	return p.runtime.Close()
}

func (p *Prepared) DuckLakeSnapshotID() int64 {
	if p == nil {
		return 0
	}
	return p.snapshotID
}

func NewManagerWithFactory(options ManagerOptions) *Manager {
	return &Manager{
		repo:        options.Repo,
		workspaceID: options.WorkspaceID,
		environment: servingstate.NormalizeEnvironment(options.Environment),
		dataDir:     options.DataDir,
		factory:     options.Factory,
		onDrained:   options.OnDrained,
		leaseTTL:    normalizedLeaseTTL(options.LeaseTTL),
		leaseOwner:  firstNonEmpty(options.LeaseOwner, "runtimehost"),
	}
}

func (m *Manager) Reload(ctx context.Context) error {
	return m.ReloadBeforePrepare(ctx, nil)
}

func (m *Manager) ReloadBeforePrepare(ctx context.Context, beforePrepare func() error) error {
	current, artifact, err := m.repo.ActiveArtifact(ctx, m.workspaceID, m.environment)
	if err != nil {
		if errors.Is(err, servingstate.ErrNotFound) {
			return m.Close()
		}
		return err
	}
	if !m.needsPrepare(current, artifact) {
		return nil
	}
	if beforePrepare != nil && current.DuckLakeSnapshotID == 0 {
		if err := beforePrepare(); err != nil {
			return err
		}
	}
	prepared, err := m.prepare(ctx, current, artifact)
	if err != nil {
		return err
	}
	if current.DuckLakeSnapshotID == 0 && prepared.DuckLakeSnapshotID() > 0 {
		if err := m.repo.RecordDuckLakeSnapshot(ctx, current.ID, prepared.DuckLakeSnapshotID()); err != nil {
			_ = prepared.Close()
			return err
		}
	}
	return m.CommitPrepared(prepared)
}

func (m *Manager) needsPrepare(current servingstate.State, artifact servingstate.Artifact) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current == nil ||
		m.activeServingStateID != current.ID ||
		m.activeDigest != artifact.Digest ||
		m.activeSnapshotID != current.DuckLakeSnapshotID
}

func (m *Manager) PrepareServingState(ctx context.Context, servingStateID string) (servingstate.PreparedRuntime, error) {
	current, err := m.repo.ByID(ctx, servingstate.ID(servingStateID))
	if err != nil {
		return nil, err
	}
	if current.WorkspaceID != m.workspaceID {
		return nil, fmt.Errorf("serving state %s is not in workspace %s", servingStateID, m.workspaceID)
	}
	artifact, err := m.repo.ArtifactByServingState(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	return m.prepare(ctx, current, artifact)
}

func (m *Manager) prepare(ctx context.Context, current servingstate.State, artifact servingstate.Artifact) (*Prepared, error) {
	m.mu.RLock()
	if m.current != nil && m.activeServingStateID == current.ID && m.activeDigest == artifact.Digest && m.activeSnapshotID == current.DuckLakeSnapshotID {
		m.mu.RUnlock()
		return &Prepared{servingStateID: current.ID, digest: artifact.Digest, noChange: true}, nil
	}
	m.mu.RUnlock()

	runtime, err := m.factory.Prepare(ctx, RuntimeInput{
		State:    current,
		Artifact: artifact,
		DataDir:  m.dataDir,
	})
	if err != nil {
		return nil, err
	}
	var snapshotID int64
	if snapshot, ok := runtime.(RuntimeSnapshot); ok {
		snapshotID = snapshot.DuckLakeSnapshotID()
	}
	if snapshotID == 0 {
		snapshotID = current.DuckLakeSnapshotID
	}
	return &Prepared{servingStateID: current.ID, digest: artifact.Digest, runtime: runtime, snapshotID: snapshotID}, nil
}

func (m *Manager) CommitPrepared(candidate servingstate.PreparedRuntime) error {
	prepared, ok := candidate.(*Prepared)
	if !ok {
		return fmt.Errorf("prepared runtime belongs to a different host")
	}
	if prepared == nil {
		return fmt.Errorf("prepared runtime is nil")
	}
	if prepared.noChange {
		return nil
	}

	m.mu.Lock()
	old := m.current
	m.current = &managedRuntime{
		servingStateID: prepared.servingStateID,
		digest:         prepared.digest,
		runtime:        prepared.runtime,
		snapshotID:     prepared.snapshotID,
	}
	m.activeServingStateID = prepared.servingStateID
	m.activeDigest = prepared.digest
	m.activeSnapshotID = prepared.snapshotID
	prepared.runtime = nil
	oldToClose := m.retireLocked(old)
	m.mu.Unlock()
	m.closeManaged(oldToClose)
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	current := m.current
	m.current = nil
	m.activeServingStateID = ""
	m.activeDigest = ""
	m.activeSnapshotID = 0
	currentToClose := m.retireLocked(current)
	m.mu.Unlock()
	if currentToClose == nil {
		return nil
	}
	return m.closeManaged(currentToClose)
}

func (m *Manager) Active() (Runtime, error) {
	lease, err := m.Acquire()
	if err != nil {
		return nil, err
	}
	runtime := lease.Runtime()
	lease.Release()
	return runtime, nil
}

func (m *Manager) Acquire() (Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil || m.current.closing {
		return nil, fmt.Errorf("no active LibreDash serving state")
	}
	leaseID, heartbeatCancel, err := m.createPersistentLeaseLocked()
	if err != nil {
		return nil, err
	}
	m.current.refs++
	return &runtimeLease{manager: m, managed: m.current, leaseID: leaseID, heartbeatCancel: heartbeatCancel}, nil
}

func (m *Manager) LeasedSnapshots() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshots := map[int64]struct{}{}
	if m.current != nil && m.current.refs > 0 && m.current.snapshotID > 0 {
		snapshots[m.current.snapshotID] = struct{}{}
	}
	for _, runtime := range m.retired {
		if runtime.refs > 0 && runtime.snapshotID > 0 {
			snapshots[runtime.snapshotID] = struct{}{}
		}
	}
	return snapshotKeys(snapshots)
}

func (m *Manager) retireLocked(runtime *managedRuntime) *managedRuntime {
	if runtime == nil {
		return nil
	}
	runtime.closing = true
	if runtime.refs > 0 {
		m.retired = append(m.retired, runtime)
		return nil
	}
	return runtime
}

func (m *Manager) release(runtime *managedRuntime, leaseID string, heartbeatCancel context.CancelFunc) {
	if heartbeatCancel != nil {
		heartbeatCancel()
	}
	if leaseID != "" {
		if repo, ok := m.repo.(SnapshotLeaseRepository); ok {
			releaseSnapshotLease(repo, leaseID)
		}
	}
	var drained *managedRuntime
	m.mu.Lock()
	if runtime != nil && runtime.refs > 0 {
		runtime.refs--
		if runtime.refs == 0 && runtime.closing {
			drained = runtime
			m.removeRetiredLocked(runtime)
		}
	}
	m.mu.Unlock()
	_ = m.closeManaged(drained)
}

func releaseSnapshotLease(repo SnapshotLeaseRepository, leaseID string) {
	if repo == nil || leaseID == "" {
		return
	}
	delay := 25 * time.Millisecond
	for attempt := 0; attempt < 5; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := repo.ReleaseQuerySnapshotLease(ctx, leaseID)
		cancel()
		if err == nil {
			return
		}
		time.Sleep(delay)
		delay *= 2
	}
	_ = repo.ReleaseQuerySnapshotLease(context.Background(), leaseID)
}

func (m *Manager) removeRetiredLocked(runtime *managedRuntime) {
	for index, retired := range m.retired {
		if retired == runtime {
			m.retired = append(m.retired[:index], m.retired[index+1:]...)
			return
		}
	}
}

func (m *Manager) closeManaged(runtime *managedRuntime) error {
	if runtime == nil || runtime.runtime == nil {
		return nil
	}
	err := runtime.runtime.Close()
	if runtime.closing && m.onDrained != nil {
		m.onDrained(runtime.servingStateID, runtime.snapshotID)
	}
	return err
}

type managedRuntime struct {
	servingStateID servingstate.ID
	digest         string
	runtime        Runtime
	snapshotID     int64
	refs           int
	closing        bool
}

type runtimeLease struct {
	manager         *Manager
	managed         *managedRuntime
	leaseID         string
	heartbeatCancel context.CancelFunc
	once            sync.Once
}

func (l *runtimeLease) Runtime() Runtime {
	if l == nil || l.managed == nil {
		return nil
	}
	return l.managed.runtime
}

func (l *runtimeLease) ServingStateID() servingstate.ID {
	if l == nil || l.managed == nil {
		return ""
	}
	return l.managed.servingStateID
}

func (l *runtimeLease) DuckLakeSnapshotID() int64 {
	if l == nil || l.managed == nil {
		return 0
	}
	return l.managed.snapshotID
}

func (l *runtimeLease) Release() {
	if l == nil || l.manager == nil || l.managed == nil {
		return
	}
	l.once.Do(func() {
		l.manager.release(l.managed, l.leaseID, l.heartbeatCancel)
	})
}

func (m *Manager) createPersistentLeaseLocked() (string, context.CancelFunc, error) {
	repo, ok := m.repo.(SnapshotLeaseRepository)
	if !ok || m.current == nil || m.current.snapshotID <= 0 {
		return "", nil, nil
	}
	expiresAt := time.Now().Add(m.leaseTTL)
	leaseID, err := repo.CreateQuerySnapshotLease(context.Background(), servingstate.SnapshotLeaseInput{
		WorkspaceID:        m.workspaceID,
		Environment:        m.environment,
		ServingStateID:     m.current.servingStateID,
		DuckLakeSnapshotID: m.current.snapshotID,
		OwnerID:            m.leaseOwner,
		ExpiresAt:          expiresAt,
	})
	if err != nil {
		return "", nil, err
	}
	heartbeatCtx, cancel := context.WithCancel(context.Background())
	go m.heartbeatLease(heartbeatCtx, repo, leaseID)
	return leaseID, cancel, nil
}

func (m *Manager) heartbeatLease(ctx context.Context, repo SnapshotLeaseRepository, leaseID string) {
	interval := m.leaseTTL / 2
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = repo.ExtendQuerySnapshotLease(context.Background(), leaseID, time.Now().Add(m.leaseTTL))
		}
	}
}

func normalizedLeaseTTL(value time.Duration) time.Duration {
	if value <= 0 {
		return 5 * time.Minute
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func snapshotKeys(values map[int64]struct{}) []int64 {
	if len(values) == 0 {
		return nil
	}
	keys := make([]int64, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	return keys
}
