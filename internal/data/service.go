package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/deploy"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/semantic"
	_ "github.com/marcboeker/go-duckdb/v2"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var aggregateWrapperPattern = regexp.MustCompile(`(?is)^\s*(?:AVG|SUM|MIN|MAX|MEDIAN)\s*\((.+)\)\s*$`)

type MissingDataError struct {
	DataDir string
	Missing []string
}

func (e *MissingDataError) Error() string {
	return fmt.Sprintf("local source files are missing in %s: %s. Run the workspace bootstrap script or set LIBREDASH_DATA_DIR.", e.DataDir, strings.Join(e.Missing, ", "))
}

type DuckDBMetrics struct {
	mu         sync.RWMutex
	dataDir    string
	catalog    dashboard.Catalog
	workspace  *semantic.Workspace
	runtimes   map[string]*modelRuntime
	defaultID  string
	defaultMID string
}

type modelRuntime struct {
	db                  *sql.DB
	dbPath              string
	model               *semantic.Model
	ready               bool
	missing             error
	lastRefresh         time.Time
	attachedConnections map[string]struct{}
}

func NewDuckDBMetrics(dataDir string) (*DuckDBMetrics, error) {
	catalogPath := os.Getenv("LIBREDASH_CATALOG_PATH")
	if catalogPath == "" {
		var err error
		catalogPath, err = discoverCatalogPath()
		if err != nil {
			return nil, err
		}
	}
	duckDBDir := dataDir
	if path := os.Getenv("LIBREDASH_DUCKDB_DIR"); path != "" {
		duckDBDir = path
	}
	return NewDuckDBMetricsFromCatalog(dataDir, catalogPath, duckDBDir)
}

func NewDuckDBMetricsFromCatalog(dataDir, catalogPath, duckDBDir string) (*DuckDBMetrics, error) {
	workspace, err := semantic.LoadWorkspace(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("loading workspace: %w", err)
	}

	metrics := &DuckDBMetrics{
		dataDir:    dataDir,
		workspace:  workspace,
		runtimes:   map[string]*modelRuntime{},
		defaultID:  workspace.Catalog.Dashboards[0].ID,
		defaultMID: workspace.Catalog.SemanticModels[0].ID,
	}
	metrics.catalog = metrics.catalogView()

	for modelID, model := range workspace.Models {
		runtime := &modelRuntime{
			model:  model,
			dbPath: duckDBPath(duckDBDir, modelID),
		}
		metrics.runtimes[modelID] = runtime
		if err := metrics.validateFiles(runtime); err != nil {
			runtime.missing = err
			continue
		}
		if err := os.MkdirAll(filepath.Dir(runtime.dbPath), 0o755); err != nil {
			return nil, err
		}
		db, err := sql.Open("duckdb", runtime.dbPath)
		if err != nil {
			return nil, err
		}
		runtime.db = db
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, err
		}
		if err := metrics.RefreshCache(context.Background(), modelID); err != nil {
			db.Close()
			return nil, err
		}
		runtime.ready = true
	}

	return metrics, nil
}

func (m *DuckDBMetrics) Close() error {
	var closeErr error
	for _, runtime := range m.runtimes {
		if runtime.db == nil {
			continue
		}
		if err := runtime.db.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (m *DuckDBMetrics) DataDir() string {
	return m.dataDir
}

func (m *DuckDBMetrics) Catalog() dashboard.Catalog {
	return m.catalog
}

func (m *DuckDBMetrics) WorkspaceAssets(workspaceID, deploymentID string) ([]platform.Asset, []platform.AssetEdge, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.workspace == nil {
		return nil, nil, false
	}
	assets, edges, err := deploy.ExtractAssets(workspaceID, deploymentID, m.workspace)
	if err != nil {
		return nil, nil, false
	}
	return assets, edges, true
}

func (m *DuckDBMetrics) MetricViews() []dashboard.MetricViewSummary {
	summaries := make([]dashboard.MetricViewSummary, 0, len(m.workspace.MetricViews))
	for _, id := range sortedKeys(m.workspace.MetricViews) {
		summary, ok := m.metricViewSummary(id)
		if ok {
			summaries = append(summaries, summary)
		}
	}
	return summaries
}

func (m *DuckDBMetrics) MetricView(id string) (dashboard.MetricViewDetail, bool) {
	summary, ok := m.metricViewSummary(id)
	if !ok {
		return dashboard.MetricViewDetail{}, false
	}
	view := m.workspace.MetricViews[id]
	detail := dashboard.MetricViewDetail{MetricViewSummary: summary}

	for _, name := range sortedKeys(view.Dimensions) {
		dimension := view.Dimensions[name]
		detail.Dimensions = append(detail.Dimensions, dashboard.MetricViewDimension{
			Name:      name,
			Label:     dimension.Label,
			Expr:      dimension.Expr,
			Where:     dimension.Where,
			OrderExpr: dimension.OrderExpr,
		})
	}
	for _, name := range sortedKeys(view.Measures) {
		measure := view.Measures[name]
		detail.Measures = append(detail.Measures, dashboard.MetricViewMeasure{
			Name:        name,
			Label:       measure.Label,
			Description: measure.Description,
			Expression:  measure.Expression,
			Unit:        measure.Unit,
			Format:      measure.Format,
		})
	}
	for _, report := range m.dashboardsForMetricView(id) {
		detail.Dashboards = append(detail.Dashboards, dashboard.MetricViewDashboard{
			ID:          report.ID,
			Title:       report.Title,
			Description: report.Description,
			Tags:        append([]string{}, report.Tags...),
			PageCount:   dashboardPageCount(m.workspace.Dashboards[report.ID]),
		})
	}
	return detail, true
}

func (m *DuckDBMetrics) DefaultDashboardID() string {
	return m.defaultID
}

func (m *DuckDBMetrics) ModelIDForDashboard(dashboardID string) string {
	report, ok := m.workspace.Dashboards[dashboardID]
	if !ok {
		return ""
	}
	view, ok := m.firstMetricView(report)
	if !ok {
		return ""
	}
	return view.SemanticModel
}

func (m *DuckDBMetrics) Report(dashboardID string) (semantic.Dashboard, *semantic.Model, bool) {
	report, ok := m.workspace.Dashboards[dashboardID]
	if !ok {
		return semantic.Dashboard{}, nil, false
	}
	view, ok := m.firstMetricView(report)
	if !ok {
		return semantic.Dashboard{}, nil, false
	}
	model, ok := m.workspace.Models[view.SemanticModel]
	if !ok {
		return semantic.Dashboard{}, nil, false
	}
	return *report, model, true
}

func (m *DuckDBMetrics) NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest {
	report, ok := m.workspace.Dashboards[dashboardID]
	if !ok {
		return request.WithDefaults()
	}
	defaults := dashboard.TableRequest{Block: "all", Start: 0, Count: dashboard.TableChunkSize}
	if table, ok := report.Tables["orders"]; ok && table.KindOrDefault() == "data_table" {
		defaults.Table = "orders"
		defaults.Sort = table.DefaultSort
	} else {
		for _, name := range sortedKeys(report.Tables) {
			table := report.Tables[name]
			if table.KindOrDefault() != "data_table" {
				continue
			}
			defaults.Table = name
			defaults.Sort = table.DefaultSort
			break
		}
	}
	if defaults.Table == "" {
		defaults = dashboard.DefaultTableRequest()
	}
	if request.Table == "" {
		request.Table = defaults.Table
	}
	if request.Block == "" {
		request.Block = defaults.Block
	}
	if request.Block != "all" && request.Block != "a" && request.Block != "b" && request.Block != "c" {
		request.Block = defaults.Block
	}
	if request.Count <= 0 {
		request.Count = defaults.Count
	}
	if request.Count > dashboard.TableMaxRequestCount {
		request.Count = dashboard.TableMaxRequestCount
	}
	if request.Start < 0 {
		request.Start = 0
	}
	if request.Sort.Key == "" {
		request.Sort = defaults.Sort
	}
	if request.Sort.Direction != "asc" && request.Sort.Direction != "desc" {
		if defaults.Sort.Direction != "" {
			request.Sort.Direction = defaults.Sort.Direction
		} else {
			request.Sort.Direction = "desc"
		}
	}
	return request
}

func (m *DuckDBMetrics) DefaultFilters(dashboardID string) dashboard.Filters {
	report, ok := m.workspace.Dashboards[dashboardID]
	if !ok {
		return dashboard.Filters{}.WithDefaults()
	}
	return report.DefaultFilters()
}

func (m *DuckDBMetrics) Pages(dashboardID string) []dashboard.Page {
	report, ok := m.workspace.Dashboards[dashboardID]
	if !ok {
		return nil
	}
	pages := make([]dashboard.Page, len(report.Pages))
	for i, page := range report.Pages {
		pages[i] = page.WithDefaults()
	}
	return pages
}

func (m *DuckDBMetrics) ModelGraph(modelID string) (dashboard.ModelGraph, bool) {
	model, ok := m.workspace.Models[modelID]
	if !ok {
		return dashboard.ModelGraph{}, false
	}
	return modelGraph(model, m.workspace.MetricViews), true
}

func (m *DuckDBMetrics) catalogView() dashboard.Catalog {
	catalog := dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{
			ID:          workspaceID(m.workspace.Catalog.Workspace),
			Title:       workspaceTitle(m.workspace.Catalog.Workspace),
			Description: m.workspace.Catalog.Workspace.Description,
		},
		Models:      make([]dashboard.CatalogModel, 0, len(m.workspace.Catalog.SemanticModels)),
		MetricViews: make([]dashboard.CatalogMetricView, 0, len(m.workspace.Catalog.MetricViews)),
		Dashboards:  make([]dashboard.CatalogDashboard, 0, len(m.workspace.Catalog.Dashboards)),
	}
	modelTitles := map[string]string{}
	for _, model := range m.workspace.Catalog.SemanticModels {
		modelTitles[model.ID] = model.Title
		catalog.Models = append(catalog.Models, dashboard.CatalogModel{
			ID:          model.ID,
			Title:       model.Title,
			Description: model.Description,
		})
	}
	metricViewTitles := map[string]string{}
	for _, view := range m.workspace.Catalog.MetricViews {
		metricViewTitles[view.ID] = view.Title
		catalog.MetricViews = append(catalog.MetricViews, dashboard.CatalogMetricView{
			ID:            view.ID,
			Title:         view.Title,
			Description:   view.Description,
			SemanticModel: view.SemanticModel,
			ModelTitle:    modelTitles[view.SemanticModel],
		})
	}
	for _, report := range m.workspace.Catalog.Dashboards {
		pageCount := 0
		metricViews := []string{}
		metricViewNames := []string{}
		if loaded, ok := m.workspace.Dashboards[report.ID]; ok {
			pageCount = len(loaded.Pages)
			metricViews = append(metricViews, loaded.MetricViews...)
			for _, viewID := range loaded.MetricViews {
				if title := metricViewTitles[viewID]; title != "" {
					metricViewNames = append(metricViewNames, title)
				}
			}
		}
		catalog.Dashboards = append(catalog.Dashboards, dashboard.CatalogDashboard{
			ID:               report.ID,
			Title:            report.Title,
			Description:      report.Description,
			MetricViews:      metricViews,
			MetricViewTitles: metricViewNames,
			Tags:             append([]string{}, report.Tags...),
			PageCount:        pageCount,
		})
	}
	return catalog
}

func (m *DuckDBMetrics) metricViewSummary(id string) (dashboard.MetricViewSummary, bool) {
	view, ok := m.workspace.MetricViews[id]
	if !ok {
		return dashboard.MetricViewSummary{}, false
	}
	modelTitle := ""
	for _, model := range m.workspace.Catalog.SemanticModels {
		if model.ID == view.SemanticModel {
			modelTitle = model.Title
			break
		}
	}
	return dashboard.MetricViewSummary{
		ID:             view.ID,
		Title:          view.Title,
		Description:    view.Description,
		SemanticModel:  view.SemanticModel,
		ModelTitle:     modelTitle,
		Dataset:        view.Dataset,
		Timeseries:     view.Timeseries,
		DimensionCount: len(view.Dimensions),
		MeasureCount:   len(view.Measures),
		DashboardCount: len(m.dashboardsForMetricView(id)),
	}, true
}

func (m *DuckDBMetrics) dashboardsForMetricView(id string) []semantic.CatalogDashboard {
	reports := []semantic.CatalogDashboard{}
	for _, report := range m.workspace.Catalog.Dashboards {
		loaded, ok := m.workspace.Dashboards[report.ID]
		if !ok || !contains(loaded.MetricViews, id) {
			continue
		}
		reports = append(reports, report)
	}
	return reports
}

func dashboardPageCount(report *semantic.Dashboard) int {
	if report == nil {
		return 0
	}
	return len(report.Pages)
}

func workspaceID(workspace semantic.CatalogWorkspace) string {
	if strings.TrimSpace(workspace.ID) != "" {
		return workspace.ID
	}
	return "libredash"
}

func workspaceTitle(workspace semantic.CatalogWorkspace) string {
	if strings.TrimSpace(workspace.Title) != "" {
		return workspace.Title
	}
	return "LibreDash Workspace"
}

func (m *DuckDBMetrics) reportRuntime(dashboardID string) (*semantic.Dashboard, *modelRuntime, error) {
	report, ok := m.workspace.Dashboards[dashboardID]
	if !ok {
		return nil, nil, fmt.Errorf("unknown dashboard %q", dashboardID)
	}
	view, ok := m.firstMetricView(report)
	if !ok {
		return nil, nil, fmt.Errorf("dashboard %q has no metrics views", dashboardID)
	}
	runtime, ok := m.runtimes[view.SemanticModel]
	if !ok {
		return nil, nil, fmt.Errorf("unknown semantic model %q", view.SemanticModel)
	}
	return report, runtime, nil
}

func (m *DuckDBMetrics) firstMetricView(report *semantic.Dashboard) (*semantic.MetricView, bool) {
	if report == nil || len(report.MetricViews) == 0 {
		return nil, false
	}
	view, ok := m.workspace.MetricViews[report.MetricViews[0]]
	return view, ok
}

func (m *DuckDBMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryDashboardPage(ctx, dashboardID, "", filters)
}

func (m *DuckDBMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	report, runtime, err := m.reportRuntime(dashboardID)
	if report != nil {
		page := dashboardPage(report, pageID)
		filters = report.NormalizeFiltersForPage(page.ID, filters)
	} else {
		filters = filters.WithDefaults()
	}
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	if !runtime.ready {
		return dashboard.EmptyPatch(filters, m.dataDir, runtime.missing), nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	patch := dashboard.Patch{
		Filters: filters,
		Status: dashboard.Status{
			Loading:       false,
			LastUpdated:   refreshLabel(runtime),
			DataDirectory: m.dataDir,
		},
		Visuals: map[string]dashboard.Visual{},
	}

	page := dashboardPage(report, pageID)
	options, err := m.filterOptions(ctx, runtime, report, report.PageFilterIDs(page.ID))
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	patch.FilterOptions = options

	visuals, err := m.visuals(ctx, runtime, report, filters, pageVisualIDs(page))
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	patch.Visuals = visuals

	return patch, nil
}

func dashboardPage(report *semantic.Dashboard, pageID string) dashboard.Page {
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

func (m *DuckDBMetrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryTablePage(ctx, dashboardID, "", filters, request)
}

func (m *DuckDBMetrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	report, runtime, err := m.reportRuntime(dashboardID)
	if report != nil {
		page := dashboardPage(report, pageID)
		filters = report.NormalizeFiltersForPage(page.ID, filters)
	} else {
		filters = filters.WithDefaults()
	}
	request = m.NormalizeTableRequest(dashboardID, request)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	if !runtime.ready {
		return dashboard.EmptyTable(request, runtime.missing), nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	tableModel, ok := report.Tables[request.Table]
	if !ok {
		return dashboard.EmptyTable(request, fmt.Errorf("unknown table %q", request.Table)), nil
	}
	if tableModel.KindOrDefault() == "matrix_table" || tableModel.KindOrDefault() == "pivot_table" {
		return m.queryAggregateTable(ctx, runtime, report, request, tableModel, filters)
	}

	totalRows, err := m.countRows(ctx, runtime, report, tableModel.MetricView, filters, "table", request.Table)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	availableRows := min(totalRows, dashboard.TableInteractiveRowCap)
	blocks, err := m.tableBlocks(ctx, runtime, report, tableModel, filters, request, availableRows)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}

	style := tableModel.Style.WithDefaults()
	return dashboard.Table{
		Version:       2,
		Kind:          tableModel.KindOrDefault(),
		Title:         tableModel.Title,
		Style:         style,
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

func (m *DuckDBMetrics) filterOptions(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, names []string) (map[string][]dashboard.FilterOption, error) {
	options := map[string][]dashboard.FilterOption{}
	names = append([]string{}, names...)
	sort.Strings(names)
	for _, name := range names {
		filter := report.Filters[name]
		if filter.Values.Source != "distinct" {
			continue
		}
		source, err := m.metricViewSource(filter.MetricView)
		if err != nil {
			return nil, err
		}
		view := m.workspace.MetricViews[filter.MetricView]
		dimension := view.Dimensions[filter.Dimension]
		expr := dimensionExpression(dimension, "e")
		where := "1 = 1"
		if dimension.Where != "" {
			where = dimensionWhere(dimension, "e")
		}
		limit := filter.Values.Limit
		if limit <= 0 {
			limit = 200
		}
		if limit > 500 {
			limit = 500
		}
		query := fmt.Sprintf(`
SELECT DISTINCT CAST(%s AS VARCHAR) AS value
FROM %s e
WHERE %s AND %s IS NOT NULL AND CAST(%s AS VARCHAR) <> ''
ORDER BY value ASC
LIMIT %d`, expr, source, where, expr, expr, limit)
		rows, err := runtime.db.QueryContext(ctx, query)
		if err != nil {
			return nil, err
		}
		values := []dashboard.FilterOption{}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return nil, err
			}
			values = append(values, dashboard.FilterOption{Value: value, Label: value})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		options[name] = values
	}
	return options, nil
}

func (m *DuckDBMetrics) RefreshCache(ctx context.Context, modelID string) error {
	runtime, ok := m.runtimes[modelID]
	if !ok {
		return fmt.Errorf("unknown semantic model %q", modelID)
	}
	if runtime.missing != nil {
		return runtime.missing
	}
	if runtime.db == nil {
		return fmt.Errorf("DuckDB is not initialized")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.registerSourceViews(ctx, runtime); err != nil {
		return err
	}
	if err := m.materializeCache(ctx, runtime); err != nil {
		return err
	}
	runtime.lastRefresh = time.Now()
	return nil
}

func (m *DuckDBMetrics) validateFiles(runtime *modelRuntime) error {
	var missing []string
	for name, source := range runtime.model.Sources {
		if source.Path == "" {
			continue
		}
		connection := runtime.model.Connections[source.Connection]
		if connection.Kind != "local" {
			continue
		}
		file, err := m.resolveSourcePath(runtime.model, source)
		if err != nil {
			return fmt.Errorf("resolving local source %s: %w", name, err)
		}
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			missing = append(missing, file)
		} else if err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return &MissingDataError{DataDir: m.dataDir, Missing: missing}
	}
	return nil
}

func (m *DuckDBMetrics) registerSourceViews(ctx context.Context, runtime *modelRuntime) error {
	if _, err := runtime.db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS raw"); err != nil {
		return err
	}
	if _, err := runtime.db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS cache"); err != nil {
		return err
	}

	if err := m.prepareSourceRuntime(ctx, runtime); err != nil {
		return err
	}

	for _, name := range sortedKeys(runtime.model.Sources) {
		source := runtime.model.Sources[name]
		if err := validateIdentifier(name); err != nil {
			return err
		}
		relation, err := m.sourceRelation(runtime.model, source)
		if err != nil {
			return fmt.Errorf("compiling source %s: %w", name, err)
		}
		stmt := fmt.Sprintf("CREATE OR REPLACE VIEW raw.%s AS %s", name, relation)
		if _, err := runtime.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("registering source %s: %w", name, err)
		}
	}
	return nil
}

func (m *DuckDBMetrics) materializeCache(ctx context.Context, runtime *modelRuntime) error {
	for _, name := range runtime.model.CacheTableNames() {
		if err := validateIdentifier(name); err != nil {
			return err
		}
		table := runtime.model.Cache.Tables[name]
		stmt := fmt.Sprintf("CREATE OR REPLACE TABLE cache.%s AS %s", name, table.SQL)
		if _, err := runtime.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("materializing cache.%s: %w", name, err)
		}
	}
	return nil
}

func (m *DuckDBMetrics) visuals(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, filters dashboard.Filters, keys []string) (map[string]dashboard.Visual, error) {
	visuals := make(map[string]dashboard.Visual, len(keys))
	for _, key := range keys {
		visual, ok := report.Visuals[key]
		if !ok {
			return nil, fmt.Errorf("page references unknown visual %q", key)
		}
		data, err := m.visualData(ctx, runtime, report, key, visual, filters)
		if err != nil {
			return nil, err
		}
		dataset := m.workspace.MetricViews[visual.MetricView]
		measureName := visual.Query.Measures[0]
		measure := dataset.Measures[measureName]
		title := visual.Title
		if title == "" {
			title = measure.Label
		}
		if title == "" {
			title = measureName
		}
		unit := measure.Unit
		if len(visual.Query.Measures) > 1 {
			unit = ""
		}
		series := []string{}
		if visual.Query.Series != "" {
			series = append(series, visual.Query.Series)
		}
		rendererOptions := map[string]map[string]any{}
		for renderer, options := range visual.RendererOptions {
			if typed, ok := options.(map[string]any); ok {
				rendererOptions[renderer] = typed
			}
		}
		visualType := visual.Type
		if visualType == "" && visual.KindOrDefault() == "kpi" {
			visualType = "kpi"
		}
		visuals[key] = dashboard.Visual{
			Version:         3,
			ID:              key,
			Kind:            visual.KindOrDefault(),
			Shape:           visual.ShapeOrDefault(),
			Renderer:        visual.RendererOrDefault(),
			Type:            visualType,
			Title:           title,
			Unit:            unit,
			Format:          measure.Format,
			Field:           visual.Interaction.Field,
			Dimensions:      append([]string{}, visual.Query.Dimensions...),
			Measure:         measureName,
			Measures:        append([]string{}, visual.Query.Measures...),
			Series:          series,
			Options:         visual.CoreOptions(),
			RendererOptions: rendererOptions,
			Selection:       selectedValues(filters, key),
			Data:            data,
		}
	}
	return visuals, nil
}

func (m *DuckDBMetrics) visualData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	switch visual.ShapeOrDefault() {
	case "single_value":
		return m.singleValueData(ctx, runtime, report, visualID, visual, filters)
	case "category_multi_measure":
		return m.categoryMultiMeasureData(ctx, runtime, report, visualID, visual, filters)
	case "category_delta":
		return m.categoryDeltaData(ctx, runtime, report, visualID, visual, filters)
	case "binned_measure":
		return m.binnedMeasureData(ctx, runtime, report, visualID, visual, filters)
	case "hierarchy":
		return m.hierarchyData(ctx, runtime, report, visualID, visual, filters)
	case "matrix":
		return m.matrixData(ctx, runtime, report, visualID, visual, filters)
	case "graph":
		return m.graphData(ctx, runtime, report, visualID, visual, filters)
	case "geo":
		return m.geoData(ctx, runtime, report, visualID, visual, filters)
	case "ohlc":
		return m.ohlcData(ctx, runtime, report, visualID, visual, filters)
	case "distribution":
		return m.distributionData(ctx, runtime, report, visualID, visual, filters)
	default:
		return m.categoryData(ctx, runtime, report, visualID, visual, filters)
	}
}

func (m *DuckDBMetrics) categoryData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	source, err := m.metricViewSource(visual.MetricView)
	if err != nil {
		return nil, err
	}
	dataset := m.workspace.MetricViews[visual.MetricView]
	labelDimension := visual.Query.Dimensions[0]
	labelExpr := dimensionExpression(dataset.Dimensions[labelDimension], "e")
	measureName := visual.Query.Measures[0]
	valueExpr, err := measureAggregateExpr(dataset.Measures[measureName])
	if err != nil {
		return nil, err
	}
	seriesExpr := "''"
	groupBy := []string{"label"}
	if visual.Query.Series != "" {
		seriesExpr = dimensionExpression(dataset.Dimensions[visual.Query.Series], "e")
		groupBy = append(groupBy, "series")
	}

	where, args := m.filterWhere("e", runtime, report, visual.MetricView, filters, "visual", visualID)
	for _, dimensionName := range append(append([]string{}, visual.Query.Dimensions...), visual.Query.Series) {
		if dimensionName == "" {
			continue
		}
		if dimension := dataset.Dimensions[dimensionName]; dimension.Where != "" {
			where = fmt.Sprintf("(%s) AND (%s)", where, dimensionWhere(dimension, "e"))
		}
	}

	orderBy := m.visualOrderBy(runtime.model, visual)
	query := fmt.Sprintf(`
SELECT %s AS label, %s AS series, %s AS value
FROM %s e
WHERE %s
GROUP BY %s
ORDER BY %s`, labelExpr, seriesExpr, valueExpr, source, where, strings.Join(groupBy, ", "), orderBy)
	if visual.Query.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", visual.Query.Limit)
	}

	data, err := m.queryDatums(ctx, runtime, query, []string{"label", "series", "value"}, args...)
	if err != nil {
		return nil, err
	}
	markSelected(data, "label", selectedValues(filters, visualID))
	return data, nil
}

func (m *DuckDBMetrics) categoryMultiMeasureData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	source, err := m.metricViewSource(visual.MetricView)
	if err != nil {
		return nil, err
	}
	dataset := m.workspace.MetricViews[visual.MetricView]
	labelExpr := dimensionExpression(dataset.Dimensions[visual.Query.Dimensions[0]], "e")
	where, args := m.visualWhere(runtime, report, visual, filters, visualID)
	orderBy := m.visualOrderBy(runtime.model, visual)
	data := []dashboard.Datum{}

	for _, measureName := range visual.Query.Measures {
		measure := dataset.Measures[measureName]
		valueExpr, err := measureAggregateExpr(measure)
		if err != nil {
			return nil, err
		}
		query := fmt.Sprintf(`
SELECT %s AS label, ? AS series, %s AS value
FROM %s e
WHERE %s
GROUP BY label
ORDER BY %s`, labelExpr, valueExpr, source, where, orderBy)
		if visual.Query.Limit > 0 {
			query += fmt.Sprintf("\nLIMIT %d", visual.Query.Limit)
		}
		measureArgs := append([]any{measureLabel(measureName, measure)}, args...)
		rows, err := m.queryDatums(ctx, runtime, query, []string{"label", "series", "value"}, measureArgs...)
		if err != nil {
			return nil, err
		}
		data = append(data, rows...)
	}
	markSelected(data, "label", selectedValues(filters, visualID))
	return data, nil
}

func (m *DuckDBMetrics) categoryDeltaData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	rows, err := m.categoryData(ctx, runtime, report, visualID, visual, filters)
	if err != nil {
		return nil, err
	}
	cumulative := 0.0
	for _, row := range rows {
		value := datumFloat(row["value"])
		start := cumulative
		cumulative += value
		row["start"] = round(start)
		row["end"] = round(cumulative)
		row["positive"] = value >= 0
	}
	return rows, nil
}

func (m *DuckDBMetrics) binnedMeasureData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	source, err := m.metricViewSource(visual.MetricView)
	if err != nil {
		return nil, err
	}
	dataset := m.workspace.MetricViews[visual.MetricView]
	measure := dataset.Measures[visual.Query.Measures[0]]
	columnExpr, err := rawValueExpression(measure)
	if err != nil {
		return nil, err
	}
	columnExpr = "CAST(" + columnExpr + " AS DOUBLE)"
	where, args := m.visualWhere(runtime, report, visual, filters, visualID)
	binCount := optionInt(visual.Options, "bin_count", 20, 5, 60)

	var minValue, maxValue sql.NullFloat64
	boundsQuery := fmt.Sprintf("SELECT MIN(%s), MAX(%s) FROM %s e WHERE %s AND %s IS NOT NULL", columnExpr, columnExpr, source, where, columnExpr)
	if err := runtime.db.QueryRowContext(ctx, boundsQuery, args...).Scan(&minValue, &maxValue); err != nil {
		return nil, err
	}
	if !minValue.Valid || !maxValue.Valid {
		return []dashboard.Datum{}, nil
	}
	if minValue.Float64 == maxValue.Float64 {
		var count int
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s e WHERE %s AND %s IS NOT NULL", source, where, columnExpr)
		if err := runtime.db.QueryRowContext(ctx, countQuery, args...).Scan(&count); err != nil {
			return nil, err
		}
		return []dashboard.Datum{{
			"label":    formatBinLabel(minValue.Float64, maxValue.Float64),
			"binStart": round(minValue.Float64),
			"binEnd":   round(maxValue.Float64),
			"value":    count,
		}}, nil
	}

	bucketExpr := fmt.Sprintf("LEAST(%d, CAST(FLOOR(((%s - ?) / NULLIF(? - ?, 0)) * ?) AS INTEGER))", binCount-1, columnExpr)
	query := fmt.Sprintf(`
SELECT %s AS bucket, COUNT(*) AS value
FROM %s e
WHERE %s AND %s IS NOT NULL
GROUP BY bucket
ORDER BY bucket ASC`, bucketExpr, source, where, columnExpr)
	queryArgs := append([]any{minValue.Float64, maxValue.Float64, minValue.Float64, binCount}, args...)
	rows, err := runtime.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	width := (maxValue.Float64 - minValue.Float64) / float64(binCount)
	data := []dashboard.Datum{}
	for rows.Next() {
		var bucket int
		var count int
		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, err
		}
		start := minValue.Float64 + float64(bucket)*width
		end := start + width
		data = append(data, dashboard.Datum{
			"label":    formatBinLabel(start, end),
			"binStart": round(start),
			"binEnd":   round(end),
			"value":    count,
		})
	}
	return data, rows.Err()
}

func (m *DuckDBMetrics) hierarchyData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	source, err := m.metricViewSource(visual.MetricView)
	if err != nil {
		return nil, err
	}
	dataset := m.workspace.MetricViews[visual.MetricView]
	levelExprs := make([]string, 0, len(visual.Query.Dimensions))
	levelAliases := make([]string, 0, len(visual.Query.Dimensions))
	for index, dimensionName := range visual.Query.Dimensions {
		levelExprs = append(levelExprs, fmt.Sprintf("%s AS level_%d", dimensionExpression(dataset.Dimensions[dimensionName], "e"), index))
		levelAliases = append(levelAliases, fmt.Sprintf("level_%d", index))
	}
	valueExpr, err := measureAggregateExpr(dataset.Measures[visual.Query.Measures[0]])
	if err != nil {
		return nil, err
	}
	where, args := m.visualWhere(runtime, report, visual, filters, visualID)
	orderBy := m.visualOrderBy(runtime.model, visual)
	query := fmt.Sprintf(`
SELECT %s, %s AS value
FROM %s e
WHERE %s
GROUP BY %s
ORDER BY %s`, strings.Join(levelExprs, ", "), valueExpr, source, where, strings.Join(levelAliases, ", "), orderBy)
	if visual.Query.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", visual.Query.Limit)
	}

	rows, err := runtime.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]any, len(levelAliases)+1)
	scans := make([]any, len(values))
	for index := range values {
		scans[index] = &values[index]
	}
	data := []dashboard.Datum{}
	for rows.Next() {
		if err := rows.Scan(scans...); err != nil {
			return nil, err
		}
		path := make([]string, 0, len(levelAliases))
		for index := range levelAliases {
			item := normalizeDatumValue(values[index])
			if item == nil || fmt.Sprint(item) == "" {
				continue
			}
			path = append(path, fmt.Sprint(item))
		}
		data = append(data, dashboard.Datum{
			"path":  path,
			"value": normalizeDatumValue(values[len(values)-1]),
		})
	}
	return data, rows.Err()
}

func (m *DuckDBMetrics) singleValueData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	source, err := m.metricViewSource(visual.MetricView)
	if err != nil {
		return nil, err
	}
	dataset := m.workspace.MetricViews[visual.MetricView]
	measureName := visual.Query.Measures[0]
	valueExpr, err := measureAggregateExpr(dataset.Measures[measureName])
	if err != nil {
		return nil, err
	}
	title := visual.Title
	if title == "" {
		title = dataset.Measures[measureName].Label
	}
	if title == "" {
		title = measureName
	}
	labelExpr := "'" + strings.ReplaceAll(title, "'", "''") + "'"
	groupBy := ""
	if len(visual.Query.Dimensions) == 1 {
		labelExpr = dimensionExpression(dataset.Dimensions[visual.Query.Dimensions[0]], "e")
		groupBy = " GROUP BY label"
	}
	where, args := m.visualWhere(runtime, report, visual, filters, visualID)
	query := fmt.Sprintf("SELECT %s AS label, '' AS series, %s AS value FROM %s e WHERE %s%s ORDER BY %s", labelExpr, valueExpr, source, where, groupBy, m.visualOrderBy(runtime.model, visual))
	if visual.Query.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", visual.Query.Limit)
	}
	data, err := m.queryDatums(ctx, runtime, query, []string{"label", "series", "value"}, args...)
	if err != nil {
		return nil, err
	}
	markSelected(data, "label", selectedValues(filters, visualID))
	return data, nil
}

func (m *DuckDBMetrics) matrixData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	return m.dimensionPairData(ctx, runtime, report, visualID, visual, filters, "row", "column")
}

func (m *DuckDBMetrics) graphData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	return m.dimensionPairData(ctx, runtime, report, visualID, visual, filters, "source", "target")
}

func (m *DuckDBMetrics) dimensionPairData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters, leftAlias, rightAlias string) ([]dashboard.Datum, error) {
	source, err := m.metricViewSource(visual.MetricView)
	if err != nil {
		return nil, err
	}
	dataset := m.workspace.MetricViews[visual.MetricView]
	leftExpr := dimensionExpression(dataset.Dimensions[visual.Query.Dimensions[0]], "e")
	rightExpr := dimensionExpression(dataset.Dimensions[visual.Query.Dimensions[1]], "e")
	rightSQLAlias := rightAlias
	if rightAlias == "column" {
		rightSQLAlias = "chart_column"
	}
	valueExpr, err := measureAggregateExpr(dataset.Measures[visual.Query.Measures[0]])
	if err != nil {
		return nil, err
	}
	where, args := m.visualWhere(runtime, report, visual, filters, visualID)
	query := fmt.Sprintf(`
SELECT %s AS %s, %s AS %s, %s AS value
FROM %s e
WHERE %s
GROUP BY %s, %s
ORDER BY %s`, leftExpr, leftAlias, rightExpr, rightSQLAlias, valueExpr, source, where, leftAlias, rightSQLAlias, m.visualOrderBy(runtime.model, visual))
	if visual.Query.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", visual.Query.Limit)
	}
	data, err := m.queryDatums(ctx, runtime, query, []string{leftAlias, rightAlias, "value"}, args...)
	if err != nil {
		return nil, err
	}
	if leftAlias == "row" {
		markSelected(data, "row", selectedValues(filters, visualID))
	}
	return data, nil
}

func (m *DuckDBMetrics) geoData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	source, err := m.metricViewSource(visual.MetricView)
	if err != nil {
		return nil, err
	}
	dataset := m.workspace.MetricViews[visual.MetricView]
	nameExpr := dimensionExpression(dataset.Dimensions[visual.Query.Dimensions[0]], "e")
	valueExpr, err := measureAggregateExpr(dataset.Measures[visual.Query.Measures[0]])
	if err != nil {
		return nil, err
	}
	where, args := m.visualWhere(runtime, report, visual, filters, visualID)
	query := fmt.Sprintf(`
SELECT %s AS name, %s AS value
FROM %s e
WHERE %s
GROUP BY name
ORDER BY %s`, nameExpr, valueExpr, source, where, m.visualOrderBy(runtime.model, visual))
	if visual.Query.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", visual.Query.Limit)
	}
	data, err := m.queryDatums(ctx, runtime, query, []string{"name", "value"}, args...)
	if err != nil {
		return nil, err
	}
	markSelected(data, "name", selectedValues(filters, visualID))
	return data, nil
}

func (m *DuckDBMetrics) ohlcData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	source, err := m.metricViewSource(visual.MetricView)
	if err != nil {
		return nil, err
	}
	dataset := m.workspace.MetricViews[visual.MetricView]
	labelExpr := dimensionExpression(dataset.Dimensions[visual.Query.Dimensions[0]], "e")
	measureExprs := make([]string, 0, 4)
	for _, measureName := range visual.Query.Measures {
		expr, err := measureAggregateExpr(dataset.Measures[measureName])
		if err != nil {
			return nil, err
		}
		measureExprs = append(measureExprs, expr)
	}
	where, args := m.visualWhere(runtime, report, visual, filters, visualID)
	query := fmt.Sprintf(`
SELECT %s AS label, %s AS open, %s AS close, %s AS low, %s AS high
FROM %s e
WHERE %s
GROUP BY label
ORDER BY %s`, labelExpr, measureExprs[0], measureExprs[1], measureExprs[2], measureExprs[3], source, where, m.visualOrderBy(runtime.model, visual))
	if visual.Query.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", visual.Query.Limit)
	}
	return m.queryDatums(ctx, runtime, query, []string{"label", "open", "close", "low", "high"}, args...)
}

func (m *DuckDBMetrics) distributionData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	source, err := m.metricViewSource(visual.MetricView)
	if err != nil {
		return nil, err
	}
	dataset := m.workspace.MetricViews[visual.MetricView]
	labelExpr := dimensionExpression(dataset.Dimensions[visual.Query.Dimensions[0]], "e")
	measure := dataset.Measures[visual.Query.Measures[0]]
	columnExpr, err := rawValueExpression(measure)
	if err != nil {
		return nil, err
	}
	where, args := m.visualWhere(runtime, report, visual, filters, visualID)
	query := fmt.Sprintf(`
SELECT %s AS label,
       MIN(%s) AS min,
       quantile_cont(%s, 0.25) AS q1,
       median(%s) AS median,
       quantile_cont(%s, 0.75) AS q3,
       MAX(%s) AS max
FROM %s e
WHERE %s AND %s IS NOT NULL
GROUP BY label
ORDER BY %s`, labelExpr, columnExpr, columnExpr, columnExpr, columnExpr, columnExpr, source, where, columnExpr, m.visualOrderBy(runtime.model, visual))
	if visual.Query.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", visual.Query.Limit)
	}
	return m.queryDatums(ctx, runtime, query, []string{"label", "min", "q1", "median", "q3", "max"}, args...)
}

func (m *DuckDBMetrics) visualWhere(runtime *modelRuntime, report *semantic.Dashboard, visual semantic.Visual, filters dashboard.Filters, visualID string) (string, []any) {
	dataset := m.workspace.MetricViews[visual.MetricView]
	where, args := m.filterWhere("e", runtime, report, visual.MetricView, filters, "visual", visualID)
	for _, dimensionName := range visualQueryDimensions(visual) {
		if dimensionName == "" {
			continue
		}
		if dimension := dataset.Dimensions[dimensionName]; dimension.Where != "" {
			where = fmt.Sprintf("(%s) AND (%s)", where, dimensionWhere(dimension, "e"))
		}
	}
	return where, args
}

func visualQueryDimensions(visual semantic.Visual) []string {
	dimensions := append([]string{}, visual.Query.Dimensions...)
	if visual.Query.Series != "" {
		dimensions = append(dimensions, visual.Query.Series)
	}
	return dimensions
}

func (m *DuckDBMetrics) queryDatums(ctx context.Context, runtime *modelRuntime, query string, columns []string, args ...any) ([]dashboard.Datum, error) {
	rows, err := runtime.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]any, len(columns))
	scans := make([]any, len(columns))
	for index := range values {
		scans[index] = &values[index]
	}
	data := []dashboard.Datum{}
	for rows.Next() {
		if err := rows.Scan(scans...); err != nil {
			return nil, err
		}
		row := dashboard.Datum{}
		for index, column := range columns {
			row[column] = normalizeDatumValue(values[index])
		}
		data = append(data, row)
	}
	return data, rows.Err()
}

func (m *DuckDBMetrics) countRows(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, metricViewID string, filters dashboard.Filters, targetKind, targetID string) (int, error) {
	source, err := m.metricViewSource(metricViewID)
	if err != nil {
		return 0, err
	}
	where, args := m.filterWhere("e", runtime, report, metricViewID, filters, targetKind, targetID)
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s e WHERE %s", source, where)

	var total int
	if err := runtime.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func measureAggregateExpr(measure semantic.MetricMeasure) (string, error) {
	if strings.TrimSpace(measure.Expression) == "" {
		return "", fmt.Errorf("measure %q is missing expression", measure.Label)
	}
	return measure.Expression, nil
}

func rawValueExpression(measure semantic.MetricMeasure) (string, error) {
	expr := strings.TrimSpace(measure.Expression)
	if expr == "" {
		return "", fmt.Errorf("measure %q is missing expression", measure.Label)
	}
	if matches := aggregateWrapperPattern.FindStringSubmatch(expr); len(matches) == 2 {
		return strings.TrimSpace(matches[1]), nil
	}
	if strings.Contains(expr, "(") {
		return "", fmt.Errorf("measure %q cannot be used as a raw value expression", measure.Label)
	}
	return expr, nil
}

func measureLabel(name string, measure semantic.MetricMeasure) string {
	if strings.TrimSpace(measure.Label) != "" {
		return measure.Label
	}
	return name
}

func optionInt(options map[string]any, key string, fallback, minValue, maxValue int) int {
	if options == nil {
		return fallback
	}
	var value int
	switch typed := options[key].(type) {
	case int:
		value = typed
	case int64:
		value = int(typed)
	case float64:
		value = int(typed)
	case string:
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return fallback
		}
		value = parsed
	default:
		return fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func datumFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		parsed, _ := strconv.ParseFloat(typed, 64)
		return parsed
	default:
		return 0
	}
}

func formatBinLabel(start, end float64) string {
	if math.Abs(start-end) < 0.000001 {
		return strconv.FormatFloat(round(start), 'f', -1, 64)
	}
	return fmt.Sprintf("%s-%s", strconv.FormatFloat(round(start), 'f', -1, 64), strconv.FormatFloat(round(end), 'f', -1, 64))
}

func (m *DuckDBMetrics) visualOrderBy(model *semantic.Model, visual semantic.Visual) string {
	if len(visual.Query.Sort) == 0 {
		return "label ASC"
	}
	metricView := m.workspace.MetricViews[visual.MetricView]
	parts := make([]string, 0, len(visual.Query.Sort))
	for _, sortSpec := range visual.Query.Sort {
		direction := "ASC"
		if strings.EqualFold(sortSpec.Direction, "desc") {
			direction = "DESC"
		}
		expr := sortSpec.Expr
		if expr == "" {
			expr = m.sortExpression(metricView, visual, sortSpec.Field)
		}
		if expr == "" {
			expr = "label"
		}
		parts = append(parts, expr+" "+direction)
	}
	return strings.Join(parts, ", ")
}

func (m *DuckDBMetrics) sortExpression(metricView *semantic.MetricView, visual semantic.Visual, field string) string {
	if field == "" {
		return defaultSortColumn(visual)
	}
	if field == "value" || field == visual.Query.Measures[0] {
		return "value"
	}
	if field == visual.Query.Series {
		return "series"
	}
	if metricView == nil {
		return ""
	}
	if dimension, ok := metricView.Dimensions[field]; ok {
		if dimension.OrderExpr != "" {
			return dimension.OrderExpr
		}
		for index, dimensionName := range visual.Query.Dimensions {
			if field == dimensionName {
				return dimensionSortColumn(visual.ShapeOrDefault(), index)
			}
		}
		return dimensionExpression(dimension, "e")
	}
	return ""
}

func defaultSortColumn(visual semantic.Visual) string {
	switch visual.ShapeOrDefault() {
	case "matrix":
		return "row"
	case "graph":
		return "source"
	case "geo":
		return "name"
	case "hierarchy":
		return "value"
	default:
		return "label"
	}
}

func dimensionSortColumn(shape string, index int) string {
	switch shape {
	case "matrix":
		if index == 1 {
			return "chart_column"
		}
		return "row"
	case "graph":
		if index == 1 {
			return "target"
		}
		return "source"
	case "geo":
		return "name"
	case "hierarchy":
		return fmt.Sprintf("level_%d", index)
	default:
		return "label"
	}
}

func (m *DuckDBMetrics) filterWhere(alias string, runtime *modelRuntime, report *semantic.Dashboard, metricViewID string, filters dashboard.Filters, targetKind, targetID string) (string, []any) {
	filters = filters.WithDefaults()
	conditions := []string{"1 = 1"}
	args := []any{}

	for _, name := range sortedKeys(report.Filters) {
		filter := report.Filters[name]
		if filter.MetricView != metricViewID {
			continue
		}
		control, ok := filters.Controls[name]
		if !ok {
			continue
		}
		metricView, ok := m.workspace.MetricViews[filter.MetricView]
		if !ok {
			continue
		}
		dimension, ok := metricView.Dimensions[filter.Dimension]
		if !ok {
			continue
		}
		expr := dimensionExpression(dimension, alias)
		switch filter.Type {
		case "date_range":
			condition, conditionArgs := m.dateFilterCondition(runtime, filter, control, expr)
			if condition != "" {
				conditions = append(conditions, condition)
				args = append(args, conditionArgs...)
			}
		case "multi_select":
			if control.Operator != "in" || len(control.Values) == 0 {
				continue
			}
			placeholders := make([]string, 0, len(control.Values))
			for _, value := range control.Values {
				placeholders = append(placeholders, "?")
				args = append(args, value)
			}
			conditions = append(conditions, expr+" IN ("+strings.Join(placeholders, ", ")+")")
		case "text":
			value := strings.TrimSpace(control.Value)
			if value == "" {
				continue
			}
			switch control.Operator {
			case "equals":
				conditions = append(conditions, "lower("+expr+") = lower(?)")
				args = append(args, value)
			case "starts_with":
				conditions = append(conditions, "lower("+expr+") LIKE lower(?)")
				args = append(args, value+"%")
			case "not_contains":
				conditions = append(conditions, "lower("+expr+") NOT LIKE lower(?)")
				args = append(args, "%"+value+"%")
			default:
				conditions = append(conditions, "lower("+expr+") LIKE lower(?)")
				args = append(args, "%"+value+"%")
			}
		}
	}

	for _, selection := range filters.VisualSelections {
		if selection.VisualID == "" || len(selection.Values) == 0 {
			continue
		}
		if targetKind == "visual" && selection.VisualID == targetID {
			continue
		}
		sourceVisual, ok := report.Visuals[selection.VisualID]
		if !ok || !targetsSelection(sourceVisual.Interaction.Targets, targetKind, targetID) {
			continue
		}
		if selection.Operator != "" && selection.Operator != "in" {
			continue
		}
		metricView, ok := m.workspace.MetricViews[metricViewID]
		if !ok {
			continue
		}
		dimension, ok := metricView.Dimensions[selection.Field]
		if !ok {
			continue
		}
		placeholders := make([]string, 0, len(selection.Values))
		for _, value := range selection.Values {
			placeholders = append(placeholders, "?")
			args = append(args, value)
		}
		conditions = append(conditions, dimensionExpression(dimension, alias)+" IN ("+strings.Join(placeholders, ", ")+")")
	}

	return strings.Join(conditions, " AND "), args
}

func (m *DuckDBMetrics) dateFilterCondition(runtime *modelRuntime, filter semantic.FilterDefinition, control dashboard.FilterControl, expr string) (string, []any) {
	if control.From != "" || control.To != "" {
		conditions := []string{}
		args := []any{}
		if control.From != "" {
			conditions = append(conditions, expr+" >= CAST(? AS TIMESTAMP)")
			args = append(args, control.From)
		}
		if control.To != "" {
			conditions = append(conditions, expr+" < CAST(? AS TIMESTAMP) + INTERVAL 1 DAY")
			args = append(args, control.To)
		}
		return strings.Join(conditions, " AND "), args
	}
	if control.Preset == "" || control.Preset == "all" {
		return "", nil
	}
	for _, preset := range filter.Presets {
		if preset.Value != control.Preset {
			continue
		}
		if preset.RelativeDays > 0 {
			source, err := m.metricViewSource(filter.MetricView)
			if err != nil {
				return "", nil
			}
			metricView := m.workspace.MetricViews[filter.MetricView]
			dimension := metricView.Dimensions[filter.Dimension]
			sourceExpr := dimensionExpression(dimension, "recent")
			return fmt.Sprintf("%s >= (SELECT max(%s) - INTERVAL %d DAY FROM %s recent)", expr, sourceExpr, preset.RelativeDays, source), nil
		}
		if preset.From != "" && preset.To != "" {
			return expr + " >= CAST(? AS TIMESTAMP) AND " + expr + " < CAST(? AS TIMESTAMP)", []any{preset.From, preset.To}
		}
	}
	return "", nil
}

func targetsSelection(targets semantic.InteractionTargets, targetKind, targetID string) bool {
	switch targetKind {
	case "visual":
		return contains(targets.Visuals, targetID)
	case "table":
		return contains(targets.Tables, targetID)
	default:
		return false
	}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func dimensionExpression(dimension semantic.MetricDimension, alias string) string {
	if identifierPattern.MatchString(dimension.Expr) {
		return alias + "." + dimension.Expr
	}
	return strings.ReplaceAll(dimension.Expr, "{alias}", alias)
}

func dimensionWhere(dimension semantic.MetricDimension, alias string) string {
	if dimension.Where == "" {
		return ""
	}
	return strings.ReplaceAll(dimension.Where, "{alias}", alias)
}

func selectedValues(filters dashboard.Filters, visualID string) []string {
	for _, selection := range filters.VisualSelections {
		if selection.VisualID == visualID {
			values := make([]string, len(selection.Values))
			copy(values, selection.Values)
			return values
		}
	}
	return []string{}
}

func markSelected(data []dashboard.Datum, key string, values []string) {
	if len(values) == 0 {
		return
	}
	selected := make(map[string]struct{}, len(values))
	for _, value := range values {
		selected[value] = struct{}{}
	}
	for _, row := range data {
		value, ok := row[key]
		if !ok {
			continue
		}
		if _, ok := selected[fmt.Sprint(value)]; ok {
			row["selected"] = true
		}
	}
}

func normalizeDatumValue(value any) any {
	switch typed := normalizeDBValue(value).(type) {
	case float64:
		return round(typed)
	case float32:
		return round(float64(typed))
	default:
		return typed
	}
}

func normalizeDBValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format("2006-01-02")
	case float32:
		return round(float64(typed))
	case float64:
		return round(typed)
	default:
		return typed
	}
}

func (m *DuckDBMetrics) metricViewSource(name string) (string, error) {
	view, ok := m.workspace.MetricViews[name]
	if !ok {
		return "", fmt.Errorf("unknown metrics view %q", name)
	}
	model, ok := m.workspace.Models[view.SemanticModel]
	if !ok {
		return "", fmt.Errorf("unknown semantic model %q", view.SemanticModel)
	}
	dataset, ok := model.Datasets[view.Dataset]
	if !ok {
		return "", fmt.Errorf("metrics view %q references unknown dataset %q", name, view.Dataset)
	}
	return cacheSource(dataset.Source)
}

func cacheSource(name string) (string, error) {
	if err := validateIdentifier(name); err != nil {
		return "", err
	}
	return "cache." + name, nil
}

func validateIdentifier(value string) error {
	if !identifierPattern.MatchString(value) {
		return fmt.Errorf("invalid identifier %q", value)
	}
	return nil
}

func discoverCatalogPath() (string, error) {
	candidates := []string{
		filepath.Join("dashboards", "catalog.yaml"),
		filepath.Join("..", "dashboards", "catalog.yaml"),
		filepath.Join("..", "..", "dashboards", "catalog.yaml"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find dashboards/catalog.yaml")
}

func duckDBPath(dataDir, modelID string) string {
	if path := os.Getenv("LIBREDASH_DUCKDB_PATH"); path != "" {
		return path
	}
	return filepath.Join(dataDir, "libredash-"+modelID+".duckdb")
}

func sqlString(path string) string {
	return strings.ReplaceAll(filepath.ToSlash(path), "'", "''")
}

func modelGraph(model *semantic.Model, metricViews map[string]*semantic.MetricView) dashboard.ModelGraph {
	graph := dashboard.ModelGraph{
		Name:  model.Name,
		Title: model.Title,
		Stats: dashboard.ModelStats{
			Sources:       len(model.Sources),
			CacheTables:   len(model.Cache.Tables),
			Metrics:       measureCount(model.Name, metricViews),
			Visuals:       0,
			ReportTables:  0,
			Relationships: len(model.Relationships),
		},
	}

	for _, name := range sortedKeys(model.Sources) {
		source := model.Sources[name]
		sourceKind := source.Kind()
		meta := []dashboard.ModelMeta{
			{Label: "Kind", Value: sourceKind},
			{Label: "Schema", Value: "raw"},
		}
		if source.Format != "" {
			meta = append(meta, dashboard.ModelMeta{Label: "Format", Value: source.Format})
		}
		if source.Path != "" {
			meta = append(meta, dashboard.ModelMeta{Label: "Path", Value: source.Path})
		}
		if source.Object != "" {
			meta = append(meta, dashboard.ModelMeta{Label: "Object", Value: source.Object})
		}
		if source.Connection != "" {
			meta = append(meta, dashboard.ModelMeta{Label: "Connection", Value: source.Connection})
			if connection, ok := model.Connections[source.Connection]; ok {
				meta = append(meta, dashboard.ModelMeta{Label: "Connection Kind", Value: connection.Kind})
			}
		}
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:     nodeID("source", name),
			Label:  name,
			Kind:   "source",
			Schema: "raw",
			Fields: []dashboard.ModelField{{Name: source.Description(), Role: source.Role()}},
			Meta:   meta,
		})
	}

	for _, name := range sortedKeys(model.Cache.Tables) {
		table := model.Cache.Tables[name]
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:          nodeID("cache", name),
			Label:       name,
			Kind:        "cache",
			Schema:      "cache",
			Description: table.Description,
			Fields:      cacheFields(),
			Meta: []dashboard.ModelMeta{
				{Label: "Mode", Value: "DuckDB import"},
				{Label: "Schema", Value: "cache"},
			},
		})
		for _, sourceName := range sortedKeys(model.Sources) {
			graph.Edges = append(graph.Edges, dashboard.ModelEdge{
				ID:     "source_" + sourceName + "_to_cache_" + name,
				Source: nodeID("source", sourceName),
				Target: nodeID("cache", name),
				Label:  "materializes",
				Kind:   "materialization",
			})
		}
	}

	for _, relationship := range model.Relationships {
		fromTable, fromField := modelEndpoint(relationship.From)
		toTable, toField := modelEndpoint(relationship.To)
		graph.Edges = append(graph.Edges, dashboard.ModelEdge{
			ID:          relationship.ID,
			Source:      nodeID("source", fromTable),
			Target:      nodeID("source", toTable),
			Label:       fromField + " -> " + toField,
			Kind:        "relationship",
			SourceField: fromField,
			TargetField: toField,
			Cardinality: relationship.Cardinality,
		})
	}

	for _, name := range sortedKeys(model.Datasets) {
		dataset := model.Datasets[name]
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:     nodeID("dataset", name),
			Label:  name,
			Kind:   "dataset",
			Schema: "semantic",
			Meta: []dashboard.ModelMeta{
				{Label: "Source", Value: dataset.Source},
			},
		})
		graph.Edges = append(graph.Edges, dashboard.ModelEdge{
			ID:     "dataset_" + name + "_from_" + dataset.Source,
			Source: nodeID("cache", dataset.Source),
			Target: nodeID("dataset", name),
			Label:  "semantic dataset",
			Kind:   "semantic",
		})
	}

	for _, name := range sortedKeys(metricViews) {
		view := metricViews[name]
		if view.SemanticModel != model.Name {
			continue
		}
		fields := make([]dashboard.ModelField, 0, len(view.Dimensions)+len(view.Measures))
		for _, dimension := range sortedKeys(view.Dimensions) {
			fields = append(fields, dashboard.ModelField{Name: dimension, Role: "dimension"})
		}
		for _, measure := range sortedKeys(view.Measures) {
			fields = append(fields, dashboard.ModelField{Name: measure, Role: "measure"})
		}
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:          nodeID("metrics_view", name),
			Label:       view.Title,
			Kind:        "metrics_view",
			Schema:      "metrics",
			Description: view.Description,
			Fields:      fields,
			Meta: []dashboard.ModelMeta{
				{Label: "Dataset", Value: view.Dataset},
				{Label: "Timeseries", Value: view.Timeseries},
				{Label: "Dimensions", Value: strconv.Itoa(len(view.Dimensions))},
				{Label: "Measures", Value: strconv.Itoa(len(view.Measures))},
			},
		})
		graph.Edges = append(graph.Edges, dashboard.ModelEdge{
			ID:     "metrics_view_" + name + "_from_" + view.Dataset,
			Source: nodeID("dataset", view.Dataset),
			Target: nodeID("metrics_view", name),
			Label:  "metrics view",
			Kind:   "metrics",
		})
	}

	return graph
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nodeID(kind, name string) string {
	return kind + ":" + name
}

func modelEndpoint(path string) (string, string) {
	parts := strings.Split(path, ".")
	if len(parts) < 3 {
		return path, ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

func cacheFields() []dashboard.ModelField {
	return []dashboard.ModelField{
		{Name: "order_id", Role: "key"},
		{Name: "purchase_month", Role: "time"},
		{Name: "status", Role: "dimension"},
		{Name: "state", Role: "dimension"},
		{Name: "category", Role: "dimension"},
		{Name: "revenue", Role: "measure"},
		{Name: "review_score", Role: "measure"},
		{Name: "delivery_days", Role: "measure"},
	}
}

func refreshLabel(runtime *modelRuntime) string {
	if runtime.lastRefresh.IsZero() {
		return time.Now().Format("15:04:05")
	}
	return runtime.lastRefresh.Format("15:04:05")
}

func measureCount(modelID string, metricViews map[string]*semantic.MetricView) int {
	count := 0
	for _, view := range metricViews {
		if view.SemanticModel == modelID {
			count += len(view.Measures)
		}
	}
	return count
}

func formatMetric(value float64, format string) string {
	switch format {
	case "currency":
		return formatCurrency(value)
	case "integer":
		return formatInt(int64(math.Round(value)))
	case "decimal":
		return fmt.Sprintf("%.2f", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func formatCurrency(value float64) string {
	if value >= 1000000 {
		return fmt.Sprintf("R$ %.1fm", value/1000000)
	}
	if value >= 1000 {
		return fmt.Sprintf("R$ %.1fk", value/1000)
	}
	return fmt.Sprintf("R$ %.0f", value)
}

func formatInt(value int64) string {
	if value >= 1000000 {
		return fmt.Sprintf("%.1fm", float64(value)/1000000)
	}
	if value >= 1000 {
		return fmt.Sprintf("%.1fk", float64(value)/1000)
	}
	return fmt.Sprintf("%d", value)
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}
