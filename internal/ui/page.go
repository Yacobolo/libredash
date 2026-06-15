package ui

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

func updateAction(dashboardID, pageID string) string {
	return "@get('/updates?dashboard=" + dashboardID + "&page=" + pageID + "', {openWhenHidden: true})"
}

func Page(dataDir, clientID string, catalog dashboard.Catalog, report semantic.Dashboard, model *semantic.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters) g.Node {
	if activePage.ID == "" {
		activePage = defaultPage()
	}
	action := updateAction(report.ID, activePage.ID)
	initAction := "window.DatastarURLSync && window.DatastarURLSync.bindPopstate($urlParamShape); " + action
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: []g.Node{
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Href("https://cdn.jsdelivr.net/npm/daisyui@5"), h.Rel("stylesheet"), h.Type("text/css")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			h.Link(h.Rel("stylesheet"), h.Href("/static/app.css")),
			h.Script(h.Type("module"), h.Src("/static/theme.js")),
			h.Script(h.Type("module"), h.Src("/static/url-sync.js")),
			h.Script(h.Type("module"), h.Src("/static/sidebar.js")),
			h.Script(h.Type("module"), h.Src("/static/filter-dock.js")),
			h.Script(h.Type("module"), h.Src("/static/filter-panel.js")),
			h.Script(h.Type("module"), h.Src("/static/report-canvas.js")),
			h.Script(h.Type("module"), h.Src("/static/report-footer.js")),
			h.Script(h.Type("module"), h.Src("/static/charts.js")),
			h.Script(h.Type("module"), h.Src("/static/table.js")),
			h.Script(h.Type("module"), h.Src("/static/datastar-inspector.js")),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		},
		Body: []g.Node{
			h.Main(
				h.ID("dashboard"),
				h.Class("report-app"),
				ds.Signals(initialSignals(dataDir, clientID, report, model, activePage, initialFilters)),
				ds.Init(initAction),
				g.Attr("data-on:datastar-url-params-sync__window", "$urlParams = evt.detail.params; $filters = window.LibreDashFilterURL.fromParams($filterConfig, $filters, $urlParams); $tableCommand.offset = 0; "+action),
				h.Div(h.Class("app-shell"),
					sidebar(sidebarConfigForReport(catalog, report, model, activePage), true, "@post('/commands/refresh-cache?model="+model.Name+"&dashboard="+report.ID+"')"),
					h.Section(h.Class("app-main report-main"), h.Aria("label", "LibreDash report canvas"),
						workspaceHeader(
							"",
							report.Title,
							"",
							pageTabs(report.ID, pages, activePage.ID),
						),
						h.Div(h.Class("report-dashboard-shell"),
							h.Div(h.Class("report-canvas-shell"),
								renderPageCanvas(activePage),
							),
							filtersDock(report, action),
						),
						g.El("ld-report-footer",
							h.Aria("label", "Report view controls"),
							g.Attr("data-attr:status", "$status"),
						),
					),
				),
				g.El("datastar-inspector"),
			),
		},
	})
}

func CatalogPage(catalog dashboard.Catalog) g.Node {
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Catalog",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: []g.Node{
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Href("https://cdn.jsdelivr.net/npm/daisyui@5"), h.Rel("stylesheet"), h.Type("text/css")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			h.Link(h.Rel("stylesheet"), h.Href("/static/app.css")),
			h.Script(h.Type("module"), h.Src("/static/theme.js")),
			h.Script(h.Type("module"), h.Src("/static/sidebar.js")),
		},
		Body: []g.Node{
			h.Main(h.Class("report-app"),
				h.Div(h.Class("app-shell"),
					sidebar(sidebarConfigForCatalog(catalog), false, ""),
					h.Section(h.Class("app-main catalog-main"), h.Aria("label", "LibreDash dashboard catalog"),
						workspaceHeader(
							"Workspace catalog",
							"Dashboards",
							"Discover reports backed by reusable semantic models and DuckDB import caches.",
							h.Div(h.Class("catalog-stats"),
								modelStat("Models", len(catalog.Models)),
								modelStat("Dashboards", len(catalog.Dashboards)),
							),
						),
						h.Div(h.Class("catalog-grid"),
							g.Map(catalog.Dashboards, dashboardCard),
						),
					),
				),
			),
		},
	})
}

func dashboardCard(report dashboard.CatalogDashboard) g.Node {
	return h.Article(h.Class("catalog-card"),
		h.Div(h.Class("catalog-card-main"),
			h.P(h.Class("report-eyebrow"), g.Text(report.ModelTitle)),
			h.H2(g.Text(report.Title)),
			h.P(g.Text(report.Description)),
		),
		h.Div(h.Class("catalog-tags"),
			g.Map(report.Tags, func(tag string) g.Node {
				return h.Span(g.Text(tag))
			}),
		),
		h.Footer(h.Class("catalog-card-footer"),
			h.Span(g.Textf("%d pages", report.PageCount)),
			h.A(h.Class("catalog-open"), h.Href("/dashboards/"+report.ID),
				lucide.ExternalLink(iconAttrs()),
				h.Span(g.Text("Open")),
			),
		),
	)
}

func ModelPage(catalog dashboard.Catalog, model dashboard.ModelGraph) g.Node {
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Model",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: []g.Node{
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Href("https://cdn.jsdelivr.net/npm/daisyui@5"), h.Rel("stylesheet"), h.Type("text/css")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			h.Link(h.Rel("stylesheet"), h.Href("/static/app.css")),
			h.Link(h.Rel("stylesheet"), h.Href("/static/model-graph.css")),
			h.Script(h.Type("module"), h.Src("/static/theme.js")),
			h.Script(h.Type("module"), h.Src("/static/sidebar.js")),
			h.Script(h.Type("module"), h.Src("/static/model-graph.js")),
		},
		Body: []g.Node{
			h.Main(
				h.ID("model"),
				h.Class("report-app"),
				h.Div(h.Class("app-shell"),
					sidebar(sidebarConfigForModel(catalog, model), false, ""),
					h.Section(h.Class("app-main model-main"), h.Aria("label", "LibreDash semantic model"),
						workspaceHeader(
							"Semantic model",
							model.Title,
							model.Name,
							modelStats(model.Stats),
						),
						h.Div(h.Class("model-shell"),
							g.El("ld-model-graph", g.Attr("data-model", modelGraphJSON(model))),
						),
					),
				),
			),
		},
	})
}

func defaultPage() dashboard.Page {
	return dashboard.Page{
		ID:     "overview",
		Title:  "Overview",
		Canvas: dashboard.PageCanvas{Width: 1366, Height: 940},
		Grid:   dashboard.PageGrid{Columns: 12, RowHeight: 48, Gap: 16, Padding: 16},
	}
}

func modelStats(stats dashboard.ModelStats) g.Node {
	return h.Div(h.Class("model-stats"),
		modelStat("Sources", stats.Sources),
		modelStat("Cache", stats.CacheTables),
		modelStat("Metrics", stats.Metrics),
		modelStat("Visuals", stats.Visuals),
		modelStat("Relations", stats.Relationships),
	)
}

func modelStat(label string, value int) g.Node {
	return h.Div(h.Class("model-stat"),
		h.Strong(g.Text(strconv.Itoa(value))),
		h.Span(g.Text(label)),
	)
}

func modelGraphJSON(model dashboard.ModelGraph) string {
	bytes, err := json.Marshal(model)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func sidebar(config map[string]any, dynamicStatus bool, refreshAction string) g.Node {
	attrs := []g.Node{
		g.Attr("config", jsonString(config)),
	}
	if dynamicStatus {
		attrs = append(attrs,
			g.Attr("data-attr:status", "$status"),
			g.Attr("data-on:ld-sidebar-refresh", refreshAction),
		)
	}
	return g.El("ld-sidebar", attrs...)
}

func sidebarConfigForCatalog(catalog dashboard.Catalog) map[string]any {
	modelID := ""
	modelTitle := ""
	if len(catalog.Dashboards) > 0 {
		report := catalog.Dashboards[0]
		modelID = report.SemanticModel
		modelTitle = report.ModelTitle
	}
	return sidebarConfig(catalog, "catalog", "", "LibreDash Workspace", "Dashboard catalog", "Dashboards", modelID, modelTitle, false)
}

func sidebarConfigForReport(catalog dashboard.Catalog, report semantic.Dashboard, model *semantic.Model, activePage dashboard.Page) map[string]any {
	return sidebarConfig(catalog, "dashboard:"+report.ID, report.ID, "LibreDash Workspace", report.Title, activePage.Title, model.Name, model.Title, true)
}

func sidebarConfigForModel(catalog dashboard.Catalog, model dashboard.ModelGraph) map[string]any {
	return sidebarConfig(catalog, "model:"+model.Name, "", "LibreDash Workspace", "Semantic model", model.Title, model.Name, model.Title, false)
}

func sidebarConfig(catalog dashboard.Catalog, active, dashboardID, workspaceTitle, dashboardTitle, pageTitle, modelID, modelTitle string, refresh bool) map[string]any {
	return map[string]any{
		"workspaceTitle": workspaceTitle,
		"active":         active,
		"dashboardId":    dashboardID,
		"dashboardTitle": dashboardTitle,
		"pageTitle":      pageTitle,
		"modelId":        modelID,
		"modelTitle":     modelTitle,
		"refresh":        refresh,
		"groups":         sidebarGroups(catalog),
	}
}

func sidebarGroups(catalog dashboard.Catalog) []map[string]any {
	return []map[string]any{
		{
			"label": "Workspace",
			"items": []map[string]any{
				{"id": "catalog", "label": "Catalog", "href": "/", "icon": "catalog", "meta": "Dashboards and models"},
			},
		},
		{
			"label": "Dashboards",
			"items": dashboardItems(catalog.Dashboards),
		},
		{
			"label": "Semantic Models",
			"items": modelItems(catalog.Models),
		},
		{
			"label": "Data Platform",
			"items": []map[string]any{
				{"id": "data:sources", "label": "Sources", "href": "/", "icon": "data", "meta": "Coming soon", "disabled": true},
				{"id": "data:cache", "label": "DuckDB Cache", "href": "/", "icon": "cache", "meta": "Import mode", "disabled": true},
			},
		},
	}
}

func dashboardItems(reports []dashboard.CatalogDashboard) []map[string]any {
	items := make([]map[string]any, 0, len(reports))
	for _, report := range reports {
		items = append(items, map[string]any{
			"id":    "dashboard:" + report.ID,
			"label": report.Title,
			"href":  "/dashboards/" + report.ID,
			"icon":  "dashboard",
			"meta":  report.ModelTitle,
		})
	}
	return items
}

func modelItems(models []dashboard.CatalogModel) []map[string]any {
	items := make([]map[string]any, 0, len(models))
	for _, model := range models {
		items = append(items, map[string]any{
			"id":    "model:" + model.ID,
			"label": model.Title,
			"href":  "/models/" + model.ID,
			"icon":  "model",
			"meta":  "Semantic model",
		})
	}
	return items
}

func workspaceHeader(eyebrow, title, detail string, actions g.Node) g.Node {
	return h.Header(h.Class("workspace-header"),
		h.Div(h.Class("workspace-title-block"),
			g.If(eyebrow != "", h.P(h.Class("report-eyebrow"), g.Text(eyebrow))),
			h.H1(h.Class("workspace-title"), g.Text(title)),
			g.If(detail != "", h.P(h.Class("workspace-detail"), g.Text(detail))),
		),
		h.Div(h.Class("workspace-actions"), actions),
	)
}

func jsonString(value any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func initialSignals(dataDir, clientID string, report semantic.Dashboard, model *semantic.Model, activePage dashboard.Page, initialFilters dashboard.Filters) map[string]any {
	tableRequest := defaultTableRequest(report)
	initialFilters = initialFilters.WithDefaults()
	return map[string]any{
		"runtime": map[string]any{
			"clientId":    clientID,
			"dashboardId": report.ID,
			"pageId":      activePage.ID,
			"modelId":     model.Name,
		},
		"filterConfig":  report.Filters,
		"filters":       initialFilters,
		"urlParams":     report.URLParamsFromFilters(initialFilters),
		"urlParamShape": report.URLParamShape(),
		"filterOptions": map[string]any{},
		"chartCommand": map[string]any{
			"visualId": "",
			"field":    "",
			"value":    "",
			"label":    "",
			"mode":     "toggle",
		},
		"tableCommand": map[string]any{
			"table":  tableRequest.Table,
			"offset": tableRequest.Offset,
			"limit":  tableRequest.Limit,
			"sort": map[string]any{
				"key":       tableRequest.Sort.Key,
				"direction": tableRequest.Sort.Direction,
			},
		},
		"tables": tableSignals(report, tableRequest),
		"charts": chartSignals(report, model),
		"kpis":   []any{},
		"status": map[string]any{
			"loading":       false,
			"error":         "",
			"lastUpdated":   "",
			"dataDirectory": dataDir,
			"setupRequired": false,
		},
	}
}

func defaultTableRequest(report semantic.Dashboard) dashboard.TableRequest {
	request := dashboard.TableRequest{Offset: 0, Limit: 120}
	for _, name := range sortedKeys(report.Tables) {
		table := report.Tables[name]
		request.Table = name
		request.Sort = table.DefaultSort
		break
	}
	if request.Table == "" {
		return dashboard.DefaultTableRequest()
	}
	return request
}

func tableSignals(report semantic.Dashboard, request dashboard.TableRequest) map[string]any {
	tables := map[string]any{}
	for _, name := range sortedKeys(report.Tables) {
		table := report.Tables[name]
		tables[name] = map[string]any{
			"title":     table.Title,
			"columns":   table.Columns,
			"rows":      []any{},
			"totalRows": 0,
			"window": map[string]any{
				"offset": 0,
				"limit":  request.Limit,
			},
			"sort": map[string]any{
				"key":       table.DefaultSort.Key,
				"direction": table.DefaultSort.Direction,
			},
			"loading": false,
			"error":   "",
		}
	}
	return tables
}

func chartSignals(report semantic.Dashboard, model *semantic.Model) map[string]any {
	charts := map[string]any{}
	for _, id := range sortedKeys(report.Visuals) {
		visual := report.Visuals[id]
		measureName := ""
		unit := ""
		if len(visual.Query.Measures) > 0 {
			measureName = visual.Query.Measures[0]
			if dataset, ok := model.Datasets[visual.Dataset]; ok {
				unit = dataset.Measures[measureName].Unit
			}
		}
		charts[id] = chartSignal(id, visual.Type, visual.Title, unit, visual.Interaction.Field, visual.Query.Dimensions, measureName, visual.Query.Series, visual.Stacked)
	}
	return charts
}

func chartSignal(id, chartType, title, unit, field string, dimensions []string, measure, series string, stacked bool) map[string]any {
	seriesList := []string{}
	if series != "" {
		seriesList = append(seriesList, series)
	}
	signal := map[string]any{
		"version":    2,
		"id":         id,
		"type":       chartType,
		"title":      title,
		"unit":       unit,
		"field":      field,
		"dimensions": dimensions,
		"measure":    measure,
		"series":     seriesList,
		"selection":  []any{},
		"data":       []any{},
	}
	if stacked {
		signal["stacked"] = true
	}
	return signal
}

func iconAttrs() g.Node {
	return g.Attr("aria-hidden", "true")
}

func canvasVisual(x, y, width, height float64, children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("canvas-visual"),
		g.Attr("data-x", formatCanvasNumber(x)),
		g.Attr("data-y", formatCanvasNumber(y)),
		g.Attr("data-w", formatCanvasNumber(width)),
		g.Attr("data-h", formatCanvasNumber(height)),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func pageTabs(dashboardID string, pages []dashboard.Page, activeID string) g.Node {
	if len(pages) == 0 {
		return nil
	}
	return h.Nav(h.Class("page-tabs"), h.Aria("label", "Report pages"),
		g.Map(pages, func(page dashboard.Page) g.Node {
			return pageTab(dashboardID, page, activeID)
		}),
	)
}

func pageTab(dashboardID string, page dashboard.Page, activeID string) g.Node {
	class := "page-tab"
	if page.ID == activeID {
		class += " active"
	}
	href := "/dashboards/" + dashboardID + "/pages/" + page.ID
	return h.A(h.Class(class), h.Href(href), g.Text(page.Title))
}

func renderPageCanvas(page dashboard.Page) g.Node {
	page = page.WithDefaults()
	nodes := []g.Node{
		g.Attr("width", strconv.Itoa(page.Canvas.Width)),
		g.Attr("height", strconv.Itoa(page.Canvas.Height)),
	}
	for _, visual := range page.PlacedVisuals() {
		nodes = append(nodes, renderPageVisual(visual))
	}
	return g.El("ld-report-canvas", nodes...)
}

func formatCanvasNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func renderPageVisual(visual dashboard.PageVisual) g.Node {
	switch visual.Kind {
	case "header":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height, reportHeader(visual))
	case "kpi_strip":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height,
			h.Div(h.Class("kpi-band"),
				g.El("ld-kpi-strip", g.Attr("data-attr:items", "$kpis")),
			),
		)
	case "line_chart", "area_chart", "bar_chart", "column_chart", "pie_chart", "donut_chart", "scatter_chart", "funnel_chart", "treemap_chart", "gauge_chart":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height,
			chartPanel(visual.Visual),
		)
	case "table":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height, tablePanel(visual.Table))
	default:
		return nil
	}
}

func reportHeader(visual dashboard.PageVisual) g.Node {
	if visual.Eyebrow == "" {
		visual.Eyebrow = "LibreDash report"
	}
	if visual.Title == "" {
		visual.Title = "Dashboard"
	}
	return h.Header(h.Class("report-header"),
		h.Div(
			h.P(h.Class("report-eyebrow"), g.Text(visual.Eyebrow)),
			h.H1(h.Class("report-title"), g.Text(visual.Title)),
		),
		h.Div(h.Class("report-summary"),
			g.Map(visual.Badges, func(badge string) g.Node {
				return h.Span(g.Text(badge))
			}),
		),
	)
}

func filtersDock(report semantic.Dashboard, action string) g.Node {
	return h.Details(h.Class("filters-dock"), h.Aria("label", "Report filters"),
		h.Summary(h.Class("filters-dock-rail"), h.Title("Toggle filters"),
			lucide.SlidersHorizontal(iconAttrs()),
			h.Span(h.Class("filters-rail-label"), g.Text("Filters")),
			h.Span(h.Class("sr-only"), g.Text("Toggle filters")),
		),
		filtersPane(report, action),
	)
}

func filtersPane(report semantic.Dashboard, action string) g.Node {
	return h.Div(h.Class("filters-pane"),
		g.El("ld-filter-panel",
			g.Attr("config", jsonString(report.Filters)),
			g.Attr("data-attr:filters", "$filters"),
			g.Attr("data-attr:options", "$filterOptions"),
			g.Attr("data-attr:loading", "$status.loading"),
			g.Attr("data-on:ld-filters-change", "$filters = evt.detail.filters; $urlParams = evt.detail.urlParams; window.DatastarURLSync && window.DatastarURLSync.replace($urlParams); $tableCommand.offset = 0; "+action),
			g.Attr("data-on:ld-filters-reset", "$filters = evt.detail.filters; $urlParams = evt.detail.urlParams; window.DatastarURLSync && window.DatastarURLSync.replace($urlParams); $tableCommand.offset = 0; @post('/commands/reset-filters')"),
			g.Attr("data-on:ld-filters-refresh", action),
			g.Attr("data-on:ld-visual-selection-clear", "$filters.visualSelections = []; @post('/commands/clear-selection')"),
		),
	)
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func chartPanel(visualID string) g.Node {
	signal := "charts." + visualID
	return h.Article(h.Class("visual-card"),
		g.El("ld-echart",
			g.Attr("visual-id", visualID),
			g.Attr("data-attr:chart", "$"+signal),
			g.Attr("data-on:ld-chart-select", "$chartCommand = evt.detail; @post('/commands/chart-select')"),
		),
	)
}

func tablePanel(tableName string) g.Node {
	if tableName == "" {
		tableName = "orders"
	}
	return h.Section(h.Class("table-card"),
		g.El("ld-data-table",
			g.Attr("data-attr:table", "$tables."+tableName),
			g.Attr("data-on:ld-table-window-change", "$tableCommand = evt.detail; @post('/commands/table-window')"),
		),
	)
}
