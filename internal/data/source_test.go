package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/semantic"
	sourcereg "github.com/Yacobolo/libredash/internal/source"
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

	relation, err := compileSourceRelation(sourcePlan{
		kind:           "object",
		connection:     "crm",
		connectionSpec: sourcereg.Connection{ObjectRelation: sourcereg.ObjectRelationAttach},
		object:         "public.accounts",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM conn_crm.public.accounts"; relation != want {
		t.Fatalf("object relation = %q, want %q", relation, want)
	}

	relation, err = compileSourceRelation(sourcePlan{
		kind:       "object",
		connection: "remote_quack",
		connectionConfig: semantic.Connection{
			Path:    "quack:quack.example.com:443",
			Options: map[string]any{"disable_ssl": true},
		},
		connectionSpec: sourcereg.Connection{ObjectRelation: sourcereg.ObjectRelationQuackQuery},
		object:         "information_schema.schemata",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT * FROM quack_query('quack:quack.example.com:443', 'SELECT * FROM information_schema.schemata', disable_ssl => true)"
	if relation != want {
		t.Fatalf("quack relation = %q, want %q", relation, want)
	}
	if strings.Contains(relation, "secret-token") {
		t.Fatalf("quack relation contains token: %q", relation)
	}
	relation, err = compileSourceRelation(sourcePlan{
		kind:       "object",
		connection: "remote_quack",
		connectionConfig: semantic.Connection{
			Path: "quack:quack.example.com:443",
		},
		connectionSpec: sourcereg.Connection{ObjectRelation: sourcereg.ObjectRelationQuackQuery},
		object:         "information_schema.schemata",
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "SELECT * FROM quack_query('quack:quack.example.com:443', 'SELECT * FROM information_schema.schemata')"
	if relation != want {
		t.Fatalf("quack relation without options = %q, want %q", relation, want)
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

	stmt, ok, err = compileConnectionSecret("remote_quack", semantic.Connection{
		Kind: "quack",
		Path: "quack:quack.example.com:443",
		Auth: semantic.ConnectionAuth{"token": "secret-token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "CREATE OR REPLACE SECRET libredash_remote_quack (TYPE quack, TOKEN 'secret-token', SCOPE 'quack:quack.example.com:443')"
	if !ok || stmt != want {
		t.Fatalf("quack secret = %q ok=%v, want %q ok=true", stmt, ok, want)
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
			"remote_quack": {
				Kind: "quack",
				Path: "quack:quack.example.com:443",
				Auth: semantic.ConnectionAuth{"token": "secret-token"},
			},
		},
		Sources: map[string]semantic.Source{
			"events":   {Format: "parquet", Path: "s3://bucket/events/*.parquet", Connection: "lake"},
			"budget":   {Format: "excel", Path: "budget.xlsx", Connection: "lake"},
			"orders":   {Format: "delta", Path: "az://warehouse/orders", Connection: "azure"},
			"archive":  {Format: "vortex", Path: "orders.vortex", Connection: "lake"},
			"vectors":  {Format: "lance", Path: "vectors/products.lance", Connection: "lake"},
			"accounts": {Connection: "crm", Object: "public.accounts"},
			"lake_tbl": {Connection: "duck", Object: "main.orders"},
			"schemata": {Connection: "remote_quack", Object: "information_schema.schemata"},
		},
	}
	if got := strings.Join(requiredExtensions(model), ","); got != "azure,delta,ducklake,excel,httpfs,lance,postgres,quack,vortex" {
		t.Fatalf("required extensions = %q, want azure,delta,ducklake,excel,httpfs,lance,postgres,quack,vortex", got)
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
			"remote_quack": {
				Kind:    "quack",
				Path:    "quack:quack.example.com:443",
				Auth:    semantic.ConnectionAuth{"token": "secret-token"},
				Options: map[string]any{"disable_ssl": false},
			},
		},
		Sources: map[string]semantic.Source{
			"orders":     {Path: "orders.csv"},
			"events":     {Connection: "prod_lake", Path: "events/*", Format: "parquet"},
			"delta":      {Connection: "azure", Path: "tables/orders", Format: "delta"},
			"embeddings": {Connection: "vectors", Path: "vectors/products.lance"},
			"schemata":   {Connection: "remote_quack", Object: "information_schema.schemata"},
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

	relation, err = metrics.sourceRelation(model, model.Sources["schemata"])
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM quack_query('quack:quack.example.com:443', 'SELECT * FROM information_schema.schemata', disable_ssl => false)"; relation != want {
		t.Fatalf("quack relation = %q, want %q", relation, want)
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

func TestDuckDBQuackSmoke(t *testing.T) {
	uri := os.Getenv("LIBREDASH_QUACK_TEST_URI")
	token := os.Getenv("LIBREDASH_QUACK_TEST_TOKEN")
	if uri == "" || token == "" {
		t.Skip("set LIBREDASH_QUACK_TEST_URI and LIBREDASH_QUACK_TEST_TOKEN to run Quack smoke test")
	}

	db, err := sql.Open("duckdb", filepath.Join(t.TempDir(), "quack-smoke.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRowContext(context.Background(), "SELECT version()").Scan(&version); err != nil {
		t.Fatalf("query DuckDB version: %v", err)
	}
	t.Logf("DuckDB version: %s", version)

	if _, err := db.ExecContext(context.Background(), "INSTALL quack"); err != nil {
		t.Fatalf("install quack: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), "LOAD quack"); err != nil {
		t.Fatalf("load quack: %v", err)
	}
	stmt := fmt.Sprintf(
		"CREATE OR REPLACE SECRET libredash_quack_smoke (TYPE quack, TOKEN '%s', SCOPE '%s')",
		sqlString(token),
		sqlString(uri),
	)
	if _, err := db.ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("create quack secret: %v", err)
	}

	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM quack_query('%s', 'select * from information_schema.schemata')",
		sqlString(uri),
	)
	var count int
	if err := db.QueryRowContext(context.Background(), query).Scan(&count); err != nil {
		t.Fatalf("query quack schemata: %v", err)
	}
	if count == 0 {
		t.Fatal("quack schemata query returned zero rows")
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
