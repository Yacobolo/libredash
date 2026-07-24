package app

import (
	"context"
	"testing"

	queryauthz "github.com/Yacobolo/leapview/internal/analytics/query/authz"
	"github.com/Yacobolo/leapview/internal/dashboard/command"
	"github.com/Yacobolo/leapview/internal/dashboard/consumer"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
	dashboardstream "github.com/Yacobolo/leapview/internal/dashboard/stream"
	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/Yacobolo/leapview/internal/workload"
)

type consumerForwardingMetrics struct {
	fakeMetrics
	calls    int
	governed bool
	admitter bool
}

func (m *consumerForwardingMetrics) ExecuteConsumersPage(ctx context.Context, request consumer.Request, publish consumer.Publisher) error {
	m.calls++
	_, m.governed = dataquery.GovernorFromContext(ctx)
	_, m.admitter = workload.FromContext(ctx)
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 1})
	for _, target := range request.Targets {
		publish(consumer.Result{Target: target, Queries: 1})
	}
	return nil
}

func (m *consumerForwardingMetrics) QueryCompiledFilterOptions(ctx context.Context, _ string, _ dashboardfilter.OptionQuery) (dashboardfilter.OptionResult, error) {
	m.calls++
	_, m.governed = dataquery.GovernorFromContext(ctx)
	_, m.admitter = workload.FromContext(ctx)
	return dashboardfilter.OptionResult{Complete: true}, nil
}

func TestProductionDashboardWrappersForwardGovernedConsumerPlan(t *testing.T) {
	underlying := &consumerForwardingMetrics{}
	controller, err := workload.New(workload.DefaultConfig())
	if err != nil {
		t.Fatalf("new workload controller: %v", err)
	}
	t.Cleanup(controller.Close)
	metrics := dashboardCommandMetrics{QueryMetrics: queryAuditMetrics{QueryMetrics: workloadMetrics{
		QueryMetrics: queryauthz.New(underlying, queryauthz.Options{}),
		admitter:     controller,
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

	if underlying.calls != 1 || !underlying.governed || !underlying.admitter || visuals != 2 {
		t.Fatalf("calls=%d governed=%v admitter=%v visuals=%d", underlying.calls, underlying.governed, underlying.admitter, visuals)
	}
}

func TestProductionDashboardWrappersForwardGovernedFilterOptions(t *testing.T) {
	underlying := &consumerForwardingMetrics{}
	controller, err := workload.New(workload.DefaultConfig())
	if err != nil {
		t.Fatalf("new workload controller: %v", err)
	}
	t.Cleanup(controller.Close)
	metrics := dashboardCommandMetrics{QueryMetrics: queryAuditMetrics{QueryMetrics: workloadMetrics{
		QueryMetrics: queryauthz.New(underlying, queryauthz.Options{}),
		admitter:     controller,
	}}}

	result, err := metrics.QueryCompiledFilterOptions(context.Background(), "sales-dashboard", dashboardfilter.OptionQuery{Field: "orders.state"})
	if err != nil {
		t.Fatal(err)
	}
	if underlying.calls != 1 || !underlying.governed || !underlying.admitter || !result.Complete {
		t.Fatalf("calls=%d governed=%v admitter=%v complete=%v", underlying.calls, underlying.governed, underlying.admitter, result.Complete)
	}
}
