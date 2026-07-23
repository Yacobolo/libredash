package signals

import (
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

func DashboardVisualWindowRequestFromDashboard(value dashboard.TableRequest) visualizationir.VisualizationWindowRequest {
	direction := visualizationir.VisualizationSortDirectionAscending
	if value.Sort.Direction == "desc" {
		direction = visualizationir.VisualizationSortDirectionDescending
	}
	return visualizationir.VisualizationWindowRequest{
		VisualID: value.Table, RequestSeq: int64(value.RequestSeq), ResetVersion: int64(value.ResetVersion),
		Start: int64(value.Start), Limit: int64(value.Count), BlockID: value.Block,
		Sort: []visualizationir.VisualizationSort{{Field: visualizationir.VisualizationFieldRef{Dataset: "primary", Field: value.Sort.Key}, Direction: direction}},
	}
}

func DashboardVisualSpatialWindowRequestFromDashboard(value dashboard.SpatialWindowRequest) visualizationir.VisualizationSpatialWindowRequest {
	return value
}

func DashboardTabularVisualFromDefinitionAtRevision(definition visualizationdefinition.Definition, value dashboard.Table, dataRevision, generation int64) visualizationir.VisualizationEnvelope {
	envelope, err := visualizationruntime.WindowEnvelopeFromDefinition(definition, value, dataRevision, generation)
	if err != nil {
		panic(fmt.Sprintf("compiled tabular visualization %q reached the signal boundary with invalid data: %v", definition.ID, err))
	}
	return envelope
}

func DashboardVisualizationSignalFromIR(value visualizationir.VisualizationEnvelope) DashboardVisualizationSignal {
	transport, err := visualizationir.EncodeDataStateTransport(value.DataState)
	if err != nil {
		panic(fmt.Sprintf("encode dashboard visualization data-state transport: %v", err))
	}
	return DashboardVisualizationSignal{
		SchemaVersion:    value.SchemaVersion,
		VisualID:         value.VisualID,
		RendererID:       value.RendererID,
		SpecRevision:     value.SpecRevision,
		Spec:             value.Spec,
		DataRevision:     value.DataRevision,
		DataState:        visualizationDataStateTransport(transport),
		Selection:        value.Selection,
		SpatialSelection: value.SpatialSelection,
		Status:           value.Status,
		Diagnostics:      value.Diagnostics,
	}
}

func dashboardSpatialSelections(values []dashboard.SpatialInteractionSelection) []visualizationir.VisualizationSpatialSelectionState {
	out := make([]visualizationir.VisualizationSpatialSelectionState, len(values))
	for index, value := range values {
		out[index] = visualizationir.VisualizationSpatialSelectionState{VisualID: value.VisualID, InteractionID: value.InteractionID, Geometry: value.Geometry}
	}
	return out
}

func visualizationDataStateTransport(value visualizationir.EncodedDataStateTransport) visualizationir.VisualizationDataStateTransport {
	return visualizationir.VisualizationDataStateTransport{
		SchemaVersion: value.SchemaVersion,
		Encoding:      visualizationir.VisualizationDataStateTransportEncoding(value.Encoding),
		Kind:          visualizationir.VisualizationDataStateKind(value.Kind),
		SpecRevision:  value.SpecRevision,
		DataRevision:  value.DataRevision,
		Generation:    value.Generation,
		Payload:       value.Payload,
	}
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
		if len(definition.Targets.Visuals) > 0 {
			targets = &ReportFilterTargets{Visuals: optionalSlice(definition.Targets.Visuals)}
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
