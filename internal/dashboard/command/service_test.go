package command

import (
	"context"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
)

type fakeMetrics struct {
	canceledTable bool
	queries       []string
}

func (fakeMetrics) DataDir() string { return ".data" }
func (fakeMetrics) RefreshMaterializations(context.Context, string) error {
	return nil
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
		Tables:  map[string]reportdef.TableVisual{"orders": {}},
		Pages:   []dashboard.Page{{ID: "overview", Visuals: []dashboard.PageVisual{{Kind: "table", Table: "orders"}}}},
	}, &semanticmodel.Model{Name: "model"}, true
}
func (m *fakeMetrics) QueryDashboardPage(_ context.Context, _, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	m.queries = append(m.queries, pageID)
	return dashboard.Patch{Filters: filters.WithDefaults(), Status: dashboard.Status{DataDirectory: ".data"}}, nil
}
func (m *fakeMetrics) QueryTablePage(_ context.Context, _, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	if m.canceledTable {
		return dashboard.EmptyTable(request, context.Canceled), nil
	}
	return dashboard.Table{Title: request.Table, Blocks: map[string]dashboard.TableBlock{"a": {Rows: []map[string]any{{"id": "1"}}}}}, nil
}

func TestTableWindowReturnsTableEvent(t *testing.T) {
	metrics := &fakeMetrics{}

	events := Service{Metrics: metrics}.TableWindow(context.Background(), Request{
		DashboardID:  "dash",
		PageID:       "overview",
		TableCommand: dashboard.TableRequest{Table: "orders", Block: "a", Start: 50, Count: 50},
	})

	if len(events) != 1 || events[0].Type != EventTable || events[0].TableName != "orders" {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Table.Title != "orders" {
		t.Fatalf("table event = %#v", events[0])
	}
}

func TestTableWindowSkipsCanceledTableEvent(t *testing.T) {
	events := Service{Metrics: &fakeMetrics{canceledTable: true}}.TableWindow(context.Background(), Request{
		DashboardID:  "dash",
		PageID:       "overview",
		TableCommand: dashboard.TableRequest{Table: "orders", Block: "a", Start: 50, Count: 50},
	})

	if len(events) != 0 {
		t.Fatalf("unexpected canceled table events: %#v", events)
	}
}

func TestSelectReturnsReloadEventsForActivePage(t *testing.T) {
	metrics := &fakeMetrics{}

	events := Service{Metrics: metrics}.Select(context.Background(), Request{
		DashboardID:  "dash",
		PageID:       "overview",
		Filters:      dashboard.Filters{Selections: []dashboard.InteractionSelection{}},
		TableCommand: dashboard.TableRequest{Table: "orders", Block: "a", Start: 50, Count: 50},
		InteractionCommand: dashboard.InteractionCommand{
			SourceKind:      "visual",
			SourceID:        "chart",
			InteractionKind: "point_selection",
			Action:          "set",
			Toggle:          true,
			Mappings: []dashboard.InteractionCommandMapping{{
				Field: "state",
				Value: "SP",
				Label: "SP",
			}},
		},
	})

	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	for i, kind := range []EventType{EventLoading, EventDashboard, EventTables} {
		if events[i].Type != kind {
			t.Fatalf("event %d = %#v, want %s", i, events[i], kind)
		}
	}
	if len(metrics.queries) != 1 || metrics.queries[0] != "overview" {
		t.Fatalf("queries = %#v", metrics.queries)
	}
}
