package semantic

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadDashboard(path string, metricViews map[string]*MetricView) (*Dashboard, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := rejectLegacyDashboardCollectionKeys(content); err != nil {
		return nil, err
	}
	if err := rejectLegacyVisualStacked(content); err != nil {
		return nil, err
	}
	if err := rejectLegacyKPIs(content); err != nil {
		return nil, err
	}
	if err := rejectLegacyDashboardQueryContract(content); err != nil {
		return nil, err
	}
	var report Dashboard
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&report); err != nil {
		return nil, err
	}
	if err := report.Validate(metricViews); err != nil {
		return nil, err
	}
	return &report, nil
}

func mappingNode(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	if node.Kind == yaml.MappingNode {
		return node
	}
	return nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func (d *Dashboard) Validate(metricViews map[string]*MetricView) error {
	if d.ID == "" || d.Title == "" {
		return fmt.Errorf("dashboard requires id and title")
	}
	if len(d.MetricViews) == 0 {
		return fmt.Errorf("dashboard %q requires metric_views", d.ID)
	}
	allowedViews := map[string]*MetricView{}
	modelName := ""
	for _, viewName := range d.MetricViews {
		view, ok := metricViews[viewName]
		if !ok {
			return fmt.Errorf("dashboard %q references unknown metric view %q", d.ID, viewName)
		}
		if modelName == "" {
			modelName = view.SemanticModel
		}
		if view.SemanticModel != modelName {
			return fmt.Errorf("dashboard %q metric views must use one semantic model", d.ID)
		}
		allowedViews[viewName] = view
	}
	if len(d.Visuals) == 0 {
		return fmt.Errorf("dashboard %q requires visuals", d.ID)
	}
	if len(d.Pages) == 0 {
		return fmt.Errorf("dashboard %q requires pages", d.ID)
	}
	for name, filter := range d.Filters {
		if filter.Type == "" || filter.Label == "" || filter.MetricView == "" || filter.Dimension == "" {
			return fmt.Errorf("filter %q requires type, label, metric_view, and field", name)
		}
		view, ok := allowedViews[filter.MetricView]
		if !ok {
			return fmt.Errorf("filter %q references unknown metric view %q", name, filter.MetricView)
		}
		field, _, err := view.ResolveDimensionRef(filter.Dimension)
		if err != nil {
			return fmt.Errorf("filter %q references unknown dimension %q", name, filter.Dimension)
		}
		filter.Dimension = field
		d.Filters[name] = filter
		switch filter.Type {
		case "date_range":
			if filter.URLParam == "" || filter.FromURLParam == "" || filter.ToURLParam == "" {
				return fmt.Errorf("filter %q date_range requires url_param, from_url_param, and to_url_param", name)
			}
			if len(filter.Presets) == 0 {
				return fmt.Errorf("filter %q date_range requires presets", name)
			}
			seen := map[string]struct{}{}
			for _, preset := range filter.Presets {
				if preset.Value == "" || preset.Label == "" {
					return fmt.Errorf("filter %q has date preset missing value or label", name)
				}
				if _, exists := seen[preset.Value]; exists {
					return fmt.Errorf("filter %q has duplicate date preset %q", name, preset.Value)
				}
				seen[preset.Value] = struct{}{}
				if preset.RelativeDays < 0 {
					return fmt.Errorf("filter %q date preset %q has invalid relative_days", name, preset.Value)
				}
				if (preset.From == "") != (preset.To == "") {
					return fmt.Errorf("filter %q date preset %q requires both from and to", name, preset.Value)
				}
			}
			if filter.Default.Preset == "" {
				return fmt.Errorf("filter %q date_range requires default preset", name)
			}
			if _, ok := seen[filter.Default.Preset]; !ok {
				return fmt.Errorf("filter %q default preset %q is unknown", name, filter.Default.Preset)
			}
		case "multi_select":
			if filter.Operator != "in" {
				return fmt.Errorf("filter %q has unsupported operator %q", name, filter.Operator)
			}
			if filter.Values.Source != "" && filter.Values.Source != "distinct" {
				return fmt.Errorf("filter %q has unsupported values source %q", name, filter.Values.Source)
			}
		case "text":
			if len(filter.Operators) == 0 {
				return fmt.Errorf("filter %q text requires operators", name)
			}
			if filter.OperatorURLParam != "" && filter.URLParam == "" {
				return fmt.Errorf("filter %q operator_url_param requires url_param", name)
			}
			if !containsString(filter.Operators, filter.DefaultOperator) {
				return fmt.Errorf("filter %q has unsupported default operator %q", name, filter.DefaultOperator)
			}
			for _, operator := range filter.Operators {
				if !containsString([]string{"contains", "equals", "starts_with", "not_contains"}, operator) {
					return fmt.Errorf("filter %q has unsupported operator %q", name, operator)
				}
			}
		default:
			return fmt.Errorf("filter %q has unsupported type %q", name, filter.Type)
		}
	}
	if err := d.validateFilterURLParams(); err != nil {
		return err
	}
	for name, visual := range d.Visuals {
		kind := visual.KindOrDefault()
		visual.MetricView = visual.Query.MetricView
		if visual.Query.MetricView == "" || (kind != "kpi" && visual.Title == "") || (kind != "kpi" && visual.Type == "") {
			return fmt.Errorf("visual %q requires title, query.metric_view, and type", name)
		}
		view, ok := allowedViews[visual.MetricView]
		if !ok {
			return fmt.Errorf("visual %q references unknown metric view %q", name, visual.MetricView)
		}
		shape := visual.ShapeOrDefault()
		renderer := visual.RendererOrDefault()
		if !supportsVisualKind(kind) {
			return fmt.Errorf("visual %q has unsupported kind %q", name, kind)
		}
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
		for index, dimension := range visual.Query.Dimensions {
			field, _, err := view.ResolveDimensionRef(dimension.Field)
			if err != nil {
				return fmt.Errorf("visual %q references unknown dimension %q", name, dimension.Field)
			}
			visual.Query.Dimensions[index].Field = field
		}
		if !visual.Query.Series.IsZero() {
			field, _, err := view.ResolveDimensionRef(visual.Query.Series.Field)
			if err != nil {
				return fmt.Errorf("visual %q references unknown series dimension %q", name, visual.Query.Series.Field)
			}
			visual.Query.Series.Field = field
			if !supportsSeries(shape) {
				return fmt.Errorf("visual %q shape %q does not support series", name, shape)
			}
			if !rendererTypeSupportsSeries(renderer, visual.Type) {
				return fmt.Errorf("visual %q renderer %q type %q does not support series", name, renderer, visual.Type)
			}
		}
		for index, measure := range visual.Query.Measures {
			field, _, err := view.ResolveMeasureRef(measure.Field)
			if err != nil {
				return fmt.Errorf("visual %q references unknown measure %q", name, measure.Field)
			}
			visual.Query.Measures[index].Field = field
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
		if visual.Interaction.Field != "" {
			field, _, err := view.ResolveDimensionRef(visual.Interaction.Field)
			if err != nil {
				return fmt.Errorf("visual %q interaction references unknown field %q", name, visual.Interaction.Field)
			}
			visual.Interaction.Field = field
		}
		d.Visuals[name] = visual
	}
	for name, table := range d.Tables {
		table.MetricView = table.Query.MetricView
		if table.Title == "" || table.Query.MetricView == "" {
			return fmt.Errorf("table %q requires title and query.metric_view", name)
		}
		if err := validateTableStyle(name, table.Style); err != nil {
			return err
		}
		view, ok := allowedViews[table.MetricView]
		if !ok {
			return fmt.Errorf("table %q references unknown metric view %q", name, table.MetricView)
		}
		normalizeTableFormatting(view, &table)
		for _, column := range table.Columns {
			if err := validateTableColumn(name, column); err != nil {
				return err
			}
		}
		for measure, rules := range table.MeasureFormatting {
			if _, ok := view.Measures[measure]; !ok {
				return fmt.Errorf("table %q measure_formatting references unknown measure %q", name, measure)
			}
			for _, rule := range rules {
				if err := validateTableFormattingRule(name, measure, rule); err != nil {
					return err
				}
			}
		}
		switch table.KindOrDefault() {
		case "data_table":
			if len(table.Columns) == 0 || len(table.Query.Columns) == 0 {
				return fmt.Errorf("table %q kind data_table requires presentation columns and query.columns", name)
			}
			table.DataColumns = make([]FieldRef, len(table.Query.Columns))
			copy(table.DataColumns, table.Query.Columns)
			for index, column := range table.DataColumns {
				field, _, err := view.ResolveDimensionRef(column.Field)
				if err == nil {
					table.DataColumns[index].Field = field
					continue
				}
				field, _, err = view.ResolveMeasureRef(column.Field)
				if err != nil {
					return fmt.Errorf("table %q query.columns references unknown field %q", name, column.Field)
				}
				table.DataColumns[index].Field = field
			}
			for _, column := range table.Columns {
				if !tableHasQueryAlias(table.DataColumns, column.Key) {
					return fmt.Errorf("table %q column %q has no matching query column alias", name, column.Key)
				}
			}
		case "matrix_table":
			if len(table.Query.Rows) == 0 || len(table.Query.Measures) == 0 {
				return fmt.Errorf("table %q kind matrix_table requires query.rows and query.measures", name)
			}
			if len(table.Query.Columns) > 1 {
				return fmt.Errorf("table %q kind matrix_table supports at most one column dimension", name)
			}
			if err := normalizeTableFields(name, view, &table); err != nil {
				return err
			}
		case "pivot_table":
			if len(table.Query.Rows) == 0 || len(table.Query.Columns) != 1 || len(table.Query.Measures) != 1 {
				return fmt.Errorf("table %q kind pivot_table requires query.rows, one query column dimension, and one query measure", name)
			}
			if err := normalizeTableFields(name, view, &table); err != nil {
				return err
			}
		default:
			return fmt.Errorf("table %q has unsupported kind %q", name, table.Kind)
		}
		d.Tables[name] = table
	}
	for name, visual := range d.Visuals {
		for _, target := range visual.Interaction.Targets.Visuals {
			if _, ok := d.Visuals[target]; !ok {
				return fmt.Errorf("visual %q interaction references unknown target visual %q", name, target)
			}
		}
		for _, target := range visual.Interaction.Targets.Tables {
			if _, ok := d.Tables[target]; !ok {
				return fmt.Errorf("visual %q interaction references unknown target table %q", name, target)
			}
		}
	}
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
