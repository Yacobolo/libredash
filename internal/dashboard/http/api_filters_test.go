package http

import (
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
)

func TestDashboardQueryFiltersDecodesVersionedAppliedStateAndIndependentSelections(t *testing.T) {
	key := dashboardfilter.BindingKey("dashboard", dashboardfilter.ScopePage, "overview", "state")
	definition := dashboarddefinition.Definition{
		FilterDefinitions: map[string]dashboardfilter.Definition{
			"state": {
				ValueKind: dashboardfilter.ValueString,
				Predicates: []dashboardfilter.PredicatePolicy{{
					Kind: dashboardfilter.ExpressionSet, Operators: []dashboardfilter.Operator{dashboardfilter.OperatorIn},
				}},
			},
		},
		Pages: []dashboard.Page{{ID: "overview", FilterBindings: map[string]dashboardfilter.Binding{
			"state": {
				Key: key, ID: "state", Filter: "state", Scope: dashboardfilter.ScopePage, PageID: "overview",
				Default: dashboardfilter.Expression{Kind: dashboardfilter.ExpressionUnfiltered},
			},
		}}},
	}
	filters, err := dashboardQueryFilters(definition, "overview", map[string]any{
		"version": "typed_v1",
		"controls": map[string]any{key: map[string]any{
			"kind": "set", "operator": "in",
			"values": []any{map[string]any{"kind": "string", "value": "SP"}},
		}},
	}, []map[string]any{{"sourceKind": "visual", "sourceId": "orders"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filters.CompiledState == nil || filters.CompiledState.AppliedControls[key].Expression.Values[0].Value != "SP" {
		t.Fatalf("compiled state = %#v", filters.CompiledState)
	}
	if len(filters.Selections) != 1 || filters.Selections[0].SourceID != "orders" {
		t.Fatalf("selections = %#v", filters.Selections)
	}
}

func TestDashboardQueryFiltersRejectsLegacyWholeMapState(t *testing.T) {
	if _, err := dashboardQueryFilters(dashboarddefinition.Definition{}, "overview", map[string]any{
		"controls": map[string]any{},
	}, nil, nil); err == nil {
		t.Fatal("legacy unversioned filter state was accepted")
	}
}
