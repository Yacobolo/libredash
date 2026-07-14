package stream

import (
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/command"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dashboard/reportmodel"
)

// aggregateProjectionGroups identifies authored grouped visuals that contain
// every additive atomic dependency of a targeted scalar. This is only a
// scheduling hint: the data runtime independently governs both branches and
// rechecks exact scope equality before any projection is allowed.
func aggregateProjectionGroups(definition reportdef.Dashboard, model *semanticmodel.Model, filters dashboard.Filters, plan command.RefreshPlan) ([][]command.Target, map[string]bool) {
	if model == nil {
		return nil, nil
	}
	visualTargets := make(map[string]command.Target)
	for _, target := range plan.Targets {
		if target.Kind == command.TargetVisual {
			visualTargets[target.ID] = target
		}
	}
	consumed := map[string]bool{}
	groups := [][]command.Target{}
	for _, sourceTarget := range plan.Targets {
		if sourceTarget.Kind != command.TargetVisual || consumed[sourceTarget.ID] {
			continue
		}
		source, ok := definition.Visuals[sourceTarget.ID]
		if !ok || source.ShapeOrDefault() != "category_multi_measure" || len(source.Query.Measures) == 0 || len(source.Query.Dimensions) == 0 && source.Query.Time.Field == "" {
			continue
		}
		selected := map[string]bool{}
		for _, measure := range source.Query.Measures {
			if atomic, exists := model.Measures[measure.Field]; exists && (atomic.Aggregation == "count" || atomic.Aggregation == "sum") {
				selected[measure.Field] = true
			}
		}
		members := map[string]bool{sourceTarget.ID: true}
		sourceFacts, factsErr := reportmodel.TargetFacts(&definition, model, "visual", sourceTarget.ID)
		sourceScope := source
		sourceScope.Query.Limit = 0
		sourceKey, sourceScopeErr := singleValueCompatibilityKey(definition, model, filters, sourceTarget.ID, sourceScope)
		for _, target := range plan.Targets {
			if target.Kind != command.TargetVisual || target.ID == sourceTarget.ID || consumed[target.ID] {
				continue
			}
			scalar, exists := definition.Visuals[target.ID]
			if !exists || scalar.ShapeOrDefault() != "single_value" || len(scalar.Query.Dimensions) != 0 || scalar.Query.Time.Field != "" || len(scalar.Query.Measures) != 1 {
				continue
			}
			dependencies, additive, err := semanticquery.AdditiveMeasureDependencies(model, scalar.Query.Measures[0].Field)
			if err != nil || !additive || !containsAllMeasures(selected, dependencies) {
				continue
			}
			sourceScope := source
			sourceScope.Query.Limit = 0
			scalarScope := scalar
			scalarScope.Query.Limit = 0
			sourceKey, sourceErr := singleValueCompatibilityKey(definition, model, filters, sourceTarget.ID, sourceScope)
			scalarKey, scalarErr := singleValueCompatibilityKey(definition, model, filters, target.ID, scalarScope)
			if sourceErr != nil || scalarErr != nil || sourceKey != scalarKey {
				continue
			}
			members[target.ID] = true
		}
		// Other grouped visuals with the same participating facts and exact
		// effective governed scope can share the per-fact expanded grouping scan.
		// The runtime still independently governs every branch and rechecks the
		// facts, filters, and masks before compiling the bundle.
		if factsErr == nil && sourceScopeErr == nil {
			for _, target := range plan.Targets {
				if target.Kind != command.TargetVisual || target.ID == sourceTarget.ID || members[target.ID] || consumed[target.ID] {
					continue
				}
				candidate, exists := definition.Visuals[target.ID]
				if !exists || candidate.ShapeOrDefault() == "single_value" || !bundleEligibleVisual(candidate) {
					continue
				}
				candidateFacts, err := reportmodel.TargetFacts(&definition, model, "visual", target.ID)
				if err != nil || strings.Join(candidateFacts, ",") != strings.Join(sourceFacts, ",") {
					continue
				}
				candidateScope := candidate
				candidateScope.Query.Limit = 0
				candidateKey, err := singleValueCompatibilityKey(definition, model, filters, target.ID, candidateScope)
				if err == nil && candidateKey == sourceKey {
					members[target.ID] = true
				}
			}
		}
		if len(members) < 2 {
			continue
		}
		group := make([]command.Target, 0, len(members))
		for _, target := range plan.Targets {
			if target.Kind == command.TargetVisual && members[target.ID] {
				group = append(group, visualTargets[target.ID])
				consumed[target.ID] = true
			}
		}
		groups = append(groups, group)
	}
	return groups, consumed
}

func containsAllMeasures(selected map[string]bool, required []string) bool {
	for _, measure := range required {
		if !selected[measure] {
			return false
		}
	}
	return true
}
