package ui

import (
	"html"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
)

func fieldRefs(fields ...string) []semantic.FieldRef {
	refs := make([]semantic.FieldRef, len(fields))
	for i, field := range fields {
		refs[i] = semantic.FieldRef{Field: field}
	}
	return refs
}

func TestPageInitialSignalsArePageScoped(t *testing.T) {
	report := semantic.Dashboard{
		ID:          "report",
		Title:       "Report",
		MetricViews: []string{"orders"},
		Filters: map[string]semantic.FilterDefinition{
			"state":    {Type: "multi_select", Label: "State", MetricView: "orders", Dimension: "state", URLParam: "state", Operator: "in"},
			"category": {Type: "text", Label: "Category", MetricView: "orders", Dimension: "category", URLParam: "category", DefaultOperator: "contains"},
		},
		Visuals: map[string]semantic.Visual{
			"active_chart":   {Title: "Active", Type: "bar", MetricView: "orders", Query: semantic.VisualQuery{Dimensions: fieldRefs("status"), Measures: fieldRefs("order_count")}},
			"active_kpi":     {Kind: "kpi", Shape: "single_value", MetricView: "orders", Query: semantic.VisualQuery{Measures: fieldRefs("order_count")}, Options: map[string]any{"note": "Filtered", "tone": "ink"}},
			"off_page_chart": {Title: "Off Page", Type: "bar", MetricView: "orders", Query: semantic.VisualQuery{Dimensions: fieldRefs("status"), Measures: fieldRefs("order_count")}},
		},
		Tables: map[string]semantic.TableVisual{
			"orders":   {Title: "Orders", MetricView: "orders", Style: dashboard.TableStyle{Density: "compact", Grid: "full"}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order", Width: 220, Format: "text"}}},
			"matrix":   {Title: "Matrix", Kind: "matrix_table", MetricView: "orders", Columns: []dashboard.TableColumn{{Key: "status", Label: "Status"}}},
			"pivot":    {Title: "Pivot", Kind: "pivot_table", MetricView: "orders", Columns: []dashboard.TableColumn{{Key: "status", Label: "Status"}}},
			"off_page": {Title: "Off Page", MetricView: "orders", Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order"}}},
		},
		Pages: []dashboard.Page{
			{
				ID:     "showcase",
				Title:  "Showcase",
				Canvas: dashboard.PageCanvas{Width: 1200, Height: 800},
				Visuals: []dashboard.PageVisual{
					{ID: "state-filter", Kind: "filter_card", Filter: "state", X: 0, Y: 0, Width: 100, Height: 40},
					{ID: "kpi", Kind: "kpi_card", Visual: "active_kpi", X: 0, Y: 0, Width: 100, Height: 100},
					{ID: "chart", Kind: "bar_chart", Visual: "active_chart", X: 0, Y: 0, Width: 100, Height: 100},
				},
			},
			{
				ID:     "tables",
				Title:  "Tables",
				Canvas: dashboard.PageCanvas{Width: 1200, Height: 800},
				Visuals: []dashboard.PageVisual{
					{ID: "orders", Kind: "table", Table: "orders", X: 0, Y: 0, Width: 100, Height: 100},
					{ID: "matrix", Kind: "table", Table: "matrix", X: 0, Y: 120, Width: 100, Height: 100},
					{ID: "pivot", Kind: "table", Table: "pivot", X: 120, Y: 120, Width: 100, Height: 100},
				},
			},
		},
	}
	model := &semantic.Model{
		Name:  "test",
		Title: "Test",
		Tables: map[string]semantic.ModelTable{
			"orders": {
				Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
				Dimensions: map[string]semantic.MetricDimension{"order_id": {Expr: "order_id"}},
				Measures:   map[string]semantic.MetricMeasure{"order_count": {Label: "Orders", Expression: "COUNT(*)"}},
			},
		},
	}

	showcase := renderPageForTest(t, report, model, report.Pages[0])
	if !strings.Contains(showcase, `"active_chart"`) || !strings.Contains(showcase, `"active_kpi"`) {
		t.Fatalf("showcase page did not seed active chart and KPI visuals:\n%s", showcase)
	}
	if strings.Contains(showcase, `"off_page_chart"`) {
		t.Fatalf("showcase page seeded off-page chart:\n%s", showcase)
	}
	if strings.Contains(showcase, `"kpis"`) {
		t.Fatalf("showcase page seeded legacy kpis signal:\n%s", showcase)
	}
	if !strings.Contains(showcase, `<ld-kpi-card`) {
		t.Fatalf("showcase page did not render kpi card component:\n%s", showcase)
	}
	if !strings.Contains(showcase, `"tables":{}`) {
		t.Fatalf("showcase page should seed no tables:\n%s", showcase)
	}
	if !strings.Contains(showcase, `"filterConfig":[{"id":"state"`) {
		t.Fatalf("showcase page did not seed active page filter config:\n%s", showcase)
	}
	if !strings.Contains(showcase, `"controls":{"state"`) {
		t.Fatalf("showcase page did not seed active page filter controls:\n%s", showcase)
	}
	if strings.Contains(showcase, `"id":"category"`) || strings.Contains(showcase, `"category":""`) {
		t.Fatalf("showcase page seeded off-page category filter:\n%s", showcase)
	}

	tables := renderPageForTest(t, report, model, report.Pages[1])
	if !strings.Contains(tables, `"orders":{"availableRows"`) || !strings.Contains(tables, `"matrix":{"availableRows"`) || !strings.Contains(tables, `"pivot":{"availableRows"`) {
		t.Fatalf("tables page did not seed its three tables:\n%s", tables)
	}
	if !strings.Contains(tables, `"style":{"density":"compact"`) || !strings.Contains(tables, `"rowHeight":28`) || !strings.Contains(tables, `"width":220`) {
		t.Fatalf("tables page did not seed table style and column display metadata:\n%s", tables)
	}
	if strings.Contains(tables, `"off_page"`) {
		t.Fatalf("tables page seeded off-page table:\n%s", tables)
	}
	if !strings.Contains(tables, `"visuals":{}`) {
		t.Fatalf("tables page should seed no visuals:\n%s", tables)
	}
}

func renderPageForTest(t *testing.T, report semantic.Dashboard, model *semantic.Model, activePage dashboard.Page) string {
	t.Helper()
	var out strings.Builder
	err := Page(".data", "client", "", dashboard.Catalog{}, report, model, report.Pages, activePage, dashboard.Filters{}).Render(&out)
	if err != nil {
		t.Fatal(err)
	}
	return html.UnescapeString(out.String())
}
