package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/dataquery"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	visualizationruntime "github.com/Yacobolo/leapview/internal/visualization/runtime"
)

type VisualizationDataService struct {
	mu       *sync.RWMutex
	reports  *ReportService
	runtimes map[string]*modelRuntime
	filters  *FilterService
}

func (s *VisualizationDataService) visuals(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, filters dashboard.Filters, keys []string) (map[string]visualizationir.VisualizationEnvelope, error) {
	visuals := make(map[string]visualizationir.VisualizationEnvelope, len(keys))
	batchedData, err := s.batchedSingleValueData(ctx, runtime, report, filters, keys)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		definition, ok := report.Visualizations[key]
		if !ok {
			return nil, fmt.Errorf("page references unknown visual %q", key)
		}
		visual, err := newVisualPlan(definition)
		if err != nil {
			return nil, err
		}
		data, batched := batchedData[key]
		if !batched {
			data, err = s.visualData(ctx, runtime, report, key, visual, filters)
			if err != nil {
				return nil, err
			}
		}
		frame, err := frameFromDatums(definition, data)
		if err != nil {
			return nil, err
		}
		envelope, err := visualizationruntime.EnvelopeFromFrame(definition, frame, selectedEntries(filters, "visual", key), 0, 0)
		if err != nil {
			return nil, err
		}
		envelope.SpatialSelection = selectedSpatialState(filters, key)
		visuals[key] = envelope
	}
	return visuals, nil
}

func (s *VisualizationDataService) spatialEnvelope(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, filters dashboard.Filters, request dashboard.SpatialWindowRequest) (visualizationir.VisualizationEnvelope, error) {
	definition, ok := report.Visualizations[request.VisualID]
	if !ok {
		return visualizationir.VisualizationEnvelope{}, fmt.Errorf("unknown spatial visual %q", request.VisualID)
	}
	visual, err := newVisualPlan(definition)
	if err != nil {
		return visualizationir.VisualizationEnvelope{}, err
	}
	if _, ok := definition.Spec.Value.(*visualizationir.GeographicVisualizationSpec); !ok {
		return visualizationir.VisualizationEnvelope{}, fmt.Errorf("visual %q is not geographic", request.VisualID)
	}
	spatial := definition.Query.Spatial
	if definition.Query.Kind != visualizationdefinition.QuerySpatial || spatial == nil || spatial.Viewport == nil {
		return visualizationir.VisualizationEnvelope{}, fmt.Errorf("visual %q has no compiled spatial viewport", request.VisualID)
	}
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", request.VisualID)
	if err != nil {
		return visualizationir.VisualizationEnvelope{}, err
	}
	precision := dataquery.SpatialPrecisionAggregated
	if request.Zoom >= spatial.Viewport.RawMinimumZoom {
		precision = dataquery.SpatialPrecisionRaw
	}
	execute := func(next dataquery.SpatialPrecision) (dataquery.Result, error) {
		query := dataquery.Query{
			Surface: dataquery.SurfaceDashboard, Operation: dataquery.OperationDashboardSpatial, ModelID: definition.Query.ModelID, Kind: dataquery.KindSemanticSpatial, Target: spatial.TableID,
			Fields: fieldBindingsToDataFields(spatial.Dimensions), Measures: fieldBindingsToDataFields(spatial.Measures), Filters: reportFiltersToDataFilters(queryFilters),
			Sort: reportSortToDataSort(aliasedVisualSorts(visual)),
			Spatial: &dataquery.SpatialWindow{
				Latitude: dataquery.Field{Field: spatial.Viewport.Latitude.FieldID, Alias: spatial.Viewport.Latitude.Alias}, Longitude: dataquery.Field{Field: spatial.Viewport.Longitude.FieldID, Alias: spatial.Viewport.Longitude.Alias},
				West: request.Bounds.West, South: request.Bounds.South, East: request.Bounds.East, North: request.Bounds.North, Width: int(request.Width), Height: int(request.Height), FeatureCap: int(spatial.Viewport.FeatureCap), Precision: next,
			},
		}
		if spatial.Time != nil {
			query.Time = dataquery.Time{Field: spatial.Time.FieldID, Alias: spatial.Time.Alias, Grain: spatial.Time.Grain}
		}
		return runtime.data.ExecuteDataQuery(ctx, query)
	}
	result, err := execute(precision)
	if err != nil {
		return visualizationir.VisualizationEnvelope{}, err
	}
	if precision == dataquery.SpatialPrecisionRaw && result.TotalRowsKnown && result.TotalRows > int(spatial.Viewport.FeatureCap) {
		precision = dataquery.SpatialPrecisionAggregated
		result, err = execute(precision)
		if err != nil {
			return visualizationir.VisualizationEnvelope{}, err
		}
	}
	base, err := visualizationir.SpecificationBase(definition.Spec)
	if err != nil || len(base.Datasets) != 1 {
		return visualizationir.VisualizationEnvelope{}, fmt.Errorf("spatial visual %q has invalid compiled dataset", request.VisualID)
	}
	columns := make([]string, len(base.Datasets[0].Fields))
	for index, field := range base.Datasets[0].Fields {
		columns[index] = field.ID
	}
	rows := make([][]any, len(result.Rows))
	for index, row := range result.Rows {
		rows[index] = make([]any, len(columns))
		for columnIndex, column := range columns {
			rows[index][columnIndex] = normalizeDatumValue(row[column])
		}
	}
	cardinality := int64(result.TotalRows)
	irPrecision := visualizationir.VisualizationSpatialPrecision(precision)
	envelope, err := visualizationruntime.SpatialEnvelopeFromFrame(definition, visualizationruntime.Frame{Columns: columns, Rows: rows}, selectedEntries(filters, "visual", request.VisualID), request, irPrecision, cardinality, 0, 0)
	if err != nil {
		return visualizationir.VisualizationEnvelope{}, err
	}
	envelope.SpatialSelection = selectedSpatialState(filters, request.VisualID)
	return envelope, nil
}

func fieldBindingsToDataFields(bindings []visualizationdefinition.FieldBinding) []dataquery.Field {
	fields := make([]dataquery.Field, len(bindings))
	for index, binding := range bindings {
		fields[index] = dataquery.Field{Field: binding.FieldID, Alias: binding.Alias}
	}
	return fields
}

func frameFromDatums(definition visualizationdefinition.Definition, data []dashboard.Datum) (visualizationruntime.Frame, error) {
	base, err := visualizationir.SpecificationBase(definition.Spec)
	if err != nil || len(base.Datasets) != 1 {
		return visualizationruntime.Frame{}, fmt.Errorf("visualization %q has invalid compiled dataset", definition.ID)
	}
	columns := make([]string, len(base.Datasets[0].Fields))
	for index, field := range base.Datasets[0].Fields {
		columns[index] = field.ID
	}
	rows := make([][]any, len(data))
	for index, datum := range data {
		rows[index] = make([]any, len(columns))
		for columnIndex, column := range columns {
			rows[index][columnIndex] = normalizeDatumValue(datum[column])
		}
	}
	return visualizationruntime.Frame{Columns: columns, Rows: rows}, nil
}

func (s *VisualizationDataService) bundledVisuals(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, filters dashboard.Filters, keys []string) (map[string]visualizationir.VisualizationEnvelope, error) {
	port, ok := runtime.data.(dataquery.BundleExecutor)
	if !ok {
		return nil, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("runtime has no governed bundle port")}
	}
	requests := make([]dataquery.BundleRequest, 0, len(keys))
	definitions := map[string]visualPlan{}
	for _, key := range keys {
		definition, ok := report.Visualizations[key]
		if !ok {
			return nil, fmt.Errorf("unknown visual %q", key)
		}
		visual, err := newVisualPlan(definition)
		if err != nil {
			return nil, err
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
	visuals := make(map[string]visualizationir.VisualizationEnvelope, len(keys))
	for _, key := range keys {
		visual := definitions[key]
		data := datumsFromDataQuery(bundle.Results[key].Rows)
		switch visual.ResultShape() {
		case visualizationdefinition.ResultScalar:
			for _, row := range data {
				if _, ok := row["label"]; !ok {
					row["label"] = singleValueTitle(runtime, visual)
				}
				row["series"] = ""
			}
		case visualizationdefinition.ResultCategoryMultiMeasure:
			data = categoryMultiMeasureDatums(runtime, visual, data)
		default:
			if visual.Series != nil {
				for _, row := range data {
					if _, ok := row["series"]; !ok {
						row["series"] = ""
					}
				}
			}
		}
		definition := report.Visualizations[key]
		frame, frameErr := frameFromDatums(definition, data)
		if frameErr != nil {
			return nil, frameErr
		}
		envelope, envelopeErr := visualizationruntime.EnvelopeFromFrame(definition, frame, selectedEntries(filters, "visual", key), 0, 0)
		if envelopeErr != nil {
			return nil, envelopeErr
		}
		envelope.SpatialSelection = selectedSpatialState(filters, key)
		visuals[key] = envelope
	}
	return visuals, nil
}

func (s *VisualizationDataService) bundleAggregateRequest(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, filters dashboard.Filters, visualID string, visual visualPlan) (reportdef.AggregateQuery, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return reportdef.AggregateQuery{}, err
	}
	switch visual.ResultShape() {
	case visualizationdefinition.ResultScalar:
		dimensions := []reportdef.QueryField{}
		if len(visual.Dimensions) == 1 {
			dimensions = append(dimensions, fieldRef(visual.Dimensions[0].FieldID, "label"))
		}
		sorts := visualSorts(visual)
		if len(dimensions) == 0 {
			sorts = nil
		}
		return reportdef.AggregateQuery{Table: visual.Table, Dimensions: dimensions, Measures: []reportdef.QueryField{queryFieldRef(visual.Measures[0], "value")}, Filters: queryFilters, Sort: sorts, Limit: visual.Limit}, nil
	case visualizationdefinition.ResultCategoryValue, visualizationdefinition.ResultCategorySeriesValue:
		dimensions, queryTime := categoryDimension(visual, "label")
		if visual.Series != nil {
			dimensions = append(dimensions, fieldRef(visual.Series.FieldID, "series"))
		}
		sorts := visualSorts(visual)
		if len(visual.Sort) == 0 {
			sorts = []reportdef.QuerySort{{Field: "label", Direction: "asc"}}
		}
		return reportdef.AggregateQuery{Table: visual.Table, Dimensions: dimensions, Measures: []reportdef.QueryField{queryFieldRef(visual.Measures[0], "value")}, Time: queryTime, Filters: queryFilters, Sort: sorts, Limit: visual.Limit}, nil
	case visualizationdefinition.ResultCategoryMultiMeasure:
		dimensions, queryTime := categoryDimension(visual, "label")
		measures := make([]reportdef.QueryField, 0, len(visual.Measures))
		for index, measure := range visual.Measures {
			measures = append(measures, queryFieldRef(measure, fmt.Sprintf("value_%d", index)))
		}
		return reportdef.AggregateQuery{Table: visual.Table, Dimensions: dimensions, Measures: measures, Time: queryTime, Filters: queryFilters, Sort: visualSorts(visual), Limit: visual.Limit}, nil
	default:
		return reportdef.AggregateQuery{}, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("visual %q result shape %q is not bundleable", visualID, visual.ResultShape())}
	}
}

func categoryMultiMeasureDatums(runtime *modelRuntime, visual visualPlan, rows []dashboard.Datum) []dashboard.Datum {
	data := make([]dashboard.Datum, 0, len(rows)*len(visual.Measures))
	for _, row := range rows {
		for index, measureRef := range visual.Measures {
			measure := aggregateMemberMetadata(runtime.model, measureRef.FieldID)
			data = append(data, dashboard.Datum{
				"label":  row["label"],
				"series": measureLabel(measureRef.FieldID, measure),
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
	visual   visualPlan
	filters  []reportdef.QueryFilter
}

func (s *VisualizationDataService) batchedSingleValueData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, filters dashboard.Filters, keys []string) (map[string][]dashboard.Datum, error) {
	groups := map[string][]singleValueBatchItem{}
	order := []string{}
	for _, visualID := range keys {
		definition, ok := report.Visualizations[visualID]
		if !ok {
			continue
		}
		visual, err := newVisualPlan(definition)
		if err != nil {
			return nil, err
		}
		if visual.ResultShape() != visualizationdefinition.ResultScalar || len(visual.Dimensions) != 0 || visual.Time != nil || len(visual.Measures) != 1 {
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
		}{Table: visual.Table, Filters: queryFilters, Limit: visual.Limit})
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
			measure := item.visual.Measures[0]
			if _, exists := measureAliases[measure.FieldID]; exists {
				continue
			}
			alias := fmt.Sprintf("value_%d", len(measures))
			measureAliases[measure.FieldID] = alias
			measures = append(measures, queryFieldRef(measure, alias))
		}
		rows, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
			Table:    items[0].visual.Table,
			Measures: measures,
			Filters:  items[0].filters,
			Limit:    items[0].visual.Limit,
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
				value = row[measureAliases[item.visual.Measures[0].FieldID]]
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

func singleValueTitle(runtime *modelRuntime, visual visualPlan) string {
	measureRef := visual.Measures[0]
	measureName := measureRef.FieldID
	title := visual.Title()
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

func (s *VisualizationDataService) visualData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	switch visual.ResultShape() {
	case visualizationdefinition.ResultScalar:
		return s.singleValueData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultCategoryMultiMeasure:
		return s.categoryMultiMeasureData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultCategoryDelta:
		return s.categoryDeltaData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultHistogramBins:
		return s.binnedMeasureData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultHierarchyNodes:
		return s.hierarchyData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultMatrixCells:
		return s.matrixData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultGraphEdges:
		return s.graphData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultGeographicFeatures:
		return s.geoData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultCustomRows:
		return s.customData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultOHLC:
		return s.ohlcData(ctx, runtime, report, visualID, visual, filters)
	case visualizationdefinition.ResultDistribution:
		return s.distributionData(ctx, runtime, report, visualID, visual, filters)
	default:
		return s.categoryData(ctx, runtime, report, visualID, visual, filters)
	}
}

func (s *VisualizationDataService) categoryData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensionAlias := "label"
	measureAlias := "value"
	dimensions, queryTime := categoryDimension(visual, dimensionAlias)
	columns := []string{dimensionAlias, measureAlias}
	if visual.Series != nil {
		dimensions = append(dimensions, fieldRef(visual.Series.FieldID, "series"))
		columns = []string{dimensionAlias, "series", measureAlias}
	}
	sorts := visualSorts(visual)
	if len(visual.Sort) == 0 {
		sorts = []reportdef.QuerySort{{Field: dimensionAlias, Direction: "asc"}}
	}
	data, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table:      visual.Table,
		Dimensions: dimensions,
		Measures:   []reportdef.QueryField{queryFieldRef(visual.Measures[0], measureAlias)},
		Time:       queryTime,
		Filters:    queryFilters,
		Sort:       sorts,
		Limit:      visual.Limit,
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

func (s *VisualizationDataService) categoryMultiMeasureData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensions, queryTime := categoryDimension(visual, "label")
	measures := make([]reportdef.QueryField, 0, len(visual.Measures))
	for index, measure := range visual.Measures {
		measures = append(measures, queryFieldRef(measure, fmt.Sprintf("value_%d", index)))
	}
	rows, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table:      visual.Table,
		Dimensions: dimensions,
		Measures:   measures,
		Time:       queryTime,
		Filters:    queryFilters,
		Sort:       visualSorts(visual),
		Limit:      visual.Limit,
	})
	if err != nil {
		return nil, err
	}
	return categoryMultiMeasureDatums(runtime, visual, rows), nil
}

func categoryDimension(visual visualPlan, alias string) ([]reportdef.QueryField, reportdef.QueryTime) {
	if visual.Time != nil {
		return nil, reportdef.QueryTime{Field: visual.Time.FieldID, Grain: visual.Time.Grain, Alias: alias}
	}
	return []reportdef.QueryField{fieldRef(visual.Dimensions[0].FieldID, alias)}, reportdef.QueryTime{}
}

func (s *VisualizationDataService) categoryDeltaData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
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

func (s *VisualizationDataService) binnedMeasureData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	bins, err := runtime.data.Histogram(ctx, reportdef.RawValueQuery{
		Table:   visual.Table,
		Measure: queryFieldRef(visual.Measures[0], "value"),
		Filters: queryFilters,
	}, visual.HistogramBins())
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

func (s *VisualizationDataService) hierarchyData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensions := make([]reportdef.QueryField, 0, len(visual.Dimensions))
	levelAliases := make([]string, 0, len(visual.Dimensions))
	for _, dimensionName := range visual.Dimensions {
		alias := dimensionName.Alias
		dimensions = append(dimensions, fieldRef(dimensionName.FieldID, alias))
		levelAliases = append(levelAliases, alias)
	}
	queryTime := reportdef.QueryTime{}
	if visual.Time != nil {
		alias := visual.Time.Alias
		queryTime = reportdef.QueryTime{Field: visual.Time.FieldID, Grain: visual.Time.Grain, Alias: alias}
		levelAliases = append(levelAliases, alias)
	}
	rows, err := runtime.data.Query(ctx, reportdef.AggregateQuery{
		Table:      visual.Table,
		Dimensions: dimensions,
		Time:       queryTime,
		Measures:   []reportdef.QueryField{queryFieldRef(visual.Measures[0], "value")},
		Filters:    queryFilters,
		Sort:       visualSorts(visual),
		Limit:      visual.Limit,
	})
	if err != nil {
		return nil, err
	}
	return flattenHierarchyRows(rows, levelAliases)
}

type hierarchyFrameNode struct {
	name   string
	parent any
	value  float64
	levels []any
}

// flattenHierarchyRows materializes the hierarchy declared by the compiled
// node/parent/value frame. Parent values are stable, escaped path identities,
// which permits the same display label under different parents without making
// renderer-specific row identities part of the public contract.
func flattenHierarchyRows(rows reportdef.QueryRows, levelAliases []string) ([]dashboard.Datum, error) {
	if len(levelAliases) == 0 {
		return nil, fmt.Errorf("hierarchy requires at least one level")
	}
	nodes := make(map[string]*hierarchyFrameNode)
	for rowIndex, row := range rows {
		value, ok := hierarchyNumericValue(normalizeDatumValue(row["value"]))
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("hierarchy row %d has a nonnumeric value", rowIndex)
		}
		segments := make([]string, 0, len(levelAliases))
		levelValues := make([]any, 0, len(levelAliases))
		for _, alias := range levelAliases {
			raw := normalizeDatumValue(row[alias])
			if raw == nil || strings.TrimSpace(fmt.Sprint(raw)) == "" {
				return nil, fmt.Errorf("hierarchy row %d has an empty level %q", rowIndex, alias)
			}
			segments = append(segments, fmt.Sprint(raw))
			levelValues = append(levelValues, raw)
		}
		for level, name := range segments {
			id := hierarchyPathID(segments[:level+1])
			var parent any
			if level > 0 {
				parent = hierarchyPathID(segments[:level])
			}
			node, exists := nodes[id]
			if !exists {
				node = &hierarchyFrameNode{name: name, parent: parent, levels: append([]any(nil), levelValues[:level+1]...)}
				nodes[id] = node
			}
			node.value += value
		}
	}
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]dashboard.Datum, 0, len(ids))
	for _, id := range ids {
		node := nodes[id]
		row := dashboard.Datum{"node": node.name, "parent": node.parent, "value": round(node.value)}
		for index, alias := range levelAliases {
			row[alias] = nil
			if index < len(node.levels) {
				row[alias] = node.levels[index]
			}
		}
		result = append(result, row)
	}
	return result, nil
}

func hierarchyPathID(segments []string) string {
	id := ""
	for _, segment := range segments {
		id = visualizationir.HierarchyNodeIdentity(id, segment)
	}
	return id
}

func hierarchyNumericValue(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	default:
		return 0, false
	}
}

func (s *VisualizationDataService) singleValueData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	measureRef := visual.Measures[0]
	title := singleValueTitle(runtime, visual)
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	dimensions := []reportdef.QueryField{}
	if len(visual.Dimensions) == 1 {
		dimensions = append(dimensions, fieldRef(visual.Dimensions[0].FieldID, "label"))
	}
	sorts := visualSorts(visual)
	if len(dimensions) == 0 {
		sorts = nil
	}
	data, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table:      visual.Table,
		Dimensions: dimensions,
		Measures:   []reportdef.QueryField{queryFieldRef(measureRef, "value")},
		Filters:    queryFilters,
		Sort:       sorts,
		Limit:      visual.Limit,
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

func (s *VisualizationDataService) matrixData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	return s.dimensionPairData(ctx, runtime, report, visualID, visual, filters, "row", "column")
}

func (s *VisualizationDataService) graphData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	return s.dimensionPairData(ctx, runtime, report, visualID, visual, filters, "source", "target")
}

func (s *VisualizationDataService) dimensionPairData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters, leftAlias, rightAlias string) ([]dashboard.Datum, error) {
	rightSQLAlias := rightAlias
	if rightAlias == "column" {
		rightSQLAlias = "chart_column"
	}
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	data, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table: visual.Table,
		Dimensions: []reportdef.QueryField{
			fieldRef(visual.Dimensions[0].FieldID, leftAlias),
			fieldRef(visual.Dimensions[1].FieldID, rightSQLAlias),
		},
		Measures: []reportdef.QueryField{queryFieldRef(visual.Measures[0], "value")},
		Filters:  queryFilters,
		Sort:     visualSorts(visual),
		Limit:    visual.Limit,
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

func (s *VisualizationDataService) geoData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	data, err := s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table: visual.Table, Dimensions: aliasedQueryFields(visual.Dimensions), Measures: aliasedQueryFields(visual.Measures),
		Filters: queryFilters, Sort: aliasedVisualSorts(visual), Limit: visual.Limit,
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *VisualizationDataService) customData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	return s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table: visual.Table, Dimensions: aliasedQueryFields(visual.Dimensions), Measures: aliasedQueryFields(visual.Measures),
		Filters: queryFilters, Sort: aliasedVisualSorts(visual), Limit: visual.Limit,
	})
}

func aliasedQueryFields(bindings []visualizationdefinition.FieldBinding) []reportdef.QueryField {
	fields := make([]reportdef.QueryField, len(bindings))
	for index, binding := range bindings {
		fields[index] = queryFieldRef(binding, binding.Alias)
	}
	return fields
}

func aliasedVisualSorts(visual visualPlan) []reportdef.QuerySort {
	if len(visual.Sort) == 0 {
		if len(visual.Dimensions) > 0 {
			return []reportdef.QuerySort{{Field: visual.Dimensions[0].Alias, Direction: "asc"}}
		}
		return nil
	}
	bindings := append(append([]visualizationdefinition.FieldBinding{}, visual.Dimensions...), visual.Measures...)
	sorts := make([]reportdef.QuerySort, len(visual.Sort))
	for index, sort := range visual.Sort {
		field := sort.FieldID
		for _, binding := range bindings {
			if field == binding.FieldID || field == binding.Alias || field == displayField(binding.FieldID) {
				field = binding.Alias
				break
			}
		}
		sorts[index] = reportdef.QuerySort{Field: field, Direction: sort.Direction}
	}
	return sorts
}

func (s *VisualizationDataService) ohlcData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	return s.querySemanticDatums(ctx, runtime, reportdef.AggregateQuery{
		Table:      visual.Table,
		Dimensions: []reportdef.QueryField{fieldRef(visual.Dimensions[0].FieldID, "label")},
		Measures: []reportdef.QueryField{
			queryFieldRef(visual.Measures[0], "open"),
			queryFieldRef(visual.Measures[1], "close"),
			queryFieldRef(visual.Measures[2], "low"),
			queryFieldRef(visual.Measures[3], "high"),
		},
		Filters: queryFilters,
		Sort:    visualSorts(visual),
		Limit:   visual.Limit,
	})
}

func (s *VisualizationDataService) distributionData(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, visualID string, visual visualPlan, filters dashboard.Filters) ([]dashboard.Datum, error) {
	queryFilters, err := s.filters.semanticFilters(ctx, runtime, report, filters, "visual", visualID)
	if err != nil {
		return nil, err
	}
	return s.queryDistributionDatums(ctx, runtime, reportdef.RawValueQuery{
		Table:      visual.Table,
		Dimensions: []reportdef.QueryField{fieldRef(visual.Dimensions[0].FieldID, "label")},
		Measure:    queryFieldRef(visual.Measures[0], "value"),
		Filters:    queryFilters,
	}, distributionSorts(visual), visual.Limit)
}

func visualQueryDimensions(visual visualPlan) []string {
	dimensions := queryDimensionFields(visual.Dimensions)
	if visual.Series != nil {
		dimensions = append(dimensions, visual.Series.FieldID)
	}
	return dimensions
}

func (s *VisualizationDataService) querySemanticDatums(ctx context.Context, runtime *modelRuntime, request reportdef.AggregateQuery) ([]dashboard.Datum, error) {
	rows, err := runtime.data.Query(ctx, request)
	if err != nil {
		return nil, err
	}
	return datumsFromAnalytics(rows), nil
}

func (s *VisualizationDataService) queryDistributionDatums(ctx context.Context, runtime *modelRuntime, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) ([]dashboard.Datum, error) {
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
