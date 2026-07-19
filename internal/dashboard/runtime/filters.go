package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dashboard/reportmodel"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type FilterService struct{}

func (s *FilterService) filterOptions(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, names []string) (map[string][]dashboard.FilterOption, error) {
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

func (s *FilterService) semanticFilters(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, filters dashboard.Filters, targetKind, targetID string) ([]reportdef.QueryFilter, error) {
	filters = filters.WithDefaults()
	result := []reportdef.QueryFilter{}
	for _, name := range sortedKeys(report.Filters) {
		filter := report.Filters[name]
		control, ok := filters.Controls[name]
		if !ok {
			continue
		}
		applies, err := reportmodel.FilterAppliesToTarget(report, runtime.model, filter, targetKind, targetID)
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
		if _, ok := report.Tables[selection.SourceID]; ok && selection.SourceKind == "visual" {
			wantInteractionKind = "row_selection"
		}
		if selection.InteractionKind != wantInteractionKind {
			return nil, fmt.Errorf("selection source %s %q has invalid interaction kind %q", selection.SourceKind, selection.SourceID, selection.InteractionKind)
		}
		resolved, err := reportmodel.ResolveSelectionInteraction(report, runtime.model, selection.SourceKind, selection.SourceID)
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
	return result, nil
}

func resolvedSelectionTargets(selection reportmodel.ResolvedSelectionInteraction, targetKind, targetID string) bool {
	for _, target := range selection.Targets {
		if target.Kind == targetKind && target.ID == targetID {
			return true
		}
	}
	return false
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

func (s *FilterService) dateSemanticFilters(runtime *modelRuntime, filter reportdef.FilterDefinition, control dashboard.FilterControl) []reportdef.QueryFilter {
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

func (s *FilterService) countRows(ctx context.Context, runtime *modelRuntime, report *reportdef.Dashboard, table string, filters dashboard.Filters, targetKind, targetID string) (int, error) {
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
