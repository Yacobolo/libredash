package ducklake

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSQLiteCatalogCanContainControlPlaneAndDuckLakeMetadataWithoutIdleControlConnections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.sqlite")
	dataPath := filepath.Join(dir, "data")

	control, err := sql.Open("sqlite", catalogPath)
	if err != nil {
		t.Fatalf("open control-plane sqlite: %v", err)
	}
	defer control.Close()
	control.SetMaxOpenConns(1)
	control.SetMaxIdleConns(0)
	for _, stmt := range []string{
		"PRAGMA journal_mode=WAL",
		"CREATE TABLE control_plane_state (id TEXT PRIMARY KEY, value TEXT NOT NULL)",
		"INSERT INTO control_plane_state (id, value) VALUES ('active_deployment', 'dep_1')",
	} {
		if _, err := control.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("control-plane setup %q: %v", stmt, err)
		}
	}

	env, err := Open(ctx, Config{CatalogPath: catalogPath, DataPath: dataPath})
	if extensionUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open shared DuckLake catalog: %v", err)
	}
	defer env.Close()
	snapshotID, err := env.Commit(ctx, "dep_1", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE TABLE model_orders AS SELECT 1 AS id")
		return err
	})
	if err != nil {
		t.Fatalf("commit DuckLake snapshot: %v", err)
	}
	if snapshotID <= 0 {
		t.Fatalf("snapshot id = %d, want committed snapshot", snapshotID)
	}

	var value string
	if err := control.QueryRowContext(ctx, "SELECT value FROM control_plane_state WHERE id = 'active_deployment'").Scan(&value); err != nil {
		t.Fatalf("query control-plane state after DuckLake commit: %v", err)
	}
	if value != "dep_1" {
		t.Fatalf("control-plane value = %q, want dep_1", value)
	}
}

func TestSQLiteCatalogCanBeReopenedByControlPlaneAfterDuckLakeCommit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.sqlite")
	dataPath := filepath.Join(dir, "data")

	control, err := sql.Open("sqlite", catalogPath)
	if err != nil {
		t.Fatalf("open control-plane sqlite: %v", err)
	}
	control.SetMaxOpenConns(1)
	for _, stmt := range []string{
		"PRAGMA journal_mode=WAL",
		"CREATE TABLE control_plane_state (id TEXT PRIMARY KEY, value TEXT NOT NULL)",
		"INSERT INTO control_plane_state (id, value) VALUES ('active_deployment', 'dep_1')",
	} {
		if _, err := control.ExecContext(ctx, stmt); err != nil {
			control.Close()
			t.Fatalf("control-plane setup %q: %v", stmt, err)
		}
	}
	if err := control.Close(); err != nil {
		t.Fatalf("close control-plane sqlite: %v", err)
	}

	env, err := Open(ctx, Config{CatalogPath: catalogPath, DataPath: dataPath})
	if extensionUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open shared DuckLake catalog: %v", err)
	}
	if _, err := env.Commit(ctx, "dep_1", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE TABLE model_orders AS SELECT 1 AS id")
		return err
	}); err != nil {
		env.Close()
		t.Fatalf("commit DuckLake snapshot: %v", err)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close DuckLake env: %v", err)
	}

	control, err = sql.Open("sqlite", catalogPath)
	if err != nil {
		t.Fatalf("reopen control-plane sqlite: %v", err)
	}
	defer control.Close()
	var value string
	if err := control.QueryRowContext(ctx, "SELECT value FROM control_plane_state WHERE id = 'active_deployment'").Scan(&value); err != nil {
		t.Fatalf("query control-plane state after DuckLake commit: %v", err)
	}
	if value != "dep_1" {
		t.Fatalf("control-plane value = %q, want dep_1", value)
	}
}
