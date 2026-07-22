package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationgeometry "github.com/Yacobolo/leapview/internal/visualization/geometry"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	visualizationmapasset "github.com/Yacobolo/leapview/internal/visualization/mapasset"
	visualizationruntime "github.com/Yacobolo/leapview/internal/visualization/runtime"
)

func CompileDashboardDefinition(authored *reportdef.Dashboard, visualizations map[string]visualizationdefinition.Definition) (dashboarddefinition.Definition, error) {
	filters := make(map[string]dashboarddefinition.FilterDefinition, len(authored.Filters))
	for id, filter := range authored.Filters {
		presets := make([]dashboarddefinition.FilterPreset, len(filter.Presets))
		for index, preset := range filter.Presets {
			presets[index] = dashboarddefinition.FilterPreset{Value: preset.Value, Label: preset.Label, From: preset.From, To: preset.To, RelativeDays: preset.RelativeDays}
		}
		options := make([]dashboarddefinition.FilterOption, len(filter.Options))
		for index, option := range filter.Options {
			options[index] = dashboarddefinition.FilterOption{Value: option.Value, Label: option.Label}
		}
		filters[id] = dashboarddefinition.FilterDefinition{
			Type: filter.Type, Label: filter.Label, Description: filter.Description, Dimension: filter.Dimension, Fact: filter.Fact,
			Default: dashboarddefinition.FilterDefault{Preset: filter.Default.Preset, From: filter.Default.From, To: filter.Default.To, Operator: filter.Default.Operator, Value: filter.Default.Value, Values: append([]string(nil), filter.Default.Values...)},
			Custom:  filter.Custom, Presets: presets, Operator: filter.Operator, Values: dashboarddefinition.FilterValues{Source: filter.Values.Source, Limit: filter.Values.Limit},
			DefaultOperator: filter.DefaultOperator, Operators: append([]string(nil), filter.Operators...), Options: options,
			URLParam: filter.URLParam, FromURLParam: filter.FromURLParam, ToURLParam: filter.ToURLParam, OperatorURLParam: filter.OperatorURLParam,
			Targets: dashboarddefinition.FilterTargets{Visuals: append([]string(nil), filter.Targets.Visuals...), Tables: append([]string(nil), filter.Targets.Tables...)},
		}
	}
	return dashboarddefinition.New(authored.ID, authored.Title, authored.Description, authored.SemanticModel, filters, authored.Pages, visualizations)
}

// compileVisualizationDefinitions is the one-way boundary from mutable YAML
// authoring objects to immutable renderer-independent serving definitions.
func compileVisualizationDefinitions(report *reportdef.Dashboard, models ...*semanticmodel.Model) (map[string]visualizationdefinition.Definition, error) {
	var model *semanticmodel.Model
	if len(models) > 0 {
		model = models[0]
	}
	out := make(map[string]visualizationdefinition.Definition, len(report.Visuals)+len(report.Tables))
	for _, id := range sortedMapKeys(report.Visuals) {
		authored := report.Visuals[id]
		var spec visualizationir.VisualizationSpec
		if authored.Type == "custom" {
			var err error
			spec, err = compileCustomVisualizationSpec(authored)
			if err != nil {
				return nil, fmt.Errorf("visual %q: %w", id, err)
			}
		} else if authored.Type == "map" {
			var err error
			spec, err = compileGeographicVisualizationSpec(authored)
			if err != nil {
				return nil, fmt.Errorf("visual %q: %w", id, err)
			}
		} else {
			var err error
			spec, err = compileBuiltInVisualizationSpec(id, authored, model)
			if err != nil {
				return nil, fmt.Errorf("visual %q: %w", id, err)
			}
		}
		limit := compiledVisualLimit(authored)
		binding := visualizationdefinition.QueryBinding{
			Kind: visualizationdefinition.QueryAggregate, ModelID: report.SemanticModel, DatasetID: "primary",
			Identity: interactionIdentity(authored.Interaction.PointSelection),
			Aggregate: &visualizationdefinition.AggregateQueryBinding{
				TableID: authored.Query.Table, Dimensions: compiledFields(authored.Query.Dimensions), Measures: compiledFields(authored.Query.Measures),
				Series: compiledOptionalField(authored.Query.Series), Time: compiledTime(authored.Query.Time), Sort: compiledSort(authored.Query.Sort), Limit: limit,
			},
		}
		if authored.Type == "map" {
			var err error
			binding, err = compiledSpatialBinding(report.SemanticModel, authored, model)
			if err != nil {
				return nil, fmt.Errorf("visual %q: %w", id, err)
			}
		} else if authored.Type == "custom" {
			binding.Kind = visualizationdefinition.QueryCustom
			binding.Aggregate = nil
			binding.Custom = &visualizationdefinition.CustomQueryBinding{TableID: authored.Query.Table, Fields: compiledVisualFields(authored.Query), Sort: compiledSort(authored.Query.Sort), Limit: limit}
		}
		definition, err := visualizationdefinition.New(id, spec, binding)
		if err != nil {
			return nil, fmt.Errorf("visual %q: %w", id, err)
		}
		out[id] = definition
	}
	for _, id := range sortedMapKeys(report.Tables) {
		authored := report.Tables[id]
		style := authored.Style.WithDefaults()
		columns := compiledDashboardTableColumns(authored, model)
		table := dashboard.Table{
			Kind: authored.KindOrDefault(), Title: firstNonEmpty(authored.Title, id), Style: style,
			Interaction: compiledInteraction("row_selection", authored.Interaction.RowSelection), Columns: columns,
			Cardinality: dashboard.TableCardinality{Kind: dashboard.CardinalityUnknown}, RowCap: dashboard.TableInteractiveRowCap,
			ChunkSize: dashboard.TableChunkSize, RowHeight: style.RowHeight(), Sort: authored.DefaultSort,
			Blocks: map[string]dashboard.TableBlock{},
		}
		envelope, err := visualizationruntime.TableEnvelope(id, table, 0, 0)
		if err != nil {
			return nil, fmt.Errorf("visual %q: %w", id, err)
		}
		binding := compiledTableBinding(report.SemanticModel, authored)
		applyCompiledGridContract(&envelope.Spec, binding, authored.MeasureFormatting)
		definition, err := visualizationdefinition.New(id, envelope.Spec, binding)
		if err != nil {
			return nil, fmt.Errorf("visual %q: %w", id, err)
		}
		out[id] = definition
	}
	return out, nil
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
		Kind: visualizationdefinition.QuerySpatial, ModelID: modelID, DatasetID: "primary", Identity: interactionIdentity(authored.Interaction.PointSelection), Spatial: spatial,
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

func compiledVisualizationShape(authored reportdef.Visual) string {
	switch authored.Type {
	case "kpi", "gauge":
		return "single_value"
	case "combo":
		return "category_multi_measure"
	case "waterfall":
		return "category_delta"
	case "histogram":
		return "binned_measure"
	case "treemap", "sunburst", "tree":
		return "hierarchy"
	case "heatmap":
		return "matrix"
	case "sankey", "graph":
		return "graph"
	case "map":
		return "geo"
	case "candlestick":
		return "ohlc"
	case "boxplot":
		return "distribution"
	}
	if !authored.Query.Series.IsZero() {
		return "category_series_value"
	}
	return "category_value"
}

func compiledDashboardTableColumns(authored reportdef.TableVisual, model *semanticmodel.Model) []dashboard.TableColumn {
	bindings := compiledTableFields(authored)
	if authored.KindOrDefault() != "data_table" {
		bindings = append(compiledFields(authored.Query.Rows), compiledFields(authored.Query.Columns)...)
		bindings = append(bindings, compiledFields(authored.Query.Measures)...)
	}
	overrides := make(map[string]dashboard.TableColumn, len(authored.Columns))
	for _, column := range authored.Columns {
		overrides[column.Key] = column
	}
	out := make([]dashboard.TableColumn, 0, len(bindings))
	for _, binding := range bindings {
		column := dashboard.TableColumn{Key: binding.Alias, Label: binding.Alias}
		if model != nil {
			if dimension, err := model.ResolveDimension(binding.FieldID); err == nil {
				column.Role = "row_header"
				column.Format = compiledPhysicalFieldFormat(model, binding.FieldID, dimension.Type)
				if dimension.Label != "" {
					column.Label = dimension.Label
				}
			} else if measure, err := model.ResolveMeasure(binding.FieldID); err == nil {
				column.Role, column.Align, column.Measure = "measure", "right", binding.Alias
				if measure.Label != "" {
					column.Label = measure.Label
				}
				column.Format = compiledMeasureFormat(measure.Format)
			}
		}
		if override, ok := overrides[binding.Alias]; ok {
			column = mergeCompiledTableColumn(column, override)
		}
		if rules := authored.MeasureFormatting[binding.FieldID]; len(rules) > 0 {
			column.Formatting = append([]dashboard.TableFormattingRule(nil), rules...)
		}
		out = append(out, column)
	}
	return out
}

func compiledDimensionFormat(semanticType string) string {
	switch semanticType {
	case "number":
		return "decimal"
	case "boolean":
		return "boolean"
	case "date":
		return "date"
	case "timestamp":
		return "timestamp"
	default:
		return ""
	}
}

func compiledPhysicalFieldFormat(model *semanticmodel.Model, fieldID, semanticType string) string {
	if format := compiledDimensionFormat(semanticType); format != "" {
		return format
	}
	if model == nil {
		return ""
	}
	for _, measureID := range sortedMapKeys(model.Measures) {
		measure := model.Measures[measureID]
		if measure.Input.Field == fieldID && (measure.Aggregation == "sum" || measure.Aggregation == "avg" || measure.Aggregation == "min" || measure.Aggregation == "max") {
			return compiledMeasureFormat(measure.Format)
		}
	}
	return ""
}

func mergeCompiledTableColumn(base, override dashboard.TableColumn) dashboard.TableColumn {
	if override.Label != "" {
		base.Label = override.Label
	}
	if override.Align != "" {
		base.Align = override.Align
	}
	if override.Role != "" {
		base.Role = override.Role
	}
	if override.Group != "" {
		base.Group = override.Group
	}
	if override.Measure != "" {
		base.Measure = override.Measure
	}
	if override.ColumnValue != "" {
		base.ColumnValue = override.ColumnValue
	}
	if override.Width > 0 {
		base.Width = override.Width
	}
	if override.Format != "" {
		base.Format = override.Format
	}
	if len(override.Formatting) > 0 {
		base.Formatting = append([]dashboard.TableFormattingRule(nil), override.Formatting...)
	}
	return base
}

func compiledMeasureFormat(value string) string {
	switch value {
	case "integer", "currency":
		return value
	default:
		return "decimal"
	}
}

func applyCompiledGridContract(spec *visualizationir.VisualizationSpec, binding visualizationdefinition.QueryBinding, formatting map[string][]dashboard.TableFormattingRule) {
	refs := func(fields []visualizationdefinition.FieldBinding) []visualizationir.VisualizationFieldRef {
		out := make([]visualizationir.VisualizationFieldRef, len(fields))
		for index, field := range fields {
			out[index] = visualizationir.VisualizationFieldRef{Dataset: "primary", Field: field.Alias}
		}
		return out
	}
	compiledFormatting := map[string][]visualizationir.TableVisualizationFormattingRule{}
	for field, rules := range formatting {
		compiledFormatting[fieldAlias(field)] = visualizationruntime.TableFormatting(rules)
	}
	switch value := spec.Value.(type) {
	case *visualizationir.MatrixVisualizationSpec:
		value.Rows, value.Columns, value.Measures = refs(binding.Matrix.Rows), refs(binding.Matrix.Columns), refs(binding.Matrix.Measures)
		value.MeasureFormatting = compiledFormatting
	case *visualizationir.PivotVisualizationSpec:
		value.Rows, value.Columns, value.Measures = refs(binding.Pivot.Rows), refs(binding.Pivot.Columns), refs(binding.Pivot.Measures)
		value.MeasureFormatting = compiledFormatting
	}
}

func CompileVisualizationDefinitions(report *reportdef.Dashboard, models ...*semanticmodel.Model) (map[string]visualizationdefinition.Definition, error) {
	return compileVisualizationDefinitions(report, models...)
}

func compiledVisualTitle(authored reportdef.Visual, id string, model *semanticmodel.Model) string {
	if authored.Title != "" {
		return authored.Title
	}
	if model != nil && len(authored.Query.Measures) > 0 {
		measureID := authored.Query.Measures[0].Field
		if measure, err := model.ResolveMeasure(measureID); err == nil && strings.TrimSpace(measure.Label) != "" {
			return measure.Label
		}
		if metric, ok := model.Metrics[measureID]; ok && strings.TrimSpace(metric.Label) != "" {
			return metric.Label
		}
	}
	return id
}

// compileBuiltInVisualizationSpec is the canonical authoring-to-IR boundary
// for first-party charts. Runtime data never participates in specification
// construction; it is shaped later against the immutable dataset schema.
func compileBuiltInVisualizationSpec(id string, authored reportdef.Visual, model *semanticmodel.Model) (visualizationir.VisualizationSpec, error) {
	shape := compiledVisualizationShape(authored)
	columns := compiledShapeColumns(shape)
	if shape == "hierarchy" {
		seen := map[string]struct{}{"node": {}, "parent": {}, "value": {}}
		for _, binding := range compiledFields(authored.Query.Dimensions) {
			if _, exists := seen[binding.Alias]; exists {
				return visualizationir.VisualizationSpec{}, fmt.Errorf("hierarchy query alias %q conflicts with a reserved frame field", binding.Alias)
			}
			seen[binding.Alias] = struct{}{}
			columns = append(columns, binding.Alias)
		}
		if binding := compiledTime(authored.Query.Time); binding != nil {
			if _, exists := seen[binding.Alias]; exists {
				return visualizationir.VisualizationSpec{}, fmt.Errorf("hierarchy query alias %q conflicts with a reserved frame field", binding.Alias)
			}
			columns = append(columns, binding.Alias)
		}
	}
	fields := make([]visualizationir.VisualizationField, len(columns))
	identities := map[string]struct{}{}
	for _, mapping := range authored.Interaction.PointSelection.Mappings {
		identities[mapping.Value] = struct{}{}
	}
	for index, column := range columns {
		role := visualizationir.VisualizationFieldRoleDimension
		if compiledShapeMeasure(column) {
			role = visualizationir.VisualizationFieldRoleMeasure
		}
		if _, ok := identities[column]; ok {
			role = visualizationir.VisualizationFieldRoleIdentity
		}
		fields[index] = visualizationir.VisualizationField{ID: column, Role: role, DataType: compiledShapeDataType(column), Nullable: true, Label: compiledShapeLabel(column)}
	}
	applyBuiltInFieldSemantics(fields, shape, authored, model)
	title := compiledVisualTitle(authored, id, model)
	accessibilityTitle := title
	if authored.Accessibility.Title != "" {
		accessibilityTitle = authored.Accessibility.Title
	}
	accessibilityDescription := title
	if authored.Accessibility.Description != "" {
		accessibilityDescription = authored.Accessibility.Description
	}
	completeness := visualizationir.VisualizationCompletenessComplete
	if authored.DataBudget.RequiredCompleteness != "" {
		completeness = visualizationir.VisualizationCompleteness(authored.DataBudget.RequiredCompleteness)
	}
	accessibility := visualizationir.VisualizationAccessibility{Title: accessibilityTitle, Description: accessibilityDescription}
	if authored.Accessibility.Summary != "" {
		accessibility.Summary = &authored.Accessibility.Summary
	}
	if authored.Accessibility.AnnounceChanges {
		accessibility.AnnounceChanges = &authored.Accessibility.AnnounceChanges
	}
	base := visualizationir.VisualizationSpecBase{
		Title: title, Datasets: []visualizationir.VisualizationDatasetSchema{{ID: "primary", Fields: fields}},
		DataBudget:    visualizationir.VisualizationDataBudget{MaxRows: compiledVisualFrameLimit(authored, shape), RequiredCompleteness: completeness},
		Accessibility: accessibility, Interactions: customVisualizationInteractions(authored.Interaction.PointSelection),
	}
	ref := func(field string) visualizationir.VisualizationFieldRef {
		return visualizationir.VisualizationFieldRef{Dataset: "primary", Field: field}
	}
	optionalRef := func(field string) *visualizationir.VisualizationFieldRef {
		for _, column := range columns {
			if column == field {
				value := ref(field)
				return &value
			}
		}
		return nil
	}
	presentation := authored.Presentation
	common := visualizationir.VisualizationPresentation{Legend: compiledLegend(presentation.Legend), ShowLabels: presentation.ShowLabels}

	switch authored.Type {
	case "kpi":
		base.Kind = "kpi"
		return visualizationir.VisualizationSpec{Value: &visualizationir.KPIVisualizationSpec{
			VisualizationSpecBase: base, Kind: "kpi", Value: ref("value"),
			Presentation: visualizationir.KPIVisualizationPresentation{Trend: compiledKPITrend(presentation.Tone), Note: optionalString(presentation.Note), Tone: compiledTone(presentation.Tone), Thresholds: compiledThresholds(presentation.Thresholds)},
		}}, nil
	case "pie", "donut", "funnel":
		base.Kind = "proportional"
		return visualizationir.VisualizationSpec{Value: &visualizationir.ProportionalVisualizationSpec{
			VisualizationSpecBase: base, Kind: "proportional", Mark: visualizationir.VisualizationProportionalMark(authored.Type), Category: ref("label"), Value: ref("value"), Series: optionalRef("series"),
			Presentation: visualizationir.ProportionalVisualizationPresentation{VisualizationPresentation: common, Orientation: compiledOrientation(presentation.Orientation), Rose: presentation.Rose, CenterLabel: optionalString(presentation.CenterLabel), LabelPosition: compiledLabelPosition(presentation.LabelPosition), InnerRadius: optionalPositiveFloat(presentation.InnerRadius), OuterRadius: optionalPositiveFloat(presentation.OuterRadius), Align: optionalString(presentation.Align), Sort: compiledSortDirection(presentation.Sort)},
		}}, nil
	case "treemap", "sunburst", "tree", "sankey", "graph":
		base.Kind = "hierarchy"
		return visualizationir.VisualizationSpec{Value: &visualizationir.HierarchyVisualizationSpec{
			VisualizationSpecBase: base, Kind: "hierarchy", Mark: visualizationir.VisualizationHierarchyMark(authored.Type), Node: ref(firstCompiledField(columns, "node", "source", "label")), Value: optionalRef("value"), Parent: optionalRef("parent"), Source: optionalRef("source"), Target: optionalRef("target"),
			Presentation: visualizationir.HierarchyVisualizationPresentation{VisualizationPresentation: common, Orientation: compiledOrientation(presentation.Orientation), InitialDepth: optionalPositiveInt32(presentation.InitialDepth), Roam: presentation.Roam, Layout: compiledHierarchyLayout(presentation.Layout), Breadcrumb: presentation.Breadcrumb, NodeGap: optionalPositiveFloat(presentation.NodeGap), Curveness: optionalPositiveFloat(presentation.Curveness), Focus: compiledGraphFocus(presentation.Focus)},
		}}, nil
	case "radar", "gauge":
		base.Kind = "polar"
		return visualizationir.VisualizationSpec{Value: &visualizationir.PolarVisualizationSpec{
			VisualizationSpecBase: base, Kind: "polar", Mark: visualizationir.VisualizationPolarMark(authored.Type), Category: optionalRef("label"), Value: ref("value"), Series: optionalRef("series"),
			Presentation: visualizationir.PolarVisualizationPresentation{VisualizationPresentation: common, Minimum: presentation.Minimum, Maximum: presentation.Maximum, ShowPointer: true, Area: presentation.Area, ProgressWidth: optionalPositiveFloat(presentation.ProgressWidth), Thresholds: compiledThresholds(presentation.Thresholds)},
		}}, nil
	default:
		mark := visualizationir.VisualizationCartesianMark(authored.Type)
		supported := map[visualizationir.VisualizationCartesianMark]bool{
			visualizationir.VisualizationCartesianMarkLine: true, visualizationir.VisualizationCartesianMarkArea: true, visualizationir.VisualizationCartesianMarkBar: true,
			visualizationir.VisualizationCartesianMarkColumn: true, visualizationir.VisualizationCartesianMarkScatter: true, visualizationir.VisualizationCartesianMarkHistogram: true,
			visualizationir.VisualizationCartesianMarkCombo: true, visualizationir.VisualizationCartesianMarkWaterfall: true, visualizationir.VisualizationCartesianMarkCandlestick: true,
			visualizationir.VisualizationCartesianMarkBoxplot: true, visualizationir.VisualizationCartesianMarkHeatmap: true,
		}
		if !supported[mark] {
			return visualizationir.VisualizationSpec{}, fmt.Errorf("unsupported visualization type %q", authored.Type)
		}
		base.Kind = "cartesian"
		xField := firstCompiledField(columns, "label", "row", "name")
		y := make([]visualizationir.VisualizationFieldRef, 0, len(columns))
		for _, column := range columns {
			if column != xField && column != "series" && column != "selected" && column != "positive" {
				y = append(y, ref(column))
			}
		}
		if len(y) == 0 {
			y = append(y, ref("value"))
		}
		showSymbols := true
		if presentation.ShowSymbols != nil {
			showSymbols = *presentation.ShowSymbols
		}
		area := authored.Type == "area"
		if presentation.Area != nil && *presentation.Area {
			area = true
		}
		return visualizationir.VisualizationSpec{Value: &visualizationir.CartesianVisualizationSpec{
			VisualizationSpecBase: base, Kind: "cartesian", Mark: mark, X: ref(xField), Y: y, Series: optionalRef("series"),
			Presentation: visualizationir.CartesianVisualizationPresentation{VisualizationPresentation: common, Smooth: presentation.Smooth, Stacked: presentation.Stacked, ShowSymbols: showSymbols, DataZoom: presentation.DataZoom, Area: area, Step: presentation.Step, Orientation: compiledOptionalOrientation(presentation.Orientation), LabelPosition: compiledLabelPosition(presentation.LabelPosition), SymbolSize: optionalPositiveFloat(presentation.SymbolSize), HistogramBins: optionalPositiveInt32(presentation.HistogramBins), ComboSeries: compiledComboSeries(presentation.SeriesTypes, presentation.DualAxis)},
		}}, nil
	}
}

func applyBuiltInFieldSemantics(fields []visualizationir.VisualizationField, shape string, authored reportdef.Visual, model *semanticmodel.Model) {
	if model == nil {
		return
	}
	byID := make(map[string]*visualizationir.VisualizationField, len(fields))
	for index := range fields {
		byID[fields[index].ID] = &fields[index]
	}
	decorate := func(id string, binding reportdef.FieldRef) {
		field := byID[id]
		if field == nil || strings.TrimSpace(binding.Field) == "" {
			return
		}
		applySemanticField(field, binding.Field, model)
	}
	var dimension reportdef.FieldRef
	if len(authored.Query.Dimensions) > 0 {
		dimension = authored.Query.Dimensions[0]
	} else if authored.Query.Time.Field != "" {
		dimension = reportdef.FieldRef{Field: authored.Query.Time.Field, Alias: authored.Query.Time.Alias}
	}
	var measure reportdef.FieldRef
	if len(authored.Query.Measures) > 0 {
		measure = authored.Query.Measures[0]
	}

	switch shape {
	case "single_value", "category_value", "category_series_value", "category_multi_measure", "category_delta", "binned_measure", "ohlc", "distribution":
		decorate("label", dimension)
	case "matrix":
		decorate("row", dimension)
	case "graph":
		if len(authored.Query.Dimensions) > 0 {
			decorate("source", authored.Query.Dimensions[0])
		}
		if len(authored.Query.Dimensions) > 1 {
			decorate("target", authored.Query.Dimensions[1])
		}
	}
	if !authored.Query.Series.IsZero() {
		decorate("series", authored.Query.Series)
	}
	// A normalized multi-measure frame stores heterogeneous measures in one
	// value column. Do not attach one measure's format or source identity to all
	// rows; row-specific formatting requires a future typed series-format map.
	if shape != "category_multi_measure" {
		for _, id := range []string{"value", "start", "end", "binStart", "binEnd"} {
			decorate(id, measure)
		}
	}
	if shape == "ohlc" || shape == "distribution" {
		for index, binding := range authored.Query.Measures {
			alias := binding.Alias
			if alias == "" {
				alias = fieldAlias(binding.Field)
			}
			if byID[alias] != nil {
				decorate(alias, binding)
				continue
			}
			ordered := map[string][]string{"ohlc": {"open", "close", "low", "high"}, "distribution": {"min", "q1", "median", "q3", "max"}}[shape]
			if index < len(ordered) {
				decorate(ordered[index], binding)
			}
		}
	}
}

func applySemanticField(field *visualizationir.VisualizationField, source string, model *semanticmodel.Model) {
	field.SourceRef = &source
	if dimension, err := model.ResolveDimension(source); err == nil {
		if dimension.Label != "" {
			field.Label = dimension.Label
		}
		field.DataType = compiledDimensionDataType(dimension.Type)
		field.Format = compiledVisualizationFormat(compiledDimensionFormat(dimension.Type), "")
		return
	}
	if measure, err := model.ResolveMeasure(source); err == nil {
		if measure.Label != "" {
			field.Label = measure.Label
		}
		field.DataType = compiledMeasureDataType(measure.Format)
		field.Format = compiledVisualizationFormat(measure.Format, measure.Unit)
		return
	}
	if metric, ok := model.Metrics[source]; ok {
		if metric.Label != "" {
			field.Label = metric.Label
		}
		field.DataType = compiledMeasureDataType(metric.Format)
		field.Format = compiledVisualizationFormat(metric.Format, metric.Unit)
	}
}

func compiledDimensionDataType(dimensionType string) visualizationir.VisualizationDataType {
	switch dimensionType {
	case "boolean":
		return visualizationir.VisualizationDataTypeBoolean
	case "date":
		return visualizationir.VisualizationDataTypeDate
	case "timestamp":
		return visualizationir.VisualizationDataTypeTemporal
	case "number":
		return visualizationir.VisualizationDataTypeDecimal
	default:
		return visualizationir.VisualizationDataTypeString
	}

}

func compiledMeasureDataType(format string) visualizationir.VisualizationDataType {
	if format == "integer" {
		return visualizationir.VisualizationDataTypeInteger
	}
	return visualizationir.VisualizationDataTypeDecimal
}

func compiledVisualizationFormat(format, unit string) *visualizationir.VisualizationFormat {
	var value visualizationir.VisualizationFormat
	switch format {
	case "integer", "decimal":
		value.Value = &visualizationir.NumberVisualizationFormat{Kind: "number"}
	case "currency":
		currency := "BRL"
		switch strings.TrimSpace(unit) {
		case "$", "USD":
			currency = "USD"
		case "€", "EUR":
			currency = "EUR"
		}
		value.Value = &visualizationir.CurrencyVisualizationFormat{Kind: "currency", Currency: currency}
	case "date", "timestamp":
		value.Value = &visualizationir.TemporalVisualizationFormat{Kind: "temporal"}
	default:
		return nil
	}
	return &value
}

func compiledShapeColumns(shape string) []string {
	columns := map[string][]string{
		"single_value": {"label", "value", "series"}, "category_value": {"label", "value"}, "category_series_value": {"label", "series", "value"},
		"category_multi_measure": {"label", "series", "value"}, "category_delta": {"label", "value", "start", "end", "positive"},
		"binned_measure": {"label", "binStart", "binEnd", "value"}, "hierarchy": {"node", "parent", "value"}, "matrix": {"row", "column", "value"},
		"graph": {"source", "target", "value"}, "ohlc": {"label", "open", "close", "low", "high"}, "distribution": {"label", "min", "q1", "median", "q3", "max"},
	}[shape]
	return append([]string(nil), columns...)
}

func compiledShapeMeasure(field string) bool {
	switch field {
	case "value", "start", "end", "binStart", "binEnd", "open", "close", "low", "high", "min", "q1", "median", "q3", "max":
		return true
	default:
		return false
	}
}

func compiledShapeDataType(field string) visualizationir.VisualizationDataType {
	if compiledShapeMeasure(field) {
		return visualizationir.VisualizationDataTypeDecimal
	}
	if field == "positive" {
		return visualizationir.VisualizationDataTypeBoolean
	}
	return visualizationir.VisualizationDataTypeString
}

func compiledShapeLabel(field string) string {
	labels := map[string]string{"binStart": "Bin Start", "binEnd": "Bin End", "q1": "Q1", "q3": "Q3"}
	if label := labels[field]; label != "" {
		return label
	}
	if field == "" {
		return "Value"
	}
	return strings.ToUpper(field[:1]) + field[1:]
}

func firstCompiledField(columns []string, candidates ...string) string {
	for _, candidate := range candidates {
		for _, column := range columns {
			if candidate == column {
				return candidate
			}
		}
	}
	if len(columns) > 0 {
		return columns[0]
	}
	return "value"
}

func compiledLegend(value string) visualizationir.VisualizationLegendPosition {
	switch value {
	case "hidden":
		return visualizationir.VisualizationLegendPositionHidden
	case "top":
		return visualizationir.VisualizationLegendPositionTop
	case "right":
		return visualizationir.VisualizationLegendPositionRight
	case "left":
		return visualizationir.VisualizationLegendPositionLeft
	default:
		return visualizationir.VisualizationLegendPositionBottom
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalPositiveFloat(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func optionalPositiveInt32(value int) *int32 {
	if value <= 0 {
		return nil
	}
	out := int32(value)
	return &out
}

func compiledOrientation(value string) visualizationir.VisualizationOrientation {
	if value == "horizontal" {
		return visualizationir.VisualizationOrientationHorizontal
	}
	return visualizationir.VisualizationOrientationVertical
}

func compiledOptionalOrientation(value string) *visualizationir.VisualizationOrientation {
	if value == "" {
		return nil
	}
	out := compiledOrientation(value)
	return &out
}

func compiledLabelPosition(value string) *visualizationir.VisualizationLabelPosition {
	if value == "" {
		return nil
	}
	out := visualizationir.VisualizationLabelPosition(value)
	return &out
}

func compiledHierarchyLayout(value string) *visualizationir.VisualizationHierarchyLayout {
	if value == "" {
		return nil
	}
	out := visualizationir.VisualizationHierarchyLayout(value)
	return &out
}

func compiledGraphFocus(value string) *visualizationir.VisualizationGraphFocus {
	if value == "" {
		return nil
	}
	out := visualizationir.VisualizationGraphFocus(value)
	return &out
}

func compiledSortDirection(value string) *visualizationir.VisualizationSortDirection {
	if value == "" {
		return nil
	}
	out := visualizationir.VisualizationSortDirection(value)
	return &out
}

func compiledTone(value string) *visualizationir.VisualizationTone {
	if value == "" {
		return nil
	}
	out := visualizationir.VisualizationTone(value)
	return &out
}

func compiledKPITrend(value string) visualizationir.VisualizationKPITrend {
	switch value {
	case "success", "positive":
		return visualizationir.VisualizationKPITrendPositive
	case "danger", "negative":
		return visualizationir.VisualizationKPITrendNegative
	default:
		return visualizationir.VisualizationKPITrendNeutral
	}
}

func compiledThresholds(values []reportdef.VisualThreshold) *[]visualizationir.VisualizationThreshold {
	if len(values) == 0 {
		return nil
	}
	out := make([]visualizationir.VisualizationThreshold, len(values))
	for index, value := range values {
		out[index] = visualizationir.VisualizationThreshold{Value: value.Value, Tone: visualizationir.VisualizationTone(value.Tone)}
	}
	return &out
}

func compiledComboSeries(values map[string]string, dualAxis bool) *[]visualizationir.VisualizationComboSeries {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]visualizationir.VisualizationComboSeries, len(keys))
	for index, key := range keys {
		axis := visualizationir.VisualizationAxisPrimary
		if dualAxis && index > 0 {
			axis = visualizationir.VisualizationAxisSecondary
		}
		out[index] = visualizationir.VisualizationComboSeries{SeriesValue: key, Mark: visualizationir.VisualizationCartesianMark(values[key]), Axis: axis}
	}
	return &out
}

func compileCustomVisualizationSpec(authored reportdef.Visual) (visualizationir.VisualizationSpec, error) {
	program, err := json.Marshal(authored.Custom.Program)
	if err != nil {
		return visualizationir.VisualizationSpec{}, fmt.Errorf("encode Vega-Lite program: %w", err)
	}
	fields := customVisualizationFields(authored.Query, authored.Interaction.PointSelection)
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field.ID] = struct{}{}
	}
	if err := validateCustomProgram(authored.Custom.Program, allowed, ""); err != nil {
		return visualizationir.VisualizationSpec{}, err
	}
	digest := sha256.Sum256(program)
	title := authored.Title
	if title == "" {
		title = "Custom visualization"
	}
	accessibilityTitle := authored.Accessibility.Title
	if accessibilityTitle == "" {
		accessibilityTitle = title
	}
	accessibilityDescription := authored.Accessibility.Description
	if accessibilityDescription == "" {
		accessibilityDescription = title
	}
	base := visualizationir.VisualizationSpecBase{
		Kind: "custom", Title: title, Datasets: []visualizationir.VisualizationDatasetSchema{{ID: "primary", Fields: fields}},
		DataBudget:    visualizationir.VisualizationDataBudget{MaxRows: compiledVisualLimit(authored), RequiredCompleteness: visualizationir.VisualizationCompletenessComplete},
		Accessibility: visualizationir.VisualizationAccessibility{Title: accessibilityTitle, Description: accessibilityDescription},
		Interactions:  customVisualizationInteractions(authored.Interaction.PointSelection),
	}
	return visualizationir.VisualizationSpec{Value: &visualizationir.CustomVisualizationSpec{
		VisualizationSpecBase: base, Kind: "custom", Engine: visualizationir.VisualizationCustomEngineVegaLite,
		Program: string(program), ProgramDigest: "sha256:" + hex.EncodeToString(digest[:]),
	}}, nil
}

func compileGeographicVisualizationSpec(authored reportdef.Visual) (visualizationir.VisualizationSpec, error) {
	fields := geographicVisualizationFields(authored)
	known := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		known[field.ID] = struct{}{}
	}
	fieldRef := func(layerID, property, alias string) (*visualizationir.VisualizationFieldRef, error) {
		if alias == "" {
			return nil, nil
		}
		if _, ok := known[alias]; !ok {
			return nil, fmt.Errorf("geographic layer %q %s references unknown query alias %q", layerID, property, alias)
		}
		ref := visualizationir.VisualizationFieldRef{Dataset: "primary", Field: alias}
		return &ref, nil
	}
	layers := make([]visualizationir.VisualizationGeographicLayer, len(authored.Geo.Layers))
	for index, authoredLayer := range authored.Geo.Layers {
		layer, err := compileGeographicLayer(authoredLayer, fieldRef)
		if err != nil {
			return visualizationir.VisualizationSpec{}, err
		}
		layers[index] = layer
	}
	title := authored.Title
	if title == "" {
		title = "Map"
	}
	accessibilityTitle := authored.Accessibility.Title
	if accessibilityTitle == "" {
		accessibilityTitle = title
	}
	accessibilityDescription := authored.Accessibility.Description
	if accessibilityDescription == "" {
		accessibilityDescription = title
	}
	base := visualizationir.VisualizationSpecBase{
		Kind: "geographic", Title: title, Datasets: []visualizationir.VisualizationDatasetSchema{{ID: "primary", Fields: fields}},
		DataBudget:    visualizationir.VisualizationDataBudget{MaxRows: compiledVisualLimit(authored), RequiredCompleteness: visualizationir.VisualizationCompletenessComplete},
		Accessibility: visualizationir.VisualizationAccessibility{Title: accessibilityTitle, Description: accessibilityDescription},
		Interactions:  customVisualizationInteractions(authored.Interaction.PointSelection),
	}
	legend := visualizationir.VisualizationLegendPosition(authored.Presentation.Legend)
	if legend == "" {
		legend = visualizationir.VisualizationLegendPositionHidden
	}
	basemapID := strings.TrimSpace(authored.Geo.Basemap)
	if basemapID == "" {
		basemapID = "streets"
	}
	var basemap *visualizationir.VisualizationMapStyleAsset
	if basemapID != "blank" {
		asset, err := visualizationmapasset.Resolve(basemapID)
		if err != nil {
			return visualizationir.VisualizationSpec{}, fmt.Errorf("geographic basemap: %w", err)
		}
		basemap = &asset
	}
	theme := visualizationir.VisualizationMapTheme(authored.Geo.Theme)
	if theme == "" {
		theme = visualizationir.VisualizationMapThemeAuto
	}
	labelDensity := visualizationir.VisualizationMapLabelDensity(authored.Geo.LabelDensity)
	if labelDensity == "" {
		labelDensity = visualizationir.VisualizationMapLabelDensityNormal
	}
	camera := compileMapCamera(authored.Geo.Camera)
	controls := compileMapControls(authored.Geo.Controls)
	spatialInteractions := compiledSpatialSelectionInteractions(authored.Interaction.SpatialSelection)
	return visualizationir.VisualizationSpec{Value: &visualizationir.GeographicVisualizationSpec{
		VisualizationSpecBase: base, Kind: "geographic", Layers: layers, SpatialInteractions: spatialInteractions,
		Presentation: visualizationir.GeographicVisualizationPresentation{
			VisualizationPresentation: visualizationir.VisualizationPresentation{Legend: legend, ShowLabels: authored.Presentation.ShowLabels},
			Roam:                      true, Basemap: basemap, Theme: theme, LabelDensity: labelDensity, Camera: camera, Controls: controls,
		},
	}}, nil
}

func compiledSpatialSelectionInteractions(selection reportdef.SpatialSelectionInteraction) []visualizationir.VisualizationSpatialSelectionInteraction {
	if selection.IsZero() {
		return []visualizationir.VisualizationSpatialSelectionInteraction{}
	}
	gestures := make([]visualizationir.VisualizationSpatialSelectionGesture, len(selection.Gestures))
	for index, gesture := range selection.Gestures {
		gestures[index] = visualizationir.VisualizationSpatialSelectionGesture(gesture)
	}
	mapping := func(value reportdef.SpatialSelectionMapping) visualizationir.VisualizationSpatialFieldMapping {
		return visualizationir.VisualizationSpatialFieldMapping{
			Source:        visualizationir.VisualizationFieldRef{Dataset: "primary", Field: value.Source},
			TargetFieldID: value.Field, TargetFactID: optionalString(value.Fact),
		}
	}
	return []visualizationir.VisualizationSpatialSelectionInteraction{{
		ID: "spatial_selection", Gestures: gestures, Latitude: mapping(selection.Latitude), Longitude: mapping(selection.Longitude), Targets: append([]string(nil), selection.Targets...),
	}}
}

type geographicFieldResolver func(layerID, property, alias string) (*visualizationir.VisualizationFieldRef, error)

func compileGeographicLayer(authored reportdef.VisualGeoLayer, fieldRef geographicFieldResolver) (visualizationir.VisualizationGeographicLayer, error) {
	ref := func(property, alias string) (*visualizationir.VisualizationFieldRef, error) {
		return fieldRef(authored.ID, property, alias)
	}
	value, err := ref("value", authored.Value)
	if err != nil {
		return visualizationir.VisualizationGeographicLayer{}, err
	}
	category, err := ref("category", authored.Category)
	if err != nil {
		return visualizationir.VisualizationGeographicLayer{}, err
	}
	label, err := ref("label", authored.Label)
	if err != nil {
		return visualizationir.VisualizationGeographicLayer{}, err
	}
	tooltip := make([]visualizationir.VisualizationFieldRef, 0, len(authored.Tooltip))
	for _, alias := range authored.Tooltip {
		field, err := ref("tooltip", alias)
		if err != nil {
			return visualizationir.VisualizationGeographicLayer{}, err
		}
		if field != nil {
			tooltip = append(tooltip, *field)
		}
	}
	base := visualizationir.VisualizationGeographicLayerBase{
		ID: authored.ID, Kind: authored.Kind, Label: label, Tooltip: tooltip,
		Position: mapLayerPosition(authored.Position), Visibility: mapVisibility(authored.Visibility),
	}
	color := mapColorScale(authored.Color)
	stroke := mapStroke(authored.Stroke)
	opacity := authored.Opacity
	if opacity == 0 {
		opacity = 0.82
	}
	coordinates := func() (visualizationir.VisualizationFieldRef, visualizationir.VisualizationFieldRef, error) {
		latitude, err := ref("latitude", authored.Latitude)
		if err != nil {
			return visualizationir.VisualizationFieldRef{}, visualizationir.VisualizationFieldRef{}, err
		}
		longitude, err := ref("longitude", authored.Longitude)
		if err != nil {
			return visualizationir.VisualizationFieldRef{}, visualizationir.VisualizationFieldRef{}, err
		}
		if latitude == nil || longitude == nil {
			return visualizationir.VisualizationFieldRef{}, visualizationir.VisualizationFieldRef{}, fmt.Errorf("geographic layer %q requires coordinates", authored.ID)
		}
		return *latitude, *longitude, nil
	}
	switch authored.Kind {
	case "point":
		latitude, longitude, err := coordinates()
		if err != nil {
			return visualizationir.VisualizationGeographicLayer{}, err
		}
		return visualizationir.VisualizationGeographicLayer{Value: &visualizationir.VisualizationPointLayer{
			VisualizationGeographicLayerBase: base, Kind: "point", Latitude: latitude, Longitude: longitude, Value: value, Category: category,
			Size: mapSizeScale(authored.Size), Color: color, Stroke: stroke, Cluster: mapCluster(authored.Cluster), Opacity: opacity,
		}}, nil
	case "choropleth":
		geometry, err := visualizationgeometry.Resolve(authored.GeometryAsset)
		if err != nil {
			return visualizationir.VisualizationGeographicLayer{}, fmt.Errorf("geographic layer %q: %w", authored.ID, err)
		}
		join, err := ref("join", authored.Join)
		if err != nil {
			return visualizationir.VisualizationGeographicLayer{}, err
		}
		if join == nil {
			return visualizationir.VisualizationGeographicLayer{}, fmt.Errorf("geographic layer %q requires join", authored.ID)
		}
		return visualizationir.VisualizationGeographicLayer{Value: &visualizationir.VisualizationChoroplethLayer{VisualizationGeographicLayerBase: base, Kind: "choropleth", Geometry: geometry, Join: *join, Value: value, Category: category, Color: color, Stroke: stroke, Opacity: opacity}}, nil
	case "heat", "density":
		latitude, longitude, err := coordinates()
		if err != nil {
			return visualizationir.VisualizationGeographicLayer{}, err
		}
		heat := mapHeatStyle(authored.Heat)
		if authored.Kind == "heat" {
			return visualizationir.VisualizationGeographicLayer{Value: &visualizationir.VisualizationHeatLayer{VisualizationGeographicLayerBase: base, Kind: "heat", Latitude: latitude, Longitude: longitude, Value: value, Color: color, Heat: heat, Opacity: opacity}}, nil
		}
		return visualizationir.VisualizationGeographicLayer{Value: &visualizationir.VisualizationDensityLayer{VisualizationGeographicLayerBase: base, Kind: "density", Latitude: latitude, Longitude: longitude, Value: value, Color: color, Heat: heat, Opacity: opacity}}, nil
	case "reference":
		geometry, err := visualizationgeometry.Resolve(authored.GeometryAsset)
		if err != nil {
			return visualizationir.VisualizationGeographicLayer{}, fmt.Errorf("geographic layer %q: %w", authored.ID, err)
		}
		return visualizationir.VisualizationGeographicLayer{Value: &visualizationir.VisualizationReferenceLayer{VisualizationGeographicLayerBase: base, Kind: "reference", Geometry: geometry, Color: color, Stroke: stroke, Opacity: opacity}}, nil
	case "path":
		latitude, longitude, err := coordinates()
		if err != nil {
			return visualizationir.VisualizationGeographicLayer{}, err
		}
		path, err := ref("path", authored.Path)
		if err != nil {
			return visualizationir.VisualizationGeographicLayer{}, err
		}
		order, err := ref("order", authored.Order)
		if err != nil {
			return visualizationir.VisualizationGeographicLayer{}, err
		}
		if path == nil || order == nil {
			return visualizationir.VisualizationGeographicLayer{}, fmt.Errorf("geographic layer %q requires path and order", authored.ID)
		}
		return visualizationir.VisualizationGeographicLayer{Value: &visualizationir.VisualizationPathLayer{VisualizationGeographicLayerBase: base, Kind: "path", Latitude: latitude, Longitude: longitude, Path: *path, Order: *order, Value: value, Category: category, Color: color, Stroke: stroke, Line: mapLineStyle(authored.Line), Opacity: opacity}}, nil
	default:
		return visualizationir.VisualizationGeographicLayer{}, fmt.Errorf("geographic layer %q has unsupported kind %q", authored.ID, authored.Kind)
	}
}

func compileMapCamera(authored reportdef.VisualGeoCamera) visualizationir.VisualizationMapCamera {
	mode := visualizationir.VisualizationMapCameraMode(authored.Mode)
	if mode == "" {
		mode = visualizationir.VisualizationMapCameraModeFitData
	}
	padding := authored.Padding
	if padding == 0 {
		padding = 32
	}
	maximumZoom := authored.MaximumZoom
	if maximumZoom == 0 {
		maximumZoom = 14
	}
	var center *[]float64
	if len(authored.Center) == 2 {
		value := append([]float64(nil), authored.Center...)
		center = &value
	}
	return visualizationir.VisualizationMapCamera{Mode: mode, Center: center, Zoom: authored.Zoom, Padding: int32(padding), MinimumZoom: authored.MinimumZoom, MaximumZoom: maximumZoom}
}

func compileMapControls(authored reportdef.VisualGeoControls) visualizationir.VisualizationMapControls {
	if !authored.Zoom && !authored.Reset && !authored.Compass {
		return visualizationir.VisualizationMapControls{Zoom: true, Reset: true, Compass: true}
	}
	return visualizationir.VisualizationMapControls{Zoom: authored.Zoom, Reset: authored.Reset, Compass: authored.Compass}
}

func mapLayerPosition(value string) visualizationir.VisualizationMapLayerPosition {
	if value == "above_labels" {
		return visualizationir.VisualizationMapLayerPositionAboveLabels
	}
	return visualizationir.VisualizationMapLayerPositionBelowLabels
}
func mapVisibility(value reportdef.VisualGeoVisibility) visualizationir.VisualizationMapVisibility {
	maximum := value.MaximumZoom
	if maximum == 0 {
		maximum = 24
	}
	return visualizationir.VisualizationMapVisibility{MinimumZoom: value.MinimumZoom, MaximumZoom: maximum}
}
func mapSizeScale(value reportdef.VisualGeoSizeScale) visualizationir.VisualizationMapSizeScale {
	minimum, maximum := value.MinimumRadius, value.MaximumRadius
	if minimum == 0 {
		minimum = 5
	}
	if maximum == 0 {
		maximum = 28
	}
	return visualizationir.VisualizationMapSizeScale{MinimumRadius: minimum, MaximumRadius: maximum, DomainMinimum: value.DomainMinimum, DomainMaximum: value.DomainMaximum}
}
func mapColorScale(value reportdef.VisualGeoColorScale) visualizationir.VisualizationMapColorScale {
	kind := visualizationir.VisualizationMapColorScaleKind(value.Kind)
	if kind == "" {
		kind = visualizationir.VisualizationMapColorScaleKindSequential
	}
	palette := value.Palette
	if palette == "" {
		palette = "blue"
	}
	nullColor := value.NullColor
	if nullColor == "" {
		nullColor = "#d0d7de"
	}
	return visualizationir.VisualizationMapColorScale{Kind: kind, Palette: palette, Reverse: value.Reverse, DomainMinimum: value.DomainMinimum, DomainMidpoint: value.DomainMidpoint, DomainMaximum: value.DomainMaximum, NullColor: nullColor}
}
func mapStroke(value reportdef.VisualGeoStroke) visualizationir.VisualizationMapStroke {
	color, width, opacity := value.Color, value.Width, value.Opacity
	if color == "" {
		color = "#ffffff"
	}
	if width == 0 {
		width = 1.5
	}
	if opacity == 0 {
		opacity = 1
	}
	return visualizationir.VisualizationMapStroke{Color: color, Width: width, Opacity: opacity}
}
func mapCluster(value reportdef.VisualGeoCluster) visualizationir.VisualizationMapCluster {
	radius, maximumZoom, minimumPoints := value.Radius, value.MaximumZoom, value.MinimumPoints
	if radius == 0 {
		radius = 50
	}
	if maximumZoom == 0 {
		maximumZoom = 14
	}
	if minimumPoints == 0 {
		minimumPoints = 2
	}
	return visualizationir.VisualizationMapCluster{Enabled: value.Enabled, Radius: int32(radius), MaximumZoom: int32(maximumZoom), MinimumPoints: int32(minimumPoints), ShowCount: value.ShowCount}
}
func mapHeatStyle(value reportdef.VisualGeoHeatStyle) visualizationir.VisualizationMapHeatStyle {
	radius, intensity := value.Radius, value.Intensity
	if radius == 0 {
		radius = 32
	}
	if intensity == 0 {
		intensity = 1
	}
	return visualizationir.VisualizationMapHeatStyle{Radius: radius, Intensity: intensity}
}
func mapLineStyle(value reportdef.VisualGeoLineStyle) visualizationir.VisualizationMapLineStyle {
	width := value.Width
	if width == 0 {
		width = 3
	}
	return visualizationir.VisualizationMapLineStyle{Width: width, Curvature: value.Curvature}
}

func geographicVisualizationFields(authored reportdef.Visual) []visualizationir.VisualizationField {
	coordinateAliases := map[string]struct{}{}
	for _, layer := range authored.Geo.Layers {
		if layer.Latitude != "" {
			coordinateAliases[layer.Latitude] = struct{}{}
		}
		if layer.Longitude != "" {
			coordinateAliases[layer.Longitude] = struct{}{}
		}
	}
	identity := map[string]bool{}
	for _, mapping := range authored.Interaction.PointSelection.Mappings {
		identity[mapping.Value] = true
	}
	fields := make([]visualizationir.VisualizationField, 0, len(authored.Query.Dimensions)+len(authored.Query.Measures)+1)
	appendField := func(field reportdef.FieldRef, role visualizationir.VisualizationFieldRole, dataType visualizationir.VisualizationDataType) {
		if field.Field == "" {
			return
		}
		alias := field.Alias
		if alias == "" {
			alias = fieldAlias(field.Field)
		}
		if identity[alias] {
			role = visualizationir.VisualizationFieldRoleIdentity
		}
		source := field.Field
		fields = append(fields, visualizationir.VisualizationField{ID: alias, SourceRef: &source, Role: role, DataType: dataType, Nullable: true, Label: alias})
	}
	for _, field := range authored.Query.Dimensions {
		dataType := visualizationir.VisualizationDataTypeString
		alias := field.Alias
		if alias == "" {
			alias = fieldAlias(field.Field)
		}
		if _, ok := coordinateAliases[alias]; ok {
			dataType = visualizationir.VisualizationDataTypeDecimal
		}
		appendField(field, visualizationir.VisualizationFieldRoleDimension, dataType)
	}
	if authored.Query.Time.Field != "" {
		appendField(reportdef.FieldRef{Field: authored.Query.Time.Field, Alias: authored.Query.Time.Alias}, visualizationir.VisualizationFieldRoleDimension, visualizationir.VisualizationDataTypeTemporal)
	}
	for _, field := range authored.Query.Measures {
		appendField(field, visualizationir.VisualizationFieldRoleMeasure, visualizationir.VisualizationDataTypeDecimal)
	}
	return fields
}

func customVisualizationFields(query reportdef.VisualQuery, selection reportdef.SelectionInteraction) []visualizationir.VisualizationField {
	identity := map[string]bool{}
	for _, mapping := range selection.Mappings {
		identity[mapping.Value] = true
	}
	out := []visualizationir.VisualizationField{}
	appendField := func(field reportdef.FieldRef, role visualizationir.VisualizationFieldRole, dataType visualizationir.VisualizationDataType) {
		if field.Field == "" {
			return
		}
		alias := field.Alias
		if alias == "" {
			alias = fieldAlias(field.Field)
		}
		if identity[alias] {
			role = visualizationir.VisualizationFieldRoleIdentity
		}
		source := field.Field
		out = append(out, visualizationir.VisualizationField{ID: alias, SourceRef: &source, Role: role, DataType: dataType, Nullable: true, Label: alias})
	}
	for _, field := range query.Dimensions {
		appendField(field, visualizationir.VisualizationFieldRoleDimension, visualizationir.VisualizationDataTypeString)
	}
	if query.Time.Field != "" {
		appendField(reportdef.FieldRef{Field: query.Time.Field, Alias: query.Time.Alias}, visualizationir.VisualizationFieldRoleDimension, visualizationir.VisualizationDataTypeString)
	}
	appendField(query.Series, visualizationir.VisualizationFieldRoleDimension, visualizationir.VisualizationDataTypeString)
	for _, field := range query.Measures {
		appendField(field, visualizationir.VisualizationFieldRoleMeasure, visualizationir.VisualizationDataTypeDecimal)
	}
	return out
}

func customVisualizationInteractions(selection reportdef.SelectionInteraction) []visualizationir.VisualizationInteraction {
	if selection.IsZero() {
		return []visualizationir.VisualizationInteraction{}
	}
	mappings := make([]visualizationir.VisualizationInteractionMapping, 0, len(selection.Mappings))
	for _, mapping := range selection.Mappings {
		value := visualizationir.VisualizationFieldRef{Dataset: "primary", Field: mapping.Value}
		item := visualizationir.VisualizationInteractionMapping{Source: value, TargetFieldID: mapping.Field}
		if mapping.Fact != "" {
			item.TargetFactID = &mapping.Fact
		}
		if mapping.Grain != "" {
			item.Grain = &mapping.Grain
		}
		if mapping.Label != "" {
			label := visualizationir.VisualizationFieldRef{Dataset: "primary", Field: mapping.Label}
			item.Label = &label
		}
		mappings = append(mappings, item)
	}
	mode := visualizationir.VisualizationSelectionModeSingle
	if selection.Toggle {
		mode = visualizationir.VisualizationSelectionModeMultiple
	}
	return []visualizationir.VisualizationInteraction{{ID: "point_selection", Kind: visualizationir.VisualizationInteractionKindSelect, Mappings: mappings, Targets: append([]string{}, selection.Targets...), Mode: mode, RequiresStableIdentity: true}}
}

func validateCustomProgram(value any, fields map[string]struct{}, path string) error {
	switch value := value.(type) {
	case []any:
		for index, item := range value {
			if err := validateCustomProgram(item, fields, fmt.Sprintf("%s/%d", path, index)); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range value {
			switch key {
			case "url", "href", "expr", "calculate", "transform", "params", "datasets", "values":
				return fmt.Errorf("Vega-Lite property %s/%s is not allowed", path, key)
			case "field":
				field, ok := item.(string)
				if !ok {
					return fmt.Errorf("Vega-Lite field at %s must be a string", path)
				}
				if _, ok := fields[field]; !ok {
					return fmt.Errorf("Vega-Lite field %q is not in the compiled dataset schema", field)
				}
			}
			if err := validateCustomProgram(item, fields, path+"/"+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyCompiledSpecContract(spec *visualizationir.VisualizationSpec, authored reportdef.Visual) {
	base := visualizationSpecBase(spec)
	if base == nil {
		return
	}
	base.DataBudget.MaxRows = compiledVisualLimit(authored)
	if authored.DataBudget.RequiredCompleteness != "" {
		base.DataBudget.RequiredCompleteness = visualizationir.VisualizationCompleteness(authored.DataBudget.RequiredCompleteness)
	}
	if authored.Accessibility.Title != "" {
		base.Accessibility.Title = authored.Accessibility.Title
	}
	if authored.Accessibility.Description != "" {
		base.Accessibility.Description = authored.Accessibility.Description
	}
	if authored.Accessibility.Summary != "" {
		base.Accessibility.Summary = &authored.Accessibility.Summary
	}
	if authored.Accessibility.AnnounceChanges {
		base.Accessibility.AnnounceChanges = &authored.Accessibility.AnnounceChanges
	}
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

func visualizationSpecBase(spec *visualizationir.VisualizationSpec) *visualizationir.VisualizationSpecBase {
	switch value := spec.Value.(type) {
	case *visualizationir.CartesianVisualizationSpec:
		return &value.VisualizationSpecBase
	case *visualizationir.ProportionalVisualizationSpec:
		return &value.VisualizationSpecBase
	case *visualizationir.HierarchyVisualizationSpec:
		return &value.VisualizationSpecBase
	case *visualizationir.PolarVisualizationSpec:
		return &value.VisualizationSpecBase
	case *visualizationir.KPIVisualizationSpec:
		return &value.VisualizationSpecBase
	case *visualizationir.GeographicVisualizationSpec:
		return &value.VisualizationSpecBase
	case *visualizationir.CustomVisualizationSpec:
		return &value.VisualizationSpecBase
	default:
		return nil
	}
}

func compiledTableBinding(modelID string, authored reportdef.TableVisual) visualizationdefinition.QueryBinding {
	binding := visualizationdefinition.QueryBinding{
		ModelID: modelID, DatasetID: "primary", Identity: interactionIdentity(authored.Interaction.RowSelection),
	}
	switch authored.KindOrDefault() {
	case "matrix_table":
		binding.Kind = visualizationdefinition.QueryMatrix
		binding.Matrix = &visualizationdefinition.MatrixQueryBinding{
			TableID: authored.Query.Table, Rows: compiledFields(authored.Query.Rows), Columns: compiledFields(authored.Query.Columns), Measures: compiledFields(authored.Query.Measures), Limit: dashboard.TableInteractiveRowCap,
		}
	case "pivot_table":
		binding.Kind = visualizationdefinition.QueryPivot
		binding.Pivot = &visualizationdefinition.PivotQueryBinding{
			TableID: authored.Query.Table, Rows: compiledFields(authored.Query.Rows), Columns: compiledFields(authored.Query.Columns), Measures: compiledFields(authored.Query.Measures), Limit: dashboard.TableInteractiveRowCap,
		}
	default:
		sort := []visualizationdefinition.Sort{}
		if authored.DefaultSort.Key != "" {
			sort = append(sort, visualizationdefinition.Sort{FieldID: authored.DefaultSort.Key, Direction: authored.DefaultSort.Direction})
		}
		binding.Kind = visualizationdefinition.QueryDetail
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

func compiledInteraction(kind string, selection reportdef.SelectionInteraction) dashboard.InteractionConfig {
	mappings := make([]dashboard.InteractionConfigMapping, len(selection.Mappings))
	for index, mapping := range selection.Mappings {
		mappings[index] = dashboard.InteractionConfigMapping{Field: mapping.Field, Fact: mapping.Fact, Grain: mapping.Grain, Value: mapping.Value, Label: mapping.Label}
	}
	return dashboard.InteractionConfig{Kind: kind, Toggle: selection.Toggle, Mappings: mappings, Targets: append([]string(nil), selection.Targets...)}
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

func interactionIdentity(selection reportdef.SelectionInteraction) []string {
	fields := make([]string, 0, len(selection.Mappings))
	for _, mapping := range selection.Mappings {
		fields = append(fields, mapping.Field)
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
