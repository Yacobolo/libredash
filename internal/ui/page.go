package ui

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

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

func inspectorScript() g.Node {
	return h.Script(h.Type("module"), h.Src(staticAsset("/static/datastar-inspector.js")))
}

func inspectorElement() g.Node {
	return g.El("datastar-inspector")
}

func Page(dataDir, clientID, csrfToken string, catalog dashboard.Catalog, report semantic.Dashboard, model *semantic.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters) g.Node {
	if activePage.ID == "" {
		activePage = defaultPage()
	}
	action := updateAction(report.ID, activePage.ID)
	initAction := "window.DatastarURLSync && window.DatastarURLSync.bindPopstate($urlParamShape); " + action
	tableReset := tableResetExpression()
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
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
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
		},
		Body: []g.Node{
			h.Main(
				h.ID("dashboard"),
				h.Class("report-app"),
				ds.Signals(initialSignals(dataDir, clientID, csrfToken, report, model, activePage, initialFilters)),
				ds.Init(initAction),
				g.Attr("data-on:datastar-url-params-sync__window", "$urlParams = evt.detail.params; $filters = window.LibreDashFilterURL.fromParams($filterConfig, $filters, $urlParams); "+tableReset+action),
				h.Div(h.Class("app-shell report-shell"),
					sidebar(sidebarConfigForReport(catalog, report, model, activePage)),
					reportSidebar(reportSidebarConfig(report, model, pages, activePage)),
					h.Section(h.Class("app-main report-main"), h.Aria("label", "LibreDash report canvas"),
						workspaceHeader(
							"",
							report.Title,
							activePage.Title,
							reportActions(model.Name, report.ID),
						),
						h.Div(h.Class("report-dashboard-shell"),
							h.Div(h.Class("report-canvas-shell"),
								renderPageCanvas(activePage, report, initialFilters, action),
							),
							filtersDock(report, action),
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
			h.Link(h.Href("https://cdn.jsdelivr.net/npm/daisyui@5"), h.Rel("stylesheet"), h.Type("text/css")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/login.js"))),
			inspectorScript(),
		},
		Body: []g.Node{
			h.Main(h.Class("login-screen"), h.Aria("label", "LibreDash login"),
				g.El("ld-topology-background"),
				h.Div(h.Class("login-shade"), h.Aria("hidden", "true")),
				h.Section(h.Class("login-panel"),
					h.Div(h.Class("login-brand"), h.Aria("hidden", "true"),
						h.H1(g.Text("LibreDash")),
					),
					h.Button(h.Type("button"), h.Class("login-azure-button"),
						h.Span(h.Class("microsoft-mark"), h.Aria("hidden", "true"),
							h.Span(),
							h.Span(),
							h.Span(),
							h.Span(),
						),
						h.Span(g.Text("Sign in with Azure Active Directory")),
					),
				),
				inspectorElement(),
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
		Head: []g.Node{
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Href("https://cdn.jsdelivr.net/npm/daisyui@5"), h.Rel("stylesheet"), h.Type("text/css")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			inspectorScript(),
		},
		Body: []g.Node{
			h.Main(h.Class("report-app"),
				h.Div(h.Class("app-shell"),
					sidebar(sidebarConfigForCatalog(catalog)),
					h.Section(h.Class("app-main catalog-main"), h.Aria("label", "LibreDash dashboard catalog"),
						workspaceHeader(
							"",
							"Dashboards",
							"Reports backed by semantic models.",
							nil,
						),
						h.Div(h.Class("catalog-grid"),
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
		Head: []g.Node{
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Href("https://cdn.jsdelivr.net/npm/daisyui@5"), h.Rel("stylesheet"), h.Type("text/css")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			inspectorScript(),
		},
		Body: []g.Node{
			h.Main(h.Class("report-app"),
				h.Div(h.Class("app-shell"),
					sidebar(sidebarConfigForModels(catalog)),
					h.Section(h.Class("app-main catalog-main"), h.Aria("label", "LibreDash semantic model catalog"),
						workspaceHeader(
							"",
							"Semantic Models",
							"Reusable model definitions.",
							nil,
						),
						h.Div(h.Class("catalog-grid"),
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
		Head: []g.Node{
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Href("https://cdn.jsdelivr.net/npm/daisyui@5"), h.Rel("stylesheet"), h.Type("text/css")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			inspectorScript(),
		},
		Body: []g.Node{
			h.Main(h.Class("report-app"),
				h.Div(h.Class("app-shell"),
					sidebar(sidebarConfigForMetrics(catalog)),
					h.Section(h.Class("app-main catalog-main"), h.Aria("label", "LibreDash metric view catalog"),
						workspaceHeader(
							"",
							"Metric Views",
							"Business-facing analytics contracts.",
							nil,
						),
						h.Div(h.Class("catalog-grid"),
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
	return h.Article(h.Class("catalog-card"),
		h.Div(h.Class("catalog-card-main"),
			h.P(h.Class("report-eyebrow"), g.Text(eyebrow)),
			h.H2(g.Text(report.Title)),
			h.P(g.Text(report.Description)),
		),
		h.Div(h.Class("catalog-tags"),
			g.Map(report.Tags, func(tag string) g.Node {
				return h.Span(g.Text(tag))
			}),
		),
		h.Footer(h.Class("catalog-card-footer"),
			h.Span(g.Textf("%d pages, %d views", report.PageCount, len(report.MetricViews))),
			h.A(h.Class("catalog-open"), h.Href("/dashboards/"+report.ID),
				lucide.ExternalLink(iconAttrs()),
				h.Span(g.Text("Open")),
			),
		),
	)
}

func metricViewCard(view dashboard.MetricViewSummary) g.Node {
	return h.Article(h.Class("catalog-card"),
		h.Div(h.Class("catalog-card-main"),
			h.P(h.Class("report-eyebrow"), g.Text(view.ModelTitle)),
			h.H2(g.Text(view.Title)),
			h.P(g.Text(view.Description)),
		),
		h.Div(h.Class("catalog-tags"),
			h.Span(g.Text(view.Dataset)),
			h.Span(g.Text(view.Timeseries)),
		),
		h.Footer(h.Class("catalog-card-footer"),
			h.Span(g.Textf("%d dimensions, %d measures", view.DimensionCount, view.MeasureCount)),
			h.A(h.Class("catalog-open"), h.Href("/metrics/"+view.ID+"/measures"),
				lucide.ExternalLink(iconAttrs()),
				h.Span(g.Text("Open")),
			),
		),
	)
}

func modelCard(model dashboard.CatalogModel) g.Node {
	return h.Article(h.Class("catalog-card"),
		h.Div(h.Class("catalog-card-main"),
			h.P(h.Class("report-eyebrow"), g.Text("Semantic model")),
			h.H2(g.Text(model.Title)),
			h.P(g.Text(model.Description)),
		),
		h.Footer(h.Class("catalog-card-footer"),
			h.Span(g.Text("Reusable model")),
			h.A(h.Class("catalog-open"), h.Href("/models/"+model.ID),
				lucide.ExternalLink(iconAttrs()),
				h.Span(g.Text("Open")),
			),
		),
	)
}

func metricViewActions(view dashboard.MetricViewDetail) g.Node {
	return h.Div(h.Class("report-actions"),
		h.A(h.Class("report-action-button"), h.Href("/models/"+view.SemanticModel), h.Title("Open semantic model"), h.Aria("label", "Open semantic model"),
			lucide.Network(iconAttrs()),
		),
	)
}

func metricViewHeader(view dashboard.MetricViewDetail) g.Node {
	return h.Header(h.Class("metric-detail-header"),
		h.Div(h.Class("metric-header-row"),
			h.Div(h.Class("metric-header-copy"),
				h.H1(h.Class("workspace-title"), g.Text(view.Title)),
				g.If(strings.TrimSpace(view.Description) != "", h.P(h.Class("workspace-detail"), g.Text(view.Description))),
			),
			metricViewActions(view),
		),
	)
}

func metricViewInfoSidebar(view dashboard.MetricViewDetail) g.Node {
	return h.Aside(h.Class("metric-info-sidebar"), h.Aria("label", "Metric view details"),
		h.Div(h.Class("metric-info-header"),
			h.H2(lucide.FileText(iconAttrs()), h.Span(g.Text("Details"))),
		),
		h.Div(h.Class("metric-info-body"),
			h.Div(h.Class("metric-info-group"),
				metricInfoItem("Model context", h.A(h.Href("/models/"+view.SemanticModel), lucide.Box(iconAttrs()), h.Span(g.Text(view.ModelTitle)))),
				metricInfoItem("Source dataset", h.Span(lucide.Table2(iconAttrs()), h.Code(g.Text(view.Dataset)))),
				metricInfoItem("Primary timeseries", h.Span(lucide.Calendar(iconAttrs()), h.Code(g.Text(view.Timeseries)))),
			),
		),
	)
}

func metricInfoItem(label string, value g.Node) g.Node {
	return h.Div(h.Class("metric-info-item"),
		h.Span(h.Class("metric-info-label"), g.Text(label)),
		h.Div(h.Class("metric-info-value"), value),
	)
}

func metricTabCount(count int) g.Node {
	return h.Span(h.Class("metric-tab-count"), g.Text(strconv.Itoa(count)))
}

func metricCountPill(label string, count int, tone string, icon g.Node) g.Node {
	return h.Span(h.Class("metric-count-pill metric-count-pill-"+tone),
		icon,
		h.Strong(g.Text(strconv.Itoa(count))),
		g.Text(" "+pluralize(label, count)),
	)
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
	return h.Section(h.ID("dimensions"), h.Class("metric-contract-section metric-contract-section-dimensions"),
		g.El("ld-data-grid", g.Attr("data-attr:grid", "$metricGrid")),
	)
}

func metricViewMeasures() g.Node {
	return h.Section(h.ID("measures"), h.Class("metric-contract-section metric-contract-section-measures"),
		g.El("ld-data-grid", g.Attr("data-attr:grid", "$metricGrid")),
	)
}

func metricViewDashboards() g.Node {
	return h.Section(h.ID("usage"), h.Class("metric-contract-section metric-contract-section-usage"),
		g.El("ld-metric-usage-graph", g.Attr("data-attr:graph", "$metricUsageGraph")),
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
				{ID: "expression", Header: "Expression", Kind: "expression", Width: "420px"},
				{ID: "unit", Header: "Unit", Kind: "badge", Width: "96px"},
				{ID: "format", Header: "Format", Kind: "badge", Width: "108px"},
			},
			Rows:     rows,
			Empty:    "No measures are defined for this metric view.",
			MinWidth: "934px",
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
		{ID: "dataset", Label: view.Dataset, Kind: "dataset", Meta: "semantic dataset"},
		{ID: "metrics_view", Label: view.Title, Kind: "metrics_view", Meta: "metric contract"},
	}
	edges := []graphEdge{
		{ID: "model_dataset", Source: "model", Target: "dataset", Label: "defines", Kind: "semantic"},
		{ID: "dataset_metric_view", Source: "dataset", Target: "metrics_view", Label: "powers", Kind: "semantic"},
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
			ID:     "metrics_view_" + report.ID,
			Source: "metrics_view",
			Target: nodeID,
			Label:  "used by",
			Kind:   "usage",
		})
	}
	return graph{Nodes: nodes, Edges: edges}
}

func metricTabs(view dashboard.MetricViewDetail, activeSection string) g.Node {
	return h.Nav(h.Class("metric-tabs"), h.Aria("label", "Metric view sections"),
		metricTabLink(view.ID, "measures", activeSection, "Measures", metricTabCount(view.MeasureCount)),
		metricTabLink(view.ID, "dimensions", activeSection, "Dimensions", metricTabCount(view.DimensionCount)),
		metricTabLink(view.ID, "usage", activeSection, "Usage", metricTabCount(view.DashboardCount)),
	)
}

func metricTabLink(viewID, section, activeSection, label string, meta g.Node) g.Node {
	className := "metric-tab"
	if section == activeSection {
		className += " metric-tab-active"
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
		Head: []g.Node{
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Href("https://cdn.jsdelivr.net/npm/daisyui@5"), h.Rel("stylesheet"), h.Type("text/css")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			metricDetailRailStateScript(),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			g.If(activeSection == "usage", h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/metric-usage-graph.css")))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/data-grid.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/detail-rail.js"))),
			g.If(activeSection == "usage", h.Script(h.Type("module"), h.Src(staticAsset("/static/metric-usage-graph.js")))),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		},
		Body: []g.Node{
			h.Main(h.Class("report-app"),
				ds.Signals(metricViewSignals(view, activeSection)),
				h.Div(h.Class("app-shell"),
					sidebar(sidebarConfigForMetricView(catalog, view)),
					h.Section(h.Class("app-main metric-main"), h.Aria("label", "LibreDash metric view"),
						metricViewHeader(view),
						g.El("ld-detail-rail", h.Class("metric-workspace"),
							h.Div(h.Class("metric-content-column"),
								metricTabs(view, activeSection),
								h.Div(h.Class("metric-contract-main"),
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
		Head: []g.Node{
			h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1")),
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Href("https://cdn.jsdelivr.net/npm/daisyui@5"), h.Rel("stylesheet"), h.Type("text/css")),
			h.Script(h.Src("https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4")),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/model-graph.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/model-graph.js"))),
			inspectorScript(),
		},
		Body: []g.Node{
			h.Main(
				h.ID("model"),
				h.Class("report-app"),
				h.Div(h.Class("app-shell"),
					sidebar(sidebarConfigForModel(catalog, model)),
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

func sidebar(config map[string]any) g.Node {
	return g.El("ld-sidebar", g.Attr("config", jsonString(config)))
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
	return sidebarConfig(catalog, "models", "", workspaceDisplayTitle(catalog), "Semantic Models", "Catalog", "", "", false)
}

func sidebarConfigForMetrics(catalog dashboard.Catalog) map[string]any {
	return sidebarConfig(catalog, "metrics", "", workspaceDisplayTitle(catalog), "Metric Views", "Catalog", "", "", false)
}

func sidebarConfigForMetricView(catalog dashboard.Catalog, view dashboard.MetricViewDetail) map[string]any {
	return sidebarConfig(catalog, "metrics", "", workspaceDisplayTitle(catalog), "Metric view", view.Title, view.SemanticModel, view.ModelTitle, false)
}

func sidebarConfigForReport(catalog dashboard.Catalog, report semantic.Dashboard, model *semantic.Model, activePage dashboard.Page) map[string]any {
	return sidebarConfig(catalog, "dashboards", report.ID, workspaceDisplayTitle(catalog), report.Title, activePage.Title, model.Name, model.Title, true)
}

func sidebarConfigForModel(catalog dashboard.Catalog, model dashboard.ModelGraph) map[string]any {
	return sidebarConfig(catalog, "models", "", workspaceDisplayTitle(catalog), "Semantic model", model.Title, model.Name, model.Title, false)
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
	return []map[string]any{
		{
			"label": "Navigation",
			"items": []map[string]any{
				{"id": "dashboards", "label": "Dashboards", "href": "/", "icon": "dashboard", "meta": "Reports and models"},
				{"id": "metrics", "label": "Metric Views", "href": "/metrics", "icon": "data", "meta": "Business metrics"},
				{"id": "models", "label": "Semantic Models", "href": "/models", "icon": "model", "meta": "Reusable data models"},
			},
		},
		{
			"label": "Data",
			"items": []map[string]any{
				{"id": "data:sources", "label": "Sources", "href": "/", "icon": "data", "meta": "Coming soon", "disabled": true},
				{"id": "data:cache", "label": "DuckDB Cache", "href": "/", "icon": "cache", "meta": "Import mode", "disabled": true},
				{"id": "settings", "label": "Settings", "href": "/", "icon": "settings", "meta": "Coming soon", "disabled": true},
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
	return g.El("ld-report-sidebar", g.Attr("config", jsonString(config)))
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
	return h.Header(h.Class("workspace-header"),
		h.Div(h.Class("workspace-title-block"),
			g.If(eyebrow != "", h.P(h.Class("report-eyebrow"), g.Text(eyebrow))),
			h.H1(h.Class("workspace-title"), g.Text(title)),
			g.If(detail != "", h.P(h.Class("workspace-detail"), g.Text(detail))),
		),
		h.Div(h.Class("workspace-actions"), actions),
	)
}

func reportActions(modelID, dashboardID string) g.Node {
	return h.Div(h.Class("report-actions"),
		h.Button(
			h.Class("report-action-button"),
			h.Type("button"),
			h.Title("Re-import DuckDB cache"),
			h.Aria("label", "Re-import DuckDB cache"),
			g.Attr("data-attr:disabled", "$status.loading"),
			g.Attr("data-on:click", postAction("/commands/refresh-cache?model="+modelID+"&dashboard="+dashboardID)),
			lucide.RefreshCw(iconAttrs()),
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
	tableRequest := defaultTableRequest(report)
	initialFilters = initialFilters.WithDefaults()
	return map[string]any{
		"runtime": map[string]any{
			"clientId":    clientID,
			"dashboardId": report.ID,
			"pageId":      activePage.ID,
			"modelId":     model.Name,
		},
		"csrfToken":     csrfToken,
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
		"tables": tableSignals(report, tableRequest),
		"charts": chartSignals(report),
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
	request := dashboard.TableRequest{Block: "all", Start: 0, Count: dashboard.TableChunkSize}
	if table, ok := report.Tables["orders"]; ok && table.KindOrDefault() == "data_table" {
		request.Table = "orders"
		request.Sort = table.DefaultSort
	} else {
		for _, name := range sortedKeys(report.Tables) {
			table := report.Tables[name]
			if table.KindOrDefault() != "data_table" {
				continue
			}
			request.Table = name
			request.Sort = table.DefaultSort
			break
		}
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
			"kind":          table.KindOrDefault(),
			"title":         table.Title,
			"columns":       table.Columns,
			"version":       2,
			"totalRows":     0,
			"availableRows": 0,
			"isCapped":      false,
			"rowCap":        dashboard.TableInteractiveRowCap,
			"chunkSize":     dashboard.TableChunkSize,
			"rowHeight":     dashboard.TableRowHeight,
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

func chartSignals(report semantic.Dashboard) map[string]any {
	charts := map[string]any{}
	for _, id := range sortedKeys(report.Visuals) {
		visual := report.Visuals[id]
		measureName := ""
		unit := ""
		if len(visual.Query.Measures) > 0 {
			measureName = visual.Query.Measures[0]
		}
		charts[id] = chartSignal(id, visual, unit, measureName)
	}
	return charts
}

func chartSignal(id string, visual semantic.Visual, unit, measure string) map[string]any {
	seriesList := []string{}
	if visual.Query.Series != "" {
		seriesList = append(seriesList, visual.Query.Series)
	}
	signal := map[string]any{
		"version":         3,
		"id":              id,
		"kind":            visual.KindOrDefault(),
		"shape":           visual.ShapeOrDefault(),
		"renderer":        visual.RendererOrDefault(),
		"type":            visual.Type,
		"title":           visual.Title,
		"unit":            unit,
		"field":           visual.Interaction.Field,
		"dimensions":      visual.Query.Dimensions,
		"measure":         measure,
		"measures":        visual.Query.Measures,
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

func canvasFilterVisual(x, y, width, height float64, children ...g.Node) g.Node {
	nodes := []g.Node{
		h.Class("canvas-visual canvas-filter-visual"),
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
		nodes = append(nodes, renderPageVisual(visual, report, filters, action))
	}
	return g.El("ld-report-canvas", nodes...)
}

func formatCanvasNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func renderPageVisual(visual dashboard.PageVisual, report semantic.Dashboard, filters dashboard.Filters, action string) g.Node {
	switch visual.Kind {
	case "header":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height, reportHeader(visual))
	case "kpi_strip":
		return canvasVisual(visual.X, visual.Y, visual.Width, visual.Height,
			h.Div(h.Class("kpi-band"),
				g.El("ld-kpi-strip", g.Attr("data-attr:items", "$kpis")),
			),
		)
	case "filter_card":
		return canvasFilterVisual(visual.X, visual.Y, visual.Width, visual.Height,
			filterCard(visual.Filter, report, filters, action),
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

func filterCard(filterID string, report semantic.Dashboard, filters dashboard.Filters, action string) g.Node {
	tableReset := tableResetExpression()
	return h.Article(h.Class("visual-card filter-visual-card"),
		g.El("ld-filter-card",
			g.Attr("filter-id", filterID),
			g.Attr("config", jsonString(report.Filters)),
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
	return h.Div(h.Class("filter-card-fallback"),
		h.Span(h.Class("filter-card-fallback-label"), g.Text(definition.Label)),
		h.Span(h.Class("filter-card-fallback-value"), g.Text(filterCardSummary(definition, control))),
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
	tableReset := tableResetExpression()
	return h.Div(h.Class("filters-pane"),
		g.El("ld-filter-panel",
			g.Attr("config", jsonString(report.Filters)),
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
	signal := "charts." + visualID
	return h.Article(h.Class("visual-card"),
		g.El("ld-echart",
			g.Attr("visual-id", visualID),
			g.Attr("data-attr:chart", "$"+signal),
			g.Attr("data-on:ld-chart-select", "$chartCommand = evt.detail; "+postAction("/commands/chart-select")),
			g.Attr("data-on:ld-chart-clear-selection", "$filters.visualSelections = []; "+postAction("/commands/clear-selection")),
		),
	)
}

func tablePanel(tableName string) g.Node {
	if tableName == "" {
		tableName = "orders"
	}
	return h.Section(h.Class("table-card"),
		g.El("ld-data-table",
			g.Attr("table-id", tableName),
			g.Attr("data-attr:table", "$tables."+tableName),
			g.Attr("data-on:ld-table-window-change", "$tableCommand = evt.detail; "+postAction("/commands/table-window")),
		),
	)
}
