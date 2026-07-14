package report

import (
	"fmt"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"strings"

	"gopkg.in/yaml.v3"
)

type Dashboard struct {
	ID            string                      `yaml:"id"`
	Title         string                      `yaml:"title"`
	Description   string                      `yaml:"description"`
	SemanticModel string                      `yaml:"semantic_model"`
	Filters       map[string]FilterDefinition `yaml:"filters"`
	Visuals       map[string]Visual           `yaml:"visuals"`
	Tables        map[string]TableVisual      `yaml:"tables"`
	Pages         []dashboard.Page            `yaml:"pages"`
}

type FilterDefinition struct {
	Type             string         `yaml:"type" json:"type"`
	Label            string         `yaml:"label" json:"label"`
	Description      string         `yaml:"description" json:"description,omitempty"`
	Dimension        string         `yaml:"field" json:"dimension"`
	Fact             string         `yaml:"fact" json:"fact,omitempty"`
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
	Targets          FilterTargets  `yaml:"targets" json:"targets,omitempty"`
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
	Title           string            `yaml:"title"`
	Description     string            `yaml:"description"`
	Kind            string            `yaml:"kind"`
	Shape           string            `yaml:"shape"`
	Renderer        string            `yaml:"renderer"`
	Type            string            `yaml:"type"`
	Query           VisualQuery       `yaml:"query"`
	Options         map[string]any    `yaml:"options"`
	RendererOptions map[string]any    `yaml:"renderer_options"`
	Interaction     Interaction       `yaml:"interaction"`
	Encode          map[string]string `yaml:"encode"`
}

type VisualQuery struct {
	Table      string     `yaml:"table"`
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
	if value.Kind == yaml.ScalarNode {
		f.Field = value.Value
		f.Alias = fieldRefAlias(value.Value)
		return nil
	}
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

func (q *VisualQuery) UnmarshalYAML(value *yaml.Node) error {
	type raw VisualQuery
	var out raw
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("visual query must be a mapping")
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		key := value.Content[index].Value
		item := value.Content[index+1]
		switch key {
		case "table":
			if err := item.Decode(&out.Table); err != nil {
				return err
			}
		case "metric_view":
			return fmt.Errorf("metric_view is not supported; use dashboard semantic_model")
		case "dimensions":
			fields, err := decodeFieldRefs(item)
			if err != nil {
				return fmt.Errorf("dimensions: %w", err)
			}
			out.Dimensions = fields
		case "series":
			if err := item.Decode(&out.Series); err != nil {
				return err
			}
		case "measures":
			fields, err := decodeMeasureRefs(item)
			if err != nil {
				return fmt.Errorf("measures: %w", err)
			}
			out.Measures = fields
		case "time":
			if err := item.Decode(&out.Time); err != nil {
				return err
			}
		case "sort":
			if err := item.Decode(&out.Sort); err != nil {
				return err
			}
		case "limit":
			if err := item.Decode(&out.Limit); err != nil {
				return err
			}
		default:
			return fmt.Errorf("field %s not found in type report.VisualQuery", key)
		}
	}
	*q = VisualQuery(out)
	return nil
}

func decodeFieldRefs(node *yaml.Node) ([]FieldRef, error) {
	switch node.Kind {
	case yaml.SequenceNode:
		fields := []FieldRef{}
		if err := node.Decode(&fields); err != nil {
			return nil, err
		}
		return fields, nil
	case yaml.MappingNode:
		fields := make([]FieldRef, 0, len(node.Content)/2)
		for index := 0; index+1 < len(node.Content); index += 2 {
			alias := node.Content[index].Value
			field := ""
			item := node.Content[index+1]
			if item.Kind == yaml.ScalarNode {
				field = item.Value
			} else {
				var payload struct {
					Field string `yaml:"field"`
				}
				if err := item.Decode(&payload); err != nil {
					return nil, err
				}
				field = payload.Field
			}
			if field == "" {
				return nil, fmt.Errorf("field %q is empty", alias)
			}
			fields = append(fields, FieldRef{Field: field, Alias: alias})
		}
		return fields, nil
	default:
		return nil, fmt.Errorf("must be a sequence or mapping")
	}
}

func decodeMeasureRefs(node *yaml.Node) ([]FieldRef, error) {
	switch node.Kind {
	case yaml.SequenceNode:
		fields := []FieldRef{}
		if err := node.Decode(&fields); err != nil {
			return nil, err
		}
		return fields, nil
	case yaml.MappingNode:
		fields := make([]FieldRef, 0, len(node.Content)/2)
		for index := 0; index+1 < len(node.Content); index += 2 {
			alias := node.Content[index].Value
			item := node.Content[index+1]
			field := alias
			if item.Kind != yaml.ScalarNode || item.Tag != "!!null" {
				var payload struct {
					Measure string `yaml:"measure"`
					Expr    string `yaml:"expr"`
				}
				if err := item.Decode(&payload); err != nil {
					return nil, err
				}
				if payload.Measure != "" {
					field = payload.Measure
				} else if payload.Expr != "" {
					return nil, fmt.Errorf("inline dashboard measures are not supported; define %q in the semantic model", alias)
				}
			}
			fields = append(fields, FieldRef{Field: field, Alias: alias})
		}
		return fields, nil
	default:
		return nil, fmt.Errorf("must be a sequence or mapping")
	}
}

func fieldRefAlias(field string) string {
	if field == "" {
		return ""
	}
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
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
	PointSelection SelectionInteraction `yaml:"point_selection" json:"pointSelection,omitempty"`
	RowSelection   SelectionInteraction `yaml:"row_selection" json:"rowSelection,omitempty"`
}

type FilterTargets struct {
	Visuals []string `yaml:"visuals" json:"visuals,omitempty"`
	Tables  []string `yaml:"tables" json:"tables,omitempty"`
}

type SelectionInteraction struct {
	Toggle   bool               `yaml:"toggle" json:"toggle,omitempty"`
	Mappings []SelectionMapping `yaml:"mappings" json:"mappings,omitempty"`
	Targets  []string           `yaml:"targets" json:"targets,omitempty"`
}

type SelectionMapping struct {
	Field string `yaml:"field" json:"field"`
	Fact  string `yaml:"fact" json:"fact,omitempty"`
	Grain string `yaml:"grain" json:"grain,omitempty"`
	Value string `yaml:"value" json:"value"`
	Label string `yaml:"label" json:"label,omitempty"`
}

func (s *SelectionInteraction) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("selection interaction must be a mapping")
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		key := value.Content[index].Value
		item := value.Content[index+1]
		switch key {
		case "toggle":
			if err := item.Decode(&s.Toggle); err != nil {
				return err
			}
		case "mappings":
			if err := item.Decode(&s.Mappings); err != nil {
				return err
			}
		case "targets":
			if err := item.Decode(&s.Targets); err != nil {
				return err
			}
		default:
			return fmt.Errorf("field %s not found in type report.SelectionInteraction", key)
		}
	}
	return nil
}

func (i *Interaction) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return nil
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("interaction must be a mapping")
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		key := value.Content[index].Value
		item := value.Content[index+1]
		switch key {
		case "point_selection":
			if err := item.Decode(&i.PointSelection); err != nil {
				return err
			}
		case "row_selection":
			if err := item.Decode(&i.RowSelection); err != nil {
				return err
			}
		default:
			return fmt.Errorf("field %s not found in type report.Interaction", key)
		}
	}
	return nil
}

func (s SelectionInteraction) IsZero() bool {
	return !s.Toggle && len(s.Mappings) == 0 && len(s.Targets) == 0
}

type TableVisual struct {
	Kind              string                                     `yaml:"kind"`
	Cardinality       string                                     `yaml:"cardinality"`
	Title             string                                     `yaml:"title"`
	Description       string                                     `yaml:"description"`
	Query             TableQuery                                 `yaml:"query"`
	DefaultSort       dashboard.TableSort                        `yaml:"default_sort"`
	Style             dashboard.TableStyle                       `yaml:"style"`
	Columns           []dashboard.TableColumn                    `yaml:"columns"`
	Interaction       Interaction                                `yaml:"interaction"`
	Rows              []string                                   `yaml:"-"`
	Measures          []string                                   `yaml:"-"`
	MeasureFormatting map[string][]dashboard.TableFormattingRule `yaml:"measure_formatting"`
	DataColumns       []FieldRef                                 `yaml:"-"`
	ColumnDims        []string                                   `yaml:"-"`
}

const (
	TableCardinalityBounded = "bounded"
	TableCardinalityExact   = "exact"
)

func (t TableVisual) CardinalityOrDefault() string {
	if t.Cardinality == "" {
		return TableCardinalityBounded
	}
	return t.Cardinality
}

type TableQuery struct {
	Table    string     `yaml:"table"`
	Fields   []string   `yaml:"fields"`
	Columns  []FieldRef `yaml:"columns"`
	Rows     []FieldRef `yaml:"rows"`
	Measures []FieldRef `yaml:"measures"`
}

func (q *TableQuery) UnmarshalYAML(value *yaml.Node) error {
	type raw TableQuery
	var out raw
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("table query must be a mapping")
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		key := value.Content[index].Value
		item := value.Content[index+1]
		switch key {
		case "metric_view":
			return fmt.Errorf("metric_view is not supported; use dashboard semantic_model")
		case "table":
			if err := item.Decode(&out.Table); err != nil {
				return err
			}
		case "fields":
			if err := item.Decode(&out.Fields); err != nil {
				return err
			}
		case "columns":
			fields, err := decodeFieldRefs(item)
			if err != nil {
				return fmt.Errorf("columns: %w", err)
			}
			out.Columns = fields
		case "rows":
			fields, err := decodeFieldRefs(item)
			if err != nil {
				return fmt.Errorf("rows: %w", err)
			}
			out.Rows = fields
		case "measures":
			fields, err := decodeMeasureRefs(item)
			if err != nil {
				return fmt.Errorf("measures: %w", err)
			}
			out.Measures = fields
		default:
			return fmt.Errorf("field %s not found in type report.TableQuery", key)
		}
	}
	*q = TableQuery(out)
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
