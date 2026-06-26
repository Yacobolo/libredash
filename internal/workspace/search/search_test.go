package search

import (
	"reflect"
	"testing"
)

func TestRankMatchesAndOrdersProgressiveDiscoveryDocuments(t *testing.T) {
	documents := []Document{
		{ID: "dashboard.sales", Type: "dashboard", Name: "Sales Overview", Description: "Executive dashboard", Terms: []string{"orders"}, Weight: 20, Refs: Refs{DashboardID: "sales"}},
		{ID: "visual.sales.orders", Type: "visual", Name: "Orders", Description: "Order trend", Terms: []string{"order_count"}, Weight: 30, Refs: Refs{DashboardID: "sales", PageID: "overview", VisualID: "orders"}},
		{ID: "dataset.orders", Type: "dataset", Name: "Fact Orders", Description: "Order rows", Terms: []string{"orders"}, Weight: 15, Refs: Refs{ModelID: "sales", DatasetID: "orders"}},
		{ID: "field.customer", Type: "field", Name: "Customer", Description: "Buyer name", Terms: []string{"users"}, Weight: 25},
	}

	results := Rank(documents, Query{Text: "orders"})
	got := resultIDs(results)
	want := []string{"visual.sales.orders", "dataset.orders", "dashboard.sales"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("result IDs = %#v, want %#v", got, want)
	}
	if results[0].VisualID != "orders" || results[0].DashboardID != "sales" {
		t.Fatalf("refs not preserved in top result: %#v", results[0])
	}
}

func TestRankFiltersByTypeAndUsesStableTies(t *testing.T) {
	types, err := ParseTypes("table, visual")
	if err != nil {
		t.Fatalf("parse types: %v", err)
	}
	documents := []Document{
		{ID: "table.b", Type: "table", Name: "Orders B", Weight: 10},
		{ID: "visual.a", Type: "visual", Name: "Orders A", Weight: 10},
		{ID: "dashboard.c", Type: "dashboard", Name: "Orders C", Weight: 100},
		{ID: "table.a", Type: "table", Name: "Orders A", Weight: 10},
	}

	results := Rank(documents, Query{Text: "orders", Types: types})
	got := resultIDs(results)
	want := []string{"table.a", "table.b", "visual.a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("result IDs = %#v, want %#v", got, want)
	}
}

func TestParseTypesRejectsUnknownTypes(t *testing.T) {
	if _, err := ParseTypes("dashboard,unknown"); err == nil {
		t.Fatal("expected unknown type error")
	}
}

func TestWorkspaceSearchPackageIsTransportNeutral(t *testing.T) {
	AssertNoForbiddenImports(t)
}

func resultIDs(results []Result) []string {
	out := make([]string, 0, len(results))
	for _, result := range results {
		out = append(out, result.ID)
	}
	return out
}
