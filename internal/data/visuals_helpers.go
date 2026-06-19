package data

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	semanticquery "github.com/Yacobolo/libredash/internal/query"
	"github.com/Yacobolo/libredash/internal/semantic"
)

func visualSemanticFilters(ctx context.Context, m *DuckDBMetrics, runtime *modelRuntime, report *semantic.Dashboard, visual semantic.Visual, filters dashboard.Filters, visualID string) ([]semanticquery.Filter, error) {
	return m.semanticFilters(ctx, runtime, report, visual.MetricView, filters, "visual", visualID)
}

func fieldRef(field string, alias string) semanticquery.Field {
	return semanticquery.Field{Field: field, Alias: alias}
}

func queryDimensionFields(dimensions []semantic.FieldRef) []string {
	fields := make([]string, len(dimensions))
	for i, dimension := range dimensions {
		fields[i] = dimension.Field
	}
	return fields
}

func queryMeasureFields(measures []semantic.FieldRef) []string {
	fields := make([]string, len(measures))
	for i, measure := range measures {
		fields[i] = measure.Field
	}
	return fields
}

func displayFields(fields []string) []string {
	values := make([]string, len(fields))
	for i, field := range fields {
		values[i] = displayField(field)
	}
	return values
}

func displayField(field string) string {
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
}

func visualSorts(visual semantic.Visual) []semanticquery.Sort {
	if len(visual.Query.Sort) == 0 {
		return []semanticquery.Sort{{Field: defaultSortColumn(visual), Direction: "asc"}}
	}
	sorts := make([]semanticquery.Sort, 0, len(visual.Query.Sort))
	for _, sort := range visual.Query.Sort {
		field := sort.Field
		if field == "" {
			field = defaultSortColumn(visual)
		}
		if field != "value" && field != "series" {
			for index, dimension := range visual.Query.Dimensions {
				if field == dimension.Field || field == dimension.Alias || field == displayField(dimension.Field) {
					field = dimensionSortColumn(visual.ShapeOrDefault(), index)
					break
				}
			}
			if !visual.Query.Series.IsZero() && (field == visual.Query.Series.Field || field == visual.Query.Series.Alias || field == displayField(visual.Query.Series.Field)) {
				field = "series"
			}
		}
		sorts = append(sorts, semanticquery.Sort{Field: field, Direction: sort.Direction})
	}
	return sorts
}

func measureLabel(name string, measure semantic.MetricMeasure) string {
	if strings.TrimSpace(measure.Label) != "" {
		return measure.Label
	}
	return name
}

func optionInt(options map[string]any, key string, fallback, minValue, maxValue int) int {
	if options == nil {
		return fallback
	}
	var value int
	switch typed := options[key].(type) {
	case int:
		value = typed
	case int64:
		value = int(typed)
	case float64:
		value = int(typed)
	case string:
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return fallback
		}
		value = parsed
	default:
		return fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func datumFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		parsed, _ := strconv.ParseFloat(typed, 64)
		return parsed
	default:
		return 0
	}
}

func formatBinLabel(start, end float64) string {
	if math.Abs(start-end) < 0.000001 {
		return strconv.FormatFloat(round(start), 'f', -1, 64)
	}
	return fmt.Sprintf("%s-%s", strconv.FormatFloat(round(start), 'f', -1, 64), strconv.FormatFloat(round(end), 'f', -1, 64))
}

func distributionOrderBy(visual semantic.Visual) string {
	if len(visual.Query.Sort) == 0 {
		return "label ASC"
	}
	parts := make([]string, 0, len(visual.Query.Sort))
	for _, sortSpec := range visual.Query.Sort {
		field := sortSpec.Field
		if field == "" {
			field = "label"
		}
		if field != "label" && field != "min" && field != "q1" && field != "median" && field != "q3" && field != "max" {
			field = "label"
		}
		direction := "ASC"
		if strings.EqualFold(sortSpec.Direction, "desc") {
			direction = "DESC"
		}
		parts = append(parts, field+" "+direction)
	}
	return strings.Join(parts, ", ")
}

func defaultSortColumn(visual semantic.Visual) string {
	switch visual.ShapeOrDefault() {
	case "matrix":
		return "row"
	case "graph":
		return "source"
	case "geo":
		return "name"
	case "hierarchy":
		return "value"
	default:
		return "label"
	}
}

func dimensionSortColumn(shape string, index int) string {
	switch shape {
	case "matrix":
		if index == 1 {
			return "chart_column"
		}
		return "row"
	case "graph":
		if index == 1 {
			return "target"
		}
		return "source"
	case "geo":
		return "name"
	case "hierarchy":
		return fmt.Sprintf("level_%d", index)
	default:
		return "label"
	}
}

func selectedValues(filters dashboard.Filters, visualID string) []string {
	for _, selection := range filters.VisualSelections {
		if selection.VisualID == visualID {
			values := make([]string, len(selection.Values))
			copy(values, selection.Values)
			return values
		}
	}
	return []string{}
}

func markSelected(data []dashboard.Datum, key string, values []string) {
	if len(values) == 0 {
		return
	}
	selected := make(map[string]struct{}, len(values))
	for _, value := range values {
		selected[value] = struct{}{}
	}
	for _, row := range data {
		value, ok := row[key]
		if !ok {
			continue
		}
		if _, ok := selected[fmt.Sprint(value)]; ok {
			row["selected"] = true
		}
	}
}

func normalizeDatumValue(value any) any {
	switch typed := normalizeDBValue(value).(type) {
	case float64:
		return round(typed)
	case float32:
		return round(float64(typed))
	default:
		return typed
	}
}

func normalizeDBValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format("2006-01-02")
	case float32:
		return round(float64(typed))
	case float64:
		return round(typed)
	default:
		return typed
	}
}

func formatMetric(value float64, format string) string {
	switch format {
	case "currency":
		return formatCurrency(value)
	case "integer":
		return formatInt(int64(math.Round(value)))
	case "decimal":
		return fmt.Sprintf("%.2f", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func formatCurrency(value float64) string {
	if value >= 1000000 {
		return fmt.Sprintf("R$ %.1fm", value/1000000)
	}
	if value >= 1000 {
		return fmt.Sprintf("R$ %.1fk", value/1000)
	}
	return fmt.Sprintf("R$ %.0f", value)
}

func formatInt(value int64) string {
	if value >= 1000000 {
		return fmt.Sprintf("%.1fm", float64(value)/1000000)
	}
	if value >= 1000 {
		return fmt.Sprintf("%.1fk", float64(value)/1000)
	}
	return fmt.Sprintf("%d", value)
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}
