package command

import (
	"math"
	"testing"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/testutil/dashboardfixture"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

type fakeMetrics struct {
	report *dashboarddefinition.Definition
}

func (fakeMetrics) DefaultFilters(string) dashboard.Filters {
	return dashboard.Filters{}.WithDefaults()
}
func (fakeMetrics) NormalizeVisualizationWindow(_ string, request dashboard.TableRequest) dashboard.TableRequest {
	if request.Table == "" {
		request.Table = "orders"
	}
	return request.WithDefaults()
}
func (m fakeMetrics) Report(string) (dashboarddefinition.Definition, *semanticmodel.Model, bool) {
	authored := reportdef.Dashboard{
		ID: "dash", SemanticModel: "model",
		FilterDefinitions: map[string]dashboardfilter.Definition{
			"state": {
				Label: "State", Field: "state",
				Predicates: []dashboardfilter.PredicatePolicy{{
					Kind: dashboardfilter.ExpressionSet, Operators: []dashboardfilter.Operator{dashboardfilter.OperatorIn},
				}},
			},
		},
		Visuals: reportdef.MergeVisualizations(reportdef.ChartVisualizations(map[string]reportdef.Visual{
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
				Interaction: reportdef.Interaction{SpatialSelection: reportdef.SpatialSelectionInteraction{
					Gestures:  []string{"box", "lasso", "radius"},
					Latitude:  reportdef.SpatialSelectionMapping{Source: "latitude", Field: "latitude"},
					Longitude: reportdef.SpatialSelectionMapping{Source: "longitude", Field: "longitude"},
					Targets:   []string{"chart", "orders"},
				}},
			},
		}), reportdef.TabularVisualizations("table", map[string]reportdef.TableVisual{"orders": {Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.state"}}}})),
		Pages: []dashboard.Page{
			{ID: "overview", FilterBindings: map[string]dashboardfilter.Binding{
				"state": {Filter: "state", Default: dashboardfilter.Expression{Kind: dashboardfilter.ExpressionUnfiltered}},
			}, Visuals: []dashboard.PageVisual{
				{ID: "state-slicer", Kind: "slicer", Binding: dashboardfilter.BindingRef{Scope: dashboardfilter.ScopePage, ID: "state"}},
				{ID: "chart", Kind: "visual", Visual: "chart"}, {ID: "customer-map", Kind: "visual", Visual: "customer_map"}, {ID: "orders", Kind: "visual", Visual: "orders"},
			}},
			{ID: "boolean", Visuals: []dashboard.PageVisual{{ID: "boolean-chart", Kind: "visual", Visual: "boolean_chart"}, {ID: "orders", Kind: "visual", Visual: "orders"}}},
		},
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

func TestPrepareSpatialSelectValidatesGeometryAndUsesExplicitTargets(t *testing.T) {
	definition, _, _ := (fakeMetrics{}).Report("dash")
	command := dashboard.SpatialSelectionCommand{
		VisualID: "customer_map", SpecRevision: definition.Visualizations["customer_map"].SpecRevision, DataRevision: 7,
		InteractionID: "spatial_selection", Action: "set", Gesture: visualizationir.VisualizationSpatialSelectionGestureBox,
		Geometry: visualizationir.VisualizationSpatialSelectionGeometry{Value: &visualizationir.VisualizationSpatialBoxSelection{
			VisualizationSpatialSelectionGeometryBase: visualizationir.VisualizationSpatialSelectionGeometryBase{Kind: "box"}, Kind: "box",
			Bounds: visualizationir.VisualizationSpatialBounds{West: -50, South: -25, East: -40, North: -15},
		}},
	}
	prepared, err := (Service{Metrics: fakeMetrics{}}).PrepareSpatialSelect(Request{DashboardID: "dash", PageID: "overview", SpatialInteractionCommand: command}, dashboard.Filters{}.WithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Filters.SpatialSelections) != 1 || len(prepared.Plan.Targets) != 2 || prepared.Plan.Targets[0].ID != "chart" || prepared.Plan.Targets[1].ID != "orders" {
		t.Fatalf("prepared = %#v", prepared)
	}

	command.Gesture = visualizationir.VisualizationSpatialSelectionGestureRadius
	if _, err := (Service{Metrics: fakeMetrics{}}).PrepareSpatialSelect(Request{DashboardID: "dash", PageID: "overview", SpatialInteractionCommand: command}, dashboard.Filters{}); err == nil {
		t.Fatal("mismatched gesture and geometry was accepted")
	}
	command.Gesture = visualizationir.VisualizationSpatialSelectionGestureBox
	box := command.Geometry.Value.(*visualizationir.VisualizationSpatialBoxSelection)
	box.Bounds.North = math.Inf(1)
	if _, err := (Service{Metrics: fakeMetrics{}}).PrepareSpatialSelect(Request{DashboardID: "dash", PageID: "overview", SpatialInteractionCommand: command}, dashboard.Filters{}); err == nil {
		t.Fatal("non-finite geometry was accepted")
	}
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

func TestPrepareVisualWindowValidatesTypedIdentityAndCoordinates(t *testing.T) {
	definition, _, _ := (fakeMetrics{}).Report("dash")
	request := dashboard.VisualizationWindowRequest{
		VisualID: "orders", SpecRevision: definition.Visualizations["orders"].SpecRevision, DataRevision: 9,
		RequestSeq: 7, ResetVersion: 2, Start: 150, Limit: 50, BlockID: "b",
		Sort: []visualizationir.VisualizationSort{{
			Field:     visualizationir.VisualizationFieldRef{Dataset: "primary", Field: "state"},
			Direction: visualizationir.VisualizationSortDirectionDescending,
		}},
	}
	prepared, err := (Service{Metrics: fakeMetrics{}}).PrepareVisualWindow(Request{DashboardID: "dash", PageID: "overview", VisualWindowCommand: request}, dashboard.Filters{}.WithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Plan.Targets) != 1 {
		t.Fatalf("targets = %#v", prepared.Plan.Targets)
	}
	target := prepared.Plan.Targets[0]
	if target.Kind != TargetWindow || target.ID != "orders" || target.WindowRequest.Block != "b" || target.WindowRequest.Start != 150 || target.WindowRequest.Count != 50 || target.WindowRequest.RequestSeq != 7 || target.WindowRequest.Sort.Key != "state" || target.WindowRequest.Sort.Direction != "desc" {
		t.Fatalf("target = %#v", target)
	}

	request.SpecRevision = "sha256:forged"
	if _, err := (Service{Metrics: fakeMetrics{}}).PrepareVisualWindow(Request{DashboardID: "dash", VisualWindowCommand: request}, dashboard.Filters{}); err == nil {
		t.Fatal("forged table revision was accepted")
	}
	request.SpecRevision = definition.Visualizations["orders"].SpecRevision
	request.RequestSeq = 0
	if _, err := (Service{Metrics: fakeMetrics{}}).PrepareVisualWindow(Request{DashboardID: "dash", VisualWindowCommand: request}, dashboard.Filters{}); err == nil {
		t.Fatal("non-positive table request sequence was accepted")
	}
}

func TestPrepareSelectUsesAuthoritativeSelectionsAndExplicitTargetsOnly(t *testing.T) {
	authoritative := dashboard.Filters{
		Selections: []dashboard.InteractionSelection{{
			SourceKind: "visual", SourceID: "existing", InteractionKind: "point_selection",
		}},
	}.WithDefaults()
	prepared, err := (Service{Metrics: fakeMetrics{}}).PrepareSelect(Request{
		DashboardID: "dash", PageID: "overview",
		InteractionCommand: dashboard.InteractionCommand{
			SourceKind: "visual", SourceID: "chart", InteractionKind: "point_selection", Action: "set",
			Mappings: []dashboard.InteractionCommandMapping{{Field: "state", Value: "RJ"}},
		},
	}, authoritative)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Filters.Selections) != 2 || prepared.Filters.Selections[0].SourceID != "existing" {
		t.Fatalf("authoritative selections = %#v", prepared.Filters.Selections)
	}
	if len(prepared.Plan.Targets) != 1 || prepared.Plan.Targets[0].Kind != TargetWindow || prepared.Plan.Targets[0].ID != "orders" {
		t.Fatalf("targets = %#v", prepared.Plan.Targets)
	}
}

func TestPrepareSelectRestrictsExplicitTargetsToActivePage(t *testing.T) {
	definition, _, _ := (fakeMetrics{}).Report("dash")
	chart := definition.Visualizations["chart"]
	spec := chart.Spec.Value.(*visualizationir.CartesianVisualizationSpec)
	spec.Interactions[0].Targets = []string{"orders", "boolean_chart"}
	definition.Visualizations["chart"] = chart

	prepared, err := (Service{Metrics: fakeMetrics{report: &definition}}).PrepareSelect(Request{
		DashboardID: "dash", PageID: "overview",
		InteractionCommand: dashboard.InteractionCommand{
			SourceKind: "visual", SourceID: "chart", InteractionKind: "point_selection", Action: "set",
			Mappings: []dashboard.InteractionCommandMapping{{Field: "state", Value: "RJ"}},
		},
	}, dashboard.Filters{}.WithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Plan.Targets) != 1 || prepared.Plan.Targets[0].ID != "orders" {
		t.Fatalf("off-page targets leaked into active stream plan: %#v", prepared.Plan.Targets)
	}
}

func TestPrepareSelectCanonicalizesTypedMappings(t *testing.T) {
	prepared, err := (Service{Metrics: fakeMetrics{}}).PrepareSelect(Request{
		DashboardID: "dash", PageID: "boolean",
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
	spec.Interactions[0].Targets = []string{"orders", "customer_map"}
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
