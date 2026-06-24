package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dashboard/reportmodel"
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
		rows, err := runtime.data.Query(ctx, reportdef.AggregateQuery{
			Table:      tableForField(filter.Dimension),
			Dimensions: []reportdef.QueryField{{Field: filter.Dimension, Alias: "value"}},
			Sort:       []reportdef.QuerySort{{Field: "value", Direction: "asc"}},
			Limit:      limit,
		})
		if err != nil {
			return nil, err
		}
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
			result = append(result, reportdef.QueryFilter{Field: filter.Dimension, Operator: "in", Values: values})
		case "text":
			value := strings.TrimSpace(control.Value)
			if value == "" {
				continue
			}
			operator := control.Operator
			if operator == "" {
				operator = filter.DefaultOperator
			}
			result = append(result, reportdef.QueryFilter{Field: filter.Dimension, Operator: operator, Values: []any{value}})
		}
	}
	for _, selection := range filters.Selections {
		if selection.SourceKind == "" || selection.SourceID == "" || len(selection.Mappings) == 0 {
			continue
		}
		if !targetsInteractionSelection(report, selection, targetKind, targetID) {
			continue
		}
		for _, mapping := range selection.Mappings {
			if len(mapping.Values) == 0 {
				continue
			}
			dimension, err := runtime.model.ResolveDimension(mapping.Field)
			if err != nil {
				continue
			}
			values := make([]any, len(mapping.Values))
			for i, value := range mapping.Values {
				values[i] = value
			}
			result = append(result, reportdef.QueryFilter{Field: dimension.Field, Operator: "in", Values: values})
		}
	}
	return result, nil
}

func (s *FilterService) dateSemanticFilters(runtime *modelRuntime, filter reportdef.FilterDefinition, control dashboard.FilterControl) []reportdef.QueryFilter {
	if control.From != "" || control.To != "" {
		result := []reportdef.QueryFilter{}
		if control.From != "" {
			result = append(result, reportdef.QueryFilter{Field: filter.Dimension, Operator: "greater_than_or_equal", Values: []any{control.From}})
		}
		if control.To != "" {
			result = append(result, reportdef.QueryFilter{Field: filter.Dimension, Operator: "less_than", Values: []any{control.To}})
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
				{Field: filter.Dimension, Operator: "greater_than_or_equal", Values: []any{preset.From}},
				{Field: filter.Dimension, Operator: "less_than", Values: []any{preset.To}},
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

func targetsInteractionSelection(report *reportdef.Dashboard, selection dashboard.InteractionSelection, targetKind, targetID string) bool {
	switch selection.SourceKind {
	case "visual":
		visual, ok := report.Visuals[selection.SourceID]
		if !ok || selection.InteractionKind != "point_selection" {
			return false
		}
		return contains(visual.Interaction.PointSelection.Targets, targetID)
	case "table":
		table, ok := report.Tables[selection.SourceID]
		if !ok || selection.InteractionKind != "row_selection" {
			return false
		}
		return contains(table.Interaction.RowSelection.Targets, targetID)
	default:
		return false
	}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
