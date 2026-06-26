package materialize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
)

type RuntimeConfig struct {
	ModelID string
	Model   *semanticmodel.Model
	DataDir string
	DBDir   string

	Database Database
	Sources  SourceRegistrar
	Resolver SourcePathResolver
}

type Runtime struct {
	model       *semanticmodel.Model
	db          Database
	sources     SourceRegistrar
	queries     *semanticquery.Service
	lastRefresh time.Time
}

type Database interface {
	Executor
	semanticquery.Executor
	Close() error
	Path() string
}

type schemaDiscoverer interface {
	DiscoverSchemas(context.Context, *semanticmodel.Model) error
}

func OpenRuntime(ctx context.Context, config RuntimeConfig) (*Runtime, error) {
	if config.Model == nil {
		return nil, fmt.Errorf("semantic model is required")
	}
	if config.Database == nil {
		return nil, fmt.Errorf("materialization database is required")
	}
	if config.Sources == nil {
		return nil, fmt.Errorf("source registrar is required")
	}
	resolver := config.Resolver
	if resolver == nil {
		resolver = defaultSourcePathResolver{}
	}
	if err := ValidateFilesWithResolver(config.Model, config.DataDir, resolver); err != nil {
		return nil, err
	}
	runtime := &Runtime{
		model:   config.Model,
		db:      config.Database,
		sources: config.Sources,
		queries: semanticquery.NewService(semanticquery.NewPlanner(config.Model), config.Database),
	}
	if err := runtime.Refresh(ctx); err != nil {
		config.Database.Close()
		return nil, err
	}
	return runtime, nil
}

func DatabasePath(dbDir, modelID string) string {
	if path := os.Getenv("LIBREDASH_DUCKDB_PATH"); path != "" {
		return path
	}
	return filepath.Join(dbDir, "libredash-"+modelID+".duckdb")
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	return r.db.Close()
}

func (r *Runtime) Refresh(ctx context.Context) error {
	lastRefresh, err := Refresh(ctx, r.db, r.sources, r.model)
	if err != nil {
		return err
	}
	if discoverer, ok := r.db.(schemaDiscoverer); ok {
		if err := discoverer.DiscoverSchemas(ctx, r.model); err != nil {
			return err
		}
	}
	r.lastRefresh = lastRefresh
	return nil
}

func (r *Runtime) RefreshModelTables(ctx context.Context, tableNames []string) error {
	lastRefresh, err := RefreshModelTables(ctx, r.db, r.sources, r.model, tableNames)
	if err != nil {
		return err
	}
	if discoverer, ok := r.db.(schemaDiscoverer); ok {
		if err := discoverer.DiscoverSchemas(ctx, r.model); err != nil {
			return err
		}
	}
	r.lastRefresh = lastRefresh
	return nil
}

func (r *Runtime) Queries() *semanticquery.Service {
	if r == nil {
		return nil
	}
	return r.queries
}

func (r *Runtime) LastRefresh() time.Time {
	if r == nil {
		return time.Time{}
	}
	return r.lastRefresh
}

func (r *Runtime) DBPath() string {
	if r == nil {
		return ""
	}
	return r.db.Path()
}
