package duckdb

import (
	"context"
	"os"
	"path/filepath"

	analyticsmaterialize "github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
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

func (r *SourceRuntime) SourceRelation(model *semanticmodel.Model, source semanticmodel.Source, dataDir string) (string, error) {
	return SourceRelation(model, source, dataDir)
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
