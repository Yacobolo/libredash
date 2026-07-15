package stream

import (
	"context"
	"errors"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
)

type fakeMetrics struct {
	queryErr error
}

func (fakeMetrics) DefaultFilters(string) dashboard.Filters {
	return dashboard.Filters{Controls: map[string]dashboard.FilterControl{"state": {Type: "multi_select", Operator: "in"}}}
}
func (fakeMetrics) NormalizeTableRequest(_ string, request dashboard.TableRequest) dashboard.TableRequest {
	if request.Table == "" {
		request.Table = "orders"
	}
	return request.WithDefaults()
}
func (fakeMetrics) Report(string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	return reportdef.Dashboard{
		Filters: map[string]reportdef.FilterDefinition{"state": {Type: "multi_select", Label: "State", Operator: "in"}},
		Tables:  map[string]reportdef.TableVisual{"orders": {DefaultSort: dashboard.TableSort{Key: "order_id", Direction: "asc"}}},
		Pages: []dashboard.Page{{ID: "overview", Visuals: []dashboard.PageVisual{
			{Kind: "filter_card", Filter: "state"},
			{Kind: "table", Table: "orders"},
		}}},
	}, &semanticmodel.Model{Name: "model"}, true
}
func (m fakeMetrics) QueryDashboardPage(_ context.Context, _, _ string, filters dashboard.Filters) (dashboard.Patch, error) {
	if m.queryErr != nil {
		return dashboard.Patch{}, m.queryErr
	}
	return dashboard.Patch{
		Filters:       filters.WithDefaults(),
		FilterOptions: map[string][]dashboard.FilterOption{"state": {{Value: "SP", Label: "SP"}}},
		Status:        dashboard.Status{},
		Visuals:       map[string]dashboard.Visual{"orders_chart": {Title: "Orders"}},
	}, nil
}
func (fakeMetrics) QueryTablePage(_ context.Context, _, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return dashboard.Table{Title: request.Table}, nil
}

func TestSnapshotIncludesDashboardPatchAndTables(t *testing.T) {
	snapshot := Service{Metrics: fakeMetrics{}}.Snapshot(context.Background(), SnapshotRequest{
		DashboardID: "dash",
		PageID:      "overview",
	})

	if _, ok := snapshot.Patch.Visuals["orders_chart"]; !ok {
		t.Fatalf("snapshot patch = %#v", snapshot.Patch)
	}
	if snapshot.Tables["orders"].Title != "orders" {
		t.Fatalf("snapshot tables = %#v", snapshot.Tables)
	}
}

func TestSnapshotReturnsEmptyPatchOnDashboardQueryError(t *testing.T) {
	snapshot := Service{Metrics: fakeMetrics{queryErr: errors.New("boom")}}.Snapshot(context.Background(), SnapshotRequest{
		DashboardID: "dash",
		PageID:      "overview",
	})

	if snapshot.Patch.Status.Error == "" {
		t.Fatalf("snapshot patch = %#v", snapshot.Patch)
	}
}
