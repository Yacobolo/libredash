package authz

import (
	"context"
	"errors"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type progressiveTableMetrics interface {
	QueryTableRowsPage(context.Context, string, string, dashboard.Filters, dashboard.TableRequest) (dashboard.Table, error)
	QueryTableCountPage(context.Context, string, string, dashboard.Filters, dashboard.TableRequest) (int, error)
}

func (m Metrics) QueryTableRowsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	port, ok := m.Metrics.(progressiveTableMetrics)
	if !ok {
		return m.Metrics.QueryTablePage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters, request)
	}
	return port.QueryTableRowsPage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters, request)
}

func (m Metrics) QueryTableCountPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error) {
	port, ok := m.Metrics.(progressiveTableMetrics)
	if !ok {
		table, err := m.Metrics.QueryTablePage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters, request)
		if err != nil {
			return 0, err
		}
		if table.Error != "" {
			return 0, errors.New(table.Error)
		}
		return table.TotalRows, nil
	}
	return port.QueryTableCountPage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters, request)
}
