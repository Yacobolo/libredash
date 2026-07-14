package dataquery

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
)

// CacheOutcomeObserver receives low-cardinality cache outcomes at the query
// boundary. The observer is request scoped so runtimes remain independent of
// HTTP and metrics packages.
type CacheOutcomeObserver func(outcome string)

type PhysicalQueryObservation struct {
	Count  int
	Result Result
}

// PhysicalQueryObserver receives the physical statement count and aggregate
// stage timings for one cache-miss execution. Cache hits and coalesced callers
// never invoke it.
type PhysicalQueryObserver func(observation PhysicalQueryObservation)

// ConnectionWaitObserver receives time spent waiting for a database/sql pool
// connection. Executors report exactly once for each public query operation.
type ConnectionWaitObserver func(wait time.Duration)

// ConnectionWaitCounter accumulates connection acquisition time across one
// logical data query, which may execute more than one physical operation.
type ConnectionWaitCounter struct{ nanoseconds atomic.Int64 }

type cacheOutcomeObserverContextKey struct{}
type physicalQueryObserverContextKey struct{}
type connectionWaitObserverContextKey struct{}

func WithCacheOutcomeObserver(ctx context.Context, observer CacheOutcomeObserver) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, cacheOutcomeObserverContextKey{}, observer)
}

func ObserveCacheOutcome(ctx context.Context, outcome string) {
	if ctx == nil || strings.TrimSpace(outcome) == "" {
		return
	}
	observer, ok := ctx.Value(cacheOutcomeObserverContextKey{}).(CacheOutcomeObserver)
	if ok && observer != nil {
		observer(outcome)
	}
}

func WithPhysicalQueryObserver(ctx context.Context, observer PhysicalQueryObserver) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, physicalQueryObserverContextKey{}, observer)
}

func ObservePhysicalQuery(ctx context.Context, observation PhysicalQueryObservation) {
	if ctx == nil {
		return
	}
	observer, ok := ctx.Value(physicalQueryObserverContextKey{}).(PhysicalQueryObserver)
	if ok && observer != nil {
		observer(observation)
	}
}

func WithConnectionWaitObserver(ctx context.Context, observer ConnectionWaitObserver) context.Context {
	if observer == nil {
		return ctx
	}
	if existing, ok := ctx.Value(connectionWaitObserverContextKey{}).(ConnectionWaitObserver); ok && existing != nil {
		return context.WithValue(ctx, connectionWaitObserverContextKey{}, ConnectionWaitObserver(func(wait time.Duration) {
			existing(wait)
			observer(wait)
		}))
	}
	return context.WithValue(ctx, connectionWaitObserverContextKey{}, observer)
}

func ObserveConnectionWait(ctx context.Context, wait time.Duration) {
	if ctx == nil || wait < 0 {
		return
	}
	observer, ok := ctx.Value(connectionWaitObserverContextKey{}).(ConnectionWaitObserver)
	if ok && observer != nil {
		observer(wait)
	}
}

func WithConnectionWaitCounter(ctx context.Context) (context.Context, *ConnectionWaitCounter) {
	counter := &ConnectionWaitCounter{}
	return WithConnectionWaitObserver(ctx, counter.Add), counter
}

func (c *ConnectionWaitCounter) Add(wait time.Duration) {
	if c != nil && wait > 0 {
		c.nanoseconds.Add(int64(wait))
	}
}

func (c *ConnectionWaitCounter) Duration() time.Duration {
	if c == nil {
		return 0
	}
	return time.Duration(c.nanoseconds.Load())
}
