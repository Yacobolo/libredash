package app

import (
	"testing"

	dashboardstream "github.com/Yacobolo/leapview/internal/dashboard/stream"
)

func TestDashboardTelemetryObservesAcceptedProgressiveTargetEvents(t *testing.T) {
	telemetry := newHTTPTelemetry()
	for _, event := range []dashboardstream.RefreshEvent{
		{Type: dashboardstream.RefreshEventVisual, Target: "revenue"},
		{Type: dashboardstream.RefreshEventTable, Target: "orders"},
		{Type: dashboardstream.RefreshEventTableCountErr, Target: "orders"},
		{Type: dashboardstream.RefreshEventFilterOptions, Target: "state"},
		{Type: dashboardstream.RefreshEventTargetError, Target: "visual:broken"},
		{Type: dashboardstream.RefreshEventTargetError, Target: "refresh"},
		{Type: dashboardstream.RefreshEventComplete},
	} {
		telemetry.dashboardRefreshEventObserved(event)
	}

	want := map[string]float64{
		"filter_options:success": 1,
		"refresh:error":          1,
		"visual_count:error":     1,
		"visual:error":           1,
		"visual:success":         2,
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
