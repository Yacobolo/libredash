package ui

import (
	"strconv"

	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

const updateAction = "@get('/updates', {openWhenHidden: true})"

func Page(dataDir, clientID string) g.Node {
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
				appBar(),
				h.Div(h.Class("report-workspace"),
					navRail(),
					h.Section(h.Class("report-canvas-shell"), h.Aria("label", "LibreDash report canvas"),
						g.El("ld-report-canvas", g.Attr("width", "1366"), g.Attr("height", "940"),
							canvasVisual(16, 16, 1334, 86,
								reportHeader(),
							),
							canvasVisual(16, 118, 1334, 116,
								h.Div(h.Class("kpi-band"),
									g.El("ld-kpi-strip", g.Attr("data-attr:items", "$kpis")),
								),
							),
							canvasVisual(16, 250, 650, 300,
								chartPanel("ld-line-chart", "revenue", "charts.revenue"),
							),
							canvasVisual(682, 250, 326, 300,
								chartPanel("ld-bar-chart", "orders", "charts.orders"),
							),
							canvasVisual(1024, 250, 326, 300,
								chartPanel("ld-bar-chart", "delivery", "charts.delivery"),
							),
							canvasVisual(16, 566, 650, 354,
								chartPanel("ld-bar-chart", "categories", "charts.categories"),
							),
							canvasVisual(682, 566, 668, 354,
								tablePanel(),
							),
							filtersPane(),
						),
					),
				),
				g.El("datastar-inspector"),
			),
		},
	})
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

func appBar() g.Node {
	return h.Header(h.Class("app-bar"),
		h.Div(h.Class("app-brand"),
			h.Span(h.Class("brand-mark"), lucide.ChartColumnIncreasing(iconAttrs())),
			h.Span(g.Text("LibreDash")),
		),
		h.Nav(h.Class("command-bar"), h.Aria("label", "Report commands"),
			h.Button(h.Type("button"), h.Class("command-button active"), lucide.LayoutDashboard(iconAttrs()), h.Span(g.Text("Report"))),
			h.Button(h.Type("button"), h.Class("command-button"), lucide.ChartColumnIncreasing(iconAttrs()), h.Span(g.Text("Analyze"))),
			h.Button(h.Type("button"), h.Class("command-button"), lucide.Database(iconAttrs()), h.Span(g.Text("Model"))),
		),
		h.Div(h.Class("app-actions"),
			h.Button(
				h.Type("button"),
				h.Class("cache-refresh-button"),
				ds.On("click", "@post('/commands/refresh-cache')"),
				ds.Attr("disabled", "$status.loading"),
				h.Title("Re-import DuckDB cache"),
				lucide.RefreshCw(iconAttrs()),
				h.Span(g.Text("Re-import")),
			),
			h.Div(h.Class("stream-chip"),
				h.Span(h.Class("pulse"), g.Attr("data-class", "{'is-active': $status.loading}")),
				h.Span(ds.Text("$status.loading ? 'Refreshing' : ($status.lastUpdated ? `Updated ${$status.lastUpdated}` : 'Live')")),
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

func navRail() g.Node {
	return h.Aside(h.Class("nav-rail"), h.Aria("label", "Workspace navigation"),
		railItem(lucide.LayoutDashboard(iconAttrs()), "Report", true),
		railItem(lucide.Table2(iconAttrs()), "Data", false),
		railItem(lucide.Database(iconAttrs()), "Model", false),
		railItem(lucide.Activity(iconAttrs()), "Signals", false),
	)
}

func railItem(icon g.Node, label string, active bool) g.Node {
	class := "rail-item"
	if active {
		class += " active"
	}
	return h.Button(h.Type("button"), h.Class(class), h.Title(label),
		icon,
		h.Span(h.Class("sr-only"), g.Text(label)),
	)
}

func reportHeader() g.Node {
	return h.Header(h.Class("report-header"),
		h.Div(
			h.P(h.Class("report-eyebrow"), g.Text("Olist commerce overview")),
			h.H1(h.Class("report-title"), g.Text("Executive Sales Dashboard")),
		),
		h.Div(h.Class("report-summary"),
			h.Span(g.Text("DuckDB compute")),
			h.Span(g.Text("Datastar stream")),
			h.Span(g.Text("Lit visuals")),
		),
	)
}

func filtersPane() g.Node {
	return h.Aside(g.Attr("slot", "filters"), h.Class("filters-pane"), h.Aria("label", "Report filters"),
		h.Div(h.Class("pane-header"),
			h.P(h.Class("pane-eyebrow"), g.Text("Controls")),
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

func chartPanel(tag, visualID, signal string) g.Node {
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

func tablePanel() g.Node {
	return h.Section(h.Class("table-card"),
		g.El("ld-data-table",
			g.Attr("data-attr:table", "$tables.orders"),
			g.Attr("data-on:ld-table-window-change", "$tableCommand = evt.detail; @post('/commands/table-window')"),
		),
	)
}
