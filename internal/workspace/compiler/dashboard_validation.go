package compiler

import (
	"fmt"
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dashboard/reportmodel"
)

func ValidateDashboard(d *report.Dashboard, models map[string]*semanticmodel.Model) error {
	if err := d.ValidateContract(); err != nil {
		return err
	}
	model, ok := models[d.SemanticModel]
	if !ok {
		return fmt.Errorf("dashboard %q references unknown semantic model %q", d.ID, d.SemanticModel)
	}
	for name, filter := range d.Filters {
		if _, err := model.ResolveDimension(filter.Dimension); err != nil {
			return fmt.Errorf("filter %q references unknown dimension %q", name, filter.Dimension)
		}
	}
	for name, visual := range d.Visuals {
		for _, dimension := range visual.Query.Dimensions {
			if _, err := model.ResolveDimension(dimension.Field); err != nil {
				return fmt.Errorf("visual %q references unknown dimension %q", name, dimension.Field)
			}
		}
		if !visual.Query.Series.IsZero() {
			if _, err := model.ResolveDimension(visual.Query.Series.Field); err != nil {
				return fmt.Errorf("visual %q references unknown series dimension %q", name, visual.Query.Series.Field)
			}
		}
		for _, measure := range visual.Query.Measures {
			if measure.Measure.Expression != "" || measure.Measure.Expr != "" {
				if _, ok := model.Tables[measure.Measure.Table]; !ok {
					return fmt.Errorf("visual %q inline measure %q references unknown table %q", name, measure.Alias, measure.Measure.Table)
				}
				continue
			}
			if _, err := model.ResolveMeasure(measure.Field); err != nil {
				return fmt.Errorf("visual %q references unknown measure %q", name, measure.Field)
			}
		}
		for _, mapping := range visual.Interaction.PointSelection.Mappings {
			if _, err := model.ResolveDimension(mapping.Field); err != nil {
				return fmt.Errorf("visual %q interaction references unknown field %q", name, mapping.Field)
			}
		}
	}
	for name, table := range d.Tables {
		normalizeTableFormatting(model, &table)
		for measure := range table.MeasureFormatting {
			if _, err := model.ResolveMeasure(measure); err != nil {
				return fmt.Errorf("table %q measure_formatting references unknown measure %q", name, measure)
			}
		}
		switch table.KindOrDefault() {
		case "data_table":
			if err := normalizeDataTableFields(name, model, &table); err != nil {
				return err
			}
		case "matrix_table", "pivot_table":
			if err := normalizeTableFields(name, model, &table); err != nil {
				return err
			}
		}
		for _, mapping := range table.Interaction.RowSelection.Mappings {
			if _, err := model.ResolveDimension(mapping.Field); err != nil {
				return fmt.Errorf("table %q interaction references unknown field %q", name, mapping.Field)
			}
			if !tableHasOutputColumn(table, mapping.Value) {
				return fmt.Errorf("table %q interaction references unknown value column %q", name, mapping.Value)
			}
			if mapping.Label != "" && !tableHasOutputColumn(table, mapping.Label) {
				return fmt.Errorf("table %q interaction references unknown label column %q", name, mapping.Label)
			}
		}
		d.Tables[name] = table
	}
	return validateFilterTargets(d, model)
}

func tableHasOutputColumn(table report.TableVisual, key string) bool {
	if key == "" {
		return false
	}
	for _, column := range table.DataColumns {
		if column.Alias == key {
			return true
		}
	}
	for _, column := range table.Columns {
		if column.Key == key {
			return true
		}
	}
	for _, row := range table.Rows {
		if displayField(row) == key {
			return true
		}
	}
	for _, measure := range table.Measures {
		if displayField(measure) == key {
			return true
		}
	}
	return false
}

func displayField(field string) string {
	if index := strings.LastIndex(field, "."); index >= 0 && index+1 < len(field) {
		return field[index+1:]
	}
	return field
}

func normalizeTableFields(name string, model *semanticmodel.Model, table *report.TableVisual) error {
	table.Rows = make([]string, len(table.Query.Rows))
	for index, dimension := range table.Query.Rows {
		item, err := model.ResolveDimension(dimension.Field)
		if err != nil {
			return fmt.Errorf("table %q query.rows references unknown dimension %q", name, dimension.Field)
		}
		table.Rows[index] = item.Field
	}
	table.ColumnDims = make([]string, len(table.Query.Columns))
	for index, dimension := range table.Query.Columns {
		item, err := model.ResolveDimension(dimension.Field)
		if err != nil {
			return fmt.Errorf("table %q query.columns references unknown dimension %q", name, dimension.Field)
		}
		table.ColumnDims[index] = item.Field
	}
	table.Measures = make([]string, len(table.Query.Measures))
	for index, measure := range table.Query.Measures {
		item, err := model.ResolveMeasure(measure.Field)
		if err != nil {
			return fmt.Errorf("table %q query.measures references unknown measure %q", name, measure.Field)
		}
		table.Measures[index] = item.Field
	}
	return nil
}

func normalizeDataTableFields(name string, model *semanticmodel.Model, table *report.TableVisual) error {
	columns := make([]report.FieldRef, 0, len(table.Query.Columns)+len(table.Query.Fields))
	if len(table.Query.Columns) > 0 {
		columns = append(columns, table.Query.Columns...)
	} else {
		for _, field := range table.Query.Fields {
			columns = append(columns, report.FieldRef{Field: field, Alias: fieldRefAlias(field)})
		}
	}
	if len(columns) == 0 {
		return fmt.Errorf("table %q kind data_table requires query.fields or query.columns", name)
	}
	seenAliases := map[string]struct{}{}
	for index, column := range columns {
		if column.Alias == "" {
			column.Alias = fieldRefAlias(column.Field)
		}
		if _, exists := seenAliases[column.Alias]; exists {
			return fmt.Errorf("table %q has duplicate query output alias %q", name, column.Alias)
		}
		seenAliases[column.Alias] = struct{}{}
		if dimension, err := model.ResolveDimension(column.Field); err == nil {
			column.Field = dimension.Field
			columns[index] = column
			continue
		}
		measure, err := model.ResolveMeasure(column.Field)
		if err != nil {
			return fmt.Errorf("table %q query column %q references unknown field %q", name, column.Alias, column.Field)
		}
		column.Field = measure.Field
		columns[index] = column
	}
	table.DataColumns = columns
	if len(table.Columns) == 0 {
		table.Columns = make([]dashboard.TableColumn, 0, len(columns))
		for _, column := range columns {
			format := "text"
			role := ""
			align := ""
			if measure, err := model.ResolveMeasure(column.Field); err == nil {
				role = "measure"
				align = "right"
				if measure.Format != "" {
					format = measure.Format
				} else {
					format = "decimal"
				}
			}
			table.Columns = append(table.Columns, dashboard.TableColumn{
				Key:    column.Alias,
				Label:  titleFromIdentifier(column.Alias),
				Format: format,
				Role:   role,
				Align:  align,
			})
		}
		return nil
	}
	for _, column := range table.Columns {
		if !tableHasQueryAlias(table.DataColumns, column.Key) {
			return fmt.Errorf("table %q column %q has no matching query column alias", name, column.Key)
		}
	}
	return nil
}

func normalizeTableFormatting(model *semanticmodel.Model, table *report.TableVisual) {
	if len(table.MeasureFormatting) == 0 {
		return
	}
	next := map[string][]dashboard.TableFormattingRule{}
	for measure, rules := range table.MeasureFormatting {
		field := measure
		if resolved, err := model.ResolveMeasure(measure); err == nil {
			field = resolved.Name
		}
		next[field] = rules
	}
	table.MeasureFormatting = next
}

func validateFilterTargets(d *report.Dashboard, model *semanticmodel.Model) error {
	for name, filter := range d.Filters {
		for _, target := range filter.Targets.Visuals {
			ok, err := reportmodel.FilterAppliesToTarget(d, model, filter, "visual", target)
			if err != nil || !ok {
				if err == nil {
					err = fmt.Errorf("filter field %q is not reachable", filter.Dimension)
				}
				return fmt.Errorf("filter %q cannot apply to visual %q: %w", name, target, err)
			}
		}
		for _, target := range filter.Targets.Tables {
			ok, err := reportmodel.FilterAppliesToTarget(d, model, filter, "table", target)
			if err != nil || !ok {
				if err == nil {
					err = fmt.Errorf("filter field %q is not reachable", filter.Dimension)
				}
				return fmt.Errorf("filter %q cannot apply to table %q: %w", name, target, err)
			}
		}
	}
	return nil
}

func tableHasQueryAlias(columns []report.FieldRef, alias string) bool {
	for _, column := range columns {
		if column.Alias == alias {
			return true
		}
	}
	return false
}

func fieldRefAlias(field string) string {
	if field == "" {
		return ""
	}
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
}

func titleFromIdentifier(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
