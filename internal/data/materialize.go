package data

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

func (m *DuckDBMetrics) RefreshMaterializations(ctx context.Context, modelID string) error {
	runtime, ok := m.runtimes[modelID]
	if !ok {
		return fmt.Errorf("unknown semantic model %q", modelID)
	}
	if runtime.missing != nil {
		return runtime.missing
	}
	if runtime.db == nil {
		return fmt.Errorf("DuckDB is not initialized")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.registerSourceViews(ctx, runtime); err != nil {
		return err
	}
	if err := m.materializeModelTables(ctx, runtime); err != nil {
		return err
	}
	runtime.lastRefresh = time.Now()
	return nil
}

func (m *DuckDBMetrics) validateFiles(runtime *modelRuntime) error {
	var missing []string
	for name, source := range runtime.model.Sources {
		if source.Path == "" {
			continue
		}
		connection := runtime.model.Connections[source.Connection]
		if connection.Kind != "local" {
			continue
		}
		file, err := m.resolveSourcePath(runtime.model, source)
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
		return &MissingDataError{DataDir: m.dataDir, Missing: missing}
	}
	return nil
}

func (m *DuckDBMetrics) registerSourceViews(ctx context.Context, runtime *modelRuntime) error {
	if _, err := runtime.db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS raw"); err != nil {
		return err
	}
	if _, err := runtime.db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS source"); err != nil {
		return err
	}
	if _, err := runtime.db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS model"); err != nil {
		return err
	}

	if err := m.prepareSourceRuntime(ctx, runtime); err != nil {
		return err
	}

	for _, name := range sortedKeys(runtime.model.Sources) {
		source := runtime.model.Sources[name]
		if err := validateIdentifier(name); err != nil {
			return err
		}
		relation, err := m.sourceRelation(runtime.model, source)
		if err != nil {
			return fmt.Errorf("compiling source %s: %w", name, err)
		}
		stmt := fmt.Sprintf("CREATE OR REPLACE VIEW raw.%s AS %s", name, relation)
		if _, err := runtime.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("registering source %s: %w", name, err)
		}
		stmt = fmt.Sprintf("CREATE OR REPLACE VIEW source.%s AS %s", name, relation)
		if _, err := runtime.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("registering source %s: %w", name, err)
		}
	}
	return nil
}

func (m *DuckDBMetrics) materializeModelTables(ctx context.Context, runtime *modelRuntime) error {
	for _, name := range runtime.model.TableNames() {
		if err := validateIdentifier(name); err != nil {
			return err
		}
		table := runtime.model.Tables[name]
		sourceSQL := table.Transform.SQL
		if table.Source != "" {
			if err := validateIdentifier(table.Source); err != nil {
				return err
			}
			if sourceSQL == "" {
				sourceSQL = "SELECT * FROM raw." + table.Source
			}
		}
		stmt := fmt.Sprintf("CREATE OR REPLACE TABLE model.%s AS %s", name, sourceSQL)
		if _, err := runtime.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("materializing model.%s: %w", name, err)
		}
	}
	return nil
}
