package authz

import (
	"context"
	"errors"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type dashboardTargetMetrics interface {
	QueryVisualPage(context.Context, string, string, dashboard.Filters, string) (dashboard.Visual, error)
	QueryVisualsPage(context.Context, string, string, dashboard.Filters, []string) (map[string]dashboard.Visual, error)
	QueryFilterOptionsPage(context.Context, string, string, []string) (map[string][]dashboard.FilterOption, error)
}

func (m Metrics) DashboardTargetConcurrency() int {
	capability, ok := m.Metrics.(interface{ DashboardTargetConcurrency() int })
	if !ok || capability.DashboardTargetConcurrency() <= 1 {
		return 1
	}
	return capability.DashboardTargetConcurrency()
}

func (m Metrics) WithDashboardRefreshLease(ctx context.Context, run func(context.Context) error) error {
	ctx = dataquery.WithGovernor(ctx, m)
	capability, ok := m.Metrics.(interface {
		WithDashboardRefreshLease(context.Context, func(context.Context) error) error
	})
	if !ok {
		return run(ctx)
	}
	return capability.WithDashboardRefreshLease(ctx, run)
}

func (m Metrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	port, err := m.dashboardTargets()
	if err != nil {
		return dashboard.Visual{}, err
	}
	return port.QueryVisualPage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters, visualID)
}

func (m Metrics) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	port, err := m.dashboardTargets()
	if err != nil {
		return nil, err
	}
	return port.QueryVisualsPage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filters, visualIDs)
}

func (m Metrics) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	port, err := m.dashboardTargets()
	if err != nil {
		return nil, err
	}
	return port.QueryFilterOptionsPage(dataquery.WithGovernor(ctx, m), dashboardID, pageID, filterIDs)
}

func (m Metrics) dashboardTargets() (dashboardTargetMetrics, error) {
	port, ok := m.Metrics.(dashboardTargetMetrics)
	if !ok {
		return nil, errors.New("dashboard target queries are not configured")
	}
	return port, nil
}
