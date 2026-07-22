package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
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

// Provider exposes an active runtime only through a lifetime-bearing lease.
type Provider interface {
	Acquire(ctx context.Context) (Lease, error)
}

type CleanupResource string

const (
	CleanupResourceRuntime       CleanupResource = "runtime"
	CleanupResourceManagedData   CleanupResource = "managed_data"
	CleanupResourceSnapshotLease CleanupResource = "snapshot_lease"
)

// CleanupFailure describes a post-publication retirement failure. Such a
// failure is operationally significant, but never reverses an activation.
type CleanupFailure struct {
	WorkspaceID        servingstate.WorkspaceID
	ServingStateID     servingstate.ID
	DuckLakeSnapshotID int64
	Resource           CleanupResource
	Err                error
}

type RuntimeFactory interface {
	Prepare(ctx context.Context, input RuntimeInput) (Runtime, error)
}

type ManagedDataResolution struct {
	RevisionID string
	Roots      map[string]string
	Lifetime   ManagedDataLifetime
}

// ManagedDataLifetime keeps all roots in a resolution available to its runtime.
type ManagedDataLifetime interface {
	Release() error
}

type ManagedDataResolver interface {
	ResolveManagedData(ctx context.Context, servingStateID servingstate.ID) (ManagedDataResolution, error)
}

type SnapshotLeaseRepository interface {
	CreateQuerySnapshotLease(ctx context.Context, input servingstate.SnapshotLeaseInput) (string, error)
	ReleaseQuerySnapshotLease(ctx context.Context, id string) error
	ExtendQuerySnapshotLease(ctx context.Context, id string, expiresAt time.Time) error
}

type RuntimeInput struct {
	State       servingstate.State
	Artifact    servingstate.Artifact
	ManagedData ManagedDataResolution
	DuckDBDir   string
	RuntimeDir  string
}

type Manager struct {
	mu                    sync.RWMutex
	repo                  ServingStateRepository
	workspaceID           servingstate.WorkspaceID
	environment           servingstate.Environment
	factory               RuntimeFactory
	managedData           ManagedDataResolver
	onDrained             func(servingstate.ID, int64)
	leaseTTL              time.Duration
	leaseOwner            string
	logger                *slog.Logger
	onLeaseRenewalFailure func(error)
	onCleanupFailure      func(CleanupFailure)
	leaseRenewalErrors    map[string]error
	activeServingStateID  servingstate.ID
	activeDigest          string
	activeManagedRevision string
	activeSnapshotID      int64
	current               *managedRuntime
	retired               []*managedRuntime
}

type ManagerOptions struct {
	Repo                  ServingStateRepository
	WorkspaceID           servingstate.WorkspaceID
	Environment           servingstate.Environment
	Factory               RuntimeFactory
	ManagedData           ManagedDataResolver
	OnDrained             func(servingstate.ID, int64)
	LeaseTTL              time.Duration
	LeaseOwner            string
	Logger                *slog.Logger
	OnLeaseRenewalFailure func(error)
	OnCleanupFailure      func(CleanupFailure)
}

type Prepared struct {
	mu              sync.Mutex
	owner           *Manager
	state           preparedState
	servingStateID  servingstate.ID
	digest          string
	managedRevision string
	runtime         Runtime
	managedData     ManagedDataLifetime
	snapshotLease   *persistentSnapshotLease
	noChange        bool
	snapshotID      int64
}

type preparedState uint8

const (
	preparedStateOpen preparedState = iota
	preparedStateSealed
	preparedStatePublished
	preparedStateClosed
)

func (p *Prepared) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != preparedStateOpen {
		return nil
	}
	p.state = preparedStateClosed
	var runtimeErr error
	if p.runtime != nil {
		runtimeErr = p.runtime.Close()
		p.runtime = nil
	}
	managedDataErr := releaseManagedDataLifetime(p.managedData)
	p.managedData = nil
	snapshotLeaseErr := p.snapshotLease.Close()
	p.snapshotLease = nil
	return errors.Join(runtimeErr, managedDataErr, snapshotLeaseErr)
}

func (p *Prepared) DuckLakeSnapshotID() int64 {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotID
}

func NewManagerWithFactory(options ManagerOptions) *Manager {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		repo:                  options.Repo,
		workspaceID:           options.WorkspaceID,
		environment:           servingstate.NormalizeEnvironment(options.Environment),
		factory:               options.Factory,
		managedData:           options.ManagedData,
		onDrained:             options.OnDrained,
		leaseTTL:              normalizedLeaseTTL(options.LeaseTTL),
		logger:                logger,
		onLeaseRenewalFailure: options.OnLeaseRenewalFailure,
		onCleanupFailure:      options.OnCleanupFailure,
		leaseRenewalErrors:    map[string]error{},
		leaseOwner:            firstNonEmpty(options.LeaseOwner, "runtimehost"),
	}
}

func (m *Manager) LeaseRenewalError() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	errs := make([]error, 0, len(m.leaseRenewalErrors))
	for _, err := range m.leaseRenewalErrors {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (m *Manager) setLeaseRenewalError(leaseID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		delete(m.leaseRenewalErrors, leaseID)
		return
	}
	m.leaseRenewalErrors[leaseID] = err
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
	// The validated artifact is immutable and its digest includes the managed-data
	// revision pins. Avoid reconstructing and verifying those revisions on every
	// runtime acquisition when the active artifact has not changed.
	if !m.needsArtifactPrepare(current, artifact) {
		return nil
	}
	managedData, err := m.resolveManagedData(ctx, current.ID)
	if err != nil {
		return err
	}
	if !m.needsPrepare(current, artifact, managedData.RevisionID) {
		return nil
	}
	if beforePrepare != nil && current.DuckLakeSnapshotID == 0 {
		if err := beforePrepare(); err != nil {
			return err
		}
	}
	prepared, err := m.prepareResolved(ctx, current, artifact, managedData)
	if err != nil {
		return err
	}
	if current.DuckLakeSnapshotID == 0 && prepared.DuckLakeSnapshotID() > 0 {
		if err := m.repo.RecordDuckLakeSnapshot(ctx, current.ID, prepared.DuckLakeSnapshotID()); err != nil {
			_ = prepared.Close()
			return err
		}
	}
	return m.PublishPrepared(prepared)
}

func (m *Manager) needsArtifactPrepare(current servingstate.State, artifact servingstate.Artifact) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current == nil ||
		m.activeServingStateID != current.ID ||
		m.activeDigest != artifact.Digest ||
		m.activeSnapshotID != current.DuckLakeSnapshotID
}

func (m *Manager) needsPrepare(current servingstate.State, artifact servingstate.Artifact, managedRevision string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current == nil ||
		m.activeServingStateID != current.ID ||
		m.activeDigest != artifact.Digest ||
		m.activeManagedRevision != managedRevision ||
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
	managedData, err := m.resolveManagedData(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	return m.prepareResolved(ctx, current, artifact, managedData)
}

func (m *Manager) resolveManagedData(ctx context.Context, servingStateID servingstate.ID) (ManagedDataResolution, error) {
	if m.managedData == nil {
		return ManagedDataResolution{}, nil
	}
	return m.managedData.ResolveManagedData(ctx, servingStateID)
}

func (m *Manager) prepareResolved(ctx context.Context, current servingstate.State, artifact servingstate.Artifact, managedData ManagedDataResolution) (*Prepared, error) {
	m.mu.RLock()
	if m.current != nil && m.activeServingStateID == current.ID && m.activeDigest == artifact.Digest && m.activeManagedRevision == managedData.RevisionID && m.activeSnapshotID == current.DuckLakeSnapshotID {
		m.mu.RUnlock()
		if err := releaseManagedDataLifetime(managedData.Lifetime); err != nil {
			return nil, err
		}
		return &Prepared{owner: m, servingStateID: current.ID, digest: artifact.Digest, managedRevision: managedData.RevisionID, noChange: true}, nil
	}
	m.mu.RUnlock()
	factoryManagedData := managedData
	factoryManagedData.Lifetime = nil
	runtime, err := m.factory.Prepare(ctx, RuntimeInput{State: current, Artifact: artifact, ManagedData: factoryManagedData})
	if err != nil {
		return nil, errors.Join(err, releaseManagedDataLifetime(managedData.Lifetime))
	}
	if runtime == nil {
		return nil, errors.Join(errors.New("runtime factory returned nil"), releaseManagedDataLifetime(managedData.Lifetime))
	}
	var snapshotID int64
	if snapshot, ok := runtime.(RuntimeSnapshot); ok {
		snapshotID = snapshot.DuckLakeSnapshotID()
	}
	if snapshotID == 0 {
		snapshotID = current.DuckLakeSnapshotID
	}
	snapshotLease, err := m.createPersistentLease(ctx, current.ID, snapshotID)
	if err != nil {
		return nil, errors.Join(err, runtime.Close(), releaseManagedDataLifetime(managedData.Lifetime))
	}
	return &Prepared{
		owner: m,
		servingStateID: current.ID, digest: artifact.Digest, managedRevision: managedData.RevisionID,
		runtime: runtime, managedData: managedData.Lifetime, snapshotLease: snapshotLease, snapshotID: snapshotID,
	}, nil
}

// PublishPrepared publishes an already-durable serving state. Cleanup of the
// retired generation is observed separately and cannot fail publication.
func (m *Manager) PublishPrepared(candidate servingstate.PreparedRuntime) error {
	sealed, err := m.sealPrepared(candidate)
	if err != nil {
		return err
	}
	retired := sealed.publish()
	m.cleanupRetired(retired)
	return nil
}

type sealedPrepared struct {
	manager         *Manager
	source          *Prepared
	servingStateID  servingstate.ID
	digest          string
	managedRevision string
	runtime         Runtime
	managedData     ManagedDataLifetime
	snapshotLease   *persistentSnapshotLease
	snapshotID      int64
	noChange        bool
}

func (m *Manager) sealPrepared(candidate servingstate.PreparedRuntime) (*sealedPrepared, error) {
	prepared, ok := candidate.(*Prepared)
	if !ok || prepared == nil {
		return nil, fmt.Errorf("prepared runtime belongs to a different host")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.owner != m {
		return nil, fmt.Errorf("prepared runtime belongs to a different host")
	}
	if prepared.state != preparedStateOpen {
		return nil, fmt.Errorf("prepared runtime is already consumed")
	}
	if !prepared.noChange && prepared.runtime == nil {
		return nil, fmt.Errorf("prepared runtime is incomplete")
	}
	sealed := &sealedPrepared{
		manager: m, source: prepared,
		servingStateID: prepared.servingStateID, digest: prepared.digest, managedRevision: prepared.managedRevision,
		runtime: prepared.runtime, managedData: prepared.managedData, snapshotLease: prepared.snapshotLease,
		snapshotID: prepared.snapshotID, noChange: prepared.noChange,
	}
	prepared.runtime = nil
	prepared.managedData = nil
	prepared.snapshotLease = nil
	prepared.state = preparedStateSealed
	return sealed, nil
}

func (p *sealedPrepared) publish() *managedRuntime {
	if p == nil {
		return nil
	}
	if p.noChange {
		p.finish(preparedStatePublished)
		return nil
	}
	managed := &managedRuntime{
		servingStateID: p.servingStateID,
		digest:         p.digest,
		runtime:        p.runtime,
		managedData:    p.managedData,
		snapshotLease:  p.snapshotLease,
		snapshotID:     p.snapshotID,
	}
	p.manager.mu.Lock()
	old := p.manager.current
	p.manager.current = managed
	p.manager.activeServingStateID = p.servingStateID
	p.manager.activeDigest = p.digest
	p.manager.activeManagedRevision = p.managedRevision
	p.manager.activeSnapshotID = p.snapshotID
	retired := p.manager.retireLocked(old)
	p.manager.mu.Unlock()
	p.runtime = nil
	p.managedData = nil
	p.snapshotLease = nil
	p.finish(preparedStatePublished)
	return retired
}

func (p *sealedPrepared) abort() error {
	if p == nil {
		return nil
	}
	managed := &managedRuntime{
		servingStateID: p.servingStateID, runtime: p.runtime, managedData: p.managedData,
		snapshotLease: p.snapshotLease, snapshotID: p.snapshotID,
	}
	p.runtime = nil
	p.managedData = nil
	p.snapshotLease = nil
	p.finish(preparedStateClosed)
	return p.manager.closeManaged(managed)
}

func (p *sealedPrepared) finish(state preparedState) {
	if p == nil || p.source == nil {
		return
	}
	p.source.mu.Lock()
	p.source.state = state
	p.source.mu.Unlock()
}

func (m *Manager) Close() error {
	m.mu.Lock()
	current := m.current
	m.current = nil
	m.activeServingStateID = ""
	m.activeDigest = ""
	m.activeManagedRevision = ""
	m.activeSnapshotID = 0
	currentToClose := m.retireLocked(current)
	m.mu.Unlock()
	if currentToClose == nil {
		return nil
	}
	return m.closeManaged(currentToClose)
}

func (m *Manager) Acquire() (Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil || m.current.closing {
		return nil, fmt.Errorf("no active LeapView serving state")
	}
	m.current.refs++
	return &runtimeLease{manager: m, managed: m.current}, nil
}

func (m *Manager) LeasedSnapshots() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshots := map[int64]struct{}{}
	if m.current != nil && m.current.snapshotLease != nil && m.current.snapshotID > 0 {
		snapshots[m.current.snapshotID] = struct{}{}
	}
	for _, runtime := range m.retired {
		if runtime.snapshotLease != nil && runtime.snapshotID > 0 {
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

func (m *Manager) release(runtime *managedRuntime) {
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
	m.cleanupRetired(drained)
}

func releaseSnapshotLease(repo SnapshotLeaseRepository, leaseID string) error {
	if repo == nil || leaseID == "" {
		return nil
	}
	delay := 25 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := repo.ReleaseQuerySnapshotLease(ctx, leaseID)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(delay)
		delay *= 2
	}
	return lastErr
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
	results := m.closeManagedResources(runtime)
	errs := make([]error, 0, len(results))
	for _, result := range results {
		errs = append(errs, result.err)
	}
	return errors.Join(errs...)
}

type cleanupResult struct {
	resource CleanupResource
	err      error
}

func (m *Manager) closeManagedResources(runtime *managedRuntime) []cleanupResult {
	if runtime == nil {
		return nil
	}
	results := make([]cleanupResult, 0, 3)
	if runtime.runtime != nil {
		if err := runtime.runtime.Close(); err != nil {
			results = append(results, cleanupResult{resource: CleanupResourceRuntime, err: err})
		}
		runtime.runtime = nil
	}
	if err := releaseManagedDataLifetime(runtime.managedData); err != nil {
		results = append(results, cleanupResult{resource: CleanupResourceManagedData, err: err})
	}
	runtime.managedData = nil
	if err := runtime.snapshotLease.Close(); err != nil {
		results = append(results, cleanupResult{resource: CleanupResourceSnapshotLease, err: err})
	}
	runtime.snapshotLease = nil
	if runtime.closing && m.onDrained != nil {
		m.onDrained(runtime.servingStateID, runtime.snapshotID)
	}
	return results
}

func (m *Manager) cleanupRetired(runtime *managedRuntime) {
	if runtime == nil {
		return
	}
	for _, result := range m.closeManagedResources(runtime) {
		failure := CleanupFailure{
			WorkspaceID: m.workspaceID, ServingStateID: runtime.servingStateID,
			DuckLakeSnapshotID: runtime.snapshotID, Resource: result.resource, Err: result.err,
		}
		m.logger.Error("retired runtime cleanup failed",
			"workspace_id", failure.WorkspaceID, "serving_state_id", failure.ServingStateID,
			"ducklake_snapshot_id", failure.DuckLakeSnapshotID, "resource", failure.Resource, "error", failure.Err)
		if m.onCleanupFailure != nil {
			m.onCleanupFailure(failure)
		}
	}
}

type managedRuntime struct {
	servingStateID servingstate.ID
	digest         string
	runtime        Runtime
	managedData    ManagedDataLifetime
	snapshotLease  *persistentSnapshotLease
	snapshotID     int64
	refs           int
	closing        bool
}

func releaseManagedDataLifetime(lifetime ManagedDataLifetime) error {
	if lifetime == nil {
		return nil
	}
	return lifetime.Release()
}

type runtimeLease struct {
	manager *Manager
	managed *managedRuntime
	once    sync.Once
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
		l.manager.release(l.managed)
	})
}

type persistentSnapshotLease struct {
	repo   SnapshotLeaseRepository
	id     string
	cancel context.CancelFunc
	once   sync.Once
	err    error
}

func (l *persistentSnapshotLease) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.cancel != nil {
			l.cancel()
		}
		l.err = releaseSnapshotLease(l.repo, l.id)
	})
	return l.err
}

func (m *Manager) createPersistentLease(ctx context.Context, servingStateID servingstate.ID, snapshotID int64) (*persistentSnapshotLease, error) {
	repo, ok := m.repo.(SnapshotLeaseRepository)
	if !ok || snapshotID <= 0 {
		return nil, nil
	}
	expiresAt := time.Now().Add(m.leaseTTL)
	leaseID, err := repo.CreateQuerySnapshotLease(ctx, servingstate.SnapshotLeaseInput{
		WorkspaceID:        m.workspaceID,
		Environment:        m.environment,
		ServingStateID:     servingStateID,
		DuckLakeSnapshotID: snapshotID,
		OwnerID:            m.leaseOwner,
		ExpiresAt:          expiresAt,
	})
	if err != nil {
		return nil, err
	}
	heartbeatCtx, cancel := context.WithCancel(context.Background())
	go m.heartbeatLease(heartbeatCtx, repo, leaseID)
	return &persistentSnapshotLease{repo: repo, id: leaseID, cancel: cancel}, nil
}

func (m *Manager) heartbeatLease(ctx context.Context, repo SnapshotLeaseRepository, leaseID string) {
	defer m.setLeaseRenewalError(leaseID, nil)
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
			err := renewSnapshotLease(ctx, repo, leaseID, time.Now().Add(m.leaseTTL), 3, 100*time.Millisecond)
			m.setLeaseRenewalError(leaseID, err)
			if err != nil {
				m.logger.Error("snapshot lease renewal failed", "lease_id", leaseID, "workspace_id", m.workspaceID, "error", err)
				if m.onLeaseRenewalFailure != nil {
					m.onLeaseRenewalFailure(err)
				}
			}
		}
	}
}

func renewSnapshotLease(ctx context.Context, repo SnapshotLeaseRepository, leaseID string, expiresAt time.Time, attempts int, backoff time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		lastErr = repo.ExtendQuerySnapshotLease(requestCtx, leaseID, expiresAt)
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt == attempts-1 {
			break
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
	}
	return fmt.Errorf("extend snapshot lease %q after %d attempts: %w", leaseID, attempts, lastErr)
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
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
