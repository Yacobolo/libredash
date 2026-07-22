// Package dashboardadapter adapts the analytics DuckDB materialization runtime to
// the dashboard runtime's data interfaces.
package dashboardadapter

import (
	"context"
	"fmt"
	"sync"
	"time"

	analyticsduckdb "github.com/Yacobolo/leapview/internal/analytics/duckdb"
	analyticsmaterialize "github.com/Yacobolo/leapview/internal/analytics/materialize"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	dashboardruntime "github.com/Yacobolo/leapview/internal/dashboard/runtime"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

type Options struct {
	SnapshotID          int64
	CatalogPath         string
	DuckLakeDataPath    string
	ServingStateID      string
	WorkspaceID         string
	Environment         string
	SemanticModelDigest string
	ArtifactDigest      string
	SourceDataDigest    string
}

type Factory struct{ options Options }

func NewFactory(options Options) Factory { return Factory{options: options} }

func (f Factory) OpenDashboardWorkspaceDataRuntimes(ctx context.Context, config dashboardruntime.WorkspaceDataRuntimeConfig) (map[string]dashboardruntime.DataRuntime, error) {
	if config.Definition == nil {
		return nil, fmt.Errorf("workspace definition is required")
	}
	options := f.options
	runtime, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, analyticsduckdb.WorkspaceRuntimeConfig{
		Models: config.Definition.Models, DBDir: config.DBDir, CatalogPath: options.CatalogPath,
		DuckLakeDataPath: options.DuckLakeDataPath, SnapshotID: options.SnapshotID,
		ServingStateID: options.ServingStateID, WorkspaceID: options.WorkspaceID, Environment: options.Environment,
		SemanticDigest: options.SemanticModelDigest, ArtifactDigest: options.ArtifactDigest, SourceDataDigest: options.SourceDataDigest,
	})
	if err != nil {
		return nil, err
	}
	sharedClose := &sharedCloser{runtime: runtime}
	runtimes := make(map[string]dashboardruntime.DataRuntime, len(config.Definition.Models))
	for modelID := range config.Definition.Models {
		queries, err := runtime.Queries(modelID)
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtimes[modelID] = workspaceRuntime{modelID: modelID, runtime: runtime, close: sharedClose, data: reportdef.NewDataQueryService(modelID, reportdef.NewAnalyticsDataService(queries), runtime)}
	}
	return runtimes, nil
}

func (Factory) OpenDashboardDataRuntime(ctx context.Context, config dashboardruntime.DataRuntimeConfig) (dashboardruntime.DataRuntime, error) {
	runtime, err := analyticsduckdb.OpenMaterializeRuntime(ctx, analyticsmaterialize.RuntimeConfig{ModelID: config.ModelID, Model: config.Model, DBDir: config.DBDir})
	if err != nil {
		return nil, err
	}
	return singleRuntime{runtime: runtime, data: reportdef.NewDataQueryService(config.ModelID, reportdef.NewAnalyticsDataService(runtime.Queries()), runtime)}, nil
}

type sharedCloser struct {
	once    sync.Once
	runtime *analyticsduckdb.WorkspaceRuntime
	err     error
}

func (c *sharedCloser) Close() error {
	c.once.Do(func() { c.err = c.runtime.Close() })
	return c.err
}

type workspaceRuntime struct {
	modelID string
	runtime *analyticsduckdb.WorkspaceRuntime
	close   *sharedCloser
	data    reportdef.DataService
}

func (r workspaceRuntime) Query(ctx context.Context, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return r.data.Query(ctx, request)
}
func (r workspaceRuntime) Rows(ctx context.Context, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return r.data.Rows(ctx, request)
}
func (r workspaceRuntime) Count(ctx context.Context, request reportdef.CountQuery) (int, error) {
	return r.data.Count(ctx, request)
}
func (r workspaceRuntime) Histogram(ctx context.Context, request reportdef.RawValueQuery, bins int) ([]reportdef.HistogramBin, error) {
	return r.data.Histogram(ctx, request, bins)
}
func (r workspaceRuntime) Distribution(ctx context.Context, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) (reportdef.QueryRows, error) {
	return r.data.Distribution(ctx, request, sort, limit)
}
func (r workspaceRuntime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	return r.runtime.ExecuteDataQuery(ctx, request)
}
func (r workspaceRuntime) ExecuteDataQueryBundle(ctx context.Context, requests []dataquery.BundleRequest) (dataquery.BundleResult, error) {
	return r.runtime.ExecuteDataQueryBundle(ctx, requests)
}
func (r workspaceRuntime) Refresh(ctx context.Context) error { return r.runtime.Refresh(ctx) }
func (r workspaceRuntime) RefreshTables(ctx context.Context, tables []string) error {
	return r.runtime.RefreshModelTables(ctx, r.modelID, tables)
}
func (r workspaceRuntime) Close() error              { return r.close.Close() }
func (r workspaceRuntime) LastRefresh() time.Time    { return r.runtime.LastRefresh() }
func (r workspaceRuntime) DuckLakeSnapshotID() int64 { return r.runtime.DuckLakeSnapshotID() }
func (r workspaceRuntime) ReadConcurrency() int      { return r.runtime.ReadConcurrency() }

type singleRuntime struct {
	runtime *analyticsmaterialize.Runtime
	data    reportdef.DataService
}

func (r singleRuntime) Query(ctx context.Context, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return r.data.Query(ctx, request)
}
func (r singleRuntime) Rows(ctx context.Context, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return r.data.Rows(ctx, request)
}
func (r singleRuntime) Count(ctx context.Context, request reportdef.CountQuery) (int, error) {
	return r.data.Count(ctx, request)
}
func (r singleRuntime) Histogram(ctx context.Context, request reportdef.RawValueQuery, bins int) ([]reportdef.HistogramBin, error) {
	return r.data.Histogram(ctx, request, bins)
}
func (r singleRuntime) Distribution(ctx context.Context, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) (reportdef.QueryRows, error) {
	return r.data.Distribution(ctx, request, sort, limit)
}
func (r singleRuntime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	return r.runtime.ExecuteDataQuery(ctx, request)
}
func (r singleRuntime) ExecuteDataQueryBundle(ctx context.Context, requests []dataquery.BundleRequest) (dataquery.BundleResult, error) {
	return r.runtime.ExecuteDataQueryBundle(ctx, requests)
}
func (r singleRuntime) Refresh(ctx context.Context) error { return r.runtime.Refresh(ctx) }
func (r singleRuntime) RefreshTables(ctx context.Context, tables []string) error {
	return r.runtime.RefreshModelTables(ctx, tables)
}
func (r singleRuntime) Close() error           { return r.runtime.Close() }
func (r singleRuntime) LastRefresh() time.Time { return r.runtime.LastRefresh() }
