package signals

import (
	"encoding/json"
	"fmt"

	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	visualizationruntime "github.com/Yacobolo/leapview/internal/visualization/runtime"
)

func optionalValue[T comparable](value T) *T {
	var zero T
	if value == zero {
		return nil
	}
	return &value
}

func optionalSlice[T any](value []T) *[]T {
	if len(value) == 0 {
		return nil
	}
	copyValue := append([]T(nil), value...)
	return &copyValue
}

func optionalMap[K comparable, V any](value map[K]V) *map[K]V {
	if len(value) == 0 {
		return nil
	}
	copyValue := make(map[K]V, len(value))
	for key, item := range value {
		copyValue[key] = item
	}
	return &copyValue
}

func Pointer[T any](value T) *T {
	return &value
}

func Optional[T comparable](value T) *T {
	return optionalValue(value)
}

func OptionalSlice[T any](value []T) *[]T {
	return optionalSlice(value)
}

func ValueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

func DashboardPageCanvasFromDashboard(value dashboard.PageCanvas) DashboardPageCanvas {
	return DashboardPageCanvas{Width: int64(value.Width), Height: int64(value.Height)}
}

func DashboardPageGridFromDashboard(value dashboard.PageGrid) DashboardPageGrid {
	return DashboardPageGrid{Columns: int64(value.Columns), RowHeight: int64(value.RowHeight), Gap: int64(value.Gap), Padding: int64(value.Padding)}
}

func DashboardPagePlacementFromDashboard(value dashboard.PagePlacement) DashboardPagePlacement {
	return DashboardPagePlacement{Col: int64(value.Col), Row: int64(value.Row), ColSpan: int64(value.ColSpan), RowSpan: int64(value.RowSpan)}
}

func DashboardStatusFromDashboard(value dashboard.Status) DashboardStatus {
	return DashboardStatus{
		Loading: value.Loading, Error: value.Error, RefreshID: value.RefreshID, Generation: value.Generation, LastUpdated: value.LastUpdated,
		SetupRequired:   value.SetupRequired,
		ProgressPercent: ValueOrZero(dashboard.NormalizeProgressPercent(value.ProgressPercent, value.Loading)),
	}
}

func DashboardFilterOptionsFromDashboard(values map[string][]dashboard.FilterOption) map[string][]DashboardFilterOption {
	out := make(map[string][]DashboardFilterOption, len(values))
	for key, options := range values {
		converted := make([]DashboardFilterOption, len(options))
		for index, option := range options {
			converted[index] = DashboardFilterOption{Value: option.Value, Label: option.Label}
		}
		out[key] = converted
	}
	return out
}

func DashboardFiltersFromDashboard(value dashboard.Filters) DashboardFilters {
	value = value.WithDefaults()
	controls := make(map[string]DashboardFilterControl, len(value.Controls))
	for key, control := range value.Controls {
		controls[key] = DashboardFilterControl{
			Type: control.Type, Operator: optionalValue(control.Operator), Preset: optionalValue(control.Preset),
			From: optionalValue(control.From), To: optionalValue(control.To), Value: optionalValue(control.Value), Values: optionalSlice(control.Values),
		}
	}
	return DashboardFilters{Controls: controls, Selections: dashboardInteractionSelections(value.Selections), SpatialSelections: dashboardSpatialSelections(value.SpatialSelections)}
}

func DashboardInteractionCommandFromDashboard(value dashboard.InteractionCommand) DashboardInteractionCommand {
	mappings := make([]DashboardInteractionCommandMapping, len(value.Mappings))
	for index, mapping := range value.Mappings {
		mappings[index] = DashboardInteractionCommandMapping{
			Field: mapping.Field, Fact: optionalValue(mapping.Fact), Grain: optionalValue(mapping.Grain),
			Value: mapping.Value, Label: optionalValue(mapping.Label),
		}
	}
	return DashboardInteractionCommand{
		SourceKind: value.SourceKind, SourceID: value.SourceID, InteractionKind: value.InteractionKind,
		Action: value.Action, Toggle: value.Toggle, Mappings: mappings,
	}
}

func DashboardVisualWindowRequestFromDashboard(value dashboard.TableRequest) VisualizationWindowRequest {
	direction := VisualizationSortDirectionAscending
	if value.Sort.Direction == "desc" {
		direction = VisualizationSortDirectionDescending
	}
	return VisualizationWindowRequest{
		VisualID: value.Table, RequestSeq: int64(value.RequestSeq), ResetVersion: int64(value.ResetVersion),
		Start: int64(value.Start), Limit: int64(value.Count), BlockID: value.Block,
		Sort: []VisualizationSort{{Field: VisualizationFieldRef{Dataset: "primary", Field: value.Sort.Key}, Direction: direction}},
	}
}

func DashboardVisualSpatialWindowRequestFromDashboard(value dashboard.SpatialWindowRequest) VisualizationSpatialWindowRequest {
	return VisualizationSpatialWindowRequest{
		VisualID: value.VisualID, SpecRevision: value.SpecRevision, DataRevision: value.DataRevision,
		RequestSeq: value.RequestSeq, ResetVersion: value.ResetVersion,
		Bounds: VisualizationSpatialBounds{West: value.Bounds.West, South: value.Bounds.South, East: value.Bounds.East, North: value.Bounds.North},
		Zoom:   value.Zoom, Width: int32(value.Width), Height: int32(value.Height), WindowID: value.WindowID,
	}
}

func DashboardTabularVisualFromDefinitionAtRevision(definition visualizationdefinition.Definition, value dashboard.Table, dataRevision, generation int64) VisualizationEnvelope {
	envelope, err := visualizationruntime.TableEnvelopeFromDefinition(definition, value, dataRevision, generation)
	if err != nil {
		panic(fmt.Sprintf("compiled tabular visualization %q reached the signal boundary with invalid data: %v", definition.ID, err))
	}
	return visualizationEnvelope(envelope)
}

func visualizationEnvelope(value any) VisualizationEnvelope {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode visualization envelope: %v", err))
	}
	var out VisualizationEnvelope
	if err := json.Unmarshal(data, &out); err != nil {
		panic(fmt.Sprintf("decode generated visualization signal envelope: %v", err))
	}
	return out
}

func VisualizationEnvelopeFromIR(value visualizationir.VisualizationEnvelope) VisualizationEnvelope {
	return visualizationEnvelope(value)
}

func DashboardVisualizationSignalFromIR(value visualizationir.VisualizationEnvelope) DashboardVisualizationSignal {
	envelope := visualizationEnvelope(value)
	transport, err := visualizationir.EncodeDataStateTransport(value.DataState)
	if err != nil {
		panic(fmt.Sprintf("encode dashboard visualization data-state transport: %v", err))
	}
	dataState := visualizationDataStateTransport(transport)
	return DashboardVisualizationSignal{
		SchemaVersion:    envelope.SchemaVersion,
		VisualID:         envelope.VisualID,
		RendererID:       envelope.RendererID,
		SpecRevision:     envelope.SpecRevision,
		Spec:             envelope.Spec,
		DataRevision:     envelope.DataRevision,
		DataState:        dataState,
		Selection:        envelope.Selection,
		SpatialSelection: envelope.SpatialSelection,
		Status:           envelope.Status,
		Diagnostics:      envelope.Diagnostics,
	}
}

func dashboardSpatialSelections(values []dashboard.SpatialInteractionSelection) []VisualizationSpatialSelectionState {
	out := make([]VisualizationSpatialSelectionState, len(values))
	for index, value := range values {
		data, err := json.Marshal(visualizationir.VisualizationSpatialSelectionState{VisualID: value.VisualID, InteractionID: value.InteractionID, Geometry: value.Geometry})
		if err != nil {
			panic(fmt.Sprintf("encode dashboard spatial selection: %v", err))
		}
		if err := json.Unmarshal(data, &out[index]); err != nil {
			panic(fmt.Sprintf("decode dashboard spatial selection signal: %v", err))
		}
	}
	return out
}

func visualizationDataStateTransport(value visualizationir.EncodedDataStateTransport) VisualizationDataStateTransport {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode visualization data-state transport: %v", err))
	}
	var out VisualizationDataStateTransport
	if err := json.Unmarshal(data, &out); err != nil {
		panic(fmt.Sprintf("decode generated visualization data-state transport: %v", err))
	}
	return out
}

func dashboardInteractionSelections(values []dashboard.InteractionSelection) []DashboardInteractionSelection {
	out := make([]DashboardInteractionSelection, len(values))
	for index, value := range values {
		out[index] = DashboardInteractionSelection{
			ID: value.ID, SourceKind: value.SourceKind, SourceID: value.SourceID, InteractionKind: value.InteractionKind,
			Entries: dashboardInteractionSelectionEntries(value.Entries), Label: value.Label, Order: int64(value.Order),
		}
	}
	return out
}

func dashboardInteractionSelectionEntries(values []dashboard.InteractionSelectionEntry) []DashboardInteractionSelectionEntry {
	out := make([]DashboardInteractionSelectionEntry, len(values))
	for index, value := range values {
		mappings := make([]DashboardInteractionSelectionMapping, len(value.Mappings))
		for mappingIndex, mapping := range value.Mappings {
			mappings[mappingIndex] = DashboardInteractionSelectionMapping{
				Field: mapping.Field, Fact: optionalValue(mapping.Fact), Grain: optionalValue(mapping.Grain),
				Value: mapping.Value, Label: optionalValue(mapping.Label),
			}
		}
		out[index] = DashboardInteractionSelectionEntry{Mappings: mappings, Label: optionalValue(value.Label)}
	}
	return out
}

func ReportFilterConfigsFromReport(values []dashboarddefinition.FilterConfig) []ReportFilterConfig {
	out := make([]ReportFilterConfig, len(values))
	for index, value := range values {
		definition := value.FilterDefinition
		options := make([]ReportFilterOption, len(definition.Options))
		for optionIndex, option := range definition.Options {
			options[optionIndex] = ReportFilterOption{Value: option.Value, Label: option.Label}
		}
		presets := make([]ReportFilterPreset, len(definition.Presets))
		for presetIndex, preset := range definition.Presets {
			relativeDays := int64(preset.RelativeDays)
			presets[presetIndex] = ReportFilterPreset{
				Value: preset.Value, Label: preset.Label, From: optionalValue(preset.From), To: optionalValue(preset.To), RelativeDays: optionalValue(relativeDays),
			}
		}
		var targets *ReportFilterTargets
		if len(definition.Targets.Visuals) > 0 || len(definition.Targets.Tables) > 0 {
			allTargets := append(append([]string{}, definition.Targets.Visuals...), definition.Targets.Tables...)
			targets = &ReportFilterTargets{Visuals: optionalSlice(allTargets)}
		}
		var filterValues *ReportFilterValues
		if definition.Values.Source != "" || definition.Values.Limit != 0 {
			limit := int64(definition.Values.Limit)
			filterValues = &ReportFilterValues{Source: optionalValue(definition.Values.Source), Limit: optionalValue(limit)}
		}
		out[index] = ReportFilterConfig{
			ID: value.ID, Type: definition.Type, Label: definition.Label, Description: optionalValue(definition.Description),
			Dimension: definition.Dimension, Fact: optionalValue(definition.Fact), Custom: optionalValue(definition.Custom), Operator: optionalValue(definition.Operator),
			DefaultOperator: optionalValue(definition.DefaultOperator), Operators: optionalSlice(definition.Operators), Options: optionalSlice(options),
			Presets: optionalSlice(presets), URLParam: optionalValue(definition.URLParam), FromURLParam: optionalValue(definition.FromURLParam),
			ToURLParam: optionalValue(definition.ToURLParam), OperatorURLParam: optionalValue(definition.OperatorURLParam), Targets: targets, Values: filterValues,
			Default: ReportFilterDefault{
				Preset: optionalValue(definition.Default.Preset), From: optionalValue(definition.Default.From), To: optionalValue(definition.Default.To),
				Operator: optionalValue(definition.Default.Operator), Value: optionalValue(definition.Default.Value), Values: optionalSlice(definition.Default.Values),
			},
		}
	}
	return out
}
