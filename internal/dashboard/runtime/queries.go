package runtime

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/libredash/internal/dashboard/definition"
	visualizationdefinition "github.com/Yacobolo/libredash/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/libredash/internal/visualization/ir"
)

type QueryService struct {
	snapshots      *SnapshotService
	visualizations *VisualizationDataService
}

type SnapshotService struct {
	mu             *sync.RWMutex
	reports        *ReportService
	runtimes       map[string]*modelRuntime
	filters        *FilterService
	visualizations *VisualizationDataService
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
	return s.visualizations.QueryTable(ctx, dashboardID, filters, request)
}

func (s *QueryService) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return s.visualizations.QueryTablePage(ctx, dashboardID, pageID, filters, request)
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
			Loading:         false,
			LastUpdated:     refreshLabel(runtime),
			ProgressPercent: dashboard.NormalizeProgressPercent(nil, false),
		},
		Visuals: map[string]visualizationir.VisualizationEnvelope{},
	}

	page := dashboardPage(report, pageID)
	options, err := s.filters.filterOptions(ctx, runtime, report, report.PageFilterIDs(page.ID))
	if err != nil {
		return dashboard.EmptyPatch(filters, err), nil
	}
	patch.FilterOptions = options

	visuals, err := s.visualizations.visuals(ctx, runtime, report, filters, pageVisualIDs(page))
	if err != nil {
		return dashboard.EmptyPatch(filters, err), nil
	}
	patch.Visuals = visuals

	return patch, nil
}

func (s *SnapshotService) queryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (visualizationir.VisualizationEnvelope, error) {
	visuals, err := s.queryVisualsPage(ctx, dashboardID, pageID, filters, []string{visualID})
	if err != nil {
		return visualizationir.VisualizationEnvelope{}, err
	}
	return visuals[visualID], nil
}

func (s *SnapshotService) querySpatialVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.SpatialWindowRequest) (visualizationir.VisualizationEnvelope, error) {
	report, runtime, err := s.reports.reportRuntime(dashboardID, s.runtimes)
	if err != nil {
		return visualizationir.VisualizationEnvelope{}, err
	}
	if !runtime.ready {
		return visualizationir.VisualizationEnvelope{}, runtime.missing
	}
	page := dashboardPage(report, pageID)
	filters = report.NormalizeFiltersForPage(page.ID, filters)
	if !contains(pageVisualIDs(page), request.VisualID) {
		return visualizationir.VisualizationEnvelope{}, fmt.Errorf("visual %q is not on page %q", request.VisualID, page.ID)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.visualizations.spatialEnvelope(ctx, runtime, report, filters, request)
}

func (s *SnapshotService) queryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]visualizationir.VisualizationEnvelope, error) {
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
	return s.visualizations.visuals(ctx, runtime, report, filters, append([]string{}, visualIDs...))
}

func (s *SnapshotService) queryVisualBundlePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]visualizationir.VisualizationEnvelope, error) {
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
	return s.visualizations.bundledVisuals(ctx, runtime, report, filters, append([]string{}, visualIDs...))
}

func (s *SnapshotService) queryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
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

func dashboardPage(report *dashboarddefinition.Definition, pageID string) dashboard.Page {
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

func pageTableIDs(page dashboard.Page) []string {
	seen := map[string]struct{}{}
	ids := []string{}
	for _, item := range page.Visuals {
		if item.Table == "" {
			continue
		}
		if _, ok := seen[item.Table]; ok {
			continue
		}
		seen[item.Table] = struct{}{}
		ids = append(ids, item.Table)
	}
	sort.Strings(ids)
	return ids
}

func (s *VisualizationDataService) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return s.QueryTablePage(ctx, dashboardID, "", filters, request)
}

func (s *VisualizationDataService) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return s.queryTablePage(ctx, dashboardID, pageID, filters, request, true)
}

// queryTableRowsPage returns the requested table window without making an
// exact count part of the row-query critical path. Callers that progressively
// render tables can publish this payload before resolving the total.
func (s *VisualizationDataService) queryTableRowsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return s.queryTablePage(ctx, dashboardID, pageID, filters, request, false)
}

// queryTableCountPage resolves exact governed table cardinality independently
// from the row window so it can be cached and delivered as secondary metadata.
func (s *VisualizationDataService) queryTableCountPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (int, error) {
	report, runtime, err := s.reports.reportRuntime(dashboardID, s.runtimes)
	if report != nil {
		page := dashboardPage(report, pageID)
		filters = report.NormalizeFiltersForPage(page.ID, filters)
	} else {
		filters = filters.WithDefaults()
	}
	request = s.reports.NormalizeTableRequest(dashboardID, request)
	if err != nil {
		return 0, err
	}
	if !runtime.ready {
		return 0, runtime.missing
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	tableModel, ok := report.Visualizations[request.Table]
	if !ok {
		return 0, fmt.Errorf("unknown table %q", request.Table)
	}
	if tableModel.Query.Kind == visualizationdefinition.QueryMatrix || tableModel.Query.Kind == visualizationdefinition.QueryPivot {
		table, err := s.queryAggregateTable(ctx, runtime, report, request, tableModel, filters)
		total, _ := table.Cardinality.ExactValue()
		return total, err
	}
	return s.queryDataTableCount(ctx, runtime, report, request, tableModel, filters)
}

func (s *VisualizationDataService) queryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest, includeTotal bool) (dashboard.Table, error) {
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

	tableModel, ok := report.Visualizations[request.Table]
	if !ok {
		return dashboard.EmptyTable(request, fmt.Errorf("unknown table %q", request.Table)), nil
	}
	if tableModel.Query.Kind == visualizationdefinition.QueryMatrix || tableModel.Query.Kind == visualizationdefinition.QueryPivot {
		return s.queryAggregateTable(ctx, runtime, report, request, tableModel, filters)
	}
	table, err := s.queryDataTableWindow(ctx, runtime, report, request, tableModel, filters)
	_, totalKnown := table.Cardinality.ExactValue()
	if err != nil || !includeTotal || table.Error != "" || totalKnown {
		return table, err
	}
	totalRows, err := s.queryDataTableCount(ctx, runtime, report, request, tableModel, filters)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	applyTableTotal(&table, totalRows)
	return table, nil
}

func (s *VisualizationDataService) queryDataTableWindow(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, request dashboard.TableRequest, definition visualizationdefinition.Definition, filters dashboard.Filters) (dashboard.Table, error) {
	tableModel, err := newTablePlan(definition)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	count := request.Count
	if count <= 0 {
		count = dashboard.TableChunkSize
	}
	if count > dashboard.TableMaxRequestCount {
		count = dashboard.TableMaxRequestCount
	}
	start := request.Start
	queryCount := count
	blockStarts := map[string]int{request.Block: start}
	if request.Block == "all" {
		currentStart := max(0, (start/count)*count)
		if currentStart == 0 {
			blockStarts = map[string]int{"a": 0, "b": count, "c": count * 2}
		} else {
			blockStarts = map[string]int{"a": max(0, currentStart-count), "b": currentStart, "c": currentStart + count}
		}
		start = blockStarts["a"]
		queryCount = count * 3
	}
	rowRequest, err := s.tableRowRequest(ctx, runtime, report, tableModel, filters, request, start, queryCount)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	result, err := runtime.data.ExecuteDataQuery(ctx, reportRowDataQuery(report.SemanticModel, rowRequest, false))
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	rows := tableRowsFromAnalytics(reportRowsFromDataQuery(result.Rows))
	// A short first page proves the exact cardinality. At a non-zero offset it
	// only proves that the requested window reached or overshot the end; the
	// true count may be lower than start and must remain unknown.
	totalRowsKnown := start == 0 && len(rows) < queryCount
	totalRows := 0
	cardinality := dashboard.TableCardinality{Kind: dashboard.CardinalityUnknown}
	availableRows := dashboard.TableInteractiveRowCap
	if totalRowsKnown {
		totalRows = start + len(rows)
		cardinality = dashboard.ExactCardinality(totalRows)
		availableRows = min(totalRows, dashboard.TableInteractiveRowCap)
	} else if len(rows) > 0 {
		cardinality = dashboard.LowerBoundCardinality(start + len(rows))
	}
	blocks := make(map[string]dashboard.TableBlock, len(blockStarts))
	for _, block := range []string{"a", "b", "c"} {
		blockStart, ok := blockStarts[block]
		if !ok {
			continue
		}
		rowStart := min(len(rows), max(0, blockStart-start))
		rowEnd := min(len(rows), rowStart+count)
		blocks[block] = dashboard.TableBlock{
			Start: blockStart, RequestSeq: request.RequestSeq, ResetVersion: request.ResetVersion,
			Sort: request.Sort, Rows: rows[rowStart:rowEnd],
		}
	}
	if request.Block != "all" {
		blocks[request.Block] = dashboard.TableBlock{
			Start: start, RequestSeq: request.RequestSeq, ResetVersion: request.ResetVersion,
			Sort: request.Sort, Rows: rows,
		}
	}
	style := tableModel.Style.WithDefaults()
	return dashboard.Table{
		Version:       2,
		Kind:          tableModel.Kind,
		Title:         tableModel.Title,
		Style:         style,
		Interaction:   tableModel.Interaction,
		Selection:     []dashboard.InteractionSelectionEntry{},
		Columns:       tableModel.Columns,
		Cardinality:   cardinality,
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

func (s *VisualizationDataService) queryDataTableCount(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, request dashboard.TableRequest, definition visualizationdefinition.Definition, filters dashboard.Filters) (int, error) {
	tableModel, err := newTablePlan(definition)
	if err != nil {
		return 0, err
	}
	rowRequest, err := s.tableRowRequest(ctx, runtime, report, tableModel, filters, request, 0, 1)
	if err != nil {
		return 0, err
	}
	query := countOnlyDataQuery(reportRowDataQuery(report.SemanticModel, rowRequest, true))
	result, err := runtime.data.ExecuteDataQuery(ctx, query)
	if err != nil {
		return 0, err
	}
	if !result.TotalRowsKnown {
		return 0, fmt.Errorf("table count query did not return total rows")
	}
	return result.TotalRows, nil
}

func applyTableTotal(table *dashboard.Table, totalRows int) {
	table.Cardinality = dashboard.ExactCardinality(totalRows)
	table.AvailableRows = min(totalRows, dashboard.TableInteractiveRowCap)
	table.IsCapped = totalRows > table.AvailableRows
}

func refreshLabel(runtime *modelRuntime) string {
	if runtime.data == nil || runtime.data.LastRefresh().IsZero() {
		return time.Now().Format("15:04:05")
	}
	return runtime.data.LastRefresh().Format("15:04:05")
}
