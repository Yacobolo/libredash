package cli

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	analyticsducklake "github.com/Yacobolo/libredash/internal/analytics/ducklake"
	"github.com/Yacobolo/libredash/internal/platform"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	storagemaintenance "github.com/Yacobolo/libredash/internal/storage/maintenance"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestAdminStorageCleanupDryRunReconcilesReferencedSnapshots(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	root := home
	first, second := seedAdminDuckLakeSnapshots(t, ctx, home, root)
	recordAdminDeploymentSnapshot(t, ctx, home, "dev", second)
	recordAdminDeploymentSnapshot(t, ctx, home, "prod", second)

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"storage", "cleanup"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin storage cleanup: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"mode: dry-run",
		"protected snapshots: " + storagemaintenance.FormatSnapshotIDs([]int64{second}),
		"expiration candidates: " + storagemaintenance.FormatSnapshotIDs([]int64{first}),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}

	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: filepath.Join(home, "libredash.db"), DataPath: filepath.Join(home, "data")})
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

func TestAdminStorageCleanupDryRunDoesNotMutateDrainingDeployments(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	root := home
	first, second := seedAdminDuckLakeSnapshots(t, ctx, home, root)
	drainingID := recordAdminDeploymentSnapshotWithStatus(t, ctx, home, "dev", first, servingstate.StatusDraining)
	recordAdminDeploymentSnapshot(t, ctx, home, "dev", second)

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"storage", "cleanup"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin storage cleanup dry-run: %v", err)
	}
	status := adminDeploymentStatus(t, ctx, home, drainingID)
	if status != string(servingstate.StatusDraining) {
		t.Fatalf("draining deployment status after dry-run = %q, want draining", status)
	}
}

func TestAdminStorageCleanupRejectsMissingReferencedSnapshot(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	root := home
	seedAdminDuckLakeSnapshots(t, ctx, home, root)
	recordAdminDeploymentSnapshot(t, ctx, home, "prod", 999)

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"storage", "cleanup"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "serving states reference missing DuckLake snapshots: 999") {
		t.Fatalf("admin storage cleanup error = %v, want missing snapshot reconciliation error", err)
	}
}

func TestAdminStorageCleanupApplyExpiresDrainingSnapshots(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	root := home
	first, second := seedAdminDuckLakeSnapshots(t, ctx, home, root)
	recordAdminDeploymentSnapshotWithStatus(t, ctx, home, "dev", first, servingstate.StatusDraining)
	recordAdminDeploymentSnapshot(t, ctx, home, "dev", second)

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"storage", "cleanup", "--apply"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin storage cleanup apply: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"mode: apply",
		"protected snapshots: " + storagemaintenance.FormatSnapshotIDs([]int64{second}),
		"expiration candidates: " + storagemaintenance.FormatSnapshotIDs([]int64{first}),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}

	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: filepath.Join(home, "libredash.db"), DataPath: filepath.Join(home, "data")})
	if adminDuckLakeUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("reopen ducklake: %v", err)
	}
	defer env.Close()
	snapshots, err := env.Snapshots(ctx)
	if err != nil {
		t.Fatalf("snapshots after apply: %v", err)
	}
	ids := map[int64]struct{}{}
	for _, snapshot := range snapshots {
		ids[snapshot.ID] = struct{}{}
	}
	if _, ok := ids[first]; ok {
		t.Fatalf("expired snapshot %d still present after cleanup apply; snapshots=%#v", first, snapshots)
	}
	if _, ok := ids[second]; !ok {
		t.Fatalf("protected snapshot %d missing after cleanup apply; snapshots=%#v", second, snapshots)
	}
}

func TestAdminStorageCleanupApplyProtectsLeasedDrainingSnapshot(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	root := home
	first, second := seedAdminDuckLakeSnapshots(t, ctx, home, root)
	drainingID := recordAdminDeploymentSnapshotWithStatus(t, ctx, home, "dev", first, servingstate.StatusDraining)
	recordAdminDeploymentSnapshot(t, ctx, home, "dev", second)
	createAdminSnapshotLease(t, ctx, home, drainingID, first)

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"storage", "cleanup", "--apply"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin storage cleanup apply: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "protected leased snapshots: "+storagemaintenance.FormatSnapshotIDs([]int64{first})) {
		t.Fatalf("output missing leased protection:\n%s", output)
	}
	if !strings.Contains(output, "expiration candidates: none") {
		t.Fatalf("output has unexpected expiration candidates:\n%s", output)
	}

	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: filepath.Join(home, "libredash.db"), DataPath: filepath.Join(home, "data")})
	if adminDuckLakeUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("reopen ducklake: %v", err)
	}
	defer env.Close()
	snapshots, err := env.Snapshots(ctx)
	if err != nil {
		t.Fatalf("snapshots after apply: %v", err)
	}
	ids := map[int64]struct{}{}
	for _, snapshot := range snapshots {
		ids[snapshot.ID] = struct{}{}
	}
	if _, ok := ids[first]; !ok {
		t.Fatalf("leased snapshot %d missing after cleanup apply; snapshots=%#v", first, snapshots)
	}
}

func setAdminStorageEnv(t *testing.T, home string) {
	t.Helper()
	t.Setenv("LIBREDASH_HOME", home)
	t.Setenv("LIBREDASH_DUCKDB_DIR", filepath.Join(home, "duckdb"))
}

func seedAdminDuckLakeSnapshots(t *testing.T, ctx context.Context, home, root string) (int64, int64) {
	t.Helper()
	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: filepath.Join(home, "libredash.db"), DataPath: filepath.Join(home, "data")})
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

func recordAdminDeploymentSnapshot(t *testing.T, ctx context.Context, home string, environment servingstate.Environment, snapshotID int64) servingstate.ID {
	t.Helper()
	return recordAdminDeploymentSnapshotWithStatus(t, ctx, home, environment, snapshotID, servingstate.StatusActive)
}

func recordAdminDeploymentSnapshotWithStatus(t *testing.T, ctx context.Context, home string, environment servingstate.Environment, snapshotID int64, status servingstate.Status) servingstate.ID {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	repo := servingstatesqlite.NewRepository(store.SQLDB())
	created, err := repo.Create(ctx, servingstate.CreateInput{WorkspaceID: "test", Environment: environment, CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if err := repo.RecordDuckLakeSnapshot(ctx, created.ID, snapshotID); err != nil {
		t.Fatalf("record snapshot: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, "UPDATE serving_states SET status = ? WHERE id = ?", string(status), string(created.ID)); err != nil {
		t.Fatalf("mark deployment %s: %v", status, err)
	}
	return created.ID
}

func createAdminSnapshotLease(t *testing.T, ctx context.Context, home string, servingStateID servingstate.ID, snapshotID int64) {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	repo := servingstatesqlite.NewRepository(store.SQLDB())
	if _, err := repo.CreateQuerySnapshotLease(ctx, servingstate.SnapshotLeaseInput{
		WorkspaceID:        "test",
		Environment:        "dev",
		ServingStateID:     servingStateID,
		DuckLakeSnapshotID: snapshotID,
		OwnerID:            "test",
		ExpiresAt:          time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create query snapshot lease: %v", err)
	}
}

func adminDeploymentStatus(t *testing.T, ctx context.Context, home string, servingStateID servingstate.ID) string {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	var status string
	if err := store.SQLDB().QueryRowContext(ctx, "SELECT status FROM serving_states WHERE id = ?", string(servingStateID)).Scan(&status); err != nil {
		t.Fatalf("read deployment status: %v", err)
	}
	return status
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
