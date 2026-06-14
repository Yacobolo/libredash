package ui

import (
	"encoding/json"
	"strconv"

	"github.com/Yacobolo/libredash/internal/dashboard"
	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

const updateAction = "@get('/updates', {openWhenHidden: true})"

func Page(dataDir, clientID string, pages []dashboard.Page, activePage dashboard.Page) g.Node {
	if activePage.ID == "" {
		activePage = defaultPage()
	}
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
			h.Script(h.Type("module"), h.Src("/static/report-canvas.js")),
			h.Script(h.Type("module"), h.Src("/static/charts.js")),
			h.Script(h.Type("module"), h.Src("/static/table.js")),
			h.Script(h.Type("module"), h.Src("/static/datastar-inspector.js")),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		},
		Body: []g.Node{
			h.Main(
				h.ID("dashboard"),
				h.Class("report-app"),
				ds.Signals(initialSignals(dataDir, clientID)),
				ds.Init(updateAction),
				appBar("report", true),
				h.Div(h.Class("report-workspace"),
					navRail("report"),
					h.Section(h.Class("report-canvas-shell"), h.Aria("label", "LibreDash report canvas"),
						pageTabs(pages, activePage.ID),
						renderPageCanvas(activePage),
					),
				),
				g.El("datastar-inspector"),
			),
		},
	})
}

func ModelPage(model dashboard.ModelGraph) g.Node {
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
			h.Script(h.Type("module"), h.Src("/static/model-graph.js")),
		},
		Body: []g.Node{
			h.Main(
				h.ID("model"),
				h.Class("report-app"),
				appBar("model", false),
				h.Div(h.Class("report-workspace"),
					navRail("model"),
					h.Section(h.Class("model-shell"), h.Aria("label", "LibreDash semantic model"),
						h.Header(h.Class("model-header"),
							h.Div(
								h.P(h.Class("report-eyebrow"), g.Text("Semantic model")),
								h.H1(h.Class("report-title"), g.Text(model.Title)),
							),
							modelStats(model.Stats),
						),
						g.El("ld-model-graph", g.Attr("data-model", modelGraphJSON(model))),
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

func initialSignals(dataDir, clientID string) map[string]any {
	return map[string]any{
		"runtime": map[string]any{
			"clientId": clientID,
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
			"table":  "orders",
			"offset": 0,
			"limit":  120,
			"sort": map[string]any{
				"key":       "purchase_date",
				"direction": "desc",
			},
		},
		"tables": map[string]any{
			"orders": map[string]any{
				"title": "Orders",
				"columns": []map[string]any{
					{"key": "order_id", "label": "Order"},
					{"key": "purchase_date", "label": "Purchased"},
					{"key": "status", "label": "Status"},
					{"key": "state", "label": "State"},
					{"key": "category", "label": "Category"},
					{"key": "revenue", "label": "Revenue", "align": "right"},
					{"key": "review_score", "label": "Review", "align": "right"},
					{"key": "delivery_days", "label": "Delivery", "align": "right"},
				},
				"rows":      []any{},
				"totalRows": 0,
				"window": map[string]any{
					"offset": 0,
					"limit":  120,
				},
				"sort": map[string]any{
					"key":       "purchase_date",
					"direction": "desc",
				},
				"loading": false,
				"error":   "",
			},
		},
		"charts": map[string]any{
			"revenue":    map[string]any{"title": "Revenue by month", "unit": "R$", "field": "purchase_month", "selection": []any{}, "data": []any{}},
			"orders":     map[string]any{"title": "Orders by status", "unit": "orders", "field": "status", "selection": []any{}, "data": []any{}},
			"categories": map[string]any{"title": "Top product categories", "unit": "R$", "field": "category", "selection": []any{}, "data": []any{}},
			"delivery":   map[string]any{"title": "Delivery speed", "unit": "orders", "field": "delivery_bucket", "selection": []any{}, "data": []any{}},
		},
		"kpis": []any{},
		"status": map[string]any{
			"loading":       false,
			"error":         "",
			"lastUpdated":   "",
			"dataDirectory": dataDir,
			"setupRequired": false,
		},
	}
}

func appBar(active string, dataActions bool) g.Node {
	return h.Header(h.Class("app-bar"),
		h.Div(h.Class("app-brand"),
			h.Span(h.Class("brand-mark"), lucide.ChartColumnIncreasing(iconAttrs())),
			h.Span(g.Text("LibreDash")),
		),
		h.Nav(h.Class("command-bar"), h.Aria("label", "Report commands"),
			commandLink("/", "Report", active == "report", lucide.LayoutDashboard(iconAttrs())),
			commandLink("/", "Analyze", false, lucide.ChartColumnIncreasing(iconAttrs())),
			commandLink("/model", "Model", active == "model", lucide.Database(iconAttrs())),
		),
		h.Div(h.Class("app-actions"),
			g.If(dataActions,
				h.Button(
					h.Type("button"),
					h.Class("cache-refresh-button"),
					ds.On("click", "@post('/commands/refresh-cache')"),
					ds.Attr("disabled", "$status.loading"),
					h.Title("Re-import DuckDB cache"),
					lucide.RefreshCw(iconAttrs()),
					h.Span(g.Text("Re-import")),
				),
			),
			g.If(dataActions,
				h.Div(h.Class("stream-chip"),
					h.Span(h.Class("pulse"), g.Attr("data-class", "{'is-active': $status.loading}")),
					h.Span(ds.Text("$status.loading ? 'Refreshing' : ($status.lastUpdated ? `Updated ${$status.lastUpdated}` : 'Live')")),
				),
			),
			h.Div(h.Class("theme-switch"), h.Aria("label", "Color mode"),
				h.Button(
					h.Type("button"),
					h.Class("theme-button"),
					g.Attr("data-theme-value", "light"),
					g.Attr("aria-pressed", "false"),
					h.Title("Light mode"),
					lucide.Sun(iconAttrs()),
					h.Span(h.Class("sr-only"), g.Text("Light mode")),
				),
				h.Button(
					h.Type("button"),
					h.Class("theme-button"),
					g.Attr("data-theme-value", "dark"),
					g.Attr("aria-pressed", "false"),
					h.Title("Dark mode"),
					lucide.Moon(iconAttrs()),
					h.Span(h.Class("sr-only"), g.Text("Dark mode")),
				),
			),
		),
	)
}

func commandLink(href, label string, active bool, icon g.Node) g.Node {
	class := "command-button"
	if active {
		class += " active"
	}
	return h.A(h.Class(class), h.Href(href), g.Attr("aria-current", ariaCurrent(active)),
		icon,
		h.Span(g.Text(label)),
	)
}

func ariaCurrent(active bool) string {
	if active {
		return "page"
	}
	return "false"
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

func navRail(active string) g.Node {
	return h.Aside(h.Class("nav-rail"), h.Aria("label", "Workspace navigation"),
		railItem("/", lucide.LayoutDashboard(iconAttrs()), "Report", active == "report"),
		railItem("/", lucide.Table2(iconAttrs()), "Data", false),
		railItem("/model", lucide.Database(iconAttrs()), "Model", active == "model"),
		railItem("", lucide.Activity(iconAttrs()), "Signals", false),
	)
}

func pageTabs(pages []dashboard.Page, activeID string) g.Node {
	if len(pages) == 0 {
		return nil
	}
	return h.Nav(h.Class("page-tabs"), h.Aria("label", "Report pages"),
		g.Map(pages, func(page dashboard.Page) g.Node {
			return pageTab(page, activeID)
		}),
	)
}

func pageTab(page dashboard.Page, activeID string) g.Node {
	class := "page-tab"
	if page.ID == activeID {
		class += " active"
	}
	href := "/pages/" + page.ID
	if page.ID == "overview" {
		href = "/"
	}
	return h.A(h.Class(class), h.Href(href), g.Text(page.Title))
}

func railItem(href string, icon g.Node, label string, active bool) g.Node {
	class := "rail-item"
	if active {
		class += " active"
	}
	if href == "" {
		return h.Button(h.Type("button"), h.Class(class), h.Title(label),
			icon,
			h.Span(h.Class("sr-only"), g.Text(label)),
		)
	}
	return h.A(h.Class(class), h.Href(href), h.Title(label), g.Attr("aria-current", ariaCurrent(active)),
		icon,
		h.Span(h.Class("sr-only"), g.Text(label)),
	)
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
	nodes = append(nodes, filtersPane())
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
	case "line_chart", "bar_chart":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height,
			chartPanel(chartTag(visual.Kind), visual.Visual),
		)
	case "table":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height, tablePanel(visual.Table))
	default:
		return nil
	}
}

func chartTag(kind string) string {
	if kind == "line_chart" {
		return "ld-line-chart"
	}
	return "ld-bar-chart"
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

func filtersPane() g.Node {
	return h.Aside(g.Attr("slot", "filters"), h.Class("filters-pane"), h.Aria("label", "Report filters"),
		h.Div(h.Class("pane-header"),
			h.H2(h.Class("pane-title"), g.Text("Filters")),
		),
		filters(),
	)
}

func filters() g.Node {
	return h.Form(
		h.ID("filters"),
		h.Class("filter-form"),
		ds.On("change", updateAction, ds.ModifierDebounce, ds.Duration(150000000)),
		ds.On("input", updateAction, ds.ModifierDebounce, ds.Duration(450000000)),
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
		h.Button(
			h.Type("button"),
			h.Class("refresh-button"),
			ds.On("click", updateAction),
			ds.Attr("disabled", "$status.loading"),
			lucide.RefreshCw(iconAttrs()),
			h.Span(g.Text("Refresh")),
		),
	)
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

func chartPanel(tag, visualID string) g.Node {
	signal := "charts." + visualID
	return h.Article(h.Class("visual-card"),
		g.El(tag,
			g.Attr("visual-id", visualID),
			g.Attr("data-attr:data", "$"+signal+".data"),
			g.Attr("data-attr:chart-title", "$"+signal+".title"),
			g.Attr("data-attr:unit", "$"+signal+".unit"),
			g.Attr("data-attr:field", "$"+signal+".field"),
			g.Attr("data-attr:selection", "$"+signal+".selection"),
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
