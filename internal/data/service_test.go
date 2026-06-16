package data

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

func TestMissingDataReturnsSetupPatch(t *testing.T) {
	dir := t.TempDir()
	metrics, err := NewDuckDBMetrics(dir)
	if err != nil {
		t.Fatal(err)
	}

	patch, err := metrics.QueryDashboard(context.Background(), "executive-sales", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if !patch.Status.SetupRequired {
		t.Fatalf("SetupRequired = false, want true")
	}
	if patch.Status.Error == "" {
		t.Fatal("expected setup error")
	}

	var missing *MissingDataError
	if !errors.As(metrics.runtimes["olist"].missing, &missing) {
		t.Fatalf("missing error type = %T, want *MissingDataError", metrics.runtimes["olist"].missing)
	}
}

func TestDuckDBMetricsTableInteractiveCap(t *testing.T) {
	dir := t.TempDir()
	const rows = dashboard.TableInteractiveRowCap + 5
	var orders, items, payments, customers, reviews strings.Builder
	orders.WriteString("order_id,customer_id,order_status,order_purchase_timestamp,order_delivered_customer_date\n")
	items.WriteString("order_id,order_item_id,product_id,price,freight_value\n")
	payments.WriteString("order_id,payment_value\n")
	customers.WriteString("customer_id,customer_state\n")
	reviews.WriteString("review_id,order_id,review_score\n")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&orders, "o%d,c%d,delivered,2018-01-10 10:00:00,2018-01-14 10:00:00\n", i, i)
		fmt.Fprintf(&items, "o%d,1,p1,100.00,10.00\n", i)
		fmt.Fprintf(&payments, "o%d,110.00\n", i)
		fmt.Fprintf(&customers, "c%d,SP\n", i)
		fmt.Fprintf(&reviews, "r%d,o%d,5\n", i, i)
	}
	writeFixture(t, dir, "olist_orders_dataset.csv", orders.String())
	writeFixture(t, dir, "olist_order_items_dataset.csv", items.String())
	writeFixture(t, dir, "olist_order_payments_dataset.csv", payments.String())
	writeFixture(t, dir, "olist_products_dataset.csv", "product_id,product_category_name\np1,beleza_saude\n")
	writeFixture(t, dir, "olist_customers_dataset.csv", customers.String())
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", reviews.String())
	writeFixture(t, dir, "product_category_name_translation.csv", "product_category_name,product_category_name_english\nbeleza_saude,health_beauty\n")

	metrics, err := NewDuckDBMetrics(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Close()

	table, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{}, dashboard.TableRequest{Table: "orders", Block: "all", RequestSeq: 9})
	if err != nil {
		t.Fatal(err)
	}
	if table.TotalRows != rows {
		t.Fatalf("total rows = %d, want %d", table.TotalRows, rows)
	}
	if table.AvailableRows != dashboard.TableInteractiveRowCap {
		t.Fatalf("available rows = %d, want %d", table.AvailableRows, dashboard.TableInteractiveRowCap)
	}
	if !table.IsCapped {
		t.Fatal("table is not capped")
	}
	if got := len(table.Blocks["a"].Rows) + len(table.Blocks["b"].Rows) + len(table.Blocks["c"].Rows); got != dashboard.TableChunkSize*3 {
		t.Fatalf("initial block rows = %d, want %d", got, dashboard.TableChunkSize*3)
	}
	if got := table.Blocks["a"].RequestSeq; got != 9 {
		t.Fatalf("block request seq = %d, want 9", got)
	}
}

func TestDuckDBMetricsQueryFixture(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_delivered_customer_date
o1,c1,delivered,2018-01-10 10:00:00,2018-01-14 10:00:00
o2,c2,shipped,2017-06-10 10:00:00,2017-06-20 10:00:00
`)
	writeFixture(t, dir, "olist_order_items_dataset.csv", `order_id,order_item_id,product_id,price,freight_value
o1,1,p1,100.00,10.00
o2,1,p2,50.00,5.00
`)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_value
o1,110.00
o2,55.00
`)
	writeFixture(t, dir, "olist_products_dataset.csv", `product_id,product_category_name
p1,beleza_saude
p2,relogios_presentes
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_state
c1,SP
c2,RJ
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `review_id,order_id,review_score
r1,o1,5
r2,o2,3
`)
	writeFixture(t, dir, "product_category_name_translation.csv", `product_category_name,product_category_name_english
beleza_saude,health_beauty
relogios_presentes,watches_gifts
`)

	metrics, err := NewDuckDBMetrics(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Close()
	if _, err := os.Stat(filepath.Join(dir, "libredash-olist.duckdb")); err != nil {
		t.Fatalf("expected DuckDB cache file: %v", err)
	}

	views := metrics.MetricViews()
	if len(views) != 1 {
		t.Fatalf("metric views = %d, want 1", len(views))
	}
	if got := views[0].ID; got != "orders" {
		t.Fatalf("metric view id = %q, want orders", got)
	}
	if got := views[0].DimensionCount; got != 7 {
		t.Fatalf("metric view dimension count = %d, want 7", got)
	}
	if got := views[0].MeasureCount; got != 5 {
		t.Fatalf("metric view measure count = %d, want 5", got)
	}
	if got := views[0].DashboardCount; got != 1 {
		t.Fatalf("metric view dashboard count = %d, want 1", got)
	}

	view, ok := metrics.MetricView("orders")
	if !ok {
		t.Fatal("metric view orders not found")
	}
	if got := view.Dataset; got != "orders" {
		t.Fatalf("metric view dataset = %q, want orders", got)
	}
	if got := view.Timeseries; got != "purchase_timestamp" {
		t.Fatalf("metric view timeseries = %q, want purchase_timestamp", got)
	}
	if !hasMetricDimension(view.Dimensions, "category", "e.category") {
		t.Fatalf("metric view dimensions missing category: %#v", view.Dimensions)
	}
	if !hasMetricMeasure(view.Measures, "revenue", "SUM(e.revenue)") {
		t.Fatalf("metric view measures missing revenue: %#v", view.Measures)
	}
	if len(view.Dashboards) != 1 || view.Dashboards[0].ID != "executive-sales" {
		t.Fatalf("metric view dashboards = %#v, want executive-sales", view.Dashboards)
	}

	graph, ok := metrics.ModelGraph("olist")
	if !ok {
		t.Fatal("model graph olist not found")
	}
	if !hasModelNode(graph.Nodes, "metrics_view:orders") {
		t.Fatalf("model graph missing metrics view node: %#v", graph.Nodes)
	}
	if !hasModelEdge(graph.Edges, "dataset:orders", "metrics_view:orders") {
		t.Fatalf("model graph missing dataset to metrics view edge: %#v", graph.Edges)
	}

	patch, err := metrics.QueryDashboard(context.Background(), "executive-sales", dashboard.Filters{Controls: map[string]dashboard.FilterControl{
		"state":         {Type: "multi_select", Operator: "in", Values: []string{"SP"}},
		"purchase_date": {Type: "date_range", Preset: "2018"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	if patch.Status.Error != "" {
		t.Fatalf("unexpected status error: %s", patch.Status.Error)
	}
	if got := patch.KPIs[0].Value; got != "1" {
		t.Fatalf("orders KPI = %q, want 1", got)
	}
	if len(patch.Charts["revenue"].Data) != 1 {
		t.Fatalf("revenue points = %d, want 1", len(patch.Charts["revenue"].Data))
	}
	if got := patch.Charts["revenue"].Type; got != "area" {
		t.Fatalf("revenue chart type = %q, want area", got)
	}
	if got := patch.Charts["revenue"].Version; got != 3 {
		t.Fatalf("revenue chart version = %d, want 3", got)
	}
	if got := patch.Charts["revenue"].Kind; got != "chart" {
		t.Fatalf("revenue chart kind = %q, want chart", got)
	}
	if got := patch.Charts["revenue"].Shape; got != "category_value" {
		t.Fatalf("revenue chart shape = %q, want category_value", got)
	}
	if got := patch.Charts["revenue"].Renderer; got != "echarts" {
		t.Fatalf("revenue chart renderer = %q, want echarts", got)
	}
	if got := patch.Charts["revenue"].Measures[0]; got != "revenue" {
		t.Fatalf("revenue chart measure = %q, want revenue", got)
	}
	if got := patch.Charts["orders"].Type; got != "donut" {
		t.Fatalf("orders chart type = %q, want donut", got)
	}
	if got := patch.Charts["orders_by_month_status"].Shape; got != "category_series_value" {
		t.Fatalf("multi-series chart shape = %q, want category_series_value", got)
	}
	if got := patch.Charts["orders_by_month_status"].Options["stacked"]; got != true {
		t.Fatalf("multi-series chart stacked option = %v, want true", got)
	}
	if got := datumString(patch.Charts["categories"].Data[0], "label"); got != "health_beauty" {
		t.Fatalf("top category = %q, want health_beauty", got)
	}
	if got := len(patch.FilterOptions["state"]); got != 2 {
		t.Fatalf("state filter options = %d, want 2", got)
	}

	selectedPatch, err := metrics.QueryDashboard(context.Background(), "executive-sales", dashboard.Filters{
		VisualSelections: []dashboard.VisualSelection{
			{VisualID: "orders", Field: "status", Operator: "in", Values: []string{"delivered"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := selectedPatch.KPIs[0].Value; got != "2" {
		t.Fatalf("selected orders KPI = %q, want 2", got)
	}
	if len(selectedPatch.Charts["orders"].Data) != 2 {
		t.Fatalf("orders chart points with self-selection = %d, want 2", len(selectedPatch.Charts["orders"].Data))
	}
	if !pointSelected(selectedPatch.Charts["orders"].Data, "delivered") {
		t.Fatalf("orders chart did not mark delivered as selected: %#v", selectedPatch.Charts["orders"].Data)
	}
	if got := datumString(selectedPatch.Charts["categories"].Data[0], "label"); got != "health_beauty" {
		t.Fatalf("category chart under status selection = %q, want health_beauty", got)
	}
	if got := datumString(selectedPatch.Charts["revenue"].Data[0], "series"); got != "" {
		t.Fatalf("single-series chart row series = %q, want empty", got)
	}
	if got := datumString(selectedPatch.Charts["orders_by_month_status"].Data[0], "series"); got == "" {
		t.Fatal("multi-series chart row series is empty")
	}
	if len(selectedPatch.Charts["orders_by_month_status"].Data) != 2 {
		t.Fatalf("non-target multi-series chart points under status selection = %d, want 2", len(selectedPatch.Charts["orders_by_month_status"].Data))
	}
	if got := selectedPatch.Charts["revenue_orders_combo"].Shape; got != "category_multi_measure" {
		t.Fatalf("combo chart shape = %q, want category_multi_measure", got)
	}
	if !hasDatumValue(selectedPatch.Charts["revenue_orders_combo"].Data, "series", "Revenue") || !hasDatumValue(selectedPatch.Charts["revenue_orders_combo"].Data, "series", "Orders") {
		t.Fatalf("combo chart rows missing expected measure series: %#v", selectedPatch.Charts["revenue_orders_combo"].Data)
	}
	if got := selectedPatch.Charts["revenue_waterfall"].Shape; got != "category_delta" {
		t.Fatalf("waterfall chart shape = %q, want category_delta", got)
	}
	if _, ok := selectedPatch.Charts["revenue_waterfall"].Data[0]["start"]; !ok {
		t.Fatalf("waterfall row missing start/end: %#v", selectedPatch.Charts["revenue_waterfall"].Data[0])
	}
	if got := selectedPatch.Charts["delivery_histogram"].Shape; got != "binned_measure" {
		t.Fatalf("histogram chart shape = %q, want binned_measure", got)
	}
	if _, ok := selectedPatch.Charts["delivery_histogram"].Data[0]["binStart"]; !ok {
		t.Fatalf("histogram row missing bin metadata: %#v", selectedPatch.Charts["delivery_histogram"].Data[0])
	}
	if got := selectedPatch.Charts["state_order_map"].Shape; got != "geo" {
		t.Fatalf("map chart shape = %q, want geo", got)
	}
	if !hasDatumValue(selectedPatch.Charts["state_order_map"].Data, "name", "SP") {
		t.Fatalf("map chart rows missing SP: %#v", selectedPatch.Charts["state_order_map"].Data)
	}
	if got := selectedPatch.Charts["status_delivery_graph"].Type; got != "graph" {
		t.Fatalf("graph visual type = %q, want graph", got)
	}
	if !hasDatumValue(selectedPatch.Charts["status_delivery_graph"].Data, "source", "delivered") {
		t.Fatalf("graph rows missing delivered source: %#v", selectedPatch.Charts["status_delivery_graph"].Data)
	}
	if got := selectedPatch.Charts["category_status_sunburst"].Shape; got != "hierarchy" {
		t.Fatalf("hierarchy chart shape = %q, want hierarchy", got)
	}
	if !hasHierarchyPathValue(selectedPatch.Charts["category_status_sunburst"].Data, "health_beauty") {
		t.Fatalf("hierarchy rows missing health_beauty path: %#v", selectedPatch.Charts["category_status_sunburst"].Data)
	}

	table, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{}, dashboard.TableRequest{
		Table:      "orders",
		Block:      "a",
		Start:      0,
		Count:      1,
		RequestSeq: 7,
		Sort:       dashboard.TableSort{Key: "revenue", Direction: "asc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if table.TotalRows != 2 {
		t.Fatalf("table total rows = %d, want 2", table.TotalRows)
	}
	if len(table.Blocks["a"].Rows) != 1 {
		t.Fatalf("table block rows = %d, want 1", len(table.Blocks["a"].Rows))
	}
	if got := table.Blocks["a"].Rows[0]["order_id"]; got != "o2" {
		t.Fatalf("first table order = %v, want o2", got)
	}
	if got := table.Blocks["a"].RequestSeq; got != 7 {
		t.Fatalf("single block request seq = %d, want 7", got)
	}
	if got := table.Blocks["a"].ResetVersion; got != table.ResetVersion {
		t.Fatalf("single block reset version = %d, want %d", got, table.ResetVersion)
	}
	if got := table.Blocks["a"].Sort; got.Key != "revenue" || got.Direction != "asc" {
		t.Fatalf("single block sort = %#v, want revenue asc", got)
	}

	filteredTable, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{
		VisualSelections: []dashboard.VisualSelection{
			{VisualID: "orders", Field: "status", Operator: "in", Values: []string{"delivered"}},
		},
	}, dashboard.TableRequest{Table: "orders", Block: "all", Count: 10, RequestSeq: 8})
	if err != nil {
		t.Fatal(err)
	}
	if filteredTable.TotalRows != 1 {
		t.Fatalf("targeted table total rows = %d, want 1", filteredTable.TotalRows)
	}
	if filteredTable.AvailableRows != 1 {
		t.Fatalf("targeted table available rows = %d, want 1", filteredTable.AvailableRows)
	}
	if got := filteredTable.Blocks["a"].RequestSeq; got != 8 {
		t.Fatalf("all block request seq = %d, want 8", got)
	}

	matrixTable, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{}, dashboard.TableRequest{
		Table:      "state_status_matrix",
		Block:      "all",
		Count:      10,
		RequestSeq: 10,
		Sort:       dashboard.TableSort{Key: "state", Direction: "asc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if matrixTable.Kind != "matrix_table" {
		t.Fatalf("matrix table kind = %q, want matrix_table", matrixTable.Kind)
	}
	if len(matrixTable.Blocks["a"].Rows) != 2 {
		t.Fatalf("matrix rows = %d, want 2", len(matrixTable.Blocks["a"].Rows))
	}
	if !tableHasColumn(matrixTable.Columns, "pivot_delivered__order_count") {
		t.Fatalf("matrix columns missing delivered order count: %#v", matrixTable.Columns)
	}
	if got := matrixTable.Columns[0].Role; got != "row_header" {
		t.Fatalf("matrix first column role = %q, want row_header", got)
	}
	if !tableRowsHaveKey(matrixTable.Blocks["a"].Rows, "pivot_delivered__order_count") {
		t.Fatalf("matrix rows missing delivered order count: %#v", matrixTable.Blocks["a"].Rows)
	}

	pivotTable, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{}, dashboard.TableRequest{
		Table:      "category_status_pivot",
		Block:      "all",
		Count:      10,
		RequestSeq: 11,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pivotTable.Kind != "pivot_table" {
		t.Fatalf("pivot table kind = %q, want pivot_table", pivotTable.Kind)
	}
	if len(pivotTable.Columns) < 3 {
		t.Fatalf("pivot columns = %d, want category plus status columns", len(pivotTable.Columns))
	}
	if !tableHasColumn(pivotTable.Columns, "pivot_delivered") {
		t.Fatalf("pivot columns missing delivered column: %#v", pivotTable.Columns)
	}
	if got := pivotTable.Columns[1].Group; got != "Orders" {
		t.Fatalf("pivot first value column group = %q, want Orders", got)
	}

	if err := metrics.RefreshCache(context.Background(), "olist"); err != nil {
		t.Fatalf("refresh cache: %v", err)
	}
}

func TestDuckDBMetricsPowerFilters(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_delivered_customer_date
o1,c1,delivered,2018-01-10 10:00:00,2018-01-14 10:00:00
o2,c2,shipped,2017-06-10 10:00:00,2017-06-20 10:00:00
`)
	writeFixture(t, dir, "olist_order_items_dataset.csv", `order_id,order_item_id,product_id,price,freight_value
o1,1,p1,100.00,10.00
o2,1,p2,50.00,5.00
`)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_value
o1,110.00
o2,55.00
`)
	writeFixture(t, dir, "olist_products_dataset.csv", `product_id,product_category_name
p1,beleza_saude
p2,relogios_presentes
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_state
c1,SP
c2,RJ
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `review_id,order_id,review_score
r1,o1,5
r2,o2,3
`)
	writeFixture(t, dir, "product_category_name_translation.csv", `product_category_name,product_category_name_english
beleza_saude,health_beauty
relogios_presentes,watches_gifts
`)

	metrics, err := NewDuckDBMetrics(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Close()

	tests := []struct {
		name    string
		filters dashboard.Filters
		want    string
	}{
		{
			name: "multi state",
			filters: dashboard.Filters{Controls: map[string]dashboard.FilterControl{
				"state": {Type: "multi_select", Operator: "in", Values: []string{"SP", "RJ"}},
			}},
			want: "2",
		},
		{
			name: "category contains",
			filters: dashboard.Filters{Controls: map[string]dashboard.FilterControl{
				"category": {Type: "text", Operator: "contains", Value: "watch"},
			}},
			want: "1",
		},
		{
			name: "category equals",
			filters: dashboard.Filters{Controls: map[string]dashboard.FilterControl{
				"category": {Type: "text", Operator: "equals", Value: "health_beauty"},
			}},
			want: "1",
		},
		{
			name: "category not contains",
			filters: dashboard.Filters{Controls: map[string]dashboard.FilterControl{
				"category": {Type: "text", Operator: "not_contains", Value: "health"},
			}},
			want: "1",
		},
		{
			name: "custom date range",
			filters: dashboard.Filters{Controls: map[string]dashboard.FilterControl{
				"purchase_date": {Type: "date_range", Preset: "custom", From: "2018-01-01", To: "2018-01-31"},
			}},
			want: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patch, err := metrics.QueryDashboard(context.Background(), "executive-sales", tt.filters)
			if err != nil {
				t.Fatal(err)
			}
			if got := patch.KPIs[0].Value; got != tt.want {
				t.Fatalf("orders KPI = %q, want %s", got, tt.want)
			}
		})
	}
}

func pointSelected(points []dashboard.Datum, label string) bool {
	for _, point := range points {
		if datumString(point, "label") == label {
			selected, _ := point["selected"].(bool)
			return selected
		}
	}
	return false
}

func hasDatumValue(rows []dashboard.Datum, key string, value string) bool {
	for _, row := range rows {
		if datumString(row, key) == value {
			return true
		}
	}
	return false
}

func hasHierarchyPathValue(rows []dashboard.Datum, value string) bool {
	for _, row := range rows {
		if strings.Contains(fmt.Sprint(row["path"]), value) {
			return true
		}
	}
	return false
}

func tableRowsHaveKey(rows []map[string]any, key string) bool {
	for _, row := range rows {
		if _, ok := row[key]; ok {
			return true
		}
	}
	return false
}

func hasMetricDimension(dimensions []dashboard.MetricViewDimension, name, expr string) bool {
	for _, dimension := range dimensions {
		if dimension.Name == name && dimension.Expr == expr {
			return true
		}
	}
	return false
}

func hasMetricMeasure(measures []dashboard.MetricViewMeasure, name, expression string) bool {
	for _, measure := range measures {
		if measure.Name == name && measure.Expression == expression {
			return true
		}
	}
	return false
}

func hasModelNode(nodes []dashboard.ModelNode, id string) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func hasModelEdge(edges []dashboard.ModelEdge, source, target string) bool {
	for _, edge := range edges {
		if edge.Source == source && edge.Target == target {
			return true
		}
	}
	return false
}

func datumString(row dashboard.Datum, key string) string {
	value, ok := row[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
