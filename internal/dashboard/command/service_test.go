package command

import (
	"context"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/libredash/internal/dashboard/definition"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/testutil/dashboardfixture"
	visualizationir "github.com/Yacobolo/libredash/internal/visualization/ir"
)

type fakeMetrics struct {
	report *dashboarddefinition.Definition
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
func (m fakeMetrics) Report(string) (dashboarddefinition.Definition, *semanticmodel.Model, bool) {
	authored := reportdef.Dashboard{
		ID: "dash", SemanticModel: "model",
		Filters: map[string]reportdef.FilterDefinition{"state": {Type: "multi_select", Label: "State", Operator: "in"}},
		Visuals: map[string]reportdef.Visual{
			"chart": {
				Type:  "bar",
				Query: reportdef.VisualQuery{Dimensions: []reportdef.FieldRef{{Field: "state", Alias: "label"}}, Measures: []reportdef.FieldRef{{Field: "order_count", Alias: "value"}}},
				Interaction: reportdef.Interaction{PointSelection: reportdef.SelectionInteraction{
					Toggle: true, Mappings: []reportdef.SelectionMapping{{Field: "state", Value: "label"}}, Targets: []string{"orders"},
				}},
			},
			"boolean_chart": {
				Type:  "bar",
				Query: reportdef.VisualQuery{Dimensions: []reportdef.FieldRef{{Field: "active", Alias: "label"}}, Measures: []reportdef.FieldRef{{Field: "order_count", Alias: "value"}}},
				Interaction: reportdef.Interaction{PointSelection: reportdef.SelectionInteraction{
					Toggle: true, Mappings: []reportdef.SelectionMapping{{Field: "active", Value: "label"}}, Targets: []string{"orders"},
				}},
			},
			"customer_map": {
				Type: "map", DataBudget: reportdef.VisualDataBudget{MaxRows: 1_000_000, RequiredCompleteness: "partial"},
				Query: reportdef.VisualQuery{
					Dimensions: []reportdef.FieldRef{{Field: "latitude", Alias: "latitude"}, {Field: "longitude", Alias: "longitude"}, {Field: "state", Alias: "state"}},
					Measures:   []reportdef.FieldRef{{Field: "order_count", Alias: "value"}},
				},
				Geo: reportdef.VisualGeo{Basemap: "blank", Layers: []reportdef.VisualGeoLayer{{ID: "customers", Kind: "point", Latitude: "latitude", Longitude: "longitude", Value: "value"}}},
			},
		},
		Tables: map[string]reportdef.TableVisual{"orders": {Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.state"}}}},
		Pages: []dashboard.Page{{ID: "overview", Visuals: []dashboard.PageVisual{
			{Kind: "filter_card", Filter: "state"}, {Kind: "visual", Visual: "chart"}, {Kind: "visual", Visual: "customer_map"}, {Kind: "table", Table: "orders"},
		}}},
	}
	model := &semanticmodel.Model{
		Name: "model",
		Tables: map[string]semanticmodel.Table{"orders": {Dimensions: map[string]semanticmodel.MetricDimension{
			"state": {Type: "string"}, "active": {Type: "boolean"}, "latitude": {Type: "number"}, "longitude": {Type: "number"},
		}}},
		Dimensions: map[string]semanticmodel.SemanticDimension{
			"state":     {Type: "string", Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.state"}}},
			"active":    {Type: "boolean", Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.active"}}},
			"latitude":  {Type: "number", Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.latitude"}}},
			"longitude": {Type: "number", Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.longitude"}}},
		},
		Measures: map[string]semanticmodel.MetricMeasure{"order_count": {Fact: "orders", Aggregation: "count"}},
	}
	definition := dashboardfixture.Compile(authored, model)
	if m.report != nil {
		definition = *m.report
	}
	return definition, model, true
}

func TestPrepareVisualSpatialWindowValidatesCompiledIdentity(t *testing.T) {
	definition, _, _ := (fakeMetrics{}).Report("dash")
	mapDefinition := definition.Visualizations["customer_map"]
	request := dashboard.SpatialWindowRequest{
		VisualID: "customer_map", SpecRevision: mapDefinition.SpecRevision, DataRevision: 9, RequestSeq: 2, ResetVersion: 1,
		Bounds: dashboard.SpatialBounds{West: 170, South: -20, East: -170, North: 25}, Zoom: 3.25, Width: 960, Height: 540,
		WindowID: "170.000000,-20.000000,-170.000000,25.000000@3.250:960x540",
	}
	prepared, err := (Service{Metrics: fakeMetrics{}}).PrepareVisualSpatialWindow(Request{DashboardID: "dash", PageID: "overview", VisualSpatialWindowCommand: request}, dashboard.Filters{}.WithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Plan.Targets) != 1 || prepared.Plan.Targets[0].Kind != TargetSpatial || prepared.Plan.Targets[0].SpatialRequest != request {
		t.Fatalf("targets = %#v", prepared.Plan.Targets)
	}

	request.SpecRevision = "sha256:forged"
	if _, err := (Service{Metrics: fakeMetrics{}}).PrepareVisualSpatialWindow(Request{DashboardID: "dash", PageID: "overview", VisualSpatialWindowCommand: request}, dashboard.Filters{}); err == nil {
		t.Fatal("forged spatial revision was accepted")
	}
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
	chart := definition.Visualizations["chart"]
	spec := chart.Spec.Value.(*visualizationir.CartesianVisualizationSpec)
	spec.Interactions[0].Targets = []string{"orders", "boolean_chart"}
	definition.Visualizations["chart"] = chart
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
