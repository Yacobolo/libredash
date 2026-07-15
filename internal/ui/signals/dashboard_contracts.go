package signals

import (
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
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
		Loading: value.Loading, Error: value.Error, LastUpdated: value.LastUpdated,
		SetupRequired: value.SetupRequired,
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
	controls := make(map[string]DashboardFilterControl, len(value.Controls))
	for key, control := range value.Controls {
		controls[key] = DashboardFilterControl{
			Type: control.Type, Operator: optionalValue(control.Operator), Preset: optionalValue(control.Preset),
			From: optionalValue(control.From), To: optionalValue(control.To), Value: optionalValue(control.Value), Values: optionalSlice(control.Values),
		}
	}
	return DashboardFilters{Controls: controls, Selections: dashboardInteractionSelections(value.Selections)}
}

func DashboardInteractionCommandFromDashboard(value dashboard.InteractionCommand) DashboardInteractionCommand {
	mappings := make([]DashboardInteractionCommandMapping, len(value.Mappings))
	for index, mapping := range value.Mappings {
		mappings[index] = DashboardInteractionCommandMapping{Field: mapping.Field, Value: mapping.Value, Label: optionalValue(mapping.Label)}
	}
	return DashboardInteractionCommand{
		SourceKind: value.SourceKind, SourceID: value.SourceID, InteractionKind: value.InteractionKind,
		Action: value.Action, Toggle: value.Toggle, Mappings: mappings,
	}
}

func DashboardTableRequestFromDashboard(value dashboard.TableRequest) DashboardTableRequest {
	return DashboardTableRequest{
		Table: value.Table, Block: value.Block, Start: int64(value.Start), Count: int64(value.Count),
		RequestSeq: int64(value.RequestSeq), Sort: dashboardTableSort(value.Sort), ResetVersion: int64(value.ResetVersion),
	}
}

func DashboardVisualFromDashboard(value dashboard.Visual) DashboardVisual {
	data := make([]map[string]any, len(value.Data))
	for index, datum := range value.Data {
		data[index] = map[string]any(datum)
	}
	return DashboardVisual{
		Version: int64(value.Version), ID: value.ID, Kind: value.Kind, Shape: value.Shape, Renderer: value.Renderer,
		Type: value.Type, Title: value.Title, Unit: value.Unit, Format: optionalValue(value.Format),
		Interaction: dashboardInteractionConfig(value.Interaction), Dimensions: append([]string(nil), value.Dimensions...),
		Measure: value.Measure, Measures: append([]string(nil), value.Measures...), Series: append([]string(nil), value.Series...),
		Options: value.Options, RendererOptions: value.RendererOptions, Selection: dashboardInteractionSelectionEntries(value.Selection), Data: data,
	}
}

func DashboardVisualsFromDashboard(values map[string]dashboard.Visual) map[string]DashboardVisual {
	out := make(map[string]DashboardVisual, len(values))
	for key, value := range values {
		out[key] = DashboardVisualFromDashboard(value)
	}
	return out
}

func DashboardTableFromDashboard(value dashboard.Table) DashboardTable {
	blocks := make(map[string]DashboardTableBlock, len(value.Blocks))
	for key, block := range value.Blocks {
		blocks[key] = DashboardTableBlock{
			Start: int64(block.Start), RequestSeq: int64(block.RequestSeq), ResetVersion: int64(block.ResetVersion),
			Sort: dashboardTableSort(block.Sort), Rows: block.Rows,
		}
	}
	columns := make([]DashboardTableColumn, len(value.Columns))
	for index, column := range value.Columns {
		formatting := make([]DashboardTableFormattingRule, len(column.Formatting))
		for ruleIndex, rule := range column.Formatting {
			formatting[ruleIndex] = DashboardTableFormattingRule{
				Kind: rule.Kind, Values: optionalMap(rule.Values), Min: rule.Min, Max: rule.Max,
				Color: optionalValue(rule.Color), Background: optionalValue(rule.Background),
				LowColor: optionalValue(rule.LowColor), HighColor: optionalValue(rule.HighColor),
			}
		}
		width := int64(column.Width)
		columns[index] = DashboardTableColumn{
			Key: column.Key, Label: column.Label, Align: optionalValue(column.Align), Role: optionalValue(column.Role),
			Group: optionalValue(column.Group), Measure: optionalValue(column.Measure), ColumnValue: optionalValue(column.ColumnValue),
			Width: optionalValue(width), Format: optionalValue(column.Format), Formatting: optionalSlice(formatting),
		}
	}
	zebra := false
	if value.Style.Zebra != nil {
		zebra = *value.Style.Zebra
	}
	return DashboardTable{
		Version: int64(value.Version), Kind: value.Kind, Title: value.Title,
		Style:       DashboardTableStyle{Density: value.Style.Density, Zebra: zebra, Grid: value.Style.Grid},
		Interaction: dashboardInteractionConfig(value.Interaction), Selection: dashboardInteractionSelectionEntries(value.Selection),
		Columns: columns, TotalRows: int64(value.TotalRows), AvailableRows: int64(value.AvailableRows), IsCapped: value.IsCapped,
		RowCap: int64(value.RowCap), ChunkSize: int64(value.ChunkSize), RowHeight: int64(value.RowHeight), ResetVersion: int64(value.ResetVersion),
		Sort: dashboardTableSort(value.Sort), Blocks: blocks, LoadingBlock: value.LoadingBlock, Error: value.Error,
	}
}

func DashboardTablesFromDashboard(values map[string]dashboard.Table) map[string]DashboardTable {
	out := make(map[string]DashboardTable, len(values))
	for key, value := range values {
		out[key] = DashboardTableFromDashboard(value)
	}
	return out
}

func dashboardInteractionConfig(value dashboard.InteractionConfig) DashboardInteractionConfig {
	mappings := make([]DashboardInteractionConfigMapping, len(value.Mappings))
	for index, mapping := range value.Mappings {
		mappings[index] = DashboardInteractionConfigMapping{Field: mapping.Field, Value: mapping.Value, Label: optionalValue(mapping.Label)}
	}
	return DashboardInteractionConfig{Kind: value.Kind, Toggle: value.Toggle, Mappings: mappings, Targets: optionalSlice(value.Targets)}
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
			mappings[mappingIndex] = DashboardInteractionSelectionMapping{Field: mapping.Field, Value: mapping.Value, Label: optionalValue(mapping.Label)}
		}
		out[index] = DashboardInteractionSelectionEntry{Mappings: mappings, Label: optionalValue(value.Label)}
	}
	return out
}

func dashboardTableSort(value dashboard.TableSort) DashboardTableSort {
	return DashboardTableSort{Key: value.Key, Direction: value.Direction}
}

func ReportFilterConfigsFromReport(values []reportdef.FilterConfig) []ReportFilterConfig {
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
			targets = &ReportFilterTargets{Visuals: optionalSlice(definition.Targets.Visuals), Tables: optionalSlice(definition.Targets.Tables)}
		}
		var filterValues *ReportFilterValues
		if definition.Values.Source != "" || definition.Values.Limit != 0 {
			limit := int64(definition.Values.Limit)
			filterValues = &ReportFilterValues{Source: optionalValue(definition.Values.Source), Limit: optionalValue(limit)}
		}
		out[index] = ReportFilterConfig{
			ID: value.ID, Type: definition.Type, Label: definition.Label, Description: optionalValue(definition.Description),
			Dimension: definition.Dimension, Custom: optionalValue(definition.Custom), Operator: optionalValue(definition.Operator),
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
