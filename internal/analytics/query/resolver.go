package query

import (
	"fmt"
	"sort"
	"strings"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
)

type Planner struct {
	Model    *semanticmodel.Model
	Compiled *CompiledModel
}

type tableAlias struct {
	Table string
	Alias string
	Path  []semanticmodel.Relationship
}

type queryView struct {
	Fact       string
	Dimensions map[string]semanticmodel.MetricDimension
	Measures   map[string]ResolvedMeasure
	Paths      map[string][]semanticmodel.Relationship
}

func NewPlanner(model *semanticmodel.Model) *Planner {
	planner, err := NewCompiledPlanner(model)
	if err != nil {
		return &Planner{Model: model}
	}
	return planner
}

func (p *Planner) metricExpression(name string, metric semanticmodel.Metric) (semanticmodel.Expression, error) {
	if p.Compiled != nil {
		if expression, ok := p.Compiled.MetricExpressions[name]; ok {
			return expression, nil
		}
	}
	return semanticmodel.ParseExpression(metric.Expression)
}

func (p *Planner) resolvedMeasure(name string, measure semanticmodel.MetricMeasure) ResolvedMeasure {
	resolved := resolvedMeasureFromSemantic(measure)
	if p.Compiled != nil {
		if expression, ok := p.Compiled.MeasureInputExpressions[name]; ok {
			resolved.InputExpression = &expression
		}
	}
	return resolved
}

type AggregateAnalysis struct {
	Facts          []string
	AtomicMeasures []string
	MultiFact      bool
}

// AnalyzeAggregate exposes the normalized semantic dependencies used by
// higher-level physical optimizers without exposing the planner's mutable
// resolution internals.
func (p *Planner) AnalyzeAggregate(request Request) (AggregateAnalysis, error) {
	resolved, err := p.resolveAggregate(request)
	if err != nil {
		return AggregateAnalysis{}, err
	}
	measures := make([]string, 0, len(resolved.Measures))
	for name := range resolved.Measures {
		measures = append(measures, name)
	}
	sort.Strings(measures)
	return AggregateAnalysis{
		Facts:          append([]string{}, resolved.Facts...),
		AtomicMeasures: measures,
		MultiFact:      resolved.MultiFact,
	}, nil
}

func (p *Planner) queryView(request Request) (*queryView, error) {
	return p.semanticView(request.Table, request.Dimensions, request.Measures, request.Filters, request.Time.Field)
}

func (p *Planner) rowView(request RowRequest) (*queryView, error) {
	if request.Table == "" && len(request.Measures) == 0 {
		return nil, fmt.Errorf("row query requires table when no measure is selected")
	}
	return p.semanticView(request.Table, request.Dimensions, request.Measures, request.Filters, "")
}

func (p *Planner) rawValueView(request RawValueRequest) (*queryView, error) {
	measures := []Field{}
	if request.Measure.Field != "" {
		measures = append(measures, request.Measure)
	}
	return p.semanticView(request.Table, request.Dimensions, measures, request.Filters, "")
}

func (p *Planner) countView(request CountRequest) (*queryView, error) {
	if request.Table == "" {
		return nil, fmt.Errorf("count query requires table")
	}
	return p.semanticView(request.Table, nil, nil, request.Filters, "")
}

func (p *Planner) semanticView(table string, dimensions []Field, measures []Field, filters []Filter, timeField string) (*queryView, error) {
	if p.Model == nil {
		return nil, fmt.Errorf("semantic model is required")
	}
	fact := table
	resolvedMeasures := map[string]ResolvedMeasure{}
	for _, item := range measures {
		if _, ok := p.Model.Metrics[item.Field]; ok {
			return nil, fmt.Errorf("metric %q is aggregate-only", item.Field)
		}
		semanticMeasure, err := p.Model.ResolveMeasure(item.Field)
		if err != nil {
			return nil, err
		}
		measure := p.resolvedMeasure(item.Field, semanticMeasure)
		if fact == "" {
			fact = measure.Fact
		}
		if measure.Fact != fact {
			return nil, fmt.Errorf("cross-fact measures are not supported")
		}
		resolvedMeasures[item.Field] = measure
	}
	if fact == "" {
		return nil, fmt.Errorf("query requires a fact table")
	}
	if _, ok := p.Model.Tables[fact]; !ok {
		return nil, fmt.Errorf("unknown table %q", fact)
	}
	if err := validateSingleFactFilterScope(fact, filters); err != nil {
		return nil, err
	}
	resolvedDimensions := map[string]semanticmodel.MetricDimension{}
	paths := map[string][]semanticmodel.Relationship{}
	for _, item := range dimensions {
		dimension, err := p.Model.ResolveDimension(item.Field)
		if err != nil {
			return nil, err
		}
		if _, err := p.relationshipPath(fact, dimension.Table); err != nil {
			return nil, err
		}
		resolvedDimensions[item.Field] = dimension
		resolvedDimensions[dimension.Field] = dimension
	}
	for _, field := range filterRefs(filters) {
		dimension, path, err := p.resolveViewFilterDimension(fact, field)
		if err != nil {
			return nil, err
		}
		resolvedDimensions[field] = dimension
		resolvedDimensions[dimension.Field] = dimension
		paths[dimension.Field] = path
	}
	if timeField != "" {
		dimension, err := p.Model.ResolveDimension(timeField)
		if err != nil {
			return nil, err
		}
		if _, err := p.relationshipPath(fact, dimension.Table); err != nil {
			return nil, err
		}
		resolvedDimensions[timeField] = dimension
		resolvedDimensions[dimension.Field] = dimension
	}
	return &queryView{
		Fact:       fact,
		Dimensions: resolvedDimensions,
		Measures:   resolvedMeasures,
		Paths:      paths,
	}, nil
}

func validateSingleFactFilterScope(fact string, filters []Filter) error {
	for _, filter := range filters {
		if filter.Fact != "" && filter.Fact != fact {
			return fmt.Errorf("filter fact %q does not match query fact %q", filter.Fact, fact)
		}
		for _, group := range filter.Groups {
			if err := validateSingleFactFilterScope(fact, group.Filters); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Planner) resolveViewFilterDimension(fact, ref string) (semanticmodel.MetricDimension, []semanticmodel.Relationship, error) {
	if semanticDimension, ok := p.Model.Dimensions[ref]; ok {
		binding, ok := semanticDimension.Bindings[fact]
		if !ok {
			return semanticmodel.MetricDimension{}, nil, fmt.Errorf("semantic dimension %q has no binding for fact %q", ref, fact)
		}
		dimension, err := p.Model.ResolveDimension(binding.Field)
		if err != nil {
			return semanticmodel.MetricDimension{}, nil, err
		}
		path, err := p.Model.ResolveBindingPath(fact, binding)
		return dimension, path, err
	}
	dimension, err := p.Model.ResolveDimension(ref)
	if err != nil {
		return semanticmodel.MetricDimension{}, nil, err
	}
	path, err := p.relationshipPath(fact, dimension.Table)
	return dimension, path, err
}

func filterRefs(filters []Filter) []string {
	fields := []string{}
	for _, filter := range filters {
		if filter.Field != "" {
			fields = append(fields, filter.Field)
		}
		for _, group := range filter.Groups {
			fields = append(fields, filterRefs(group.Filters)...)
		}
	}
	return fields
}

func resolvedMeasureFromSemantic(measure semanticmodel.MetricMeasure) ResolvedMeasure {
	filters := make([]MeasureFilter, 0, len(measure.Filters))
	for _, filter := range measure.Filters {
		filters = append(filters, MeasureFilter{Field: filter.Field, Operator: filter.Operator, Values: append([]any{}, filter.Values...)})
	}
	return ResolvedMeasure{
		Field:       measure.Field,
		Name:        measure.Name,
		Label:       measure.Label,
		Description: measure.Description,
		Fact:        measure.Fact,
		Aggregation: measure.Aggregation,
		InputField:  measure.Input.Field,
		InputExpr:   measure.Input.Expression,
		Filters:     filters,
		Empty:       measure.Empty,
		Unit:        measure.Unit,
		Format:      measure.Format,
	}
}

func (s *queryView) ResolveDimensionRef(ref string) (string, semanticmodel.MetricDimension, error) {
	if dimension, ok := s.Dimensions[ref]; ok {
		return dimension.Field, dimension, nil
	}
	return "", semanticmodel.MetricDimension{}, fmt.Errorf("field %q is not exposed", ref)
}

func (s *queryView) ResolveMeasureRef(ref string) (string, ResolvedMeasure, error) {
	if measure, ok := s.Measures[ref]; ok {
		return ref, measure, nil
	}
	return "", ResolvedMeasure{}, fmt.Errorf("field %q is not exposed", ref)
}

func (p *Planner) aliases(view *queryView, fields []string) (map[string]tableAlias, error) {
	aliases := map[string]tableAlias{
		view.Fact: {Table: view.Fact, Alias: "t0"},
	}
	nextAlias := 1
	for _, field := range fields {
		table, _, err := splitField(field)
		if err != nil {
			return nil, err
		}
		if _, ok := aliases[table]; ok {
			continue
		}
		path, ok := view.Paths[field]
		if !ok {
			path, err = p.relationshipPath(view.Fact, table)
			if err != nil {
				return nil, err
			}
		}
		for _, step := range pathTables(view.Fact, path) {
			if _, ok := aliases[step.Table]; ok {
				continue
			}
			aliases[step.Table] = tableAlias{Table: step.Table, Alias: fmt.Sprintf("t%d", nextAlias), Path: step.Path}
			nextAlias++
		}
	}
	return aliases, nil
}

func (p *Planner) relationshipPath(base, target string) ([]semanticmodel.Relationship, error) {
	return p.Model.SafeRelationshipPath(base, target)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func pathTables(base string, path []semanticmodel.Relationship) []tablePath {
	current := base
	tables := []tablePath{}
	for index, relationship := range path {
		fromTable, _, err := splitField(relationship.From)
		if err != nil {
			return tables
		}
		toTable, _, err := splitField(relationship.To)
		if err != nil {
			return tables
		}
		next := ""
		switch {
		case current == fromTable:
			next = toTable
		case relationship.Cardinality == "one_to_one" && current == toTable:
			next = fromTable
		default:
			return tables
		}
		tables = append(tables, tablePath{Table: next, Path: append([]semanticmodel.Relationship{}, path[:index+1]...)})
		current = next
	}
	return tables
}

type tablePath struct {
	Table string
	Path  []semanticmodel.Relationship
}

func splitField(field string) (string, string, error) {
	parts := strings.Split(field, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("field %q must be qualified as table.field", field)
	}
	return parts[0], parts[1], nil
}
