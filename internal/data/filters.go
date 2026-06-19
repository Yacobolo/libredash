package data

import (
	"context"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	semanticquery "github.com/Yacobolo/libredash/internal/query"
	"github.com/Yacobolo/libredash/internal/semantic"
)

func (m *DuckDBMetrics) filterOptions(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, names []string) (map[string][]dashboard.FilterOption, error) {
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
		plan, err := semanticquery.NewPlanner(runtime.model, m.workspace.MetricViews).Plan(semanticquery.Request{
			MetricView: filter.MetricView,
			Dimensions: []semanticquery.Field{{Field: filter.Dimension, Alias: "value"}},
			Sort:       []semanticquery.Sort{{Field: "value", Direction: "asc"}},
			Limit:      limit,
		})
		if err != nil {
			return nil, err
		}
		rows, err := runtime.db.QueryContext(ctx, plan.SQL, plan.Args...)
		if err != nil {
			return nil, err
		}
		values := []dashboard.FilterOption{}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return nil, err
			}
			values = append(values, dashboard.FilterOption{Value: value, Label: value})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		options[name] = values
	}
	return options, nil
}

func (m *DuckDBMetrics) semanticFilters(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, metricViewID string, filters dashboard.Filters, targetKind, targetID string) ([]semanticquery.Filter, error) {
	filters = filters.WithDefaults()
	result := []semanticquery.Filter{}
	for _, name := range sortedKeys(report.Filters) {
		filter := report.Filters[name]
		if filter.MetricView != metricViewID {
			continue
		}
		control, ok := filters.Controls[name]
		if !ok {
			continue
		}
		switch filter.Type {
		case "date_range":
			dateFilters := m.dateSemanticFilters(runtime, filter, control)
			result = append(result, dateFilters...)
		case "multi_select":
			if control.Operator != "in" || len(control.Values) == 0 {
				continue
			}
			values := make([]any, len(control.Values))
			for i, value := range control.Values {
				values[i] = value
			}
			result = append(result, semanticquery.Filter{Field: filter.Dimension, Operator: "in", Values: values})
		case "text":
			value := strings.TrimSpace(control.Value)
			if value == "" {
				continue
			}
			operator := control.Operator
			if operator == "" {
				operator = filter.DefaultOperator
			}
			result = append(result, semanticquery.Filter{Field: filter.Dimension, Operator: operator, Values: []any{value}})
		}
	}
	for _, selection := range filters.VisualSelections {
		if selection.VisualID == "" || len(selection.Values) == 0 {
			continue
		}
		if targetKind == "visual" && selection.VisualID == targetID {
			continue
		}
		sourceVisual, ok := report.Visuals[selection.VisualID]
		if !ok || !targetsSelection(sourceVisual.Interaction.Targets, targetKind, targetID) {
			continue
		}
		if selection.Operator != "" && selection.Operator != "in" {
			continue
		}
		metricView, ok := m.workspace.MetricViews[metricViewID]
		if !ok {
			continue
		}
		field, _, err := metricView.ResolveDimensionRef(selection.Field)
		if err != nil {
			continue
		}
		values := make([]any, len(selection.Values))
		for i, value := range selection.Values {
			values[i] = value
		}
		result = append(result, semanticquery.Filter{Field: field, Operator: "in", Values: values})
	}
	return result, nil
}

func (m *DuckDBMetrics) dateSemanticFilters(runtime *modelRuntime, filter semantic.FilterDefinition, control dashboard.FilterControl) []semanticquery.Filter {
	if control.From != "" || control.To != "" {
		result := []semanticquery.Filter{}
		if control.From != "" {
			result = append(result, semanticquery.Filter{Field: filter.Dimension, Operator: "greater_than_or_equal", Values: []any{control.From}})
		}
		if control.To != "" {
			result = append(result, semanticquery.Filter{Field: filter.Dimension, Operator: "less_than", Values: []any{control.To}})
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
			return []semanticquery.Filter{
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

func (m *DuckDBMetrics) countRows(ctx context.Context, runtime *modelRuntime, report *semantic.Dashboard, metricViewID string, filters dashboard.Filters, targetKind, targetID string) (int, error) {
	queryFilters, err := m.semanticFilters(ctx, runtime, report, metricViewID, filters, targetKind, targetID)
	if err != nil {
		return 0, err
	}
	plan, err := semanticquery.NewPlanner(runtime.model, m.workspace.MetricViews).PlanCount(semanticquery.CountRequest{
		MetricView: metricViewID,
		Filters:    queryFilters,
	})
	if err != nil {
		return 0, err
	}

	var total int
	if err := runtime.db.QueryRowContext(ctx, plan.SQL, plan.Args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func targetsSelection(targets semantic.InteractionTargets, targetKind, targetID string) bool {
	switch targetKind {
	case "visual":
		return contains(targets.Visuals, targetID)
	case "table":
		return contains(targets.Tables, targetID)
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
