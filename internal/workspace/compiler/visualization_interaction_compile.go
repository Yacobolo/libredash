package compiler

import (
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

func compiledSelectionInteractions(id string, selection reportdef.SelectionInteraction) []visualizationir.VisualizationInteraction {
	if selection.IsZero() {
		return []visualizationir.VisualizationInteraction{}
	}
	mappings := make([]visualizationir.VisualizationInteractionMapping, 0, len(selection.Mappings))
	for _, mapping := range selection.Mappings {
		value := visualizationir.VisualizationFieldRef{Dataset: "primary", Field: mapping.Value}
		item := visualizationir.VisualizationInteractionMapping{Source: value, TargetFieldID: mapping.Field}
		if mapping.Fact != "" {
			item.TargetFactID = &mapping.Fact
		}
		if mapping.Grain != "" {
			item.Grain = &mapping.Grain
		}
		if mapping.Label != "" {
			label := visualizationir.VisualizationFieldRef{Dataset: "primary", Field: mapping.Label}
			item.Label = &label
		}
		mappings = append(mappings, item)
	}
	mode := visualizationir.VisualizationSelectionModeSingle
	if selection.Toggle {
		mode = visualizationir.VisualizationSelectionModeMultiple
	}
	return []visualizationir.VisualizationInteraction{{ID: id, Kind: visualizationir.VisualizationInteractionKindSelect, Mappings: mappings, Targets: append([]string{}, selection.Targets...), Mode: mode, RequiresStableIdentity: true}}
}

func interactionIdentity(selection reportdef.SelectionInteraction) []string {
	fields := make([]string, 0, len(selection.Mappings))
	for _, mapping := range selection.Mappings {
		fields = append(fields, mapping.Field)
	}
	return uniqueStrings(fields)
}
