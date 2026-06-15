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
	"github.com/Yacobolo/libredash/internal/semantic"
	_ "github.com/marcboeker/go-duckdb/v2"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type MissingDataError struct {
	DataDir string
	Missing []string
}

func (e *MissingDataError) Error() string {
	return fmt.Sprintf("Olist CSVs are missing in %s: %s. Run scripts/bootstrap_olist.py or set LIBREDASH_DATA_DIR.", e.DataDir, strings.Join(e.Missing, ", "))
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
	db          *sql.DB
	dbPath      string
	model       *semantic.Model
	ready       bool
	missing     error
	lastRefresh time.Time
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
			dbPath: duckDBPath(dataDir, modelID),
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

func (m *DuckDBMetrics) DefaultDashboardID() string {
	return m.defaultID
}

func (m *DuckDBMetrics) ModelIDForDashboard(dashboardID string) string {
	report, ok := m.workspace.Dashboards[dashboardID]
	if !ok {
		return ""
	}
	return report.SemanticModel
}

func (m *DuckDBMetrics) Report(dashboardID string) (semantic.Dashboard, *semantic.Model, bool) {
	report, ok := m.workspace.Dashboards[dashboardID]
	if !ok {
		return semantic.Dashboard{}, nil, false
	}
	model, ok := m.workspace.Models[report.SemanticModel]
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
	for _, name := range sortedKeys(report.Tables) {
		table := report.Tables[name]
		defaults.Table = name
		defaults.Sort = table.DefaultSort
		break
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
	return modelGraph(model), true
}

func (m *DuckDBMetrics) catalogView() dashboard.Catalog {
	catalog := dashboard.Catalog{
		Models:     make([]dashboard.CatalogModel, 0, len(m.workspace.Catalog.SemanticModels)),
		Dashboards: make([]dashboard.CatalogDashboard, 0, len(m.workspace.Catalog.Dashboards)),
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
	for _, report := range m.workspace.Catalog.Dashboards {
		pageCount := 0
		if loaded, ok := m.workspace.Dashboards[report.ID]; ok {
			pageCount = len(loaded.Pages)
		}
		catalog.Dashboards = append(catalog.Dashboards, dashboard.CatalogDashboard{
			ID:            report.ID,
			Title:         report.Title,
			Description:   report.Description,
			SemanticModel: report.SemanticModel,
			ModelTitle:    modelTitles[report.SemanticModel],
			Tags:          append([]string{}, report.Tags...),
			PageCount:     pageCount,
		})
	}
	return catalog
}

func (m *DuckDBMetrics) reportRuntime(dashboardID string) (*semantic.Dashboard, *modelRuntime, error) {
	report, ok := m.workspace.Dashboards[dashboardID]
	if !ok {
		return nil, nil, fmt.Errorf("unknown dashboard %q", dashboardID)
	}
	runtime, ok := m.runtimes[report.SemanticModel]
	if !ok {
		return nil, nil, fmt.Errorf("unknown semantic model %q", report.SemanticModel)
	}
	return report, runtime, nil
}

func (m *DuckDBMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	report, runtime, err := m.reportRuntime(dashboardID)
	if report != nil {
		filters = normalizeFilters(report, filters)
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
		Charts: map[string]dashboard.Chart{},
	}

	options, err := m.filterOptions(ctx, runtime, report)
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	patch.FilterOptions = options

	kpis, err := m.kpis(ctx, runtime, report, filters)
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	patch.KPIs = kpis

	charts, err := m.charts(ctx, runtime, report, filters)
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	patch.Charts = charts

	return patch, nil
}

func (m *DuckDBMetrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	report, runtime, err := m.reportRuntime(dashboardID)
	if report != nil {
		filters = normalizeFilters(report, filters)
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

	totalRows, err := m.countRows(ctx, runtime, report, tableModel.Dataset, filters, "table", request.Table)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	availableRows := min(totalRows, dashboard.TableInteractiveRowCap)
	blocks, err := m.tableBlocks(ctx, runtime, report, tableModel, filters, request, availableRows)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}

	return dashboard.Table{
		Version:       2,
		Title:         tableModel.Title,
		Columns:       tableModel.Columns,
		TotalRows:     totalRows,
		AvailableRows: availableRows,
		IsCapped:      totalRows > availableRows,
		RowCap:        dashboard.TableInteractiveRowCap,
		ChunkSize:     dashboard.TableChunkSize,
		RowHeight:     dashboard.TableRowHeight,
		ResetVersion:  request.ResetVersion,
		Sort:          request.Sort,
		Blocks:        blocks,
		LoadingBlock:  "",
		Error:         "",
	}, nil
}

func (m *DuckDBMetrics) filterOptions(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard) (map[string][]dashboard.FilterOption, error) {
	options := map[string][]dashboard.FilterOption{}
	for _, name := range sortedKeys(report.Filters) {
		filter := report.Filters[name]
		if filter.Values.Source != "distinct" {
			continue
		}
		source, err := datasetSource(runtime.model, filter.Dataset)
		if err != nil {
			return nil, err
		}
		dataset := runtime.model.Datasets[filter.Dataset]
		dimension := dataset.Dimensions[filter.Dimension]
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

func normalizeFilters(report *semantic.Dashboard, filters dashboard.Filters) dashboard.Filters {
	defaults := report.DefaultFilters()
	filters = filters.WithDefaults()
	for name, control := range filters.Controls {
		filter, ok := report.Filters[name]
		if !ok {
			continue
		}
		base := defaults.Controls[name]
		if control.Type == "" {
			control.Type = filter.Type
		}
		switch filter.Type {
		case "date_range":
			if control.Preset == "" && control.From == "" && control.To == "" {
				control.Preset = base.Preset
			}
		case "multi_select":
			if control.Operator == "" {
				control.Operator = base.Operator
			}
			if control.Values == nil {
				control.Values = []string{}
			}
		case "text":
			if control.Operator == "" {
				control.Operator = base.Operator
			}
		}
		defaults.Controls[name] = control
	}
	defaults.VisualSelections = append([]dashboard.VisualSelection{}, filters.VisualSelections...)
	return defaults.WithDefaults()
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
	for _, file := range runtime.model.SourceFiles() {
		if _, err := os.Stat(filepath.Join(m.dataDir, file)); errors.Is(err, os.ErrNotExist) {
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

	for name, source := range runtime.model.Sources {
		if err := validateIdentifier(name); err != nil {
			return err
		}
		path := filepath.Join(m.dataDir, source.File)
		stmt := fmt.Sprintf("CREATE OR REPLACE VIEW raw.%s AS SELECT * FROM read_csv_auto('%s', header=true)", name, sqlString(path))
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

func (m *DuckDBMetrics) kpis(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, filters dashboard.Filters) ([]dashboard.KPI, error) {
	keys := []string{"total_orders", "revenue", "aov", "review"}
	seen := map[string]struct{}{}
	for _, key := range keys {
		seen[key] = struct{}{}
	}
	for _, key := range sortedKeys(report.KPIs) {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	kpis := make([]dashboard.KPI, 0, len(keys))
	for _, key := range keys {
		kpi, ok := report.KPIs[key]
		if !ok {
			continue
		}
		value, err := m.kpiValue(ctx, runtime, report, kpi, filters)
		if err != nil {
			return nil, err
		}
		measure := runtime.model.Datasets[kpi.Dataset].Measures[kpi.Measure]
		kpis = append(kpis, dashboard.KPI{
			Label: kpi.Title,
			Value: formatMetric(value, measure.Format),
			Note:  kpi.Note,
			Tone:  kpi.Tone,
		})
	}
	return kpis, nil
}

func (m *DuckDBMetrics) kpiValue(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, kpi semantic.KPI, filters dashboard.Filters) (float64, error) {
	source, err := datasetSource(runtime.model, kpi.Dataset)
	if err != nil {
		return 0, err
	}
	dataset := runtime.model.Datasets[kpi.Dataset]
	measure := dataset.Measures[kpi.Measure]
	expr, err := measureAggregateExpr(measure)
	if err != nil {
		return 0, err
	}
	where, args := m.filterWhere("e", runtime, report, kpi.Dataset, filters, "kpi", kpi.Measure)
	query := fmt.Sprintf("SELECT COALESCE(%s, 0) FROM %s e WHERE %s", expr, source, where)

	var value float64
	if err := runtime.db.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
		return 0, err
	}
	return value, nil
}

func (m *DuckDBMetrics) charts(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, filters dashboard.Filters) (map[string]dashboard.Chart, error) {
	charts := make(map[string]dashboard.Chart, len(report.Visuals))
	keys := make([]string, 0, len(report.Visuals))
	for key := range report.Visuals {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		visual := report.Visuals[key]
		data, err := m.visualData(ctx, runtime, report, key, visual, filters)
		if err != nil {
			return nil, err
		}
		dataset := runtime.model.Datasets[visual.Dataset]
		measureName := visual.Query.Measures[0]
		measure := dataset.Measures[measureName]
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
		charts[key] = dashboard.Chart{
			Version:         3,
			ID:              key,
			Kind:            visual.KindOrDefault(),
			Shape:           visual.ShapeOrDefault(),
			Renderer:        visual.RendererOrDefault(),
			Type:            visual.Type,
			Title:           visual.Title,
			Unit:            measure.Unit,
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
	return charts, nil
}

func (m *DuckDBMetrics) visualData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	switch visual.ShapeOrDefault() {
	case "single_value":
		return m.singleValueData(ctx, runtime, report, visualID, visual, filters)
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
	source, err := datasetSource(runtime.model, visual.Dataset)
	if err != nil {
		return nil, err
	}
	dataset := runtime.model.Datasets[visual.Dataset]
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

	where, args := m.filterWhere("e", runtime, report, visual.Dataset, filters, "visual", visualID)
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

func (m *DuckDBMetrics) singleValueData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	source, err := datasetSource(runtime.model, visual.Dataset)
	if err != nil {
		return nil, err
	}
	dataset := runtime.model.Datasets[visual.Dataset]
	measureName := visual.Query.Measures[0]
	valueExpr, err := measureAggregateExpr(dataset.Measures[measureName])
	if err != nil {
		return nil, err
	}
	labelExpr := "'" + strings.ReplaceAll(visual.Title, "'", "''") + "'"
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
	source, err := datasetSource(runtime.model, visual.Dataset)
	if err != nil {
		return nil, err
	}
	dataset := runtime.model.Datasets[visual.Dataset]
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
	source, err := datasetSource(runtime.model, visual.Dataset)
	if err != nil {
		return nil, err
	}
	dataset := runtime.model.Datasets[visual.Dataset]
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
	source, err := datasetSource(runtime.model, visual.Dataset)
	if err != nil {
		return nil, err
	}
	dataset := runtime.model.Datasets[visual.Dataset]
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
	source, err := datasetSource(runtime.model, visual.Dataset)
	if err != nil {
		return nil, err
	}
	dataset := runtime.model.Datasets[visual.Dataset]
	labelExpr := dimensionExpression(dataset.Dimensions[visual.Query.Dimensions[0]], "e")
	measure := dataset.Measures[visual.Query.Measures[0]]
	if err := validateIdentifier(measure.Column); err != nil {
		return nil, err
	}
	columnExpr := "e." + measure.Column
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
	dataset := runtime.model.Datasets[visual.Dataset]
	where, args := m.filterWhere("e", runtime, report, visual.Dataset, filters, "visual", visualID)
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

func (m *DuckDBMetrics) countRows(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, datasetName string, filters dashboard.Filters, targetKind, targetID string) (int, error) {
	source, err := datasetSource(runtime.model, datasetName)
	if err != nil {
		return 0, err
	}
	where, args := m.filterWhere("e", runtime, report, datasetName, filters, targetKind, targetID)
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s e WHERE %s", source, where)

	var total int
	if err := runtime.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (m *DuckDBMetrics) tableBlocks(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, table semantic.TableVisual, filters dashboard.Filters, request dashboard.TableRequest, availableRows int) (map[string]dashboard.TableBlock, error) {
	blocks := map[string]dashboard.TableBlock{}
	count := request.Count
	if count <= 0 {
		count = dashboard.TableChunkSize
	}
	if count > dashboard.TableMaxRequestCount {
		count = dashboard.TableMaxRequestCount
	}
	if request.Block == "all" {
		starts := initialBlockStarts(request.Start, count, availableRows)
		for block, start := range starts {
			rows, err := m.tableRows(ctx, runtime, report, table, filters, request, start, count, availableRows)
			if err != nil {
				return nil, err
			}
			blocks[block] = dashboard.TableBlock{
				Start:        start,
				RequestSeq:   request.RequestSeq,
				ResetVersion: request.ResetVersion,
				Sort:         request.Sort,
				Rows:         rows,
			}
		}
		return blocks, nil
	}

	start := clampTableStart(request.Start, availableRows)
	rows, err := m.tableRows(ctx, runtime, report, table, filters, request, start, count, availableRows)
	if err != nil {
		return nil, err
	}
	blocks[request.Block] = dashboard.TableBlock{
		Start:        start,
		RequestSeq:   request.RequestSeq,
		ResetVersion: request.ResetVersion,
		Sort:         request.Sort,
		Rows:         rows,
	}
	return blocks, nil
}

func initialBlockStarts(start, count, availableRows int) map[string]int {
	start = clampTableStart(start, availableRows)
	if start <= 0 {
		return map[string]int{"a": 0, "b": count, "c": count * 2}
	}
	base := (start / count) * count
	return map[string]int{"a": max(0, base-count), "b": base, "c": base + count}
}

func clampTableStart(start, availableRows int) int {
	if start < 0 {
		return 0
	}
	if availableRows <= 0 {
		return 0
	}
	if start >= availableRows {
		return max(0, availableRows-1)
	}
	return start
}

func (m *DuckDBMetrics) tableRows(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, table semantic.TableVisual, filters dashboard.Filters, request dashboard.TableRequest, start, count, availableRows int) ([]map[string]any, error) {
	if count <= 0 || start >= availableRows {
		return []map[string]any{}, nil
	}
	if start+count > availableRows {
		count = availableRows - start
	}
	source, err := datasetSource(runtime.model, table.Dataset)
	if err != nil {
		return nil, err
	}
	where, args := m.filterWhere("e", runtime, report, table.Dataset, filters, "table", request.Table)
	sortExpr := tableSortExpr(table, request.Sort.Key)
	direction := "DESC"
	if request.Sort.Direction == "asc" {
		direction = "ASC"
	}

	selects := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		if err := validateIdentifier(column.Key); err != nil {
			return nil, err
		}
		selects = append(selects, "e."+column.Key)
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s e
WHERE %s
ORDER BY %s %s, e.order_id ASC
LIMIT ? OFFSET ?`, strings.Join(selects, ", "), source, where, sortExpr, direction)

	args = append(args, count, start)
	rows, err := runtime.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]any, len(table.Columns))
	scans := make([]any, len(table.Columns))
	for i := range values {
		scans[i] = &values[i]
	}

	result := []map[string]any{}
	for rows.Next() {
		if err := rows.Scan(scans...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, column := range table.Columns {
			row[column.Key] = normalizeDBValue(values[i])
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func measureAggregateExpr(measure semantic.Measure) (string, error) {
	switch measure.Aggregate {
	case "count":
		return "COUNT(*)", nil
	case "count_distinct":
		if err := validateIdentifier(measure.Column); err != nil {
			return "", err
		}
		return "COUNT(DISTINCT e." + measure.Column + ")", nil
	case "sum":
		if err := validateIdentifier(measure.Column); err != nil {
			return "", err
		}
		return "SUM(e." + measure.Column + ")", nil
	case "avg":
		if err := validateIdentifier(measure.Column); err != nil {
			return "", err
		}
		return "AVG(e." + measure.Column + ")", nil
	case "expression":
		if measure.Expression == "" {
			return "", fmt.Errorf("measure %q is missing expression", measure.Label)
		}
		return measure.Expression, nil
	default:
		return "", fmt.Errorf("unsupported measure aggregate %q", measure.Aggregate)
	}
}

func tableSortExpr(table semantic.TableVisual, key string) string {
	if key == "" {
		key = table.DefaultSort.Key
	}
	for _, column := range table.Columns {
		if column.Key == key {
			return "e." + column.Key
		}
	}
	if table.DefaultSort.Key != "" {
		return "e." + table.DefaultSort.Key
	}
	return "e.order_id"
}

func (m *DuckDBMetrics) visualOrderBy(model *semantic.Model, visual semantic.Visual) string {
	if len(visual.Query.Sort) == 0 {
		return "label ASC"
	}
	dataset := model.Datasets[visual.Dataset]
	parts := make([]string, 0, len(visual.Query.Sort))
	for _, sortSpec := range visual.Query.Sort {
		direction := "ASC"
		if strings.EqualFold(sortSpec.Direction, "desc") {
			direction = "DESC"
		}
		expr := sortSpec.Expr
		if expr == "" {
			expr = m.sortExpression(dataset, visual, sortSpec.Field)
		}
		if expr == "" {
			expr = "label"
		}
		parts = append(parts, expr+" "+direction)
	}
	return strings.Join(parts, ", ")
}

func (m *DuckDBMetrics) sortExpression(dataset semantic.Dataset, visual semantic.Visual, field string) string {
	if field == "" {
		return defaultSortColumn(visual)
	}
	if field == "value" || field == visual.Query.Measures[0] {
		return "value"
	}
	if field == visual.Query.Series {
		return "series"
	}
	if dimension, ok := dataset.Dimensions[field]; ok {
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
	default:
		return "label"
	}
}

func (m *DuckDBMetrics) filterWhere(alias string, runtime *modelRuntime, report *semantic.Dashboard, datasetName string, filters dashboard.Filters, targetKind, targetID string) (string, []any) {
	filters = normalizeFilters(report, filters)
	conditions := []string{"1 = 1"}
	args := []any{}

	for _, name := range sortedKeys(report.Filters) {
		filter := report.Filters[name]
		if filter.Dataset != datasetName {
			continue
		}
		control, ok := filters.Controls[name]
		if !ok {
			continue
		}
		dataset, ok := runtime.model.Datasets[filter.Dataset]
		if !ok {
			continue
		}
		dimension, ok := dataset.Dimensions[filter.Dimension]
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
		dataset, ok := runtime.model.Datasets[datasetName]
		if !ok {
			continue
		}
		dimension, ok := dataset.Dimensions[selection.Field]
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
			source, err := datasetSource(runtime.model, filter.Dataset)
			if err != nil {
				return "", nil
			}
			dataset := runtime.model.Datasets[filter.Dataset]
			dimension := dataset.Dimensions[filter.Dimension]
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

func dimensionExpression(dimension semantic.Dimension, alias string) string {
	if identifierPattern.MatchString(dimension.Expr) {
		return alias + "." + dimension.Expr
	}
	return strings.ReplaceAll(dimension.Expr, "{alias}", alias)
}

func dimensionWhere(dimension semantic.Dimension, alias string) string {
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

func datasetSource(model *semantic.Model, name string) (string, error) {
	dataset, ok := model.Datasets[name]
	if !ok {
		return "", fmt.Errorf("unknown dataset %q", name)
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

func modelGraph(model *semantic.Model) dashboard.ModelGraph {
	graph := dashboard.ModelGraph{
		Name:  model.Name,
		Title: model.Title,
		Stats: dashboard.ModelStats{
			Sources:       len(model.Sources),
			CacheTables:   len(model.Cache.Tables),
			Metrics:       measureCount(model),
			Visuals:       0,
			ReportTables:  0,
			Relationships: len(model.Relationships),
		},
	}

	for _, name := range sortedKeys(model.Sources) {
		source := model.Sources[name]
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:     nodeID("source", name),
			Label:  name,
			Kind:   "source",
			Schema: "raw",
			Fields: []dashboard.ModelField{{Name: source.File, Role: "csv"}},
			Meta: []dashboard.ModelMeta{
				{Label: "File", Value: source.File},
				{Label: "Schema", Value: "raw"},
			},
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
		fields := make([]dashboard.ModelField, 0, len(dataset.Dimensions)+len(dataset.Measures))
		for _, dimension := range sortedKeys(dataset.Dimensions) {
			fields = append(fields, dashboard.ModelField{Name: dimension, Role: "dimension"})
		}
		for _, measure := range sortedKeys(dataset.Measures) {
			fields = append(fields, dashboard.ModelField{Name: measure, Role: "measure"})
		}
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:     nodeID("dataset", name),
			Label:  name,
			Kind:   "dataset",
			Schema: "semantic",
			Fields: fields,
			Meta: []dashboard.ModelMeta{
				{Label: "Source", Value: dataset.Source},
				{Label: "Dimensions", Value: strconv.Itoa(len(dataset.Dimensions))},
				{Label: "Measures", Value: strconv.Itoa(len(dataset.Measures))},
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

func measureCount(model *semantic.Model) int {
	count := 0
	for _, dataset := range model.Datasets {
		count += len(dataset.Measures)
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
