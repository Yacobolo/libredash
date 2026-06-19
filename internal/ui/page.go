package ui

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

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

func postAction(path string) string {
	return "@post('" + path + "', {headers: {'X-CSRF-Token': $csrfToken}})"
}

func staticAsset(path string) string {
	return path + "?v=dev"
}

const (
	appRootClass               = "min-h-svh bg-app text-fg-default"
	appShellClass              = "grid min-h-svh grid-cols-app-shell bg-app max-sm:grid-cols-1"
	reportShellClass           = "grid min-h-svh grid-cols-report-shell bg-app max-sm:grid-cols-1"
	appMainClass               = "grid min-w-0 min-h-svh grid-rows-app-main bg-app"
	catalogMainClass           = appMainClass + " gap-3 px-4 py-4 max-sm:min-h-0 max-sm:p-3"
	reportMainClass            = appMainClass + " h-svh min-h-0 overflow-hidden"
	metricMainClass            = "grid h-svh min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-app"
	modelMainClass             = appMainClass + " gap-3 px-4 py-4 max-sm:min-h-0 max-sm:p-3"
	cardClass                  = "grid min-h-min-card max-w-card grid-rows-card rounded-default border border-outline-variant bg-panel p-4 shadow-resting-sm"
	cardTitleClass             = "m-0 mt-1 text-body-md leading-snug font-semibold text-fg-default"
	cardDescriptionClass       = "m-0 mt-2 text-body-sm leading-relaxed font-normal text-fg-muted"
	cardFooterClass            = "mt-4 flex items-center justify-between gap-3 border-t border-outline-muted pt-3 text-caption font-medium text-fg-muted"
	eyebrowClass               = "m-0 mb-1 text-caption leading-tight font-medium uppercase text-fg-muted"
	visualCardClass            = "h-full min-h-0 w-full overflow-hidden rounded-default border border-outline-variant bg-panel"
	actionButtonClass          = "inline-flex size-action min-h-action items-center justify-center rounded-default border border-outline-variant bg-transparent p-0 text-fg-default hover:bg-control-hover focus-visible:bg-control-hover focus-visible:outline-0 disabled:cursor-not-allowed disabled:opacity-disabled"
	metricActionButtonClass    = "inline-flex size-8 items-center justify-center rounded-small border border-transparent bg-transparent p-0 text-fg-muted no-underline transition-colors duration-fast hover:border-outline-muted hover:bg-control-hover hover:text-fg-default focus-visible:border-outline-accent focus-visible:bg-control-hover focus-visible:text-fg-default focus-visible:outline-0"
	primaryLinkButtonClass     = "inline-flex min-h-control-xs items-center justify-center gap-1.5 rounded-small bg-button-primary px-2.5 text-caption font-semibold text-on-primary no-underline hover:bg-button-primary-hover focus-visible:bg-button-primary-hover focus-visible:outline-0"
	tagClass                   = "rounded-full border border-outline-muted bg-panel-muted px-2 py-0.5 text-caption font-medium uppercase text-fg-muted"
	metricContractSectionClass = "grid min-h-0 gap-3 overflow-hidden bg-transparent"
	metricWorkspaceClass       = "grid min-h-0 min-w-0 grid-cols-metric-workspace overflow-hidden bg-app data-[rail-collapsed]:grid-cols-metric-workspace-collapsed max-md:grid-cols-1"
	metricContentColumnClass   = "grid min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
	metricInfoSidebarClass     = "grid min-h-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden border-l border-outline-variant bg-app max-md:border-l-0 max-md:border-t"
)

func inspectorScript() g.Node {
	return h.Script(h.Type("module"), h.Src(staticAsset("/static/datastar-inspector.js")))
}

func inspectorElement() g.Node {
	return g.El("datastar-inspector")
}

func pageHead(extra ...g.Node) []g.Node {
	nodes := []g.Node{
		h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
		h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
		h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
		h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
	}
	return append(nodes, extra...)
}

func Page(dataDir, clientID, csrfToken string, catalog dashboard.Catalog, report semantic.Dashboard, model *semantic.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters) g.Node {
	if activePage.ID == "" {
		activePage = defaultPage()
	}
	action := updateAction(report.ID, activePage.ID)
	initAction := "window.DatastarURLSync && window.DatastarURLSync.bindPopstate($urlParamShape); " + action
	tableReset := tableResetExpression()
	initialFilters = report.NormalizeFiltersForPage(activePage.ID, initialFilters)
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/url-sync.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/report-sidebar.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/filter-dock.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/filter-panel.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/filter-card.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/report-canvas.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/report-footer.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/charts.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/table.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/visual-modal.js"))),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		),
		Body: []g.Node{
			h.Main(
				h.ID("dashboard"),
				h.Class(appRootClass),
				ds.Signals(initialSignals(dataDir, clientID, csrfToken, report, model, activePage, initialFilters)),
				ds.Init(initAction),
				g.Attr("data-on:datastar-url-params-sync__window", "$urlParams = evt.detail.params; $filters = window.LibreDashFilterURL.fromParams($filterConfig, $filters, $urlParams); "+tableReset+action),
				h.Div(h.Class(reportShellClass),
					sidebar(sidebarConfigForReport(catalog, report, model, activePage)),
					reportSidebar(reportSidebarConfig(report, model, pages, activePage)),
					h.Section(h.Class(reportMainClass), h.Aria("label", "LibreDash report canvas"),
						workspaceHeader(
							"",
							report.Title,
							reportPageHeaderDetail(pages, activePage),
							reportActions(model.Name, report.ID),
						),
						h.Div(h.Class("grid min-h-0 min-w-0 grid-cols-report-dashboard items-stretch overflow-hidden max-sm:grid-cols-1 max-sm:overflow-auto"),
							h.Div(h.Class("grid min-h-0 min-w-0 overflow-hidden bg-transparent px-6 py-5 max-sm:p-3"),
								renderPageCanvas(activePage, report, initialFilters, action),
							),
							filtersDock(report, activePage.ID, action),
						),
						g.El("ld-report-footer",
							h.Aria("label", "Report view controls"),
							g.Attr("data-attr:status", "$status"),
						),
					),
				),
				g.El("ld-visual-modal"),
				inspectorElement(),
			),
		},
	})
}

func LoginPage() g.Node {
	favicon := "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='10' fill='%230969da'/%3E%3Ctext x='32' y='39' text-anchor='middle' font-family='Arial,sans-serif' font-size='20' font-weight='700' fill='white'%3ELD%3C/text%3E%3C/svg%3E"
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Login",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: []g.Node{
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Rel("icon"), h.Href(favicon)),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
			loginBackgroundLoaderScript(),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		},
		Body: []g.Node{
			h.Main(h.Class("relative grid h-svh min-h-svh place-items-center overflow-hidden bg-app p-6 text-fg-default"), h.Aria("label", "LibreDash login"),
				g.El("ld-topology-background",
					h.Class("absolute inset-0 block bg-app"),
					g.Attr("data-login-background", ""),
					g.Attr("data-module-src", staticAsset("/static/topology-background.js")),
					ds.Init("document.dispatchEvent(new CustomEvent('libredash-login-background-init'))", ds.ModifierDelay, ds.Duration(900*time.Millisecond)),
				),
				h.Div(h.Class("pointer-events-none absolute inset-0 z-10 bg-app/80"), h.Aria("hidden", "true")),
				h.Button(h.Type("button"), h.Class("absolute right-4 top-4 z-20 inline-grid size-8 appearance-none place-items-center rounded-default border border-outline-variant bg-control text-fg-muted shadow-resting-sm hover:bg-control-hover hover:text-fg-default focus-visible:border-outline-accent focus-visible:bg-control-hover focus-visible:text-fg-default focus-visible:outline-0 max-sm:right-3 max-sm:top-3"), g.Attr("data-theme-toggle", ""),
					lucide.Monitor(loginThemeIconAttrs("system")...),
					lucide.Sun(loginThemeIconAttrs("light")...),
					lucide.Moon(loginThemeIconAttrs("dark")...),
				),
				h.Section(h.Class("relative z-20 grid w-full max-w-login-panel justify-items-center gap-5 rounded-default border border-outline-variant bg-panel p-6 text-center shadow-resting-md max-sm:px-5 max-sm:py-6"),
					h.Div(h.Class("grid justify-items-center"), h.Aria("hidden", "true"),
						h.H1(h.Class("m-0 text-title-md font-semibold leading-snug text-fg-default"), g.Text("LibreDash")),
					),
					h.Button(h.Type("button"), h.Class("inline-grid min-h-control-xl w-full appearance-none grid-cols-login-button items-center gap-3 rounded-default border border-outline-variant bg-control px-4 text-body-md font-medium text-fg-default shadow-resting-sm hover:border-outline-accent hover:bg-control-hover focus-visible:border-outline-accent focus-visible:bg-control-hover focus-visible:outline-0"),
						h.Span(h.Class("grid size-5 grid-cols-2 grid-rows-2 gap-px"), h.Aria("hidden", "true"),
							h.Span(h.Class("block bg-danger")),
							h.Span(h.Class("block bg-success")),
							h.Span(h.Class("block bg-accent")),
							h.Span(h.Class("block bg-warning")),
						),
						h.Span(g.Text("Sign in with Azure Active Directory")),
					),
				),
				h.Div(h.Class("absolute"), inspectorElement()),
			),
		},
	})
}

func CatalogPage(catalog dashboard.Catalog) g.Node {
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Dashboards",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			inspectorScript(),
		),
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				h.Div(h.Class(appShellClass),
					sidebar(sidebarConfigForCatalog(catalog)),
					h.Section(h.Class(catalogMainClass), h.Aria("label", "LibreDash dashboard catalog"),
						workspaceHeader(
							"",
							"Dashboards",
							"Reports backed by semantic models.",
							nil,
						),
						h.Div(h.Class("grid grid-cols-catalog-grid items-start justify-start gap-4"),
							g.Map(catalog.Dashboards, dashboardCard),
						),
					),
				),
				inspectorElement(),
			),
		},
	})
}

func ModelsPage(catalog dashboard.Catalog) g.Node {
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Semantic Models",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			inspectorScript(),
		),
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				h.Div(h.Class(appShellClass),
					sidebar(sidebarConfigForModels(catalog)),
					h.Section(h.Class(catalogMainClass), h.Aria("label", "LibreDash semantic model catalog"),
						workspaceHeader(
							"",
							"Semantic Models",
							"Reusable model definitions.",
							nil,
						),
						h.Div(h.Class("grid grid-cols-catalog-grid items-start justify-start gap-4"),
							g.Map(catalog.Models, modelCard),
						),
					),
				),
				inspectorElement(),
			),
		},
	})
}

func MetricViewsPage(catalog dashboard.Catalog, views []dashboard.MetricViewSummary) g.Node {
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Metric Views",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			inspectorScript(),
		),
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				h.Div(h.Class(appShellClass),
					sidebar(sidebarConfigForMetrics(catalog)),
					h.Section(h.Class(catalogMainClass), h.Aria("label", "LibreDash metric view catalog"),
						workspaceHeader(
							"",
							"Metric Views",
							"Business-facing analytics contracts.",
							nil,
						),
						h.Div(h.Class("grid grid-cols-catalog-grid items-start justify-start gap-4"),
							g.Map(views, metricViewCard),
						),
					),
				),
				inspectorElement(),
			),
		},
	})
}

func dashboardCard(report dashboard.CatalogDashboard) g.Node {
	eyebrow := strings.Join(report.MetricViewTitles, ", ")
	if eyebrow == "" {
		eyebrow = "Dashboard"
	}
	return h.Article(h.Class(cardClass),
		h.Div(h.Class("min-w-0"),
			h.P(h.Class(eyebrowClass), g.Text(eyebrow)),
			h.H2(h.Class(cardTitleClass), g.Text(report.Title)),
			h.P(h.Class(cardDescriptionClass), g.Text(report.Description)),
		),
		h.Div(h.Class("mt-4 flex flex-wrap gap-2"),
			g.Map(report.Tags, func(tag string) g.Node {
				return h.Span(h.Class(tagClass), g.Text(tag))
			}),
		),
		h.Footer(h.Class(cardFooterClass),
			h.Span(g.Textf("%d pages, %d views", report.PageCount, len(report.MetricViews))),
			h.A(h.Class(primaryLinkButtonClass), h.Href("/dashboards/"+report.ID),
				lucide.ExternalLink(buttonIconAttrs()...),
				h.Span(g.Text("Open")),
			),
		),
	)
}

func metricViewCard(view dashboard.MetricViewSummary) g.Node {
	return h.Article(h.Class(cardClass),
		h.Div(h.Class("min-w-0"),
			h.P(h.Class(eyebrowClass), g.Text(view.ModelTitle)),
			h.H2(h.Class(cardTitleClass), g.Text(view.Title)),
			h.P(h.Class(cardDescriptionClass), g.Text(view.Description)),
		),
		h.Div(h.Class("mt-4 flex flex-wrap gap-2"),
			h.Span(h.Class(tagClass), g.Text(view.BaseTable)),
			h.Span(h.Class(tagClass), g.Text(view.Timeseries)),
		),
		h.Footer(h.Class(cardFooterClass),
			h.Span(g.Textf("%d dimensions, %d measures", view.DimensionCount, view.MeasureCount)),
			h.A(h.Class(primaryLinkButtonClass), h.Href("/metrics/"+view.ID+"/measures"),
				lucide.ExternalLink(buttonIconAttrs()...),
				h.Span(g.Text("Open")),
			),
		),
	)
}

func modelCard(model dashboard.CatalogModel) g.Node {
	return h.Article(h.Class(cardClass),
		h.Div(h.Class("min-w-0"),
			h.P(h.Class(eyebrowClass), g.Text("Semantic model")),
			h.H2(h.Class(cardTitleClass), g.Text(model.Title)),
			h.P(h.Class(cardDescriptionClass), g.Text(model.Description)),
		),
		h.Footer(h.Class(cardFooterClass),
			h.Span(g.Text("Reusable model")),
			h.A(h.Class(primaryLinkButtonClass), h.Href("/models/"+model.ID),
				lucide.ExternalLink(buttonIconAttrs()...),
				h.Span(g.Text("Open")),
			),
		),
	)
}

func metricViewActions(view dashboard.MetricViewDetail) g.Node {
	return h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"),
		h.A(h.Class(metricActionButtonClass), h.Href("/models/"+view.SemanticModel), h.Title("Open semantic model"), h.Aria("label", "Open semantic model"),
			lucide.Network(metricActionIconAttrs()...),
		),
	)
}

func metricViewHeader(view dashboard.MetricViewDetail) g.Node {
	return h.Header(h.Class("grid min-w-0 grid-cols-workspace-header items-center gap-4 border-b border-outline-muted px-5 py-4"),
		h.Div(h.Class("min-w-0"),
			h.H1(h.Class("m-0 truncate text-title-sm font-semibold leading-snug text-fg-default"), g.Text(view.Title)),
			g.If(strings.TrimSpace(view.Description) != "", h.P(h.Class("m-0 mt-1 truncate text-body-sm font-normal leading-snug text-fg-muted"), g.Text(view.Description))),
		),
		metricViewActions(view),
	)
}

func metricViewInfoSidebar(view dashboard.MetricViewDetail) g.Node {
	return h.Aside(h.Class(metricInfoSidebarClass), h.Aria("label", "Metric view details"), g.Attr("data-metric-info-sidebar", ""),
		h.Div(h.Class("flex min-h-control-xl items-center justify-between gap-2 border-b border-outline-muted px-4 py-2"), g.Attr("data-metric-info-header", ""),
			h.H2(h.Class("m-0 flex min-w-0 items-center gap-2 truncate text-body-sm font-semibold text-fg-default"), lucide.FileText(metricInfoIconAttrs()...), h.Span(g.Text("Details"))),
		),
		h.Div(h.Class("grid content-start overflow-auto"), g.Attr("data-metric-info-body", ""),
			h.Div(h.Class("grid content-start"),
				metricInfoItem("Model context", h.A(h.Class("inline-flex min-w-0 items-center gap-2 text-fg-accent no-underline hover:underline"), h.Href("/models/"+view.SemanticModel), lucide.Box(metricInfoIconAttrs()...), h.Span(h.Class("truncate"), g.Text(view.ModelTitle)))),
				metricInfoItem("Base table", h.Span(h.Class("inline-flex min-w-0 items-center gap-2"), lucide.Table2(metricInfoIconAttrs()...), h.Code(h.Class("truncate font-mono"), g.Text(view.BaseTable)))),
				metricInfoItem("Primary timeseries", h.Span(h.Class("inline-flex min-w-0 items-center gap-2"), lucide.Calendar(metricInfoIconAttrs()...), h.Code(h.Class("truncate font-mono"), g.Text(view.Timeseries)))),
			),
		),
	)
}

func metricInfoItem(label string, value g.Node) g.Node {
	return h.Div(h.Class("grid content-start gap-2 border-b border-outline-muted px-4 py-4 text-body-sm last:border-b-0"),
		h.Span(h.Class("text-caption font-medium uppercase leading-none text-fg-muted"), g.Text(label)),
		h.Div(h.Class("min-w-0 text-body-sm font-medium text-fg-default"), value),
	)
}

func metricTabCount(count int) g.Node {
	return h.Span(h.Class("inline-flex min-w-4 items-center justify-center rounded-full bg-panel-muted px-1.5 py-px text-caption font-medium leading-none text-fg-muted"), g.Text(strconv.Itoa(count)))
}

func pluralize(label string, count int) string {
	if count == 1 {
		return label
	}
	return label + "s"
}

func ValidMetricViewSection(section string) bool {
	switch section {
	case "measures", "dimensions", "usage":
		return true
	default:
		return false
	}
}

func normalizeMetricViewSection(section string) string {
	if ValidMetricViewSection(section) {
		return section
	}
	return "measures"
}

type metricGrid struct {
	Columns  []metricGridColumn `json:"columns"`
	Rows     []map[string]any   `json:"rows"`
	Empty    string             `json:"empty"`
	MinWidth string             `json:"minWidth,omitempty"`
}

type metricGridColumn struct {
	ID      string `json:"id"`
	Header  string `json:"header"`
	Kind    string `json:"kind,omitempty"`
	Align   string `json:"align,omitempty"`
	HrefKey string `json:"hrefKey,omitempty"`
	Width   string `json:"width,omitempty"`
}

type metricGridBadge struct {
	Label string `json:"label"`
	Tone  string `json:"tone,omitempty"`
}

func metricViewDimensions() g.Node {
	return h.Section(h.ID("dimensions"), h.Class(metricContractSectionClass+" metric-contract-section-dimensions"),
		g.El("ld-data-grid", g.Attr("data-attr:grid", "$metricGrid")),
	)
}

func metricViewMeasures() g.Node {
	return h.Section(h.ID("measures"), h.Class(metricContractSectionClass+" metric-contract-section-measures"),
		g.El("ld-data-grid", g.Attr("data-attr:grid", "$metricGrid")),
	)
}

func metricViewDashboards() g.Node {
	return h.Section(h.ID("usage"), h.Class(metricContractSectionClass+" metric-contract-section-usage"),
		g.El("ld-metric-usage-graph", h.Class("block h-metric-usage min-h-0 rounded-default border border-outline-muted bg-panel"), g.Attr("data-attr:graph", "$metricUsageGraph")),
		g.El("ld-data-grid", g.Attr("data-attr:grid", "$metricGrid")),
	)
}

func metricViewGrid(view dashboard.MetricViewDetail, activeSection string) metricGrid {
	switch activeSection {
	case "dimensions":
		rows := make([]map[string]any, 0, len(view.Dimensions))
		for _, dimension := range view.Dimensions {
			rows = append(rows, map[string]any{
				"name":       dimension.Name,
				"label":      displayLabel(dimension.Label, dimension.Name),
				"expression": dimension.Expr,
				"filter":     emptyDash(dimension.Where),
				"order":      emptyDash(dimension.OrderExpr),
			})
		}
		return metricGrid{
			Columns: []metricGridColumn{
				{ID: "name", Header: "Name", Kind: "code", Width: "170px"},
				{ID: "label", Header: "Label", Width: "180px"},
				{ID: "expression", Header: "Expression", Kind: "expression", Width: "260px"},
				{ID: "filter", Header: "Filter", Kind: "expression", Width: "220px"},
				{ID: "order", Header: "Order", Kind: "expression", Width: "190px"},
			},
			Rows:     rows,
			Empty:    "No dimensions are defined for this metric view.",
			MinWidth: "1020px",
		}
	case "usage":
		rows := make([]map[string]any, 0, len(view.Dashboards))
		for _, report := range view.Dashboards {
			rows = append(rows, map[string]any{
				"dashboard":     report.Title,
				"dashboardHref": "/dashboards/" + report.ID,
				"description":   emptyDash(report.Description),
				"tags":          report.Tags,
				"pages":         report.PageCount,
			})
		}
		return metricGrid{
			Columns: []metricGridColumn{
				{ID: "dashboard", Header: "Dashboard", Kind: "link", HrefKey: "dashboardHref", Width: "250px"},
				{ID: "description", Header: "Description", Width: "420px"},
				{ID: "tags", Header: "Tags", Kind: "tags", Width: "220px"},
				{ID: "pages", Header: "Pages", Kind: "number", Align: "right", Width: "90px"},
			},
			Rows:     rows,
			Empty:    "No dashboards reference this metric view yet.",
			MinWidth: "980px",
		}
	default:
		rows := make([]map[string]any, 0, len(view.Measures))
		for _, measure := range view.Measures {
			rows = append(rows, map[string]any{
				"name":       measure.Name,
				"label":      displayLabel(measure.Label, measure.Name),
				"expression": measure.Expression,
				"unit":       metricGridBadgeValue(measure.Unit, "success"),
				"format":     metricGridBadgeValue(measure.Format, "accent"),
			})
		}
		return metricGrid{
			Columns: []metricGridColumn{
				{ID: "name", Header: "Name", Kind: "code", Width: "150px"},
				{ID: "label", Header: "Label", Width: "160px"},
				{ID: "expression", Header: "Expression", Kind: "expression"},
				{ID: "unit", Header: "Unit", Kind: "badge", Width: "82px"},
				{ID: "format", Header: "Format", Kind: "badge", Width: "88px"},
			},
			Rows:     rows,
			Empty:    "No measures are defined for this metric view.",
			MinWidth: "100%",
		}
	}
}

func metricGridBadgeValue(value, tone string) any {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return metricGridBadge{Label: value, Tone: tone}
}

func metricUsageGraph(view dashboard.MetricViewDetail) any {
	type graphNode struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Kind  string `json:"kind"`
		Meta  string `json:"meta,omitempty"`
	}
	type graphEdge struct {
		ID     string `json:"id"`
		Source string `json:"source"`
		Target string `json:"target"`
		Label  string `json:"label,omitempty"`
		Kind   string `json:"kind"`
	}
	type graph struct {
		Nodes []graphNode `json:"nodes"`
		Edges []graphEdge `json:"edges"`
	}
	nodes := []graphNode{
		{ID: "model", Label: view.ModelTitle, Kind: "model", Meta: view.SemanticModel},
		{ID: "model_table", Label: view.BaseTable, Kind: "model_table", Meta: "model table"},
		{ID: "metric_view", Label: view.Title, Kind: "metric_view", Meta: "metric contract"},
	}
	edges := []graphEdge{
		{ID: "model_table_edge", Source: "model", Target: "model_table", Label: "defines", Kind: "semantic"},
		{ID: "metric_view_edge", Source: "model_table", Target: "metric_view", Label: "exposes", Kind: "semantic"},
	}
	for _, report := range view.Dashboards {
		nodeID := "dashboard:" + report.ID
		nodes = append(nodes, graphNode{
			ID:    nodeID,
			Label: report.Title,
			Kind:  "dashboard",
			Meta:  strconv.Itoa(report.PageCount) + " " + pluralize("page", report.PageCount),
		})
		edges = append(edges, graphEdge{
			ID:     "metric_view_" + report.ID,
			Source: "metric_view",
			Target: nodeID,
			Label:  "used by",
			Kind:   "usage",
		})
	}
	return graph{Nodes: nodes, Edges: edges}
}

func metricTabs(view dashboard.MetricViewDetail, activeSection string) g.Node {
	return h.Nav(h.Class("flex min-w-0 gap-6 overflow-x-auto border-b border-outline-variant bg-app px-3"), h.Aria("label", "Metric view sections"),
		metricTabLink(view.ID, "measures", activeSection, "Measures", metricTabCount(view.MeasureCount)),
		metricTabLink(view.ID, "dimensions", activeSection, "Dimensions", metricTabCount(view.DimensionCount)),
		metricTabLink(view.ID, "usage", activeSection, "Usage", metricTabCount(view.DashboardCount)),
	)
}

func metricTabLink(viewID, section, activeSection, label string, meta g.Node) g.Node {
	className := "relative -mb-px inline-flex min-h-control-xl items-center gap-2 whitespace-nowrap border-b-2 px-1 text-body-sm font-medium no-underline transition-colors duration-fast"
	if section == activeSection {
		className += " border-fg-accent font-semibold text-fg-default"
	} else {
		className += " border-transparent text-fg-muted hover:border-outline-muted hover:text-fg-default"
	}
	return h.A(h.Class(className), h.Href("/metrics/"+viewID+"/"+section), g.If(section == activeSection, h.Aria("current", "page")), h.Span(g.Text(label)), meta)
}

func metricViewActiveSection(view dashboard.MetricViewDetail, activeSection string) g.Node {
	switch activeSection {
	case "dimensions":
		return metricViewDimensions()
	case "usage":
		return metricViewDashboards()
	default:
		return metricViewMeasures()
	}
}

func metricViewSignals(view dashboard.MetricViewDetail, activeSection string) map[string]any {
	signals := map[string]any{
		"metricGrid": metricViewGrid(view, activeSection),
	}
	if activeSection == "usage" {
		signals["metricUsageGraph"] = metricUsageGraph(view)
	}
	return signals
}

func metricDetailRailStateScript() g.Node {
	return h.Script(g.Raw(`try{if(window.localStorage.getItem("libredash.metricDetailRail")==="collapsed"){document.documentElement.setAttribute("data-metric-detail-rail","collapsed")}}catch(e){}`))
}

func loginBackgroundLoaderScript() g.Node {
	return h.Script(g.Raw(`(()=>{const schedule=(task)=>{const run=()=>{"requestIdleCallback"in window?requestIdleCallback(task,{timeout:1600}):setTimeout(task,600)};document.readyState==="complete"?run():window.addEventListener("load",run,{once:true})};document.addEventListener("libredash-login-background-init",()=>schedule(()=>{const el=document.querySelector("[data-login-background]");if(!el)return;const state=el.dataset.backgroundState;if(state==="loading"||state==="loaded")return;const src=el.dataset.moduleSrc;if(!src)return;el.dataset.backgroundState="loading";import(src).then(()=>{el.dataset.backgroundState="loaded"}).catch((error)=>{el.dataset.backgroundState="error";console.error("LibreDash login background failed to load",error)})}))})();`))
}

func displayLabel(label, fallback string) string {
	if strings.TrimSpace(label) != "" {
		return label
	}
	return fallback
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func MetricViewPage(catalog dashboard.Catalog, view dashboard.MetricViewDetail, activeSection string) g.Node {
	activeSection = normalizeMetricViewSection(activeSection)
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Metric View",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			metricDetailRailStateScript(),
			g.If(activeSection == "usage", h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/metric-usage-graph.css")))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/data-grid.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/detail-rail.js"))),
			g.If(activeSection == "usage", h.Script(h.Type("module"), h.Src(staticAsset("/static/metric-usage-graph.js")))),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		),
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				ds.Signals(metricViewSignals(view, activeSection)),
				h.Div(h.Class(appShellClass),
					sidebar(sidebarConfigForMetricView(catalog, view)),
					h.Section(h.Class(metricMainClass), h.Aria("label", "LibreDash metric view"),
						metricViewHeader(view),
						g.El("ld-detail-rail", h.Class(metricWorkspaceClass), g.Attr("data-detail-rail", ""),
							h.Div(h.Class(metricContentColumnClass),
								metricTabs(view, activeSection),
								h.Div(h.Class("min-h-0 overflow-auto px-3 py-4"),
									metricViewActiveSection(view, activeSection),
								),
							),
							metricViewInfoSidebar(view),
						),
					),
				),
				inspectorElement(),
			),
		},
	})
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
		Head: pageHead(
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/model-graph.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/model-graph.js"))),
			inspectorScript(),
		),
		Body: []g.Node{
			h.Main(
				h.ID("model"),
				h.Class(appRootClass),
				h.Div(h.Class(appShellClass),
					sidebar(sidebarConfigForModel(catalog, model)),
					h.Section(h.Class(modelMainClass), h.Aria("label", "LibreDash semantic model"),
						workspaceHeader(
							"Semantic model",
							model.Title,
							model.Name,
							modelStats(model.Stats),
						),
						h.Div(h.Class("grid min-h-model-graph min-w-0 flex-1 overflow-hidden rounded-default border border-outline-variant bg-panel shadow-resting-md"),
							g.El("ld-model-graph", h.Class("block min-h-0 w-full"), g.Attr("data-model", modelGraphJSON(model))),
						),
					),
				),
				inspectorElement(),
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

func reportPageHeaderDetail(pages []dashboard.Page, activePage dashboard.Page) string {
	title := displayLabel(activePage.Title, activePage.ID)
	for index, page := range pages {
		if page.ID == activePage.ID {
			return formatReportPageNumber(index, len(pages)) + ". " + title
		}
	}
	return title
}

func formatReportPageNumber(index, pageCount int) string {
	pageNumber := strconv.Itoa(index + 1)
	if pageCount >= 10 {
		width := len(strconv.Itoa(pageCount))
		if len(pageNumber) < width {
			return strings.Repeat("0", width-len(pageNumber)) + pageNumber
		}
	}
	return pageNumber
}

func modelStats(stats dashboard.ModelStats) g.Node {
	return h.Div(h.Class("model-stats"),
		modelStat("Sources", stats.Sources),
		modelStat("Tables", stats.ModelTables),
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

func sidebar(config map[string]any) g.Node {
	return g.El("ld-sidebar", h.Class("border-r border-outline-variant max-sm:border-b max-sm:border-r-0"), g.Attr("config", jsonString(config)))
}

func sidebarConfigForCatalog(catalog dashboard.Catalog) map[string]any {
	modelID := ""
	modelTitle := ""
	if len(catalog.MetricViews) > 0 {
		view := catalog.MetricViews[0]
		modelID = view.SemanticModel
		modelTitle = view.ModelTitle
	}
	return sidebarConfig(catalog, "dashboards", "", workspaceDisplayTitle(catalog), "Dashboards", "Discovery", modelID, modelTitle, false)
}

func sidebarConfigForModels(catalog dashboard.Catalog) map[string]any {
	return sidebarConfig(catalog, "workspaces", "", workspaceDisplayTitle(catalog), "Semantic Models", "Catalog", "", "", false)
}

func sidebarConfigForMetrics(catalog dashboard.Catalog) map[string]any {
	return sidebarConfig(catalog, "workspaces", "", workspaceDisplayTitle(catalog), "Metric Views", "Catalog", "", "", false)
}

func sidebarConfigForMetricView(catalog dashboard.Catalog, view dashboard.MetricViewDetail) map[string]any {
	return sidebarConfig(catalog, "workspaces", "", workspaceDisplayTitle(catalog), "Metric view", view.Title, view.SemanticModel, view.ModelTitle, false)
}

func sidebarConfigForReport(catalog dashboard.Catalog, report semantic.Dashboard, model *semantic.Model, activePage dashboard.Page) map[string]any {
	return sidebarConfig(catalog, "workspaces", report.ID, workspaceDisplayTitle(catalog), report.Title, activePage.Title, model.Name, model.Title, true)
}

func sidebarConfigForModel(catalog dashboard.Catalog, model dashboard.ModelGraph) map[string]any {
	return sidebarConfig(catalog, "workspaces", "", workspaceDisplayTitle(catalog), "Semantic model", model.Title, model.Name, model.Title, false)
}

func sidebarConfigForWorkspace(catalog dashboard.Catalog, active, roleLabel string) map[string]any {
	config := sidebarConfig(catalog, active, "", workspaceDisplayTitle(catalog), "Workspace", "Published assets", "", "", false)
	if roleLabel != "" {
		config["userRole"] = roleLabel
	}
	return config
}

func sidebarConfig(catalog dashboard.Catalog, active, dashboardID, workspaceTitle, dashboardTitle, pageTitle, modelID, modelTitle string, compact bool) map[string]any {
	return map[string]any{
		"workspaceTitle": workspaceTitle,
		"active":         active,
		"dashboardId":    dashboardID,
		"dashboardTitle": dashboardTitle,
		"pageTitle":      pageTitle,
		"modelId":        modelID,
		"modelTitle":     modelTitle,
		"compact":        compact,
		"groups":         sidebarGroups(catalog),
	}
}

func sidebarGroups(catalog dashboard.Catalog) []map[string]any {
	workspaceID := catalog.Workspace.ID
	if strings.TrimSpace(workspaceID) == "" {
		workspaceID = "libredash"
	}
	return []map[string]any{
		{
			"label": "Navigation",
			"items": []map[string]any{
				{"id": "dashboards", "label": "Dashboards", "href": "/", "icon": "dashboard", "meta": "Reports"},
				{"id": "workspaces", "label": "Workspaces", "href": "/workspaces", "icon": "catalog", "meta": "Published assets"},
				{"id": "metrics", "label": "Metric Views", "href": "/metrics", "icon": "data", "meta": "Business metrics"},
				{"id": "models", "label": "Semantic Models", "href": "/models", "icon": "model", "meta": "Reusable data models"},
				{"id": "connections", "label": "Connections", "href": "/connections", "icon": "data", "meta": "Data access"},
				{"id": "settings", "label": "Settings", "href": "/workspaces/" + workspaceID + "/permissions", "icon": "settings", "meta": "Permissions"},
			},
		},
	}
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

func workspaceDisplayTitle(catalog dashboard.Catalog) string {
	if strings.TrimSpace(catalog.Workspace.Title) != "" {
		return catalog.Workspace.Title
	}
	return "LibreDash Workspace"
}

func reportSidebar(config map[string]any) g.Node {
	return g.El("ld-report-sidebar", h.Class("border-l border-outline-variant max-sm:border-l-0 max-sm:border-t"), g.Attr("config", jsonString(config)))
}

func reportSidebarConfig(report semantic.Dashboard, model *semantic.Model, pages []dashboard.Page, activePage dashboard.Page) map[string]any {
	items := make([]map[string]any, 0, len(pages))
	for _, page := range pages {
		items = append(items, map[string]any{
			"id":     page.ID,
			"title":  page.Title,
			"href":   "/dashboards/" + report.ID + "/pages/" + page.ID,
			"active": page.ID == activePage.ID,
		})
	}
	return map[string]any{
		"dashboardId":    report.ID,
		"dashboardTitle": report.Title,
		"pageId":         activePage.ID,
		"pageTitle":      activePage.Title,
		"modelId":        model.Name,
		"modelTitle":     model.Title,
		"modelHref":      "/models/" + model.Name,
		"pages":          items,
	}
}

func workspaceHeader(eyebrow, title, detail string, actions g.Node) g.Node {
	return h.Header(h.Class("grid min-w-0 grid-cols-workspace-header items-center gap-2 border-b border-outline-muted px-4 py-2.5"),
		h.Div(h.Class("min-w-0"),
			g.If(eyebrow != "", h.P(h.Class(eyebrowClass), g.Text(eyebrow))),
			h.H1(h.Class("m-0 truncate text-title-sm font-semibold leading-snug text-fg-default"), g.Text(title)),
			g.If(detail != "", h.P(h.Class("m-0 mt-1 truncate text-body-sm font-normal leading-snug text-fg-muted"), g.Text(detail))),
		),
		h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"), actions),
	)
}

func reportActions(modelID, dashboardID string) g.Node {
	return h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"),
		h.Button(
			h.Class(actionButtonClass),
			h.Type("button"),
			h.Title("Refresh model materializations"),
			h.Aria("label", "Refresh model materializations"),
			g.Attr("data-attr:disabled", "$status.loading"),
			g.Attr("data-on:click", postAction("/commands/refresh-materializations?model="+modelID+"&dashboard="+dashboardID)),
			lucide.RefreshCw(buttonIconAttrs()...),
		),
	)
}

func jsonString(value any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func initialSignals(dataDir, clientID, csrfToken string, report semantic.Dashboard, model *semantic.Model, activePage dashboard.Page, initialFilters dashboard.Filters) map[string]any {
	tableRequest := defaultTableRequest(report, activePage)
	initialFilters = initialFilters.WithDefaults()
	return map[string]any{
		"runtime": map[string]any{
			"clientId":    clientID,
			"dashboardId": report.ID,
			"pageId":      activePage.ID,
			"modelId":     model.Name,
		},
		"csrfToken":     csrfToken,
		"filterConfig":  report.FilterConfigForPage(activePage.ID),
		"filters":       initialFilters,
		"urlParams":     report.URLParamsFromFiltersForPage(activePage.ID, initialFilters),
		"urlParamShape": report.URLParamShapeForPage(activePage.ID),
		"filterOptions": map[string]any{},
		"visualCommand": map[string]any{
			"visualId": "",
			"field":    "",
			"value":    "",
			"label":    "",
			"mode":     "toggle",
		},
		"tableCommand": map[string]any{
			"table":        tableRequest.Table,
			"block":        tableRequest.Block,
			"start":        tableRequest.Start,
			"count":        tableRequest.Count,
			"resetVersion": tableRequest.ResetVersion,
			"sort": map[string]any{
				"key":       tableRequest.Sort.Key,
				"direction": tableRequest.Sort.Direction,
			},
		},
		"tables":  tableSignals(report, activePage, tableRequest),
		"visuals": visualSignals(report, model, activePage),
		"status": map[string]any{
			"loading":       false,
			"error":         "",
			"lastUpdated":   "",
			"dataDirectory": dataDir,
			"setupRequired": false,
		},
	}
}

func defaultTableRequest(report semantic.Dashboard, page dashboard.Page) dashboard.TableRequest {
	request := dashboard.TableRequest{Block: "all", Start: 0, Count: dashboard.TableChunkSize}
	for _, name := range pageTableIDs(page) {
		table, ok := report.Tables[name]
		if !ok {
			continue
		}
		if table.KindOrDefault() == "data_table" {
			request.Table = name
			request.Sort = table.DefaultSort
			break
		}
	}
	return request
}

func tableSignals(report semantic.Dashboard, page dashboard.Page, request dashboard.TableRequest) map[string]any {
	tables := map[string]any{}
	for _, name := range pageTableIDs(page) {
		table, ok := report.Tables[name]
		if !ok {
			continue
		}
		style := table.Style.WithDefaults()
		tables[name] = map[string]any{
			"kind":          table.KindOrDefault(),
			"title":         table.Title,
			"style":         style,
			"columns":       table.Columns,
			"version":       2,
			"totalRows":     0,
			"availableRows": 0,
			"isCapped":      false,
			"rowCap":        dashboard.TableInteractiveRowCap,
			"chunkSize":     dashboard.TableChunkSize,
			"rowHeight":     style.RowHeight(),
			"resetVersion":  request.ResetVersion,
			"sort": map[string]any{
				"key":       table.DefaultSort.Key,
				"direction": table.DefaultSort.Direction,
			},
			"blocks": map[string]any{
				"a": map[string]any{"start": 0, "rows": []any{}},
				"b": map[string]any{"start": dashboard.TableChunkSize, "rows": []any{}},
				"c": map[string]any{"start": dashboard.TableChunkSize * 2, "rows": []any{}},
			},
			"loadingBlock": "",
			"error":        "",
		}
	}
	return tables
}

func tableResetExpression() string {
	count := strconv.Itoa(dashboard.TableChunkSize)
	return "$tableCommand.block = 'all'; $tableCommand.start = 0; $tableCommand.count = " + count + "; $tableCommand.resetVersion = ($tableCommand.resetVersion || 0) + 1; "
}

func visualSignals(report semantic.Dashboard, model *semantic.Model, page dashboard.Page) map[string]any {
	visuals := map[string]any{}
	for _, id := range pageVisualIDs(page) {
		visual, ok := report.Visuals[id]
		if !ok {
			continue
		}
		measureName := ""
		unit := ""
		format := ""
		title := visual.Title
		if model != nil && len(visual.Query.Measures) > 0 {
			measureName = displayField(visual.Query.Measures[0].Field)
		}
		if title == "" {
			title = measureName
		}
		visuals[id] = visualSignal(id, visual, title, unit, format, measureName)
	}
	return visuals
}

func pageVisualIDs(page dashboard.Page) []string {
	seen := map[string]struct{}{}
	ids := []string{}
	for _, item := range page.Visuals {
		if item.Visual == "" {
			continue
		}
		if _, ok := seen[item.Visual]; ok {
			continue
		}
		seen[item.Visual] = struct{}{}
		ids = append(ids, item.Visual)
	}
	sort.Strings(ids)
	return ids
}

func pageTableIDs(page dashboard.Page) []string {
	seen := map[string]struct{}{}
	ids := []string{}
	for _, item := range page.Visuals {
		if item.Table == "" {
			continue
		}
		if _, ok := seen[item.Table]; ok {
			continue
		}
		seen[item.Table] = struct{}{}
		ids = append(ids, item.Table)
	}
	return ids
}

func visualSignal(id string, visual semantic.Visual, title, unit, format, measure string) map[string]any {
	seriesList := []string{}
	if !visual.Query.Series.IsZero() {
		seriesList = append(seriesList, displayField(visual.Query.Series.Field))
	}
	visualType := visual.Type
	if visualType == "" && visual.KindOrDefault() == "kpi" {
		visualType = "kpi"
	}
	signal := map[string]any{
		"version":         3,
		"id":              id,
		"kind":            visual.KindOrDefault(),
		"shape":           visual.ShapeOrDefault(),
		"renderer":        visual.RendererOrDefault(),
		"type":            visualType,
		"title":           title,
		"unit":            unit,
		"format":          format,
		"field":           visual.Interaction.Field,
		"dimensions":      displayFieldRefs(visual.Query.Dimensions),
		"measure":         measure,
		"measures":        displayFieldRefs(visual.Query.Measures),
		"series":          seriesList,
		"options":         visual.CoreOptions(),
		"rendererOptions": map[string]any{},
		"selection":       []any{},
		"data":            []any{},
	}
	if len(visual.RendererOptions) > 0 {
		signal["rendererOptions"] = visual.RendererOptions
	}
	return signal
}

func displayFieldRefs(refs []semantic.FieldRef) []string {
	fields := make([]string, len(refs))
	for i, ref := range refs {
		fields[i] = displayField(ref.Field)
	}
	return fields
}

func displayField(field string) string {
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
}

func iconAttrs() g.Node {
	return g.Attr("aria-hidden", "true")
}

func buttonIconAttrs() []g.Node {
	return []g.Node{g.Attr("aria-hidden", "true"), h.Class("size-4 shrink-0"), h.Style("stroke-width: 1.75")}
}

func metricInfoIconAttrs() []g.Node {
	return []g.Node{g.Attr("aria-hidden", "true"), h.Class("size-4 shrink-0 text-icon-muted"), h.Style("stroke-width: 1.75")}
}

func metricActionIconAttrs() []g.Node {
	return []g.Node{g.Attr("aria-hidden", "true"), h.Class("size-4 shrink-0"), h.Style("stroke-width: 1.75")}
}

func filterDockIconAttrs() []g.Node {
	return []g.Node{g.Attr("aria-hidden", "true"), h.Class("size-4 shrink-0"), h.Style("stroke-width: 1.75")}
}

func loginThemeIconAttrs(mode string) []g.Node {
	return []g.Node{g.Attr("aria-hidden", "true"), g.Attr("data-theme-icon", mode), h.Class("hidden size-4 shrink-0"), h.Style("stroke-width: 1.75")}
}

func canvasVisual(x, y, width, height float64, children ...g.Node) g.Node {
	nodes := []g.Node{
		g.Attr("data-canvas-visual", ""),
		g.Attr("data-x", formatCanvasNumber(x)),
		g.Attr("data-y", formatCanvasNumber(y)),
		g.Attr("data-w", formatCanvasNumber(width)),
		g.Attr("data-h", formatCanvasNumber(height)),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func canvasFilterVisual(x, y, width, height float64, children ...g.Node) g.Node {
	nodes := []g.Node{
		g.Attr("data-canvas-visual", ""),
		g.Attr("data-canvas-filter-visual", ""),
		g.Attr("data-x", formatCanvasNumber(x)),
		g.Attr("data-y", formatCanvasNumber(y)),
		g.Attr("data-w", formatCanvasNumber(width)),
		g.Attr("data-h", formatCanvasNumber(height)),
	}
	nodes = append(nodes, children...)
	return h.Div(nodes...)
}

func renderPageCanvas(page dashboard.Page, report semantic.Dashboard, filters dashboard.Filters, action string) g.Node {
	page = page.WithDefaults()
	filters = filters.WithDefaults()
	nodes := []g.Node{
		g.Attr("width", strconv.Itoa(page.Canvas.Width)),
		g.Attr("height", strconv.Itoa(page.Canvas.Height)),
	}
	for _, visual := range page.PlacedVisuals() {
		nodes = append(nodes, renderPageVisual(page.ID, visual, report, filters, action))
	}
	return g.El("ld-report-canvas", nodes...)
}

func formatCanvasNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func renderPageVisual(pageID string, visual dashboard.PageVisual, report semantic.Dashboard, filters dashboard.Filters, action string) g.Node {
	switch visual.Kind {
	case "header":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height, reportHeader(visual))
	case "kpi_card":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height, kpiCard(visual.Visual))
	case "filter_card":
		return canvasFilterVisual(visual.X, visual.Y, visual.Width, visual.Height,
			filterCard(pageID, visual.Filter, report, filters, action),
		)
	case "line_chart", "area_chart", "bar_chart", "column_chart", "pie_chart", "donut_chart", "scatter_chart", "funnel_chart", "treemap_chart", "gauge_chart", "heatmap_chart", "sankey_chart", "graph_chart", "map_chart", "candlestick_chart", "boxplot_chart", "combo_chart", "waterfall_chart", "histogram_chart", "radar_chart", "tree_chart", "sunburst_chart":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height,
			chartPanel(visual.Visual),
		)
	case "table":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height, tablePanel(visual.Table))
	default:
		return nil
	}
}

func filterCard(pageID, filterID string, report semantic.Dashboard, filters dashboard.Filters, action string) g.Node {
	tableReset := tableResetExpression()
	return h.Article(h.Class(visualCardClass), g.Attr("data-canvas-filter-visual", ""),
		g.El("ld-filter-card",
			g.Attr("filter-id", filterID),
			g.Attr("config", jsonString(report.FilterConfigForPage(pageID))),
			g.Attr("filters", jsonString(filters)),
			g.Attr("options", "{}"),
			g.Attr("data-attr:config", "$filterConfig"),
			g.Attr("data-attr:filters", "$filters"),
			g.Attr("data-attr:options", "$filterOptions"),
			g.Attr("data-attr:loading", "$status.loading"),
			g.Attr("data-on:ld-filters-change", "$filters = evt.detail.filters; $urlParams = evt.detail.urlParams; window.DatastarURLSync && window.DatastarURLSync.replace($urlParams); "+tableReset+action),
			filterCardFallback(filterID, report, filters),
		),
	)
}

func filterCardFallback(filterID string, report semantic.Dashboard, filters dashboard.Filters) g.Node {
	definition, ok := report.Filters[filterID]
	if !ok {
		return nil
	}
	control := filters.Controls[filterID]
	return h.Div(h.Class("grid h-full min-h-0 content-center gap-1 rounded-default bg-panel p-3 text-body-sm"),
		h.Span(h.Class("text-caption font-medium uppercase leading-tight text-fg-muted"), g.Text(definition.Label)),
		h.Span(h.Class("truncate text-body-sm font-semibold text-fg-default"), g.Text(filterCardSummary(definition, control))),
	)
}

func filterCardSummary(definition semantic.FilterDefinition, control dashboard.FilterControl) string {
	switch definition.Type {
	case "date_range":
		if control.From != "" || control.To != "" {
			if control.From != "" && control.To != "" {
				return control.From + " - " + control.To
			}
			if control.From != "" {
				return "From " + control.From
			}
			return "Until " + control.To
		}
		preset := control.Preset
		if preset == "" {
			preset = definition.Default.Preset
		}
		if preset == "" {
			preset = "all"
		}
		for _, item := range definition.Presets {
			if item.Value == preset {
				return item.Label
			}
		}
		return "Custom range"
	case "multi_select":
		if len(control.Values) == 0 {
			label := strings.ToLower(definition.Label)
			if label == "state" {
				return "All states"
			}
			return "All " + label
		}
		if len(control.Values) == 1 {
			return control.Values[0]
		}
		return strconv.Itoa(len(control.Values)) + " selected"
	case "text":
		if strings.TrimSpace(control.Value) == "" {
			return "Any " + strings.ToLower(definition.Label)
		}
		return control.Value
	default:
		return definition.Label
	}
}

func reportHeader(visual dashboard.PageVisual) g.Node {
	if visual.Eyebrow == "" {
		visual.Eyebrow = "LibreDash report"
	}
	if visual.Title == "" {
		visual.Title = "Dashboard"
	}
	return h.Header(h.Class("grid h-full min-h-0 grid-cols-workspace-header items-center gap-3 rounded-default bg-transparent p-2"),
		h.Div(
			h.P(h.Class(eyebrowClass), g.Text(visual.Eyebrow)),
			h.H1(h.Class("m-0 text-title-lg font-semibold leading-tight text-fg-default"), g.Text(visual.Title)),
		),
		h.Div(h.Class("flex flex-wrap justify-end gap-2"),
			g.Map(visual.Badges, func(badge string) g.Node {
				return h.Span(h.Class(tagClass), g.Text(badge))
			}),
		),
	)
}

func filtersDock(report semantic.Dashboard, pageID string, action string) g.Node {
	return h.Details(h.Class("group grid min-h-0 w-full border-l border-outline-variant bg-panel-muted transition-[width,background-color] duration-normal ease-ld sm:w-filter-closed"), h.Aria("label", "Report filters"), g.Attr("data-filter-dock", ""),
		h.Summary(h.Class("flex min-h-control-xl cursor-pointer list-none items-center justify-center gap-2 border-b border-outline-variant px-2 text-caption font-medium uppercase text-fg-muted marker:hidden transition-colors duration-fast hover:text-fg-default focus-visible:text-fg-default focus-visible:outline-0 sm:flex sm:h-full sm:w-filter-closed sm:flex-col sm:justify-start sm:border-b-0 sm:px-0 sm:py-4"), h.Title("Toggle filters"), g.Attr("data-filter-summary", ""),
			lucide.SlidersHorizontal(filterDockIconAttrs()...),
			h.Span(h.Class("sm:[writing-mode:vertical-rl]"), g.Text("Filters")),
			h.Span(h.Class("sr-only"), g.Text("Toggle filters")),
		),
		filtersPane(report, pageID, action),
	)
}

func filtersPane(report semantic.Dashboard, pageID string, action string) g.Node {
	tableReset := tableResetExpression()
	return h.Div(h.Class("min-h-0 w-full overflow-auto border-t border-outline-variant bg-app p-3 sm:hidden sm:w-filter-dock sm:border-l sm:border-t-0"), g.Attr("data-filter-pane", ""),
		g.El("ld-filter-panel",
			g.Attr("config", jsonString(report.FilterConfigForPage(pageID))),
			g.Attr("data-attr:filters", "$filters"),
			g.Attr("data-attr:options", "$filterOptions"),
			g.Attr("data-attr:loading", "$status.loading"),
			g.Attr("data-on:ld-filters-change", "$filters = evt.detail.filters; $urlParams = evt.detail.urlParams; window.DatastarURLSync && window.DatastarURLSync.replace($urlParams); "+tableReset+action),
			g.Attr("data-on:ld-filters-reset", "$filters = evt.detail.filters; $urlParams = evt.detail.urlParams; window.DatastarURLSync && window.DatastarURLSync.replace($urlParams); "+tableReset+postAction("/commands/reset-filters")),
			g.Attr("data-on:ld-filters-refresh", action),
			g.Attr("data-on:ld-visual-selection-clear", "$filters.visualSelections = []; "+postAction("/commands/clear-selection")),
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
	signal := "visuals." + visualID
	return h.Article(h.Class(visualCardClass),
		g.El("ld-echart",
			g.Attr("visual-id", visualID),
			g.Attr("data-attr:chart", "$"+signal),
			g.Attr("data-on:ld-chart-select", "$visualCommand = evt.detail; "+postAction("/commands/chart-select")),
			g.Attr("data-on:ld-chart-clear-selection", "$filters.visualSelections = []; "+postAction("/commands/clear-selection")),
		),
	)
}

func kpiCard(visualID string) g.Node {
	return h.Article(h.Class(visualCardClass),
		g.El("ld-kpi-card",
			g.Attr("visual-id", visualID),
			g.Attr("data-attr:visual", "$visuals."+visualID),
		),
	)
}

func tablePanel(tableName string) g.Node {
	if tableName == "" {
		tableName = "orders"
	}
	return h.Section(h.Class(visualCardClass),
		g.El("ld-data-table",
			g.Attr("table-id", tableName),
			g.Attr("data-attr:table", "$tables."+tableName),
			g.Attr("data-on:ld-table-window-change", "$tableCommand = evt.detail; "+postAction("/commands/table-window")),
		),
	)
}
