package compiler

import (
	"fmt"
	"strings"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
)

func compileVisualizationQueryBinding(ctx compileContext, authored reportdef.Visual) (visualizationdefinition.QueryBinding, error) {
	limit := compiledVisualLimit(authored)
	resultShape, err := compiledVisualResultShape(authored)
	if err != nil {
		return visualizationdefinition.QueryBinding{}, err
	}
	binding := visualizationdefinition.QueryBinding{
		Kind: visualizationdefinition.QueryAggregate, ResultShape: resultShape, ModelID: ctx.modelID, DatasetID: ctx.datasetID,
		Identity: interactionIdentity(authored.Interaction.PointSelection),
		Aggregate: &visualizationdefinition.AggregateQueryBinding{
			TableID: authored.Query.Table, Dimensions: compiledFields(authored.Query.Dimensions), Measures: compiledFields(authored.Query.Measures),
			Series: compiledOptionalField(authored.Query.Series), Time: compiledTime(authored.Query.Time), Sort: compiledSort(authored.Query.Sort), Limit: limit,
		},
	}
	switch ctx.capability.Renderer {
	case visualizationdefinition.RendererMapLibre:
		return compiledSpatialBinding(ctx.modelID, authored, ctx.model)
	case visualizationdefinition.RendererVegaLite:
		binding.Kind = visualizationdefinition.QueryCustom
		binding.ResultShape = visualizationdefinition.ResultCustomRows
		binding.Aggregate = nil
		binding.Custom = &visualizationdefinition.CustomQueryBinding{TableID: authored.Query.Table, Fields: compiledVisualFields(authored.Query), Sort: compiledSort(authored.Query.Sort), Limit: limit}
	}
	return binding, nil
}

func compiledVisualResultShape(authored reportdef.Visual) (visualizationdefinition.ResultShape, error) {
	if _, ok := reportdef.VisualizationCapabilityForType(authored.Type); !ok {
		return "", fmt.Errorf("unsupported visualization type %q", authored.Type)
	}
	switch authored.ResultShape() {
	case "single_value":
		return visualizationdefinition.ResultScalar, nil
	case "category_multi_measure":
		return visualizationdefinition.ResultCategoryMultiMeasure, nil
	case "category_delta":
		return visualizationdefinition.ResultCategoryDelta, nil
	case "binned_measure":
		return visualizationdefinition.ResultHistogramBins, nil
	case "hierarchy":
		return visualizationdefinition.ResultHierarchyNodes, nil
	case "matrix":
		return visualizationdefinition.ResultMatrixCells, nil
	case "graph":
		return visualizationdefinition.ResultGraphEdges, nil
	case "geo":
		return visualizationdefinition.ResultGeographicFeatures, nil
	case "ohlc":
		return visualizationdefinition.ResultOHLC, nil
	case "distribution":
		return visualizationdefinition.ResultDistribution, nil
	case "custom":
		return visualizationdefinition.ResultCustomRows, nil
	case "category_series_value":
		return visualizationdefinition.ResultCategorySeriesValue, nil
	case "category_value":
		return visualizationdefinition.ResultCategoryValue, nil
	default:
		return "", fmt.Errorf("unsupported visualization result shape %q", authored.ResultShape())
	}
}

func compiledSpatialBinding(modelID string, authored reportdef.Visual, model *semanticmodel.Model) (visualizationdefinition.QueryBinding, error) {
	limit := compiledVisualLimit(authored)
	spatial := &visualizationdefinition.SpatialQueryBinding{
		TableID: authored.Query.Table, Dimensions: compiledFields(authored.Query.Dimensions), Measures: compiledFields(authored.Query.Measures),
		Series: compiledOptionalField(authored.Query.Series), Time: compiledTime(authored.Query.Time), Sort: compiledSort(authored.Query.Sort), Limit: limit,
	}
	if limit > 20_000 {
		latitudeAlias, longitudeAlias, found, err := authoredViewportCoordinates(authored.Geo.Layers)
		if err != nil {
			return visualizationdefinition.QueryBinding{}, err
		}
		if found {
			if model != nil {
				for _, field := range authored.Query.Measures {
					measure, err := model.ResolveMeasure(field.Field)
					if err != nil {
						return visualizationdefinition.QueryBinding{}, fmt.Errorf("windowed geographic measure %q must be an atomic measure: %w", field.Field, err)
					}
					switch measure.Aggregation {
					case "count", "sum", "min", "max":
					default:
						return visualizationdefinition.QueryBinding{}, fmt.Errorf("windowed geographic measure %q uses non-reaggregatable %q aggregation", field.Field, measure.Aggregation)
					}
				}
			}
			fields := compiledVisualFields(authored.Query)
			latitude, latitudeOK := fieldBindingByAlias(fields, latitudeAlias)
			longitude, longitudeOK := fieldBindingByAlias(fields, longitudeAlias)
			if !latitudeOK || !longitudeOK {
				return visualizationdefinition.QueryBinding{}, fmt.Errorf("spatial viewport coordinates %q and %q must reference compiled query aliases", latitudeAlias, longitudeAlias)
			}
			spatial.Viewport = &visualizationdefinition.SpatialViewportBinding{Latitude: latitude, Longitude: longitude, FeatureCap: 5000, RawMinimumZoom: 10}
		}
	}
	return visualizationdefinition.QueryBinding{
		Kind: visualizationdefinition.QuerySpatial, ResultShape: visualizationdefinition.ResultGeographicFeatures, ModelID: modelID, DatasetID: "primary", Identity: interactionIdentity(authored.Interaction.PointSelection), Spatial: spatial,
	}, nil
}

func authoredViewportCoordinates(layers []reportdef.VisualGeoLayer) (latitude, longitude string, found bool, err error) {
	for _, layer := range layers {
		switch layer.Kind {
		case "point", "heat", "density", "path":
		default:
			continue
		}
		if strings.TrimSpace(layer.Latitude) == "" || strings.TrimSpace(layer.Longitude) == "" {
			continue
		}
		if !found {
			latitude, longitude, found = layer.Latitude, layer.Longitude, true
			continue
		}
		if latitude != layer.Latitude || longitude != layer.Longitude {
			return "", "", false, fmt.Errorf("windowed geographic coordinate layers must share one latitude/longitude pair")
		}
	}
	return latitude, longitude, found, nil
}

func fieldBindingByAlias(fields []visualizationdefinition.FieldBinding, alias string) (visualizationdefinition.FieldBinding, bool) {
	for _, field := range fields {
		if field.Alias == alias {
			return field, true
		}
	}
	return visualizationdefinition.FieldBinding{}, false
}

func compiledVisualLimit(authored reportdef.Visual) int64 {
	if authored.DataBudget.MaxRows > 0 {
		return int64(authored.DataBudget.MaxRows)
	}
	if authored.Query.Limit > 0 {
		return int64(authored.Query.Limit)
	}
	if authored.Type == "kpi" || authored.Type == "gauge" {
		return 1
	}
	if authored.Type == "map" {
		return 20_000
	}
	return 1000
}

func compiledVisualFrameLimit(authored reportdef.Visual, shape string) int64 {
	limit := compiledVisualLimit(authored)
	if authored.DataBudget.MaxRows > 0 {
		return limit
	}
	switch shape {
	case "category_multi_measure":
		series := len(authored.Query.Measures)
		if series < 1 {
			series = 1
		}
		return limit * int64(series)
	case "hierarchy":
		levels := len(authored.Query.Dimensions)
		if authored.Query.Time.Field != "" {
			levels++
		}
		if levels < 1 {
			levels = 1
		}
		return limit * int64(levels)
	default:
		return limit
	}
}

func compiledTableBinding(modelID, visualType string, authored reportdef.TableVisual) visualizationdefinition.QueryBinding {
	binding := visualizationdefinition.QueryBinding{
		ModelID: modelID, DatasetID: "primary", Identity: interactionIdentity(authored.Interaction.RowSelection),
	}
	switch visualType {
	case "matrix":
		binding.Kind = visualizationdefinition.QueryMatrix
		binding.ResultShape = visualizationdefinition.ResultMatrixWindow
		binding.Matrix = &visualizationdefinition.MatrixQueryBinding{
			TableID: authored.Query.Table, Rows: compiledFields(authored.Query.Rows), Columns: compiledFields(authored.Query.Columns), Measures: compiledFields(authored.Query.Measures), Limit: dashboard.TableInteractiveRowCap,
		}
	case "pivot":
		binding.Kind = visualizationdefinition.QueryPivot
		binding.ResultShape = visualizationdefinition.ResultPivotWindow
		binding.Pivot = &visualizationdefinition.PivotQueryBinding{
			TableID: authored.Query.Table, Rows: compiledFields(authored.Query.Rows), Columns: compiledFields(authored.Query.Columns), Measures: compiledFields(authored.Query.Measures), Limit: dashboard.TableInteractiveRowCap,
		}
	default:
		sort := []visualizationdefinition.Sort{}
		if authored.DefaultSort.Key != "" {
			sort = append(sort, visualizationdefinition.Sort{FieldID: authored.DefaultSort.Key, Direction: authored.DefaultSort.Direction})
		}
		binding.Kind = visualizationdefinition.QueryDetail
		binding.ResultShape = visualizationdefinition.ResultDetailWindow
		binding.Detail = &visualizationdefinition.DetailQueryBinding{
			TableID: authored.Query.Table, Fields: compiledTableFields(authored), DefaultSort: sort, Limit: dashboard.TableInteractiveRowCap,
		}
	}
	return binding
}

func compiledFields(fields []reportdef.FieldRef) []visualizationdefinition.FieldBinding {
	out := make([]visualizationdefinition.FieldBinding, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Field) == "" {
			continue
		}
		alias := field.Alias
		if alias == "" {
			alias = fieldAlias(field.Field)
		}
		out = append(out, visualizationdefinition.FieldBinding{FieldID: field.Field, Alias: alias})
	}
	return out
}

func compiledVisualFields(query reportdef.VisualQuery) []visualizationdefinition.FieldBinding {
	out := compiledFields(query.Dimensions)
	if query.Time.Field != "" {
		alias := query.Time.Alias
		if alias == "" {
			alias = fieldAlias(query.Time.Field)
		}
		out = append(out, visualizationdefinition.FieldBinding{FieldID: query.Time.Field, Alias: alias})
	}
	if series := compiledOptionalField(query.Series); series != nil {
		out = append(out, *series)
	}
	out = append(out, compiledFields(query.Measures)...)
	return out
}

func compiledOptionalField(field reportdef.FieldRef) *visualizationdefinition.FieldBinding {
	values := compiledFields([]reportdef.FieldRef{field})
	if len(values) == 0 {
		return nil
	}
	return &values[0]
}

func compiledTime(value reportdef.QueryTime) *visualizationdefinition.TimeBinding {
	if value.Field == "" {
		return nil
	}
	alias := value.Alias
	if alias == "" {
		alias = fieldAlias(value.Field)
	}
	return &visualizationdefinition.TimeBinding{FieldID: value.Field, Alias: alias, Grain: value.Grain}
}

func compiledTableFields(table reportdef.TableVisual) []visualizationdefinition.FieldBinding {
	fields := compiledFields(table.DataColumns)
	if len(fields) > 0 {
		return fields
	}
	out := make([]visualizationdefinition.FieldBinding, 0, len(table.Query.Fields))
	for _, field := range table.Query.Fields {
		out = append(out, visualizationdefinition.FieldBinding{FieldID: field, Alias: fieldAlias(field)})
	}
	return out
}

func fieldAlias(field string) string {
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
}

func visualQueryFields(query reportdef.VisualQuery) []string {
	fields := make([]string, 0, len(query.Dimensions)+len(query.Measures)+2)
	for _, value := range query.Dimensions {
		fields = append(fields, value.Field)
	}
	if !query.Series.IsZero() {
		fields = append(fields, query.Series.Field)
	}
	if query.Time.Field != "" {
		fields = append(fields, query.Time.Field)
	}
	for _, value := range query.Measures {
		fields = append(fields, value.Field)
	}
	return uniqueStrings(fields)
}

func tableQueryFields(table reportdef.TableVisual) []string {
	fields := make([]string, 0, len(table.DataColumns)+len(table.Query.Fields)+len(table.Query.Rows)+len(table.Query.Columns)+len(table.Query.Measures))
	for _, value := range table.DataColumns {
		fields = append(fields, value.Field)
	}
	fields = append(fields, table.Query.Fields...)
	for _, values := range [][]reportdef.FieldRef{table.Query.Rows, table.Query.Columns, table.Query.Measures} {
		for _, value := range values {
			fields = append(fields, value.Field)
		}
	}
	return uniqueStrings(fields)
}

func compiledSort(values []reportdef.Sort) []visualizationdefinition.Sort {
	out := make([]visualizationdefinition.Sort, len(values))
	for index, value := range values {
		out[index] = visualizationdefinition.Sort{FieldID: value.Field, Direction: value.Direction}
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
