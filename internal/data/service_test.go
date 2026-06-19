package data

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

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
)

func TestCompileSourceRelation(t *testing.T) {
	cases := map[string]struct {
		plan sourcePlan
		want string
	}{
		"csv": {
			plan: sourcePlan{kind: "path", format: "csv", path: "/data/orders.csv", options: map[string]any{"header": true, "sample_size": 1000}},
			want: "SELECT * FROM read_csv('/data/orders.csv', header = true, sample_size = 1000)",
		},
		"json": {
			plan: sourcePlan{kind: "path", format: "json", path: "/data/orders.json", options: map[string]any{"format": "array"}},
			want: "SELECT * FROM read_json('/data/orders.json', format = 'array')",
		},
		"parquet": {
			plan: sourcePlan{kind: "path", format: "parquet", path: "s3://bucket/orders/*.parquet", options: map[string]any{"union_by_name": true}},
			want: "SELECT * FROM read_parquet('s3://bucket/orders/*.parquet', union_by_name = true)",
		},
		"excel": {
			plan: sourcePlan{kind: "path", format: "excel", path: "/data/budget.xlsx", options: map[string]any{"sheet": "FY2026"}},
			want: "SELECT * FROM read_xlsx('/data/budget.xlsx', sheet = 'FY2026')",
		},
		"text": {
			plan: sourcePlan{kind: "path", format: "text", path: "/data/readme.txt"},
			want: "SELECT * FROM read_text('/data/readme.txt')",
		},
		"blob": {
			plan: sourcePlan{kind: "path", format: "blob", path: "/data/archive.blob"},
			want: "SELECT * FROM read_blob('/data/archive.blob')",
		},
		"vortex": {
			plan: sourcePlan{kind: "path", format: "vortex", path: "/data/orders.vortex"},
			want: "SELECT * FROM read_vortex('/data/orders.vortex')",
		},
		"lance": {
			plan: sourcePlan{kind: "path", format: "lance", path: "s3://bucket/vectors/products.lance"},
			want: "SELECT * FROM 's3://bucket/vectors/products.lance'",
		},
		"delta": {
			plan: sourcePlan{kind: "path", format: "delta", path: "az://warehouse/orders"},
			want: "SELECT * FROM delta_scan('az://warehouse/orders')",
		},
		"iceberg": {
			plan: sourcePlan{kind: "path", format: "iceberg", path: "s3://warehouse/orders/metadata/v1.metadata.json", options: map[string]any{"allow_moved_paths": true}},
			want: "SELECT * FROM iceberg_scan('s3://warehouse/orders/metadata/v1.metadata.json', allow_moved_paths = true)",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			relation, err := compileSourceRelation(tc.plan)
			if err != nil {
				t.Fatal(err)
			}
			if relation != tc.want {
				t.Fatalf("relation = %q, want %q", relation, tc.want)
			}
		})
	}

	relation, err := compileSourceRelation(sourcePlan{kind: "object", connection: "crm", object: "public.accounts"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM conn_crm.public.accounts"; relation != want {
		t.Fatalf("object relation = %q, want %q", relation, want)
	}

	_, err = compileSourceRelation(sourcePlan{kind: "path", format: "csv", path: "/data/orders.csv", options: map[string]any{"bad-key": true}})
	if err == nil || !strings.Contains(err.Error(), "invalid source option") {
		t.Fatalf("invalid option error = %v, want invalid source option", err)
	}

	_, err = compileSourceRelation(sourcePlan{kind: "path", format: "lance", path: "/data/products.lance", options: map[string]any{"sample_size": 1000}})
	if err == nil || !strings.Contains(err.Error(), "lance source cannot set options") {
		t.Fatalf("lance option error = %v, want lance option rejection", err)
	}
}

func TestCompileConnectionSecret(t *testing.T) {
	stmt, ok, err := compileConnectionSecret("prod_lake", semantic.Connection{
		Kind:  "s3",
		Scope: "s3://analytics-prod/",
		Auth: semantic.ConnectionAuth{
			"access_key_id":     "key",
			"secret_access_key": "secret",
			"region":            "us-east-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("secret ok = false, want true")
	}
	want := "CREATE OR REPLACE SECRET libredash_prod_lake (TYPE s3, PROVIDER config, KEY_ID 'key', REGION 'us-east-1', SECRET 'secret', SCOPE 's3://analytics-prod/')"
	if stmt != want {
		t.Fatalf("s3 secret = %q, want %q", stmt, want)
	}

	stmt, ok, err = compileConnectionSecret("azure_lake", semantic.Connection{
		Kind: "azure_blob",
		Auth: semantic.ConnectionAuth{"connection_string": "DefaultEndpointsProtocol=https;AccountName=mystorageaccount"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "CREATE OR REPLACE SECRET libredash_azure_lake (TYPE azure, PROVIDER config, CONNECTION_STRING 'DefaultEndpointsProtocol=https;AccountName=mystorageaccount')"
	if !ok || stmt != want {
		t.Fatalf("azure secret = %q ok=%v, want %q ok=true", stmt, ok, want)
	}

	stmt, ok, err = compileConnectionSecret("azure_lake", semantic.Connection{
		Kind: "azure_blob",
		Auth: semantic.ConnectionAuth{
			"account_name":  "mystorageaccount",
			"tenant_id":     "tenant",
			"client_id":     "client",
			"client_secret": "secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "CREATE OR REPLACE SECRET libredash_azure_lake (TYPE azure, PROVIDER service_principal, ACCOUNT_NAME 'mystorageaccount', CLIENT_ID 'client', CLIENT_SECRET 'secret', TENANT_ID 'tenant')"
	if !ok || stmt != want {
		t.Fatalf("azure service principal secret = %q ok=%v, want %q ok=true", stmt, ok, want)
	}

	stmt, ok, err = compileConnectionSecret("crm", semantic.Connection{
		Kind: "postgres",
		Auth: semantic.ConnectionAuth{"connection_string": "postgres://crm"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok || stmt != "" {
		t.Fatalf("postgres secret = %q ok=%v, want empty ok=false", stmt, ok)
	}

	stmt, ok, err = compileConnectionSecret("lakehouse", semantic.Connection{
		Kind:  "ducklake",
		Path:  "metadata.ducklake",
		Scope: "s3://analytics-prod/ducklake/",
		Auth: semantic.ConnectionAuth{
			"access_key_id":     "key",
			"secret_access_key": "secret",
			"region":            "us-east-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "CREATE OR REPLACE SECRET libredash_lakehouse (TYPE ducklake, PROVIDER config, KEY_ID 'key', REGION 'us-east-1', SECRET 'secret', SCOPE 's3://analytics-prod/ducklake/')"
	if !ok || stmt != want {
		t.Fatalf("ducklake secret = %q ok=%v, want %q ok=true", stmt, ok, want)
	}
}

func TestCompileSourceSecretStatements(t *testing.T) {
	model := &semantic.Model{
		Connections: map[string]semantic.Connection{
			"prod_lake": {
				Kind:  "s3",
				Scope: "s3://analytics-prod/",
				Auth: semantic.ConnectionAuth{
					"access_key_id":     "key",
					"secret_access_key": "secret",
				},
			},
			"public": {
				Kind: "http",
			},
		},
		Sources: map[string]semantic.Source{
			"embeddings": {Connection: "prod_lake", Path: "vectors/products.lance", Format: "lance"},
			"orders":     {Connection: "prod_lake", Path: "orders.parquet", Format: "parquet"},
			"public":     {Connection: "public", Path: "https://example.com/products.lance", Format: "lance"},
		},
	}
	statements, err := compileSourceSecretStatements(model)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"CREATE OR REPLACE SECRET libredash_prod_lake_lance (TYPE lance, PROVIDER config, KEY_ID 'key', SECRET 'secret', SCOPE 's3://analytics-prod/')"}
	if fmt.Sprint(statements) != fmt.Sprint(want) {
		t.Fatalf("lance secrets = %#v, want %#v", statements, want)
	}
}

func TestCompileDatabaseAttach(t *testing.T) {
	cases := map[string]struct {
		connection semantic.Connection
		want       string
	}{
		"postgres_auth": {
			connection: semantic.Connection{Kind: "postgres", Auth: semantic.ConnectionAuth{"connection_string": "postgres://crm"}},
			want:       "ATTACH 'postgres://crm' AS conn_crm (TYPE postgres, READ_ONLY)",
		},
		"mysql_auth": {
			connection: semantic.Connection{Kind: "mysql", Auth: semantic.ConnectionAuth{"connection_string": "mysql://sales"}},
			want:       "ATTACH 'mysql://sales' AS conn_crm (TYPE mysql, READ_ONLY)",
		},
		"sqlite_option_path": {
			connection: semantic.Connection{Kind: "sqlite", Options: map[string]any{"path": "/tmp/source.sqlite"}},
			want:       "ATTACH '/tmp/source.sqlite' AS conn_crm (TYPE sqlite, READ_ONLY)",
		},
		"sqlite_auth_path": {
			connection: semantic.Connection{Kind: "sqlite", Auth: semantic.ConnectionAuth{"path": "/tmp/source.sqlite"}},
			want:       "ATTACH '/tmp/source.sqlite' AS conn_crm (TYPE sqlite, READ_ONLY)",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			stmt, err := compileDatabaseAttach("crm", tc.connection)
			if err != nil {
				t.Fatal(err)
			}
			if stmt != tc.want {
				t.Fatalf("attach = %q, want %q", stmt, tc.want)
			}
		})
	}
}

func TestCompileDuckLakeAttach(t *testing.T) {
	dir := t.TempDir()
	metrics := &DuckDBMetrics{dataDir: dir}
	stmt, err := metrics.compileObjectAttach(&semantic.Model{}, "lakehouse", semantic.Connection{
		Kind: "ducklake",
		Path: "metadata.ducklake",
		Options: map[string]any{
			"data_path": "data_files",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "ATTACH 'ducklake:" + sqlString(filepath.Join(dir, "metadata.ducklake")) + "' AS conn_lakehouse (DATA_PATH '" + sqlString(filepath.Join(dir, "data_files")) + "')"
	if stmt != want {
		t.Fatalf("local ducklake attach = %q, want %q", stmt, want)
	}

	stmt, err = metrics.compileObjectAttach(&semantic.Model{}, "lakehouse", semantic.Connection{
		Kind:  "ducklake",
		Scope: "s3://analytics-prod/ducklake/",
		Path:  "metadata.ducklake",
		Options: map[string]any{
			"data_path": "data",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "ATTACH 'ducklake:s3://analytics-prod/ducklake/metadata.ducklake' AS conn_lakehouse (DATA_PATH 's3://analytics-prod/ducklake/data')"
	if stmt != want {
		t.Fatalf("remote ducklake attach = %q, want %q", stmt, want)
	}
}

func TestRequiredExtensions(t *testing.T) {
	model := &semantic.Model{
		Connections: map[string]semantic.Connection{
			"lake":  {Kind: "s3"},
			"azure": {Kind: "azure_blob"},
			"crm":   {Kind: "postgres"},
			"duck":  {Kind: "ducklake", Path: "metadata.ducklake"},
		},
		Sources: map[string]semantic.Source{
			"events":   {Format: "parquet", Path: "s3://bucket/events/*.parquet", Connection: "lake"},
			"budget":   {Format: "excel", Path: "budget.xlsx", Connection: "lake"},
			"orders":   {Format: "delta", Path: "az://warehouse/orders", Connection: "azure"},
			"archive":  {Format: "vortex", Path: "orders.vortex", Connection: "lake"},
			"vectors":  {Format: "lance", Path: "vectors/products.lance", Connection: "lake"},
			"accounts": {Connection: "crm", Object: "public.accounts"},
			"lake_tbl": {Connection: "duck", Object: "main.orders"},
		},
	}
	if got := strings.Join(requiredExtensions(model), ","); got != "azure,delta,ducklake,excel,httpfs,lance,postgres,vortex" {
		t.Fatalf("required extensions = %q, want azure,delta,ducklake,excel,httpfs,lance,postgres,vortex", got)
	}
}

func TestDuckDBMetricsResolvesSourcePlans(t *testing.T) {
	dir := t.TempDir()
	model := &semantic.Model{
		Name:              "test",
		DefaultConnection: "local_files",
		Connections: map[string]semantic.Connection{
			"local_files": {
				Kind: "local",
				Root: "fixtures",
				Defaults: semantic.ConnectionDefaults{
					Options: map[string]any{"header": true},
				},
			},
			"prod_lake": {Kind: "s3", Scope: "s3://analytics-prod/", Auth: semantic.ConnectionAuth{"access_key_id": "key", "secret_access_key": "secret"}},
			"azure":     {Kind: "azure_blob", Scope: "az://warehouse/", Auth: semantic.ConnectionAuth{"connection_string": "DefaultEndpointsProtocol=https;AccountName=warehouse"}},
			"vectors":   {Kind: "s3", Scope: "s3://analytics-prod/", Auth: semantic.ConnectionAuth{"access_key_id": "key", "secret_access_key": "secret"}},
		},
		Sources: map[string]semantic.Source{
			"orders":     {Path: "orders.csv"},
			"events":     {Connection: "prod_lake", Path: "events/*", Format: "parquet"},
			"delta":      {Connection: "azure", Path: "tables/orders", Format: "delta"},
			"embeddings": {Connection: "vectors", Path: "vectors/products.lance"},
		},
		Tables: map[string]semantic.ModelTable{
			"orders": {
				Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
				Dimensions: map[string]semantic.MetricDimension{"order_id": {Expr: "order_id"}},
				Measures:   map[string]semantic.MetricMeasure{"orders": {Label: "Orders", Expression: "COUNT(*)"}},
			},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	metrics := &DuckDBMetrics{dataDir: dir}

	relation, err := metrics.sourceRelation(model, model.Sources["orders"])
	if err != nil {
		t.Fatal(err)
	}
	wantLocal := "SELECT * FROM read_csv('" + sqlString(filepath.Join(dir, "fixtures", "orders.csv")) + "', header = true)"
	if relation != wantLocal {
		t.Fatalf("local relation = %q, want %q", relation, wantLocal)
	}

	relation, err = metrics.sourceRelation(model, model.Sources["events"])
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM read_parquet('s3://analytics-prod/events/*')"; relation != want {
		t.Fatalf("remote relation = %q, want %q", relation, want)
	}

	relation, err = metrics.sourceRelation(model, model.Sources["delta"])
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM delta_scan('az://warehouse/tables/orders')"; relation != want {
		t.Fatalf("delta relation = %q, want %q", relation, want)
	}

	relation, err = metrics.sourceRelation(model, model.Sources["embeddings"])
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM 's3://analytics-prod/vectors/products.lance'"; relation != want {
		t.Fatalf("lance relation = %q, want %q", relation, want)
	}

	bad := model.Sources["events"]
	bad.Path = "s3://other-bucket/events/*"
	_, err = metrics.sourceRelation(model, bad)
	if err == nil || !strings.Contains(err.Error(), "outside connection") {
		t.Fatalf("mismatched remote path error = %v, want outside connection", err)
	}
}

func TestDuckDBMetricsRegistersCSVSources(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "orders.csv", "order_id,revenue\no1,10.50\no2,20.25\n")
	db, err := sql.Open("duckdb", filepath.Join(dir, "test.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	metrics := &DuckDBMetrics{dataDir: dir}
	runtime := &modelRuntime{
		db: db,
		model: &semantic.Model{
			Name:              "test",
			DefaultConnection: "local_files",
			Connections: map[string]semantic.Connection{
				"local_files": {
					Kind:     "local",
					Defaults: semantic.ConnectionDefaults{Options: map[string]any{"header": true}},
				},
			},
			Sources: map[string]semantic.Source{
				"orders": {
					Path:       "orders.csv",
					Connection: "local_files",
				},
			},
			Tables: map[string]semantic.ModelTable{
				"orders": {
					Kind: "fact",
					Transform: semantic.ModelTransform{SQL: `
						SELECT order_id, try_cast(revenue AS DOUBLE) AS revenue
						FROM raw.orders
					`},
					PrimaryKey: "order_id",
					Grain:      "order_id",
					Dimensions: map[string]semantic.MetricDimension{"order_id": {Expr: "order_id"}},
					Measures:   map[string]semantic.MetricMeasure{"revenue": {Label: "Revenue", Expression: "SUM(orders.revenue)"}},
				},
			},
		},
	}
	if err := runtime.model.Validate(); err != nil {
		t.Fatalf("validate model: %v", err)
	}
	if err := metrics.registerSourceViews(context.Background(), runtime); err != nil {
		t.Fatalf("register sources: %v", err)
	}
	if err := metrics.materializeModelTables(context.Background(), runtime); err != nil {
		t.Fatalf("materialize model tables: %v", err)
	}

	var total float64
	if err := db.QueryRowContext(context.Background(), "SELECT SUM(revenue) FROM model.orders").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 30.75 {
		t.Fatalf("total revenue = %v, want 30.75", total)
	}
}

func TestDuckDBMetricsRegistersDatabaseSourceTwice(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	db, err := sql.Open("duckdb", filepath.Join(dir, "test.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(context.Background(), "INSTALL sqlite"); err != nil {
		t.Skipf("sqlite extension unavailable: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), "LOAD sqlite"); err != nil {
		t.Skipf("sqlite extension unavailable: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), "ATTACH '"+sqlString(sourcePath)+"' AS seed (TYPE sqlite)"); err != nil {
		t.Fatalf("attach seed sqlite: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), "CREATE TABLE seed.accounts (id INTEGER, name VARCHAR)"); err != nil {
		t.Fatalf("create seed table: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), "INSERT INTO seed.accounts VALUES (1, 'Acme')"); err != nil {
		t.Fatalf("insert seed table: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), "DETACH seed"); err != nil {
		t.Fatalf("detach seed sqlite: %v", err)
	}

	metrics := &DuckDBMetrics{dataDir: dir}
	runtime := &modelRuntime{
		db: db,
		model: &semantic.Model{
			Name: "test",
			Connections: map[string]semantic.Connection{
				"crm": {Kind: "sqlite", Options: map[string]any{"path": sourcePath}},
			},
			Sources: map[string]semantic.Source{
				"accounts": {Connection: "crm", Object: "accounts"},
			},
			Tables: map[string]semantic.ModelTable{
				"accounts": {
					Kind: "dimension", Source: "accounts", PrimaryKey: "id", Grain: "id",
					Dimensions: map[string]semantic.MetricDimension{"id": {Expr: "id"}, "name": {Expr: "name"}},
				},
			},
		},
	}
	for i := 0; i < 2; i++ {
		if err := metrics.registerSourceViews(context.Background(), runtime); err != nil {
			t.Fatalf("register sources pass %d: %v", i+1, err)
		}
	}
	if err := metrics.materializeModelTables(context.Background(), runtime); err != nil {
		t.Fatalf("materialize model tables: %v", err)
	}
	var name string
	if err := db.QueryRowContext(context.Background(), "SELECT name FROM model.accounts WHERE id = 1").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Acme" {
		t.Fatalf("name = %q, want Acme", name)
	}
}

func TestDuckDBMetricsValidateFilesIgnoresRemoteSources(t *testing.T) {
	metrics := &DuckDBMetrics{dataDir: t.TempDir()}
	runtime := &modelRuntime{model: &semantic.Model{
		Connections: map[string]semantic.Connection{
			"lake": {Kind: "s3"},
		},
		Sources: map[string]semantic.Source{
			"events": {Format: "parquet", Path: "s3://bucket/events/*.parquet", Connection: "lake"},
		},
	}}
	if err := metrics.validateFiles(runtime); err != nil {
		t.Fatalf("validate files = %v, want nil", err)
	}
}

func TestDuckDBMetricsValidateFilesUsesLocalConnectionRoot(t *testing.T) {
	dir := t.TempDir()
	model := &semantic.Model{
		Connections: map[string]semantic.Connection{
			"local_files": {Kind: "local", Root: "fixtures"},
		},
		Sources: map[string]semantic.Source{
			"orders": {Format: "csv", Path: "orders.csv", Connection: "local_files"},
		},
	}
	metrics := &DuckDBMetrics{dataDir: dir}
	err := metrics.validateFiles(&modelRuntime{model: model})
	var missing *MissingDataError
	if !errors.As(err, &missing) {
		t.Fatalf("validate files error = %v, want MissingDataError", err)
	}
	want := filepath.Join(dir, "fixtures", "orders.csv")
	if len(missing.Missing) != 1 || missing.Missing[0] != want {
		t.Fatalf("missing files = %#v, want %q", missing.Missing, want)
	}
}

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
	if got := views[0].DimensionCount; got != 9 {
		t.Fatalf("metric view dimension count = %d, want 9", got)
	}
	if got := views[0].MeasureCount; got != 13 {
		t.Fatalf("metric view measure count = %d, want 13", got)
	}
	if got := views[0].DashboardCount; got != 1 {
		t.Fatalf("metric view dashboard count = %d, want 1", got)
	}

	view, ok := metrics.MetricView("orders")
	if !ok {
		t.Fatal("metric view orders not found")
	}
	if got := view.BaseTable; got != "orders" {
		t.Fatalf("metric view base table = %q, want orders", got)
	}
	if got := view.Timeseries; got != "orders.purchase_timestamp" {
		t.Fatalf("metric view timeseries = %q, want orders.purchase_timestamp", got)
	}
	if !hasMetricDimension(view.Dimensions, "orders.category", "category") {
		t.Fatalf("metric view dimensions missing category: %#v", view.Dimensions)
	}
	if !hasMetricMeasure(view.Measures, "orders.revenue", "SUM(orders.revenue)") {
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
	if !hasModelEdge(graph.Edges, "model_table:orders", "metrics_view:orders") {
		t.Fatalf("model graph missing model table to metrics view edge: %#v", graph.Edges)
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
	if len(patch.Visuals["revenue"].Data) != 1 {
		t.Fatalf("revenue points = %d, want 1", len(patch.Visuals["revenue"].Data))
	}
	if got := patch.Visuals["revenue"].Type; got != "area" {
		t.Fatalf("revenue chart type = %q, want area", got)
	}
	if got := patch.Visuals["revenue"].Version; got != 3 {
		t.Fatalf("revenue chart version = %d, want 3", got)
	}
	if got := patch.Visuals["revenue"].Kind; got != "chart" {
		t.Fatalf("revenue chart kind = %q, want chart", got)
	}
	if got := patch.Visuals["revenue"].Shape; got != "category_value" {
		t.Fatalf("revenue chart shape = %q, want category_value", got)
	}
	if got := patch.Visuals["revenue"].Renderer; got != "echarts" {
		t.Fatalf("revenue chart renderer = %q, want echarts", got)
	}
	if got := patch.Visuals["revenue"].Measures[0]; got != "revenue" {
		t.Fatalf("revenue chart measure = %q, want revenue", got)
	}
	if got := patch.Visuals["orders"].Type; got != "donut" {
		t.Fatalf("orders chart type = %q, want donut", got)
	}
	if got := datumString(patch.Visuals["categories"].Data[0], "label"); got != "health_beauty" {
		t.Fatalf("top category = %q, want health_beauty", got)
	}
	if got := len(patch.FilterOptions["state"]); got != 2 {
		t.Fatalf("state filter options = %d, want 2", got)
	}
	if _, ok := patch.Filters.Controls["category"]; ok {
		t.Fatalf("overview patch included off-page category filter: %#v", patch.Filters.Controls)
	}

	selectedFilters := dashboard.Filters{
		VisualSelections: []dashboard.VisualSelection{
			{VisualID: "orders", Field: "orders.status", Operator: "in", Values: []string{"delivered"}},
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
		t.Fatalf("orders chart points with self-selection = %d, want 2", len(selectedPatch.Visuals["orders"].Data))
	}
	if !pointSelected(selectedPatch.Visuals["orders"].Data, "delivered") {
		t.Fatalf("orders chart did not mark delivered as selected: %#v", selectedPatch.Visuals["orders"].Data)
	}
	if got := datumString(selectedPatch.Visuals["categories"].Data[0], "label"); got != "health_beauty" {
		t.Fatalf("category chart under status selection = %q, want health_beauty", got)
	}
	if got := datumString(selectedPatch.Visuals["revenue"].Data[0], "series"); got != "" {
		t.Fatalf("single-series chart row series = %q, want empty", got)
	}

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
		VisualSelections: []dashboard.VisualSelection{
			{VisualID: "orders", Field: "orders.status", Operator: "in", Values: []string{"delivered"}},
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
			name: "off-page category contains ignored",
			filters: dashboard.Filters{Controls: map[string]dashboard.FilterControl{
				"category": {Type: "text", Operator: "contains", Value: "watch"},
			}},
			want: "2",
		},
		{
			name: "off-page category equals ignored",
			filters: dashboard.Filters{Controls: map[string]dashboard.FilterControl{
				"category": {Type: "text", Operator: "equals", Value: "health_beauty"},
			}},
			want: "2",
		},
		{
			name: "off-page category not contains ignored",
			filters: dashboard.Filters{Controls: map[string]dashboard.FilterControl{
				"category": {Type: "text", Operator: "not_contains", Value: "health"},
			}},
			want: "2",
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

func pointSelected(points []dashboard.Datum, label string) bool {
	for _, point := range points {
		if datumString(point, "label") == label {
			selected, _ := point["selected"].(bool)
			return selected
		}
	}
	return false
}

func overviewVisualKeys() []string {
	return []string{"aov_kpi", "categories", "delivery", "orders", "revenue", "revenue_kpi", "review_kpi", "total_orders"}
}

func chartShowcaseMatrix() map[string][]string {
	return map[string][]string{
		"line":        {"revenue_line", "revenue_line_status", "revenue_line_step"},
		"area":        {"revenue", "revenue_area_status", "revenue_area_smooth"},
		"bar":         {"categories", "delivery", "categories_by_status_bar"},
		"column":      {"orders_by_month_column", "orders_by_month_status", "orders_by_month_status_grouped"},
		"pie":         {"status_pie", "status_pie_rose", "category_pie_inside"},
		"donut":       {"orders", "category_donut", "orders_donut_center"},
		"scatter":     {"delivery_scatter", "delivery_scatter_status", "delivery_scatter_labeled"},
		"funnel":      {"status_funnel", "delivery_funnel", "status_funnel_left"},
		"treemap":     {"category_treemap", "state_treemap", "category_treemap_roam"},
		"gauge":       {"total_orders_gauge", "review_gauge", "review_gauge_thresholds"},
		"heatmap":     {"state_status_heatmap", "category_status_heatmap", "category_status_heatmap_labels"},
		"sankey":      {"status_delivery_flow", "category_status_flow", "category_status_flow_spacious"},
		"graph":       {"status_delivery_graph", "category_status_graph", "category_status_graph_circular"},
		"map":         {"state_order_map", "state_revenue_map", "state_revenue_map_labeled"},
		"candlestick": {"delivery_candlestick", "revenue_candlestick"},
		"boxplot":     {"delivery_distribution", "review_distribution", "revenue_distribution"},
		"combo":       {"revenue_orders_combo", "review_delivery_combo", "revenue_orders_dual_axis_combo"},
		"waterfall":   {"revenue_waterfall", "orders_waterfall", "revenue_waterfall_labeled"},
		"histogram":   {"delivery_histogram", "revenue_histogram", "review_histogram"},
		"radar":       {"status_radar", "delivery_radar", "state_radar"},
		"tree":        {"state_status_tree", "category_status_tree", "category_state_status_tree"},
		"sunburst":    {"category_status_sunburst", "state_status_sunburst", "category_state_status_sunburst"},
	}
}

func assertVisualKeys(t *testing.T, patch dashboard.Patch, want []string) {
	t.Helper()
	got := make([]string, 0, len(patch.Visuals))
	for key := range patch.Visuals {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("visual keys = %#v, want %#v; status error = %q", got, want, patch.Status.Error)
	}
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

func tableColumnHasFormatting(columns []dashboard.TableColumn, key, kind string) bool {
	for _, column := range columns {
		if column.Key != key {
			continue
		}
		for _, rule := range column.Formatting {
			if rule.Kind == kind {
				return true
			}
		}
	}
	return false
}

func tableHasAnyFormatting(columns []dashboard.TableColumn, kind string) bool {
	for _, column := range columns {
		for _, rule := range column.Formatting {
			if rule.Kind == kind {
				return true
			}
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

func datumInt(row dashboard.Datum, key string) int {
	value, ok := row[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		var out int
		_, _ = fmt.Sscan(fmt.Sprint(value), &out)
		return out
	}
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
