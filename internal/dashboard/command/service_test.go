package command

import (
	"context"
	"testing"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
)

type fakeMetrics struct {
	report *reportdef.Dashboard
}

func (fakeMetrics) DefaultFilters(string) dashboard.Filters {
	return dashboard.Filters{Controls: map[string]dashboard.FilterControl{"state": {Type: "multi_select", Operator: "in"}}}
}
func (fakeMetrics) NormalizeTableRequest(_ string, request dashboard.TableRequest) dashboard.TableRequest {
	if request.Table == "" {
		request.Table = "orders"
	}
	return request.WithDefaults()
}
func (fakeMetrics) QueryTablePage(context.Context, string, string, dashboard.Filters, dashboard.TableRequest) (dashboard.Table, error) {
	return dashboard.Table{}, nil
}
func (m fakeMetrics) Report(string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	definition := reportdef.Dashboard{
		Filters: map[string]reportdef.FilterDefinition{"state": {Type: "multi_select", Label: "State", Operator: "in"}},
		Visuals: map[string]reportdef.Visual{
			"chart": {
				Query: reportdef.VisualQuery{Dimensions: []reportdef.FieldRef{{Field: "state", Alias: "label"}}, Measures: []reportdef.FieldRef{{Field: "order_count", Alias: "value"}}},
				Interaction: reportdef.Interaction{PointSelection: reportdef.SelectionInteraction{
					Toggle: true, Mappings: []reportdef.SelectionMapping{{Field: "state", Value: "label"}}, Targets: []string{"orders"},
				}},
			},
			"boolean_chart": {
				Query: reportdef.VisualQuery{Dimensions: []reportdef.FieldRef{{Field: "active", Alias: "label"}}, Measures: []reportdef.FieldRef{{Field: "order_count", Alias: "value"}}},
				Interaction: reportdef.Interaction{PointSelection: reportdef.SelectionInteraction{
					Toggle: true, Mappings: []reportdef.SelectionMapping{{Field: "active", Value: "label"}}, Targets: []string{"orders"},
				}},
			},
		},
		Tables: map[string]reportdef.TableVisual{"orders": {Query: reportdef.TableQuery{Table: "orders"}}},
		Pages: []dashboard.Page{{ID: "overview", Visuals: []dashboard.PageVisual{
			{Kind: "filter_card", Filter: "state"}, {Kind: "visual", Visual: "chart"}, {Kind: "table", Table: "orders"},
		}}},
	}
	if m.report != nil {
		definition = *m.report
	}
	return definition, &semanticmodel.Model{
		Name: "model",
		Tables: map[string]semanticmodel.Table{"orders": {Dimensions: map[string]semanticmodel.MetricDimension{
			"state": {Type: "string"}, "active": {Type: "boolean"},
		}}},
		Dimensions: map[string]semanticmodel.SemanticDimension{
			"state":  {Type: "string", Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.state"}}},
			"active": {Type: "boolean", Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.active"}}},
		},
		Measures: map[string]semanticmodel.MetricMeasure{"order_count": {Fact: "orders"}},
	}, true
}

func TestPrepareSelectUsesAuthoritativeFiltersAndExplicitTargetsOnly(t *testing.T) {
	prepared, err := (Service{Metrics: fakeMetrics{}}).PrepareSelect(Request{
		DashboardID: "dash", PageID: "overview",
		InteractionCommand: dashboard.InteractionCommand{
			SourceKind: "visual", SourceID: "chart", InteractionKind: "point_selection", Action: "set",
			Mappings: []dashboard.InteractionCommandMapping{{Field: "state", Value: "RJ"}},
		},
	}, dashboard.Filters{Controls: map[string]dashboard.FilterControl{
		"state": {Type: "multi_select", Values: []string{"SP"}},
	}}.WithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if got := prepared.Filters.Controls["state"].Values; len(got) != 1 || got[0] != "SP" {
		t.Fatalf("authoritative controls = %#v", prepared.Filters.Controls)
	}
	if len(prepared.Plan.Targets) != 1 || prepared.Plan.Targets[0].Kind != TargetTable || prepared.Plan.Targets[0].ID != "orders" {
		t.Fatalf("targets = %#v", prepared.Plan.Targets)
	}
}

func TestPrepareSelectCanonicalizesTypedMappings(t *testing.T) {
	prepared, err := (Service{Metrics: fakeMetrics{}}).PrepareSelect(Request{
		DashboardID: "dash", PageID: "overview",
		InteractionCommand: dashboard.InteractionCommand{
			SourceKind: "visual", SourceID: "boolean_chart", InteractionKind: "point_selection", Action: "set",
			Mappings: []dashboard.InteractionCommandMapping{{Field: "active", Value: false}},
		},
	}, dashboard.Filters{}.WithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	value := prepared.Filters.Selections[0].Entries[0].Mappings[0].Value
	if typed, ok := value.(bool); !ok || typed {
		t.Fatalf("typed value = %#v", value)
	}
}

func TestPrepareSelectRejectsForgedMapping(t *testing.T) {
	_, err := (Service{Metrics: fakeMetrics{}}).PrepareSelect(Request{
		DashboardID: "dash", PageID: "overview",
		InteractionCommand: dashboard.InteractionCommand{
			SourceKind: "visual", SourceID: "chart", InteractionKind: "point_selection", Action: "set",
			Mappings: []dashboard.InteractionCommandMapping{{Field: "orders.secret", Value: "x"}},
		},
	}, dashboard.Filters{}.WithDefaults())
	if err == nil {
		t.Fatal("forged mapping was accepted")
	}
}

func TestPrepareClearSelectionPlansAffectedTargetUnion(t *testing.T) {
	definition, _, _ := (fakeMetrics{}).Report("dash")
	chart := definition.Visuals["chart"]
	chart.Interaction.PointSelection.Targets = []string{"orders", "boolean_chart"}
	definition.Visuals["chart"] = chart
	prepared, err := (Service{Metrics: fakeMetrics{report: &definition}}).PrepareClearSelection(Request{
		DashboardID: "dash", PageID: "overview",
	}, dashboard.Filters{Selections: []dashboard.InteractionSelection{
		{SourceKind: "visual", SourceID: "chart", InteractionKind: "point_selection"},
		{SourceKind: "visual", SourceID: "boolean_chart", InteractionKind: "point_selection"},
	}}.WithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Filters.Selections) != 0 || len(prepared.Plan.Targets) != 2 {
		t.Fatalf("prepared = %#v", prepared)
	}
}
