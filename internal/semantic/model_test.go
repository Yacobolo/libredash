package semantic

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

func TestLoadWorkspaceCatalog(t *testing.T) {
	workspace, err := LoadWorkspace(filepath.Join("..", "..", "dashboards", "catalog.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	if len(workspace.Catalog.SemanticModels) != 1 {
		t.Fatalf("model catalog count = %d, want 1", len(workspace.Catalog.SemanticModels))
	}
	if len(workspace.Catalog.MetricViews) != 1 {
		t.Fatalf("metrics view catalog count = %d, want 1", len(workspace.Catalog.MetricViews))
	}
	if len(workspace.Catalog.Dashboards) != 1 {
		t.Fatalf("dashboard catalog count = %d, want 1", len(workspace.Catalog.Dashboards))
	}
	if got := workspace.Catalog.Workspace.Title; got != "LibreDash Workspace" {
		t.Fatalf("workspace title = %q, want LibreDash Workspace", got)
	}
	if _, ok := workspace.Models["olist"]; !ok {
		t.Fatal("workspace missing olist model")
	}
	if _, ok := workspace.MetricViews["orders"]; !ok {
		t.Fatal("workspace missing orders metrics view")
	}
	if _, ok := workspace.Dashboards["executive-sales"]; !ok {
		t.Fatal("workspace missing executive-sales dashboard")
	}
}

func TestCatalogValidateRejectsDuplicateIDs(t *testing.T) {
	baseDir := filepath.Join("..", "..", "dashboards")
	catalog := Catalog{
		SemanticModels: []CatalogModel{
			{ID: "olist", Title: "Olist", Path: "olist/model.yaml"},
			{ID: "olist", Title: "Olist Copy", Path: "olist/model.yaml"},
		},
		MetricViews: []CatalogMetricView{
			{ID: "orders", Title: "Orders", Path: "olist/orders.metrics.yaml", SemanticModel: "olist"},
		},
		Dashboards: []CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales", Path: "olist/executive-sales.yaml"},
		},
	}

	assertCatalogValidateError(t, catalog, baseDir, "duplicate semantic model")
}

func TestCatalogValidateRejectsUnknownMetricViewModel(t *testing.T) {
	baseDir := filepath.Join("..", "..", "dashboards")
	catalog := Catalog{
		SemanticModels: []CatalogModel{
			{ID: "olist", Title: "Olist", Path: "olist/model.yaml"},
		},
		MetricViews: []CatalogMetricView{
			{ID: "orders", Title: "Orders", Path: "olist/orders.metrics.yaml", SemanticModel: "missing"},
		},
		Dashboards: []CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales", Path: "olist/executive-sales.yaml"},
		},
	}

	assertCatalogValidateError(t, catalog, baseDir, "unknown semantic model")
}

func TestCatalogValidateRejectsMissingPath(t *testing.T) {
	baseDir := filepath.Join("..", "..", "dashboards")
	catalog := Catalog{
		SemanticModels: []CatalogModel{
			{ID: "olist", Title: "Olist", Path: "olist/missing.yaml"},
		},
		MetricViews: []CatalogMetricView{
			{ID: "orders", Title: "Orders", Path: "olist/orders.metrics.yaml", SemanticModel: "olist"},
		},
		Dashboards: []CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales", Path: "olist/executive-sales.yaml"},
		},
	}

	assertCatalogValidateError(t, catalog, baseDir, "missing.yaml")
}

func TestLoadOlistModel(t *testing.T) {
	model := loadOlistModel(t)

	if model.Name != "olist" {
		t.Fatalf("model name = %q, want olist", model.Name)
	}
	if len(model.Sources) != 7 {
		t.Fatalf("source count = %d, want 7", len(model.Sources))
	}
	if got := model.DefaultConnection; got != "olist" {
		t.Fatalf("default connection = %q, want olist", got)
	}
	if got := model.Connections["olist"].Kind; got != "local" {
		t.Fatalf("olist connection kind = %q, want local", got)
	}
	if got := model.Sources["orders"].Format; got != "csv" {
		t.Fatalf("orders source format = %q, want csv", got)
	}
	if got := model.Sources["orders"].Connection; got != "olist" {
		t.Fatalf("orders source connection = %q, want olist", got)
	}
	if got := model.Sources["orders"].Path; got != "olist_orders_dataset.csv" {
		t.Fatalf("orders source path = %q, want olist_orders_dataset.csv", got)
	}
	if got := model.Tables["orders"].Kind; got != "fact" {
		t.Fatalf("orders table kind = %q, want fact", got)
	}
	if len(model.Relationships) != 6 {
		t.Fatalf("relationship count = %d, want 6", len(model.Relationships))
	}
}

func TestModelValidateAcceptsNativeSourceFamilies(t *testing.T) {
	model := minimalSourceModel()
	model.DefaultConnection = "local_files"
	model.Connections = map[string]Connection{
		"local_files": {
			Kind: "local",
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
			model.Connections["local_files"] = Connection{Kind: "local"}
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
		"database_missing_connection": {
			source:    Source{Object: "public.accounts"},
			contains:  "requires connection",
			noDefault: true,
		},
		"database_wrong_connection_kind": {
			source:   Source{Connection: "local_files", Object: "public.accounts"},
			contains: "object cannot use local connection",
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
				"local_files": {Kind: "local"},
				"crm":         {Kind: "mysql", Auth: ConnectionAuth{"connection_string": "mysql://crm"}},
				"lakehouse":   {Kind: "ducklake", Path: "metadata.ducklake"},
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
			connection: Connection{Kind: "local", Defaults: ConnectionDefaults{Options: map[string]any{"bad-key": true}}},
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
    kind: local
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
    kind: local
sources:
  products:
    path: products.csv
cache:
  tables:
    orders_cache:
      sql: SELECT * FROM raw.orders
datasets:
  orders:
    source: orders_cache
`+fragment, "\t", "  ")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load() error = nil, want removed field rejection")
			}
		})
	}
}

func TestLoadOlistDashboard(t *testing.T) {
	model := loadOlistModel(t)
	report, err := LoadDashboard(filepath.Join("..", "..", "dashboards", "olist", "executive-sales.yaml"), loadOlistMetricViews(t, model))
	if err != nil {
		t.Fatal(err)
	}

	if report.ID != "executive-sales" {
		t.Fatalf("dashboard id = %q, want executive-sales", report.ID)
	}
	if got := report.Visuals["revenue"].MetricView; got != "orders" {
		t.Fatalf("revenue visual dataset = %q, want orders", got)
	}
	if got := report.Visuals["orders"].Type; got != "donut" {
		t.Fatalf("orders visual type = %q, want donut", got)
	}
	if got := report.Visuals["orders_by_month_status"].Query.Series.Field; got != "orders.status" {
		t.Fatalf("multi-series visual series = %q, want status", got)
	}
	if got := report.Visuals["revenue"].KindOrDefault(); got != "chart" {
		t.Fatalf("revenue visual kind = %q, want chart", got)
	}
	if got := report.Visuals["revenue"].ShapeOrDefault(); got != "category_value" {
		t.Fatalf("revenue visual shape = %q, want category_value", got)
	}
	if got := report.Visuals["orders_by_month_status"].ShapeOrDefault(); got != "category_series_value" {
		t.Fatalf("multi-series visual shape = %q, want category_series_value", got)
	}
	if got := report.Visuals["orders_by_month_status"].Options["stacked"]; got != true {
		t.Fatalf("multi-series visual options.stacked = %v, want true", got)
	}
	if got := report.Visuals["revenue_orders_combo"].ShapeOrDefault(); got != "category_multi_measure" {
		t.Fatalf("combo visual shape = %q, want category_multi_measure", got)
	}
	if got := report.Visuals["delivery_histogram"].ShapeOrDefault(); got != "binned_measure" {
		t.Fatalf("histogram visual shape = %q, want binned_measure", got)
	}
	if got := report.Visuals["category_status_sunburst"].ShapeOrDefault(); got != "hierarchy" {
		t.Fatalf("hierarchy visual shape = %q, want hierarchy", got)
	}
	if got := report.Visuals["revenue"].RendererOrDefault(); got != "echarts" {
		t.Fatalf("revenue visual renderer = %q, want echarts", got)
	}
	if got := report.Tables["orders"].DefaultSort.Key; got != "purchase_date" {
		t.Fatalf("orders table default sort = %q, want purchase_date", got)
	}
	if got := report.Tables["orders"].KindOrDefault(); got != "data_table" {
		t.Fatalf("orders table kind = %q, want data_table", got)
	}
	if got := report.Tables["state_status_matrix"].KindOrDefault(); got != "matrix_table" {
		t.Fatalf("state_status_matrix table kind = %q, want matrix_table", got)
	}
	if got := report.Tables["category_status_pivot"].ColumnDims[0]; got != "orders.status" {
		t.Fatalf("category_status_pivot column dimension = %q, want orders.status", got)
	}
	if got := report.Tables["orders_compact"].Style.RowHeight(); got != 28 {
		t.Fatalf("orders_compact row height = %d, want 28", got)
	}
	conditional := report.Tables["orders_conditional"]
	if !hasTableColumnFormatting(conditional.Columns, "status", "badge") {
		t.Fatalf("orders_conditional status column missing badge formatting: %#v", conditional.Columns)
	}
	if !hasTableColumnFormatting(conditional.Columns, "revenue", "data_bar") {
		t.Fatalf("orders_conditional revenue column missing data bar formatting: %#v", conditional.Columns)
	}
	if len(report.Tables["category_status_pivot_heat"].MeasureFormatting["orders.order_count"]) == 0 {
		t.Fatalf("category_status_pivot_heat missing order_count measure formatting")
	}
	if len(report.Pages) != 24 {
		t.Fatalf("page count = %d, want 24", len(report.Pages))
	}
	if got := report.Pages[1].ID; got != "chart-line" {
		t.Fatalf("second page id = %q, want chart-line", got)
	}
	tablePageIndex := -1
	for index, page := range report.Pages {
		if page.ID == "tables" {
			tablePageIndex = index
			break
		}
	}
	if tablePageIndex == -1 {
		t.Fatal("tables page missing")
	}
	tablePage := report.Pages[tablePageIndex].WithDefaults()
	tableVisualCount := 0
	for _, visual := range tablePage.PlacedVisuals() {
		if visual.Kind == "table" {
			tableVisualCount++
		}
	}
	if tableVisualCount != 9 {
		t.Fatalf("tables page table visual count = %d, want 9", tableVisualCount)
	}
	page := report.Pages[0].WithDefaults()
	if page.Grid.Columns != 12 || page.Grid.RowHeight != 48 {
		t.Fatalf("overview grid = %#v, want 12 columns and 48 row height", page.Grid)
	}
	visuals := page.PlacedVisuals()
	if visuals[0].Kind != "filter_card" || visuals[0].Filter != "purchase_date" {
		t.Fatalf("first page visual = %#v, want purchase_date filter card", visuals[0])
	}
	kpiCards := 0
	for _, visual := range visuals {
		if visual.Kind == "kpi_card" {
			kpiCards++
			if visual.Width != 321.5 {
				t.Fatalf("kpi card compiled width = %v, want 321.5", visual.Width)
			}
		}
	}
	if kpiCards != 4 {
		t.Fatalf("overview kpi card count = %d, want 4", kpiCards)
	}
	if got := report.Filters["purchase_date"].URLParam; got != "period" {
		t.Fatalf("purchase_date url param = %q, want period", got)
	}
}

func TestOlistDashboardChartShowcaseContract(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	matrix := chartShowcaseMatrix()
	pageByID := map[string]dashboard.Page{}
	for _, page := range report.Pages {
		pageByID[page.ID] = page
	}

	chartPageCount := 0
	for _, page := range report.Pages {
		if !strings.HasPrefix(page.ID, "chart-") {
			continue
		}
		chartPageCount++
		if _, ok := matrix[strings.TrimPrefix(page.ID, "chart-")]; !ok {
			t.Fatalf("unexpected chart showcase page %q", page.ID)
		}
	}
	if chartPageCount != len(matrix) {
		t.Fatalf("chart showcase page count = %d, want %d", chartPageCount, len(matrix))
	}

	for chartType, expectedVisualIDs := range matrix {
		page, ok := pageByID["chart-"+chartType]
		if !ok {
			t.Fatalf("missing chart showcase page for %q", chartType)
		}
		gotVisualIDs := chartPageVisualIDs(page)
		wantVisualIDs := append([]string(nil), expectedVisualIDs...)
		sort.Strings(gotVisualIDs)
		sort.Strings(wantVisualIDs)
		if !reflect.DeepEqual(gotVisualIDs, wantVisualIDs) {
			t.Fatalf("%s visual ids = %#v, want %#v", page.ID, gotVisualIDs, wantVisualIDs)
		}
		for _, pageVisual := range page.Visuals {
			if pageVisual.Visual == "" {
				continue
			}
			visual := report.Visuals[pageVisual.Visual]
			if got := visual.Type; got != chartType {
				t.Fatalf("%s visual %q type = %q, want %q", page.ID, pageVisual.Visual, got, chartType)
			}
			if got := pageVisual.Kind; got != chartType+"_chart" {
				t.Fatalf("%s page visual %q kind = %q, want %q", page.ID, pageVisual.ID, got, chartType+"_chart")
			}
		}
	}

	tablePage, ok := pageByID["tables"]
	if !ok {
		t.Fatal("tables showcase page missing")
	}
	gotTables := make([]string, 0, 9)
	for _, pageVisual := range tablePage.Visuals {
		if pageVisual.Kind == "table" {
			gotTables = append(gotTables, pageVisual.Table)
		}
	}
	wantTables := []string{
		"category_status_pivot",
		"category_status_pivot_heat",
		"orders",
		"orders_compact",
		"orders_conditional",
		"orders_full_grid",
		"orders_spacious",
		"state_status_matrix",
		"state_status_matrix_formatted",
	}
	sort.Strings(gotTables)
	if !reflect.DeepEqual(gotTables, wantTables) {
		t.Fatalf("tables showcase tables = %#v, want %#v", gotTables, wantTables)
	}
}

func TestOlistShowcaseVisualOptions(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	cases := map[string]string{
		"revenue_line_step":              "step",
		"status_pie_rose":                "rose_type",
		"orders_donut_center":            "center_label",
		"status_funnel_left":             "funnel_align",
		"review_gauge_thresholds":        "thresholds",
		"category_status_graph_circular": "layout",
		"revenue_orders_dual_axis_combo": "dual_axis",
		"category_status_heatmap_labels": "show_labels",
		"category_treemap_roam":          "breadcrumb",
		"category_state_status_sunburst": "initial_depth",
	}
	for visualID, option := range cases {
		visual, ok := report.Visuals[visualID]
		if !ok {
			t.Fatalf("missing showcase visual %q", visualID)
		}
		if _, ok := visual.Options[option]; !ok {
			t.Fatalf("showcase visual %q missing option %q", visualID, option)
		}
	}
}

func TestDashboardValidateAcceptsV3VisualMetadata(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.Kind = "chart"
	visual.Shape = "category_value"
	visual.Renderer = "echarts"
	visual.Options = map[string]any{"stacked": false}
	visual.RendererOptions = map[string]any{
		"echarts": map[string]any{
			"legend": map[string]any{"show": true},
			"dataZoom": []any{
				map[string]any{"type": "inside"},
			},
		},
	}
	report.Visuals["revenue"] = visual

	if err := report.Validate(loadOlistMetricViews(t, model)); err != nil {
		t.Fatalf("validate v3 visual: %v", err)
	}
}

func TestDashboardValidateRejectsInvalidVisualShape(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.Shape = "missing_shape"
	report.Visuals["revenue"] = visual

	assertDashboardValidateError(t, report, model, "unsupported shape")
}

func TestLoadDashboardRejectsLegacyTopLevelStacked(t *testing.T) {
	model := loadOlistModel(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.yaml")
	content := strings.ReplaceAll(`id: executive-sales
title: Executive Sales Dashboard
metrics_views: [orders]
filters: {}
visuals:
  total_orders:
    kind: kpi
    shape: single_value
    metrics_view: orders
    query:
      measures: [order_count]
  revenue:
    title: Revenue
    type: area
    stacked: true
    metrics_view: orders
    query:
      dimensions: [purchase_month]
      measures: [revenue]
tables:
  orders:
    title: Orders
    metrics_view: orders
    columns:
      - key: order_id
        label: Order
pages:
  - id: overview
    title: Overview
    visuals:
      - id: revenue
        kind: area_chart
        visual: revenue
        placement: { col: 1, row: 1, col_span: 1, row_span: 1 }
`, "\t", "  ")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadDashboard(path, loadOlistMetricViews(t, model))
	if err == nil || !strings.Contains(err.Error(), "legacy top-level stacked") {
		t.Fatalf("LoadDashboard error = %v, want legacy stacked rejection", err)
	}
}

func TestLoadDashboardRejectsLegacyKPIs(t *testing.T) {
	model := loadOlistModel(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.yaml")
	content := strings.ReplaceAll(`id: executive-sales
title: Executive Sales Dashboard
semantic_model: olist
filters: {}
kpis:
  total_orders:
    title: Orders
    dataset: orders
    measure: order_count
visuals:
  revenue:
    title: Revenue
    type: area
    dataset: orders
    query:
      dimensions: [purchase_month]
      measures: [revenue]
tables: {}
pages:
  - id: overview
    title: Overview
    visuals:
      - id: revenue
        kind: area_chart
        visual: revenue
        placement: { col: 1, row: 1, col_span: 1, row_span: 1 }
`, "\t", "  ")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadDashboard(path, loadOlistMetricViews(t, model))
	if err == nil || !strings.Contains(err.Error(), "legacy kpis") {
		t.Fatalf("LoadDashboard error = %v, want legacy kpis rejection", err)
	}
}

func TestDashboardValidateRejectsInvalidVisualKind(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.Kind = "map"
	report.Visuals["revenue"] = visual

	assertDashboardValidateError(t, report, model, "unsupported kind")
}

func TestDashboardValidateRejectsInvalidRenderer(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.Renderer = "canvas"
	report.Visuals["revenue"] = visual

	assertDashboardValidateError(t, report, model, "unsupported renderer")
}

func TestDashboardValidateRejectsShapeQueryMismatch(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.Shape = "category_series_value"
	report.Visuals["revenue"] = visual

	assertDashboardValidateError(t, report, model, "requires query series")
}

func TestDashboardValidateAcceptsAdvancedVisualShapes(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	cases := map[string]Visual{
		"heatmap": {
			Title:      "Heatmap",
			Shape:      "matrix",
			Renderer:   "echarts",
			Type:       "heatmap",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: fieldRefs("state", "status"), Measures: fieldRefs("order_count")},
		},
		"sankey": {
			Title:      "Flow",
			Shape:      "graph",
			Renderer:   "echarts",
			Type:       "sankey",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: fieldRefs("status", "delivery_bucket"), Measures: fieldRefs("order_count")},
		},
		"geo": {
			Title:      "Map",
			Shape:      "geo",
			Renderer:   "echarts",
			Type:       "map",
			MetricView: "orders",
			Options:    map[string]any{"map": "brazil_states"},
			Query:      VisualQuery{Dimensions: fieldRefs("state"), Measures: fieldRefs("order_count")},
		},
		"boxplot": {
			Title:      "Distribution",
			Shape:      "distribution",
			Renderer:   "echarts",
			Type:       "boxplot",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: fieldRefs("delivery_bucket"), Measures: fieldRefs("delivery_days")},
		},
		"combo": {
			Title:      "Combo",
			Shape:      "category_multi_measure",
			Renderer:   "echarts",
			Type:       "combo",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: fieldRefs("purchase_month"), Measures: fieldRefs("revenue", "order_count")},
		},
		"waterfall": {
			Title:      "Waterfall",
			Shape:      "category_delta",
			Renderer:   "echarts",
			Type:       "waterfall",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: fieldRefs("purchase_month"), Measures: fieldRefs("revenue")},
		},
		"histogram": {
			Title:      "Histogram",
			Shape:      "binned_measure",
			Renderer:   "echarts",
			Type:       "histogram",
			MetricView: "orders",
			Query:      VisualQuery{Measures: fieldRefs("delivery_days")},
		},
		"radar": {
			Title:      "Radar",
			Shape:      "category_value",
			Renderer:   "echarts",
			Type:       "radar",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: fieldRefs("status"), Measures: fieldRefs("order_count")},
		},
		"tree": {
			Title:      "Tree",
			Shape:      "hierarchy",
			Renderer:   "echarts",
			Type:       "tree",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: fieldRefs("state", "status"), Measures: fieldRefs("order_count")},
		},
		"sunburst": {
			Title:      "Sunburst",
			Shape:      "hierarchy",
			Renderer:   "echarts",
			Type:       "sunburst",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: fieldRefs("category", "status"), Measures: fieldRefs("order_count")},
		},
	}
	for name, visual := range cases {
		visual.Query.MetricView = "orders"
		report.Visuals = map[string]Visual{name: visual}
		report.Pages = []dashboard.Page{{ID: "overview", Title: "Overview", Visuals: []dashboard.PageVisual{{ID: name, Kind: visual.Type + "_chart", Visual: name, Placement: dashboard.PagePlacement{Col: 1, Row: 1, ColSpan: 1, RowSpan: 1}}}}}
		if err := report.Validate(loadOlistMetricViews(t, model)); err != nil {
			t.Fatalf("validate advanced shape %s: %v", name, err)
		}
	}
}

func TestDashboardValidateRejectsAdvancedShapeMismatch(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	visual := report.Visuals["revenue_orders_combo"]
	visual.Query.Measures = fieldRefs("revenue")
	report.Visuals["revenue_orders_combo"] = visual
	assertDashboardValidateError(t, report, model, "at least two query measures")

	report = loadOlistDashboard(t, model)
	visual = report.Visuals["delivery_histogram"]
	visual.Query.Dimensions = fieldRefs("status")
	report.Visuals["delivery_histogram"] = visual
	assertDashboardValidateError(t, report, model, "does not support query dimensions")
}

func TestDashboardValidateRejectsRendererTypeMismatch(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.Renderer = "echarts"
	visual.Shape = "category_value"
	visual.Type = "sankey"
	report.Visuals["revenue"] = visual

	assertDashboardValidateError(t, report, model, "does not support shape")
}

func TestDashboardValidateRejectsUnsafeRendererOptions(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.RendererOptions = map[string]any{
		"echarts": map[string]any{
			"series": []any{map[string]any{"renderItem": "function() {}"}},
		},
	}
	report.Visuals["revenue"] = visual

	assertDashboardValidateError(t, report, model, "unsafe renderer option")
}

func TestDashboardValidateRejectsNonObjectRendererOptions(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.RendererOptions = map[string]any{"echarts": "bad"}
	report.Visuals["revenue"] = visual

	assertDashboardValidateError(t, report, model, "must be an object")
}

func TestValidateRejectsUnknownModelTableSource(t *testing.T) {
	model := loadOlistModel(t)
	table := model.Tables["customers"]
	table.Source = "missing_source"
	model.Tables["customers"] = table

	assertModelValidateError(t, model, "unknown source")
}

func TestDashboardValidateRejectsUnknownVisualDimension(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.Query.Dimensions = fieldRefs("missing_dimension")
	report.Visuals["revenue"] = visual

	assertDashboardValidateError(t, report, model, "unknown dimension")
}

func TestDashboardValidateRejectsUnknownInteractionTarget(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["orders"]
	visual.Interaction.Targets.Visuals = append(visual.Interaction.Targets.Visuals, "missing_visual")
	report.Visuals["orders"] = visual

	assertDashboardValidateError(t, report, model, "unknown target visual")
}

func TestDashboardValidateRejectsSeriesOnUnsupportedChart(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["orders"]
	visual.Shape = "category_value"
	visual.Query.Series = FieldRef{Field: "status"}
	report.Visuals["orders"] = visual

	assertDashboardValidateError(t, report, model, "does not support series")
}

func TestDashboardValidateRejectsInvalidTableVariant(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	table := report.Tables["state_status_matrix"]
	table.Query.Rows = fieldRefs("missing_dimension")
	report.Tables["state_status_matrix"] = table
	assertDashboardValidateError(t, report, model, "unknown dimension")

	report = loadOlistDashboard(t, model)
	table = report.Tables["category_status_pivot"]
	table.Query.Columns = fieldRefs("missing_dimension")
	report.Tables["category_status_pivot"] = table
	assertDashboardValidateError(t, report, model, "unknown dimension")
}

func TestDashboardValidateRejectsLegacyTableKinds(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	table := report.Tables["state_status_matrix"]
	table.Kind = "matrix"
	report.Tables["state_status_matrix"] = table
	assertDashboardValidateError(t, report, model, "unsupported kind")

	report = loadOlistDashboard(t, model)
	table = report.Tables["category_status_pivot"]
	table.Kind = "pivot"
	report.Tables["category_status_pivot"] = table
	assertDashboardValidateError(t, report, model, "unsupported kind")
}

func TestDashboardValidateRejectsUnknownFilterDimension(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["state"]
	filter.Dimension = "missing_dimension"
	report.Filters["state"] = filter

	assertDashboardValidateError(t, report, model, "unknown dimension")
}

func TestDashboardValidateRejectsUnsupportedFilterOperator(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["category"]
	filter.Operators = append(filter.Operators, "regex")
	report.Filters["category"] = filter

	assertDashboardValidateError(t, report, model, "unsupported operator")
}

func TestDashboardValidateRejectsInvalidDatePreset(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["purchase_date"]
	filter.Presets = append(filter.Presets, FilterPreset{Value: "bad", Label: "Bad", From: "2018-01-01"})
	report.Filters["purchase_date"] = filter

	assertDashboardValidateError(t, report, model, "requires both from and to")
}

func TestDashboardValidateRejectsInvalidPagePlacement(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	report.Pages[0].Visuals[0].Placement.Col = 12
	report.Pages[0].Visuals[0].Placement.ColSpan = 2

	assertDashboardValidateError(t, report, model, "placement exceeds")
}

func TestDashboardValidateRejectsUnknownFilterCardReference(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	report.Pages[0].Visuals[0].Filter = "missing_filter"

	assertDashboardValidateError(t, report, model, "unknown filter")
}

func TestDashboardValidateRejectsMissingFilterCardReference(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	report.Pages[0].Visuals[0].Filter = ""

	assertDashboardValidateError(t, report, model, "requires filter")
}

func TestDashboardValidateRejectsDuplicateFilterURLParam(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["state"]
	filter.URLParam = "period"
	report.Filters["state"] = filter

	assertDashboardValidateError(t, report, model, "duplicates")
}

func TestDashboardFiltersFromURL(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	values := url.Values{
		"period":      {"custom"},
		"from":        {"2018-01-01"},
		"to":          {"2018-01-31"},
		"state":       {"SP", "RJ", "SP"},
		"category":    {"health"},
		"category_op": {"starts_with"},
	}

	filters := report.FiltersFromURL(values)

	date := filters.Controls["purchase_date"]
	if date.Preset != "custom" || date.From != "2018-01-01" || date.To != "2018-01-31" {
		t.Fatalf("date filter = %#v, want custom January 2018", date)
	}
	state := filters.Controls["state"]
	if strings.Join(state.Values, ",") != "RJ,SP" {
		t.Fatalf("state values = %#v, want RJ/SP", state.Values)
	}
	category := filters.Controls["category"]
	if category.Value != "health" || category.Operator != "starts_with" {
		t.Fatalf("category filter = %#v, want starts_with health", category)
	}
}

func TestDashboardURLParamsFromFiltersOmitsDefaults(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	if params := report.URLParamsFromFilters(report.DefaultFilters()); len(params) != 0 {
		t.Fatalf("default url params = %#v, want empty", params)
	}

	filters := report.FiltersFromURL(url.Values{
		"state":       {"SP", "RJ"},
		"category":    {"health"},
		"category_op": {"starts_with"},
	})
	params := report.URLParamsFromFilters(filters)

	if got := strings.Join(params["state"].([]string), ","); got != "RJ,SP" {
		t.Fatalf("state params = %q, want RJ,SP", got)
	}
	if got := params["category"]; got != "health" {
		t.Fatalf("category param = %#v, want health", got)
	}
	if got := params["category_op"]; got != "starts_with" {
		t.Fatalf("category_op param = %#v, want starts_with", got)
	}
}

func TestDashboardPageScopedFilters(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	if got := strings.Join(report.PageFilterIDs("chart-pie"), ","); got != "purchase_date,state" {
		t.Fatalf("chart-pie filter ids = %q, want purchase_date,state", got)
	}
	config := report.FiltersForPage("chart-pie")
	if _, ok := config["purchase_date"]; !ok {
		t.Fatal("chart-pie filter config missing purchase_date")
	}
	if _, ok := config["state"]; !ok {
		t.Fatal("chart-pie filter config missing state")
	}
	if _, ok := config["category"]; ok {
		t.Fatalf("chart-pie filter config included off-page category: %#v", config)
	}

	values := url.Values{
		"period":      {"2018"},
		"state":       {"SP"},
		"category":    {"health"},
		"category_op": {"equals"},
	}
	filters := report.FiltersFromURLForPage("chart-pie", values)
	if _, ok := filters.Controls["category"]; ok {
		t.Fatalf("chart-pie URL filters included off-page category: %#v", filters.Controls)
	}
	if got := filters.Controls["purchase_date"].Preset; got != "2018" {
		t.Fatalf("chart-pie period preset = %q, want 2018", got)
	}
	if got := strings.Join(filters.Controls["state"].Values, ","); got != "SP" {
		t.Fatalf("chart-pie state values = %q, want SP", got)
	}

	shape := report.URLParamShapeForPage("chart-pie")
	if _, ok := shape["category"]; ok {
		t.Fatalf("chart-pie URL shape included category: %#v", shape)
	}
	if _, ok := shape["state"]; !ok {
		t.Fatalf("chart-pie URL shape missing state: %#v", shape)
	}
}

func loadOlistModel(t *testing.T) *Model {
	t.Helper()
	model, err := Load(filepath.Join("..", "..", "dashboards", "olist", "model.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func loadOlistMetricViews(t *testing.T, model *Model) map[string]*MetricView {
	t.Helper()
	view, err := LoadMetricView(filepath.Join("..", "..", "dashboards", "olist", "orders.metrics.yaml"), model)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]*MetricView{view.ID: view}
}

func loadOlistDashboard(t *testing.T, model *Model) *Dashboard {
	t.Helper()
	report, err := LoadDashboard(filepath.Join("..", "..", "dashboards", "olist", "executive-sales.yaml"), loadOlistMetricViews(t, model))
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func minimalSourceModel() *Model {
	return &Model{
		Name:              "test",
		Title:             "Test",
		Description:       "Test semantic model",
		DefaultConnection: "local_files",
		Connections: map[string]Connection{
			"local_files": {
				Kind: "local",
			},
		},
		Sources: map[string]Source{
			"orders": {Path: "orders.csv"},
		},
		Tables: map[string]ModelTable{
			"orders": {
				Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
				Dimensions: map[string]MetricDimension{"order_id": {Expr: "order_id"}},
				Measures:   map[string]MetricMeasure{"order_count": {Label: "Orders", Expression: "COUNT(*)"}},
			},
		},
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
	err := report.Validate(loadOlistMetricViews(t, model))
	if err == nil {
		t.Fatalf("Validate() error = nil, want %q", contains)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("Validate() error = %q, want containing %q", err.Error(), contains)
	}
}

func assertCatalogValidateError(t *testing.T, catalog Catalog, baseDir, contains string) {
	t.Helper()
	err := catalog.Validate(baseDir)
	if err == nil {
		t.Fatalf("Validate() error = nil, want %q", contains)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("Validate() error = %q, want containing %q", err.Error(), contains)
	}
}
