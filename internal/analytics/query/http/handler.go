package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	queryauthz "github.com/Yacobolo/libredash/internal/analytics/query/authz"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/go-chi/chi/v5"
)

type Metrics interface {
	Catalog() dashboard.Catalog
	ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error)
	Pages(dashboardID string) []dashboard.Page
	Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool)
	SemanticModel(modelID string) (*semanticmodel.Model, bool)
}

type Handler struct {
	Metrics             Metrics
	MetricsForWorkspace func(workspaceID string) (Metrics, bool)
	CurrentPrincipalID  func(r *nethttp.Request) string
}

func SemanticDatasetObjectRefs(r *nethttp.Request, workspaceID string) []access.ObjectRef {
	objects := []access.ObjectRef{}
	modelID := strings.TrimSpace(chi.URLParam(r, "model"))
	if modelID != "" {
		model := access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, modelID, access.WorkspaceObject(workspaceID))
		if datasetID := strings.TrimSpace(chi.URLParam(r, "dataset")); datasetID != "" {
			objects = append(objects, access.ItemObjectWithParent(access.SecurableDataset, workspaceID, modelID+"/"+datasetID, model))
		}
		objects = append(objects, model)
	}
	if strings.TrimSpace(workspaceID) != "" {
		objects = append(objects, access.WorkspaceObject(workspaceID))
	}
	return objects
}

func (h Handler) ListSemanticModels(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	catalog := metrics.Catalog()
	out := make([]api.SemanticModelSummary, 0, len(catalog.Models))
	for _, row := range catalog.Models {
		out = append(out, semanticModelSummaryDTO(row))
	}
	page, nextCursor, ok := pageSliceForRequest(w, r, out)
	if !ok {
		return
	}
	writeJSON(w, nethttp.StatusOK, api.SemanticModelListResponse{Items: page, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (h Handler) GetSemanticModel(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	modelID := chi.URLParam(r, "model")
	model, ok := modelDescription(metrics, modelID)
	if !ok {
		writeJSONError(w, fmt.Errorf("model %q not found", modelID), nethttp.StatusNotFound)
		return
	}
	writeJSON(w, nethttp.StatusOK, model)
}

func (h Handler) ListSemanticDatasets(w nethttp.ResponseWriter, r *nethttp.Request) {
	model, ok := h.semanticModelForRequest(w, r)
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
	writeJSON(w, nethttp.StatusOK, api.SemanticDatasetListResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (h Handler) GetSemanticDataset(w nethttp.ResponseWriter, r *nethttp.Request) {
	model, table, datasetID, ok := h.semanticDatasetForRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, nethttp.StatusOK, semanticDatasetDTO(model, datasetID, table))
}

func (h Handler) ListSemanticFields(w nethttp.ResponseWriter, r *nethttp.Request) {
	model, table, datasetID, ok := h.semanticDatasetForRequest(w, r)
	if !ok {
		return
	}
	fields := semanticDatasetFields(model, datasetID, table)
	items, nextCursor, ok := pageSliceForRequest(w, r, fields)
	if !ok {
		return
	}
	writeJSON(w, nethttp.StatusOK, api.SemanticFieldListResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (h Handler) QuerySemanticDataset(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	var input api.SemanticQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	modelID, datasetID := chi.URLParam(r, "model"), chi.URLParam(r, "dataset")
	if _, _, _, ok := h.semanticDatasetForRequest(w, r); !ok {
		return
	}
	request, limit, err := semanticAggregateRequest(datasetID, input, true)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	plan, err := semanticExplainAggregate(metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	ctx := dataquery.WithMetadata(r.Context(), h.requestQueryMetadata(r, dataquery.SurfaceAPI, dataquery.OperationAPIQuery, "semantic_dataset", modelID+":"+datasetID))
	rows, err := executeAggregateRows(ctx, metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, statusForDataExecutionError(err))
		return
	}
	writeJSON(w, nethttp.StatusOK, semanticQueryResponse(plan.Columns, rows, limit, request.Offset))
}

func (h Handler) PreviewSemanticDataset(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	var input api.SemanticPreviewRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	modelID, datasetID := chi.URLParam(r, "model"), chi.URLParam(r, "dataset")
	if _, _, _, ok := h.semanticDatasetForRequest(w, r); !ok {
		return
	}
	request, limit, err := semanticRowRequest(datasetID, input, true)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	plan, err := semanticExplainRows(metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	ctx := dataquery.WithMetadata(r.Context(), h.requestQueryMetadata(r, dataquery.SurfaceAPI, dataquery.OperationAPIPreview, "semantic_dataset", modelID+":"+datasetID))
	rows, err := executePreviewRows(ctx, metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, statusForDataExecutionError(err))
		return
	}
	writeJSON(w, nethttp.StatusOK, semanticQueryResponse(plan.Columns, rows, limit, request.Offset))
}

func (h Handler) ExplainSemanticQuery(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	var input api.SemanticQueryRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	modelID, datasetID := chi.URLParam(r, "model"), chi.URLParam(r, "dataset")
	if _, _, _, ok := h.semanticDatasetForRequest(w, r); !ok {
		return
	}
	request, _, err := semanticAggregateRequest(datasetID, input, false)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	plan, err := semanticExplainAggregate(metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	writeJSON(w, nethttp.StatusOK, semanticExplainResponse("query", plan, semanticQueryWarnings(input.Sort)))
}

func (h Handler) ExplainSemanticPreview(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return
	}
	var input api.SemanticPreviewRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	modelID, datasetID := chi.URLParam(r, "model"), chi.URLParam(r, "dataset")
	if _, _, _, ok := h.semanticDatasetForRequest(w, r); !ok {
		return
	}
	request, _, err := semanticRowRequest(datasetID, input, false)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	plan, err := semanticExplainRows(metrics, modelID, request)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	writeJSON(w, nethttp.StatusOK, semanticExplainResponse("preview", plan, semanticQueryWarnings(input.Sort)))
}

func (h Handler) biMetrics(w nethttp.ResponseWriter, r *nethttp.Request) (Metrics, bool) {
	metrics, ok := h.metricsForRequest(r)
	if !ok {
		writeJSONError(w, fmt.Errorf("workspace %q not found", chi.URLParam(r, "workspace")), nethttp.StatusNotFound)
		return nil, false
	}
	return metrics, true
}

func (h Handler) metricsForRequest(r *nethttp.Request) (Metrics, bool) {
	workspaceID := chi.URLParam(r, "workspace")
	if workspaceID != "" && h.MetricsForWorkspace != nil {
		return h.MetricsForWorkspace(workspaceID)
	}
	if h.Metrics == nil {
		return nil, false
	}
	return h.Metrics, true
}

func (h Handler) semanticModelForRequest(w nethttp.ResponseWriter, r *nethttp.Request) (*semanticmodel.Model, bool) {
	metrics, ok := h.biMetrics(w, r)
	if !ok {
		return nil, false
	}
	modelID := chi.URLParam(r, "model")
	model := semanticModelForID(metrics, modelID)
	if model == nil {
		writeJSONError(w, fmt.Errorf("model %q not found", modelID), nethttp.StatusNotFound)
		return nil, false
	}
	return model, true
}

func (h Handler) semanticDatasetForRequest(w nethttp.ResponseWriter, r *nethttp.Request) (*semanticmodel.Model, semanticmodel.Table, string, bool) {
	model, ok := h.semanticModelForRequest(w, r)
	if !ok {
		return nil, semanticmodel.Table{}, "", false
	}
	datasetID := chi.URLParam(r, "dataset")
	table, exists := model.Tables[datasetID]
	if !exists {
		writeJSONError(w, fmt.Errorf("dataset %q not found", datasetID), nethttp.StatusNotFound)
		return nil, semanticmodel.Table{}, "", false
	}
	return model, table, datasetID, true
}

func semanticModelSummaryDTO(row dashboard.CatalogModel) api.SemanticModelSummary {
	return api.SemanticModelSummary{ID: row.ID, Title: row.Title, Description: row.Description}
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

func modelDescription(metrics Metrics, id string) (api.SemanticModelDescriptionResponse, bool) {
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

func dashboardsForModel(metrics Metrics, modelID string) []api.ModelDashboardUsage {
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

func semanticModelForID(metrics Metrics, modelID string) *semanticmodel.Model {
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

func semanticExplainAggregate(metrics Metrics, modelID string, request reportdef.AggregateQuery) (semanticquery.Plan, error) {
	model := semanticModelForID(metrics, modelID)
	if model == nil {
		return semanticquery.Plan{}, fmt.Errorf("unknown semantic model %q", modelID)
	}
	return semanticquery.NewPlanner(model).Plan(reportdef.SemanticAggregateRequest(request))
}

func semanticExplainRows(metrics Metrics, modelID string, request reportdef.RowQuery) (semanticquery.Plan, error) {
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

func executeAggregateRows(ctx context.Context, metrics Metrics, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	result, err := metrics.ExecuteDataQuery(ctx, dataquery.Query{
		ModelID:  modelID,
		Kind:     dataquery.KindSemanticAggregate,
		Target:   request.Table,
		Fields:   queryFieldsToDataFields(request.Dimensions),
		Measures: queryFieldsToDataFields(request.Measures),
		Time:     dataquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
		Filters:  queryFiltersToDataFilters(request.Filters),
		Sort:     querySortToDataSort(request.Sort),
		Limit:    request.Limit,
		Offset:   request.Offset,
	})
	return queryRowsFromDataResult(result.Rows), err
}

func executePreviewRows(ctx context.Context, metrics Metrics, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	result, err := metrics.ExecuteDataQuery(ctx, dataquery.Query{
		ModelID:  modelID,
		Kind:     dataquery.KindSemanticRows,
		Target:   request.Table,
		Fields:   queryFieldsToDataFields(request.Dimensions),
		Measures: queryFieldsToDataFields(request.Measures),
		Filters:  queryFiltersToDataFilters(request.Filters),
		Sort:     querySortToDataSort(request.Sort),
		Limit:    request.Limit,
		Offset:   request.Offset,
	})
	return queryRowsFromDataResult(result.Rows), err
}

func statusForDataExecutionError(err error) int {
	if err == nil {
		return nethttp.StatusOK
	}
	if queryauthz.IsDenied(err) {
		return nethttp.StatusForbidden
	}
	return nethttp.StatusBadRequest
}

func queryFieldsToDataFields(fields []reportdef.QueryField) []dataquery.Field {
	out := make([]dataquery.Field, 0, len(fields))
	for _, field := range fields {
		out = append(out, dataquery.Field{
			Field: field.Field,
			Alias: field.Alias,
			Measure: dataquery.InlineMeasure{
				Field:       field.Measure.Field,
				Name:        field.Measure.Name,
				Label:       field.Measure.Label,
				Description: field.Measure.Description,
				Expr:        field.Measure.Expr,
				Expression:  field.Measure.Expression,
				Table:       field.Measure.Table,
				Grain:       field.Measure.Grain,
				Time:        field.Measure.Time,
				Grains:      append([]string{}, field.Measure.Grains...),
				Unit:        field.Measure.Unit,
				Format:      field.Measure.Format,
			},
		})
	}
	return out
}

func queryFiltersToDataFilters(filters []reportdef.QueryFilter) []dataquery.Filter {
	out := make([]dataquery.Filter, 0, len(filters))
	for _, filter := range filters {
		groups := make([]dataquery.FilterGroup, 0, len(filter.Groups))
		for _, group := range filter.Groups {
			groups = append(groups, dataquery.FilterGroup{Filters: queryFiltersToDataFilters(group.Filters)})
		}
		out = append(out, dataquery.Filter{
			Field:    filter.Field,
			Operator: filter.Operator,
			Values:   append([]any{}, filter.Values...),
			Groups:   groups,
		})
	}
	return out
}

func querySortToDataSort(sort []reportdef.QuerySort) []dataquery.Sort {
	out := make([]dataquery.Sort, 0, len(sort))
	for _, item := range sort {
		out = append(out, dataquery.Sort{Field: item.Field, Direction: item.Direction})
	}
	return out
}

func queryRowsFromDataResult(rows []dataquery.Row) reportdef.QueryRows {
	out := make(reportdef.QueryRows, 0, len(rows))
	for _, row := range rows {
		converted := reportdef.QueryRow{}
		for key, value := range row {
			converted[key] = value
		}
		out = append(out, converted)
	}
	return out
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

func sortedMapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func pageSliceForRequest[T any](w nethttp.ResponseWriter, r *nethttp.Request, items []T) ([]T, string, bool) {
	limit, ok := apiLimitForRequest(w, r)
	if !ok {
		return nil, "", false
	}
	start, ok := apiCursorOffsetForRequest(w, r)
	if !ok {
		return nil, "", false
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	nextCursor := ""
	if end < len(items) {
		nextCursor = encodeIndexCursor(end)
	}
	return append([]T(nil), items[start:end]...), nextCursor, true
}

const (
	defaultAPILimit = 50
	maxAPILimit     = 100
)

func apiLimitForRequest(w nethttp.ResponseWriter, r *nethttp.Request) (int, bool) {
	limit, err := parseAPILimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return 0, false
	}
	return limit, true
}

func parseAPILimit(value string) (int, error) {
	if value == "" {
		return defaultAPILimit, nil
	}
	var limit int
	if _, err := fmt.Sscanf(value, "%d", &limit); err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit < 1 {
		return 0, fmt.Errorf("limit must be at least 1")
	}
	if limit > maxAPILimit {
		return maxAPILimit, nil
	}
	return limit, nil
}

func apiCursorOffsetForRequest(w nethttp.ResponseWriter, r *nethttp.Request) (int, bool) {
	offset, err := decodeIndexCursor(r.URL.Query().Get("pageToken"))
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return 0, false
	}
	return offset, true
}

func decodeIndexCursor(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("invalid page token")
	}
	var offset int
	if _, err := fmt.Sscanf(string(raw), "offset:%d", &offset); err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid page token")
	}
	return offset, nil
}

func encodeIndexCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("offset:%d", offset)))
}

func writeJSON(w nethttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w nethttp.ResponseWriter, err error, status int) {
	writeJSON(w, status, api.ErrorResponse{
		Code:      status,
		Message:   err.Error(),
		Details:   map[string]any{},
		RequestID: "",
	})
}

func decodeOptionalJSONBody(r *nethttp.Request, dst any) error {
	if r.Body == nil || r.Body == nethttp.NoBody {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	return fmt.Errorf("malformed JSON: multiple JSON values")
}
