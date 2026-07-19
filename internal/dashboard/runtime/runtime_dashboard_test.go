package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	materializeruntime "github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/consumer"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
)

type runtimeAuditRecorder struct {
	queries []dataquery.Query
	results []dataquery.Result
}

func (r *runtimeAuditRecorder) RecordDataQuery(_ context.Context, query dataquery.Query, result dataquery.Result) error {
	r.queries = append(r.queries, query)
	r.results = append(r.results, result)
	return nil
}

func newLegacyRuntime(t *testing.T, dataDir string) (*Service, error) {
	t.Helper()
	return newManagedFixtureRuntime(dataDir, "sales")
}

func newOperationsRuntime(t *testing.T, dataDir string) (*Service, error) {
	t.Helper()
	return newManagedFixtureRuntime(dataDir, "operations")
}

func newManagedFixtureRuntime(dataDir, workspaceID string) (*Service, error) {
	projectPath := filepath.Join("..", "..", "..", "dashboards", "libredash.yaml")
	compiled, err := workspacecompiler.CompileProject(projectPath, workspacecompiler.Options{})
	if err != nil {
		return nil, err
	}
	compiledWorkspace, ok := compiled.Workspaces[workspaceID]
	if !ok {
		return nil, fmt.Errorf("showcase project has no %s workspace", workspaceID)
	}
	bindManagedFixtureRoots(compiledWorkspace.Definition, dataDir)
	return NewFromDefinition(filepath.Join(dataDir, workspaceID), testDataRuntimeFactory{}, compiledWorkspace.Definition)
}

func bindManagedFixtureRoots(definition *workspace.Definition, root string) {
	for _, model := range definition.Models {
		for name, connection := range model.Connections {
			if connection.Kind != "managed" {
				continue
			}
			connection.Root = root
			model.Connections[name] = connection
		}
	}
}

func TestWorkspaceRuntimeUsesSingleDuckDBForSharedModelTables(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "orders.csv", `order_id,revenue
o1,10
o2,20
`)
	definition := sharedOrdersWorkspaceDefinition(t)
	bindManagedFixtureRoots(definition, dir)
	metrics, err := NewFromDefinition(dir, testDataRuntimeFactory{}, definition)
	if err != nil {
		t.Fatal(err)
	}

	for _, modelID := range []string{"model_a", "model_b"} {
		runtime := metrics.runtimes[modelID]
		if runtime == nil || !runtime.ready {
			t.Fatalf("%s runtime is not ready: %#v", modelID, runtime)
		}
		count, err := runtime.data.Count(context.Background(), reportdef.CountQuery{Table: "orders"})
		if err != nil {
			t.Fatalf("%s count: %v", modelID, err)
		}
		if count != 2 {
			t.Fatalf("%s count = %d, want 2", modelID, count)
		}
	}
	if err := metrics.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "catalog.sqlite")); err != nil {
		t.Fatalf("catalog.sqlite stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data")); err != nil {
		t.Fatalf("data dir stat error = %v", err)
	}
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		"LOAD sqlite",
		"LOAD ducklake",
		"ATTACH 'ducklake:sqlite:" + strings.ReplaceAll(filepath.Join(dir, "catalog.sqlite"), "'", "''") + "' AS lake",
	} {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	var physicalTables int
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM ducklake_table_info('lake') WHERE table_name = 'orders'").Scan(&physicalTables); err != nil {
		t.Fatal(err)
	}
	if physicalTables != 1 {
		t.Fatalf("ducklake model.orders tables = %d, want 1", physicalTables)
	}
}

func sharedOrdersWorkspaceDefinition(t *testing.T) *workspace.Definition {
	t.Helper()
	modelA := sharedOrdersModel("model_a")
	modelA.Measures = map[string]semanticmodel.MetricMeasure{
		"order_count": {Fact: "orders", Aggregation: "count", Empty: "zero", Label: "Orders"},
	}
	modelB := sharedOrdersModel("model_b")
	modelB.Measures = map[string]semanticmodel.MetricMeasure{
		"revenue": {Fact: "orders", Aggregation: "sum", Input: semanticmodel.MeasureInput{Field: "orders.revenue"}, Empty: "zero", Label: "Revenue"},
	}
	if err := modelA.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := modelB.Validate(); err != nil {
		t.Fatal(err)
	}
	return &workspace.Definition{
		Catalog: workspace.Catalog{
			Workspace: workspace.CatalogWorkspace{ID: "shared", Title: "Shared"},
			SemanticModels: []workspace.CatalogModel{
				{ID: "model_a", Title: "Model A"},
				{ID: "model_b", Title: "Model B"},
			},
			Dashboards: []workspace.CatalogDashboard{{ID: "dashboard", Title: "Dashboard"}},
		},
		Models: map[string]*semanticmodel.Model{
			"model_a": modelA,
			"model_b": modelB,
		},
		Dashboards: map[string]*reportdef.Dashboard{"dashboard": {ID: "dashboard", Title: "Dashboard", SemanticModel: "model_a"}},
	}
}

func sharedOrdersModel(name string) *semanticmodel.Model {
	return &semanticmodel.Model{
		Name:              name,
		DefaultConnection: "local",
		Connections:       map[string]semanticmodel.Connection{"local": {Kind: "managed"}},
		Sources: map[string]semanticmodel.Source{
			"orders": {Connection: "local", Path: "orders.csv", Format: "csv"},
		},
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source:     "orders",
				PrimaryKey: "order_id",
				Grain:      "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Label: "Order ID"},
					"revenue":  {Label: "Revenue"},
				},
			},
		},
	}
}

func TestMissingDataReturnsSetupPatch(t *testing.T) {
	dir := t.TempDir()
	metrics, err := newLegacyRuntime(t, dir)
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

	var missing *materializeruntime.MissingDataError
	if !errors.As(metrics.runtimes["sales"].missing, &missing) {
		t.Fatalf("missing error type = %T, want *MissingDataError", metrics.runtimes["sales"].missing)
	}
}

func TestOperationsFulfillmentDashboardQueryFixture(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_delivered_customer_date
o1,c1,delivered,2018-01-01 10:00:00,2018-01-03 10:00:00
o2,c2,shipped,2018-01-05 10:00:00,2018-01-15 10:00:00
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `order_id,review_score
o1,5
o2,3
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_state
c1,SP
c2,RJ
`)

	metrics, err := newOperationsRuntime(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Close()

	patch, err := metrics.QueryDashboardPage(context.Background(), "fulfillment-operations", "overview", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if patch.Status.Error != "" {
		t.Fatalf("unexpected status error: %s", patch.Status.Error)
	}
	assertVisualKeys(t, patch, []string{"delivery_days", "delivery_speed", "orders_by_status", "review_by_status", "review_score", "total_orders"})
	if got := datumInt(patch.Visuals["total_orders"].Data[0], "value"); got != 2 {
		t.Fatalf("orders KPI value = %d, want 2", got)
	}
	if len(patch.Visuals["orders_by_status"].Data) == 0 {
		t.Fatal("orders by status chart has no data")
	}
}

func TestDashboardPageQueriesFlowThroughAuditedDataQueryBoundary(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_delivered_customer_date
o1,c1,delivered,2018-01-01 10:00:00,2018-01-03 10:00:00
o2,c2,shipped,2018-01-05 10:00:00,2018-01-15 10:00:00
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `order_id,review_score
o1,5
o2,3
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_state
c1,SP
c2,RJ
`)

	metrics, err := newOperationsRuntime(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Close()

	recorder := &runtimeAuditRecorder{}
	ctx := dataquery.WithAuditRecorder(context.Background(), recorder)
	ctx = dataquery.WithMetadata(ctx, dataquery.Metadata{PrincipalID: "test_principal"})
	patch, err := metrics.QueryDashboardPage(ctx, "fulfillment-operations", "overview", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if patch.Status.Error != "" {
		t.Fatalf("unexpected status error: %s", patch.Status.Error)
	}
	if len(recorder.queries) == 0 {
		t.Fatal("dashboard page query recorded no dataquery events")
	}
	for _, query := range recorder.queries {
		if query.WorkspaceID != "operations" {
			t.Fatalf("query workspace = %q, want operations: %#v", query.WorkspaceID, query)
		}
		if query.Surface != dataquery.SurfaceDashboard {
			t.Fatalf("query surface = %q, want dashboard: %#v", query.Surface, query)
		}
		if query.PrincipalID != "test_principal" {
			t.Fatalf("query principal = %q, want test_principal: %#v", query.PrincipalID, query)
		}
	}
	for _, result := range recorder.results {
		if result.PlanningMS <= 0 || result.DatabaseMS <= 0 {
			t.Fatalf("query stage timings were not populated: %#v", result)
		}
	}
	batchedKPIs := false
	for _, query := range recorder.queries {
		if query.Kind == dataquery.KindSemanticAggregate && len(query.Fields) == 0 && len(query.Measures) == 3 {
			batchedKPIs = true
			break
		}
	}
	if !batchedKPIs {
		t.Fatalf("compatible KPI queries were not batched: %#v", recorder.queries)
	}

	recorder.queries = nil
	recorder.results = nil
	options := map[string][]dashboard.FilterOption{}
	err = metrics.ExecuteConsumersPage(ctx, consumer.Request{DashboardID: "fulfillment-operations", PageID: "overview", Targets: []consumer.Target{
		{Kind: consumer.KindFilterOptions, ID: "state"},
		{Kind: consumer.KindFilterOptions, ID: "status"},
	}}, func(result consumer.Result) bool {
		for id, values := range result.FilterOptions {
			options[id] = values
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(options["state"]) == 0 || len(options["status"]) == 0 {
		t.Fatalf("targeted filter options = %#v", options)
	}
	for _, result := range recorder.results {
		if result.CacheOutcome != dataquery.CacheHit {
			t.Fatalf("warm filter option cache outcome = %q, want hit: %#v", result.CacheOutcome, recorder.results)
		}
	}
	recorder.queries = nil
	recorder.results = nil
	visuals := map[string]dashboard.Visual{}
	progress := []consumer.Progress{}
	err = metrics.ExecuteConsumersPage(ctx, consumer.Request{DashboardID: "fulfillment-operations", PageID: "overview", Progress: func(value consumer.Progress) {
		progress = append(progress, value)
	}, Targets: []consumer.Target{
		{Kind: consumer.KindVisual, ID: "total_orders"},
		{Kind: consumer.KindVisual, ID: "delivery_days"},
		{Kind: consumer.KindVisual, ID: "review_score"},
	}}, func(result consumer.Result) bool {
		visuals[result.Target.ID] = result.Visual
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(visuals) != 3 || len(visuals["total_orders"].Data) == 0 {
		t.Fatalf("targeted visuals = %#v", visuals)
	}
	if len(recorder.queries) != 1 || len(recorder.queries[0].Measures) != 3 {
		t.Fatalf("targeted KPI queries = %#v, want one three-measure query", recorder.queries)
	}
	if len(progress) != 2 || progress[0] != (consumer.Progress{Total: 1}) || progress[1].Completed != 1 || progress[1].Total != 1 || progress[1].WorkDuration <= 0 || progress[1].CriticalPathDuration <= 0 {
		t.Fatalf("optimizer job progress = %#v", progress)
	}
}

func TestServicePreviewsRawModelTableRows(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_approved_at,order_delivered_carrier_date,order_delivered_customer_date,order_estimated_delivery_date
o1,c1,delivered,2018-01-10 10:00:00,2018-01-10 11:00:00,2018-01-11 10:00:00,2018-01-14 10:00:00,2018-01-20 10:00:00
o2,c2,shipped,2018-01-11 10:00:00,2018-01-11 11:00:00,2018-01-12 10:00:00,2018-01-15 10:00:00,2018-01-20 10:00:00
`)
	writeFixture(t, dir, "olist_order_items_dataset.csv", `order_id,order_item_id,product_id,seller_id,shipping_limit_date,price,freight_value
o1,1,p1,s1,2018-01-12 10:00:00,100.00,10.00
o2,1,p2,s2,2018-01-13 10:00:00,150.00,15.00
`)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_sequential,payment_type,payment_installments,payment_value
o1,1,credit_card,1,110.00
o2,1,credit_card,1,165.00
`)
	writeFixture(t, dir, "olist_products_dataset.csv", `product_id,product_category_name,product_name_lenght,product_description_lenght,product_photos_qty,product_weight_g,product_length_cm,product_height_cm,product_width_cm
p1,beleza_saude,10,20,1,500,20,10,15
p2,relogios_presentes,12,24,1,600,22,11,16
`)
	writeFixture(t, dir, "product_category_name_translation.csv", `product_category_name,product_category_name_english
beleza_saude,health_beauty
relogios_presentes,watches_gifts
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_unique_id,customer_zip_code_prefix,customer_city,customer_state
c1,u1,01001,Sao Paulo,SP
c2,u2,01002,Sao Paulo,SP
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `review_id,order_id,review_score,review_comment_title,review_comment_message,review_creation_date,review_answer_timestamp
r1,o1,5,,,2018-01-15,2018-01-15 10:00:00
r2,o2,4,,,2018-01-16,2018-01-16 10:00:00
`)

	metrics, err := newLegacyRuntime(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Close()

	ctx := dataquery.WithMetadata(context.Background(), dataquery.Metadata{PrincipalID: "test_principal"})
	modelResult, err := metrics.ExecuteDataQuery(ctx, dataquery.ModelTableRows("sales", "orders", []string{"order_id", "status"}, []dataquery.Sort{{Field: "status", Direction: "desc"}}, 0, 1, true))
	if err != nil {
		t.Fatalf("unified model table query: %v", err)
	}
	if modelResult.TotalRows != 2 || len(modelResult.Rows) != 1 || modelResult.Rows[0]["order_id"] != "o2" {
		t.Fatalf("unified model table result = %#v", modelResult)
	}
	sourceResult, err := metrics.ExecuteDataQuery(ctx, dataquery.SourceRows("sales", "olist.orders", []string{"order_id", "order_status"}, []dataquery.Sort{{Field: "order_status", Direction: "desc"}}, 0, 1, true))
	if err != nil {
		t.Fatalf("unified source query: %v", err)
	}
	if sourceResult.TotalRows != 2 || len(sourceResult.Rows) != 1 || sourceResult.Rows[0]["order_id"] != "o2" {
		t.Fatalf("unified source result = %#v", sourceResult)
	}
	if _, err := metrics.ExecuteDataQuery(ctx, dataquery.ModelTableRows("sales", "missing", nil, nil, 0, 1, false)); err == nil {
		t.Fatal("missing model table preview error = nil")
	}
}

func TestServiceTableInteractiveCap(t *testing.T) {
	dir := t.TempDir()
	const rows = dashboard.TableInteractiveRowCap + 5
	var orders, items, payments, customers, reviews strings.Builder
	orders.WriteString("order_id,customer_id,order_status,order_purchase_timestamp,order_approved_at,order_delivered_carrier_date,order_delivered_customer_date,order_estimated_delivery_date\n")
	items.WriteString("order_id,order_item_id,product_id,seller_id,shipping_limit_date,price,freight_value\n")
	payments.WriteString("order_id,payment_sequential,payment_type,payment_installments,payment_value\n")
	customers.WriteString("customer_id,customer_unique_id,customer_zip_code_prefix,customer_city,customer_state\n")
	reviews.WriteString("review_id,order_id,review_score,review_comment_title,review_comment_message,review_creation_date,review_answer_timestamp\n")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&orders, "o%d,c%d,delivered,2018-01-10 10:00:00,2018-01-10 11:00:00,2018-01-11 10:00:00,2018-01-14 10:00:00,2018-01-20 10:00:00\n", i, i)
		fmt.Fprintf(&items, "o%d,1,p1,s1,2018-01-12 10:00:00,100.00,10.00\n", i)
		fmt.Fprintf(&payments, "o%d,1,credit_card,1,110.00\n", i)
		fmt.Fprintf(&customers, "c%d,u%d,01001,Sao Paulo,SP\n", i, i)
		fmt.Fprintf(&reviews, "r%d,o%d,5,,,,2018-01-15 10:00:00\n", i, i)
	}
	writeFixture(t, dir, "olist_orders_dataset.csv", orders.String())
	writeFixture(t, dir, "olist_order_items_dataset.csv", items.String())
	writeFixture(t, dir, "olist_order_payments_dataset.csv", payments.String())
	writeFixture(t, dir, "olist_products_dataset.csv", "product_id,product_category_name,product_name_lenght,product_description_lenght,product_photos_qty,product_weight_g,product_length_cm,product_height_cm,product_width_cm\np1,beleza_saude,10,20,1,500,20,10,15\n")
	writeFixture(t, dir, "olist_customers_dataset.csv", customers.String())
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", reviews.String())
	writeFixture(t, dir, "product_category_name_translation.csv", "product_category_name,product_category_name_english\nbeleza_saude,health_beauty\n")

	metrics, err := newLegacyRuntime(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Close()

	recorder := &runtimeAuditRecorder{}
	ctx := dataquery.WithAuditRecorder(context.Background(), recorder)
	ctx = dataquery.WithMetadata(ctx, dataquery.Metadata{PrincipalID: "table_test"})
	table, err := metrics.QueryTable(ctx, "executive-sales", dashboard.Filters{}, dashboard.TableRequest{Table: "orders_table", Block: "all", RequestSeq: 9})
	if err != nil {
		t.Fatal(err)
	}
	if table.Error != "" {
		t.Fatalf("table error = %q", table.Error)
	}
	if total := exactTableRows(t, table); total != rows {
		t.Fatalf("total rows = %d, want %d", total, rows)
	}
	if table.AvailableRows != dashboard.TableInteractiveRowCap {
		t.Fatalf("available rows = %d, want %d", table.AvailableRows, dashboard.TableInteractiveRowCap)
	}
	if !table.IsCapped {
		t.Fatal("table is not capped")
	}
	if got := len(table.Blocks["a"].Rows); got != dashboard.TableChunkSize {
		t.Fatalf("initial block rows = %d, want %d", got, dashboard.TableChunkSize)
	}
	if _, ok := table.Blocks["b"]; ok {
		t.Fatalf("initial table unexpectedly loaded block b: %#v", table.Blocks["b"])
	}
	if _, ok := table.Blocks["c"]; ok {
		t.Fatalf("initial table unexpectedly loaded block c: %#v", table.Blocks["c"])
	}
	if len(recorder.queries) != 2 {
		t.Fatalf("initial table data queries = %d, want rows plus count: %#v", len(recorder.queries), recorder.queries)
	}
	if recorder.queries[0].IncludeTotal {
		t.Fatalf("initial row query IncludeTotal = true: %#v", recorder.queries[0])
	}
	if strings.Contains(recorder.results[0].SQL, "COUNT(*) OVER") {
		t.Fatalf("initial row SQL blocks on a window count: %s", recorder.results[0].SQL)
	}
	if !recorder.queries[1].IncludeTotal || len(recorder.queries[1].Fields) != 0 || len(recorder.queries[1].Measures) != 0 {
		t.Fatalf("initial count query = %#v, want count-only semantic rows request", recorder.queries[1])
	}
	if got := table.Blocks["a"].RequestSeq; got != 9 {
		t.Fatalf("block request seq = %d, want 9", got)
	}

	recorder.queries = nil
	recorder.results = nil
	next, err := metrics.QueryTable(ctx, "executive-sales", dashboard.Filters{}, dashboard.TableRequest{Table: "orders_table", Block: "b", Start: dashboard.TableChunkSize, Count: dashboard.TableChunkSize, RequestSeq: 10})
	if err != nil {
		t.Fatal(err)
	}
	if next.Error != "" {
		t.Fatalf("next table block error = %q", next.Error)
	}
	if len(next.Blocks["b"].Rows) != dashboard.TableChunkSize {
		t.Fatalf("next block rows = %d, want %d", len(next.Blocks["b"].Rows), dashboard.TableChunkSize)
	}
	if total := exactTableRows(t, next); total != rows {
		t.Fatalf("next block total rows = %d, want %d", total, rows)
	}
	if len(recorder.queries) != 2 || recorder.queries[0].IncludeTotal || !recorder.queries[1].IncludeTotal {
		t.Fatalf("next block queries = %#v, want independent rows and count", recorder.queries)
	}

	overshoot, err := metrics.queries.tables.queryTableRowsPage(ctx, "executive-sales", "", dashboard.Filters{}, dashboard.TableRequest{
		Table: "orders_table", Block: "b", Start: rows + dashboard.TableChunkSize, Count: dashboard.TableChunkSize, RequestSeq: 11,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, exact := overshoot.Cardinality.ExactValue(); exact {
		t.Fatalf("overshoot cardinality = %#v, must remain inexact", overshoot.Cardinality)
	}
}

func TestServiceQueryFixture(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_approved_at,order_delivered_carrier_date,order_delivered_customer_date,order_estimated_delivery_date
o1,c1,delivered,2018-01-10 10:00:00,2018-01-10 11:00:00,2018-01-11 10:00:00,2018-01-14 10:00:00,2018-01-20 10:00:00
o2,c2,shipped,2017-06-10 10:00:00,2017-06-10 11:00:00,2017-06-11 10:00:00,2017-06-20 10:00:00,2017-06-25 10:00:00
`)
	writeFixture(t, dir, "olist_order_items_dataset.csv", `order_id,order_item_id,product_id,seller_id,shipping_limit_date,price,freight_value
o1,1,p1,s1,2018-01-12 10:00:00,100.00,10.00
o2,1,p2,s2,2017-06-12 10:00:00,50.00,5.00
`)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_sequential,payment_type,payment_installments,payment_value
o1,1,credit_card,1,110.00
o2,1,credit_card,1,55.00
`)
	writeFixture(t, dir, "olist_products_dataset.csv", `product_id,product_category_name,product_name_lenght,product_description_lenght,product_photos_qty,product_weight_g,product_length_cm,product_height_cm,product_width_cm
p1,beleza_saude,10,20,1,500,20,10,15
p2,relogios_presentes,12,24,1,600,22,11,16
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_unique_id,customer_zip_code_prefix,customer_city,customer_state
c1,u1,01001,Sao Paulo,SP
c2,u2,20000,Rio de Janeiro,RJ
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `review_id,order_id,review_score,review_comment_title,review_comment_message,review_creation_date,review_answer_timestamp
r1,o1,5,,,2018-01-15,2018-01-15 10:00:00
r2,o2,3,,,2017-06-21,2017-06-21 10:00:00
`)
	writeFixture(t, dir, "product_category_name_translation.csv", `product_category_name,product_category_name_english
beleza_saude,health_beauty
relogios_presentes,watches_gifts
`)

	metrics, err := newLegacyRuntime(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Close()
	if _, err := os.Stat(filepath.Join(dir, "sales", "catalog.sqlite")); err != nil {
		t.Fatalf("expected DuckLake catalog: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sales", "data")); err != nil {
		t.Fatalf("expected DuckLake data directory: %v", err)
	}

	patch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "overview", dashboard.Filters{Controls: map[string]dashboard.FilterControl{
		"state":         {Type: "multi_select", Operator: "in", Values: []string{"SP"}},
		"purchase_date": {Type: "date_range", Preset: "2018"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	if patch.Status.Error != "" {
		t.Fatalf("unexpected status error: %s", patch.Status.Error)
	}
	assertVisualKeys(t, patch, overviewVisualKeys())
	if got := datumInt(patch.Visuals["total_orders"].Data[0], "value"); got != 1 {
		t.Fatalf("orders KPI value = %d, want 1", got)
	}
	if got := patch.Visuals["total_orders"].Kind; got != "kpi" {
		t.Fatalf("orders KPI kind = %q, want kpi", got)
	}
	if got := patch.Visuals["total_orders"].Type; got != "kpi" {
		t.Fatalf("orders KPI type = %q, want kpi", got)
	}
	if got := patch.Visuals["total_orders"].Title; got != "Orders" {
		t.Fatalf("orders KPI title = %q, want Orders", got)
	}
	if len(patch.Visuals["revenue_by_month"].Data) != 1 {
		t.Fatalf("revenue points = %d, want 1", len(patch.Visuals["revenue_by_month"].Data))
	}
	if got := patch.Visuals["revenue_by_month"].Type; got != "area" {
		t.Fatalf("revenue chart type = %q, want area", got)
	}
	if got := patch.Visuals["revenue_by_month"].Version; got != 3 {
		t.Fatalf("revenue chart version = %d, want 3", got)
	}
	if got := patch.Visuals["revenue_by_month"].Kind; got != "chart" {
		t.Fatalf("revenue chart kind = %q, want chart", got)
	}
	if got := patch.Visuals["revenue_by_month"].Shape; got != "category_value" {
		t.Fatalf("revenue chart shape = %q, want category_value", got)
	}
	if got := patch.Visuals["revenue_by_month"].Renderer; got != "echarts" {
		t.Fatalf("revenue chart renderer = %q, want echarts", got)
	}
	if got := patch.Visuals["revenue_by_month"].Measures[0]; got != "revenue" {
		t.Fatalf("revenue chart measure = %q, want revenue", got)
	}
	if got := datumString(patch.Visuals["category_revenue"].Data[0], "label"); got != "health_beauty" {
		t.Fatalf("top category = %q, want health_beauty", got)
	}
	if got := len(patch.FilterOptions["state"]); got != 2 {
		t.Fatalf("state filter options = %d, want 2", got)
	}

	defaultPagePatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, defaultPagePatch, overviewVisualKeys())

	unknownDefaultPagePatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "missing", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, unknownDefaultPagePatch, overviewVisualKeys())
	// These legacy showcase assertions require the full Olist fixture rather
	// than the compact service fixture used above. Keep them opt-in until they
	// are migrated to the dedicated integration harness.
	if os.Getenv("LIBREDASH_EXTENDED_FIXTURE_ASSERTIONS") == "" {
		return
	}

	selectedFilters := dashboard.Filters{
		Selections: []dashboard.InteractionSelection{
			interactionSelection("visual", "orders", "point_selection", "orders.status", "delivered"),
		},
	}
	selectedPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "overview", selectedFilters)
	if err != nil {
		t.Fatal(err)
	}
	if got := datumInt(selectedPatch.Visuals["total_orders"].Data[0], "value"); got != 2 {
		t.Fatalf("selected orders KPI value = %d, want 2", got)
	}
	if len(selectedPatch.Visuals["orders"].Data) != 2 {
		t.Fatalf("orders chart points without explicit self-target = %d, want 2", len(selectedPatch.Visuals["orders"].Data))
	}
	if !pointSelected(selectedPatch.Visuals["orders"].Data, "delivered") {
		t.Fatalf("orders chart did not mark delivered as selected: %#v", selectedPatch.Visuals["orders"].Data)
	}
	if got := selectedEntryValue(selectedPatch.Visuals["orders"].Selection, "orders.status"); got != "delivered" {
		t.Fatalf("orders chart selection entry = %q, want delivered: %#v", got, selectedPatch.Visuals["orders"].Selection)
	}
	if got := datumString(selectedPatch.Visuals["categories"].Data[0], "label"); got != "health_beauty" {
		t.Fatalf("category chart under status selection = %q, want health_beauty", got)
	}
	if got := datumString(selectedPatch.Visuals["revenue"].Data[0], "series"); got != "" {
		t.Fatalf("single-series chart row series = %q, want empty", got)
	}

	report := metrics.reports.workspace.Dashboards["executive-sales"]
	ordersVisual := report.Visuals["orders"]
	ordersVisual.Interaction.PointSelection.Targets = append(ordersVisual.Interaction.PointSelection.Targets, "orders")
	report.Visuals["orders"] = ordersVisual
	metrics.reports.workspace.Dashboards["executive-sales"] = report
	selfTargetPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "overview", selectedFilters)
	if err != nil {
		t.Fatal(err)
	}
	if len(selfTargetPatch.Visuals["orders"].Data) != 1 {
		t.Fatalf("orders chart points with explicit self-target = %d, want 1", len(selfTargetPatch.Visuals["orders"].Data))
	}
	if !pointSelected(selfTargetPatch.Visuals["orders"].Data, "delivered") {
		t.Fatalf("self-targeted orders chart did not mark delivered as selected: %#v", selfTargetPatch.Visuals["orders"].Data)
	}
	report = metrics.reports.workspace.Dashboards["executive-sales"]
	ordersVisual.Interaction.PointSelection.Targets = removeString(ordersVisual.Interaction.PointSelection.Targets, "orders")
	report.Visuals["orders"] = ordersVisual
	metrics.reports.workspace.Dashboards["executive-sales"] = report

	columnPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-column", selectedFilters)
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, columnPatch, []string{"orders_by_month_column", "orders_by_month_status", "orders_by_month_status_grouped"})
	if got := columnPatch.Visuals["orders_by_month_status"].Shape; got != "category_series_value" {
		t.Fatalf("multi-series chart shape = %q, want category_series_value", got)
	}
	if got := columnPatch.Visuals["orders_by_month_status"].Options["stacked"]; got != true {
		t.Fatalf("multi-series chart stacked option = %v, want true", got)
	}
	if got := datumString(columnPatch.Visuals["orders_by_month_status"].Data[0], "series"); got == "" {
		t.Fatal("multi-series chart row series is empty")
	}
	if len(columnPatch.Visuals["orders_by_month_status"].Data) != 2 {
		t.Fatalf("non-target multi-series chart points under status selection = %d, want 2", len(columnPatch.Visuals["orders_by_month_status"].Data))
	}

	boxplotPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-boxplot", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, boxplotPatch, []string{"delivery_distribution", "review_distribution", "revenue_distribution"})
	if len(boxplotPatch.Visuals["revenue_distribution"].Data) == 0 {
		t.Fatal("revenue distribution payload is empty")
	}

	funnelPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-funnel", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, funnelPatch, []string{"delivery_funnel", "status_funnel", "status_funnel_left"})

	piePatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-pie", dashboard.Filters{Controls: map[string]dashboard.FilterControl{
		"category": {Type: "text", Operator: "contains", Value: "health"},
		"state":    {Type: "multi_select", Operator: "in", Values: []string{"SP"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, piePatch, []string{"category_pie_inside", "status_pie", "status_pie_rose"})
	if _, ok := piePatch.Filters.Controls["category"]; ok {
		t.Fatalf("pie patch included off-page category filter: %#v", piePatch.Filters.Controls)
	}
	if _, ok := piePatch.FilterOptions["category"]; ok {
		t.Fatalf("pie patch included off-page category options: %#v", piePatch.FilterOptions)
	}
	if got := len(piePatch.FilterOptions["state"]); got != 2 {
		t.Fatalf("pie state filter options = %d, want 2", got)
	}

	emptyPagePatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, emptyPagePatch, overviewVisualKeys())

	unknownPagePatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "missing", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, unknownPagePatch, overviewVisualKeys())

	for chartType, visualKeys := range chartShowcaseMatrix() {
		pagePatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-"+chartType, dashboard.Filters{})
		if err != nil {
			t.Fatalf("query chart-%s: %v", chartType, err)
		}
		assertVisualKeys(t, pagePatch, visualKeys)
	}
	candlestickPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-candlestick", dashboard.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(candlestickPatch.Visuals["revenue_candlestick"].Data) == 0 {
		t.Fatal("revenue candlestick payload is empty")
	}

	comboPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-combo", selectedFilters)
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, comboPatch, []string{"review_delivery_combo", "revenue_orders_combo", "revenue_orders_dual_axis_combo"})
	if got := comboPatch.Visuals["revenue_orders_combo"].Shape; got != "category_multi_measure" {
		t.Fatalf("combo chart shape = %q, want category_multi_measure", got)
	}
	if !hasDatumValue(comboPatch.Visuals["revenue_orders_combo"].Data, "series", "Revenue") || !hasDatumValue(comboPatch.Visuals["revenue_orders_combo"].Data, "series", "Orders") {
		t.Fatalf("combo chart rows missing expected measure series: %#v", comboPatch.Visuals["revenue_orders_combo"].Data)
	}

	waterfallPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-waterfall", selectedFilters)
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, waterfallPatch, []string{"orders_waterfall", "revenue_waterfall", "revenue_waterfall_labeled"})
	if got := waterfallPatch.Visuals["revenue_waterfall"].Shape; got != "category_delta" {
		t.Fatalf("waterfall chart shape = %q, want category_delta", got)
	}
	if _, ok := waterfallPatch.Visuals["revenue_waterfall"].Data[0]["start"]; !ok {
		t.Fatalf("waterfall row missing start/end: %#v", waterfallPatch.Visuals["revenue_waterfall"].Data[0])
	}

	histogramPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-histogram", selectedFilters)
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, histogramPatch, []string{"delivery_histogram", "review_histogram", "revenue_histogram"})
	if got := histogramPatch.Visuals["delivery_histogram"].Shape; got != "binned_measure" {
		t.Fatalf("histogram chart shape = %q, want binned_measure", got)
	}
	if _, ok := histogramPatch.Visuals["delivery_histogram"].Data[0]["binStart"]; !ok {
		t.Fatalf("histogram row missing bin metadata: %#v", histogramPatch.Visuals["delivery_histogram"].Data[0])
	}

	mapPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-map", selectedFilters)
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, mapPatch, []string{"state_order_map", "state_revenue_map", "state_revenue_map_labeled"})
	if got := mapPatch.Visuals["state_order_map"].Shape; got != "geo" {
		t.Fatalf("map chart shape = %q, want geo", got)
	}
	if !hasDatumValue(mapPatch.Visuals["state_order_map"].Data, "name", "SP") {
		t.Fatalf("map chart rows missing SP: %#v", mapPatch.Visuals["state_order_map"].Data)
	}

	graphPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-graph", selectedFilters)
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, graphPatch, []string{"category_status_graph", "category_status_graph_circular", "status_delivery_graph"})
	if got := graphPatch.Visuals["status_delivery_graph"].Type; got != "graph" {
		t.Fatalf("graph visual type = %q, want graph", got)
	}
	if !hasDatumValue(graphPatch.Visuals["status_delivery_graph"].Data, "source", "delivered") {
		t.Fatalf("graph rows missing delivered source: %#v", graphPatch.Visuals["status_delivery_graph"].Data)
	}

	sunburstPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "chart-sunburst", selectedFilters)
	if err != nil {
		t.Fatal(err)
	}
	assertVisualKeys(t, sunburstPatch, []string{"category_state_status_sunburst", "category_status_sunburst", "state_status_sunburst"})
	if got := sunburstPatch.Visuals["category_status_sunburst"].Shape; got != "hierarchy" {
		t.Fatalf("hierarchy chart shape = %q, want hierarchy", got)
	}
	if !hasHierarchyPathValue(sunburstPatch.Visuals["category_status_sunburst"].Data, "health_beauty") {
		t.Fatalf("hierarchy rows missing health_beauty path: %#v", sunburstPatch.Visuals["category_status_sunburst"].Data)
	}

	table, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{}, dashboard.TableRequest{
		Table:      "orders_table",
		Block:      "a",
		Start:      0,
		Count:      1,
		RequestSeq: 7,
		Sort:       dashboard.TableSort{Key: "revenue", Direction: "asc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total := exactTableRows(t, table); total != 2 {
		t.Fatalf("table total rows = %d, want 2", total)
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
	if got := table.Columns[5].Format; got != "currency" {
		t.Fatalf("orders revenue format = %q, want currency", got)
	}

	conditionalTable, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{}, dashboard.TableRequest{
		Table: "orders_conditional",
		Block: "all",
		Count: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := conditionalTable.Style.Grid; got != "full" {
		t.Fatalf("conditional table grid = %q, want full", got)
	}
	if conditionalTable.RowHeight != dashboard.TableRowHeight {
		t.Fatalf("conditional table row height = %d, want %d", conditionalTable.RowHeight, dashboard.TableRowHeight)
	}
	if !tableColumnHasFormatting(conditionalTable.Columns, "status", "badge") {
		t.Fatalf("conditional table status column missing badge formatting: %#v", conditionalTable.Columns)
	}

	filteredTable, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{
		Selections: []dashboard.InteractionSelection{
			interactionSelection("visual", "orders", "point_selection", "orders.status", "delivered"),
		},
	}, dashboard.TableRequest{Table: "orders_table", Block: "all", Count: 10, RequestSeq: 8})
	if err != nil {
		t.Fatal(err)
	}
	if total := exactTableRows(t, filteredTable); total != 1 {
		t.Fatalf("targeted table total rows = %d, want 1", total)
	}
	if filteredTable.AvailableRows != 1 {
		t.Fatalf("targeted table available rows = %d, want 1", filteredTable.AvailableRows)
	}

	andFilteredTable, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{
		Selections: []dashboard.InteractionSelection{
			interactionSelection("visual", "orders", "point_selection", "orders.status", "delivered"),
			interactionSelection("visual", "categories", "point_selection", "orders.category", "watches_gifts"),
		},
	}, dashboard.TableRequest{Table: "orders_table", Block: "all", Count: 10, RequestSeq: 9})
	if err != nil {
		t.Fatal(err)
	}
	if total := exactTableRows(t, andFilteredTable); total != 0 {
		t.Fatalf("AND-filtered table total rows = %d, want 0", total)
	}
	if got := filteredTable.Blocks["a"].RequestSeq; got != 8 {
		t.Fatalf("all block request seq = %d, want 8", got)
	}

	selectedRowTable, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{
		Selections: []dashboard.InteractionSelection{
			interactionSelection("visual", "orders_table", "row_selection", "orders.order_id", "o1"),
		},
	}, dashboard.TableRequest{Table: "orders_table", Block: "all", Count: 10, RequestSeq: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := selectedEntryValue(selectedRowTable.Selection, "orders.order_id"); got != "o1" {
		t.Fatalf("table selection entry = %q, want o1: %#v", got, selectedRowTable.Selection)
	}
	if selected, unfiltered := exactTableRows(t, selectedRowTable), exactTableRows(t, table); selected != unfiltered {
		t.Fatalf("selected source table total rows = %d, want self selection not applied to source table total %d", selected, unfiltered)
	}

	uiOnlyRowSelection := dashboard.Filters{
		Selections: []dashboard.InteractionSelection{
			interactionSelection("visual", "orders_table", "row_selection", dashboard.UIRowSelectionField, "o1"),
		},
	}
	uiOnlyRowTable, err := metrics.QueryTable(context.Background(), "executive-sales", uiOnlyRowSelection, dashboard.TableRequest{Table: "orders_table", Block: "all", Count: 10, RequestSeq: 11})
	if err != nil {
		t.Fatal(err)
	}
	if got := selectedEntryValue(uiOnlyRowTable.Selection, dashboard.UIRowSelectionField); got != "o1" {
		t.Fatalf("UI-only table selection entry = %q, want o1: %#v", got, uiOnlyRowTable.Selection)
	}
	if selected, unfiltered := exactTableRows(t, uiOnlyRowTable), exactTableRows(t, table); selected != unfiltered {
		t.Fatalf("UI-only selected source table total rows = %d, want unfiltered total %d", selected, unfiltered)
	}
	uiOnlyRowPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "overview", uiOnlyRowSelection)
	if err != nil {
		t.Fatal(err)
	}
	if got := datumInt(uiOnlyRowPatch.Visuals["total_orders"].Data[0], "value"); got != 2 {
		t.Fatalf("UI-only row selection KPI value = %d, want unfiltered value 2", got)
	}

	multiRowSelection := dashboard.Filters{
		Selections: []dashboard.InteractionSelection{
			interactionSelection("visual", "orders_table", "row_selection", "orders.order_id", "o1", "o2"),
		},
	}
	multiRowPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "overview", multiRowSelection)
	if err != nil {
		t.Fatal(err)
	}
	if got := datumInt(multiRowPatch.Visuals["total_orders"].Data[0], "value"); got != 2 {
		t.Fatalf("multi-row selected orders KPI value = %d, want 2", got)
	}
	if len(multiRowPatch.Visuals["orders"].Data) != 2 {
		t.Fatalf("orders chart points under multi-row table selection = %d, want 2", len(multiRowPatch.Visuals["orders"].Data))
	}
	andMultiRowPatch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "overview", dashboard.Filters{
		Controls: map[string]dashboard.FilterControl{
			"state": {Type: "multi_select", Operator: "in", Values: []string{"SP"}},
		},
		Selections: multiRowSelection.Selections,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := datumInt(andMultiRowPatch.Visuals["total_orders"].Data[0], "value"); got != 1 {
		t.Fatalf("page filter AND multi-row selected orders KPI value = %d, want 1", got)
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
	if !tableRowsHaveValue(matrixTable.Blocks["a"].Rows, "pivot_delivered__order_count") {
		t.Fatalf("matrix rows missing delivered order count values: %#v", matrixTable.Blocks["a"].Rows)
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
	if !tableRowsHaveValue(pivotTable.Blocks["a"].Rows, "pivot_delivered") {
		t.Fatalf("pivot rows missing delivered values: %#v", pivotTable.Blocks["a"].Rows)
	}

	formattedMatrix, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{}, dashboard.TableRequest{
		Table: "state_status_matrix_formatted",
		Block: "all",
		Count: 10,
		Sort:  dashboard.TableSort{Key: "state", Direction: "asc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if formattedMatrix.RowHeight != dashboard.TableRowHeight {
		t.Fatalf("formatted matrix row height = %d, want %d", formattedMatrix.RowHeight, dashboard.TableRowHeight)
	}
	if !tableColumnHasFormatting(formattedMatrix.Columns, "revenue", "data_bar") {
		t.Fatalf("formatted matrix revenue column missing data bar formatting: %#v", formattedMatrix.Columns)
	}

	heatPivot, err := metrics.QueryTable(context.Background(), "executive-sales", dashboard.Filters{}, dashboard.TableRequest{
		Table: "category_status_pivot_heat",
		Block: "all",
		Count: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if heatPivot.RowHeight != 28 {
		t.Fatalf("heat pivot row height = %d, want 28", heatPivot.RowHeight)
	}
	if !tableHasAnyFormatting(heatPivot.Columns, "background_scale") {
		t.Fatalf("heat pivot generated columns missing background scale formatting: %#v", heatPivot.Columns)
	}
	if !tableRowsHaveValue(heatPivot.Blocks["a"].Rows, "pivot_delivered") {
		t.Fatalf("heat pivot rows missing delivered values: %#v", heatPivot.Blocks["a"].Rows)
	}

}

func TestServiceInteractionSelectionPreservesCompositeTuples(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_approved_at,order_delivered_carrier_date,order_delivered_customer_date,order_estimated_delivery_date
o1,c1,delivered,2018-01-10 10:00:00,2018-01-10 11:00:00,2018-01-11 10:00:00,2018-01-14 10:00:00,2018-01-20 10:00:00
o2,c2,delivered,2018-01-11 10:00:00,2018-01-11 11:00:00,2018-01-12 10:00:00,2018-01-15 10:00:00,2018-01-20 10:00:00
o3,c3,shipped,2018-01-12 10:00:00,2018-01-12 11:00:00,2018-01-13 10:00:00,2018-01-16 10:00:00,2018-01-20 10:00:00
o4,c4,shipped,2018-01-13 10:00:00,2018-01-13 11:00:00,2018-01-14 10:00:00,2018-01-17 10:00:00,2018-01-20 10:00:00
`)
	writeFixture(t, dir, "olist_order_items_dataset.csv", `order_id,order_item_id,product_id,seller_id,shipping_limit_date,price,freight_value
o1,1,p1,s1,2018-01-12 10:00:00,100.00,10.00
o2,1,p2,s2,2018-01-13 10:00:00,150.00,15.00
o3,1,p1,s1,2018-01-14 10:00:00,50.00,5.00
o4,1,p2,s2,2018-01-15 10:00:00,200.00,20.00
`)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_sequential,payment_type,payment_installments,payment_value
o1,1,credit_card,1,110.00
o2,1,credit_card,1,165.00
o3,1,credit_card,1,55.00
o4,1,credit_card,1,220.00
`)
	writeFixture(t, dir, "olist_products_dataset.csv", `product_id,product_category_name,product_name_lenght,product_description_lenght,product_photos_qty,product_weight_g,product_length_cm,product_height_cm,product_width_cm
p1,beleza_saude,10,20,1,500,20,10,15
p2,relogios_presentes,12,24,1,600,22,11,16
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_unique_id,customer_zip_code_prefix,customer_city,customer_state
c1,u1,01001,Sao Paulo,SP
c2,u2,01002,Sao Paulo,SP
c3,u3,20000,Rio de Janeiro,RJ
c4,u4,20001,Rio de Janeiro,RJ
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `review_id,order_id,review_score,review_comment_title,review_comment_message,review_creation_date,review_answer_timestamp
r1,o1,5,,,2018-01-15,2018-01-15 10:00:00
r2,o2,4,,,2018-01-16,2018-01-16 10:00:00
r3,o3,3,,,2018-01-17,2018-01-17 10:00:00
r4,o4,4,,,2018-01-18,2018-01-18 10:00:00
`)
	writeFixture(t, dir, "product_category_name_translation.csv", `product_category_name,product_category_name_english
beleza_saude,health_beauty
relogios_presentes,watches_gifts
`)

	metrics, err := newLegacyRuntime(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Close()

	filters := dashboard.Filters{
		Selections: []dashboard.InteractionSelection{
			compositeInteractionSelection("visual", "orders_table", "row_selection",
				map[string]string{"orders.order_id": "o1", "orders.status": "delivered", "orders.category": "health_beauty"},
				map[string]string{"orders.order_id": "o4", "orders.status": "shipped", "orders.category": "watches_gifts"},
			),
		},
	}
	patch, err := metrics.QueryDashboardPage(context.Background(), "executive-sales", "overview", filters)
	if err != nil {
		t.Fatal(err)
	}
	if patch.Status.Error != "" {
		t.Fatalf("unexpected status error: %s", patch.Status.Error)
	}
	if got := categoryRevenue(patch.Visuals["category_revenue"].Data, "health_beauty"); got != 110 {
		t.Fatalf("health_beauty revenue = %v, want 110", got)
	}
	if got := categoryRevenue(patch.Visuals["category_revenue"].Data, "watches_gifts"); got != 220 {
		t.Fatalf("watches_gifts revenue = %v, want 220", got)
	}
	if got := categoryRevenueTotal(patch.Visuals["category_revenue"].Data); got != 330 {
		t.Fatalf("category revenue total = %v, want 330 without cross-matched tuples", got)
	}

	malformedFilters := dashboard.Filters{
		Selections: []dashboard.InteractionSelection{
			compositeInteractionSelection("visual", "orders_table", "row_selection",
				map[string]string{"orders.order_id": "o1", "orders.status": "delivered", "orders.unknown": "health_beauty"},
				map[string]string{"orders.order_id": "o4", "orders.status": "shipped", "orders.category": "watches_gifts"},
			),
		},
	}
	patch, err = metrics.QueryDashboardPage(context.Background(), "executive-sales", "overview", malformedFilters)
	if err != nil {
		t.Fatal(err)
	}
	if patch.Status.Error == "" {
		t.Fatalf("malformed tuple selection was not rejected: %#v", patch)
	}
	if len(patch.Visuals) != 0 {
		t.Fatalf("malformed tuple returned visual data: %#v", patch.Visuals)
	}
}

func TestServicePowerFilters(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "olist_orders_dataset.csv", `order_id,customer_id,order_status,order_purchase_timestamp,order_approved_at,order_delivered_carrier_date,order_delivered_customer_date,order_estimated_delivery_date
o1,c1,delivered,2018-01-10 10:00:00,2018-01-10 11:00:00,2018-01-11 10:00:00,2018-01-14 10:00:00,2018-01-20 10:00:00
o2,c2,shipped,2017-06-10 10:00:00,2017-06-10 11:00:00,2017-06-11 10:00:00,2017-06-20 10:00:00,2017-06-25 10:00:00
`)
	writeFixture(t, dir, "olist_order_items_dataset.csv", `order_id,order_item_id,product_id,seller_id,shipping_limit_date,price,freight_value
o1,1,p1,s1,2018-01-12 10:00:00,100.00,10.00
o2,1,p2,s2,2017-06-12 10:00:00,50.00,5.00
`)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_sequential,payment_type,payment_installments,payment_value
o1,1,credit_card,1,110.00
o2,1,credit_card,1,55.00
`)
	writeFixture(t, dir, "olist_products_dataset.csv", `product_id,product_category_name,product_name_lenght,product_description_lenght,product_photos_qty,product_weight_g,product_length_cm,product_height_cm,product_width_cm
p1,beleza_saude,10,20,1,500,20,10,15
p2,relogios_presentes,12,24,1,600,22,11,16
`)
	writeFixture(t, dir, "olist_customers_dataset.csv", `customer_id,customer_unique_id,customer_zip_code_prefix,customer_city,customer_state
c1,u1,01001,Sao Paulo,SP
c2,u2,20000,Rio de Janeiro,RJ
`)
	writeFixture(t, dir, "olist_order_reviews_dataset.csv", `review_id,order_id,review_score,review_comment_title,review_comment_message,review_creation_date,review_answer_timestamp
r1,o1,5,,,2018-01-15,2018-01-15 10:00:00
r2,o2,3,,,2017-06-21,2017-06-21 10:00:00
`)
	writeFixture(t, dir, "product_category_name_translation.csv", `product_category_name,product_category_name_english
beleza_saude,health_beauty
relogios_presentes,watches_gifts
`)

	metrics, err := newLegacyRuntime(t, dir)
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
			if got := fmt.Sprint(datumInt(patch.Visuals["total_orders"].Data[0], "value")); got != tt.want {
				t.Fatalf("orders KPI value = %q, want %s", got, tt.want)
			}
		})
	}
}

func interactionSelection(sourceKind, sourceID, interactionKind, field string, values ...string) dashboard.InteractionSelection {
	fact := ""
	if parts := strings.SplitN(field, ".", 2); len(parts) == 2 && field != dashboard.UIRowSelectionField {
		fact = parts[0]
	}
	entries := make([]dashboard.InteractionSelectionEntry, 0, len(values))
	for _, value := range values {
		entries = append(entries, dashboard.InteractionSelectionEntry{
			Mappings: []dashboard.InteractionSelectionMapping{{
				Field: field,
				Fact:  fact,
				Value: value,
				Label: value,
			}},
			Label: value,
		})
	}
	return dashboard.InteractionSelection{
		ID:              sourceKind + ":" + sourceID + ":" + interactionKind,
		SourceKind:      sourceKind,
		SourceID:        sourceID,
		InteractionKind: interactionKind,
		Entries:         entries,
		Label:           strings.Join(values, ", "),
	}
}

func compositeInteractionSelection(sourceKind, sourceID, interactionKind string, tuples ...map[string]string) dashboard.InteractionSelection {
	entries := make([]dashboard.InteractionSelectionEntry, 0, len(tuples))
	for _, tuple := range tuples {
		fields := make([]string, 0, len(tuple))
		for field := range tuple {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		entry := dashboard.InteractionSelectionEntry{}
		labels := make([]string, 0, len(fields))
		for _, field := range fields {
			value := tuple[field]
			fact := ""
			if parts := strings.SplitN(field, ".", 2); len(parts) == 2 && field != dashboard.UIRowSelectionField {
				fact = parts[0]
			}
			entry.Mappings = append(entry.Mappings, dashboard.InteractionSelectionMapping{
				Field: field,
				Fact:  fact,
				Value: value,
				Label: value,
			})
			labels = append(labels, value)
		}
		entry.Label = strings.Join(labels, ", ")
		entries = append(entries, entry)
	}
	return dashboard.InteractionSelection{
		ID:              sourceKind + ":" + sourceID + ":" + interactionKind,
		SourceKind:      sourceKind,
		SourceID:        sourceID,
		InteractionKind: interactionKind,
		Entries:         entries,
	}
}

func exactTableRows(t *testing.T, table dashboard.Table) int {
	t.Helper()
	value, exact := table.Cardinality.ExactValue()
	if !exact {
		t.Fatalf("table cardinality = %#v, want exact", table.Cardinality)
	}
	return value
}

func categoryRevenue(data []dashboard.Datum, label string) float64 {
	for _, row := range data {
		if datumString(row, "label") == label {
			return datumFloat(row["value"])
		}
	}
	return 0
}

func categoryRevenueTotal(data []dashboard.Datum) float64 {
	var total float64
	for _, row := range data {
		total += datumFloat(row["value"])
	}
	return total
}

func selectedEntryValue(entries []dashboard.InteractionSelectionEntry, field string) string {
	for _, entry := range entries {
		for _, mapping := range entry.Mappings {
			if mapping.Field == field {
				return fmt.Sprint(mapping.Value)
			}
		}
	}
	return ""
}

func removeString(values []string, value string) []string {
	next := make([]string, 0, len(values))
	for _, candidate := range values {
		if candidate != value {
			next = append(next, candidate)
		}
	}
	return next
}
