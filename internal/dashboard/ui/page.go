package ui

import (
	"encoding/json"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"sort"
	"strconv"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
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

func postActionWithCSRFSignal(path, signal string) string {
	return "@post('" + path + "', {headers: {'X-CSRF-Token': " + signal + "}})"
}

func staticAsset(path string) string {
	return path + "?v=dev"
}

const (
	appRootClass             = "min-h-svh bg-app text-fg-default"
	appShellClass            = "grid min-h-svh grid-cols-app-shell bg-app max-sm:grid-cols-1"
	reportShellClass         = "grid min-h-svh grid-cols-report-shell bg-app max-sm:grid-cols-1"
	appMainClass             = "grid min-w-0 min-h-svh grid-rows-app-main bg-app"
	catalogMainClass         = appMainClass + " gap-3 px-4 py-4 max-sm:min-h-0 max-sm:p-3"
	reportMainClass          = appMainClass + " h-svh min-h-0 overflow-hidden"
	metricMainClass          = "grid h-svh min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-app"
	cardClass                = "grid min-h-min-card max-w-card grid-rows-card rounded-default border border-outline-variant bg-panel p-4 shadow-resting-sm"
	cardTitleClass           = "m-0 mt-1 text-body-md leading-snug font-semibold text-fg-default"
	cardDescriptionClass     = "m-0 mt-2 text-body-sm leading-relaxed font-normal text-fg-muted"
	cardFooterClass          = "mt-4 flex items-center justify-between gap-3 border-t border-outline-muted pt-3 text-caption font-medium text-fg-muted"
	eyebrowClass             = "m-0 mb-1 text-caption leading-tight font-medium uppercase text-fg-muted"
	visualCardClass          = "h-full min-h-0 w-full overflow-hidden rounded-default border border-outline-variant bg-panel"
	actionButtonClass        = "inline-flex size-action min-h-action items-center justify-center rounded-default border border-outline-variant bg-transparent p-0 text-fg-default hover:bg-control-hover focus-visible:bg-control-hover focus-visible:outline-0 disabled:cursor-not-allowed disabled:border-control-border-disabled disabled:bg-control-disabled disabled:text-control-fg-disabled"
	metricActionButtonClass  = "inline-flex size-8 items-center justify-center rounded-small border border-transparent bg-transparent p-0 text-fg-muted no-underline transition-colors duration-micro ease-hover hover:border-outline-muted hover:bg-control-hover hover:text-fg-default focus-visible:border-outline-accent focus-visible:bg-control-hover focus-visible:text-fg-default focus-visible:outline-0"
	primaryLinkButtonClass   = "inline-flex min-h-control-xs items-center justify-center gap-1.5 rounded-small bg-button-primary px-2.5 text-caption font-semibold text-on-primary no-underline hover:bg-button-primary-hover focus-visible:bg-button-primary-hover focus-visible:outline-0"
	tagClass                 = "rounded-full border border-outline-muted bg-panel-muted px-2 py-0.5 text-caption font-medium uppercase text-fg-muted"
	metricContentColumnClass = "grid min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
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

func Page(dataDir, clientID, csrfToken string, catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters) g.Node {
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
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sub-sidebar.js"))),
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

func metricTabCount(count int) g.Node {
	return h.Span(h.Class("inline-flex min-w-4 items-center justify-center rounded-full bg-panel-muted px-1.5 py-px text-caption font-medium leading-none text-fg-muted"), g.Text(strconv.Itoa(count)))
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

func metricGridBadgeValue(value, tone string) any {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return metricGridBadge{Label: value, Tone: tone}
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

func sidebar(config map[string]any) g.Node {
	return g.El("ld-sidebar", h.Class("border-r border-outline-variant max-sm:border-b max-sm:border-r-0"), g.Attr("config", jsonString(config)))
}

func sidebarConfigForCatalog(catalog dashboard.Catalog) map[string]any {
	modelID := ""
	modelTitle := ""
	if len(catalog.Models) > 0 {
		modelID = catalog.Models[0].ID
		modelTitle = catalog.Models[0].Title
	}
	return sidebarConfig(catalog, "dashboards", "", workspaceDisplayTitle(catalog), "Dashboards", "Discovery", modelID, modelTitle, false)
}

func sidebarConfigForReport(catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, activePage dashboard.Page) map[string]any {
	return sidebarConfig(catalog, "workspaces", report.ID, workspaceDisplayTitle(catalog), report.Title, activePage.Title, model.Name, model.Title, true)
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
				{"id": "chat", "label": "Chats", "href": "/chat", "icon": "chat", "meta": "Agent interface"},
				{"id": "workspaces", "label": "Workspaces", "href": "/workspaces", "icon": "catalog", "meta": "Published assets"},
				{"id": "connections", "label": "Connections", "href": "/connections", "icon": "data", "meta": "Data access"},
				{"id": "settings", "label": "Settings", "href": "/workspaces/" + workspaceID + "/permissions", "icon": "settings", "meta": "Permissions"},
			},
		},
	}
}

func workspaceDisplayTitle(catalog dashboard.Catalog) string {
	if strings.TrimSpace(catalog.Workspace.Title) != "" {
		return catalog.Workspace.Title
	}
	return "LibreDash Workspace"
}

func reportSidebar(config map[string]any) g.Node {
	return g.El("ld-sub-sidebar", h.Class("border-l border-outline-variant max-sm:border-l-0 max-sm:border-t"), g.Attr("config", jsonString(config)))
}

func reportSidebarConfig(report reportdef.Dashboard, _ *semanticmodel.Model, pages []dashboard.Page, activePage dashboard.Page) map[string]any {
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
		"label":      "Pages",
		"railLabel":  "Pages",
		"ariaLabel":  "Report pages",
		"storageKey": "libredash-report-sidebar-collapsed",
		"activeId":   activePage.ID,
		"items":      items,
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

func initialSignals(dataDir, clientID, csrfToken string, report reportdef.Dashboard, model *semanticmodel.Model, activePage dashboard.Page, initialFilters dashboard.Filters) map[string]any {
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
		"interactionCommand": map[string]any{
			"sourceKind":      "",
			"sourceId":        "",
			"interactionKind": "",
			"action":          "",
			"toggle":          true,
			"mappings":        []any{},
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

func defaultTableRequest(report reportdef.Dashboard, page dashboard.Page) dashboard.TableRequest {
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

func tableSignals(report reportdef.Dashboard, page dashboard.Page, request dashboard.TableRequest) map[string]any {
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
			"interaction":   interactionSignal("row_selection", table.Interaction.RowSelection),
			"selection":     []any{},
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

func visualSignals(report reportdef.Dashboard, model *semanticmodel.Model, page dashboard.Page) map[string]any {
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

func visualSignal(id string, visual reportdef.Visual, title, unit, format, measure string) map[string]any {
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
		"interaction":     interactionSignal("point_selection", visual.Interaction.PointSelection),
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

func interactionSignal(kind string, selection reportdef.SelectionInteraction) map[string]any {
	mappings := make([]map[string]any, 0, len(selection.Mappings))
	for _, mapping := range selection.Mappings {
		mappings = append(mappings, map[string]any{
			"field": mapping.Field,
			"value": mapping.Value,
			"label": mapping.Label,
		})
	}
	return map[string]any{
		"kind":     kind,
		"toggle":   selection.Toggle,
		"mappings": mappings,
		"targets":  append([]string{}, selection.Targets...),
	}
}

func displayFieldRefs(refs []reportdef.FieldRef) []string {
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

func renderPageCanvas(page dashboard.Page, report reportdef.Dashboard, filters dashboard.Filters, action string) g.Node {
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

func renderPageVisual(pageID string, visual dashboard.PageVisual, report reportdef.Dashboard, filters dashboard.Filters, action string) g.Node {
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

func filterCard(pageID, filterID string, report reportdef.Dashboard, filters dashboard.Filters, action string) g.Node {
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

func filterCardFallback(filterID string, report reportdef.Dashboard, filters dashboard.Filters) g.Node {
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

func filterCardSummary(definition reportdef.FilterDefinition, control dashboard.FilterControl) string {
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

func filtersDock(report reportdef.Dashboard, pageID string, action string) g.Node {
	return h.Details(h.Class("group grid min-h-0 w-full border-l border-outline-variant bg-panel-muted transition-[width,background-color] duration-short ease-move sm:w-filter-closed"), h.Aria("label", "Report filters"), g.Attr("data-filter-dock", ""),
		h.Summary(h.Class("flex min-h-control-xl cursor-pointer list-none items-center justify-center gap-2 border-b border-outline-variant px-2 text-caption font-medium uppercase text-fg-muted marker:hidden transition-colors duration-micro ease-hover hover:text-fg-default focus-visible:text-fg-default focus-visible:outline-0 sm:flex sm:h-full sm:w-filter-closed sm:flex-col sm:justify-start sm:border-b-0 sm:px-0 sm:py-4"), h.Title("Toggle filters"), g.Attr("data-filter-summary", ""),
			lucide.SlidersHorizontal(filterDockIconAttrs()...),
			h.Span(h.Class("sm:[writing-mode:vertical-rl]"), g.Text("Filters")),
			h.Span(h.Class("sr-only"), g.Text("Toggle filters")),
		),
		filtersPane(report, pageID, action),
	)
}

func filtersPane(report reportdef.Dashboard, pageID string, action string) g.Node {
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
			g.Attr("data-on:ld-selection-clear", "$filters.selections = []; "+postAction("/commands/clear-selection")),
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
			g.Attr("data-on:ld-interaction-select", "$interactionCommand = evt.detail; "+postAction("/commands/select")),
			g.Attr("data-on:ld-selection-clear", "$filters.selections = []; "+postAction("/commands/clear-selection")),
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
			g.Attr("data-on:ld-interaction-select", "$interactionCommand = evt.detail; "+postAction("/commands/select")),
			g.Attr("data-on:ld-selection-clear", "$filters.selections = []; "+postAction("/commands/clear-selection")),
		),
	)
}
