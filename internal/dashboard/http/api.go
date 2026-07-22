package http

import (
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/api"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	"github.com/Yacobolo/leapview/internal/dataquery"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	"github.com/go-chi/chi/v5"
)

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
	component, onPage := pageComponentForVisual(page, visualID)
	definition, exists := report.Visualizations[visualID]
	if !exists {
		writeJSONError(w, fmt.Errorf("visual %q not found", visualID), nethttp.StatusNotFound)
		return
	}
	if isGridQueryKind(definition.Query.Kind) {
		component, onPage = pageComponentForTable(page, visualID)
		if !onPage {
			writeJSONError(w, fmt.Errorf("visual %q not found on page %q", visualID, page.ID), nethttp.StatusNotFound)
			return
		}
	} else if !onPage {
		writeJSONError(w, fmt.Errorf("visual %q not found on page %q", visualID, page.ID), nethttp.StatusNotFound)
		return
	}
	writeJSON(w, nethttp.StatusOK, dashboardVisualizationDefinitionDTO(definition, component))
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
	if !dashboardFiltersProvided(filters) {
		filters = metrics.DefaultFilters(dashboardID)
	}
	pageID := chi.URLParam(r, "page")
	ctx := dataquery.WithMetadata(r.Context(), h.requestQueryMetadata(r, dataquery.SurfaceAPI, dataquery.OperationAPIQuery, "dashboard_page", dashboardID+":"+pageID))
	patch, err := metrics.QueryDashboardPage(ctx, dashboardID, pageID, filters)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	visuals := make(map[string]visualizationir.VisualizationEnvelope, len(patch.Visuals))
	for id, envelope := range patch.Visuals {
		visuals[id] = envelope
	}
	writeJSON(w, nethttp.StatusOK, map[string]any{"filters": patch.Filters, "filterOptions": patch.FilterOptions, "status": patch.Status, "visuals": visuals})
}

func (h Handler) QueryDashboardVisualData(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	var input api.DashboardVisualQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	report, page, ok := h.dashboardReportPage(w, r)
	if !ok {
		return
	}
	visualID := chi.URLParam(r, "visual")
	definition, exists := report.Visualizations[visualID]
	if !exists {
		writeJSONError(w, fmt.Errorf("visual %q not found", visualID), nethttp.StatusNotFound)
		return
	}
	onPage := false
	if !isGridQueryKind(definition.Query.Kind) {
		_, onPage = pageComponentForVisual(page, visualID)
	} else {
		_, onPage = pageComponentForTable(page, visualID)
	}
	if !onPage {
		writeJSONError(w, fmt.Errorf("visual %q not found on page %q", visualID, page.ID), nethttp.StatusNotFound)
		return
	}
	if isGridQueryKind(definition.Query.Kind) {
		h.queryDashboardTabularVisual(w, r, metrics, page, visualID, input)
		return
	}
	if input.Limit != 0 || input.PageToken != "" {
		writeJSONError(w, fmt.Errorf("pagination is only supported for table, matrix, and pivot visuals"), nethttp.StatusBadRequest)
		return
	}
	if acceptsDashboardMediaType(r.Header.Get("Accept"), dashboardArrowMediaType) {
		writeJSONError(w, fmt.Errorf("Arrow output is only supported for table, matrix, and pivot visuals"), nethttp.StatusNotAcceptable)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	filters := dashboardFilters(input.Filters)
	if !dashboardFiltersProvided(filters) {
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
	writeJSON(w, nethttp.StatusOK, visual)
}

func isGridQueryKind(kind visualizationdefinition.QueryKind) bool {
	return kind == visualizationdefinition.QueryDetail || kind == visualizationdefinition.QueryMatrix || kind == visualizationdefinition.QueryPivot
}

func (h Handler) queryDashboardTabularVisual(w nethttp.ResponseWriter, r *nethttp.Request, metrics Metrics, page dashboard.Page, visualID string, input api.DashboardVisualQueryRequest) {
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
	if !dashboardFiltersProvided(filters) {
		filters = metrics.DefaultFilters(dashboardID)
	}
	ctx := dataquery.WithMetadata(r.Context(), h.requestQueryMetadata(r, dataquery.SurfaceAPI, dataquery.OperationAPIQuery, "dashboard_visual", dashboardID+":"+visualID))
	definition, exists := metrics.VisualizationDefinition(dashboardID, visualID)
	if !exists {
		writeJSONError(w, fmt.Errorf("compiled visualization %q not found", visualID), nethttp.StatusInternalServerError)
		return
	}
	request := visualizationir.VisualizationWindowRequest{VisualID: visualID, SpecRevision: definition.SpecRevision, DataRevision: 1, BlockID: "a", Start: int64(start), Limit: int64(limit)}
	envelope, err := metrics.QueryVisualizationWindow(ctx, dashboardID, page.ID, filters, request)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	if !acceptsDashboardMediaType(r.Header.Get("Accept"), dashboardArrowMediaType) {
		writeJSON(w, nethttp.StatusOK, envelope)
		return
	}
	rowset, err := dashboardVisualizationRowset(envelope, request.BlockID, start, limit, scope, snapshot)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	writeDashboardTableRowset(w, r, rowset, envelope)
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
	if !dashboardFiltersProvided(filters) {
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

func (h Handler) dashboardReportPage(w nethttp.ResponseWriter, r *nethttp.Request) (dashboarddefinition.Definition, dashboard.Page, bool) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return dashboarddefinition.Definition{}, dashboard.Page{}, false
	}
	dashboardID := chi.URLParam(r, "dashboard")
	report, _, ok := metrics.Report(dashboardID)
	if !ok {
		writeJSONError(w, fmt.Errorf("dashboard %q not found", dashboardID), nethttp.StatusNotFound)
		return dashboarddefinition.Definition{}, dashboard.Page{}, false
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
	return dashboarddefinition.Definition{}, dashboard.Page{}, false
}

func (h Handler) requestQueryMetadata(r *nethttp.Request, surface, operation, objectType, objectID string) dataquery.Metadata {
	if surface == dataquery.SurfaceAPI && r.Header.Get("X-LeapView-Client") == dataquery.SurfaceCLI {
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
	case "dashboard_page", "dashboard_visual", "dashboard_filter":
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

func dashboardFiltersProvided(filters dashboard.Filters) bool {
	return filters.Controls != nil || filters.Selections != nil || filters.SpatialSelections != nil
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

func dashboardComponentDTO(component dashboard.PageVisual, report dashboarddefinition.Definition) api.DashboardComponentResponse {
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

func dashboardVisualizationDefinitionDTO(definition visualizationdefinition.Definition, component dashboard.PageVisual) api.DashboardVisualDescribeResponse {
	return api.DashboardVisualDescribeResponse{
		ID: definition.ID, ComponentID: component.ID, RendererID: definition.RendererID,
		SpecRevision: definition.SpecRevision, Spec: definition.Spec, Placement: componentPlacement(component),
		X: component.X, Y: component.Y, Width: component.Width, Height: component.Height,
	}
}

func modelSummary(model *semanticmodel.Model) *api.ModelRef {
	if model == nil {
		return nil
	}
	return &api.ModelRef{ID: model.Name, Title: model.Title}
}

func dashboardManifest(report dashboarddefinition.Definition, model *semanticmodel.Model, pages []dashboard.Page) api.DashboardManifestResponse {
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
			Visuals: len(report.Visualizations),
			Filters: len(report.Filters),
		},
		Pages: make([]api.DashboardManifestPage, 0, len(pages)),
		DetailTools: map[string]string{
			"model":       "describe_model",
			"page_data":   "query_dashboard_page",
			"visual_data": "query_dashboard_visual",
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

func dashboardComponentSummary(component dashboard.PageVisual, report dashboarddefinition.Definition) api.DashboardManifestComponent {
	switch {
	case component.Visual != "":
		title := component.Title
		if title == "" {
			title = dashboarddefinition.SpecTitle(report.Visualizations[component.Visual].Spec)
		}
		return api.DashboardManifestComponent{ID: component.ID, Kind: "visual", Ref: component.Visual, Title: title}
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
		if component.Kind == "table" && component.Visual == tableID {
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
