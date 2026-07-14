package stream

import (
	"context"
	"errors"
	"strings"
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

type progressiveTableTargetMetricsStub struct {
	targetMetrics
	rows         dashboard.Table
	rowsErr      error
	count        int
	countErr     error
	countStarted chan struct{}
	countRelease <-chan struct{}
	countOnce    sync.Once
}

func (m *progressiveTableTargetMetricsStub) QueryTableRowsPage(ctx context.Context, _ string, _ string, _ dashboard.Filters, _ dashboard.TableRequest) (dashboard.Table, error) {
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 1})
	return m.rows, m.rowsErr
}

func (m *progressiveTableTargetMetricsStub) QueryTableCountPage(ctx context.Context, _ string, _ string, _ dashboard.Filters, _ dashboard.TableRequest) (int, error) {
	m.countOnce.Do(func() {
		if m.countStarted != nil {
			close(m.countStarted)
		}
	})
	if m.countRelease != nil {
		select {
		case <-m.countRelease:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 1})
	return m.count, m.countErr
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
	batches         [][]string
	report          reportdef.Dashboard
	model           *semanticmodel.Model
	metadata        []dataquery.Metadata
	failBatch       map[string]error
	calls           []string
	batchQueryCount int
	visualErrors    map[string]error
}

type bundleTargetMetrics struct {
	batchTargetMetrics
	bundleCalls [][]string
	bundleErr   error
}

func (m *bundleTargetMetrics) QueryVisualBundlePage(ctx context.Context, _, _ string, _ dashboard.Filters, ids []string) (map[string]dashboard.Visual, error) {
	m.bundleCalls = append(m.bundleCalls, append([]string{}, ids...))
	m.calls = append(m.calls, "bundle:"+strings.Join(ids, ","))
	if m.bundleErr != nil {
		return nil, m.bundleErr
	}
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 1})
	out := map[string]dashboard.Visual{}
	for _, id := range ids {
		out[id] = dashboard.Visual{ID: id}
	}
	return out, nil
}

func (m *batchTargetMetrics) Report(string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	model := m.model
	if model == nil {
		model = &semanticmodel.Model{}
	}
	return m.report, model, true
}
func (m *batchTargetMetrics) DashboardTargetConcurrency() int { return 1 }
func (m *batchTargetMetrics) QueryVisualsPage(ctx context.Context, _, _ string, _ dashboard.Filters, ids []string) (map[string]dashboard.Visual, error) {
	m.batches = append(m.batches, append([]string(nil), ids...))
	m.calls = append(m.calls, "visuals:"+strings.Join(ids, ","))
	m.metadata = append(m.metadata, dataquery.MetadataFromContext(ctx))
	if err := m.failBatch[strings.Join(ids, ",")]; err != nil {
		return nil, err
	}
	queryCount := m.batchQueryCount
	if queryCount == 0 {
		queryCount = 2
	}
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: queryCount, Result: dataquery.Result{QueueWaitMS: 2, ConnectionWaitMS: 3, PlanningMS: 4, DatabaseMS: 5, ExecutionMS: 6}})
	visuals := make(map[string]dashboard.Visual, len(ids))
	for _, id := range ids {
		visuals[id] = dashboard.Visual{ID: id}
	}
	return visuals, nil
}

func (m *batchTargetMetrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	m.calls = append(m.calls, "visual:"+visualID)
	if err := m.visualErrors[visualID]; err != nil {
		return dashboard.Visual{}, err
	}
	return m.targetMetrics.QueryVisualPage(ctx, dashboardID, pageID, filters, visualID)
}

func (m *batchTargetMetrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	m.calls = append(m.calls, "table:"+request.Table)
	return m.targetMetrics.QueryTablePage(ctx, dashboardID, pageID, filters, request)
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

func TestTargetWorkPublishesTableRowsBeforeExactCount(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	metrics := &progressiveTableTargetMetricsStub{
		rows: dashboard.Table{
			Title:          "Orders",
			TotalRowsKnown: false,
			AvailableRows:  dashboard.TableInteractiveRowCap,
			RowCap:         dashboard.TableInteractiveRowCap,
			Blocks: map[string]dashboard.TableBlock{
				"a": {Rows: []map[string]any{{"id": "first"}}},
			},
		},
		count:        42,
		countStarted: started,
		countRelease: release,
	}
	events := make(chan RefreshEvent, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		TargetWork(metrics, WorkRequest{
			DashboardID: "dash",
			PageID:      "overview",
			Plan: command.RefreshPlan{Targets: []command.Target{{
				Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{Table: "orders"},
			}}},
		})(context.Background(), func(event RefreshEvent) bool {
			events <- event
			return true
		})
	}()

	select {
	case event := <-events:
		if event.Type != RefreshEventTable || event.Target != "orders" {
			t.Fatalf("first event = %#v, want table rows", event)
		}
		table, ok := event.Value.(dashboard.Table)
		if !ok || table.TotalRowsKnown || len(table.Blocks["a"].Rows) != 1 {
			t.Fatalf("row payload = %#v", event.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("rows did not publish while count was blocked")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("count query did not start after rows published")
	}
	close(release)
	select {
	case event := <-events:
		if event.Type != RefreshEventTableMetadata || event.Target != "orders" {
			t.Fatalf("second event = %#v, want table metadata", event)
		}
		table, ok := event.Value.(dashboard.Table)
		if !ok || !table.TotalRowsKnown || table.TotalRows != 42 || len(table.Blocks["a"].Rows) != 1 {
			t.Fatalf("count payload = %#v", event.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("exact count metadata did not publish")
	}
	<-done
}

func TestTargetWorkTableCountFailureRetainsPublishedRows(t *testing.T) {
	metrics := &progressiveTableTargetMetricsStub{
		rows: dashboard.Table{
			Title:          "Orders",
			TotalRowsKnown: false,
			AvailableRows:  dashboard.TableInteractiveRowCap,
			Blocks: map[string]dashboard.TableBlock{
				"a": {Rows: []map[string]any{{"id": "first"}}},
			},
		},
		countErr: errors.New("count unavailable"),
	}
	events := []RefreshEvent{}
	TargetWork(metrics, WorkRequest{
		Plan: command.RefreshPlan{Targets: []command.Target{{
			Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{Table: "orders"},
		}}},
	})(context.Background(), func(event RefreshEvent) bool {
		events = append(events, event)
		return true
	})
	if len(events) != 2 || events[0].Type != RefreshEventTable || events[1].Type != RefreshEventTableCountErr {
		t.Fatalf("events = %#v, want rows then non-blocking count error", events)
	}
	if events[1].Target != "orders" || events[1].Err == nil || events[1].Err.Error() != "count unavailable" {
		t.Fatalf("count error event = %#v", events[1])
	}
}

func TestTargetWorkScrollingWindowRunsRowsOnly(t *testing.T) {
	countStarted := make(chan struct{})
	metrics := &progressiveTableTargetMetricsStub{
		rows: dashboard.Table{
			TotalRowsKnown: false,
			AvailableRows:  dashboard.TableInteractiveRowCap,
			Blocks: map[string]dashboard.TableBlock{
				"b": {Start: dashboard.TableChunkSize, Rows: []map[string]any{{"id": "next"}}},
			},
		},
		countStarted: countStarted,
	}
	events := []RefreshEvent{}
	TargetWork(metrics, WorkRequest{
		Plan: command.RefreshPlan{Command: "table_window", Targets: []command.Target{{
			Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{
				Table: "orders", Block: "b", Start: dashboard.TableChunkSize, Count: dashboard.TableChunkSize,
			},
		}}},
	})(context.Background(), func(event RefreshEvent) bool {
		events = append(events, event)
		return true
	})
	if len(events) != 1 || events[0].Type != RefreshEventTable || events[0].Queries != 1 {
		t.Fatalf("scroll events = %#v, want one row query", events)
	}
	select {
	case <-countStarted:
		t.Fatal("scrolling window unexpectedly queried exact count")
	default:
	}
}

func TestTableMetadataDoesNotCountAsSecondTargetSuccess(t *testing.T) {
	metrics := &progressiveTableTargetMetricsStub{
		rows:  dashboard.Table{TotalRowsKnown: false, AvailableRows: dashboard.TableInteractiveRowCap},
		count: 42,
	}
	summaries := make(chan RefreshSummary, 1)
	coordinator := NewCoordinator(context.Background(), func(RefreshEvent) {})
	defer coordinator.Close()
	coordinator.SetObserver(func(summary RefreshSummary) { summaries <- summary })
	_, err := coordinator.BeginPrepared(func(dashboard.Filters) (RefreshPreparation, error) {
		return RefreshPreparation{
			Command: "initial",
			Targets: []string{"table:orders"},
			Plan: command.RefreshPlan{Targets: []command.Target{{
				Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{Table: "orders", Block: "all"},
			}}},
		}, nil
	}, func(preparation RefreshPreparation) RefreshWork {
		return TargetWork(metrics, WorkRequest{Plan: preparation.Plan.(command.RefreshPlan)})
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case summary := <-summaries:
		if summary.TargetSuccesses != 1 || summary.TargetErrors != 0 || summary.QueryCount != 2 {
			t.Fatalf("summary = %#v, want one successful table target", summary)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh did not complete")
	}
}

func TestSupersededTableCountCannotPublishMetadata(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	metrics := &progressiveTableTargetMetricsStub{
		rows:         dashboard.Table{TotalRowsKnown: false, AvailableRows: dashboard.TableInteractiveRowCap},
		count:        42,
		countStarted: started,
		countRelease: release,
	}
	var mu sync.Mutex
	events := []RefreshEvent{}
	coordinator := NewCoordinator(context.Background(), func(event RefreshEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})
	defer coordinator.Close()
	start := func(work RefreshWork) (Refresh, error) {
		return coordinator.BeginPrepared(func(current dashboard.Filters) (RefreshPreparation, error) {
			return RefreshPreparation{
				Filters: current,
				Command: "initial",
				Targets: []string{"table:orders"},
				Plan: command.RefreshPlan{Targets: []command.Target{{
					Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{Table: "orders", Block: "all"},
				}}},
			}, nil
		}, func(RefreshPreparation) RefreshWork { return work })
	}
	firstWork := TargetWork(metrics, WorkRequest{Plan: command.RefreshPlan{Targets: []command.Target{{
		Kind: command.TargetTable, ID: "orders", TableRequest: dashboard.TableRequest{Table: "orders", Block: "all"},
	}}}})
	first, err := start(firstWork)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first generation count did not start")
	}
	second, err := start(nil)
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	for _, event := range events {
		if event.Generation == first.Generation && event.Type == RefreshEventTableMetadata {
			t.Fatalf("superseded generation published count metadata: %#v", events)
		}
	}
	if second.Generation <= first.Generation {
		t.Fatalf("generations first=%d second=%d", first.Generation, second.Generation)
	}
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

func TestTargetWorkPartitionsIncompatibleSingleValueScopesAndIsolatesFailures(t *testing.T) {
	metrics := &batchTargetMetrics{
		report: reportdef.Dashboard{Visuals: map[string]reportdef.Visual{
			"orders":   {Shape: "single_value", Query: reportdef.VisualQuery{Table: "orders"}},
			"revenue":  {Shape: "single_value", Query: reportdef.VisualQuery{Table: "orders"}},
			"users":    {Shape: "single_value", Query: reportdef.VisualQuery{Table: "users"}},
			"sessions": {Shape: "single_value", Query: reportdef.VisualQuery{Table: "users"}},
		}},
		failBatch: map[string]error{"users,sessions": errors.New("users unavailable")},
	}
	events := []RefreshEvent{}
	TargetWork(metrics, WorkRequest{
		DashboardID: "dash",
		Plan: command.RefreshPlan{Targets: []command.Target{
			{Kind: command.TargetVisual, ID: "orders"},
			{Kind: command.TargetVisual, ID: "users"},
			{Kind: command.TargetVisual, ID: "revenue"},
			{Kind: command.TargetVisual, ID: "sessions"},
		}},
	})(context.Background(), func(event RefreshEvent) bool {
		events = append(events, event)
		return true
	})

	if got := metrics.batches; len(got) != 2 || strings.Join(got[0], ",") != "orders,revenue" || strings.Join(got[1], ",") != "users,sessions" {
		t.Fatalf("compatibility batches = %#v", got)
	}
	successes, failures := 0, 0
	for _, event := range events {
		if event.Type == RefreshEventVisual {
			successes++
		}
		if event.Type == RefreshEventTargetError {
			failures++
		}
	}
	if successes != 2 || failures != 2 {
		t.Fatalf("successes=%d failures=%d events=%#v", successes, failures, events)
	}
}

func TestTargetWorkPartitionsKPIsByApplicableGovernedFilterScope(t *testing.T) {
	visuals := map[string]reportdef.Visual{}
	for _, id := range []string{"filtered_count", "filtered_average", "global_count", "global_average"} {
		visuals[id] = reportdef.Visual{Shape: "single_value", Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "rating_count"}}}}
	}
	metrics := &batchTargetMetrics{
		report: reportdef.Dashboard{
			Visuals: visuals,
			Filters: map[string]reportdef.FilterDefinition{
				"decade": {
					Dimension: "release_decade",
					Targets:   reportdef.FilterTargets{Visuals: []string{"filtered_count", "filtered_average"}},
				},
			},
		},
		model: &semanticmodel.Model{
			Measures: map[string]semanticmodel.MetricMeasure{"rating_count": {Fact: "ratings"}},
			Dimensions: map[string]semanticmodel.SemanticDimension{
				"release_decade": {Bindings: map[string]semanticmodel.DimensionBinding{"ratings": {Field: "movies.release_decade"}}},
			},
		},
	}
	TargetWork(metrics, WorkRequest{
		DashboardID: "dash",
		Filters: dashboard.Filters{Controls: map[string]dashboard.FilterControl{
			"decade": {Type: "multi_select", Operator: "in", Values: []string{"1990s"}},
		}},
		Plan: command.RefreshPlan{Targets: []command.Target{
			{Kind: command.TargetVisual, ID: "filtered_count"},
			{Kind: command.TargetVisual, ID: "global_count"},
			{Kind: command.TargetVisual, ID: "filtered_average"},
			{Kind: command.TargetVisual, ID: "global_average"},
		}},
	})(context.Background(), func(RefreshEvent) bool { return true })

	if got := metrics.batches; len(got) != 2 || strings.Join(got[0], ",") != "filtered_count,filtered_average" || strings.Join(got[1], ",") != "global_count,global_average" {
		t.Fatalf("governed filter-scope batches = %#v", got)
	}
}

func TestTargetWorkSchedulesKPIBatchesBeforeAggregateVisualsAndTables(t *testing.T) {
	metrics := &batchTargetMetrics{report: reportdef.Dashboard{Visuals: map[string]reportdef.Visual{
		"revenue": {Shape: "single_value"},
		"orders":  {Shape: "single_value"},
		"trend":   {Shape: "category"},
	}}}
	TargetWork(metrics, WorkRequest{
		DashboardID: "dash",
		Plan: command.RefreshPlan{Targets: []command.Target{
			{Kind: command.TargetTable, ID: "orders_table", TableRequest: dashboard.TableRequest{Table: "orders_table"}},
			{Kind: command.TargetVisual, ID: "trend"},
			{Kind: command.TargetVisual, ID: "revenue"},
			{Kind: command.TargetVisual, ID: "orders"},
		}},
	})(context.Background(), func(RefreshEvent) bool { return true })

	want := []string{"visuals:revenue,orders", "visual:trend", "table:orders_table"}
	if strings.Join(metrics.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("execution order = %#v, want %#v", metrics.calls, want)
	}
}

func TestTargetWorkMovieLensLikePlanHasDeterministicPhysicalQueryBudget(t *testing.T) {
	visuals := map[string]reportdef.Visual{}
	plan := command.RefreshPlan{}
	for _, id := range []string{"rating_count", "average_rating", "active_users", "rated_movies", "tags_per_rating"} {
		visuals[id] = reportdef.Visual{Shape: "single_value"}
		plan.Targets = append(plan.Targets, command.Target{Kind: command.TargetVisual, ID: id})
	}
	for _, id := range []string{"activity_by_month", "ratings_by_month", "rating_distribution"} {
		visuals[id] = reportdef.Visual{Shape: "category"}
		plan.Targets = append(plan.Targets, command.Target{Kind: command.TargetVisual, ID: id})
	}
	plan.Targets = append(plan.Targets, command.Target{Kind: command.TargetTable, ID: "movie_table", TableRequest: dashboard.TableRequest{Table: "movie_table"}})
	metrics := &batchTargetMetrics{report: reportdef.Dashboard{Visuals: visuals}, batchQueryCount: 1}
	queryCount := 0
	TargetWork(metrics, WorkRequest{DashboardID: "ratings-overview", Plan: plan})(context.Background(), func(event RefreshEvent) bool {
		queryCount += event.Queries
		return true
	})

	if len(metrics.batches) != 1 || len(metrics.batches[0]) != 5 {
		t.Fatalf("KPI batches = %#v, want one five-consumer query", metrics.batches)
	}
	if queryCount != 5 {
		t.Fatalf("physical query count = %d, want 5 (one KPI bundle, three charts, one table)", queryCount)
	}
	wantCalls := "visuals:rating_count,average_rating,active_users,rated_movies,tags_per_rating|visual:activity_by_month|visual:ratings_by_month|visual:rating_distribution|table:movie_table"
	if got := strings.Join(metrics.calls, "|"); got != wantCalls {
		t.Fatalf("deterministic target schedule = %q, want %q", got, wantCalls)
	}
}

func TestTargetWorkFusesMovieLensLikeSingleFactKPIsAndChartsIntoOnePhysicalQuery(t *testing.T) {
	visuals := map[string]reportdef.Visual{
		"rating_count":        {Shape: "single_value", Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "rating_count"}}}},
		"average_rating":      {Shape: "single_value", Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "average_rating"}}}},
		"ratings_by_month":    {Shape: "category_value", Query: reportdef.VisualQuery{Dimensions: []reportdef.FieldRef{{Field: "ratings.rating_month"}}, Measures: []reportdef.FieldRef{{Field: "rating_count"}}}},
		"rating_distribution": {Shape: "category_value", Query: reportdef.VisualQuery{Dimensions: []reportdef.FieldRef{{Field: "ratings.rating_bucket"}}, Measures: []reportdef.FieldRef{{Field: "rating_count"}}}},
	}
	model := &semanticmodel.Model{Tables: map[string]semanticmodel.Table{"ratings": {}}, Measures: map[string]semanticmodel.MetricMeasure{
		"rating_count": {Fact: "ratings"}, "average_rating": {Fact: "ratings"},
	}}
	metrics := &bundleTargetMetrics{batchTargetMetrics: batchTargetMetrics{report: reportdef.Dashboard{Visuals: visuals}, model: model}}
	queryCount := 0
	TargetWork(metrics, WorkRequest{DashboardID: "ratings-overview", Plan: command.RefreshPlan{Targets: []command.Target{
		{Kind: command.TargetVisual, ID: "rating_count"}, {Kind: command.TargetVisual, ID: "average_rating"}, {Kind: command.TargetVisual, ID: "ratings_by_month"}, {Kind: command.TargetVisual, ID: "rating_distribution"},
	}}})(context.Background(), func(event RefreshEvent) bool { queryCount += event.Queries; return true })
	if len(metrics.bundleCalls) != 1 || strings.Join(metrics.bundleCalls[0], ",") != "rating_count,average_rating,ratings_by_month,rating_distribution" {
		t.Fatalf("bundle calls = %#v", metrics.bundleCalls)
	}
	if queryCount != 1 {
		t.Fatalf("physical queries = %d, want 1", queryCount)
	}
}

func TestTargetWorkBatchesCompatibleScalarsAcrossFactsBeforeFactBundles(t *testing.T) {
	visuals := map[string]reportdef.Visual{
		"rating_count": {
			Shape: "single_value",
			Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "rating_count"}}},
		},
		"tags_per_rating": {
			Shape: "single_value",
			Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "tags_per_rating"}}},
		},
	}
	model := &semanticmodel.Model{
		Tables: map[string]semanticmodel.Table{"ratings": {}, "tags": {}},
		Measures: map[string]semanticmodel.MetricMeasure{
			"rating_count": {Fact: "ratings", Aggregation: "count"},
			"tag_count":    {Fact: "tags", Aggregation: "count"},
		},
		Metrics: map[string]semanticmodel.Metric{
			"tags_per_rating": {Expression: "safe_divide(${tag_count}, ${rating_count})"},
		},
	}
	metrics := &bundleTargetMetrics{batchTargetMetrics: batchTargetMetrics{
		report: reportdef.Dashboard{Visuals: visuals},
		model:  model,
	}}
	TargetWork(metrics, WorkRequest{DashboardID: "ratings-overview", Plan: command.RefreshPlan{Targets: []command.Target{
		{Kind: command.TargetVisual, ID: "rating_count"},
		{Kind: command.TargetVisual, ID: "tags_per_rating"},
	}}})(context.Background(), func(RefreshEvent) bool { return true })

	if len(metrics.batches) != 1 || strings.Join(metrics.batches[0], ",") != "rating_count,tags_per_rating" {
		t.Fatalf("KPI batches = %#v, want one model-scoped cross-fact batch", metrics.batches)
	}
	if len(metrics.bundleCalls) != 0 {
		t.Fatalf("fact bundle calls = %#v, want none", metrics.bundleCalls)
	}
}

func TestTargetWorkGroupsCrossFactScalarWithCompleteAdditiveGroupedSource(t *testing.T) {
	visuals := map[string]reportdef.Visual{
		"rating_count": {
			Shape: "single_value",
			Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "rating_count"}}},
		},
		"tags_per_rating": {
			Shape: "single_value",
			Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "tags_per_rating"}}},
		},
		"activity_by_month": {
			Shape: "category_multi_measure",
			Query: reportdef.VisualQuery{
				Time:     reportdef.QueryTime{Field: "activity_date", Grain: "month"},
				Measures: []reportdef.FieldRef{{Field: "rating_count"}, {Field: "tag_count"}},
				Limit:    360,
			},
		},
		"tags_per_rating_by_decade": {
			Shape: "category_value",
			Query: reportdef.VisualQuery{Dimensions: []reportdef.FieldRef{{Field: "release_decade"}}, Measures: []reportdef.FieldRef{{Field: "tags_per_rating"}}},
		},
	}
	model := &semanticmodel.Model{
		Tables: map[string]semanticmodel.Table{"ratings": {}, "tags": {}},
		Measures: map[string]semanticmodel.MetricMeasure{
			"rating_count": {Fact: "ratings", Aggregation: "count"},
			"tag_count":    {Fact: "tags", Aggregation: "count"},
		},
		Metrics: map[string]semanticmodel.Metric{
			"tags_per_rating": {Expression: "safe_divide(${tag_count}, ${rating_count})"},
		},
	}
	metrics := &bundleTargetMetrics{batchTargetMetrics: batchTargetMetrics{report: reportdef.Dashboard{Visuals: visuals}, model: model}}
	TargetWork(metrics, WorkRequest{DashboardID: "ratings-overview", Plan: command.RefreshPlan{Targets: []command.Target{
		{Kind: command.TargetVisual, ID: "rating_count"},
		{Kind: command.TargetVisual, ID: "tags_per_rating"},
		{Kind: command.TargetVisual, ID: "activity_by_month"},
		{Kind: command.TargetVisual, ID: "tags_per_rating_by_decade"},
	}}})(context.Background(), func(RefreshEvent) bool { return true })

	if len(metrics.bundleCalls) != 1 || strings.Join(metrics.bundleCalls[0], ",") != "rating_count,tags_per_rating,activity_by_month,tags_per_rating_by_decade" {
		t.Fatalf("projection bundle calls = %#v", metrics.bundleCalls)
	}
}

func TestTargetWorkProjectionDenialKeepsGroupedSourceAndScopesScalarError(t *testing.T) {
	visuals := map[string]reportdef.Visual{
		"ratio": {Shape: "single_value", Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "ratio"}}}},
		"trend": {Shape: "category_multi_measure", Query: reportdef.VisualQuery{
			Time: reportdef.QueryTime{Field: "activity_date", Grain: "month"}, Measures: []reportdef.FieldRef{{Field: "left_count"}, {Field: "right_count"}},
		}},
	}
	model := &semanticmodel.Model{
		Measures: map[string]semanticmodel.MetricMeasure{
			"left_count": {Fact: "left", Aggregation: "count"}, "right_count": {Fact: "right", Aggregation: "count"},
		},
		Metrics: map[string]semanticmodel.Metric{"ratio": {Expression: "safe_divide(${left_count}, ${right_count})"}},
	}
	metrics := &bundleTargetMetrics{
		batchTargetMetrics: batchTargetMetrics{report: reportdef.Dashboard{Visuals: visuals}, model: model, visualErrors: map[string]error{"ratio": errors.New("metric denied")}},
		bundleErr:          &dataquery.BundleBranchError{ID: "ratio", Err: errors.New("metric denied")},
	}
	events := []RefreshEvent{}
	TargetWork(metrics, WorkRequest{DashboardID: "dashboard", Plan: command.RefreshPlan{Targets: []command.Target{
		{Kind: command.TargetVisual, ID: "ratio"}, {Kind: command.TargetVisual, ID: "trend"},
	}}})(context.Background(), func(event RefreshEvent) bool {
		events = append(events, event)
		return true
	})

	visualsPublished, scalarErrors := 0, 0
	for _, event := range events {
		if event.Type == RefreshEventVisual && event.Target == "trend" {
			visualsPublished++
		}
		if event.Type == RefreshEventTargetError && event.Target == "visual:ratio" {
			scalarErrors++
		}
	}
	if visualsPublished != 1 || scalarErrors != 1 {
		t.Fatalf("events = %#v, want source success and scalar-scoped denial", events)
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
