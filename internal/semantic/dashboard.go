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
	ID            string                      `yaml:"id"`
	Title         string                      `yaml:"title"`
	Description   string                      `yaml:"description"`
	SemanticModel string                      `yaml:"semantic_model"`
	Filters       map[string]FilterDefinition `yaml:"filters"`
	KPIs          map[string]KPI              `yaml:"kpis"`
	Visuals       map[string]Visual           `yaml:"visuals"`
	Tables        map[string]TableVisual      `yaml:"tables"`
	Pages         []dashboard.Page            `yaml:"pages"`
}

type FilterDefinition struct {
	Type             string         `yaml:"type" json:"type"`
	Label            string         `yaml:"label" json:"label"`
	Dataset          string         `yaml:"dataset" json:"dataset"`
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

type KPI struct {
	Title   string `yaml:"title"`
	Dataset string `yaml:"dataset"`
	Measure string `yaml:"measure"`
	Note    string `yaml:"note"`
	Tone    string `yaml:"tone"`
}

type Visual struct {
	Title       string      `yaml:"title"`
	Type        string      `yaml:"type"`
	Stacked     bool        `yaml:"stacked"`
	Dataset     string      `yaml:"dataset"`
	Query       VisualQuery `yaml:"query"`
	Interaction Interaction `yaml:"interaction"`
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
	Title       string                  `yaml:"title"`
	Dataset     string                  `yaml:"dataset"`
	DefaultSort dashboard.TableSort     `yaml:"default_sort"`
	Columns     []dashboard.TableColumn `yaml:"columns"`
}

func LoadDashboard(path string, model *Model) (*Dashboard, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report Dashboard
	if err := yaml.Unmarshal(bytes, &report); err != nil {
		return nil, err
	}
	if err := report.Validate(model); err != nil {
		return nil, err
	}
	return &report, nil
}

func (d *Dashboard) Validate(model *Model) error {
	if d.ID == "" || d.Title == "" || d.SemanticModel == "" {
		return fmt.Errorf("dashboard requires id, title, and semantic_model")
	}
	if model == nil {
		return fmt.Errorf("dashboard %q requires semantic model %q", d.ID, d.SemanticModel)
	}
	if d.SemanticModel != model.Name {
		return fmt.Errorf("dashboard %q semantic_model %q does not match model %q", d.ID, d.SemanticModel, model.Name)
	}
	if len(d.KPIs) == 0 {
		return fmt.Errorf("dashboard %q requires kpis", d.ID)
	}
	if len(d.Visuals) == 0 {
		return fmt.Errorf("dashboard %q requires visuals", d.ID)
	}
	if len(d.Pages) == 0 {
		return fmt.Errorf("dashboard %q requires pages", d.ID)
	}
	for name, filter := range d.Filters {
		if filter.Type == "" || filter.Label == "" || filter.Dataset == "" || filter.Dimension == "" {
			return fmt.Errorf("filter %q requires type, label, dataset, and dimension", name)
		}
		dataset, ok := model.Datasets[filter.Dataset]
		if !ok {
			return fmt.Errorf("filter %q references unknown dataset %q", name, filter.Dataset)
		}
		if _, ok := dataset.Dimensions[filter.Dimension]; !ok {
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
	for name, kpi := range d.KPIs {
		if kpi.Title == "" || kpi.Dataset == "" || kpi.Measure == "" {
			return fmt.Errorf("kpi %q requires title, dataset, and measure", name)
		}
		dataset, ok := model.Datasets[kpi.Dataset]
		if !ok {
			return fmt.Errorf("kpi %q references unknown dataset %q", name, kpi.Dataset)
		}
		if _, ok := dataset.Measures[kpi.Measure]; !ok {
			return fmt.Errorf("kpi %q references unknown measure %q", name, kpi.Measure)
		}
	}
	for name, visual := range d.Visuals {
		if visual.Title == "" || visual.Dataset == "" || visual.Type == "" {
			return fmt.Errorf("visual %q requires title, dataset, and type", name)
		}
		dataset, ok := model.Datasets[visual.Dataset]
		if !ok {
			return fmt.Errorf("visual %q references unknown dataset %q", name, visual.Dataset)
		}
		if !supportsChartType(visual.Type) {
			return fmt.Errorf("visual %q has unsupported type %q", name, visual.Type)
		}
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q requires exactly one query dimension", name)
		}
		if len(visual.Query.Measures) != 1 {
			return fmt.Errorf("visual %q requires exactly one query measure", name)
		}
		for _, dimension := range visual.Query.Dimensions {
			if _, ok := dataset.Dimensions[dimension]; !ok {
				return fmt.Errorf("visual %q references unknown dimension %q", name, dimension)
			}
		}
		if visual.Query.Series != "" {
			if _, ok := dataset.Dimensions[visual.Query.Series]; !ok {
				return fmt.Errorf("visual %q references unknown series dimension %q", name, visual.Query.Series)
			}
			if !supportsSeries(visual.Type) {
				return fmt.Errorf("visual %q type %q does not support series", name, visual.Type)
			}
		}
		for _, measure := range visual.Query.Measures {
			if _, ok := dataset.Measures[measure]; !ok {
				return fmt.Errorf("visual %q references unknown measure %q", name, measure)
			}
		}
		for _, sort := range visual.Query.Sort {
			if sort.Field == "" && sort.Expr == "" {
				return fmt.Errorf("visual %q has sort missing field or expr", name)
			}
			if sort.Field != "" && sort.Field != "value" && sort.Field != visual.Query.Series {
				if _, ok := dataset.Dimensions[sort.Field]; !ok {
					if _, ok := dataset.Measures[sort.Field]; !ok {
						return fmt.Errorf("visual %q sort references unknown field %q", name, sort.Field)
					}
				}
			}
		}
		if visual.Interaction.Field != "" {
			if _, ok := dataset.Dimensions[visual.Interaction.Field]; !ok {
				return fmt.Errorf("visual %q interaction references unknown field %q", name, visual.Interaction.Field)
			}
		}
	}
	for name, table := range d.Tables {
		if table.Title == "" || table.Dataset == "" || len(table.Columns) == 0 {
			return fmt.Errorf("table %q requires title, dataset, and columns", name)
		}
		if _, ok := model.Datasets[table.Dataset]; !ok {
			return fmt.Errorf("table %q references unknown dataset %q", name, table.Dataset)
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
			case "header", "kpi_strip":
			case "line_chart", "area_chart", "bar_chart", "column_chart", "pie_chart", "donut_chart", "scatter_chart", "funnel_chart", "treemap_chart", "gauge_chart":
				if visual.Visual == "" {
					return fmt.Errorf("page %q visual %q requires visual", page.ID, visual.ID)
				}
				if _, ok := d.Visuals[visual.Visual]; !ok {
					return fmt.Errorf("page %q references unknown visual %q", page.ID, visual.Visual)
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
	filters := dashboard.Filters{
		Controls:         map[string]dashboard.FilterControl{},
		VisualSelections: []dashboard.VisualSelection{},
	}
	for name, filter := range d.Filters {
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
	filters := d.DefaultFilters()
	for _, name := range sortedFilterNames(d.Filters) {
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
	shape := map[string]any{}
	for _, name := range sortedFilterNames(d.Filters) {
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
	params := map[string]any{}
	defaults := d.DefaultFilters()
	filters = filters.WithDefaults()
	for _, name := range sortedFilterNames(d.Filters) {
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
