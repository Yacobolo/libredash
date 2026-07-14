package app

import (
	"context"
	"errors"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

type progressiveTableMetrics interface {
	QueryTableRowsPage(context.Context, string, string, dashboard.Filters, dashboard.TableRequest) (dashboard.Table, error)
	QueryTableCountPage(context.Context, string, string, dashboard.Filters, dashboard.TableRequest) (int, error)
}

func queryTableRowsPage(ctx context.Context, metrics QueryMetrics, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if port, ok := metrics.(progressiveTableMetrics); ok {
		return port.QueryTableRowsPage(ctx, dashboardID, pageID, filters, request)
	}
	return metrics.QueryTablePage(ctx, dashboardID, pageID, filters, request)
}

func queryTableCountPage(ctx context.Context, metrics QueryMetrics, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error) {
	if port, ok := metrics.(progressiveTableMetrics); ok {
		return port.QueryTableCountPage(ctx, dashboardID, pageID, filters, request)
	}
	table, err := metrics.QueryTablePage(ctx, dashboardID, pageID, filters, request)
	if err != nil {
		return 0, err
	}
	if table.Error != "" {
		return 0, errors.New(table.Error)
	}
	return table.TotalRows, nil
}

func (m runtimeMetrics) QueryTableRowsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	runtime, release, err := m.activeForDashboardRefresh(ctx)
	if err != nil {
		return dashboard.EmptyTable(request.WithDefaults(), err), nil
	}
	defer release()
	port, ok := runtime.(progressiveTableMetrics)
	if !ok {
		tablePort, ok := runtime.(tableRuntime)
		if !ok {
			return dashboard.EmptyTable(request.WithDefaults(), errors.New("active runtime does not provide table data")), nil
		}
		return tablePort.QueryTablePage(ctx, dashboardID, pageID, filters, request)
	}
	return port.QueryTableRowsPage(ctx, dashboardID, pageID, filters, request)
}

func (m runtimeMetrics) QueryTableCountPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error) {
	runtime, release, err := m.activeForDashboardRefresh(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	if port, ok := runtime.(progressiveTableMetrics); ok {
		return port.QueryTableCountPage(ctx, dashboardID, pageID, filters, request)
	}
	tablePort, ok := runtime.(tableRuntime)
	if !ok {
		return 0, errors.New("active runtime does not provide table data")
	}
	table, err := tablePort.QueryTablePage(ctx, dashboardID, pageID, filters, request)
	if err != nil {
		return 0, err
	}
	if table.Error != "" {
		return 0, errors.New(table.Error)
	}
	return table.TotalRows, nil
}

func (m multiWorkspaceMetrics) QueryTableRowsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return queryTableRowsPage(ctx, metrics, dashboardID, pageID, filters, request)
	}
	return dashboard.EmptyTable(request.WithDefaults(), errors.New("workspace metrics are not configured")), nil
}

func (m multiWorkspaceMetrics) QueryTableCountPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return queryTableCountPage(ctx, metrics, dashboardID, pageID, filters, request)
	}
	return 0, errors.New("workspace metrics are not configured")
}

func (m *dynamicRuntimeMetrics) QueryTableRowsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return queryTableRowsPage(ctx, metrics, dashboardID, pageID, filters, request)
	}
	return dashboard.EmptyTable(request.WithDefaults(), errors.New("workspace metrics are not configured")), nil
}

func (m *dynamicRuntimeMetrics) QueryTableCountPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error) {
	if metrics := m.defaultMetrics(); metrics != nil {
		return queryTableCountPage(ctx, metrics, dashboardID, pageID, filters, request)
	}
	return 0, errors.New("workspace metrics are not configured")
}

func (m executionMetrics) QueryTableRowsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return queryTableRowsPage(m.readContext(ctx), m.QueryMetrics, dashboardID, pageID, filters, request)
}

func (m executionMetrics) QueryTableCountPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error) {
	return queryTableCountPage(m.readContext(ctx), m.QueryMetrics, dashboardID, pageID, filters, request)
}

func (m queryAuditMetrics) QueryTableRowsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if m.QueryMetrics == nil {
		return dashboard.EmptyTable(request.WithDefaults(), errors.New("query metrics are not configured")), nil
	}
	return queryTableRowsPage(m.auditContext(ctx), m.QueryMetrics, dashboardID, pageID, filters, request)
}

func (m queryAuditMetrics) QueryTableCountPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error) {
	if m.QueryMetrics == nil {
		return 0, errors.New("query metrics are not configured")
	}
	return queryTableCountPage(m.auditContext(ctx), m.QueryMetrics, dashboardID, pageID, filters, request)
}

func (m dashboardCommandMetrics) QueryTableRowsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return queryTableRowsPage(ctx, m.QueryMetrics, dashboardID, pageID, filters, request)
}

func (m dashboardCommandMetrics) QueryTableCountPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error) {
	return queryTableCountPage(ctx, m.QueryMetrics, dashboardID, pageID, filters, request)
}
