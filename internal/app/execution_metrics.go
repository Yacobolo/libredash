package app

import (
	"context"
	"errors"

	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/execution"
)

type executionMetrics struct {
	QueryMetrics
	executor           *execution.Service
	defaultWorkspaceID string
}

func (m executionMetrics) executionService() *execution.Service {
	if m.executor == nil {
		return execution.New(execution.DefaultConfig())
	}
	return m.executor
}

func (m executionMetrics) readContext(ctx context.Context) context.Context {
	return execution.WithReadAdmission(ctx, m.executionService())
}

func (m executionMetrics) MetricsForWorkspace(workspaceID string) (QueryMetrics, bool) {
	provider, ok := m.QueryMetrics.(workspaceMetrics)
	if ok {
		metrics, found := provider.MetricsForWorkspace(workspaceID)
		if !found || metrics == nil {
			return nil, found
		}
		return executionMetrics{QueryMetrics: metrics, executor: m.executor, defaultWorkspaceID: workspaceID}, true
	}
	if m.QueryMetrics == nil {
		return nil, false
	}
	if m.defaultWorkspaceID != "" && workspaceID != "" && workspaceID != m.defaultWorkspaceID {
		return nil, false
	}
	return m, true
}

func (m executionMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryDashboardPage(ctx, dashboardID, "", filters)
}

func (m executionMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryMetrics.QueryDashboardPage(m.readContext(ctx), dashboardID, pageID, filters)
}

func (m executionMetrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryTablePage(ctx, dashboardID, "", filters, request)
}

func (m executionMetrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryMetrics.QueryTablePage(m.readContext(ctx), dashboardID, pageID, filters, request)
}

func (m executionMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	return execution.SubmitReadFromContext(m.readContext(ctx), request, func(ctx context.Context) (dataquery.Result, error) {
		return m.QueryMetrics.ExecuteDataQuery(ctx, request)
	})
}

func (m executionMetrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return m.QueryMetrics.QuerySemantic(m.readContext(ctx), modelID, request)
}

func (m executionMetrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return m.QueryMetrics.PreviewSemantic(m.readContext(ctx), modelID, request)
}

func (m executionMetrics) RefreshModelTables(ctx context.Context, modelID string, tableNames []string) error {
	if port, ok := m.QueryMetrics.(modelTableRefreshMetrics); ok {
		return port.RefreshModelTables(ctx, modelID, tableNames)
	}
	if port, ok := m.QueryMetrics.(modelTableRefreshRuntimeMetrics); ok {
		return port.RefreshTables(ctx, modelID, tableNames)
	}
	return errors.New("model table refresh is not configured")
}

func (m executionMetrics) RefreshTables(ctx context.Context, modelID string, tableNames []string) error {
	return m.RefreshModelTables(ctx, modelID, tableNames)
}
