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

func DashboardVisualWindowRequestFromDashboard(value dashboard.TableRequest) DashboardVisualWindowRequest {
	return DashboardVisualWindowRequest{
		Visual: value.Table, Block: value.Block, Start: int64(value.Start), Count: int64(value.Count),
		RequestSeq: int64(value.RequestSeq), Sort: dashboardVisualSort(value.Sort), ResetVersion: int64(value.ResetVersion),
	}
}

func DashboardVisualFromDashboard(value dashboard.Visual) DashboardVisual {
	data := make([]map[string]any, len(value.Data))
	for index, datum := range value.Data {
		data[index] = map[string]any(datum)
	}
	base := DashboardVisualBase{
		Version: int64(value.Version), ID: value.ID, Shape: optionalValue(value.Shape), Renderer: optionalValue(value.Renderer),
		Type: value.Type, Title: value.Title, Unit: value.Unit, Format: optionalValue(value.Format),
		Interaction: dashboardInteractionConfig(value.Interaction), Dimensions: optionalSlice(value.Dimensions),
		Measure: optionalValue(value.Measure), Measures: optionalSlice(value.Measures), Series: optionalSlice(value.Series),
		Options: optionalMap(value.Options), RendererOptions: optionalMap(value.RendererOptions), Selection: dashboardInteractionSelectionEntries(value.Selection), Data: optionalSlice(data),
	}
	return dashboardVisualVariant(value.Type, base)
}

func DashboardVisualsFromDashboard(values map[string]dashboard.Visual, tables map[string]dashboard.Table) map[string]DashboardVisual {
	out := make(map[string]DashboardVisual, len(values)+len(tables))
	for key, value := range values {
		out[key] = DashboardVisualFromDashboard(value)
	}
	for key, value := range tables {
		out[key] = DashboardTabularVisualFromDashboard(key, value)
	}
	return out
}

func DashboardTabularVisualFromDashboard(id string, value dashboard.Table) DashboardVisual {
	blocks := make(map[string]DashboardVisualBlock, len(value.Blocks))
	for key, block := range value.Blocks {
		blocks[key] = DashboardVisualBlock{
			Start: int64(block.Start), RequestSeq: int64(block.RequestSeq), ResetVersion: int64(block.ResetVersion),
			Sort: dashboardVisualSort(block.Sort), Rows: block.Rows,
		}
	}
	columns := make([]DashboardVisualColumn, len(value.Columns))
	for index, column := range value.Columns {
		formatting := make([]DashboardVisualFormattingRule, len(column.Formatting))
		for ruleIndex, rule := range column.Formatting {
			formatting[ruleIndex] = DashboardVisualFormattingRule{
				Kind: rule.Kind, Values: optionalMap(rule.Values), Min: rule.Min, Max: rule.Max,
				Color: optionalValue(rule.Color), Background: optionalValue(rule.Background),
				LowColor: optionalValue(rule.LowColor), HighColor: optionalValue(rule.HighColor),
			}
		}
		width := int64(column.Width)
		columns[index] = DashboardVisualColumn{
			Key: column.Key, Label: column.Label, Align: optionalValue(column.Align), Role: optionalValue(column.Role),
			Group: optionalValue(column.Group), Measure: optionalValue(column.Measure), ColumnValue: optionalValue(column.ColumnValue),
			Width: optionalValue(width), Format: optionalValue(column.Format), Formatting: optionalSlice(formatting),
		}
	}
	zebra := false
	if value.Style.Zebra != nil {
		zebra = *value.Style.Zebra
	}
	visualType := map[string]string{"data_table": "table", "matrix_table": "matrix", "pivot_table": "pivot"}[value.Kind]
	base := DashboardVisualBase{
		Version: int64(value.Version), ID: id, Type: visualType, Title: value.Title, Unit: "",
		Style:       &DashboardVisualStyle{Density: value.Style.Density, Zebra: zebra, Grid: value.Style.Grid},
		Interaction: dashboardInteractionConfig(value.Interaction), Selection: dashboardInteractionSelectionEntries(value.Selection),
		Columns: &columns, Cardinality: &DashboardVisualCardinality{Kind: value.Cardinality.Kind, Value: int64(value.Cardinality.Value)}, AvailableRows: optionalValue(int64(value.AvailableRows)), IsCapped: optionalValue(value.IsCapped),
		RowCap: optionalValue(int64(value.RowCap)), ChunkSize: optionalValue(int64(value.ChunkSize)), RowHeight: optionalValue(int64(value.RowHeight)), ResetVersion: optionalValue(int64(value.ResetVersion)),
		Sort: optionalValue(dashboardVisualSort(value.Sort)), Blocks: &blocks, LoadingBlock: optionalValue(value.LoadingBlock), Error: optionalValue(value.Error),
	}
	return dashboardVisualVariant(visualType, base)
}

func dashboardVisualVariant(visualType string, base DashboardVisualBase) DashboardVisual {
	base.Type = visualType
	switch visualType {
	case "line":
		return DashboardVisual{Value: LineDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "area":
		return DashboardVisual{Value: AreaDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "bar":
		return DashboardVisual{Value: BarDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "column":
		return DashboardVisual{Value: ColumnDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "pie":
		return DashboardVisual{Value: PieDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "donut":
		return DashboardVisual{Value: DonutDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "scatter":
		return DashboardVisual{Value: ScatterDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "funnel":
		return DashboardVisual{Value: FunnelDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "treemap":
		return DashboardVisual{Value: TreemapDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "gauge":
		return DashboardVisual{Value: GaugeDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "heatmap":
		return DashboardVisual{Value: HeatmapDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "sankey":
		return DashboardVisual{Value: SankeyDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "graph":
		return DashboardVisual{Value: GraphDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "map":
		return DashboardVisual{Value: MapDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "candlestick":
		return DashboardVisual{Value: CandlestickDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "boxplot":
		return DashboardVisual{Value: BoxplotDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "combo":
		return DashboardVisual{Value: ComboDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "waterfall":
		return DashboardVisual{Value: WaterfallDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "histogram":
		return DashboardVisual{Value: HistogramDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "radar":
		return DashboardVisual{Value: RadarDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "tree":
		return DashboardVisual{Value: TreeDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "sunburst":
		return DashboardVisual{Value: SunburstDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "kpi":
		return DashboardVisual{Value: KPIDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "table":
		return DashboardVisual{Value: TableDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "matrix":
		return DashboardVisual{Value: MatrixDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	case "pivot":
		return DashboardVisual{Value: PivotDashboardVisual{DashboardVisualBase: base, Type: visualType}}
	default:
		panic("unsupported dashboard visual type: " + visualType)
	}
}

func dashboardInteractionConfig(value dashboard.InteractionConfig) DashboardInteractionConfig {
	mappings := make([]DashboardInteractionConfigMapping, len(value.Mappings))
	for index, mapping := range value.Mappings {
		mappings[index] = DashboardInteractionConfigMapping{
			Field: mapping.Field, Fact: optionalValue(mapping.Fact), Grain: optionalValue(mapping.Grain),
			Value: mapping.Value, Label: optionalValue(mapping.Label),
		}
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
			mappings[mappingIndex] = DashboardInteractionSelectionMapping{
				Field: mapping.Field, Fact: optionalValue(mapping.Fact), Grain: optionalValue(mapping.Grain),
				Value: mapping.Value, Label: optionalValue(mapping.Label),
			}
		}
		out[index] = DashboardInteractionSelectionEntry{Mappings: mappings, Label: optionalValue(value.Label)}
	}
	return out
}

func dashboardVisualSort(value dashboard.TableSort) DashboardVisualSort {
	return DashboardVisualSort{Key: value.Key, Direction: value.Direction}
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
