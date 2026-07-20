package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/libredash/internal/dashboard/definition"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	visualizationdefinition "github.com/Yacobolo/libredash/internal/visualization/definition"
	visualizationgeometry "github.com/Yacobolo/libredash/internal/visualization/geometry"
	visualizationir "github.com/Yacobolo/libredash/internal/visualization/ir"
	visualizationruntime "github.com/Yacobolo/libredash/internal/visualization/runtime"
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
		visual := dashboard.Visual{
			ID: id, Type: authored.Type, Title: compiledVisualTitle(authored, id, model),
			Shape: compiledVisualizationShape(authored), Options: authored.CoreOptions(),
			Interaction: compiledInteraction("point_selection", authored.Interaction.PointSelection),
		}
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
			envelope, err := visualizationruntime.VisualEnvelope(visual, 0, 0)
			if err != nil {
				return nil, fmt.Errorf("visual %q: %w", id, err)
			}
			spec = envelope.Spec
			applyCompiledSpecContract(&spec, authored)
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
		if authored.Type == "custom" {
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
		layer := visualizationir.VisualizationGeographicLayer{ID: authoredLayer.ID, Kind: visualizationir.VisualizationGeographicLayerKind(authoredLayer.Kind)}
		var err error
		if layer.Join, err = fieldRef(layer.ID, "join", authoredLayer.Join); err != nil {
			return visualizationir.VisualizationSpec{}, err
		}
		if layer.Value, err = fieldRef(layer.ID, "value", authoredLayer.Value); err != nil {
			return visualizationir.VisualizationSpec{}, err
		}
		if layer.Latitude, err = fieldRef(layer.ID, "latitude", authoredLayer.Latitude); err != nil {
			return visualizationir.VisualizationSpec{}, err
		}
		if layer.Longitude, err = fieldRef(layer.ID, "longitude", authoredLayer.Longitude); err != nil {
			return visualizationir.VisualizationSpec{}, err
		}
		if authoredLayer.GeometryAsset != "" {
			geometry, err := visualizationgeometry.Resolve(authoredLayer.GeometryAsset)
			if err != nil {
				return visualizationir.VisualizationSpec{}, fmt.Errorf("geographic layer %q: %w", layer.ID, err)
			}
			layer.Geometry = &geometry
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
	return visualizationir.VisualizationSpec{Value: &visualizationir.GeographicVisualizationSpec{
		VisualizationSpecBase: base, Kind: "geographic", Layers: layers,
		Presentation: visualizationir.GeographicVisualizationPresentation{
			VisualizationPresentation: visualizationir.VisualizationPresentation{Legend: visualizationir.VisualizationLegendPosition(authored.Presentation.Legend), ShowLabels: authored.Presentation.ShowLabels},
			Roam:                      authored.Presentation.Roam,
		},
	}}, nil
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
	return 1000
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
