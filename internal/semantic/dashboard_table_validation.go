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

func normalizeTableFields(name string, view *MetricView, table *TableVisual) error {
	table.Rows = make([]string, len(table.Query.Rows))
	for index, dimension := range table.Query.Rows {
		field, _, err := view.ResolveDimensionRef(dimension.Field)
		if err != nil {
			return fmt.Errorf("table %q query.rows references unknown dimension %q", name, dimension.Field)
		}
		table.Rows[index] = field
	}
	table.ColumnDims = make([]string, len(table.Query.Columns))
	for index, dimension := range table.Query.Columns {
		field, _, err := view.ResolveDimensionRef(dimension.Field)
		if err != nil {
			return fmt.Errorf("table %q query.columns references unknown dimension %q", name, dimension.Field)
		}
		table.ColumnDims[index] = field
	}
	table.Measures = make([]string, len(table.Query.Measures))
	for index, measure := range table.Query.Measures {
		field, _, err := view.ResolveMeasureRef(measure.Field)
		if err != nil {
			return fmt.Errorf("table %q query.measures references unknown measure %q", name, measure.Field)
		}
		table.Measures[index] = field
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

func normalizeTableFormatting(view *MetricView, table *TableVisual) {
	if len(table.MeasureFormatting) == 0 {
		return
	}
	next := map[string][]dashboard.TableFormattingRule{}
	for measure, rules := range table.MeasureFormatting {
		field := measure
		if resolved, _, err := view.ResolveMeasureRef(measure); err == nil {
			field = resolved
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
