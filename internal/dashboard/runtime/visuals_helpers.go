package runtime

import (
	"fmt"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
)

func fieldRef(field string, alias string) reportdef.QueryField {
	return reportdef.QueryField{Field: field, Alias: alias}
}

func queryFieldRef(ref reportdef.FieldRef, alias string) reportdef.QueryField {
	return reportdef.QueryField{
		Field:   ref.Field,
		Alias:   alias,
		Measure: queryInlineMeasure(ref.Measure),
	}
}

func queryInlineMeasure(measure semanticmodel.MetricMeasure) reportdef.InlineMeasure {
	return reportdef.InlineMeasure{
		Field:       measure.Field,
		Name:        measure.Name,
		Label:       measure.Label,
		Description: measure.Description,
		Expr:        measure.Expr,
		Expression:  measure.Expression,
		Table:       measure.Table,
		Grain:       measure.Grain,
		Time:        measure.Time,
		Grains:      append([]string{}, measure.Grains...),
		Unit:        measure.Unit,
		Format:      measure.Format,
	}
}

func queryDimensionFields(dimensions []reportdef.FieldRef) []string {
	fields := make([]string, len(dimensions))
	for i, dimension := range dimensions {
		fields[i] = dimension.Field
	}
	return fields
}

func queryMeasureFields(measures []reportdef.FieldRef) []string {
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

func visualSorts(visual reportdef.Visual) []reportdef.QuerySort {
	if len(visual.Query.Sort) == 0 {
		return []reportdef.QuerySort{{Field: defaultSortColumn(visual), Direction: "asc"}}
	}
	sorts := make([]reportdef.QuerySort, 0, len(visual.Query.Sort))
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
		sorts = append(sorts, reportdef.QuerySort{Field: field, Direction: sort.Direction})
	}
	return sorts
}

func measureLabel(name string, measure semanticmodel.MetricMeasure) string {
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

func distributionSorts(visual reportdef.Visual) []reportdef.QuerySort {
	if len(visual.Query.Sort) == 0 {
		return nil
	}
	sorts := make([]reportdef.QuerySort, 0, len(visual.Query.Sort))
	for _, sortSpec := range visual.Query.Sort {
		field := sortSpec.Field
		if field == "" {
			field = "label"
		}
		if field != "label" && field != "min" && field != "q1" && field != "median" && field != "q3" && field != "max" {
			field = "label"
		}
		sorts = append(sorts, reportdef.QuerySort{Field: field, Direction: sortSpec.Direction})
	}
	return sorts
}

func defaultSortColumn(visual reportdef.Visual) string {
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

func visualInteractionConfig(selection reportdef.SelectionInteraction) dashboard.InteractionConfig {
	return interactionConfig("point_selection", selection)
}

func tableInteractionConfig(selection reportdef.SelectionInteraction) dashboard.InteractionConfig {
	return interactionConfig("row_selection", selection)
}

func interactionConfig(kind string, selection reportdef.SelectionInteraction) dashboard.InteractionConfig {
	mappings := make([]dashboard.InteractionConfigMapping, 0, len(selection.Mappings))
	for _, mapping := range selection.Mappings {
		mappings = append(mappings, dashboard.InteractionConfigMapping{
			Field: mapping.Field,
			Value: mapping.Value,
			Label: mapping.Label,
		})
	}
	return dashboard.InteractionConfig{
		Kind:     kind,
		Toggle:   selection.Toggle,
		Mappings: mappings,
		Targets:  append([]string{}, selection.Targets...),
	}
}

func selectedEntries(filters dashboard.Filters, sourceKind, sourceID string) []dashboard.InteractionSelectionEntry {
	entries := []dashboard.InteractionSelectionEntry{}
	for _, selection := range filters.Selections {
		if selection.SourceKind != sourceKind || selection.SourceID != sourceID {
			continue
		}
		for _, entry := range selection.Entries {
			entries = append(entries, copySelectionEntry(entry))
		}
	}
	return entries
}

func copySelectionEntry(entry dashboard.InteractionSelectionEntry) dashboard.InteractionSelectionEntry {
	next := dashboard.InteractionSelectionEntry{
		Label:    entry.Label,
		Mappings: make([]dashboard.InteractionSelectionMapping, len(entry.Mappings)),
	}
	copy(next.Mappings, entry.Mappings)
	return next
}

func markSelected(data []dashboard.Datum, selection reportdef.SelectionInteraction, entries []dashboard.InteractionSelectionEntry) {
	if len(data) == 0 || len(selection.Mappings) == 0 || len(entries) == 0 {
		return
	}
	for _, row := range data {
		if datumMatchesAnySelectionEntry(row, selection.Mappings, entries) {
			row["selected"] = true
		}
	}
}

func datumMatchesAnySelectionEntry(row dashboard.Datum, mappings []reportdef.SelectionMapping, entries []dashboard.InteractionSelectionEntry) bool {
	for _, entry := range entries {
		if datumMatchesSelectionEntry(row, mappings, entry) {
			return true
		}
	}
	return false
}

func datumMatchesSelectionEntry(row dashboard.Datum, mappings []reportdef.SelectionMapping, entry dashboard.InteractionSelectionEntry) bool {
	if len(entry.Mappings) == 0 {
		return false
	}
	for _, mapping := range mappings {
		selectedValue, ok := selectionEntryMappingValue(entry, mapping.Field)
		if !ok || selectedValue == "" {
			return false
		}
		value, ok := row[mapping.Value]
		if !ok || fmt.Sprint(value) != selectedValue {
			return false
		}
	}
	return true
}

func selectionEntryMappingValue(entry dashboard.InteractionSelectionEntry, field string) (string, bool) {
	for _, mapping := range entry.Mappings {
		if mapping.Field == field {
			return mapping.Value, true
		}
	}
	return "", false
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
