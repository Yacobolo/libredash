package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	analyticsmaterialize "github.com/Yacobolo/libredash/internal/analytics/materialize"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	dashboardruntime "github.com/Yacobolo/libredash/internal/dashboard/runtime"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type servingStateRuntimeFactory struct {
	duckDBDir        string
	runtimeDir       string
	catalogPath      string
	duckLakeDataPath string
}

func (f servingStateRuntimeFactory) Prepare(_ context.Context, input runtimehost.RuntimeInput) (runtimehost.Runtime, error) {
	duckDBDir := runtimeFirstNonEmpty(input.DuckDBDir, f.duckDBDir)
	runtimeDir := runtimeFirstNonEmpty(input.RuntimeDir, f.runtimeDir)
	targetDir := filepath.Join(runtimeDir, string(input.State.ID)+"-"+shortDigest(input.Artifact.Digest))
	if err := os.RemoveAll(targetDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, err
	}
	if err := servingstatefs.ExtractArtifact(input.Artifact.Path, targetDir); err != nil {
		return nil, err
	}
	duckDir := filepath.Join(duckDBDir, string(servingstate.NormalizeEnvironment(input.State.Environment)))
	compiled, _, err := servingstatefs.LoadCompiledWorkspaceArtifact(targetDir)
	if err != nil {
		return nil, err
	}
	if compiled.WorkspaceID != string(input.State.WorkspaceID) {
		return nil, fmt.Errorf("compiled artifact workspace = %q, want %q", compiled.WorkspaceID, input.State.WorkspaceID)
	}
	if err := bindManagedDataRoots(compiled.Definition, input.ManagedData); err != nil {
		return nil, err
	}
	dataPath := runtimeFirstNonEmpty(f.duckLakeDataPath, filepath.Join(duckDir, "data"))
	service, err := dashboardruntime.NewFromDefinition(duckDir, dashboardDataRuntimeFactory{
		snapshotID:          input.State.DuckLakeSnapshotID,
		catalogPath:         f.catalogPath,
		duckLakeDataPath:    dataPath,
		servingStateID:      string(input.State.ID),
		workspaceID:         string(input.State.WorkspaceID),
		environment:         string(servingstate.NormalizeEnvironment(input.State.Environment)),
		semanticModelDigest: input.State.Digest,
		artifactDigest:      input.Artifact.Digest,
		sourceDataDigest:    input.ManagedData.RevisionID,
	}, compiled.Definition)
	if err != nil {
		return nil, err
	}
	if input.State.DuckLakeSnapshotID == 0 {
		snapshotID := service.DuckLakeSnapshotID()
		if snapshotID > 0 {
			if err := service.Close(); err != nil {
				return nil, err
			}
			service, err = dashboardruntime.NewFromDefinition(duckDir, dashboardDataRuntimeFactory{
				snapshotID:          snapshotID,
				catalogPath:         f.catalogPath,
				duckLakeDataPath:    dataPath,
				servingStateID:      string(input.State.ID),
				workspaceID:         string(input.State.WorkspaceID),
				environment:         string(servingstate.NormalizeEnvironment(input.State.Environment)),
				semanticModelDigest: input.State.Digest,
				artifactDigest:      input.Artifact.Digest,
				sourceDataDigest:    input.ManagedData.RevisionID,
			}, compiled.Definition)
			if err != nil {
				return nil, err
			}
		}
	}
	return service, nil
}

func bindManagedDataRoots(definition *workspace.Definition, resolution runtimehost.ManagedDataResolution) error {
	if definition == nil {
		return fmt.Errorf("workspace definition is required")
	}
	for modelID, model := range definition.Models {
		if model == nil {
			continue
		}
		for connectionName, connection := range model.Connections {
			if connection.Kind != "managed" {
				continue
			}
			root := filepath.Clean(resolution.Roots[connectionName])
			if resolution.Roots[connectionName] == "" {
				return fmt.Errorf("semantic model %q managed connection %q has no bound revision", modelID, connectionName)
			}
			if !filepath.IsAbs(root) {
				return fmt.Errorf("semantic model %q managed connection %q revision root must be absolute", modelID, connectionName)
			}
			connection.Root = root
			connection.Scope = ""
			model.Connections[connectionName] = connection
		}
	}
	return nil
}

type dashboardDataRuntimeFactory struct {
	snapshotID          int64
	catalogPath         string
	duckLakeDataPath    string
	servingStateID      string
	workspaceID         string
	environment         string
	semanticModelDigest string
	artifactDigest      string
	sourceDataDigest    string
}

func (f dashboardDataRuntimeFactory) OpenDashboardWorkspaceDataRuntimes(ctx context.Context, config dashboardruntime.WorkspaceDataRuntimeConfig) (map[string]dashboardruntime.DataRuntime, error) {
	if config.Definition == nil {
		return nil, fmt.Errorf("workspace definition is required")
	}
	runtime, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, analyticsduckdb.WorkspaceRuntimeConfig{
		Models:           config.Definition.Models,
		DBDir:            config.DBDir,
		CatalogPath:      f.catalogPath,
		DuckLakeDataPath: f.duckLakeDataPath,
		SnapshotID:       f.snapshotID,
		ServingStateID:   f.servingStateID,
		WorkspaceID:      f.workspaceID,
		Environment:      f.environment,
		SemanticDigest:   f.semanticModelDigest,
		ArtifactDigest:   f.artifactDigest,
		SourceDataDigest: f.sourceDataDigest,
	})
	if err != nil {
		return nil, err
	}
	sharedClose := &sharedDashboardDataRuntimeCloser{runtime: runtime}
	runtimes := make(map[string]dashboardruntime.DataRuntime, len(config.Definition.Models))
	for modelID := range config.Definition.Models {
		queries, err := runtime.Queries(modelID)
		if err != nil {
			runtime.Close()
			return nil, err
		}
		runtimes[modelID] = dashboardWorkspaceDataRuntime{
			modelID: modelID,
			runtime: runtime,
			close:   sharedClose,
			data:    reportdef.NewDataQueryService(modelID, reportdef.NewAnalyticsDataService(queries), runtime),
		}
	}
	return runtimes, nil
}

func (dashboardDataRuntimeFactory) OpenDashboardDataRuntime(ctx context.Context, config dashboardruntime.DataRuntimeConfig) (dashboardruntime.DataRuntime, error) {
	runtime, err := analyticsduckdb.OpenMaterializeRuntime(ctx, analyticsmaterialize.RuntimeConfig{
		ModelID: config.ModelID,
		Model:   config.Model,
		DBDir:   config.DBDir,
	})
	if err != nil {
		return nil, err
	}
	return dashboardDataRuntime{
		runtime: runtime,
		data:    reportdef.NewDataQueryService(config.ModelID, reportdef.NewAnalyticsDataService(runtime.Queries()), runtime),
	}, nil
}

type sharedDashboardDataRuntimeCloser struct {
	once    sync.Once
	runtime *analyticsduckdb.WorkspaceRuntime
	err     error
}

func (c *sharedDashboardDataRuntimeCloser) Close() error {
	if c == nil {
		return nil
	}
	c.once.Do(func() {
		c.err = c.runtime.Close()
	})
	return c.err
}

type dashboardWorkspaceDataRuntime struct {
	modelID string
	runtime *analyticsduckdb.WorkspaceRuntime
	close   *sharedDashboardDataRuntimeCloser
	data    reportdef.DataService
}

func (r dashboardWorkspaceDataRuntime) Query(ctx context.Context, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return r.data.Query(ctx, request)
}

func (r dashboardWorkspaceDataRuntime) Rows(ctx context.Context, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return r.data.Rows(ctx, request)
}

func (r dashboardWorkspaceDataRuntime) Count(ctx context.Context, request reportdef.CountQuery) (int, error) {
	return r.data.Count(ctx, request)
}

func (r dashboardWorkspaceDataRuntime) Histogram(ctx context.Context, request reportdef.RawValueQuery, binCount int) ([]reportdef.HistogramBin, error) {
	return r.data.Histogram(ctx, request, binCount)
}

func (r dashboardWorkspaceDataRuntime) Distribution(ctx context.Context, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) (reportdef.QueryRows, error) {
	return r.data.Distribution(ctx, request, sort, limit)
}

func (r dashboardWorkspaceDataRuntime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	return r.runtime.ExecuteDataQuery(ctx, request)
}

func (r dashboardWorkspaceDataRuntime) Refresh(ctx context.Context) error {
	return r.runtime.Refresh(ctx)
}

func (r dashboardWorkspaceDataRuntime) RefreshTables(ctx context.Context, tableNames []string) error {
	return r.runtime.RefreshModelTables(ctx, r.modelID, tableNames)
}

func (r dashboardWorkspaceDataRuntime) Close() error {
	return r.close.Close()
}

func (r dashboardWorkspaceDataRuntime) LastRefresh() time.Time {
	return r.runtime.LastRefresh()
}

func (r dashboardWorkspaceDataRuntime) DuckLakeSnapshotID() int64 {
	return r.runtime.DuckLakeSnapshotID()
}

type dashboardDataRuntime struct {
	runtime *analyticsmaterialize.Runtime
	data    reportdef.DataService
}

func (r dashboardDataRuntime) Query(ctx context.Context, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return r.data.Query(ctx, request)
}

func (r dashboardDataRuntime) Rows(ctx context.Context, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return r.data.Rows(ctx, request)
}

func (r dashboardDataRuntime) Count(ctx context.Context, request reportdef.CountQuery) (int, error) {
	return r.data.Count(ctx, request)
}

func (r dashboardDataRuntime) Histogram(ctx context.Context, request reportdef.RawValueQuery, binCount int) ([]reportdef.HistogramBin, error) {
	return r.data.Histogram(ctx, request, binCount)
}

func (r dashboardDataRuntime) Distribution(ctx context.Context, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) (reportdef.QueryRows, error) {
	return r.data.Distribution(ctx, request, sort, limit)
}

func (r dashboardDataRuntime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	return r.runtime.ExecuteDataQuery(ctx, request)
}

func (r dashboardDataRuntime) Refresh(ctx context.Context) error {
	return r.runtime.Refresh(ctx)
}

func (r dashboardDataRuntime) RefreshTables(ctx context.Context, tableNames []string) error {
	return r.runtime.RefreshModelTables(ctx, tableNames)
}

func (r dashboardDataRuntime) Close() error {
	return r.runtime.Close()
}

func (r dashboardDataRuntime) LastRefresh() time.Time {
	return r.runtime.LastRefresh()
}

func runtimeFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
