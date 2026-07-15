package command

import (
	"context"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
)

type Metrics interface {
	report.Metrics
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
	RefreshMaterializations(ctx context.Context, modelID string) error
}

type Service struct {
	Metrics Metrics
}

type Request struct {
	DashboardID        string
	PageID             string
	ModelID            string
	Filters            dashboard.Filters
	TableCommand       dashboard.TableRequest
	InteractionCommand dashboard.InteractionCommand
}

type EventType string

const (
	EventLoading   EventType = "loading"
	EventDashboard EventType = "dashboard"
	EventTables    EventType = "tables"
	EventTable     EventType = "table"
)

type Event struct {
	Type      EventType
	Patch     dashboard.Patch
	Tables    map[string]dashboard.Table
	TableName string
	Table     dashboard.Table
}

func (s Service) TableWindow(ctx context.Context, request Request) []Event {
	tableRequest := s.Metrics.NormalizeTableRequest(request.DashboardID, request.TableCommand)
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, request.Filters)
	table := report.QueryTable(ctx, s.Metrics, request.DashboardID, request.PageID, filters, tableRequest)
	if report.IsCanceledTable(table) {
		return nil
	}
	return []Event{{
		Type:      EventTable,
		TableName: tableRequest.Table,
		Table:     table,
	}}
}

func (s Service) Select(ctx context.Context, request Request) []Event {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, request.Filters).ApplyInteraction(request.InteractionCommand)
	filters = report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, filters)
	return s.reload(ctx, request, filters)
}

func (s Service) ClearSelection(ctx context.Context, request Request) []Event {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, request.Filters)
	filters.Selections = nil
	return s.reload(ctx, request, filters)
}

func (s Service) Reload(ctx context.Context, request Request) []Event {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, request.Filters)
	return s.reload(ctx, request, filters)
}

func (s Service) ResetFilters(ctx context.Context, request Request) []Event {
	return s.reload(ctx, request, report.DefaultFilters(s.Metrics, request.DashboardID, request.PageID))
}

func (s Service) RefreshMaterializations(ctx context.Context, request Request) []Event {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, request.Filters)
	events := []Event{{Type: EventLoading}}
	if err := s.Metrics.RefreshMaterializations(ctx, request.ModelID); err != nil {
		events = append(events, Event{
			Type:  EventDashboard,
			Patch: dashboard.EmptyPatch(filters, err),
		})
		return events
	}
	return append(events, s.reloadEvents(ctx, request, filters)...)
}

func (s Service) reload(ctx context.Context, request Request, filters dashboard.Filters) []Event {
	events := []Event{{Type: EventLoading}}
	return append(events, s.reloadEvents(ctx, request, filters)...)
}

func (s Service) reloadEvents(ctx context.Context, request Request, filters dashboard.Filters) []Event {
	tableRequest := s.Metrics.NormalizeTableRequest(request.DashboardID, request.TableCommand).Reset()
	patch, err := s.Metrics.QueryDashboardPage(ctx, request.DashboardID, request.PageID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, err)
	}
	return []Event{
		{Type: EventDashboard, Patch: patch},
		{Type: EventTables, Tables: report.Tables(ctx, s.Metrics, request.DashboardID, request.PageID, filters, tableRequest)},
	}
}
