package datastar

import (
	"errors"
	"slices"
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboardstream "github.com/Yacobolo/leapview/internal/dashboard/stream"
)

func TestPatchKeys(t *testing.T) {
	patch := DashboardPatch(dashboard.Patch{
		Filters:       dashboard.Filters{}.WithDefaults(),
		FilterOptions: map[string][]dashboard.FilterOption{"state": {{Value: "SP", Label: "SP"}}},
		Status:        dashboard.Status{Loading: false},
		Visuals:       map[string]dashboard.Visual{"orders": {ID: "orders", Type: "bar", Title: "Orders"}},
	})

	for _, key := range []string{"filters", "filterOptions", "status", "visuals"} {
		if _, ok := patch[key]; !ok {
			t.Fatalf("dashboard patch missing key %q: %#v", key, patch)
		}
	}
	if _, ok := patch["tables"]; ok {
		t.Fatalf("dashboard patch should not include tables: %#v", patch)
	}
	if _, ok := patch["kpis"]; ok {
		t.Fatalf("dashboard patch should not include legacy kpis: %#v", patch)
	}

	tablePatch := TablePatch("orders", dashboard.Table{Title: "Orders"})
	visuals, ok := tablePatch["visuals"].(map[string]dashboard.TabularVisual)
	if !ok || visuals["orders"].Title != "Orders" || visuals["orders"].Type != "table" {
		t.Fatalf("table patch = %#v", tablePatch)
	}

	status, ok := LoadingPatch()["status"].(map[string]any)
	if !ok || status["loading"] != true {
		t.Fatalf("loading patch = %#v", LoadingPatch())
	}
	if _, exists := status["dataDirectory"]; exists {
		t.Fatalf("loading patch exposes dataDirectory: %#v", status)
	}
	progress, ok := status["progressPercent"].(*float64)
	if !ok || progress == nil || *progress != 0 {
		t.Fatalf("loading progress = %#v, want 0", status["progressPercent"])
	}
}

func TestRefreshCompletePreservesFatalError(t *testing.T) {
	patch := RefreshEventPatch(dashboardstream.RefreshEvent{
		Type: dashboardstream.RefreshEventComplete, RefreshID: "refresh-1", Generation: 1, Err: errors.New("refresh failed"),
	})
	status, ok := patch["status"].(map[string]any)
	if !ok || status["loading"] != false || status["error"] != "refresh failed" {
		t.Fatalf("terminal patch = %#v", patch)
	}
}

func TestRefreshProgressIsBackendOwned(t *testing.T) {
	for _, test := range []struct {
		name    string
		event   dashboardstream.RefreshEvent
		percent *float64
	}{
		{
			name: "planning",
			event: dashboardstream.RefreshEvent{
				Type: dashboardstream.RefreshEventStart, Generation: 4,
			},
			percent: testPercent(0),
		},
		{
			name: "executing",
			event: dashboardstream.RefreshEvent{
				Type: dashboardstream.RefreshEventProgress, Generation: 4,
				ProgressPercent: testPercent(50),
			},
			percent: testPercent(50),
		},
		{
			name: "complete",
			event: dashboardstream.RefreshEvent{
				Type: dashboardstream.RefreshEventComplete, Generation: 4,
				ProgressPercent: testPercent(100),
			},
			percent: testPercent(100),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			patch := RefreshEventPatch(test.event)
			status, ok := patch["status"].(map[string]any)
			progress, progressOK := status["progressPercent"].(*float64)
			if !ok || !progressOK || !equalOptionalPercent(progress, test.percent) {
				t.Fatalf("progress patch = %#v", patch)
			}
			if _, legacy := status["progress"]; legacy {
				t.Fatalf("progress patch retains legacy work counters: %#v", patch)
			}
		})
	}
}

func testPercent(value float64) *float64 { return &value }

func equalOptionalPercent(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func TestRefreshEventEnvelopeCarriesExplicitDeliveryMetadata(t *testing.T) {
	tests := []struct {
		name          string
		event         dashboardstream.RefreshEvent
		wantBoundary  bool
		wantGroup     string
		wantMergeRoot string
	}{
		{
			name: "progress boundary",
			event: dashboardstream.RefreshEvent{
				Type: dashboardstream.RefreshEventProgress, RefreshID: "refresh-9", Generation: 9,
			},
			wantBoundary: true,
		},
		{
			name: "visual result batch",
			event: dashboardstream.RefreshEvent{
				Type: dashboardstream.RefreshEventVisual, RefreshID: "refresh-9", Generation: 9, Target: "rating_count", Value: dashboard.Visual{ID: "rating_count", Type: "bar"},
			},
			wantGroup:     "dashboard-results",
			wantMergeRoot: "visuals",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envelope := RefreshEventEnvelope(test.event)
			if envelope.Delivery.Generation != 9 || envelope.Delivery.Boundary != test.wantBoundary || envelope.Delivery.CoalesceGroup != test.wantGroup {
				t.Fatalf("delivery metadata = %#v", envelope.Delivery)
			}
			if envelope.Trace.Origin != "dashboard.refresh" || envelope.Trace.CorrelationID != "refresh-9" {
				t.Fatalf("trace metadata = %#v", envelope.Trace)
			}
			if test.wantMergeRoot != "" && !slices.Contains(envelope.Delivery.MergeRoots, test.wantMergeRoot) {
				t.Fatalf("merge roots = %#v", envelope.Delivery.MergeRoots)
			}
		})
	}
}

func TestTableMetadataUpdatesDataWithoutChangingComponentStatus(t *testing.T) {
	table := dashboard.Table{Title: "Orders", Cardinality: dashboard.ExactCardinality(42)}
	patch := RefreshEventPatch(dashboardstream.RefreshEvent{
		Type: dashboardstream.RefreshEventTableMetadata, Target: "orders", Value: table,
	})
	visuals, ok := patch["visuals"].(map[string]dashboard.TabularVisual)
	total, exact := visuals["orders"].Cardinality.ExactValue()
	if !ok || total != 42 || !exact {
		t.Fatalf("metadata patch = %#v", patch)
	}
	if _, ok := patch["componentStatus"]; ok {
		t.Fatalf("metadata patch changed target status: %#v", patch)
	}
}

type setupRequiredPatchError struct{}

func (setupRequiredPatchError) Error() string       { return "source data is missing" }
func (setupRequiredPatchError) SetupRequired() bool { return true }

func TestRefreshCompleteMarksSetupRequiredErrors(t *testing.T) {
	patch := RefreshEventPatch(dashboardstream.RefreshEvent{
		Type: dashboardstream.RefreshEventComplete, RefreshID: "refresh-1", Generation: 1, Err: setupRequiredPatchError{},
	})
	status, ok := patch["status"].(map[string]any)
	if !ok || status["setupRequired"] != true || status["error"] != "source data is missing" {
		t.Fatalf("terminal patch = %#v", patch)
	}
}
