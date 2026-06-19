package query

import (
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/semantic"
)

func TestPlannerSingleTableMeasure(t *testing.T) {
	planner := NewPlanner(testModel(), testViews())
	plan, err := planner.Plan(Request{
		MetricView: "orders",
		Measures:   []Field{{Field: "orders.revenue", Alias: "value"}},
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
	planner := NewPlanner(testModel(), testViews())
	plan, err := planner.Plan(Request{
		MetricView: "orders",
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "orders.revenue", Alias: "revenue"}},
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

func TestPlannerTimeGrain(t *testing.T) {
	planner := NewPlanner(testModel(), testViews())
	plan, err := planner.Plan(Request{
		MetricView: "orders",
		Time:       Time{Field: "orders.purchase_timestamp", Grain: "month", Alias: "month"},
		Measures:   []Field{{Field: "orders.revenue", Alias: "revenue"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.SQL, "date_trunc('month', t0.purchase_timestamp) AS month") {
		t.Fatalf("plan SQL missing date_trunc:\n%s", plan.SQL)
	}
}

func TestPlannerFilters(t *testing.T) {
	planner := NewPlanner(testModel(), testViews())
	plan, err := planner.Plan(Request{
		MetricView: "orders",
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "orders.revenue", Alias: "revenue"}},
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
	planner := NewPlanner(testModel(), testViews())
	plan, err := planner.PlanRows(RowRequest{
		MetricView: "orders",
		Dimensions: []Field{
			{Field: "orders.order_id", Alias: "order_id"},
			{Field: "customers.state", Alias: "state"},
		},
		Measures: []Field{{Field: "orders.revenue", Alias: "revenue"}},
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
	planner := NewPlanner(testModel(), testViews())
	plan, err := planner.PlanRawValues(RawValueRequest{
		MetricView: "orders",
		Dimensions: []Field{{Field: "customers.state", Alias: "label"}},
		Measure:    Field{Field: "orders.revenue", Alias: "value"},
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
	planner := NewPlanner(testModel(), testViews())
	plan, err := planner.PlanCount(CountRequest{
		MetricView: "orders",
		Filters:    []Filter{{Field: "customers.state", Operator: "in", Values: []any{"SP"}}},
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
		Kind: "dimension", Source: "regions", PrimaryKey: "region_id", Grain: "region_id",
		Dimensions: map[string]semantic.MetricDimension{
			"region_id": {Expr: "region_id"},
			"name":      {Expr: "region_name"},
		},
	}
	model.Tables["customers"] = semantic.ModelTable{
		Kind: "dimension", Source: "customers", PrimaryKey: "customer_id", Grain: "customer_id",
		Dimensions: map[string]semantic.MetricDimension{
			"customer_id": {Expr: "customer_id"},
			"region_id":   {Expr: "region_id"},
			"state":       {Expr: "customer_state"},
		},
	}
	model.Relationships = append(model.Relationships, semantic.Relationship{
		ID: "customers_regions", From: "customers.region_id", To: "regions.region_id", Cardinality: "many_to_one", Active: true,
	})
	views := testViews()
	views["orders"].Dimensions["regions.name"] = semantic.MetricDimension{Field: "regions.name", Table: "regions", Name: "name", Expr: "region_name"}

	plan, err := NewPlanner(model, views).Plan(Request{
		MetricView: "orders",
		Dimensions: []Field{{Field: "regions.name", Alias: "region"}},
		Measures:   []Field{{Field: "orders.revenue", Alias: "revenue"}},
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
		Kind: "fact", Source: "items", PrimaryKey: "item_id", Grain: "item_id",
		Dimensions: map[string]semantic.MetricDimension{"category": {Expr: "category"}},
	}
	view := testViews()["orders"]
	view.DimensionRefs = append(view.DimensionRefs, "items.category")
	view.Dimensions["items.category"] = semantic.MetricDimension{Field: "items.category", Table: "items", Name: "category", Expr: "category"}
	_, err := NewPlanner(model, map[string]*semantic.MetricView{"orders": view}).Plan(Request{
		MetricView: "orders",
		Dimensions: []Field{{Field: "items.category", Alias: "category"}},
		Measures:   []Field{{Field: "orders.revenue", Alias: "revenue"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no safe relationship path") {
		t.Fatalf("error = %v, want unsafe path rejection", err)
	}
}

func TestPlannerRejectsInactiveAndManyToManyPaths(t *testing.T) {
	tests := []struct {
		name         string
		cardinality  string
		active       bool
		wantContains string
	}{
		{name: "inactive", cardinality: "many_to_one", active: false, wantContains: "no safe relationship path"},
		{name: "many to many", cardinality: "many_to_many", active: true, wantContains: "no safe relationship path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := testModel()
			model.Sources["segments"] = semantic.Source{Path: "segments.csv", Format: "csv", Connection: "local"}
			model.Tables["segments"] = semantic.ModelTable{
				Kind: "dimension", Source: "segments", PrimaryKey: "segment_id", Grain: "segment_id",
				Dimensions: map[string]semantic.MetricDimension{"name": {Expr: "segment_name"}},
			}
			model.Relationships = append(model.Relationships, semantic.Relationship{
				ID: "orders_segments", From: "orders.customer_id", To: "segments.segment_id", Cardinality: tt.cardinality, Active: tt.active,
			})
			view := testViews()["orders"]
			view.Dimensions["segments.name"] = semantic.MetricDimension{Field: "segments.name", Table: "segments", Name: "name", Expr: "segment_name"}
			_, err := NewPlanner(model, map[string]*semantic.MetricView{"orders": view}).Plan(Request{
				MetricView: "orders",
				Dimensions: []Field{{Field: "segments.name", Alias: "segment"}},
				Measures:   []Field{{Field: "orders.revenue", Alias: "revenue"}},
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantContains) {
				t.Fatalf("error = %v, want %q", err, tt.wantContains)
			}
		})
	}
}

func TestPlannerRejectsAmbiguousRelationshipPath(t *testing.T) {
	model := testModel()
	model.Relationships = append(model.Relationships, semantic.Relationship{
		ID: "orders_customers_alt", From: "orders.customer_id", To: "customers.customer_id", Cardinality: "many_to_one", Active: true,
	})
	_, err := NewPlanner(model, testViews()).Plan(Request{
		MetricView: "orders",
		Dimensions: []Field{{Field: "customers.state", Alias: "state"}},
		Measures:   []Field{{Field: "orders.revenue", Alias: "revenue"}},
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous relationship path") {
		t.Fatalf("error = %v, want ambiguous path rejection", err)
	}
}

func TestPlannerRejectsCyclicUnsafePath(t *testing.T) {
	model := testModel()
	model.Sources["segments"] = semantic.Source{Path: "segments.csv", Format: "csv", Connection: "local"}
	model.Tables["segments"] = semantic.ModelTable{
		Kind: "dimension", Source: "segments", PrimaryKey: "segment_id", Grain: "segment_id",
		Dimensions: map[string]semantic.MetricDimension{"name": {Expr: "segment_name"}},
	}
	model.Relationships = append(model.Relationships,
		semantic.Relationship{ID: "customers_orders_cycle", From: "customers.customer_id", To: "orders.customer_id", Cardinality: "one_to_one", Active: true},
		semantic.Relationship{ID: "customers_segments_fanout", From: "customers.customer_id", To: "segments.segment_id", Cardinality: "one_to_many", Active: true},
	)
	view := testViews()["orders"]
	view.Dimensions["segments.name"] = semantic.MetricDimension{Field: "segments.name", Table: "segments", Name: "name", Expr: "segment_name"}
	_, err := NewPlanner(model, map[string]*semantic.MetricView{"orders": view}).Plan(Request{
		MetricView: "orders",
		Dimensions: []Field{{Field: "segments.name", Alias: "segment"}},
		Measures:   []Field{{Field: "orders.revenue", Alias: "revenue"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no safe relationship path") {
		t.Fatalf("error = %v, want cyclic unsafe path rejection", err)
	}
}

func TestPlannerRejectsUnknownAndUnexposedFields(t *testing.T) {
	planner := NewPlanner(testModel(), testViews())
	for _, tt := range []struct {
		field string
		want  string
	}{
		{field: "orders.missing", want: "not exposed"},
		{field: "orders.customer_id", want: "not exposed"},
	} {
		_, err := planner.Plan(Request{
			MetricView: "orders",
			Dimensions: []Field{{Field: tt.field, Alias: "bad"}},
			Measures:   []Field{{Field: "orders.revenue", Alias: "revenue"}},
		})
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("field %q error = %v, want %q", tt.field, err, tt.want)
		}
	}
}

func TestPlannerRejectsNonBaseMeasureAndMultiFactMix(t *testing.T) {
	model := testModel()
	model.Sources["items"] = semantic.Source{Path: "items.csv", Format: "csv", Connection: "local"}
	model.Tables["items"] = semantic.ModelTable{
		Kind: "fact", Source: "items", PrimaryKey: "item_id", Grain: "item_id",
		Measures: map[string]semantic.MetricMeasure{"item_revenue": {Expression: "SUM(items.revenue)"}},
	}
	view := testViews()["orders"]
	view.Measures["items.item_revenue"] = semantic.MetricMeasure{Field: "items.item_revenue", Table: "items", Name: "item_revenue", Expression: "SUM(items.revenue)"}
	_, err := NewPlanner(model, map[string]*semantic.MetricView{"orders": view}).Plan(Request{
		MetricView: "orders",
		Measures: []Field{
			{Field: "orders.revenue", Alias: "revenue"},
			{Field: "items.item_revenue", Alias: "item_revenue"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not owned by base table") {
		t.Fatalf("error = %v, want non-base measure rejection", err)
	}
}

func TestPlannerRejectsUnsafeAliasesAndSorts(t *testing.T) {
	planner := NewPlanner(testModel(), testViews())
	tests := []struct {
		name    string
		request Request
		want    string
	}{
		{
			name: "unsafe alias",
			request: Request{
				MetricView: "orders",
				Measures:   []Field{{Field: "orders.revenue", Alias: "value;drop"}},
			},
			want: "invalid identifier",
		},
		{
			name: "duplicate alias",
			request: Request{
				MetricView: "orders",
				Dimensions: []Field{{Field: "orders.order_id", Alias: "value"}},
				Measures:   []Field{{Field: "orders.revenue", Alias: "value"}},
			},
			want: "duplicate output alias",
		},
		{
			name: "unknown sort alias",
			request: Request{
				MetricView: "orders",
				Measures:   []Field{{Field: "orders.revenue", Alias: "value"}},
				Sort:       []Sort{{Field: "missing_alias", Direction: "desc"}},
			},
			want: "not a selected output alias",
		},
		{
			name: "unsafe sort alias",
			request: Request{
				MetricView: "orders",
				Measures:   []Field{{Field: "orders.revenue", Alias: "value"}},
				Sort:       []Sort{{Field: "value;drop", Direction: "desc"}},
			},
			want: "invalid identifier",
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
		},
		Tables: map[string]semantic.ModelTable{
			"orders": {
				Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
				Dimensions: map[string]semantic.MetricDimension{
					"order_id":           {Expr: "order_id"},
					"customer_id":        {Expr: "customer_id"},
					"purchase_timestamp": {Expr: "purchase_timestamp"},
				},
				Measures: map[string]semantic.MetricMeasure{
					"revenue": {Label: "Revenue", Expression: "SUM(orders.revenue)"},
				},
			},
			"customers": {
				Kind: "dimension", Source: "customers", PrimaryKey: "customer_id", Grain: "customer_id",
				Dimensions: map[string]semantic.MetricDimension{
					"customer_id": {Expr: "customer_id"},
					"state":       {Expr: "customer_state"},
				},
			},
		},
		Relationships: []semantic.Relationship{
			{ID: "orders_customers", From: "orders.customer_id", To: "customers.customer_id", Cardinality: "many_to_one", Active: true},
		},
	}
}

func testViews() map[string]*semantic.MetricView {
	return map[string]*semantic.MetricView{
		"orders": {
			ID: "orders", SemanticModel: "commerce", BaseTable: "orders", Grain: "order_id",
			Dimensions: map[string]semantic.MetricDimension{
				"orders.order_id":           {Field: "orders.order_id", Table: "orders", Name: "order_id", Expr: "order_id"},
				"orders.purchase_timestamp": {Field: "orders.purchase_timestamp", Table: "orders", Name: "purchase_timestamp", Expr: "purchase_timestamp"},
				"customers.state":           {Field: "customers.state", Table: "customers", Name: "state", Expr: "customer_state"},
			},
			Measures: map[string]semantic.MetricMeasure{
				"orders.revenue": {Field: "orders.revenue", Table: "orders", Name: "revenue", Label: "Revenue", Expression: "SUM(orders.revenue)"},
			},
		},
	}
}
