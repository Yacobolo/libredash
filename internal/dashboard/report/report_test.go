package report

import (
	"context"
	"reflect"
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"gopkg.in/yaml.v3"
)

type fakeMetrics struct {
	report Dashboard
	tables []dashboard.TableRequest
}

func (m *fakeMetrics) DefaultFilters(string) dashboard.Filters {
	return dashboard.Filters{Controls: map[string]dashboard.FilterControl{
		"state": {Type: "multi_select", Operator: "in"},
	}}
}

func (m *fakeMetrics) Report(string) (Dashboard, *semanticmodel.Model, bool) {
	return m.report, &semanticmodel.Model{Name: "model"}, true
}

func (m *fakeMetrics) QueryTablePage(_ context.Context, _, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	m.tables = append(m.tables, request)
	return dashboard.Table{Title: request.Table, Sort: request.Sort}, nil
}

func TestActivePageResolution(t *testing.T) {
	page, ok := ActivePage(nil, "")
	if !ok || page.ID != "overview" || page.Canvas.Width != 1366 {
		t.Fatalf("default page = %#v, %v", page, ok)
	}

	pages := []dashboard.Page{{ID: "a", Title: "A"}, {ID: "b", Title: "B", Canvas: dashboard.PageCanvas{Width: 900}}}
	page, ok = ActivePage(pages, "b")
	if !ok || page.ID != "b" || page.Canvas.Width != 900 || page.Canvas.Height != 940 {
		t.Fatalf("active page = %#v, %v", page, ok)
	}
	if _, ok := ActivePage(pages, "missing"); ok {
		t.Fatal("missing explicit page resolved")
	}
}

func TestVisualQueryRejectsInlineMeasure(t *testing.T) {
	var query VisualQuery
	err := yaml.Unmarshal([]byte("measures:\n  revenue:\n    expr: SUM(orders.revenue)\n"), &query)
	if err == nil || !strings.Contains(err.Error(), "inline dashboard measures are not supported") {
		t.Fatalf("Unmarshal() error = %v", err)
	}
}

func TestNormalizeFiltersUsesActivePageDefinitions(t *testing.T) {
	metrics := &fakeMetrics{report: Dashboard{
		Filters: map[string]FilterDefinition{
			"state":    {Type: "multi_select", Label: "State", URLParam: "state", Operator: "in"},
			"category": {Type: "text", Label: "Category", URLParam: "category", DefaultOperator: "contains"},
		},
		Pages: []dashboard.Page{
			{ID: "overview", Visuals: []dashboard.PageVisual{{Kind: "filter_card", Filter: "state"}}},
			{ID: "ops", Visuals: []dashboard.PageVisual{{Kind: "filter_card", Filter: "category"}}},
		},
	}}

	filters := NormalizeFilters(metrics, "dash", "overview", dashboard.Filters{Controls: map[string]dashboard.FilterControl{
		"state":    {Type: "multi_select", Operator: "in", Values: []string{"SP"}},
		"category": {Type: "text", Operator: "contains", Value: "ignored"},
	}})

	if _, ok := filters.Controls["state"]; !ok {
		t.Fatalf("state filter missing: %#v", filters.Controls)
	}
	if _, ok := filters.Controls["category"]; ok {
		t.Fatalf("off-page category filter kept: %#v", filters.Controls)
	}
}

func TestTablesBuildsPageScopedRequests(t *testing.T) {
	metrics := &fakeMetrics{report: Dashboard{
		Tables: map[string]TableVisual{
			"orders": {DefaultSort: dashboard.TableSort{Key: "purchase_date", Direction: "desc"}},
			"states": {DefaultSort: dashboard.TableSort{Key: "state", Direction: "asc"}},
		},
		Pages: []dashboard.Page{{ID: "overview", Visuals: []dashboard.PageVisual{
			{Kind: "table", Table: "orders"},
			{Kind: "table", Table: "orders"},
			{Kind: "table", Table: "states"},
		}}},
	}}

	tables := Tables(context.Background(), metrics, "dash", "overview", dashboard.Filters{}, dashboard.TableRequest{Start: 200, Count: 10})
	if !reflect.DeepEqual(PageTableNames(metrics.report.Pages, "overview"), []string{"orders", "states"}) {
		t.Fatalf("table names = %#v", PageTableNames(metrics.report.Pages, "overview"))
	}
	if len(tables) != 2 || len(metrics.tables) != 2 {
		t.Fatalf("tables = %#v, requests = %#v", tables, metrics.tables)
	}
	if got := metrics.tables[0]; got.Table != "orders" || got.Start != 0 || got.Count != dashboard.TableChunkSize || got.Sort.Key != "purchase_date" {
		t.Fatalf("orders request = %#v", got)
	}
}
