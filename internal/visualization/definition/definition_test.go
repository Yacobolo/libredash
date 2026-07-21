package definition

import (
	"testing"

	"github.com/Yacobolo/libredash/internal/visualization/ir"
)

func TestDefinitionValidateRejectsRendererAndQueryMismatches(t *testing.T) {
	t.Parallel()

	tests := map[string]Definition{
		"missing identity": {ID: "orders", RendererID: RendererTanStack, Query: QueryBinding{}},
		"wrong renderer":   {ID: "orders", RendererID: RendererECharts, Query: QueryBinding{Kind: QueryDetail}, Spec: tableSpec()},
		"wrong query":      {ID: "orders", RendererID: RendererTanStack, Query: QueryBinding{Kind: QueryAggregate}, Spec: tableSpec()},
	}
	for name, definition := range tests {
		definition := definition
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := definition.Validate(); err == nil {
				t.Fatal("expected invalid definition to fail")
			}
		})
	}
}

func TestNewComputesRevisionAndSelectsOwnedRenderer(t *testing.T) {
	t.Parallel()

	definition, err := New("orders", tableSpec(), QueryBinding{
		Kind: QueryDetail, ModelID: "sales", DatasetID: "primary",
		Detail: &DetailQueryBinding{TableID: "orders", Fields: []FieldBinding{{FieldID: "orders.order_id", Alias: "order_id"}}, Limit: 1000},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if definition.RendererID != RendererTanStack {
		t.Fatalf("renderer = %q, want %q", definition.RendererID, RendererTanStack)
	}
	computed, err := ir.ComputeSpecRevision(definition.Spec)
	if err != nil {
		t.Fatalf("ComputeSpecRevision: %v", err)
	}
	if definition.SpecRevision != computed.String() {
		t.Fatalf("revision = %q, want %q", definition.SpecRevision, computed)
	}
}

func TestQueryBindingRejectsMissingAndConflictingBranches(t *testing.T) {
	t.Parallel()

	for name, binding := range map[string]QueryBinding{
		"missing branch": {Kind: QueryDetail, ModelID: "sales", DatasetID: "primary"},
		"conflicting branch": {
			Kind: QueryDetail, ModelID: "sales", DatasetID: "primary",
			Detail:    &DetailQueryBinding{TableID: "orders", Fields: []FieldBinding{{FieldID: "orders.id", Alias: "id"}}, Limit: 100},
			Aggregate: &AggregateQueryBinding{TableID: "orders", Measures: []FieldBinding{{FieldID: "orders.count", Alias: "value"}}, Limit: 1},
		},
		"spatial viewport without coordinates": {
			Kind: QuerySpatial, ModelID: "sales", DatasetID: "primary",
			Spatial: &SpatialQueryBinding{
				TableID: "orders", Dimensions: []FieldBinding{{FieldID: "orders.state", Alias: "state"}}, Limit: 1_000_000,
				Viewport: &SpatialViewportBinding{FeatureCap: 5000},
			},
		},
	} {
		binding := binding
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := binding.Validate(); err == nil {
				t.Fatal("expected invalid query binding to fail")
			}
		})
	}
}

func TestGeographicDefinitionOwnsExplicitSpatialQuery(t *testing.T) {
	t.Parallel()

	binding := QueryBinding{
		Kind: QuerySpatial, ModelID: "sales", DatasetID: "primary", Identity: []string{"order_id"},
		Spatial: &SpatialQueryBinding{
			TableID: "orders",
			Dimensions: []FieldBinding{
				{FieldID: "orders.order_id", Alias: "order_id"},
				{FieldID: "orders.latitude", Alias: "latitude"},
				{FieldID: "orders.longitude", Alias: "longitude"},
			},
			Measures: []FieldBinding{{FieldID: "orders.revenue", Alias: "revenue"}},
			Limit:    1_000_000,
			Viewport: &SpatialViewportBinding{
				Latitude: FieldBinding{FieldID: "orders.latitude", Alias: "latitude"}, Longitude: FieldBinding{FieldID: "orders.longitude", Alias: "longitude"},
				FeatureCap: 5000, RawMinimumZoom: 10,
			},
		},
	}
	definition, err := New("orders", geographicSpec(), binding)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if definition.RendererID != RendererMapLibre || definition.Query.Kind != QuerySpatial || definition.Query.Spatial == nil {
		t.Fatalf("geographic ownership = %#v", definition)
	}
}

func tableSpec() ir.VisualizationSpec {
	return ir.VisualizationSpec{Value: &ir.TableVisualizationSpec{
		VisualizationSpecBase: ir.VisualizationSpecBase{
			Kind: "table", Title: "Orders",
			Datasets:      []ir.VisualizationDatasetSchema{{ID: "primary", Fields: []ir.VisualizationField{{ID: "order_id", Role: ir.VisualizationFieldRoleIdentity, DataType: ir.VisualizationDataTypeString, Label: "Order ID"}}}},
			DataBudget:    ir.VisualizationDataBudget{MaxRows: 1000, RequiredCompleteness: ir.VisualizationCompletenessPartial},
			Accessibility: ir.VisualizationAccessibility{Title: "Orders", Description: "Order details"}, Interactions: []ir.VisualizationInteraction{},
		},
		Kind: "table", Columns: []ir.TableVisualizationColumn{{Field: ir.VisualizationFieldRef{Dataset: "primary", Field: "order_id"}, Label: "Order ID"}},
		Presentation: ir.GridVisualizationPresentation{RowHeight: 32, ShowHeader: true},
	}}
}

func geographicSpec() ir.VisualizationSpec {
	latitude := ir.VisualizationField{ID: "latitude", Role: ir.VisualizationFieldRoleDimension, DataType: ir.VisualizationDataTypeDecimal, Nullable: true, Label: "Latitude"}
	longitude := ir.VisualizationField{ID: "longitude", Role: ir.VisualizationFieldRoleDimension, DataType: ir.VisualizationDataTypeDecimal, Nullable: true, Label: "Longitude"}
	return ir.VisualizationSpec{Value: &ir.GeographicVisualizationSpec{
		VisualizationSpecBase: ir.VisualizationSpecBase{
			Kind: "geographic", Title: "Orders", Datasets: []ir.VisualizationDatasetSchema{{ID: "primary", Fields: []ir.VisualizationField{latitude, longitude}}},
			DataBudget:    ir.VisualizationDataBudget{MaxRows: 1_000_000, RequiredCompleteness: ir.VisualizationCompletenessPartial},
			Accessibility: ir.VisualizationAccessibility{Title: "Orders", Description: "Order locations"}, Interactions: []ir.VisualizationInteraction{},
		},
		Kind: "geographic",
		Layers: []ir.VisualizationGeographicLayer{{Value: &ir.VisualizationPointLayer{
			VisualizationGeographicLayerBase: ir.VisualizationGeographicLayerBase{ID: "orders", Kind: "point", Position: ir.VisualizationMapLayerPositionAboveLabels, Visibility: ir.VisualizationMapVisibility{MinimumZoom: 0, MaximumZoom: 22}},
			Kind:                             "point", Latitude: ir.VisualizationFieldRef{Dataset: "primary", Field: "latitude"}, Longitude: ir.VisualizationFieldRef{Dataset: "primary", Field: "longitude"},
			Size: ir.VisualizationMapSizeScale{MinimumRadius: 3, MaximumRadius: 20}, Color: ir.VisualizationMapColorScale{Kind: ir.VisualizationMapColorScaleKindSequential, Palette: "blues", NullColor: "#ccc"}, Stroke: ir.VisualizationMapStroke{Color: "#fff", Width: 1, Opacity: 1}, Cluster: ir.VisualizationMapCluster{Radius: 50, MaximumZoom: 14, MinimumPoints: 2}, Opacity: 1,
		}}},
		Presentation: ir.GeographicVisualizationPresentation{
			VisualizationPresentation: ir.VisualizationPresentation{Legend: ir.VisualizationLegendPositionHidden}, Roam: true, Theme: ir.VisualizationMapThemeAuto, LabelDensity: ir.VisualizationMapLabelDensityNormal,
			Camera: ir.VisualizationMapCamera{Mode: ir.VisualizationMapCameraModeFitData, Padding: 20, MinimumZoom: 0, MaximumZoom: 22}, Controls: ir.VisualizationMapControls{},
		},
	}}
}
