package data

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	semanticquery "github.com/Yacobolo/libredash/internal/query"
	"github.com/Yacobolo/libredash/internal/semantic"
)

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

func (m *DuckDBMetrics) queryAggregateTable(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, request dashboard.TableRequest, table semantic.TableVisual, filters dashboard.Filters) (dashboard.Table, error) {
	var (
		columns []dashboard.TableColumn
		rows    []map[string]any
		err     error
	)
	switch table.KindOrDefault() {
	case "matrix_table":
		columns, rows, err = m.matrixTableRows(ctx, runtime, report, table, filters, request)
	case "pivot_table":
		columns, rows, err = m.pivotTableRows(ctx, runtime, report, table, filters, request)
	default:
		err = fmt.Errorf("unsupported aggregate table kind %q", table.KindOrDefault())
	}
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	totalRows := len(rows)
	isCapped := totalRows > dashboard.TableInteractiveRowCap
	if isCapped {
		rows = rows[:dashboard.TableInteractiveRowCap]
	}
	chunkSize := max(dashboard.TableChunkSize, len(rows))
	style := table.Style.WithDefaults()
	return dashboard.Table{
		Version:       2,
		Kind:          table.KindOrDefault(),
		Title:         table.Title,
		Style:         style,
		Columns:       columns,
		TotalRows:     totalRows,
		AvailableRows: len(rows),
		IsCapped:      isCapped,
		RowCap:        dashboard.TableInteractiveRowCap,
		ChunkSize:     chunkSize,
		RowHeight:     style.RowHeight(),
		ResetVersion:  request.ResetVersion,
		Sort:          request.Sort,
		Blocks: map[string]dashboard.TableBlock{
			"a": {Start: 0, RequestSeq: request.RequestSeq, ResetVersion: request.ResetVersion, Sort: request.Sort, Rows: rows},
			"b": {Start: chunkSize, RequestSeq: request.RequestSeq, ResetVersion: request.ResetVersion, Sort: request.Sort, Rows: []map[string]any{}},
			"c": {Start: chunkSize * 2, RequestSeq: request.RequestSeq, ResetVersion: request.ResetVersion, Sort: request.Sort, Rows: []map[string]any{}},
		},
		LoadingBlock: "",
		Error:        "",
	}, nil
}

func (m *DuckDBMetrics) matrixTableRows(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, table semantic.TableVisual, filters dashboard.Filters, request dashboard.TableRequest) ([]dashboard.TableColumn, []map[string]any, error) {
	if len(table.ColumnDims) == 1 {
		return m.crossTabTableRows(ctx, runtime, report, table, filters, request, false)
	}
	metricView := m.workspace.MetricViews[table.MetricView]
	columns := make([]dashboard.TableColumn, 0, len(table.Rows)+len(table.Measures))
	dimensions := make([]semanticquery.Field, 0, len(table.Rows))
	measures := make([]semanticquery.Field, 0, len(table.Measures))
	for _, dimensionName := range table.Rows {
		dimension := metricView.Dimensions[dimensionName]
		key := displayField(dimensionName)
		dimensions = append(dimensions, fieldRef(dimensionName, key))
		column := dashboard.TableColumn{Key: key, Label: dimensionLabel(key, dimension), Role: "row_header", Format: "text"}
		columns = append(columns, mergeTableColumn(column, tableColumnOverride(table, dimensionName)))
	}
	for _, measureName := range table.Measures {
		measure := metricView.Measures[measureName]
		key := displayField(measureName)
		measures = append(measures, fieldRef(measureName, key))
		column := dashboard.TableColumn{Key: key, Label: measureLabel(key, measure), Align: "right", Role: "measure", Measure: key, Format: tableMeasureFormat(measure), Formatting: tableMeasureFormatting(table, measureName)}
		columns = append(columns, mergeTableColumn(column, tableColumnOverride(table, measureName)))
	}
	queryFilters, err := m.semanticFilters(ctx, runtime, report, table.MetricView, filters, "table", request.Table)
	if err != nil {
		return nil, nil, err
	}
	sorts := make([]semanticquery.Sort, 0, len(dimensions))
	for _, dimension := range dimensions {
		sorts = append(sorts, semanticquery.Sort{Field: dimension.Alias, Direction: "asc"})
	}
	if request.Sort.Key != "" && tableHasColumn(columns, request.Sort.Key) {
		sorts = []semanticquery.Sort{{Field: request.Sort.Key, Direction: request.Sort.Direction}}
	}
	plan, err := semanticquery.NewPlanner(runtime.model, m.workspace.MetricViews).Plan(semanticquery.Request{
		MetricView: table.MetricView,
		Dimensions: dimensions,
		Measures:   measures,
		Filters:    queryFilters,
		Sort:       sorts,
		Limit:      dashboard.TableInteractiveRowCap + 1,
	})
	if err != nil {
		return nil, nil, err
	}
	rows, err := m.queryTableDatums(ctx, runtime, plan.SQL, plan.Columns, plan.Args...)
	return columns, rows, err
}

func (m *DuckDBMetrics) pivotTableRows(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, table semantic.TableVisual, filters dashboard.Filters, request dashboard.TableRequest) ([]dashboard.TableColumn, []map[string]any, error) {
	return m.crossTabTableRows(ctx, runtime, report, table, filters, request, true)
}

func (m *DuckDBMetrics) crossTabTableRows(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, table semantic.TableVisual, filters dashboard.Filters, request dashboard.TableRequest, pivotMode bool) ([]dashboard.TableColumn, []map[string]any, error) {
	metricView := m.workspace.MetricViews[table.MetricView]
	dimensions := make([]semanticquery.Field, 0, len(table.Rows)+1)
	baseColumns := make([]dashboard.TableColumn, 0, len(table.Rows))
	for _, dimensionName := range table.Rows {
		dimension := metricView.Dimensions[dimensionName]
		key := displayField(dimensionName)
		dimensions = append(dimensions, fieldRef(dimensionName, key))
		column := dashboard.TableColumn{Key: key, Label: dimensionLabel(key, dimension), Role: "row_header", Format: "text"}
		baseColumns = append(baseColumns, mergeTableColumn(column, tableColumnOverride(table, dimensionName)))
	}
	columnDimensionName := table.ColumnDims[0]
	dimensions = append(dimensions, fieldRef(columnDimensionName, "pivot_label"))
	measures := make([]semanticquery.Field, 0, len(table.Measures))
	valueColumns := make([]string, 0, len(table.Measures))
	for _, measureName := range table.Measures {
		key := displayField(measureName)
		measures = append(measures, fieldRef(measureName, key))
		valueColumns = append(valueColumns, key)
	}
	queryFilters, err := m.semanticFilters(ctx, runtime, report, table.MetricView, filters, "table", request.Table)
	if err != nil {
		return nil, nil, err
	}
	sorts := make([]semanticquery.Sort, 0, len(dimensions))
	for _, dimension := range dimensions {
		sorts = append(sorts, semanticquery.Sort{Field: dimension.Alias, Direction: "asc"})
	}
	plan, err := semanticquery.NewPlanner(runtime.model, m.workspace.MetricViews).Plan(semanticquery.Request{
		MetricView: table.MetricView,
		Dimensions: dimensions,
		Measures:   measures,
		Filters:    queryFilters,
		Sort:       sorts,
		Limit:      dashboard.TableInteractiveRowCap + 1,
	})
	if err != nil {
		return nil, nil, err
	}
	rawRows, err := m.queryTableDatums(ctx, runtime, plan.SQL, plan.Columns, plan.Args...)
	if err != nil {
		return nil, nil, err
	}
	columns := append([]dashboard.TableColumn{}, baseColumns...)
	pivotKeys := map[string]string{}
	usedKeys := map[string]string{}
	columnKeys := map[string]string{}
	for _, column := range baseColumns {
		usedKeys[column.Key] = column.Key
	}
	resultByKey := map[string]map[string]any{}
	order := []string{}
	for _, raw := range rawRows {
		rowKeyParts := make([]string, 0, len(table.Rows))
		for _, dimension := range table.Rows {
			rowKeyParts = append(rowKeyParts, fmt.Sprint(raw[displayField(dimension)]))
		}
		resultKey := strings.Join(rowKeyParts, "\x00")
		row, exists := resultByKey[resultKey]
		if !exists {
			row = map[string]any{}
			for _, dimension := range table.Rows {
				key := displayField(dimension)
				row[key] = raw[key]
			}
			resultByKey[resultKey] = row
			order = append(order, resultKey)
		}
		label := fmt.Sprint(raw["pivot_label"])
		groupLabel := label
		if pivotMode {
			groupLabel = measureLabel(displayField(table.Measures[0]), metricView.Measures[table.Measures[0]])
		}
		pivotKey, exists := pivotKeys[label]
		if !exists {
			pivotKey = sanitizeTableKey(label)
			pivotKeys[label] = pivotKey
		}
		for _, measureName := range table.Measures {
			measure := metricView.Measures[measureName]
			measureKey := displayField(measureName)
			columnIdentity := label + "\x00" + measureName
			columnKey, columnExists := columnKeys[columnIdentity]
			candidate := "pivot_" + pivotKey
			columnLabel := label
			if !pivotMode || len(table.Measures) > 1 {
				candidate += "__" + sanitizeTableKey(measureKey)
				columnLabel = measureLabel(measureKey, measure)
			}
			if !columnExists {
				columnKey = uniqueTableColumnKey(candidate, usedKeys)
				columnKeys[columnIdentity] = columnKey
				usedKeys[columnKey] = columnKey
				column := dashboard.TableColumn{
					Key:         columnKey,
					Label:       columnLabel,
					Align:       "right",
					Role:        "measure",
					Group:       groupLabel,
					Measure:     measureKey,
					ColumnValue: label,
					Format:      tableMeasureFormat(measure),
					Formatting:  tableMeasureFormatting(table, measureName),
				}
				columns = append(columns, mergeTableColumn(column, tableColumnOverride(table, measureName)))
			}
			row[columnKey] = raw[measureKey]
		}
	}
	result := make([]map[string]any, 0, len(order))
	for _, key := range order {
		result = append(result, resultByKey[key])
	}
	sortAggregateTableRows(result, request.Sort)
	return columns, result, nil
}

func (m *DuckDBMetrics) queryTableDatums(ctx context.Context, runtime *modelRuntime, query string, columns []string, args ...any) ([]map[string]any, error) {
	rows, err := runtime.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]any, len(columns))
	scans := make([]any, len(columns))
	for i := range values {
		scans[i] = &values[i]
	}
	result := []map[string]any{}
	for rows.Next() {
		if err := rows.Scan(scans...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, column := range columns {
			row[column] = normalizeDBValue(values[i])
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func tableColumnKeys(columns []dashboard.TableColumn) []string {
	keys := make([]string, len(columns))
	for i, column := range columns {
		keys[i] = column.Key
	}
	return keys
}

func tableHasColumn(columns []dashboard.TableColumn, key string) bool {
	for _, column := range columns {
		if column.Key == key {
			return true
		}
	}
	return false
}

func sortAggregateTableRows(rows []map[string]any, tableSort dashboard.TableSort) {
	if tableSort.Key == "" {
		return
	}
	direction := tableSort.Direction
	sort.SliceStable(rows, func(i, j int) bool {
		cmp := compareTableValues(rows[i][tableSort.Key], rows[j][tableSort.Key])
		if direction == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
}

func compareTableValues(a, b any) int {
	aFloat, aNumeric := numericTableValue(a)
	bFloat, bNumeric := numericTableValue(b)
	if aNumeric && bNumeric {
		switch {
		case aFloat < bFloat:
			return -1
		case aFloat > bFloat:
			return 1
		default:
			return 0
		}
	}
	aText := strings.ToLower(fmt.Sprint(a))
	bText := strings.ToLower(fmt.Sprint(b))
	switch {
	case aText < bText:
		return -1
	case aText > bText:
		return 1
	default:
		return 0
	}
}

func numericTableValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func dimensionLabel(name string, dimension semantic.MetricDimension) string {
	if strings.TrimSpace(dimension.Label) != "" {
		return dimension.Label
	}
	return name
}

func tableMeasureFormat(measure semantic.MetricMeasure) string {
	switch measure.Format {
	case "integer", "decimal", "currency":
		return measure.Format
	default:
		return "decimal"
	}
}

func tableMeasureFormatting(table semantic.TableVisual, measure string) []dashboard.TableFormattingRule {
	if len(table.MeasureFormatting[measure]) == 0 {
		return nil
	}
	return append([]dashboard.TableFormattingRule{}, table.MeasureFormatting[measure]...)
}

func tableColumnOverride(table semantic.TableVisual, key string) dashboard.TableColumn {
	for _, column := range table.Columns {
		if column.Key == key {
			return column
		}
	}
	return dashboard.TableColumn{}
}

func mergeTableColumn(column, override dashboard.TableColumn) dashboard.TableColumn {
	if override.Label != "" {
		column.Label = override.Label
	}
	if override.Align != "" {
		column.Align = override.Align
	}
	if override.Group != "" {
		column.Group = override.Group
	}
	if override.Width > 0 {
		column.Width = override.Width
	}
	if override.Format != "" {
		column.Format = override.Format
	}
	if len(override.Formatting) > 0 {
		column.Formatting = append([]dashboard.TableFormattingRule{}, override.Formatting...)
	}
	return column
}

func sanitizeTableKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}
	key := strings.Trim(builder.String(), "_")
	if key == "" {
		return "value"
	}
	return key
}

func uniqueTableColumnKey(candidate string, existing map[string]string) string {
	used := map[string]struct{}{}
	for _, key := range existing {
		used[key] = struct{}{}
	}
	key := candidate
	for i := 2; ; i++ {
		if _, ok := used[key]; !ok {
			return key
		}
		key = fmt.Sprintf("%s_%d", candidate, i)
	}
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
	metricView := m.workspace.MetricViews[table.MetricView]
	dimensions := []semanticquery.Field{}
	measures := []semanticquery.Field{}
	for _, column := range table.DataColumns {
		if _, ok := metricView.Dimensions[column.Field]; ok {
			dimensions = append(dimensions, fieldRef(column.Field, column.Alias))
			continue
		}
		measures = append(measures, fieldRef(column.Field, column.Alias))
	}
	queryFilters, err := m.semanticFilters(ctx, runtime, report, table.MetricView, filters, "table", request.Table)
	if err != nil {
		return nil, err
	}
	sortKey := tableSortKey(table, request.Sort.Key)
	direction := request.Sort.Direction
	if direction == "" {
		direction = "desc"
	}
	sorts := []semanticquery.Sort{}
	if sortKey != "" {
		sorts = append(sorts, semanticquery.Sort{Field: sortKey, Direction: direction})
	}
	if sortKey != "order_id" && tableHasQueryAlias(table.DataColumns, "order_id") {
		sorts = append(sorts, semanticquery.Sort{Field: "order_id", Direction: "asc"})
	}
	plan, err := semanticquery.NewPlanner(runtime.model, m.workspace.MetricViews).PlanRows(semanticquery.RowRequest{
		MetricView: table.MetricView,
		Dimensions: dimensions,
		Measures:   measures,
		Filters:    queryFilters,
		Sort:       sorts,
		Limit:      count,
		Offset:     start,
	})
	if err != nil {
		return nil, err
	}
	return m.queryTableDatums(ctx, runtime, plan.SQL, plan.Columns, plan.Args...)
}

func tableSortKey(table semantic.TableVisual, key string) string {
	if key == "" {
		key = table.DefaultSort.Key
	}
	if tableHasQueryAlias(table.DataColumns, key) {
		return key
	}
	if tableHasQueryAlias(table.DataColumns, table.DefaultSort.Key) {
		return table.DefaultSort.Key
	}
	if tableHasQueryAlias(table.DataColumns, "order_id") {
		return "order_id"
	}
	if len(table.DataColumns) > 0 {
		return table.DataColumns[0].Alias
	}
	return ""
}

func tableHasQueryAlias(columns []semantic.FieldRef, alias string) bool {
	for _, column := range columns {
		if column.Alias == alias {
			return true
		}
	}
	return false
}
