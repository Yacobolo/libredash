package compiler

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	visualizationir "github.com/Yacobolo/libredash/internal/visualization/ir"
)

func TestVisualPayloadIncludesPointSelectionContract(t *testing.T) {
	dashboardDefinition := &report.Dashboard{SemanticModel: "model", Visuals: map[string]report.Visual{"source": {Type: "bar", Title: "Source", Query: report.VisualQuery{
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
	}}}}}

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
	dashboardDefinition := &report.Dashboard{SemanticModel: "model", Tables: map[string]report.TableVisual{"source": {Title: "Source", Query: report.TableQuery{Table: "ratings", Fields: []string{"ratings.rating_bucket"}}, Columns: []dashboard.TableColumn{{Key: "rating_bucket", Label: "Rating"}}, Interaction: report.Interaction{RowSelection: report.SelectionInteraction{
		Mappings: []report.SelectionMapping{{
			Field: "ratings.rating_bucket",
			Fact:  "ratings",
			Value: "rating_bucket",
		}},
		Targets: []string{"tags_per_rating"},
	}}}}}

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
	dashboardDefinition := &report.Dashboard{SemanticModel: "model", Visuals: map[string]report.Visual{"custom": {
		Type: "custom", Title: "Custom", Query: report.VisualQuery{Table: "orders", Dimensions: []report.FieldRef{{Field: "orders.month", Alias: "month"}}, Measures: []report.FieldRef{{Field: "orders.revenue", Alias: "revenue"}}, Limit: 100},
		Custom: report.VisualCustom{Engine: "vega_lite", Program: map[string]any{"mark": "bar", "data": map[string]any{"name": "primary"}, "encoding": map[string]any{"x": map[string]any{"field": "month"}, "y": map[string]any{"field": "revenue"}}}},
	}}}

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
	dashboardDefinition := &report.Dashboard{SemanticModel: "model", Visuals: map[string]report.Visual{"locations": {
		Type: "map", Title: "Locations", Query: report.VisualQuery{
			Table: "orders",
			Dimensions: []report.FieldRef{
				{Field: "orders.state", Alias: "state"},
				{Field: "orders.latitude", Alias: "latitude"},
				{Field: "orders.longitude", Alias: "longitude"},
			},
			Measures: []report.FieldRef{{Field: "orders.revenue", Alias: "revenue"}}, Limit: 100,
		},
		Geo: report.VisualGeo{Layers: []report.VisualGeoLayer{
			{ID: "states", Kind: "choropleth", GeometryAsset: "brazil_states", Join: "state", Value: "revenue"},
			{ID: "stores", Kind: "point", Latitude: "latitude", Longitude: "longitude", Value: "revenue"},
			{ID: "demand", Kind: "heat", Latitude: "latitude", Longitude: "longitude", Value: "revenue"},
			{ID: "density", Kind: "density", Latitude: "latitude", Longitude: "longitude"},
		}},
	}}}

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
	if got, want := len(spec.Layers), 4; got != want {
		t.Fatalf("layers = %d, want %d", got, want)
	}
	for index, want := range []visualizationir.VisualizationGeographicLayerKind{
		visualizationir.VisualizationGeographicLayerKindChoropleth,
		visualizationir.VisualizationGeographicLayerKindPoint,
		visualizationir.VisualizationGeographicLayerKindHeat,
		visualizationir.VisualizationGeographicLayerKindDensity,
	} {
		if got := spec.Layers[index].Kind; got != want {
			t.Fatalf("layer %d kind = %q, want %q", index, got, want)
		}
	}
	if spec.Layers[0].Geometry == nil || spec.Layers[0].Geometry.Digest == "" || spec.Layers[0].Join == nil {
		t.Fatalf("choropleth layer = %#v", spec.Layers[0])
	}
	if spec.Layers[1].Latitude == nil || spec.Layers[1].Latitude.Field != "latitude" || spec.Layers[1].Longitude == nil || spec.Layers[1].Longitude.Field != "longitude" {
		t.Fatalf("point layer = %#v", spec.Layers[1])
	}
}
