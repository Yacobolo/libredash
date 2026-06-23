package semantic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSemanticModelDesignWorkspaceContract(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspace(t)

	workspace, err := LoadWorkspace(catalogPath)
	if err != nil {
		t.Fatalf("LoadWorkspace() error = %v, want semantic-model-first workspace to load", err)
	}
	model := workspace.Models["olist"]
	if model == nil {
		t.Fatal("semantic model olist was not loaded")
	}
	if _, ok := model.Sources["olist_orders"]; !ok {
		t.Fatalf("raw source olist_orders missing: %#v", model.Sources)
	}
	if _, ok := model.Tables["orders"]; !ok {
		t.Fatalf("model table orders missing: %#v", model.Tables)
	}
	if _, ok := workspace.Dashboards["sales"]; !ok {
		t.Fatalf("dashboard sales missing: %#v", workspace.Dashboards)
	}
}

func TestSemanticModelDesignMeasureDefaultsAndOwnership(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspace(t)

	workspace, err := LoadWorkspace(catalogPath)
	if err != nil {
		t.Fatalf("LoadWorkspace() error = %v, want semantic model measures to load", err)
	}
	model := workspace.Models["olist"]
	if model == nil {
		t.Fatal("semantic model olist missing")
	}

	revenue, err := model.ResolveMeasure("revenue")
	if err != nil {
		t.Fatalf("ResolveMeasure(revenue) error = %v, want semantic-model measure", err)
	}
	if revenue.Table != "orders" {
		t.Fatalf("revenue table = %q, want inherited default table orders", revenue.Table)
	}
	if revenue.Expression != "SUM(orders.revenue)" {
		t.Fatalf("revenue expression = %q, want SUM(orders.revenue)", revenue.Expression)
	}
}

func TestSemanticModelDesignRejectsSourceSemantics(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
    measures:
      revenue:
        expr: SUM(revenue)
`)

	_, err := LoadWorkspace(catalogPath)
	if err == nil || !strings.Contains(err.Error(), "sources do not define business semantics") {
		t.Fatalf("LoadWorkspace() error = %v, want raw source semantics rejection", err)
	}
}

func TestSemanticModelDesignRequiresExplicitModelTable(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithSemanticFragment(t, `
semantic_models:
  olist:
    tables:
      missing:
        primary_key: id
        fields:
          id: {expr: id}
    measures:
      defaults: {table: missing, grain: id}
      count: {expr: COUNT(DISTINCT missing.id)}
`)

	_, err := LoadWorkspace(catalogPath)
	if err == nil || !strings.Contains(err.Error(), `references unknown model "missing"`) {
		t.Fatalf("LoadWorkspace() error = %v, want missing model rejection", err)
	}
}

func TestSemanticModelDesignExplicitPassThroughModelSucceeds(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithSemanticFragment(t, `
semantic_models:
  olist:
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
          customer_id: {expr: customer_id}
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
      defaults: {table: orders, grain: order_id}
      customer_count: {expr: COUNT(DISTINCT orders.customer_id)}
      revenue: {expr: COUNT(DISTINCT orders.order_id)}
`)

	if _, err := LoadWorkspace(catalogPath); err != nil {
		t.Fatalf("LoadWorkspace() error = %v, want explicit passthrough model to load", err)
	}
}

func TestSemanticModelDesignRejectsUnknownRelationshipEndpoint(t *testing.T) {
	tests := map[string]string{
		"table": `
semantic_models:
  olist:
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
    relationships:
      - from: orders.order_id
        to: missing.id
        cardinality: many_to_one
        active: true
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue)}
`,
		"field": `
semantic_models:
  olist:
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
      customers:
        model: customers
        primary_key: customer_id
        fields:
          customer_id: {expr: customer_id}
    relationships:
      - from: orders.missing_customer_id
        to: customers.customer_id
        cardinality: many_to_one
        active: true
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue)}
`,
	}
	for name, semanticFragment := range tests {
		t.Run(name, func(t *testing.T) {
			catalogPath := writeSemanticModelDesignWorkspaceWithSemanticFragment(t, semanticFragment)
			_, err := LoadWorkspace(catalogPath)
			if err == nil || !strings.Contains(err.Error(), "relationship") {
				t.Fatalf("LoadWorkspace() error = %v, want relationship endpoint rejection", err)
			}
		})
	}
}

func TestSemanticModelDesignRejectsMeasureSpecificUnsafePath(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
  olist_customers:
    connection: olist
    path: customers.csv
    format: csv
  olist_refunds:
    connection: olist
    path: refunds.csv
    format: csv

models:
  orders:
    source: olist_orders
  customers:
    source: olist_customers
  refunds:
    source: olist_refunds

semantic_models:
  olist:
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          customer_id: {expr: customer_id}
      customers:
        model: customers
        primary_key: customer_id
        fields:
          customer_id: {expr: customer_id}
      refunds:
        model: refunds
        primary_key: refund_id
        fields:
          refund_id: {expr: refund_id}
    relationships:
      - from: orders.customer_id
        to: customers.customer_id
        cardinality: many_to_one
        active: true
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue)}
      refunds:
        table: refunds
        grain: refund_id
        expr: SUM(refunds.amount)
`)

	_, err := LoadWorkspace(catalogPath)
	if err == nil || !strings.Contains(err.Error(), "unsafe relationship path") {
		t.Fatalf("LoadWorkspace() error = %v, want measure-specific unsafe path rejection", err)
	}
}

func TestSemanticModelDesignSQLModelRequiresExplicitSources(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    transform:
      sql: SELECT order_id FROM source.olist_orders
semantic_models:
  olist:
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
    measures:
      defaults: {table: orders, grain: order_id}
      order_count: {expr: COUNT(DISTINCT orders.order_id)}
      revenue: {expr: COUNT(DISTINCT orders.order_id)}
`)

	_, err := LoadWorkspace(catalogPath)
	if err == nil || !strings.Contains(err.Error(), "requires sources") {
		t.Fatalf("LoadWorkspace() error = %v, want SQL sources rejection", err)
	}
}

func TestSemanticModelDesignSQLModelWithExplicitSourcesSucceeds(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
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
    sources: [olist_orders]
    transform:
      sql: SELECT order_id, customer_id FROM source.olist_orders
  customers:
    source: olist_customers
semantic_models:
  olist:
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
          customer_id: {expr: customer_id}
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
      defaults: {table: orders, grain: order_id}
      order_count: {expr: COUNT(DISTINCT orders.order_id)}
      revenue: {expr: COUNT(DISTINCT orders.order_id)}
`)

	if _, err := LoadWorkspace(catalogPath); err != nil {
		t.Fatalf("LoadWorkspace() error = %v, want SQL model with explicit sources to load", err)
	}
}

func TestSemanticModelDesignSQLModelSourceMismatchFails(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
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
    sources: [olist_orders]
    transform:
      sql: SELECT order_id FROM source.olist_customers
semantic_models:
  olist:
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
    measures:
      defaults: {table: orders, grain: order_id}
      order_count: {expr: COUNT(DISTINCT orders.order_id)}
`)

	_, err := LoadWorkspace(catalogPath)
	if err == nil || !strings.Contains(err.Error(), "do not match declared sources") {
		t.Fatalf("LoadWorkspace() error = %v, want SQL source mismatch rejection", err)
	}
}

func TestSemanticModelDesignRejectsAmbiguousAndUnsafeRelationshipPaths(t *testing.T) {
	tests := map[string]string{
		"one_to_many": `
	semantic_models:
	  olist:
	    tables:
	      orders:
	        model: orders
	        primary_key: order_id
	        fields:
	          order_id: {expr: order_id}
	      items:
	        model: items
	        primary_key: item_id
	        fields:
	          order_id: {expr: order_id}
    relationships:
      - from: orders.order_id
        to: items.order_id
        cardinality: one_to_many
        active: true
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue)}
`,
		"inactive": `
	semantic_models:
	  olist:
	    tables:
	      orders:
	        model: orders
	        primary_key: order_id
	        fields:
	          customer_id: {expr: customer_id}
	      customers:
	        model: customers
	        primary_key: customer_id
	        fields:
	          customer_id: {expr: customer_id}
    relationships:
      - from: orders.customer_id
        to: customers.customer_id
        cardinality: many_to_one
        active: false
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue)}
`,
		"ambiguous": `
	semantic_models:
	  olist:
	    tables:
	      orders:
	        model: orders
	        primary_key: order_id
	        fields:
	          customer_id: {expr: customer_id}
	          customer_id_alt: {expr: customer_id}
	      customers:
	        model: customers
	        primary_key: customer_id
	        fields:
	          customer_id: {expr: customer_id}
    relationships:
      - from: orders.customer_id
        to: customers.customer_id
        cardinality: many_to_one
        active: true
      - from: orders.customer_id_alt
        to: customers.customer_id
        cardinality: many_to_one
        active: true
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue)}
`,
	}
	for name, semanticFragment := range tests {
		t.Run(name, func(t *testing.T) {
			catalogPath := writeSemanticModelDesignWorkspaceWithSemanticFragment(t, semanticFragment)
			_, err := LoadWorkspace(catalogPath)
			if err == nil || !strings.Contains(err.Error(), "unsafe relationship path") {
				t.Fatalf("LoadWorkspace() error = %v, want unsafe relationship path rejection", err)
			}
		})
	}
}

func writeSemanticModelDesignWorkspace(t *testing.T) string {
	t.Helper()
	return writeSemanticModelDesignWorkspaceWithSemanticFragment(t, `
	semantic_models:
	  olist:
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
      order_count:
        expr: COUNT(DISTINCT orders.order_id)
        format: integer
`)
}

func writeSemanticModelDesignWorkspaceWithSemanticFragment(t *testing.T, semanticFragment string) string {
	t.Helper()
	return writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
	  olist_customers:
	    connection: olist
	    path: customers.csv
	    format: csv
	  olist_items:
	    connection: olist
	    path: items.csv
	    format: csv

	models:
	  orders:
	    sources: [olist_orders]
	    transform:
	      sql: |
	        SELECT order_id, customer_id, purchase_timestamp, revenue
	        FROM source.olist_orders
	  customers:
	    source: olist_customers
	  items:
	    source: olist_items
	`+semanticFragment)
}

func writeSemanticModelDesignWorkspaceWithModelFragment(t *testing.T, modelFragment string) string {
	t.Helper()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "catalog.yaml"), `
workspace:
  id: libredash
  title: LibreDash Workspace
  description: Local BI workspace.

semantic_models:
  - id: olist
    title: Olist Commerce
    path: model.yaml
    description: Olist model.

dashboards:
  - id: sales
    title: Sales
    path: dashboard.yaml
    description: Sales dashboard.
`)
	mustWriteFile(t, filepath.Join(dir, "model.yaml"), `
name: olist
title: Olist Commerce
description: Olist semantic model.

connections:
  olist:
    kind: local
`+modelFragment)
	mustWriteFile(t, filepath.Join(dir, "dashboard.yaml"), `
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
    encode:
      x: state
      y: revenue
tables:
  orders:
    title: Orders
    query:
      table: orders
      fields:
        - orders.order_id
        - customers.state
pages:
  - id: overview
    title: Overview
    visuals: []
`)
	return filepath.Join(dir, "catalog.yaml")
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	content = strings.ReplaceAll(content, "\t", "")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
