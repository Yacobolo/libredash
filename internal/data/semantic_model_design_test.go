package data

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

func TestSemanticModelDesignRuntimeQueryByRelatedDimension(t *testing.T) {
	dir := t.TempDir()
	catalogPath := writeSemanticDesignRuntimeWorkspace(t, dir)
	writeFixture(t, dir, "orders.csv", `order_id,customer_id,purchase_timestamp,revenue
o1,c1,2018-01-01 00:00:00,100
o2,c2,2018-01-02 00:00:00,50
o3,c1,2018-01-03 00:00:00,25
`)
	writeFixture(t, dir, "customers.csv", `customer_id,state
c1,SP
c2,RJ
`)

	metrics, err := NewDuckDBMetricsFromCatalog(dir, catalogPath, dir)
	if err != nil {
		t.Fatalf("NewDuckDBMetricsFromCatalog() error = %v, want semantic-model-first runtime", err)
	}
	defer metrics.Close()

	patch, err := metrics.QueryDashboardPage(context.Background(), "sales", "overview", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if patch.Status.Error != "" {
		t.Fatalf("status error = %q", patch.Status.Error)
	}
	rows := patch.Visuals["revenue_by_state"].Data
	if !hasSemanticDesignDatum(rows, "label", "SP", "value", 125) {
		t.Fatalf("rows missing SP revenue 125: %#v", rows)
	}
	if !hasSemanticDesignDatum(rows, "label", "RJ", "value", 50) {
		t.Fatalf("rows missing RJ revenue 50: %#v", rows)
	}
}

func hasSemanticDesignDatum(rows []dashboard.Datum, dimKey, dimValue, measureKey string, measureValue int) bool {
	for _, row := range rows {
		if datumString(row, dimKey) == dimValue && datumInt(row, measureKey) == measureValue {
			return true
		}
	}
	return false
}

func writeSemanticDesignRuntimeWorkspace(t *testing.T, dir string) string {
	t.Helper()
	writeDataFixture(t, filepath.Join(dir, "catalog.yaml"), `
workspace:
  id: libredash
  title: LibreDash Workspace
semantic_models:
  - id: olist
    title: Olist Commerce
    path: model.yaml
dashboards:
  - id: sales
    title: Sales
    path: dashboard.yaml
`)
	writeDataFixture(t, filepath.Join(dir, "model.yaml"), `
name: olist
title: Olist Commerce
connections:
  olist:
    kind: local
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
  olist_customers:
    connection: olist
    path: customers.csv
    format: csv
models:
  orders:
    source: olist_orders
  customers:
    source: olist_customers
semantic_models:
  olist:
    base_table: orders
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
          customer_id: {expr: customer_id}
          purchase_timestamp: {expr: purchase_timestamp, type: time}
          revenue: {expr: revenue, type: number}
      customers:
        model: customers
        primary_key: customer_id
        fields:
          customer_id: {expr: customer_id}
          state: {expr: state}
    relationships:
      - from: orders.customer_id
        to: customers.customer_id
        cardinality: many_to_one
        active: true
    measures:
      defaults:
        table: orders
        grain: order_id
        time: orders.purchase_timestamp
        grains: [day, week, month, quarter, year]
      revenue:
        expr: SUM(orders.revenue)
        format: currency
`)
	writeDataFixture(t, filepath.Join(dir, "dashboard.yaml"), `
id: sales
title: Sales
semantic_model: olist
filters: {}
visuals:
  revenue_by_state:
    title: Revenue by state
    type: bar
    query:
      dimensions:
        state: customers.state
      measures:
        revenue:
pages:
  - id: overview
    title: Overview
    visuals:
      - id: revenue_by_state
        kind: bar_chart
        visual: revenue_by_state
        placement: {col: 1, row: 1, col_span: 4, row_span: 4}
tables: {}
`)
	return filepath.Join(dir, "catalog.yaml")
}

func writeDataFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
