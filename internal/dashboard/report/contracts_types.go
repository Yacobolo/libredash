package report

import (
	"fmt"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
	"strings"

	"gopkg.in/yaml.v3"
)

type Dashboard struct {
	ID                string                                `yaml:"id"`
	Title             string                                `yaml:"title"`
	Description       string                                `yaml:"description"`
	SemanticModel     string                                `yaml:"semantic_model"`
	FilterDefinitions map[string]dashboardfilter.Definition `yaml:"filter_definitions,omitempty"`
	FilterBindings    map[string]dashboardfilter.Binding    `yaml:"filter_bindings,omitempty"`
	FilterApplication dashboardfilter.ApplicationPolicy     `yaml:"filter_application,omitempty"`
	Visuals           map[string]AuthoringVisualization     `yaml:"visuals"`
	Pages             []dashboard.Page                      `yaml:"pages"`
}

// AuthoringVisualization is the closed visualization union used from YAML
// loading through compilation. Exactly one variant is populated.
type AuthoringVisualization struct {
	Type    string
	Chart   *Visual
	Tabular *TableVisual
}

func (v *AuthoringVisualization) UnmarshalYAML(value *yaml.Node) error {
	var discriminator struct {
		Type string `yaml:"type"`
	}
	if err := value.Decode(&discriminator); err != nil {
		return err
	}
	if discriminator.Type == "" {
		return fmt.Errorf("visualization requires type")
	}
	v.Type = discriminator.Type
	switch discriminator.Type {
	case "table", "matrix", "pivot":
		if err := rejectUnknownVisualizationFields(value, map[string]struct{}{
			"type": {}, "title": {}, "description": {}, "cardinality": {}, "query": {}, "default_sort": {},
			"presentation": {}, "columns": {}, "interaction": {}, "measure_formatting": {},
		}); err != nil {
			return err
		}
		var definition TableVisual
		if err := value.Decode(&definition); err != nil {
			return err
		}
		v.Tabular = &definition
	default:
		if err := rejectUnknownVisualizationFields(value, map[string]struct{}{
			"type": {}, "title": {}, "description": {}, "query": {}, "presentation": {}, "accessibility": {},
			"data_budget": {}, "interaction": {}, "geo": {}, "custom": {},
		}); err != nil {
			return err
		}
		var definition Visual
		if err := value.Decode(&definition); err != nil {
			return err
		}
		definition.Type = discriminator.Type
		v.Chart = &definition
	}
	return nil
}

func rejectUnknownVisualizationFields(value *yaml.Node, allowed map[string]struct{}) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("visual must be a mapping")
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		field := value.Content[index].Value
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("unsupported visualization property %q", field)
		}
	}
	return nil
}

func ChartVisualization(value Visual) AuthoringVisualization {
	return AuthoringVisualization{Type: value.Type, Chart: &value}
}

func TabularVisualization(kind string, value TableVisual) AuthoringVisualization {
	return AuthoringVisualization{Type: kind, Tabular: &value}
}

func ChartVisualizations(values map[string]Visual) map[string]AuthoringVisualization {
	result := make(map[string]AuthoringVisualization, len(values))
	for id, value := range values {
		result[id] = ChartVisualization(value)
	}
	return result
}

func TabularVisualizations(kind string, values map[string]TableVisual) map[string]AuthoringVisualization {
	result := make(map[string]AuthoringVisualization, len(values))
	for id, value := range values {
		result[id] = TabularVisualization(kind, value)
	}
	return result
}

func MergeVisualizations(groups ...map[string]AuthoringVisualization) map[string]AuthoringVisualization {
	result := map[string]AuthoringVisualization{}
	for _, group := range groups {
		for id, value := range group {
			result[id] = value
		}
	}
	return result
}

type Visual struct {
	Title         string              `yaml:"title"`
	Description   string              `yaml:"description"`
	Type          string              `yaml:"type"`
	Query         VisualQuery         `yaml:"query"`
	Presentation  VisualPresentation  `yaml:"presentation" json:"presentation"`
	Accessibility VisualAccessibility `yaml:"accessibility" json:"accessibility"`
	DataBudget    VisualDataBudget    `yaml:"data_budget" json:"dataBudget"`
	Geo           VisualGeo           `yaml:"geo" json:"geo"`
	Custom        VisualCustom        `yaml:"custom" json:"custom"`
	Interaction   Interaction         `yaml:"interaction"`
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

func (v Visual) KindOrDefault() string {
	if capability, ok := VisualizationCapabilityForType(v.Type); ok {
		return capability.Kind
	}
	return "chart"
}

func (v Visual) ResultShape() string {
	capability, ok := VisualizationCapabilityForType(v.Type)
	if ok && capability.SupportsSeries && !v.Query.Series.IsZero() {
		return "category_series_value"
	}
	if ok {
		return capability.ResultShape
	}
	return "category_value"
}

func (v Visual) ownedRenderer() string {
	if capability, ok := VisualizationCapabilityForType(v.Type); ok {
		return capability.Renderer
	}
	return ""
}
