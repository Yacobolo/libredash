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
	mu          sync.RWMutex
	db          *sql.DB
	dataDir     string
	dbPath      string
	model       *semantic.Model
	modelPath   string
	ready       bool
	missing     error
	lastRefresh time.Time
}

func NewDuckDBMetrics(dataDir string) (*DuckDBMetrics, error) {
	modelPath := os.Getenv("LIBREDASH_MODEL_PATH")
	if modelPath == "" {
		var err error
		modelPath, err = discoverModelPath()
		if err != nil {
			return nil, err
		}
	}

	model, err := semantic.Load(modelPath)
	if err != nil {
		return nil, fmt.Errorf("loading semantic model: %w", err)
	}

	metrics := &DuckDBMetrics{
		dataDir:   dataDir,
		dbPath:    duckDBPath(dataDir),
		model:     model,
		modelPath: modelPath,
	}
	if err := metrics.validateFiles(); err != nil {
		metrics.missing = err
		return metrics, nil
	}

	if err := os.MkdirAll(filepath.Dir(metrics.dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("duckdb", metrics.dbPath)
	if err != nil {
		return nil, err
	}
	metrics.db = db

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := metrics.RefreshCache(context.Background()); err != nil {
		db.Close()
		return nil, err
	}

	metrics.ready = true
	return metrics, nil
}

func (m *DuckDBMetrics) Close() error {
	if m.db == nil {
		return nil
	}
	return m.db.Close()
}

func (m *DuckDBMetrics) DataDir() string {
	return m.dataDir
}

func (m *DuckDBMetrics) Pages() []dashboard.Page {
	pages := make([]dashboard.Page, len(m.model.Pages))
	for i, page := range m.model.Pages {
		pages[i] = page.WithDefaults()
	}
	return pages
}

func (m *DuckDBMetrics) ModelGraph() dashboard.ModelGraph {
	return modelGraph(m.model)
}

func (m *DuckDBMetrics) QueryDashboard(ctx context.Context, filters dashboard.Filters) (dashboard.Patch, error) {
	filters = filters.WithDefaults()
	if !m.ready {
		return dashboard.EmptyPatch(filters, m.dataDir, m.missing), nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	patch := dashboard.Patch{
		Filters: filters,
		Status: dashboard.Status{
			Loading:       false,
			LastUpdated:   m.refreshLabel(),
			DataDirectory: m.dataDir,
		},
		Charts: map[string]dashboard.Chart{},
	}

	kpis, err := m.kpis(ctx, filters)
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	patch.KPIs = kpis

	charts, err := m.charts(ctx, filters)
	if err != nil {
		return dashboard.EmptyPatch(filters, m.dataDir, err), nil
	}
	patch.Charts = charts

	return patch, nil
}

func (m *DuckDBMetrics) QueryTable(ctx context.Context, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	filters = filters.WithDefaults()
	request = request.WithDefaults()
	if !m.ready {
		return dashboard.EmptyTable(request, m.missing), nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	tableModel, ok := m.model.Tables[request.Table]
	if !ok {
		return dashboard.EmptyTable(request, fmt.Errorf("unknown table %q", request.Table)), nil
	}

	totalRows, err := m.countRows(ctx, tableModel.Source, filters)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	rows, err := m.tableRows(ctx, tableModel, filters, request)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}

	return dashboard.Table{
		Title:     tableModel.Title,
		Columns:   tableModel.Columns,
		Rows:      rows,
		TotalRows: totalRows,
		Window:    dashboard.TableWindow{Offset: request.Offset, Limit: request.Limit},
		Sort:      request.Sort,
		Loading:   false,
		Error:     "",
	}, nil
}

func (m *DuckDBMetrics) RefreshCache(ctx context.Context) error {
	if m.missing != nil {
		return m.missing
	}
	if m.db == nil {
		return fmt.Errorf("DuckDB is not initialized")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.registerSourceViews(ctx); err != nil {
		return err
	}
	if err := m.materializeCache(ctx); err != nil {
		return err
	}
	m.lastRefresh = time.Now()
	return nil
}

func (m *DuckDBMetrics) validateFiles() error {
	var missing []string
	for _, file := range m.model.SourceFiles() {
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

func (m *DuckDBMetrics) registerSourceViews(ctx context.Context) error {
	if _, err := m.db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS raw"); err != nil {
		return err
	}
	if _, err := m.db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS cache"); err != nil {
		return err
	}

	for name, source := range m.model.Sources {
		if err := validateIdentifier(name); err != nil {
			return err
		}
		path := filepath.Join(m.dataDir, source.File)
		stmt := fmt.Sprintf("CREATE OR REPLACE VIEW raw.%s AS SELECT * FROM read_csv_auto('%s', header=true)", name, sqlString(path))
		if _, err := m.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("registering source %s: %w", name, err)
		}
	}
	return nil
}

func (m *DuckDBMetrics) materializeCache(ctx context.Context) error {
	for _, name := range m.model.CacheTableNames() {
		if err := validateIdentifier(name); err != nil {
			return err
		}
		table := m.model.Cache.Tables[name]
		stmt := fmt.Sprintf("CREATE OR REPLACE TABLE cache.%s AS %s", name, table.SQL)
		if _, err := m.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("materializing cache.%s: %w", name, err)
		}
	}
	return nil
}

func (m *DuckDBMetrics) kpis(ctx context.Context, filters dashboard.Filters) ([]dashboard.KPI, error) {
	keys := []string{"total_orders", "revenue", "aov", "review"}
	kpis := make([]dashboard.KPI, 0, len(keys))
	for _, key := range keys {
		metric, ok := m.model.Metrics[key]
		if !ok {
			continue
		}
		value, err := m.metricValue(ctx, metric, filters)
		if err != nil {
			return nil, err
		}
		kpis = append(kpis, dashboard.KPI{
			Label: metric.Title,
			Value: formatMetric(value, metric.Format),
			Note:  metric.Note,
			Tone:  metric.Tone,
		})
	}
	return kpis, nil
}

func (m *DuckDBMetrics) metricValue(ctx context.Context, metric semantic.Metric, filters dashboard.Filters) (float64, error) {
	source, err := cacheSource(metric.Source)
	if err != nil {
		return 0, err
	}
	expr, err := metricAggregateExpr(metric)
	if err != nil {
		return 0, err
	}
	where, args := filterWhere("e", filters, "")
	query := fmt.Sprintf("SELECT COALESCE(%s, 0) FROM %s e WHERE %s", expr, source, where)

	var value float64
	if err := m.db.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
		return 0, err
	}
	return value, nil
}

func (m *DuckDBMetrics) charts(ctx context.Context, filters dashboard.Filters) (map[string]dashboard.Chart, error) {
	charts := make(map[string]dashboard.Chart, len(m.model.Visuals))
	keys := make([]string, 0, len(m.model.Visuals))
	for key := range m.model.Visuals {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		visual := m.model.Visuals[key]
		points, err := m.visualPoints(ctx, key, visual, filters)
		if err != nil {
			return nil, err
		}
		charts[key] = dashboard.Chart{
			Version:   1,
			ID:        key,
			Type:      chartType(visual),
			Title:     visual.Title,
			Unit:      visual.Unit,
			Field:     visualField(visual),
			Selection: selectedValues(filters, key),
			Data:      points,
		}
	}
	return charts, nil
}

func (m *DuckDBMetrics) visualPoints(ctx context.Context, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Point, error) {
	source, err := cacheSource(visual.Source)
	if err != nil {
		return nil, err
	}
	labelExpr, err := labelExpression(visual)
	if err != nil {
		return nil, err
	}
	valueExpr, err := visualAggregateExpr(visual)
	if err != nil {
		return nil, err
	}

	where, args := filterWhere("e", filters, visualID)
	if visual.Where != "" {
		where = fmt.Sprintf("(%s) AND (%s)", where, visual.Where)
	}

	orderBy := visual.OrderBy
	if orderBy == "" {
		orderBy = "label ASC"
	}
	query := fmt.Sprintf(`
SELECT %s AS label, %s AS value
FROM %s e
WHERE %s
GROUP BY label
ORDER BY %s`, labelExpr, valueExpr, source, where, orderBy)
	if visual.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", visual.Limit)
	}

	points, err := m.queryPoints(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	markSelected(points, selectedValues(filters, visualID))
	return points, nil
}

func (m *DuckDBMetrics) queryPoints(ctx context.Context, query string, args ...any) ([]dashboard.Point, error) {
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []dashboard.Point{}
	for rows.Next() {
		var label string
		var value float64
		if err := rows.Scan(&label, &value); err != nil {
			return nil, err
		}
		points = append(points, dashboard.Point{Label: label, Value: round(value)})
	}
	return points, rows.Err()
}

func (m *DuckDBMetrics) countRows(ctx context.Context, sourceName string, filters dashboard.Filters) (int, error) {
	source, err := cacheSource(sourceName)
	if err != nil {
		return 0, err
	}
	where, args := filterWhere("e", filters, "")
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s e WHERE %s", source, where)

	var total int
	if err := m.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (m *DuckDBMetrics) tableRows(ctx context.Context, table semantic.TableVisual, filters dashboard.Filters, request dashboard.TableRequest) ([]map[string]any, error) {
	source, err := cacheSource(table.Source)
	if err != nil {
		return nil, err
	}
	where, args := filterWhere("e", filters, "")
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

	args = append(args, request.Limit, request.Offset)
	rows, err := m.db.QueryContext(ctx, query, args...)
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

func metricAggregateExpr(metric semantic.Metric) (string, error) {
	switch metric.Aggregate {
	case "count":
		return "COUNT(*)", nil
	case "count_distinct":
		if err := validateIdentifier(metric.Column); err != nil {
			return "", err
		}
		return "COUNT(DISTINCT e." + metric.Column + ")", nil
	case "sum":
		if err := validateIdentifier(metric.Column); err != nil {
			return "", err
		}
		return "SUM(e." + metric.Column + ")", nil
	case "avg":
		if err := validateIdentifier(metric.Column); err != nil {
			return "", err
		}
		return "AVG(e." + metric.Column + ")", nil
	case "expression":
		if metric.Expression == "" {
			return "", fmt.Errorf("metric %q is missing expression", metric.Title)
		}
		return metric.Expression, nil
	default:
		return "", fmt.Errorf("unsupported metric aggregate %q", metric.Aggregate)
	}
}

func visualAggregateExpr(visual semantic.Visual) (string, error) {
	switch visual.Aggregate {
	case "count":
		return "COUNT(*)", nil
	case "count_distinct":
		if err := validateIdentifier(visual.Value); err != nil {
			return "", err
		}
		return "COUNT(DISTINCT e." + visual.Value + ")", nil
	case "sum":
		if err := validateIdentifier(visual.Value); err != nil {
			return "", err
		}
		return "SUM(e." + visual.Value + ")", nil
	case "avg":
		if err := validateIdentifier(visual.Value); err != nil {
			return "", err
		}
		return "AVG(e." + visual.Value + ")", nil
	case "expression":
		if visual.ValueExpr == "" {
			return "", fmt.Errorf("visual %q is missing value_expr", visual.Title)
		}
		return visual.ValueExpr, nil
	default:
		return "", fmt.Errorf("unsupported visual aggregate %q", visual.Aggregate)
	}
}

func labelExpression(visual semantic.Visual) (string, error) {
	if visual.LabelExpr != "" {
		return visual.LabelExpr, nil
	}
	if err := validateIdentifier(visual.Label); err != nil {
		return "", err
	}
	return "e." + visual.Label, nil
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

func filterWhere(alias string, filters dashboard.Filters, excludeVisualID string) (string, []any) {
	filters = filters.WithDefaults()
	conditions := []string{"1 = 1"}
	args := []any{}

	if filters.State != "" && filters.State != "all" {
		conditions = append(conditions, alias+".state = ?")
		args = append(args, strings.ToUpper(filters.State))
	}

	switch filters.DateRange {
	case "2017":
		conditions = append(conditions, alias+".purchase_timestamp >= TIMESTAMP '2017-01-01' AND "+alias+".purchase_timestamp < TIMESTAMP '2018-01-01'")
	case "2018":
		conditions = append(conditions, alias+".purchase_timestamp >= TIMESTAMP '2018-01-01' AND "+alias+".purchase_timestamp < TIMESTAMP '2019-01-01'")
	case "recent":
		conditions = append(conditions, alias+".purchase_timestamp >= (SELECT max(purchase_timestamp) - INTERVAL 90 DAY FROM cache.orders_enriched)")
	}

	if filters.Category != "" && filters.Category != "all" {
		conditions = append(conditions, "lower("+alias+".category) LIKE lower(?)")
		args = append(args, "%"+filters.Category+"%")
	}

	for _, selection := range filters.VisualSelections {
		if selection.VisualID == "" || selection.VisualID == excludeVisualID || len(selection.Values) == 0 {
			continue
		}
		if selection.Operator != "" && selection.Operator != "in" {
			continue
		}
		if err := validateIdentifier(selection.Field); err != nil {
			continue
		}
		placeholders := make([]string, 0, len(selection.Values))
		for _, value := range selection.Values {
			placeholders = append(placeholders, "?")
			args = append(args, value)
		}
		conditions = append(conditions, alias+"."+selection.Field+" IN ("+strings.Join(placeholders, ", ")+")")
	}

	return strings.Join(conditions, " AND "), args
}

func visualField(visual semantic.Visual) string {
	if visual.Label != "" {
		return visual.Label
	}
	return "label"
}

func chartType(visual semantic.Visual) string {
	if visual.Type != "" {
		return visual.Type
	}
	return "bar"
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

func markSelected(points []dashboard.Point, values []string) {
	if len(values) == 0 {
		return
	}
	selected := make(map[string]struct{}, len(values))
	for _, value := range values {
		selected[value] = struct{}{}
	}
	for i := range points {
		if _, ok := selected[points[i].Label]; ok {
			points[i].Selected = true
		}
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

func discoverModelPath() (string, error) {
	candidates := []string{
		filepath.Join("dashboards", "olist.yaml"),
		filepath.Join("..", "dashboards", "olist.yaml"),
		filepath.Join("..", "..", "dashboards", "olist.yaml"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find dashboards/olist.yaml")
}

func duckDBPath(dataDir string) string {
	if path := os.Getenv("LIBREDASH_DUCKDB_PATH"); path != "" {
		return path
	}
	return filepath.Join(dataDir, "libredash.duckdb")
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
			Metrics:       len(model.Metrics),
			Visuals:       len(model.Visuals),
			ReportTables:  len(model.Tables),
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

	for _, name := range sortedKeys(model.Metrics) {
		metric := model.Metrics[name]
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:          nodeID("metric", name),
			Label:       metric.Title,
			Kind:        "metric",
			Description: metric.Note,
			Fields: []dashboard.ModelField{
				{Name: metric.Source, Role: "source"},
				{Name: metricAggregateLabel(metric.Aggregate, metric.Column), Role: "measure"},
			},
			Meta: []dashboard.ModelMeta{
				{Label: "Format", Value: metric.Format},
				{Label: "Aggregate", Value: metric.Aggregate},
			},
		})
		graph.Edges = append(graph.Edges, semanticEdge("metric", name, metric.Source, "measure"))
	}

	for _, name := range sortedKeys(model.Visuals) {
		visual := model.Visuals[name]
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:    nodeID("visual", name),
			Label: visual.Title,
			Kind:  "visual",
			Fields: []dashboard.ModelField{
				{Name: visualField(visual), Role: "axis"},
				{Name: metricAggregateLabel(visual.Aggregate, visual.Value), Role: "value"},
			},
			Meta: []dashboard.ModelMeta{
				{Label: "Unit", Value: visual.Unit},
				{Label: "Limit", Value: intLabel(visual.Limit)},
			},
		})
		graph.Edges = append(graph.Edges, semanticEdge("visual", name, visual.Source, "visual"))
	}

	for _, name := range sortedKeys(model.Tables) {
		table := model.Tables[name]
		fields := make([]dashboard.ModelField, 0, len(table.Columns))
		for _, column := range table.Columns {
			role := "column"
			if column.Align == "right" {
				role = "measure"
			}
			fields = append(fields, dashboard.ModelField{Name: column.Key, Role: role})
		}
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:     nodeID("table", name),
			Label:  table.Title,
			Kind:   "report_table",
			Fields: fields,
			Meta: []dashboard.ModelMeta{
				{Label: "Default sort", Value: table.DefaultSort.Key + " " + table.DefaultSort.Direction},
				{Label: "Columns", Value: strconv.Itoa(len(table.Columns))},
			},
		})
		graph.Edges = append(graph.Edges, semanticEdge("table", name, table.Source, "table"))
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

func semanticEdge(kind, name, source, label string) dashboard.ModelEdge {
	return dashboard.ModelEdge{
		ID:     kind + "_" + name + "_from_" + source,
		Source: nodeID("cache", source),
		Target: nodeID(kind, name),
		Label:  label,
		Kind:   "semantic",
	}
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

func metricAggregateLabel(aggregate, column string) string {
	if column == "" {
		return aggregate
	}
	return aggregate + "(" + column + ")"
}

func intLabel(value int) string {
	if value == 0 {
		return "all"
	}
	return strconv.Itoa(value)
}

func (m *DuckDBMetrics) refreshLabel() string {
	if m.lastRefresh.IsZero() {
		return time.Now().Format("15:04:05")
	}
	return m.lastRefresh.Format("15:04:05")
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
