package query

import (
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/semantic"
)

func TestPlannerSingleTableMeasure(t *testing.T) {
	plan, err := NewPlanner(testModel()).Plan(Request{
		Measures: []Field{{Field: "revenue", Alias: "value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "SUM(t0.revenue) AS value") {
		t.Fatalf("plan SQL missing measure expression:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "FROM model.orders t0") {
		t.Fatalf("plan SQL missing base table:\n%s", plan.SQL)
	}
}

func TestPlannerSafeManyToOneDimensionJoin(t *testing.T) {
	plan, err := NewPlanner(testModel()).Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "LEFT JOIN model.customers t1 ON t0.customer_id = t1.customer_id") {
		t.Fatalf("plan SQL missing customer join:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "t1.customer_state AS state") {
		t.Fatalf("plan SQL missing related dimension:\n%s", plan.SQL)
	}
}

func TestPlannerRelationshipJoinUsesSemanticEndpointExpression(t *testing.T) {
	model := testModel()
	orders := model.Tables["orders"]
	orders.Dimensions["customer_id"] = semantic.MetricDimension{Expr: "raw_customer_id"}
	model.Tables["orders"] = orders

	plan, err := NewPlanner(model).Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "LEFT JOIN model.customers t1 ON t0.raw_customer_id = t1.customer_id") {
		t.Fatalf("plan SQL should join through semantic endpoint expression:\n%s", plan.SQL)
	}
}

func TestPlannerRelationshipJoinFallsBackToPhysicalPrimaryKey(t *testing.T) {
	model := testModel()
	customers := model.Tables["customers"]
	delete(customers.Dimensions, "customer_id")
	model.Tables["customers"] = customers

	plan, err := NewPlanner(model).Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "LEFT JOIN model.customers t1 ON t0.customer_id = t1.customer_id") {
		t.Fatalf("plan SQL should join through physical primary key fallback:\n%s", plan.SQL)
	}
}

func TestPlannerTimeGrain(t *testing.T) {
	plan, err := NewPlanner(testModel()).Plan(Request{
		Time:     Time{Field: "orders.purchase_timestamp", Grain: "month", Alias: "month"},
		Measures: []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "date_trunc('month', t0.purchase_timestamp) AS month") {
		t.Fatalf("plan SQL missing date_trunc:\n%s", plan.SQL)
	}
}

func TestPlannerFilters(t *testing.T) {
	plan, err := NewPlanner(testModel()).Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
		Filters:    []Filter{{Field: "customers.state", Operator: "in", Values: []any{"SP", "RJ"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "t1.customer_state IN (?, ?)") {
		t.Fatalf("plan SQL missing semantic filter:\n%s", plan.SQL)
	}
	if len(plan.Args) != 2 {
		t.Fatalf("args = %#v, want two filter args", plan.Args)
	}
}

func TestPlannerRowQueryWithRelatedDimension(t *testing.T) {
	plan, err := NewPlanner(testModel()).PlanRows(RowRequest{
		Table: "orders",
		Dimensions: []Field{
			{Field: "orders.order_id", Alias: "order_id"},
			{Field: "customers.state", Alias: "state"},
		},
		Measures: []Field{{Field: "revenue", Alias: "revenue"}},
		Sort:     []Sort{{Field: "order_id", Direction: "asc"}},
		Limit:    25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "LEFT JOIN model.customers t1 ON t0.customer_id = t1.customer_id") {
		t.Fatalf("row plan SQL missing customer join:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "t0.order_id AS order_id") || !strings.Contains(plan.SQL, "t1.customer_state AS state") {
		t.Fatalf("row plan SQL missing selected dimensions:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "t0.revenue AS revenue") {
		t.Fatalf("row plan SQL missing raw measure:\n%s", plan.SQL)
	}
}

func TestPlannerRawValues(t *testing.T) {
	plan, err := NewPlanner(testModel()).PlanRawValues(RawValueRequest{
		Dimensions: []Field{{Field: "customers.state", Alias: "label"}},
		Measure:    Field{Field: "revenue", Alias: "value"},
		Filters:    []Filter{{Field: "customers.state", Operator: "equals", Values: []any{"SP"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "t1.customer_state AS label") || !strings.Contains(plan.SQL, "CAST(t0.revenue AS DOUBLE) AS value") {
		t.Fatalf("raw value plan SQL missing fields:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "t1.customer_state = ?") {
		t.Fatalf("raw value plan SQL missing filter:\n%s", plan.SQL)
	}
}

func TestPlannerCountWithRelatedFilter(t *testing.T) {
	plan, err := NewPlanner(testModel()).PlanCount(CountRequest{
		Table:   "orders",
		Filters: []Filter{{Field: "customers.state", Operator: "in", Values: []any{"SP"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "COUNT(*) AS value") || !strings.Contains(plan.SQL, "LEFT JOIN model.customers") {
		t.Fatalf("count plan SQL missing count or join:\n%s", plan.SQL)
	}
}

func TestPlannerSafeMultiHopDimensionJoin(t *testing.T) {
	model := testModel()
	model.Sources["regions"] = semantic.Source{Path: "regions.csv", Format: "csv", Connection: "local"}
	model.Tables["regions"] = semantic.ModelTable{
		Source: "regions", PrimaryKey: "region_id",
		Dimensions: map[string]semantic.MetricDimension{
			"region_id": {Expr: "region_id"},
			"name":      {Expr: "region_name"},
		},
	}
	model.Tables["customers"] = semantic.ModelTable{
		Source: "customers", PrimaryKey: "customer_id",
		Dimensions: map[string]semantic.MetricDimension{
			"customer_id": {Expr: "customer_id"},
			"region_id":   {Expr: "region_id"},
			"state":       {Expr: "customer_state"},
		},
	}
	model.Relationships = append(model.Relationships, semantic.Relationship{
		ID: "customers_regions", From: "customers.region_id", To: "regions.region_id", Cardinality: "many_to_one", Active: true,
	})

	plan, err := NewPlanner(model).Plan(Request{
		Dimensions: []Field{{Field: "regions.name", Alias: "region"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "LEFT JOIN model.customers t1 ON t0.customer_id = t1.customer_id") {
		t.Fatalf("plan SQL missing intermediate customer join:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "LEFT JOIN model.regions t2 ON t1.region_id = t2.region_id") {
		t.Fatalf("plan SQL missing region join:\n%s", plan.SQL)
	}
}

func TestPlannerRejectsUnsafeFanout(t *testing.T) {
	model := testModel()
	model.Relationships = append(model.Relationships, semantic.Relationship{
		ID: "orders_items", From: "orders.order_id", To: "items.order_id", Cardinality: "one_to_many", Active: true,
	})
	model.Tables["items"] = semantic.ModelTable{
		Source: "items", PrimaryKey: "item_id",
		Dimensions: map[string]semantic.MetricDimension{"category": {Expr: "category"}},
	}
	_, err := NewPlanner(model).Plan(Request{
		Dimensions: []Field{{Field: "items.category", Alias: "category"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no safe relationship path") {
		t.Fatalf("error = %v, want unsafe path rejection", err)
	}
}

func TestPlannerRejectsInactiveAndManyToManyPaths(t *testing.T) {
	tests := []struct {
		name        string
		cardinality string
		active      bool
	}{
		{name: "inactive", cardinality: "many_to_one", active: false},
		{name: "many to many", cardinality: "many_to_many", active: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := testModel()
			model.Sources["segments"] = semantic.Source{Path: "segments.csv", Format: "csv", Connection: "local"}
			model.Tables["segments"] = semantic.ModelTable{
				Source: "segments", PrimaryKey: "segment_id",
				Dimensions: map[string]semantic.MetricDimension{"name": {Expr: "segment_name"}},
			}
			model.Relationships = append(model.Relationships, semantic.Relationship{
				ID: "orders_segments", From: "orders.customer_id", To: "segments.segment_id", Cardinality: tt.cardinality, Active: tt.active,
			})
			_, err := NewPlanner(model).Plan(Request{
				Dimensions: []Field{{Field: "segments.name", Alias: "segment"}},
				Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
			})
			if err == nil || !strings.Contains(err.Error(), "no safe relationship path") {
				t.Fatalf("error = %v, want no safe path rejection", err)
			}
		})
	}
}

func TestPlannerRejectsAmbiguousRelationshipPath(t *testing.T) {
	model := testModel()
	model.Relationships = append(model.Relationships, semantic.Relationship{
		ID: "orders_customers_alt", From: "orders.customer_id", To: "customers.customer_id", Cardinality: "many_to_one", Active: true,
	})
	_, err := NewPlanner(model).Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous relationship path") {
		t.Fatalf("error = %v, want ambiguous path rejection", err)
	}
}

func TestPlannerRejectsCyclicUnsafePath(t *testing.T) {
	model := testModel()
	model.Sources["segments"] = semantic.Source{Path: "segments.csv", Format: "csv", Connection: "local"}
	model.Tables["segments"] = semantic.ModelTable{
		Source: "segments", PrimaryKey: "segment_id",
		Dimensions: map[string]semantic.MetricDimension{"name": {Expr: "segment_name"}},
	}
	model.Relationships = append(model.Relationships,
		semantic.Relationship{ID: "customers_orders_cycle", From: "customers.customer_id", To: "orders.customer_id", Cardinality: "one_to_one", Active: true},
		semantic.Relationship{ID: "customers_segments_fanout", From: "customers.customer_id", To: "segments.segment_id", Cardinality: "one_to_many", Active: true},
	)
	_, err := NewPlanner(model).Plan(Request{
		Dimensions: []Field{{Field: "segments.name", Alias: "segment"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no safe relationship path") {
		t.Fatalf("error = %v, want cyclic unsafe path rejection", err)
	}
}

func TestPlannerRejectsUnknownFields(t *testing.T) {
	planner := NewPlanner(testModel())
	for _, tt := range []struct {
		field string
		want  string
	}{
		{field: "orders.missing", want: "unknown dimension"},
		{field: "missing.state", want: "unknown table"},
	} {
		_, err := planner.Plan(Request{
			Dimensions: []Field{{Field: tt.field, Alias: "bad"}},
			Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
		})
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("field %q error = %v, want %q", tt.field, err, tt.want)
		}
	}
}

func TestPlannerRejectsCrossFactMeasures(t *testing.T) {
	model := testModel()
	model.Measures["refund_amount"] = semantic.MetricMeasure{
		Field: "refund_amount", Table: "refunds", Name: "refund_amount", Grain: "refund_id", Expression: "SUM(refunds.refund_amount)",
	}
	_, err := NewPlanner(model).Plan(Request{
		Measures: []Field{
			{Field: "revenue", Alias: "revenue"},
			{Field: "refund_amount", Alias: "refund_amount"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cross-fact measures are not supported") {
		t.Fatalf("error = %v, want cross-fact rejection", err)
	}
}

func TestPlannerInlineMeasureUsesQueryOwnedType(t *testing.T) {
	plan, err := NewPlanner(testModel()).Plan(Request{
		Measures: []Field{{
			Field: "one_off_orders",
			Alias: "orders",
			Measure: InlineMeasure{
				Expression: "COUNT(DISTINCT orders.order_id)",
				Table:      "orders",
				Grain:      "order_id",
				Time:       "orders.purchase_timestamp",
				Grains:     []string{"month"},
				Format:     "integer",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "COUNT(DISTINCT t0.order_id) AS orders") {
		t.Fatalf("plan SQL missing inline measure expression:\n%s", plan.SQL)
	}
}

func TestPlannerRejectsUnsafeAliasesAndSorts(t *testing.T) {
	planner := NewPlanner(testModel())
	tests := []struct {
		name    string
		request Request
		want    string
	}{
		{
			name:    "unsafe alias",
			request: Request{Measures: []Field{{Field: "revenue", Alias: "value;drop"}}},
			want:    "invalid identifier",
		},
		{
			name: "duplicate alias",
			request: Request{
				Dimensions: []Field{{Field: "orders.order_id", Alias: "value"}},
				Measures:   []Field{{Field: "revenue", Alias: "value"}},
			},
			want: "duplicate output alias",
		},
		{
			name:    "unknown sort alias",
			request: Request{Measures: []Field{{Field: "revenue", Alias: "value"}}, Sort: []Sort{{Field: "missing_alias", Direction: "desc"}}},
			want:    "not a selected output alias",
		},
		{
			name:    "unsafe sort alias",
			request: Request{Measures: []Field{{Field: "revenue", Alias: "value"}}, Sort: []Sort{{Field: "value;drop", Direction: "desc"}}},
			want:    "invalid identifier",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := planner.Plan(tt.request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func testModel() *semantic.Model {
	return &semantic.Model{
		Name: "commerce",
		Sources: map[string]semantic.Source{
			"orders":    {Path: "orders.csv", Format: "csv", Connection: "local"},
			"customers": {Path: "customers.csv", Format: "csv", Connection: "local"},
			"refunds":   {Path: "refunds.csv", Format: "csv", Connection: "local"},
		},
		BaseTable: "orders",
		Tables: map[string]semantic.ModelTable{
			"orders": {
				Source: "orders", PrimaryKey: "order_id",
				Dimensions: map[string]semantic.MetricDimension{
					"order_id":           {Expr: "order_id"},
					"customer_id":        {Expr: "customer_id"},
					"purchase_timestamp": {Expr: "purchase_timestamp"},
				},
			},
			"refunds": {
				Source: "refunds", PrimaryKey: "refund_id",
				Dimensions: map[string]semantic.MetricDimension{
					"refund_id": {Expr: "refund_id"},
					"order_id":  {Expr: "order_id"},
				},
			},
			"customers": {
				Source: "customers", PrimaryKey: "customer_id",
				Dimensions: map[string]semantic.MetricDimension{
					"customer_id": {Expr: "customer_id"},
					"state":       {Expr: "customer_state"},
				},
			},
		},
		Relationships: []semantic.Relationship{
			{ID: "orders_customers", From: "orders.customer_id", To: "customers.customer_id", Cardinality: "many_to_one", Active: true},
		},
		Measures: map[string]semantic.MetricMeasure{
			"revenue":       {Label: "Revenue", Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)"},
			"order_count":   {Label: "Orders", Table: "orders", Grain: "order_id", Expression: "COUNT(DISTINCT orders.order_id)"},
			"refund_amount": {Label: "Refunds", Table: "refunds", Grain: "refund_id", Expression: "SUM(refunds.refund_amount)"},
		},
	}
}
