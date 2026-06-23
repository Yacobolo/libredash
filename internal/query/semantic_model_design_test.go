package query

import (
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/semantic"
)

func TestSemanticModelPlannerNamedMeasure(t *testing.T) {
	planner := NewPlanner(testModel())

	plan, err := planner.Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v, want semantic-model query", err)
	}
	if !strings.Contains(plan.SQL, "SUM(t0.revenue) AS revenue") {
		t.Fatalf("plan SQL missing semantic measure:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "LEFT JOIN model.customers") {
		t.Fatalf("plan SQL missing safe related dimension join:\n%s", plan.SQL)
	}
}

func TestSemanticModelPlannerAliasedMeasure(t *testing.T) {
	planner := NewPlanner(testModel())

	plan, err := planner.Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "order_count", Alias: "orders"}},
	})
	if err != nil {
		t.Fatalf("Plan() error = %v, want aliased semantic-model measure", err)
	}
	if !strings.Contains(plan.SQL, "COUNT(DISTINCT t0.order_id) AS orders") {
		t.Fatalf("plan SQL missing aliased order count measure:\n%s", plan.SQL)
	}
}

func TestSemanticModelPlannerRowQueryRequiresTableWithoutMeasures(t *testing.T) {
	planner := NewPlanner(testModel())

	_, err := planner.PlanRows(RowRequest{
		Dimensions: []Field{{Field: "orders.order_id", Alias: "order_id"}},
		Limit:      25,
	})
	if err == nil || !strings.Contains(err.Error(), "row query requires table") {
		t.Fatalf("PlanRows() error = %v, want missing row table rejection", err)
	}
}

func TestSemanticModelPlannerRejectsCrossFactMeasures(t *testing.T) {
	planner := NewPlanner(testModel())

	_, err := planner.Plan(Request{
		Measures: []Field{
			{Field: "revenue", Alias: "revenue"},
			{Field: "refund_amount", Alias: "refund_amount"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cross-fact measures are not supported") {
		t.Fatalf("Plan() error = %v, want cross-fact measure rejection", err)
	}
}

func TestSemanticModelPlannerRejectsUnsafeDimensionPath(t *testing.T) {
	model := testModel()
	model.Sources["items"] = semantic.Source{Path: "items.csv", Format: "csv", Connection: "local"}
	model.Tables["items"] = semantic.ModelTable{
		Kind:       "fact",
		Source:     "items",
		PrimaryKey: "item_id",
		Grain:      "item_id",
		Dimensions: map[string]semantic.MetricDimension{"category": {Expr: "category"}},
	}
	model.Relationships = append(model.Relationships, semantic.Relationship{
		ID: "orders_items", From: "orders.order_id", To: "items.order_id", Cardinality: "one_to_many", Active: true,
	})
	planner := NewPlanner(model)

	_, err := planner.Plan(Request{
		Dimensions: []Field{{Field: "items.category", Alias: "category"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no safe relationship path") {
		t.Fatalf("Plan() error = %v, want unsafe path rejection", err)
	}
}
