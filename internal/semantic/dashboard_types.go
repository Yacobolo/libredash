package semantic

import (
	"fmt"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"gopkg.in/yaml.v3"
)

type Dashboard struct {
	ID          string                      `yaml:"id"`
	Title       string                      `yaml:"title"`
	Description string                      `yaml:"description"`
	MetricViews []string                    `yaml:"metric_views"`
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
