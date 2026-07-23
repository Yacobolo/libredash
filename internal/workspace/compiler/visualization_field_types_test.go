package compiler

import (
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

func TestCompiledKPIFieldRetainsSemanticPresentation(t *testing.T) {
	model := &semanticmodel.Model{Measures: map[string]semanticmodel.MetricMeasure{
		"revenue": {Label: "Revenue", Aggregation: "sum", Unit: "R$", Format: "currency"},
	}}
	authored := reportdef.Visual{Type: "kpi", Query: reportdef.VisualQuery{
		Measures: []reportdef.FieldRef{{Field: "revenue"}},
	}}

	spec, err := compileBuiltInVisualizationSpec("revenue", authored, model)
	if err != nil {
		t.Fatalf("compileBuiltInVisualizationSpec() error = %v", err)
	}
	kpi, ok := spec.Value.(*visualizationir.KPIVisualizationSpec)
	if !ok {
		t.Fatalf("spec = %T, want KPIVisualizationSpec", spec.Value)
	}
	value := kpi.Datasets[0].Fields[1]
	if value.Label != "Revenue" || value.SourceRef == nil || *value.SourceRef != "revenue" {
		t.Fatalf("value semantic identity = %#v, want Revenue sourced from revenue", value)
	}
	if value.Format == nil {
		t.Fatal("value format is nil, want BRL currency")
	}
	format, ok := value.Format.Value.(*visualizationir.CurrencyVisualizationFormat)
	if !ok || format.Currency != "BRL" {
		t.Fatalf("value format = %#v, want BRL currency", value.Format)
	}
}

func TestCompiledCategoricalFieldRetainsStringType(t *testing.T) {
	model := &semanticmodel.Model{
		Tables: map[string]semanticmodel.Table{
			"orders": {Dimensions: map[string]semanticmodel.MetricDimension{"month": {Label: "Month", Type: "string"}}},
		},
		Measures: map[string]semanticmodel.MetricMeasure{"revenue": {Aggregation: "sum", Format: "currency"}},
	}
	authored := reportdef.Visual{Type: "line", Query: reportdef.VisualQuery{
		Dimensions: []reportdef.FieldRef{{Field: "orders.month"}}, Measures: []reportdef.FieldRef{{Field: "revenue"}},
	}}
	spec, err := compileBuiltInVisualizationSpec("revenue", authored, model)
	if err != nil {
		t.Fatal(err)
	}
	chart := spec.Value.(*visualizationir.CartesianVisualizationSpec)
	if got := chart.Datasets[0].Fields[0].DataType; got != visualizationir.VisualizationDataTypeString {
		t.Fatalf("category data type = %q, want string", got)
	}
}

func TestCompiledMultiMeasureValueDoesNotClaimOneMeasureFormat(t *testing.T) {
	model := &semanticmodel.Model{
		Tables: map[string]semanticmodel.Table{"orders": {Dimensions: map[string]semanticmodel.MetricDimension{"month": {Type: "string"}}}},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Aggregation: "sum", Format: "currency"},
			"orders":  {Aggregation: "count", Format: "integer"},
		},
	}
	authored := reportdef.Visual{Type: "combo", Query: reportdef.VisualQuery{
		Dimensions: []reportdef.FieldRef{{Field: "orders.month"}},
		Measures:   []reportdef.FieldRef{{Field: "revenue"}, {Field: "orders"}},
	}}
	spec, err := compileBuiltInVisualizationSpec("summary", authored, model)
	if err != nil {
		t.Fatal(err)
	}
	chart := spec.Value.(*visualizationir.CartesianVisualizationSpec)
	value := chart.Datasets[0].Fields[2]
	if value.ID != "value" || value.SourceRef != nil || value.Format != nil {
		t.Fatalf("heterogeneous value field = %#v, want renderer-neutral unformatted value", value)
	}
}

func TestCompiledDimensionFormatPreservesSemanticScalarTypes(t *testing.T) {
	t.Parallel()
	for semanticType, want := range map[string]string{
		"string": "", "number": "decimal", "boolean": "boolean", "date": "date", "timestamp": "timestamp",
	} {
		if got := compiledDimensionFormat(semanticType); got != want {
			t.Errorf("compiledDimensionFormat(%q) = %q, want %q", semanticType, got, want)
		}
	}
}

func TestCompiledHierarchyRejectsReservedFrameAliases(t *testing.T) {
	t.Parallel()
	authored := reportdef.Visual{Title: "Hierarchy", Type: "tree", Query: reportdef.VisualQuery{
		Dimensions: []reportdef.FieldRef{{Field: "orders.category", Alias: "node"}},
		Measures:   []reportdef.FieldRef{{Field: "order_count", Alias: "value"}},
	}}
	_, err := compileBuiltInVisualizationSpec("hierarchy", authored, nil)
	if err == nil || !strings.Contains(err.Error(), `alias "node" conflicts with a reserved frame field`) {
		t.Fatalf("compileBuiltInVisualizationSpec() error = %v", err)
	}
}

func TestCompiledHierarchyFrameBudgetAccountsForMaterializedAncestors(t *testing.T) {
	t.Parallel()

	authored := reportdef.Visual{Title: "Hierarchy", Type: "treemap", Query: reportdef.VisualQuery{
		Dimensions: []reportdef.FieldRef{{Field: "orders.category", Alias: "category"}, {Field: "orders.status", Alias: "status"}},
		Measures:   []reportdef.FieldRef{{Field: "order_count", Alias: "order_count"}},
		Limit:      80,
	}}
	spec, err := compileBuiltInVisualizationSpec("hierarchy", authored, nil)
	if err != nil {
		t.Fatal(err)
	}
	base, err := visualizationir.SpecificationBase(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := base.DataBudget.MaxRows, int64(160); got != want {
		t.Fatalf("hierarchy frame budget = %d, want %d", got, want)
	}
}

func TestCompiledMultiMeasureFrameBudgetAccountsForNormalizedSeriesRows(t *testing.T) {
	t.Parallel()

	authored := reportdef.Visual{Title: "Revenue and orders", Type: "combo", Query: reportdef.VisualQuery{
		Dimensions: []reportdef.FieldRef{{Field: "orders.month", Alias: "month"}},
		Measures: []reportdef.FieldRef{
			{Field: "revenue", Alias: "revenue"},
			{Field: "order_count", Alias: "order_count"},
		},
		Limit: 30,
	}}
	spec, err := compileBuiltInVisualizationSpec("combo", authored, nil)
	if err != nil {
		t.Fatal(err)
	}
	base, err := visualizationir.SpecificationBase(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := base.DataBudget.MaxRows, int64(60); got != want {
		t.Fatalf("multi-measure frame budget = %d, want %d", got, want)
	}
}

func TestCompiledPhysicalFieldFormatUsesMeasureSemanticsWhenModelTypeIsUnknown(t *testing.T) {
	model := &semanticmodel.Model{Measures: map[string]semanticmodel.MetricMeasure{
		"revenue": {Input: semanticmodel.MeasureInput{Field: "orders.revenue"}, Aggregation: "sum", Format: "currency"},
	}}
	if got := compiledPhysicalFieldFormat(model, "orders.revenue", ""); got != "currency" {
		t.Fatalf("compiledPhysicalFieldFormat = %q, want currency", got)
	}
}

func TestCompiledPhysicalFieldFormatDoesNotTreatCountIdentityAsNumeric(t *testing.T) {
	model := &semanticmodel.Model{Measures: map[string]semanticmodel.MetricMeasure{
		"orders": {Input: semanticmodel.MeasureInput{Field: "orders.order_id"}, Aggregation: "count_distinct"},
	}}
	if got := compiledPhysicalFieldFormat(model, "orders.order_id", ""); got != "" {
		t.Fatalf("compiledPhysicalFieldFormat = %q, want string default", got)
	}
}
