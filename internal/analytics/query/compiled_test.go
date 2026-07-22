package query

import (
	"reflect"
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
)

func TestCompileModelBuildsReusableMetricDependencyMetadata(t *testing.T) {
	model := testModel()
	model.Metrics["nested_ratio"] = semanticmodel.Metric{Expression: "${tags_per_order} * 100"}
	model.Measures["net_revenue"] = semanticmodel.MetricMeasure{
		Fact:        "orders",
		Aggregation: "sum",
		Input:       semanticmodel.MeasureInput{Expression: "${orders.revenue} - coalesce(${orders.discount}, 0)"},
		Empty:       "zero",
	}
	orders := model.Tables["orders"]
	orders.Dimensions["discount"] = semanticmodel.MetricDimension{Expr: "discount", Type: "number"}
	model.Tables["orders"] = orders

	compiled, err := CompileModel(model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(compiled.MemberFacts["nested_ratio"], []string{"orders", "tags"}) {
		t.Fatalf("nested metric facts = %#v", compiled.MemberFacts["nested_ratio"])
	}
	if len(compiled.MetricExpressions["tags_per_order"].References()) == 0 {
		t.Fatal("metric expression was not compiled")
	}
	if len(compiled.MeasureInputExpressions["net_revenue"].References()) != 2 {
		t.Fatalf("measure input references = %#v", compiled.MeasureInputExpressions["net_revenue"].References())
	}
}

func TestCompileModelFailsClosedForInvalidMetricDAG(t *testing.T) {
	model := testModel()
	model.Metrics["broken"] = semanticmodel.Metric{Expression: "${missing_member} + 1"}
	if _, err := CompileModel(model); err == nil || !strings.Contains(err.Error(), "unknown aggregate member") {
		t.Fatalf("unknown dependency error = %v", err)
	}

	model = testModel()
	model.Metrics["cycle_a"] = semanticmodel.Metric{Expression: "${cycle_b}"}
	model.Metrics["cycle_b"] = semanticmodel.Metric{Expression: "${cycle_a}"}
	if _, err := CompileModel(model); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}
