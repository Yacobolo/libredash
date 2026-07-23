package compiler

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/report"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

func TestVisualPayloadIncludesPointSelectionContract(t *testing.T) {
	dashboardDefinition := &report.Dashboard{SemanticModel: "model", Visuals: report.ChartVisualizations(map[string]report.Visual{"source": {Type: "bar", Title: "Source", Query: report.VisualQuery{
		Dimensions: []report.FieldRef{{Field: "activity_date", Alias: "label"}}, Measures: []report.FieldRef{{Field: "event_count", Alias: "value"}}, Limit: 100,
	}, Interaction: report.Interaction{PointSelection: report.SelectionInteraction{
		Toggle: true,
		Mappings: []report.SelectionMapping{{
			Field: "activity_date",
			Grain: "month",
			Value: "label",
			Label: "label",
		}},
		Targets: []string{"tags_per_rating"},
	}}}})}

	definitions, err := compileVisualizationDefinitions(dashboardDefinition)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(definitions["source"])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"interactions"`, `"activity_date"`, `"month"`, `"tags_per_rating"`} {
		if !bytes.Contains(payload, []byte(want)) {
			t.Fatalf("visual payload = %s, want %s", payload, want)
		}
	}
}

func TestTablePayloadIncludesFactLocalRowSelectionContract(t *testing.T) {
	dashboardDefinition := &report.Dashboard{SemanticModel: "model", Visuals: report.TabularVisualizations("table", map[string]report.TableVisual{"source": {Title: "Source", Query: report.TableQuery{Table: "ratings", Fields: []string{"ratings.rating_bucket"}}, Columns: []dashboard.TableColumn{{Key: "rating_bucket", Label: "Rating"}}, Interaction: report.Interaction{RowSelection: report.SelectionInteraction{
		Mappings: []report.SelectionMapping{{
			Field: "ratings.rating_bucket",
			Fact:  "ratings",
			Value: "rating_bucket",
		}},
		Targets: []string{"tags_per_rating"},
	}}}})}

	definitions, err := compileVisualizationDefinitions(dashboardDefinition)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(definitions["source"])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"interactions"`, `"ratings.rating_bucket"`, `"ratings"`, `"tags_per_rating"`} {
		if !bytes.Contains(payload, []byte(want)) {
			t.Fatalf("table payload = %s, want %s", payload, want)
		}
	}
}

func TestCustomVisualCompilesToSandboxedVegaLiteDefinition(t *testing.T) {
	dashboardDefinition := &report.Dashboard{SemanticModel: "model", Visuals: report.ChartVisualizations(map[string]report.Visual{"custom": {
		Type: "custom", Title: "Custom", Query: report.VisualQuery{Table: "orders", Dimensions: []report.FieldRef{{Field: "orders.month", Alias: "month"}}, Measures: []report.FieldRef{{Field: "orders.revenue", Alias: "revenue"}}, Limit: 100},
		Custom: report.VisualCustom{Engine: "vega_lite", Program: map[string]any{"mark": "bar", "data": map[string]any{"name": "primary"}, "encoding": map[string]any{"x": map[string]any{"field": "month"}, "y": map[string]any{"field": "revenue"}}}},
	}})}

	definitions, err := compileVisualizationDefinitions(dashboardDefinition)
	if err != nil {
		t.Fatal(err)
	}
	definition := definitions["custom"]
	if definition.RendererID != "vega-lite-sandbox" || definition.Query.Kind != "custom" || definition.Query.Custom == nil {
		t.Fatalf("custom definition = %#v", definition)
	}
	spec, ok := definition.Spec.Value.(*visualizationir.CustomVisualizationSpec)
	if !ok || spec.ProgramDigest == "" || spec.Program == "" {
		t.Fatalf("custom spec = %#v", definition.Spec.Value)
	}
}

func TestGeographicVisualCompilesEveryLayerKind(t *testing.T) {
	dashboardDefinition := &report.Dashboard{SemanticModel: "model", Visuals: report.ChartVisualizations(map[string]report.Visual{"locations": {
		Type: "map", Title: "Locations", Query: report.VisualQuery{
			Table: "orders",
			Dimensions: []report.FieldRef{
				{Field: "orders.state", Alias: "state"},
				{Field: "orders.latitude", Alias: "latitude"},
				{Field: "orders.longitude", Alias: "longitude"},
			},
			Measures: []report.FieldRef{{Field: "orders.revenue", Alias: "revenue"}},
		},
		Geo: report.VisualGeo{
			Basemap: "streets", Theme: "auto", LabelDensity: "normal",
			Camera:   report.VisualGeoCamera{Mode: "fit_data", Padding: 32, MinimumZoom: 2, MaximumZoom: 14},
			Controls: report.VisualGeoControls{Zoom: true, Reset: true, Compass: true},
			Layers: []report.VisualGeoLayer{
				{ID: "states", Kind: "choropleth", GeometryAsset: "brazil_states", Join: "state", Value: "revenue", Tooltip: []string{"state", "revenue"}, Color: report.VisualGeoColorScale{Kind: "sequential", Palette: "blue"}},
				{ID: "stores", Kind: "point", Latitude: "latitude", Longitude: "longitude", Value: "revenue", Label: "state", Size: report.VisualGeoSizeScale{MinimumRadius: 5, MaximumRadius: 28}, Cluster: report.VisualGeoCluster{Enabled: true, Radius: 48, MaximumZoom: 10, ShowCount: true}},
				{ID: "demand", Kind: "heat", Latitude: "latitude", Longitude: "longitude", Value: "revenue"},
				{ID: "density", Kind: "density", Latitude: "latitude", Longitude: "longitude"},
			}},
		Interaction: report.Interaction{PointSelection: report.SelectionInteraction{
			Toggle: true,
			Mappings: []report.SelectionMapping{
				{Field: "orders.state", Fact: "orders", Value: "state", Label: "state"},
				{Field: "orders.latitude", Fact: "orders", Value: "latitude", Label: "revenue"},
			},
			Targets: []string{"detail", "summary"},
		}, SpatialSelection: report.SpatialSelectionInteraction{
			Gestures:  []string{"box", "lasso", "radius"},
			Latitude:  report.SpatialSelectionMapping{Source: "latitude", Field: "orders.latitude", Fact: "orders"},
			Longitude: report.SpatialSelectionMapping{Source: "longitude", Field: "orders.longitude", Fact: "orders"},
			Targets:   []string{"detail", "summary"},
		}},
	}})}

	definitions, err := compileVisualizationDefinitions(dashboardDefinition)
	if err != nil {
		t.Fatal(err)
	}
	definition := definitions["locations"]
	if definition.RendererID != "maplibre" {
		t.Fatalf("renderer = %q, want maplibre", definition.RendererID)
	}
	spec, ok := definition.Spec.Value.(*visualizationir.GeographicVisualizationSpec)
	if !ok {
		t.Fatalf("geographic spec = %#v", definition.Spec.Value)
	}
	if got, want := spec.DataBudget.MaxRows, int64(20_000); got != want {
		t.Fatalf("geographic data budget = %d, want %d", got, want)
	}
	if definition.Query.Kind != visualizationdefinition.QuerySpatial || definition.Query.Spatial == nil {
		t.Fatalf("geographic query binding = %#v, want explicit spatial binding", definition.Query)
	}
	if definition.Query.Spatial.Viewport != nil {
		t.Fatalf("20,000-row map unexpectedly compiled viewport strategy: %#v", definition.Query.Spatial.Viewport)
	}
	if got, want := spec.Presentation.Legend, visualizationir.VisualizationLegendPositionHidden; got != want {
		t.Fatalf("geographic legend = %q, want %q", got, want)
	}
	if spec.Presentation.Basemap == nil || spec.Presentation.Basemap.ID != "leapview-streets" || spec.Presentation.Basemap.ArchiveDigest == "" {
		t.Fatalf("geographic basemap = %#v, want content-addressed streets asset", spec.Presentation.Basemap)
	}
	if spec.Presentation.Camera.Mode != visualizationir.VisualizationMapCameraModeFitData || !spec.Presentation.Controls.Reset {
		t.Fatalf("geographic presentation = %#v", spec.Presentation)
	}
	if got, want := len(spec.Layers), 4; got != want {
		t.Fatalf("layers = %d, want %d", got, want)
	}
	for index, want := range []string{
		"choropleth",
		"point",
		"heat",
		"density",
	} {
		got, err := spec.Layers[index].Kind()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("layer %d kind = %q, want %q", index, got, want)
		}
	}
	choropleth, ok := spec.Layers[0].Value.(*visualizationir.VisualizationChoroplethLayer)
	if !ok || choropleth.Geometry.Digest == "" || len(choropleth.Tooltip) != 2 {
		t.Fatalf("choropleth layer = %#v", spec.Layers[0].Value)
	}
	point, ok := spec.Layers[1].Value.(*visualizationir.VisualizationPointLayer)
	if !ok || point.Latitude.Field != "latitude" || point.Longitude.Field != "longitude" || !point.Cluster.Enabled || point.Size.MaximumRadius != 28 {
		t.Fatalf("point layer = %#v", spec.Layers[1].Value)
	}
	if got, want := len(spec.Interactions), 1; got != want {
		t.Fatalf("geographic interactions = %d, want %d", got, want)
	}
	interaction := spec.Interactions[0]
	if interaction.Mode != visualizationir.VisualizationSelectionModeMultiple || !interaction.RequiresStableIdentity || len(interaction.Mappings) != 2 {
		t.Fatalf("geographic interaction = %#v", interaction)
	}
	if got, want := interaction.Targets, []string{"detail", "summary"}; !slices.Equal(got, want) {
		t.Fatalf("geographic targets = %#v, want %#v", got, want)
	}
	if got, want := len(spec.SpatialInteractions), 1; got != want {
		t.Fatalf("geographic spatial interactions = %d, want %d", got, want)
	}
	spatial := spec.SpatialInteractions[0]
	if spatial.ID != "spatial_selection" || spatial.Latitude.Source.Field != "latitude" || spatial.Longitude.TargetFieldID != "orders.longitude" || spatial.Longitude.TargetFactID == nil || *spatial.Longitude.TargetFactID != "orders" {
		t.Fatalf("geographic spatial interaction = %#v", spatial)
	}
	if got, want := spatial.Gestures, []visualizationir.VisualizationSpatialSelectionGesture{"box", "lasso", "radius"}; !slices.Equal(got, want) {
		t.Fatalf("geographic spatial gestures = %#v, want %#v", got, want)
	}
	roles := map[string]visualizationir.VisualizationFieldRole{}
	for _, field := range spec.Datasets[0].Fields {
		roles[field.ID] = field.Role
	}
	if roles["state"] != visualizationir.VisualizationFieldRoleIdentity || roles["latitude"] != visualizationir.VisualizationFieldRoleIdentity || roles["revenue"] != visualizationir.VisualizationFieldRoleMeasure {
		t.Fatalf("geographic roles = %#v", roles)
	}
}

func TestGeographicVisualCanExplicitlyDisableTheDefaultBasemap(t *testing.T) {
	dashboardDefinition := &report.Dashboard{SemanticModel: "model", Visuals: report.ChartVisualizations(map[string]report.Visual{"locations": {
		Type: "map", Query: report.VisualQuery{
			Table: "orders", Dimensions: []report.FieldRef{{Field: "orders.latitude", Alias: "latitude"}, {Field: "orders.longitude", Alias: "longitude"}}, Measures: []report.FieldRef{{Field: "orders.revenue", Alias: "revenue"}}, Limit: 100,
		},
		Geo: report.VisualGeo{Basemap: "blank", Layers: []report.VisualGeoLayer{{ID: "stores", Kind: "point", Latitude: "latitude", Longitude: "longitude"}}},
	}})}

	definitions, err := compileVisualizationDefinitions(dashboardDefinition)
	if err != nil {
		t.Fatal(err)
	}
	spec := definitions["locations"].Spec.Value.(*visualizationir.GeographicVisualizationSpec)
	if spec.Presentation.Basemap != nil {
		t.Fatalf("geographic basemap = %#v, want none", spec.Presentation.Basemap)
	}

	dashboardDefinition.Visuals["locations"] = func() report.AuthoringVisualization {
		visual := *dashboardDefinition.Visuals["locations"].Chart
		visual.Geo.Basemap = "unknown"
		return report.ChartVisualization(visual)
	}()
	if _, err := compileVisualizationDefinitions(dashboardDefinition); err == nil || !strings.Contains(err.Error(), `geographic basemap: unknown map style asset "unknown"`) {
		t.Fatalf("unknown basemap error = %v", err)
	}
}
