package http

import (
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/go-chi/chi/v5"
)

const maxAPIVisualDatums = 1000

func (h Handler) ListDashboards(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	catalog := metrics.Catalog()
	out := make([]api.DashboardSummary, 0, len(catalog.Dashboards))
	for _, row := range catalog.Dashboards {
		out = append(out, dashboardSummaryDTO(row))
	}
	workspaceID := chi.URLParam(r, "workspace")
	if workspaceID == "" {
		workspaceID = catalog.Workspace.ID
	}
	principalID := ""
	if h.CurrentPrincipalID != nil {
		principalID = h.CurrentPrincipalID(r)
	}
	out, err := h.filterAuthorizedDashboards(r.Context(), principalID, workspaceID, out)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	page, nextCursor, ok := pageSliceForRequest(w, r, out)
	if !ok {
		return
	}
	writeJSON(w, nethttp.StatusOK, api.DashboardListResponse{Items: page, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (h Handler) GetDashboard(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	report, model, ok := metrics.Report(dashboardID)
	if !ok {
		writeJSONError(w, fmt.Errorf("dashboard %q not found", dashboardID), nethttp.StatusNotFound)
		return
	}
	writeJSON(w, nethttp.StatusOK, dashboardManifest(report, model, metrics.Pages(dashboardID)))
}

func (h Handler) ListDashboardComponents(w nethttp.ResponseWriter, r *nethttp.Request) {
	report, page, ok := h.dashboardReportPage(w, r)
	if !ok {
		return
	}
	out := make([]api.DashboardComponentResponse, 0, len(page.Visuals))
	for _, component := range page.PlacedVisuals() {
		out = append(out, dashboardComponentDTO(component, report))
	}
	items, nextCursor, ok := pageSliceForRequest(w, r, out)
	if !ok {
		return
	}
	writeJSON(w, nethttp.StatusOK, api.DashboardComponentListResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (h Handler) GetDashboardPage(w nethttp.ResponseWriter, r *nethttp.Request) {
	report, page, ok := h.dashboardReportPage(w, r)
	if !ok {
		return
	}
	components := make([]api.DashboardComponentResponse, 0, len(page.Visuals))
	for _, component := range page.PlacedVisuals() {
		components = append(components, dashboardComponentDTO(component, report))
	}
	writeJSON(w, nethttp.StatusOK, api.DashboardPageResponse{
		ID: page.ID, Title: page.Title, Description: page.Description, Components: components,
	})
}

func (h Handler) GetDashboardTable(w nethttp.ResponseWriter, r *nethttp.Request) {
	report, page, ok := h.dashboardReportPage(w, r)
	if !ok {
		return
	}
	tableID := chi.URLParam(r, "table")
	table, exists := report.Tables[tableID]
	if !exists {
		writeJSONError(w, fmt.Errorf("table %q not found", tableID), nethttp.StatusNotFound)
		return
	}
	component, exists := pageComponentForTable(page, tableID)
	if !exists {
		writeJSONError(w, fmt.Errorf("table %q not found on page %q", tableID, page.ID), nethttp.StatusNotFound)
		return
	}
	columns := make([]api.DashboardTableColumn, 0, len(table.Columns))
	for _, column := range table.Columns {
		columns = append(columns, api.DashboardTableColumn{Key: column.Key, Label: column.Label})
	}
	writeJSON(w, nethttp.StatusOK, api.DashboardTableDescribeResponse{
		ID: tableID, ComponentID: component.ID, Title: firstNonEmpty(component.Title, table.Title),
		Description: firstNonEmpty(component.Description, table.Description), Columns: columns,
		Query: jsonMap(table.Query), Placement: componentPlacement(component),
	})
}

func (h Handler) GetDashboardFilter(w nethttp.ResponseWriter, r *nethttp.Request) {
	report, page, ok := h.dashboardReportPage(w, r)
	if !ok {
		return
	}
	filterID := chi.URLParam(r, "filter")
	filter, exists := report.Filters[filterID]
	if !exists {
		writeJSONError(w, fmt.Errorf("filter %q not found", filterID), nethttp.StatusNotFound)
		return
	}
	component, exists := pageComponentForFilter(page, filterID)
	if !exists {
		writeJSONError(w, fmt.Errorf("filter %q not found on page %q", filterID, page.ID), nethttp.StatusNotFound)
		return
	}
	multiSelect := filter.Type == "multi_select"
	writeJSON(w, nethttp.StatusOK, api.DashboardFilterDescribeResponse{
		ID: filterID, ComponentID: component.ID, Title: firstNonEmpty(component.Title, filter.Label),
		Description: firstNonEmpty(component.Description, filter.Description), Field: filter.Dimension,
		MultiSelect: multiSelect, Placement: componentPlacement(component),
	})
}

func (h Handler) GetDashboardVisual(w nethttp.ResponseWriter, r *nethttp.Request) {
	report, page, ok := h.dashboardReportPage(w, r)
	if !ok {
		return
	}
	visualID := chi.URLParam(r, "visual")
	visual, exists := report.Visuals[visualID]
	if !exists {
		writeJSONError(w, fmt.Errorf("visual %q not found", visualID), nethttp.StatusNotFound)
		return
	}
	component, ok := pageComponentForVisual(page, visualID)
	if !ok {
		writeJSONError(w, fmt.Errorf("visual %q not found on page %q", visualID, page.ID), nethttp.StatusNotFound)
		return
	}
	writeJSON(w, nethttp.StatusOK, dashboardVisualDTO(visualID, visual, component))
}

func (h Handler) QueryDashboardPage(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	var input api.DashboardPageQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	filters := dashboardFilters(input.Filters)
	if filters.Controls == nil && filters.Selections == nil {
		filters = metrics.DefaultFilters(dashboardID)
	}
	pageID := chi.URLParam(r, "page")
	ctx := dataquery.WithMetadata(r.Context(), h.requestQueryMetadata(r, dataquery.SurfaceAPI, dataquery.OperationAPIQuery, "dashboard_page", dashboardID+":"+pageID))
	patch, err := metrics.QueryDashboardPage(ctx, dashboardID, pageID, filters)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	writeJSON(w, nethttp.StatusOK, publicDashboardPatch(patch))
}

func (h Handler) QueryDashboardVisualData(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	var input api.DashboardPageQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	report, page, ok := h.dashboardReportPage(w, r)
	if !ok {
		return
	}
	visualID := chi.URLParam(r, "visual")
	if _, exists := report.Visuals[visualID]; !exists {
		writeJSONError(w, fmt.Errorf("visual %q not found", visualID), nethttp.StatusNotFound)
		return
	}
	if _, ok := pageComponentForVisual(page, visualID); !ok {
		writeJSONError(w, fmt.Errorf("visual %q not found on page %q", visualID, page.ID), nethttp.StatusNotFound)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	filters := dashboardFilters(input.Filters)
	if filters.Controls == nil && filters.Selections == nil {
		filters = metrics.DefaultFilters(dashboardID)
	}
	ctx := dataquery.WithMetadata(r.Context(), h.requestQueryMetadata(r, dataquery.SurfaceAPI, dataquery.OperationAPIQuery, "dashboard_visual", dashboardID+":"+visualID))
	patch, err := metrics.QueryDashboardPage(ctx, dashboardID, page.ID, filters)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	visual, ok := patch.Visuals[visualID]
	if !ok {
		writeJSONError(w, fmt.Errorf("visual %q data not found", visualID), nethttp.StatusNotFound)
		return
	}
	writeJSON(w, nethttp.StatusOK, publicDashboardVisual(boundedVisual(visual)))
}

func (h Handler) QueryDashboardTableData(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	var input api.DashboardTableDataRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	report, page, ok := h.dashboardReportPage(w, r)
	if !ok {
		return
	}
	tableID := chi.URLParam(r, "table")
	if _, exists := report.Tables[tableID]; !exists {
		writeJSONError(w, fmt.Errorf("table %q not found", tableID), nethttp.StatusNotFound)
		return
	}
	if _, ok := pageComponentForTable(page, tableID); !ok {
		writeJSONError(w, fmt.Errorf("table %q not found on page %q", tableID, page.ID), nethttp.StatusNotFound)
		return
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > dashboard.TableMaxRequestCount {
		limit = dashboard.TableMaxRequestCount
	}
	cursorInput := input
	cursorInput.PageToken = ""
	scope, snapshot := dashboardRequestCursorScope(r, cursorInput), dashboardServingSnapshot(r)
	start, err := decodeIndexCursor(input.PageToken, scope, snapshot)
	if err != nil {
		status := nethttp.StatusBadRequest
		if errors.Is(err, errDashboardCursorSnapshot) {
			status = nethttp.StatusConflict
		}
		writeJSONError(w, err, status)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	filters := dashboardFilters(input.Filters)
	if filters.Controls == nil && filters.Selections == nil {
		filters = metrics.DefaultFilters(dashboardID)
	}
	request := metrics.NormalizeTableRequest(dashboardID, dashboard.TableRequest{Table: tableID, Block: "a", Start: start, Count: limit})
	request.Start, request.Count = start, limit
	ctx := dataquery.WithMetadata(r.Context(), h.requestQueryMetadata(r, dataquery.SurfaceAPI, dataquery.OperationAPIQuery, "dashboard_table", dashboardID+":"+tableID))
	table, err := metrics.QueryTablePage(ctx, dashboardID, page.ID, filters, request)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	writeDashboardTableRowset(w, r, dashboardTableRowset(table, request.Block, start, limit, scope, snapshot))
}

func (h Handler) ListDashboardFilterOptions(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	var input api.DashboardPageQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	report, page, ok := h.dashboardReportPage(w, r)
	if !ok {
		return
	}
	filterID := chi.URLParam(r, "filter")
	if _, exists := report.Filters[filterID]; !exists {
		writeJSONError(w, fmt.Errorf("filter %q not found", filterID), nethttp.StatusNotFound)
		return
	}
	if _, ok := pageComponentForFilter(page, filterID); !ok {
		writeJSONError(w, fmt.Errorf("filter %q not found on page %q", filterID, page.ID), nethttp.StatusNotFound)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	filters := dashboardFilters(input.Filters)
	if filters.Controls == nil && filters.Selections == nil {
		filters = metrics.DefaultFilters(dashboardID)
	}
	ctx := dataquery.WithMetadata(r.Context(), h.requestQueryMetadata(r, dataquery.SurfaceAPI, dataquery.OperationAPIQuery, "dashboard_filter", dashboardID+":"+filterID))
	patch, err := metrics.QueryDashboardPage(ctx, dashboardID, page.ID, filters)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	options := patch.FilterOptions[filterID]
	out := make([]api.DashboardFilterOptionResponse, 0, len(options))
	for _, option := range options {
		out = append(out, api.DashboardFilterOptionResponse{Value: option.Value, Label: option.Label})
	}
	items, nextCursor, ok := pageSliceForRequest(w, r, out)
	if !ok {
		return
	}
	writeJSON(w, nethttp.StatusOK, api.DashboardFilterOptionListResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (h Handler) biMetrics(w nethttp.ResponseWriter, r *nethttp.Request) (Metrics, bool) {
	metrics, ok := h.metricsForRequest(r)
	if !ok {
		writeJSONError(w, fmt.Errorf("workspace %q not found", chi.URLParam(r, "workspace")), nethttp.StatusNotFound)
		return nil, false
	}
	return metrics, true
}

func (h Handler) dashboardReportPage(w nethttp.ResponseWriter, r *nethttp.Request) (reportdef.Dashboard, dashboard.Page, bool) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return reportdef.Dashboard{}, dashboard.Page{}, false
	}
	dashboardID := chi.URLParam(r, "dashboard")
	report, _, ok := metrics.Report(dashboardID)
	if !ok {
		writeJSONError(w, fmt.Errorf("dashboard %q not found", dashboardID), nethttp.StatusNotFound)
		return reportdef.Dashboard{}, dashboard.Page{}, false
	}
	pageID := chi.URLParam(r, "page")
	pages := metrics.Pages(dashboardID)
	if pages == nil {
		pages = report.Pages
	}
	for _, page := range pages {
		if page.ID == pageID {
			return report, page.WithDefaults(), true
		}
	}
	writeJSONError(w, fmt.Errorf("page %q not found", pageID), nethttp.StatusNotFound)
	return reportdef.Dashboard{}, dashboard.Page{}, false
}

func (h Handler) requestQueryMetadata(r *nethttp.Request, surface, operation, objectType, objectID string) dataquery.Metadata {
	if surface == dataquery.SurfaceAPI && r.Header.Get("X-LibreDash-Client") == dataquery.SurfaceCLI {
		surface = dataquery.SurfaceCLI
	}
	metadata := dataquery.Metadata{
		WorkspaceID:   chi.URLParam(r, "workspace"),
		Surface:       surface,
		Operation:     requestQueryOperation(operation, objectType),
		ObjectType:    objectType,
		ObjectID:      objectID,
		RequestID:     r.Header.Get("X-Request-ID"),
		CorrelationID: r.Header.Get("X-Correlation-ID"),
	}
	if h.CurrentPrincipalID != nil {
		metadata.PrincipalID = h.CurrentPrincipalID(r)
	}
	existing := dataquery.MetadataFromContext(r.Context())
	if existing.WorkspaceID != "" {
		metadata.WorkspaceID = existing.WorkspaceID
	}
	if existing.Surface != "" {
		metadata.Surface = existing.Surface
	}
	if existing.Operation != "" {
		metadata.Operation = existing.Operation
	}
	if existing.PrincipalID != "" {
		metadata.PrincipalID = existing.PrincipalID
	}
	if existing.RequestID != "" {
		metadata.RequestID = existing.RequestID
	}
	if existing.ObjectType != "" {
		metadata.ObjectType = existing.ObjectType
	}
	if existing.ObjectID != "" {
		metadata.ObjectID = existing.ObjectID
	}
	if existing.CorrelationID != "" {
		metadata.CorrelationID = existing.CorrelationID
	}
	return metadata
}

func requestQueryOperation(operation, objectType string) string {
	if operation != dataquery.OperationAPIQuery {
		return operation
	}
	switch objectType {
	case "dashboard_page", "dashboard_table", "dashboard_visual", "dashboard_filter":
		return ""
	default:
		return operation
	}
}

func dashboardFilters(raw map[string]any) dashboard.Filters {
	if len(raw) == 0 {
		return dashboard.Filters{}
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return dashboard.Filters{}
	}
	var filters dashboard.Filters
	if err := json.Unmarshal(bytes, &filters); err != nil {
		return dashboard.Filters{}
	}
	return filters
}

func boundedPatch(patch dashboard.Patch) dashboard.Patch {
	for key, visual := range patch.Visuals {
		patch.Visuals[key] = boundedVisual(visual)
	}
	return patch
}

func boundedVisual(visual dashboard.Visual) dashboard.Visual {
	if len(visual.Data) > maxAPIVisualDatums {
		visual.Data = visual.Data[:maxAPIVisualDatums]
	}
	return visual
}

func publicDashboardPatch(patch dashboard.Patch) map[string]any {
	visuals := make(map[string]any, len(patch.Visuals))
	for key, visual := range patch.Visuals {
		visuals[key] = publicDashboardVisual(boundedVisual(visual))
	}
	return map[string]any{"filters": patch.Filters, "filterOptions": patch.FilterOptions, "status": patch.Status, "visuals": visuals}
}

func publicDashboardVisual(visual dashboard.Visual) map[string]any {
	extensions := map[string]map[string]any{}
	if len(visual.Options) > 0 {
		extensions["libredash"] = visual.Options
	}
	for namespace, options := range visual.RendererOptions {
		extensions[namespace] = options
	}
	data := make([]map[string]any, 0, len(visual.Data))
	known := map[string]struct{}{
		"label": {}, "series": {}, "value": {}, "selected": {}, "start": {}, "end": {}, "positive": {},
		"binStart": {}, "binEnd": {}, "path": {}, "row": {}, "column": {}, "source": {}, "target": {},
		"name": {}, "open": {}, "close": {}, "low": {}, "high": {}, "min": {}, "q1": {}, "median": {}, "q3": {}, "max": {},
	}
	for _, datum := range visual.Data {
		item, plugin := map[string]any{}, map[string]any{}
		for key, value := range datum {
			if _, ok := known[key]; ok || key == "extensions" {
				item[key] = value
			} else {
				plugin[key] = value
			}
		}
		if len(plugin) > 0 {
			item["extensions"] = map[string]any{visual.Renderer: plugin}
		}
		data = append(data, item)
	}
	out := map[string]any{
		"version": visual.Version, "id": visual.ID, "kind": visual.Kind, "shape": visual.Shape, "renderer": visual.Renderer,
		"type": visual.Type, "title": visual.Title, "unit": visual.Unit, "format": visual.Format, "interaction": visual.Interaction,
		"dimensions": visual.Dimensions, "measure": visual.Measure, "measures": visual.Measures, "series": visual.Series,
		"selection": visual.Selection, "data": data,
	}
	if len(extensions) > 0 {
		out["extensions"] = extensions
	}
	return out
}

func dashboardSummaryDTO(row dashboard.CatalogDashboard) api.DashboardSummary {
	return api.DashboardSummary{
		ID:            row.ID,
		Title:         row.Title,
		Description:   row.Description,
		SemanticModel: row.SemanticModel,
		Tags:          row.Tags,
		PageCount:     row.PageCount,
	}
}

func dashboardComponentDTO(component dashboard.PageVisual, report reportdef.Dashboard) api.DashboardComponentResponse {
	summary := dashboardComponentSummary(component, report)
	out := api.DashboardComponentResponse{
		ID:          component.ID,
		Kind:        summary.Kind,
		Ref:         summary.Ref,
		Title:       summary.Title,
		Description: component.Description,
		X:           component.X,
		Y:           component.Y,
		Width:       component.Width,
		Height:      component.Height,
	}
	if !component.Placement.IsZero() {
		out.Placement = &api.DashboardComponentPlacement{
			Col:     component.Placement.Col,
			Row:     component.Placement.Row,
			ColSpan: component.Placement.ColSpan,
			RowSpan: component.Placement.RowSpan,
		}
	}
	switch out.Kind {
	case "visual":
		out.VisualID = out.Ref
	case "table":
		out.TableID = out.Ref
	case "filter":
		out.FilterID = out.Ref
	}
	return out
}

func componentPlacement(component dashboard.PageVisual) *api.DashboardComponentPlacement {
	if component.Placement.IsZero() {
		return nil
	}
	return &api.DashboardComponentPlacement{
		Col: component.Placement.Col, Row: component.Placement.Row,
		ColSpan: component.Placement.ColSpan, RowSpan: component.Placement.RowSpan,
	}
}

func dashboardVisualDTO(visualID string, visual reportdef.Visual, component dashboard.PageVisual) api.DashboardVisualDescribeResponse {
	out := api.DashboardVisualDescribeResponse{
		ID:          visualID,
		ComponentID: component.ID,
		Kind:        firstNonEmpty(visual.Kind, component.Kind),
		Shape:       visual.Shape,
		Renderer:    visual.Renderer,
		Type:        visual.Type,
		Title:       firstNonEmpty(component.Title, visual.Title),
		Description: firstNonEmpty(component.Description, visual.Description),
		Query:       jsonMap(visual.Query),
		Interaction: jsonMap(visual.Interaction),
		X:           component.X,
		Y:           component.Y,
		Width:       component.Width,
		Height:      component.Height,
	}
	if len(visual.Options) > 0 || len(visual.RendererOptions) > 0 {
		out.Extensions = map[string]map[string]any{}
		if len(visual.Options) > 0 {
			out.Extensions["libredash"] = visual.Options
		}
		if len(visual.RendererOptions) > 0 {
			out.Extensions[firstNonEmpty(visual.Renderer, "plugin")] = visual.RendererOptions
		}
	}
	if !component.Placement.IsZero() {
		out.Placement = &api.DashboardComponentPlacement{
			Col:     component.Placement.Col,
			Row:     component.Placement.Row,
			ColSpan: component.Placement.ColSpan,
			RowSpan: component.Placement.RowSpan,
		}
	}
	return out
}

func modelSummary(model *semanticmodel.Model) *api.ModelRef {
	if model == nil {
		return nil
	}
	return &api.ModelRef{ID: model.Name, Title: model.Title}
}

func dashboardManifest(report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page) api.DashboardManifestResponse {
	if pages == nil {
		pages = report.Pages
	}
	out := api.DashboardManifestResponse{
		ID:            report.ID,
		Title:         report.Title,
		Description:   report.Description,
		SemanticModel: report.SemanticModel,
		Model:         modelSummary(model),
		Counts: api.DashboardManifestCounts{
			Pages:   len(pages),
			Visuals: len(report.Visuals),
			Tables:  len(report.Tables),
			Filters: len(report.Filters),
		},
		Pages: make([]api.DashboardManifestPage, 0, len(pages)),
		DetailTools: map[string]string{
			"model":      "describe_model",
			"page_data":  "query_dashboard_page",
			"table_data": "query_table",
		},
	}
	for _, page := range pages {
		pageSummary := api.DashboardManifestPage{
			ID:          page.ID,
			Title:       page.Title,
			Description: page.Description,
			Components:  make([]api.DashboardManifestComponent, 0, len(page.Visuals)),
		}
		for _, component := range page.Visuals {
			pageSummary.Components = append(pageSummary.Components, dashboardComponentSummary(component, report))
		}
		out.Pages = append(out.Pages, pageSummary)
	}
	return out
}

func dashboardComponentSummary(component dashboard.PageVisual, report reportdef.Dashboard) api.DashboardManifestComponent {
	switch {
	case component.Visual != "":
		title := component.Title
		if title == "" {
			title = report.Visuals[component.Visual].Title
		}
		return api.DashboardManifestComponent{ID: component.ID, Kind: "visual", Ref: component.Visual, Title: title}
	case component.Table != "":
		title := component.Title
		if title == "" {
			title = report.Tables[component.Table].Title
		}
		return api.DashboardManifestComponent{ID: component.ID, Kind: "table", Ref: component.Table, Title: title}
	case component.Filter != "":
		title := component.Title
		if title == "" {
			title = report.Filters[component.Filter].Label
		}
		return api.DashboardManifestComponent{ID: component.ID, Kind: "filter", Ref: component.Filter, Title: title}
	default:
		kind := component.Kind
		if kind == "" {
			kind = "component"
		}
		return api.DashboardManifestComponent{ID: component.ID, Kind: kind, Title: component.Title}
	}
}

func pageComponentForVisual(page dashboard.Page, visualID string) (dashboard.PageVisual, bool) {
	for _, component := range page.PlacedVisuals() {
		if component.Visual == visualID {
			return component, true
		}
	}
	return dashboard.PageVisual{}, false
}

func pageComponentForTable(page dashboard.Page, tableID string) (dashboard.PageVisual, bool) {
	for _, component := range page.PlacedVisuals() {
		if component.Table == tableID {
			return component, true
		}
	}
	return dashboard.PageVisual{}, false
}

func pageComponentForFilter(page dashboard.Page, filterID string) (dashboard.PageVisual, bool) {
	for _, component := range page.PlacedVisuals() {
		if component.Filter == filterID {
			return component, true
		}
	}
	return dashboard.PageVisual{}, false
}

func jsonMap(value any) map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}
