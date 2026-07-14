package datastar

import (
	"errors"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
	dashboardstream "github.com/Yacobolo/libredash/internal/dashboard/stream"
)

func TestPatchKeys(t *testing.T) {
	patch := DashboardPatch(dashboard.Patch{
		Filters:       dashboard.Filters{}.WithDefaults(),
		FilterOptions: map[string][]dashboard.FilterOption{"state": {{Value: "SP", Label: "SP"}}},
		Status:        dashboard.Status{Loading: false},
		Visuals:       map[string]dashboard.Visual{"orders": {Title: "Orders"}},
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
	tables, ok := tablePatch["tables"].(map[string]dashboard.Table)
	if !ok || tables["orders"].Title != "Orders" {
		t.Fatalf("table patch = %#v", tablePatch)
	}

	status, ok := LoadingPatch(".data")["status"].(map[string]any)
	if !ok || status["loading"] != true || status["dataDirectory"] != ".data" {
		t.Fatalf("loading patch = %#v", LoadingPatch(".data"))
	}
}

func TestRefreshCompletePreservesFatalError(t *testing.T) {
	patch := RefreshEventPatch(dashboardstream.RefreshEvent{
		Type: dashboardstream.RefreshEventComplete, RefreshID: "refresh-1", Generation: 1, Err: errors.New("refresh failed"),
	}, ".data")
	status, ok := patch["status"].(map[string]any)
	if !ok || status["loading"] != false || status["error"] != "refresh failed" {
		t.Fatalf("terminal patch = %#v", patch)
	}
}

func TestTableMetadataUpdatesDataWithoutChangingComponentStatus(t *testing.T) {
	table := dashboard.Table{Title: "Orders", Cardinality: dashboard.ExactCardinality(42)}
	patch := RefreshEventPatch(dashboardstream.RefreshEvent{
		Type: dashboardstream.RefreshEventTableMetadata, Target: "orders", Value: table,
	}, ".data")
	tables, ok := patch["tables"].(map[string]dashboard.Table)
	total, exact := tables["orders"].Cardinality.ExactValue()
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
	}, ".data")
	status, ok := patch["status"].(map[string]any)
	if !ok || status["setupRequired"] != true || status["error"] != "source data is missing" {
		t.Fatalf("terminal patch = %#v", patch)
	}
}
