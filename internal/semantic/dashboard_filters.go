package semantic

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

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
