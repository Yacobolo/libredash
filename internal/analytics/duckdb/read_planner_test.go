package duckdb

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	analyticsmaterialize "github.com/Yacobolo/leapview/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	_ "github.com/duckdb/duckdb-go/v2"
)

func TestPlanModelTableCompilesCSVSQLModelToInlineRelations(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningModel(map[string][]string{
		"orders":   {"order_id", "customer_id", "status"},
		"payments": {"order_id", "payment_value"},
	}, semanticmodel.Table{
		Sources:    []string{"orders", "payments"},
		PrimaryKey: "order_id",
		Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}},
		Transform: semanticmodel.Transform{SQL: `
			SELECT o.order_id, o.customer_id, SUM(try_cast(p.payment_value AS DOUBLE)) AS revenue
			FROM source.orders o
			JOIN source.payments p USING (order_id)
			WHERE o.status = 'delivered'
			GROUP BY o.order_id, o.customer_id
		`},
	})
	validateAndBindPlanningManagedRoot(t, model, managedPlanningRoot)

	plan, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != analyticsmaterialize.PlanModeProjectedSourceInline {
		t.Fatalf("mode = %q, want inline", plan.Mode)
	}
	for _, want := range []string{
		"CREATE OR REPLACE TABLE model.orders AS",
		"FROM (SELECT customer_id, order_id, status FROM read_csv('/managed/revision/orders.csv')) o",
		"JOIN (SELECT order_id, payment_value FROM read_csv('/managed/revision/payments.csv')) p",
	} {
		if !strings.Contains(plan.SQL, want) {
			t.Fatalf("plan SQL = %s, want %q", plan.SQL, want)
		}
	}
	if strings.Contains(plan.SQL, "source.orders") || strings.Contains(plan.SQL, "source.payments") {
		t.Fatalf("plan SQL still contains source refs: %s", plan.SQL)
	}
}

func TestPlanModelTableCompilesDirectSourceFromColumns(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningModel(map[string][]string{
		"orders": {"raw_order_id", "gross_revenue", "status"},
	}, semanticmodel.Table{
		Source:     "orders",
		PrimaryKey: "order_id",
		Columns: map[string]semanticmodel.ModelColumn{
			"order_id": {SourceField: "raw_order_id"},
			"revenue":  {SourceField: "gross_revenue"},
			"status":   {},
		},
		Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}, "status": {Label: "Status"}},
	})
	validateAndBindPlanningManagedRoot(t, model, managedPlanningRoot)

	plan, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != analyticsmaterialize.PlanModeDirectSourceRead {
		t.Fatalf("mode = %q, want direct", plan.Mode)
	}
	want := "CREATE OR REPLACE TABLE model.orders AS SELECT raw_order_id AS order_id, gross_revenue AS revenue, status FROM read_csv('/managed/revision/orders.csv')"
	if plan.SQL != want {
		t.Fatalf("plan SQL = %q, want %q", plan.SQL, want)
	}
}

func TestPlanModelTableCompilesCountStarToInlineRowPresence(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningModel(map[string][]string{
		"orders": {"order_id", "customer_id"},
	}, semanticmodel.Table{
		Sources:    []string{"orders"},
		PrimaryKey: "order_count",
		Dimensions: map[string]semanticmodel.MetricDimension{
			"order_count": {Label: "Order Count"},
		},
		Transform: semanticmodel.Transform{SQL: `SELECT COUNT(*) AS order_count FROM source.orders`},
	})
	validateAndBindPlanningManagedRoot(t, model, managedPlanningRoot)

	plan, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "FROM (SELECT 1 AS __leapview_row_present FROM read_csv('/managed/revision/orders.csv'))") {
		t.Fatalf("plan SQL = %s, want row-presence inline relation", plan.SQL)
	}
}

func TestPlanModelTableAliasesUnaliasedInlineSourceRefs(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/orders.csv", []byte("order_id\n1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := planningModel(map[string][]string{
		"orders": {"order_id"},
	}, semanticmodel.Table{
		Sources:    []string{"orders"},
		PrimaryKey: "order_id",
		Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}},
		Transform:  semanticmodel.Transform{SQL: `SELECT orders.order_id FROM source.orders`},
	})
	validateAndBindPlanningManagedRoot(t, model, dir)

	plan, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "AS orders") {
		t.Fatalf("plan SQL = %s, want alias for unaliased source relation", plan.SQL)
	}
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA model"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, plan.SQL); err != nil {
		t.Fatalf("executing plan SQL failed: %v\nSQL: %s", err, plan.SQL)
	}
}

func TestPlanModelTablePreservesMixedInlineSourceAliases(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningModel(map[string][]string{
		"orders":   {"order_id"},
		"payments": {"order_id", "payment_value"},
	}, semanticmodel.Table{
		Sources:    []string{"orders", "payments"},
		PrimaryKey: "order_id",
		Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}},
		Transform:  semanticmodel.Transform{SQL: `SELECT orders.order_id, p.payment_value FROM source.orders JOIN source.payments p USING (order_id)`},
	})
	validateAndBindPlanningManagedRoot(t, model, managedPlanningRoot)

	plan, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "FROM (SELECT order_id FROM read_csv('/managed/revision/orders.csv')) AS orders") {
		t.Fatalf("plan SQL = %s, want generated alias for orders", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "JOIN (SELECT order_id, payment_value FROM read_csv('/managed/revision/payments.csv')) p") {
		t.Fatalf("plan SQL = %s, want explicit payments alias preserved", plan.SQL)
	}
}

func TestPlanModelTableRejectsQualifiedSourceColumnRefs(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningModel(map[string][]string{
		"orders": {"order_id"},
	}, semanticmodel.Table{
		Sources:    []string{"orders"},
		PrimaryKey: "order_id",
		Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}},
		Transform:  semanticmodel.Transform{SQL: `SELECT source.orders.order_id FROM source.orders`},
	})
	validateAndBindPlanningManagedRoot(t, model, managedPlanningRoot)

	_, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err == nil || !strings.Contains(err.Error(), `column reference "source.orders.order_id" must use a table alias`) {
		t.Fatalf("PlanModelTable error = %v, want qualified source column rejection", err)
	}
}

func TestPlanModelTableCompilesEligibleQuackModelToWholeQuery(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningQuackModel()
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != analyticsmaterialize.PlanModeWholeQueryPushdown {
		t.Fatalf("mode = %q, want whole query pushdown", plan.Mode)
	}
	for _, want := range []string{
		"FROM quack_query('quack:quack.example.com:443'",
		"FROM main.orders o",
		"JOIN main.payments p USING (order_id)",
	} {
		if !strings.Contains(plan.SQL, want) {
			t.Fatalf("plan SQL = %s, want %q", plan.SQL, want)
		}
	}
	if strings.Contains(plan.SQL, "source.") || strings.Contains(plan.SQL, "secret-token") {
		t.Fatalf("plan SQL contains source namespace or secret: %s", plan.SQL)
	}
}

func TestPlanModelTablePushesDownQuackWithoutDiscoveredSourceSchemas(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningQuackModel()
	for name, source := range model.Sources {
		source.Schema = semanticmodel.TableSchema{}
		model.Sources[name] = source
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != analyticsmaterialize.PlanModeWholeQueryPushdown {
		t.Fatalf("mode = %q, want whole query pushdown", plan.Mode)
	}
	if strings.Contains(plan.SQL, "secret-token") || strings.Contains(plan.SQL, "source.") {
		t.Fatalf("plan SQL contains secret or source namespace: %s", plan.SQL)
	}
}

func TestPlanModelTablePushesDownQuackBeforeEmptyExplainPlan(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningQuackModel()
	table := model.Tables["orders"]
	table.Sources = []string{"orders"}
	table.SourceDependencies = []string{"orders"}
	table.Transform.SQL = `SELECT * FROM source.orders WHERE 1=0`
	model.Tables["orders"] = table
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != analyticsmaterialize.PlanModeWholeQueryPushdown {
		t.Fatalf("mode = %q, want whole query pushdown", plan.Mode)
	}
	if !strings.Contains(plan.SQL, "FROM main.orders WHERE 1=0") {
		t.Fatalf("plan SQL = %s, want rewritten empty-result transform", plan.SQL)
	}
	if strings.Contains(plan.SQL, "source.orders") || strings.Contains(plan.SQL, "secret-token") {
		t.Fatalf("plan SQL contains source namespace or secret: %s", plan.SQL)
	}
}

func TestPlanModelTableFailsClosedWhenInlineExplainOmitsSourceScan(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningModel(map[string][]string{
		"orders": {"order_id", "customer_id"},
	}, semanticmodel.Table{
		Sources:    []string{"orders"},
		PrimaryKey: "order_id",
		Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}},
		Transform:  semanticmodel.Transform{SQL: `SELECT * FROM source.orders WHERE 1=0`},
	})
	validateAndBindPlanningManagedRoot(t, model, managedPlanningRoot)

	_, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err == nil || !strings.Contains(err.Error(), `SQL plan did not expose projections for source "orders"`) {
		t.Fatalf("PlanModelTable error = %v, want fail-closed missing projection error", err)
	}
}

func TestPlanModelTableFailsClosedForUnusedCTESourceRef(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningModel(map[string][]string{
		"orders":   {"order_id"},
		"payments": {"order_id"},
	}, semanticmodel.Table{
		Sources:    []string{"orders", "payments"},
		PrimaryKey: "order_id",
		Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}},
		Transform: semanticmodel.Transform{SQL: `
			WITH unused_payments AS (SELECT order_id FROM source.payments)
			SELECT order_id FROM source.orders
		`},
	})
	validateAndBindPlanningManagedRoot(t, model, managedPlanningRoot)

	_, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err == nil || !strings.Contains(err.Error(), `SQL plan did not expose projections for source "payments"`) {
		t.Fatalf("PlanModelTable error = %v, want fail-closed unused source error", err)
	}
}

func TestPlanModelTableFallsBackToInlineQuackForModelDependency(t *testing.T) {
	ctx := context.Background()
	db := openPlanningRuntimeDB(t)
	defer db.Close()
	model := planningQuackModel()
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	summary := model.Tables["orders"]
	summary.SourceDependencies = []string{"orders"}
	summary.ModelDependencies = []string{"previous_orders"}
	summary.Transform.SQL = `SELECT o.order_id FROM source.orders o JOIN model.previous_orders p USING (order_id)`
	model.Tables["orders"] = summary
	model.Tables["previous_orders"] = semanticmodel.Table{
		PrimaryKey:  "order_id",
		Columns:     map[string]semanticmodel.ModelColumn{"order_id": {Name: "order_id", Field: "previous_orders.order_id", SourceField: "order_id", Type: "VARCHAR"}},
		Dimensions:  map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}},
		Grain:       "order_id",
		Description: "stub",
	}

	plan, err := PlanModelTable(ctx, db, model, "orders", model.Tables["orders"])
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != analyticsmaterialize.PlanModeProjectedSourceInline {
		t.Fatalf("mode = %q, want inline fallback", plan.Mode)
	}
	if !strings.Contains(plan.SQL, "FROM (SELECT * FROM quack_query") || strings.Contains(plan.SQL, "source.orders") {
		t.Fatalf("plan SQL = %s, want inline quack relation without source refs", plan.SQL)
	}
}

func openPlanningRuntimeDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

const managedPlanningRoot = "/managed/revision"

func validateAndBindPlanningManagedRoot(t *testing.T, model *semanticmodel.Model, root string) {
	t.Helper()
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, connection := range model.Connections {
		if connection.Kind != "managed" {
			continue
		}
		connection.Root = filepath.Clean(root)
		model.Connections[name] = connection
	}
}

func planningModel(sourceColumns map[string][]string, table semanticmodel.Table) *semanticmodel.Model {
	sources := map[string]semanticmodel.Source{}
	for name, columns := range sourceColumns {
		schemaColumns := make([]semanticmodel.ColumnSchema, 0, len(columns))
		for index, column := range columns {
			schemaColumns = append(schemaColumns, semanticmodel.ColumnSchema{Name: column, Ordinal: index + 1, PhysicalType: "VARCHAR"})
		}
		sources[name] = semanticmodel.Source{
			Connection: "local_files",
			Path:       name + ".csv",
			Format:     "csv",
			Schema:     semanticmodel.TableSchema{Columns: schemaColumns},
		}
	}
	return &semanticmodel.Model{
		Name:        "test",
		Connections: map[string]semanticmodel.Connection{"local_files": {Kind: "managed"}},
		Sources:     sources,
		Tables:      map[string]semanticmodel.Table{"orders": table},
		Measures:    map[string]semanticmodel.MetricMeasure{},
	}
}

func planningQuackModel() *semanticmodel.Model {
	return &semanticmodel.Model{
		Name: "test",
		Connections: map[string]semanticmodel.Connection{
			"remote_quack": {
				Kind: "quack",
				Path: "quack:quack.example.com:443",
				Auth: semanticmodel.ConnectionAuth{"token": "secret-token"},
			},
		},
		Sources: map[string]semanticmodel.Source{
			"orders": {
				Connection: "remote_quack",
				Object:     "main.orders",
				Schema: semanticmodel.TableSchema{Columns: []semanticmodel.ColumnSchema{
					{Name: "order_id", Ordinal: 1, PhysicalType: "VARCHAR"},
					{Name: "customer_id", Ordinal: 2, PhysicalType: "VARCHAR"},
				}},
			},
			"payments": {
				Connection: "remote_quack",
				Object:     "main.payments",
				Schema: semanticmodel.TableSchema{Columns: []semanticmodel.ColumnSchema{
					{Name: "order_id", Ordinal: 1, PhysicalType: "VARCHAR"},
					{Name: "payment_value", Ordinal: 2, PhysicalType: "DOUBLE"},
				}},
			},
		},
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Sources:    []string{"orders", "payments"},
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}},
				Transform: semanticmodel.Transform{SQL: `
					SELECT o.order_id, SUM(p.payment_value) AS revenue
					FROM source.orders o
					JOIN source.payments p USING (order_id)
					GROUP BY o.order_id
				`},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{},
	}
}
