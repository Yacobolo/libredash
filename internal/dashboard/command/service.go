package command

import (
	"fmt"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dashboard/reportmodel"
)

type Metrics interface {
	report.Metrics
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
}

type Service struct {
	Metrics Metrics
}

type Request struct {
	DashboardID         string
	PageID              string
	ModelID             string
	Filters             dashboard.Filters
	VisualWindowCommand dashboard.TableRequest
	InteractionCommand  dashboard.InteractionCommand
}

func canonicalInteractionCommand(metrics Metrics, dashboardID string, command dashboard.InteractionCommand) (dashboard.InteractionCommand, error) {
	definition, model, ok := metrics.Report(dashboardID)
	if !ok || model == nil {
		return dashboard.InteractionCommand{}, fmt.Errorf("dashboard %q is not published", dashboardID)
	}
	wantKind := "point_selection"
	var toggle bool
	semanticMappingCount := 0
	switch command.SourceKind {
	case "visual":
		if source, ok := definition.Visuals[command.SourceID]; ok {
			toggle = source.Interaction.PointSelection.Toggle
			semanticMappingCount = len(source.Interaction.PointSelection.Mappings)
		} else if source, ok := definition.Tables[command.SourceID]; ok {
			wantKind = "row_selection"
			toggle = source.Interaction.RowSelection.Toggle
			semanticMappingCount = len(source.Interaction.RowSelection.Mappings)
		} else {
			return dashboard.InteractionCommand{}, fmt.Errorf("unknown source visual %q", command.SourceID)
		}
	default:
		return dashboard.InteractionCommand{}, fmt.Errorf("unknown source kind %q", command.SourceKind)
	}
	if command.InteractionKind != wantKind {
		return dashboard.InteractionCommand{}, fmt.Errorf("source %s %q requires interaction kind %q", command.SourceKind, command.SourceID, wantKind)
	}
	if command.Action != "set" && command.Action != "replace" && command.Action != "clear" {
		return dashboard.InteractionCommand{}, fmt.Errorf("unsupported selection action %q", command.Action)
	}
	command.Toggle = toggle
	if command.Action == "clear" {
		command.Mappings = nil
		return command, nil
	}
	if wantKind == "row_selection" && semanticMappingCount == 0 {
		if len(command.Mappings) != 1 || command.Mappings[0].Field != dashboard.UIRowSelectionField || command.Mappings[0].Fact != "" || command.Mappings[0].Grain != "" || !dashboard.IsInteractionSelectionScalar(command.Mappings[0].Value) {
			return dashboard.InteractionCommand{}, fmt.Errorf("table %q without semantic selection mappings accepts only the UI row key", command.SourceID)
		}
		return command, nil
	}
	if semanticMappingCount == 0 {
		return dashboard.InteractionCommand{}, fmt.Errorf("%s %q has no semantic selection mappings", command.SourceKind, command.SourceID)
	}
	resolved, err := reportmodel.ResolveSelectionInteraction(&definition, model, command.SourceKind, command.SourceID)
	if err != nil {
		return dashboard.InteractionCommand{}, err
	}
	identities := make([]reportmodel.SelectionMappingIdentity, len(command.Mappings))
	incoming := make(map[reportmodel.SelectionMappingIdentity]dashboard.InteractionCommandMapping, len(command.Mappings))
	for index, mapping := range command.Mappings {
		if !mapping.HasValue() {
			return dashboard.InteractionCommand{}, fmt.Errorf("mapping %d must include value", index)
		}
		if !dashboard.IsInteractionSelectionScalar(mapping.Value) {
			return dashboard.InteractionCommand{}, fmt.Errorf("mapping %d value must be a JSON scalar", index)
		}
		identity := reportmodel.SelectionMappingIdentity{Field: mapping.Field, Fact: mapping.Fact, Grain: mapping.Grain}
		identities[index] = identity
		incoming[identity] = mapping
	}
	canonical, err := resolved.CanonicalizeMappings(identities)
	if err != nil {
		return dashboard.InteractionCommand{}, err
	}
	command.Mappings = make([]dashboard.InteractionCommandMapping, 0, len(canonical))
	for _, mapping := range canonical {
		identity := reportmodel.SelectionMappingIdentity{Field: mapping.Field, Fact: mapping.Fact, Grain: mapping.Grain}
		value := incoming[identity]
		if !dashboard.InteractionSelectionValueMatchesType(value.Value, mapping.Type, mapping.Grain) {
			return dashboard.InteractionCommand{}, fmt.Errorf("mapping field %q value type %T does not match semantic type %q", mapping.Field, value.Value, mapping.Type)
		}
		command.Mappings = append(command.Mappings, value)
	}
	return command, nil
}
