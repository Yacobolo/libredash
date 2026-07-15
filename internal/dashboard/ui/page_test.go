package ui

import (
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"html"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

func fieldRefs(fields ...string) []reportdef.FieldRef {
	refs := make([]reportdef.FieldRef, len(fields))
	for i, field := range fields {
		refs[i] = reportdef.FieldRef{Field: field}
	}
	return refs
}

func TestPageInitialSignalsArePageScoped(t *testing.T) {
	report := reportdef.Dashboard{
		ID:            "report",
		Title:         "Report",
		SemanticModel: "test",
		Filters: map[string]reportdef.FilterDefinition{
			"state":    {Type: "multi_select", Label: "State", Dimension: "orders.state", URLParam: "state", Operator: "in"},
			"category": {Type: "text", Label: "Category", Dimension: "orders.category", URLParam: "category", DefaultOperator: "contains"},
		},
		Visuals: map[string]reportdef.Visual{
			"active_chart":   {Title: "Active", Type: "bar", Query: reportdef.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Interaction: reportdef.Interaction{PointSelection: reportdef.SelectionInteraction{Mappings: []reportdef.SelectionMapping{{Field: "orders.status", Value: "label"}}, Targets: []string{"orders"}}}},
			"active_kpi":     {Kind: "kpi", Shape: "single_value", Query: reportdef.VisualQuery{Measures: fieldRefs("order_count")}, Options: map[string]any{"note": "Filtered", "tone": "ink"}},
			"off_page_chart": {Title: "Off Page", Type: "bar", Query: reportdef.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}},
		},
		Tables: map[string]reportdef.TableVisual{
			"orders":   {Title: "Orders", Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}, Interaction: reportdef.Interaction{RowSelection: reportdef.SelectionInteraction{Mappings: []reportdef.SelectionMapping{{Field: "orders.order_id", Value: "order_id"}}, Targets: []string{"active_chart"}}}, Style: dashboard.TableStyle{Density: "compact", Grid: "full"}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order", Width: 220, Format: "text"}}},
			"matrix":   {Title: "Matrix", Kind: "matrix_table", Query: reportdef.TableQuery{Rows: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Columns: []dashboard.TableColumn{{Key: "status", Label: "Status"}}},
			"pivot":    {Title: "Pivot", Kind: "pivot_table", Query: reportdef.TableQuery{Rows: fieldRefs("orders.status"), Columns: fieldRefs("orders.category"), Measures: fieldRefs("order_count")}, Columns: []dashboard.TableColumn{{Key: "status", Label: "Status"}}},
			"off_page": {Title: "Off Page", Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order"}}},
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
	model := &semanticmodel.Model{
		Name:  "test",
		Title: "Test",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Expr: "order_id"}, "status": {Expr: "status"}, "state": {Expr: "state"}, "category": {Expr: "category"}},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{"order_count": {Table: "orders", Grain: "order_id", Label: "Orders", Expression: "COUNT(*)"}},
	}

	showcase := renderPageForTest(t, report, model, report.Pages[0])
	if !strings.Contains(showcase, `<ld-dashboard-page`) || !strings.Contains(showcase, `data-on:ld-filters-change`) || !strings.Contains(showcase, `data-on:ld-interaction-select`) {
		t.Fatalf("showcase page did not mount dashboard route root with command bridge:\n%s", showcase)
	}
	if strings.Contains(showcase, `data-signals=`) || !strings.Contains(showcase, `data-init="@get('/updates?`) {
		t.Fatalf("showcase page did not render stream-first structural shell:\n%s", showcase)
	}
	for _, attr := range []string{
		` chrome="`, ` page="`, ` filterconfig="`, ` filters="`, ` filteroptions="`, ` visuals="`, ` tables="`, ` status="`,
		`data-attr:chrome`, `data-attr:page`, `data-attr:filterconfig`, `data-attr:filters`, `data-attr:filteroptions`, `data-attr:visuals`, `data-attr:tables`, `data-attr:status`,
	} {
		if strings.Contains(showcase, attr) {
			t.Fatalf("showcase page rendered migrated dashboard bridge attribute %q:\n%s", attr, showcase)
		}
	}
	if !strings.Contains(showcase, `/commands/reload`) || strings.Contains(showcase, `data-url-param-shape`) {
		t.Fatalf("showcase page did not wire dashboard reload command without URL shape attribute:\n%s", showcase)
	}
	for _, attr := range []string{"data-on:ld-filters-change", "data-on:ld-filters-refresh", "data-on:datastar-url-params-sync__window"} {
		segment := renderedAttrSegment(showcase, attr)
		if !strings.Contains(segment, `/commands/reload`) || strings.Contains(segment, `@get($runtime.updatesUrl`) {
			t.Fatalf("%s segment = %q, want reload command without updates stream reopen:\n%s", attr, segment, showcase)
		}
	}
	showcaseSignals := html.UnescapeString(jsonString(BootstrapSignals("client", dashboard.Catalog{}, report, model, report.Pages, report.Pages[0], dashboard.Filters{})))
	if !strings.Contains(showcaseSignals, `"active_chart"`) || !strings.Contains(showcaseSignals, `"active_kpi"`) {
		t.Fatalf("showcase bootstrap did not include active chart and KPI visuals:\n%s", showcaseSignals)
	}
	if strings.Contains(showcaseSignals, `"off_page_chart"`) {
		t.Fatalf("showcase bootstrap included off-page chart:\n%s", showcaseSignals)
	}
	if strings.Contains(showcaseSignals, `"kpis"`) {
		t.Fatalf("showcase bootstrap included legacy kpis signal:\n%s", showcaseSignals)
	}
	for _, forbidden := range []string{`"updatesUrl"`, `"routeKey"`, `"csrfToken"`} {
		if strings.Contains(showcaseSignals, forbidden) {
			t.Fatalf("showcase bootstrap leaked %s:\n%s", forbidden, showcaseSignals)
		}
	}
	assertNoDashboardProductDOM(t, showcase)
	if !strings.Contains(showcaseSignals, `"tables":{}`) {
		t.Fatalf("showcase bootstrap should include no tables:\n%s", showcaseSignals)
	}
	if !strings.Contains(showcaseSignals, `"filterConfig":[{`) || !strings.Contains(showcaseSignals, `"id":"state"`) {
		t.Fatalf("showcase bootstrap did not include active page filter config:\n%s", showcaseSignals)
	}
	if !strings.Contains(showcaseSignals, `"controls":{"state"`) {
		t.Fatalf("showcase bootstrap did not include active page filter controls:\n%s", showcaseSignals)
	}
	if strings.Contains(showcaseSignals, `"id":"category"`) || strings.Contains(showcaseSignals, `"category":""`) {
		t.Fatalf("showcase bootstrap included off-page category filter:\n%s", showcaseSignals)
	}

	tables := renderPageForTest(t, report, model, report.Pages[1])
	tableSignals := html.UnescapeString(jsonString(BootstrapSignals("client", dashboard.Catalog{}, report, model, report.Pages, report.Pages[1], dashboard.Filters{})))
	for _, tableID := range []string{"orders", "matrix", "pivot"} {
		if !strings.Contains(tableSignals, `"`+tableID+`":{`) || !strings.Contains(tableSignals, `"availableRows"`) {
			t.Fatalf("tables bootstrap did not include table %q with row metadata:\n%s", tableID, tableSignals)
		}
	}
	if !strings.Contains(tableSignals, `"style":{"density":"compact"`) || !strings.Contains(tableSignals, `"rowHeight":28`) || !strings.Contains(tableSignals, `"width":220`) {
		t.Fatalf("tables bootstrap did not include table style and column display metadata:\n%s", tableSignals)
	}
	assertNoDashboardProductDOM(t, tables)
	if !strings.Contains(showcaseSignals, `"kind":"point_selection"`) || !strings.Contains(showcaseSignals, `"field":"orders.status","value":"label"`) || strings.Contains(showcaseSignals, `"mode":"multi"`) {
		t.Fatalf("showcase bootstrap did not include point selection without mode:\n%s", showcaseSignals)
	}
	if !strings.Contains(tableSignals, `"kind":"row_selection"`) || !strings.Contains(tableSignals, `"field":"orders.order_id","value":"order_id"`) || strings.Contains(tableSignals, `"mode":"multi"`) {
		t.Fatalf("tables bootstrap did not include row selection without mode:\n%s", tableSignals)
	}
	if strings.Contains(tableSignals, `"off_page"`) {
		t.Fatalf("tables bootstrap included off-page table:\n%s", tableSignals)
	}
	if !strings.Contains(tableSignals, `"visuals":{}`) {
		t.Fatalf("tables bootstrap should include no visuals:\n%s", tableSignals)
	}
}

func renderPageForTest(t *testing.T, report reportdef.Dashboard, model *semanticmodel.Model, activePage dashboard.Page) string {
	t.Helper()
	var out strings.Builder
	err := Page("client", "", dashboard.Catalog{}, report, model, report.Pages, activePage, dashboard.Filters{}).Render(&out)
	if err != nil {
		t.Fatal(err)
	}
	return html.UnescapeString(out.String())
}

func renderedAttrSegment(body, name string) string {
	prefix := name + `="`
	start := strings.Index(body, prefix)
	if start < 0 {
		return ""
	}
	end := start + 1000
	if end > len(body) {
		end = len(body)
	}
	return body[start:end]
}

func assertNoDashboardProductDOM(t *testing.T, body string) {
	t.Helper()
	for _, tag := range []string{
		"ld-sub-sidebar",
		"ld-report-canvas",
		"ld-filter-panel",
		"ld-filter-card",
		"ld-kpi-card",
		"ld-echart",
		"ld-report-table",
		"ld-report-footer",
		"ld-visual-modal",
	} {
		if strings.Contains(body, "<"+tag) {
			t.Fatalf("Go rendered dashboard product DOM <%s> below route root:\n%s", tag, body)
		}
	}
}
