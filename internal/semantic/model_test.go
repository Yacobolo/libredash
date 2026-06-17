package semantic

import (
	"net/url"
	"os"
	"path/filepath"
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
	if got := model.Sources["orders"].Location; got != "olist_orders_dataset.csv" {
		t.Fatalf("orders source location = %q, want olist_orders_dataset.csv", got)
	}
	if got := model.Datasets["orders"].Source; got != "orders_enriched" {
		t.Fatalf("orders dataset source = %q, want orders_enriched", got)
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
			Auth:  ConnectionAuth{Method: "credential_chain", Profile: "analytics"},
		},
		"azure_lake": {
			Kind: "azure_blob",
			Auth: ConnectionAuth{Method: "credential_chain", Account: "mystorageaccount"},
		},
		"crm": {
			Kind:   "postgres",
			Secret: "crm_readonly",
		},
	}
	model.Sources = map[string]Source{
		"orders": {
			Location: "olist_orders_dataset.csv",
		},
		"sales_events": {
			Format:     "parquet",
			Location:   "events/*",
			Connection: "prod_lake",
		},
		"delta_orders": {
			Format:     "delta",
			Location:   "az://warehouse/tables/orders",
			Connection: "azure_lake",
		},
		"crm_accounts": {
			Connection: "crm",
			Object:     "public.accounts",
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

func TestModelValidateInfersFileFormats(t *testing.T) {
	cases := map[string]struct {
		location string
		want     string
	}{
		"csv":     {location: "orders.csv", want: "csv"},
		"csv_gz":  {location: "orders.csv.gz", want: "csv"},
		"json":    {location: "orders.json", want: "json"},
		"jsonl":   {location: "orders.jsonl", want: "json"},
		"ndjson":  {location: "orders.ndjson", want: "json"},
		"parquet": {location: "orders.parquet", want: "parquet"},
		"excel":   {location: "orders.xlsx", want: "excel"},
		"text":    {location: "orders.txt", want: "text"},
		"blob":    {location: "orders.blob", want: "blob"},
		"vortex":  {location: "orders.vortex", want: "vortex"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := minimalSourceModel()
			model.Connections["local_files"] = Connection{Kind: "local"}
			model.Sources = map[string]Source{"orders": {Location: tc.location}}
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
			contains: "exactly one of location or object",
		},
		"multiple_source_shapes": {
			source:   Source{Location: "orders.csv", Object: "public.orders"},
			contains: "exactly one of location or object",
		},
		"location_bad_format": {
			source:   Source{Format: "orc", Location: "orders.orc", Connection: "local_files"},
			contains: "unsupported format",
		},
		"ambiguous_location_missing_format": {
			source:   Source{Location: "events/*", Connection: "local_files"},
			contains: "requires format",
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
			source:   Source{Format: "parquet", Location: "s3://bucket/*.parquet", Connection: "missing"},
			contains: "unknown connection",
		},
		"bad_source_option_key": {
			source:   Source{Format: "csv", Location: "orders.csv", Connection: "local_files", Options: map[string]any{"bad-key": true}},
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
				"crm":         {Kind: "mysql"},
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
			connection: Connection{Kind: "s3", Auth: ConnectionAuth{Method: "shell"}},
			contains:   "unsupported auth method",
		},
		"bad_auth_param": {
			connection: Connection{Kind: "s3", Auth: ConnectionAuth{Method: "config", Params: map[string]any{"bad-key": "value"}}},
			contains:   "auth param",
		},
		"bad_attach_option": {
			connection: Connection{Kind: "postgres", Options: map[string]any{"password": "secret"}},
			contains:   "unsupported option",
		},
		"bad_secret_name": {
			connection: Connection{Kind: "postgres", Secret: "bad-secret"},
			contains:   "secret",
		},
		"bad_default_option": {
			connection: Connection{Kind: "local", Defaults: ConnectionDefaults{Options: map[string]any{"bad-key": true}}},
			contains:   "default option",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := minimalSourceModel()
			model.Connections = map[string]Connection{"crm": tc.connection}
			model.Sources = map[string]Source{
				"orders": {Format: "csv", Location: "orders.csv", Connection: "crm"},
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
    location: orders.csv
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
		"source_query": `
sources:
  orders:
    query: SELECT 1 AS id
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
    location: products.csv
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
	if got := report.Visuals["orders_by_month_status"].Query.Series; got != "status" {
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
	if got := report.Tables["category_status_pivot"].ColumnDims[0]; got != "status" {
		t.Fatalf("category_status_pivot column dimension = %q, want status", got)
	}
	if len(report.Pages) != 3 {
		t.Fatalf("page count = %d, want 3", len(report.Pages))
	}
	if got := report.Pages[1].ID; got != "operations" {
		t.Fatalf("second page id = %q, want operations", got)
	}
	if got := report.Pages[2].ID; got != "tables" {
		t.Fatalf("third page id = %q, want tables", got)
	}
	tablePage := report.Pages[2].WithDefaults()
	tableVisualCount := 0
	for _, visual := range tablePage.PlacedVisuals() {
		if visual.Kind == "table" {
			tableVisualCount++
		}
	}
	if tableVisualCount != 3 {
		t.Fatalf("tables page table visual count = %d, want 3", tableVisualCount)
	}
	page := report.Pages[0].WithDefaults()
	if page.Grid.Columns != 12 || page.Grid.RowHeight != 48 {
		t.Fatalf("overview grid = %#v, want 12 columns and 48 row height", page.Grid)
	}
	visuals := page.PlacedVisuals()
	if visuals[0].Kind != "filter_card" || visuals[0].Filter != "purchase_date" {
		t.Fatalf("first page visual = %#v, want purchase_date filter card", visuals[0])
	}
	if got := visuals[2].Width; got != 1334 {
		t.Fatalf("kpi compiled width = %v, want 1334", got)
	}
	if got := report.Filters["purchase_date"].URLParam; got != "period" {
		t.Fatalf("purchase_date url param = %q, want period", got)
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
kpis:
  total_orders:
    title: Orders
    metrics_view: orders
    measure: order_count
visuals:
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
			Query:      VisualQuery{Dimensions: []string{"state", "status"}, Measures: []string{"order_count"}},
		},
		"sankey": {
			Title:      "Flow",
			Shape:      "graph",
			Renderer:   "echarts",
			Type:       "sankey",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: []string{"status", "delivery_bucket"}, Measures: []string{"order_count"}},
		},
		"geo": {
			Title:      "Map",
			Shape:      "geo",
			Renderer:   "echarts",
			Type:       "map",
			MetricView: "orders",
			Options:    map[string]any{"map": "brazil_states"},
			Query:      VisualQuery{Dimensions: []string{"state"}, Measures: []string{"order_count"}},
		},
		"boxplot": {
			Title:      "Distribution",
			Shape:      "distribution",
			Renderer:   "echarts",
			Type:       "boxplot",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: []string{"delivery_bucket"}, Measures: []string{"delivery_days"}},
		},
		"combo": {
			Title:      "Combo",
			Shape:      "category_multi_measure",
			Renderer:   "echarts",
			Type:       "combo",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: []string{"purchase_month"}, Measures: []string{"revenue", "order_count"}},
		},
		"waterfall": {
			Title:      "Waterfall",
			Shape:      "category_delta",
			Renderer:   "echarts",
			Type:       "waterfall",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: []string{"purchase_month"}, Measures: []string{"revenue"}},
		},
		"histogram": {
			Title:      "Histogram",
			Shape:      "binned_measure",
			Renderer:   "echarts",
			Type:       "histogram",
			MetricView: "orders",
			Query:      VisualQuery{Measures: []string{"delivery_days"}},
		},
		"radar": {
			Title:      "Radar",
			Shape:      "category_value",
			Renderer:   "echarts",
			Type:       "radar",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: []string{"status"}, Measures: []string{"order_count"}},
		},
		"tree": {
			Title:      "Tree",
			Shape:      "hierarchy",
			Renderer:   "echarts",
			Type:       "tree",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: []string{"state", "status"}, Measures: []string{"order_count"}},
		},
		"sunburst": {
			Title:      "Sunburst",
			Shape:      "hierarchy",
			Renderer:   "echarts",
			Type:       "sunburst",
			MetricView: "orders",
			Query:      VisualQuery{Dimensions: []string{"category", "status"}, Measures: []string{"order_count"}},
		},
	}
	for name, visual := range cases {
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
	visual.Query.Measures = []string{"revenue"}
	report.Visuals["revenue_orders_combo"] = visual
	assertDashboardValidateError(t, report, model, "at least two query measures")

	report = loadOlistDashboard(t, model)
	visual = report.Visuals["delivery_histogram"]
	visual.Query.Dimensions = []string{"status"}
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

func TestValidateRejectsUnknownDatasetSource(t *testing.T) {
	model := loadOlistModel(t)
	dataset := model.Datasets["orders"]
	dataset.Source = "missing_cache"
	model.Datasets["orders"] = dataset

	assertModelValidateError(t, model, "unknown cache table")
}

func TestDashboardValidateRejectsUnknownVisualDimension(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	visual := report.Visuals["revenue"]
	visual.Query.Dimensions = []string{"missing_dimension"}
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
	visual.Query.Series = "status"
	report.Visuals["orders"] = visual

	assertDashboardValidateError(t, report, model, "does not support series")
}

func TestDashboardValidateRejectsInvalidTableVariant(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	table := report.Tables["state_status_matrix"]
	table.Rows = []string{"missing_dimension"}
	report.Tables["state_status_matrix"] = table
	assertDashboardValidateError(t, report, model, "unknown dimension")

	report = loadOlistDashboard(t, model)
	table = report.Tables["category_status_pivot"]
	table.ColumnDims = []string{"missing_dimension"}
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
			"orders": {Location: "orders.csv"},
		},
		Cache: Cache{Tables: map[string]CacheTable{
			"orders_cache": {SQL: "SELECT * FROM raw.orders"},
		}},
		Datasets: map[string]Dataset{
			"orders": {Source: "orders_cache"},
		},
	}
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
