package platform

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	agentconfig "github.com/Yacobolo/leapview/internal/agent/config"
	"github.com/Yacobolo/leapview/internal/configspec"
	"github.com/Yacobolo/leapview/internal/platform/db"
	"github.com/Yacobolo/leapview/internal/securefs"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const (
	DefaultWorkspaceID = "leapview"
	databaseFileMode   = securefs.PrivateFileMode
)

type Paths struct {
	HomeDir     string
	DBPath      string
	ArtifactDir string
	DuckDBDir   string
}

func DefaultPaths() Paths {
	home := os.Getenv(configspec.EnvLEAPVIEW_HOME)
	if home == "" {
		home = ".leapview"
	}
	return Paths{
		HomeDir:     home,
		DBPath:      filepath.Join(home, "leapview.db"),
		ArtifactDir: filepath.Join(home, "artifacts"),
		DuckDBDir:   filepath.Join(home, "duckdb"),
	}
}

type Store struct {
	db *sql.DB
	q  *db.Queries
}

func Open(ctx context.Context, path string) (*Store, error) {
	if err := securefs.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(0)
	store := &Store{db: conn, q: db.New(conn)}
	if err := store.migrate(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	if err := store.seedDefaults(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	if err := chmodDatabaseFile(path); err != nil {
		conn.Close()
		return nil, err
	}
	return store, nil
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
}

func sqliteString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SQLDB() *sql.DB {
	return s.db
}

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("platform store is not open")
	}
	return s.db.PingContext(ctx)
}

func (s *Store) Backup(ctx context.Context, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("backup path is required")
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("platform store is not open")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("backup path %q already exists", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(FULL)`); err != nil {
		return fmt.Errorf("checkpoint platform db: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM main INTO '`+sqliteString(path)+`'`); err != nil {
		return fmt.Errorf("backup platform db: %w", err)
	}
	if err := chmodDatabaseFile(path); err != nil {
		return fmt.Errorf("secure platform db backup: %w", err)
	}
	return nil
}

func chmodDatabaseFile(path string) error {
	if path == "" || path == ":memory:" {
		return nil
	}
	if strings.Contains(path, "?") {
		path = strings.SplitN(path, "?", 2)[0]
	}
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(candidate, databaseFileMode); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func Restore(ctx context.Context, targetPath, backupPath, currentBackupPath string) error {
	targetPath = strings.TrimSpace(targetPath)
	backupPath = strings.TrimSpace(backupPath)
	currentBackupPath = strings.TrimSpace(currentBackupPath)
	if targetPath == "" {
		return fmt.Errorf("restore target path is required")
	}
	if backupPath == "" {
		return fmt.Errorf("restore backup path is required")
	}
	if samePath(targetPath, backupPath) {
		return fmt.Errorf("restore backup path must differ from target path")
	}
	if currentBackupPath != "" && samePath(currentBackupPath, backupPath) {
		return fmt.Errorf("current backup path must differ from restore backup path")
	}
	if err := validateBackupDatabase(ctx, backupPath); err != nil {
		return err
	}

	if _, err := os.Stat(targetPath); err == nil {
		if currentBackupPath == "" {
			return fmt.Errorf("current backup path is required when restoring over an existing database")
		}
		if samePath(targetPath, currentBackupPath) {
			return fmt.Errorf("current backup path must differ from target path")
		}
		current, err := Open(ctx, targetPath)
		if err != nil {
			return fmt.Errorf("open current platform db: %w", err)
		}
		if err := current.Backup(ctx, currentBackupPath); err != nil {
			_ = current.Close()
			return fmt.Errorf("backup current platform db: %w", err)
		}
		if err := current.Close(); err != nil {
			return fmt.Errorf("close current platform db: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), ".leapview-restore-*.db")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	backup, err := os.Open(backupPath)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, backup); err != nil {
		_ = backup.Close()
		_ = tmp.Close()
		return err
	}
	if err := backup.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := removeSQLiteSidecars(targetPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	cleanupTmp = false
	return nil
}

func validateBackupDatabase(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("restore backup path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("restore backup path %q is a directory", path)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=query_only(1)")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("check backup integrity: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("backup integrity check failed: %s", integrity)
	}
	rows, err := db.QueryContext(ctx, `
SELECT name
FROM sqlite_master
WHERE type = 'table'
  AND name IN ('platform_settings', 'workspaces', 'serving_states', 'roles')
`)
	if err != nil {
		return fmt.Errorf("inspect backup schema: %w", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan backup schema: %w", err)
		}
		seen[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, name := range []string{"platform_settings", "workspaces", "serving_states", "roles"} {
		if !seen[name] {
			return fmt.Errorf("backup is not a LeapView platform database: missing table %s", name)
		}
	}
	return nil
}

func removeSQLiteSidecars(path string) error {
	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return absA == absB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	return s.q.GetPlatformSetting(ctx, key)
}

func (s *Store) UpsertSetting(ctx context.Context, key, value string) error {
	return s.q.UpsertPlatformSetting(ctx, db.UpsertPlatformSettingParams{Key: key, Value: value})
}

func (s *Store) migrate(ctx context.Context) error {
	for _, stmt := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, s.db, "migrations"); err != nil {
		return fmt.Errorf("migrating platform db: %w", err)
	}
	return nil
}

func (s *Store) seedDefaults(ctx context.Context) error {
	if err := s.q.InsertPlatformSettingIfMissing(ctx, db.InsertPlatformSettingIfMissingParams{
		Key:   agentconfig.SystemPromptSettingKey,
		Value: agentconfig.DefaultSystemPrompt,
	}); err != nil {
		return err
	}
	for _, role := range access.DefaultRoles() {
		bytes, err := json.Marshal(role.Privileges)
		if err != nil {
			return err
		}
		roleID := "role_" + role.Name
		if err := s.q.UpsertRole(ctx, db.UpsertRoleParams{
			ID:             roleID,
			Name:           role.Name,
			PrivilegesJson: string(bytes),
		}); err != nil {
			return err
		}
		if err := s.q.DeleteRoleGrantTemplates(ctx, role.Name); err != nil {
			return err
		}
		for _, privilege := range role.Privileges {
			if err := s.q.InsertRoleGrantTemplate(ctx, db.InsertRoleGrantTemplateParams{
				RoleName:  role.Name,
				Privilege: string(privilege),
			}); err != nil {
				return err
			}
		}
	}
	if err := s.q.UpsertSecurableObject(ctx, db.UpsertSecurableObjectParams{
		ID:          "platform",
		ObjectType:  "platform",
		DisplayName: "Platform",
	}); err != nil {
		return err
	}
	return nil
}
