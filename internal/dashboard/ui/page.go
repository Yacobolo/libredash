package ui

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/staticasset"
	uiactions "github.com/Yacobolo/libredash/internal/ui/actions"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/pkg/pagestream"

	"github.com/Yacobolo/libredash/internal/dashboard"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func updatesURL(workspaceID, dashboardID, pageID string) string {
	values := url.Values{}
	values.Set("route", string(uisignals.RouteDashboard))
	values.Set("workspace", workspaceID)
	values.Set("dashboard", dashboardID)
	values.Set("page", pageID)
	return "/updates?" + values.Encode()
}

func updatesURLWithParams(workspaceID, dashboardID, pageID string, params map[string]any) string {
	values := url.Values{}
	values.Set("route", string(uisignals.RouteDashboard))
	values.Set("workspace", workspaceID)
	values.Set("dashboard", dashboardID)
	values.Set("page", pageID)
	for key, raw := range params {
		switch typed := raw.(type) {
		case []string:
			for _, value := range typed {
				if strings.TrimSpace(value) != "" {
					values.Add(key, value)
				}
			}
		case string:
			if strings.TrimSpace(typed) != "" {
				values.Set(key, typed)
			}
		}
	}
	return "/updates?" + values.Encode()
}

func staticAsset(path string) string {
	return staticasset.URL(path)
}

func datastarScriptURL() string {
	return staticAsset(staticasset.DatastarScriptPath)
}

const (
	appRootClass = "min-h-svh bg-app text-fg-default"
)

type ChromeDecorator func(*uisignals.ChromeSignal)

func inspectorScript() g.Node {
	if staticasset.Production() {
		return nil
	}
	return h.Script(h.Type("module"), h.Src(staticAsset("/static/datastar-inspector.js")))
}

func inspectorElement() g.Node {
	if staticasset.Production() {
		return nil
	}
	return g.El("datastar-inspector", g.Attr("signals-url", "/__dev/pagestream/signals"))
}

func pageHead(extra ...g.Node) []g.Node {
	nodes := []g.Node{
		h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
		h.Script(h.Src(staticAsset("/static/theme.js"))),
		h.Script(h.Type("module"), h.Src(staticAsset("/static/command.js"))),
	}
	return append(nodes, extra...)
}

func csrfMeta(token string) g.Node {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return h.Meta(h.Name("csrf-token"), h.Content(token))
}

func Page(clientID, csrfToken string, catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters, chromeDecorators ...ChromeDecorator) g.Node {
	if activePage.ID == "" {
		activePage = defaultPage()
	}
	tableReset := tableResetExpression()
	initialFilters = report.NormalizeFiltersForPage(activePage.ID, initialFilters)
	initialURLParams := report.URLParamsFromFiltersForPage(activePage.ID, initialFilters)
	initialURLParams["streamInstance"] = newStreamInstanceID()
	dashboardUpdatesURL := updatesURLWithParams(catalog.Workspace.ID, report.ID, activePage.ID, initialURLParams)
	reloadAction := uiactions.Post("/workspaces/"+catalog.Workspace.ID+"/commands/reload", "runtime", "filters.controls")
	filtersUpdate := "$filters = evt.detail.filters; $urlParams = evt.detail.urlParams; window.DatastarURLSync && window.DatastarURLSync.replace($urlParams); " + tableReset
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:             "LibreDash",
		DatastarScriptURL: datastarScriptURL(),
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			csrfMeta(csrfToken),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/dashboard-page.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/url-sync.js"))),
			inspectorScript(),
		),
		MainAttrs: []g.Node{
			h.ID("dashboard"),
			h.Class(appRootClass),
			g.Attr("data-on:datastar-url-params-sync__window", "$urlParams = evt.detail.params; $filters = window.LibreDashFilterURL.fromParams($filterConfig, $filters, $urlParams); "+tableReset+reloadAction),
		},
		UpdatesURL: dashboardUpdatesURL,
		Body: []g.Node{
			g.El("ld-app-shell",
				g.El("ld-dashboard-page",
					g.Attr("slot", "page"),
					g.Attr("workspace-id", catalog.Workspace.ID),
					g.Attr("dashboard-id", report.ID),
					g.Attr("page-id", activePage.ID),
					g.Attr("data-on:ld-filters-change", filtersUpdate+reloadAction),
					g.Attr("data-on:ld-filters-reset", filtersUpdate+uiactions.Post("/workspaces/"+catalog.Workspace.ID+"/commands/reset-filters", "runtime")),
					g.Attr("data-on:ld-filters-refresh", reloadAction),
					g.Attr("data-on:ld-selection-clear", "$filters.selections = []; "+uiactions.Post("/workspaces/"+catalog.Workspace.ID+"/commands/clear-selection", "runtime")),
					g.Attr("data-on:ld-interaction-select", "$interactionCommand = evt.detail; "+uiactions.Post("/workspaces/"+catalog.Workspace.ID+"/commands/select", "runtime", "interactionCommand")),
					g.Attr("data-on:ld-table-window-change", "$tableCommand = evt.detail; "+uiactions.Post("/workspaces/"+catalog.Workspace.ID+"/commands/table-window", "runtime", "tableCommand")),
				),
			),
			inspectorElement(),
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

func BootstrapSignals(clientID, streamInstanceID string, catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters, chromeDecorators ...ChromeDecorator) map[string]any {
	envelope := uisignals.DashboardInitialEnvelope(clientID, streamInstanceID, catalog, report, model, pages, activePage, initialFilters)
	envelope.Runtime.WorkspaceID = uisignals.Optional(catalog.Workspace.ID)
	for _, decorate := range chromeDecorators {
		if decorate != nil {
			decorate(&envelope.Chrome)
		}
	}
	return map[string]any{
		"chrome":             envelope.Chrome,
		"componentStatus":    envelope.ComponentStatus,
		"page":               envelope.Page,
		"runtime":            envelope.Runtime,
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

func newStreamInstanceID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
}

func tableResetExpression() string {
	count := strconv.Itoa(dashboard.TableChunkSize)
	return "$tableCommand.block = 'all'; $tableCommand.start = 0; $tableCommand.count = " + count + "; $tableCommand.resetVersion = ($tableCommand.resetVersion || 0) + 1; "
}
