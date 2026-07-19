package report

import (
	"fmt"
	"strings"
)

func (d *Dashboard) ValidateContract() error {
	return d.validateContract()
}

func (d *Dashboard) validateContract() error {
	if d.ID == "" || d.Title == "" {
		return fmt.Errorf("dashboard requires id and title")
	}
	if d.SemanticModel == "" {
		return fmt.Errorf("dashboard %q requires semantic_model", d.ID)
	}
	if len(d.Visuals) == 0 {
		return fmt.Errorf("dashboard %q requires visuals", d.ID)
	}
	if len(d.Pages) == 0 {
		return fmt.Errorf("dashboard %q requires pages", d.ID)
	}
	for name, filter := range d.Filters {
		if filter.Type == "" || filter.Label == "" || filter.Dimension == "" {
			return fmt.Errorf("filter %q requires type, label, and field", name)
		}
		switch filter.Type {
		case "date_range":
			if filter.URLParam == "" || filter.FromURLParam == "" || filter.ToURLParam == "" {
				return fmt.Errorf("filter %q date_range requires url_param, from_url_param, and to_url_param", name)
			}
			if len(filter.Presets) == 0 {
				return fmt.Errorf("filter %q date_range requires presets", name)
			}
			presets := map[string]struct{}{}
			for _, preset := range filter.Presets {
				if preset.Value == "" || preset.Label == "" {
					return fmt.Errorf("filter %q date_range preset requires value and label", name)
				}
				if _, exists := presets[preset.Value]; exists {
					return fmt.Errorf("filter %q date_range has duplicate preset %q", name, preset.Value)
				}
				presets[preset.Value] = struct{}{}
				if (preset.From == "") != (preset.To == "") {
					return fmt.Errorf("filter %q date_range preset %q requires both from and to", name, preset.Value)
				}
				if preset.RelativeDays < 0 {
					return fmt.Errorf("filter %q date_range preset %q has negative relative_days", name, preset.Value)
				}
			}
			if filter.Default.Preset != "" {
				if _, ok := presets[filter.Default.Preset]; !ok {
					return fmt.Errorf("filter %q date_range default preset %q is not defined", name, filter.Default.Preset)
				}
			}
		case "multi_select":
			if filter.Operator != "in" {
				return fmt.Errorf("filter %q has unsupported operator %q", name, filter.Operator)
			}
			if filter.Values.Source != "" && filter.Values.Source != "distinct" {
				return fmt.Errorf("filter %q has unsupported values.source %q", name, filter.Values.Source)
			}
		case "text":
			if len(filter.Operators) == 0 {
				return fmt.Errorf("filter %q text requires operators", name)
			}
			operators := map[string]struct{}{}
			for _, operator := range filter.Operators {
				if !supportedTextOperator(operator) {
					return fmt.Errorf("filter %q has unsupported operator %q", name, operator)
				}
				operators[operator] = struct{}{}
			}
			if filter.OperatorURLParam != "" && filter.URLParam == "" {
				return fmt.Errorf("filter %q text operator_url_param requires url_param", name)
			}
			if filter.DefaultOperator != "" {
				if _, ok := operators[filter.DefaultOperator]; !ok {
					return fmt.Errorf("filter %q default_operator %q is not in operators", name, filter.DefaultOperator)
				}
			}
		default:
			return fmt.Errorf("filter %q has unsupported type %q", name, filter.Type)
		}
		d.Filters[name] = filter
	}
	if err := d.validateFilterURLParams(); err != nil {
		return err
	}
	for name, visual := range d.Visuals {
		kind := visual.KindOrDefault()
		if kind != "kpi" && visual.Title == "" {
			return fmt.Errorf("visual %q requires title", name)
		}
		if kind != "kpi" && visual.Type == "" {
			return fmt.Errorf("visual %q requires type", name)
		}
		if !supportsVisualKind(kind) {
			return fmt.Errorf("visual %q has unsupported kind %q", name, kind)
		}
		shape := visual.ShapeOrDefault()
		renderer := visual.RendererOrDefault()
		if !supportsVisualShape(shape) {
			return fmt.Errorf("visual %q has unsupported shape %q", name, shape)
		}
		if kind != "kpi" && !supportsRenderer(renderer) {
			return fmt.Errorf("visual %q has unsupported renderer %q", name, renderer)
		}
		if kind != "kpi" && !rendererSupportsType(renderer, visual.Type) {
			return fmt.Errorf("visual %q renderer %q does not support type %q", name, renderer, visual.Type)
		}
		if kind != "kpi" && !rendererSupportsShapeType(renderer, shape, visual.Type) {
			return fmt.Errorf("visual %q renderer %q type %q does not support shape %q", name, renderer, visual.Type, shape)
		}
		if err := validateVisualQueryShape(name, visual); err != nil {
			return err
		}
		if err := validateRendererOptions(name, visual.RendererOptions); err != nil {
			return err
		}
		if !visual.Query.Series.IsZero() {
			if !supportsSeries(shape) {
				return fmt.Errorf("visual %q shape %q does not support series", name, shape)
			}
			if !rendererTypeSupportsSeries(renderer, visual.Type) {
				return fmt.Errorf("visual %q renderer %q type %q does not support series", name, renderer, visual.Type)
			}
		}
		if shape == "geo" {
			if mapName, ok := visual.Options["map"].(string); !ok || strings.TrimSpace(mapName) == "" {
				return fmt.Errorf("visual %q shape geo requires options.map", name)
			}
		}
		for _, sort := range visual.Query.Sort {
			if sort.Field == "" && sort.Expr == "" {
				return fmt.Errorf("visual %q has sort missing field or expr", name)
			}
		}
		if !visual.Interaction.RowSelection.IsZero() {
			return fmt.Errorf("visual %q does not support row_selection", name)
		}
		if !visual.Interaction.PointSelection.IsZero() {
			if kind == "kpi" {
				return fmt.Errorf("visual %q kind kpi does not support point_selection", name)
			}
			if err := d.validateSelectionInteraction("visual", name, "point_selection", visual.Interaction.PointSelection); err != nil {
				return err
			}
		}
		d.Visuals[name] = visual
	}
	for name, table := range d.Tables {
		if table.Title == "" {
			return fmt.Errorf("table %q requires title", name)
		}
		if err := validateTableStyle(name, table.Style); err != nil {
			return err
		}
		switch table.CardinalityOrDefault() {
		case TableCardinalityBounded, TableCardinalityExact:
		default:
			return fmt.Errorf("table %q has unsupported cardinality %q", name, table.Cardinality)
		}
		for _, column := range table.Columns {
			if err := validateTableColumn(name, column); err != nil {
				return err
			}
		}
		for measure, rules := range table.MeasureFormatting {
			for _, rule := range rules {
				if err := validateTableFormattingRule(name, measure, rule); err != nil {
					return err
				}
			}
		}
		switch table.KindOrDefault() {
		case "data_table":
			if table.Query.Table == "" {
				return fmt.Errorf("table %q kind data_table requires query.table", name)
			}
			if len(table.Query.Fields) == 0 && len(table.Query.Columns) == 0 {
				return fmt.Errorf("table %q kind data_table requires query.fields or query.columns", name)
			}
		case "matrix_table":
			if !table.Interaction.RowSelection.IsZero() {
				return fmt.Errorf("table %q kind matrix_table does not support row_selection", name)
			}
			if len(table.Query.Rows) == 0 || len(table.Query.Measures) == 0 {
				return fmt.Errorf("table %q kind matrix_table requires query.rows and query.measures", name)
			}
			if len(table.Query.Columns) > 1 {
				return fmt.Errorf("table %q kind matrix_table supports at most one column dimension", name)
			}
		case "pivot_table":
			if !table.Interaction.RowSelection.IsZero() {
				return fmt.Errorf("table %q kind pivot_table does not support row_selection", name)
			}
			if len(table.Query.Rows) == 0 || len(table.Query.Columns) != 1 || len(table.Query.Measures) != 1 {
				return fmt.Errorf("table %q kind pivot_table requires query.rows, one query column dimension, and one query measure", name)
			}
		default:
			return fmt.Errorf("table %q has unsupported kind %q", name, table.Kind)
		}
		if !table.Interaction.PointSelection.IsZero() {
			return fmt.Errorf("table %q does not support point_selection", name)
		}
		if !table.Interaction.RowSelection.IsZero() {
			if err := d.validateSelectionInteraction("visual", name, "row_selection", table.Interaction.RowSelection); err != nil {
				return err
			}
		}
		d.Tables[name] = table
	}
	if err := d.validateFilterTargetReferences(); err != nil {
		return err
	}
	return d.validatePages()
}

func (d *Dashboard) validateSelectionInteraction(sourceKind, sourceID, kind string, selection SelectionInteraction) error {
	if len(selection.Mappings) == 0 {
		return fmt.Errorf("%s %q %s requires mappings", sourceKind, sourceID, kind)
	}
	for index, mapping := range selection.Mappings {
		if mapping.Field == "" || mapping.Value == "" {
			return fmt.Errorf("%s %q %s mapping %d requires field and value", sourceKind, sourceID, kind, index)
		}
	}
	for _, target := range selection.Targets {
		if err := d.validateInteractionTarget(sourceKind, sourceID, kind, target); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dashboard) validateInteractionTarget(sourceKind, sourceID, kind, target string) error {
	if target == "" {
		return fmt.Errorf("%s %q %s has empty target", sourceKind, sourceID, kind)
	}
	_, visualOK := d.Visuals[target]
	_, tableOK := d.Tables[target]
	if visualOK && tableOK {
		return fmt.Errorf("%s %q %s target %q is ambiguous across visuals and tables", sourceKind, sourceID, kind, target)
	}
	if !visualOK && !tableOK {
		return fmt.Errorf("%s %q %s references unknown target %q", sourceKind, sourceID, kind, target)
	}
	return nil
}

func (d *Dashboard) validateFilterTargetReferences() error {
	for name, filter := range d.Filters {
		for _, target := range filter.Targets.Visuals {
			if _, ok := d.Visuals[target]; !ok {
				return fmt.Errorf("filter %q references unknown target visual %q", name, target)
			}
		}
		for _, target := range filter.Targets.Tables {
			if _, ok := d.Tables[target]; !ok {
				return fmt.Errorf("filter %q references unknown target table %q", name, target)
			}
		}
	}
	return nil
}

func supportedTextOperator(operator string) bool {
	switch operator {
	case "contains", "not_contains", "equals", "starts_with":
		return true
	default:
		return false
	}
}

func (d *Dashboard) validatePages() error {
	seenPages := map[string]struct{}{}
	for index, page := range d.Pages {
		if page.ID == "" || page.Title == "" {
			return fmt.Errorf("page %d requires id and title", index)
		}
		page = page.WithDefaults()
		if _, exists := seenPages[page.ID]; exists {
			return fmt.Errorf("duplicate page id %q", page.ID)
		}
		seenPages[page.ID] = struct{}{}
		for _, visual := range page.Visuals {
			if visual.ID == "" || visual.Kind == "" {
				return fmt.Errorf("page %q has a visual missing id or kind", page.ID)
			}
			if err := validatePlacement(page, visual); err != nil {
				return err
			}
			switch visual.Kind {
			case "header":
			case "filter_card":
				if visual.Filter == "" {
					return fmt.Errorf("page %q visual %q requires filter", page.ID, visual.ID)
				}
				if _, ok := d.Filters[visual.Filter]; !ok {
					return fmt.Errorf("page %q references unknown filter %q", page.ID, visual.Filter)
				}
			case "kpi_card":
				if visual.Visual == "" {
					return fmt.Errorf("page %q visual %q requires visual", page.ID, visual.ID)
				}
				target, ok := d.Visuals[visual.Visual]
				if !ok {
					return fmt.Errorf("page %q references unknown visual %q", page.ID, visual.Visual)
				}
				if target.KindOrDefault() != "kpi" {
					return fmt.Errorf("page %q visual %q requires a kpi visual", page.ID, visual.ID)
				}
			case "line_chart", "area_chart", "bar_chart", "column_chart", "pie_chart", "donut_chart", "scatter_chart", "funnel_chart", "treemap_chart", "gauge_chart", "heatmap_chart", "sankey_chart", "graph_chart", "map_chart", "candlestick_chart", "boxplot_chart", "combo_chart", "waterfall_chart", "histogram_chart", "radar_chart", "tree_chart", "sunburst_chart":
				if visual.Visual == "" {
					return fmt.Errorf("page %q visual %q requires visual", page.ID, visual.ID)
				}
				target, ok := d.Visuals[visual.Visual]
				if !ok {
					return fmt.Errorf("page %q references unknown visual %q", page.ID, visual.Visual)
				}
				if target.KindOrDefault() == "kpi" {
					return fmt.Errorf("page %q visual %q requires a chart visual", page.ID, visual.ID)
				}
			case "table":
				if visual.Table == "" {
					return fmt.Errorf("page %q visual %q requires table", page.ID, visual.ID)
				}
				if _, ok := d.Tables[visual.Table]; !ok {
					return fmt.Errorf("page %q references unknown table %q", page.ID, visual.Table)
				}
			default:
				return fmt.Errorf("page %q visual %q has unsupported kind %q", page.ID, visual.ID, visual.Kind)
			}
		}
	}
	return nil
}
