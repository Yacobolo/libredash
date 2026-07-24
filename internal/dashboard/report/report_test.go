package report

import (
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
	"gopkg.in/yaml.v3"
)

type fakeMetrics struct {
	report dashboarddefinition.Definition
}

func (m *fakeMetrics) DefaultFilters(string) dashboard.Filters {
	return m.report.DefaultFilters()
}

func (m *fakeMetrics) Report(string) (dashboarddefinition.Definition, *semanticmodel.Model, bool) {
	model := &semanticmodel.Model{Name: "model"}
	return m.report, model, true
}

func TestActivePageResolution(t *testing.T) {
	page, ok := ActivePage(nil, "")
	if !ok || page.ID != "overview" || page.Canvas.Width != 1366 {
		t.Fatalf("default page = %#v, %v", page, ok)
	}

	pages := []dashboard.Page{{ID: "a", Title: "A"}, {ID: "b", Title: "B", Canvas: dashboard.PageCanvas{Width: 900}}}
	page, ok = ActivePage(pages, "b")
	if !ok || page.ID != "b" || page.Canvas.Width != 900 || page.Canvas.Height != 940 {
		t.Fatalf("active page = %#v, %v", page, ok)
	}
	if _, ok := ActivePage(pages, "missing"); ok {
		t.Fatal("missing explicit page resolved")
	}
}

func TestVisualQueryRejectsInlineMeasure(t *testing.T) {
	var query VisualQuery
	err := yaml.Unmarshal([]byte("measures:\n  revenue:\n    expr: SUM(orders.revenue)\n"), &query)
	if err == nil || !strings.Contains(err.Error(), "inline dashboard measures are not supported") {
		t.Fatalf("Unmarshal() error = %v", err)
	}
}

func TestNormalizeFiltersUsesActivePageDefinitions(t *testing.T) {
	stateKey := dashboardfilter.BindingKey("dash", dashboardfilter.ScopePage, "overview", "state")
	categoryKey := dashboardfilter.BindingKey("dash", dashboardfilter.ScopePage, "ops", "category")
	metrics := &fakeMetrics{report: dashboarddefinition.Definition{
		ID: "dash",
		FilterDefinitions: map[string]dashboardfilter.Definition{
			"state":    {ValueKind: dashboardfilter.ValueString},
			"category": {ValueKind: dashboardfilter.ValueString},
		},
		Pages: []dashboard.Page{
			{ID: "overview", FilterBindings: map[string]dashboardfilter.Binding{"state": {
				Key: stateKey, ID: "state", Filter: "state", Scope: dashboardfilter.ScopePage, PageID: "overview",
				Default: dashboardfilter.Expression{Kind: dashboardfilter.ExpressionUnfiltered},
			}}},
			{ID: "ops", FilterBindings: map[string]dashboardfilter.Binding{"category": {
				Key: categoryKey, ID: "category", Filter: "category", Scope: dashboardfilter.ScopePage, PageID: "ops",
				Default: dashboardfilter.Expression{Kind: dashboardfilter.ExpressionUnfiltered},
			}}},
		},
	}}

	filters := NormalizeFilters(metrics, "dash", "overview", dashboard.Filters{})

	if filters.CompiledState == nil {
		t.Fatal("canonical filter state missing")
	}
	if _, ok := filters.CompiledState.AppliedControls[stateKey]; !ok {
		t.Fatalf("state binding missing: %#v", filters.CompiledState)
	}
	if _, ok := filters.CompiledState.AppliedControls[categoryKey]; !ok {
		t.Fatalf("off-page dashboard session binding missing: %#v", filters.CompiledState)
	}
}
