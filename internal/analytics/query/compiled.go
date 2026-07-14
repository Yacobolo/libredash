package query

import (
	"fmt"
	"sort"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
)

// CompiledModel is the immutable semantic metadata shared by every query in a
// serving-state runtime. Expressions and dependency DAGs are parsed once at
// activation instead of being rediscovered for every dashboard consumer.
type CompiledModel struct {
	Model                   *semanticmodel.Model
	MetricExpressions       map[string]semanticmodel.Expression
	MeasureInputExpressions map[string]semanticmodel.Expression
	MemberFacts             map[string][]string
}

func CompileModel(model *semanticmodel.Model) (*CompiledModel, error) {
	if model == nil {
		return nil, fmt.Errorf("semantic model is required")
	}
	compiled := &CompiledModel{
		Model:                   model,
		MetricExpressions:       make(map[string]semanticmodel.Expression, len(model.Metrics)),
		MeasureInputExpressions: map[string]semanticmodel.Expression{},
		MemberFacts:             map[string][]string{},
	}
	for name, measure := range model.Measures {
		compiled.MemberFacts[name] = []string{measure.Fact}
		if measure.Input.Expression == "" {
			continue
		}
		expression, err := semanticmodel.ParseExpression(measure.Input.Expression)
		if err != nil {
			return nil, fmt.Errorf("measure %q: %w", name, err)
		}
		compiled.MeasureInputExpressions[name] = expression
	}
	for name, metric := range model.Metrics {
		expression, err := semanticmodel.ParseExpression(metric.Expression)
		if err != nil {
			return nil, fmt.Errorf("metric %q: %w", name, err)
		}
		compiled.MetricExpressions[name] = expression
	}
	visiting := map[string]bool{}
	var factsFor func(string) ([]string, error)
	factsFor = func(name string) ([]string, error) {
		if facts, ok := compiled.MemberFacts[name]; ok {
			return append([]string{}, facts...), nil
		}
		expression, ok := compiled.MetricExpressions[name]
		if !ok {
			return nil, fmt.Errorf("unknown aggregate member %q", name)
		}
		if visiting[name] {
			return nil, fmt.Errorf("metric dependency cycle includes %q", name)
		}
		visiting[name] = true
		facts := map[string]bool{}
		for _, dependency := range expression.References() {
			dependencyFacts, err := factsFor(dependency)
			if err != nil {
				delete(visiting, name)
				return nil, fmt.Errorf("metric %q: %w", name, err)
			}
			for _, fact := range dependencyFacts {
				facts[fact] = true
			}
		}
		delete(visiting, name)
		resolved := make([]string, 0, len(facts))
		for fact := range facts {
			resolved = append(resolved, fact)
		}
		sort.Strings(resolved)
		compiled.MemberFacts[name] = resolved
		return append([]string{}, resolved...), nil
	}
	metricNames := make([]string, 0, len(model.Metrics))
	for name := range model.Metrics {
		metricNames = append(metricNames, name)
	}
	sort.Strings(metricNames)
	for _, name := range metricNames {
		if _, err := factsFor(name); err != nil {
			return nil, err
		}
	}
	return compiled, nil
}

func NewCompiledPlanner(model *semanticmodel.Model) (*Planner, error) {
	compiled, err := CompileModel(model)
	if err != nil {
		return nil, err
	}
	return &Planner{Model: model, Compiled: compiled}, nil
}
