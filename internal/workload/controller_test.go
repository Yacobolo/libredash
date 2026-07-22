package workload

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"negative node limit", func(c *Config) { c.MaxRunning = -1 }},
		{"reservation above maximum", func(c *Config) {
			p := c.Classes[Interactive]
			p.ReservedRunning = p.MaximumRunning + 1
			c.Classes[Interactive] = p
		}},
		{"class maximum above node", func(c *Config) {
			p := c.Classes[Interactive]
			p.MaximumRunning = c.MaxRunning + 1
			c.Classes[Interactive] = p
		}},
		{"reservations above node", func(c *Config) {
			p := c.Classes[Background]
			p.ReservedRunning = p.MaximumRunning
			c.Classes[Background] = p
			c.MaxRunning = 3
		}},
		{"negative queue", func(c *Config) { p := c.Classes[Interactive]; p.MaximumQueued = -1; c.Classes[Interactive] = p }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.edit(&cfg)
			if _, err := New(cfg); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}
}

func TestReservedDemandReceivesNextCapacityBeforeBorrowing(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 2, MaximumQueued: 8, Classes: map[Class]Policy{
		Interactive: {ReservedRunning: 1, MaximumRunning: 2, MaximumQueued: 8, MaximumQueuedPerWorkspace: 8},
		Refresh:     {ReservedRunning: 1, MaximumRunning: 1, MaximumQueued: 8, MaximumQueuedPerWorkspace: 8},
	}})
	first := acquire(t, c, Interactive, "one")
	second := acquire(t, c, Interactive, "two")

	interactiveGranted := acquireAsync(c, Interactive, "three")
	refreshGranted := acquireAsync(c, Refresh, "refresh")
	waitQueued(t, c, 2)
	first.Release()
	refresh := receiveLease(t, refreshGranted)
	assertNotGranted(t, interactiveGranted)
	refresh.Release()
	second.Release()
	receiveLease(t, interactiveGranted).Release()
}

func TestClassRoundRobinPreventsBorrowerStarvation(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 1, MaximumQueued: 8, Classes: map[Class]Policy{
		Interactive: {MaximumRunning: 1, MaximumQueued: 8, MaximumQueuedPerWorkspace: 8},
		Background:  {MaximumRunning: 1, MaximumQueued: 8, MaximumQueuedPerWorkspace: 8},
	}})
	running := acquire(t, c, Interactive, "running")
	i1 := acquireAsync(c, Interactive, "i1")
	waitQueued(t, c, 1)
	b1 := acquireAsync(c, Background, "b1")
	waitQueued(t, c, 2)
	i2 := acquireAsync(c, Interactive, "i2")
	waitQueued(t, c, 3)
	running.Release()

	first := receiveLease(t, b1)
	assertNotGranted(t, i1)
	first.Release()
	receiveLease(t, i1).Release()
	receiveLease(t, i2).Release()
}

func TestWorkspaceRoundRobinAndFIFO(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 1, MaximumQueued: 8, Classes: map[Class]Policy{
		Interactive: {MaximumRunning: 1, MaximumQueued: 8, MaximumQueuedPerWorkspace: 8},
	}})
	running := acquire(t, c, Interactive, "a")
	a1 := acquireAsyncOperation(c, Interactive, "a", "a1")
	waitQueued(t, c, 1)
	a2 := acquireAsyncOperation(c, Interactive, "a", "a2")
	waitQueued(t, c, 2)
	b1 := acquireAsyncOperation(c, Interactive, "b", "b1")
	waitQueued(t, c, 3)
	running.Release()

	first := receiveLease(t, a1)
	first.Release()
	second := receiveLease(t, b1)
	assertNotGranted(t, a2)
	second.Release()
	receiveLease(t, a2).Release()
}

func TestQueueLimitsExposeTypedRejections(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 1, MaximumQueued: 2, Classes: map[Class]Policy{
		Interactive: {MaximumRunning: 1, MaximumQueued: 1, MaximumQueuedPerWorkspace: 1},
		Background:  {MaximumRunning: 1, MaximumQueued: 2, MaximumQueuedPerWorkspace: 1},
	}})
	running := acquire(t, c, Interactive, "a")
	queued := acquireAsync(c, Interactive, "a")
	waitQueued(t, c, 1)

	_, err := c.Acquire(context.Background(), Request{Class: Interactive, WorkspaceID: "b", Operation: "query"})
	assertReason(t, err, ClassQueueFull)
	background := acquireAsync(c, Background, "b")
	waitQueued(t, c, 2)
	_, err = c.Acquire(context.Background(), Request{Class: Background, WorkspaceID: "c", Operation: "query"})
	assertReason(t, err, NodeQueueFull)

	running.Release()
	first := receiveAny(t, queued, background)
	first.lease.Release()
	receiveLease(t, first.other).Release()
}

func TestWorkspaceQueueLimit(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 1, MaximumQueued: 4, Classes: map[Class]Policy{
		Interactive: {MaximumRunning: 1, MaximumQueued: 4, MaximumQueuedPerWorkspace: 1},
	}})
	running := acquire(t, c, Interactive, "a")
	queued := acquireAsync(c, Interactive, "a")
	waitQueued(t, c, 1)
	_, err := c.Acquire(context.Background(), Request{Class: Interactive, WorkspaceID: "a", Operation: "query"})
	assertReason(t, err, WorkspaceQueueFull)
	running.Release()
	receiveLease(t, queued).Release()
}

func TestQueueTimeoutCancellationAndShutdownRemoveWaiters(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 1, MaximumQueued: 4, Classes: map[Class]Policy{
		Interactive: {MaximumRunning: 1, MaximumQueued: 4, MaximumQueuedPerWorkspace: 4, QueueTimeout: 20 * time.Millisecond},
	}})
	running := acquire(t, c, Interactive, "a")
	_, err := c.Acquire(context.Background(), Request{Class: Interactive, WorkspaceID: "b", Operation: "query"})
	assertReason(t, err, QueueTimeout)

	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() {
		_, err := c.Acquire(ctx, Request{Class: Interactive, WorkspaceID: "c", Operation: "query"})
		canceled <- err
	}()
	waitQueued(t, c, 1)
	cancel()
	if err := <-canceled; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	waitQueued(t, c, 0)

	shutdown := acquireAsync(c, Interactive, "d")
	waitQueued(t, c, 1)
	c.Close()
	assertReason(t, receiveError(t, shutdown), ControllerShutdown)
	select {
	case <-running.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel running lease")
	}
	running.Release()
}

func TestExecutionDeadlineAndIdempotentRelease(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 1, Classes: map[Class]Policy{
		Interactive: {MaximumRunning: 1, ExecutionTimeout: 20 * time.Millisecond},
	}})
	lease := acquire(t, c, Interactive, "a")
	select {
	case <-lease.Context().Done():
		if !errors.Is(lease.Context().Err(), context.DeadlineExceeded) {
			t.Fatalf("context error = %v", lease.Context().Err())
		}
	case <-time.After(time.Second):
		t.Fatal("execution context did not expire")
	}
	lease.Release()
	lease.Release()
	if got := c.Stats().Running; got != 0 {
		t.Fatalf("running = %d", got)
	}
}

func TestNestedAdmissionReuseAndConflict(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 1, Classes: map[Class]Policy{
		Interactive: {MaximumRunning: 1}, Background: {MaximumRunning: 1},
	}})
	outer := acquire(t, c, Interactive, "a")
	nested, err := c.Acquire(outer.Context(), Request{Class: Interactive, WorkspaceID: "a", Operation: "nested"})
	if err != nil {
		t.Fatal(err)
	}
	if nested.Context() != outer.Context() {
		t.Fatal("nested admission did not reuse execution context")
	}
	nested.Release()
	if c.Stats().Running != 1 {
		t.Fatal("nested release released parent permit")
	}
	_, err = c.Acquire(outer.Context(), Request{Class: Background, WorkspaceID: "a", Operation: "conflict"})
	assertReason(t, err, ConflictingNestedAdmission)
	outer.Release()
}

func TestStatisticsSnapshotsDoNotExposeControllerState(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 1, Classes: map[Class]Policy{Interactive: {MaximumRunning: 1}}})
	lease := acquire(t, c, Interactive, "sales")
	snapshot := c.Stats()
	class := snapshot.Classes[Interactive]
	class.Workspaces["sales"] = WorkspaceStats{Running: 99}
	snapshot.Classes[Interactive] = class
	if got := c.Stats().Classes[Interactive].Workspaces["sales"].Running; got != 1 {
		t.Fatalf("controller statistics mutated through snapshot: %d", got)
	}
	lease.Release()
}

func TestNodeScopedClassesNormalizeWorkspace(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 2, Classes: map[Class]Policy{
		Control: {MaximumRunning: 1}, Maintenance: {MaximumRunning: 1},
	}})
	for _, class := range []Class{Control, Maintenance} {
		lease, err := c.Acquire(context.Background(), Request{Class: class, WorkspaceID: "ignored", Operation: "node"})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := c.Stats().Classes[class].Workspaces[NodeWorkspace]; !ok {
			t.Fatalf("%s workspace was not normalized", class)
		}
		lease.Release()
	}
	if _, err := c.Acquire(context.Background(), Request{Class: Interactive, Operation: "query"}); err == nil {
		t.Fatal("workspace-scoped admission accepted an empty workspace")
	}
}

func TestCrossWorkspaceIdentityIsBackgroundOnly(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 2, Classes: map[Class]Policy{Interactive: {MaximumRunning: 1}, Background: {MaximumRunning: 1}}})
	lease, err := c.Acquire(context.Background(), Request{Class: Background, WorkspaceID: GlobalWorkspace, Operation: "agent.run"})
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	if _, err := c.Acquire(context.Background(), Request{Class: Interactive, WorkspaceID: GlobalWorkspace, Operation: "query"}); err == nil {
		t.Fatal("interactive work accepted the global agent identity")
	}
}

type asyncResult struct {
	lease Lease
	err   error
}

func newTestController(t *testing.T, cfg Config) *Controller {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func acquire(t *testing.T, c *Controller, class Class, workspace string) Lease {
	t.Helper()
	lease, err := c.Acquire(context.Background(), Request{Class: class, WorkspaceID: workspace, Operation: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return lease
}

func acquireAsync(c *Controller, class Class, workspace string) <-chan asyncResult {
	return acquireAsyncOperation(c, class, workspace, "test")
}

func acquireAsyncOperation(c *Controller, class Class, workspace, operation string) <-chan asyncResult {
	ch := make(chan asyncResult, 1)
	go func() {
		lease, err := c.Acquire(context.Background(), Request{Class: class, WorkspaceID: workspace, Operation: operation})
		ch <- asyncResult{lease, err}
	}()
	return ch
}

func receiveLease(t *testing.T, ch <-chan asyncResult) Lease {
	t.Helper()
	select {
	case result := <-ch:
		if result.err != nil {
			t.Fatal(result.err)
		}
		return result.lease
	case <-time.After(time.Second):
		t.Fatal("admission was not granted")
		return nil
	}
}

func receiveError(t *testing.T, ch <-chan asyncResult) error {
	t.Helper()
	select {
	case result := <-ch:
		return result.err
	case <-time.After(time.Second):
		t.Fatal("admission did not finish")
		return nil
	}
}

func assertNotGranted(t *testing.T, ch <-chan asyncResult) {
	t.Helper()
	select {
	case result := <-ch:
		if result.err != nil {
			t.Fatal(result.err)
		}
		result.lease.Release()
		t.Fatal("admission granted out of order")
	case <-time.After(20 * time.Millisecond):
	}
}

func waitQueued(t *testing.T, c *Controller, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if c.Stats().Queued == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("queued = %d, want %d", c.Stats().Queued, want)
}

func assertReason(t *testing.T, err error, want RejectionReason) {
	t.Helper()
	var rejection *Rejection
	if !errors.As(err, &rejection) || rejection.Reason != want {
		t.Fatalf("error = %v, want rejection %s", err, want)
	}
}

type anyResult struct {
	class Class
	lease Lease
	other <-chan asyncResult
}

func receiveAny(t *testing.T, first, second <-chan asyncResult) anyResult {
	t.Helper()
	select {
	case result := <-first:
		if result.err != nil {
			t.Fatal(result.err)
		}
		return anyResult{class: Interactive, lease: result.lease, other: second}
	case result := <-second:
		if result.err != nil {
			t.Fatal(result.err)
		}
		return anyResult{class: Background, lease: result.lease, other: first}
	case <-time.After(time.Second):
		t.Fatal("no admission granted")
		return anyResult{}
	}
}

func TestConcurrentStatsReleaseAndCancellation(t *testing.T) {
	c := newTestController(t, Config{MaxRunning: 4, MaximumQueued: 64, Classes: map[Class]Policy{
		Interactive: {MaximumRunning: 4, MaximumQueued: 64, MaximumQueuedPerWorkspace: 64},
	}})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			lease, err := c.Acquire(ctx, Request{Class: Interactive, WorkspaceID: "race", Operation: "race"})
			if err == nil {
				_ = c.Stats()
				lease.Release()
				lease.Release()
			}
		}()
	}
	wg.Wait()
	if stats := c.Stats(); stats.Running != 0 || stats.Queued != 0 {
		t.Fatalf("unbalanced stats: %#v", stats)
	}
}
