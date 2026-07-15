package runtime

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
)

type QueryService struct {
	snapshots *SnapshotService
	tables    *TableQueryService
}

type SnapshotService struct {
	mu       *sync.RWMutex
	reports  *ReportService
	runtimes map[string]*modelRuntime
	filters  *FilterService
	visuals  *VisualQueryService
}

type TableQueryService struct {
	mu       *sync.RWMutex
	reports  *ReportService
	runtimes map[string]*modelRuntime
	filters  *FilterService
}

func (m *Service) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.queries.QueryDashboard(ctx, dashboardID, filters)
}

func (m *Service) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.queries.QueryDashboardPage(ctx, dashboardID, pageID, filters)
}

func (m *Service) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.queries.QueryTable(ctx, dashboardID, filters, request)
}

func (m *Service) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.queries.QueryTablePage(ctx, dashboardID, pageID, filters, request)
}

func (s *QueryService) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return s.snapshots.QueryDashboard(ctx, dashboardID, filters)
}

func (s *QueryService) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return s.snapshots.QueryDashboardPage(ctx, dashboardID, pageID, filters)
}

func (s *QueryService) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return s.tables.QueryTable(ctx, dashboardID, filters, request)
}

func (s *QueryService) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return s.tables.QueryTablePage(ctx, dashboardID, pageID, filters, request)
}

func (s *SnapshotService) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return s.QueryDashboardPage(ctx, dashboardID, "", filters)
}

func (s *SnapshotService) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	report, runtime, err := s.reports.reportRuntime(dashboardID, s.runtimes)
	if report != nil {
		page := dashboardPage(report, pageID)
		filters = report.NormalizeFiltersForPage(page.ID, filters)
	} else {
		filters = filters.WithDefaults()
	}
	if err != nil {
		return dashboard.EmptyPatch(filters, err), nil
	}
	if !runtime.ready {
		return dashboard.EmptyPatch(filters, runtime.missing), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	patch := dashboard.Patch{
		Filters: filters,
		Status: dashboard.Status{
			Loading:     false,
			LastUpdated: refreshLabel(runtime),
		},
		Visuals: map[string]dashboard.Visual{},
	}

	page := dashboardPage(report, pageID)
	options, err := s.filters.filterOptions(ctx, runtime, report, report.PageFilterIDs(page.ID))
	if err != nil {
		return dashboard.EmptyPatch(filters, err), nil
	}
	patch.FilterOptions = options

	visuals, err := s.visuals.visuals(ctx, runtime, report, filters, pageVisualIDs(page))
	if err != nil {
		return dashboard.EmptyPatch(filters, err), nil
	}
	patch.Visuals = visuals

	return patch, nil
}

func dashboardPage(report *reportdef.Dashboard, pageID string) dashboard.Page {
	if report == nil || len(report.Pages) == 0 {
		return dashboard.Page{}
	}
	if pageID != "" {
		for _, page := range report.Pages {
			if page.ID == pageID {
				return page.WithDefaults()
			}
		}
	}
	return report.Pages[0].WithDefaults()
}

func pageVisualIDs(page dashboard.Page) []string {
	seen := map[string]struct{}{}
	ids := []string{}
	for _, item := range page.Visuals {
		if item.Visual == "" {
			continue
		}
		if _, ok := seen[item.Visual]; ok {
			continue
		}
		seen[item.Visual] = struct{}{}
		ids = append(ids, item.Visual)
	}
	sort.Strings(ids)
	return ids
}

func (s *TableQueryService) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return s.QueryTablePage(ctx, dashboardID, "", filters, request)
}

func (s *TableQueryService) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	report, runtime, err := s.reports.reportRuntime(dashboardID, s.runtimes)
	if report != nil {
		page := dashboardPage(report, pageID)
		filters = report.NormalizeFiltersForPage(page.ID, filters)
	} else {
		filters = filters.WithDefaults()
	}
	request = s.reports.NormalizeTableRequest(dashboardID, request)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	if !runtime.ready {
		return dashboard.EmptyTable(request, runtime.missing), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	tableModel, ok := report.Tables[request.Table]
	if !ok {
		return dashboard.EmptyTable(request, fmt.Errorf("unknown table %q", request.Table)), nil
	}
	if tableModel.KindOrDefault() == "matrix_table" || tableModel.KindOrDefault() == "pivot_table" {
		return s.queryAggregateTable(ctx, runtime, report, request, tableModel, filters)
	}

	totalRows, err := s.filters.countRows(ctx, runtime, report, tableModel.Query.Table, filters, "table", request.Table)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	availableRows := min(totalRows, dashboard.TableInteractiveRowCap)
	blocks, err := s.tableBlocks(ctx, runtime, report, tableModel, filters, request, availableRows)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}

	style := tableModel.Style.WithDefaults()
	return dashboard.Table{
		Version:       2,
		Kind:          tableModel.KindOrDefault(),
		Title:         tableModel.Title,
		Style:         style,
		Interaction:   tableInteractionConfig(tableModel.Interaction.RowSelection),
		Selection:     selectedEntries(filters, "table", request.Table),
		Columns:       tableModel.Columns,
		TotalRows:     totalRows,
		AvailableRows: availableRows,
		IsCapped:      totalRows > availableRows,
		RowCap:        dashboard.TableInteractiveRowCap,
		ChunkSize:     dashboard.TableChunkSize,
		RowHeight:     style.RowHeight(),
		ResetVersion:  request.ResetVersion,
		Sort:          request.Sort,
		Blocks:        blocks,
		LoadingBlock:  "",
		Error:         "",
	}, nil
}

func refreshLabel(runtime *modelRuntime) string {
	if runtime.data == nil || runtime.data.LastRefresh().IsZero() {
		return time.Now().Format("15:04:05")
	}
	return runtime.data.LastRefresh().Format("15:04:05")
}
