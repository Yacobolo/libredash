package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

type dashboardTargetRuntime interface {
	QueryVisualPage(context.Context, string, string, dashboard.Filters, string) (dashboard.Visual, error)
	QueryVisualsPage(context.Context, string, string, dashboard.Filters, []string) (map[string]dashboard.Visual, error)
	QueryFilterOptionsPage(context.Context, string, string, []string) (map[string][]dashboard.FilterOption, error)
}

type dashboardTargetMetrics interface {
	QueryVisualPage(context.Context, string, string, dashboard.Filters, string) (dashboard.Visual, error)
	QueryVisualsPage(context.Context, string, string, dashboard.Filters, []string) (map[string]dashboard.Visual, error)
	QueryFilterOptionsPage(context.Context, string, string, []string) (map[string][]dashboard.FilterOption, error)
}

type dashboardConcurrencyMetrics interface {
	DashboardTargetConcurrency() int
}

type dashboardRefreshLeaseMetrics interface {
	WithDashboardRefreshLease(context.Context, func(context.Context) error) error
}

func withDashboardRefreshLease(ctx context.Context, metrics any, run func(context.Context) error) error {
	if capability, ok := metrics.(dashboardRefreshLeaseMetrics); ok {
		return capability.WithDashboardRefreshLease(ctx, run)
	}
	return run(ctx)
}

func (m multiWorkspaceMetrics) WithDashboardRefreshLease(ctx context.Context, run func(context.Context) error) error {
	return withDashboardRefreshLease(ctx, m.defaultMetrics(), run)
}

func (m *dynamicRuntimeMetrics) WithDashboardRefreshLease(ctx context.Context, run func(context.Context) error) error {
	return withDashboardRefreshLease(ctx, m.defaultMetrics(), run)
}

func (m executionMetrics) WithDashboardRefreshLease(ctx context.Context, run func(context.Context) error) error {
	return withDashboardRefreshLease(m.readContext(ctx), m.QueryMetrics, run)
}

func (m queryAuditMetrics) WithDashboardRefreshLease(ctx context.Context, run func(context.Context) error) error {
	return withDashboardRefreshLease(m.auditContext(ctx), m.QueryMetrics, run)
}

func (m dashboardCommandMetrics) WithDashboardRefreshLease(ctx context.Context, run func(context.Context) error) error {
	return withDashboardRefreshLease(ctx, m.QueryMetrics, run)
}

func dashboardConcurrencyFrom(metrics any) int {
	capability, ok := metrics.(dashboardConcurrencyMetrics)
	if !ok || capability.DashboardTargetConcurrency() <= 1 {
		return 1
	}
	return capability.DashboardTargetConcurrency()
}

func (m runtimeMetrics) DashboardTargetConcurrency() int {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return 1
	}
	defer release()
	return dashboardConcurrencyFrom(runtime)
}

func (m multiWorkspaceMetrics) DashboardTargetConcurrency() int {
	return dashboardConcurrencyFrom(m.defaultMetrics())
}

func (m *dynamicRuntimeMetrics) DashboardTargetConcurrency() int {
	return dashboardConcurrencyFrom(m.defaultMetrics())
}

func (m executionMetrics) DashboardTargetConcurrency() int {
	return dashboardConcurrencyFrom(m.QueryMetrics)
}

func (m queryAuditMetrics) DashboardTargetConcurrency() int {
	return dashboardConcurrencyFrom(m.QueryMetrics)
}

func (m dashboardCommandMetrics) DashboardTargetConcurrency() int {
	return dashboardConcurrencyFrom(m.QueryMetrics)
}

func (m runtimeMetrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	runtime, release, err := m.activeForDashboardRefresh(ctx)
	if err != nil {
		return dashboard.Visual{}, err
	}
	defer release()
	port, ok := runtime.(dashboardTargetRuntime)
	if !ok {
		return dashboard.Visual{}, errors.New("active runtime does not provide targeted visual data")
	}
	return port.QueryVisualPage(ctx, dashboardID, pageID, filters, visualID)
}

func (m runtimeMetrics) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	runtime, release, err := m.activeForDashboardRefresh(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	port, ok := runtime.(dashboardTargetRuntime)
	if !ok {
		return nil, errors.New("active runtime does not provide targeted visual data")
	}
	return port.QueryVisualsPage(ctx, dashboardID, pageID, filters, visualIDs)
}

func (m runtimeMetrics) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	runtime, release, err := m.activeForDashboardRefresh(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	port, ok := runtime.(dashboardTargetRuntime)
	if !ok {
		return nil, errors.New("active runtime does not provide targeted filter options")
	}
	return port.QueryFilterOptionsPage(ctx, dashboardID, pageID, filterIDs)
}

func (m multiWorkspaceMetrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.defaultMetrics())
	if err != nil {
		return dashboard.Visual{}, err
	}
	return port.QueryVisualPage(ctx, dashboardID, pageID, filters, visualID)
}

func (m multiWorkspaceMetrics) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.defaultMetrics())
	if err != nil {
		return nil, err
	}
	return port.QueryVisualsPage(ctx, dashboardID, pageID, filters, visualIDs)
}

func (m multiWorkspaceMetrics) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	port, err := targetMetricsFrom(m.defaultMetrics())
	if err != nil {
		return nil, err
	}
	return port.QueryFilterOptionsPage(ctx, dashboardID, pageID, filterIDs)
}

func (m *dynamicRuntimeMetrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.defaultMetrics())
	if err != nil {
		return dashboard.Visual{}, err
	}
	return port.QueryVisualPage(ctx, dashboardID, pageID, filters, visualID)
}

func (m *dynamicRuntimeMetrics) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.defaultMetrics())
	if err != nil {
		return nil, err
	}
	return port.QueryVisualsPage(ctx, dashboardID, pageID, filters, visualIDs)
}

func (m *dynamicRuntimeMetrics) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	port, err := targetMetricsFrom(m.defaultMetrics())
	if err != nil {
		return nil, err
	}
	return port.QueryFilterOptionsPage(ctx, dashboardID, pageID, filterIDs)
}

func (m executionMetrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.QueryMetrics)
	if err != nil {
		return dashboard.Visual{}, err
	}
	return port.QueryVisualPage(m.readContext(ctx), dashboardID, pageID, filters, visualID)
}

func (m executionMetrics) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.QueryMetrics)
	if err != nil {
		return nil, err
	}
	return port.QueryVisualsPage(m.readContext(ctx), dashboardID, pageID, filters, visualIDs)
}

func (m executionMetrics) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	port, err := targetMetricsFrom(m.QueryMetrics)
	if err != nil {
		return nil, err
	}
	return port.QueryFilterOptionsPage(m.readContext(ctx), dashboardID, pageID, filterIDs)
}

func (m queryAuditMetrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.QueryMetrics)
	if err != nil {
		return dashboard.Visual{}, err
	}
	return port.QueryVisualPage(m.auditContext(ctx), dashboardID, pageID, filters, visualID)
}

func (m queryAuditMetrics) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.QueryMetrics)
	if err != nil {
		return nil, err
	}
	return port.QueryVisualsPage(m.auditContext(ctx), dashboardID, pageID, filters, visualIDs)
}

func (m queryAuditMetrics) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	port, err := targetMetricsFrom(m.QueryMetrics)
	if err != nil {
		return nil, err
	}
	return port.QueryFilterOptionsPage(m.auditContext(ctx), dashboardID, pageID, filterIDs)
}

func (m dashboardCommandMetrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.QueryMetrics)
	if err != nil {
		return dashboard.Visual{}, err
	}
	return port.QueryVisualPage(ctx, dashboardID, pageID, filters, visualID)
}

func (m dashboardCommandMetrics) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	port, err := targetMetricsFrom(m.QueryMetrics)
	if err != nil {
		return nil, err
	}
	return port.QueryVisualsPage(ctx, dashboardID, pageID, filters, visualIDs)
}

func (m dashboardCommandMetrics) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	port, err := targetMetricsFrom(m.QueryMetrics)
	if err != nil {
		return nil, err
	}
	return port.QueryFilterOptionsPage(ctx, dashboardID, pageID, filterIDs)
}

func targetMetricsFrom(metrics any) (dashboardTargetMetrics, error) {
	if metrics == nil {
		return nil, errors.New("dashboard target metrics are not configured")
	}
	port, ok := metrics.(dashboardTargetMetrics)
	if !ok {
		return nil, fmt.Errorf("%T does not provide dashboard target queries", metrics)
	}
	return port, nil
}
