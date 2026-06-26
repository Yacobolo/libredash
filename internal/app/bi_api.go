package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/go-chi/chi/v5"
)

const maxAgentRows = 50

func (s *Server) listDashboards(w http.ResponseWriter, r *http.Request) {
	catalog := s.metrics.Catalog()
	out := make([]api.DashboardSummary, 0, len(catalog.Dashboards))
	for _, row := range catalog.Dashboards {
		out = append(out, dashboardSummaryDTO(row))
	}
	page, nextCursor, ok := pageSliceForRequest(w, r, out)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, api.DashboardListResponse{Items: page, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (s *Server) getDashboard(w http.ResponseWriter, r *http.Request) {
	dashboardID := chi.URLParam(r, "dashboard")
	report, model, ok := s.metrics.Report(dashboardID)
	if !ok {
		writeJSONError(w, fmt.Errorf("dashboard %q not found", dashboardID), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, dashboardManifest(report, model, s.metrics.Pages(dashboardID)))
}

func (s *Server) listDashboardComponents(w http.ResponseWriter, r *http.Request) {
	report, page, ok := s.dashboardReportPage(w, r)
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
	writeJSON(w, http.StatusOK, api.DashboardComponentListResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (s *Server) getDashboardVisual(w http.ResponseWriter, r *http.Request) {
	report, page, ok := s.dashboardReportPage(w, r)
	if !ok {
		return
	}
	visualID := chi.URLParam(r, "visual")
	visual, exists := report.Visuals[visualID]
	if !exists {
		writeJSONError(w, fmt.Errorf("visual %q not found", visualID), http.StatusNotFound)
		return
	}
	component, ok := pageComponentForVisual(page, visualID)
	if !ok {
		writeJSONError(w, fmt.Errorf("visual %q not found on page %q", visualID, page.ID), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, dashboardVisualDTO(visualID, visual, component))
}

func (s *Server) listSemanticModels(w http.ResponseWriter, r *http.Request) {
	catalog := s.metrics.Catalog()
	out := make([]api.SemanticModelSummary, 0, len(catalog.Models))
	for _, row := range catalog.Models {
		out = append(out, semanticModelSummaryDTO(row))
	}
	page, nextCursor, ok := pageSliceForRequest(w, r, out)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, api.SemanticModelListResponse{Items: page, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (s *Server) getSemanticModel(w http.ResponseWriter, r *http.Request) {
	modelID := chi.URLParam(r, "model")
	model, ok := modelDescription(s.metrics, modelID)
	if !ok {
		writeJSONError(w, fmt.Errorf("model %q not found", modelID), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, model)
}

func (s *Server) listSemanticDatasets(w http.ResponseWriter, r *http.Request) {
	model, ok := s.semanticModelForRequest(w, r)
	if !ok {
		return
	}
	out := make([]api.SemanticDatasetSummary, 0, len(model.Tables))
	for _, datasetID := range sortedMapKeys(model.Tables) {
		table := model.Tables[datasetID]
		out = append(out, api.SemanticDatasetSummary{
			ID:           datasetID,
			Kind:         table.Kind,
			Source:       table.Source,
			Description:  table.Description,
			FieldCount:   len(table.Dimensions),
			MeasureCount: semanticDatasetMeasureCount(model, datasetID),
		})
	}
	items, nextCursor, ok := pageSliceForRequest(w, r, out)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, api.SemanticDatasetListResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (s *Server) getSemanticDataset(w http.ResponseWriter, r *http.Request) {
	model, table, datasetID, ok := s.semanticDatasetForRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, semanticDatasetDTO(model, datasetID, table))
}

func (s *Server) listSemanticFields(w http.ResponseWriter, r *http.Request) {
	model, table, datasetID, ok := s.semanticDatasetForRequest(w, r)
	if !ok {
		return
	}
	fields := semanticDatasetFields(model, datasetID, table)
	items, nextCursor, ok := pageSliceForRequest(w, r, fields)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, api.SemanticFieldListResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (s *Server) querySemanticDataset(w http.ResponseWriter, r *http.Request) {
	var input api.SemanticQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	modelID, datasetID := chi.URLParam(r, "model"), chi.URLParam(r, "dataset")
	if _, _, _, ok := s.semanticDatasetForRequest(w, r); !ok {
		return
	}
	request, limit, err := semanticAggregateRequest(datasetID, input, true)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	plan, err := semanticExplainAggregate(s.metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	rows, err := s.metrics.QuerySemantic(r.Context(), modelID, request)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, semanticQueryResponse(plan.Columns, rows, limit, request.Offset))
}

func (s *Server) previewSemanticDataset(w http.ResponseWriter, r *http.Request) {
	var input api.SemanticPreviewRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	modelID, datasetID := chi.URLParam(r, "model"), chi.URLParam(r, "dataset")
	if _, _, _, ok := s.semanticDatasetForRequest(w, r); !ok {
		return
	}
	request, limit, err := semanticRowRequest(datasetID, input, true)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	plan, err := semanticExplainRows(s.metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	rows, err := s.metrics.PreviewSemantic(r.Context(), modelID, request)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, semanticQueryResponse(plan.Columns, rows, limit, request.Offset))
}

func (s *Server) explainSemanticQuery(w http.ResponseWriter, r *http.Request) {
	var input api.SemanticQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	modelID, datasetID := chi.URLParam(r, "model"), chi.URLParam(r, "dataset")
	if _, _, _, ok := s.semanticDatasetForRequest(w, r); !ok {
		return
	}
	request, _, err := semanticAggregateRequest(datasetID, input, false)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	plan, err := semanticExplainAggregate(s.metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, semanticExplainResponse("query", plan, semanticQueryWarnings(input.Sort)))
}

func (s *Server) explainSemanticPreview(w http.ResponseWriter, r *http.Request) {
	var input api.SemanticPreviewRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	modelID, datasetID := chi.URLParam(r, "model"), chi.URLParam(r, "dataset")
	if _, _, _, ok := s.semanticDatasetForRequest(w, r); !ok {
		return
	}
	request, _, err := semanticRowRequest(datasetID, input, false)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	plan, err := semanticExplainRows(s.metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, semanticExplainResponse("preview", plan, semanticQueryWarnings(input.Sort)))
}

func (s *Server) queryDashboardPage(w http.ResponseWriter, r *http.Request) {
	var input api.DashboardPageQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	filters := dashboardFilters(input.Filters)
	if filters.Controls == nil && filters.Selections == nil {
		filters = s.metrics.DefaultFilters(dashboardID)
	}
	patch, err := s.metrics.QueryDashboardPage(r.Context(), dashboardID, chi.URLParam(r, "page"), filters)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, boundedPatch(patch))
}

func (s *Server) queryDashboardTable(w http.ResponseWriter, r *http.Request) {
	var input api.DashboardTableQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	count := input.Count
	if count <= 0 || count > maxAgentRows {
		count = maxAgentRows
	}
	filters := dashboardFilters(input.Filters)
	if filters.Controls == nil && filters.Selections == nil {
		filters = s.metrics.DefaultFilters(dashboardID)
	}
	request := s.metrics.NormalizeTableRequest(dashboardID, dashboard.TableRequest{Table: chi.URLParam(r, "table"), Block: "a", Count: count})
	request.Count = count
	table, err := s.metrics.QueryTablePage(r.Context(), dashboardID, input.PageID, filters, request)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, boundedTable(table))
}

func (s *Server) queryDashboardVisualData(w http.ResponseWriter, r *http.Request) {
	var input api.DashboardPageQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	report, page, ok := s.dashboardReportPage(w, r)
	if !ok {
		return
	}
	visualID := chi.URLParam(r, "visual")
	if _, exists := report.Visuals[visualID]; !exists {
		writeJSONError(w, fmt.Errorf("visual %q not found", visualID), http.StatusNotFound)
		return
	}
	if _, ok := pageComponentForVisual(page, visualID); !ok {
		writeJSONError(w, fmt.Errorf("visual %q not found on page %q", visualID, page.ID), http.StatusNotFound)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	filters := dashboardFilters(input.Filters)
	if filters.Controls == nil && filters.Selections == nil {
		filters = s.metrics.DefaultFilters(dashboardID)
	}
	patch, err := s.metrics.QueryDashboardPage(r.Context(), dashboardID, page.ID, filters)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	visual, ok := patch.Visuals[visualID]
	if !ok {
		writeJSONError(w, fmt.Errorf("visual %q data not found", visualID), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, boundedVisual(visual))
}

func (s *Server) queryDashboardTableData(w http.ResponseWriter, r *http.Request) {
	var input api.DashboardTableDataRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	report, page, ok := s.dashboardReportPage(w, r)
	if !ok {
		return
	}
	tableID := chi.URLParam(r, "table")
	if _, exists := report.Tables[tableID]; !exists {
		writeJSONError(w, fmt.Errorf("table %q not found", tableID), http.StatusNotFound)
		return
	}
	if _, ok := pageComponentForTable(page, tableID); !ok {
		writeJSONError(w, fmt.Errorf("table %q not found on page %q", tableID, page.ID), http.StatusNotFound)
		return
	}
	count := input.Count
	if count <= 0 || count > maxAgentRows {
		count = maxAgentRows
	}
	dashboardID := chi.URLParam(r, "dashboard")
	filters := dashboardFilters(input.Filters)
	if filters.Controls == nil && filters.Selections == nil {
		filters = s.metrics.DefaultFilters(dashboardID)
	}
	request := s.metrics.NormalizeTableRequest(dashboardID, dashboard.TableRequest{Table: tableID, Block: "a", Count: count})
	request.Count = count
	table, err := s.metrics.QueryTablePage(r.Context(), dashboardID, page.ID, filters, request)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, boundedTable(table))
}

func (s *Server) listDashboardFilterOptions(w http.ResponseWriter, r *http.Request) {
	var input api.DashboardPageQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	report, page, ok := s.dashboardReportPage(w, r)
	if !ok {
		return
	}
	filterID := chi.URLParam(r, "filter")
	if _, exists := report.Filters[filterID]; !exists {
		writeJSONError(w, fmt.Errorf("filter %q not found", filterID), http.StatusNotFound)
		return
	}
	if _, ok := pageComponentForFilter(page, filterID); !ok {
		writeJSONError(w, fmt.Errorf("filter %q not found on page %q", filterID, page.ID), http.StatusNotFound)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	filters := dashboardFilters(input.Filters)
	if filters.Controls == nil && filters.Selections == nil {
		filters = s.metrics.DefaultFilters(dashboardID)
	}
	patch, err := s.metrics.QueryDashboardPage(r.Context(), dashboardID, page.ID, filters)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
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
	writeJSON(w, http.StatusOK, api.DashboardFilterOptionListResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
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
	if len(visual.Data) > maxAgentRows {
		visual.Data = visual.Data[:maxAgentRows]
	}
	return visual
}

func boundedTable(table dashboard.Table) dashboard.Table {
	for key, block := range table.Blocks {
		if len(block.Rows) > maxAgentRows {
			block.Rows = block.Rows[:maxAgentRows]
		}
		table.Blocks[key] = block
	}
	if table.AvailableRows > maxAgentRows {
		table.AvailableRows = maxAgentRows
	}
	return table
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

func semanticModelSummaryDTO(row dashboard.CatalogModel) api.SemanticModelSummary {
	return api.SemanticModelSummary{ID: row.ID, Title: row.Title, Description: row.Description}
}

func (s *Server) semanticModelForRequest(w http.ResponseWriter, r *http.Request) (*semanticmodel.Model, bool) {
	modelID := chi.URLParam(r, "model")
	model := semanticModelForID(s.metrics, modelID)
	if model == nil {
		writeJSONError(w, fmt.Errorf("model %q not found", modelID), http.StatusNotFound)
		return nil, false
	}
	return model, true
}

func (s *Server) semanticDatasetForRequest(w http.ResponseWriter, r *http.Request) (*semanticmodel.Model, semanticmodel.Table, string, bool) {
	model, ok := s.semanticModelForRequest(w, r)
	if !ok {
		return nil, semanticmodel.Table{}, "", false
	}
	datasetID := chi.URLParam(r, "dataset")
	table, exists := model.Tables[datasetID]
	if !exists {
		writeJSONError(w, fmt.Errorf("dataset %q not found", datasetID), http.StatusNotFound)
		return nil, semanticmodel.Table{}, "", false
	}
	return model, table, datasetID, true
}

func semanticDatasetDTO(model *semanticmodel.Model, datasetID string, table semanticmodel.Table) api.SemanticDatasetResponse {
	sources := append([]string{}, table.Sources...)
	if table.Source != "" && len(sources) == 0 {
		sources = []string{table.Source}
	}
	sort.Strings(sources)
	return api.SemanticDatasetResponse{
		ID:           datasetID,
		Kind:         table.Kind,
		Source:       table.Source,
		Sources:      sources,
		Description:  table.Description,
		PrimaryKey:   table.PrimaryKey,
		Grain:        table.Grain,
		FieldCount:   len(table.Dimensions),
		MeasureCount: semanticDatasetMeasureCount(model, datasetID),
	}
}

func semanticDatasetMeasureCount(model *semanticmodel.Model, datasetID string) int {
	if model == nil {
		return 0
	}
	count := 0
	if table, ok := model.Tables[datasetID]; ok {
		count += len(table.Measures)
	}
	for _, measure := range model.Measures {
		if measure.Table == datasetID {
			count++
		}
	}
	return count
}

func semanticDatasetFields(model *semanticmodel.Model, datasetID string, table semanticmodel.Table) []api.SemanticFieldResponse {
	out := make([]api.SemanticFieldResponse, 0, len(table.Dimensions)+semanticDatasetMeasureCount(model, datasetID))
	for _, fieldID := range sortedMapKeys(table.Dimensions) {
		dimension := table.Dimensions[fieldID]
		out = append(out, api.SemanticFieldResponse{
			ID:          datasetID + "." + fieldID,
			Kind:        "dimension",
			Table:       datasetID,
			Name:        fieldID,
			Label:       dimension.Label,
			Description: dimension.Description,
		})
	}
	for _, measureID := range sortedMapKeys(table.Measures) {
		measure := table.Measures[measureID]
		out = append(out, semanticMeasureFieldDTO(datasetID+"."+measureID, datasetID, measureID, measure))
	}
	for _, measureID := range sortedMapKeys(model.Measures) {
		measure := model.Measures[measureID]
		if measure.Table != datasetID {
			continue
		}
		out = append(out, semanticMeasureFieldDTO(measureID, datasetID, measureID, measure))
	}
	return out
}

func semanticMeasureFieldDTO(id, datasetID, name string, measure semanticmodel.MetricMeasure) api.SemanticFieldResponse {
	return api.SemanticFieldResponse{
		ID:          id,
		Kind:        "measure",
		Table:       datasetID,
		Name:        name,
		Label:       measure.Label,
		Description: measure.Description,
		Unit:        measure.Unit,
		Format:      measure.Format,
		Grain:       measure.Grain,
		Time:        measure.Time,
		Grains:      append([]string{}, measure.Grains...),
	}
}

func semanticAggregateRequest(datasetID string, input api.SemanticQueryRequest, includeExtraRow bool) (reportdef.AggregateQuery, int, error) {
	limit, offset, err := semanticLimitAndOffset(input.Limit, input.PageToken)
	if err != nil {
		return reportdef.AggregateQuery{}, 0, err
	}
	requestLimit := limit
	if includeExtraRow {
		requestLimit++
	}
	request := reportdef.AggregateQuery{
		Table:      datasetID,
		Dimensions: semanticQueryFields(input.Dimensions),
		Measures:   semanticQueryFields(input.Measures),
		Filters:    semanticFilters(input.Filters),
		Sort:       semanticSorts(input.Sort),
		Limit:      requestLimit,
		Offset:     offset,
	}
	if input.Time != nil {
		request.Time = reportdef.QueryTime{Field: input.Time.Field, Grain: input.Time.Grain, Alias: input.Time.Alias}
	}
	return request, limit, nil
}

func semanticRowRequest(datasetID string, input api.SemanticPreviewRequest, includeExtraRow bool) (reportdef.RowQuery, int, error) {
	limit, offset, err := semanticLimitAndOffset(input.Limit, input.PageToken)
	if err != nil {
		return reportdef.RowQuery{}, 0, err
	}
	requestLimit := limit
	if includeExtraRow {
		requestLimit++
	}
	return reportdef.RowQuery{
		Table:      datasetID,
		Dimensions: semanticQueryFields(input.Dimensions),
		Measures:   semanticQueryFields(input.Measures),
		Filters:    semanticFilters(input.Filters),
		Sort:       semanticSorts(input.Sort),
		Limit:      requestLimit,
		Offset:     offset,
	}, limit, nil
}

func semanticLimitAndOffset(limitValue int, pageToken string) (int, int, error) {
	limit := limitValue
	if limit <= 0 {
		limit = defaultAPILimit
	}
	if limit > maxAPILimit {
		limit = maxAPILimit
	}
	offset, err := decodeIndexCursor(pageToken)
	if err != nil {
		return 0, 0, err
	}
	return limit, offset, nil
}

func semanticQueryFields(fields []api.SemanticFieldRef) []reportdef.QueryField {
	out := make([]reportdef.QueryField, 0, len(fields))
	for _, field := range fields {
		out = append(out, reportdef.QueryField{Field: field.Field, Alias: field.Alias})
	}
	return out
}

func semanticExplainAggregate(metrics queryMetrics, modelID string, request reportdef.AggregateQuery) (semanticquery.Plan, error) {
	model := semanticModelForID(metrics, modelID)
	if model == nil {
		return semanticquery.Plan{}, fmt.Errorf("unknown semantic model %q", modelID)
	}
	return semanticquery.NewPlanner(model).Plan(reportdef.SemanticAggregateRequest(request))
}

func semanticExplainRows(metrics queryMetrics, modelID string, request reportdef.RowQuery) (semanticquery.Plan, error) {
	model := semanticModelForID(metrics, modelID)
	if model == nil {
		return semanticquery.Plan{}, fmt.Errorf("unknown semantic model %q", modelID)
	}
	return semanticquery.NewPlanner(model).PlanRows(reportdef.SemanticRowRequest(request))
}

func semanticFilters(filters []api.SemanticFilter) []reportdef.QueryFilter {
	out := make([]reportdef.QueryFilter, 0, len(filters))
	for _, filter := range filters {
		out = append(out, reportdef.QueryFilter{
			Field:    filter.Field,
			Operator: filter.Operator,
			Values:   append([]any{}, filter.Values...),
			Groups:   semanticFilterGroups(filter.Groups),
		})
	}
	return out
}

func semanticFilterGroups(groups []api.SemanticFilterGroup) []reportdef.QueryFilterGroup {
	out := make([]reportdef.QueryFilterGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, reportdef.QueryFilterGroup{Filters: semanticFilters(group.Filters)})
	}
	return out
}

func semanticSorts(sorts []api.SemanticSort) []reportdef.QuerySort {
	out := make([]reportdef.QuerySort, 0, len(sorts))
	for _, sortSpec := range sorts {
		out = append(out, reportdef.QuerySort{Field: sortSpec.Field, Direction: sortSpec.Direction})
	}
	return out
}

func semanticQueryResponse(columns []string, rows reportdef.QueryRows, limit, offset int) api.SemanticQueryResponse {
	items := make([]map[string]any, 0, min(len(rows), limit))
	for i, row := range rows {
		if i >= limit {
			break
		}
		item := make(map[string]any, len(row))
		for key, value := range row {
			item[key] = value
		}
		items = append(items, item)
	}
	nextCursor := ""
	if len(rows) > limit {
		nextCursor = encodeIndexCursor(offset + limit)
	}
	return api.SemanticQueryResponse{Columns: columns, Items: items, Page: api.PageInfo{NextCursor: nextCursor}}
}

func semanticExplainResponse(mode string, plan semanticquery.Plan, warnings []string) api.SemanticExplainResponse {
	return api.SemanticExplainResponse{
		Mode:     mode,
		SQL:      plan.SQL,
		Args:     semanticExplainArgs(plan.Args),
		Columns:  append([]string{}, plan.Columns...),
		Warnings: warnings,
	}
}

func semanticExplainArgs(args []any) []map[string]any {
	out := make([]map[string]any, 0, len(args))
	for i, value := range args {
		out = append(out, map[string]any{"index": i + 1, "value": value})
	}
	return out
}

func semanticQueryWarnings(sorts []api.SemanticSort) []string {
	if len(sorts) == 0 {
		return []string{"result order is not stable without sort"}
	}
	return nil
}

func sortedMapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *Server) dashboardReportPage(w http.ResponseWriter, r *http.Request) (reportdef.Dashboard, dashboard.Page, bool) {
	dashboardID := chi.URLParam(r, "dashboard")
	report, _, ok := s.metrics.Report(dashboardID)
	if !ok {
		writeJSONError(w, fmt.Errorf("dashboard %q not found", dashboardID), http.StatusNotFound)
		return reportdef.Dashboard{}, dashboard.Page{}, false
	}
	pageID := chi.URLParam(r, "page")
	pages := s.metrics.Pages(dashboardID)
	if pages == nil {
		pages = report.Pages
	}
	for _, page := range pages {
		if page.ID == pageID {
			return report, page.WithDefaults(), true
		}
	}
	writeJSONError(w, fmt.Errorf("page %q not found", pageID), http.StatusNotFound)
	return reportdef.Dashboard{}, dashboard.Page{}, false
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
	return out
}

func dashboardVisualDTO(visualID string, visual reportdef.Visual, component dashboard.PageVisual) api.DashboardVisualDescribeResponse {
	out := api.DashboardVisualDescribeResponse{
		ID:              visualID,
		ComponentID:     component.ID,
		Kind:            firstNonEmpty(visual.Kind, component.Kind),
		Shape:           visual.Shape,
		Renderer:        visual.Renderer,
		Type:            visual.Type,
		Title:           firstNonEmpty(component.Title, visual.Title),
		Description:     firstNonEmpty(component.Description, visual.Description),
		Query:           jsonMap(visual.Query),
		Options:         visual.Options,
		RendererOptions: visual.RendererOptions,
		Interaction:     jsonMap(visual.Interaction),
		X:               component.X,
		Y:               component.Y,
		Width:           component.Width,
		Height:          component.Height,
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

func modelSummary(model *semanticmodel.Model) *api.ModelRef {
	if model == nil {
		return nil
	}
	return &api.ModelRef{ID: model.Name, Title: model.Title}
}

func modelDescription(metrics queryMetrics, id string) (api.SemanticModelDescriptionResponse, bool) {
	catalog := metrics.Catalog()
	var catalogModel dashboard.CatalogModel
	for _, model := range catalog.Models {
		if model.ID == id {
			catalogModel = model
			break
		}
	}
	if catalogModel.ID == "" {
		return api.SemanticModelDescriptionResponse{}, false
	}

	out := api.SemanticModelDescriptionResponse{
		ID:          catalogModel.ID,
		Title:       catalogModel.Title,
		Description: catalogModel.Description,
		Dashboards:  dashboardsForModel(metrics, id),
	}
	if model := semanticModelForID(metrics, id); model != nil {
		fieldCount := 0
		for _, table := range model.Tables {
			fieldCount += len(table.Dimensions)
		}
		out.Counts = &api.SemanticModelCounts{
			Sources:       len(model.Sources),
			ModelTables:   len(model.Tables),
			Fields:        fieldCount,
			Measures:      len(model.Measures),
			Relationships: len(model.Relationships),
		}
		tables := make([]api.SemanticModelTableSummary, 0, len(model.Tables))
		for tableID, table := range model.Tables {
			tables = append(tables, api.SemanticModelTableSummary{
				ID:          tableID,
				Kind:        table.Kind,
				Source:      table.Source,
				Description: table.Description,
				Fields:      len(table.Dimensions),
			})
		}
		out.Tables = tables
	}
	return out, true
}

func dashboardsForModel(metrics queryMetrics, modelID string) []api.ModelDashboardUsage {
	out := make([]api.ModelDashboardUsage, 0)
	for _, dashboardSummary := range metrics.Catalog().Dashboards {
		report, model, ok := metrics.Report(dashboardSummary.ID)
		if !ok || (report.SemanticModel != modelID && (model == nil || model.Name != modelID)) {
			continue
		}
		out = append(out, api.ModelDashboardUsage{
			ID:            report.ID,
			Title:         report.Title,
			SemanticModel: report.SemanticModel,
			Pages:         len(metrics.Pages(report.ID)),
		})
	}
	return out
}

func semanticModelForID(metrics queryMetrics, modelID string) *semanticmodel.Model {
	if model, ok := metrics.SemanticModel(modelID); ok {
		return model
	}
	for _, dashboardSummary := range metrics.Catalog().Dashboards {
		_, model, ok := metrics.Report(dashboardSummary.ID)
		if ok && model != nil && model.Name == modelID {
			return model
		}
	}
	return nil
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
