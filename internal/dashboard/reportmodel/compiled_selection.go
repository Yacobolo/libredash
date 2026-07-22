package reportmodel

import (
	"fmt"
	"strings"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

// ResolveCompiledSelectionInteraction resolves the semantic types of the
// compiler-owned IR mappings without reconstructing authoring dashboard models.
func ResolveCompiledSelectionInteraction(definition *dashboarddefinition.Definition, model *semanticmodel.Model, sourceKind, sourceID string) (ResolvedSelectionInteraction, error) {
	if sourceKind != "visual" {
		return ResolvedSelectionInteraction{}, fmt.Errorf("unknown source kind %q", sourceKind)
	}
	source, ok := definition.Visualizations[sourceID]
	if !ok {
		return ResolvedSelectionInteraction{}, fmt.Errorf("unknown source visualization %q", sourceID)
	}
	base, err := visualizationir.SpecificationBase(source.Spec)
	if err != nil {
		return ResolvedSelectionInteraction{}, err
	}
	if len(base.Interactions) == 0 {
		return ResolvedSelectionInteraction{}, fmt.Errorf("visualization %q has no selection interaction", sourceID)
	}
	interaction := base.Interactions[0]
	resolved := ResolvedSelectionInteraction{Mappings: make([]ResolvedSelectionMapping, 0, len(interaction.Mappings))}
	for index, mapping := range interaction.Mappings {
		item, err := resolveCompiledMapping(model, mapping)
		if err != nil {
			return ResolvedSelectionInteraction{}, fmt.Errorf("visualization %q interaction mapping %d: %w", sourceID, index, err)
		}
		resolved.Mappings = append(resolved.Mappings, item)
	}
	for _, targetID := range interaction.Targets {
		target, ok := definition.Visualizations[targetID]
		if !ok {
			return ResolvedSelectionInteraction{}, fmt.Errorf("interaction references unknown target %q", targetID)
		}
		kind := "visual"
		if target.Query.Kind == visualizationdefinition.QueryDetail || target.Query.Kind == visualizationdefinition.QueryMatrix || target.Query.Kind == visualizationdefinition.QueryPivot {
			kind = "table"
		}
		resolved.Targets = append(resolved.Targets, ResolvedSelectionTarget{Kind: kind, ID: targetID})
	}
	return resolved, nil
}

// ResolveCompiledSpatialSelectionInteraction resolves compiler-owned
// geographic mappings without reconstructing the authoring dashboard.
func ResolveCompiledSpatialSelectionInteraction(definition *dashboarddefinition.Definition, model *semanticmodel.Model, sourceID, interactionID string) (ResolvedSpatialSelectionInteraction, error) {
	source, ok := definition.Visualizations[sourceID]
	if !ok {
		return ResolvedSpatialSelectionInteraction{}, fmt.Errorf("unknown source visualization %q", sourceID)
	}
	spec, ok := source.Spec.Value.(*visualizationir.GeographicVisualizationSpec)
	if !ok {
		if value, valueOK := source.Spec.Value.(visualizationir.GeographicVisualizationSpec); valueOK {
			spec = &value
		} else {
			return ResolvedSpatialSelectionInteraction{}, fmt.Errorf("visualization %q is not geographic", sourceID)
		}
	}
	var interaction *visualizationir.VisualizationSpatialSelectionInteraction
	for index := range spec.SpatialInteractions {
		if spec.SpatialInteractions[index].ID == interactionID {
			interaction = &spec.SpatialInteractions[index]
			break
		}
	}
	if interaction == nil {
		return ResolvedSpatialSelectionInteraction{}, fmt.Errorf("visualization %q has no spatial interaction %q", sourceID, interactionID)
	}
	resolve := func(mapping visualizationir.VisualizationSpatialFieldMapping) (ResolvedSelectionMapping, error) {
		compiled := visualizationir.VisualizationInteractionMapping{TargetFieldID: mapping.TargetFieldID, TargetFactID: mapping.TargetFactID}
		resolved, err := resolveCompiledMapping(model, compiled)
		if err != nil {
			return ResolvedSelectionMapping{}, err
		}
		if resolved.Type != "number" {
			return ResolvedSelectionMapping{}, fmt.Errorf("field %q must be numeric", resolved.Field)
		}
		return resolved, nil
	}
	latitude, err := resolve(interaction.Latitude)
	if err != nil {
		return ResolvedSpatialSelectionInteraction{}, fmt.Errorf("visualization %q spatial latitude: %w", sourceID, err)
	}
	longitude, err := resolve(interaction.Longitude)
	if err != nil {
		return ResolvedSpatialSelectionInteraction{}, fmt.Errorf("visualization %q spatial longitude: %w", sourceID, err)
	}
	resolved := ResolvedSpatialSelectionInteraction{Latitude: latitude, Longitude: longitude}
	for _, targetID := range interaction.Targets {
		target, ok := definition.Visualizations[targetID]
		if !ok {
			return ResolvedSpatialSelectionInteraction{}, fmt.Errorf("spatial interaction references unknown target %q", targetID)
		}
		kind := "visual"
		if target.Query.Kind == visualizationdefinition.QueryDetail || target.Query.Kind == visualizationdefinition.QueryMatrix || target.Query.Kind == visualizationdefinition.QueryPivot {
			kind = "table"
		}
		resolved.Targets = append(resolved.Targets, ResolvedSelectionTarget{Kind: kind, ID: targetID})
	}
	return resolved, nil
}

func resolveCompiledMapping(model *semanticmodel.Model, mapping visualizationir.VisualizationInteractionMapping) (ResolvedSelectionMapping, error) {
	field, fact, grain := mapping.TargetFieldID, "", ""
	if mapping.TargetFactID != nil {
		fact = *mapping.TargetFactID
	}
	if mapping.Grain != nil {
		grain = *mapping.Grain
	}
	if !strings.Contains(field, ".") {
		dimension, err := model.ResolveSemanticDimension(field)
		if err != nil {
			return ResolvedSelectionMapping{}, err
		}
		return ResolvedSelectionMapping{Field: field, Grain: grain, Type: dimension.Type, Scope: SelectionScopeConformed}, nil
	}
	if fact == "" {
		return ResolvedSelectionMapping{}, fmt.Errorf("physical field %q requires fact", field)
	}
	dimension, err := model.ResolveDimension(field)
	if err != nil {
		return ResolvedSelectionMapping{}, err
	}
	if err := model.CanReachField(fact, field); err != nil {
		return ResolvedSelectionMapping{}, err
	}
	return ResolvedSelectionMapping{Field: field, Fact: fact, Grain: grain, Type: dimension.Type, Scope: SelectionScopeFactLocal}, nil
}
