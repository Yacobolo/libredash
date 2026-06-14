package semantic

import (
	"path/filepath"
	"testing"
)

func TestLoadOlistModel(t *testing.T) {
	model, err := Load(filepath.Join("..", "..", "dashboards", "olist.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	if model.Name != "olist" {
		t.Fatalf("model name = %q, want olist", model.Name)
	}
	if len(model.Sources) != 7 {
		t.Fatalf("source count = %d, want 7", len(model.Sources))
	}
	if model.Visuals["revenue"].Source != "orders_enriched" {
		t.Fatalf("revenue visual source = %q, want orders_enriched", model.Visuals["revenue"].Source)
	}
	if got := model.Visuals["orders"].Type; got != "donut" {
		t.Fatalf("orders visual type = %q, want donut", got)
	}
	if got := model.Tables["orders"].DefaultSort.Key; got != "purchase_date" {
		t.Fatalf("orders table default sort = %q, want purchase_date", got)
	}
	if len(model.Pages) != 2 {
		t.Fatalf("page count = %d, want 2", len(model.Pages))
	}
	if got := model.Pages[1].ID; got != "operations" {
		t.Fatalf("second page id = %q, want operations", got)
	}
	if len(model.Relationships) != 6 {
		t.Fatalf("relationship count = %d, want 6", len(model.Relationships))
	}
}
