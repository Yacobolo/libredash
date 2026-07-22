package arrowquery

import (
	"context"

	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/apache/arrow-go/v18/arrow"
	arrowutil "github.com/apache/arrow-go/v18/arrow/util"
)

// Sink consumes DuckDB-owned record batches synchronously. Implementations must
// not retain schemas, records, arrays, or their buffers after a method returns.
type Sink interface {
	WriteSchema(*arrow.Schema) error
	WriteRecord(arrow.RecordBatch) error
}

// SinkStats optionally reports logical rows actually delivered by a transport.
// It excludes pagination probes that were consumed but not emitted.
type SinkStats interface {
	RowsWritten() int
}

// Executor is the governed native-Arrow path used by Arrow transports. The call
// does not return until the sink has consumed every record batch, so the runtime
// generation, workload permit, and physical connection stay pinned.
type Executor interface {
	ExecuteDataQueryArrow(context.Context, dataquery.Query, Sink) (dataquery.Result, error)
}

func ConsumeResultBudget(ctx context.Context, record arrow.RecordBatch) error {
	if record == nil {
		return nil
	}
	if budget, ok := dataquery.ResultBudgetFromContext(ctx); ok {
		return budget.ConsumeSize(int(record.NumRows()), arrowutil.TotalRecordSize(record))
	}
	return nil
}
