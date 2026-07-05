package app

import (
	"context"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

func TestRuntimeMetricsQueryDashboardUsesRuntimeLease(t *testing.T) {
	provider := &leaseRecordingProvider{}
	metrics := NewRuntimeMetrics(provider, "/data", "test")

	if _, err := metrics.QueryDashboardPage(context.Background(), "dashboard", "page", dashboard.Filters{}); err != nil {
		t.Fatalf("query dashboard: %v", err)
	}
	if provider.lease == nil {
		t.Fatal("runtime lease was not acquired")
	}
	if !provider.runtime.called {
		t.Fatal("runtime was not queried")
	}
	if !provider.lease.released {
		t.Fatal("runtime lease was not released after query")
	}
	if provider.runtime.releasedDuringCall {
		t.Fatal("runtime lease was released before query completed")
	}
}

type leaseRecordingProvider struct {
	runtime leaseRecordingRuntime
	lease   *recordingLease
}

func (p *leaseRecordingProvider) Active(context.Context) (runtimehost.Runtime, error) {
	return &p.runtime, nil
}

func (p *leaseRecordingProvider) Acquire(context.Context) (runtimehost.Lease, error) {
	p.lease = &recordingLease{runtime: &p.runtime, snapshotID: 42}
	p.runtime.lease = p.lease
	return p.lease, nil
}

type leaseRecordingRuntime struct {
	lease              *recordingLease
	called             bool
	releasedDuringCall bool
}

func (r *leaseRecordingRuntime) Close() error {
	return nil
}

func (r *leaseRecordingRuntime) QueryDashboardPage(context.Context, string, string, dashboard.Filters) (dashboard.Patch, error) {
	r.called = true
	if r.lease != nil && r.lease.released {
		r.releasedDuringCall = true
	}
	return dashboard.Patch{}, nil
}

type recordingLease struct {
	runtime    runtimehost.Runtime
	snapshotID int64
	released   bool
}

func (l *recordingLease) Runtime() runtimehost.Runtime {
	return l.runtime
}

func (l *recordingLease) ServingStateID() servingstate.ID {
	return "dep_test"
}

func (l *recordingLease) DuckLakeSnapshotID() int64 {
	return l.snapshotID
}

func (l *recordingLease) Release() {
	l.released = true
}
