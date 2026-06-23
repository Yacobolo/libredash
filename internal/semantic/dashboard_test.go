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

func TestLoadOlistDashboard(t *testing.T) {
	model := loadOlistModel(t)
	report, err := LoadDashboard(filepath.Join("..", "..", "dashboards", "olist", "executive-sales.yaml"), map[string]*Model{"olist": model})
	if err != nil {
		t.Fatal(err)
	}

	if report.ID != "executive-sales" {
		t.Fatalf("dashboard id = %q, want executive-sales", report.ID)
	}
	if got := report.SemanticModel; got != "olist" {
		t.Fatalf("dashboard semantic model = %q, want olist", got)
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
	if len(report.Tables["category_status_pivot_heat"].MeasureFormatting["order_count"]) == 0 {
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

func TestDataTableFieldsNormalizeToDataColumns(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	table := report.Tables["orders"]
	table.Query.Columns = nil
	table.Query.Fields = []string{"orders.order_id", "revenue"}
	table.Columns = nil
	report.Tables["orders"] = table

	if err := report.Validate(map[string]*Model{"olist": model}); err != nil {
		t.Fatal(err)
	}
	normalized := report.Tables["orders"]
	if got := len(normalized.DataColumns); got != 2 {
		t.Fatalf("data columns = %d, want 2", got)
	}
	if got := normalized.DataColumns[0].Alias; got != "order_id" {
		t.Fatalf("field alias = %q, want order_id", got)
	}
	if got := normalized.DataColumns[1].Field; got != "revenue" {
		t.Fatalf("measure field = %q, want revenue", got)
	}
	if got := len(normalized.Columns); got != 2 {
		t.Fatalf("presentation columns = %d, want generated columns", got)
	}
}

func TestDataTableColumnsOverrideFields(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	table := report.Tables["orders"]
	table.Query.Fields = []string{"missing.field"}
	table.Query.Columns = []FieldRef{{Field: "orders.order_id", Alias: "order"}}
	table.Columns = []dashboard.TableColumn{{Key: "order", Label: "Order"}}
	report.Tables["orders"] = table

	if err := report.Validate(map[string]*Model{"olist": model}); err != nil {
		t.Fatal(err)
	}
	if got := report.Tables["orders"].DataColumns[0].Alias; got != "order" {
		t.Fatalf("query column alias = %q, want order", got)
	}
}

func TestDataTableRejectsDuplicateOutputAliases(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	table := report.Tables["orders"]
	table.Query.Columns = nil
	table.Query.Fields = []string{"orders.customer_id", "customers.customer_id"}
	table.Columns = nil
	report.Tables["orders"] = table

	assertDashboardValidateError(t, report, model, "duplicate query output alias")
}

func TestGlobalFilterSkipsIncompatibleTarget(t *testing.T) {
	model := loadOlistModel(t)
	addIsolatedProductTable(model)
	report := loadOlistDashboard(t, model)
	filter := FilterDefinition{
		Type:            "text",
		Label:           "Product",
		Dimension:       "products.category",
		DefaultOperator: "contains",
		Operators:       []string{"contains"},
		URLParam:        "product",
	}
	report.Filters["product"] = filter

	if err := report.Validate(map[string]*Model{"olist": model}); err != nil {
		t.Fatal(err)
	}
	applies, err := report.FilterAppliesToTarget(model, filter, "visual", "revenue")
	if err != nil {
		t.Fatal(err)
	}
	if applies {
		t.Fatal("global incompatible product filter applied to orders visual")
	}
}

func TestExplicitFilterTargetRejectsIncompatibleTarget(t *testing.T) {
	model := loadOlistModel(t)
	addIsolatedProductTable(model)
	report := loadOlistDashboard(t, model)
	report.Filters["product"] = FilterDefinition{
		Type:            "text",
		Label:           "Product",
		Dimension:       "products.category",
		DefaultOperator: "contains",
		Operators:       []string{"contains"},
		URLParam:        "product",
		Targets:         InteractionTargets{Visuals: []string{"revenue"}},
	}

	assertDashboardValidateError(t, report, model, "cannot apply to visual")
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

	if err := report.Validate(map[string]*Model{"olist": model}); err != nil {
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

func TestLoadDashboardRejectsLegacyMetricViewsKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.yaml")
	content := `id: executive-sales
title: Executive Sales Dashboard
metrics_views: [orders]
filters: {}
visuals: {}
tables: {}
pages: []
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadDashboard(path, nil)
	if err == nil || !strings.Contains(err.Error(), "legacy metrics_views") {
		t.Fatalf("LoadDashboard error = %v, want legacy metrics_views rejection", err)
	}
}

func TestLoadDashboardRejectsLegacyTopLevelStacked(t *testing.T) {
	model := loadOlistModel(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.yaml")
	content := strings.ReplaceAll(`id: executive-sales
title: Executive Sales Dashboard
semantic_model: olist
filters: {}
visuals:
  total_orders:
    kind: kpi
    shape: single_value
    query:
      measures: {order_count:}
  revenue:
    title: Revenue
    type: area
    stacked: true
    query:
      dimensions: {purchase_month: orders.purchase_month}
      measures: {revenue:}
tables:
  orders:
    title: Orders
    query:
      table: orders
      fields: [orders.order_id]
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
	_, err := LoadDashboard(path, map[string]*Model{"olist": model})
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
	_, err := LoadDashboard(path, map[string]*Model{"olist": model})
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
			Title:    "Heatmap",
			Shape:    "matrix",
			Renderer: "echarts",
			Type:     "heatmap",
			Query:    VisualQuery{Dimensions: fieldRefs("state", "status"), Measures: measureRefs("order_count")},
		},
		"sankey": {
			Title:    "Flow",
			Shape:    "graph",
			Renderer: "echarts",
			Type:     "sankey",
			Query:    VisualQuery{Dimensions: fieldRefs("status", "delivery_bucket"), Measures: measureRefs("order_count")},
		},
		"geo": {
			Title:    "Map",
			Shape:    "geo",
			Renderer: "echarts",
			Type:     "map",
			Options:  map[string]any{"map": "brazil_states"},
			Query:    VisualQuery{Dimensions: fieldRefs("state"), Measures: measureRefs("order_count")},
		},
		"boxplot": {
			Title:    "Distribution",
			Shape:    "distribution",
			Renderer: "echarts",
			Type:     "boxplot",
			Query:    VisualQuery{Dimensions: fieldRefs("delivery_bucket"), Measures: measureRefs("delivery_days")},
		},
		"combo": {
			Title:    "Combo",
			Shape:    "category_multi_measure",
			Renderer: "echarts",
			Type:     "combo",
			Query:    VisualQuery{Dimensions: fieldRefs("purchase_month"), Measures: measureRefs("revenue", "order_count")},
		},
		"waterfall": {
			Title:    "Waterfall",
			Shape:    "category_delta",
			Renderer: "echarts",
			Type:     "waterfall",
			Query:    VisualQuery{Dimensions: fieldRefs("purchase_month"), Measures: measureRefs("revenue")},
		},
		"histogram": {
			Title:    "Histogram",
			Shape:    "binned_measure",
			Renderer: "echarts",
			Type:     "histogram",
			Query:    VisualQuery{Measures: measureRefs("delivery_days")},
		},
		"radar": {
			Title:    "Radar",
			Shape:    "category_value",
			Renderer: "echarts",
			Type:     "radar",
			Query:    VisualQuery{Dimensions: fieldRefs("status"), Measures: measureRefs("order_count")},
		},
		"tree": {
			Title:    "Tree",
			Shape:    "hierarchy",
			Renderer: "echarts",
			Type:     "tree",
			Query:    VisualQuery{Dimensions: fieldRefs("state", "status"), Measures: measureRefs("order_count")},
		},
		"sunburst": {
			Title:    "Sunburst",
			Shape:    "hierarchy",
			Renderer: "echarts",
			Type:     "sunburst",
			Query:    VisualQuery{Dimensions: fieldRefs("category", "status"), Measures: measureRefs("order_count")},
		},
	}
	for name, visual := range cases {
		report.Visuals = map[string]Visual{name: visual}
		report.Pages = []dashboard.Page{{ID: "overview", Title: "Overview", Visuals: []dashboard.PageVisual{{ID: name, Kind: visual.Type + "_chart", Visual: name, Placement: dashboard.PagePlacement{Col: 1, Row: 1, ColSpan: 1, RowSpan: 1}}}}}
		if err := report.Validate(map[string]*Model{"olist": model}); err != nil {
			t.Fatalf("validate advanced shape %s: %v", name, err)
		}
	}
}

func TestDashboardValidateRejectsAdvancedShapeMismatch(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	visual := report.Visuals["revenue_orders_combo"]
	visual.Query.Measures = measureRefs("revenue")
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

func TestDashboardValidateRejectsDuplicateDatePreset(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["purchase_date"]
	filter.Presets = append(filter.Presets, filter.Presets[0])
	report.Filters["purchase_date"] = filter

	assertDashboardValidateError(t, report, model, "duplicate preset")
}

func TestDashboardValidateRejectsNegativeRelativeDatePreset(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["purchase_date"]
	filter.Presets = append(filter.Presets, FilterPreset{Value: "future", Label: "Future", RelativeDays: -7})
	report.Filters["purchase_date"] = filter

	assertDashboardValidateError(t, report, model, "negative relative_days")
}

func TestDashboardValidateRejectsUnknownDefaultDatePreset(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["purchase_date"]
	filter.Default.Preset = "missing"
	report.Filters["purchase_date"] = filter

	assertDashboardValidateError(t, report, model, "default preset")
}

func TestDashboardValidateRejectsUnsupportedFilterValuesSource(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["state"]
	filter.Values.Source = "static"
	report.Filters["state"] = filter

	assertDashboardValidateError(t, report, model, "unsupported values.source")
}

func TestDashboardValidateRejectsTextOperatorURLParamWithoutValueParam(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["category"]
	filter.URLParam = ""
	filter.OperatorURLParam = "category_op"
	report.Filters["category"] = filter

	assertDashboardValidateError(t, report, model, "operator_url_param requires url_param")
}

func TestDashboardValidateRejectsDefaultTextOperatorOutsideAllowedSet(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["category"]
	filter.DefaultOperator = "starts_with"
	filter.Operators = []string{"contains", "equals"}
	report.Filters["category"] = filter

	assertDashboardValidateError(t, report, model, "default_operator")
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

func TestDashboardValidateRejectsDuplicateFilterURLParamWithinFilter(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	filter := report.Filters["category"]
	filter.OperatorURLParam = filter.URLParam
	report.Filters["category"] = filter

	assertDashboardValidateError(t, report, model, "duplicates")
}

func TestDashboardValidateRejectsDataTableWithoutQueryTable(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)
	table := report.Tables["orders"]
	table.Query.Table = ""
	report.Tables["orders"] = table

	assertDashboardValidateError(t, report, model, "kind data_table requires query.table")
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

func TestDashboardFiltersFromURLDateRangeForcesCustomPreset(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	filters := report.FiltersFromURL(url.Values{
		"from": {"2018-01-01"},
	})

	date := filters.Controls["purchase_date"]
	if date.Preset != "custom" || date.From != "2018-01-01" {
		t.Fatalf("date filter = %#v, want custom preset from URL bound", date)
	}
}

func TestDashboardFiltersFromURLIgnoresUnsupportedTextOperator(t *testing.T) {
	model := loadOlistModel(t)
	report := loadOlistDashboard(t, model)

	filters := report.FiltersFromURL(url.Values{
		"category":    {"health"},
		"category_op": {"unsupported"},
	})

	category := filters.Controls["category"]
	if category.Value != "health" {
		t.Fatalf("category value = %q, want health", category.Value)
	}
	if category.Operator != "contains" {
		t.Fatalf("category operator = %q, want default contains", category.Operator)
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
