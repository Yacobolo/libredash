package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

var supportedAggregations = map[string]struct{}{
	"sum": {}, "count": {}, "count_distinct": {}, "avg": {}, "min": {}, "max": {},
}

var supportedSemanticDimensionTypes = map[string]struct{}{
	"string": {}, "number": {}, "boolean": {}, "date": {}, "timestamp": {},
}

var supportedTimeGrains = map[string]struct{}{
	"day": {}, "week": {}, "month": {}, "quarter": {}, "year": {},
}

func (m *Model) FactNames() []string {
	seen := map[string]struct{}{}
	for _, measure := range m.Measures {
		if measure.Fact != "" {
			seen[measure.Fact] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for fact := range seen {
		out = append(out, fact)
	}
	sort.Strings(out)
	return out
}

func (m *Model) validateSemanticDefinitions() error {
	for name, measure := range m.Measures {
		if err := validateSemanticIdentifier(name); err != nil {
			return fmt.Errorf("semantic model measure %q is invalid: %w", name, err)
		}
		measure.Name = name
		measure.Field = name
		measure.Label = defaultString(measure.Label, titleFromIdentifier(name))
		if err := m.validateMeasure(name, measure); err != nil {
			return err
		}
		m.Measures[name] = measure
	}
	facts := map[string]struct{}{}
	for _, fact := range m.FactNames() {
		facts[fact] = struct{}{}
	}
	for name, dimension := range m.Dimensions {
		if err := validateSemanticIdentifier(name); err != nil {
			return fmt.Errorf("semantic dimension %q is invalid: %w", name, err)
		}
		dimension.Name = name
		dimension.Label = defaultString(dimension.Label, titleFromIdentifier(name))
		if dimension.Timezone == "" {
			dimension.Timezone = "UTC"
		}
		if dimension.Calendar == "" {
			dimension.Calendar = "gregorian"
		}
		if dimension.WeekStart == "" {
			dimension.WeekStart = "monday"
		}
		if _, err := time.LoadLocation(dimension.Timezone); err != nil {
			return fmt.Errorf("semantic dimension %q has invalid timezone %q", name, dimension.Timezone)
		}
		if dimension.Calendar != "gregorian" {
			return fmt.Errorf("semantic dimension %q has unsupported calendar %q", name, dimension.Calendar)
		}
		switch dimension.WeekStart {
		case "monday", "sunday":
		default:
			return fmt.Errorf("semantic dimension %q has unsupported week_start %q", name, dimension.WeekStart)
		}
		if _, ok := supportedSemanticDimensionTypes[dimension.Type]; !ok {
			return fmt.Errorf("semantic dimension %q has unsupported type %q", name, dimension.Type)
		}
		if len(dimension.Grains) > 0 && dimension.Type != "date" && dimension.Type != "timestamp" {
			return fmt.Errorf("semantic dimension %q defines time grains for type %q", name, dimension.Type)
		}
		for _, grain := range dimension.Grains {
			if _, ok := supportedTimeGrains[grain]; !ok {
				return fmt.Errorf("semantic dimension %q has unsupported time grain %q", name, grain)
			}
		}
		if len(dimension.Bindings) == 0 {
			return fmt.Errorf("semantic dimension %q requires bindings", name)
		}
		for fact, binding := range dimension.Bindings {
			if _, ok := facts[fact]; !ok {
				return fmt.Errorf("semantic dimension %q binding references non-fact table %q", name, fact)
			}
			physical, err := m.ResolveDimension(binding.Field)
			if err != nil {
				return fmt.Errorf("semantic dimension %q binding for fact %q: %w", name, fact, err)
			}
			if physicalType := canonicalDimensionType(physical.Type); physicalType != "" && !compatibleDimensionTypes(dimension.Type, physicalType) {
				return fmt.Errorf("semantic dimension %q type %q is incompatible with binding %q type %q", name, dimension.Type, binding.Field, physical.Type)
			}
			if _, err := m.ResolveBindingPath(fact, binding); err != nil {
				return fmt.Errorf("semantic dimension %q binding for fact %q: %w", name, fact, err)
			}
		}
		m.Dimensions[name] = dimension
	}
	return m.validateMetrics()
}

func canonicalDimensionType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case value == "string" || strings.Contains(value, "char") || strings.Contains(value, "text") || value == "uuid":
		return "string"
	case value == "number" || strings.Contains(value, "int") || strings.Contains(value, "decimal") || strings.Contains(value, "numeric") || strings.Contains(value, "double") || strings.Contains(value, "float") || strings.Contains(value, "real"):
		return "number"
	case value == "boolean" || strings.Contains(value, "bool"):
		return "boolean"
	case value == "date":
		return "date"
	case strings.Contains(value, "timestamp") || strings.Contains(value, "datetime"):
		return "timestamp"
	default:
		return ""
	}
}

func compatibleDimensionTypes(canonical, physical string) bool {
	if canonical == physical {
		return true
	}
	return (canonical == "date" || canonical == "timestamp") && (physical == "date" || physical == "timestamp")
}

func (m *Model) validateMeasure(name string, measure MetricMeasure) error {
	if _, ok := m.Tables[measure.Fact]; !ok {
		return fmt.Errorf("semantic measure %q references unknown fact table %q", name, measure.Fact)
	}
	if _, ok := supportedAggregations[measure.Aggregation]; !ok {
		return fmt.Errorf("semantic measure %q has unsupported aggregation %q", name, measure.Aggregation)
	}
	if measure.Empty != "zero" && measure.Empty != "null" {
		return fmt.Errorf("semantic measure %q empty must be zero or null", name)
	}
	if measure.Aggregation == "count" || measure.Aggregation == "count_distinct" {
		if measure.Empty != "zero" {
			return fmt.Errorf("semantic measure %q aggregation %s requires empty: zero", name, measure.Aggregation)
		}
	}
	hasField := strings.TrimSpace(measure.Input.Field) != ""
	hasExpression := strings.TrimSpace(measure.Input.Expression) != ""
	if measure.Aggregation == "count" {
		if hasField || hasExpression {
			return fmt.Errorf("semantic measure %q count must not define input", name)
		}
	} else if hasField == hasExpression {
		return fmt.Errorf("semantic measure %q requires exactly one input field or expression", name)
	}
	refs := []string{}
	if hasField {
		refs = append(refs, measure.Input.Field)
	}
	if hasExpression {
		expression, err := ParseExpression(measure.Input.Expression)
		if err != nil {
			return fmt.Errorf("semantic measure %q input expression: %w", name, err)
		}
		for _, function := range expression.Functions() {
			if function == "safe_divide" {
				return fmt.Errorf("semantic measure %q input expression function %q is metric-only", name, function)
			}
		}
		refs = append(refs, expression.References()...)
	}
	for _, ref := range refs {
		dimension, err := m.ResolveDimension(ref)
		if err != nil {
			return fmt.Errorf("semantic measure %q input: %w", name, err)
		}
		if dimension.Table != measure.Fact {
			return fmt.Errorf("semantic measure %q input field %q is not owned by fact %q", name, ref, measure.Fact)
		}
	}
	for index, filter := range measure.Filters {
		if _, err := m.ResolveDimension(filter.Field); err != nil {
			return fmt.Errorf("semantic measure %q filter %d: %w", name, index, err)
		}
		if err := validateMeasureFilter(filter); err != nil {
			return fmt.Errorf("semantic measure %q filter %d: %w", name, index, err)
		}
		dimension, _ := m.ResolveDimension(filter.Field)
		if _, err := m.SafeRelationshipPath(measure.Fact, dimension.Table); err != nil {
			return fmt.Errorf("semantic measure %q filter %d: %w", name, index, err)
		}
	}
	return nil
}

func validateMeasureFilter(filter MeasureFilter) error {
	switch filter.Operator {
	case "equals", "in", "contains", "starts_with", "greater_than_or_equal", "less_than":
	default:
		return fmt.Errorf("unsupported operator %q", filter.Operator)
	}
	if len(filter.Values) == 0 {
		return fmt.Errorf("filter values are required")
	}
	return nil
}

func (m *Model) validateMetrics() error {
	parsed := map[string]Expression{}
	for name, metric := range m.Metrics {
		if err := validateSemanticIdentifier(name); err != nil {
			return fmt.Errorf("semantic metric %q is invalid: %w", name, err)
		}
		metric.Name = name
		metric.Label = defaultString(metric.Label, titleFromIdentifier(name))
		expression, err := ParseExpression(metric.Expression)
		if err != nil {
			return fmt.Errorf("semantic metric %q: %w", name, err)
		}
		for _, ref := range expression.References() {
			if _, ok := m.Measures[ref]; ok {
				continue
			}
			if _, ok := m.Metrics[ref]; !ok {
				return fmt.Errorf("semantic metric %q references unknown measure or metric %q", name, ref)
			}
		}
		parsed[name] = expression
		m.Metrics[name] = metric
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("semantic metric dependency cycle includes %q", name)
		case 2:
			return nil
		}
		state[name] = 1
		for _, ref := range parsed[name].References() {
			if _, ok := parsed[ref]; ok {
				if err := visit(ref); err != nil {
					return err
				}
			}
		}
		state[name] = 2
		return nil
	}
	for name := range parsed {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func (m *Model) ResolveBindingPath(fact string, binding DimensionBinding) ([]Relationship, error) {
	dimension, err := m.ResolveDimension(binding.Field)
	if err != nil {
		return nil, err
	}
	if len(binding.Path) == 0 {
		return m.SafeRelationshipPath(fact, dimension.Table)
	}
	current := fact
	path := make([]Relationship, 0, len(binding.Path))
	for _, id := range binding.Path {
		relationship, ok := m.RelationshipByID(id)
		if !ok {
			return nil, fmt.Errorf("unknown relationship %q", id)
		}
		fromTable, _, _ := splitSemanticField(relationship.From)
		toTable, _, _ := splitSemanticField(relationship.To)
		switch {
		case current == fromTable:
			current = toTable
		case relationship.Cardinality == "one_to_one" && current == toTable:
			current = fromTable
		default:
			return nil, fmt.Errorf("relationship %q does not safely continue from %q", id, current)
		}
		path = append(path, relationship)
	}
	if current != dimension.Table {
		return nil, fmt.Errorf("relationship path ends at %q, want %q", current, dimension.Table)
	}
	return path, nil
}

func (m *Model) RelationshipByID(id string) (Relationship, bool) {
	for _, relationship := range m.Relationships {
		if relationship.ID == id {
			return relationship, true
		}
	}
	return Relationship{}, false
}
