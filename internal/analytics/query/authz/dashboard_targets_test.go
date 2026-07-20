package authz

import (
	"context"
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard/consumer"
	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/Yacobolo/leapview/internal/queryruntime"
)

type targetMetrics struct {
	queryruntime.Metrics
	governed bool
}

func (m *targetMetrics) ExecuteConsumersPage(ctx context.Context, _ consumer.Request, _ consumer.Publisher) error {
	_, m.governed = dataquery.GovernorFromContext(ctx)
	return nil
}

func TestTargetedDashboardQueriesPreserveGovernor(t *testing.T) {
	underlying := &targetMetrics{}
	metrics := New(underlying, Options{})
	if err := metrics.ExecuteConsumersPage(context.Background(), consumer.Request{DashboardID: "dash"}, func(consumer.Result) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if !underlying.governed {
		t.Fatal("targeted visual query did not receive the authorization governor")
	}
}
