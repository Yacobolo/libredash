package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	analyticsducklake "github.com/Yacobolo/leapview/internal/analytics/ducklake"
	analyticsmaterialize "github.com/Yacobolo/leapview/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	semanticquery "github.com/Yacobolo/leapview/internal/analytics/query"
	"github.com/Yacobolo/leapview/internal/configspec"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

type SourceRuntime struct {
	db                  sqlDBProvider
	attachedConnections map[string]struct{}
}

func NewSourceRuntime(db sqlDBProvider) *SourceRuntime {
	return &SourceRuntime{
		db:                  db,
		attachedConnections: map[string]struct{}{},
	}
}

func (r *SourceRuntime) PrepareSourceRuntime(ctx context.Context, model *semanticmodel.Model) error {
	return PrepareSourceRuntime(ctx, r.db.SQLDB(), model, r.attachedConnections)
}

func (r *SourceRuntime) SourceRelation(model *semanticmodel.Model, source semanticmodel.Source) (string, error) {
	return SourceRelation(model, source)
}

func (r *SourceRuntime) PlanModelTable(ctx context.Context, model *semanticmodel.Model, tableName string, table semanticmodel.Table) (analyticsmaterialize.ModelTablePlan, error) {
	return PlanModelTable(ctx, r.db.SQLDB(), model, tableName, table)
}

func (r *SourceRuntime) ResolveSourcePath(model *semanticmodel.Model, source semanticmodel.Source) (string, error) {
	return ResolveSourcePath(model, source)
}

func OpenMaterializeRuntime(ctx context.Context, config analyticsmaterialize.RuntimeConfig) (*analyticsmaterialize.Runtime, error) {
	dbPath := analyticsmaterialize.DatabasePath(config.DBDir, config.ModelID)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := Open(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	sources := NewSourceRuntime(db)
	config.Database = db
	config.Sources = sources
	config.Resolver = sources
	runtime, err := analyticsmaterialize.OpenRuntime(ctx, config)
	if err != nil {
		db.Close()
		return nil, err
	}
	return runtime, nil
}

type WorkspaceRuntimeConfig struct {
	Models             map[string]*semanticmodel.Model
	DBDir              string
	CatalogPath        string
	DuckLakeDataPath   string
	SnapshotID         int64
	ServingStateID     string
	WorkspaceID        string
	Environment        string
	TargetType         string
	TargetID           string
	SemanticDigest     string
	ArtifactDigest     string
	SourceDataDigest   string
	SkipInitialRefresh bool
	MaxReaders         int
}

type WorkspaceRuntime struct {
	mu                   sync.Mutex
	db                   analyticsmaterialize.Database
	sqlDB                sqlDBProvider
	committer            duckLakeCommitter
	sources              *SourceRuntime
	models               map[string]*semanticmodel.Model
	materializationModel *semanticmodel.Model
	queries              map[string]*semanticquery.Service
	views                map[string]*analyticsmaterialize.Runtime
	lastRefresh          time.Time
	lastSnapshotID       int64
	commitMetadata       map[string]string
}

type duckLakeCommitter interface {
	Commit(ctx context.Context, servingStateID string, extra map[string]string, fn func(*sql.Tx) error) (int64, error)
}

func OpenWorkspaceMaterializeRuntime(ctx context.Context, config WorkspaceRuntimeConfig) (*WorkspaceRuntime, error) {
	if len(config.Models) == 0 {
		return nil, fmt.Errorf("workspace semantic models are required")
	}
	layout := analyticsducklake.NewLayout(config.DBDir)
	if config.CatalogPath != "" {
		layout.CatalogPath = config.CatalogPath
	}
	if config.DuckLakeDataPath != "" {
		layout.DataPath = config.DuckLakeDataPath
	}
	if err := os.MkdirAll(layout.RootDir, 0o755); err != nil {
		return nil, err
	}
	var db *analyticsducklake.Environment
	var err error
	if config.SnapshotID > 0 {
		db, err = analyticsducklake.OpenSnapshot(ctx, analyticsducklake.Config{RootDir: config.DBDir, CatalogPath: layout.CatalogPath, DataPath: layout.DataPath, SnapshotID: config.SnapshotID, MaxReaders: workspaceReaderCount(config.MaxReaders)})
	} else {
		db, err = analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: config.DBDir, CatalogPath: layout.CatalogPath, DataPath: layout.DataPath})
	}
	if err != nil {
		return nil, err
	}
	sources := NewSourceRuntime(db)
	materializationModel, err := physicalWorkspaceModel(config.Models)
	if err != nil {
		db.Close()
		return nil, err
	}
	for modelID, model := range config.Models {
		if err := analyticsmaterialize.ValidateFilesWithResolver(model, sources); err != nil {
			db.Close()
			return nil, fmt.Errorf("semantic model %q: %w", modelID, err)
		}
	}
	runtime := &WorkspaceRuntime{
		db:                   db,
		sqlDB:                db,
		committer:            db,
		sources:              sources,
		models:               config.Models,
		materializationModel: materializationModel,
		queries:              map[string]*semanticquery.Service{},
		views:                map[string]*analyticsmaterialize.Runtime{},
		commitMetadata:       workspaceCommitMetadata(config),
	}
	for modelID, model := range config.Models {
		view, err := analyticsmaterialize.NewRuntimeView(ctx, analyticsmaterialize.RuntimeConfig{
			ModelID:             modelID,
			Model:               model,
			QueryCacheNamespace: workspaceQueryCacheNamespace(config),
			Database:            db,
			Sources:             sources,
			Resolver:            sources,
		})
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("compile semantic model %q runtime: %w", modelID, err)
		}
		runtime.views[modelID] = view
		runtime.queries[modelID] = view.Queries()
	}
	if config.SnapshotID > 0 {
		runtime.lastSnapshotID = config.SnapshotID
	} else if !config.SkipInitialRefresh {
		if err := runtime.Refresh(ctx); err != nil {
			db.Close()
			return nil, err
		}
	}
	return runtime, nil
}

func workspaceQueryCacheNamespace(config WorkspaceRuntimeConfig) string {
	return fmt.Sprintf(
		"snapshot=%d;serving=%q;workspace=%q;environment=%q;semantic=%q;artifact=%q;source=%q",
		config.SnapshotID,
		config.ServingStateID,
		config.WorkspaceID,
		config.Environment,
		config.SemanticDigest,
		config.ArtifactDigest,
		config.SourceDataDigest,
	)
}

func workspaceReaderCount(configured int) int {
	if configured > 0 {
		return configured
	}
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(configspec.EnvLEAPVIEW_EXEC_MAX_RUNNING_READS))); err == nil && value > 0 {
		return value
	}
	return 4
}

func (r *WorkspaceRuntime) Queries(modelID string) (*semanticquery.Service, error) {
	if r == nil {
		return nil, fmt.Errorf("workspace runtime is not initialized")
	}
	queries, ok := r.queries[modelID]
	if !ok {
		return nil, fmt.Errorf("unknown semantic model %q", modelID)
	}
	return queries, nil
}

func (r *WorkspaceRuntime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	if r == nil || r.db == nil {
		return dataquery.Result{}, fmt.Errorf("workspace runtime is not initialized")
	}
	modelID := strings.TrimSpace(request.ModelID)
	if modelID == "" && len(r.models) == 1 {
		for id := range r.models {
			modelID = id
		}
	}
	_, ok := r.models[modelID]
	if !ok {
		return dataquery.Result{}, fmt.Errorf("unknown semantic model %q", modelID)
	}
	request.ModelID = modelID
	view := r.views[modelID]
	if view == nil {
		return dataquery.Result{}, fmt.Errorf("semantic model %q runtime is not compiled", modelID)
	}
	return view.ExecuteDataQuery(ctx, request)
}

func (r *WorkspaceRuntime) ExecuteDataQueryBundle(ctx context.Context, requests []dataquery.BundleRequest) (dataquery.BundleResult, error) {
	if len(requests) == 0 {
		return dataquery.BundleResult{}, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("bundle is empty")}
	}
	modelID := strings.TrimSpace(requests[0].Query.ModelID)
	if modelID == "" && len(r.models) == 1 {
		for id := range r.models {
			modelID = id
		}
	}
	view := r.views[modelID]
	if view == nil {
		return dataquery.BundleResult{}, fmt.Errorf("semantic model %q runtime is not compiled", modelID)
	}
	for i := range requests {
		if requests[i].Query.ModelID == "" {
			requests[i].Query.ModelID = modelID
		}
		if requests[i].Query.ModelID != modelID {
			return dataquery.BundleResult{}, &dataquery.BundleIncompatibleError{Err: fmt.Errorf("bundle spans semantic models")}
		}
	}
	return view.ExecuteDataQueryBundle(ctx, requests)
}

func (r *WorkspaceRuntime) Refresh(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("workspace runtime is not initialized")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	lastRefresh, snapshotID, err := r.refreshModel(ctx, r.materializationModel, nil)
	if err != nil {
		return err
	}
	r.clearQueryCaches()
	for modelID, model := range r.models {
		if err := discoverSchemas(ctx, r.sqlDB, model); err != nil {
			return fmt.Errorf("discovering semantic model %q schemas: %w", modelID, err)
		}
	}
	r.lastRefresh = lastRefresh
	r.lastSnapshotID = snapshotID
	return nil
}

func (r *WorkspaceRuntime) RefreshModelTables(ctx context.Context, modelID string, tableNames []string) error {
	if r == nil {
		return fmt.Errorf("workspace runtime is not initialized")
	}
	model, ok := r.models[modelID]
	if !ok {
		return fmt.Errorf("unknown semantic model %q", modelID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	lastRefresh, snapshotID, err := r.refreshModel(ctx, model, tableNames)
	if err != nil {
		return err
	}
	r.clearQueryCaches()
	for discoverModelID, discoverModel := range r.models {
		if err := discoverSchemas(ctx, r.sqlDB, discoverModel); err != nil {
			return fmt.Errorf("discovering semantic model %q schemas: %w", discoverModelID, err)
		}
	}
	r.lastRefresh = lastRefresh
	r.lastSnapshotID = snapshotID
	return nil
}

func (r *WorkspaceRuntime) RefreshWorkspaceTables(ctx context.Context, tableNames []string) error {
	if r == nil {
		return fmt.Errorf("workspace runtime is not initialized")
	}
	if len(tableNames) == 0 {
		return fmt.Errorf("model table refresh plan is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	lastRefresh, snapshotID, err := r.refreshModel(ctx, r.materializationModel, tableNames)
	if err != nil {
		return err
	}
	r.clearQueryCaches()
	for discoverModelID, discoverModel := range r.models {
		if err := discoverSchemas(ctx, r.sqlDB, discoverModel); err != nil {
			return fmt.Errorf("discovering semantic model %q schemas: %w", discoverModelID, err)
		}
	}
	r.lastRefresh = lastRefresh
	r.lastSnapshotID = snapshotID
	return nil
}

func (r *WorkspaceRuntime) clearQueryCaches() {
	for _, view := range r.views {
		view.ClearQueryCache()
	}
}

func WorkspaceModelTableDependencyOrder(models map[string]*semanticmodel.Model, selectedTable string) ([]string, error) {
	model, err := physicalWorkspaceModel(models)
	if err != nil {
		return nil, err
	}
	return analyticsmaterialize.ModelTableDependencyOrder(model, selectedTable)
}

func (r *WorkspaceRuntime) refreshModel(ctx context.Context, model *semanticmodel.Model, tableNames []string) (time.Time, int64, error) {
	if r.committer == nil {
		if len(tableNames) > 0 {
			lastRefresh, err := analyticsmaterialize.RefreshModelTables(ctx, r.db, r.sources, model, tableNames)
			return lastRefresh, 0, err
		}
		lastRefresh, err := analyticsmaterialize.Refresh(ctx, r.db, r.sources, model)
		return lastRefresh, 0, err
	}
	if err := r.sources.PrepareSourceRuntime(ctx, model); err != nil {
		return time.Time{}, 0, err
	}
	metadata := map[string]string{"workspace": model.Name}
	for key, value := range r.commitMetadata {
		metadata[key] = value
	}
	servingStateID := firstNonEmpty(r.commitMetadata["servingStateId"], "workspace-refresh")
	snapshotID, err := r.committer.Commit(ctx, servingStateID, metadata, func(tx *sql.Tx) error {
		executor := txExecutor{tx: tx}
		sources := txSourceRuntime{SourceRuntime: r.sources, tx: tx}
		if len(tableNames) > 0 {
			return analyticsmaterialize.ModelTablesNamed(ctx, executor, sources, model, tableNames)
		}
		return analyticsmaterialize.ModelTables(ctx, executor, sources, model)
	})
	if err != nil {
		return time.Time{}, 0, err
	}
	return time.Now(), snapshotID, nil
}

func workspaceCommitMetadata(config WorkspaceRuntimeConfig) map[string]string {
	metadata := map[string]string{}
	addCommitMetadata(metadata, "servingStateId", config.ServingStateID)
	addCommitMetadata(metadata, "workspaceId", config.WorkspaceID)
	addCommitMetadata(metadata, "environment", config.Environment)
	addCommitMetadata(metadata, "targetType", config.TargetType)
	addCommitMetadata(metadata, "targetId", config.TargetID)
	addCommitMetadata(metadata, "semanticModelDigest", config.SemanticDigest)
	addCommitMetadata(metadata, "artifactDigest", config.ArtifactDigest)
	addCommitMetadata(metadata, "sourceDataDigest", config.SourceDataDigest)
	return metadata
}

func addCommitMetadata(metadata map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		metadata[key] = value
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (r *WorkspaceRuntime) Close() error {
	if r == nil {
		return nil
	}
	return r.db.Close()
}

func (r *WorkspaceRuntime) LastRefresh() time.Time {
	if r == nil {
		return time.Time{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastRefresh
}

func (r *WorkspaceRuntime) DBPath() string {
	if r == nil || r.db == nil {
		return ""
	}
	return r.db.Path()
}

func (r *WorkspaceRuntime) DuckLakeSnapshotID() int64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastSnapshotID
}

func (r *WorkspaceRuntime) ReadConcurrency() int {
	if r == nil {
		return 1
	}
	r.mu.Lock()
	snapshotID := r.lastSnapshotID
	r.mu.Unlock()
	if snapshotID <= 0 {
		return 1
	}
	if concurrency, ok := r.db.(interface{ ReadConcurrency() int }); ok {
		return max(1, concurrency.ReadConcurrency())
	}
	return 1
}

type txExecutor struct {
	tx *sql.Tx
}

func (e txExecutor) Exec(ctx context.Context, statement string) error {
	_, err := e.tx.ExecContext(ctx, statement)
	return err
}

type txSourceRuntime struct {
	*SourceRuntime
	tx *sql.Tx
}

func (r txSourceRuntime) PlanModelTable(ctx context.Context, model *semanticmodel.Model, tableName string, table semanticmodel.Table) (analyticsmaterialize.ModelTablePlan, error) {
	return PlanModelTable(ctx, r.tx, model, tableName, table)
}

func physicalWorkspaceModel(models map[string]*semanticmodel.Model) (*semanticmodel.Model, error) {
	workspaceModel := &semanticmodel.Model{
		Name:              "workspace",
		DefaultConnection: "",
		Connections:       map[string]semanticmodel.Connection{},
		Sources:           map[string]semanticmodel.Source{},
		Tables:            map[string]semanticmodel.Table{},
		Measures:          map[string]semanticmodel.MetricMeasure{},
	}
	for modelID, model := range models {
		if model == nil {
			return nil, fmt.Errorf("semantic model %q is required", modelID)
		}
		if workspaceModel.DefaultConnection == "" {
			workspaceModel.DefaultConnection = model.DefaultConnection
		}
		for name, connection := range model.Connections {
			existing, ok := workspaceModel.Connections[name]
			if ok && !reflect.DeepEqual(existing, connection) {
				return nil, fmt.Errorf("semantic model %q connection %q conflicts with another workspace model", modelID, name)
			}
			workspaceModel.Connections[name] = connection
		}
		for name, source := range model.Sources {
			existing, ok := workspaceModel.Sources[name]
			if ok && !reflect.DeepEqual(sourcePhysicalSignature(existing), sourcePhysicalSignature(source)) {
				return nil, fmt.Errorf("semantic model %q source %q conflicts with another workspace model", modelID, name)
			}
			workspaceModel.Sources[name] = source
		}
		for name, table := range model.Tables {
			existing, ok := workspaceModel.Tables[name]
			if ok && !reflect.DeepEqual(tablePhysicalSignature(existing), tablePhysicalSignature(table)) {
				return nil, fmt.Errorf("semantic model %q model table %q conflicts with another workspace model", modelID, name)
			}
			workspaceModel.Tables[name] = table
		}
	}
	return workspaceModel, nil
}

func sourcePhysicalSignature(source semanticmodel.Source) semanticmodel.Source {
	source.Description = ""
	source.Fields = nil
	source.Schema = semanticmodel.TableSchema{}
	return source
}

type tablePhysicalSignatureValue struct {
	Source             string
	Sources            []string
	SQL                string
	Transform          semanticmodel.Transform
	Columns            map[string]semanticmodel.ModelColumn
	PrimaryKey         string
	Grain              string
	SourceDependencies []string
	ModelDependencies  []string
}

func tablePhysicalSignature(table semanticmodel.Table) tablePhysicalSignatureValue {
	return tablePhysicalSignatureValue{
		Source:             table.Source,
		Sources:            append([]string{}, table.Sources...),
		SQL:                table.SQL,
		Transform:          table.Transform,
		Columns:            table.Columns,
		PrimaryKey:         table.PrimaryKey,
		Grain:              table.Grain,
		SourceDependencies: append([]string{}, table.SourceDependencies...),
		ModelDependencies:  append([]string{}, table.ModelDependencies...),
	}
}
