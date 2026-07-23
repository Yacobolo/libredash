// Package definition owns immutable compiler-produced dashboard serving state.
package definition

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/Yacobolo/leapview/internal/dashboard"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	"github.com/Yacobolo/leapview/internal/visualization/ir"
)

type FilterDefinition struct {
	Type             string         `json:"type"`
	Label            string         `json:"label"`
	Description      string         `json:"description,omitempty"`
	Dimension        string         `json:"dimension"`
	Fact             string         `json:"fact,omitempty"`
	Default          FilterDefault  `json:"default"`
	Custom           bool           `json:"custom,omitempty"`
	Presets          []FilterPreset `json:"presets,omitempty"`
	Operator         string         `json:"operator,omitempty"`
	Values           FilterValues   `json:"values,omitempty"`
	DefaultOperator  string         `json:"defaultOperator,omitempty"`
	Operators        []string       `json:"operators,omitempty"`
	Options          []FilterOption `json:"options,omitempty"`
	URLParam         string         `json:"urlParam,omitempty"`
	FromURLParam     string         `json:"fromURLParam,omitempty"`
	ToURLParam       string         `json:"toURLParam,omitempty"`
	OperatorURLParam string         `json:"operatorURLParam,omitempty"`
	Targets          FilterTargets  `json:"targets,omitempty"`
}

type FilterConfig struct {
	ID string `json:"id"`
	FilterDefinition
}

type FilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type FilterDefault struct {
	Preset   string   `json:"preset,omitempty"`
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	Operator string   `json:"operator,omitempty"`
	Value    string   `json:"value,omitempty"`
	Values   []string `json:"values,omitempty"`
}
type FilterPreset struct {
	Value        string `json:"value"`
	Label        string `json:"label"`
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	RelativeDays int    `json:"relativeDays,omitempty"`
}
type FilterValues struct {
	Source string `json:"source,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type FilterTargets struct {
	Visuals []string `json:"visuals,omitempty"`
}

func (targets FilterTargets) IsEmpty() bool {
	return len(targets.Visuals) == 0
}
func (targets FilterTargets) Contains(kind, id string) bool {
	if kind != "visual" {
		return false
	}
	for _, value := range targets.Visuals {
		if value == id {
			return true
		}
	}
	return false
}

type Definition struct {
	ID             string                                        `json:"id"`
	Title          string                                        `json:"title"`
	Description    string                                        `json:"description,omitempty"`
	SemanticModel  string                                        `json:"semanticModel"`
	Filters        map[string]FilterDefinition                   `json:"filters"`
	Pages          []dashboard.Page                              `json:"pages"`
	Visualizations map[string]visualizationdefinition.Definition `json:"visualizations"`
}

func New(id, title, description, semanticModel string, filters map[string]FilterDefinition, pages []dashboard.Page, visualizations map[string]visualizationdefinition.Definition) (Definition, error) {
	if id == "" || semanticModel == "" {
		return Definition{}, fmt.Errorf("compiled dashboard requires ID and semantic model")
	}
	return Definition{ID: id, Title: title, Description: description, SemanticModel: semanticModel, Filters: cloneFilters(filters), Pages: append([]dashboard.Page(nil), pages...), Visualizations: cloneVisualizations(visualizations)}, nil
}

func (definition Definition) DefaultFilters() dashboard.Filters {
	filters := dashboard.Filters{}.WithDefaults()
	for name, filter := range definition.Filters {
		control := dashboard.FilterControl{Type: filter.Type, Operator: filter.Operator}
		if filter.DefaultOperator != "" {
			control.Operator = filter.DefaultOperator
		}
		control.Preset, control.From, control.To, control.Value = filter.Default.Preset, filter.Default.From, filter.Default.To, filter.Default.Value
		control.Values = append([]string(nil), filter.Default.Values...)
		filters.Controls[name] = control
	}
	return filters
}

func (definition Definition) PageFilterIDs(pageID string) []string {
	ids, seen := []string{}, map[string]struct{}{}
	for _, page := range definition.Pages {
		if page.ID != pageID {
			continue
		}
		for _, item := range page.Visuals {
			if item.Kind != "filter" || item.Filter == "" {
				continue
			}
			if _, ok := seen[item.Filter]; ok {
				continue
			}
			seen[item.Filter] = struct{}{}
			ids = append(ids, item.Filter)
		}
	}
	sort.Strings(ids)
	return ids
}

func (definition Definition) NormalizeFiltersForPage(pageID string, filters dashboard.Filters) dashboard.Filters {
	filters, defaults, next := filters.WithDefaults(), definition.DefaultFilters(), dashboard.Filters{}.WithDefaults()
	for _, id := range definition.PageFilterIDs(pageID) {
		if control, ok := filters.Controls[id]; ok {
			next.Controls[id] = control
		} else if control, ok := defaults.Controls[id]; ok {
			next.Controls[id] = control
		}
	}
	next.Selections = append([]dashboard.InteractionSelection(nil), filters.Selections...)
	next.SpatialSelections = append([]dashboard.SpatialInteractionSelection(nil), filters.SpatialSelections...)
	return next
}

func (definition Definition) DefaultFiltersForPage(pageID string) dashboard.Filters {
	return definition.NormalizeFiltersForPage(pageID, definition.DefaultFilters())
}
func (definition Definition) PageOrDefault(pageID string) (dashboard.Page, bool) {
	if len(definition.Pages) == 0 {
		return dashboard.Page{}, false
	}
	if pageID != "" {
		for _, page := range definition.Pages {
			if page.ID == pageID {
				return page, true
			}
		}
	}
	return definition.Pages[0], true
}
func (definition Definition) FilterConfigForPage(pageID string) []FilterConfig {
	ids := definition.PageFilterIDs(pageID)
	out := make([]FilterConfig, 0, len(ids))
	for _, id := range ids {
		out = append(out, FilterConfig{ID: id, FilterDefinition: definition.Filters[id]})
	}
	return out
}

func (definition Definition) FiltersFromURLForPage(pageID string, values url.Values) dashboard.Filters {
	allowed := map[string]struct{}{}
	for _, id := range definition.PageFilterIDs(pageID) {
		allowed[id] = struct{}{}
	}
	filters := definition.DefaultFilters()
	for name, filter := range definition.Filters {
		if _, ok := allowed[name]; !ok {
			delete(filters.Controls, name)
			continue
		}
		control := filters.Controls[name]
		first := func(key string) string {
			if key == "" || len(values[key]) == 0 {
				return ""
			}
			return values[key][0]
		}
		switch filter.Type {
		case "date_range":
			if value := first(filter.URLParam); value != "" {
				control.Preset = value
			}
			from, to := first(filter.FromURLParam), first(filter.ToURLParam)
			if from != "" || to != "" {
				control.Preset, control.From, control.To = "custom", from, to
			}
		case "multi_select":
			if filter.URLParam != "" {
				control.Values = uniqueSorted(values[filter.URLParam])
			}
		case "text":
			if value := first(filter.URLParam); value != "" {
				control.Value = value
			}
			if value := first(filter.OperatorURLParam); value != "" && containsString(filter.Operators, value) {
				control.Operator = value
			}
		}
		filters.Controls[name] = control
	}
	return filters.WithDefaults()
}

func (definition Definition) URLParamsFromFiltersForPage(pageID string, filters dashboard.Filters) map[string]any {
	filters, defaults := filters.WithDefaults(), definition.DefaultFilters().WithDefaults()
	allowed := map[string]struct{}{}
	for _, id := range definition.PageFilterIDs(pageID) {
		allowed[id] = struct{}{}
	}
	params := map[string]any{}
	for name, filter := range definition.Filters {
		if _, ok := allowed[name]; !ok {
			continue
		}
		control, def := filters.Controls[name], defaults.Controls[name]
		switch filter.Type {
		case "date_range":
			if control.Preset != "" && control.Preset != def.Preset {
				params[filter.URLParam] = control.Preset
			}
			if control.From != "" && control.From != def.From {
				params[filter.FromURLParam] = control.From
			}
			if control.To != "" && control.To != def.To {
				params[filter.ToURLParam] = control.To
			}
		case "multi_select":
			if len(control.Values) > 0 {
				params[filter.URLParam] = uniqueSorted(control.Values)
			}
		case "text":
			if control.Value != "" {
				params[filter.URLParam] = control.Value
			}
			if control.Operator != "" && control.Operator != def.Operator {
				params[filter.OperatorURLParam] = control.Operator
			}
		}
	}
	return params
}

func (definition Definition) URLParamShapeForPage(pageID string) map[string]any {
	shape := map[string]any{}
	for _, id := range definition.PageFilterIDs(pageID) {
		filter := definition.Filters[id]
		for _, parameter := range []string{filter.URLParam, filter.FromURLParam, filter.ToURLParam, filter.OperatorURLParam} {
			if parameter != "" {
				shape[parameter] = id
			}
		}
	}
	return shape
}

func SpecTitle(spec ir.VisualizationSpec) string {
	base, _ := ir.SpecificationBase(spec)
	return base.Title
}

func TableColumns(spec ir.VisualizationSpec) []dashboard.TableColumn {
	var columns []ir.TableVisualizationColumn
	if value, ok := spec.Value.(*ir.TableVisualizationSpec); ok {
		columns = value.Columns
	} else if base, err := ir.SpecificationBase(spec); err == nil && len(base.Datasets) > 0 {
		for _, field := range base.Datasets[0].Fields {
			columns = append(columns, ir.TableVisualizationColumn{Field: ir.VisualizationFieldRef{Dataset: "primary", Field: field.ID}, Label: field.Label})
		}
	}
	base, _ := ir.SpecificationBase(spec)
	fields := map[string]ir.VisualizationField{}
	if len(base.Datasets) > 0 {
		for _, field := range base.Datasets[0].Fields {
			fields[field.ID] = field
		}
	}
	out := make([]dashboard.TableColumn, len(columns))
	for index, column := range columns {
		out[index] = dashboard.TableColumn{Key: column.Field.Field, Label: column.Label, Formatting: formatting(column.Formatting)}
		if column.Width != nil {
			out[index].Width = int(*column.Width)
		}
		if column.Group != nil {
			out[index].Group = *column.Group
		}
		if column.Measure != nil {
			out[index].Measure = *column.Measure
		}
		if column.ColumnValue != nil {
			out[index].ColumnValue = *column.ColumnValue
		}
		if field, ok := fields[column.Field.Field]; ok {
			if field.Role == ir.VisualizationFieldRoleMeasure {
				out[index].Role, out[index].Align = "measure", "right"
			} else {
				out[index].Role = "row_header"
			}
			out[index].Format = fieldFormat(field)
		}
	}
	return out
}

func MeasureFormatting(spec ir.VisualizationSpec, measures []visualizationdefinition.FieldBinding) map[string][]dashboard.TableFormattingRule {
	values := map[string][]ir.TableVisualizationFormattingRule{}
	switch value := spec.Value.(type) {
	case *ir.MatrixVisualizationSpec:
		values = value.MeasureFormatting
	case *ir.PivotVisualizationSpec:
		values = value.MeasureFormatting
	}
	out := make(map[string][]dashboard.TableFormattingRule, len(values))
	for _, measure := range measures {
		if rules := values[measure.Alias]; len(rules) > 0 {
			out[measure.FieldID] = formatting(rules)
		}
	}
	return out
}

func formatting(values []ir.TableVisualizationFormattingRule) []dashboard.TableFormattingRule {
	out := make([]dashboard.TableFormattingRule, 0, len(values))
	for _, value := range values {
		switch rule := value.Value.(type) {
		case *ir.TableBadgeFormattingRule:
			out = append(out, dashboard.TableFormattingRule{Kind: rule.Kind, Values: cloneStringMap(rule.Values)})
		case *ir.TableTextColorFormattingRule:
			item := dashboard.TableFormattingRule{Kind: rule.Kind, Color: rule.Color, Min: rule.Minimum, Max: rule.Maximum}
			if rule.Values != nil {
				item.Values = cloneStringMap(*rule.Values)
			}
			out = append(out, item)
		case *ir.TableBackgroundScaleFormattingRule:
			item := dashboard.TableFormattingRule{Kind: rule.Kind, Min: rule.Minimum, Max: rule.Maximum}
			if rule.LowColor != nil {
				item.LowColor = *rule.LowColor
			}
			if rule.HighColor != nil {
				item.HighColor = *rule.HighColor
			}
			out = append(out, item)
		case *ir.TableDataBarFormattingRule:
			item := dashboard.TableFormattingRule{Kind: rule.Kind, Min: rule.Minimum, Max: rule.Maximum, Color: rule.Color}
			if rule.Background != nil {
				item.Background = *rule.Background
			}
			out = append(out, item)
		}
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func fieldFormat(field ir.VisualizationField) string {
	if field.Format != nil {
		switch field.Format.Value.(type) {
		case *ir.CurrencyVisualizationFormat:
			return "currency"
		case *ir.DurationVisualizationFormat:
			return "days"
		case *ir.NumberVisualizationFormat:
			if field.DataType == ir.VisualizationDataTypeInteger {
				return "integer"
			}
			return "decimal"
		}
	}
	return "text"
}
func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func cloneFilters(values map[string]FilterDefinition) map[string]FilterDefinition {
	out := make(map[string]FilterDefinition, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
func cloneVisualizations(values map[string]visualizationdefinition.Definition) map[string]visualizationdefinition.Definition {
	out := make(map[string]visualizationdefinition.Definition, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
