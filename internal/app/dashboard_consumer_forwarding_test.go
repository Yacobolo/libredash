package app

import (
	"context"
	"testing"

	queryauthz "github.com/Yacobolo/leapview/internal/analytics/query/authz"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/command"
	"github.com/Yacobolo/leapview/internal/dashboard/consumer"
	dashboardstream "github.com/Yacobolo/leapview/internal/dashboard/stream"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

type consumerForwardingMetrics struct {
	fakeMetrics
	calls    int
	governed bool
}

func (m *consumerForwardingMetrics) ExecuteConsumersPage(ctx context.Context, request consumer.Request, publish consumer.Publisher) error {
	m.calls++
	_, m.governed = dataquery.GovernorFromContext(ctx)
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 1})
	for _, target := range request.Targets {
		publish(consumer.Result{Target: target, Visual: dashboard.Visual{ID: target.ID}, Queries: 1})
	}
	return nil
}

func TestProductionDashboardWrappersForwardGovernedConsumerPlan(t *testing.T) {
	underlying := &consumerForwardingMetrics{}
	metrics := dashboardCommandMetrics{QueryMetrics: queryAuditMetrics{QueryMetrics: executionMetrics{
		QueryMetrics: queryauthz.New(underlying, queryauthz.Options{}),
	}}}

	visuals := 0
	dashboardstream.TargetWork(metrics, dashboardstream.WorkRequest{
		DashboardID: "sales-dashboard",
		PageID:      "overview",
		Plan: command.RefreshPlan{Targets: []command.Target{
			{Kind: command.TargetVisual, ID: "orders"},
			{Kind: command.TargetVisual, ID: "revenue"},
		}},
	})(context.Background(), func(event dashboardstream.RefreshEvent) bool {
		if event.Type == dashboardstream.RefreshEventVisual {
			visuals++
		}
		return true
	})

	if underlying.calls != 1 || !underlying.governed || visuals != 2 {
		t.Fatalf("calls=%d governed=%v visuals=%d", underlying.calls, underlying.governed, visuals)
	}
}
