package app

import (
	"context"
	"testing"
	"time"

	dashboardstream "github.com/Yacobolo/leapview/internal/dashboard/stream"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	"github.com/Yacobolo/leapview/internal/workload"
)

func TestWorkloadTelemetryUsesBoundedLabelsAndBalancesGauges(t *testing.T) {
	telemetry := newHTTPTelemetry()
	controller, err := workload.New(workload.Config{MaxRunning: 1, MaximumQueued: 1, Classes: map[workload.Class]workload.Policy{
		workload.Interactive: {MaximumRunning: 1, MaximumQueued: 1, MaximumQueuedPerWorkspace: 1, QueueTimeout: time.Second},
	}}, workload.WithObserver(telemetry))
	if err != nil {
		t.Fatal(err)
	}
	lease, err := controller.Acquire(context.Background(), workload.Request{Class: workload.Interactive, WorkspaceID: "sales", Operation: "request-id-must-not-be-a-label"})
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()

	families, err := telemetry.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"leapview_workload_running": false, "leapview_workload_queued": false,
		"leapview_workload_borrowed": false, "leapview_workload_admissions_total": false,
		"leapview_workload_queue_wait_seconds": false, "leapview_workload_execution_duration_seconds": false,
	}
	for _, family := range families {
		if _, ok := want[family.GetName()]; !ok {
			continue
		}
		want[family.GetName()] = true
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "operation" || label.GetName() == "request_id" || label.GetValue() == "request-id-must-not-be-a-label" {
					t.Fatalf("unbounded workload metric label: %s=%s", label.GetName(), label.GetValue())
				}
			}
			if family.GetName() == "leapview_workload_running" && metric.Gauge.GetValue() != 0 {
				t.Fatalf("running gauge = %v", metric.Gauge.GetValue())
			}
			if family.GetName() == "leapview_workload_queued" && metric.Gauge.GetValue() != 0 {
				t.Fatalf("queued gauge = %v", metric.Gauge.GetValue())
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("metric %s was not registered", name)
		}
	}
}

func TestDashboardTelemetryObservesAcceptedProgressiveTargetEvents(t *testing.T) {
	telemetry := newHTTPTelemetry()
	for _, event := range []dashboardstream.RefreshEvent{
		{Type: dashboardstream.RefreshEventVisual, Target: "revenue", Value: visualizationir.VisualizationEnvelope{
			Spec:      visualizationir.VisualizationSpec{Value: &visualizationir.KPIVisualizationSpec{}},
			DataState: visualizationir.VisualizationDataState{Value: &visualizationir.InlineVisualizationDataState{Kind: "inline", Datasets: []visualizationir.VisualizationInlineDataset{{Rows: [][]any{{1}}}}}},
		}},
		{Type: dashboardstream.RefreshEventVisual, Target: "orders", Value: visualizationir.VisualizationEnvelope{
			Spec:      visualizationir.VisualizationSpec{Value: &visualizationir.TableVisualizationSpec{Kind: "table"}},
			DataState: visualizationir.VisualizationDataState{Value: &visualizationir.WindowedVisualizationDataState{Kind: "windowed", AvailableRows: 1, Cardinality: visualizationir.VisualizationCardinality{Kind: visualizationir.VisualizationCardinalityKindExact, Count: int64Pointer(1)}, Blocks: map[string]visualizationir.VisualizationWindowBlock{"a": {Rows: [][]any{{"o1"}}}}}},
		}},
		{Type: dashboardstream.RefreshEventTargetError, Target: "visual:broken"},
		{Type: dashboardstream.RefreshEventTargetError, Target: "refresh"},
		{Type: dashboardstream.RefreshEventComplete},
	} {
		telemetry.dashboardRefreshEventObserved(event)
	}

	want := map[string]float64{
		"refresh:error":  1,
		"visual:error":   1,
		"visual:success": 2,
	}
	got := dashboardTargetMetricValues(t, telemetry)
	if len(got) != len(want) {
		t.Fatalf("target outcome metric series = %#v, want %#v", got, want)
	}
	for labels, count := range want {
		if got[labels] != count {
			t.Fatalf("target outcome %s = %v, want %v (all %#v)", labels, got[labels], count, got)
		}
	}
	for _, name := range []string{"leapview_visualization_frame_rows", "leapview_visualization_frame_size_bytes", "leapview_visualization_cardinality"} {
		if got := histogramSampleCount(t, telemetry, name); got != 2 {
			t.Fatalf("%s sample count = %d, want 2", name, got)
		}
	}
}

func int64Pointer(value int64) *int64 { return &value }

func histogramSampleCount(t *testing.T, telemetry *httpTelemetry, name string) uint64 {
	t.Helper()
	families, err := telemetry.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var count uint64
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			count += metric.Histogram.GetSampleCount()
		}
	}
	return count
}

func TestDashboardHTTPWiresProgressiveObservers(t *testing.T) {
	handler := New(nil).dashboardHTTP()
	if handler.RefreshEventObserved == nil {
		t.Fatal("dashboard refresh event observer is not configured")
	}
	if handler.CacheObserved == nil {
		t.Fatal("dashboard cache observer is not configured")
	}
}

func dashboardTargetMetricValues(t *testing.T, telemetry *httpTelemetry) map[string]float64 {
	t.Helper()
	families, err := telemetry.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "leapview_dashboard_target_outcomes_total" {
			continue
		}
		for _, metric := range family.Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			values[labels["kind"]+":"+labels["outcome"]] = metric.Counter.GetValue()
		}
	}
	return values
}

func TestDashboardTelemetryUsesBoundedLabelsAndRecordsRefreshLifecycle(t *testing.T) {
	telemetry := newHTTPTelemetry()
	telemetry.dashboardRefreshStarted("select")
	telemetry.dashboardRefreshFinished(dashboardstream.RefreshSummary{
		Command:           "select",
		Outcome:           "complete",
		CancellationCount: 2,
		StageTimingsMs: map[string]float64{
			"endToEnd": 42,
			"planning": 3,
		},
	})
	telemetry.dashboardCacheObserved("hit")
	telemetry.dashboardCacheObserved("coalesced")
	telemetry.dashboardTargetObserved("visual", "success")

	metricValue := func(name string) float64 {
		families, err := telemetry.registry.Gather()
		if err != nil {
			t.Fatal(err)
		}
		for _, family := range families {
			if family.GetName() != name || len(family.Metric) == 0 {
				continue
			}
			metric := family.Metric[0]
			if metric.Gauge != nil {
				return metric.Gauge.GetValue()
			}
			if metric.Counter != nil {
				return metric.Counter.GetValue()
			}
		}
		t.Fatalf("metric %q not found", name)
		return 0
	}
	if got := metricValue("leapview_dashboard_refreshes_in_flight"); got != 0 {
		t.Fatalf("refresh in flight = %v, want 0", got)
	}
	if got := metricValue("leapview_dashboard_refresh_cancellations_total"); got != 2 {
		t.Fatalf("refresh cancellations = %v, want 2", got)
	}
	if got := metricValue("leapview_dashboard_cache_outcomes_total"); got != 1 {
		t.Fatalf("first cache outcome series = %v, want 1", got)
	}
	if got := metricValue("leapview_dashboard_target_outcomes_total"); got != 1 {
		t.Fatalf("visual target successes = %v, want 1", got)
	}

	for name, raw := range map[string]string{
		"command": "select:dashboard-tenant-123",
		"outcome": "failed-for-user-123",
		"stage":   "target:customer-123",
		"cache":   "hit:customer-123",
		"kind":    "visual:customer-123",
	} {
		var got string
		switch name {
		case "command":
			got = dashboardCommandLabel(raw)
		case "outcome":
			got = dashboardOutcomeLabel(raw)
		case "stage":
			got = dashboardStageLabel(raw)
		case "cache":
			got = dashboardCacheLabel(raw)
		case "kind":
			got = dashboardTargetKindLabel(raw)
		}
		if got != "other" {
			t.Fatalf("%s label for %q = %q, want other", name, raw, got)
		}
	}
	if got := dashboardCacheLabel("coalesced"); got != "coalesced" {
		t.Fatalf("coalesced cache label = %q, want coalesced", got)
	}
	if got := dashboardStageLabel("targetWorkSum"); got != "target_work_sum" {
		t.Fatalf("target work sum stage label = %q, want target_work_sum", got)
	}
	if got := dashboardStageLabel("targetCriticalPath"); got != "target_critical_path" {
		t.Fatalf("target critical path stage label = %q, want target_critical_path", got)
	}
}
