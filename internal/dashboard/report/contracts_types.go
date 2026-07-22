package report

import (
	"fmt"
	"github.com/Yacobolo/leapview/internal/dashboard"
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
	Title           string              `yaml:"title"`
	Description     string              `yaml:"description"`
	Kind            string              `yaml:"-" json:"-"`
	Shape           string              `yaml:"-" json:"-"`
	Renderer        string              `yaml:"-" json:"-"`
	Type            string              `yaml:"type"`
	Query           VisualQuery         `yaml:"query"`
	Presentation    VisualPresentation  `yaml:"presentation" json:"presentation"`
	Accessibility   VisualAccessibility `yaml:"accessibility" json:"accessibility"`
	DataBudget      VisualDataBudget    `yaml:"data_budget" json:"dataBudget"`
	Geo             VisualGeo           `yaml:"geo" json:"geo"`
	Custom          VisualCustom        `yaml:"custom" json:"custom"`
	Options         map[string]any      `yaml:"-" json:"-"`
	RendererOptions map[string]any      `yaml:"-" json:"-"`
	Interaction     Interaction         `yaml:"interaction"`
	Encode          map[string]string   `yaml:"-" json:"-"`
}

type VisualPresentation struct {
	Legend        string            `yaml:"legend" json:"legend,omitempty"`
	ShowLabels    bool              `yaml:"show_labels" json:"showLabels,omitempty"`
	Stacked       bool              `yaml:"stacked" json:"stacked,omitempty"`
	Smooth        bool              `yaml:"smooth" json:"smooth,omitempty"`
	ShowSymbols   *bool             `yaml:"show_symbols" json:"showSymbols,omitempty"`
	DataZoom      bool              `yaml:"data_zoom" json:"dataZoom,omitempty"`
	Area          *bool             `yaml:"area" json:"area,omitempty"`
	Step          bool              `yaml:"step" json:"step,omitempty"`
	Orientation   string            `yaml:"orientation" json:"orientation,omitempty"`
	LabelPosition string            `yaml:"label_position" json:"labelPosition,omitempty"`
	SymbolSize    float64           `yaml:"symbol_size" json:"symbolSize,omitempty"`
	HistogramBins int               `yaml:"histogram_bins" json:"histogramBins,omitempty"`
	SeriesTypes   map[string]string `yaml:"series_types" json:"seriesTypes,omitempty"`
	DualAxis      bool              `yaml:"dual_axis" json:"dualAxis,omitempty"`
	Rose          bool              `yaml:"rose" json:"rose,omitempty"`
	CenterLabel   string            `yaml:"center_label" json:"centerLabel,omitempty"`
	InnerRadius   float64           `yaml:"inner_radius" json:"innerRadius,omitempty"`
	OuterRadius   float64           `yaml:"outer_radius" json:"outerRadius,omitempty"`
	Align         string            `yaml:"align" json:"align,omitempty"`
	Sort          string            `yaml:"sort" json:"sort,omitempty"`
	InitialDepth  int               `yaml:"initial_depth" json:"initialDepth,omitempty"`
	Basemap       string            `yaml:"basemap" json:"basemap,omitempty"`
	Roam          bool              `yaml:"roam" json:"roam,omitempty"`
	Layout        string            `yaml:"layout" json:"layout,omitempty"`
	Breadcrumb    *bool             `yaml:"breadcrumb" json:"breadcrumb,omitempty"`
	NodeGap       float64           `yaml:"node_gap" json:"nodeGap,omitempty"`
	Curveness     float64           `yaml:"curveness" json:"curveness,omitempty"`
	Focus         string            `yaml:"focus" json:"focus,omitempty"`
	Minimum       *float64          `yaml:"minimum" json:"minimum,omitempty"`
	Maximum       *float64          `yaml:"maximum" json:"maximum,omitempty"`
	ProgressWidth float64           `yaml:"progress_width" json:"progressWidth,omitempty"`
	Thresholds    []VisualThreshold `yaml:"thresholds" json:"thresholds,omitempty"`
	Note          string            `yaml:"note" json:"note,omitempty"`
	Tone          string            `yaml:"tone" json:"tone,omitempty"`
}

type VisualThreshold struct {
	Value float64 `yaml:"value" json:"value"`
	Tone  string  `yaml:"tone" json:"tone"`
}

type VisualAccessibility struct {
	Title           string `yaml:"title"`
	Description     string `yaml:"description"`
	Summary         string `yaml:"summary"`
	AnnounceChanges bool   `yaml:"announce_changes"`
}

type VisualDataBudget struct {
	MaxRows              int    `yaml:"max_rows"`
	RequiredCompleteness string `yaml:"required_completeness"`
}

type VisualGeo struct {
	Basemap      string            `yaml:"basemap" json:"basemap"`
	Theme        string            `yaml:"theme" json:"theme"`
	LabelDensity string            `yaml:"label_density" json:"labelDensity"`
	Camera       VisualGeoCamera   `yaml:"camera" json:"camera"`
	Controls     VisualGeoControls `yaml:"controls" json:"controls"`
	Layers       []VisualGeoLayer  `yaml:"layers" json:"layers"`
}

type VisualGeoLayer struct {
	ID            string              `yaml:"id" json:"id"`
	Kind          string              `yaml:"kind" json:"kind"`
	GeometryAsset string              `yaml:"geometry_asset" json:"geometryAsset,omitempty"`
	Join          string              `yaml:"join" json:"join,omitempty"`
	Value         string              `yaml:"value" json:"value,omitempty"`
	Category      string              `yaml:"category" json:"category,omitempty"`
	Label         string              `yaml:"label" json:"label,omitempty"`
	Tooltip       []string            `yaml:"tooltip" json:"tooltip,omitempty"`
	Latitude      string              `yaml:"latitude" json:"latitude,omitempty"`
	Longitude     string              `yaml:"longitude" json:"longitude,omitempty"`
	Path          string              `yaml:"path" json:"path,omitempty"`
	Order         string              `yaml:"order" json:"order,omitempty"`
	Position      string              `yaml:"position" json:"position,omitempty"`
	Visibility    VisualGeoVisibility `yaml:"visibility" json:"visibility"`
	Size          VisualGeoSizeScale  `yaml:"size" json:"size"`
	Color         VisualGeoColorScale `yaml:"color" json:"color"`
	Stroke        VisualGeoStroke     `yaml:"stroke" json:"stroke"`
	Cluster       VisualGeoCluster    `yaml:"cluster" json:"cluster"`
	Heat          VisualGeoHeatStyle  `yaml:"heat" json:"heat"`
	Line          VisualGeoLineStyle  `yaml:"line" json:"line"`
	Opacity       float64             `yaml:"opacity" json:"opacity,omitempty"`
}

type VisualGeoCamera struct {
	Mode        string    `yaml:"mode" json:"mode"`
	Center      []float64 `yaml:"center" json:"center,omitempty"`
	Zoom        *float64  `yaml:"zoom" json:"zoom,omitempty"`
	Padding     int       `yaml:"padding" json:"padding,omitempty"`
	MinimumZoom float64   `yaml:"min_zoom" json:"minimumZoom,omitempty"`
	MaximumZoom float64   `yaml:"max_zoom" json:"maximumZoom,omitempty"`
}

type VisualGeoControls struct {
	Zoom    bool `yaml:"zoom" json:"zoom"`
	Reset   bool `yaml:"reset" json:"reset"`
	Compass bool `yaml:"compass" json:"compass"`
}

type VisualGeoVisibility struct {
	MinimumZoom float64 `yaml:"min_zoom" json:"minimumZoom,omitempty"`
	MaximumZoom float64 `yaml:"max_zoom" json:"maximumZoom,omitempty"`
}

type VisualGeoSizeScale struct {
	MinimumRadius float64  `yaml:"minimum_radius" json:"minimumRadius,omitempty"`
	MaximumRadius float64  `yaml:"maximum_radius" json:"maximumRadius,omitempty"`
	DomainMinimum *float64 `yaml:"domain_minimum" json:"domainMinimum,omitempty"`
	DomainMaximum *float64 `yaml:"domain_maximum" json:"domainMaximum,omitempty"`
}

type VisualGeoColorScale struct {
	Kind           string   `yaml:"kind" json:"kind,omitempty"`
	Palette        string   `yaml:"palette" json:"palette,omitempty"`
	Reverse        bool     `yaml:"reverse" json:"reverse,omitempty"`
	DomainMinimum  *float64 `yaml:"domain_minimum" json:"domainMinimum,omitempty"`
	DomainMidpoint *float64 `yaml:"domain_midpoint" json:"domainMidpoint,omitempty"`
	DomainMaximum  *float64 `yaml:"domain_maximum" json:"domainMaximum,omitempty"`
	NullColor      string   `yaml:"null_color" json:"nullColor,omitempty"`
}

type VisualGeoStroke struct {
	Color   string  `yaml:"color" json:"color,omitempty"`
	Width   float64 `yaml:"width" json:"width,omitempty"`
	Opacity float64 `yaml:"opacity" json:"opacity,omitempty"`
}

type VisualGeoCluster struct {
	Enabled       bool `yaml:"enabled" json:"enabled"`
	Radius        int  `yaml:"radius" json:"radius,omitempty"`
	MaximumZoom   int  `yaml:"max_zoom" json:"maximumZoom,omitempty"`
	MinimumPoints int  `yaml:"minimum_points" json:"minimumPoints,omitempty"`
	ShowCount     bool `yaml:"show_count" json:"showCount"`
}

type VisualGeoHeatStyle struct {
	Radius    float64 `yaml:"radius" json:"radius,omitempty"`
	Intensity float64 `yaml:"intensity" json:"intensity,omitempty"`
}

type VisualGeoLineStyle struct {
	Width     float64 `yaml:"width" json:"width,omitempty"`
	Curvature float64 `yaml:"curvature" json:"curvature,omitempty"`
}

type VisualCustom struct {
	Engine  string         `yaml:"engine"`
	Program map[string]any `yaml:"program"`
}

type VisualQuery struct {
	Table      string     `yaml:"table" json:"table,omitempty"`
	Dimensions []FieldRef `yaml:"dimensions" json:"dimensions,omitempty"`
	Series     FieldRef   `yaml:"series" json:"series,omitempty"`
	Measures   []FieldRef `yaml:"measures" json:"measures,omitempty"`
	Time       QueryTime  `yaml:"time" json:"time,omitempty"`
	Sort       []Sort     `yaml:"sort" json:"sort,omitempty"`
	Limit      int        `yaml:"limit" json:"limit,omitempty"`
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
	Field     string `yaml:"field" json:"field"`
	Direction string `yaml:"direction" json:"direction"`
	Expr      string `yaml:"expr" json:"expr,omitempty"`
}

type Interaction struct {
	PointSelection   SelectionInteraction        `yaml:"point_selection" json:"pointSelection,omitempty"`
	RowSelection     SelectionInteraction        `yaml:"row_selection" json:"rowSelection,omitempty"`
	SpatialSelection SpatialSelectionInteraction `yaml:"spatial_selection" json:"spatialSelection,omitempty"`
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

type SpatialSelectionInteraction struct {
	Gestures  []string                `yaml:"gestures" json:"gestures"`
	Latitude  SpatialSelectionMapping `yaml:"latitude" json:"latitude"`
	Longitude SpatialSelectionMapping `yaml:"longitude" json:"longitude"`
	Targets   []string                `yaml:"targets" json:"targets"`
}

type SpatialSelectionMapping struct {
	Source string `yaml:"source" json:"source"`
	Field  string `yaml:"field" json:"field"`
	Fact   string `yaml:"fact" json:"fact,omitempty"`
}

func (s SpatialSelectionInteraction) IsZero() bool {
	return len(s.Gestures) == 0 && s.Latitude == (SpatialSelectionMapping{}) && s.Longitude == (SpatialSelectionMapping{}) && len(s.Targets) == 0
}

func (s *SpatialSelectionInteraction) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("spatial selection interaction must be a mapping")
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		key := value.Content[index].Value
		item := value.Content[index+1]
		switch key {
		case "gestures":
			if err := item.Decode(&s.Gestures); err != nil {
				return err
			}
		case "latitude":
			if err := item.Decode(&s.Latitude); err != nil {
				return err
			}
		case "longitude":
			if err := item.Decode(&s.Longitude); err != nil {
				return err
			}
		case "targets":
			if err := item.Decode(&s.Targets); err != nil {
				return err
			}
		default:
			return fmt.Errorf("field %s not found in type report.SpatialSelectionInteraction", key)
		}
	}
	return nil
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
		case "spatial_selection":
			if err := item.Decode(&i.SpatialSelection); err != nil {
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
	Style             dashboard.TableStyle                       `yaml:"presentation"`
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
	if v.Type == "kpi" {
		return "kpi"
	}
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
	case "tree", "treemap", "sunburst":
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
	if v.Type == "map" {
		return "maplibre"
	}
	if v.Type == "custom" {
		return "vega-lite-sandbox"
	}
	return "echarts"
}

func (v Visual) CoreOptions() map[string]any {
	options := copyMap(v.Options)
	presentation := v.Presentation
	set := func(key string, value any, include bool) {
		if include {
			options[key] = value
		}
	}
	set("legend", presentation.Legend, presentation.Legend != "")
	set("show_labels", presentation.ShowLabels, presentation.ShowLabels)
	set("stacked", presentation.Stacked, presentation.Stacked)
	set("smooth", presentation.Smooth, presentation.Smooth)
	if presentation.ShowSymbols != nil {
		options["show_symbols"] = *presentation.ShowSymbols
	}
	set("data_zoom", presentation.DataZoom, presentation.DataZoom)
	if presentation.Area != nil {
		options["area"] = *presentation.Area
	}
	set("step", presentation.Step, presentation.Step)
	set("orientation", presentation.Orientation, presentation.Orientation != "")
	set("label_position", presentation.LabelPosition, presentation.LabelPosition != "")
	set("symbol_size", presentation.SymbolSize, presentation.SymbolSize > 0)
	set("bin_count", presentation.HistogramBins, presentation.HistogramBins > 0)
	set("series_types", presentation.SeriesTypes, len(presentation.SeriesTypes) > 0)
	set("dual_axis", presentation.DualAxis, presentation.DualAxis)
	set("rose_type", "radius", presentation.Rose)
	set("center_label", presentation.CenterLabel, presentation.CenterLabel != "")
	set("inner_radius", presentation.InnerRadius, presentation.InnerRadius > 0)
	set("outer_radius", presentation.OuterRadius, presentation.OuterRadius > 0)
	set("align", presentation.Align, presentation.Align != "")
	set("sort", presentation.Sort, presentation.Sort != "")
	set("initial_depth", presentation.InitialDepth, presentation.InitialDepth > 0)
	set("roam", presentation.Roam, presentation.Roam)
	set("layout", presentation.Layout, presentation.Layout != "")
	if presentation.Breadcrumb != nil {
		options["breadcrumb"] = *presentation.Breadcrumb
	}
	set("node_gap", presentation.NodeGap, presentation.NodeGap > 0)
	set("curveness", presentation.Curveness, presentation.Curveness > 0)
	set("focus", presentation.Focus, presentation.Focus != "")
	if presentation.Minimum != nil {
		options["min"] = *presentation.Minimum
	}
	if presentation.Maximum != nil {
		options["max"] = *presentation.Maximum
	}
	set("progress_width", presentation.ProgressWidth, presentation.ProgressWidth > 0)
	set("note", presentation.Note, presentation.Note != "")
	set("tone", presentation.Tone, presentation.Tone != "")
	if len(presentation.Thresholds) > 0 {
		thresholds := make([]map[string]any, len(presentation.Thresholds))
		for index, threshold := range presentation.Thresholds {
			thresholds[index] = map[string]any{"value": threshold.Value, "tone": threshold.Tone}
		}
		options["thresholds"] = thresholds
	}
	return options
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
