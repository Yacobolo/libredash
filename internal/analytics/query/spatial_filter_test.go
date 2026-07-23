package query

import (
	"strings"
	"testing"
)

func TestSpatialFilterSQLSupportsExactBoxLassoAndRadiusPredicates(t *testing.T) {
	tests := []struct {
		name   string
		filter SpatialFilter
		parts  []string
	}{
		{name: "box", filter: SpatialFilter{Kind: "box", West: -74, South: -34, East: -34, North: 6}, parts: []string{"lat >= ?", "lat <= ?", "lon >= ?", "lon <= ?"}},
		{name: "antimeridian box", filter: SpatialFilter{Kind: "box", West: 170, South: -10, East: -170, North: 10}, parts: []string{"lon >= ? OR lon <= ?"}},
		{name: "lasso", filter: SpatialFilter{Kind: "lasso", Points: []SpatialPoint{{Longitude: -50, Latitude: -20}, {Longitude: -40, Latitude: -20}, {Longitude: -45, Latitude: -10}}}, parts: []string{"MOD(", "CASE WHEN", "NULLIF"}},
		{name: "radius", filter: SpatialFilter{Kind: "radius", Center: SpatialPoint{Longitude: -46.63, Latitude: -23.55}, RadiusMeters: 50000}, parts: []string{"ASIN", "RADIANS", "<= ?"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := spatialFilterSQL("lat", "lon", tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range tt.parts {
				if !strings.Contains(sql, part) {
					t.Fatalf("SQL = %s, want containing %q", sql, part)
				}
			}
			if len(args) == 0 {
				t.Fatalf("SQL predicate has no bound arguments: %s", sql)
			}
		})
	}
}

func TestSpatialFilterSQLRejectsUnsafeGeometry(t *testing.T) {
	tests := []SpatialFilter{
		{Kind: "box", West: -181, South: 0, East: 1, North: 2},
		{Kind: "box", West: 0, South: 2, East: 1, North: 1},
		{Kind: "lasso", Points: []SpatialPoint{{}, {}}},
		{Kind: "lasso", Points: []SpatialPoint{{Longitude: -170, Latitude: 0}, {Longitude: 170, Latitude: 1}, {Longitude: 0, Latitude: 2}}},
		{Kind: "radius", Center: SpatialPoint{}, RadiusMeters: 0},
		{Kind: "radius", Center: SpatialPoint{}, RadiusMeters: 5_000_001},
		{Kind: "polygon"},
	}
	for _, filter := range tests {
		if _, _, err := spatialFilterSQL("lat", "lon", filter); err == nil {
			t.Fatalf("unsafe spatial filter accepted: %#v", filter)
		}
	}
}
