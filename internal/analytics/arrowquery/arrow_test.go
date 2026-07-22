package arrowquery

import (
	"context"
	"testing"

	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func TestConsumeResultBudgetAccountsNativeRecord(t *testing.T) {
	allocator := memory.NewCheckedAllocator(memory.DefaultAllocator)
	defer allocator.AssertSize(t, 0)
	builder := array.NewInt64Builder(allocator)
	builder.AppendValues([]int64{1, 2}, nil)
	values := builder.NewArray()
	builder.Release()
	defer values.Release()
	record := array.NewRecord(arrow.NewSchema([]arrow.Field{{Name: "id", Type: arrow.PrimitiveTypes.Int64}}, nil), []arrow.Array{values}, 2)
	defer record.Release()
	ctx := dataquery.WithResultBudget(context.Background(), dataquery.ResultLimits{MaxRows: 2, MaxBytes: 1 << 20})
	if err := ConsumeResultBudget(ctx, record); err != nil {
		t.Fatal(err)
	}
	budget, _ := dataquery.ResultBudgetFromContext(ctx)
	if rows, bytes := budget.Usage(); rows != 2 || bytes <= 0 {
		t.Fatalf("usage = (%d, %d)", rows, bytes)
	}
}
