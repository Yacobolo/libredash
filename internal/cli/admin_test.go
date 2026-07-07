package cli

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	analyticsducklake "github.com/Yacobolo/libredash/internal/analytics/ducklake"
	"github.com/Yacobolo/libredash/internal/platform"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	storagemaintenance "github.com/Yacobolo/libredash/internal/storage/maintenance"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestAdminBootstrapProductionRequiresConfiguredEmail(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	t.Setenv("LIBREDASH_PRODUCTION", "1")

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	cmd.SetArgs([]string{"bootstrap"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "LIBREDASH_BOOTSTRAP_ADMIN_EMAIL") {
		t.Fatalf("bootstrap error = %v, want bootstrap admin email requirement", err)
	}
}

func TestAdminBootstrapProductionCreatesConfiguredAdminToken(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	t.Setenv("LIBREDASH_PRODUCTION", "1")
	t.Setenv("LIBREDASH_BOOTSTRAP_ADMIN_EMAIL", "owner@example.com")

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"bootstrap"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin bootstrap: %v", err)
	}

	token := strings.TrimSpace(out.String())
	if token == "" {
		t.Fatal("bootstrap token output is empty")
	}
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	repo := accesssqlite.NewRepository(store.SQLDB())
	principal, err := repo.PrincipalForAPIToken(ctx, token)
	if err != nil {
		t.Fatalf("resolve bootstrap token: %v", err)
	}
	if principal.Email != "owner@example.com" {
		t.Fatalf("bootstrap principal email = %q, want owner@example.com", principal.Email)
	}
	decision, err := repo.Authorize(ctx, principal.ID, access.PrivilegeManagePlatform, access.PlatformObject())
	if err != nil {
		t.Fatalf("authorize bootstrap principal: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("bootstrap principal missing platform admin authorization: %#v", decision)
	}
}

func TestAdminBootstrapRotatesPreviousBootstrapTokens(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	t.Setenv("LIBREDASH_PRODUCTION", "1")
	t.Setenv("LIBREDASH_BOOTSTRAP_ADMIN_EMAIL", "owner@example.com")

	first := runAdminBootstrapForTest(t, ctx)
	second := runAdminBootstrapForTest(t, ctx)
	if first == second {
		t.Fatal("bootstrap returned the same token twice, want rotation")
	}

	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	repo := accesssqlite.NewRepository(store.SQLDB())
	if _, err := repo.PrincipalForAPIToken(ctx, first); err == nil {
		t.Fatal("first bootstrap token still authenticates after second bootstrap")
	}
	if principal, err := repo.PrincipalForAPIToken(ctx, second); err != nil {
		t.Fatalf("second bootstrap token does not authenticate: %v", err)
	} else if principal.Email != "owner@example.com" {
		t.Fatalf("second bootstrap token principal = %q, want owner@example.com", principal.Email)
	}
}

func runAdminBootstrapForTest(t *testing.T, ctx context.Context) string {
	t.Helper()
	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"bootstrap"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin bootstrap: %v", err)
	}
	token := strings.TrimSpace(out.String())
	if token == "" {
		t.Fatal("bootstrap token output is empty")
	}
	return token
}

func TestAdminBackupWritesInstanceArchive(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close platform store: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, "artifacts"), 0o755); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "artifacts", "dep_1.tar.gz"), []byte("artifact"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "libredash.backup.tar.gz")
	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"backup", "--out", backupPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin backup: %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if !strings.Contains(out.String(), "instance backup written: "+backupPath) {
		t.Fatalf("backup output = %q", out.String())
	}
}

func TestAdminBackupRejectsExternalDuckLakeCatalog(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	t.Setenv("LIBREDASH_DUCKLAKE_CATALOG_PATH", filepath.Join(t.TempDir(), "catalog.sqlite"))
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close platform store: %v", err)
	}

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	cmd.SetArgs([]string{"backup", "--out", filepath.Join(t.TempDir(), "libredash.backup.tar.gz")})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "DuckLake catalog path inside LIBREDASH_HOME") {
		t.Fatalf("admin backup error = %v, want external DuckLake catalog rejection", err)
	}
}

func TestAdminRestoreRequiresConfirmation(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	backupPath := filepath.Join(t.TempDir(), "backup.db")
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	if err := store.Backup(ctx, backupPath); err != nil {
		t.Fatalf("backup platform store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close platform store: %v", err)
	}
	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	cmd.SetArgs([]string{"restore", "--from", backupPath, "--current-out", filepath.Join(home, "before.db")})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "admin restore requires --confirm") {
		t.Fatalf("admin restore error = %v, want confirmation requirement", err)
	}
}

func TestAdminRestoreRejectsExternalDuckLakeCatalog(t *testing.T) {
	ctx := context.Background()
	backupHome := filepath.Join(t.TempDir(), "backup-source")
	backupSource, err := platform.Open(ctx, filepath.Join(backupHome, "libredash.db"))
	if err != nil {
		t.Fatalf("open backup source: %v", err)
	}
	if err := backupSource.Close(); err != nil {
		t.Fatalf("close backup source: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "restore.tar.gz")
	if err := platform.BackupInstance(ctx, platform.InstanceBackupOptions{
		HomeDir: backupHome,
		DBPath:  filepath.Join(backupHome, "libredash.db"),
		OutPath: backupPath,
	}); err != nil {
		t.Fatalf("backup source: %v", err)
	}

	home := t.TempDir()
	setAdminStorageEnv(t, home)
	t.Setenv("LIBREDASH_DUCKLAKE_CATALOG_PATH", filepath.Join(t.TempDir(), "catalog.sqlite"))
	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	cmd.SetArgs([]string{"restore", "--from", backupPath, "--current-out", filepath.Join(t.TempDir(), "before.tar.gz"), "--confirm"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "DuckLake catalog path inside LIBREDASH_HOME") {
		t.Fatalf("admin restore error = %v, want external DuckLake catalog rejection", err)
	}
}

func TestAdminRestoreReplacesPlatformDatabase(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	currentPath := filepath.Join(home, "libredash.db")
	current, err := platform.Open(ctx, currentPath)
	if err != nil {
		t.Fatalf("open current platform store: %v", err)
	}
	if err := current.UpsertSetting(ctx, "restore-test", "current"); err != nil {
		t.Fatalf("seed current platform store: %v", err)
	}
	if err := current.Close(); err != nil {
		t.Fatalf("close current platform store: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, "artifacts"), 0o755); err != nil {
		t.Fatalf("mkdir current artifacts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "artifacts", "old.tar.gz"), []byte("old artifact"), 0o644); err != nil {
		t.Fatalf("write current artifact: %v", err)
	}

	backupHome := filepath.Join(home, "backup-source")
	backupSource, err := platform.Open(ctx, filepath.Join(backupHome, "libredash.db"))
	if err != nil {
		t.Fatalf("open backup source: %v", err)
	}
	if err := backupSource.UpsertSetting(ctx, "restore-test", "restored"); err != nil {
		t.Fatalf("seed backup source: %v", err)
	}
	if err := backupSource.Close(); err != nil {
		t.Fatalf("close backup source: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(backupHome, "artifacts"), 0o755); err != nil {
		t.Fatalf("mkdir backup artifacts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupHome, "artifacts", "new.tar.gz"), []byte("new artifact"), 0o644); err != nil {
		t.Fatalf("write backup artifact: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "restore.tar.gz")
	if err := platform.BackupInstance(ctx, platform.InstanceBackupOptions{
		HomeDir: backupHome,
		DBPath:  filepath.Join(backupHome, "libredash.db"),
		OutPath: backupPath,
	}); err != nil {
		t.Fatalf("backup source: %v", err)
	}

	beforePath := filepath.Join(t.TempDir(), "before-restore.tar.gz")
	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"restore", "--from", backupPath, "--current-out", beforePath, "--confirm"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin restore: %v", err)
	}
	for _, want := range []string{
		"instance restored from: " + backupPath,
		"previous instance backup: " + beforePath,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("restore output missing %q:\n%s", want, out.String())
		}
	}

	restored, err := platform.Open(ctx, currentPath)
	if err != nil {
		t.Fatalf("open restored platform store: %v", err)
	}
	value, err := restored.GetSetting(ctx, "restore-test")
	if err != nil {
		t.Fatalf("read restored setting: %v", err)
	}
	if value != "restored" {
		t.Fatalf("restored setting = %q, want restored", value)
	}
	if err := restored.Close(); err != nil {
		t.Fatalf("close restored platform store: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(home, "artifacts", "new.tar.gz")); err != nil || string(got) != "new artifact" {
		t.Fatalf("restored artifact = %q, %v; want new artifact", string(got), err)
	}
	if _, err := os.Stat(filepath.Join(home, "artifacts", "old.tar.gz")); !os.IsNotExist(err) {
		t.Fatalf("old artifact survived restore: %v", err)
	}
	if _, err := os.Stat(beforePath); err != nil {
		t.Fatalf("before-restore backup missing: %v", err)
	}
}

func TestAdminMaintenanceDryRunReportsOperationalRetentionCandidates(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	seedAdminOperationalHistory(t, ctx, home, time.Now().UTC())

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"maintenance", "--audit-days", "30", "--query-days", "30", "--archived-agent-days", "30"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin maintenance dry-run: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"mode: dry-run",
		"audit events: 1",
		"query events: 1",
		"archived agent conversations: 1",
		"expired oauth states: 1",
		"stale sessions: 1",
		"stale api tokens: 2",
		"stale service principal secrets: 2",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	requireAdminTableCount(t, ctx, home, "audit_events", 2)
	requireAdminTableCount(t, ctx, home, "query_events", 2)
	requireAdminTableCount(t, ctx, home, "agent_conversations", 2)
	requireAdminTableCount(t, ctx, home, "oauth_states", 2)
	requireAdminTableCount(t, ctx, home, "sessions", 2)
	requireAdminTableCount(t, ctx, home, "api_tokens", 3)
	requireAdminTableCount(t, ctx, home, "service_principal_secrets", 3)
}

func TestAdminMaintenanceApplyPrunesOperationalHistory(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	setAdminStorageEnv(t, home)
	seedAdminOperationalHistory(t, ctx, home, time.Now().UTC())

	opts := &rootOptions{}
	cmd := adminCommand(ctx, opts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"maintenance", "--apply", "--audit-days", "30", "--query-days", "30", "--archived-agent-days", "30"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin maintenance apply: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "mode: apply") {
		t.Fatalf("output missing apply mode:\n%s", output)
	}
	requireAdminTableCount(t, ctx, home, "audit_events", 1)
	requireAdminTableCount(t, ctx, home, "query_events", 1)
	requireAdminTableCount(t, ctx, home, "agent_conversations", 1)
	requireAdminTableCount(t, ctx, home, "oauth_states", 1)
	requireAdminTableCount(t, ctx, home, "sessions", 1)
	requireAdminTableCount(t, ctx, home, "api_tokens", 1)
	requireAdminTableCount(t, ctx, home, "service_principal_secrets", 1)
	requireAdminRowExists(t, ctx, home, "audit_events", "audit_recent")
	requireAdminRowExists(t, ctx, home, "query_events", "query_recent")
	requireAdminRowExists(t, ctx, home, "agent_conversations", "agent_recent")
	requireAdminRowExists(t, ctx, home, "oauth_states", "oauth_recent")
	requireAdminRowExists(t, ctx, home, "sessions", "session_recent")
	requireAdminRowExists(t, ctx, home, "api_tokens", "token_recent")
	requireAdminRowExists(t, ctx, home, "service_principal_secrets", "secret_recent")
}

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

	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: adminDuckLakeCatalogPath(home), DataPath: filepath.Join(home, "data")})
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

	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: adminDuckLakeCatalogPath(home), DataPath: filepath.Join(home, "data")})
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

	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: adminDuckLakeCatalogPath(home), DataPath: filepath.Join(home, "data")})
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

func seedAdminOperationalHistory(t *testing.T, ctx context.Context, home string, now time.Time) {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	db := store.SQLDB()
	if _, err := db.ExecContext(ctx, `INSERT INTO workspaces (id, title) VALUES ('sales', 'Sales')`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO principals (id, kind, email, display_name) VALUES
		('principal_1', 'user', 'owner@example.com', 'Owner'),
		('service_1', 'service_principal', '', 'Service')`); err != nil {
		t.Fatalf("seed principal: %v", err)
	}
	old := adminSQLiteTime(now.Add(-(31 * 24 * time.Hour)))
	recent := adminSQLiteTime(now.Add(-time.Hour))
	future := adminSQLiteTime(now.Add(time.Hour))
	if _, err := db.ExecContext(ctx, `INSERT INTO audit_events (id, workspace_id, principal_id, action, created_at) VALUES
		('audit_old', 'sales', 'principal_1', 'old', ?),
		('audit_recent', 'sales', 'principal_1', 'recent', ?)`, old, recent); err != nil {
		t.Fatalf("seed audit events: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO query_events (id, workspace_id, principal_id, status, created_at) VALUES
		('query_old', 'sales', 'principal_1', 'success', ?),
		('query_recent', 'sales', 'principal_1', 'success', ?)`, old, recent); err != nil {
		t.Fatalf("seed query events: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO agent_conversations (id, workspace_id, principal_id, title, status, archived_at, created_at, updated_at) VALUES
		('agent_old', 'sales', 'principal_1', 'Old', 'archived', ?, ?, ?),
		('agent_recent', 'sales', 'principal_1', 'Recent', 'archived', ?, ?, ?)`,
		old, old, old, recent, recent, recent); err != nil {
		t.Fatalf("seed agent conversations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO oauth_states (id, state_hash, expires_at) VALUES
		('oauth_old', 'oauth_hash_old', ?),
		('oauth_recent', 'oauth_hash_recent', ?)`, old, recent); err != nil {
		t.Fatalf("seed oauth states: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions (id, principal_id, token_fingerprint, token_verifier, expires_at) VALUES
		('session_old', 'principal_1', 'session_fp_old', 'verifier', ?),
		('session_recent', 'principal_1', 'session_fp_recent', 'verifier', ?)`, old, recent); err != nil {
		t.Fatalf("seed sessions: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO api_tokens (id, principal_id, workspace_id, name, token_fingerprint, token_verifier, privileges_json, expires_at, revoked_at) VALUES
		('token_expired_old', 'principal_1', 'sales', 'expired old', 'token_fp_expired_old', 'verifier', '[]', ?, NULL),
		('token_revoked_old', 'principal_1', 'sales', 'revoked old', 'token_fp_revoked_old', 'verifier', '[]', NULL, ?),
		('token_recent', 'principal_1', 'sales', 'recent', 'token_fp_recent', 'verifier', '[]', ?, NULL)`, old, old, future); err != nil {
		t.Fatalf("seed api tokens: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO service_principal_secrets (id, service_principal_id, name, secret_fingerprint, secret_verifier, expires_at, revoked_at) VALUES
		('secret_expired_old', 'service_1', 'expired old', 'secret_fp_expired_old', 'verifier', ?, NULL),
		('secret_revoked_old', 'service_1', 'revoked old', 'secret_fp_revoked_old', 'verifier', NULL, ?),
		('secret_recent', 'service_1', 'recent', 'secret_fp_recent', 'verifier', ?, NULL)`, old, old, future); err != nil {
		t.Fatalf("seed service principal secrets: %v", err)
	}
}

func requireAdminTableCount(t *testing.T, ctx context.Context, home, table string, want int64) {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	var got int64
	if err := store.SQLDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}

func requireAdminRowExists(t *testing.T, ctx context.Context, home, table, id string) {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	var got int64
	if err := store.SQLDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE id = ?", id).Scan(&got); err != nil {
		t.Fatalf("count %s.%s: %v", table, id, err)
	}
	if got != 1 {
		t.Fatalf("%s.%s count = %d, want 1", table, id, got)
	}
}

func adminSQLiteTime(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05")
}

func adminDuckLakeCatalogPath(home string) string {
	return filepath.Join(home, "ducklake", "catalog.sqlite")
}

func seedAdminDuckLakeSnapshots(t *testing.T, ctx context.Context, home, root string) (int64, int64) {
	t.Helper()
	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: adminDuckLakeCatalogPath(home), DataPath: filepath.Join(home, "data")})
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
