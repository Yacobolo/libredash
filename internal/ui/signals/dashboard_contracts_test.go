package signals

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
)

func TestDashboardContractConversionsPreserveJSON(t *testing.T) {
	t.Parallel()

	trueValue := true
	min := 1.5
	max := 9.5
	visual := dashboard.Visual{
		Version: 3, ID: "orders", Kind: "chart", Shape: "bar", Renderer: "echarts", Type: "bar",
		Title: "Orders", Unit: "orders", Format: "0,0", Dimensions: []string{"state"}, Measure: "orders",
		Measures: []string{"orders"}, Series: []string{"status"}, Options: map[string]any{"stacked": true},
		RendererOptions: map[string]map[string]any{"echarts": {"animation": false}},
		Interaction:     dashboard.InteractionConfig{Kind: "point_selection", Toggle: true, Targets: []string{"orders_table"}, Mappings: []dashboard.InteractionConfigMapping{{Field: "activity_date", Grain: "month", Value: "state", Label: "State"}}},
		Selection:       []dashboard.InteractionSelectionEntry{{Label: "42", Mappings: []dashboard.InteractionSelectionMapping{{Field: "ratings.rating_bucket", Fact: "ratings", Value: float64(42), Label: "Rating"}}}},
		Data:            []dashboard.Datum{{"state": "SP", "orders": 42}},
	}
	table := dashboard.Table{
		Version: 2, Kind: "data_table", Title: "Orders", Style: dashboard.TableStyle{Density: "compact", Zebra: &trueValue, Grid: "rows"},
		Interaction: dashboard.InteractionConfig{Kind: "row_selection", Toggle: true, Mappings: []dashboard.InteractionConfigMapping{}},
		Selection:   []dashboard.InteractionSelectionEntry{},
		Columns: []dashboard.TableColumn{{Key: "amount", Label: "Amount", Align: "right", Role: "measure", Group: "sales", Measure: "amount", ColumnValue: "amount", Width: 120, Format: "currency", Formatting: []dashboard.TableFormattingRule{{
			Kind: "gradient", Values: map[string]string{"high": "green"}, Min: &min, Max: &max, Color: "white", Background: "black", LowColor: "red", HighColor: "green",
		}}}},
		Cardinality: dashboard.ExactCardinality(1), AvailableRows: 1, RowCap: 100, ChunkSize: 50, RowHeight: 28, ResetVersion: 2,
		Sort: dashboard.TableSort{Key: "amount", Direction: "desc"}, Blocks: map[string]dashboard.TableBlock{"a": {Start: 0, RequestSeq: 3, ResetVersion: 2, Sort: dashboard.TableSort{Key: "amount", Direction: "desc"}, Rows: []map[string]any{{"amount": 42}}}},
	}
	filters := dashboard.Filters{
		Controls:   map[string]dashboard.FilterControl{"state": {Type: "multi_select", Operator: "in", Values: []string{"SP"}}},
		Selections: []dashboard.InteractionSelection{{ID: "visual:orders:point", SourceKind: "visual", SourceID: "orders", InteractionKind: "point", Label: "42", Order: 1, Entries: []dashboard.InteractionSelectionEntry{{Label: "42", Mappings: []dashboard.InteractionSelectionMapping{{Field: "ratings.rating_bucket", Fact: "ratings", Value: float64(42), Label: "Rating"}}}}}},
	}
	filterConfig := []reportdef.FilterConfig{{
		ID: "state",
		FilterDefinition: reportdef.FilterDefinition{
			Type: "multi_select", Label: "State", Description: "Order state", Dimension: "orders.state", Fact: "orders", Custom: true,
			Default: reportdef.FilterDefault{Operator: "in", Values: []string{"SP"}}, Operator: "in", DefaultOperator: "in", Operators: []string{"in"},
			Options: []reportdef.FilterOption{{Value: "SP", Label: "Sao Paulo"}}, Presets: []reportdef.FilterPreset{{Value: "recent", Label: "Recent", From: "2026-01-01", To: "2026-12-31", RelativeDays: 30}},
			Values: reportdef.FilterValues{Source: "orders.state", Limit: 100}, URLParam: "state", FromURLParam: "from", ToURLParam: "to", OperatorURLParam: "op",
			Targets: reportdef.FilterTargets{Visuals: []string{"orders"}, Tables: []string{"orders_table"}},
		},
	}}

	chartSignal := DashboardVisualFromDashboard(visual)
	chartVariant, ok := chartSignal.Value.(BarDashboardVisual)
	if !ok || chartVariant.Type != "bar" || chartVariant.Data == nil || len(*chartVariant.Data) != 1 || chartVariant.Shape == nil || *chartVariant.Shape != "bar" {
		t.Fatalf("chart signal = %#v", chartSignal)
	}
	tableSignal := DashboardTabularVisualFromDashboard("orders_table", table)
	tableVariant, ok := tableSignal.Value.(TableDashboardVisual)
	if !ok || tableVariant.ID != "orders_table" || tableVariant.Type != "table" || tableVariant.Blocks == nil || tableVariant.Columns == nil {
		t.Fatalf("table visual signal = %#v", tableSignal)
	}
	assertSameJSON(t, filters, DashboardFiltersFromDashboard(filters))
	convertedFilters := ReportFilterConfigsFromReport(filterConfig)
	if convertedFilters[0].Targets == nil || convertedFilters[0].Targets.Visuals == nil || !reflect.DeepEqual(*convertedFilters[0].Targets.Visuals, []string{"orders", "orders_table"}) {
		t.Fatalf("filter targets = %#v", convertedFilters[0].Targets)
	}
}

func assertSameJSON(t *testing.T, left, right any) {
	t.Helper()
	leftJSON, err := json.Marshal(left)
	if err != nil {
		t.Fatalf("marshal source: %v", err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	var leftValue, rightValue any
	if err := json.Unmarshal(leftJSON, &leftValue); err != nil {
		t.Fatalf("decode source: %v", err)
	}
	if err := json.Unmarshal(rightJSON, &rightValue); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	if !reflect.DeepEqual(leftValue, rightValue) {
		t.Fatalf("JSON differs:\nsource:   %s\ncontract: %s", leftJSON, rightJSON)
	}
}
