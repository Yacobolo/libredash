package data

import (
	"reflect"
	"testing"

	"github.com/Yacobolo/libredash/internal/semantic"
)

func TestQueryInlineMeasurePreservesSemanticFields(t *testing.T) {
	measure := semantic.MetricMeasure{
		Field:       "one_off_orders",
		Name:        "one_off_orders",
		Label:       "One-off orders",
		Description: "Inline order count",
		Expr:        "COUNT(DISTINCT orders.order_id)",
		Expression:  "COUNT(DISTINCT orders.order_id)",
		Table:       "orders",
		Grain:       "order_id",
		Time:        "orders.purchase_timestamp",
		Grains:      []string{"day", "month"},
		Unit:        "orders",
		Format:      "integer",
	}

	got := queryInlineMeasure(measure)

	if got.Field != measure.Field || got.Name != measure.Name || got.Label != measure.Label || got.Description != measure.Description {
		t.Fatalf("identity fields = %#v, want copied from %#v", got, measure)
	}
	if got.Expr != measure.Expr || got.Expression != measure.Expression || got.Table != measure.Table || got.Grain != measure.Grain || got.Time != measure.Time {
		t.Fatalf("definition fields = %#v, want copied from %#v", got, measure)
	}
	if !reflect.DeepEqual(got.Grains, measure.Grains) || got.Unit != measure.Unit || got.Format != measure.Format {
		t.Fatalf("format fields = %#v, want copied from %#v", got, measure)
	}
	got.Grains[0] = "week"
	if measure.Grains[0] != "day" {
		t.Fatalf("grains share backing array: got %#v source %#v", got.Grains, measure.Grains)
	}
}
