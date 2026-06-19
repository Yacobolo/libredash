package data

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Yacobolo/libredash/internal/dashboard"
	semanticquery "github.com/Yacobolo/libredash/internal/query"
	"github.com/Yacobolo/libredash/internal/semantic"
)

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
		metricView := m.workspace.MetricViews[visual.MetricView]
		measureName := visual.Query.Measures[0].Field
		measure := metricView.Measures[measureName]
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
		if !visual.Query.Series.IsZero() {
			series = append(series, visual.Query.Series.Field)
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
			Dimensions:      displayFields(queryDimensionFields(visual.Query.Dimensions)),
			Measure:         displayField(measureName),
			Measures:        displayFields(queryMeasureFields(visual.Query.Measures)),
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
	queryFilters, err := m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensions := []semanticquery.Field{fieldRef(visual.Query.Dimensions[0].Field, "label")}
	columns := []string{"label", "value"}
	if !visual.Query.Series.IsZero() {
		dimensions = append(dimensions, fieldRef(visual.Query.Series.Field, "series"))
		columns = []string{"label", "series", "value"}
	}
	data, err := m.querySemanticDatums(ctx, runtime, semanticquery.Request{
		MetricView: visual.MetricView,
		Dimensions: dimensions,
		Measures:   []semanticquery.Field{fieldRef(visual.Query.Measures[0].Field, "value")},
		Filters:    queryFilters,
		Sort:       visualSorts(visual),
		Limit:      visual.Query.Limit,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range data {
		for _, column := range columns {
			if _, ok := row[column]; !ok && column == "series" {
				row[column] = ""
			}
		}
	}
	markSelected(data, "label", selectedValues(filters, visualID))
	return data, nil
}

func (m *DuckDBMetrics) categoryMultiMeasureData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	metricView := m.workspace.MetricViews[visual.MetricView]
	data := []dashboard.Datum{}

	for _, measureName := range visual.Query.Measures {
		rows, err := m.querySemanticDatums(ctx, runtime, semanticquery.Request{
			MetricView: visual.MetricView,
			Dimensions: []semanticquery.Field{fieldRef(visual.Query.Dimensions[0].Field, "label")},
			Measures:   []semanticquery.Field{fieldRef(measureName.Field, "value")},
			Filters:    queryFilters,
			Sort:       visualSorts(visual),
			Limit:      visual.Query.Limit,
		})
		if err != nil {
			return nil, err
		}
		measure := metricView.Measures[measureName.Field]
		for _, row := range rows {
			row["series"] = measureLabel(measureName.Field, measure)
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
	queryFilters, err := m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	rawPlan, err := semanticquery.NewPlanner(runtime.model, m.workspace.MetricViews).PlanRawValues(semanticquery.RawValueRequest{
		MetricView: visual.MetricView,
		Measure:    fieldRef(visual.Query.Measures[0].Field, "value"),
		Filters:    queryFilters,
	})
	if err != nil {
		return nil, err
	}
	binCount := optionInt(visual.Options, "bin_count", 20, 5, 60)

	var minValue, maxValue sql.NullFloat64
	boundsQuery := "WITH raw AS (" + rawPlan.SQL + ")\nSELECT MIN(value), MAX(value) FROM raw"
	if err := runtime.db.QueryRowContext(ctx, boundsQuery, rawPlan.Args...).Scan(&minValue, &maxValue); err != nil {
		return nil, err
	}
	if !minValue.Valid || !maxValue.Valid {
		return []dashboard.Datum{}, nil
	}
	if minValue.Float64 == maxValue.Float64 {
		var count int
		countQuery := "WITH raw AS (" + rawPlan.SQL + ")\nSELECT COUNT(*) FROM raw"
		if err := runtime.db.QueryRowContext(ctx, countQuery, rawPlan.Args...).Scan(&count); err != nil {
			return nil, err
		}
		return []dashboard.Datum{{
			"label":    formatBinLabel(minValue.Float64, maxValue.Float64),
			"binStart": round(minValue.Float64),
			"binEnd":   round(maxValue.Float64),
			"value":    count,
		}}, nil
	}

	bucketExpr := fmt.Sprintf("LEAST(%d, CAST(FLOOR(((value - ?) / NULLIF(? - ?, 0)) * ?) AS INTEGER))", binCount-1)
	query := fmt.Sprintf(`WITH raw AS (%s)
SELECT %s AS bucket, COUNT(*) AS value
FROM raw
GROUP BY bucket
ORDER BY bucket ASC`, rawPlan.SQL, bucketExpr)
	queryArgs := append(append([]any{}, rawPlan.Args...), minValue.Float64, maxValue.Float64, minValue.Float64, binCount)
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
	queryFilters, err := m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensions := make([]semanticquery.Field, 0, len(visual.Query.Dimensions))
	levelAliases := make([]string, 0, len(visual.Query.Dimensions))
	for index, dimensionName := range visual.Query.Dimensions {
		alias := fmt.Sprintf("level_%d", index)
		dimensions = append(dimensions, fieldRef(dimensionName.Field, alias))
		levelAliases = append(levelAliases, alias)
	}
	plan, err := semanticquery.NewPlanner(runtime.model, m.workspace.MetricViews).Plan(semanticquery.Request{
		MetricView: visual.MetricView,
		Dimensions: dimensions,
		Measures:   []semanticquery.Field{fieldRef(visual.Query.Measures[0].Field, "value")},
		Filters:    queryFilters,
		Sort:       visualSorts(visual),
		Limit:      visual.Query.Limit,
	})
	if err != nil {
		return nil, err
	}
	rows, err := runtime.db.QueryContext(ctx, plan.SQL, plan.Args...)
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
	metricView := m.workspace.MetricViews[visual.MetricView]
	measureName := visual.Query.Measures[0].Field
	title := visual.Title
	if title == "" {
		title = metricView.Measures[measureName].Label
	}
	if title == "" {
		title = measureName
	}
	queryFilters, err := m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensions := []semanticquery.Field{}
	if len(visual.Query.Dimensions) == 1 {
		dimensions = append(dimensions, fieldRef(visual.Query.Dimensions[0].Field, "label"))
	}
	sorts := visualSorts(visual)
	if len(dimensions) == 0 {
		sorts = nil
	}
	data, err := m.querySemanticDatums(ctx, runtime, semanticquery.Request{
		MetricView: visual.MetricView,
		Dimensions: dimensions,
		Measures:   []semanticquery.Field{fieldRef(measureName, "value")},
		Filters:    queryFilters,
		Sort:       sorts,
		Limit:      visual.Query.Limit,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range data {
		if _, ok := row["label"]; !ok {
			row["label"] = title
		}
		row["series"] = ""
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
	rightSQLAlias := rightAlias
	if rightAlias == "column" {
		rightSQLAlias = "chart_column"
	}
	queryFilters, err := m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	data, err := m.querySemanticDatums(ctx, runtime, semanticquery.Request{
		MetricView: visual.MetricView,
		Dimensions: []semanticquery.Field{
			fieldRef(visual.Query.Dimensions[0].Field, leftAlias),
			fieldRef(visual.Query.Dimensions[1].Field, rightSQLAlias),
		},
		Measures: []semanticquery.Field{fieldRef(visual.Query.Measures[0].Field, "value")},
		Filters:  queryFilters,
		Sort:     visualSorts(visual),
		Limit:    visual.Query.Limit,
	})
	if err != nil {
		return nil, err
	}
	if rightAlias == "column" {
		for _, row := range data {
			row["column"] = row[rightSQLAlias]
			delete(row, rightSQLAlias)
		}
	}
	if leftAlias == "row" {
		markSelected(data, "row", selectedValues(filters, visualID))
	}
	return data, nil
}

func (m *DuckDBMetrics) geoData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	data, err := m.querySemanticDatums(ctx, runtime, semanticquery.Request{
		MetricView: visual.MetricView,
		Dimensions: []semanticquery.Field{fieldRef(visual.Query.Dimensions[0].Field, "name")},
		Measures:   []semanticquery.Field{fieldRef(visual.Query.Measures[0].Field, "value")},
		Filters:    queryFilters,
		Sort:       visualSorts(visual),
		Limit:      visual.Query.Limit,
	})
	if err != nil {
		return nil, err
	}
	markSelected(data, "name", selectedValues(filters, visualID))
	return data, nil
}

func (m *DuckDBMetrics) ohlcData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	return m.querySemanticDatums(ctx, runtime, semanticquery.Request{
		MetricView: visual.MetricView,
		Dimensions: []semanticquery.Field{fieldRef(visual.Query.Dimensions[0].Field, "label")},
		Measures: []semanticquery.Field{
			fieldRef(visual.Query.Measures[0].Field, "open"),
			fieldRef(visual.Query.Measures[1].Field, "close"),
			fieldRef(visual.Query.Measures[2].Field, "low"),
			fieldRef(visual.Query.Measures[3].Field, "high"),
		},
		Filters: queryFilters,
		Sort:    visualSorts(visual),
		Limit:   visual.Query.Limit,
	})
}

func (m *DuckDBMetrics) distributionData(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, visualID string, visual semantic.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	rawPlan, err := semanticquery.NewPlanner(runtime.model, m.workspace.MetricViews).PlanRawValues(semanticquery.RawValueRequest{
		MetricView: visual.MetricView,
		Dimensions: []semanticquery.Field{fieldRef(visual.Query.Dimensions[0].Field, "label")},
		Measure:    fieldRef(visual.Query.Measures[0].Field, "value"),
		Filters:    queryFilters,
	})
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`WITH raw AS (%s)
SELECT label,
       MIN(value) AS min,
       quantile_cont(value, 0.25) AS q1,
       median(value) AS median,
       quantile_cont(value, 0.75) AS q3,
       MAX(value) AS max
FROM raw
GROUP BY label
ORDER BY %s`, rawPlan.SQL, distributionOrderBy(visual))
	if visual.Query.Limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d", visual.Query.Limit)
	}
	return m.queryDatums(ctx, runtime, query, []string{"label", "min", "q1", "median", "q3", "max"}, rawPlan.Args...)
}

func visualQueryDimensions(visual semantic.Visual) []string {
	dimensions := queryDimensionFields(visual.Query.Dimensions)
	if !visual.Query.Series.IsZero() {
		dimensions = append(dimensions, visual.Query.Series.Field)
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

func (m *DuckDBMetrics) querySemanticDatums(ctx context.Context, runtime *modelRuntime, request semanticquery.Request) ([]dashboard.Datum, error) {
	plan, err := semanticquery.NewPlanner(runtime.model, m.workspace.MetricViews).Plan(request)
	if err != nil {
		return nil, err
	}
	return m.queryDatums(ctx, runtime, plan.SQL, plan.Columns, plan.Args...)
}
