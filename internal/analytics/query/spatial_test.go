package query

import (
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestSpatialAggregationExecutesAcrossEveryGovernedRowBeforeFeatureCap(t *testing.T) {
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		"CREATE SCHEMA model",
		"CREATE TABLE model.orders(order_id VARCHAR, customer_id VARCHAR, ordered_at TIMESTAMP, revenue DOUBLE, status VARCHAR, latitude DOUBLE, longitude DOUBLE)",
		"INSERT INTO model.orders SELECT 'order-' || i, 'customer-' || i, TIMESTAMP '2026-01-01', 1, 'paid', -30 + (i % 60), -120 + (i % 240) FROM range(10000) t(i)",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := NewPlanner(testModel()).PlanSpatial(SpatialRequest{
		Table:      "orders",
		Dimensions: []Field{{Field: "orders.order_id", Alias: "order_id"}, {Field: "orders.latitude", Alias: "latitude"}, {Field: "orders.longitude", Alias: "longitude"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
		Latitude:   Field{Field: "orders.latitude", Alias: "latitude"}, Longitude: Field{Field: "orders.longitude", Alias: "longitude"},
		West: -180, South: -85, East: 180, North: 85, Width: 320, Height: 240, FeatureCap: 10, Precision: SpatialPrecisionAggregated,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := db.Query(plan.SQL, plan.Args...)
	if err != nil {
		t.Fatalf("execute spatial plan: %v\n%s", err, plan.SQL)
	}
	defer result.Close()
	var rows, total int
	var revenue float64
	for result.Next() {
		var orderID string
		var latitude, longitude, cellRevenue float64
		var cellTotal int
		if err := result.Scan(&orderID, &latitude, &longitude, &cellRevenue, &cellTotal); err != nil {
			t.Fatal(err)
		}
		_ = fmt.Sprintf("%s/%f/%f", orderID, latitude, longitude)
		rows++
		revenue += cellRevenue
		total = cellTotal
	}
	if err := result.Err(); err != nil {
		t.Fatal(err)
	}
	if rows == 0 || rows > 10 {
		t.Fatalf("aggregated rows = %d, want 1..10", rows)
	}
	if total != 10_000 || revenue != 10_000 {
		t.Fatalf("aggregated total/revenue = %d/%v, want 10000/10000", total, revenue)
	}
}
