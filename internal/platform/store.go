package platform

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Yacobolo/libredash/internal/access"
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
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
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

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SQLDB() *sql.DB {
	return s.db
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
	for _, role := range access.DefaultRoles() {
		bytes, err := json.Marshal(role.Permissions)
		if err != nil {
			return err
		}
		roleID := "role_" + role.Name
		if err := s.q.UpsertRole(ctx, db.UpsertRoleParams{
			ID:              roleID,
			Name:            role.Name,
			PermissionsJson: string(bytes),
		}); err != nil {
			return err
		}
		if err := s.q.ClearRolePermissions(ctx, roleID); err != nil {
			return err
		}
		for _, permission := range role.Permissions {
			if err := s.q.UpsertPermission(ctx, permission); err != nil {
				return err
			}
			if err := s.q.InsertRolePermission(ctx, db.InsertRolePermissionParams{
				RoleID:         roleID,
				PermissionName: permission,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
