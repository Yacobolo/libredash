package app

import (
	"context"
	"fmt"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func (m multiWorkspaceMetrics) Catalog() dashboard.Catalog {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.Catalog()
	}
	return dashboard.Catalog{}
}

func (m multiWorkspaceMetrics) DefaultDashboardID() string {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.DefaultDashboardID()
	}
	return ""
}

func (m multiWorkspaceMetrics) ModelIDForDashboard(dashboardID string) string {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.ModelIDForDashboard(dashboardID)
	}
	return ""
}

func (m multiWorkspaceMetrics) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.Report(dashboardID)
	}
	return reportdef.Dashboard{}, nil, false
}

func (m multiWorkspaceMetrics) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.SemanticModel(modelID)
	}
	return nil, false
}

func (m multiWorkspaceMetrics) DefaultFilters(dashboardID string) dashboard.Filters {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.DefaultFilters(dashboardID)
	}
	return dashboard.Filters{}.WithDefaults()
}

func (m multiWorkspaceMetrics) NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.NormalizeTableRequest(dashboardID, request)
	}
	return request.WithDefaults()
}

func (m multiWorkspaceMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QueryDashboard(ctx, dashboardID, filters)
	}
	return dashboard.EmptyPatch(filters.WithDefaults(), fmt.Errorf("workspace metrics are not configured")), nil
}

func (m multiWorkspaceMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QueryDashboardPage(ctx, dashboardID, pageID, filters)
	}
	return dashboard.EmptyPatch(filters.WithDefaults(), fmt.Errorf("workspace metrics are not configured")), nil
}

func (m multiWorkspaceMetrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QueryTable(ctx, dashboardID, filters, request)
	}
	return dashboard.EmptyTable(request.WithDefaults(), fmt.Errorf("workspace metrics are not configured")), nil
}

func (m multiWorkspaceMetrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QueryTablePage(ctx, dashboardID, pageID, filters, request)
	}
	return dashboard.EmptyTable(request.WithDefaults(), fmt.Errorf("workspace metrics are not configured")), nil
}

func (m multiWorkspaceMetrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QuerySemantic(ctx, modelID, request)
	}
	return nil, fmt.Errorf("workspace metrics are not configured")
}

func (m multiWorkspaceMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	metrics := m.defaultMetrics()
	if request.WorkspaceID != "" {
		metrics = m.workspaces[request.WorkspaceID]
	}
	if metrics != nil {
		return metrics.ExecuteDataQuery(ctx, request)
	}
	return dataquery.Result{}, fmt.Errorf("workspace metrics are not configured")
}

func (m multiWorkspaceMetrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.PreviewSemantic(ctx, modelID, request)
	}
	return nil, fmt.Errorf("workspace metrics are not configured")
}

func (m multiWorkspaceMetrics) RefreshMaterializations(ctx context.Context, modelID string) error {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.RefreshMaterializations(ctx, modelID)
	}
	return fmt.Errorf("workspace metrics are not configured")
}

func (m multiWorkspaceMetrics) Pages(dashboardID string) []dashboard.Page {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.Pages(dashboardID)
	}
	return nil
}

func (m multiWorkspaceMetrics) WorkspaceAssets(workspaceID, servingStateID string) ([]workspace.Asset, []workspace.AssetEdge, bool) {
	metrics, ok := m.MetricsForWorkspace(workspaceID)
	if !ok {
		return nil, nil, false
	}
	provider, ok := metrics.(workspaceAssetRuntime)
	if !ok {
		return nil, nil, false
	}
	return provider.WorkspaceAssets(workspaceID, servingStateID)
}

func (m *dynamicRuntimeMetrics) Catalog() dashboard.Catalog {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.Catalog()
	}
	return dashboard.Catalog{}
}

func (m *dynamicRuntimeMetrics) DefaultDashboardID() string {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.DefaultDashboardID()
	}
	return ""
}

func (m *dynamicRuntimeMetrics) ModelIDForDashboard(dashboardID string) string {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.ModelIDForDashboard(dashboardID)
	}
	return ""
}

func (m *dynamicRuntimeMetrics) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.Report(dashboardID)
	}
	return reportdef.Dashboard{}, nil, false
}

func (m *dynamicRuntimeMetrics) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.SemanticModel(modelID)
	}
	return nil, false
}

func (m *dynamicRuntimeMetrics) DefaultFilters(dashboardID string) dashboard.Filters {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.DefaultFilters(dashboardID)
	}
	return dashboard.Filters{}.WithDefaults()
}

func (m *dynamicRuntimeMetrics) NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.NormalizeTableRequest(dashboardID, request)
	}
	return request.WithDefaults()
}

func (m *dynamicRuntimeMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QueryDashboard(ctx, dashboardID, filters)
	}
	return dashboard.EmptyPatch(filters.WithDefaults(), fmt.Errorf("workspace metrics are not configured")), nil
}

func (m *dynamicRuntimeMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QueryDashboardPage(ctx, dashboardID, pageID, filters)
	}
	return dashboard.EmptyPatch(filters.WithDefaults(), fmt.Errorf("workspace metrics are not configured")), nil
}

func (m *dynamicRuntimeMetrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QueryTable(ctx, dashboardID, filters, request)
	}
	return dashboard.EmptyTable(request.WithDefaults(), fmt.Errorf("workspace metrics are not configured")), nil
}

func (m *dynamicRuntimeMetrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QueryTablePage(ctx, dashboardID, pageID, filters, request)
	}
	return dashboard.EmptyTable(request.WithDefaults(), fmt.Errorf("workspace metrics are not configured")), nil
}

func (m *dynamicRuntimeMetrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.QuerySemantic(ctx, modelID, request)
	}
	return nil, fmt.Errorf("workspace metrics are not configured")
}

func (m *dynamicRuntimeMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	workspaceID := request.WorkspaceID
	if workspaceID == "" {
		workspaceID = m.defaultID
	}
	if metrics, ok := m.MetricsForWorkspace(workspaceID); ok {
		if request.WorkspaceID == "" {
			request.WorkspaceID = workspaceID
		}
		return metrics.ExecuteDataQuery(ctx, request)
	}
	return dataquery.Result{}, fmt.Errorf("workspace metrics are not configured")
}

func (m *dynamicRuntimeMetrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.PreviewSemantic(ctx, modelID, request)
	}
	return nil, fmt.Errorf("workspace metrics are not configured")
}

func (m *dynamicRuntimeMetrics) RefreshMaterializations(ctx context.Context, modelID string) error {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.RefreshMaterializations(ctx, modelID)
	}
	return fmt.Errorf("workspace metrics are not configured")
}

func (m *dynamicRuntimeMetrics) Pages(dashboardID string) []dashboard.Page {
	if metrics := m.defaultMetrics(); metrics != nil {
		return metrics.Pages(dashboardID)
	}
	return nil
}

func (m *dynamicRuntimeMetrics) WorkspaceAssets(workspaceID, servingStateID string) ([]workspace.Asset, []workspace.AssetEdge, bool) {
	metrics, ok := m.MetricsForWorkspace(workspaceID)
	if !ok {
		return nil, nil, false
	}
	provider, ok := metrics.(workspaceAssetRuntime)
	if !ok {
		return nil, nil, false
	}
	return provider.WorkspaceAssets(workspaceID, servingStateID)
}
