package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/dashboard/reportmodel"
	"github.com/Yacobolo/leapview/internal/dataquery"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

type FilterService struct{}

func (s *FilterService) filterOptions(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, names []string) (map[string][]dashboard.FilterOption, error) {
	options := map[string][]dashboard.FilterOption{}
	names = append([]string{}, names...)
	sort.Strings(names)
	for _, name := range names {
		filter := report.Filters[name]
		if filter.Values.Source != "distinct" {
			continue
		}
		limit := filter.Values.Limit
		if limit <= 0 {
			limit = 200
		}
		if limit > 500 {
			limit = 500
		}
		query := reportdef.AggregateQuery{
			Table:      tableForField(filter.Dimension),
			Dimensions: []reportdef.QueryField{{Field: filter.Dimension, Alias: "value"}},
			Sort:       []reportdef.QuerySort{{Field: "value", Direction: "asc"}},
			Limit:      limit,
		}
		request := reportAggregateDataQuery(report.SemanticModel, query)
		request.Surface = dataquery.SurfaceDashboard
		request.Operation = dataquery.OperationDashboardFilterOptions
		result, err := runtime.data.ExecuteDataQuery(ctx, request)
		if err != nil {
			return nil, err
		}
		rows := reportRowsFromDataQuery(result.Rows)
		values := []dashboard.FilterOption{}
		for _, row := range rows {
			value, ok := row["value"]
			if !ok || value == nil {
				continue
			}
			label := fmt.Sprint(normalizeDBValue(value))
			values = append(values, dashboard.FilterOption{Value: label, Label: label})
		}
		options[name] = values
	}
	return options, nil
}

func (s *FilterService) semanticFilters(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, filters dashboard.Filters, targetKind, targetID string) ([]reportdef.QueryFilter, error) {
	filters = filters.WithDefaults()
	result := []reportdef.QueryFilter{}
	for _, name := range sortedKeys(report.Filters) {
		filter := report.Filters[name]
		control, ok := filters.Controls[name]
		if !ok {
			continue
		}
		applies, err := compiledFilterAppliesToTarget(report, runtime.model, filter, targetKind, targetID)
		if err != nil {
			return nil, err
		}
		if !applies {
			continue
		}
		switch filter.Type {
		case "date_range":
			dateFilters := s.dateSemanticFilters(runtime, filter, control)
			result = append(result, dateFilters...)
		case "multi_select":
			if control.Operator != "in" || len(control.Values) == 0 {
				continue
			}
			values := make([]any, len(control.Values))
			for i, value := range control.Values {
				values[i] = value
			}
			result = append(result, reportdef.QueryFilter{Field: filter.Dimension, Fact: filter.Fact, Operator: "in", Values: values})
		case "text":
			value := strings.TrimSpace(control.Value)
			if value == "" {
				continue
			}
			operator := control.Operator
			if operator == "" {
				operator = filter.DefaultOperator
			}
			result = append(result, reportdef.QueryFilter{Field: filter.Dimension, Fact: filter.Fact, Operator: operator, Values: []any{value}})
		}
	}
	for _, selection := range filters.Selections {
		if selection.SourceKind == "" || selection.SourceID == "" || len(selection.Entries) == 0 {
			continue
		}
		if isUIOnlyRowSelection(selection) {
			continue
		}
		wantInteractionKind := "point_selection"
		if source, ok := report.Visualizations[selection.SourceID]; ok && isGridQuery(source.Query.Kind) && selection.SourceKind == "visual" {
			wantInteractionKind = "row_selection"
		}
		if selection.InteractionKind != wantInteractionKind {
			return nil, fmt.Errorf("selection source %s %q has invalid interaction kind %q", selection.SourceKind, selection.SourceID, selection.InteractionKind)
		}
		resolved, err := reportmodel.ResolveCompiledSelectionInteraction(report, runtime.model, selection.SourceKind, selection.SourceID)
		if err != nil {
			return nil, fmt.Errorf("resolve interaction selection: %w", err)
		}
		if !resolvedSelectionTargets(resolved, targetKind, targetID) {
			continue
		}
		groups := make([]reportdef.QueryFilterGroup, 0, len(selection.Entries))
		for _, entry := range selection.Entries {
			group := reportdef.QueryFilterGroup{}
			canonical, err := canonicalSelectionEntry(resolved, entry)
			if err != nil {
				return nil, fmt.Errorf("selection source %s %q: %w", selection.SourceKind, selection.SourceID, err)
			}
			for index, mapping := range resolved.Mappings {
				mappingFilters, err := selectionMappingFilters(mapping, canonical[index].Value)
				if err != nil {
					return nil, fmt.Errorf("selection source %s %q field %q: %w", selection.SourceKind, selection.SourceID, mapping.Field, err)
				}
				group.Filters = append(group.Filters, mappingFilters...)
			}
			groups = append(groups, group)
		}
		switch len(groups) {
		case 0:
			continue
		case 1:
			result = append(result, groups[0].Filters...)
		default:
			result = append(result, reportdef.QueryFilter{Groups: groups})
		}
	}
	for _, selection := range filters.SpatialSelections {
		if selection.VisualID == "" || selection.InteractionID == "" || selection.Geometry.Value == nil {
			continue
		}
		resolved, err := reportmodel.ResolveCompiledSpatialSelectionInteraction(report, runtime.model, selection.VisualID, selection.InteractionID)
		if err != nil {
			return nil, fmt.Errorf("resolve spatial interaction selection: %w", err)
		}
		if !resolvedSpatialSelectionTargets(resolved, targetKind, targetID) {
			continue
		}
		spatial, err := spatialFilterFromSelection(resolved, selection.Geometry)
		if err != nil {
			return nil, fmt.Errorf("spatial selection source visual %q: %w", selection.VisualID, err)
		}
		result = append(result, reportdef.QueryFilter{Spatial: &spatial})
	}
	return result, nil
}

func (s *FilterService) validateSelections(runtime *modelRuntime, report *dashboarddefinition.Definition, filters dashboard.Filters) error {
	for _, selection := range filters.Selections {
		if selection.SourceKind == "" || selection.SourceID == "" || len(selection.Entries) == 0 || isUIOnlyRowSelection(selection) {
			continue
		}
		wantInteractionKind := "point_selection"
		if source, ok := report.Visualizations[selection.SourceID]; ok && isGridQuery(source.Query.Kind) && selection.SourceKind == "visual" {
			wantInteractionKind = "row_selection"
		}
		if selection.InteractionKind != wantInteractionKind {
			return fmt.Errorf("selection source %s %q has invalid interaction kind %q", selection.SourceKind, selection.SourceID, selection.InteractionKind)
		}
		resolved, err := reportmodel.ResolveCompiledSelectionInteraction(report, runtime.model, selection.SourceKind, selection.SourceID)
		if err != nil {
			return fmt.Errorf("resolve interaction selection: %w", err)
		}
		for _, entry := range selection.Entries {
			if _, err := canonicalSelectionEntry(resolved, entry); err != nil {
				return fmt.Errorf("selection source %s %q: %w", selection.SourceKind, selection.SourceID, err)
			}
		}
	}
	for _, selection := range filters.SpatialSelections {
		if selection.VisualID == "" || selection.InteractionID == "" || selection.Geometry.Value == nil {
			continue
		}
		resolved, err := reportmodel.ResolveCompiledSpatialSelectionInteraction(report, runtime.model, selection.VisualID, selection.InteractionID)
		if err != nil {
			return fmt.Errorf("resolve spatial interaction selection: %w", err)
		}
		if _, err := spatialFilterFromSelection(resolved, selection.Geometry); err != nil {
			return fmt.Errorf("spatial selection source visual %q: %w", selection.VisualID, err)
		}
	}
	return nil
}

func resolvedSelectionTargets(selection reportmodel.ResolvedSelectionInteraction, targetKind, targetID string) bool {
	for _, target := range selection.Targets {
		if target.Kind == targetKind && target.ID == targetID {
			return true
		}
	}
	return false
}

func resolvedSpatialSelectionTargets(selection reportmodel.ResolvedSpatialSelectionInteraction, targetKind, targetID string) bool {
	for _, target := range selection.Targets {
		if target.Kind == targetKind && target.ID == targetID {
			return true
		}
	}
	return false
}

func spatialFilterFromSelection(selection reportmodel.ResolvedSpatialSelectionInteraction, geometry visualizationir.VisualizationSpatialSelectionGeometry) (reportdef.SpatialFilter, error) {
	filter := reportdef.SpatialFilter{
		LatitudeField: selection.Latitude.Field, LongitudeField: selection.Longitude.Field, Fact: selection.Latitude.Fact,
	}
	if selection.Latitude.Fact != selection.Longitude.Fact {
		return reportdef.SpatialFilter{}, fmt.Errorf("coordinate fields must resolve to the same fact")
	}
	switch value := geometry.Value.(type) {
	case *visualizationir.VisualizationSpatialBoxSelection:
		if value == nil {
			break
		}
		filter.Kind, filter.West, filter.South, filter.East, filter.North = "box", value.Bounds.West, value.Bounds.South, value.Bounds.East, value.Bounds.North
	case *visualizationir.VisualizationSpatialLassoSelection:
		if value == nil {
			break
		}
		filter.Kind = "lasso"
		for _, point := range value.Points {
			filter.Points = append(filter.Points, reportdef.SpatialPoint{Longitude: point.Longitude, Latitude: point.Latitude})
		}
	case *visualizationir.VisualizationSpatialRadiusSelection:
		if value == nil {
			break
		}
		filter.Kind, filter.Center, filter.RadiusMeters = "radius", reportdef.SpatialPoint{Longitude: value.Center.Longitude, Latitude: value.Center.Latitude}, value.RadiusMeters
	}
	if filter.Kind == "" {
		return reportdef.SpatialFilter{}, fmt.Errorf("spatial selection geometry is required")
	}
	return filter, nil
}

func canonicalSelectionEntry(selection reportmodel.ResolvedSelectionInteraction, entry dashboard.InteractionSelectionEntry) ([]dashboard.InteractionSelectionMapping, error) {
	identities := make([]reportmodel.SelectionMappingIdentity, len(entry.Mappings))
	incoming := make(map[reportmodel.SelectionMappingIdentity]dashboard.InteractionSelectionMapping, len(entry.Mappings))
	for index, mapping := range entry.Mappings {
		if !mapping.HasValue() {
			return nil, fmt.Errorf("mapping %d must include value", index)
		}
		if !dashboard.IsInteractionSelectionScalar(mapping.Value) {
			return nil, fmt.Errorf("mapping %d value must be a JSON scalar", index)
		}
		identity := reportmodel.SelectionMappingIdentity{Field: mapping.Field, Fact: mapping.Fact, Grain: mapping.Grain}
		identities[index] = identity
		incoming[identity] = mapping
	}
	canonical, err := selection.CanonicalizeMappings(identities)
	if err != nil {
		return nil, err
	}
	result := make([]dashboard.InteractionSelectionMapping, 0, len(canonical))
	for _, mapping := range canonical {
		identity := reportmodel.SelectionMappingIdentity{Field: mapping.Field, Fact: mapping.Fact, Grain: mapping.Grain}
		value := incoming[identity]
		if !dashboard.InteractionSelectionValueMatchesType(value.Value, mapping.Type, mapping.Grain) {
			return nil, fmt.Errorf("mapping field %q value type %T does not match semantic type %q", mapping.Field, value.Value, mapping.Type)
		}
		result = append(result, value)
	}
	return result, nil
}

func selectionMappingFilters(mapping reportmodel.ResolvedSelectionMapping, value dashboard.InteractionSelectionValue) ([]reportdef.QueryFilter, error) {
	if value == nil {
		return []reportdef.QueryFilter{{Field: mapping.Field, Fact: mapping.Fact, Operator: "is_null"}}, nil
	}
	if mapping.Grain == "" {
		return []reportdef.QueryFilter{{Field: mapping.Field, Fact: mapping.Fact, Operator: "equals", Values: []any{value}}}, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("grained time value must be a string, got %T", value)
	}
	start, err := dashboard.ParseInteractionSelectionTime(text, mapping.Grain)
	if err != nil {
		return nil, err
	}
	var end time.Time
	switch mapping.Grain {
	case "day":
		end = start.AddDate(0, 0, 1)
	case "week":
		end = start.AddDate(0, 0, 7)
	case "month":
		end = start.AddDate(0, 1, 0)
	case "quarter":
		end = start.AddDate(0, 3, 0)
	case "year":
		end = start.AddDate(1, 0, 0)
	default:
		return nil, fmt.Errorf("unsupported time grain %q", mapping.Grain)
	}
	return []reportdef.QueryFilter{
		{Field: mapping.Field, Fact: mapping.Fact, Operator: "greater_than_or_equal", Values: []any{start}},
		{Field: mapping.Field, Fact: mapping.Fact, Operator: "less_than", Values: []any{end}},
	}, nil
}

func isUIOnlyRowSelection(selection dashboard.InteractionSelection) bool {
	if selection.SourceKind != "visual" || selection.InteractionKind != "row_selection" || len(selection.Entries) == 0 {
		return false
	}
	for _, entry := range selection.Entries {
		if len(entry.Mappings) != 1 {
			return false
		}
		mapping := entry.Mappings[0]
		if mapping.Field != dashboard.UIRowSelectionField || mapping.Fact != "" || mapping.Grain != "" {
			return false
		}
	}
	return true
}

func (s *FilterService) dateSemanticFilters(runtime *modelRuntime, filter dashboarddefinition.FilterDefinition, control dashboard.FilterControl) []reportdef.QueryFilter {
	if control.From != "" || control.To != "" {
		result := []reportdef.QueryFilter{}
		if control.From != "" {
			result = append(result, reportdef.QueryFilter{Field: filter.Dimension, Fact: filter.Fact, Operator: "greater_than_or_equal", Values: []any{control.From}})
		}
		if control.To != "" {
			result = append(result, reportdef.QueryFilter{Field: filter.Dimension, Fact: filter.Fact, Operator: "less_than", Values: []any{control.To}})
		}
		return result
	}
	if control.Preset == "" || control.Preset == "all" {
		return nil
	}
	for _, preset := range filter.Presets {
		if preset.Value != control.Preset {
			continue
		}
		if preset.From != "" && preset.To != "" {
			return []reportdef.QueryFilter{
				{Field: filter.Dimension, Fact: filter.Fact, Operator: "greater_than_or_equal", Values: []any{preset.From}},
				{Field: filter.Dimension, Fact: filter.Fact, Operator: "less_than", Values: []any{preset.To}},
			}
		}
		if preset.RelativeDays > 0 {
			// The demo relative preset is anchored to the imported order timeline. Leave
			// it unbounded here rather than injecting physical SQL into semantic filters.
			return nil
		}
	}
	return nil
}

func (s *FilterService) countRows(ctx context.Context, runtime *modelRuntime, report *dashboarddefinition.Definition, table string, filters dashboard.Filters, targetKind, targetID string) (int, error) {
	queryFilters, err := s.semanticFilters(ctx, runtime, report, filters, targetKind, targetID)
	if err != nil {
		return 0, err
	}
	total, err := runtime.data.Count(ctx, reportdef.CountQuery{
		Table:   table,
		Filters: queryFilters,
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func tableForField(field string) string {
	if index := strings.IndexByte(field, '.'); index > 0 {
		return field[:index]
	}
	return ""
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func isGridQuery(kind visualizationdefinition.QueryKind) bool {
	return kind == visualizationdefinition.QueryDetail || kind == visualizationdefinition.QueryMatrix || kind == visualizationdefinition.QueryPivot
}

func compiledFilterAppliesToTarget(definition *dashboarddefinition.Definition, model *semanticmodel.Model, filter dashboarddefinition.FilterDefinition, targetKind, targetID string) (bool, error) {
	if !filter.Targets.IsEmpty() && !filter.Targets.Contains(targetKind, targetID) {
		return false, nil
	}
	facts, err := compiledTargetFacts(definition, model, targetID)
	if err != nil {
		return false, err
	}
	if dimension, ok := model.Dimensions[filter.Dimension]; ok {
		for _, fact := range facts {
			if _, ok := dimension.Bindings[fact]; !ok {
				if !filter.Targets.IsEmpty() {
					return false, fmt.Errorf("semantic dimension %q has no binding for fact %q", filter.Dimension, fact)
				}
				return false, nil
			}
		}
		return true, nil
	}
	if len(facts) != 1 {
		if filter.Fact == "" || !contains(facts, filter.Fact) {
			return false, nil
		}
		return model.CanReachField(filter.Fact, filter.Dimension) == nil, nil
	}
	if err := model.CanReachField(facts[0], filter.Dimension); err != nil {
		if !filter.Targets.IsEmpty() {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func compiledTargetFacts(definition *dashboarddefinition.Definition, model *semanticmodel.Model, targetID string) ([]string, error) {
	target, ok := definition.Visualizations[targetID]
	if !ok {
		return nil, fmt.Errorf("unknown visualization %q", targetID)
	}
	table, measures := compiledQueryTableAndMeasures(target.Query)
	if table != "" {
		if _, ok := model.Tables[table]; !ok {
			return nil, fmt.Errorf("query references unknown table %q", table)
		}
		return []string{table}, nil
	}
	facts := map[string]struct{}{}
	visiting := map[string]bool{}
	var addMember func(string) error
	addMember = func(name string) error {
		if measure, ok := model.Measures[name]; ok {
			facts[measure.Fact] = struct{}{}
			return nil
		}
		metric, ok := model.Metrics[name]
		if !ok {
			return fmt.Errorf("unknown measure or metric %q", name)
		}
		if visiting[name] {
			return fmt.Errorf("metric dependency cycle includes %q", name)
		}
		visiting[name] = true
		expression, err := semanticmodel.ParseExpression(metric.Expression)
		if err != nil {
			return err
		}
		for _, reference := range expression.References() {
			if err := addMember(reference); err != nil {
				return err
			}
		}
		delete(visiting, name)
		return nil
	}
	for _, measure := range measures {
		if err := addMember(measure.FieldID); err != nil {
			return nil, err
		}
	}
	result := make([]string, 0, len(facts))
	for fact := range facts {
		result = append(result, fact)
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil, fmt.Errorf("query requires at least one fact")
	}
	return result, nil
}

func compiledQueryTableAndMeasures(query visualizationdefinition.QueryBinding) (string, []visualizationdefinition.FieldBinding) {
	switch query.Kind {
	case visualizationdefinition.QueryAggregate:
		return query.Aggregate.TableID, query.Aggregate.Measures
	case visualizationdefinition.QueryDetail:
		return query.Detail.TableID, nil
	case visualizationdefinition.QueryMatrix:
		return query.Matrix.TableID, query.Matrix.Measures
	case visualizationdefinition.QueryPivot:
		return query.Pivot.TableID, query.Pivot.Measures
	case visualizationdefinition.QueryCustom:
		return query.Custom.TableID, query.Custom.Fields
	case visualizationdefinition.QuerySpatial:
		return query.Spatial.TableID, query.Spatial.Measures
	default:
		return "", nil
	}
}
