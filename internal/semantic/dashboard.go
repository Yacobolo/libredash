package semantic

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadDashboard(path string, models map[string]*Model) (*Dashboard, error) {
	return LoadDashboardWithModels(path, models)
}

func LoadDashboardWithModels(path string, models map[string]*Model) (*Dashboard, error) {
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
	if err := report.ValidateWithModels(models); err != nil {
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

func (d *Dashboard) Validate(models map[string]*Model) error {
	return d.ValidateWithModels(models)
}

func (d *Dashboard) ValidateWithModels(models map[string]*Model) error {
	return d.validateSemanticModelDashboard(models)
}

func (d *Dashboard) validateSemanticModelDashboard(models map[string]*Model) error {
	if d.ID == "" || d.Title == "" {
		return fmt.Errorf("dashboard requires id and title")
	}
	model, ok := models[d.SemanticModel]
	if !ok {
		return fmt.Errorf("dashboard %q references unknown semantic model %q", d.ID, d.SemanticModel)
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
		if _, err := model.ResolveDimension(filter.Dimension); err != nil {
			return fmt.Errorf("filter %q references unknown dimension %q", name, filter.Dimension)
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
		for _, dimension := range visual.Query.Dimensions {
			if _, err := model.ResolveDimension(dimension.Field); err != nil {
				return fmt.Errorf("visual %q references unknown dimension %q", name, dimension.Field)
			}
		}
		if !visual.Query.Series.IsZero() {
			if _, err := model.ResolveDimension(visual.Query.Series.Field); err != nil {
				return fmt.Errorf("visual %q references unknown series dimension %q", name, visual.Query.Series.Field)
			}
			if !supportsSeries(shape) {
				return fmt.Errorf("visual %q shape %q does not support series", name, shape)
			}
			if !rendererTypeSupportsSeries(renderer, visual.Type) {
				return fmt.Errorf("visual %q renderer %q type %q does not support series", name, renderer, visual.Type)
			}
		}
		for _, measure := range visual.Query.Measures {
			if measure.Measure.Expression != "" {
				if measure.Measure.Table == "" || measure.Measure.Grain == "" || measure.Measure.Time == "" || len(measure.Measure.Grains) == 0 || measure.Measure.Format == "" {
					return fmt.Errorf("visual %q inline measure %q requires expr, table, grain, time, grains, and format", name, measure.Alias)
				}
				if _, ok := model.Tables[measure.Measure.Table]; !ok {
					return fmt.Errorf("visual %q inline measure %q references unknown table %q", name, measure.Alias, measure.Measure.Table)
				}
				continue
			}
			if _, err := model.ResolveMeasure(measure.Field); err != nil {
				return fmt.Errorf("visual %q references unknown measure %q", name, measure.Field)
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
		if visual.Interaction.Field != "" {
			if _, err := model.ResolveDimension(visual.Interaction.Field); err != nil {
				return fmt.Errorf("visual %q interaction references unknown field %q", name, visual.Interaction.Field)
			}
		}
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
		d.Visuals[name] = visual
	}
	for name, table := range d.Tables {
		if table.Title == "" {
			return fmt.Errorf("table %q requires title", name)
		}
		if err := validateTableStyle(name, table.Style); err != nil {
			return err
		}
		normalizeTableFormatting(model, &table)
		for _, column := range table.Columns {
			if err := validateTableColumn(name, column); err != nil {
				return err
			}
		}
		for measure, rules := range table.MeasureFormatting {
			if _, err := model.ResolveMeasure(measure); err != nil {
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
			if table.Query.Table == "" {
				return fmt.Errorf("table %q kind data_table requires query.table", name)
			}
			if err := normalizeDataTableFields(name, model, &table); err != nil {
				return err
			}
		case "matrix_table":
			if len(table.Query.Rows) == 0 || len(table.Query.Measures) == 0 {
				return fmt.Errorf("table %q kind matrix_table requires query.rows and query.measures", name)
			}
			if len(table.Query.Columns) > 1 {
				return fmt.Errorf("table %q kind matrix_table supports at most one column dimension", name)
			}
			if err := normalizeTableFields(name, model, &table); err != nil {
				return err
			}
		case "pivot_table":
			if len(table.Query.Rows) == 0 || len(table.Query.Columns) != 1 || len(table.Query.Measures) != 1 {
				return fmt.Errorf("table %q kind pivot_table requires query.rows, one query column dimension, and one query measure", name)
			}
			if err := normalizeTableFields(name, model, &table); err != nil {
				return err
			}
		default:
			return fmt.Errorf("table %q has unsupported kind %q", name, table.Kind)
		}
		d.Tables[name] = table
	}
	if err := d.validateFilterTargets(model); err != nil {
		return err
	}
	return d.validatePages()
}

func (d *Dashboard) validateFilterTargets(model *Model) error {
	for name, filter := range d.Filters {
		for _, target := range filter.Targets.Visuals {
			if _, ok := d.Visuals[target]; !ok {
				return fmt.Errorf("filter %q references unknown target visual %q", name, target)
			}
			ok, err := d.FilterAppliesToTarget(model, filter, "visual", target)
			if err != nil || !ok {
				if err == nil {
					err = fmt.Errorf("filter field %q is not reachable", filter.Dimension)
				}
				return fmt.Errorf("filter %q cannot apply to visual %q: %w", name, target, err)
			}
		}
		for _, target := range filter.Targets.Tables {
			if _, ok := d.Tables[target]; !ok {
				return fmt.Errorf("filter %q references unknown target table %q", name, target)
			}
			ok, err := d.FilterAppliesToTarget(model, filter, "table", target)
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
