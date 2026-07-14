package consumer

import (
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

func TestOptimizerGroupsSemanticConsumersWithoutPresentationShapes(t *testing.T) {
	model := optimizerTestModel()
	scope := []dataquery.Filter{{Field: "segment", Operator: "equals", Values: []any{"consumer"}}}
	plan, err := Optimize(model, []LogicalQuery{
		{
			Target: Target{Kind: KindVisual, ID: "trend"},
			Query:  dataquery.Query{Kind: dataquery.KindSemanticAggregate, Fields: []dataquery.Field{{Field: "customer", Alias: "label"}}, Measures: []dataquery.Field{{Field: "order_count", Alias: "orders"}, {Field: "tag_count", Alias: "tags"}}, Filters: scope, Limit: 500},
		},
		{
			Target: Target{Kind: KindVisual, ID: "ratio"},
			Query:  dataquery.Query{Kind: dataquery.KindSemanticAggregate, Measures: []dataquery.Field{{Field: "tags_per_order", Alias: "value"}}, Filters: scope},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Jobs) != 1 || plan.Jobs[0].Strategy != StrategyBundle {
		t.Fatalf("plan = %#v, want one semantic bundle", plan)
	}
	if got := []string{plan.Jobs[0].Queries[0].Target.ID, plan.Jobs[0].Queries[1].Target.ID}; got[0] != "trend" || got[1] != "ratio" {
		t.Fatalf("authored consumer order = %#v", got)
	}
}

func TestOptimizerKeepsDifferentGovernedScopesSeparate(t *testing.T) {
	model := optimizerTestModel()
	plan, err := Optimize(model, []LogicalQuery{
		{Target: Target{Kind: KindVisual, ID: "consumer"}, Query: dataquery.Query{Kind: dataquery.KindSemanticAggregate, Measures: []dataquery.Field{{Field: "order_count"}}, Filters: []dataquery.Filter{{Field: "segment", Operator: "equals", Values: []any{"consumer"}}}}},
		{Target: Target{Kind: KindVisual, ID: "business"}, Query: dataquery.Query{Kind: dataquery.KindSemanticAggregate, Measures: []dataquery.Field{{Field: "order_count"}}, Filters: []dataquery.Filter{{Field: "segment", Operator: "equals", Values: []any{"business"}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Jobs) != 2 {
		t.Fatalf("jobs = %#v, want separate governed scopes", plan.Jobs)
	}
}

func TestOptimizerBatchesScalarConsumersAcrossFacts(t *testing.T) {
	plan, err := Optimize(optimizerTestModel(), []LogicalQuery{
		{Target: Target{Kind: KindVisual, ID: "orders"}, Query: dataquery.Query{Kind: dataquery.KindSemanticAggregate, Measures: []dataquery.Field{{Field: "order_count"}}}},
		{Target: Target{Kind: KindVisual, ID: "tags"}, Query: dataquery.Query{Kind: dataquery.KindSemanticAggregate, Measures: []dataquery.Field{{Field: "tag_count"}}}},
		{Target: Target{Kind: KindVisual, ID: "ratio"}, Query: dataquery.Query{Kind: dataquery.KindSemanticAggregate, Measures: []dataquery.Field{{Field: "tags_per_order"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Jobs) != 1 || plan.Jobs[0].Strategy != StrategyBatch || len(plan.Jobs[0].Queries) != 3 {
		t.Fatalf("cross-fact scalar plan = %#v", plan)
	}
}

func optimizerTestModel() *semanticmodel.Model {
	return &semanticmodel.Model{
		Name: "commerce",
		Tables: map[string]semanticmodel.Table{
			"orders": {Dimensions: map[string]semanticmodel.MetricDimension{"customer": {Field: "orders.customer_id", Table: "orders", Name: "customer"}, "segment": {Field: "orders.segment", Table: "orders", Name: "segment"}}},
			"tags":   {Dimensions: map[string]semanticmodel.MetricDimension{"customer": {Field: "tags.customer_id", Table: "tags", Name: "customer"}, "segment": {Field: "tags.segment", Table: "tags", Name: "segment"}}},
		},
		Dimensions: map[string]semanticmodel.SemanticDimension{
			"customer": {Type: "string", Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.customer_id"}, "tags": {Field: "tags.customer_id"}}},
			"segment":  {Type: "string", Bindings: map[string]semanticmodel.DimensionBinding{"orders": {Field: "orders.segment"}, "tags": {Field: "tags.segment"}}},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"order_count": {Fact: "orders", Aggregation: "count", Empty: "zero"},
			"tag_count":   {Fact: "tags", Aggregation: "count", Empty: "zero"},
		},
		Metrics: map[string]semanticmodel.Metric{
			"tags_per_order": {Expression: "safe_divide(${tag_count}, ${order_count})"},
		},
	}
}
