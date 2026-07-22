package runtime

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

type tablePlan struct {
	Definition        visualizationdefinition.Definition
	Kind              string
	Title             string
	Table             string
	Rows              []visualizationdefinition.FieldBinding
	ColumnDims        []visualizationdefinition.FieldBinding
	Measures          []visualizationdefinition.FieldBinding
	DataColumns       []visualizationdefinition.FieldBinding
	DefaultSort       dashboard.TableSort
	Columns           []dashboard.TableColumn
	MeasureFormatting map[string][]dashboard.TableFormattingRule
	Style             dashboard.TableStyle
	Interaction       dashboard.InteractionConfig
}

func newTablePlan(definition visualizationdefinition.Definition) (tablePlan, error) {
	plan := tablePlan{Definition: definition, Title: dashboarddefinition.SpecTitle(definition.Spec), Columns: dashboarddefinition.TableColumns(definition.Spec), Style: dashboard.TableStyle{}.WithDefaults()}
	base, err := visualizationir.SpecificationBase(definition.Spec)
	if err != nil {
		return tablePlan{}, err
	}
	if len(base.Interactions) > 0 {
		plan.Interaction = compiledInteractionConfig(base.Interactions[0])
	}
	switch definition.Query.Kind {
	case visualizationdefinition.QueryDetail:
		query := definition.Query.Detail
		plan.Kind, plan.Table, plan.DataColumns = "data_table", query.TableID, query.Fields
		if len(query.DefaultSort) > 0 {
			plan.DefaultSort = dashboard.TableSort{Key: query.DefaultSort[0].FieldID, Direction: query.DefaultSort[0].Direction}
		}
	case visualizationdefinition.QueryMatrix:
		query := definition.Query.Matrix
		plan.Kind, plan.Table, plan.Rows, plan.ColumnDims, plan.Measures = "matrix_table", query.TableID, query.Rows, query.Columns, query.Measures
		plan.MeasureFormatting = dashboarddefinition.MeasureFormatting(definition.Spec, query.Measures)
	case visualizationdefinition.QueryPivot:
		query := definition.Query.Pivot
		plan.Kind, plan.Table, plan.Rows, plan.ColumnDims, plan.Measures = "pivot_table", query.TableID, query.Rows, query.Columns, query.Measures
		plan.MeasureFormatting = dashboarddefinition.MeasureFormatting(definition.Spec, query.Measures)
	default:
		return tablePlan{}, fmt.Errorf("visualization %q query kind %q is not a grid query", definition.ID, definition.Query.Kind)
	}
	return plan, nil
}

func (s *VisualizationDataService) queryAggregateTable(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, request dashboard.TableRequest, definition visualizationdefinition.Definition, filters dashboard.Filters) (dashboard.Table, error) {
	table, err := newTablePlan(definition)
	if err != nil {
		return dashboard.EmptyTable(request, err), nil
	}
	var (
		columns  []dashboard.TableColumn
		rows     []map[string]any
		queryErr error
	)
	switch table.Kind {
	case "matrix_table":
		columns, rows, queryErr = s.matrixTableRows(ctx, runtime, report, table, filters, request)
	case "pivot_table":
		columns, rows, queryErr = s.pivotTableRows(ctx, runtime, report, table, filters, request)
	default:
		queryErr = fmt.Errorf("unsupported aggregate table kind %q", table.Kind)
	}
	if queryErr != nil {
		return dashboard.EmptyTable(request, queryErr), nil
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
		Kind:          table.Kind,
		Title:         table.Title,
		Style:         style,
		Interaction:   table.Interaction,
		Selection:     []dashboard.InteractionSelectionEntry{},
		Columns:       columns,
		Cardinality:   dashboard.ExactCardinality(totalRows),
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

func (s *VisualizationDataService) matrixTableRows(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, table tablePlan, filters dashboard.Filters, request dashboard.TableRequest) ([]dashboard.TableColumn, []map[string]any, error) {
	if len(table.ColumnDims) == 1 {
		return s.crossTabTableRows(ctx, runtime, report, table, filters, request, false)
	}
	columns := make([]dashboard.TableColumn, 0, len(table.Rows)+len(table.Measures))
	dimensions := make([]reportdef.QueryField, 0, len(table.Rows))
	measures := make([]reportdef.QueryField, 0, len(table.Measures))
	for _, dimensionBinding := range table.Rows {
		dimensionName := dimensionBinding.FieldID
		dimension, _ := runtime.model.ResolveDimension(dimensionName)
		key := dimensionBinding.Alias
		dimensions = append(dimensions, fieldRef(dimensionName, key))
		column := dashboard.TableColumn{Key: key, Label: dimensionLabel(key, dimension), Role: "row_header", Format: "text"}
		columns = append(columns, mergeTableColumn(column, tableColumnOverride(table, dimensionBinding.Alias)))
	}
	for _, measureBinding := range table.Measures {
		measureName := measureBinding.FieldID
		measure := aggregateMemberMetadata(runtime.model, measureName)
		key := measureBinding.Alias
		measures = append(measures, fieldRef(measureName, key))
		column := dashboard.TableColumn{Key: key, Label: measureLabel(key, measure), Align: "right", Role: "measure", Measure: key, Format: tableMeasureFormat(measure), Formatting: tableMeasureFormatting(table, measureName)}
		columns = append(columns, mergeTableColumn(column, tableColumnOverride(table, measureBinding.Alias)))
	}
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "table", request.Table)
	if err != nil {
		return nil, nil, err
	}
	sorts := make([]reportdef.QuerySort, 0, len(dimensions))
	for _, dimension := range dimensions {
		sorts = append(sorts, reportdef.QuerySort{Field: dimension.Alias, Direction: "asc"})
	}
	if request.Sort.Key != "" && tableHasColumn(columns, request.Sort.Key) {
		sorts = []reportdef.QuerySort{{Field: request.Sort.Key, Direction: request.Sort.Direction}}
	}
	rows, err := runtime.data.Query(ctx, reportdef.AggregateQuery{
		Table:      table.Table,
		Dimensions: dimensions,
		Measures:   measures,
		Filters:    queryFilters,
		Sort:       sorts,
		Limit:      dashboard.TableInteractiveRowCap + 1,
	})
	if err != nil {
		return nil, nil, err
	}
	return columns, tableRowsFromAnalytics(rows), nil
}

func (s *VisualizationDataService) pivotTableRows(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, table tablePlan, filters dashboard.Filters, request dashboard.TableRequest) ([]dashboard.TableColumn, []map[string]any, error) {
	return s.crossTabTableRows(ctx, runtime, report, table, filters, request, true)
}

func (s *VisualizationDataService) crossTabTableRows(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, table tablePlan, filters dashboard.Filters, request dashboard.TableRequest, pivotMode bool) ([]dashboard.TableColumn, []map[string]any, error) {
	dimensions := make([]reportdef.QueryField, 0, len(table.Rows)+1)
	baseColumns := make([]dashboard.TableColumn, 0, len(table.Rows))
	for _, dimensionBinding := range table.Rows {
		dimensionName := dimensionBinding.FieldID
		dimension, _ := runtime.model.ResolveDimension(dimensionName)
		key := dimensionBinding.Alias
		dimensions = append(dimensions, fieldRef(dimensionName, key))
		column := dashboard.TableColumn{Key: key, Label: dimensionLabel(key, dimension), Role: "row_header", Format: "text"}
		baseColumns = append(baseColumns, mergeTableColumn(column, tableColumnOverride(table, dimensionBinding.Alias)))
	}
	columnDimensionName := table.ColumnDims[0].FieldID
	dimensions = append(dimensions, fieldRef(columnDimensionName, "pivot_label"))
	measures := make([]reportdef.QueryField, 0, len(table.Measures))
	valueColumns := make([]string, 0, len(table.Measures))
	for _, measureBinding := range table.Measures {
		measureName := measureBinding.FieldID
		key := measureBinding.Alias
		measures = append(measures, fieldRef(measureName, key))
		valueColumns = append(valueColumns, key)
	}
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "table", request.Table)
	if err != nil {
		return nil, nil, err
	}
	sorts := make([]reportdef.QuerySort, 0, len(dimensions))
	for _, dimension := range dimensions {
		sorts = append(sorts, reportdef.QuerySort{Field: dimension.Alias, Direction: "asc"})
	}
	rawRows, err := runtime.data.Query(ctx, reportdef.AggregateQuery{
		Table:      table.Table,
		Dimensions: dimensions,
		Measures:   measures,
		Filters:    queryFilters,
		Sort:       sorts,
		Limit:      dashboard.TableInteractiveRowCap + 1,
	})
	if err != nil {
		return nil, nil, err
	}
	normalizedRows := tableRowsFromAnalytics(rawRows)
	columns := append([]dashboard.TableColumn{}, baseColumns...)
	pivotKeys := map[string]string{}
	usedKeys := map[string]string{}
	columnKeys := map[string]string{}
	for _, column := range baseColumns {
		usedKeys[column.Key] = column.Key
	}
	resultByKey := map[string]map[string]any{}
	order := []string{}
	for _, raw := range normalizedRows {
		rowKeyParts := make([]string, 0, len(table.Rows))
		for _, dimension := range table.Rows {
			rowKeyParts = append(rowKeyParts, fmt.Sprint(raw[dimension.Alias]))
		}
		resultKey := strings.Join(rowKeyParts, "\x00")
		row, exists := resultByKey[resultKey]
		if !exists {
			row = map[string]any{}
			for _, dimension := range table.Rows {
				key := dimension.Alias
				row[key] = raw[key]
			}
			resultByKey[resultKey] = row
			order = append(order, resultKey)
		}
		label := fmt.Sprint(raw["pivot_label"])
		groupLabel := label
		if pivotMode {
			measure := aggregateMemberMetadata(runtime.model, table.Measures[0].FieldID)
			groupLabel = measureLabel(table.Measures[0].Alias, measure)
		}
		pivotKey, exists := pivotKeys[label]
		if !exists {
			pivotKey = sanitizeTableKey(label)
			pivotKeys[label] = pivotKey
		}
		for _, measureBinding := range table.Measures {
			measureName := measureBinding.FieldID
			measure := aggregateMemberMetadata(runtime.model, measureName)
			measureKey := measureBinding.Alias
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
				columns = append(columns, mergeTableColumn(column, tableColumnOverride(table, measureBinding.Alias)))
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

func tableRowsFromAnalytics(rows reportdef.QueryRows) []map[string]any {
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		normalized := map[string]any{}
		for column, value := range row {
			normalized[column] = normalizeDBValue(value)
		}
		result = append(result, normalized)
	}
	return result
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

func dimensionLabel(name string, dimension semanticmodel.MetricDimension) string {
	if strings.TrimSpace(dimension.Label) != "" {
		return dimension.Label
	}
	return name
}

func tableMeasureFormat(measure semanticmodel.MetricMeasure) string {
	switch measure.Format {
	case "integer", "decimal", "currency":
		return measure.Format
	default:
		return "decimal"
	}
}

func tableMeasureFormatting(table tablePlan, measure string) []dashboard.TableFormattingRule {
	if len(table.MeasureFormatting[measure]) == 0 {
		return nil
	}
	return append([]dashboard.TableFormattingRule{}, table.MeasureFormatting[measure]...)
}

func tableColumnOverride(table tablePlan, key string) dashboard.TableColumn {
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

func (s *VisualizationDataService) tableRowRequest(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, table tablePlan, filters dashboard.Filters, request dashboard.TableRequest, start, count int) (reportdef.RowQuery, error) {
	dimensions := []reportdef.QueryField{}
	measures := []reportdef.QueryField{}
	for _, column := range table.DataColumns {
		if _, err := runtime.model.ResolveDimension(column.FieldID); err == nil {
			dimensions = append(dimensions, fieldRef(column.FieldID, column.Alias))
			continue
		}
		measures = append(measures, fieldRef(column.FieldID, column.Alias))
	}
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "table", request.Table)
	if err != nil {
		return reportdef.RowQuery{}, err
	}
	sortKey := tableSortKey(table, request.Sort.Key)
	direction := request.Sort.Direction
	if direction == "" {
		direction = "desc"
	}
	sorts := []reportdef.QuerySort{}
	if sortKey != "" {
		sorts = append(sorts, reportdef.QuerySort{Field: sortKey, Direction: direction})
	}
	if sortKey != "order_id" && tableHasQueryAlias(table.DataColumns, "order_id") {
		sorts = append(sorts, reportdef.QuerySort{Field: "order_id", Direction: "asc"})
	}
	return reportdef.RowQuery{
		Table:      table.Table,
		Dimensions: dimensions,
		Measures:   measures,
		Filters:    queryFilters,
		Sort:       sorts,
		Limit:      count,
		Offset:     start,
	}, nil
}

func tableSortKey(table tablePlan, key string) string {
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

func tableHasQueryAlias(columns []visualizationdefinition.FieldBinding, alias string) bool {
	for _, column := range columns {
		if column.Alias == alias {
			return true
		}
	}
	return false
}
