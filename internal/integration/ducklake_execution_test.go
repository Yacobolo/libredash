package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	analyticsmaterialize "github.com/Yacobolo/libredash/internal/analytics/materialize"
	analyticsmaterializesqlite "github.com/Yacobolo/libredash/internal/analytics/materialize/sqlite"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/app"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	dashboardruntime "github.com/Yacobolo/libredash/internal/dashboard/runtime"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/execution"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/queryaudit"
	queryauditsqlite "github.com/Yacobolo/libredash/internal/queryaudit/sqlite"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	storagemaintenance "github.com/Yacobolo/libredash/internal/storage/maintenance"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
	_ "github.com/duckdb/duckdb-go/v2"
)

type duckLakeHarness struct {
	*harness
	homeDir     string
	dataDir     string
	artifactDir string
	duckDBDir   string
	runtimeDir  string
	catalogPath string
	dataPath    string
	deployments *servingstatesqlite.Repository
	registry    *runtimehost.Registry
	appServer   *app.Server
}

func newDuckLakeHarness(t *testing.T, opts ...func(*app.Options)) *duckLakeHarness {
	t.Helper()
	ctx := context.Background()
	homeDir := t.TempDir()
	dataDir := filepath.Join(homeDir, "source")
	artifactDir := filepath.Join(homeDir, "artifacts")
	duckDBDir := filepath.Join(homeDir, ".libredash", "duckdb")
	runtimeDir := filepath.Join(homeDir, ".libredash", "runtime")
	dataPath := filepath.Join(homeDir, ".libredash", "data")
	platformDBPath := filepath.Join(homeDir, ".libredash", "libredash.db")
	catalogPath := filepath.Join(homeDir, ".libredash", "ducklake", "catalog.sqlite")
	for _, dir := range []string{dataDir, artifactDir, duckDBDir, runtimeDir, dataPath, filepath.Dir(platformDBPath), filepath.Dir(catalogPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create harness dir %s: %v", dir, err)
		}
	}
	writeMinimalOlistFixture(t, dataDir)
	store, err := platform.Open(ctx, platformDBPath)
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	workspaceID := "sales"
	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: workspace.WorkspaceID(workspaceID), Title: "Sales Workspace"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	if err := app.SeedLocalDeveloperPlatformAdmin(ctx, accessRepo); err != nil {
		t.Fatalf("seed local developer: %v", err)
	}
	deploymentRepo := servingstatesqlite.NewRepository(store.SQLDB())
	projectPath := discoverCatalogPath(t)
	initial := createAndActivateProjectDeployment(t, ctx, deploymentRepo, artifactDir, projectPath, dataDir, duckDBDir, workspaceID, "integration")
	var registry *runtimehost.Registry
	registry = runtimehost.NewRegistryWithFactory(runtimehost.RegistryOptions{
		Repo:         deploymentRepo,
		WorkspaceIDs: []servingstate.WorkspaceID{servingstate.WorkspaceID(workspaceID)},
		Environment:  servingstate.DefaultEnvironment,
		OnDrained: func(servingstate.ID, int64) {
			_, _ = storagemaintenance.Run(context.Background(), deploymentRepo, storagemaintenance.Options{
				RootDir:                      homeDir,
				CatalogPath:                  catalogPath,
				DataPath:                     dataPath,
				AdditionalProtectedSnapshots: registryLeasedSnapshots(registry),
				DryRun:                       false,
			})
		},
		Factory: duckLakeIntegrationRuntimeFactory{
			managedRoot:      dataDir,
			duckDBDir:        duckDBDir,
			runtimeDir:       runtimeDir,
			catalogPath:      catalogPath,
			duckLakeDataPath: dataPath,
		},
	})
	if err := registry.Reload(ctx); err != nil {
		t.Fatalf("reload registry for %s: %v", initial.ID, err)
	}
	t.Cleanup(func() { _ = registry.Close() })
	runtimeMetrics := app.NewDynamicRuntimeMetrics("", func(workspaceID string) app.RuntimeProvider {
		return registry.ProviderForWorkspace(servingstate.WorkspaceID(workspaceID))
	})
	auth := app.NewAuth(accessRepo, "", app.AuthConfig{DevBypass: true})
	options := app.Options{
		Store:               store,
		ServingStateRepo:    deploymentRepo,
		WorkspaceRepo:       workspaceRepo,
		AssetCatalog:        workspace.NewAssetCatalogService(workspaceRepo),
		AccessRepo:          accessRepo,
		Auth:                auth,
		Reloader:            registry,
		ArtifactDir:         artifactDir,
		DuckDBDir:           duckDBDir,
		DuckLakeCatalogPath: catalogPath,
		DuckLakeDataPath:    dataPath,
		DefaultWorkspaceID:  workspaceID,
		DefaultEnvironment:  string(servingstate.DefaultEnvironment),
		Executor:            execution.New(execution.DefaultConfig()),
	}
	for _, opt := range opts {
		opt(&options)
	}
	server := app.NewWithOptions(runtimeMetrics, options)
	backgroundCtx, stopBackground := context.WithCancel(ctx)
	server.StartBackgroundJobs(backgroundCtx)
	h := &duckLakeHarness{
		harness: &harness{
			handler:     server.Routes(),
			store:       store,
			workspaceID: workspaceID,
		},
		homeDir:     homeDir,
		dataDir:     dataDir,
		artifactDir: artifactDir,
		duckDBDir:   duckDBDir,
		runtimeDir:  runtimeDir,
		catalogPath: catalogPath,
		dataPath:    dataPath,
		deployments: deploymentRepo,
		registry:    registry,
		appServer:   server,
	}
	h.server = httptest.NewServer(h.handler)
	t.Cleanup(h.server.Close)
	t.Cleanup(func() {
		stopBackground()
		stopServerBackgroundForTest(t, server)
	})
	return h
}

func registryLeasedSnapshots(registry *runtimehost.Registry) []int64 {
	if registry == nil {
		return nil
	}
	return registry.LeasedSnapshots()
}

func createAndActivateProjectDeployment(t *testing.T, ctx context.Context, repo *servingstatesqlite.Repository, artifactDir, projectPath, dataDir, duckDBDir, workspaceID, createdBy string) servingstate.State {
	t.Helper()
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: servingstate.WorkspaceID(workspaceID), CreatedBy: createdBy})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	artifactPath := filepath.Join(artifactDir, string(created.ID)+".tar.gz")
	file, err := os.Create(artifactPath)
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if _, _, err := servingstatefs.PackProject(projectPath, servingstatefs.PackProjectOptions{
		WorkspaceID:          workspaceID,
		Environment:          servingstate.DefaultEnvironment,
		ServingStateID:       created.ID,
		ManagedDataRevisions: integrationOlistManagedDataRevisions(),
	}, file); err != nil {
		_ = file.Close()
		t.Fatalf("pack artifact: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close artifact: %v", err)
	}
	bindManagedConnectionRootsInArtifact(t, artifactPath, dataDir)
	validation, err := servingstatefs.ValidateArtifactWithOptions(artifactPath, servingstate.WorkspaceID(workspaceID), created.ID, servingstatefs.ValidateOptions{
		Environment: servingstate.DefaultEnvironment,
	})
	if err != nil {
		t.Fatalf("validate artifact: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(validation.RootDir) })
	info, err := os.Stat(artifactPath)
	if err != nil {
		t.Fatalf("stat artifact: %v", err)
	}
	saved, err := repo.SaveValidated(ctx, created.ID, validation, servingstate.Artifact{
		ID:             "artifact_" + string(created.ID),
		ServingStateID: created.ID,
		WorkspaceID:    servingstate.WorkspaceID(workspaceID),
		Environment:    servingstate.DefaultEnvironment,
		Digest:         validation.Digest,
		Format:         servingstatefs.BundleFormat,
		Path:           artifactPath,
		ManifestJSON:   validation.ManifestJSON,
		SizeBytes:      info.Size(),
	})
	if err != nil {
		t.Fatalf("save validated deployment: %v", err)
	}
	active, err := repo.Activate(ctx, servingstate.WorkspaceID(workspaceID), servingstate.DefaultEnvironment, saved.ID)
	if err != nil {
		t.Fatalf("activate serving state: %v", err)
	}
	return active
}

type duckLakeIntegrationRuntimeFactory struct {
	managedRoot      string
	duckDBDir        string
	runtimeDir       string
	catalogPath      string
	duckLakeDataPath string
}

func (f duckLakeIntegrationRuntimeFactory) Prepare(_ context.Context, input runtimehost.RuntimeInput) (runtimehost.Runtime, error) {
	targetDir := filepath.Join(f.runtimeDir, string(input.State.ID))
	if err := os.RemoveAll(targetDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, err
	}
	if err := servingstatefs.ExtractArtifact(input.Artifact.Path, targetDir); err != nil {
		return nil, err
	}
	compiled, _, err := servingstatefs.LoadCompiledWorkspaceArtifact(targetDir)
	if err != nil {
		return nil, err
	}
	if err := bindManagedConnectionRoots(compiled.Definition, f.managedRoot); err != nil {
		return nil, err
	}
	service, err := dashboardruntime.NewFromDefinition(filepath.Join(f.duckDBDir, string(servingstate.NormalizeEnvironment(input.State.Environment))), duckLakeIntegrationDataRuntimeFactory{
		snapshotID:       input.State.DuckLakeSnapshotID,
		catalogPath:      f.catalogPath,
		duckLakeDataPath: f.duckLakeDataPath,
		deploymentID:     string(input.State.ID),
		workspaceID:      string(input.State.WorkspaceID),
		environment:      string(servingstate.NormalizeEnvironment(input.State.Environment)),
		semanticDigest:   input.State.Digest,
		artifactDigest:   input.Artifact.Digest,
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
			service, err = dashboardruntime.NewFromDefinition(filepath.Join(f.duckDBDir, string(servingstate.NormalizeEnvironment(input.State.Environment))), duckLakeIntegrationDataRuntimeFactory{
				snapshotID:       snapshotID,
				catalogPath:      f.catalogPath,
				duckLakeDataPath: f.duckLakeDataPath,
				deploymentID:     string(input.State.ID),
				workspaceID:      string(input.State.WorkspaceID),
				environment:      string(servingstate.NormalizeEnvironment(input.State.Environment)),
				semanticDigest:   input.State.Digest,
				artifactDigest:   input.Artifact.Digest,
			}, compiled.Definition)
			if err != nil {
				return nil, err
			}
		}
	}
	return service, nil
}

type duckLakeIntegrationDataRuntimeFactory struct {
	snapshotID       int64
	catalogPath      string
	duckLakeDataPath string
	deploymentID     string
	workspaceID      string
	environment      string
	semanticDigest   string
	artifactDigest   string
}

func (f duckLakeIntegrationDataRuntimeFactory) OpenDashboardWorkspaceDataRuntimes(ctx context.Context, config dashboardruntime.WorkspaceDataRuntimeConfig) (map[string]dashboardruntime.DataRuntime, error) {
	runtime, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, analyticsduckdb.WorkspaceRuntimeConfig{
		Models:           config.Definition.Models,
		DBDir:            config.DBDir,
		CatalogPath:      f.catalogPath,
		DuckLakeDataPath: f.duckLakeDataPath,
		SnapshotID:       f.snapshotID,
		ServingStateID:   f.deploymentID,
		WorkspaceID:      f.workspaceID,
		Environment:      f.environment,
		SemanticDigest:   f.semanticDigest,
		ArtifactDigest:   f.artifactDigest,
	})
	if err != nil {
		return nil, err
	}
	closer := &sharedDuckLakeRuntimeCloser{runtime: runtime}
	runtimes := make(map[string]dashboardruntime.DataRuntime, len(config.Definition.Models))
	for modelID := range config.Definition.Models {
		queries, err := runtime.Queries(modelID)
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtimes[modelID] = duckLakeIntegrationDataRuntime{
			modelID: modelID,
			runtime: runtime,
			close:   closer,
			data:    reportdef.NewDataQueryService(modelID, reportdef.NewAnalyticsDataService(queries), runtime),
		}
	}
	return runtimes, nil
}

func (duckLakeIntegrationDataRuntimeFactory) OpenDashboardDataRuntime(context.Context, dashboardruntime.DataRuntimeConfig) (dashboardruntime.DataRuntime, error) {
	return nil, fmt.Errorf("integration requires workspace data runtime")
}

type sharedDuckLakeRuntimeCloser struct {
	once    sync.Once
	runtime *analyticsduckdb.WorkspaceRuntime
	err     error
}

func (c *sharedDuckLakeRuntimeCloser) Close() error {
	if c == nil || c.runtime == nil {
		return nil
	}
	c.once.Do(func() {
		c.err = c.runtime.Close()
		c.runtime = nil
	})
	return c.err
}

type duckLakeIntegrationDataRuntime struct {
	modelID string
	runtime *analyticsduckdb.WorkspaceRuntime
	close   *sharedDuckLakeRuntimeCloser
	data    reportdef.DataService
}

func (r duckLakeIntegrationDataRuntime) Query(ctx context.Context, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return r.data.Query(ctx, request)
}

func (r duckLakeIntegrationDataRuntime) Rows(ctx context.Context, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return r.data.Rows(ctx, request)
}

func (r duckLakeIntegrationDataRuntime) Count(ctx context.Context, request reportdef.CountQuery) (int, error) {
	return r.data.Count(ctx, request)
}

func (r duckLakeIntegrationDataRuntime) Histogram(ctx context.Context, request reportdef.RawValueQuery, binCount int) ([]reportdef.HistogramBin, error) {
	return r.data.Histogram(ctx, request, binCount)
}

func (r duckLakeIntegrationDataRuntime) Distribution(ctx context.Context, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) (reportdef.QueryRows, error) {
	return r.data.Distribution(ctx, request, sort, limit)
}

func (r duckLakeIntegrationDataRuntime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	return r.runtime.ExecuteDataQuery(ctx, request)
}

func (r duckLakeIntegrationDataRuntime) Refresh(ctx context.Context) error {
	return r.runtime.Refresh(ctx)
}

func (r duckLakeIntegrationDataRuntime) RefreshTables(ctx context.Context, tableNames []string) error {
	return r.runtime.RefreshModelTables(ctx, r.modelID, tableNames)
}

func (r duckLakeIntegrationDataRuntime) Close() error {
	return r.close.Close()
}

func (r duckLakeIntegrationDataRuntime) LastRefresh() time.Time {
	return r.runtime.LastRefresh()
}

func (r duckLakeIntegrationDataRuntime) DuckLakeSnapshotID() int64 {
	return r.runtime.DuckLakeSnapshotID()
}

func TestDuckLakeAtomicRefreshCutover(t *testing.T) {
	h := newDuckLakeHarness(t)
	ordersAssetID := integrationAssetID(t, h.store, "sales", "model_table", "sales.orders")

	initialRevenue := h.queryRevenue(t)
	if initialRevenue != 165 {
		t.Fatalf("initial revenue = %v, want 165", initialRevenue)
	}
	initialSnapshot := h.activeSnapshot(t)
	if initialSnapshot <= 0 {
		t.Fatalf("initial snapshot = %d, want positive", initialSnapshot)
	}

	writeMutatedOlistFixture(t, h.dataDir)
	if got := h.postAuthenticated(t, "/workspaces/sales/assets/"+ordersAssetID+"/refresh"); got != http.StatusNoContent {
		t.Fatalf("refresh status = %d, want %d", got, http.StatusNoContent)
	}
	run := h.waitLatestRun(t, analyticsmaterialize.TargetModelTable, "sales.orders", analyticsmaterialize.RunStatusSucceeded)
	if run.ServingStateID == "" {
		t.Fatalf("run has no deployment id: %#v", run)
	}
	newRevenue := h.queryRevenue(t)
	if newRevenue != 265 {
		t.Fatalf("new revenue = %v, want 265", newRevenue)
	}
	newSnapshot := h.activeSnapshot(t)
	if newSnapshot <= initialSnapshot {
		t.Fatalf("new snapshot = %d, want greater than initial %d", newSnapshot, initialSnapshot)
	}
	fileCount, fileBytes, tableCount, snapshotCount, storedDataPath := h.duckLakeCatalogSummary(t)
	if tableCount == 0 || snapshotCount == 0 {
		t.Fatalf("DuckLake catalog has %d active tables / %d snapshots, want nonzero", tableCount, snapshotCount)
	}
	if fileCount == 0 {
		t.Logf("DuckLake catalog has no active data files for this tiny fixture; active tables=%d snapshots=%d bytes=%d", tableCount, snapshotCount, fileBytes)
	}
	if filepath.Clean(storedDataPath) != filepath.Clean(h.dataPath) {
		t.Fatalf("DuckLake metadata data_path = %q, want %q", storedDataPath, h.dataPath)
	}
}

func TestAdminStorageReflectsDuckLakeAfterCleanup(t *testing.T) {
	h := newDuckLakeHarness(t)
	legacyDir := filepath.Join(h.duckDBDir, "dev")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "libredash-stale.duckdb"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy duckdb file: %v", err)
	}
	body := h.getAuthenticatedHydrated(t, "/admin/storage")
	for _, want := range []string{"DuckLake catalog", "model", "orders", "Snapshots", "Tables"} {
		if !strings.Contains(body, want) {
			t.Fatalf("admin storage missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "libredash-stale.duckdb") {
		t.Fatalf("admin storage exposed legacy duckdb artifact:\n%s", body)
	}
}

func TestGlobalReadExecutionAuditsQueueTelemetry(t *testing.T) {
	h := newDuckLakeHarness(t)
	req := h.authedJSONRequest(t, http.MethodPost, "/api/v1/workspaces/sales/semantic-models/sales/query", `{"measures":[{"field":"revenue"}],"limit":1}`)
	req.Header.Set("X-Request-ID", "integration-read-telemetry")
	res, body := h.do(t, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("query status = %d body=%s", res.StatusCode, body)
	}
	repo := queryauditsqlite.NewRepository(h.store.SQLDB())
	events, err := repo.ListQueryEvents(context.Background(), queryaudit.Filter{
		WorkspaceID: "sales",
		Search:      "integration-read-telemetry",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list query events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one query event", events)
	}
	event := events[0]
	if event.Surface != dataquery.SurfaceAPI || event.Operation != dataquery.OperationAPIQuery || event.ExecutionState != dataquery.ExecutionSucceeded || event.ExecutionMS <= 0 {
		t.Fatalf("query event telemetry = %#v", event)
	}
}

func TestReadOverloadDoesNotBlockWriteRefresh(t *testing.T) {
	executor := execution.New(execution.Config{
		MaxRunningReads: 1,
		MaxQueuedReads:  -1,
		ReadQueueWait:   50 * time.Millisecond,
		MaxRunningJobs:  1,
		MaxQueuedJobs:   4,
	})
	h := newDuckLakeHarness(t, func(options *app.Options) {
		options.Executor = executor
	})
	started := make(chan struct{})
	release := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		_, err := executor.SubmitRead(context.Background(), dataquery.Query{Kind: dataquery.KindSemanticAggregate}, func(context.Context) (dataquery.Result, error) {
			close(started)
			<-release
			return dataquery.Result{}, nil
		})
		readDone <- err
	}()
	<-started
	req := h.authedJSONRequest(t, http.MethodPost, "/api/v1/workspaces/sales/semantic-models/sales/query", `{"measures":[{"field":"revenue"}],"limit":1}`)
	res, body := h.do(t, req)
	if res.StatusCode == http.StatusOK || !strings.Contains(body, execution.ErrReadQueueFull.Error()) {
		close(release)
		t.Fatalf("overloaded read status=%d body=%s, want read queue full error", res.StatusCode, body)
	}
	writeMutatedOlistFixture(t, h.dataDir)
	ordersAssetID := integrationAssetID(t, h.store, "sales", "model_table", "sales.orders")
	if got := h.postAuthenticated(t, "/workspaces/sales/assets/"+ordersAssetID+"/refresh"); got != http.StatusNoContent {
		close(release)
		t.Fatalf("refresh status = %d", got)
	}
	h.waitLatestRun(t, analyticsmaterialize.TargetModelTable, "sales.orders", analyticsmaterialize.RunStatusSucceeded)
	close(release)
	if err := <-readDone; err != nil {
		t.Fatalf("held read returned error: %v", err)
	}
}

func TestDuckLakeCleanupProtectsLeasedSnapshots(t *testing.T) {
	h := newDuckLakeHarness(t)
	ctx := context.Background()
	initial := h.activeSnapshot(t)
	leaseID, err := h.deployments.CreateQuerySnapshotLease(ctx, servingstate.SnapshotLeaseInput{
		WorkspaceID:        "sales",
		Environment:        servingstate.DefaultEnvironment,
		ServingStateID:     h.activeServingStateID(t),
		DuckLakeSnapshotID: initial,
		OwnerID:            "integration",
		ExpiresAt:          time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("create query snapshot lease: %v", err)
	}
	writeMutatedOlistFixture(t, h.dataDir)
	ordersAssetID := integrationAssetID(t, h.store, "sales", "model_table", "sales.orders")
	if got := h.postAuthenticated(t, "/workspaces/sales/assets/"+ordersAssetID+"/refresh"); got != http.StatusNoContent {
		t.Fatalf("refresh status = %d", got)
	}
	h.waitLatestRun(t, analyticsmaterialize.TargetModelTable, "sales.orders", analyticsmaterialize.RunStatusSucceeded)
	report, err := storagemaintenance.Run(ctx, h.deployments, storagemaintenance.Options{RootDir: h.homeDir, CatalogPath: h.catalogPath, DataPath: h.dataPath, DryRun: true})
	if err != nil {
		t.Fatalf("cleanup dry-run: %v", err)
	}
	if !containsSnapshot(report.LeaseProtectedSnapshots, initial) {
		t.Fatalf("leased snapshots = %#v, want %d", report.LeaseProtectedSnapshots, initial)
	}
	if err := h.deployments.ReleaseQuerySnapshotLease(ctx, leaseID); err != nil {
		t.Fatalf("release lease: %v", err)
	}
	report, err = storagemaintenance.Run(ctx, h.deployments, storagemaintenance.Options{RootDir: h.homeDir, CatalogPath: h.catalogPath, DataPath: h.dataPath, DryRun: false})
	if err != nil {
		t.Fatalf("cleanup apply: %v", err)
	}
	if containsSnapshot(report.ProtectedSnapshots, initial) {
		t.Fatalf("old snapshot %d still protected after lease release: %#v", initial, report)
	}
}

func TestDuckLakeSnapshotProtectedByRunningQueryLease(t *testing.T) {
	h := newDuckLakeHarness(t)
	ctx := context.Background()
	provider := h.registry.ProviderForWorkspace("sales")
	lease, err := provider.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire runtime lease: %v", err)
	}
	initial := lease.DuckLakeSnapshotID()
	if initial <= 0 {
		t.Fatalf("lease snapshot = %d, want positive", initial)
	}
	writeMutatedOlistFixture(t, h.dataDir)
	ordersAssetID := integrationAssetID(t, h.store, "sales", "model_table", "sales.orders")
	if got := h.postAuthenticated(t, "/workspaces/sales/assets/"+ordersAssetID+"/refresh"); got != http.StatusNoContent {
		lease.Release()
		t.Fatalf("refresh status = %d", got)
	}
	h.waitLatestRun(t, analyticsmaterialize.TargetModelTable, "sales.orders", analyticsmaterialize.RunStatusSucceeded)
	if got := h.activeSnapshot(t); got <= initial {
		lease.Release()
		t.Fatalf("active snapshot = %d, want newer than leased snapshot %d", got, initial)
	}
	report, err := storagemaintenance.Run(ctx, h.deployments, storagemaintenance.Options{RootDir: h.homeDir, CatalogPath: h.catalogPath, DataPath: h.dataPath, DryRun: true})
	if err != nil {
		lease.Release()
		t.Fatalf("cleanup dry-run: %v", err)
	}
	if !containsSnapshot(report.LeaseProtectedSnapshots, initial) {
		lease.Release()
		t.Fatalf("lease-protected snapshots = %#v, want %d", report.LeaseProtectedSnapshots, initial)
	}
	lease.Release()
	report, err = storagemaintenance.Run(ctx, h.deployments, storagemaintenance.Options{RootDir: h.homeDir, CatalogPath: h.catalogPath, DataPath: h.dataPath, DryRun: false})
	if err != nil {
		t.Fatalf("cleanup apply after lease release: %v", err)
	}
	if containsSnapshot(report.ProtectedSnapshots, initial) {
		t.Fatalf("old snapshot %d stayed protected after final lease release: %#v", initial, report)
	}
}

func TestDuckLakeCleanupRemovesUnleasedStaleSnapshots(t *testing.T) {
	h := newDuckLakeHarness(t)
	ctx := context.Background()
	provider := h.registry.ProviderForWorkspace("sales")
	lease, err := provider.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire runtime lease: %v", err)
	}
	initial := lease.DuckLakeSnapshotID()
	writeMutatedOlistFixture(t, h.dataDir)
	ordersAssetID := integrationAssetID(t, h.store, "sales", "model_table", "sales.orders")
	if got := h.postAuthenticated(t, "/workspaces/sales/assets/"+ordersAssetID+"/refresh"); got != http.StatusNoContent {
		lease.Release()
		t.Fatalf("refresh status = %d", got)
	}
	h.waitLatestRun(t, analyticsmaterialize.TargetModelTable, "sales.orders", analyticsmaterialize.RunStatusSucceeded)
	if !containsSnapshot(h.duckLakeSnapshotIDs(t), initial) {
		lease.Release()
		t.Fatalf("snapshot %d disappeared while lease was still held", initial)
	}
	lease.Release()
	report, err := storagemaintenance.Run(ctx, h.deployments, storagemaintenance.Options{RootDir: h.homeDir, CatalogPath: h.catalogPath, DataPath: h.dataPath, DryRun: false})
	if err != nil {
		t.Fatalf("cleanup apply: %v", err)
	}
	if containsSnapshot(report.ProtectedSnapshots, initial) {
		t.Fatalf("snapshot %d still protected after lease release: %#v", initial, report)
	}
	if containsSnapshot(h.duckLakeSnapshotIDs(t), initial) {
		t.Fatalf("snapshot %d still exists after cleanup", initial)
	}
}

func TestFailedRefreshLeavesActiveSnapshotQueryable(t *testing.T) {
	h := newDuckLakeHarness(t)
	initialRevenue := h.queryRevenue(t)
	initialSnapshot := h.activeSnapshot(t)
	writeBrokenOlistFixture(t, h.dataDir)
	ordersAssetID := integrationAssetID(t, h.store, "sales", "model_table", "sales.orders")
	if got := h.postAuthenticated(t, "/workspaces/sales/assets/"+ordersAssetID+"/refresh"); got != http.StatusNoContent {
		t.Fatalf("refresh status = %d", got)
	}
	h.waitLatestRun(t, analyticsmaterialize.TargetModelTable, "sales.orders", analyticsmaterialize.RunStatusFailed)
	if got := h.activeSnapshot(t); got != initialSnapshot {
		t.Fatalf("active snapshot = %d after failed refresh, want %d", got, initialSnapshot)
	}
	if got := h.queryRevenue(t); got != initialRevenue {
		t.Fatalf("revenue after failed refresh = %v, want previous %v", got, initialRevenue)
	}
}

func TestDurableRefreshQueueResumesAfterStartup(t *testing.T) {
	h := newDuckLakeHarness(t)
	run := h.createQueuedWorkspaceAssetRefreshRun(t, analyticsmaterialize.TargetModelTable, "sales.orders", "sales")
	h.stopBackgroundJobs(t)
	h.registry.Close()
	h.registry = nil
	h.startReplacementRegistry(t)
	stored := h.waitRun(t, run.ID, analyticsmaterialize.RunStatusSucceeded)
	if stored.Status != analyticsmaterialize.RunStatusSucceeded {
		t.Fatalf("stored run = %#v", stored)
	}
}

func TestExpiredRefreshJobLeaseIsReclaimed(t *testing.T) {
	h := newDuckLakeHarness(t)
	ctx := context.Background()
	repo := analyticsmaterializesqlite.NewSQLRunRepository(h.store.SQLDB())
	run := h.createQueuedWorkspaceAssetRefreshRun(t, analyticsmaterialize.TargetModelTable, "sales.orders", "sales")
	job, ok, err := repo.ClaimNextExecutableJob(ctx, "stale-worker", time.Second)
	if err != nil || !ok {
		t.Fatalf("claim job ok=%v err=%v", ok, err)
	}
	if _, err := h.store.SQLDB().ExecContext(ctx, `UPDATE refresh_jobs SET lease_expires_at = datetime('now', '-1 second') WHERE id = ?`, job.ID); err != nil {
		t.Fatalf("expire job lease: %v", err)
	}
	h.startReplacementRegistry(t)
	stored := h.waitRun(t, run.ID, analyticsmaterialize.RunStatusSucceeded)
	if stored.Status != analyticsmaterialize.RunStatusSucceeded {
		t.Fatalf("stored run = %#v", stored)
	}
}

func (h *duckLakeHarness) startReplacementRegistry(t *testing.T) {
	t.Helper()
	h.stopBackgroundJobs(t)
	registry := runtimehost.NewRegistryWithFactory(runtimehost.RegistryOptions{
		Repo:         h.deployments,
		WorkspaceIDs: []servingstate.WorkspaceID{"sales"},
		Environment:  servingstate.DefaultEnvironment,
		Factory: duckLakeIntegrationRuntimeFactory{
			managedRoot:      h.dataDir,
			duckDBDir:        h.duckDBDir,
			runtimeDir:       h.runtimeDir,
			catalogPath:      h.catalogPath,
			duckLakeDataPath: h.dataPath,
		},
	})
	if err := registry.Reload(context.Background()); err != nil {
		t.Fatalf("reload replacement registry: %v", err)
	}
	h.registry = registry
	server := app.NewWithOptions(app.NewDynamicRuntimeMetrics("", func(workspaceID string) app.RuntimeProvider {
		return registry.ProviderForWorkspace(servingstate.WorkspaceID(workspaceID))
	}), app.Options{
		Store:               h.store,
		ServingStateRepo:    h.deployments,
		WorkspaceRepo:       workspacesqlite.NewRepository(h.store.SQLDB()),
		AssetCatalog:        workspace.NewAssetCatalogService(workspacesqlite.NewRepository(h.store.SQLDB())),
		Auth:                app.NewAuth(accesssqlite.NewRepository(h.store.SQLDB()), "", app.AuthConfig{DevBypass: true}),
		Reloader:            registry,
		ArtifactDir:         h.artifactDir,
		DuckDBDir:           h.duckDBDir,
		DuckLakeCatalogPath: h.catalogPath,
		DuckLakeDataPath:    h.dataPath,
		DefaultWorkspaceID:  "sales",
		DefaultEnvironment:  string(servingstate.DefaultEnvironment),
	})
	backgroundCtx, stopBackground := context.WithCancel(context.Background())
	server.StartBackgroundJobs(backgroundCtx)
	h.appServer = server
	h.handler = server.Routes()
	if h.server != nil {
		h.server.Close()
	}
	h.server = httptest.NewServer(h.handler)
	t.Cleanup(h.server.Close)
	t.Cleanup(func() { _ = registry.Close() })
	t.Cleanup(func() {
		stopBackground()
		stopServerBackgroundForTest(t, server)
	})
}

func (h *duckLakeHarness) stopBackgroundJobs(t *testing.T) {
	t.Helper()
	if h == nil || h.appServer == nil {
		return
	}
	stopServerBackgroundForTest(t, h.appServer)
	h.appServer = nil
}

func stopServerBackgroundForTest(t *testing.T, server *app.Server) {
	t.Helper()
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.StopBackgroundJobs(ctx); err != nil {
		t.Errorf("stop background jobs: %v", err)
	}
}

func (h *duckLakeHarness) createQueuedWorkspaceAssetRefreshRun(t *testing.T, targetType, targetID, modelID string) analyticsmaterialize.RunRecord {
	t.Helper()
	ctx := context.Background()
	active, artifact, err := h.deployments.ActiveArtifact(ctx, "sales", servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("active artifact for refresh candidate: %v", err)
	}
	root := t.TempDir()
	if err := servingstatefs.ExtractArtifact(artifact.Path, root); err != nil {
		t.Fatalf("extract active artifact: %v", err)
	}
	compiled, _, err := servingstatefs.LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		t.Fatalf("load active compiled artifact: %v", err)
	}
	created, err := h.deployments.Create(ctx, servingstate.CreateInput{
		WorkspaceID: active.WorkspaceID,
		Environment: active.Environment,
		CreatedBy:   "integration",
		Source:      servingstate.SourceRefresh,
	})
	if err != nil {
		t.Fatalf("create refresh candidate deployment: %v", err)
	}
	candidateArtifact := servingstate.Artifact{
		ID:             "artifact_" + string(created.ID),
		ServingStateID: created.ID,
		WorkspaceID:    active.WorkspaceID,
		Environment:    active.Environment,
		Digest:         artifact.Digest,
		Format:         artifact.Format,
		Path:           artifact.Path,
		ManifestJSON:   artifact.ManifestJSON,
		SizeBytes:      artifact.SizeBytes,
	}
	var accessPolicy workspace.AccessPolicy
	if err := json.Unmarshal([]byte(active.AccessPolicyJSON), &accessPolicy); err != nil {
		t.Fatalf("decode active access policy: %v", err)
	}
	if _, err := h.deployments.SaveValidated(ctx, created.ID, servingstate.Validation{
		Digest:            active.Digest,
		ManifestJSON:      active.ManifestJSON,
		ProjectID:         active.ProjectID,
		ProjectDigest:     active.ProjectDigest,
		ProjectWorkspaces: append([]string(nil), active.ProjectWorkspaces...),
		AccessPolicy:      accessPolicy,
		Graph:             integrationRetargetAssetGraph(compiled.Graph, workspace.WorkspaceID(active.WorkspaceID), workspace.ServingStateID(created.ID)),
	}, candidateArtifact); err != nil {
		t.Fatalf("save refresh candidate deployment: %v", err)
	}
	repo := analyticsmaterializesqlite.NewSQLRunRepository(h.store.SQLDB())
	run, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{
		WorkspaceID:    "sales",
		ModelID:        modelID,
		ServingStateID: string(created.ID),
		TargetType:     targetType,
		TargetID:       targetID,
		TriggerType:    analyticsmaterialize.TriggerDirect,
		JobKind:        analyticsmaterialize.JobKindWorkspaceAssetRefresh,
		PayloadJSON:    fmt.Sprintf(`{"assetKey":%q,"assetType":%q}`, targetID, targetType),
	})
	if err != nil {
		t.Fatalf("create queued workspace asset refresh run: %v", err)
	}
	return run
}

func integrationRetargetAssetGraph(graph workspace.AssetGraph, workspaceID workspace.WorkspaceID, deploymentID workspace.ServingStateID) workspace.AssetGraph {
	out := workspace.AssetGraph{
		Assets: make([]workspace.Asset, 0, len(graph.Assets)),
		Edges:  make([]workspace.AssetEdge, 0, len(graph.Edges)),
	}
	for _, asset := range graph.Assets {
		asset.WorkspaceID = workspaceID
		asset.ServingStateID = deploymentID
		asset.SnapshotID = workspace.NewAssetSnapshotID(deploymentID, asset.ID)
		out.Assets = append(out.Assets, asset)
	}
	for _, edge := range graph.Edges {
		edge.WorkspaceID = workspaceID
		edge.ServingStateID = deploymentID
		edge.ID = workspace.NewAssetEdgeID(deploymentID, edge.FromAssetID, edge.ToAssetID, edge.Type)
		out.Edges = append(out.Edges, edge)
	}
	return out
}

func (h *duckLakeHarness) authedJSONRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, h.serverURL(t)+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer dev")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func (h *duckLakeHarness) do(t *testing.T, req *http.Request) (*http.Response, string) {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, string(body)
}

func (h *duckLakeHarness) queryRevenue(t *testing.T) float64 {
	t.Helper()
	req := h.authedJSONRequest(t, http.MethodPost, "/api/v1/workspaces/sales/semantic-models/sales/query", `{"measures":[{"field":"revenue"}],"limit":1}`)
	res, body := h.do(t, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("semantic query status=%d body=%s", res.StatusCode, body)
	}
	var decoded api.SemanticQueryResponse
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode semantic query: %v body=%s", err, body)
	}
	if len(decoded.Rows) != 1 || len(decoded.Columns) != 1 || len(decoded.Rows[0]) != 1 {
		t.Fatalf("semantic query rowset = %#v, want one cell", decoded)
	}
	value, err := strconv.ParseFloat(decoded.Rows[0][0], 64)
	if err != nil {
		t.Fatalf("parse semantic revenue %q: %v", decoded.Rows[0][0], err)
	}
	return value
}

func integrationNumberValue(t *testing.T, value any) float64 {
	t.Helper()
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		t.Fatalf("value %T %#v is not numeric", value, value)
	}
	return 0
}

func (h *duckLakeHarness) activeSnapshot(t *testing.T) int64 {
	t.Helper()
	active, _, err := h.deployments.ActiveArtifact(context.Background(), "sales", servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("active artifact: %v", err)
	}
	return active.DuckLakeSnapshotID
}

func (h *duckLakeHarness) activeServingStateID(t *testing.T) servingstate.ID {
	t.Helper()
	active, _, err := h.deployments.ActiveArtifact(context.Background(), "sales", servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("active artifact: %v", err)
	}
	return active.ID
}

func (h *duckLakeHarness) waitLatestRun(t *testing.T, targetType, targetID, status string) analyticsmaterialize.RunRecord {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	repo := analyticsmaterializesqlite.NewSQLRunRepository(h.store.SQLDB())
	lastStatus := "missing"
	lastError := ""
	for time.Now().Before(deadline) {
		runs, err := repo.ListTargetRuns(context.Background(), "sales", targetType, targetID, analyticsmaterialize.RunPage{Limit: 1})
		if err != nil {
			t.Fatalf("list target runs: %v", err)
		}
		if len(runs) > 0 {
			lastStatus = runs[0].Status
			lastError = runs[0].Error
			if runs[0].Status == status {
				return runs[0]
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s %s run status %s; last status=%s error=%q", targetType, targetID, status, lastStatus, lastError)
	return analyticsmaterialize.RunRecord{}
}

func (h *duckLakeHarness) waitRun(t *testing.T, runID, status string) analyticsmaterialize.RunRecord {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	repo := analyticsmaterializesqlite.NewSQLRunRepository(h.store.SQLDB())
	lastStatus := "missing"
	lastError := ""
	for time.Now().Before(deadline) {
		run, err := repo.GetRun(context.Background(), "sales", runID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		lastStatus = run.Status
		lastError = run.Error
		if run.Status == status {
			return run
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for run %s status %s; last status=%s error=%q", runID, status, lastStatus, lastError)
	return analyticsmaterialize.RunRecord{}
}

func writeMutatedOlistFixture(t *testing.T, dir string) {
	t.Helper()
	writeMinimalOlistFixture(t, dir)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_sequential,payment_type,payment_installments,payment_value
o1,1,credit_card,1,210.00
o2,1,boleto,1,55.00
`)
}

func writeBrokenOlistFixture(t *testing.T, dir string) {
	t.Helper()
	writeMinimalOlistFixture(t, dir)
	writeFixture(t, dir, "olist_order_payments_dataset.csv", `order_id,payment_sequential,payment_type,payment_installments
o1,1,credit_card,1
o2,1,boleto,1
`)
}

func (h *duckLakeHarness) duckLakeCatalogSummary(t *testing.T) (int64, int64, int64, int64, string) {
	t.Helper()
	db := h.openDuckLakeMetadata(t)
	defer db.Close()
	var dataPath string
	if err := db.QueryRow(`SELECT value FROM meta.ducklake_metadata WHERE "key" = 'data_path' AND scope IS NULL LIMIT 1`).Scan(&dataPath); err != nil {
		t.Fatalf("query DuckLake data path metadata: %v", err)
	}
	var files, bytes int64
	if err := db.QueryRow(`SELECT count(*), coalesce(sum(file_size_bytes), 0) FROM meta.ducklake_data_file WHERE end_snapshot IS NULL`).Scan(&files, &bytes); err != nil {
		t.Fatalf("query DuckLake data files: %v", err)
	}
	var tables, snapshots int64
	if err := db.QueryRow(`SELECT count(*) FROM meta.ducklake_table WHERE end_snapshot IS NULL`).Scan(&tables); err != nil {
		t.Fatalf("query DuckLake tables: %v", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM meta.ducklake_snapshot`).Scan(&snapshots); err != nil {
		t.Fatalf("query DuckLake snapshots: %v", err)
	}
	return files, bytes, tables, snapshots, dataPath
}

func (h *duckLakeHarness) duckLakeSnapshotIDs(t *testing.T) []int64 {
	t.Helper()
	db := h.openDuckLakeMetadata(t)
	defer db.Close()
	rows, err := db.Query(`SELECT snapshot_id FROM meta.ducklake_snapshot ORDER BY snapshot_id`)
	if err != nil {
		t.Fatalf("query DuckLake snapshots: %v", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan DuckLake snapshot: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate DuckLake snapshots: %v", err)
	}
	return ids
}

func (h *duckLakeHarness) openDuckLakeMetadata(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatalf("open DuckDB metadata connection: %v", err)
	}
	for _, stmt := range []string{
		"LOAD sqlite",
		"LOAD ducklake",
		fmt.Sprintf("ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s')", integrationSQLString(h.catalogPath), integrationSQLString(h.dataPath)),
		fmt.Sprintf("ATTACH '%s' AS meta (TYPE sqlite)", integrationSQLString(h.catalogPath)),
	} {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			_ = db.Close()
			t.Fatalf("prepare DuckLake metadata inspection %q: %v", stmt, err)
		}
	}
	return db
}

func integrationSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func containsSnapshot(values []int64, want int64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
