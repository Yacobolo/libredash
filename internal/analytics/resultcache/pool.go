// Package resultcache owns node-wide retention and coalescing for governed
// analytical query results.
package resultcache

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Yacobolo/leapview/internal/dataquery"
	"golang.org/x/sync/singleflight"
)

type Constraint string

const (
	ConstraintRuntime   Constraint = "runtime"
	ConstraintWorkspace Constraint = "workspace"
	ConstraintNode      Constraint = "node"
)

type StoreOutcome string

const (
	StoreStored    StoreOutcome = "stored"
	StoreOversized StoreOutcome = "oversized"
	StoreStale     StoreOutcome = "stale"
	StoreClosed    StoreOutcome = "closed"
)

type Limits struct {
	RuntimeEntries   int
	RuntimeBytes     int64
	WorkspaceEntries int
	WorkspaceBytes   int64
	NodeEntries      int
	NodeBytes        int64
}

func (l Limits) Validate() error {
	if l.RuntimeEntries <= 0 || l.WorkspaceEntries <= 0 || l.NodeEntries <= 0 || l.RuntimeBytes <= 0 || l.WorkspaceBytes <= 0 || l.NodeBytes <= 0 {
		return fmt.Errorf("query cache limits must be positive")
	}
	if l.RuntimeEntries > l.WorkspaceEntries || l.WorkspaceEntries > l.NodeEntries {
		return fmt.Errorf("query cache entry limits must satisfy runtime <= workspace <= node")
	}
	if l.RuntimeBytes > l.WorkspaceBytes || l.WorkspaceBytes > l.NodeBytes {
		return fmt.Errorf("query cache byte limits must satisfy runtime <= workspace <= node")
	}
	return nil
}

type ScopeID struct{ WorkspaceID, RuntimeID string }
type Token uint64

type Pool struct {
	mu         sync.Mutex
	limits     Limits
	closed     bool
	entries    map[string]*list.Element
	lru        *list.List
	scopes     map[string]*scopeState
	workspaces map[string]*usage
	bytes      int64
	evictions  map[Constraint]uint64
	stores     map[StoreOutcome]uint64
	group      singleflight.Group
}

type Scope struct {
	pool *Pool
	key  string
}
type scopeState struct {
	id         ScopeID
	generation Token
	closed     bool
	entries    map[string]struct{}
	usage      usage
}
type usage struct {
	entries int
	bytes   int64
}
type entry struct {
	composite, key, scope string
	result                dataquery.Result
	bytes                 int64
}

type UsageSnapshot struct {
	Entries int
	Bytes   int64
}
type ScopeSnapshot struct {
	ScopeID
	Entries    int
	Bytes      int64
	Generation Token
}

func (s *Scope) Stats() UsageSnapshot {
	if s == nil || s.pool == nil {
		return UsageSnapshot{}
	}
	s.pool.mu.Lock()
	defer s.pool.mu.Unlock()
	if state := s.pool.scopes[s.key]; state != nil {
		return UsageSnapshot{Entries: state.usage.entries, Bytes: state.usage.bytes}
	}
	return UsageSnapshot{}
}

type Snapshot struct {
	Entries    int
	Bytes      int64
	Workspaces map[string]UsageSnapshot
	Scopes     map[string]ScopeSnapshot
	Evictions  map[Constraint]uint64
	Stores     map[StoreOutcome]uint64
}

func New(limits Limits) (*Pool, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return &Pool{limits: limits, entries: map[string]*list.Element{}, lru: list.New(), scopes: map[string]*scopeState{}, workspaces: map[string]*usage{}, evictions: map[Constraint]uint64{}, stores: map[StoreOutcome]uint64{}}, nil
}

func (p *Pool) OpenScope(id ScopeID) (*Scope, error) {
	if p == nil {
		return nil, fmt.Errorf("result cache pool is required")
	}
	if id.WorkspaceID == "" || id.RuntimeID == "" {
		return nil, fmt.Errorf("result cache workspace and runtime IDs are required")
	}
	key := id.WorkspaceID + "\x00" + id.RuntimeID
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("result cache pool is closed")
	}
	if existing := p.scopes[key]; existing != nil && !existing.closed {
		return nil, fmt.Errorf("result cache scope already exists")
	}
	p.scopes[key] = &scopeState{id: id, entries: map[string]struct{}{}}
	return &Scope{pool: p, key: key}, nil
}

func (s *Scope) Generation() Token {
	if s == nil || s.pool == nil {
		return 0
	}
	s.pool.mu.Lock()
	defer s.pool.mu.Unlock()
	if state := s.pool.scopes[s.key]; state != nil {
		return state.generation
	}
	return 0
}

func (s *Scope) Lookup(key string) (dataquery.Result, Token, bool, error) {
	if s == nil || s.pool == nil {
		return dataquery.Result{}, 0, false, fmt.Errorf("result cache scope is required")
	}
	p := s.pool
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.scopes[s.key]
	if state == nil || state.closed {
		return dataquery.Result{}, 0, false, fmt.Errorf("result cache scope is closed")
	}
	element := p.entries[s.key+"\x00"+key]
	if element == nil {
		return dataquery.Result{}, state.generation, false, nil
	}
	p.lru.MoveToFront(element)
	result := cloneResult(element.Value.(entry).result)
	result.CacheOutcome = dataquery.CacheHit
	result.QueueWaitMS, result.PlanningMS, result.ConnectionWaitMS, result.DatabaseMS, result.ExecutionMS = 0, 0, 0, 0, 0
	return result, state.generation, true, nil
}

func (s *Scope) Store(key string, token Token, result dataquery.Result) StoreOutcome {
	if s == nil || s.pool == nil {
		return StoreClosed
	}
	p := s.pool
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.scopes[s.key]
	if p.closed || state == nil || state.closed {
		p.stores[StoreClosed]++
		return StoreClosed
	}
	if token != state.generation {
		p.stores[StoreStale]++
		return StoreStale
	}
	bytes := int64(len(key)) + EstimateResultBytes(result)
	if bytes > p.limits.RuntimeBytes || bytes > p.limits.WorkspaceBytes || bytes > p.limits.NodeBytes {
		p.stores[StoreOversized]++
		return StoreOversized
	}
	composite := s.key + "\x00" + key
	if old := p.entries[composite]; old != nil {
		p.removeLocked(old, "")
	}
	e := entry{composite: composite, key: key, scope: s.key, result: cloneResult(result), bytes: bytes}
	element := p.lru.PushFront(e)
	p.entries[composite] = element
	state.entries[composite] = struct{}{}
	state.usage.entries++
	state.usage.bytes += bytes
	workspace := p.workspaceLocked(state.id.WorkspaceID)
	workspace.entries++
	workspace.bytes += bytes
	p.bytes += bytes
	p.enforceLocked(state)
	p.stores[StoreStored]++
	return StoreStored
}

func (s *Scope) Delete(key string) {
	if s == nil || s.pool == nil {
		return
	}
	p := s.pool
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.scopes[s.key]
	if state == nil || state.closed {
		return
	}
	p.removeLocked(p.entries[s.key+"\x00"+key], "")
}

func (p *Pool) enforceLocked(state *scopeState) {
	for state.usage.entries > p.limits.RuntimeEntries || state.usage.bytes > p.limits.RuntimeBytes {
		p.removeLocked(p.oldestLocked(func(e entry) bool { return e.scope == scopeKey(state.id) }), ConstraintRuntime)
	}
	workspace := p.workspaceLocked(state.id.WorkspaceID)
	for workspace.entries > p.limits.WorkspaceEntries || workspace.bytes > p.limits.WorkspaceBytes {
		p.removeLocked(p.oldestLocked(func(e entry) bool { return p.scopes[e.scope].id.WorkspaceID == state.id.WorkspaceID }), ConstraintWorkspace)
	}
	for len(p.entries) > p.limits.NodeEntries || p.bytes > p.limits.NodeBytes {
		p.removeLocked(p.lru.Back(), ConstraintNode)
	}
}

func (p *Pool) oldestLocked(match func(entry) bool) *list.Element {
	for e := p.lru.Back(); e != nil; e = e.Prev() {
		if match(e.Value.(entry)) {
			return e
		}
	}
	return nil
}
func (p *Pool) removeLocked(element *list.Element, constraint Constraint) {
	if element == nil {
		return
	}
	e := element.Value.(entry)
	state := p.scopes[e.scope]
	delete(p.entries, e.composite)
	p.lru.Remove(element)
	p.bytes -= e.bytes
	if state != nil {
		delete(state.entries, e.composite)
		state.usage.entries--
		state.usage.bytes -= e.bytes
		ws := p.workspaceLocked(state.id.WorkspaceID)
		ws.entries--
		ws.bytes -= e.bytes
	}
	if constraint != "" {
		p.evictions[constraint]++
	}
}
func (p *Pool) workspaceLocked(id string) *usage {
	if p.workspaces[id] == nil {
		p.workspaces[id] = &usage{}
	}
	return p.workspaces[id]
}
func scopeKey(id ScopeID) string { return id.WorkspaceID + "\x00" + id.RuntimeID }

func (s *Scope) Invalidate() {
	if s == nil || s.pool == nil {
		return
	}
	p := s.pool
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.scopes[s.key]
	if state == nil || state.closed {
		return
	}
	for composite := range state.entries {
		p.removeLocked(p.entries[composite], "")
	}
	state.generation++
}

func (s *Scope) Close() error {
	if s == nil || s.pool == nil {
		return nil
	}
	p := s.pool
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.scopes[s.key]
	if state == nil || state.closed {
		return nil
	}
	for composite := range state.entries {
		p.removeLocked(p.entries[composite], "")
	}
	state.closed = true
	state.generation++
	delete(p.scopes, s.key)
	return nil
}

type canceledFlight struct{ err error }

func (e canceledFlight) Error() string { return e.err.Error() }
func (e canceledFlight) Unwrap() error { return e.err }

// OwnerCanceled marks a coalesced execution whose owning context was canceled,
// allowing a still-live waiter to replace that flight.
func OwnerCanceled(err error) error { return canceledFlight{err: err} }

func (s *Scope) Coalesce(ctx context.Context, key string, execute func() (any, error)) (any, bool, error) {
	if s == nil || s.pool == nil {
		return nil, false, fmt.Errorf("result cache scope is required")
	}
	flightKey := s.key + "\x00" + key
	for {
		ch := s.pool.group.DoChan(flightKey, func() (any, error) {
			value, err := execute()
			if ownerErr := ctx.Err(); ownerErr != nil {
				return nil, canceledFlight{ownerErr}
			}
			return value, err
		})
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case call := <-ch:
			if call.Err != nil {
				var canceled canceledFlight
				if ctx.Err() == nil && errors.As(call.Err, &canceled) {
					continue
				}
				return nil, call.Shared, call.Err
			}
			return call.Val, call.Shared, nil
		}
	}
}

func (p *Pool) Stats() Snapshot {
	if p == nil {
		return Snapshot{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	result := Snapshot{Entries: len(p.entries), Bytes: p.bytes, Workspaces: map[string]UsageSnapshot{}, Scopes: map[string]ScopeSnapshot{}, Evictions: map[Constraint]uint64{}, Stores: map[StoreOutcome]uint64{}}
	for id, u := range p.workspaces {
		if u.entries != 0 || u.bytes != 0 {
			result.Workspaces[id] = UsageSnapshot{Entries: u.entries, Bytes: u.bytes}
		}
	}
	for key, state := range p.scopes {
		result.Scopes[key] = ScopeSnapshot{ScopeID: state.id, Entries: state.usage.entries, Bytes: state.usage.bytes, Generation: state.generation}
	}
	for key, value := range p.evictions {
		result.Evictions[key] = value
	}
	for key, value := range p.stores {
		result.Stores[key] = value
	}
	return result
}

func (p *Pool) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	for e := p.lru.Back(); e != nil; {
		previous := e.Prev()
		p.removeLocked(e, "")
		e = previous
	}
	for _, state := range p.scopes {
		state.closed = true
		state.generation++
	}
	p.scopes = map[string]*scopeState{}
	return nil
}

func EstimateResultBytes(result dataquery.Result) int64 {
	size := int64(len(result.SQL)+len(result.PlanText)+len(result.Error)+len(result.ExecutionState)+len(result.CacheOutcome)+len(result.Status)) + 128
	for _, column := range result.Columns {
		size += int64(len(column.Name)) + 16
	}
	for _, warning := range result.Warnings {
		size += int64(len(warning)) + 16
	}
	for _, row := range result.Rows {
		size += 48
		for key, value := range row {
			size += int64(len(key)) + estimateValue(value)
		}
	}
	return size
}

func CloneResult(result dataquery.Result) dataquery.Result { return cloneResult(result) }
func estimateValue(value any) int64 {
	switch v := value.(type) {
	case nil:
		return 1
	case string:
		return int64(len(v)) + 16
	case []byte:
		return int64(len(v)) + 24
	case []string:
		n := int64(24)
		for _, x := range v {
			n += int64(len(x)) + 16
		}
		return n
	case []any:
		n := int64(24)
		for _, x := range v {
			n += estimateValue(x)
		}
		return n
	case map[string]any:
		n := int64(48)
		for k, x := range v {
			n += int64(len(k)) + estimateValue(x)
		}
		return n
	case bool:
		return 1
	default:
		return 16
	}
}
func cloneResult(result dataquery.Result) dataquery.Result {
	clone := result
	clone.Columns = append([]dataquery.Column{}, result.Columns...)
	clone.Rows = make([]dataquery.Row, len(result.Rows))
	for i, row := range result.Rows {
		clone.Rows[i] = make(dataquery.Row, len(row))
		for k, v := range row {
			clone.Rows[i][k] = cloneValue(v)
		}
	}
	clone.Warnings = append([]string{}, result.Warnings...)
	return clone
}
func cloneValue(value any) any {
	switch v := value.(type) {
	case []byte:
		return append([]byte{}, v...)
	case []string:
		return append([]string{}, v...)
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = cloneValue(x)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = cloneValue(x)
		}
		return out
	default:
		return value
	}
}
