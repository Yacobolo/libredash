package definition

import (
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	"github.com/Yacobolo/leapview/internal/visualization/ir"
)

func TestCompiledDashboardOwnsVisualizationsWithoutAuthoringVisualMaps(t *testing.T) {
	spec := ir.VisualizationSpec{Value: &ir.KPIVisualizationSpec{VisualizationSpecBase: ir.VisualizationSpecBase{
		Kind: "kpi", Title: "Orders", Datasets: []ir.VisualizationDatasetSchema{{ID: "primary", Fields: []ir.VisualizationField{{ID: "value", Role: ir.VisualizationFieldRoleMeasure, DataType: ir.VisualizationDataTypeDecimal, Label: "Orders"}}}},
		DataBudget: ir.VisualizationDataBudget{MaxRows: 1, RequiredCompleteness: ir.VisualizationCompletenessComplete}, Accessibility: ir.VisualizationAccessibility{Title: "Orders", Description: "Orders"}, Interactions: []ir.VisualizationInteraction{},
	}, Kind: "kpi", Value: ir.VisualizationFieldRef{Dataset: "primary", Field: "value"}, Presentation: ir.KPIVisualizationPresentation{Trend: ir.VisualizationKPITrendNeutral}}}
	visual, err := visualizationdefinition.New("orders", spec, visualizationdefinition.QueryBinding{Kind: visualizationdefinition.QueryAggregate, ResultShape: visualizationdefinition.ResultScalar, ModelID: "sales", DatasetID: "primary", Aggregate: &visualizationdefinition.AggregateQueryBinding{TableID: "orders", Measures: []visualizationdefinition.FieldBinding{{FieldID: "order_count", Alias: "value"}}, Limit: 1}})
	if err != nil {
		t.Fatal(err)
	}
	pages := []dashboard.Page{{ID: "overview"}}
	compiled, err := New("sales", "Sales", "", "sales", nil, pages, map[string]visualizationdefinition.Definition{"orders": visual})
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Visualizations["orders"].SpecRevision != visual.SpecRevision {
		t.Fatal("compiled dashboard changed with authoring input")
	}
	if compiled.Visualizations["orders"].Query.Aggregate.Measures[0].FieldID != "order_count" {
		t.Fatalf("compiled binding = %#v", compiled.Visualizations["orders"].Query)
	}
}

func TestCompiledDashboardOwnsPageFilterNormalization(t *testing.T) {
	compiled := Definition{
		Filters: map[string]FilterDefinition{
			"region": {Type: "multi_select", Default: FilterDefault{Values: []string{"EU"}}},
			"hidden": {Type: "text", Default: FilterDefault{Value: "ignored"}},
		},
		Pages: []dashboard.Page{{ID: "overview", Visuals: []dashboard.PageVisual{{Kind: "filter", Filter: "region"}}}},
	}
	filters := compiled.NormalizeFiltersForPage("overview", dashboard.Filters{})
	if got := filters.Controls["region"].Values; len(got) != 1 || got[0] != "EU" {
		t.Fatalf("region defaults = %#v", got)
	}
	if _, ok := filters.Controls["hidden"]; ok {
		t.Fatal("normalization retained a filter outside the page scope")
	}
}
