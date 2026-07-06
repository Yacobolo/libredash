package platform

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	agentconfig "github.com/Yacobolo/libredash/internal/agent/config"
	"github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const (
	DefaultWorkspaceID = "libredash"
)

type Paths struct {
	HomeDir     string
	DBPath      string
	ArtifactDir string
	DuckDBDir   string
}

func DefaultPaths() Paths {
	home := os.Getenv("LIBREDASH_HOME")
	if home == "" {
		home = ".libredash"
	}
	return Paths{
		HomeDir:     home,
		DBPath:      filepath.Join(home, "libredash.db"),
		ArtifactDir: filepath.Join(home, "artifacts"),
		DuckDBDir:   filepath.Join(home, "duckdb"),
	}
}

type Store struct {
	db *sql.DB
	q  *db.Queries
}

func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
	return store, nil
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SQLDB() *sql.DB {
	return s.db
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM platform_settings WHERE key = ?`, key).Scan(&value)
	return value, err
}

func (s *Store) UpsertSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO platform_settings (key, value)
VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
`, key, value)
	return err
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
	if err := s.insertSettingIfMissing(ctx, agentconfig.SystemPromptSettingKey, agentconfig.DefaultSystemPrompt); err != nil {
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
		if _, err := s.db.ExecContext(ctx, `DELETE FROM role_grant_templates WHERE role_name = ?`, role.Name); err != nil {
			return err
		}
		for _, privilege := range role.Privileges {
			if _, err := s.db.ExecContext(ctx, `
		INSERT INTO role_grant_templates (role_name, privilege)
		VALUES (?, ?)
	ON CONFLICT(role_name, privilege) DO NOTHING
	`, role.Name, string(privilege)); err != nil {
				return err
			}
		}
	}
	if _, err := s.db.ExecContext(ctx, `
	INSERT INTO securable_objects (id, object_type, display_name)
	VALUES ('platform', 'platform', 'Platform')
	ON CONFLICT(id) DO UPDATE SET object_type = excluded.object_type, display_name = excluded.display_name, updated_at = CURRENT_TIMESTAMP
	`); err != nil {
		return err
	}
	return nil
}

func (s *Store) insertSettingIfMissing(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO platform_settings (key, value)
VALUES (?, ?)
ON CONFLICT(key) DO NOTHING
`, key, value)
	return err
}
