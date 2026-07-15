package stream

import (
	"context"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
)

type Metrics interface {
	report.Metrics
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
}

type Service struct {
	Metrics Metrics
}

type SnapshotRequest struct {
	DashboardID  string
	PageID       string
	Filters      dashboard.Filters
	TableCommand dashboard.TableRequest
}

type Snapshot struct {
	Patch  dashboard.Patch
	Tables map[string]dashboard.Table
}

func (s Service) Snapshot(ctx context.Context, request SnapshotRequest) Snapshot {
	filters := report.NormalizeFilters(s.Metrics, request.DashboardID, request.PageID, request.Filters)
	tableRequest := s.Metrics.NormalizeTableRequest(request.DashboardID, request.TableCommand)
	patch, err := s.Metrics.QueryDashboardPage(ctx, request.DashboardID, request.PageID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, err)
	}
	return Snapshot{
		Patch:  patch,
		Tables: report.Tables(ctx, s.Metrics, request.DashboardID, request.PageID, filters, tableRequest),
	}
}
