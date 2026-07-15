package datastar

import (
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
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

	status, ok := LoadingPatch()["status"].(map[string]any)
	if !ok || status["loading"] != true {
		t.Fatalf("loading patch = %#v", LoadingPatch())
	}
	if _, exists := status["dataDirectory"]; exists {
		t.Fatalf("loading patch exposes dataDirectory: %#v", status)
	}
}
