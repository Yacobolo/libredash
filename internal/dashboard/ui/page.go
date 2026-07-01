package ui

import (
	"encoding/json"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"strconv"

	"github.com/Yacobolo/libredash/internal/dashboard"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

func updateAction(workspaceID, dashboardID, pageID string) string {
	return "@get('/workspaces/" + workspaceID + "/updates?dashboard=" + dashboardID + "&page=" + pageID + "', {openWhenHidden: true})"
}

func postAction(path string) string {
	return "@post('" + path + "', {headers: {'X-CSRF-Token': $csrfToken}})"
}

func staticAsset(path string) string {
	return path + "?v=dev"
}

const (
	appRootClass = "min-h-svh bg-app text-fg-default"
)

type ChromeDecorator func(*uisignals.ChromeSignal)

func inspectorScript() g.Node {
	return h.Script(h.Type("module"), h.Src(staticAsset("/static/datastar-inspector.js")))
}

func inspectorElement() g.Node {
	return g.El("datastar-inspector")
}

func pageHead(extra ...g.Node) []g.Node {
	nodes := []g.Node{
		h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
		h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
		h.Script(h.Src(staticAsset("/static/theme.js"))),
	}
	return append(nodes, extra...)
}

func Page(dataDir, clientID, csrfToken string, catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters, chromeDecorators ...ChromeDecorator) g.Node {
	if activePage.ID == "" {
		activePage = defaultPage()
	}
	action := updateAction(catalog.Workspace.ID, report.ID, activePage.ID)
	initAction := "window.DatastarURLSync && window.DatastarURLSync.bindPopstate($urlParamShape); " + action
	tableReset := tableResetExpression()
	filtersUpdate := "$filters = evt.detail.filters; $urlParams = evt.detail.urlParams; window.DatastarURLSync && window.DatastarURLSync.replace($urlParams); " + tableReset
	initialFilters = report.NormalizeFiltersForPage(activePage.ID, initialFilters)
	signals := initialSignals(dataDir, clientID, csrfToken, catalog, report, model, pages, activePage, initialFilters, chromeDecorators...)
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash",
		Language: "en",
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
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		),
		Body: []g.Node{
			h.Main(
				h.ID("dashboard"),
				h.Class(appRootClass),
				ds.Signals(signals),
				ds.Init(initAction),
				g.Attr("data-on:datastar-url-params-sync__window", "$urlParams = evt.detail.params; $filters = window.LibreDashFilterURL.fromParams($filterConfig, $filters, $urlParams); "+tableReset+action),
				g.El("ld-app-shell",
					g.Attr("chrome", jsonString(signals["chrome"])),
					g.Attr("data-attr:chrome", "JSON.stringify($chrome)"),
					g.El("ld-dashboard-page",
						g.Attr("slot", "page"),
						g.Attr("page", jsonString(signals["page"])),
						g.Attr("filterconfig", jsonString(signals["filterConfig"])),
						g.Attr("filters", jsonString(signals["filters"])),
						g.Attr("filteroptions", jsonString(signals["filterOptions"])),
						g.Attr("visuals", jsonString(signals["visuals"])),
						g.Attr("tables", jsonString(signals["tables"])),
						g.Attr("status", jsonString(signals["status"])),
						g.Attr("data-attr:page", "JSON.stringify($page)"),
						g.Attr("data-attr:filterconfig", "JSON.stringify($filterConfig)"),
						g.Attr("data-attr:filters", "JSON.stringify($filters)"),
						g.Attr("data-attr:filteroptions", "JSON.stringify($filterOptions)"),
						g.Attr("data-attr:visuals", "JSON.stringify($visuals)"),
						g.Attr("data-attr:tables", "JSON.stringify($tables)"),
						g.Attr("data-attr:status", "JSON.stringify($status)"),
						g.Attr("data-on:ld-filters-change", filtersUpdate+action),
						g.Attr("data-on:ld-filters-reset", filtersUpdate+postAction("/workspaces/"+catalog.Workspace.ID+"/commands/reset-filters")),
						g.Attr("data-on:ld-filters-refresh", action),
						g.Attr("data-on:ld-selection-clear", "$filters.selections = []; "+postAction("/workspaces/"+catalog.Workspace.ID+"/commands/clear-selection")),
						g.Attr("data-on:ld-interaction-select", "$interactionCommand = evt.detail; "+postAction("/workspaces/"+catalog.Workspace.ID+"/commands/select")),
						g.Attr("data-on:ld-table-window-change", "$tableCommand = evt.detail; "+postAction("/workspaces/"+catalog.Workspace.ID+"/commands/table-window")),
						g.Attr("data-on:ld-refresh-materializations", postAction("/workspaces/"+catalog.Workspace.ID+"/commands/refresh-materializations?model="+model.Name+"&dashboard="+report.ID)),
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

func jsonString(value any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func initialSignals(dataDir, clientID, csrfToken string, catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters, chromeDecorators ...ChromeDecorator) map[string]any {
	envelope := uisignals.DashboardInitialEnvelope(dataDir, clientID, csrfToken, catalog, report, model, pages, activePage, initialFilters)
	for _, decorate := range chromeDecorators {
		if decorate != nil {
			decorate(&envelope.Chrome)
		}
	}
	return map[string]any{
		"chrome":             envelope.Chrome,
		"page":               envelope.Page,
		"runtime":            envelope.Runtime,
		"csrfToken":          envelope.CSRFToken,
		"filterConfig":       envelope.FilterConfig,
		"filters":            envelope.Filters,
		"urlParams":          envelope.URLParams,
		"urlParamShape":      envelope.URLParamShape,
		"filterOptions":      envelope.FilterOptions,
		"interactionCommand": envelope.InteractionCommand,
		"tableCommand":       envelope.TableCommand,
		"tables":             envelope.Tables,
		"visuals":            envelope.Visuals,
		"status":             envelope.Status,
	}
}

func tableResetExpression() string {
	count := strconv.Itoa(dashboard.TableChunkSize)
	return "$tableCommand.block = 'all'; $tableCommand.start = 0; $tableCommand.count = " + count + "; $tableCommand.resetVersion = ($tableCommand.resetVersion || 0) + 1; "
}
