package report

import (
	"strings"
	"testing"
)

func TestValidateGeographicVisualUsesClosedAliasBoundLayers(t *testing.T) {
	base := Visual{
		Type: "map",
		Query: VisualQuery{
			Dimensions: []FieldRef{{Field: "orders.state", Alias: "state"}, {Field: "orders.latitude", Alias: "latitude"}, {Field: "orders.longitude", Alias: "longitude"}},
			Measures:   []FieldRef{{Field: "revenue", Alias: "revenue"}},
		},
		Geo: VisualGeo{Layers: []VisualGeoLayer{
			{ID: "states", Kind: "choropleth", GeometryAsset: "brazil_states", Join: "state", Value: "revenue"},
			{ID: "orders", Kind: "point", Latitude: "latitude", Longitude: "longitude", Value: "revenue"},
		}},
	}
	if err := validateGeographicVisual("map", base); err != nil {
		t.Fatalf("valid geographic visual: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Visual)
		want   string
	}{
		{name: "unknown alias", mutate: func(visual *Visual) { visual.Geo.Layers[1].Latitude = "missing" }, want: `references unknown query alias "missing"`},
		{name: "duplicate layer", mutate: func(visual *Visual) { visual.Geo.Layers[1].ID = "states" }, want: `duplicate geographic layer "states"`},
		{name: "mixed coordinate and geometry", mutate: func(visual *Visual) { visual.Geo.Layers[1].GeometryAsset = "brazil_states" }, want: "does not accept geometry_asset or join"},
		{name: "unknown kind", mutate: func(visual *Visual) { visual.Geo.Layers[1].Kind = "tiles" }, want: `unsupported kind "tiles"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visual := base
			visual.Geo.Layers = append([]VisualGeoLayer(nil), base.Geo.Layers...)
			tt.mutate(&visual)
			err := validateGeographicVisual("map", visual)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
