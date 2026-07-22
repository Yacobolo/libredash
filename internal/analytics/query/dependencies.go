package query

import (
	"sort"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
)

// Dependencies is the shared, authorization-safe semantic resolution result.
// It intentionally contains both logical members and their physical lineage so
// callers never need to infer governance scope from user-supplied names.
type Dependencies struct {
	LogicalFields      []string
	MetricDependencies []string
	Facts              []string
	PhysicalFields     []string
	RelationshipPaths  []string
}

func ResolveDependencies(model *semanticmodel.Model, request Request) (Dependencies, error) {
	planner := NewPlanner(model)
	resolved, err := planner.resolveAggregate(request)
	if err != nil {
		return Dependencies{}, err
	}
	logical := map[string]struct{}{}
	metricDependencies := map[string]struct{}{}
	physical := map[string]struct{}{}
	paths := map[string]struct{}{}
	for _, dimension := range resolved.Dimensions {
		logical[dimension.Name] = struct{}{}
		for _, fact := range resolved.Facts {
			field, path, err := planner.aggregateDimensionBinding(fact, dimension)
			if err != nil {
				return Dependencies{}, err
			}
			physical[field] = struct{}{}
			if signature := relationshipPathSignature(path); signature != "" {
				paths[fact+":"+signature] = struct{}{}
			}
			for _, relationship := range path {
				physical[relationship.From] = struct{}{}
				physical[relationship.To] = struct{}{}
			}
		}
	}
	for name, measure := range resolved.Measures {
		logical[name] = struct{}{}
		for _, field := range measurePhysicalFields(measure) {
			physical[field] = struct{}{}
			resolvedField, err := model.ResolveDimension(field)
			if err != nil {
				return Dependencies{}, err
			}
			path, err := model.SafeRelationshipPath(measure.Fact, resolvedField.Table)
			if err != nil {
				return Dependencies{}, err
			}
			if signature := relationshipPathSignature(path); signature != "" {
				paths[measure.Fact+":"+signature] = struct{}{}
			}
			for _, relationship := range path {
				physical[relationship.From] = struct{}{}
				physical[relationship.To] = struct{}{}
			}
		}
	}
	for name, expression := range resolved.Metrics {
		logical[name] = struct{}{}
		for _, ref := range expression.References() {
			metricDependencies[ref] = struct{}{}
		}
	}
	for _, fact := range resolved.Facts {
		bindings, err := planner.factFilterFields(request.Filters, resolved, fact)
		if err != nil {
			return Dependencies{}, err
		}
		for _, binding := range bindings {
			physical[binding.Field] = struct{}{}
			path := binding.Path
			if signature := relationshipPathSignature(path); signature != "" {
				paths[fact+":"+signature] = struct{}{}
			}
			for _, relationship := range path {
				physical[relationship.From] = struct{}{}
				physical[relationship.To] = struct{}{}
			}
		}
	}
	for _, ref := range filterRefs(request.Filters) {
		logical[ref] = struct{}{}
	}
	return Dependencies{
		LogicalFields:      sortedSet(logical),
		MetricDependencies: sortedSet(metricDependencies),
		Facts:              append([]string{}, resolved.Facts...),
		PhysicalFields:     sortedSet(physical),
		RelationshipPaths:  sortedSet(paths),
	}, nil
}

func sortedSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
