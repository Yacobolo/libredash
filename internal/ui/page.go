package ui

import (
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
			h.Script(h.Type("module"), h.Src("/static/charts.js")),
			h.Script(h.Type("module"), h.Src("/static/table.js")),
			h.Script(h.Type("module"), h.Src("/static/datastar-inspector.js")),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		},
		Body: []g.Node{
			h.Main(
				h.ID("dashboard"),
				ds.Signals(initialSignals(dataDir, clientID)),
				ds.Init(updateAction),
				h.Section(h.Class("mx-auto w-[min(1460px,calc(100vw-32px))] px-0 py-7 max-sm:w-[min(100vw-20px,560px)] max-sm:py-3"),
					header(),
					filters(),
					statusBar(),
					h.Div(h.Class("mb-3.5"),
						g.El("ld-kpi-strip", g.Attr("data-attr:items", "$kpis")),
					),
					h.Section(h.Class("grid grid-cols-4 gap-3.5 max-lg:grid-cols-2 max-sm:grid-cols-1"), h.Aria("label", "Olist dashboard charts"),
						chartPanel(true, "ld-line-chart", "charts.revenue"),
						chartPanel(false, "ld-bar-chart", "charts.orders"),
						chartPanel(false, "ld-bar-chart", "charts.delivery"),
						chartPanel(true, "ld-bar-chart", "charts.categories"),
					),
					tablePanel(),
					debugPanel(),
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
			"dateRange": "all",
			"state":     "all",
			"category":  "",
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
			"revenue":    map[string]any{"title": "Revenue by month", "unit": "R$", "data": []any{}},
			"orders":     map[string]any{"title": "Orders by status", "unit": "orders", "data": []any{}},
			"categories": map[string]any{"title": "Top product categories", "unit": "R$", "data": []any{}},
			"delivery":   map[string]any{"title": "Delivery speed", "unit": "orders", "data": []any{}},
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

func header() g.Node {
	return h.Header(h.Class("flex items-end justify-between gap-6 border-b-2 border-[var(--borderColor-emphasis)] py-5 max-sm:flex-col max-sm:items-stretch"),
		h.Div(
			h.P(h.Class("mb-1 text-xs font-extrabold uppercase text-[var(--fgColor-muted)]"), g.Text("DuckDB semantic cockpit")),
			h.H1(h.Class("m-0 text-6xl font-black leading-none tracking-normal max-sm:text-5xl lg:text-8xl"), g.Text("LibreDash")),
		),
		h.Div(h.Class("inline-flex min-h-10 items-center gap-2.5 whitespace-nowrap border border-[var(--borderColor-emphasis)] bg-[var(--bgColor-default)] px-3 py-2 text-sm font-extrabold text-[var(--fgColor-default)] shadow-[var(--shadow-resting-small)]"),
			h.Span(h.Class("pulse"), g.Attr("data-class", "{'is-active': $status.loading}")),
			h.Span(ds.Text("$status.loading ? 'Refreshing' : ($status.lastUpdated ? `Updated ${$status.lastUpdated}` : 'Ready')")),
		),
	)
}

func filters() g.Node {
	return h.Form(
		h.ID("filters"),
		h.Class("my-4 grid grid-cols-[minmax(130px,0.8fr)_minmax(120px,0.65fr)_minmax(220px,1.8fr)_auto] items-end gap-2.5 max-lg:grid-cols-2 max-sm:grid-cols-1"),
		ds.On("change", updateAction, ds.ModifierDebounce, ds.Duration(150000000)),
		ds.On("input", updateAction, ds.ModifierDebounce, ds.Duration(450000000)),
		h.Label(h.Class("grid gap-1.5"),
			filterLabel("Period"),
			h.Select(controlClass(), ds.Bind("filters.dateRange"),
				option("all", "All orders"),
				option("recent", "Latest 90 days"),
				option("2018", "2018"),
				option("2017", "2017"),
			),
		),
		h.Label(h.Class("grid gap-1.5"),
			filterLabel("State"),
			h.Select(controlClass(), ds.Bind("filters.state"),
				option("all", "Brazil"),
				option("SP", "SP"), option("RJ", "RJ"), option("MG", "MG"), option("RS", "RS"),
				option("PR", "PR"), option("SC", "SC"), option("BA", "BA"), option("DF", "DF"),
				option("GO", "GO"), option("ES", "ES"), option("PE", "PE"), option("CE", "CE"),
			),
		),
		h.Label(h.Class("grid gap-1.5"),
			filterLabel("Category contains"),
			h.Input(
				controlClass(),
				h.Type("search"),
				h.Placeholder("health, watches, furniture..."),
				ds.Bind("filters.category"),
			),
		),
		h.Button(
			h.Type("button"),
			h.Class("min-h-11 w-full cursor-pointer border border-[var(--button-primary-bgColor-rest)] bg-[var(--button-primary-bgColor-rest)] px-3 font-black text-[var(--button-primary-fgColor-rest)] shadow-[var(--shadow-resting-small)] outline-offset-2 focus:outline-3 focus:outline-[var(--borderColor-accent-emphasis)] disabled:cursor-wait disabled:opacity-70"),
			ds.On("click", updateAction),
			ds.Attr("disabled", "$status.loading"),
			g.Text("Refresh"),
		),
	)
}

func filterLabel(label string) g.Node {
	return h.Span(h.Class("text-xs font-black uppercase text-[var(--fgColor-muted)]"), g.Text(label))
}

func controlClass() g.Node {
	return h.Class("min-h-11 w-full border border-[var(--borderColor-emphasis)] bg-[var(--control-bgColor-rest)] px-3 text-[var(--fgColor-default)] outline-offset-2 focus:outline-3 focus:outline-[var(--borderColor-accent-emphasis)]")
}

func option(value, label string) g.Node {
	return h.Option(h.Value(value), g.Text(label))
}

func statusBar() g.Node {
	return h.Section(h.Class("status-band mb-3.5 flex min-h-14 items-center justify-between gap-4 border border-[var(--borderColor-default)] bg-[var(--bgColor-success-muted)] px-3.5 py-3 max-sm:flex-col max-sm:items-stretch"),
		g.Attr("data-class", "{'has-error': $status.error}"),
		h.Div(
			h.Strong(h.Class("mb-0.5 block text-[var(--fgColor-default)]"), ds.Text("$status.error ? 'Setup needed' : 'Signal stream'")),
			h.P(h.Class("m-0 text-[var(--fgColor-muted)]"), ds.Text("$status.error || `Reading ${$status.dataDirectory}`")),
		),
		h.Code(h.Class("border border-[var(--borderColor-default)] bg-[var(--bgColor-default)] px-2.5 py-2 font-sans text-sm text-[var(--fgColor-default)]"), ds.Text("$status.setupRequired ? 'python3 scripts/bootstrap_olist.py' : '/updates'")),
	)
}

func chartPanel(wide bool, tag, signal string) g.Node {
	class := "min-h-[310px] border border-[var(--borderColor-emphasis)] bg-[var(--bgColor-default)] shadow-[var(--shadow-resting-medium)]"
	if wide {
		class += " col-span-2 max-sm:col-span-1"
	}
	return h.Article(h.Class(class),
		g.El(tag,
			g.Attr("data-attr:data", "$"+signal+".data"),
			g.Attr("data-attr:chart-title", "$"+signal+".title"),
			g.Attr("data-attr:unit", "$"+signal+".unit"),
		),
	)
}

func tablePanel() g.Node {
	return h.Section(h.Class("mt-4 border border-[var(--borderColor-emphasis)] bg-[var(--bgColor-default)] shadow-[var(--shadow-resting-medium)]"),
		g.El("ld-data-table",
			g.Attr("data-attr:table", "$tables.orders"),
			g.Attr("data-on:ld-table-window-change", "$tableCommand = evt.detail; @post('/commands/table-window')"),
		),
	)
}

func debugPanel() g.Node {
	return h.Details(h.Class("mt-4 border-t border-[var(--borderColor-default)] text-[var(--fgColor-muted)]"),
		h.Summary(h.Class("cursor-pointer py-3 font-extrabold"), g.Text("Signals")),
		h.Pre(h.Class("max-h-72 overflow-auto bg-[var(--bgColor-emphasis)] p-3 font-sans text-[var(--fgColor-onEmphasis)]"), ds.JSONSignals(ds.Filter{}, ds.ModifierTerse)),
	)
}
