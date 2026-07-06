package ui

import (
	"encoding/json"
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/pkg/pagestream"
	g "maragu.dev/gomponents"
	dsattr "maragu.dev/gomponents-datastar"
	h "maragu.dev/gomponents/html"
)

func BenchmarkDashboardJSONAttributeBridge(b *testing.B) {
	benchmarkDashboardBridge(b, true)
}

func BenchmarkDashboardDatastarLitBridge(b *testing.B) {
	benchmarkDashboardBridge(b, false)
}

func benchmarkDashboardBridge(b *testing.B, legacy bool) {
	report, model, catalog := benchmarkDashboardFixture()
	activePage := report.Pages[0]
	htmlBytes := 0
	jsonAttrBytes := 0

	b.ReportAllocs()
	for b.Loop() {
		signals := BootstrapSignals(".data/olist", "client", catalog, report, model, report.Pages, activePage, dashboard.Filters{})
		node := benchmarkDashboardDocument(catalog, report, model, activePage, signals, legacy)
		var out strings.Builder
		if err := node.Render(&out); err != nil {
			b.Fatal(err)
		}
		htmlBytes = out.Len()
		if legacy {
			jsonAttrBytes = benchmarkDashboardJSONAttrBytes(signals)
		}
	}

	b.ReportMetric(float64(htmlBytes), "html_bytes/op")
	if legacy {
		b.ReportMetric(float64(jsonAttrBytes), "json_attr_bytes/op")
	}
}

func benchmarkDashboardDocument(catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, activePage dashboard.Page, signals map[string]any, legacy bool) g.Node {
	dashboardUpdatesURL := updatesURL(catalog.Workspace.ID, report.ID, activePage.ID)
	reloadAction := postAction("/workspaces/" + catalog.Workspace.ID + "/commands/reload")
	tableReset := tableResetExpression()
	filtersUpdate := "$filters = evt.detail.filters; $urlParams = evt.detail.urlParams; window.DatastarURLSync && window.DatastarURLSync.replace($urlParams); " + tableReset
	body := benchmarkDatastarLitDashboardRoot(catalog, report, model, filtersUpdate, reloadAction)
	if legacy {
		body = benchmarkLegacyDashboardRoot(catalog, report, model, signals, filtersUpdate, reloadAction)
	}
	mainAttrs := []g.Node{
		h.ID("dashboard"),
		h.Class(appRootClass),
		g.Attr("data-on:datastar-url-params-sync__window", "$urlParams = evt.detail.params; $filters = window.LibreDashFilterURL.fromParams($filterConfig, $filters, $urlParams); "+tableReset+reloadAction),
	}
	if legacy {
		mainAttrs = append(mainAttrs,
			dsattr.Signals(signals),
			g.Attr("data-url-param-shape", jsonString(signals["urlParamShape"])),
		)
	}
	return pagestream.RenderPage(pagestream.PageSpec{
		Title: "LibreDash",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/dashboard-page.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/url-sync.js"))),
			inspectorScript(),
		),
		MainAttrs:  mainAttrs,
		UpdatesURL: dashboardUpdatesURL,
		Body: []g.Node{
			body,
			inspectorElement(),
		},
	})
}

func jsonString(value any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func benchmarkDatastarLitDashboardRoot(catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, filtersUpdate, reloadAction string) g.Node {
	attrs := append([]g.Node{g.Attr("slot", "page")}, benchmarkDashboardCommandAttrs(catalog, report, model, filtersUpdate, reloadAction)...)
	return g.El("ld-app-shell",
		g.El("ld-dashboard-page", attrs...),
	)
}

func benchmarkLegacyDashboardRoot(catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, signals map[string]any, filtersUpdate, reloadAction string) g.Node {
	attrs := []g.Node{
		g.Attr("slot", "page"),
		g.Attr("page", jsonString(signals["page"])),
		g.Attr("filterconfig", jsonString(signals["filterConfig"])),
		g.Attr("filters", jsonString(signals["filters"])),
		g.Attr("filteroptions", jsonString(signals["filterOptions"])),
		g.Attr("visuals", jsonString(signals["visuals"])),
		g.Attr("tables", jsonString(signals["tables"])),
		g.Attr("status", jsonString(signals["status"])),
		g.Attr("data-attr:page", "$page"),
		g.Attr("data-attr:filterconfig", "$filterConfig"),
		g.Attr("data-attr:filters", "$filters"),
		g.Attr("data-attr:filteroptions", "$filterOptions"),
		g.Attr("data-attr:visuals", "$visuals"),
		g.Attr("data-attr:tables", "$tables"),
		g.Attr("data-attr:status", "$status"),
	}
	attrs = append(attrs, benchmarkDashboardCommandAttrs(catalog, report, model, filtersUpdate, reloadAction)...)
	return g.El("ld-app-shell",
		g.Attr("chrome", jsonString(signals["chrome"])),
		g.Attr("data-attr:chrome", "$chrome"),
		g.El("ld-dashboard-page", attrs...),
	)
}

func benchmarkDashboardCommandAttrs(catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, filtersUpdate, reloadAction string) []g.Node {
	return []g.Node{
		g.Attr("data-on:ld-filters-change", filtersUpdate+reloadAction),
		g.Attr("data-on:ld-filters-reset", filtersUpdate+postAction("/workspaces/"+catalog.Workspace.ID+"/commands/reset-filters")),
		g.Attr("data-on:ld-filters-refresh", reloadAction),
		g.Attr("data-on:ld-selection-clear", "$filters.selections = []; "+postAction("/workspaces/"+catalog.Workspace.ID+"/commands/clear-selection")),
		g.Attr("data-on:ld-interaction-select", "$interactionCommand = evt.detail; "+postAction("/workspaces/"+catalog.Workspace.ID+"/commands/select")),
		g.Attr("data-on:ld-table-window-change", "$tableCommand = evt.detail; "+postAction("/workspaces/"+catalog.Workspace.ID+"/commands/table-window")),
		g.Attr("data-on:ld-refresh-materializations", postAction("/workspaces/"+catalog.Workspace.ID+"/commands/refresh-materializations?model="+model.Name+"&dashboard="+report.ID)),
	}
}

func benchmarkDashboardJSONAttrBytes(signals map[string]any) int {
	total := 0
	for _, key := range []string{"chrome", "page", "filterConfig", "filters", "filterOptions", "visuals", "tables", "status"} {
		total += len(jsonString(signals[key]))
	}
	return total
}

func benchmarkDashboardFixture() (reportdef.Dashboard, *semanticmodel.Model, dashboard.Catalog) {
	zebra := true
	filters := map[string]reportdef.FilterDefinition{
		"state":    {Type: "multi_select", Label: "State", Dimension: "orders.state", URLParam: "state", Operator: "in"},
		"category": {Type: "multi_select", Label: "Category", Dimension: "orders.category", URLParam: "category", Operator: "in"},
		"status":   {Type: "multi_select", Label: "Status", Dimension: "orders.status", URLParam: "status", Operator: "in"},
		"channel":  {Type: "multi_select", Label: "Channel", Dimension: "orders.channel", URLParam: "channel", Operator: "in"},
	}
	visuals := map[string]reportdef.Visual{}
	components := []dashboard.PageVisual{}
	for i, kind := range []string{"bar_chart", "line_chart", "area_chart", "column_chart", "pie_chart", "donut_chart", "scatter_chart", "treemap_chart"} {
		id := "visual_" + string(rune('a'+i))
		visuals[id] = reportdef.Visual{
			Title: "Benchmark Visual " + string(rune('A'+i)),
			Type:  "bar",
			Query: reportdef.VisualQuery{
				Dimensions: fieldRefs("orders.status"),
				Measures:   fieldRefs("order_count"),
			},
		}
		components = append(components, dashboard.PageVisual{ID: id, Kind: kind, Visual: id, X: float64((i % 4) * 300), Y: float64((i / 4) * 180), Width: 280, Height: 160})
	}
	for i, filterID := range []string{"state", "category", "status", "channel"} {
		components = append(components, dashboard.PageVisual{ID: filterID + "_filter", Kind: "filter_card", Filter: filterID, X: float64(i * 220), Y: 390, Width: 200, Height: 120})
	}
	tables := map[string]reportdef.TableVisual{}
	for i := 0; i < 4; i++ {
		id := "table_" + string(rune('a'+i))
		tables[id] = reportdef.TableVisual{
			Title: "Benchmark Table " + string(rune('A'+i)),
			Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id", "orders.status", "orders.state", "orders.category"}},
			Style: dashboard.TableStyle{Density: "compact", Grid: "full", Zebra: &zebra},
			Columns: []dashboard.TableColumn{
				{Key: "order_id", Label: "Order", Width: 180, Format: "text"},
				{Key: "status", Label: "Status", Width: 140, Format: "text"},
				{Key: "state", Label: "State", Width: 100, Format: "text"},
				{Key: "category", Label: "Category", Width: 180, Format: "text"},
			},
		}
		components = append(components, dashboard.PageVisual{ID: id, Kind: "table", Table: id, X: float64(i * 300), Y: 540, Width: 280, Height: 220})
	}
	report := reportdef.Dashboard{
		ID:            "benchmark-dashboard",
		Title:         "Benchmark Dashboard",
		SemanticModel: "benchmark",
		Filters:       filters,
		Visuals:       visuals,
		Tables:        tables,
		Pages: []dashboard.Page{{
			ID:      "overview",
			Title:   "Overview",
			Canvas:  dashboard.PageCanvas{Width: 1366, Height: 940},
			Grid:    dashboard.PageGrid{Columns: 12, RowHeight: 48, Gap: 16, Padding: 16},
			Visuals: components,
		}},
	}
	model := &semanticmodel.Model{
		Name:  "benchmark",
		Title: "Benchmark Semantic Model",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Kind:       "fact",
				Source:     "orders",
				PrimaryKey: "order_id",
				Grain:      "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Expr: "order_id"},
					"status":   {Expr: "status"},
					"state":    {Expr: "state"},
					"category": {Expr: "category"},
					"channel":  {Expr: "channel"},
				},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{"order_count": {Table: "orders", Grain: "order_id", Label: "Orders", Expression: "COUNT(*)"}},
	}
	catalog := dashboard.Catalog{Workspace: dashboard.CatalogWorkspace{ID: "benchmark", Title: "Benchmark Workspace"}}
	return report, model, catalog
}
