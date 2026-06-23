package semantic

import (
	"fmt"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

func validateTableStyle(name string, style dashboard.TableStyle) error {
	switch style.Density {
	case "", "compact", "comfortable", "spacious":
	default:
		return fmt.Errorf("table %q has unsupported density %q", name, style.Density)
	}
	switch style.Grid {
	case "", "none", "rows", "columns", "full":
	default:
		return fmt.Errorf("table %q has unsupported grid %q", name, style.Grid)
	}
	return nil
}

func validateTableColumn(tableName string, column dashboard.TableColumn) error {
	switch column.Format {
	case "", "text", "integer", "decimal", "currency", "days":
	default:
		return fmt.Errorf("table %q column %q has unsupported format %q", tableName, column.Key, column.Format)
	}
	for _, rule := range column.Formatting {
		if err := validateTableFormattingRule(tableName, column.Key, rule); err != nil {
			return err
		}
	}
	return nil
}

func normalizeTableFields(name string, model *Model, table *TableVisual) error {
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

func normalizeDataTableFields(name string, model *Model, table *TableVisual) error {
	columns := make([]FieldRef, 0, len(table.Query.Columns)+len(table.Query.Fields))
	if len(table.Query.Columns) > 0 {
		columns = append(columns, table.Query.Columns...)
	} else {
		for _, field := range table.Query.Fields {
			columns = append(columns, FieldRef{Field: field, Alias: fieldRefAlias(field)})
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

func tableHasQueryAlias(columns []FieldRef, alias string) bool {
	for _, column := range columns {
		if column.Alias == alias {
			return true
		}
	}
	return false
}

func normalizeTableFormatting(model *Model, table *TableVisual) {
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

func validateTableFormattingRule(tableName, field string, rule dashboard.TableFormattingRule) error {
	switch rule.Kind {
	case "badge", "text_color", "background_scale", "data_bar":
	default:
		return fmt.Errorf("table %q column %q has unsupported formatting kind %q", tableName, field, rule.Kind)
	}
	return nil
}

func validatePlacement(page dashboard.Page, visual dashboard.PageVisual) error {
	placement := visual.Placement
	if placement.IsZero() {
		return fmt.Errorf("page %q visual %q requires placement", page.ID, visual.ID)
	}
	if placement.Col <= 0 || placement.Row <= 0 || placement.ColSpan <= 0 || placement.RowSpan <= 0 {
		return fmt.Errorf("page %q visual %q has invalid placement", page.ID, visual.ID)
	}
	if placement.Col+placement.ColSpan-1 > page.Grid.Columns {
		return fmt.Errorf("page %q visual %q placement exceeds %d grid columns", page.ID, visual.ID, page.Grid.Columns)
	}
	return nil
}
