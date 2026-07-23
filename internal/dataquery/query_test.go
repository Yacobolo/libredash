package dataquery

import "testing"

func TestQueryValidateAllowsSemanticCountOnlyAndRequiresRawTargets(t *testing.T) {
	if err := (Query{ModelID: "sales", Kind: KindSemanticRows, IncludeTotal: true}).Validate(); err != nil {
		t.Fatalf("semantic count-only query validate error = %v", err)
	}
	if err := (Query{ModelID: "sales", Kind: Kind("source_rows"), Target: "orders"}).Validate(); err == nil {
		t.Fatal("removed source query kind error = nil")
	}
	if err := (Query{ModelID: "sales", Kind: KindModelTableRows, Target: "orders", Sort: []Sort{{Field: "status", Direction: "sideways"}}}).Validate(); err == nil {
		t.Fatal("invalid sort direction error = nil")
	}
}

func TestQueryValidateRequiresCompleteSpatialWindow(t *testing.T) {
	query := Query{
		ModelID: "sales", Kind: KindSemanticSpatial, Target: "orders",
		Fields:  []Field{{Field: "orders.latitude", Alias: "latitude"}, {Field: "orders.longitude", Alias: "longitude"}},
		Spatial: &SpatialWindow{Latitude: Field{Field: "orders.latitude", Alias: "latitude"}, Longitude: Field{Field: "orders.longitude", Alias: "longitude"}, West: -10, South: -10, East: 10, North: 10, Width: 800, Height: 600, FeatureCap: 5000, Precision: SpatialPrecisionAggregated},
	}
	if err := query.Validate(); err != nil {
		t.Fatalf("valid spatial query: %v", err)
	}
	query.Spatial.FeatureCap = 0
	if err := query.Validate(); err == nil {
		t.Fatal("spatial query without feature cap was accepted")
	}
}
