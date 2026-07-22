package authz

import (
	"context"
	"errors"

	"github.com/Yacobolo/leapview/internal/dashboard/consumer"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

func (m Metrics) ExecuteConsumersPage(ctx context.Context, request consumer.Request, publish consumer.Publisher) error {
	port, ok := m.Metrics.(consumer.Executor)
	if !ok {
		return errors.New("dashboard consumer execution is not configured")
	}
	return port.ExecuteConsumersPage(dataquery.WithGovernor(ctx, m), request, publish)
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
