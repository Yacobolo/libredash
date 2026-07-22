package resultcache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Yacobolo/leapview/internal/dataquery"
)

func TestPoolEnforcesRuntimeWorkspaceAndNodeBudgets(t *testing.T) {
	pool, err := New(Limits{RuntimeEntries: 2, RuntimeBytes: 1 << 20, WorkspaceEntries: 3, WorkspaceBytes: 1 << 20, NodeEntries: 4, NodeBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	a1 := mustScope(t, pool, ScopeID{WorkspaceID: "a", RuntimeID: "a1"})
	a2 := mustScope(t, pool, ScopeID{WorkspaceID: "a", RuntimeID: "a2"})
	b1 := mustScope(t, pool, ScopeID{WorkspaceID: "b", RuntimeID: "b1"})

	put(t, a1, "a1-1", "one")
	put(t, a1, "a1-2", "two")
	put(t, a1, "a1-3", "three")
	assertMiss(t, a1, "a1-1")

	put(t, a2, "a2-1", "four")
	put(t, a2, "a2-2", "five")
	if got := pool.Stats().Workspaces["a"].Entries; got != 3 {
		t.Fatalf("workspace entries = %d, want 3", got)
	}

	put(t, b1, "b1-1", "six")
	put(t, b1, "b1-2", "seven")
	if got := pool.Stats().Entries; got != 4 {
		t.Fatalf("node entries = %d, want 4", got)
	}
	if pool.Stats().Evictions[ConstraintRuntime] == 0 || pool.Stats().Evictions[ConstraintWorkspace] == 0 || pool.Stats().Evictions[ConstraintNode] == 0 {
		t.Fatalf("evictions = %#v", pool.Stats().Evictions)
	}
}

func TestScopeInvalidationPreventsStaleStoreAndCloseBalancesAccounting(t *testing.T) {
	pool, _ := New(testLimits())
	scope := mustScope(t, pool, ScopeID{WorkspaceID: "a", RuntimeID: "one"})
	token := scope.Generation()
	scope.Invalidate()
	if outcome := scope.Store("stale", token, result("stale")); outcome != StoreStale {
		t.Fatalf("outcome = %q", outcome)
	}
	put(t, scope, "live", "live")
	if err := scope.Close(); err != nil {
		t.Fatal(err)
	}
	if stats := pool.Stats(); stats.Entries != 0 || stats.Bytes != 0 {
		t.Fatalf("stats after close = %#v", stats)
	}
	if outcome := scope.Store("closed", scope.Generation(), result("closed")); outcome != StoreClosed {
		t.Fatalf("outcome = %q", outcome)
	}
}

func TestOversizedEntryIsSkipped(t *testing.T) {
	pool, _ := New(Limits{RuntimeEntries: 2, RuntimeBytes: 64, WorkspaceEntries: 2, WorkspaceBytes: 64, NodeEntries: 2, NodeBytes: 64})
	scope := mustScope(t, pool, ScopeID{WorkspaceID: "a", RuntimeID: "one"})
	if outcome := scope.Store("large", scope.Generation(), result(string(make([]byte, 256)))); outcome != StoreOversized {
		t.Fatalf("outcome = %q", outcome)
	}
	if pool.Stats().Entries != 0 {
		t.Fatal("oversized entry was retained")
	}
}

func TestCoalesceCancellationDoesNotPoisonLiveWaiter(t *testing.T) {
	pool, _ := New(testLimits())
	scope := mustScope(t, pool, ScopeID{WorkspaceID: "a", RuntimeID: "one"})
	owner, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	go func() {
		_, _, _ = scope.Coalesce(owner, "key", func() (any, error) {
			calls.Add(1)
			close(started)
			<-release
			return "owner", owner.Err()
		})
	}()
	<-started
	cancel()
	close(release)
	value, _, err := scope.Coalesce(context.Background(), "key", func() (any, error) { calls.Add(1); return "live", nil })
	if err != nil || value != "live" {
		t.Fatalf("value=%v err=%v", value, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestPoolConcurrentStatsInvalidateAndClose(t *testing.T) {
	pool, _ := New(testLimits())
	scope := mustScope(t, pool, ScopeID{WorkspaceID: "a", RuntimeID: "one"})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = pool.Stats()
			scope.Invalidate()
			scope.Store("key", scope.Generation(), result("value"))
		}()
	}
	wg.Wait()
	if err := scope.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
}

func testLimits() Limits {
	return Limits{RuntimeEntries: 8, RuntimeBytes: 1 << 20, WorkspaceEntries: 16, WorkspaceBytes: 2 << 20, NodeEntries: 32, NodeBytes: 4 << 20}
}
func mustScope(t *testing.T, p *Pool, id ScopeID) *Scope {
	t.Helper()
	s, err := p.OpenScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func result(value string) dataquery.Result {
	return dataquery.Result{Rows: []dataquery.Row{{"value": value}}}
}
func put(t *testing.T, s *Scope, key, value string) {
	t.Helper()
	if got := s.Store(key, s.Generation(), result(value)); got != StoreStored {
		t.Fatalf("store %q = %q", key, got)
	}
}
func assertMiss(t *testing.T, s *Scope, key string) {
	t.Helper()
	if _, _, ok, err := s.Lookup(key); err != nil || ok {
		t.Fatalf("lookup %q ok=%v err=%v", key, ok, err)
	}
}
