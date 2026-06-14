package data

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

func TestMissingDataReturnsSetupPatch(t *testing.T) {
	dir := t.TempDir()
	metrics, err := NewDuckDBMetrics(dir)
	if err != nil {
		t.Fatal(err)
	}

	patch, err := metrics.QueryDashboard(context.Background(), dashboard.Filters{})
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
	if !errors.As(metrics.missing, &missing) {
		t.Fatalf("missing error type = %T, want *MissingDataError", metrics.missing)
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
	if _, err := os.Stat(filepath.Join(dir, "libredash.duckdb")); err != nil {
		t.Fatalf("expected DuckDB cache file: %v", err)
	}

	patch, err := metrics.QueryDashboard(context.Background(), dashboard.Filters{State: "SP", DateRange: "2018"})
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
	if got := patch.Charts["orders"].Type; got != "donut" {
		t.Fatalf("orders chart type = %q, want donut", got)
	}
	if got := patch.Charts["categories"].Data[0].Label; got != "health_beauty" {
		t.Fatalf("top category = %q, want health_beauty", got)
	}

	selectedPatch, err := metrics.QueryDashboard(context.Background(), dashboard.Filters{
		VisualSelections: []dashboard.VisualSelection{
			{VisualID: "orders", Field: "status", Operator: "in", Values: []string{"delivered"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := selectedPatch.KPIs[0].Value; got != "1" {
		t.Fatalf("selected orders KPI = %q, want 1", got)
	}
	if len(selectedPatch.Charts["orders"].Data) != 2 {
		t.Fatalf("orders chart points with self-selection = %d, want 2", len(selectedPatch.Charts["orders"].Data))
	}
	if !pointSelected(selectedPatch.Charts["orders"].Data, "delivered") {
		t.Fatalf("orders chart did not mark delivered as selected: %#v", selectedPatch.Charts["orders"].Data)
	}
	if got := selectedPatch.Charts["categories"].Data[0].Label; got != "health_beauty" {
		t.Fatalf("category chart under status selection = %q, want health_beauty", got)
	}

	table, err := metrics.QueryTable(context.Background(), dashboard.Filters{}, dashboard.TableRequest{
		Table:  "orders",
		Offset: 0,
		Limit:  1,
		Sort:   dashboard.TableSort{Key: "revenue", Direction: "asc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if table.TotalRows != 2 {
		t.Fatalf("table total rows = %d, want 2", table.TotalRows)
	}
	if len(table.Rows) != 1 {
		t.Fatalf("table rows = %d, want 1", len(table.Rows))
	}
	if got := table.Rows[0]["order_id"]; got != "o2" {
		t.Fatalf("first table order = %v, want o2", got)
	}

	if err := metrics.RefreshCache(context.Background()); err != nil {
		t.Fatalf("refresh cache: %v", err)
	}
}

func pointSelected(points []dashboard.Point, label string) bool {
	for _, point := range points {
		if point.Label == label {
			return point.Selected
		}
	}
	return false
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
