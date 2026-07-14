package stream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/command"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type targetMetrics struct {
	visualRelease <-chan struct{}
	visualErr     error
}

func (targetMetrics) DataDir() string                 { return ".data" }
func (targetMetrics) DashboardTargetConcurrency() int { return 2 }
func (m targetMetrics) QueryVisualPage(ctx context.Context, _, _ string, _ dashboard.Filters, visualID string) (dashboard.Visual, error) {
	if m.visualRelease != nil {
		select {
		case <-m.visualRelease:
		case <-ctx.Done():
			return dashboard.Visual{}, ctx.Err()
		}
	}
	if m.visualErr != nil {
		return dashboard.Visual{}, m.visualErr
	}
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 1})
	return dashboard.Visual{ID: visualID, Title: visualID}, nil
}

type serialTargetMetrics struct {
	started chan struct{}
}

type leaseContextKey struct{}

type leaseTargetMetrics struct {
	leaseCalls int
	sawLease   bool
}

func (m *leaseTargetMetrics) DataDir() string { return ".data" }
func (m *leaseTargetMetrics) WithDashboardRefreshLease(ctx context.Context, run func(context.Context) error) error {
	m.leaseCalls++
	return run(context.WithValue(ctx, leaseContextKey{}, true))
}
func (m *leaseTargetMetrics) QueryTablePage(ctx context.Context, _, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	m.sawLease, _ = ctx.Value(leaseContextKey{}).(bool)
	dataquery.ObserveCacheOutcome(ctx, dataquery.CacheHit)
	return dashboard.Table{Title: request.Table}, nil
}

type batchTargetMetrics struct {
	targetMetrics
	batches  [][]string
	report   reportdef.Dashboard
	metadata []dataquery.Metadata
}

func (m *batchTargetMetrics) Report(string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	return m.report, &semanticmodel.Model{}, true
}
func (m *batchTargetMetrics) QueryVisualsPage(ctx context.Context, _, _ string, _ dashboard.Filters, ids []string) (map[string]dashboard.Visual, error) {
	m.batches = append(m.batches, append([]string(nil), ids...))
	m.metadata = append(m.metadata, dataquery.MetadataFromContext(ctx))
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 2, Result: dataquery.Result{QueueWaitMS: 2, ConnectionWaitMS: 3, PlanningMS: 4, DatabaseMS: 5, ExecutionMS: 6}})
	visuals := make(map[string]dashboard.Visual, len(ids))
	for _, id := range ids {
		visuals[id] = dashboard.Visual{ID: id}
	}
	return visuals, nil
}

func (serialTargetMetrics) DataDir() string { return ".data" }
func (m serialTargetMetrics) QueryTablePage(ctx context.Context, _, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if request.Table == "first" {
		close(m.started)
		<-ctx.Done()
		return dashboard.Table{}, ctx.Err()
	}
	return dashboard.Table{Title: request.Table}, nil
}
func (targetMetrics) QueryFilterOptionsPage(ctx context.Context, _ string, _ string, _ []string) (map[string][]dashboard.FilterOption, error) {
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 1})
	return map[string][]dashboard.FilterOption{}, nil
}
func (targetMetrics) QueryTablePage(ctx context.Context, _, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 1})
	return dashboard.Table{Title: request.Table}, nil
}

func TestTargetWorkPublishesCompletedComponentsProgressively(t *testing.T) {
	release := make(chan struct{})
	events := make(chan RefreshEvent, 4)
	work := TargetWork(targetMetrics{visualRelease: release}, WorkRequest{
		DashboardID: "dash",
		PageID:      "overview",
		Plan: command.RefreshPlan{Targets: []command.Target{
			{Kind: command.TargetVisual, ID: "slow"},
			{Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{Table: "orders"}},
		}},
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		work(context.Background(), func(event RefreshEvent) bool {
			events <- event
			return true
		})
	}()

	select {
	case event := <-events:
		if event.Type != RefreshEventTable || event.Target != "orders" {
			t.Fatalf("first event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("table did not publish while visual was blocked")
	}
	close(release)
	select {
	case event := <-events:
		if event.Type != RefreshEventVisual || event.Target != "slow" {
			t.Fatalf("second event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("visual did not publish")
	}
	<-done
}

func TestTargetWorkIsolatesTargetFailures(t *testing.T) {
	events := make(chan RefreshEvent, 4)
	work := TargetWork(targetMetrics{visualErr: errors.New("boom")}, WorkRequest{
		DashboardID: "dash",
		PageID:      "overview",
		Plan: command.RefreshPlan{Targets: []command.Target{
			{Kind: command.TargetVisual, ID: "broken"},
			{Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{Table: "orders"}},
		}},
	})
	work(context.Background(), func(event RefreshEvent) bool {
		events <- event
		return true
	})
	close(events)

	var sawError, sawTable bool
	for event := range events {
		sawError = sawError || event.Type == RefreshEventTargetError && event.Target == "visual:broken"
		sawTable = sawTable || event.Type == RefreshEventTable && event.Target == "orders"
	}
	if !sawError || !sawTable {
		t.Fatalf("saw error=%t table=%t", sawError, sawTable)
	}
}

func TestTargetWorkCancellationWhileWaitingForSerialAdmissionReturns(t *testing.T) {
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	work := TargetWork(serialTargetMetrics{started: started}, WorkRequest{
		Plan: command.RefreshPlan{Targets: []command.Target{
			{Kind: command.TargetTable, ID: "first", TableRequest: dashboard.TableRequest{Table: "first"}},
			{Kind: command.TargetTable, ID: "queued", TableRequest: dashboard.TableRequest{Table: "queued"}},
		}},
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		work(ctx, func(RefreshEvent) bool { return true })
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled serial target work deadlocked")
	}
}

func TestTargetWorkBatchesSingleValueVisualsAndPublishesEachConsumer(t *testing.T) {
	metrics := &batchTargetMetrics{report: reportdef.Dashboard{Visuals: map[string]reportdef.Visual{
		"revenue": {Shape: "single_value"},
		"orders":  {Shape: "single_value"},
		"trend":   {Shape: "category"},
	}}}
	var eventsMu sync.Mutex
	events := []RefreshEvent{}
	TargetWork(metrics, WorkRequest{
		DashboardID: "dash",
		Plan: command.RefreshPlan{Targets: []command.Target{
			{Kind: command.TargetVisual, ID: "revenue"},
			{Kind: command.TargetVisual, ID: "trend"},
			{Kind: command.TargetVisual, ID: "orders"},
		}},
	})(context.Background(), func(event RefreshEvent) bool {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, event)
		return true
	})

	if len(metrics.batches) != 1 || len(metrics.batches[0]) != 2 || metrics.batches[0][0] != "revenue" || metrics.batches[0][1] != "orders" {
		t.Fatalf("KPI batches = %#v", metrics.batches)
	}
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	queryCount := 0
	targetCounts := map[string]int{}
	for _, event := range events {
		queryCount += event.Queries
		targetCounts[event.Target]++
	}
	if queryCount != 3 {
		t.Fatalf("physical query count = %d, want 3", queryCount)
	}
	for _, target := range []string{"revenue", "orders", "trend"} {
		if targetCounts[target] != 1 {
			t.Fatalf("target %q execution events = %d, want 1; all=%#v", target, targetCounts[target], events)
		}
	}
	var batchTimings map[string]float64
	for _, event := range events {
		if event.Queries == 2 {
			batchTimings = event.StageTimingsMs
		}
	}
	if batchTimings["admissionWait"] != 2 || batchTimings["connectionWait"] != 3 || batchTimings["planning"] != 4 || batchTimings["database"] != 5 || batchTimings["execution"] != 6 {
		t.Fatalf("batch stage timings = %#v", batchTimings)
	}
	if len(metrics.metadata) != 1 || metrics.metadata[0].ObjectType != "dashboard_refresh_targets" || metrics.metadata[0].ObjectID != "visual:orders,visual:revenue" {
		t.Fatalf("batch consumer metadata = %#v", metrics.metadata)
	}
}

func TestTargetWorkUsesOneRefreshLeaseAndAcceptedObservers(t *testing.T) {
	metrics := &leaseTargetMetrics{}
	observedEvents := 0
	observedQueries := -1
	cacheOutcomes := []string{}
	TargetWork(metrics, WorkRequest{
		Plan: command.RefreshPlan{Targets: []command.Target{{
			Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{Table: "orders"},
		}}},
		EventObserved: func(event RefreshEvent) {
			observedEvents++
			observedQueries = event.Queries
		},
		CacheObserved: func(outcome string) { cacheOutcomes = append(cacheOutcomes, outcome) },
	})(context.Background(), func(RefreshEvent) bool { return true })

	if metrics.leaseCalls != 1 || !metrics.sawLease {
		t.Fatalf("lease calls=%d saw lease=%t", metrics.leaseCalls, metrics.sawLease)
	}
	if observedEvents != 2 {
		t.Fatalf("observed events = %d, want 2", observedEvents)
	}
	if observedQueries != 0 {
		t.Fatalf("cache-hit physical queries = %d, want 0", observedQueries)
	}
	if len(cacheOutcomes) != 1 || cacheOutcomes[0] != dataquery.CacheHit {
		t.Fatalf("cache outcomes = %#v", cacheOutcomes)
	}
}

func TestTargetWorkPublishesCacheOutcomeForRefreshSummary(t *testing.T) {
	metrics := &leaseTargetMetrics{}
	events := []RefreshEvent{}
	TargetWork(metrics, WorkRequest{
		DashboardID: "dashboard",
		Plan:        command.RefreshPlan{Targets: []command.Target{{Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{Table: "orders"}}}},
	})(context.Background(), func(event RefreshEvent) bool {
		events = append(events, event)
		return true
	})
	for _, event := range events {
		if event.Type == RefreshEventCacheOutcome && event.CacheOutcome == dataquery.CacheHit {
			return
		}
	}
	t.Fatalf("cache outcome event not published: %#v", events)
}
