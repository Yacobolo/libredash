package stream

import (
	"context"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/command"
	"github.com/Yacobolo/libredash/internal/dashboard/consumer"
)

type unifiedConsumerMetrics struct {
	called bool
}

func (*unifiedConsumerMetrics) DataDir() string { return "" }

func (*unifiedConsumerMetrics) QueryTablePage(context.Context, string, string, dashboard.Filters, dashboard.TableRequest) (dashboard.Table, error) {
	return dashboard.Table{}, nil
}

func (m *unifiedConsumerMetrics) ExecuteConsumersPage(_ context.Context, request consumer.Request, publish consumer.Publisher) error {
	m.called = true
	for _, target := range request.Targets {
		publish(consumer.Result{Target: target, Visual: dashboard.Visual{ID: target.ID}})
	}
	return nil
}

func TestTargetWorkDelegatesTheWholePlanToUnifiedConsumerExecutor(t *testing.T) {
	metrics := &unifiedConsumerMetrics{}
	request := WorkRequest{
		DashboardID: "sales",
		PageID:      "overview",
		Filters:     dashboard.Filters{},
		Plan: command.RefreshPlan{Command: "select", Targets: []command.Target{
			{Kind: command.TargetVisual, ID: "revenue"},
			{Kind: command.TargetVisual, ID: "orders"},
		}},
	}
	events := []RefreshEvent{}
	TargetWork(metrics, request)(context.Background(), func(event RefreshEvent) bool {
		events = append(events, event)
		return true
	})
	if !metrics.called {
		t.Fatal("unified consumer executor was not called")
	}
	if len(events) != 2 || events[0].Target != "revenue" || events[1].Target != "orders" {
		t.Fatalf("events = %#v", events)
	}
}
