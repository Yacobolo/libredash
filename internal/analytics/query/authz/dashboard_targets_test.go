package authz

import (
	"context"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/queryruntime"
)

type targetMetrics struct {
	queryruntime.Metrics
	governed bool
}

func (m *targetMetrics) QueryVisualPage(ctx context.Context, _, _ string, _ dashboard.Filters, id string) (dashboard.Visual, error) {
	_, m.governed = dataquery.GovernorFromContext(ctx)
	return dashboard.Visual{ID: id}, nil
}
func (m *targetMetrics) QueryVisualsPage(ctx context.Context, _, _ string, _ dashboard.Filters, ids []string) (map[string]dashboard.Visual, error) {
	_, m.governed = dataquery.GovernorFromContext(ctx)
	return map[string]dashboard.Visual{}, nil
}
func (m *targetMetrics) QueryFilterOptionsPage(ctx context.Context, _, _ string, _ []string) (map[string][]dashboard.FilterOption, error) {
	_, m.governed = dataquery.GovernorFromContext(ctx)
	return map[string][]dashboard.FilterOption{}, nil
}

func TestTargetedDashboardQueriesPreserveGovernor(t *testing.T) {
	underlying := &targetMetrics{}
	metrics := New(underlying, Options{})
	if _, err := metrics.QueryVisualPage(context.Background(), "dash", "page", dashboard.Filters{}, "orders"); err != nil {
		t.Fatal(err)
	}
	if !underlying.governed {
		t.Fatal("targeted visual query did not receive the authorization governor")
	}
	underlying.governed = false
	if _, err := metrics.QueryFilterOptionsPage(context.Background(), "dash", "page", []string{"state"}); err != nil {
		t.Fatal(err)
	}
	if !underlying.governed {
		t.Fatal("targeted filter option query did not receive the authorization governor")
	}
}
