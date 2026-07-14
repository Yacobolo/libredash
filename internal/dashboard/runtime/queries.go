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
	dataDir  string
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

func (m *Service) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	return m.queries.QueryVisualPage(ctx, dashboardID, pageID, filters, visualID)
}

func (m *Service) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	return m.queries.QueryVisualsPage(ctx, dashboardID, pageID, filters, visualIDs)
}

func (m *Service) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	return m.queries.QueryFilterOptionsPage(ctx, dashboardID, pageID, filterIDs)
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

func (s *QueryService) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	return s.snapshots.QueryVisualPage(ctx, dashboardID, pageID, filters, visualID)
}

func (s *QueryService) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	return s.snapshots.QueryVisualsPage(ctx, dashboardID, pageID, filters, visualIDs)
}

func (s *QueryService) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	return s.snapshots.QueryFilterOptionsPage(ctx, dashboardID, pageID, filterIDs)
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
		return dashboard.EmptyPatch(filters, s.dataDir, err), nil
	}
	if !runtime.ready {
		return dashboard.EmptyPatch(filters, s.dataDir, runtime.missing), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	patch := dashboard.Patch{
		Filters: filters,
		Status: dashboard.Status{
			Loading:       false,
			LastUpdated:   refreshLabel(runtime),
			DataDirectory: s.dataDir,
		},
		Visuals: map[string]dashboard.Visual{},
	}

	page := dashboardPage(report, pageID)
	options, err := s.filters.filterOptions(ctx, runtime, report, report.PageFilterIDs(page.ID))
	if err != nil {
		return dashboard.EmptyPatch(filters, s.dataDir, err), nil
	}
	patch.FilterOptions = options

	visuals, err := s.visuals.visuals(ctx, runtime, report, filters, pageVisualIDs(page))
	if err != nil {
		return dashboard.EmptyPatch(filters, s.dataDir, err), nil
	}
	patch.Visuals = visuals

	return patch, nil
}

func (s *SnapshotService) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	visuals, err := s.QueryVisualsPage(ctx, dashboardID, pageID, filters, []string{visualID})
	if err != nil {
		return dashboard.Visual{}, err
	}
	return visuals[visualID], nil
}

func (s *SnapshotService) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	report, runtime, err := s.reports.reportRuntime(dashboardID, s.runtimes)
	if err != nil {
		return nil, err
	}
	if !runtime.ready {
		return nil, runtime.missing
	}
	page := dashboardPage(report, pageID)
	filters = report.NormalizeFiltersForPage(page.ID, filters)
	pageIDs := pageVisualIDs(page)
	for _, visualID := range visualIDs {
		if !contains(pageIDs, visualID) {
			return nil, fmt.Errorf("visual %q is not on page %q", visualID, page.ID)
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.visuals.visuals(ctx, runtime, report, filters, append([]string{}, visualIDs...))
}

func (s *SnapshotService) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	report, runtime, err := s.reports.reportRuntime(dashboardID, s.runtimes)
	if err != nil {
		return nil, err
	}
	if !runtime.ready {
		return nil, runtime.missing
	}
	page := dashboardPage(report, pageID)
	pageFilterIDs := report.PageFilterIDs(page.ID)
	allowed := make(map[string]struct{}, len(pageFilterIDs))
	for _, filterID := range pageFilterIDs {
		allowed[filterID] = struct{}{}
	}
	if len(filterIDs) == 0 {
		filterIDs = pageFilterIDs
	}
	for _, filterID := range filterIDs {
		if _, ok := allowed[filterID]; !ok {
			return nil, fmt.Errorf("filter %q is not on page %q", filterID, page.ID)
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filters.filterOptions(ctx, runtime, report, filterIDs)
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
	return s.queryDataTableWindow(ctx, runtime, report, request, tableModel, filters)
}

func (s *TableQueryService) queryDataTableWindow(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, request dashboard.TableRequest, tableModel reportdef.TableVisual, filters dashboard.Filters) (dashboard.Table, error) {
	count := request.Count
	if count <= 0 {
		count = dashboard.TableChunkSize
	}
	if count > dashboard.TableMaxRequestCount {
		count = dashboard.TableMaxRequestCount
	}
	start := request.Start
	if request.Block == "all" {
		start = 0
	}
	rowRequest, err := s.tableRowRequest(ctx, runtime, report, tableModel, filters, request, start, count)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	result, err := runtime.data.ExecuteDataQuery(ctx, reportRowDataQuery(report.SemanticModel, rowRequest, true))
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	totalRows := result.TotalRows
	if !result.TotalRowsKnown {
		return dashboard.EmptyTable(request, fmt.Errorf("initial table query did not return total rows")), nil
	}
	availableRows := min(totalRows, dashboard.TableInteractiveRowCap)
	rows := tableRowsFromAnalytics(reportRowsFromDataQuery(result.Rows))
	block := request.Block
	if block == "all" {
		block = "a"
	}
	style := tableModel.Style.WithDefaults()
	return dashboard.Table{
		Version:       2,
		Kind:          tableModel.KindOrDefault(),
		Title:         tableModel.Title,
		Style:         style,
		Interaction:   tableInteractionConfig(tableModel.Interaction.RowSelection),
		Selection:     []dashboard.InteractionSelectionEntry{},
		Columns:       tableModel.Columns,
		TotalRows:     totalRows,
		AvailableRows: availableRows,
		IsCapped:      totalRows > availableRows,
		RowCap:        dashboard.TableInteractiveRowCap,
		ChunkSize:     dashboard.TableChunkSize,
		RowHeight:     style.RowHeight(),
		ResetVersion:  request.ResetVersion,
		Sort:          request.Sort,
		Blocks: map[string]dashboard.TableBlock{
			block: {Start: start, RequestSeq: request.RequestSeq, ResetVersion: request.ResetVersion, Sort: request.Sort, Rows: rows},
		},
		LoadingBlock: "",
		Error:        "",
	}, nil
}

func refreshLabel(runtime *modelRuntime) string {
	if runtime.data == nil || runtime.data.LastRefresh().IsZero() {
		return time.Now().Format("15:04:05")
	}
	return runtime.data.LastRefresh().Format("15:04:05")
}
