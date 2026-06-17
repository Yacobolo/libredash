package semantic

import (
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
	MetricView       string         `yaml:"metrics_view" json:"metricsView"`
	Dimension        string         `yaml:"dimension" json:"dimension"`
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
	MetricView      string         `yaml:"metrics_view"`
	Query           VisualQuery    `yaml:"query"`
	Options         map[string]any `yaml:"options"`
	RendererOptions map[string]any `yaml:"renderer_options"`
	Interaction     Interaction    `yaml:"interaction"`
}

type VisualQuery struct {
	Dimensions []string `yaml:"dimensions"`
	Series     string   `yaml:"series"`
	Measures   []string `yaml:"measures"`
	Sort       []Sort   `yaml:"sort"`
	Limit      int      `yaml:"limit"`
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
	Kind        string                  `yaml:"kind"`
	Title       string                  `yaml:"title"`
	MetricView  string                  `yaml:"metrics_view"`
	DefaultSort dashboard.TableSort     `yaml:"default_sort"`
	Columns     []dashboard.TableColumn `yaml:"columns"`
	Rows        []string                `yaml:"rows"`
	Measures    []string                `yaml:"measures"`
	ColumnDims  []string                `yaml:"-"`
}

func (t *TableVisual) UnmarshalYAML(value *yaml.Node) error {
	type rawTableVisual struct {
		Kind        string              `yaml:"kind"`
		Title       string              `yaml:"title"`
		MetricView  string              `yaml:"metrics_view"`
		DefaultSort dashboard.TableSort `yaml:"default_sort"`
		Rows        []string            `yaml:"rows"`
		Measures    []string            `yaml:"measures"`
	}
	var raw rawTableVisual
	if err := value.Decode(&raw); err != nil {
		return err
	}
	t.Kind = raw.Kind
	t.Title = raw.Title
	t.MetricView = raw.MetricView
	t.DefaultSort = raw.DefaultSort
	t.Rows = raw.Rows
	t.Measures = raw.Measures

	columnsNode := mappingValue(value, "columns")
	if columnsNode == nil {
		return nil
	}
	if columnsNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("table %q columns must be a sequence", raw.Title)
	}
	if len(columnsNode.Content) == 0 {
		return nil
	}
	switch columnsNode.Content[0].Kind {
	case yaml.MappingNode:
		if err := columnsNode.Decode(&t.Columns); err != nil {
			return err
		}
	case yaml.ScalarNode:
		t.ColumnDims = make([]string, 0, len(columnsNode.Content))
		for _, item := range columnsNode.Content {
			t.ColumnDims = append(t.ColumnDims, item.Value)
		}
	default:
		return fmt.Errorf("table %q columns must contain column objects or dimension names", raw.Title)
	}
	return nil
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
	if v.Query.Series != "" {
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
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report Dashboard
	if err := yaml.Unmarshal(bytes, &report); err != nil {
		return nil, err
	}
	if err := rejectLegacyVisualStacked(bytes); err != nil {
		return nil, err
	}
	if err := rejectLegacyKPIs(bytes); err != nil {
		return nil, err
	}
	if err := report.Validate(metricViews); err != nil {
		return nil, err
	}
	return &report, nil
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
			return fmt.Errorf("filter %q requires type, label, metrics_view, and dimension", name)
		}
		view, ok := allowedViews[filter.MetricView]
		if !ok {
			return fmt.Errorf("filter %q references unknown metrics view %q", name, filter.MetricView)
		}
		if _, ok := view.Dimensions[filter.Dimension]; !ok {
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
		if visual.MetricView == "" || (kind != "kpi" && visual.Title == "") || (kind != "kpi" && visual.Type == "") {
			return fmt.Errorf("visual %q requires title, metrics_view, and type", name)
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
		for _, dimension := range visual.Query.Dimensions {
			if _, ok := view.Dimensions[dimension]; !ok {
				return fmt.Errorf("visual %q references unknown dimension %q", name, dimension)
			}
		}
		if visual.Query.Series != "" {
			if _, ok := view.Dimensions[visual.Query.Series]; !ok {
				return fmt.Errorf("visual %q references unknown series dimension %q", name, visual.Query.Series)
			}
			if !supportsSeries(shape) {
				return fmt.Errorf("visual %q shape %q does not support series", name, shape)
			}
			if !rendererTypeSupportsSeries(renderer, visual.Type) {
				return fmt.Errorf("visual %q renderer %q type %q does not support series", name, renderer, visual.Type)
			}
		}
		for _, measure := range visual.Query.Measures {
			if _, ok := view.Measures[measure]; !ok {
				return fmt.Errorf("visual %q references unknown measure %q", name, measure)
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
			if sort.Field != "" && sort.Field != "value" && sort.Field != visual.Query.Series {
				if _, ok := view.Dimensions[sort.Field]; !ok {
					if _, ok := view.Measures[sort.Field]; !ok {
						return fmt.Errorf("visual %q sort references unknown field %q", name, sort.Field)
					}
				}
			}
		}
		if visual.Interaction.Field != "" {
			if _, ok := view.Dimensions[visual.Interaction.Field]; !ok {
				return fmt.Errorf("visual %q interaction references unknown field %q", name, visual.Interaction.Field)
			}
		}
	}
	for name, table := range d.Tables {
		if table.Title == "" || table.MetricView == "" {
			return fmt.Errorf("table %q requires title and metrics_view", name)
		}
		view, ok := allowedViews[table.MetricView]
		if !ok {
			return fmt.Errorf("table %q references unknown metrics view %q", name, table.MetricView)
		}
		switch table.KindOrDefault() {
		case "data_table":
			if len(table.Columns) == 0 {
				return fmt.Errorf("table %q kind data_table requires columns", name)
			}
		case "matrix_table":
			if len(table.Rows) == 0 || len(table.Measures) == 0 {
				return fmt.Errorf("table %q kind matrix_table requires rows and measures", name)
			}
			if len(table.ColumnDims) > 1 {
				return fmt.Errorf("table %q kind matrix_table supports at most one column dimension", name)
			}
			for _, dimension := range append(append([]string{}, table.Rows...), table.ColumnDims...) {
				if _, ok := view.Dimensions[dimension]; !ok {
					return fmt.Errorf("table %q references unknown dimension %q", name, dimension)
				}
			}
			for _, measure := range table.Measures {
				if _, ok := view.Measures[measure]; !ok {
					return fmt.Errorf("table %q references unknown measure %q", name, measure)
				}
			}
		case "pivot_table":
			if len(table.Rows) == 0 || len(table.ColumnDims) != 1 || len(table.Measures) != 1 {
				return fmt.Errorf("table %q kind pivot_table requires rows, one column dimension, and one measure", name)
			}
			for _, dimension := range append(append([]string{}, table.Rows...), table.ColumnDims...) {
				if _, ok := view.Dimensions[dimension]; !ok {
					return fmt.Errorf("table %q references unknown dimension %q", name, dimension)
				}
			}
			if _, ok := view.Measures[table.Measures[0]]; !ok {
				return fmt.Errorf("table %q references unknown measure %q", name, table.Measures[0])
			}
		default:
			return fmt.Errorf("table %q has unsupported kind %q", name, table.Kind)
		}
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
		if visual.Query.Series != "" {
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
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape category_value does not support series", name)
		}
	case "category_series_value":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_series_value requires exactly one query dimension", name)
		}
		if visual.Query.Series == "" {
			return fmt.Errorf("visual %q shape category_series_value requires query series", name)
		}
	case "category_multi_measure":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_multi_measure requires exactly one query dimension", name)
		}
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape category_multi_measure does not support series", name)
		}
	case "category_delta":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_delta requires exactly one query dimension", name)
		}
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape category_delta does not support series", name)
		}
	case "binned_measure":
		if len(visual.Query.Dimensions) != 0 {
			return fmt.Errorf("visual %q shape binned_measure does not support query dimensions", name)
		}
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape binned_measure does not support series", name)
		}
	case "hierarchy":
		if len(visual.Query.Dimensions) == 0 {
			return fmt.Errorf("visual %q shape hierarchy requires at least one query dimension", name)
		}
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape hierarchy does not support series", name)
		}
	case "single_value":
		if len(visual.Query.Dimensions) > 1 {
			return fmt.Errorf("visual %q shape single_value supports at most one query dimension", name)
		}
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape single_value does not support series", name)
		}
	case "matrix":
		if len(visual.Query.Dimensions) != 2 {
			return fmt.Errorf("visual %q shape matrix requires exactly two query dimensions", name)
		}
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape matrix does not support series", name)
		}
	case "graph":
		if len(visual.Query.Dimensions) != 2 {
			return fmt.Errorf("visual %q shape graph requires exactly two query dimensions", name)
		}
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape graph does not support series", name)
		}
	case "geo":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape geo requires exactly one query dimension", name)
		}
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape geo does not support series", name)
		}
	case "ohlc":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape ohlc requires exactly one query dimension", name)
		}
		if visual.Query.Series != "" {
			return fmt.Errorf("visual %q shape ohlc does not support series", name)
		}
	case "distribution":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape distribution requires exactly one query dimension", name)
		}
		if visual.Query.Series != "" {
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
