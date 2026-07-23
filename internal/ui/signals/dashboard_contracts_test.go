package signals

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
)

func TestDashboardContractConversionsPreserveJSON(t *testing.T) {
	t.Parallel()

	filters := dashboard.Filters{
		Controls:          map[string]dashboard.FilterControl{"state": {Type: "multi_select", Operator: "in", Values: []string{"SP"}}},
		Selections:        []dashboard.InteractionSelection{{ID: "visual:orders:point", SourceKind: "visual", SourceID: "orders", InteractionKind: "point", Label: "42", Order: 1, Entries: []dashboard.InteractionSelectionEntry{{Label: "42", Mappings: []dashboard.InteractionSelectionMapping{{Field: "ratings.rating_bucket", Fact: "ratings", Value: float64(42), Label: "Rating"}}}}}},
		SpatialSelections: []dashboard.SpatialInteractionSelection{},
	}
	filterConfig := []dashboarddefinition.FilterConfig{{
		ID: "state",
		FilterDefinition: dashboarddefinition.FilterDefinition{
			Type: "multi_select", Label: "State", Description: "Order state", Dimension: "orders.state", Fact: "orders", Custom: true,
			Default: dashboarddefinition.FilterDefault{Operator: "in", Values: []string{"SP"}}, Operator: "in", DefaultOperator: "in", Operators: []string{"in"},
			Options: []dashboarddefinition.FilterOption{{Value: "SP", Label: "Sao Paulo"}}, Presets: []dashboarddefinition.FilterPreset{{Value: "recent", Label: "Recent", From: "2026-01-01", To: "2026-12-31", RelativeDays: 30}},
			Values: dashboarddefinition.FilterValues{Source: "orders.state", Limit: 100}, URLParam: "state", FromURLParam: "from", ToURLParam: "to", OperatorURLParam: "op",
			Targets: dashboarddefinition.FilterTargets{Visuals: []string{"orders", "orders_table"}},
		},
	}}

	assertSameJSON(t, filters, DashboardFiltersFromDashboard(filters))
	convertedFilters := ReportFilterConfigsFromReport(filterConfig)
	if convertedFilters[0].Targets == nil || convertedFilters[0].Targets.Visuals == nil || !reflect.DeepEqual(*convertedFilters[0].Targets.Visuals, []string{"orders", "orders_table"}) {
		t.Fatalf("filter targets = %#v", convertedFilters[0].Targets)
	}
}

func assertSameJSON(t *testing.T, left, right any) {
	t.Helper()
	leftJSON, err := json.Marshal(left)
	if err != nil {
		t.Fatalf("marshal source: %v", err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	var leftValue, rightValue any
	if err := json.Unmarshal(leftJSON, &leftValue); err != nil {
		t.Fatalf("decode source: %v", err)
	}
	if err := json.Unmarshal(rightJSON, &rightValue); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	if !reflect.DeepEqual(leftValue, rightValue) {
		t.Fatalf("JSON differs:\nsource:   %s\ncontract: %s", leftJSON, rightJSON)
	}
}
