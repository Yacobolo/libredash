package cli

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	analyticsducklake "github.com/Yacobolo/libredash/internal/analytics/ducklake"
	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentsqlite "github.com/Yacobolo/libredash/internal/deployment/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestAdminStorageCleanupDryRunReconcilesReferencedSnapshots(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	root := filepath.Join(home, "duckdb", string(deployment.DefaultEnvironment))
	first, second := seedAdminDuckLakeSnapshots(t, ctx, home, root)
	recordAdminDeploymentSnapshot(t, ctx, home, second)

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"storage", "cleanup", "--environment", string(deployment.DefaultEnvironment)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin storage cleanup: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"mode: dry-run",
		"protected snapshots: " + formatSnapshotIDs([]int64{second}),
		"expiration candidates: " + formatSnapshotIDs([]int64{first}),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}

	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: filepath.Join(home, "libredash.db"), DataPath: filepath.Join(root, "data")})
	if adminDuckLakeUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("reopen ducklake: %v", err)
	}
	defer env.Close()
	snapshots, err := env.Snapshots(ctx)
	if err != nil {
		t.Fatalf("snapshots after dry-run: %v", err)
	}
	ids := map[int64]struct{}{}
	for _, snapshot := range snapshots {
		ids[snapshot.ID] = struct{}{}
	}
	for _, want := range []int64{first, second} {
		if _, ok := ids[want]; !ok {
			t.Fatalf("snapshot %d missing after dry-run; snapshots=%#v", want, snapshots)
		}
	}
}

func TestAdminStorageCleanupRejectsMissingReferencedSnapshot(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	root := filepath.Join(home, "duckdb", string(deployment.DefaultEnvironment))
	seedAdminDuckLakeSnapshots(t, ctx, home, root)
	recordAdminDeploymentSnapshot(t, ctx, home, 999)

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"storage", "cleanup", "--environment", string(deployment.DefaultEnvironment)})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "deployment references missing DuckLake snapshots: 999") {
		t.Fatalf("admin storage cleanup error = %v, want missing snapshot reconciliation error", err)
	}
}

func setAdminStorageEnv(t *testing.T, home string) {
	t.Helper()
	t.Setenv("LIBREDASH_HOME", home)
	t.Setenv("LIBREDASH_DUCKDB_DIR", filepath.Join(home, "duckdb"))
}

func seedAdminDuckLakeSnapshots(t *testing.T, ctx context.Context, home, root string) (int64, int64) {
	t.Helper()
	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: filepath.Join(home, "libredash.db"), DataPath: filepath.Join(root, "data")})
	if adminDuckLakeUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open ducklake: %v", err)
	}
	defer env.Close()
	first, err := env.Commit(ctx, "dep_first", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE TABLE model_orders AS SELECT 1 AS id")
		return err
	})
	if err != nil {
		t.Fatalf("commit first snapshot: %v", err)
	}
	second, err := env.Commit(ctx, "dep_second", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE OR REPLACE TABLE model_orders AS SELECT 2 AS id")
		return err
	})
	if err != nil {
		t.Fatalf("commit second snapshot: %v", err)
	}
	return first, second
}

func recordAdminDeploymentSnapshot(t *testing.T, ctx context.Context, home string, snapshotID int64) {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	repo := deploymentsqlite.NewRepository(store.SQLDB())
	created, err := repo.Create(ctx, deployment.CreateInput{WorkspaceID: "test", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if err := repo.RecordDuckLakeSnapshot(ctx, created.ID, snapshotID); err != nil {
		t.Fatalf("record snapshot: %v", err)
	}
}

func adminDuckLakeUnavailable(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "extension") &&
		(strings.Contains(text, "not found") ||
			strings.Contains(text, "failed to download") ||
			strings.Contains(text, "failed to install") ||
			strings.Contains(text, "not be loaded"))
}
