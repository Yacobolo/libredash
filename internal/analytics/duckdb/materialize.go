package duckdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	analyticsmaterialize "github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
)

type SourceRuntime struct {
	db                  *Database
	dataDir             string
	attachedConnections map[string]struct{}
}

func NewSourceRuntime(db *Database, dataDir string) *SourceRuntime {
	return &SourceRuntime{
		db:                  db,
		dataDir:             dataDir,
		attachedConnections: map[string]struct{}{},
	}
}

func (r *SourceRuntime) PrepareSourceRuntime(ctx context.Context, model *semanticmodel.Model) error {
	return PrepareSourceRuntime(ctx, r.db.SQLDB(), model, r.dataDir, r.attachedConnections)
}

func (r *SourceRuntime) PlanModelTable(ctx context.Context, model *semanticmodel.Model, tableName string, table semanticmodel.Table) (analyticsmaterialize.ModelTablePlan, error) {
	return PlanModelTable(ctx, r.db.SQLDB(), model, r.dataDir, tableName, table)
}

func (r *SourceRuntime) ResolveSourcePath(model *semanticmodel.Model, source semanticmodel.Source, dataDir string) (string, error) {
	return ResolveSourcePath(model, source, dataDir)
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
	sources := NewSourceRuntime(db, config.DataDir)
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
	Models  map[string]*semanticmodel.Model
	DataDir string
	DBDir   string
}

type WorkspaceRuntime struct {
	mu                   sync.Mutex
	db                   *Database
	sources              *SourceRuntime
	dataDir              string
	models               map[string]*semanticmodel.Model
	materializationModel *semanticmodel.Model
	queries              map[string]*semanticquery.Service
	lastRefresh          time.Time
}

func OpenWorkspaceMaterializeRuntime(ctx context.Context, config WorkspaceRuntimeConfig) (*WorkspaceRuntime, error) {
	if len(config.Models) == 0 {
		return nil, fmt.Errorf("workspace semantic models are required")
	}
	dbPath := analyticsmaterialize.WorkspaceDatabasePath(config.DBDir)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	if os.Getenv("LIBREDASH_DUCKDB_PATH") == "" {
		if err := removeStaleModelDatabases(config.DBDir, dbPath); err != nil {
			return nil, err
		}
	}
	db, err := Open(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	sources := NewSourceRuntime(db, config.DataDir)
	materializationModel, err := physicalWorkspaceModel(config.Models)
	if err != nil {
		db.Close()
		return nil, err
	}
	for modelID, model := range config.Models {
		if err := analyticsmaterialize.ValidateFilesWithResolver(model, config.DataDir, sources); err != nil {
			db.Close()
			return nil, fmt.Errorf("semantic model %q: %w", modelID, err)
		}
	}
	runtime := &WorkspaceRuntime{
		db:                   db,
		sources:              sources,
		dataDir:              config.DataDir,
		models:               config.Models,
		materializationModel: materializationModel,
		queries:              map[string]*semanticquery.Service{},
	}
	for modelID, model := range config.Models {
		runtime.queries[modelID] = semanticquery.NewService(semanticquery.NewPlanner(model), db)
	}
	if err := runtime.Refresh(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return runtime, nil
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

func (r *WorkspaceRuntime) Refresh(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("workspace runtime is not initialized")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	lastRefresh, err := analyticsmaterialize.Refresh(ctx, r.db, r.sources, r.materializationModel)
	if err != nil {
		return err
	}
	for modelID, model := range r.models {
		if err := DiscoverSchemasWithDataDir(ctx, r.db, model, r.dataDir); err != nil {
			return fmt.Errorf("discovering semantic model %q schemas: %w", modelID, err)
		}
	}
	r.lastRefresh = lastRefresh
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

	lastRefresh, err := analyticsmaterialize.RefreshModelTables(ctx, r.db, r.sources, model, tableNames)
	if err != nil {
		return err
	}
	for discoverModelID, discoverModel := range r.models {
		if err := DiscoverSchemasWithDataDir(ctx, r.db, discoverModel, r.dataDir); err != nil {
			return fmt.Errorf("discovering semantic model %q schemas: %w", discoverModelID, err)
		}
	}
	r.lastRefresh = lastRefresh
	return nil
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

func removeStaleModelDatabases(dbDir, workspacePath string) error {
	matches, err := filepath.Glob(filepath.Join(dbDir, "libredash-*.duckdb*"))
	if err != nil {
		return err
	}
	workspaceBase := filepath.Base(workspacePath)
	for _, match := range matches {
		base := filepath.Base(match)
		if base == workspaceBase || base == workspaceBase+".wal" {
			continue
		}
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
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
	Kind               string
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
		Kind:               table.Kind,
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
