package workload

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Controller struct {
	mu sync.Mutex

	config   Config
	clock    Clock
	observer Observer
	closed   bool

	running      int
	runningClass map[Class]int
	runningWS    map[Class]map[string]int
	active       map[*lease]struct{}
	queues       map[Class]*classQueue
	classCursor  int
}

type waiter struct {
	request  Request
	parent   context.Context
	enqueued time.Time
	result   chan acquireResult
	state    waiterState
}

type waiterState uint8

const (
	waiting waiterState = iota
	granted
	rejected
)

type acquireResult struct {
	lease Lease
	err   error
}

type classQueue struct {
	workspaces map[string][]*waiter
	order      []string
	cursor     int
	queued     int
}

type admissionContext struct {
	controller *Controller
	class      Class
	workspace  string
}

type admissionContextKey struct{}

type lease struct {
	controller *Controller
	request    Request
	ctx        context.Context
	cancel     context.CancelFunc
	queueWait  time.Duration
	started    time.Time
	once       sync.Once
}

type nestedLease struct {
	ctx context.Context
}

func New(config Config, options ...Option) (*Controller, error) {
	if config.MaxRunning == 0 && config.MaximumQueued == 0 && len(config.Classes) == 0 {
		config = DefaultConfig()
	}
	config.Classes = clonePolicies(config.Classes)
	if err := config.Validate(); err != nil {
		return nil, err
	}
	c := &Controller{
		config:       config,
		clock:        realClock{},
		runningClass: make(map[Class]int, len(classOrder)),
		runningWS:    make(map[Class]map[string]int, len(classOrder)),
		active:       make(map[*lease]struct{}),
		queues:       make(map[Class]*classQueue, len(classOrder)),
	}
	for _, class := range classOrder {
		c.runningWS[class] = make(map[string]int)
		c.queues[class] = &classQueue{workspaces: make(map[string][]*waiter)}
	}
	for _, option := range options {
		if option != nil {
			option(c)
		}
	}
	if c.clock == nil {
		return nil, fmt.Errorf("workload clock is required")
	}
	return c, nil
}

func (c *Controller) Acquire(ctx context.Context, request Request) (Lease, error) {
	if c == nil {
		return nil, &Rejection{Reason: ControllerShutdown, Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request, err := c.normalize(request)
	if err != nil {
		c.observeAdmission(AdmissionEvent{Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, Outcome: "rejected", Reason: InvalidRequest})
		return nil, err
	}
	if active, _ := ctx.Value(admissionContextKey{}).(*admissionContext); active != nil {
		if active.controller == c && active.class == request.Class && active.workspace == request.WorkspaceID {
			return &nestedLease{ctx: ctx}, nil
		}
		err := c.rejection(request, ConflictingNestedAdmission, nil)
		c.observeAdmission(AdmissionEvent{Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, Outcome: "rejected", Reason: ConflictingNestedAdmission})
		return nil, err
	}

	w := &waiter{request: request, parent: ctx, enqueued: c.clock.Now(), result: make(chan acquireResult, 1), state: waiting}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		err := c.rejection(request, ControllerShutdown, nil)
		c.observeAdmission(AdmissionEvent{Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, Outcome: "rejected", Reason: ControllerShutdown})
		return nil, err
	}
	queue := c.queues[request.Class]
	queue.enqueue(w)
	c.scheduleLocked()
	if w.state == waiting {
		reason := c.queueLimitReasonLocked(request)
		if reason != "" {
			queue.remove(w)
			w.state = rejected
			stats := c.statsLocked()
			c.mu.Unlock()
			err := c.rejection(request, reason, nil)
			err.(*Rejection).QueueWait = c.clock.Now().Sub(w.enqueued)
			c.observeStats(stats)
			c.observeAdmission(AdmissionEvent{Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, Outcome: "rejected", Reason: reason})
			return nil, err
		}
	}
	stats := c.statsLocked()
	immediate := w.state == granted
	c.mu.Unlock()
	c.observeStats(stats)

	if immediate {
		result := <-w.result
		c.observeAdmission(AdmissionEvent{Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, Outcome: "admitted", QueueWait: result.lease.QueueWait()})
		return result.lease, nil
	}

	policy := c.config.Classes[request.Class]
	var timer Timer
	var timeout <-chan time.Time
	if policy.QueueTimeout > 0 {
		timer = c.clock.NewTimer(policy.QueueTimeout)
		timeout = timer.C()
		defer timer.Stop()
	}
	select {
	case result := <-w.result:
		if result.err != nil {
			return nil, result.err
		}
		if err := ctx.Err(); err != nil {
			result.lease.Release()
			return nil, err
		}
		c.observeAdmission(AdmissionEvent{Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, Outcome: "admitted", QueueWait: result.lease.QueueWait()})
		return result.lease, nil
	case <-ctx.Done():
		if acquired := c.cancelWaiter(w); acquired != nil {
			acquired.Release()
		} else {
			c.observeAdmission(AdmissionEvent{Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, Outcome: "canceled", QueueWait: c.clock.Now().Sub(w.enqueued)})
		}
		return nil, ctx.Err()
	case <-timeout:
		if acquired := c.cancelWaiter(w); acquired != nil {
			if err := ctx.Err(); err == nil {
				c.observeAdmission(AdmissionEvent{Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, Outcome: "admitted", QueueWait: acquired.QueueWait()})
				return acquired, nil
			}
			acquired.Release()
			return nil, ctx.Err()
		}
		err := c.rejection(request, QueueTimeout, context.DeadlineExceeded)
		err.(*Rejection).QueueWait = c.clock.Now().Sub(w.enqueued)
		c.observeAdmission(AdmissionEvent{Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, Outcome: "rejected", Reason: QueueTimeout, QueueWait: c.clock.Now().Sub(w.enqueued)})
		return nil, err
	}
}

func (c *Controller) cancelWaiter(w *waiter) Lease {
	c.mu.Lock()
	if w.state == waiting {
		c.queues[w.request.Class].remove(w)
		w.state = rejected
		c.scheduleLocked()
		stats := c.statsLocked()
		c.mu.Unlock()
		c.observeStats(stats)
		return nil
	}
	c.mu.Unlock()
	select {
	case result := <-w.result:
		return result.lease
	default:
		return nil
	}
}

func (c *Controller) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	var rejectedWaiters []*waiter
	for _, class := range classOrder {
		queue := c.queues[class]
		for _, workspace := range queue.order {
			for _, w := range queue.workspaces[workspace] {
				w.state = rejected
				rejectedWaiters = append(rejectedWaiters, w)
			}
		}
		queue.workspaces = make(map[string][]*waiter)
		queue.order = nil
		queue.cursor = 0
		queue.queued = 0
	}
	stats := c.statsLocked()
	active := make([]*lease, 0, len(c.active))
	for running := range c.active {
		active = append(active, running)
	}
	c.mu.Unlock()
	for _, running := range active {
		running.cancel()
	}
	for _, w := range rejectedWaiters {
		err := c.rejection(w.request, ControllerShutdown, nil)
		err.(*Rejection).QueueWait = c.clock.Now().Sub(w.enqueued)
		w.result <- acquireResult{err: err}
		c.observeAdmission(AdmissionEvent{Class: w.request.Class, WorkspaceID: w.request.WorkspaceID, Operation: w.request.Operation, Outcome: "rejected", Reason: ControllerShutdown, QueueWait: c.clock.Now().Sub(w.enqueued)})
	}
	c.observeStats(stats)
}

func (c *Controller) Stats() Stats {
	if c == nil {
		return Stats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statsLocked()
}

func (c *Controller) SetObserver(observer Observer) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.observer = observer
	stats := c.statsLocked()
	c.mu.Unlock()
	c.observeStats(stats)
}

func (c *Controller) scheduleLocked() {
	for !c.closed && c.running < c.config.MaxRunning {
		class, ok := c.nextClassLocked(true)
		if !ok {
			class, ok = c.nextClassLocked(false)
		}
		if !ok {
			return
		}
		w := c.queues[class].pop()
		if w == nil {
			return
		}
		w.state = granted
		c.running++
		c.runningClass[class]++
		c.runningWS[class][w.request.WorkspaceID]++
		wait := c.clock.Now().Sub(w.enqueued)
		policy := c.config.Classes[class]
		var execCtx context.Context
		var cancel context.CancelFunc
		if policy.ExecutionTimeout > 0 {
			execCtx, cancel = context.WithTimeout(w.parent, policy.ExecutionTimeout)
		} else {
			execCtx, cancel = context.WithCancel(w.parent)
		}
		execCtx = context.WithValue(execCtx, admissionContextKey{}, &admissionContext{controller: c, class: class, workspace: w.request.WorkspaceID})
		grantedLease := &lease{controller: c, request: w.request, ctx: execCtx, cancel: cancel, queueWait: wait, started: c.clock.Now()}
		c.active[grantedLease] = struct{}{}
		w.result <- acquireResult{lease: grantedLease}
	}
}

func (c *Controller) nextClassLocked(reservedOnly bool) (Class, bool) {
	for offset := 0; offset < len(classOrder); offset++ {
		index := (c.classCursor + offset) % len(classOrder)
		class := classOrder[index]
		policy := c.config.Classes[class]
		if c.queues[class].queued == 0 || c.runningClass[class] >= policy.MaximumRunning {
			continue
		}
		if reservedOnly && c.runningClass[class] >= policy.ReservedRunning {
			continue
		}
		c.classCursor = (index + 1) % len(classOrder)
		return class, true
	}
	return "", false
}

func (c *Controller) queueLimitReasonLocked(request Request) RejectionReason {
	queue := c.queues[request.Class]
	policy := c.config.Classes[request.Class]
	total := 0
	for _, class := range classOrder {
		total += c.queues[class].queued
	}
	if total > c.config.MaximumQueued {
		return NodeQueueFull
	}
	if queue.queued > policy.MaximumQueued {
		return ClassQueueFull
	}
	if len(queue.workspaces[request.WorkspaceID]) > policy.MaximumQueuedPerWorkspace {
		return WorkspaceQueueFull
	}
	return ""
}

func (c *Controller) normalize(request Request) (Request, error) {
	known := false
	for _, class := range classOrder {
		if request.Class == class {
			known = true
			break
		}
	}
	if !known || request.Operation == "" || len(request.Operation) > 96 {
		return request, c.rejection(request, InvalidRequest, nil)
	}
	if request.Class == Control || request.Class == Maintenance {
		request.WorkspaceID = NodeWorkspace
	} else if request.WorkspaceID == "" || request.WorkspaceID == NodeWorkspace || (request.WorkspaceID == GlobalWorkspace && request.Class != Background) {
		return request, c.rejection(request, InvalidRequest, nil)
	}
	return request, nil
}

func (c *Controller) rejection(request Request, reason RejectionReason, cause error) error {
	return &Rejection{Reason: reason, Class: request.Class, WorkspaceID: request.WorkspaceID, Operation: request.Operation, cause: cause}
}

func (c *Controller) statsLocked() Stats {
	stats := Stats{MaxRunning: c.config.MaxRunning, Running: c.running, Classes: make(map[Class]ClassStats, len(classOrder))}
	for _, class := range classOrder {
		queue := c.queues[class]
		classStats := ClassStats{Policy: c.config.Classes[class], Running: c.runningClass[class], Queued: queue.queued, Workspaces: make(map[string]WorkspaceStats)}
		classStats.Borrowed = classStats.Running - classStats.Policy.ReservedRunning
		if classStats.Borrowed < 0 {
			classStats.Borrowed = 0
		}
		for workspace, running := range c.runningWS[class] {
			classStats.Workspaces[workspace] = WorkspaceStats{Running: running}
		}
		for workspace, waiters := range queue.workspaces {
			workspaceStats := classStats.Workspaces[workspace]
			workspaceStats.Queued = len(waiters)
			classStats.Workspaces[workspace] = workspaceStats
		}
		stats.Queued += queue.queued
		stats.Classes[class] = classStats
	}
	return stats
}

func (c *Controller) observeStats(stats Stats) {
	c.mu.Lock()
	observer := c.observer
	c.mu.Unlock()
	if observer != nil {
		observer.ObserveWorkload(stats)
	}
}
func (c *Controller) observeAdmission(event AdmissionEvent) {
	c.mu.Lock()
	observer := c.observer
	c.mu.Unlock()
	if observer != nil {
		observer.ObserveAdmission(event)
	}
}

func (l *lease) Context() context.Context { return l.ctx }
func (l *lease) QueueWait() time.Duration { return l.queueWait }
func (l *lease) Release() {
	if l == nil || l.controller == nil {
		return
	}
	l.once.Do(func() {
		contextErr := l.ctx.Err()
		l.cancel()
		c := l.controller
		c.mu.Lock()
		c.running--
		c.runningClass[l.request.Class]--
		c.runningWS[l.request.Class][l.request.WorkspaceID]--
		delete(c.active, l)
		if c.runningWS[l.request.Class][l.request.WorkspaceID] == 0 {
			delete(c.runningWS[l.request.Class], l.request.WorkspaceID)
		}
		c.scheduleLocked()
		stats := c.statsLocked()
		c.mu.Unlock()
		c.observeStats(stats)
		outcome := "completed"
		if contextErr == context.DeadlineExceeded {
			outcome = "timeout"
		} else if contextErr == context.Canceled {
			outcome = "canceled"
		}
		c.observeAdmission(AdmissionEvent{Class: l.request.Class, WorkspaceID: l.request.WorkspaceID, Operation: l.request.Operation, Outcome: outcome, QueueWait: l.queueWait, Execution: c.clock.Now().Sub(l.started)})
	})
}

func (l *nestedLease) Context() context.Context { return l.ctx }
func (l *nestedLease) QueueWait() time.Duration { return 0 }
func (l *nestedLease) Release()                 {}

func (q *classQueue) enqueue(w *waiter) {
	workspace := w.request.WorkspaceID
	if _, ok := q.workspaces[workspace]; !ok {
		q.order = append(q.order, workspace)
	}
	q.workspaces[workspace] = append(q.workspaces[workspace], w)
	q.queued++
}

func (q *classQueue) pop() *waiter {
	if q.queued == 0 || len(q.order) == 0 {
		return nil
	}
	if q.cursor >= len(q.order) {
		q.cursor = 0
	}
	workspace := q.order[q.cursor]
	waiters := q.workspaces[workspace]
	w := waiters[0]
	waiters = waiters[1:]
	q.queued--
	if len(waiters) == 0 {
		delete(q.workspaces, workspace)
		q.order = append(q.order[:q.cursor], q.order[q.cursor+1:]...)
		if len(q.order) == 0 {
			q.cursor = 0
		} else if q.cursor >= len(q.order) {
			q.cursor = 0
		}
	} else {
		q.workspaces[workspace] = waiters
		q.cursor = (q.cursor + 1) % len(q.order)
	}
	return w
}

func (q *classQueue) remove(target *waiter) bool {
	workspace := target.request.WorkspaceID
	waiters := q.workspaces[workspace]
	for i, w := range waiters {
		if w != target {
			continue
		}
		waiters = append(waiters[:i], waiters[i+1:]...)
		q.queued--
		if len(waiters) > 0 {
			q.workspaces[workspace] = waiters
			return true
		}
		delete(q.workspaces, workspace)
		for index, candidate := range q.order {
			if candidate != workspace {
				continue
			}
			q.order = append(q.order[:index], q.order[index+1:]...)
			if index < q.cursor {
				q.cursor--
			}
			if len(q.order) == 0 || q.cursor >= len(q.order) {
				q.cursor = 0
			}
			break
		}
		return true
	}
	return false
}

func clonePolicies(source map[Class]Policy) map[Class]Policy {
	result := make(map[Class]Policy, len(classOrder))
	for _, class := range classOrder {
		result[class] = source[class]
	}
	return result
}

type realClock struct{}
type realTimer struct{ timer *time.Timer }

func (realClock) Now() time.Time { return time.Now() }
func (realClock) NewTimer(duration time.Duration) Timer {
	return realTimer{timer: time.NewTimer(duration)}
}
func (t realTimer) C() <-chan time.Time { return t.timer.C }
func (t realTimer) Stop() bool          { return t.timer.Stop() }
