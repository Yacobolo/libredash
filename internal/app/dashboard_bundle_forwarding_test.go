package app

import (
	"context"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	queryauthz "github.com/Yacobolo/libredash/internal/analytics/query/authz"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/command"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	dashboardstream "github.com/Yacobolo/libredash/internal/dashboard/stream"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type bundleForwardingMetrics struct {
	fakeMetrics
	bundleCalls   int
	fallbackCalls int
	governed      bool
}

func (m *bundleForwardingMetrics) Report(string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	return reportdef.Dashboard{
			SemanticModel: "sales",
			Visuals: map[string]reportdef.Visual{
				"orders": {
					Shape: "single_value",
					Query: reportdef.VisualQuery{Table: "orders", Measures: []reportdef.FieldRef{{Field: "order_count"}}},
				},
				"revenue": {
					Shape: "single_value",
					Query: reportdef.VisualQuery{Table: "orders", Measures: []reportdef.FieldRef{{Field: "revenue"}}},
				},
			},
		}, &semanticmodel.Model{
			Tables: map[string]semanticmodel.Table{"orders": {}},
			Measures: map[string]semanticmodel.MetricMeasure{
				"order_count": {Fact: "orders", Aggregation: "count", Empty: "zero"},
				"revenue":     {Fact: "orders", Aggregation: "sum", Input: semanticmodel.MeasureInput{Field: "orders.amount"}, Empty: "zero"},
			},
		}, true
}

func (m *bundleForwardingMetrics) QueryVisualBundlePage(ctx context.Context, _, _ string, _ dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	m.bundleCalls++
	_, m.governed = dataquery.GovernorFromContext(ctx)
	dataquery.ObservePhysicalQuery(ctx, dataquery.PhysicalQueryObservation{Count: 1})
	visuals := make(map[string]dashboard.Visual, len(visualIDs))
	for _, id := range visualIDs {
		visuals[id] = dashboard.Visual{ID: id}
	}
	return visuals, nil
}

func (m *bundleForwardingMetrics) QueryVisualsPage(context.Context, string, string, dashboard.Filters, []string) (map[string]dashboard.Visual, error) {
	m.fallbackCalls++
	return nil, nil
}

func TestProductionDashboardWrappersForwardGovernedVisualBundlesToTargetWork(t *testing.T) {
	underlying := &bundleForwardingMetrics{}
	metrics := dashboardCommandMetrics{QueryMetrics: queryAuditMetrics{QueryMetrics: executionMetrics{
		QueryMetrics: queryauthz.New(underlying, queryauthz.Options{}),
	}}}

	queryCount := 0
	visuals := 0
	dashboardstream.TargetWork(metrics, dashboardstream.WorkRequest{
		DashboardID: "sales-dashboard",
		PageID:      "overview",
		Plan: command.RefreshPlan{Targets: []command.Target{
			{Kind: command.TargetVisual, ID: "orders"},
			{Kind: command.TargetVisual, ID: "revenue"},
		}},
	})(context.Background(), func(event dashboardstream.RefreshEvent) bool {
		queryCount += event.Queries
		if event.Type == dashboardstream.RefreshEventVisual {
			visuals++
		}
		return true
	})

	if underlying.bundleCalls != 1 || underlying.fallbackCalls != 0 {
		t.Fatalf("bundle calls=%d fallback calls=%d, want 1 and 0", underlying.bundleCalls, underlying.fallbackCalls)
	}
	if !underlying.governed {
		t.Fatal("bundle query bypassed the authorization governor")
	}
	if queryCount != 1 || visuals != 2 {
		t.Fatalf("physical queries=%d visual events=%d, want 1 and 2", queryCount, visuals)
	}
}
