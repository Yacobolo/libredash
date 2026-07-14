package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type VisualQueryService struct {
	filters *FilterService
}

func (s *VisualQueryService) visuals(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, filters dashboard.Filters, keys []string) (map[string]dashboard.Visual, error) {
	visuals := make(map[string]dashboard.Visual, len(keys))
	batchedData, err := s.batchedSingleValueData(ctx, runtime, report, filters, keys)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		visual, ok := report.Visuals[key]
		if !ok {
			return nil, fmt.Errorf("page references unknown visual %q", key)
		}
		data, batched := batchedData[key]
		if !batched {
			data, err = s.visualData(ctx, runtime, report, key, visual, filters)
			if err != nil {
				return nil, err
			}
		}
		visuals[key] = buildVisualPayload(runtime, key, visual, data)
	}
	return visuals, nil
}

func buildVisualPayload(runtime *modelRuntime, key string, visual reportdef.Visual, data []dashboard.Datum) dashboard.Visual {
	measureName := visual.Query.Measures[0].Field
	measure := aggregateMemberMetadata(runtime.model, measureName)
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
	return dashboard.Visual{Version: 3, ID: key, Kind: visual.KindOrDefault(), Shape: visual.ShapeOrDefault(), Renderer: visual.RendererOrDefault(), Type: visualType, Title: title, Unit: unit, Format: measure.Format, Interaction: visualInteractionConfig(visual.Interaction.PointSelection), Dimensions: visualDimensionNames(visual.Query), Measure: displayField(measureName), Measures: displayFields(queryMeasureFields(visual.Query.Measures)), Series: series, Options: visual.CoreOptions(), RendererOptions: rendererOptions, Selection: []dashboard.InteractionSelectionEntry{}, Data: data}
}

func (s *VisualQueryService) bundledVisuals(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, filters dashboard.Filters, keys []string) (map[string]dashboard.Visual, error) {
	port, ok := runtime.data.(dataquery.BundleExecutor)
	if !ok {
		return nil, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("runtime has no governed bundle port")}
	}
	requests := make([]dataquery.BundleRequest, 0, len(keys))
	definitions := map[string]reportdef.Visual{}
	for _, key := range keys {
		visual, ok := report.Visuals[key]
		if !ok {
			return nil, fmt.Errorf("unknown visual %q", key)
		}
		request, err := s.bundleAggregateRequest(ctx, runtime, report, filters, key, visual)
		if err != nil {
			return nil, err
		}
		requests = append(requests, dataquery.BundleRequest{ID: key, Query: reportAggregateDataQuery(report.SemanticModel, request)})
		definitions[key] = visual
	}
	bundle, err := port.ExecuteDataQueryBundle(ctx, requests)
	if err != nil {
		return nil, err
	}
	visuals := make(map[string]dashboard.Visual, len(keys))
	for _, key := range keys {
		visual := definitions[key]
		data := datumsFromDataQuery(bundle.Results[key].Rows)
		switch visual.ShapeOrDefault() {
		case "single_value":
			for _, row := range data {
				if _, ok := row["label"]; !ok {
					row["label"] = singleValueTitle(runtime, visual)
				}
				row["series"] = ""
			}
		case "category_multi_measure":
			data = categoryMultiMeasureDatums(runtime, visual, data)
		default:
			if !visual.Query.Series.IsZero() {
				for _, row := range data {
					if _, ok := row["series"]; !ok {
						row["series"] = ""
					}
				}
			}
		}
		visuals[key] = buildVisualPayload(runtime, key, visual, data)
	}
	return visuals, nil
}

func (s *VisualQueryService) bundleAggregateRequest(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, filters dashboard.Filters, visualID string, visual reportdef.Visual) (reportdef.AggregateQuery, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return reportdef.AggregateQuery{}, err
	}
	switch visual.ShapeOrDefault() {
	case "single_value":
		dimensions := []reportdef.QueryField{}
		if len(visual.Query.Dimensions) == 1 {
			dimensions = append(dimensions, fieldRef(visual.Query.Dimensions[0].Field, "label"))
		}
		sorts := visualSorts(visual)
		if len(dimensions) == 0 {
			sorts = nil
		}
		return reportdef.AggregateQuery{Table: visual.Query.Table, Dimensions: dimensions, Measures: []reportdef.QueryField{queryFieldRef(visual.Query.Measures[0], "value")}, Filters: queryFilters, Sort: sorts, Limit: visual.Query.Limit}, nil
	case "category_value", "category_series_value":
		dimensions, queryTime := categoryDimension(visual.Query, "label")
		if !visual.Query.Series.IsZero() {
			dimensions = append(dimensions, fieldRef(visual.Query.Series.Field, "series"))
		}
		sorts := visualSorts(visual)
		if len(visual.Query.Sort) == 0 {
			sorts = []reportdef.QuerySort{{Field: "label", Direction: "asc"}}
		}
		return reportdef.AggregateQuery{Table: visual.Query.Table, Dimensions: dimensions, Measures: []reportdef.QueryField{queryFieldRef(visual.Query.Measures[0], "value")}, Time: queryTime, Filters: queryFilters, Sort: sorts, Limit: visual.Query.Limit}, nil
	case "category_multi_measure":
		dimensions, queryTime := categoryDimension(visual.Query, "label")
		measures := make([]reportdef.QueryField, 0, len(visual.Query.Measures))
		for index, measure := range visual.Query.Measures {
			measures = append(measures, queryFieldRef(measure, fmt.Sprintf("value_%d", index)))
		}
		return reportdef.AggregateQuery{Table: visual.Query.Table, Dimensions: dimensions, Measures: measures, Time: queryTime, Filters: queryFilters, Sort: visualSorts(visual), Limit: visual.Query.Limit}, nil
	default:
		return reportdef.AggregateQuery{}, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("visual %q shape %q is not bundleable", visualID, visual.ShapeOrDefault())}
	}
}

func categoryMultiMeasureDatums(runtime *modelRuntime, visual reportdef.Visual, rows []dashboard.Datum) []dashboard.Datum {
	data := make([]dashboard.Datum, 0, len(rows)*len(visual.Query.Measures))
	for _, row := range rows {
		for index, measureRef := range visual.Query.Measures {
			measure := aggregateMemberMetadata(runtime.model, measureRef.Field)
			data = append(data, dashboard.Datum{
				"label":  row["label"],
				"series": measureLabel(measureRef.Field, measure),
				"value":  row[fmt.Sprintf("value_%d", index)],
			})
		}
	}
	return data
}

func datumsFromDataQuery(rows []dataquery.Row) []dashboard.Datum {
	out := make([]dashboard.Datum, 0, len(rows))
	for _, row := range rows {
		datum := dashboard.Datum{}
		for key, value := range row {
			datum[key] = normalizeDatumValue(value)
		}
		out = append(out, datum)
	}
	return out
}

type singleValueBatchItem struct {
	visualID string
	visual   reportdef.Visual
	filters  []reportdef.QueryFilter
}

func (s *VisualQueryService) batchedSingleValueData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, filters dashboard.Filters, keys []string) (map[string][]dashboard.Datum, error) {
	groups := map[string][]singleValueBatchItem{}
	order := []string{}
	for _, visualID := range keys {
		visual, ok := report.Visuals[visualID]
		if !ok || visual.ShapeOrDefault() != "single_value" || len(visual.Query.Dimensions) != 0 || visual.Query.Time.Field != "" || len(visual.Query.Measures) != 1 {
			continue
		}
		queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
		if err != nil {
			return nil, err
		}
		scope, err := json.Marshal(struct {
			Table   string                  `json:"table"`
			Filters []reportdef.QueryFilter `json:"filters"`
			Limit   int                     `json:"limit"`
		}{Table: visual.Query.Table, Filters: queryFilters, Limit: visual.Query.Limit})
		if err != nil {
			return nil, fmt.Errorf("encode visual %q query scope: %w", visualID, err)
		}
		key := string(scope)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], singleValueBatchItem{visualID: visualID, visual: visual, filters: queryFilters})
	}
	result := map[string][]dashboard.Datum{}
	for _, key := range order {
		items := groups[key]
		if len(items) < 2 {
			continue
		}
		measureAliases := map[string]string{}
		measures := make([]reportdef.QueryField, 0, len(items))
		for _, item := range items {
			measure := item.visual.Query.Measures[0]
			if _, exists := measureAliases[measure.Field]; exists {
				continue
			}
			alias := fmt.Sprintf("value_%d", len(measures))
			measureAliases[measure.Field] = alias
			measures = append(measures, queryFieldRef(measure, alias))
		}
		rows, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
			Table:    items[0].visual.Query.Table,
			Measures: measures,
			Filters:  items[0].filters,
			Limit:    items[0].visual.Query.Limit,
		})
		if err != nil {
			return nil, err
		}
		var row dashboard.Datum
		if len(rows) > 0 {
			row = rows[0]
		}
		for _, item := range items {
			value := any(nil)
			if row != nil {
				value = row[measureAliases[item.visual.Query.Measures[0].Field]]
			}
			result[item.visualID] = []dashboard.Datum{{
				"label":  singleValueTitle(runtime, item.visual),
				"series": "",
				"value":  value,
			}}
		}
	}
	return result, nil
}

func singleValueTitle(runtime *modelRuntime, visual reportdef.Visual) string {
	measureRef := visual.Query.Measures[0]
	measureName := measureRef.Field
	title := visual.Title
	if title == "" {
		if measure, err := runtime.model.ResolveMeasure(measureName); err == nil {
			title = measure.Label
		} else if metric, ok := runtime.model.Metrics[measureName]; ok {
			title = metric.Label
		}
	}
	if title == "" {
		title = defaultString(measureName, measureRef.Alias)
	}
	return title
}

func (s *VisualQueryService) visualData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	switch visual.ShapeOrDefault() {
	case "single_value":
		return s.singleValueData(ctx, runtime, report, visualID, visual, filters)
	case "category_multi_measure":
		return s.categoryMultiMeasureData(ctx, runtime, report, visualID, visual, filters)
	case "category_delta":
		return s.categoryDeltaData(ctx, runtime, report, visualID, visual, filters)
	case "binned_measure":
		return s.binnedMeasureData(ctx, runtime, report, visualID, visual, filters)
	case "hierarchy":
		return s.hierarchyData(ctx, runtime, report, visualID, visual, filters)
	case "matrix":
		return s.matrixData(ctx, runtime, report, visualID, visual, filters)
	case "graph":
		return s.graphData(ctx, runtime, report, visualID, visual, filters)
	case "geo":
		return s.geoData(ctx, runtime, report, visualID, visual, filters)
	case "ohlc":
		return s.ohlcData(ctx, runtime, report, visualID, visual, filters)
	case "distribution":
		return s.distributionData(ctx, runtime, report, visualID, visual, filters)
	default:
		return s.categoryData(ctx, runtime, report, visualID, visual, filters)
	}
}

func (s *VisualQueryService) categoryData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensionAlias := "label"
	measureAlias := "value"
	dimensions, queryTime := categoryDimension(visual.Query, dimensionAlias)
	columns := []string{dimensionAlias, measureAlias}
	if !visual.Query.Series.IsZero() {
		dimensions = append(dimensions, fieldRef(visual.Query.Series.Field, "series"))
		columns = []string{dimensionAlias, "series", measureAlias}
	}
	sorts := visualSorts(visual)
	if len(visual.Query.Sort) == 0 {
		sorts = []reportdef.QuerySort{{Field: dimensionAlias, Direction: "asc"}}
	}
	data, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table:      visual.Query.Table,
		Dimensions: dimensions,
		Measures:   []reportdef.QueryField{queryFieldRef(visual.Query.Measures[0], measureAlias)},
		Time:       queryTime,
		Filters:    queryFilters,
		Sort:       sorts,
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
	return data, nil
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func fieldAlias(field string) string {
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
}

func (s *VisualQueryService) categoryMultiMeasureData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensions, queryTime := categoryDimension(visual.Query, "label")
	measures := make([]reportdef.QueryField, 0, len(visual.Query.Measures))
	for index, measure := range visual.Query.Measures {
		measures = append(measures, queryFieldRef(measure, fmt.Sprintf("value_%d", index)))
	}
	rows, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table:      visual.Query.Table,
		Dimensions: dimensions,
		Measures:   measures,
		Time:       queryTime,
		Filters:    queryFilters,
		Sort:       visualSorts(visual),
		Limit:      visual.Query.Limit,
	})
	if err != nil {
		return nil, err
	}
	return categoryMultiMeasureDatums(runtime, visual, rows), nil
}

func categoryDimension(query reportdef.VisualQuery, alias string) ([]reportdef.QueryField, reportdef.QueryTime) {
	if query.Time.Field != "" {
		return nil, reportdef.QueryTime{Field: query.Time.Field, Grain: query.Time.Grain, Alias: alias}
	}
	return []reportdef.QueryField{fieldRef(query.Dimensions[0].Field, alias)}, reportdef.QueryTime{}
}

func visualDimensionNames(query reportdef.VisualQuery) []string {
	names := displayFields(queryDimensionFields(query.Dimensions))
	if query.Time.Field != "" {
		names = append(names, displayField(query.Time.Field))
	}
	return names
}

func (s *VisualQueryService) categoryDeltaData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	rows, err := s.categoryData(ctx, runtime, report, visualID, visual, filters)
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

func (s *VisualQueryService) binnedMeasureData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	bins, err := runtime.data.Histogram(ctx, reportdef.RawValueQuery{
		Table:   visual.Query.Table,
		Measure: queryFieldRef(visual.Query.Measures[0], "value"),
		Filters: queryFilters,
	}, optionInt(visual.Options, "bin_count", 20, 5, 60))
	if err != nil {
		return nil, err
	}
	data := make([]dashboard.Datum, 0, len(bins))
	for _, bin := range bins {
		data = append(data, dashboard.Datum{
			"label":    formatBinLabel(bin.Start, bin.End),
			"binStart": round(bin.Start),
			"binEnd":   round(bin.End),
			"value":    bin.Count,
		})
	}
	return data, nil
}

func (s *VisualQueryService) hierarchyData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensions := make([]reportdef.QueryField, 0, len(visual.Query.Dimensions))
	levelAliases := make([]string, 0, len(visual.Query.Dimensions))
	for index, dimensionName := range visual.Query.Dimensions {
		alias := fmt.Sprintf("level_%d", index)
		dimensions = append(dimensions, fieldRef(dimensionName.Field, alias))
		levelAliases = append(levelAliases, alias)
	}
	rows, err := runtime.data.Query(ctx, reportdef.AggregateQuery{
		Table:      visual.Query.Table,
		Dimensions: dimensions,
		Measures:   []reportdef.QueryField{queryFieldRef(visual.Query.Measures[0], "value")},
		Filters:    queryFilters,
		Sort:       visualSorts(visual),
		Limit:      visual.Query.Limit,
	})
	if err != nil {
		return nil, err
	}
	data := make([]dashboard.Datum, 0, len(rows))
	for _, row := range rows {
		path := make([]string, 0, len(levelAliases))
		for _, alias := range levelAliases {
			item := normalizeDatumValue(row[alias])
			if item == nil || fmt.Sprint(item) == "" {
				continue
			}
			path = append(path, fmt.Sprint(item))
		}
		data = append(data, dashboard.Datum{
			"path":  path,
			"value": normalizeDatumValue(row["value"]),
		})
	}
	return data, nil
}

func (s *VisualQueryService) singleValueData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	measureRef := visual.Query.Measures[0]
	title := singleValueTitle(runtime, visual)
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensions := []reportdef.QueryField{}
	if len(visual.Query.Dimensions) == 1 {
		dimensions = append(dimensions, fieldRef(visual.Query.Dimensions[0].Field, "label"))
	}
	sorts := visualSorts(visual)
	if len(dimensions) == 0 {
		sorts = nil
	}
	data, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table:      visual.Query.Table,
		Dimensions: dimensions,
		Measures:   []reportdef.QueryField{queryFieldRef(measureRef, "value")},
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
	return data, nil
}

func (s *VisualQueryService) matrixData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	return s.dimensionPairData(ctx, runtime, report, visualID, visual, filters, "row", "column")
}

func (s *VisualQueryService) graphData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	return s.dimensionPairData(ctx, runtime, report, visualID, visual, filters, "source", "target")
}

func (s *VisualQueryService) dimensionPairData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters, leftAlias, rightAlias string) ([]dashboard.Datum, error) {
	rightSQLAlias := rightAlias
	if rightAlias == "column" {
		rightSQLAlias = "chart_column"
	}
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	data, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table: visual.Query.Table,
		Dimensions: []reportdef.QueryField{
			fieldRef(visual.Query.Dimensions[0].Field, leftAlias),
			fieldRef(visual.Query.Dimensions[1].Field, rightSQLAlias),
		},
		Measures: []reportdef.QueryField{queryFieldRef(visual.Query.Measures[0], "value")},
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
	return data, nil
}

func (s *VisualQueryService) geoData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	data, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table:      visual.Query.Table,
		Dimensions: []reportdef.QueryField{fieldRef(visual.Query.Dimensions[0].Field, "name")},
		Measures:   []reportdef.QueryField{queryFieldRef(visual.Query.Measures[0], "value")},
		Filters:    queryFilters,
		Sort:       visualSorts(visual),
		Limit:      visual.Query.Limit,
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *VisualQueryService) ohlcData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	return s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table:      visual.Query.Table,
		Dimensions: []reportdef.QueryField{fieldRef(visual.Query.Dimensions[0].Field, "label")},
		Measures: []reportdef.QueryField{
			queryFieldRef(visual.Query.Measures[0], "open"),
			queryFieldRef(visual.Query.Measures[1], "close"),
			queryFieldRef(visual.Query.Measures[2], "low"),
			queryFieldRef(visual.Query.Measures[3], "high"),
		},
		Filters: queryFilters,
		Sort:    visualSorts(visual),
		Limit:   visual.Query.Limit,
	})
}

func (s *VisualQueryService) distributionData(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, visualID string, visual reportdef.Visual, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	return s.queryDistributionDatums(ctx, runtime, reportdef.RawValueQuery{
		Table:      visual.Query.Table,
		Dimensions: []reportdef.QueryField{fieldRef(visual.Query.Dimensions[0].Field, "label")},
		Measure:    queryFieldRef(visual.Query.Measures[0], "value"),
		Filters:    queryFilters,
	}, distributionSorts(visual), visual.Query.Limit)
}

func visualQueryDimensions(visual reportdef.Visual) []string {
	dimensions := queryDimensionFields(visual.Query.Dimensions)
	if !visual.Query.Series.IsZero() {
		dimensions = append(dimensions, visual.Query.Series.Field)
	}
	return dimensions
}

func (s *VisualQueryService) querySemanticDatums(ctx context.Context, runtime *modelRuntime, request reportdef.AggregateQuery) ([]dashboard.Datum, error) {
	rows, err := runtime.data.Query(ctx, request)
	if err != nil {
		return nil, err
	}
	return datumsFromAnalytics(rows), nil
}

func (s *VisualQueryService) queryDistributionDatums(ctx context.Context, runtime *modelRuntime, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) ([]dashboard.Datum, error) {
	rows, err := runtime.data.Distribution(ctx, request, sort, limit)
	if err != nil {
		return nil, err
	}
	return datumsFromAnalytics(rows), nil
}

func datumsFromAnalytics(rows reportdef.QueryRows) []dashboard.Datum {
	data := make([]dashboard.Datum, 0, len(rows))
	for _, row := range rows {
		datum := dashboard.Datum{}
		for column, value := range row {
			datum[column] = normalizeDatumValue(value)
		}
		data = append(data, datum)
	}
	return data
}
