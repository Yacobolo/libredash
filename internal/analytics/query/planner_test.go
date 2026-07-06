package query

import (
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
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
	if !strings.Contains(plan.SQL, "t1.state AS state") {
		t.Fatalf("plan SQL missing related dimension:\n%s", plan.SQL)
	}
}

func TestPlannerRelationshipJoinUsesIdentityEndpointColumns(t *testing.T) {
	model := testModel()

	plan, err := NewPlanner(model).Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "LEFT JOIN model.customers t1 ON t0.customer_id = t1.customer_id") {
		t.Fatalf("plan SQL should join through identity endpoint columns:\n%s", plan.SQL)
	}
}

func TestPlannerRelationshipJoinRejectsMissingEndpointField(t *testing.T) {
	model := testModel()
	customers := model.Tables["customers"]
	delete(customers.Dimensions, "customer_id")
	model.Tables["customers"] = customers

	_, err := NewPlanner(model).Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown relationship endpoint field") {
		t.Fatalf("Plan() error = %v, want missing relationship endpoint field rejection", err)
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

func TestPlannerAggregateLimitOffset(t *testing.T) {
	plan, err := NewPlanner(testModel()).Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
		Sort:       []Sort{{Field: "state", Direction: "asc"}},
		Limit:      25,
		Offset:     50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "ORDER BY state ASC\nLIMIT 25\nOFFSET 50") {
		t.Fatalf("plan SQL missing aggregate limit/offset:\n%s", plan.SQL)
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
	if !strings.Contains(plan.SQL, "t1.state IN (?, ?)") {
		t.Fatalf("plan SQL missing semantic filter:\n%s", plan.SQL)
	}
	if len(plan.Args) != 2 {
		t.Fatalf("args = %#v, want two filter args", plan.Args)
	}
}

func TestPlannerGroupedFiltersUseOROfANDPredicates(t *testing.T) {
	plan, err := NewPlanner(testModel()).Plan(Request{
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "revenue", Alias: "revenue"}},
		Filters: []Filter{{
			Groups: []FilterGroup{
				{Filters: []Filter{
					{Field: "orders.order_id", Operator: "equals", Values: []any{"o1"}},
					{Field: "customers.state", Operator: "equals", Values: []any{"SP"}},
				}},
				{Filters: []Filter{
					{Field: "orders.order_id", Operator: "equals", Values: []any{"o2"}},
					{Field: "customers.state", Operator: "equals", Values: []any{"RJ"}},
				}},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "((t0.order_id = ? AND t1.state = ?) OR (t0.order_id = ? AND t1.state = ?))"
	if !strings.Contains(plan.SQL, want) {
		t.Fatalf("plan SQL missing grouped tuple predicate %q:\n%s", want, plan.SQL)
	}
	if got := len(plan.Args); got != 4 {
		t.Fatalf("args = %#v, want four filter args", plan.Args)
	}
	if plan.Args[0] != "o1" || plan.Args[1] != "SP" || plan.Args[2] != "o2" || plan.Args[3] != "RJ" {
		t.Fatalf("args = %#v, want [o1 SP o2 RJ]", plan.Args)
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
	if !strings.Contains(plan.SQL, "t0.order_id AS order_id") || !strings.Contains(plan.SQL, "t1.state AS state") {
		t.Fatalf("row plan SQL missing selected dimensions:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "t0.revenue AS revenue") {
		t.Fatalf("row plan SQL missing raw measure:\n%s", plan.SQL)
	}
}

func TestPlannerMasksSelectedRowFieldInSQL(t *testing.T) {
	plan, err := NewPlanner(testModel()).PlanRows(RowRequest{
		Table:       "orders",
		Dimensions:  []Field{{Field: "orders.order_id", Alias: "order_id"}},
		Measures:    []Field{{Field: "revenue", Alias: "revenue"}},
		ColumnMasks: []ColumnMask{{Field: "orders.order_id", Mask: "redact"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "'REDACTED' AS order_id") {
		t.Fatalf("row plan SQL did not mask selected field:\n%s", plan.SQL)
	}
	if strings.Contains(plan.SQL, "t0.order_id AS order_id") {
		t.Fatalf("row plan SQL leaked unmasked selected field:\n%s", plan.SQL)
	}
}

func TestPlannerRejectsMaskedAggregateMeasureDependency(t *testing.T) {
	_, err := NewPlanner(testModel()).Plan(Request{
		Measures:    []Field{{Field: "revenue", Alias: "revenue"}},
		ColumnMasks: []ColumnMask{{Field: "orders.revenue", Mask: "zero"}},
	})
	if err == nil || !strings.Contains(err.Error(), "depends on masked field") {
		t.Fatalf("Plan() error = %v, want masked measure rejection", err)
	}
}

func TestPlannerRejectsMaskedFieldInsideAggregateMeasureExpression(t *testing.T) {
	model := testModel()
	model.Measures["net_revenue"] = semanticmodel.MetricMeasure{
		Label:      "Net revenue",
		Table:      "orders",
		Grain:      "order_id",
		Expression: "SUM(orders.revenue - orders.discount)",
	}
	_, err := NewPlanner(model).Plan(Request{
		Measures:    []Field{{Field: "net_revenue", Alias: "net_revenue"}},
		ColumnMasks: []ColumnMask{{Field: "orders.discount", Mask: "zero"}},
	})
	if err == nil || !strings.Contains(err.Error(), "depends on masked field") {
		t.Fatalf("Plan() error = %v, want masked expression dependency rejection", err)
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
	if !strings.Contains(plan.SQL, "t1.state AS label") || !strings.Contains(plan.SQL, "CAST(t0.revenue AS DOUBLE) AS value") {
		t.Fatalf("raw value plan SQL missing fields:\n%s", plan.SQL)
	}
	if !strings.Contains(plan.SQL, "t1.state = ?") {
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
	model.Sources["regions"] = semanticmodel.Source{Path: "regions.csv", Format: "csv", Connection: "local"}
	model.Tables["regions"] = semanticmodel.Table{
		Source: "regions", PrimaryKey: "region_id",
		Dimensions: map[string]semanticmodel.MetricDimension{
			"region_id": {Expr: "region_id"},
			"name":      {Expr: "region_name"},
		},
	}
	model.Tables["customers"] = semanticmodel.Table{
		Source: "customers", PrimaryKey: "customer_id",
		Dimensions: map[string]semanticmodel.MetricDimension{
			"customer_id": {Expr: "customer_id"},
			"region_id":   {Expr: "region_id"},
			"state":       {Expr: "customer_state"},
		},
	}
	model.Relationships = append(model.Relationships, semanticmodel.Relationship{
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
	model.Relationships = append(model.Relationships, semanticmodel.Relationship{
		ID: "orders_items", From: "orders.order_id", To: "items.order_id", Cardinality: "one_to_many", Active: true,
	})
	model.Tables["items"] = semanticmodel.Table{
		Source: "items", PrimaryKey: "item_id",
		Dimensions: map[string]semanticmodel.MetricDimension{"category": {Expr: "category"}},
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
			model.Sources["segments"] = semanticmodel.Source{Path: "segments.csv", Format: "csv", Connection: "local"}
			model.Tables["segments"] = semanticmodel.Table{
				Source: "segments", PrimaryKey: "segment_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"name": {Expr: "segment_name"}},
			}
			model.Relationships = append(model.Relationships, semanticmodel.Relationship{
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
	model.Relationships = append(model.Relationships, semanticmodel.Relationship{
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
	model.Sources["segments"] = semanticmodel.Source{Path: "segments.csv", Format: "csv", Connection: "local"}
	model.Tables["segments"] = semanticmodel.Table{
		Source: "segments", PrimaryKey: "segment_id",
		Dimensions: map[string]semanticmodel.MetricDimension{"name": {Expr: "segment_name"}},
	}
	model.Relationships = append(model.Relationships,
		semanticmodel.Relationship{ID: "customers_orders_cycle", From: "customers.customer_id", To: "orders.customer_id", Cardinality: "one_to_one", Active: true},
		semanticmodel.Relationship{ID: "customers_segments_fanout", From: "customers.customer_id", To: "segments.segment_id", Cardinality: "one_to_many", Active: true},
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
		{field: "orders.missing", want: "unknown field"},
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
	model.Measures["refund_amount"] = semanticmodel.MetricMeasure{
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

func testModel() *semanticmodel.Model {
	return &semanticmodel.Model{
		Name: "commerce",
		Sources: map[string]semanticmodel.Source{
			"orders":    {Path: "orders.csv", Format: "csv", Connection: "local"},
			"customers": {Path: "customers.csv", Format: "csv", Connection: "local"},
			"refunds":   {Path: "refunds.csv", Format: "csv", Connection: "local"},
		},
		BaseTable: "orders",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source: "orders", PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id":           {Expr: "order_id"},
					"customer_id":        {Expr: "customer_id"},
					"purchase_timestamp": {Expr: "purchase_timestamp"},
				},
			},
			"refunds": {
				Source: "refunds", PrimaryKey: "refund_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"refund_id": {Expr: "refund_id"},
					"order_id":  {Expr: "order_id"},
				},
			},
			"customers": {
				Source: "customers", PrimaryKey: "customer_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"customer_id": {Expr: "customer_id"},
					"state":       {Expr: "customer_state"},
				},
			},
		},
		Relationships: []semanticmodel.Relationship{
			{ID: "orders_customers", From: "orders.customer_id", To: "customers.customer_id", Cardinality: "many_to_one", Active: true},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue":       {Label: "Revenue", Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)"},
			"order_count":   {Label: "Orders", Table: "orders", Grain: "order_id", Expression: "COUNT(DISTINCT orders.order_id)"},
			"refund_amount": {Label: "Refunds", Table: "refunds", Grain: "refund_id", Expression: "SUM(refunds.refund_amount)"},
		},
	}
}
