package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/deploy"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/semantic"
	_ "github.com/duckdb/duckdb-go/v2"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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
		if err := metrics.RefreshMaterializations(context.Background(), modelID); err != nil {
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
		return nil, nil, fmt.Errorf("dashboard %q has no metric views", dashboardID)
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

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func refreshLabel(runtime *modelRuntime) string {
	if runtime.lastRefresh.IsZero() {
		return time.Now().Format("15:04:05")
	}
	return runtime.lastRefresh.Format("15:04:05")
}
