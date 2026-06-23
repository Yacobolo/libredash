package semantic

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestSemanticModelDesignRequiresBaseTable(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    source: olist_orders
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
	if err == nil || !strings.Contains(err.Error(), "requires base_table") {
		t.Fatalf("LoadWorkspace() error = %v, want missing base_table rejection", err)
	}
}

func TestSemanticModelDesignRejectsUnknownBaseTable(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    source: olist_orders
semantic_models:
  olist:
    base_table: missing
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
	if err == nil || !strings.Contains(err.Error(), `base_table "missing" references unknown table`) {
		t.Fatalf("LoadWorkspace() error = %v, want unknown base_table rejection", err)
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
    base_table: orders
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
    base_table: orders
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
    base_table: orders
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
    base_table: orders
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
    base_table: orders
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
	if err == nil || !strings.Contains(err.Error(), "connected relationship graph") {
		t.Fatalf("LoadWorkspace() error = %v, want measure-specific connected graph rejection", err)
	}
}

func TestSemanticModelDesignAllowsUnrelatedFactsInSeparateSemanticModels(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "catalog.yaml"), `
workspace:
  id: libredash
  title: LibreDash Workspace
semantic_models:
  - id: orders
    title: Orders
    path: orders-model.yaml
  - id: refunds
    title: Refunds
    path: refunds-model.yaml
dashboards:
  - id: sales
    title: Sales
    path: dashboard.yaml
`)
	mustWriteFile(t, filepath.Join(dir, "orders-model.yaml"), `
name: orders
title: Orders
connections:
  local:
    kind: local
sources:
  orders:
    connection: local
    path: orders.csv
    format: csv
models:
  orders:
    source: orders
semantic_models:
  orders:
    base_table: orders
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
          revenue: {expr: revenue}
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue)}
`)
	mustWriteFile(t, filepath.Join(dir, "refunds-model.yaml"), `
name: refunds
title: Refunds
connections:
  local:
    kind: local
sources:
  refunds:
    connection: local
    path: refunds.csv
    format: csv
models:
  refunds:
    source: refunds
semantic_models:
  refunds:
    base_table: refunds
    tables:
      refunds:
        model: refunds
        primary_key: refund_id
        fields:
          refund_id: {expr: refund_id}
          amount: {expr: amount}
    measures:
      defaults: {table: refunds, grain: refund_id}
      refund_amount: {expr: SUM(refunds.amount)}
`)
	mustWriteFile(t, filepath.Join(dir, "dashboard.yaml"), `
id: sales
title: Sales
semantic_model: orders
filters: {}
visuals:
  revenue:
    title: Revenue
    kind: kpi
    query:
      measures:
        revenue:
tables: {}
pages:
  - id: overview
    title: Overview
    visuals: []
`)

	workspace, err := LoadWorkspace(filepath.Join(dir, "catalog.yaml"))
	if err != nil {
		t.Fatalf("LoadWorkspace() error = %v, want unrelated facts split by semantic model to load", err)
	}
	if workspace.Models["orders"] == nil || workspace.Models["refunds"] == nil {
		t.Fatalf("workspace models = %#v, want orders and refunds semantic models", workspace.Models)
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
    base_table: orders
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
    base_table: orders
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

func TestSemanticModelDesignSQLModelWithQuotedSourceReferenceSucceeds(t *testing.T) {
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
      sql: SELECT order_id, customer_id FROM "source"."olist_orders"
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
		t.Fatalf("LoadWorkspace() error = %v, want quoted source reference to load", err)
	}
}

func TestSemanticModelDesignSQLModelRejectsRawNamespace(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    sources: [olist_orders]
    transform:
      sql: SELECT order_id FROM raw.olist_orders
semantic_models:
  olist:
    base_table: orders
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
	if err == nil || !strings.Contains(err.Error(), "model SQL must reference sources through source.<name>; raw.<name> is internal") {
		t.Fatalf("LoadWorkspace() error = %v, want raw namespace rejection", err)
	}
}

func TestSemanticModelDesignSQLModelRejectsQuotedRawNamespace(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    sources: [olist_orders]
    transform:
      sql: SELECT order_id FROM "raw"."olist_orders"
semantic_models:
  olist:
    base_table: orders
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
	if err == nil || !strings.Contains(err.Error(), "model SQL must reference sources through source.<name>; raw.<name> is internal") {
		t.Fatalf("LoadWorkspace() error = %v, want quoted raw namespace rejection", err)
	}
}

func TestSemanticModelDesignSQLScannerIgnoresCommentsAndStrings(t *testing.T) {
	model := &Model{Sources: map[string]Source{"olist_orders": {}, "order_id": {}}}
	sourceRefs, rawRefs, unqualifiedRefs := model.modelSQLSourceRefs(`
		-- raw.orders and source.fake are comments
		SELECT 'raw.orders', 'source.fake', source.order_id
		FROM source.olist_orders
		/* raw.other is also a comment */
	`)
	if !reflect.DeepEqual(sourceRefs, []string{"olist_orders"}) {
		t.Fatalf("source refs = %#v, want only executable source ref", sourceRefs)
	}
	if len(rawRefs) != 0 {
		t.Fatalf("raw refs = %#v, want comments and strings ignored", rawRefs)
	}
	if len(unqualifiedRefs) != 0 {
		t.Fatalf("unqualified refs = %#v, want dotted columns outside relation contexts ignored", unqualifiedRefs)
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
    base_table: orders
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

func TestSemanticModelDesignSQLModelRejectsUnqualifiedSourceRead(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    sources: [olist_orders]
    transform:
      sql: SELECT order_id FROM olist_orders
semantic_models:
  olist:
    base_table: orders
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
	if err == nil || !strings.Contains(err.Error(), "SQL must reference sources through source.<name>") {
		t.Fatalf("LoadWorkspace() error = %v, want unqualified source rejection", err)
	}
}

func TestSemanticModelDesignSQLModelRejectsUnqualifiedExternalRelation(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    sources: [olist_orders]
    transform:
      sql: SELECT o.order_id FROM source.olist_orders o JOIN leaked_table l ON l.order_id = o.order_id
semantic_models:
  olist:
    base_table: orders
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
	if err == nil || !strings.Contains(err.Error(), `found unqualified relation "leaked_table"`) {
		t.Fatalf("LoadWorkspace() error = %v, want hidden unqualified relation rejection", err)
	}
}

func TestSemanticModelDesignSQLModelRejectsMissingSourceRefs(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    sources: [olist_orders]
    transform:
      sql: SELECT 1 AS order_id
semantic_models:
  olist:
    base_table: orders
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
		t.Fatalf("LoadWorkspace() error = %v, want missing source reference rejection", err)
	}
}

func TestSemanticModelDesignSQLModelAllowsCTERelation(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.yaml")
	if err := os.WriteFile(modelPath, []byte(`
name: olist
connections:
  olist: {kind: local}
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    sources: [olist_orders]
    transform:
      sql: |
        WITH cleaned AS (
          SELECT order_id FROM source.olist_orders
        )
        SELECT order_id FROM cleaned
semantic_models:
  olist:
    base_table: orders
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
    measures:
      defaults: {table: orders, grain: order_id}
      order_count: {expr: COUNT(DISTINCT orders.order_id)}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(modelPath); err != nil {
		t.Fatalf("Load() error = %v, want CTE relation to load", err)
	}
}

func TestSemanticModelDesignSQLModelRejectsNonQuerySQL(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithModelFragment(t, `
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    sources: [olist_orders]
    transform:
      sql: UPDATE source.olist_orders SET order_id = order_id
semantic_models:
  olist:
    base_table: orders
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
	if err == nil || !strings.Contains(err.Error(), "must be a read-only SELECT or WITH query") {
		t.Fatalf("LoadWorkspace() error = %v, want non-query SQL rejection", err)
	}
}

func TestSemanticModelDesignSQLModelScansSubquerySourceRefs(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.yaml")
	if err := os.WriteFile(modelPath, []byte(`
name: olist
connections:
  olist: {kind: local}
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    sources: [olist_orders]
    transform:
      sql: SELECT order_id FROM (SELECT order_id FROM source.olist_orders) orders
semantic_models:
  olist:
    base_table: orders
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          order_id: {expr: order_id}
    measures:
      defaults: {table: orders, grain: order_id}
      order_count: {expr: COUNT(DISTINCT orders.order_id)}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(modelPath); err != nil {
		t.Fatalf("Load() error = %v, want subquery source reference to load", err)
	}
}

func TestSemanticModelDesignRejectsIsolatedSemanticTable(t *testing.T) {
	catalogPath := writeSemanticModelDesignWorkspaceWithSemanticFragment(t, `
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
      customers:
        model: customers
        primary_key: customer_id
        fields:
          customer_id: {expr: customer_id}
          state: {expr: state}
      items:
        model: items
        primary_key: item_id
        fields:
          item_id: {expr: item_id}
    relationships:
      - from: orders.customer_id
        to: customers.customer_id
        cardinality: many_to_one
        active: true
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue)}
`)

	_, err := LoadWorkspace(catalogPath)
	if err == nil || !strings.Contains(err.Error(), "connected relationship graph") {
		t.Fatalf("LoadWorkspace() error = %v, want isolated table rejection", err)
	}
}

func TestSemanticModelDesignRejectsDisconnectedNoMeasureModel(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.yaml")
	mustWriteFile(t, modelPath, `
name: inventory
connections:
  local:
    kind: local
sources:
  products:
    connection: local
    path: products.csv
    format: csv
  warehouses:
    connection: local
    path: warehouses.csv
    format: csv
models:
  products:
    source: products
  warehouses:
    source: warehouses
semantic_models:
  inventory:
    base_table: products
    tables:
      products:
        model: products
        primary_key: product_id
        fields:
          product_id: {expr: product_id}
      warehouses:
        model: warehouses
        primary_key: warehouse_id
        fields:
          warehouse_id: {expr: warehouse_id}
`)

	_, err := Load(modelPath)
	if err == nil || !strings.Contains(err.Error(), "connected relationship graph") {
		t.Fatalf("Load() error = %v, want disconnected no-measure model rejection", err)
	}
}

func TestSemanticModelDesignRejectsAmbiguousAndUnsafeRelationshipPaths(t *testing.T) {
	tests := map[string]struct {
		fragment string
		want     string
	}{
		"one_to_many": {
			fragment: `
	semantic_models:
	  olist:
	    base_table: orders
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
			want: "unsafe relationship path",
		},
		"inactive": {
			fragment: `
	semantic_models:
	  olist:
	    base_table: orders
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
			want: "unsafe relationship path",
		},
		"ambiguous": {
			fragment: `
	semantic_models:
	  olist:
	    base_table: orders
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
			want: "ambiguous relationship path",
		},
		"ambiguous_different_lengths": {
			fragment: `
	semantic_models:
	  olist:
	    base_table: orders
	    tables:
	      orders:
	        model: orders
	        primary_key: order_id
	        fields:
	          customer_id: {expr: customer_id}
	          item_id: {expr: item_id}
	      items:
	        model: items
	        primary_key: item_id
	        fields:
	          item_id: {expr: item_id}
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
	        active: true
	      - from: orders.item_id
	        to: items.item_id
	        cardinality: many_to_one
	        active: true
	      - from: items.customer_id
	        to: customers.customer_id
	        cardinality: many_to_one
	        active: true
	    measures:
	      defaults: {table: orders, grain: order_id}
	      revenue: {expr: SUM(orders.revenue)}
`,
			want: "ambiguous relationship path",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			catalogPath := writeSemanticModelDesignWorkspaceWithSemanticFragment(t, tt.fragment)
			_, err := LoadWorkspace(catalogPath)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadWorkspace() error = %v, want %q rejection", err, tt.want)
			}
		})
	}
}

func writeSemanticModelDesignWorkspace(t *testing.T) string {
	t.Helper()
	return writeSemanticModelDesignWorkspaceWithSemanticFragment(t, `
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
