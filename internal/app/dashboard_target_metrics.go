package app

import "context"

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
