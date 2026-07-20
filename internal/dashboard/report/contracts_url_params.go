package report

import (
	"github.com/Yacobolo/leapview/internal/dashboard"
	"net/url"
	"sort"
)

func (d *Dashboard) validateFilterURLParams() error {
	seen := map[string]struct{}{}
	for _, filter := range d.Filters {
		for _, param := range filterURLParams(filter) {
			if param == "" {
				continue
			}
			if _, exists := seen[param]; exists {
				return errDuplicateFilterURLParam(param)
			}
			seen[param] = struct{}{}
		}
	}
	return nil
}

func filterURLParams(filter FilterDefinition) []string {
	return []string{filter.URLParam, filter.FromURLParam, filter.ToURLParam, filter.OperatorURLParam}
}

func errDuplicateFilterURLParam(param string) error {
	return &duplicateFilterURLParamError{Param: param}
}

type duplicateFilterURLParamError struct {
	Param string
}

func (e *duplicateFilterURLParamError) Error() string {
	return "filter URL param " + e.Param + " duplicates another filter URL param"
}

func (d *Dashboard) DefaultFilters() dashboard.Filters {
	filters := dashboard.Filters{}.WithDefaults()
	for name, filter := range d.Filters {
		filters.Controls[name] = defaultFilterControl(filter)
	}
	return filters
}

func defaultFilterControl(filter FilterDefinition) dashboard.FilterControl {
	control := dashboard.FilterControl{Type: filter.Type, Operator: filter.Operator}
	if filter.DefaultOperator != "" {
		control.Operator = filter.DefaultOperator
	}
	control.Preset = filter.Default.Preset
	control.From = filter.Default.From
	control.To = filter.Default.To
	control.Value = filter.Default.Value
	control.Values = append([]string{}, filter.Default.Values...)
	return control
}

func (d *Dashboard) DefaultFiltersForPage(pageID string) dashboard.Filters {
	return d.NormalizeFiltersForPage(pageID, d.DefaultFilters())
}

func (d *Dashboard) NormalizeFiltersForPage(pageID string, filters dashboard.Filters) dashboard.Filters {
	filters = filters.WithDefaults()
	allowed := map[string]struct{}{}
	for _, id := range d.PageFilterIDs(pageID) {
		allowed[id] = struct{}{}
	}
	next := dashboard.Filters{}.WithDefaults()
	for id := range allowed {
		if control, ok := filters.Controls[id]; ok {
			next.Controls[id] = control
		} else if control, ok := d.DefaultFilters().Controls[id]; ok {
			next.Controls[id] = control
		}
	}
	next.Selections = append([]dashboard.InteractionSelection{}, filters.Selections...)
	return next
}

func (d *Dashboard) FiltersFromURL(values url.Values) dashboard.Filters {
	return d.filtersFromURL(values, nil)
}

func (d *Dashboard) FiltersFromURLForPage(pageID string, values url.Values) dashboard.Filters {
	allowed := map[string]struct{}{}
	for _, id := range d.PageFilterIDs(pageID) {
		allowed[id] = struct{}{}
	}
	return d.filtersFromURL(values, allowed)
}

func (d *Dashboard) filtersFromURL(values url.Values, allowed map[string]struct{}) dashboard.Filters {
	filters := d.DefaultFilters()
	for name, filter := range d.Filters {
		if allowed != nil {
			if _, ok := allowed[name]; !ok {
				delete(filters.Controls, name)
				continue
			}
		}
		control := filters.Controls[name]
		switch filter.Type {
		case "date_range":
			control = dateFilterFromURL(control, filter, values)
		case "multi_select":
			control = multiSelectFilterFromURL(control, filter, values)
		case "text":
			control = textFilterFromURL(control, filter, values)
		}
		filters.Controls[name] = control
	}
	return filters.WithDefaults()
}

func dateFilterFromURL(control dashboard.FilterControl, filter FilterDefinition, values url.Values) dashboard.FilterControl {
	if value := first(values, filter.URLParam); value != "" {
		control.Preset = value
	}
	from := first(values, filter.FromURLParam)
	to := first(values, filter.ToURLParam)
	if from != "" || to != "" {
		control.Preset = "custom"
		control.From = from
		control.To = to
	}
	return control
}

func multiSelectFilterFromURL(control dashboard.FilterControl, filter FilterDefinition, values url.Values) dashboard.FilterControl {
	if filter.URLParam != "" {
		control.Values = uniqueSorted(values[filter.URLParam])
	}
	return control
}

func textFilterFromURL(control dashboard.FilterControl, filter FilterDefinition, values url.Values) dashboard.FilterControl {
	if value := first(values, filter.URLParam); value != "" {
		control.Value = value
	}
	if value := first(values, filter.OperatorURLParam); value != "" && filterAllowsOperator(filter, value) {
		control.Operator = value
	}
	return control
}

func filterAllowsOperator(filter FilterDefinition, operator string) bool {
	for _, candidate := range filter.Operators {
		if candidate == operator {
			return true
		}
	}
	return false
}

func (d *Dashboard) URLParamsFromFilters(filters dashboard.Filters) map[string]any {
	return d.urlParamsFromFilters("", filters)
}

func (d *Dashboard) URLParamsFromFiltersForPage(pageID string, filters dashboard.Filters) map[string]any {
	return d.urlParamsFromFilters(pageID, filters)
}

func (d *Dashboard) urlParamsFromFilters(pageID string, filters dashboard.Filters) map[string]any {
	filters = filters.WithDefaults()
	defaults := d.DefaultFilters().WithDefaults()
	allowed := map[string]struct{}{}
	if pageID != "" {
		for _, id := range d.PageFilterIDs(pageID) {
			allowed[id] = struct{}{}
		}
	}
	params := map[string]any{}
	for name, filter := range d.Filters {
		if pageID != "" {
			if _, ok := allowed[name]; !ok {
				continue
			}
		}
		control := filters.Controls[name]
		def := defaults.Controls[name]
		switch filter.Type {
		case "date_range":
			encodeDateFilterURLParams(params, filter, control, def)
		case "multi_select":
			encodeMultiSelectFilterURLParams(params, filter, control)
		case "text":
			encodeTextFilterURLParams(params, filter, control, def)
		}
	}
	return params
}

func encodeDateFilterURLParams(params map[string]any, filter FilterDefinition, control dashboard.FilterControl, def dashboard.FilterControl) {
	if control.Preset != "" && control.Preset != def.Preset {
		params[filter.URLParam] = control.Preset
	}
	if control.From != "" && control.From != def.From {
		params[filter.FromURLParam] = control.From
	}
	if control.To != "" && control.To != def.To {
		params[filter.ToURLParam] = control.To
	}
}

func encodeMultiSelectFilterURLParams(params map[string]any, filter FilterDefinition, control dashboard.FilterControl) {
	if len(control.Values) > 0 {
		params[filter.URLParam] = uniqueSorted(control.Values)
	}
}

func encodeTextFilterURLParams(params map[string]any, filter FilterDefinition, control dashboard.FilterControl, def dashboard.FilterControl) {
	if control.Value != "" {
		params[filter.URLParam] = control.Value
	}
	if control.Operator != "" && control.Operator != def.Operator {
		params[filter.OperatorURLParam] = control.Operator
	}
}

func (d *Dashboard) PageFilterIDs(pageID string) []string {
	ids := []string{}
	seen := map[string]struct{}{}
	for _, page := range d.Pages {
		if page.ID != pageID {
			continue
		}
		for _, visual := range page.Visuals {
			if visual.Kind != "filter_card" || visual.Filter == "" {
				continue
			}
			if _, ok := seen[visual.Filter]; ok {
				continue
			}
			seen[visual.Filter] = struct{}{}
			ids = append(ids, visual.Filter)
		}
	}
	sort.Strings(ids)
	return ids
}

func (d *Dashboard) FiltersForPage(pageID string) map[string]FilterDefinition {
	filters := map[string]FilterDefinition{}
	for _, id := range d.PageFilterIDs(pageID) {
		if filter, ok := d.Filters[id]; ok {
			filters[id] = filter
		}
	}
	return filters
}

func (d *Dashboard) FilterConfigForPage(pageID string) []FilterConfig {
	ids := d.PageFilterIDs(pageID)
	config := make([]FilterConfig, 0, len(ids))
	for _, id := range ids {
		filter := d.Filters[id]
		config = append(config, FilterConfig{ID: id, FilterDefinition: filter})
	}
	return config
}

func (d *Dashboard) PageOrDefault(pageID string) (dashboard.Page, bool) {
	if len(d.Pages) == 0 {
		return dashboard.Page{}, false
	}
	if pageID != "" {
		for _, page := range d.Pages {
			if page.ID == pageID {
				return page, true
			}
		}
	}
	return d.Pages[0], true
}

func (d *Dashboard) URLParamShapeForPage(pageID string) map[string]any {
	shape := map[string]any{}
	for _, id := range d.PageFilterIDs(pageID) {
		filter := d.Filters[id]
		if filter.URLParam != "" {
			shape[filter.URLParam] = id
		}
		if filter.FromURLParam != "" {
			shape[filter.FromURLParam] = id
		}
		if filter.ToURLParam != "" {
			shape[filter.ToURLParam] = id
		}
		if filter.OperatorURLParam != "" {
			shape[filter.OperatorURLParam] = id
		}
	}
	return shape
}

func first(values url.Values, key string) string {
	if key == "" {
		return ""
	}
	items := values[key]
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
