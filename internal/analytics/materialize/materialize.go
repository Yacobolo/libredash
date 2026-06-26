package materialize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
)

type Executor interface {
	Exec(ctx context.Context, statement string) error
}

type SourceRegistrar interface {
	PrepareSourceRuntime(ctx context.Context, model *semanticmodel.Model) error
	PlanModelTable(ctx context.Context, model *semanticmodel.Model, tableName string, table semanticmodel.Table) (ModelTablePlan, error)
}

type ModelTablePlan struct {
	Mode string
	SQL  string
}

const (
	PlanModeDirectSourceRead      = "direct_source_read"
	PlanModeProjectedSourceInline = "projected_source_inline"
	PlanModeWholeQueryPushdown    = "whole_query_pushdown"
	PlanModeModelSQL              = "model_sql"
)

type SourcePathResolver interface {
	ResolveSourcePath(model *semanticmodel.Model, source semanticmodel.Source, dataDir string) (string, error)
}

type MissingDataError struct {
	DataDir string
	Missing []string
}

func (e *MissingDataError) Error() string {
	return fmt.Sprintf("local source files are missing in %s: %s. Run the workspace bootstrap script or set LIBREDASH_DATA_DIR.", e.DataDir, strings.Join(e.Missing, ", "))
}

func (e *MissingDataError) SetupRequired() bool {
	return true
}

func Refresh(ctx context.Context, executor Executor, sources SourceRegistrar, model *semanticmodel.Model) (time.Time, error) {
	if executor == nil {
		return time.Time{}, fmt.Errorf("materialization executor is required")
	}
	if sources == nil {
		return time.Time{}, fmt.Errorf("source registrar is required")
	}
	if err := sources.PrepareSourceRuntime(ctx, model); err != nil {
		return time.Time{}, err
	}
	if err := ModelTables(ctx, executor, sources, model); err != nil {
		return time.Time{}, err
	}
	return time.Now(), nil
}

func RefreshModelTables(ctx context.Context, executor Executor, sources SourceRegistrar, model *semanticmodel.Model, tableNames []string) (time.Time, error) {
	if executor == nil {
		return time.Time{}, fmt.Errorf("materialization executor is required")
	}
	if sources == nil {
		return time.Time{}, fmt.Errorf("source registrar is required")
	}
	if err := sources.PrepareSourceRuntime(ctx, model); err != nil {
		return time.Time{}, err
	}
	if err := ModelTablesNamed(ctx, executor, sources, model, tableNames); err != nil {
		return time.Time{}, err
	}
	return time.Now(), nil
}

func ValidateFiles(model *semanticmodel.Model, dataDir string) error {
	return ValidateFilesWithResolver(model, dataDir, defaultSourcePathResolver{})
}

func ValidateFilesWithResolver(model *semanticmodel.Model, dataDir string, resolver SourcePathResolver) error {
	if resolver == nil {
		return fmt.Errorf("source path resolver is required")
	}
	var missing []string
	for name, source := range model.Sources {
		if source.Path == "" {
			continue
		}
		connection := model.Connections[source.Connection]
		if connection.Kind != "local" {
			continue
		}
		file, err := resolver.ResolveSourcePath(model, source, dataDir)
		if err != nil {
			return fmt.Errorf("resolving local source %s: %w", name, err)
		}
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			missing = append(missing, file)
		} else if err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return &MissingDataError{DataDir: dataDir, Missing: missing}
	}
	return nil
}

func ResolveSourcePath(model *semanticmodel.Model, source semanticmodel.Source, dataDir string) (string, error) {
	return defaultSourcePathResolver{}.ResolveSourcePath(model, source, dataDir)
}

type defaultSourcePathResolver struct{}

func (defaultSourcePathResolver) ResolveSourcePath(model *semanticmodel.Model, source semanticmodel.Source, dataDir string) (string, error) {
	connection := model.Connections[source.Connection]
	switch connection.Kind {
	case "local":
		if filepath.IsAbs(source.Path) {
			return source.Path, nil
		}
		root := connection.Root
		if root == "" {
			root = dataDir
		} else if !filepath.IsAbs(root) {
			root = filepath.Join(dataDir, root)
		}
		return filepath.Join(root, source.Path), nil
	default:
		if connection.Scope == "" {
			return source.Path, nil
		}
		if semanticmodel.IsLocalPath(source.Path) {
			return semanticmodel.JoinScope(connection.Scope, source.Path), nil
		}
		if !semanticmodel.WithinScope(connection.Scope, source.Path) {
			return "", fmt.Errorf("path %q is outside connection %q scope %q", source.Path, source.Connection, connection.Scope)
		}
		return source.Path, nil
	}
}

func ModelTables(ctx context.Context, executor Executor, sources SourceRegistrar, model *semanticmodel.Model) error {
	if executor == nil {
		return fmt.Errorf("materialization executor is required")
	}
	if sources == nil {
		return fmt.Errorf("source registrar is required")
	}
	order, err := ModelTableOrder(model)
	if err != nil {
		return err
	}
	return ModelTablesNamed(ctx, executor, sources, model, order)
}

func ModelTablesNamed(ctx context.Context, executor Executor, sources SourceRegistrar, model *semanticmodel.Model, tableNames []string) error {
	if executor == nil {
		return fmt.Errorf("materialization executor is required")
	}
	if sources == nil {
		return fmt.Errorf("source registrar is required")
	}
	if model == nil {
		return fmt.Errorf("semantic model is required")
	}
	if err := executor.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS model"); err != nil {
		return err
	}
	for _, name := range tableNames {
		if err := validateIdentifier(name); err != nil {
			return err
		}
		if _, ok := model.Tables[name]; !ok {
			return fmt.Errorf("unknown model table %q", name)
		}
		if err := materializeModelTable(ctx, executor, sources, model, name); err != nil {
			return err
		}
	}
	return nil
}

func ModelTableDependencyOrder(model *semanticmodel.Model, selectedTable string) ([]string, error) {
	selectedTable = strings.TrimSpace(selectedTable)
	if selectedTable == "" {
		return nil, fmt.Errorf("model table is required")
	}
	if model == nil {
		return nil, fmt.Errorf("semantic model is required")
	}
	temporary := map[string]bool{}
	permanent := map[string]bool{}
	order := []string{}
	var visit func(string) error
	visit = func(name string) error {
		if permanent[name] {
			return nil
		}
		if temporary[name] {
			return fmt.Errorf("model table dependency cycle includes %q", name)
		}
		table, ok := model.Tables[name]
		if !ok {
			return fmt.Errorf("unknown model table %q", name)
		}
		temporary[name] = true
		for _, dependency := range table.ModelDependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		temporary[name] = false
		permanent[name] = true
		order = append(order, name)
		return nil
	}
	if err := visit(selectedTable); err != nil {
		return nil, err
	}
	return order, nil
}

func materializeModelTable(ctx context.Context, executor Executor, sources SourceRegistrar, model *semanticmodel.Model, name string) error {
	table := model.Tables[name]
	plan, err := sources.PlanModelTable(ctx, model, name, table)
	if err != nil {
		return err
	}
	if plan.SQL == "" {
		return fmt.Errorf("model table %q produced empty materialization SQL", name)
	}
	if err := executor.Exec(ctx, plan.SQL); err != nil {
		return fmt.Errorf("materializing model.%s: %w", name, err)
	}
	return nil
}

func ModelTableOrder(model *semanticmodel.Model) ([]string, error) {
	if model == nil {
		return nil, fmt.Errorf("semantic model is required")
	}
	temporary := map[string]bool{}
	permanent := map[string]bool{}
	order := []string{}
	var visit func(string) error
	visit = func(name string) error {
		if permanent[name] {
			return nil
		}
		if temporary[name] {
			return fmt.Errorf("model table dependency cycle includes %q", name)
		}
		table, ok := model.Tables[name]
		if !ok {
			return fmt.Errorf("unknown model table %q", name)
		}
		temporary[name] = true
		for _, dependency := range table.ModelDependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		temporary[name] = false
		permanent[name] = true
		order = append(order, name)
		return nil
	}
	for _, name := range model.TableNames() {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func validateIdentifier(value string) error {
	for i, r := range value {
		if i == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '_' {
				return fmt.Errorf("invalid identifier %q", value)
			}
			continue
		}
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return fmt.Errorf("invalid identifier %q", value)
		}
	}
	if value == "" {
		return fmt.Errorf("invalid identifier %q", value)
	}
	return nil
}
