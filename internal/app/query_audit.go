package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Yacobolo/leapview/internal/analytics/arrowquery"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/Yacobolo/leapview/internal/queryaudit"
	queryauditsqlite "github.com/Yacobolo/leapview/internal/queryaudit/sqlite"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	"github.com/go-chi/chi/v5"
)

const cliClientHeader = "X-LeapView-Client"

type queryAuditMetrics struct {
	QueryMetrics
	recorder           queryaudit.Repository
	defaultWorkspaceID string
}

func (s *Server) queryAuditRepository() (queryaudit.Repository, error) {
	if s.store == nil {
		return nil, nil
	}
	return queryauditsqlite.NewRepository(s.store.SQLDB()), nil
}

func (m queryAuditMetrics) MetricsForWorkspace(workspaceID string) (QueryMetrics, bool) {
	provider, ok := m.QueryMetrics.(workspaceMetrics)
	if ok {
		metrics, ok := provider.MetricsForWorkspace(workspaceID)
		if !ok || metrics == nil {
			return nil, ok
		}
		return queryAuditMetrics{QueryMetrics: metrics, recorder: m.recorder, defaultWorkspaceID: workspaceID}, true
	}
	if m.QueryMetrics == nil {
		return nil, false
	}
	if m.defaultWorkspaceID != "" && workspaceID == m.defaultWorkspaceID {
		return m, true
	}
	catalog := m.QueryMetrics.Catalog()
	if catalog.Workspace.ID == "" || catalog.Workspace.ID == workspaceID {
		return m, true
	}
	return nil, false
}

func (m queryAuditMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	if m.QueryMetrics == nil {
		return dataquery.Result{}, errors.New("query metrics are not configured")
	}
	ctx = m.auditContext(ctx)
	if request.WorkspaceID == "" {
		request.WorkspaceID = m.defaultWorkspaceID
	}
	return dataquery.ExecuteAudited(ctx, request, m.QueryMetrics.ExecuteDataQuery)
}

func (m queryAuditMetrics) ExecuteDataQueryArrow(ctx context.Context, request dataquery.Query, sink arrowquery.Sink) (dataquery.Result, error) {
	if m.QueryMetrics == nil {
		return dataquery.Result{}, errors.New("query metrics are not configured")
	}
	executor, ok := m.QueryMetrics.(arrowquery.Executor)
	if !ok {
		return dataquery.Result{}, errors.New("query metrics do not support native Arrow execution")
	}
	ctx = m.auditContext(ctx)
	if request.WorkspaceID == "" {
		request.WorkspaceID = m.defaultWorkspaceID
	}
	return dataquery.ExecuteAudited(ctx, request, func(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
		return executor.ExecuteDataQueryArrow(ctx, request, sink)
	})
}

func (m queryAuditMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryDashboardPage(ctx, dashboardID, "", filters)
}

func (m queryAuditMetrics) QueryCompiledFilterOptions(ctx context.Context, dashboardID string, query dashboardfilter.OptionQuery) (dashboardfilter.OptionResult, error) {
	provider, ok := m.QueryMetrics.(interface {
		QueryCompiledFilterOptions(context.Context, string, dashboardfilter.OptionQuery) (dashboardfilter.OptionResult, error)
	})
	if !ok {
		return dashboardfilter.OptionResult{}, errors.New("compiled filter options are not supported by this runtime")
	}
	return provider.QueryCompiledFilterOptions(m.auditContext(ctx), dashboardID, query)
}

func (m queryAuditMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	if m.QueryMetrics == nil {
		return dashboard.EmptyPatch(filters.WithDefaults(), errors.New("query metrics are not configured")), nil
	}
	return m.QueryMetrics.QueryDashboardPage(m.auditContext(ctx), dashboardID, pageID, filters)
}

func (m queryAuditMetrics) QueryDashboardVisualizations(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	if m.QueryMetrics == nil {
		return dashboard.EmptyPatch(filters.WithDefaults(), errors.New("query metrics are not configured")), nil
	}
	return m.QueryMetrics.QueryDashboardVisualizations(m.auditContext(ctx), dashboardID, pageID, filters)
}

func (m queryAuditMetrics) QueryVisualization(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (visualizationir.VisualizationEnvelope, error) {
	if m.QueryMetrics == nil {
		return visualizationir.VisualizationEnvelope{}, errors.New("query metrics are not configured")
	}
	return m.QueryMetrics.QueryVisualization(m.auditContext(ctx), dashboardID, pageID, filters, visualID)
}

func (m queryAuditMetrics) QueryVisualizationWindow(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request visualizationir.VisualizationWindowRequest) (visualizationir.VisualizationEnvelope, error) {
	if m.QueryMetrics == nil {
		return visualizationir.VisualizationEnvelope{}, errors.New("query metrics are not configured")
	}
	return m.QueryMetrics.QueryVisualizationWindow(m.auditContext(ctx), dashboardID, pageID, filters, request)
}

func (m queryAuditMetrics) QueryVisualizationSpatialWindow(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request visualizationir.VisualizationSpatialWindowRequest) (visualizationir.VisualizationEnvelope, error) {
	if m.QueryMetrics == nil {
		return visualizationir.VisualizationEnvelope{}, errors.New("query metrics are not configured")
	}
	return m.QueryMetrics.QueryVisualizationSpatialWindow(m.auditContext(ctx), dashboardID, pageID, filters, request)
}

func (m queryAuditMetrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	if m.QueryMetrics == nil {
		return nil, errors.New("query metrics are not configured")
	}
	return m.QueryMetrics.QuerySemantic(m.auditContext(ctx), modelID, request)
}

func (m queryAuditMetrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	if m.QueryMetrics == nil {
		return nil, errors.New("query metrics are not configured")
	}
	return m.QueryMetrics.PreviewSemantic(m.auditContext(ctx), modelID, request)
}

func (m queryAuditMetrics) auditContext(ctx context.Context) context.Context {
	metadata := dataquery.MetadataFromContext(ctx)
	if metadata.WorkspaceID == "" {
		metadata.WorkspaceID = m.defaultWorkspaceID
	}
	if metadata.PrincipalID == "" {
		if principal, ok := principalFromContext(ctx); ok {
			metadata.PrincipalID = principal.ID
		}
	}
	ctx = dataquery.WithMetadata(ctx, metadata)
	if m.recorder != nil {
		ctx = dataquery.WithAuditRecorder(ctx, queryEventRecorder{repo: m.recorder})
	}
	return ctx
}

type queryEventRecorder struct {
	repo queryaudit.Repository
}

func (r queryEventRecorder) RecordDataQuery(ctx context.Context, request dataquery.Query, result dataquery.Result) error {
	if r.repo == nil {
		return nil
	}
	return r.repo.RecordQueryEvent(ctx, queryEventInput(request, result))
}

func queryEventInput(request dataquery.Query, result dataquery.Result) queryaudit.EventInput {
	return queryaudit.EventInput{
		WorkspaceID:      request.WorkspaceID,
		PrincipalID:      request.PrincipalID,
		Surface:          request.Surface,
		Operation:        request.Operation,
		QueryKind:        string(request.Kind),
		ModelID:          request.ModelID,
		Target:           request.Target,
		ObjectType:       request.ObjectType,
		ObjectID:         request.ObjectID,
		RequestID:        request.RequestID,
		CorrelationID:    request.CorrelationID,
		Status:           queryFirstNonEmpty(result.Status, dataquery.StatusSuccess),
		DurationMS:       result.DurationMS,
		QueueWaitMS:      result.QueueWaitMS,
		PlanningMS:       result.PlanningMS,
		ConnectionWaitMS: result.ConnectionWaitMS,
		DatabaseMS:       result.DatabaseMS,
		ExecutionMS:      result.ExecutionMS,
		ExecutionState:   result.ExecutionState,
		RowsReturned:     result.RowsReturned,
		BytesEstimate:    result.BytesEstimate,
		Error:            result.Error,
		SQL:              result.SQL,
		PlanText:         result.PlanText,
		QueryJSON:        queryShapeJSON(request),
	}
}

func queryShapeJSON(request dataquery.Query) string {
	type queryShape struct {
		WorkspaceID   string             `json:"workspaceId,omitempty"`
		Surface       string             `json:"surface,omitempty"`
		Operation     string             `json:"operation,omitempty"`
		RequestID     string             `json:"requestId,omitempty"`
		ObjectType    string             `json:"objectType,omitempty"`
		ObjectID      string             `json:"objectId,omitempty"`
		CorrelationID string             `json:"correlationId,omitempty"`
		ModelID       string             `json:"modelId,omitempty"`
		Kind          dataquery.Kind     `json:"kind"`
		Target        string             `json:"target,omitempty"`
		Fields        []dataquery.Field  `json:"fields,omitempty"`
		Measures      []dataquery.Field  `json:"measures,omitempty"`
		Value         dataquery.Field    `json:"value,omitempty"`
		Time          dataquery.Time     `json:"time,omitempty"`
		Filters       []dataquery.Filter `json:"filters,omitempty"`
		Sort          []dataquery.Sort   `json:"sort,omitempty"`
		Offset        int                `json:"offset,omitempty"`
		Limit         int                `json:"limit,omitempty"`
		BinCount      int                `json:"binCount,omitempty"`
		IncludeTotal  bool               `json:"includeTotal,omitempty"`
	}
	bytes, err := json.Marshal(queryShape{
		WorkspaceID:   request.WorkspaceID,
		Surface:       request.Surface,
		Operation:     request.Operation,
		RequestID:     request.RequestID,
		ObjectType:    request.ObjectType,
		ObjectID:      request.ObjectID,
		CorrelationID: request.CorrelationID,
		ModelID:       request.ModelID,
		Kind:          request.Kind,
		Target:        request.Target,
		Fields:        request.Fields,
		Measures:      request.Measures,
		Value:         request.Value,
		Time:          request.Time,
		Filters:       request.Filters,
		Sort:          request.Sort,
		Offset:        request.Offset,
		Limit:         request.Limit,
		BinCount:      request.BinCount,
		IncludeTotal:  request.IncludeTotal,
	})
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func requestQueryMetadata(r *http.Request, surface, operation, objectType, objectID string) dataquery.Metadata {
	if surface == dataquery.SurfaceAPI && r.Header.Get(cliClientHeader) == dataquery.SurfaceCLI {
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
	if principal, ok := principalFromContext(r.Context()); ok {
		metadata.PrincipalID = principal.ID
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

func queryFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
