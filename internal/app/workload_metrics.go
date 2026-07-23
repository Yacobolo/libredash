package app

import (
	"context"
	"errors"
	"time"

	"github.com/Yacobolo/leapview/internal/analytics/arrowquery"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/dataquery"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	"github.com/Yacobolo/leapview/internal/workload"
)

type workloadMetrics struct {
	QueryMetrics
	admitter           workload.Admitter
	defaultWorkspaceID string
}

func (m workloadMetrics) readContext(ctx context.Context) context.Context {
	return workload.WithAdmitter(ctx, m.admitter)
}

func (m workloadMetrics) MetricsForWorkspace(workspaceID string) (QueryMetrics, bool) {
	provider, ok := m.QueryMetrics.(workspaceMetrics)
	if ok {
		metrics, found := provider.MetricsForWorkspace(workspaceID)
		if !found || metrics == nil {
			return nil, found
		}
		return workloadMetrics{QueryMetrics: metrics, admitter: m.admitter, defaultWorkspaceID: workspaceID}, true
	}
	if m.QueryMetrics == nil {
		return nil, false
	}
	if m.defaultWorkspaceID != "" && workspaceID != "" && workspaceID != m.defaultWorkspaceID {
		return nil, false
	}
	return m, true
}

func (m workloadMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryDashboardPage(ctx, dashboardID, "", filters)
}

func (m workloadMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryMetrics.QueryDashboardPage(m.readContext(ctx), dashboardID, pageID, filters)
}

func (m workloadMetrics) QueryDashboardVisualizations(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryMetrics.QueryDashboardVisualizations(m.readContext(ctx), dashboardID, pageID, filters)
}

func (m workloadMetrics) QueryVisualization(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (visualizationir.VisualizationEnvelope, error) {
	return m.QueryMetrics.QueryVisualization(m.readContext(ctx), dashboardID, pageID, filters, visualID)
}

func (m workloadMetrics) QueryVisualizationWindow(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request visualizationir.VisualizationWindowRequest) (visualizationir.VisualizationEnvelope, error) {
	return m.QueryMetrics.QueryVisualizationWindow(m.readContext(ctx), dashboardID, pageID, filters, request)
}

func (m workloadMetrics) QueryVisualizationSpatialWindow(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request visualizationir.VisualizationSpatialWindowRequest) (visualizationir.VisualizationEnvelope, error) {
	return m.QueryMetrics.QueryVisualizationSpatialWindow(m.readContext(ctx), dashboardID, pageID, filters, request)
}

func (m workloadMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	ctx = m.readContext(ctx)
	if m.admitter == nil {
		return m.QueryMetrics.ExecuteDataQuery(ctx, request)
	}
	workspaceID := request.WorkspaceID
	if workspaceID == "" {
		workspaceID = m.defaultWorkspaceID
	}
	class := workload.Interactive
	if request.Surface == dataquery.SurfaceAgent {
		class = workload.Background
		if activeClass, activeWorkspace, admitted := workload.Current(ctx); admitted && activeClass == workload.Background {
			workspaceID = activeWorkspace
		}
	}
	operation := request.Operation
	if operation == "" {
		operation = string(request.Kind)
	}
	lease, err := m.admitter.Acquire(ctx, workload.Request{Class: class, WorkspaceID: workspaceID, Operation: operation})
	if err != nil {
		result := dataquery.Result{ExecutionState: executionStateForWorkloadError(ctx, err)}
		var rejection *workload.Rejection
		if errors.As(err, &rejection) {
			result.QueueWaitMS = rejection.QueueWait.Milliseconds()
		}
		return result, err
	}
	defer lease.Release()
	started := time.Now()
	result, err := m.QueryMetrics.ExecuteDataQuery(lease.Context(), request)
	if result.QueueWaitMS == 0 {
		result.QueueWaitMS = lease.QueueWait().Milliseconds()
	}
	if result.ExecutionMS == 0 {
		result.ExecutionMS = elapsedMillis(time.Since(started))
	}
	if result.ExecutionState == "" {
		if err == nil {
			result.ExecutionState = dataquery.ExecutionSucceeded
		} else {
			result.ExecutionState = executionStateForWorkloadError(lease.Context(), err)
		}
	}
	return result, err
}

func (m workloadMetrics) ExecuteDataQueryArrow(ctx context.Context, request dataquery.Query, sink arrowquery.Sink) (dataquery.Result, error) {
	ctx = m.readContext(ctx)
	executor, ok := m.QueryMetrics.(arrowquery.Executor)
	if !ok {
		return dataquery.Result{}, errors.New("query metrics do not support native Arrow execution")
	}
	if m.admitter == nil {
		return executor.ExecuteDataQueryArrow(ctx, request, sink)
	}
	workspaceID := request.WorkspaceID
	if workspaceID == "" {
		workspaceID = m.defaultWorkspaceID
	}
	class := workload.Interactive
	if request.Surface == dataquery.SurfaceAgent {
		class = workload.Background
		if activeClass, activeWorkspace, admitted := workload.Current(ctx); admitted && activeClass == workload.Background {
			workspaceID = activeWorkspace
		}
	}
	operation := request.Operation
	if operation == "" {
		operation = string(request.Kind)
	}
	lease, err := m.admitter.Acquire(ctx, workload.Request{Class: class, WorkspaceID: workspaceID, Operation: operation})
	if err != nil {
		result := dataquery.Result{ExecutionState: executionStateForWorkloadError(ctx, err)}
		var rejection *workload.Rejection
		if errors.As(err, &rejection) {
			result.QueueWaitMS = rejection.QueueWait.Milliseconds()
		}
		return result, err
	}
	defer lease.Release()
	started := time.Now()
	result, err := executor.ExecuteDataQueryArrow(lease.Context(), request, sink)
	if result.QueueWaitMS == 0 {
		result.QueueWaitMS = lease.QueueWait().Milliseconds()
	}
	if result.ExecutionMS == 0 {
		result.ExecutionMS = elapsedMillis(time.Since(started))
	}
	if result.ExecutionState == "" {
		if err == nil {
			result.ExecutionState = dataquery.ExecutionSucceeded
		} else {
			result.ExecutionState = executionStateForWorkloadError(lease.Context(), err)
		}
	}
	return result, err
}

func (m workloadMetrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return m.QueryMetrics.QuerySemantic(m.readContext(ctx), modelID, request)
}

func (m workloadMetrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return m.QueryMetrics.PreviewSemantic(m.readContext(ctx), modelID, request)
}

func elapsedMillis(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	if milliseconds := duration.Milliseconds(); milliseconds > 0 {
		return milliseconds
	}
	return 1
}

func executionStateForWorkloadError(ctx context.Context, err error) string {
	if err == context.DeadlineExceeded || ctx.Err() == context.DeadlineExceeded {
		return dataquery.ExecutionTimeout
	}
	if err == context.Canceled || ctx.Err() == context.Canceled {
		return dataquery.ExecutionCanceled
	}
	if reason, ok := workload.ReasonOf(err); ok && reason == workload.QueueTimeout {
		return dataquery.ExecutionTimeout
	}
	return dataquery.ExecutionRejected
}
