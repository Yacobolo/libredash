package semantic

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorkspaceCatalog(t *testing.T) {
	workspace, err := LoadWorkspace(filepath.Join("..", "..", "dashboards", "catalog.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	if len(workspace.Catalog.SemanticModels) != 1 {
		t.Fatalf("model catalog count = %d, want 1", len(workspace.Catalog.SemanticModels))
	}
	if len(workspace.Catalog.Dashboards) != 1 {
		t.Fatalf("dashboard catalog count = %d, want 1", len(workspace.Catalog.Dashboards))
	}
	if _, ok := workspace.Models["olist"]; !ok {
		t.Fatal("workspace missing olist model")
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
		Dashboards: []CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales", SemanticModel: "olist", Path: "olist/executive-sales.yaml"},
		},
	}

	assertCatalogValidateError(t, catalog, baseDir, "duplicate semantic model")
}

func TestCatalogValidateRejectsUnknownDashboardModel(t *testing.T) {
	baseDir := filepath.Join("..", "..", "dashboards")
	catalog := Catalog{
		SemanticModels: []CatalogModel{
			{ID: "olist", Title: "Olist", Path: "olist/model.yaml"},
		},
		Dashboards: []CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales", SemanticModel: "missing", Path: "olist/executive-sales.yaml"},
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
		Dashboards: []CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales", SemanticModel: "olist", Path: "olist/executive-sales.yaml"},
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
	if got := model.Datasets["orders"].Source; got != "orders_enriched" {
		t.Fatalf("orders dataset source = %q, want orders_enriched", got)
	}
	if len(model.Relationships) != 6 {
		t.Fatalf("relationship count = %d, want 6", len(model.Relationships))
	}
}

func TestLoadOlistDashboard(t *testing.T) {
	model := loadOlistModel(t)
	report, err := LoadDashboard(filepath.Join("..", "..", "dashboards", "olist", "executive-sales.yaml"), model)
	if err != nil {
		t.Fatal(err)
	}

	if report.ID != "executive-sales" {
		t.Fatalf("dashboard id = %q, want executive-sales", report.ID)
	}
	if got := report.Visuals["revenue"].Dataset; got != "orders" {
		t.Fatalf("revenue visual dataset = %q, want orders", got)
	}
	if got := report.Visuals["orders"].Type; got != "donut" {
		t.Fatalf("orders visual type = %q, want donut", got)
	}
	if got := report.Visuals["orders_by_month_status"].Query.Series; got != "status" {
		t.Fatalf("multi-series visual series = %q, want status", got)
	}
	if got := report.Tables["orders"].DefaultSort.Key; got != "purchase_date" {
		t.Fatalf("orders table default sort = %q, want purchase_date", got)
	}
	if len(report.Pages) != 2 {
		t.Fatalf("page count = %d, want 2", len(report.Pages))
	}
	if got := report.Pages[1].ID; got != "operations" {
		t.Fatalf("second page id = %q, want operations", got)
	}
	page := report.Pages[0].WithDefaults()
	if page.Grid.Columns != 12 || page.Grid.RowHeight != 48 {
		t.Fatalf("overview grid = %#v, want 12 columns and 48 row height", page.Grid)
	}
	visuals := page.PlacedVisuals()
	if got := visuals[0].Width; got != 1334 {
		t.Fatalf("kpi compiled width = %v, want 1334", got)
	}
	if got := report.Filters["purchase_date"].URLParam; got != "period" {
		t.Fatalf("purchase_date url param = %q, want period", got)
	}
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
	visual.Query.Series = "status"
	report.Visuals["orders"] = visual

	assertDashboardValidateError(t, report, model, "does not support series")
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

func loadOlistDashboard(t *testing.T, model *Model) *Dashboard {
	t.Helper()
	report, err := LoadDashboard(filepath.Join("..", "..", "dashboards", "olist", "executive-sales.yaml"), model)
	if err != nil {
		t.Fatal(err)
	}
	return report
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
	err := report.Validate(model)
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
