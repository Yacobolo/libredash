package semantic_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
)

func TestLoadOlistModel(t *testing.T) {
	model := loadOlistModel(t)

	if model.Name != "sales" {
		t.Fatalf("model name = %q, want sales", model.Name)
	}
	if len(model.Sources) != 6 {
		t.Fatalf("source count = %d, want 6", len(model.Sources))
	}
	if got := model.DefaultConnection; got != "olist" {
		t.Fatalf("default connection = %q, want olist", got)
	}
	if got := model.Connections["olist"].Kind; got != "managed" {
		t.Fatalf("olist connection kind = %q, want managed", got)
	}
	if got := model.Sources["olist_orders"].Format; got != "csv" {
		t.Fatalf("orders source format = %q, want csv", got)
	}
	if got := model.Sources["olist_orders"].Connection; got != "olist" {
		t.Fatalf("orders source connection = %q, want olist", got)
	}
	if got := model.Sources["olist_orders"].Path; got != "olist_orders_dataset.csv" {
		t.Fatalf("orders source path = %q, want olist_orders_dataset.csv", got)
	}
	if _, ok := model.Tables["orders"]; !ok {
		t.Fatal("orders model table missing")
	}
	if len(model.Relationships) != 1 {
		t.Fatalf("relationship count = %d, want 1", len(model.Relationships))
	}
}

func TestLoadRejectsLegacyFieldTransformKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.yaml")
	if err := os.WriteFile(path, []byte(`
name: olist
connections:
  local: {kind: managed}
sources:
  orders:
    connection: local
    path: orders.csv
models:
  orders:
    source: orders
    primaryKey: order_id
    fields:
      order_id:
        expr: order_id
semantic_models:
  olist:
    base_table: orders
    tables:
      - orders
    measures:
      defaults: {table: orders, grain: order_id}
      order_count: {expr: COUNT(DISTINCT orders.order_id)}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := semantic.Load(path)
	if err == nil || !strings.Contains(err.Error(), "legacy semantic model files are not supported") {
		t.Fatalf("semantic.Load() error = %v, want unsupported legacy format", err)
	}
}

func TestModelValidateAllowsMeasureOnDisconnectedFactTable(t *testing.T) {
	model := minimalSourceModel()
	model.Sources["customers"] = Source{Path: "customers.csv"}
	model.Measures = map[string]MetricMeasure{}
	model.Tables["customers"] = ModelTable{
		Kind:       "dimension",
		Source:     "customers",
		PrimaryKey: "customer_id",
		Grain:      "customer_id",
		Dimensions: map[string]MetricDimension{
			"customer_id": {Expr: "customer_id"},
			"state":       {Expr: "customer_state"},
		},
		Measures: map[string]MetricMeasure{
			"customer_count": {Label: "Customers", Expression: "COUNT(DISTINCT customers.customer_id)"},
		},
	}
	model.Measures["customer_count"] = MetricMeasure{
		Table:      "customers",
		Grain:      "customer_id",
		Expression: "COUNT(DISTINCT customers.customer_id)",
	}
	err := model.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v, want disconnected fact table to load", err)
	}
}

func TestModelValidateRejectsMissingRequiredModelColumn(t *testing.T) {
	model := minimalSourceModel()
	table := model.Tables["orders"]
	table.Columns = map[string]ModelColumn{
		"order_id": {},
	}
	table.Dimensions["status"] = MetricDimension{Label: "Status"}
	model.Tables["orders"] = table
	model.Measures = map[string]MetricMeasure{
		"revenue": {Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)", Label: "Revenue"},
	}
	err := model.Validate()
	if err == nil || !strings.Contains(err.Error(), `column contract missing field "status"`) {
		t.Fatalf("Validate() error = %v, want missing model column rejection", err)
	}
}

func TestModelValidateAcceptsNativeSourceFamilies(t *testing.T) {
	model := minimalSourceModel()
	model.DefaultConnection = "local_files"
	model.Connections = map[string]Connection{
		"local_files": {
			Kind: "managed",
			Defaults: ConnectionDefaults{
				Options: map[string]any{"header": true},
			},
		},
		"prod_lake": {
			Kind:  "s3",
			Scope: "s3://analytics-prod/",
			Auth: ConnectionAuth{
				"access_key_id":     "key",
				"secret_access_key": "secret",
				"region":            "us-east-1",
			},
		},
		"azure_lake": {
			Kind: "azure_blob",
			Auth: ConnectionAuth{"connection_string": "DefaultEndpointsProtocol=https;AccountName=mystorageaccount"},
		},
		"crm": {
			Kind: "postgres",
			Auth: ConnectionAuth{"connection_string": "postgres://crm"},
		},
		"lakehouse": {
			Kind: "ducklake",
			Path: "metadata.ducklake",
		},
		"remote_quack": {
			Kind: "quack",
			Path: "quack:quack.example.com:443",
			Auth: ConnectionAuth{"token": "secret-token"},
		},
	}
	model.Sources = map[string]Source{
		"orders": {
			Path: "olist_orders_dataset.csv",
		},
		"sales_events": {
			Format:     "parquet",
			Path:       "events/*",
			Connection: "prod_lake",
		},
		"delta_orders": {
			Format:     "delta",
			Path:       "az://warehouse/tables/orders",
			Connection: "azure_lake",
		},
		"crm_accounts": {
			Connection: "crm",
			Object:     "public.accounts",
		},
		"embeddings": {
			Connection: "prod_lake",
			Path:       "vectors/products.lance",
		},
		"ducklake_orders": {
			Connection: "lakehouse",
			Object:     "main.orders",
		},
		"remote_schemata": {
			Connection: "remote_quack",
			Object:     "information_schema.schemata",
		},
	}

	if err := model.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got := model.Sources["orders"].Format; got != "csv" {
		t.Fatalf("orders source format = %q, want csv", got)
	}
	if got := model.Sources["orders"].Options["header"]; got != true {
		t.Fatalf("orders header option = %#v, want true", got)
	}
}

func TestModelValidateResolvesConnectionAuthEnv(t *testing.T) {
	t.Setenv("LIBREDASH_TEST_S3_KEY", "env-key")
	t.Setenv("LIBREDASH_TEST_S3_SECRET", "env-secret")
	model := minimalSourceModel()
	model.Connections["prod_lake"] = Connection{
		Kind:  "s3",
		Scope: "s3://analytics-prod/",
		Auth: ConnectionAuth{
			"access_key_id":     "${LIBREDASH_TEST_S3_KEY}",
			"secret_access_key": "${LIBREDASH_TEST_S3_SECRET}",
		},
	}
	model.Sources = map[string]Source{
		"orders": {Connection: "prod_lake", Path: "orders.parquet"},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := model.Connections["prod_lake"].Auth["access_key_id"]; got != "env-key" {
		t.Fatalf("resolved access key = %#v, want env-key", got)
	}
}

func TestModelValidateConnectionCredentialsEnvReference(t *testing.T) {
	t.Setenv("LIBREDASH_TEST_CRM_URL", "postgres://crm")
	model := minimalSourceModel()
	model.Connections["crm"] = Connection{
		Kind:        "postgres",
		Credentials: ConnectionCredentials{Provider: "env", Secret: "LIBREDASH_TEST_CRM_URL"},
	}
	model.Sources = map[string]Source{
		"orders": {Connection: "crm", Object: "public.orders"},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(model.Connections["crm"].Auth) != 0 {
		t.Fatalf("compiled model stored resolved credentials in auth: %#v", model.Connections["crm"].Auth)
	}
}

func TestModelValidateConnectionCredentialsNone(t *testing.T) {
	model := minimalSourceModel()
	model.Connections["default"] = Connection{
		Kind:        "managed",
		Credentials: ConnectionCredentials{Provider: "none"},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	if semanticmodel.ConnectionCredentialsConfigured(model.Connections["default"]) {
		t.Fatalf("none credentials reported as configured")
	}
}

func TestModelValidateRejectsConnectionCredentialsNoneSecret(t *testing.T) {
	model := minimalSourceModel()
	model.Connections["default"] = Connection{
		Kind:        "managed",
		Credentials: ConnectionCredentials{Provider: "none", Secret: "LIBREDASH_TEST_SECRET"},
	}
	assertModelValidateError(t, model, `none credentials cannot set secret`)
}

func TestModelValidateRejectsMissingConnectionCredentialsEnv(t *testing.T) {
	model := minimalSourceModel()
	model.Connections["crm"] = Connection{
		Kind:        "postgres",
		Credentials: ConnectionCredentials{Provider: "env", Secret: "LIBREDASH_TEST_MISSING_CRM_URL"},
	}
	model.Sources = map[string]Source{
		"orders": {Connection: "crm", Object: "public.orders"},
	}
	assertModelValidateError(t, model, `env credential "LIBREDASH_TEST_MISSING_CRM_URL" is not set`)
}

func TestModelValidateRejectsMissingConnectionAuthEnv(t *testing.T) {
	model := minimalSourceModel()
	model.Connections["prod_lake"] = Connection{
		Kind:  "s3",
		Scope: "s3://analytics-prod/",
		Auth: ConnectionAuth{
			"access_key_id":     "${LIBREDASH_TEST_MISSING_KEY}",
			"secret_access_key": "secret",
		},
	}
	model.Sources = map[string]Source{
		"orders": {Connection: "prod_lake", Path: "orders.parquet"},
	}
	assertModelValidateError(t, model, "missing environment variable")
}

func TestModelValidateInfersFileFormats(t *testing.T) {
	cases := map[string]struct {
		path string
		want string
	}{
		"csv":     {path: "orders.csv", want: "csv"},
		"csv_gz":  {path: "orders.csv.gz", want: "csv"},
		"json":    {path: "orders.json", want: "json"},
		"jsonl":   {path: "orders.jsonl", want: "json"},
		"ndjson":  {path: "orders.ndjson", want: "json"},
		"parquet": {path: "orders.parquet", want: "parquet"},
		"excel":   {path: "orders.xlsx", want: "excel"},
		"text":    {path: "orders.txt", want: "text"},
		"blob":    {path: "orders.blob", want: "blob"},
		"vortex":  {path: "orders.vortex", want: "vortex"},
		"lance":   {path: "products.lance", want: "lance"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := minimalSourceModel()
			model.Connections["local_files"] = Connection{Kind: "managed"}
			model.Sources = map[string]Source{"orders": {Path: tc.path}}
			if err := model.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := model.Sources["orders"].Format; got != tc.want {
				t.Fatalf("format = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestModelValidateRejectsInvalidSources(t *testing.T) {
	cases := map[string]struct {
		source    Source
		contains  string
		noDefault bool
	}{
		"missing_source_shape": {
			source:   Source{Format: "csv"},
			contains: "exactly one of path or object",
		},
		"multiple_source_shapes": {
			source:   Source{Path: "orders.csv", Object: "public.orders"},
			contains: "exactly one of path or object",
		},
		"path_bad_format": {
			source:   Source{Format: "orc", Path: "orders.orc", Connection: "local_files"},
			contains: "unsupported format",
		},
		"ambiguous_path_missing_format": {
			source:   Source{Path: "events/*", Connection: "local_files"},
			contains: "requires format",
		},
		"lance_with_options": {
			source:   Source{Path: "vectors/products.lance", Connection: "local_files", Options: map[string]any{"sample_size": 1000}},
			contains: "lance path cannot set options",
		},
		"ducklake_path_source": {
			source:   Source{Path: "main.orders", Connection: "lakehouse", Format: "parquet"},
			contains: "path cannot use ducklake connection",
		},
		"quack_path_source": {
			source:   Source{Path: "main.orders", Connection: "remote_quack", Format: "parquet"},
			contains: "path cannot use quack connection",
		},
		"database_missing_connection": {
			source:    Source{Object: "public.accounts"},
			contains:  "requires connection",
			noDefault: true,
		},
		"database_wrong_connection_kind": {
			source:   Source{Connection: "local_files", Object: "public.accounts"},
			contains: "object cannot use managed connection",
		},
		"unknown_connection": {
			source:   Source{Format: "parquet", Path: "s3://bucket/*.parquet", Connection: "missing"},
			contains: "unknown connection",
		},
		"bad_source_option_key": {
			source:   Source{Format: "csv", Path: "orders.csv", Connection: "local_files", Options: map[string]any{"bad-key": true}},
			contains: "option",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := minimalSourceModel()
			if tc.noDefault {
				model.DefaultConnection = ""
			}
			model.Connections = map[string]Connection{
				"local_files": {Kind: "managed"},
				"crm":         {Kind: "mysql", Auth: ConnectionAuth{"connection_string": "mysql://crm"}},
				"lakehouse":   {Kind: "ducklake", Path: "metadata.ducklake"},
				"remote_quack": {
					Kind: "quack",
					Path: "quack:quack.example.com:443",
					Auth: ConnectionAuth{"token": "secret-token"},
				},
			}
			model.Sources = map[string]Source{"orders": tc.source}
			assertModelValidateError(t, model, tc.contains)
		})
	}
}

func TestModelValidateRejectsInvalidConnections(t *testing.T) {
	cases := map[string]struct {
		connection Connection
		contains   string
	}{
		"bad_auth_method": {
			connection: Connection{Kind: "s3", Auth: ConnectionAuth{"method": "credential_chain"}},
			contains:   "unsupported auth key",
		},
		"bad_auth_param": {
			connection: Connection{Kind: "s3", Auth: ConnectionAuth{"bad-key": "value"}},
			contains:   "auth key",
		},
		"bad_attach_option": {
			connection: Connection{Kind: "postgres", Auth: ConnectionAuth{"connection_string": "postgres://crm"}, Options: map[string]any{"password": "secret"}},
			contains:   "unsupported option",
		},
		"missing_required_auth": {
			connection: Connection{Kind: "s3", Auth: ConnectionAuth{"access_key_id": "key"}},
			contains:   "missing required credentials",
		},
		"bad_default_option": {
			connection: Connection{Kind: "managed", Defaults: ConnectionDefaults{Options: map[string]any{"bad-key": true}}},
			contains:   "default option",
		},
		"ducklake_missing_path": {
			connection: Connection{Kind: "ducklake"},
			contains:   "ducklake requires path",
		},
		"ducklake_bad_option": {
			connection: Connection{Kind: "ducklake", Path: "metadata.ducklake", Options: map[string]any{"read_only": true}},
			contains:   "unsupported option",
		},
		"non_ducklake_path": {
			connection: Connection{Kind: "s3", Path: "s3://bucket/", Auth: ConnectionAuth{"access_key_id": "key", "secret_access_key": "secret"}},
			contains:   "path is only supported for path-backed connections",
		},
		"quack_missing_path": {
			connection: Connection{Kind: "quack", Auth: ConnectionAuth{"token": "secret-token"}},
			contains:   "quack requires path",
		},
		"quack_bad_uri": {
			connection: Connection{Kind: "quack", Path: "https://quack.example.com", Auth: ConnectionAuth{"token": "secret-token"}},
			contains:   "quack path must start with quack:",
		},
		"quack_missing_token": {
			connection: Connection{Kind: "quack", Path: "quack:quack.example.com:443"},
			contains:   "requires auth",
		},
		"quack_bad_auth_key": {
			connection: Connection{Kind: "quack", Path: "quack:quack.example.com:443", Auth: ConnectionAuth{"password": "secret-token"}},
			contains:   "unsupported auth key",
		},
		"quack_bad_option": {
			connection: Connection{Kind: "quack", Path: "quack:quack.example.com:443", Auth: ConnectionAuth{"token": "secret-token"}, Options: map[string]any{"threads": 4}},
			contains:   "unsupported option",
		},
		"quack_disable_ssl_not_bool": {
			connection: Connection{Kind: "quack", Path: "quack:quack.example.com:443", Auth: ConnectionAuth{"token": "secret-token"}, Options: map[string]any{"disable_ssl": "true"}},
			contains:   "disable_ssl option must be a boolean",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := minimalSourceModel()
			model.Connections = map[string]Connection{"crm": tc.connection}
			model.Sources = map[string]Source{
				"orders": {Format: "csv", Path: "orders.csv", Connection: "crm"},
			}
			assertModelValidateError(t, model, tc.contains)
		})
	}
}

func TestLoadModelRejectsRemovedSourceFields(t *testing.T) {
	cases := map[string]string{
		"source_defaults": `
source_defaults:
  format: csv
`,
		"source_type": `
sources:
  orders:
    type: file
    path: orders.csv
`,
		"source_engine": `
sources:
  orders:
    engine: postgres
    object: public.orders
`,
		"connection_default_format": `
connections:
  other_files:
    kind: managed
    defaults:
      format: csv
`,
		"connection_secret": `
connections:
  crm:
    kind: postgres
    secret: crm_readonly
`,
		"auth_method": `
connections:
  prod_lake:
    kind: s3
    auth:
      method: credential_chain
`,
		"auth_params": `
connections:
  prod_lake:
    kind: s3
    auth:
      params:
        region: us-east-1
`,
		"source_query": `
sources:
  orders:
    query: SELECT 1 AS id
`,
		"source_location": `
sources:
  orders:
    location: orders.csv
`,
		"scalar_source": `
sources:
  orders: orders.csv
`,
		"source_field_label": `
sources:
  orders:
    path: orders.csv
    fields:
      order_id:
        label: Order ID
`,
	}
	for name, fragment := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "model.yaml")
			content := strings.ReplaceAll(`name: test
title: Test
default_connection: local_files
connections:
  local_files:
    kind: managed
sources:
  products:
    path: products.csv
cache:
  tables:
    orders_cache:
      sql: SELECT * FROM source.orders
datasets:
  orders:
    source: orders_cache
`+fragment, "\t", "  ")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := semantic.Load(path); err == nil {
				t.Fatal("Load() error = nil, want removed field rejection")
			}
		})
	}
}

func loadOlistModel(t *testing.T) *Model {
	t.Helper()
	compiled, err := workspacecompiler.CompileProject(filepath.Join("..", "..", "dashboards", "libredash.yaml"), workspacecompiler.Options{})
	if err != nil {
		t.Fatal(err)
	}
	model := compiled.Workspaces["sales"].Definition.Models["sales"]
	if model == nil {
		t.Fatal("sales model missing")
	}
	return model
}

func minimalSourceModel() *Model {
	return &Model{
		Name:              "test",
		Title:             "Test",
		Description:       "Test semantic model",
		DefaultConnection: "local_files",
		Connections: map[string]Connection{
			"local_files": {
				Kind: "managed",
			},
		},
		Sources: map[string]Source{
			"orders": {Path: "orders.csv"},
		},
		BaseTable: "orders",
		Tables: map[string]ModelTable{
			"orders": {
				Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
				Dimensions: map[string]MetricDimension{"order_id": {Expr: "order_id"}},
				Measures:   map[string]MetricMeasure{"order_count": {Label: "Orders", Expression: "COUNT(*)"}},
			},
		},
	}
}

func addIsolatedProductTable(model *Model) {
	model.Tables["products"] = ModelTable{
		Kind:       "model",
		Source:     "products",
		PrimaryKey: "product_id",
		Grain:      "product_id",
		Dimensions: map[string]MetricDimension{
			"category": {Expr: "category"},
		},
	}
	if model.Sources["products"].Path == "" {
		model.Sources["products"] = Source{Path: "products.csv", Connection: model.DefaultConnection}
	}
}

func fieldRefs(fields ...string) []FieldRef {
	refs := make([]FieldRef, len(fields))
	for i, field := range fields {
		qualified := field
		if !strings.Contains(field, ".") {
			if field == "state" {
				qualified = "customers.state"
			} else {
				qualified = "orders." + field
			}
		}
		refs[i] = FieldRef{Field: qualified, Alias: displayFieldName(qualified)}
	}
	return refs
}

func measureRefs(fields ...string) []FieldRef {
	refs := make([]FieldRef, len(fields))
	for i, field := range fields {
		refs[i] = FieldRef{Field: field, Alias: displayFieldName(field)}
	}
	return refs
}

func displayFieldName(field string) string {
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
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

func chartPageVisualIDs(page dashboard.Page) []string {
	visualIDs := make([]string, 0, len(page.Visuals))
	for _, pageVisual := range page.Visuals {
		if pageVisual.Visual != "" {
			visualIDs = append(visualIDs, pageVisual.Visual)
		}
	}
	return visualIDs
}

func hasTableColumnFormatting(columns []dashboard.TableColumn, key, kind string) bool {
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

func assertModelValidateError(t *testing.T, model *Model, contains string) {
	t.Helper()
	err := model.Validate()
	if err == nil {
		t.Fatalf("Validate() error = nil, want %q", contains)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("Validate() error = %q, want containing %q", err.Error(), contains)
	}
}

func assertDashboardValidateError(t *testing.T, report *Dashboard, model *Model, contains string) {
	t.Helper()
	err := workspacecompiler.ValidateDashboard(report, map[string]*Model{"olist": model})
	if err == nil {
		t.Fatalf("Validate() error = nil, want %q", contains)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("Validate() error = %q, want containing %q", err.Error(), contains)
	}
}
