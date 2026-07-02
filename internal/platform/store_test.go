package platform

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	analyticsducklake "github.com/Yacobolo/libredash/internal/analytics/ducklake"
)

func TestStoreMigratesAndSeedsRoles(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "libredash.db")
	store, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	store, err = Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	}
	defer store.Close()

	rows, err := store.SQLDB().QueryContext(ctx, `SELECT name FROM roles ORDER BY name`)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			t.Fatalf("scan role: %v", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("roles rows: %v", err)
	}
	defaultRoles := access.DefaultRoles()
	want := make([]string, 0, len(defaultRoles))
	for _, role := range defaultRoles {
		want = append(want, role.Name)
	}
	sort.Strings(want)
	if len(roles) != len(want) {
		t.Fatalf("roles = %#v, want %#v", roles, want)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("roles = %#v, want %#v", roles, want)
		}
	}
}

func TestStoreCatalogCanBeSharedWithDuckLake(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "libredash.db")
	store, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	var roleCount int
	if err := store.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM roles`).Scan(&roleCount); err != nil {
		t.Fatalf("query roles before DuckLake: %v", err)
	}

	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{
		RootDir:     filepath.Join(dir, "duckdb", "dev"),
		CatalogPath: dbPath,
		DataPath:    filepath.Join(dir, "duckdb", "dev", "data"),
	})
	if duckLakeExtensionUnavailable(err) {
		t.Skipf("ducklake extension unavailable: %v", err)
	}
	if err != nil {
		t.Fatalf("open DuckLake on platform catalog: %v", err)
	}
	defer env.Close()
	if _, err := env.Commit(ctx, "dep_1", nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "CREATE TABLE model_orders AS SELECT 1 AS id")
		return err
	}); err != nil {
		t.Fatalf("commit DuckLake snapshot: %v", err)
	}
	if err := store.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM roles`).Scan(&roleCount); err != nil {
		t.Fatalf("query roles after DuckLake: %v", err)
	}
	if roleCount == 0 {
		t.Fatal("roles were not preserved after DuckLake commit")
	}
}

func duckLakeExtensionUnavailable(err error) bool {
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
