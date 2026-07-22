// Package workload owns node-local workload admission, fairness, deadlines,
// and admission telemetry. It deliberately has no product-capability imports.
package workload

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Class string

const (
	Interactive Class = "interactive"
	Background  Class = "background"
	Refresh     Class = "refresh"
	Control     Class = "control"
	Maintenance Class = "maintenance"

	NodeWorkspace   = "_node"
	GlobalWorkspace = "_global"
)

var classOrder = []Class{Interactive, Background, Refresh, Control, Maintenance}

type Request struct {
	Class       Class
	WorkspaceID string
	Operation   string
}

type Policy struct {
	ReservedRunning           int
	MaximumRunning            int
	MaximumQueued             int
	MaximumQueuedPerWorkspace int
	QueueTimeout              time.Duration
	ExecutionTimeout          time.Duration
}

type Config struct {
	MaxRunning    int
	MaximumQueued int
	Classes       map[Class]Policy
}

func DefaultConfig() Config {
	return Config{
		MaxRunning:    5,
		MaximumQueued: 112,
		Classes: map[Class]Policy{
			Interactive: {ReservedRunning: 3, MaximumRunning: 4, MaximumQueued: 64, MaximumQueuedPerWorkspace: 16, QueueTimeout: 30 * time.Second, ExecutionTimeout: 2 * time.Minute},
			Background:  {MaximumRunning: 1, MaximumQueued: 16, MaximumQueuedPerWorkspace: 4, QueueTimeout: 2 * time.Minute, ExecutionTimeout: 15 * time.Minute},
			Refresh:     {ReservedRunning: 1, MaximumRunning: 1, MaximumQueued: 16, MaximumQueuedPerWorkspace: 1, QueueTimeout: 2 * time.Minute},
			Control:     {ReservedRunning: 1, MaximumRunning: 1, MaximumQueued: 16, MaximumQueuedPerWorkspace: 16, QueueTimeout: 2 * time.Minute, ExecutionTimeout: 15 * time.Minute},
			Maintenance: {MaximumRunning: 1, ExecutionTimeout: 30 * time.Minute},
		},
	}
}

func (c Config) Validate() error {
	if c.MaxRunning < 0 || c.MaximumQueued < 0 {
		return fmt.Errorf("workload node limits must not be negative")
	}
	reserved := 0
	for _, class := range classOrder {
		policy := c.Classes[class]
		if policy.ReservedRunning < 0 || policy.MaximumRunning < 0 || policy.MaximumQueued < 0 || policy.MaximumQueuedPerWorkspace < 0 || policy.QueueTimeout < 0 || policy.ExecutionTimeout < 0 {
			return fmt.Errorf("workload %s limits must not be negative", class)
		}
		if policy.ReservedRunning > policy.MaximumRunning {
			return fmt.Errorf("workload %s reserved running exceeds maximum running", class)
		}
		if policy.MaximumRunning > c.MaxRunning {
			return fmt.Errorf("workload %s maximum running exceeds node maximum", class)
		}
		if policy.MaximumQueuedPerWorkspace > policy.MaximumQueued {
			return fmt.Errorf("workload %s per-workspace queue exceeds class queue", class)
		}
		reserved += policy.ReservedRunning
	}
	if reserved > c.MaxRunning {
		return fmt.Errorf("workload reservations exceed node capacity")
	}
	return nil
}

type RejectionReason string

const (
	NodeQueueFull              RejectionReason = "node_queue_full"
	ClassQueueFull             RejectionReason = "class_queue_full"
	WorkspaceQueueFull         RejectionReason = "workspace_queue_full"
	QueueTimeout               RejectionReason = "queue_timeout"
	ConflictingNestedAdmission RejectionReason = "conflicting_nested_admission"
	ControllerShutdown         RejectionReason = "controller_shutdown"
	InvalidRequest             RejectionReason = "invalid_request"
)

type Rejection struct {
	Reason      RejectionReason
	Class       Class
	WorkspaceID string
	Operation   string
	QueueWait   time.Duration
	cause       error
}

func (e *Rejection) Error() string {
	if e == nil {
		return "workload admission rejected"
	}
	return fmt.Sprintf("workload admission rejected: %s (class=%s workspace=%s)", e.Reason, e.Class, e.WorkspaceID)
}

func (e *Rejection) Unwrap() error                   { return e.cause }
func (e *Rejection) WorkloadRejectionReason() string { return string(e.Reason) }

func ReasonOf(err error) (RejectionReason, bool) {
	var rejection *Rejection
	if !errors.As(err, &rejection) {
		return "", false
	}
	return rejection.Reason, true
}

type Lease interface {
	Context() context.Context
	QueueWait() time.Duration
	Release()
}

type Admitter interface {
	Acquire(context.Context, Request) (Lease, error)
}

type admitterContextKey struct{}

func WithAdmitter(ctx context.Context, admitter Admitter) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if admitter == nil {
		return ctx
	}
	return context.WithValue(ctx, admitterContextKey{}, admitter)
}

func FromContext(ctx context.Context) (Admitter, bool) {
	if ctx == nil {
		return nil, false
	}
	admitter, ok := ctx.Value(admitterContextKey{}).(Admitter)
	return admitter, ok && admitter != nil
}

func Current(ctx context.Context) (Class, string, bool) {
	if ctx == nil {
		return "", "", false
	}
	active, ok := ctx.Value(admissionContextKey{}).(*admissionContext)
	if !ok || active == nil {
		return "", "", false
	}
	return active.class, active.workspace, true
}

type WorkspaceStats struct {
	Running int
	Queued  int
}

type ClassStats struct {
	Policy     Policy
	Running    int
	Queued     int
	Borrowed   int
	Workspaces map[string]WorkspaceStats
}

type Stats struct {
	MaxRunning int
	Running    int
	Queued     int
	Classes    map[Class]ClassStats
}

type AdmissionEvent struct {
	Class       Class
	WorkspaceID string
	Operation   string
	Outcome     string
	Reason      RejectionReason
	QueueWait   time.Duration
	Execution   time.Duration
}

type Observer interface {
	ObserveWorkload(Stats)
	ObserveAdmission(AdmissionEvent)
}

type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

type Option func(*Controller)

func WithObserver(observer Observer) Option { return func(c *Controller) { c.observer = observer } }
func WithClock(clock Clock) Option          { return func(c *Controller) { c.clock = clock } }
