package ducklake

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

var errIntentionalFailure = errors.New("intentional failure")

func TestLayoutUsesOneCatalogAndDataStore(t *testing.T) {
	layout := NewLayout(filepath.Join("tmp", "env"))

	if layout.CatalogPath != filepath.Join("tmp", "env", "catalog.sqlite") {
		t.Fatalf("CatalogPath = %q", layout.CatalogPath)
	}
	if layout.DataPath != filepath.Join("tmp", "env", "data") {
		t.Fatalf("DataPath = %q", layout.DataPath)
	}
	if _, ok := reflect.TypeOf(layout).FieldByName("LegacyDuckDBPath"); ok {
		t.Fatal("Layout still exposes LegacyDuckDBPath")
	}
}

func TestOpenCreatesPrivateCatalogAndDataDirectories(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	root := filepath.Join(parent, "ducklake")
	restoreUmask := setUmask(t, 0)
	env, err := Open(ctx, Config{RootDir: root})
	restoreUmask()
	if extensionUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer env.Close()

	assertFileMode(t, root, 0o700)
	assertFileMode(t, filepath.Join(root, "data"), 0o700)
}

func TestEnvironmentCommitsAndReadsStableSnapshots(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	env, err := Open(ctx, Config{RootDir: dir})
	if extensionUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer env.Close()

	snapshot1, err := env.Commit(ctx, "dep_1", map[string]string{"workspace": "test"}, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS model"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "CREATE OR REPLACE TABLE model.orders AS SELECT 1 AS id, 'first' AS label")
		return err
	})
	if err != nil {
		t.Fatalf("commit first snapshot: %v", err)
	}
	if snapshot1 <= 0 {
		t.Fatalf("snapshot1 = %d, want positive committed snapshot", snapshot1)
	}

	snapshot2, err := env.Commit(ctx, "dep_2", map[string]string{"workspace": "test"}, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE OR REPLACE TABLE model.orders AS SELECT 2 AS id, 'second' AS label")
		return err
	})
	if err != nil {
		t.Fatalf("commit second snapshot: %v", err)
	}
	if snapshot2 <= snapshot1 {
		t.Fatalf("snapshot2 = %d, want > snapshot1 %d", snapshot2, snapshot1)
	}

	first, err := OpenSnapshot(ctx, Config{RootDir: dir, SnapshotID: snapshot1, MaxReaders: 2})
	if err != nil {
		t.Fatalf("open first snapshot: %v", err)
	}
	defer first.Close()
	second, err := OpenSnapshot(ctx, Config{RootDir: dir, SnapshotID: snapshot2})
	if err != nil {
		t.Fatalf("open second snapshot: %v", err)
	}
	defer second.Close()

	assertOrder := func(t *testing.T, db *Environment, wantID int, wantLabel string) {
		t.Helper()
		var gotID int
		var gotLabel string
		if err := db.SQLDB().QueryRowContext(ctx, "SELECT id, label FROM model.orders").Scan(&gotID, &gotLabel); err != nil {
			t.Fatalf("query order: %v", err)
		}
		if gotID != wantID || gotLabel != wantLabel {
			t.Fatalf("order = (%d, %q), want (%d, %q)", gotID, gotLabel, wantID, wantLabel)
		}
	}
	assertOrder(t, first, 1, "first")
	assertOrder(t, second, 2, "second")
	if first.ReadConcurrency() != 2 {
		t.Fatalf("snapshot read concurrency = %d, want 2", first.ReadConcurrency())
	}
	connections := make([]*sql.Conn, 0, 2)
	for range 2 {
		connection, err := first.SQLDB().Conn(ctx)
		if err != nil {
			t.Fatalf("acquire snapshot reader: %v", err)
		}
		connections = append(connections, connection)
	}
	for index, connection := range connections {
		var id int
		if err := connection.QueryRowContext(ctx, "SELECT id FROM model.orders").Scan(&id); err != nil {
			t.Fatalf("query snapshot reader %d: %v", index, err)
		}
		if id != 1 {
			t.Fatalf("snapshot reader %d id = %d, want 1", index, id)
		}
		connection.Close()
	}

	if _, err := os.Stat(filepath.Join(dir, "catalog.sqlite")); err != nil {
		t.Fatalf("catalog.sqlite missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data")); err != nil {
		t.Fatalf("data dir missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "leapview-workspace.duckdb")); !os.IsNotExist(err) {
		t.Fatalf("legacy DuckDB workspace file exists or stat failed: %v", err)
	}
}

func TestOpenSnapshotRejectsMissingSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	env, err := Open(ctx, Config{RootDir: dir})
	if extensionUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer env.Close()

	if _, err := env.Commit(ctx, "dep_1", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE TABLE model_check AS SELECT 1 AS id")
		return err
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if _, err := OpenSnapshot(ctx, Config{RootDir: dir, SnapshotID: 999}); err == nil {
		t.Fatal("OpenSnapshot missing snapshot error = nil")
	}
}

func TestFailedCommitDoesNotAdvanceVisibleSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	env, err := Open(ctx, Config{RootDir: dir})
	if extensionUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer env.Close()

	snapshot1, err := env.Commit(ctx, "dep_1", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE TABLE model_orders AS SELECT 1 AS id")
		return err
	})
	if err != nil {
		t.Fatalf("commit first snapshot: %v", err)
	}
	if _, err := env.Commit(ctx, "dep_fail", nil, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "CREATE OR REPLACE TABLE model_orders AS SELECT 2 AS id"); err != nil {
			return err
		}
		return errIntentionalFailure
	}); !errors.Is(err, errIntentionalFailure) {
		t.Fatalf("failed commit error = %v, want intentional failure", err)
	}
	snapshots, err := env.Snapshots(ctx)
	if err != nil {
		t.Fatalf("snapshots: %v", err)
	}
	for _, snapshot := range snapshots {
		if snapshot.ID > snapshot1 {
			t.Fatalf("snapshots = %#v, want no committed snapshot after %d", snapshots, snapshot1)
		}
	}
	var id int
	if err := env.SQLDB().QueryRowContext(ctx, "SELECT id FROM model_orders").Scan(&id); err != nil {
		t.Fatalf("query visible table: %v", err)
	}
	if id != 1 {
		t.Fatalf("visible id = %d, want first committed value", id)
	}
}

func TestRetentionCandidatesPreserveProtectedSnapshots(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	env, err := Open(ctx, Config{RootDir: dir})
	if extensionUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer env.Close()

	snapshot1, err := env.Commit(ctx, "dep_1", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE TABLE model_orders AS SELECT 1 AS id")
		return err
	})
	if err != nil {
		t.Fatalf("commit first: %v", err)
	}
	snapshot2, err := env.Commit(ctx, "dep_2", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE OR REPLACE TABLE model_orders AS SELECT 2 AS id")
		return err
	})
	if err != nil {
		t.Fatalf("commit second: %v", err)
	}

	candidates, err := env.RetentionCandidates(ctx, map[int64]struct{}{snapshot2: {}})
	if err != nil {
		t.Fatalf("retention candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0] != snapshot1 {
		t.Fatalf("candidates = %#v, want only %d", candidates, snapshot1)
	}
}

func TestMaintenanceDryRunsUseDuckLakeMetadata(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	env, err := Open(ctx, Config{RootDir: dir})
	if extensionUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer env.Close()

	snapshot1, err := env.Commit(ctx, "dep_1", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE TABLE model_orders AS SELECT 1 AS id")
		return err
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := env.ExpireSnapshots(ctx, []int64{snapshot1}, true); err != nil {
		t.Fatalf("expire dry run: %v", err)
	}
	if err := env.CleanupOldFiles(ctx, true); err != nil {
		t.Fatalf("cleanup dry run: %v", err)
	}
	if err := env.DeleteOrphanedFiles(ctx, true); err != nil {
		t.Fatalf("orphan dry run: %v", err)
	}
	snapshots, err := env.Snapshots(ctx)
	if err != nil {
		t.Fatalf("snapshots after dry run: %v", err)
	}
	if len(snapshots) == 0 {
		t.Fatal("dry-run maintenance removed snapshots")
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %#o, want %#o", path, got, want)
	}
}

func setUmask(t *testing.T, mask int) func() {
	t.Helper()
	old := syscall.Umask(mask)
	return func() {
		syscall.Umask(old)
	}
}
