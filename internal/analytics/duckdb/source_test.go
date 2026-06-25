package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	analyticsmaterialize "github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	_ "github.com/duckdb/duckdb-go/v2"
)

func TestDiscoverSchemasCapturesSourceAndModelColumns(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "orders.csv"), []byte("order_id,revenue\n1,10.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, filepath.Join(dir, "test.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	model := &semanticmodel.Model{
		Name:              "olist",
		DefaultConnection: "local",
		Connections:       map[string]semanticmodel.Connection{"local": {Kind: "local"}},
		Sources: map[string]semanticmodel.Source{"orders": {
			Connection: "local",
			Path:       "orders.csv",
			Format:     "csv",
			Fields: map[string]semanticmodel.SourceField{
				"order_id": {Description: "Raw order identifier."},
			},
		}},
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
		BaseTable: "orders",
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)", Label: "Revenue"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := analyticsmaterialize.Refresh(ctx, db, NewSourceRuntime(db, dir), model); err != nil {
		t.Fatal(err)
	}
	if err := DiscoverSchemas(ctx, db, model); err != nil {
		t.Fatal(err)
	}
	if got := model.Sources["orders"].Schema.Columns; len(got) != 2 || got[0].Name != "order_id" || got[0].Ordinal != 1 {
		t.Fatalf("source schema = %#v, want ordered source columns", got)
	}
	if got := model.Sources["orders"].Fields["order_id"].Description; got != "Raw order identifier." {
		t.Fatalf("source field description = %q, want docs preserved", got)
	}
	columns := model.Tables["orders"].Schema.Columns
	if len(columns) != 2 {
		t.Fatalf("model schema column count = %d, want 2: %#v", len(columns), columns)
	}
	if columns[0].Name != "order_id" || columns[0].PhysicalType == "" || !columns[0].PrimaryKey || columns[0].Nullable == nil {
		t.Fatalf("model order_id column = %#v, want physical type and primary key marker", columns[0])
	}
	if columns[1].Name != "revenue" || columns[1].PhysicalType == "" || columns[1].Nullable == nil {
		t.Fatalf("model revenue column = %#v, want physical type", columns[1])
	}
}

func TestDiscoverSchemasIgnoresAttachedDatabaseSchemas(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "orders.csv"), []byte("order_id,revenue\n1,10.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, filepath.Join(dir, "test.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	model := &semanticmodel.Model{
		Name:              "olist",
		DefaultConnection: "local",
		Connections:       map[string]semanticmodel.Connection{"local": {Kind: "local"}},
		Sources: map[string]semanticmodel.Source{"orders": {
			Connection: "local",
			Path:       "orders.csv",
			Format:     "csv",
		}},
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source:     "orders",
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Label: "Order ID"},
					"revenue":  {Label: "Revenue"},
				},
			},
		},
		BaseTable: "orders",
		Measures:  map[string]semanticmodel.MetricMeasure{},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := analyticsmaterialize.Refresh(ctx, db, NewSourceRuntime(db, dir), model); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQLDB().ExecContext(ctx, `
ATTACH ':memory:' AS attached_catalog;
CREATE SCHEMA attached_catalog.source;
CREATE TABLE attached_catalog.source.orders (attached_only INTEGER);
CREATE SCHEMA attached_catalog.model;
CREATE TABLE attached_catalog.model.orders (attached_only INTEGER);`); err != nil {
		t.Fatal(err)
	}
	if err := DiscoverSchemas(ctx, db, model); err != nil {
		t.Fatal(err)
	}
	sourceColumns := model.Sources["orders"].Schema.Columns
	for _, column := range sourceColumns {
		if column.Name == "attached_only" {
			t.Fatalf("source schema included attached database column: %#v", sourceColumns)
		}
	}
	tableColumns := model.Tables["orders"].Schema.Columns
	for _, column := range tableColumns {
		if column.Name == "attached_only" {
			t.Fatalf("model schema included attached database column: %#v", tableColumns)
		}
	}
}

func TestDiscoverSchemasRejectsMissingDocumentedSourceField(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "orders.csv"), []byte("order_id,revenue\n1,10.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, filepath.Join(dir, "test.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	model := &semanticmodel.Model{
		Name:              "olist",
		DefaultConnection: "local",
		Connections:       map[string]semanticmodel.Connection{"local": {Kind: "local"}},
		Sources: map[string]semanticmodel.Source{"orders": {
			Connection: "local",
			Path:       "orders.csv",
			Format:     "csv",
			Fields: map[string]semanticmodel.SourceField{
				"missing": {Description: "Missing source field."},
			},
		}},
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
		BaseTable: "orders",
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)", Label: "Revenue"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := analyticsmaterialize.Refresh(ctx, db, NewSourceRuntime(db, dir), model); err != nil {
		t.Fatal(err)
	}
	err = DiscoverSchemas(ctx, db, model)
	if err == nil || !strings.Contains(err.Error(), `source "orders" field "missing" is not in discovered schema`) {
		t.Fatalf("DiscoverSchemas() error = %v, want missing source field validation", err)
	}
}

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
		connectionSpec: semanticmodel.ConnectionSpec{ObjectRelation: semanticmodel.ObjectRelationAttach},
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
		connectionConfig: semanticmodel.Connection{
			Path:    "quack:quack.example.com:443",
			Options: map[string]any{"disable_ssl": true},
		},
		connectionSpec: semanticmodel.ConnectionSpec{ObjectRelation: semanticmodel.ObjectRelationQuackQuery},
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
		connectionConfig: semanticmodel.Connection{
			Path: "quack:quack.example.com:443",
		},
		connectionSpec: semanticmodel.ConnectionSpec{ObjectRelation: semanticmodel.ObjectRelationQuackQuery},
		object:         "information_schema.schemata",
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "SELECT * FROM quack_query('quack:quack.example.com:443', 'SELECT * FROM information_schema.schemata')"
	if relation != want {
		t.Fatalf("quack relation without options = %q, want %q", relation, want)
	}

	relation, err = compileSourceRelation(sourcePlan{
		kind:       "object",
		connection: "remote_quack",
		connectionConfig: semanticmodel.Connection{
			Path: "quack:quack.example.com:443",
			Auth: semanticmodel.ConnectionAuth{"token": "secret-token"},
		},
		connectionSpec: semanticmodel.ConnectionSpec{ObjectRelation: semanticmodel.ObjectRelationQuackQuery},
		object:         "oeducklake.oe_aravind.fact_general_ledger_line",
		fields:         []string{"gl_line_fact_key", "amount_dkk", "transaction_date"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "SELECT * FROM quack_query('quack:quack.example.com:443', 'SELECT gl_line_fact_key, amount_dkk, transaction_date FROM oeducklake.oe_aravind.fact_general_ledger_line')"
	if relation != want {
		t.Fatalf("quack projected relation = %q, want %q", relation, want)
	}
	if strings.Contains(relation, "CAST(") || strings.Contains(relation, "VARCHAR") {
		t.Fatalf("quack projected relation should preserve native types: %q", relation)
	}
	if strings.Contains(relation, "secret-token") {
		t.Fatalf("quack projected relation contains token: %q", relation)
	}
	relation, err = compileSourceRelation(sourcePlan{
		kind:       "object",
		connection: "remote_quack",
		connectionConfig: semanticmodel.Connection{
			Path: "quack:quack.example.com:443",
		},
		connectionSpec:  semanticmodel.ConnectionSpec{ObjectRelation: semanticmodel.ObjectRelationQuackQuery},
		object:          "oeducklake.oe_aravind.fact_general_ledger_line",
		rowPresenceOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "SELECT * FROM quack_query('quack:quack.example.com:443', 'SELECT 1 AS __libredash_row_present FROM oeducklake.oe_aravind.fact_general_ledger_line')"
	if relation != want {
		t.Fatalf("quack row-presence relation = %q, want %q", relation, want)
	}

	relation, err = compileSourceRelation(sourcePlan{
		kind:   "path",
		format: "csv",
		path:   "/data/orders.csv",
		columns: []sourceReadColumn{
			{SourceField: "raw_order_id", OutputField: "order_id"},
			{SourceField: "gross_revenue", OutputField: "revenue"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "SELECT raw_order_id AS order_id, gross_revenue AS revenue FROM read_csv('/data/orders.csv')"
	if relation != want {
		t.Fatalf("projected column relation = %q, want %q", relation, want)
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

func TestQuackMetadataColumnsSQLUsesInformationSchema(t *testing.T) {
	sqlText, err := quackMetadataColumnsSQL(
		"quack:quack.example.com:443",
		"oeducklake.oe_aravind.fact_general_ledger_line",
		map[string]any{"disable_ssl": true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "information_schema.columns") {
		t.Fatalf("metadata SQL = %q, want information_schema.columns", sqlText)
	}
	if strings.Contains(sqlText, "SELECT * FROM oeducklake.oe_aravind.fact_general_ledger_line") {
		t.Fatalf("metadata SQL scans source object: %q", sqlText)
	}
	if strings.Contains(sqlText, "secret-token") {
		t.Fatalf("metadata SQL contains token: %q", sqlText)
	}
}

func TestQuackLimitZeroSchemaRelationAvoidsFactScan(t *testing.T) {
	relation, err := quackLimitZeroSchemaRelation(
		"quack:quack.example.com:443",
		"information_schema.columns",
		map[string]any{"disable_ssl": true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(relation, "SELECT * FROM information_schema.columns LIMIT 0") {
		t.Fatalf("limit-zero schema relation = %q, want zero-row remote read", relation)
	}
	if strings.Contains(relation, "secret-token") {
		t.Fatalf("schema relation contains token: %q", relation)
	}
}

func TestCompileConnectionSecret(t *testing.T) {
	stmt, ok, err := compileConnectionSecret("prod_lake", semanticmodel.Connection{
		Kind:  "s3",
		Scope: "s3://analytics-prod/",
		Auth: semanticmodel.ConnectionAuth{
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

	stmt, ok, err = compileConnectionSecret("azure_lake", semanticmodel.Connection{
		Kind: "azure_blob",
		Auth: semanticmodel.ConnectionAuth{"connection_string": "DefaultEndpointsProtocol=https;AccountName=mystorageaccount"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = "CREATE OR REPLACE SECRET libredash_azure_lake (TYPE azure, PROVIDER config, CONNECTION_STRING 'DefaultEndpointsProtocol=https;AccountName=mystorageaccount')"
	if !ok || stmt != want {
		t.Fatalf("azure secret = %q ok=%v, want %q ok=true", stmt, ok, want)
	}

	stmt, ok, err = compileConnectionSecret("azure_lake", semanticmodel.Connection{
		Kind: "azure_blob",
		Auth: semanticmodel.ConnectionAuth{
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

	stmt, ok, err = compileConnectionSecret("crm", semanticmodel.Connection{
		Kind: "postgres",
		Auth: semanticmodel.ConnectionAuth{"connection_string": "postgres://crm"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok || stmt != "" {
		t.Fatalf("postgres secret = %q ok=%v, want empty ok=false", stmt, ok)
	}

	stmt, ok, err = compileConnectionSecret("lakehouse", semanticmodel.Connection{
		Kind:  "ducklake",
		Path:  "metadata.ducklake",
		Scope: "s3://analytics-prod/ducklake/",
		Auth: semanticmodel.ConnectionAuth{
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

	stmt, ok, err = compileConnectionSecret("remote_quack", semanticmodel.Connection{
		Kind: "quack",
		Path: "quack:quack.example.com:443",
		Auth: semanticmodel.ConnectionAuth{"token": "secret-token"},
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
	model := &semanticmodel.Model{
		Connections: map[string]semanticmodel.Connection{
			"prod_lake": {
				Kind:  "s3",
				Scope: "s3://analytics-prod/",
				Auth: semanticmodel.ConnectionAuth{
					"access_key_id":     "key",
					"secret_access_key": "secret",
				},
			},
			"public": {
				Kind: "http",
			},
		},
		Sources: map[string]semanticmodel.Source{
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
		connection semanticmodel.Connection
		want       string
	}{
		"postgres_auth": {
			connection: semanticmodel.Connection{Kind: "postgres", Auth: semanticmodel.ConnectionAuth{"connection_string": "postgres://crm"}},
			want:       "ATTACH 'postgres://crm' AS conn_crm (TYPE postgres, READ_ONLY)",
		},
		"mysql_auth": {
			connection: semanticmodel.Connection{Kind: "mysql", Auth: semanticmodel.ConnectionAuth{"connection_string": "mysql://sales"}},
			want:       "ATTACH 'mysql://sales' AS conn_crm (TYPE mysql, READ_ONLY)",
		},
		"sqlite_option_path": {
			connection: semanticmodel.Connection{Kind: "sqlite", Options: map[string]any{"path": "/tmp/source.sqlite"}},
			want:       "ATTACH '/tmp/source.sqlite' AS conn_crm (TYPE sqlite, READ_ONLY)",
		},
		"sqlite_auth_path": {
			connection: semanticmodel.Connection{Kind: "sqlite", Auth: semanticmodel.ConnectionAuth{"path": "/tmp/source.sqlite"}},
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
	stmt, err := compileObjectAttach(&semanticmodel.Model{}, "lakehouse", semanticmodel.Connection{
		Kind: "ducklake",
		Path: "metadata.ducklake",
		Options: map[string]any{
			"data_path": "data_files",
		},
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := "ATTACH 'ducklake:" + SQLString(filepath.Join(dir, "metadata.ducklake")) + "' AS conn_lakehouse (DATA_PATH '" + SQLString(filepath.Join(dir, "data_files")) + "')"
	if stmt != want {
		t.Fatalf("local ducklake attach = %q, want %q", stmt, want)
	}

	stmt, err = compileObjectAttach(&semanticmodel.Model{}, "lakehouse", semanticmodel.Connection{
		Kind:  "ducklake",
		Scope: "s3://analytics-prod/ducklake/",
		Path:  "metadata.ducklake",
		Options: map[string]any{
			"data_path": "data",
		},
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	want = "ATTACH 'ducklake:s3://analytics-prod/ducklake/metadata.ducklake' AS conn_lakehouse (DATA_PATH 's3://analytics-prod/ducklake/data')"
	if stmt != want {
		t.Fatalf("remote ducklake attach = %q, want %q", stmt, want)
	}
}

func TestRequiredExtensions(t *testing.T) {
	model := &semanticmodel.Model{
		Connections: map[string]semanticmodel.Connection{
			"lake":  {Kind: "s3"},
			"azure": {Kind: "azure_blob"},
			"crm":   {Kind: "postgres"},
			"duck":  {Kind: "ducklake", Path: "metadata.ducklake"},
			"remote_quack": {
				Kind: "quack",
				Path: "quack:quack.example.com:443",
				Auth: semanticmodel.ConnectionAuth{"token": "secret-token"},
			},
		},
		Sources: map[string]semanticmodel.Source{
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
	if got := strings.Join(RequiredExtensions(model), ","); got != "azure,delta,ducklake,excel,httpfs,lance,postgres,quack,vortex" {
		t.Fatalf("required extensions = %q, want azure,delta,ducklake,excel,httpfs,lance,postgres,quack,vortex", got)
	}
}

func TestSourceRelationResolvesSourcePlans(t *testing.T) {
	dir := t.TempDir()
	model := &semanticmodel.Model{
		Name:              "test",
		DefaultConnection: "local_files",
		Connections: map[string]semanticmodel.Connection{
			"local_files": {
				Kind: "local",
				Root: "fixtures",
				Defaults: semanticmodel.ConnectionDefaults{
					Options: map[string]any{"header": true},
				},
			},
			"prod_lake": {Kind: "s3", Scope: "s3://analytics-prod/", Auth: semanticmodel.ConnectionAuth{"access_key_id": "key", "secret_access_key": "secret"}},
			"azure":     {Kind: "azure_blob", Scope: "az://warehouse/", Auth: semanticmodel.ConnectionAuth{"connection_string": "DefaultEndpointsProtocol=https;AccountName=warehouse"}},
			"vectors":   {Kind: "s3", Scope: "s3://analytics-prod/", Auth: semanticmodel.ConnectionAuth{"access_key_id": "key", "secret_access_key": "secret"}},
			"remote_quack": {
				Kind:    "quack",
				Path:    "quack:quack.example.com:443",
				Auth:    semanticmodel.ConnectionAuth{"token": "secret-token"},
				Options: map[string]any{"disable_ssl": false},
			},
		},
		Sources: map[string]semanticmodel.Source{
			"orders":     {Path: "orders.csv"},
			"events":     {Connection: "prod_lake", Path: "events/*", Format: "parquet"},
			"delta":      {Connection: "azure", Path: "tables/orders", Format: "delta"},
			"embeddings": {Connection: "vectors", Path: "vectors/products.lance"},
			"schemata":   {Connection: "remote_quack", Object: "information_schema.schemata"},
		},
		BaseTable: "orders",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Expr: "order_id"}},
				Measures:   map[string]semanticmodel.MetricMeasure{"orders": {Label: "Orders", Expression: "COUNT(*)"}},
			},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	relation, err := SourceRelation(model, model.Sources["orders"], dir)
	if err != nil {
		t.Fatal(err)
	}
	wantLocal := "SELECT * FROM read_csv('" + SQLString(filepath.Join(dir, "fixtures", "orders.csv")) + "', header = true)"
	if relation != wantLocal {
		t.Fatalf("local relation = %q, want %q", relation, wantLocal)
	}

	relation, err = SourceRelation(model, model.Sources["events"], dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM read_parquet('s3://analytics-prod/events/*')"; relation != want {
		t.Fatalf("remote relation = %q, want %q", relation, want)
	}

	relation, err = SourceRelation(model, model.Sources["delta"], dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM delta_scan('az://warehouse/tables/orders')"; relation != want {
		t.Fatalf("delta relation = %q, want %q", relation, want)
	}

	relation, err = SourceRelation(model, model.Sources["embeddings"], dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM 's3://analytics-prod/vectors/products.lance'"; relation != want {
		t.Fatalf("lance relation = %q, want %q", relation, want)
	}

	relation, err = SourceRelation(model, model.Sources["schemata"], dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM quack_query('quack:quack.example.com:443', 'SELECT * FROM information_schema.schemata', disable_ssl => false)"; relation != want {
		t.Fatalf("quack relation = %q, want %q", relation, want)
	}

	bad := model.Sources["events"]
	bad.Path = "s3://other-bucket/events/*"
	_, err = SourceRelation(model, bad, dir)
	if err == nil || !strings.Contains(err.Error(), "outside connection") {
		t.Fatalf("mismatched remote path error = %v, want outside connection", err)
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
		SQLString(token),
		SQLString(uri),
	)
	if _, err := db.ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("create quack secret: %v", err)
	}

	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM quack_query('%s', 'select * from information_schema.schemata')",
		SQLString(uri),
	)
	var count int
	if err := db.QueryRowContext(context.Background(), query).Scan(&count); err != nil {
		t.Fatalf("query quack schemata: %v", err)
	}
	if count == 0 {
		t.Fatal("quack schemata query returned zero rows")
	}
}
