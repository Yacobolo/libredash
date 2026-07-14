package report

import (
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

func TestDashboardRejectsUnknownTableCardinalityPolicy(t *testing.T) {
	dashboardDefinition := Dashboard{
		ID:            "commerce",
		Title:         "Commerce",
		SemanticModel: "commerce",
		Visuals: map[string]Visual{
			"orders": {Kind: "kpi", Title: "Orders", Shape: "single_value", Query: VisualQuery{Measures: []FieldRef{{Field: "order_count"}}}},
		},
		Tables: map[string]TableVisual{
			"orders": {Title: "Orders", Cardinality: "automatic", Query: TableQuery{Table: "orders", Fields: []string{"order_id"}}},
		},
		Pages: []dashboard.Page{{ID: "overview", Title: "Overview", Visuals: []dashboard.PageVisual{{Kind: "kpi", Visual: "orders"}, {Kind: "table", Table: "orders"}}}},
	}
	err := dashboardDefinition.ValidateContract()
	if err == nil || !strings.Contains(err.Error(), "unsupported cardinality") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestTableCardinalityDefaultsToBounded(t *testing.T) {
	if got := (TableVisual{}).CardinalityOrDefault(); got != TableCardinalityBounded {
		t.Fatalf("default cardinality = %q", got)
	}
}
