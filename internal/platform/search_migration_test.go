package platform

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestGlobalSearchMigrationBackfillsExistingActiveServingStates(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "migration.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, db, "migrations", 38); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, title, description) VALUES ('sales', 'Sales', '');
		INSERT INTO serving_states (id, workspace_id, environment, status, source) VALUES ('active', 'sales', 'prod', 'active', 'publish');
		INSERT INTO assets (snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key, parent_logical_asset_id, title, description, payload_schema, payload_json, content_hash)
		VALUES ('dashboard', 'dashboard:sales.orders', 'sales', 'active', 'dashboard', 'sales.orders', '', 'Existing Orders', '', 'dashboard.v1', '{}', 'one');
		INSERT INTO workspace_active_serving_states (workspace_id, environment, serving_state_id) VALUES ('sales', 'prod', 'active');
	`); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, db, "migrations", 39); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := db.QueryRowContext(ctx, `SELECT title FROM active_search_documents WHERE workspace_id = 'sales' AND environment = 'prod'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Existing Orders" {
		t.Fatalf("backfilled title = %q", title)
	}
}
