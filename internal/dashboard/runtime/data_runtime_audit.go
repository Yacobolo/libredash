package runtime

import (
	"context"
	"fmt"

	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type governedDataRuntime struct {
	DataRuntime
	workspaceID string
	service     reportdef.DataService
}

func newGovernedDataRuntime(workspaceID, modelID string, runtime DataRuntime) DataRuntime {
	wrapped := &governedDataRuntime{DataRuntime: runtime, workspaceID: workspaceID}
	wrapped.service = reportdef.NewDataQueryService(modelID, runtime, wrapped)
	return wrapped
}

func (r *governedDataRuntime) Query(ctx context.Context, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return r.service.Query(ctx, request)
}

func (r *governedDataRuntime) Rows(ctx context.Context, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return r.service.Rows(ctx, request)
}

func (r *governedDataRuntime) Count(ctx context.Context, request reportdef.CountQuery) (int, error) {
	return r.service.Count(ctx, request)
}

func (r *governedDataRuntime) Histogram(ctx context.Context, request reportdef.RawValueQuery, binCount int) ([]reportdef.HistogramBin, error) {
	return r.service.Histogram(ctx, request, binCount)
}

func (r *governedDataRuntime) Distribution(ctx context.Context, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) (reportdef.QueryRows, error) {
	return r.service.Distribution(ctx, request, sort, limit)
}

func (r *governedDataRuntime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	if request.WorkspaceID == "" {
		request.WorkspaceID = r.workspaceID
	}
	return dataquery.ExecuteAudited(ctx, request, r.DataRuntime.ExecuteDataQuery)
}

func (r *governedDataRuntime) RefreshTables(ctx context.Context, tableNames []string) error {
	port, ok := r.DataRuntime.(interface {
		RefreshTables(context.Context, []string) error
	})
	if !ok {
		return fmt.Errorf("dashboard data runtime does not support model table refresh")
	}
	return port.RefreshTables(ctx, tableNames)
}
