package report

import (
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	"gopkg.in/yaml.v3"
)

type fakeMetrics struct {
	report Dashboard
}

func (m *fakeMetrics) DefaultFilters(string) dashboard.Filters {
	return dashboard.Filters{Controls: map[string]dashboard.FilterControl{
		"state": {Type: "multi_select", Operator: "in"},
	}}
}

func (m *fakeMetrics) Report(string) (dashboarddefinition.Definition, *semanticmodel.Model, bool) {
	model := &semanticmodel.Model{Name: "model"}
	filters := map[string]dashboarddefinition.FilterDefinition{}
	for id, filter := range m.report.Filters {
		filters[id] = dashboarddefinition.FilterDefinition{Type: filter.Type, Label: filter.Label, Dimension: filter.Dimension}
	}
	visualizations := map[string]visualizationdefinition.Definition{}
	for id, authored := range m.report.Visuals {
		if authored.Tabular == nil {
			continue
		}
		table := *authored.Tabular
		fields := make([]visualizationdefinition.FieldBinding, len(table.DataColumns))
		if len(fields) == 0 {
			fields = []visualizationdefinition.FieldBinding{{FieldID: "value", Alias: "value"}}
		}
		visualizations[id] = visualizationdefinition.Definition{ID: id, Query: visualizationdefinition.QueryBinding{Kind: visualizationdefinition.QueryDetail, ResultShape: visualizationdefinition.ResultDetailWindow, Detail: &visualizationdefinition.DetailQueryBinding{Fields: fields, DefaultSort: []visualizationdefinition.Sort{{FieldID: table.DefaultSort.Key, Direction: table.DefaultSort.Direction}}, Limit: 100}}}
	}
	return dashboarddefinition.Definition{ID: m.report.ID, Title: m.report.Title, SemanticModel: "model", Filters: filters, Pages: m.report.Pages, Visualizations: visualizations}, model, true
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
	metrics := &fakeMetrics{report: Dashboard{
		Filters: map[string]FilterDefinition{
			"state":    {Type: "multi_select", Label: "State", URLParam: "state", Operator: "in"},
			"category": {Type: "text", Label: "Category", URLParam: "category", DefaultOperator: "contains"},
		},
		Pages: []dashboard.Page{
			{ID: "overview", Visuals: []dashboard.PageVisual{{Kind: "filter_card", Filter: "state"}}},
			{ID: "ops", Visuals: []dashboard.PageVisual{{Kind: "filter_card", Filter: "category"}}},
		},
	}}

	filters := NormalizeFilters(metrics, "dash", "overview", dashboard.Filters{Controls: map[string]dashboard.FilterControl{
		"state":    {Type: "multi_select", Operator: "in", Values: []string{"SP"}},
		"category": {Type: "text", Operator: "contains", Value: "ignored"},
	}})

	if _, ok := filters.Controls["state"]; !ok {
		t.Fatalf("state filter missing: %#v", filters.Controls)
	}
	if _, ok := filters.Controls["category"]; ok {
		t.Fatalf("off-page category filter kept: %#v", filters.Controls)
	}
}
