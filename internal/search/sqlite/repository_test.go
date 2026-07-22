package sqlite_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/leapview/internal/platform"
	productsearch "github.com/Yacobolo/leapview/internal/search"
	searchsqlite "github.com/Yacobolo/leapview/internal/search/sqlite"
)

func BenchmarkRepositorySearch100K(b *testing.B) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(b.TempDir(), "leapview.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	db := store.SQLDB()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, title, description) VALUES ('benchmark', 'Benchmark', '');
		INSERT INTO serving_states (id, workspace_id, environment, status, source) VALUES ('benchmark_active', 'benchmark', 'dev', 'active', 'publish');
	`); err != nil {
		b.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO active_search_documents
		(workspace_id, environment, serving_state_id, asset_snapshot_id, asset_id, asset_type, asset_key, parent_asset_id, title, description, workspace_title, terms)
		VALUES ('benchmark', 'dev', 'benchmark_active', ?, ?, 'dashboard', ?, '', ?, 'Benchmark dashboard', 'Benchmark', 'orders revenue sales')`)
	if err != nil {
		b.Fatal(err)
	}
	for index := 0; index < 100_000; index++ {
		id := fmt.Sprintf("dashboard-%06d", index)
		if _, err := statement.ExecContext(ctx, id, id, "benchmark."+id, "Orders "+id); err != nil {
			b.Fatal(err)
		}
	}
	_ = statement.Close()
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	repository := searchsqlite.New(db)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, _, err := repository.Candidates(ctx, productsearch.RepositoryQuery{Text: "orders dashboard", Environment: "dev"}, 0, 20); err != nil {
			b.Fatal(err)
		}
	}
}

func TestPlatformMigrationBackfillsOnlyActiveServingStateSearchDocuments(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "leapview.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()

	db := store.SQLDB()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, title, description) VALUES ('sales', 'Sales', 'Sales workspace');
		INSERT INTO serving_states (id, workspace_id, environment, status, source) VALUES
			('sales_active', 'sales', 'dev', 'active', 'publish'),
			('sales_candidate', 'sales', 'dev', 'validated', 'publish');
		INSERT INTO assets (
			snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key,
			parent_logical_asset_id, title, description, payload_schema, payload_json, content_hash
		) VALUES
			('sales_active:dashboard', 'dashboard:sales.orders', 'sales', 'sales_active', 'dashboard', 'sales.orders', '', 'Orders', 'Active orders dashboard', 'dashboard.v1', '{}', 'active'),
			('sales_candidate:dashboard', 'dashboard:sales.secret', 'sales', 'sales_candidate', 'dashboard', 'sales.secret', '', 'Secret', 'Candidate dashboard', 'dashboard.v1', '{}', 'candidate');
		INSERT INTO workspace_active_serving_states (workspace_id, environment, serving_state_id)
		VALUES ('sales', 'dev', 'sales_active');
	`); err != nil {
		t.Fatalf("seed active search data: %v", err)
	}

	var activeCount, candidateCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM active_search_documents WHERE title = 'Orders'`).Scan(&activeCount); err != nil {
		t.Fatalf("count active search documents: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM active_search_documents WHERE title = 'Secret'`).Scan(&candidateCount); err != nil {
		t.Fatalf("count candidate search documents: %v", err)
	}
	if activeCount != 1 || candidateCount != 0 {
		t.Fatalf("search document counts = active %d candidate %d, want 1 and 0", activeCount, candidateCount)
	}
}

func TestActiveServingStateSwitchReplacesSearchDocumentsAtomically(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "leapview.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()

	db := store.SQLDB()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, title, description) VALUES ('sales', 'Sales', 'Sales workspace');
		INSERT INTO serving_states (id, workspace_id, environment, status, source) VALUES
			('sales_old', 'sales', 'dev', 'active', 'publish'),
			('sales_new', 'sales', 'dev', 'validated', 'publish');
		INSERT INTO assets (
			snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key,
			parent_logical_asset_id, title, description, payload_schema, payload_json, content_hash
		) VALUES
			('old:dashboard', 'dashboard:sales.old', 'sales', 'sales_old', 'dashboard', 'sales.old', '', 'Old dashboard', '', 'dashboard.v1', '{}', 'old'),
			('new:dashboard', 'dashboard:sales.new', 'sales', 'sales_new', 'dashboard', 'sales.new', '', 'New dashboard', '', 'dashboard.v1', '{}', 'new');
		INSERT INTO workspace_active_serving_states (workspace_id, environment, serving_state_id)
		VALUES ('sales', 'dev', 'sales_old');
		UPDATE workspace_active_serving_states SET serving_state_id = 'sales_new' WHERE workspace_id = 'sales' AND environment = 'dev';
	`); err != nil {
		t.Fatalf("switch active serving state: %v", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT title FROM active_search_documents ORDER BY title`)
	if err != nil {
		t.Fatalf("query active search documents: %v", err)
	}
	defer rows.Close()
	var titles []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatalf("scan title: %v", err)
		}
		titles = append(titles, title)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate titles: %v", err)
	}
	if len(titles) != 1 || titles[0] != "New dashboard" {
		t.Fatalf("active search titles = %#v, want New dashboard", titles)
	}
}

func TestActiveAssetUpdatesAndDeletesRemainSynchronizedWithFTS(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "leapview.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.SQLDB()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, title, description) VALUES ('sales', 'Sales', '');
		INSERT INTO serving_states (id, workspace_id, environment, status, source) VALUES ('active', 'sales', 'dev', 'active', 'publish');
		INSERT INTO assets (snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key, parent_logical_asset_id, title, description, payload_schema, payload_json, content_hash)
		VALUES ('dashboard', 'dashboard:sales.orders', 'sales', 'active', 'dashboard', 'sales.orders', '', 'Orders', '', 'dashboard.v1', '{}', 'one');
		INSERT INTO workspace_active_serving_states (workspace_id, environment, serving_state_id) VALUES ('sales', 'dev', 'active');
		UPDATE assets SET title = 'Revenue', content_hash = 'two' WHERE logical_asset_id = 'dashboard:sales.orders';
	`); err != nil {
		t.Fatal(err)
	}
	repository := searchsqlite.New(db)
	rows, _, err := repository.Candidates(ctx, productsearch.RepositoryQuery{Text: "revenue", Environment: "dev"}, 0, 10)
	if err != nil || len(rows) != 1 || rows[0].Result.Name != "Revenue" {
		t.Fatalf("updated search rows=%#v err=%v", rows, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM assets WHERE logical_asset_id = 'dashboard:sales.orders'`); err != nil {
		t.Fatal(err)
	}
	rows, _, err = repository.Candidates(ctx, productsearch.RepositoryQuery{Text: "revenue", Environment: "dev"}, 0, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("deleted search rows=%#v err=%v", rows, err)
	}
}

func TestRepositorySearchUsesFTSAndHydratesEveryVisualLocation(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "leapview.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	db := store.SQLDB()

	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, title, description) VALUES ('sales', 'Sales Workspace', 'Sales');
		INSERT INTO serving_states (id, workspace_id, environment, status, source) VALUES ('sales_active', 'sales', 'dev', 'active', 'publish');
		INSERT INTO assets (
			snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key,
			parent_logical_asset_id, title, description, payload_schema, payload_json, content_hash
		) VALUES
			('catalog', 'catalog:sales', 'sales', 'sales_active', 'catalog', 'sales', '', 'Sales Workspace', 'Sales', 'catalog.v1', '{}', 'catalog'),
			('dashboard', 'dashboard:sales.executive', 'sales', 'sales_active', 'dashboard', 'sales.executive', 'catalog:sales', 'Executive Sales', 'Executive dashboard', 'dashboard.v1', '{"id":"executive"}', 'dashboard'),
			('page-overview', 'page:sales.executive.overview', 'sales', 'sales_active', 'page', 'sales.executive.overview', 'dashboard:sales.executive', 'Overview', '', 'page.v1', '{"id":"overview"}', 'page-overview'),
			('page-detail', 'page:sales.executive.detail', 'sales', 'sales_active', 'page', 'sales.executive.detail', 'dashboard:sales.executive', 'Detail', '', 'page.v1', '{"id":"detail"}', 'page-detail'),
			('visual', 'visual:sales.executive.orders_by_category', 'sales', 'sales_active', 'visual', 'sales.executive.orders_by_category', 'dashboard:sales.executive', 'Orders by category', 'Order count grouped by category', 'visual.v1', '{"Type":"bar","Query":{"Measures":["order_count"]}}', 'visual'),
			('item-overview', 'page_item:sales.executive.overview.orders', 'sales', 'sales_active', 'page_item', 'sales.executive.overview.orders', 'page:sales.executive.overview', 'Orders by category', '', 'page_item.v1', '{}', 'item-overview'),
			('item-detail', 'page_item:sales.executive.detail.orders', 'sales', 'sales_active', 'page_item', 'sales.executive.detail.orders', 'page:sales.executive.detail', 'Orders by category', '', 'page_item.v1', '{}', 'item-detail');
		INSERT INTO asset_edges (id, workspace_id, serving_state_id, from_logical_asset_id, to_logical_asset_id, edge_type) VALUES
			('edge-overview', 'sales', 'sales_active', 'page_item:sales.executive.overview.orders', 'visual:sales.executive.orders_by_category', 'uses_visual'),
			('edge-detail', 'sales', 'sales_active', 'page_item:sales.executive.detail.orders', 'visual:sales.executive.orders_by_category', 'uses_visual');
		INSERT INTO workspace_active_serving_states (workspace_id, environment, serving_state_id) VALUES ('sales', 'dev', 'sales_active');
	`); err != nil {
		t.Fatalf("seed search graph: %v", err)
	}

	repository := searchsqlite.New(db)
	rows, more, err := repository.Candidates(ctx, productsearch.RepositoryQuery{
		Text: "orders by", Environment: "dev", Context: productsearch.SearchContext{
			WorkspaceID: "sales", DashboardID: "executive", PageID: "detail",
		},
	}, 0, 20)
	if err != nil {
		t.Fatalf("search candidates: %v", err)
	}
	if more {
		t.Fatal("search unexpectedly has more candidates")
	}
	var visual *productsearch.Result
	for index := range rows {
		if rows[index].Result.Reference.Type == productsearch.TypeVisual {
			visual = &rows[index].Result
			break
		}
	}
	if visual == nil {
		t.Fatalf("visual missing from candidates: %#v", rows)
	}
	if visual.Reference.ID != "executive.orders_by_category" {
		t.Fatalf("visual reference = %#v", visual.Reference)
	}
	if visual.VisualType != "bar" {
		t.Fatalf("visual type = %q, want bar", visual.VisualType)
	}
	if len(visual.Locations) != 2 || visual.Locations[0].PageID != "detail" || visual.Href != visual.Locations[0].Href {
		t.Fatalf("visual locations = %#v href=%q, want contextual detail first and both pages", visual.Locations, visual.Href)
	}
	if len(visual.Context) == 0 || visual.Context[0] != productsearch.ContextCurrentPage {
		t.Fatalf("visual context = %#v, want current page", visual.Context)
	}
}

func TestRepositoryHydratesMeasureSemanticModelHierarchy(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "leapview.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.SQLDB()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, title, description) VALUES ('sales', 'Sales', '');
		INSERT INTO serving_states (id, workspace_id, environment, status, source) VALUES ('active', 'sales', 'dev', 'active', 'publish');
		INSERT INTO assets (
			snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key,
			parent_logical_asset_id, title, description, payload_schema, payload_json, content_hash
		) VALUES
			('catalog', 'catalog:sales', 'sales', 'active', 'catalog', 'sales', '', 'Sales', '', 'catalog.v1', '{}', 'catalog'),
			('model', 'semantic_model:sales.orders', 'sales', 'active', 'semantic_model', 'sales.orders', 'catalog:sales', 'Orders', '', 'semantic_model.v1', '{}', 'model'),
			('measure', 'measure:sales.orders.revenue', 'sales', 'active', 'measure', 'sales.orders.revenue', 'semantic_model:sales.orders', 'Revenue', '', 'measure.v1', '{}', 'measure');
		INSERT INTO workspace_active_serving_states (workspace_id, environment, serving_state_id) VALUES ('sales', 'dev', 'active');
	`); err != nil {
		t.Fatal(err)
	}

	repository := searchsqlite.New(db)
	rows, _, err := repository.Candidates(ctx, productsearch.RepositoryQuery{
		Text: "revenue", Environment: "dev", Types: []productsearch.Type{productsearch.TypeMeasure},
	}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("measure candidates = %#v", rows)
	}
	hierarchy := rows[0].Result.Hierarchy
	if len(hierarchy) != 1 || hierarchy[0].Type != productsearch.TypeSemanticModel || hierarchy[0].Name != "Orders" {
		t.Fatalf("measure hierarchy = %#v, want Orders semantic model", hierarchy)
	}
}
