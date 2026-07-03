package runtime

import (
	"context"
	"testing"
	"time"

	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

func TestServiceDuckLakeSnapshotIDRequiresOneWorkspaceSnapshot(t *testing.T) {
	service := &Service{
		runtimes: map[string]*modelRuntime{
			"orders":   {data: snapshotDataRuntime{snapshotID: 42}},
			"products": {data: snapshotDataRuntime{snapshotID: 42}},
		},
	}
	if got := service.DuckLakeSnapshotID(); got != 42 {
		t.Fatalf("DuckLakeSnapshotID = %d, want 42", got)
	}

	service.runtimes["products"].data = snapshotDataRuntime{snapshotID: 43}
	if got := service.DuckLakeSnapshotID(); got != 0 {
		t.Fatalf("DuckLakeSnapshotID with mixed snapshots = %d, want 0", got)
	}
}

func TestGovernedDataRuntimeForwardsDuckLakeSnapshotID(t *testing.T) {
	runtime := newGovernedDataRuntime("sales", "sales", snapshotDataRuntime{snapshotID: 42})
	snapshot, ok := runtime.(DataRuntimeSnapshot)
	if !ok {
		t.Fatalf("governed runtime does not expose DuckLake snapshot")
	}
	if got := snapshot.DuckLakeSnapshotID(); got != 42 {
		t.Fatalf("DuckLakeSnapshotID = %d, want 42", got)
	}
}

type snapshotDataRuntime struct {
	snapshotID int64
}

func (r snapshotDataRuntime) Query(context.Context, reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return nil, nil
}

func (r snapshotDataRuntime) Rows(context.Context, reportdef.RowQuery) (reportdef.QueryRows, error) {
	return nil, nil
}

func (r snapshotDataRuntime) Count(context.Context, reportdef.CountQuery) (int, error) {
	return 0, nil
}

func (r snapshotDataRuntime) Histogram(context.Context, reportdef.RawValueQuery, int) ([]reportdef.HistogramBin, error) {
	return nil, nil
}

func (r snapshotDataRuntime) Distribution(context.Context, reportdef.RawValueQuery, []reportdef.QuerySort, int) (reportdef.QueryRows, error) {
	return nil, nil
}

func (r snapshotDataRuntime) ExecuteDataQuery(context.Context, dataquery.Query) (dataquery.Result, error) {
	return dataquery.Result{}, nil
}

func (r snapshotDataRuntime) Refresh(context.Context) error {
	return nil
}

func (r snapshotDataRuntime) Close() error {
	return nil
}

func (r snapshotDataRuntime) LastRefresh() time.Time {
	return time.Time{}
}

func (r snapshotDataRuntime) DuckLakeSnapshotID() int64 {
	return r.snapshotID
}
