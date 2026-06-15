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

func Page(dataDir, clientID string, catalog dashboard.Catalog, report semantic.Dashboard, model *semantic.Model, pages []dashboard.Page, activePage dashboard.Page) g.Node {
	if activePage.ID == "" {
		activePage = defaultPage()
	}
	action := updateAction(report.ID, activePage.ID)
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
			h.Script(h.Type("module"), h.Src("/static/sidebar.js")),
			h.Script(h.Type("module"), h.Src("/static/filter-dock.js")),
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
				ds.Signals(initialSignals(dataDir, clientID, report, model, activePage)),
				ds.Init(action),
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
							filtersDock(action),
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
		Width:  1366,
		Height: 940,
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

func initialSignals(dataDir, clientID string, report semantic.Dashboard, model *semantic.Model, activePage dashboard.Page) map[string]any {
	tableRequest := defaultTableRequest(report)
	return map[string]any{
		"runtime": map[string]any{
			"clientId":    clientID,
			"dashboardId": report.ID,
			"pageId":      activePage.ID,
			"modelId":     model.Name,
		},
		"filters": map[string]any{
			"dateRange":        "all",
			"state":            "all",
			"category":         "",
			"visualSelections": []any{},
		},
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

func canvasVisual(x, y, width, height int, children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("canvas-visual"),
		g.Attr("data-x", strconv.Itoa(x)),
		g.Attr("data-y", strconv.Itoa(y)),
		g.Attr("data-w", strconv.Itoa(width)),
		g.Attr("data-h", strconv.Itoa(height)),
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
		g.Attr("width", strconv.Itoa(page.Width)),
		g.Attr("height", strconv.Itoa(page.Height)),
	}
	for _, visual := range page.Visuals {
		nodes = append(nodes, renderPageVisual(visual))
	}
	return g.El("ld-report-canvas", nodes...)
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

func filtersDock(action string) g.Node {
	return h.Details(h.Class("filters-dock"), h.Aria("label", "Report filters"),
		h.Summary(h.Class("filters-dock-rail"), h.Title("Toggle filters"),
			lucide.SlidersHorizontal(iconAttrs()),
			h.Span(h.Class("filters-rail-label"), g.Text("Filters")),
			h.Span(h.Class("sr-only"), g.Text("Toggle filters")),
		),
		filtersPane(action),
	)
}

func filtersPane(action string) g.Node {
	return h.Div(h.Class("filters-pane"),
		h.Div(h.Class("pane-header"),
			h.H2(h.Class("pane-title"), g.Text("Filters")),
			h.Span(h.Class("filter-count"), ds.Text("`${"+activeFilterCountExpr()+"} active`")),
		),
		filters(action),
	)
}

func filters(action string) g.Node {
	activeCount := activeFilterCountExpr()
	return h.Form(
		h.ID("filters"),
		h.Class("filter-form"),
		ds.On("change", action, ds.ModifierDebounce, ds.Duration(150000000)),
		ds.On("input", action, ds.ModifierDebounce, ds.Duration(450000000)),
		h.Label(h.Class("filter-control"),
			filterLabel("Period"),
			h.Select(controlClass(), ds.Bind("filters.dateRange"),
				option("all", "All orders"),
				option("recent", "Latest 90 days"),
				option("2018", "2018"),
				option("2017", "2017"),
			),
		),
		h.Label(h.Class("filter-control"),
			filterLabel("State"),
			h.Select(controlClass(), ds.Bind("filters.state"),
				option("all", "Brazil"),
				option("SP", "SP"), option("RJ", "RJ"), option("MG", "MG"), option("RS", "RS"),
				option("PR", "PR"), option("SC", "SC"), option("BA", "BA"), option("DF", "DF"),
				option("GO", "GO"), option("ES", "ES"), option("PE", "PE"), option("CE", "CE"),
			),
		),
		h.Label(h.Class("filter-control"),
			filterLabel("Category contains"),
			h.Input(
				controlClass(),
				h.Type("search"),
				h.Placeholder("health, watches, furniture..."),
				ds.Bind("filters.category"),
			),
		),
		h.Div(
			h.Class("selection-summary"),
			g.Attr("data-show", "$filters.visualSelections.length > 0"),
			h.Span(ds.Text("`${$filters.visualSelections.length} visual filter${$filters.visualSelections.length === 1 ? '' : 's'} active`")),
			h.Button(
				h.Type("button"),
				h.Class("clear-selection-button"),
				ds.On("click", "@post('/commands/clear-selection')"),
				ds.Attr("disabled", "$status.loading"),
				lucide.X(iconAttrs()),
				h.Span(g.Text("Clear")),
			),
		),
		h.Div(
			h.Class("filter-summary"),
			h.Span(ds.Text("`${"+activeCount+"} total filter${("+activeCount+") === 1 ? '' : 's'} applied`")),
			h.Button(
				h.Type("button"),
				h.Class("reset-filters-button"),
				ds.On("click", "$filters.dateRange = 'all'; $filters.state = 'all'; $filters.category = ''; $filters.visualSelections = []; $tableCommand.offset = 0; @post('/commands/reset-filters')"),
				ds.Attr("disabled", "$status.loading || ("+activeCount+") === 0"),
				lucide.RotateCcw(iconAttrs()),
				h.Span(g.Text("Reset")),
			),
		),
		h.Button(
			h.Type("button"),
			h.Class("refresh-button"),
			ds.On("click", action),
			ds.Attr("disabled", "$status.loading"),
			lucide.RefreshCw(iconAttrs()),
			h.Span(g.Text("Refresh")),
		),
	)
}

func activeFilterCountExpr() string {
	return "($filters.dateRange !== 'all' ? 1 : 0) + ($filters.state !== 'all' ? 1 : 0) + ((($filters.category || '').trim() !== '') ? 1 : 0) + ($filters.visualSelections ? $filters.visualSelections.length : 0)"
}

func filterLabel(label string) g.Node {
	return h.Span(h.Class("filter-label"), g.Text(label))
}

func controlClass() g.Node {
	return h.Class("filter-input")
}

func option(value, label string) g.Node {
	return h.Option(h.Value(value), g.Text(label))
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
