package semantic

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"gopkg.in/yaml.v3"
)

type Dashboard struct {
	ID          string                      `yaml:"id"`
	Title       string                      `yaml:"title"`
	Description string                      `yaml:"description"`
	MetricViews []string                    `yaml:"metrics_views"`
	Filters     map[string]FilterDefinition `yaml:"filters"`
	Visuals     map[string]Visual           `yaml:"visuals"`
	Tables      map[string]TableVisual      `yaml:"tables"`
	Pages       []dashboard.Page            `yaml:"pages"`
}

type FilterDefinition struct {
	Type             string         `yaml:"type" json:"type"`
	Label            string         `yaml:"label" json:"label"`
	MetricView       string         `yaml:"metric_view" json:"metricsView"`
	Dimension        string         `yaml:"field" json:"dimension"`
	Default          FilterDefault  `yaml:"default" json:"default"`
	Custom           bool           `yaml:"custom" json:"custom,omitempty"`
	Presets          []FilterPreset `yaml:"presets" json:"presets,omitempty"`
	Operator         string         `yaml:"operator" json:"operator,omitempty"`
	Values           FilterValues   `yaml:"values" json:"values,omitempty"`
	DefaultOperator  string         `yaml:"default_operator" json:"defaultOperator,omitempty"`
	Operators        []string       `yaml:"operators" json:"operators,omitempty"`
	Options          []FilterOption `yaml:"options" json:"options,omitempty"`
	URLParam         string         `yaml:"url_param" json:"urlParam,omitempty"`
	FromURLParam     string         `yaml:"from_url_param" json:"fromURLParam,omitempty"`
	ToURLParam       string         `yaml:"to_url_param" json:"toURLParam,omitempty"`
	OperatorURLParam string         `yaml:"operator_url_param" json:"operatorURLParam,omitempty"`
}

type FilterConfig struct {
	ID string `json:"id"`
	FilterDefinition
}

type FilterOption struct {
	Value string `yaml:"value" json:"value"`
	Label string `yaml:"label" json:"label"`
}

type FilterDefault struct {
	Preset   string   `yaml:"preset" json:"preset,omitempty"`
	From     string   `yaml:"from" json:"from,omitempty"`
	To       string   `yaml:"to" json:"to,omitempty"`
	Operator string   `yaml:"operator" json:"operator,omitempty"`
	Value    string   `yaml:"value" json:"value,omitempty"`
	Values   []string `yaml:"values" json:"values,omitempty"`
}

type FilterPreset struct {
	Value        string `yaml:"value" json:"value"`
	Label        string `yaml:"label" json:"label"`
	From         string `yaml:"from" json:"from,omitempty"`
	To           string `yaml:"to" json:"to,omitempty"`
	RelativeDays int    `yaml:"relative_days" json:"relativeDays,omitempty"`
}

type FilterValues struct {
	Source string `yaml:"source" json:"source,omitempty"`
	Limit  int    `yaml:"limit" json:"limit,omitempty"`
}

type Visual struct {
	Title           string         `yaml:"title"`
	Kind            string         `yaml:"kind"`
	Shape           string         `yaml:"shape"`
	Renderer        string         `yaml:"renderer"`
	Type            string         `yaml:"type"`
	MetricView      string         `yaml:"-"`
	Query           VisualQuery    `yaml:"query"`
	Options         map[string]any `yaml:"options"`
	RendererOptions map[string]any `yaml:"renderer_options"`
	Interaction     Interaction    `yaml:"interaction"`
}

type VisualQuery struct {
	MetricView string     `yaml:"metric_view"`
	Dimensions []FieldRef `yaml:"dimensions"`
	Series     FieldRef   `yaml:"series"`
	Measures   []FieldRef `yaml:"measures"`
	Time       QueryTime  `yaml:"time"`
	Sort       []Sort     `yaml:"sort"`
	Limit      int        `yaml:"limit"`
}

type FieldRef struct {
	Field string `yaml:"field" json:"field"`
	Alias string `yaml:"alias,omitempty" json:"alias,omitempty"`
}

type QueryTime struct {
	Field string `yaml:"field" json:"field"`
	Grain string `yaml:"grain" json:"grain"`
	Alias string `yaml:"alias,omitempty" json:"alias,omitempty"`
}

func (f *FieldRef) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("field ref must be a mapping with field and alias")
	}
	type raw FieldRef
	if err := value.Decode((*raw)(f)); err != nil {
		return err
	}
	if f.Field == "" || f.Alias == "" {
		return fmt.Errorf("field ref requires field and alias")
	}
	return nil
}

func (f FieldRef) IsZero() bool {
	return f.Field == ""
}

type Sort struct {
	Field     string `yaml:"field"`
	Direction string `yaml:"direction"`
	Expr      string `yaml:"expr"`
}

type Interaction struct {
	Field   string             `yaml:"field"`
	Targets InteractionTargets `yaml:"targets"`
}

type InteractionTargets struct {
	Visuals []string `yaml:"visuals"`
	Tables  []string `yaml:"tables"`
}

type TableVisual struct {
	Kind              string                                     `yaml:"kind"`
	Title             string                                     `yaml:"title"`
	MetricView        string                                     `yaml:"-"`
	Query             TableQuery                                 `yaml:"query"`
	DefaultSort       dashboard.TableSort                        `yaml:"default_sort"`
	Style             dashboard.TableStyle                       `yaml:"style"`
	Columns           []dashboard.TableColumn                    `yaml:"columns"`
	Rows              []string                                   `yaml:"-"`
	Measures          []string                                   `yaml:"-"`
	MeasureFormatting map[string][]dashboard.TableFormattingRule `yaml:"measure_formatting"`
	DataColumns       []FieldRef                                 `yaml:"-"`
	ColumnDims        []string                                   `yaml:"-"`
}

type TableQuery struct {
	MetricView string     `yaml:"metric_view"`
	Columns    []FieldRef `yaml:"columns"`
	Rows       []FieldRef `yaml:"rows"`
	Measures   []FieldRef `yaml:"measures"`
}

func (t TableVisual) KindOrDefault() string {
	if t.Kind != "" {
		return t.Kind
	}
	return "data_table"
}

func (v Visual) KindOrDefault() string {
	if v.Kind != "" {
		return v.Kind
	}
	return "chart"
}

func (v Visual) ShapeOrDefault() string {
	if v.Shape != "" {
		return v.Shape
	}
	if v.KindOrDefault() == "kpi" {
		return "single_value"
	}
	switch v.Type {
	case "combo":
		return "category_multi_measure"
	case "waterfall":
		return "category_delta"
	case "histogram":
		return "binned_measure"
	case "tree", "sunburst":
		return "hierarchy"
	case "heatmap":
		return "matrix"
	case "sankey", "graph":
		return "graph"
	case "map":
		return "geo"
	case "candlestick":
		return "ohlc"
	case "boxplot":
		return "distribution"
	case "gauge":
		return "single_value"
	}
	if v.Type == "gauge" {
		return "single_value"
	}
	if !v.Query.Series.IsZero() {
		return "category_series_value"
	}
	return "category_value"
}

func (v Visual) RendererOrDefault() string {
	if v.Renderer != "" {
		return v.Renderer
	}
	if v.KindOrDefault() == "kpi" {
		return "html"
	}
	return "echarts"
}

func (v Visual) CoreOptions() map[string]any {
	return copyMap(v.Options)
}

func copyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return map[string]any{}
	}
	next := make(map[string]any, len(source))
	for key, value := range source {
		next[key] = value
	}
	return next
}

func LoadDashboard(path string, metricViews map[string]*MetricView) (*Dashboard, error) {
	content, err := os.ReadFile(path)
	if err != nil {
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

func rejectLegacyDashboardQueryContract(bytes []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(bytes, &node); err != nil {
		return err
	}
	root := mappingNode(&node)
	if root == nil {
		return nil
	}
	for _, section := range []string{"filters", "visuals", "tables"} {
		items := mappingValue(root, section)
		if items == nil || items.Kind != yaml.MappingNode {
			continue
		}
		for index := 0; index+1 < len(items.Content); index += 2 {
			name := items.Content[index].Value
			item := items.Content[index+1]
			if item.Kind != yaml.MappingNode {
				continue
			}
			if mappingValue(item, "metrics_view") != nil {
				return fmt.Errorf("%s %q uses legacy metrics_view; use metric_view or query.metric_view", strings.TrimSuffix(section, "s"), name)
			}
			if section == "visuals" && mappingValue(item, "metric_view") != nil {
				return fmt.Errorf("visual %q uses top-level metric_view; use query.metric_view", name)
			}
			if section == "tables" {
				if mappingValue(item, "rows") != nil || mappingValue(item, "measures") != nil {
					return fmt.Errorf("table %q uses legacy rows/measures; use query.rows/query.measures", name)
				}
			}
			queryNode := mappingValue(item, "query")
			if queryNode == nil {
				continue
			}
			if rawSQL := mappingValue(queryNode, "sql"); rawSQL != nil {
				return fmt.Errorf("%s %q uses raw SQL; dashboards must query metric views", strings.TrimSuffix(section, "s"), name)
			}
			for _, key := range []string{"dimensions", "measures", "rows", "columns"} {
				if err := rejectScalarFieldRefs(section, name, queryNode, key); err != nil {
					return err
				}
			}
			if series := mappingValue(queryNode, "series"); series != nil && series.Kind == yaml.ScalarNode {
				return fmt.Errorf("%s %q query.series must be a field object", strings.TrimSuffix(section, "s"), name)
			}
		}
	}
	return nil
}

func rejectScalarFieldRefs(section, name string, queryNode *yaml.Node, key string) error {
	node := mappingValue(queryNode, key)
	if node == nil {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s %q query.%s must be a sequence", strings.TrimSuffix(section, "s"), name, key)
	}
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return fmt.Errorf("%s %q query.%s must contain field objects", strings.TrimSuffix(section, "s"), name, key)
		}
	}
	return nil
}

func rejectLegacyVisualStacked(bytes []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(bytes, &node); err != nil {
		return err
	}
	root := mappingNode(&node)
	if root == nil {
		return nil
	}
	visuals := mappingValue(root, "visuals")
	if visuals == nil || visuals.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(visuals.Content); index += 2 {
		name := visuals.Content[index].Value
		visualNode := visuals.Content[index+1]
		if visualNode.Kind != yaml.MappingNode {
			continue
		}
		if mappingValue(visualNode, "stacked") != nil {
			return fmt.Errorf("visual %q uses legacy top-level stacked; use options.stacked", name)
		}
	}
	return nil
}

func rejectLegacyKPIs(bytes []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(bytes, &node); err != nil {
		return err
	}
	root := mappingNode(&node)
	if root == nil {
		return nil
	}
	if mappingValue(root, "kpis") != nil {
		return fmt.Errorf("dashboard uses legacy kpis; define KPI cards as visuals with kind kpi")
	}
	return nil
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
		return fmt.Errorf("dashboard %q requires metrics_views", d.ID)
	}
	allowedViews := map[string]*MetricView{}
	modelName := ""
	for _, viewName := range d.MetricViews {
		view, ok := metricViews[viewName]
		if !ok {
			return fmt.Errorf("dashboard %q references unknown metrics view %q", d.ID, viewName)
		}
		if modelName == "" {
			modelName = view.SemanticModel
		}
		if view.SemanticModel != modelName {
			return fmt.Errorf("dashboard %q metrics views must use one semantic model", d.ID)
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
			return fmt.Errorf("filter %q references unknown metrics view %q", name, filter.MetricView)
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
			return fmt.Errorf("visual %q references unknown metrics view %q", name, visual.MetricView)
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
			return fmt.Errorf("table %q references unknown metrics view %q", name, table.MetricView)
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

func validateVisualQueryShape(name string, visual Visual) error {
	if visual.KindOrDefault() == "kpi" {
		if visual.ShapeOrDefault() != "single_value" {
			return fmt.Errorf("visual %q kind kpi requires shape single_value", name)
		}
		if len(visual.Query.Measures) != 1 {
			return fmt.Errorf("visual %q kind kpi requires exactly one query measure", name)
		}
		if len(visual.Query.Dimensions) != 0 {
			return fmt.Errorf("visual %q kind kpi does not support query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q kind kpi does not support series", name)
		}
		return nil
	}
	shape := visual.ShapeOrDefault()
	switch shape {
	case "ohlc":
		if len(visual.Query.Measures) != 4 {
			return fmt.Errorf("visual %q shape ohlc requires exactly four query measures", name)
		}
	case "category_multi_measure":
		if len(visual.Query.Measures) < 2 {
			return fmt.Errorf("visual %q shape category_multi_measure requires at least two query measures", name)
		}
	default:
		if len(visual.Query.Measures) != 1 {
			return fmt.Errorf("visual %q requires exactly one query measure", name)
		}
	}
	if len(visual.Query.Measures) == 0 {
		return fmt.Errorf("visual %q requires exactly one query measure", name)
	}
	switch shape {
	case "category_value":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_value requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_value does not support series", name)
		}
	case "category_series_value":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_series_value requires exactly one query dimension", name)
		}
		if visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_series_value requires query series", name)
		}
	case "category_multi_measure":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_multi_measure requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_multi_measure does not support series", name)
		}
	case "category_delta":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_delta requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_delta does not support series", name)
		}
	case "binned_measure":
		if len(visual.Query.Dimensions) != 0 {
			return fmt.Errorf("visual %q shape binned_measure does not support query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape binned_measure does not support series", name)
		}
	case "hierarchy":
		if len(visual.Query.Dimensions) == 0 {
			return fmt.Errorf("visual %q shape hierarchy requires at least one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape hierarchy does not support series", name)
		}
	case "single_value":
		if len(visual.Query.Dimensions) > 1 {
			return fmt.Errorf("visual %q shape single_value supports at most one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape single_value does not support series", name)
		}
	case "matrix":
		if len(visual.Query.Dimensions) != 2 {
			return fmt.Errorf("visual %q shape matrix requires exactly two query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape matrix does not support series", name)
		}
	case "graph":
		if len(visual.Query.Dimensions) != 2 {
			return fmt.Errorf("visual %q shape graph requires exactly two query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape graph does not support series", name)
		}
	case "geo":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape geo requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape geo does not support series", name)
		}
	case "ohlc":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape ohlc requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape ohlc does not support series", name)
		}
	case "distribution":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape distribution requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape distribution does not support series", name)
		}
	}
	return nil
}

func validateRendererOptions(name string, options map[string]any) error {
	for renderer, value := range options {
		if !supportsRenderer(renderer) {
			return fmt.Errorf("visual %q has renderer_options for unsupported renderer %q", name, renderer)
		}
		option, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("visual %q renderer_options.%s must be an object", name, renderer)
		}
		if err := validateSafeRendererOption(name, "renderer_options."+renderer, option); err != nil {
			return err
		}
	}
	return nil
}

func validateSafeRendererOption(name, path string, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			nextPath := path + "." + key
			if key == "renderItem" {
				return fmt.Errorf("visual %q has unsafe renderer option %q", name, nextPath)
			}
			if err := validateSafeRendererOption(name, nextPath, item); err != nil {
				return err
			}
		}
	case []any:
		for index, item := range typed {
			if err := validateSafeRendererOption(name, fmt.Sprintf("%s[%d]", path, index), item); err != nil {
				return err
			}
		}
	case string:
		if strings.Contains(typed, "function(") || strings.Contains(typed, "=>") {
			return fmt.Errorf("visual %q has unsafe renderer option %q", name, path)
		}
	}
	return nil
}

func (d *Dashboard) validateFilterURLParams() error {
	seen := map[string]string{}
	for name, filter := range d.Filters {
		for _, param := range []string{filter.URLParam, filter.FromURLParam, filter.ToURLParam, filter.OperatorURLParam} {
			if param == "" {
				continue
			}
			if owner, exists := seen[param]; exists {
				return fmt.Errorf("filter %q url param %q duplicates filter %q", name, param, owner)
			}
			seen[param] = name
		}
	}
	return nil
}

func (d *Dashboard) DefaultFilters() dashboard.Filters {
	return d.defaultFiltersForNames(sortedFilterNames(d.Filters))
}

func (d *Dashboard) DefaultFiltersForPage(pageID string) dashboard.Filters {
	return d.defaultFiltersForNames(d.PageFilterIDs(pageID))
}

func (d *Dashboard) defaultFiltersForNames(names []string) dashboard.Filters {
	filters := dashboard.Filters{
		Controls:         map[string]dashboard.FilterControl{},
		VisualSelections: []dashboard.VisualSelection{},
	}
	for _, name := range names {
		filter, ok := d.Filters[name]
		if !ok {
			continue
		}
		control := dashboard.FilterControl{Type: filter.Type}
		switch filter.Type {
		case "date_range":
			control.Preset = filter.Default.Preset
			control.From = filter.Default.From
			control.To = filter.Default.To
		case "multi_select":
			control.Operator = filter.Operator
			control.Values = append([]string{}, filter.Default.Values...)
		case "text":
			control.Operator = filter.DefaultOperator
			if filter.Default.Operator != "" {
				control.Operator = filter.Default.Operator
			}
			control.Value = filter.Default.Value
		}
		filters.Controls[name] = control
	}
	return filters.WithDefaults()
}

func (d *Dashboard) FiltersFromURL(values url.Values) dashboard.Filters {
	return d.filtersFromURLForNames(sortedFilterNames(d.Filters), values)
}

func (d *Dashboard) FiltersFromURLForPage(pageID string, values url.Values) dashboard.Filters {
	return d.filtersFromURLForNames(d.PageFilterIDs(pageID), values)
}

func (d *Dashboard) filtersFromURLForNames(names []string, values url.Values) dashboard.Filters {
	filters := d.defaultFiltersForNames(names)
	for _, name := range names {
		filter := d.Filters[name]
		control := filters.Controls[name]
		switch filter.Type {
		case "date_range":
			control = d.dateFilterFromURL(filter, control, values)
		case "multi_select":
			if filter.URLParam != "" {
				control.Values = compactStrings(values[filter.URLParam])
			}
		case "text":
			if filter.URLParam != "" {
				control.Value = strings.TrimSpace(values.Get(filter.URLParam))
			}
			if filter.OperatorURLParam != "" {
				operator := strings.TrimSpace(values.Get(filter.OperatorURLParam))
				if containsString(filter.Operators, operator) {
					control.Operator = operator
				}
			}
		}
		filters.Controls[name] = control
	}
	return filters.WithDefaults()
}

func (d *Dashboard) dateFilterFromURL(filter FilterDefinition, control dashboard.FilterControl, values url.Values) dashboard.FilterControl {
	preset := strings.TrimSpace(values.Get(filter.URLParam))
	from := strings.TrimSpace(values.Get(filter.FromURLParam))
	to := strings.TrimSpace(values.Get(filter.ToURLParam))
	if from != "" || to != "" {
		control.Preset = "custom"
		control.From = from
		control.To = to
		return control
	}
	if preset == "" {
		control.From = ""
		control.To = ""
		return control
	}
	if preset == "custom" {
		control.Preset = "custom"
		control.From = ""
		control.To = ""
		return control
	}
	if d.hasPreset(filter, preset) {
		control.Preset = preset
		control.From = ""
		control.To = ""
	}
	return control
}

func (d *Dashboard) hasPreset(filter FilterDefinition, preset string) bool {
	for _, item := range filter.Presets {
		if item.Value == preset {
			return true
		}
	}
	return false
}

func (d *Dashboard) URLParamShape() map[string]any {
	return d.urlParamShapeForNames(sortedFilterNames(d.Filters))
}

func (d *Dashboard) URLParamShapeForPage(pageID string) map[string]any {
	return d.urlParamShapeForNames(d.PageFilterIDs(pageID))
}

func (d *Dashboard) urlParamShapeForNames(names []string) map[string]any {
	shape := map[string]any{}
	for _, name := range names {
		filter := d.Filters[name]
		switch filter.Type {
		case "date_range":
			addStringShape(shape, filter.URLParam)
			addStringShape(shape, filter.FromURLParam)
			addStringShape(shape, filter.ToURLParam)
		case "multi_select":
			if filter.URLParam != "" {
				shape[filter.URLParam] = []string{}
			}
		case "text":
			addStringShape(shape, filter.URLParam)
			addStringShape(shape, filter.OperatorURLParam)
		}
	}
	return shape
}

func (d *Dashboard) URLParamsFromFilters(filters dashboard.Filters) map[string]any {
	return d.urlParamsFromFiltersForNames(sortedFilterNames(d.Filters), filters)
}

func (d *Dashboard) URLParamsFromFiltersForPage(pageID string, filters dashboard.Filters) map[string]any {
	return d.urlParamsFromFiltersForNames(d.PageFilterIDs(pageID), filters)
}

func (d *Dashboard) urlParamsFromFiltersForNames(names []string, filters dashboard.Filters) map[string]any {
	params := map[string]any{}
	defaults := d.defaultFiltersForNames(names)
	filters = filters.WithDefaults()
	for _, name := range names {
		filter := d.Filters[name]
		control, ok := filters.Controls[name]
		if !ok {
			control = defaults.Controls[name]
		}
		switch filter.Type {
		case "date_range":
			defaultPreset := defaults.Controls[name].Preset
			if control.From != "" || control.To != "" || control.Preset == "custom" {
				params[filter.URLParam] = "custom"
				addStringParam(params, filter.FromURLParam, control.From)
				addStringParam(params, filter.ToURLParam, control.To)
				continue
			}
			if control.Preset != "" && control.Preset != defaultPreset {
				params[filter.URLParam] = control.Preset
			}
		case "multi_select":
			if filter.URLParam != "" && len(control.Values) > 0 {
				params[filter.URLParam] = append([]string{}, control.Values...)
			}
		case "text":
			value := strings.TrimSpace(control.Value)
			if filter.URLParam == "" || value == "" {
				continue
			}
			params[filter.URLParam] = value
			defaultOperator := defaults.Controls[name].Operator
			if filter.OperatorURLParam != "" && control.Operator != "" && control.Operator != defaultOperator {
				params[filter.OperatorURLParam] = control.Operator
			}
		}
	}
	return params
}

func (d *Dashboard) NormalizeFiltersForPage(pageID string, filters dashboard.Filters) dashboard.Filters {
	names := d.PageFilterIDs(pageID)
	defaults := d.defaultFiltersForNames(names)
	activeFilters := map[string]struct{}{}
	for _, name := range names {
		activeFilters[name] = struct{}{}
	}

	filters = filters.WithDefaults()
	for name, control := range filters.Controls {
		if _, ok := activeFilters[name]; !ok {
			continue
		}
		filter := d.Filters[name]
		base := defaults.Controls[name]
		if control.Type == "" {
			control.Type = filter.Type
		}
		switch filter.Type {
		case "date_range":
			if control.Preset == "" && control.From == "" && control.To == "" {
				control.Preset = base.Preset
			}
		case "multi_select":
			if control.Operator == "" {
				control.Operator = base.Operator
			}
			if control.Values == nil {
				control.Values = []string{}
			}
		case "text":
			if control.Operator == "" {
				control.Operator = base.Operator
			}
		}
		defaults.Controls[name] = control
	}

	activeVisuals := d.pageVisualIDSet(pageID)
	defaults.VisualSelections = make([]dashboard.VisualSelection, 0, len(filters.VisualSelections))
	for _, selection := range filters.VisualSelections {
		if _, ok := activeVisuals[selection.VisualID]; ok {
			defaults.VisualSelections = append(defaults.VisualSelections, selection)
		}
	}
	return defaults.WithDefaults()
}

func (d *Dashboard) FiltersForPage(pageID string) map[string]FilterDefinition {
	filters := map[string]FilterDefinition{}
	for _, name := range d.PageFilterIDs(pageID) {
		if filter, ok := d.Filters[name]; ok {
			filters[name] = filter
		}
	}
	return filters
}

func (d *Dashboard) FilterConfigForPage(pageID string) []FilterConfig {
	config := []FilterConfig{}
	for _, name := range d.PageFilterIDs(pageID) {
		filter, ok := d.Filters[name]
		if !ok {
			continue
		}
		config = append(config, FilterConfig{ID: name, FilterDefinition: filter})
	}
	return config
}

func (d *Dashboard) PageFilterIDs(pageID string) []string {
	page, ok := d.PageOrDefault(pageID)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	ids := []string{}
	for _, item := range page.Visuals {
		if item.Kind != "filter_card" || item.Filter == "" {
			continue
		}
		if _, ok := seen[item.Filter]; ok {
			continue
		}
		seen[item.Filter] = struct{}{}
		ids = append(ids, item.Filter)
	}
	return ids
}

func (d *Dashboard) PageOrDefault(pageID string) (dashboard.Page, bool) {
	if d == nil || len(d.Pages) == 0 {
		return dashboard.Page{}, false
	}
	if pageID != "" {
		for _, page := range d.Pages {
			if page.ID == pageID {
				return page.WithDefaults(), true
			}
		}
	}
	return d.Pages[0].WithDefaults(), true
}

func (d *Dashboard) pageVisualIDSet(pageID string) map[string]struct{} {
	page, ok := d.PageOrDefault(pageID)
	if !ok {
		return map[string]struct{}{}
	}
	ids := map[string]struct{}{}
	for _, item := range page.Visuals {
		if item.Visual != "" {
			ids[item.Visual] = struct{}{}
		}
	}
	return ids
}

func addStringShape(shape map[string]any, param string) {
	if param != "" {
		shape[param] = ""
	}
}

func addStringParam(params map[string]any, param, value string) {
	value = strings.TrimSpace(value)
	if param != "" && value != "" {
		params[param] = value
	}
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	next := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		next = append(next, value)
	}
	sort.Strings(next)
	return next
}

func sortedFilterNames(filters map[string]FilterDefinition) []string {
	names := make([]string, 0, len(filters))
	for name := range filters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
