package materialize_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	analyticsmaterialize "github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
	_ "github.com/duckdb/duckdb-go/v2"
)

func TestModelTableExecutesPlannedSQL(t *testing.T) {
	model := &semanticmodel.Model{
		Name: "test",
		Connections: map[string]semanticmodel.Connection{
			"local_files": {Kind: "local"},
		},
		Sources: map[string]semanticmodel.Source{
			"orders": {Path: "orders.csv", Format: "csv", Connection: "local_files"},
		},
		BaseTable: "orders",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source:     "orders",
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Label: "Order ID"},
					"status":   {Label: "Status"},
				},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)", Label: "Revenue"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	sources := &recordingSourceRegistrar{plan: analyticsmaterialize.ModelTablePlan{Mode: analyticsmaterialize.PlanModeDirectSourceRead, SQL: "CREATE OR REPLACE TABLE model.orders AS SELECT 1 AS order_id"}}
	executor := &recordingStatementsExecutor{}
	if _, err := analyticsmaterialize.Refresh(context.Background(), executor, sources, model); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sources.ops, []string{"prepare", "plan:orders"}) {
		t.Fatalf("source ops = %#v, want prepare/plan", sources.ops)
	}
	if !reflect.DeepEqual(executor.statements, []string{"CREATE SCHEMA IF NOT EXISTS model", "CREATE OR REPLACE TABLE model.orders AS SELECT 1 AS order_id"}) {
		t.Fatalf("statements = %#v, want planned SQL", executor.statements)
	}
}

func TestModelTablePlannerErrorStopsMaterialization(t *testing.T) {
	model := &semanticmodel.Model{
		Name:        "test",
		Connections: map[string]semanticmodel.Connection{"local_files": {Kind: "local"}},
		Sources:     map[string]semanticmodel.Source{"orders": {Path: "orders.csv", Format: "csv", Connection: "local_files"}},
		BaseTable:   "orders",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source:     "orders",
				PrimaryKey: "order_id",
				Columns: map[string]semanticmodel.ModelColumn{
					"order_id": {SourceField: "raw_order_id"},
					"status":   {},
					"revenue":  {SourceField: "gross_revenue"},
				},
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Label: "Order ID"},
					"status":   {Label: "Status"},
				},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)", Label: "Revenue"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	sources := &recordingSourceRegistrar{planErr: errors.New("plan failed")}
	executor := &recordingStatementsExecutor{}
	if _, err := analyticsmaterialize.Refresh(context.Background(), executor, sources, model); err == nil || !strings.Contains(err.Error(), "plan failed") {
		t.Fatalf("Refresh() error = %v, want plan failed", err)
	}
	if !reflect.DeepEqual(executor.statements, []string{"CREATE SCHEMA IF NOT EXISTS model"}) {
		t.Fatalf("statements = %#v, want only schema setup", executor.statements)
	}
}

func TestSQLModelTableUsesPlannedSQL(t *testing.T) {
	model := &semanticmodel.Model{
		Name: "test",
		Connections: map[string]semanticmodel.Connection{
			"local_files": {Kind: "local"},
		},
		Sources: map[string]semanticmodel.Source{
			"orders": {
				Path:       "orders.csv",
				Format:     "csv",
				Connection: "local_files",
				Fields: map[string]semanticmodel.SourceField{
					"unwanted_source_level_field": {},
				},
			},
		},
		BaseTable: "orders",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Sources:    []string{"orders"},
				Transform:  semanticmodel.Transform{SQL: "SELECT order_id, revenue FROM source.orders"},
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}, "revenue": {Label: "Revenue"}},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)", Label: "Revenue"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	sources := &recordingSourceRegistrar{plan: analyticsmaterialize.ModelTablePlan{Mode: analyticsmaterialize.PlanModeProjectedSourceInline, SQL: "CREATE OR REPLACE TABLE model.orders AS SELECT order_id, revenue FROM read_csv('orders.csv')"}}
	executor := &recordingStatementsExecutor{}
	if _, err := analyticsmaterialize.Refresh(context.Background(), executor, sources, model); err != nil {
		t.Fatal(err)
	}
	if len(executor.statements) != 2 || !strings.Contains(executor.statements[1], "read_csv") {
		t.Fatalf("statements = %#v, want planned inline SQL", executor.statements)
	}
}

func TestModelTableExecutionErrorReturnsMaterializationError(t *testing.T) {
	model := &semanticmodel.Model{
		Name:        "test",
		Connections: map[string]semanticmodel.Connection{"local_files": {Kind: "local"}},
		Sources:     map[string]semanticmodel.Source{"orders": {Path: "orders.csv", Format: "csv", Connection: "local_files"}},
		BaseTable:   "orders",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source:     "orders",
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Label: "Order ID"}},
			},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	sources := &recordingSourceRegistrar{plan: analyticsmaterialize.ModelTablePlan{Mode: analyticsmaterialize.PlanModeDirectSourceRead, SQL: "CREATE OR REPLACE TABLE model.orders AS SELECT 1"}}
	if _, err := analyticsmaterialize.Refresh(context.Background(), failingExecutor{}, sources, model); err == nil {
		t.Fatal("refresh unexpectedly succeeded")
	}
	want := []string{"prepare"}
	if !reflect.DeepEqual(sources.ops, want) {
		t.Fatalf("source ops = %#v, want %#v", sources.ops, want)
	}
}

func TestModelTablesMaterializeAfterModelDependencies(t *testing.T) {
	model := &semanticmodel.Model{
		Name: "test",
		Connections: map[string]semanticmodel.Connection{
			"local_files": {Kind: "local"},
		},
		Sources: map[string]semanticmodel.Source{
			"orders": {Path: "orders.csv", Format: "csv", Connection: "local_files"},
		},
		BaseTable: "order_summary",
		Tables: map[string]semanticmodel.Table{
			"order_summary": {
				Sources:    []string{},
				PrimaryKey: "status",
				Transform:  semanticmodel.Transform{SQL: "SELECT status, SUM(revenue) AS revenue FROM model.orders GROUP BY status"},
				Dimensions: map[string]semanticmodel.MetricDimension{
					"status":  {Label: "Status"},
					"revenue": {Label: "Revenue"},
				},
			},
			"orders": {
				Source:     "orders",
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Label: "Order ID"},
					"status":   {Label: "Status"},
					"revenue":  {Label: "Revenue"},
				},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Table: "order_summary", Grain: "status", Expression: "SUM(order_summary.revenue)", Label: "Revenue"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	executor := &recordingStatementsExecutor{}
	if _, err := analyticsmaterialize.Refresh(context.Background(), executor, &recordingSourceRegistrar{}, model); err != nil {
		t.Fatal(err)
	}
	if len(executor.statements) != 3 {
		t.Fatalf("statements = %#v, want schema setup and two materializations", executor.statements)
	}
	if !strings.Contains(executor.statements[1], "model.orders") || !strings.Contains(executor.statements[2], "model.order_summary") {
		t.Fatalf("materialization order = %#v, want orders before order_summary", executor.statements)
	}
}

func TestMovieLensModelTableOrderMaterializesDimensionsBeforeFacts(t *testing.T) {
	model := &semanticmodel.Model{
		Name:        "movielens",
		Connections: map[string]semanticmodel.Connection{"movielens": {Kind: "local"}},
		Sources: map[string]semanticmodel.Source{
			"ratings": {Path: "ratings.csv", Format: "csv", Connection: "movielens"},
			"movies":  {Path: "movies.csv", Format: "csv", Connection: "movielens"},
			"tags":    {Path: "tags.csv", Format: "csv", Connection: "movielens"},
			"links":   {Path: "links.csv", Format: "csv", Connection: "movielens"},
		},
		BaseTable: "ratings",
		Tables: map[string]semanticmodel.Table{
			"ratings": {
				Source:     "ratings",
				PrimaryKey: "rating_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"rating_id": {Label: "Rating ID"}},
			},
			"movies": {
				Sources:    []string{"movies", "links"},
				Transform:  semanticmodel.Transform{SQL: "SELECT m.movieId AS movie_id FROM source.movies m LEFT JOIN source.links l ON l.movieId = m.movieId"},
				PrimaryKey: "movie_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"movie_id": {Label: "Movie ID"}},
			},
			"users": {
				Sources:    []string{"ratings", "tags"},
				Transform:  semanticmodel.Transform{SQL: "SELECT r.userId AS user_id FROM source.ratings r LEFT JOIN source.tags t ON t.userId = r.userId"},
				PrimaryKey: "user_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"user_id": {Label: "User ID"}},
			},
			"rating_genres": {
				Transform:  semanticmodel.Transform{SQL: "SELECT rating_id, genre FROM model.ratings JOIN model.movies USING (movie_id)"},
				PrimaryKey: "rating_genre_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"rating_genre_id": {Label: "Rating Genre ID"}},
			},
			"tags": {
				Source:     "tags",
				PrimaryKey: "tag_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"tag_id": {Label: "Tag ID"}},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"rating_count": {Table: "ratings", Grain: "rating_id", Expression: "COUNT(*)", Label: "Ratings"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}

	order, err := analyticsmaterialize.ModelTableOrder(model)
	if err != nil {
		t.Fatal(err)
	}
	ratingsIndex := indexOf(order, "ratings")
	moviesIndex := indexOf(order, "movies")
	ratingGenresIndex := indexOf(order, "rating_genres")
	if ratingsIndex < 0 || moviesIndex < 0 || ratingGenresIndex < 0 {
		t.Fatalf("order = %#v, want ratings, movies, and rating_genres", order)
	}
	if ratingGenresIndex < ratingsIndex || ratingGenresIndex < moviesIndex {
		t.Fatalf("order = %#v, want rating_genres after ratings and movies", order)
	}
}

func TestWorkspaceRuntimeCommitsModelTablesInOneDuckLakeSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "orders.csv"), []byte("order_id,status,revenue\no1,paid,10\no2,paid,15\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := &semanticmodel.Model{
		Name:        "workspace",
		Connections: map[string]semanticmodel.Connection{"local_files": {Kind: "local"}},
		Sources: map[string]semanticmodel.Source{
			"orders": {Path: "orders.csv", Format: "csv", Connection: "local_files"},
		},
		BaseTable: "order_summary",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source:     "orders",
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Label: "Order ID"},
					"status":   {Label: "Status"},
					"revenue":  {Label: "Revenue"},
				},
			},
			"order_summary": {
				ModelDependencies: []string{"orders"},
				Transform:         semanticmodel.Transform{SQL: "SELECT status, SUM(revenue) AS revenue FROM model.orders GROUP BY status"},
				PrimaryKey:        "status",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"status":  {Label: "Status"},
					"revenue": {Label: "Revenue"},
				},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Table: "order_summary", Grain: "status", Expression: "SUM(order_summary.revenue)", Label: "Revenue"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}

	runtime, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, analyticsduckdb.WorkspaceRuntimeConfig{
		Models:  map[string]*semanticmodel.Model{"sales": model},
		DataDir: dir,
		DBDir:   dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if runtime.DuckLakeSnapshotID() <= 0 {
		t.Fatalf("DuckLakeSnapshotID = %d, want committed snapshot", runtime.DuckLakeSnapshotID())
	}

	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		"LOAD sqlite",
		"LOAD ducklake",
		"ATTACH 'ducklake:sqlite:" + strings.ReplaceAll(filepath.Join(dir, "catalog.sqlite"), "'", "''") + "' AS lake",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	var matchingSnapshots int
	if err := db.QueryRowContext(ctx, `
SELECT count(*)
FROM lake.snapshots()
WHERE snapshot_id = ?
  AND CAST(changes AS VARCHAR) LIKE '%model.orders%'
  AND CAST(changes AS VARCHAR) LIKE '%model.order_summary%'`, runtime.DuckLakeSnapshotID()).Scan(&matchingSnapshots); err != nil {
		t.Fatal(err)
	}
	if matchingSnapshots != 1 {
		t.Fatalf("matching committed snapshots = %d, want 1", matchingSnapshots)
	}
}

func TestWorkspaceRuntimeUsesPlatformDBAsDuckLakeCatalog(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "orders.csv"), []byte("order_id,status,revenue\no1,paid,10\no2,paid,15\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(dir, "libredash.db")
	store, err := platform.Open(ctx, catalogPath)
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "sales", Title: "Sales"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	model := &semanticmodel.Model{
		Name:        "workspace",
		Connections: map[string]semanticmodel.Connection{"local_files": {Kind: "local"}},
		Sources: map[string]semanticmodel.Source{
			"orders": {Path: "orders.csv", Format: "csv", Connection: "local_files"},
		},
		BaseTable: "orders",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source:     "orders",
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Label: "Order ID"},
					"status":   {Label: "Status"},
					"revenue":  {Label: "Revenue"},
				},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)", Label: "Revenue"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	duckRoot := filepath.Join(dir, "duckdb", "dev")
	duckLakeDataPath := filepath.Join(dir, ".libredash", "data")
	runtime, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, analyticsduckdb.WorkspaceRuntimeConfig{
		Models:           map[string]*semanticmodel.Model{"sales": model},
		DataDir:          dataDir,
		DBDir:            duckRoot,
		CatalogPath:      catalogPath,
		DuckLakeDataPath: duckLakeDataPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if runtime.DuckLakeSnapshotID() <= 0 {
		t.Fatalf("DuckLakeSnapshotID = %d, want committed snapshot", runtime.DuckLakeSnapshotID())
	}
	if _, err := os.Stat(filepath.Join(duckRoot, "catalog.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("workspace-local catalog exists or stat failed: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, "SELECT 1 FROM workspaces WHERE id = 'sales'"); err != nil {
		t.Fatalf("platform control-plane table unavailable after materialization: %v", err)
	}

	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		"LOAD sqlite",
		"LOAD ducklake",
		"ATTACH 'ducklake:sqlite:" + strings.ReplaceAll(catalogPath, "'", "''") + "' AS lake (DATA_PATH '" + strings.ReplaceAll(duckLakeDataPath, "'", "''") + "')",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	var physicalTables int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM ducklake_table_info('lake') WHERE table_name = 'orders'").Scan(&physicalTables); err != nil {
		t.Fatal(err)
	}
	if physicalTables != 1 {
		t.Fatalf("DuckLake physical tables = %d, want one orders table in platform catalog", physicalTables)
	}
}

func TestWorkspaceRuntimeQueriesPinnedDuckLakeSnapshots(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(dir, "libredash.db")
	store, err := platform.Open(ctx, catalogPath)
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	model := simpleOrdersModel(t)
	duckRoot := filepath.Join(dir, "duckdb", "dev")
	config := analyticsduckdb.WorkspaceRuntimeConfig{
		Models:           map[string]*semanticmodel.Model{"sales": model},
		DataDir:          dataDir,
		DBDir:            duckRoot,
		CatalogPath:      catalogPath,
		DuckLakeDataPath: filepath.Join(dir, ".libredash", "data"),
	}

	writeOrdersCSV(t, dataDir, 10, 15)
	writer, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	snapshot1 := writer.DuckLakeSnapshotID()
	writeOrdersCSV(t, dataDir, 100, 150)
	if err := writer.Refresh(ctx); err != nil {
		t.Fatalf("refresh second snapshot: %v", err)
	}
	snapshot2 := writer.DuckLakeSnapshotID()
	if snapshot2 <= snapshot1 {
		t.Fatalf("snapshot2 = %d, want > snapshot1 %d", snapshot2, snapshot1)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	config.SnapshotID = snapshot1
	first, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, config)
	if err != nil {
		t.Fatalf("open first snapshot: %v", err)
	}
	defer first.Close()
	if got := queryRevenue(t, ctx, first); got != 25 {
		t.Fatalf("first snapshot revenue = %v, want 25", got)
	}

	config.SnapshotID = snapshot2
	second, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, config)
	if err != nil {
		t.Fatalf("open second snapshot: %v", err)
	}
	defer second.Close()
	if got := queryRevenue(t, ctx, second); got != 250 {
		t.Fatalf("second snapshot revenue = %v, want 250", got)
	}
}

func TestWorkspaceRuntimeWritesDuckLakeDataUnderConfiguredInstanceStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "source")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeOrdersCSV(t, dataDir, 10, 15)
	catalogPath := filepath.Join(dir, ".libredash", "libredash.db")
	dataPath := filepath.Join(dir, ".libredash", "data")
	oldEnvironmentDataPath := filepath.Join(dir, ".libredash", "duckdb", "dev", "data")

	runtime, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, analyticsduckdb.WorkspaceRuntimeConfig{
		Models:           map[string]*semanticmodel.Model{"sales": simpleOrdersModel(t)},
		DataDir:          dataDir,
		DBDir:            filepath.Join(dir, ".libredash", "duckdb", "dev"),
		CatalogPath:      catalogPath,
		DuckLakeDataPath: dataPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	info, err := os.Stat(dataPath)
	if err != nil {
		t.Fatalf("DuckLake data path was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("DuckLake data path %s is not a directory", dataPath)
	}
	if _, err := os.Stat(oldEnvironmentDataPath); !os.IsNotExist(err) {
		t.Fatalf("old environment-scoped DuckLake data path exists or stat failed: %v", err)
	}
}

func TestWorkspaceRuntimeWritesDuckLakeCommitMetadata(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "source")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeOrdersCSV(t, dataDir, 10, 15)
	catalogPath := filepath.Join(dir, ".libredash", "libredash.db")
	dataPath := filepath.Join(dir, ".libredash", "data")

	runtime, err := analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, analyticsduckdb.WorkspaceRuntimeConfig{
		Models:           map[string]*semanticmodel.Model{"sales": simpleOrdersModel(t)},
		DataDir:          dataDir,
		DBDir:            filepath.Join(dir, ".libredash", "duckdb", "dev"),
		CatalogPath:      catalogPath,
		DuckLakeDataPath: dataPath,
		ServingStateID:   "dep_123",
		WorkspaceID:      "sales",
		Environment:      "prod",
		SemanticDigest:   "semantic-digest",
		ArtifactDigest:   "artifact-digest",
		SourceDataDigest: "source-digest",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshotID := runtime.DuckLakeSnapshotID()
	defer runtime.Close()

	extra := duckLakeSnapshotExtraInfo(t, ctx, catalogPath, dataPath, snapshotID)
	for _, want := range []string{
		`"servingStateId":"dep_123"`,
		`"workspaceId":"sales"`,
		`"environment":"prod"`,
		`"semanticModelDigest":"semantic-digest"`,
		`"artifactDigest":"artifact-digest"`,
		`"sourceDataDigest":"source-digest"`,
	} {
		if !strings.Contains(extra, want) {
			t.Fatalf("DuckLake snapshot metadata missing %s in %s", want, extra)
		}
	}
}

func duckLakeSnapshotExtraInfo(t *testing.T, ctx context.Context, catalogPath, dataPath string, snapshotID int64) string {
	t.Helper()
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		"LOAD sqlite",
		"LOAD ducklake",
		"ATTACH 'ducklake:sqlite:" + strings.ReplaceAll(catalogPath, "'", "''") + "' AS lake (DATA_PATH '" + strings.ReplaceAll(dataPath, "'", "''") + "')",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	var extra string
	if err := db.QueryRowContext(ctx, "SELECT CAST(commit_extra_info AS VARCHAR) FROM lake.snapshots() WHERE snapshot_id = ?", snapshotID).Scan(&extra); err != nil {
		t.Fatal(err)
	}
	return extra
}

func simpleOrdersModel(t *testing.T) *semanticmodel.Model {
	t.Helper()
	model := &semanticmodel.Model{
		Name:        "workspace",
		Connections: map[string]semanticmodel.Connection{"local_files": {Kind: "local"}},
		Sources: map[string]semanticmodel.Source{
			"orders": {Path: "orders.csv", Format: "csv", Connection: "local_files"},
		},
		BaseTable: "orders",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source:     "orders",
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Label: "Order ID"},
					"status":   {Label: "Status"},
					"revenue":  {Label: "Revenue"},
				},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
			"revenue": {Table: "orders", Grain: "order_id", Expression: "SUM(orders.revenue)", Label: "Revenue"},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	return model
}

func writeOrdersCSV(t *testing.T, dir string, firstRevenue, secondRevenue int) {
	t.Helper()
	content := fmt.Sprintf("order_id,status,revenue\no1,paid,%d\no2,paid,%d\n", firstRevenue, secondRevenue)
	if err := os.WriteFile(filepath.Join(dir, "orders.csv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func queryRevenue(t *testing.T, ctx context.Context, runtime *analyticsduckdb.WorkspaceRuntime) float64 {
	t.Helper()
	queries, err := runtime.Queries("sales")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := queries.Query(ctx, semanticquery.Request{
		Measures: []semanticquery.Field{{Field: "revenue", Alias: "revenue"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v, want one row", rows)
	}
	switch value := rows[0]["revenue"].(type) {
	case int64:
		return float64(value)
	case float64:
		return value
	case *big.Int:
		return float64(value.Int64())
	default:
		t.Fatalf("revenue value = %#v (%T), want numeric", rows[0]["revenue"], rows[0]["revenue"])
		return 0
	}
}

func TestModelTableDependencyOrderIncludesUpstreamBeforeSelected(t *testing.T) {
	model := &semanticmodel.Model{
		Name:      "test",
		BaseTable: "daily_summary",
		Tables: map[string]semanticmodel.Table{
			"customers":     {PrimaryKey: "customer_id"},
			"orders":        {PrimaryKey: "order_id"},
			"order_summary": {PrimaryKey: "status", Transform: semanticmodel.Transform{SQL: "SELECT status FROM model.orders"}, ModelDependencies: []string{"orders"}},
			"daily_summary": {PrimaryKey: "day", ModelDependencies: []string{"order_summary", "customers"}},
		},
	}

	order, err := analyticsmaterialize.ModelTableDependencyOrder(model, "daily_summary")
	if err != nil {
		t.Fatalf("dependency order: %v", err)
	}
	want := []string{"orders", "order_summary", "customers", "daily_summary"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("dependency order = %#v, want %#v", order, want)
	}
}

func indexOf(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}

func TestModelTablesNamedMaterializesOnlyRequestedOrder(t *testing.T) {
	model := &semanticmodel.Model{
		Name: "test",
		Connections: map[string]semanticmodel.Connection{
			"local_files": {Kind: "local"},
		},
		Sources: map[string]semanticmodel.Source{
			"orders": {Path: "orders.csv", Format: "csv", Connection: "local_files"},
		},
		BaseTable: "order_summary",
		Tables: map[string]semanticmodel.Table{
			"orders":        {Source: "orders", PrimaryKey: "order_id"},
			"customers":     {Source: "orders", PrimaryKey: "customer_id"},
			"order_summary": {PrimaryKey: "status", Transform: semanticmodel.Transform{SQL: "SELECT status FROM model.orders"}, ModelDependencies: []string{"orders"}},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	sources := &recordingSourceRegistrar{}
	executor := &recordingStatementsExecutor{}

	if _, err := analyticsmaterialize.RefreshModelTables(context.Background(), executor, sources, model, []string{"orders", "order_summary"}); err != nil {
		t.Fatalf("refresh model tables: %v", err)
	}
	if !reflect.DeepEqual(sources.ops, []string{"prepare", "plan:orders", "plan:order_summary"}) {
		t.Fatalf("source ops = %#v, want selected tables only", sources.ops)
	}
	if len(executor.statements) != 3 || strings.Contains(strings.Join(executor.statements, "\n"), "customers") {
		t.Fatalf("statements = %#v, want schema plus selected table materializations", executor.statements)
	}
}

func TestRegistersCSVSourcesAndMaterializesModelTables(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "orders.csv", "order_id,revenue\no1,10.50\no2,20.25\n")
	db, err := analyticsduckdb.Open(context.Background(), filepath.Join(dir, "test.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	model := &semanticmodel.Model{
		Name:              "test",
		DefaultConnection: "local_files",
		Connections: map[string]semanticmodel.Connection{
			"local_files": {
				Kind:     "local",
				Defaults: semanticmodel.ConnectionDefaults{Options: map[string]any{"header": true}},
			},
		},
		Sources: map[string]semanticmodel.Source{
			"orders": {Path: "orders.csv", Connection: "local_files"},
		},
		BaseTable: "orders",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Kind: "fact", Sources: []string{"orders"},
				Transform: semanticmodel.Transform{SQL: `
					SELECT order_id, try_cast(revenue AS DOUBLE) AS revenue
					FROM source.orders
				`},
				PrimaryKey: "order_id",
				Grain:      "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Expr: "order_id"}},
				Measures:   map[string]semanticmodel.MetricMeasure{"revenue": {Label: "Revenue", Expression: "SUM(orders.revenue)"}},
			},
		},
	}
	if err := model.Validate(); err != nil {
		t.Fatalf("validate model: %v", err)
	}
	if _, err := analyticsmaterialize.Refresh(context.Background(), db, analyticsduckdb.NewSourceRuntime(db, dir), model); err != nil {
		t.Fatalf("refresh materializations: %v", err)
	}

	var total float64
	if err := db.SQLDB().QueryRowContext(context.Background(), "SELECT SUM(revenue) FROM model.orders").Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 30.75 {
		t.Fatalf("total revenue = %v, want 30.75", total)
	}
	var rawObjects int
	if err := db.SQLDB().QueryRowContext(context.Background(), "SELECT count(*) FROM duckdb_tables() WHERE schema_name = 'raw'").Scan(&rawObjects); err != nil {
		t.Fatal(err)
	}
	if rawObjects != 0 {
		t.Fatalf("raw schema object count = %d, want 0", rawObjects)
	}
	var sourceObjects int
	if err := db.SQLDB().QueryRowContext(context.Background(), "SELECT count(*) FROM duckdb_views() WHERE schema_name = 'source'").Scan(&sourceObjects); err != nil {
		t.Fatal(err)
	}
	if sourceObjects != 0 {
		t.Fatalf("source schema view count = %d, want 0", sourceObjects)
	}
}

type recordingSourceRegistrar struct {
	plan    analyticsmaterialize.ModelTablePlan
	planErr error
	ops     []string
}

func (r *recordingSourceRegistrar) PrepareSourceRuntime(_ context.Context, _ *semanticmodel.Model) error {
	r.ops = append(r.ops, "prepare")
	return nil
}

func (r *recordingSourceRegistrar) PlanModelTable(_ context.Context, _ *semanticmodel.Model, tableName string, _ semanticmodel.Table) (analyticsmaterialize.ModelTablePlan, error) {
	r.ops = append(r.ops, "plan:"+tableName)
	if r.planErr != nil {
		return analyticsmaterialize.ModelTablePlan{}, r.planErr
	}
	if r.plan.SQL != "" {
		return r.plan, nil
	}
	return analyticsmaterialize.ModelTablePlan{Mode: analyticsmaterialize.PlanModeModelSQL, SQL: "CREATE OR REPLACE TABLE model." + tableName + " AS SELECT 1"}, nil
}

type recordingExecutor struct{}

func (recordingExecutor) Exec(context.Context, string) error {
	return nil
}

type failingExecutor struct{}

func (failingExecutor) Exec(context.Context, string) error {
	return errors.New("exec failed")
}

type recordingStatementsExecutor struct {
	statements []string
}

func (r *recordingStatementsExecutor) Exec(_ context.Context, statement string) error {
	r.statements = append(r.statements, statement)
	return nil
}

func TestRegistersDatabaseSourceTwice(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.sqlite")
	db, err := analyticsduckdb.Open(context.Background(), filepath.Join(dir, "test.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.SQLDB().ExecContext(context.Background(), "INSTALL sqlite"); err != nil {
		t.Skipf("sqlite extension unavailable: %v", err)
	}
	if _, err := db.SQLDB().ExecContext(context.Background(), "LOAD sqlite"); err != nil {
		t.Skipf("sqlite extension unavailable: %v", err)
	}
	if _, err := db.SQLDB().ExecContext(context.Background(), "ATTACH '"+analyticsduckdb.SQLString(sourcePath)+"' AS seed (TYPE sqlite)"); err != nil {
		t.Fatalf("attach seed sqlite: %v", err)
	}
	if _, err := db.SQLDB().ExecContext(context.Background(), "CREATE TABLE seed.accounts (id INTEGER, name VARCHAR)"); err != nil {
		t.Fatalf("create seed table: %v", err)
	}
	if _, err := db.SQLDB().ExecContext(context.Background(), "INSERT INTO seed.accounts VALUES (1, 'Acme')"); err != nil {
		t.Fatalf("insert seed table: %v", err)
	}
	if _, err := db.SQLDB().ExecContext(context.Background(), "DETACH seed"); err != nil {
		t.Fatalf("detach seed sqlite: %v", err)
	}

	model := &semanticmodel.Model{
		Name: "test",
		Connections: map[string]semanticmodel.Connection{
			"crm": {Kind: "sqlite", Options: map[string]any{"path": sourcePath}},
		},
		Sources: map[string]semanticmodel.Source{
			"accounts": {Connection: "crm", Object: "accounts"},
		},
		BaseTable: "accounts",
		Tables: map[string]semanticmodel.Table{
			"accounts": {
				Kind: "dimension", Source: "accounts", PrimaryKey: "id", Grain: "id",
				Dimensions: map[string]semanticmodel.MetricDimension{"id": {Expr: "id"}, "name": {Expr: "name"}},
			},
		},
	}
	sources := analyticsduckdb.NewSourceRuntime(db, dir)
	for i := 0; i < 2; i++ {
		if _, err := analyticsmaterialize.Refresh(context.Background(), db, sources, model); err != nil {
			t.Fatalf("refresh pass %d: %v", i+1, err)
		}
	}

	var name string
	if err := db.SQLDB().QueryRowContext(context.Background(), "SELECT name FROM model.accounts WHERE id = 1").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Acme" {
		t.Fatalf("name = %q, want Acme", name)
	}
}

func TestValidateFilesIgnoresRemoteSources(t *testing.T) {
	model := &semanticmodel.Model{
		Connections: map[string]semanticmodel.Connection{
			"lake": {Kind: "s3"},
		},
		Sources: map[string]semanticmodel.Source{
			"events": {Format: "parquet", Path: "s3://bucket/events/*.parquet", Connection: "lake"},
		},
	}
	if err := analyticsmaterialize.ValidateFiles(model, t.TempDir()); err != nil {
		t.Fatalf("validate files = %v, want nil", err)
	}
}

func TestValidateFilesUsesLocalConnectionRoot(t *testing.T) {
	dir := t.TempDir()
	model := &semanticmodel.Model{
		Connections: map[string]semanticmodel.Connection{
			"local_files": {Kind: "local", Root: "fixtures"},
		},
		Sources: map[string]semanticmodel.Source{
			"orders": {Format: "csv", Path: "orders.csv", Connection: "local_files"},
		},
	}
	err := analyticsmaterialize.ValidateFiles(model, dir)
	var missing *analyticsmaterialize.MissingDataError
	if !errors.As(err, &missing) {
		t.Fatalf("validate files error = %v, want MissingDataError", err)
	}
	want := filepath.Join(dir, "fixtures", "orders.csv")
	if len(missing.Missing) != 1 || missing.Missing[0] != want {
		t.Fatalf("missing files = %#v, want %q", missing.Missing, want)
	}
}

func TestRunRepositoryPersistsPrincipalAttribution(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())
	seedMaterializationPrincipal(t, ctx, store, "principal_alice", "alice@example.com", "Alice")

	queued, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "model.orders", PrincipalID: "principal_alice"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if queued.PrincipalID != "principal_alice" || queued.PrincipalDisplayName != "Alice" {
		t.Fatalf("queued attribution = %#v, want Alice principal", queued)
	}
	if _, err := repo.MarkRunSucceeded(ctx, "test", queued.ID); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}

	stored, err := repo.GetRun(ctx, "test", queued.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if stored.PrincipalID != "principal_alice" || stored.PrincipalDisplayName != "Alice" {
		t.Fatalf("stored attribution = %#v, want Alice principal", stored)
	}
	listed, err := repo.ListModelRuns(ctx, "test", "model.orders", analyticsmaterialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list model runs: %v", err)
	}
	if len(listed) != 1 || listed[0].PrincipalID != "principal_alice" || listed[0].PrincipalDisplayName != "Alice" {
		t.Fatalf("listed attribution = %#v, want Alice principal", listed)
	}
	latest, ok, err := repo.LatestSuccessfulModelRun(ctx, "test", "model.orders")
	if err != nil || !ok || latest.PrincipalDisplayName != "Alice" {
		t.Fatalf("latest attribution = %#v ok=%v err=%v, want Alice", latest, ok, err)
	}

	legacy, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "model.legacy"})
	if err != nil {
		t.Fatalf("create legacy run: %v", err)
	}
	if legacy.PrincipalID != "" || legacy.PrincipalDisplayName != "" {
		t.Fatalf("legacy attribution = %#v, want empty principal fields", legacy)
	}
}

func TestRunRepositoryListsAndFindsLatestByModel(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())

	ordersSucceeded, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "model.orders"})
	if err != nil {
		t.Fatalf("create succeeded run: %v", err)
	}
	if _, err := repo.MarkRunSucceeded(ctx, "test", ordersSucceeded.ID); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
	other, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "model.customers"})
	if err != nil {
		t.Fatalf("create other run: %v", err)
	}
	if _, err := repo.MarkRunSucceeded(ctx, "test", other.ID); err != nil {
		t.Fatalf("mark other succeeded: %v", err)
	}
	ordersFailed, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "model.orders"})
	if err != nil {
		t.Fatalf("create failed run: %v", err)
	}
	if _, err := repo.MarkRunFailed(ctx, "test", ordersFailed.ID, "boom"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	runs, err := repo.ListModelRuns(ctx, "test", "model.orders", analyticsmaterialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list model runs: %v", err)
	}
	if len(runs) != 2 || runs[0].ID != ordersFailed.ID || runs[1].ID != ordersSucceeded.ID {
		t.Fatalf("model runs = %#v, want failed then succeeded orders runs", runs)
	}
	for _, run := range runs {
		if run.ModelID != "model.orders" {
			t.Fatalf("list included wrong model run: %#v", run)
		}
	}

	latest, ok, err := repo.LatestModelRun(ctx, "test", "model.orders")
	if err != nil || !ok || latest.ID != ordersFailed.ID {
		t.Fatalf("latest = %#v ok=%v err=%v, want failed latest", latest, ok, err)
	}
	latestSucceeded, ok, err := repo.LatestSuccessfulModelRun(ctx, "test", "model.orders")
	if err != nil || !ok || latestSucceeded.ID != ordersSucceeded.ID {
		t.Fatalf("latest succeeded = %#v ok=%v err=%v, want older succeeded", latestSucceeded, ok, err)
	}
}

func TestRunRepositoryPagesRunsInSQLOrder(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())

	first, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "model.orders"})
	if err != nil {
		t.Fatalf("create first run: %v", err)
	}
	second, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "model.customers"})
	if err != nil {
		t.Fatalf("create second run: %v", err)
	}
	third, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "model.orders"})
	if err != nil {
		t.Fatalf("create third run: %v", err)
	}

	pageOne, err := repo.ListRuns(ctx, "test", analyticsmaterialize.RunPage{Limit: 2})
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if got, want := runIDs(pageOne), []string{third.ID, second.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first page ids = %#v, want %#v", got, want)
	}
	pageTwo, err := repo.ListRuns(ctx, "test", analyticsmaterialize.RunPage{Limit: 2, After: second.ID})
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if got, want := runIDs(pageTwo), []string{first.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second page ids = %#v, want %#v", got, want)
	}
	unknown, err := repo.ListRuns(ctx, "test", analyticsmaterialize.RunPage{Limit: 2, After: "matrun_missing"})
	if err != nil {
		t.Fatalf("list unknown cursor: %v", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown cursor page = %#v, want empty", unknown)
	}
}

func TestRunRepositoryPagesTargetRunsInSQLOrder(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())

	first, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "olist", TargetType: analyticsmaterialize.TargetModelTable, TargetID: "olist.orders"})
	if err != nil {
		t.Fatalf("create first target run: %v", err)
	}
	if _, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "olist", TargetType: analyticsmaterialize.TargetModelTable, TargetID: "olist.customers"}); err != nil {
		t.Fatalf("create other target run: %v", err)
	}
	second, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "olist", TargetType: analyticsmaterialize.TargetModelTable, TargetID: "olist.orders"})
	if err != nil {
		t.Fatalf("create second target run: %v", err)
	}
	third, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "olist", TargetType: analyticsmaterialize.TargetModelTable, TargetID: "olist.orders"})
	if err != nil {
		t.Fatalf("create third target run: %v", err)
	}

	pageOne, err := repo.ListTargetRuns(ctx, "test", analyticsmaterialize.TargetModelTable, "olist.orders", analyticsmaterialize.RunPage{Limit: 2})
	if err != nil {
		t.Fatalf("list first target page: %v", err)
	}
	if got, want := runIDs(pageOne), []string{third.ID, second.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first target page ids = %#v, want %#v", got, want)
	}
	pageTwo, err := repo.ListTargetRuns(ctx, "test", analyticsmaterialize.TargetModelTable, "olist.orders", analyticsmaterialize.RunPage{Limit: 2, After: second.ID})
	if err != nil {
		t.Fatalf("list second target page: %v", err)
	}
	if got, want := runIDs(pageTwo), []string{first.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second target page ids = %#v, want %#v", got, want)
	}
	unknown, err := repo.ListTargetRuns(ctx, "test", analyticsmaterialize.TargetModelTable, "olist.orders", analyticsmaterialize.RunPage{Limit: 2, After: "matrun_missing"})
	if err != nil {
		t.Fatalf("list unknown target cursor: %v", err)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown target cursor page = %#v, want empty", unknown)
	}
}

func TestRunRepositoryPersistsTargetTriggerAndParentRun(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())

	parent, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{
		WorkspaceID:    "test",
		ModelID:        "olist",
		TargetType:     analyticsmaterialize.TargetSemanticModel,
		TargetID:       "olist",
		TriggerType:    analyticsmaterialize.TriggerDirect,
		ServingStateID: "dep_1",
	})
	if err != nil {
		t.Fatalf("create parent run: %v", err)
	}
	child, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{
		WorkspaceID:    "test",
		ModelID:        "olist",
		TargetType:     analyticsmaterialize.TargetModelTable,
		TargetID:       "olist.orders",
		TriggerType:    analyticsmaterialize.TriggerSemanticModel,
		ParentRunID:    parent.ID,
		ServingStateID: "dep_1",
	})
	if err != nil {
		t.Fatalf("create child run: %v", err)
	}
	if _, err := repo.MarkRunSucceeded(ctx, "test", child.ID); err != nil {
		t.Fatalf("mark child succeeded: %v", err)
	}

	stored, err := repo.GetRun(ctx, "test", child.ID)
	if err != nil {
		t.Fatalf("get child run: %v", err)
	}
	if stored.TargetType != analyticsmaterialize.TargetModelTable || stored.TargetID != "olist.orders" || stored.TriggerType != analyticsmaterialize.TriggerSemanticModel || stored.ParentRunID != parent.ID {
		t.Fatalf("child run metadata = %#v", stored)
	}
	tableRuns, err := repo.ListTargetRuns(ctx, "test", analyticsmaterialize.TargetModelTable, "olist.orders", analyticsmaterialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list table runs: %v", err)
	}
	if len(tableRuns) != 1 || tableRuns[0].ID != child.ID {
		t.Fatalf("table runs = %#v, want child only", tableRuns)
	}
	modelRuns, err := repo.ListModelRuns(ctx, "test", "olist", analyticsmaterialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list model runs: %v", err)
	}
	if len(modelRuns) != 1 || modelRuns[0].ID != parent.ID {
		t.Fatalf("semantic model runs = %#v, want parent only", modelRuns)
	}
	latest, ok, err := repo.LatestSuccessfulTargetRun(ctx, "test", analyticsmaterialize.TargetModelTable, "olist.orders")
	if err != nil || !ok || latest.ID != child.ID {
		t.Fatalf("latest successful table run = %#v ok=%v err=%v, want child", latest, ok, err)
	}

	legacy, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "legacy"})
	if err != nil {
		t.Fatalf("create legacy-shaped run: %v", err)
	}
	if legacy.TargetType != analyticsmaterialize.TargetSemanticModel || legacy.TargetID != "legacy" || legacy.TriggerType != analyticsmaterialize.TriggerDirect {
		t.Fatalf("default target metadata = %#v", legacy)
	}
}

func TestRunRepositoryFailsRunsForTerminalDeployments(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())
	if _, err := store.SQLDB().ExecContext(ctx, `
		INSERT INTO serving_states (id, workspace_id, status, digest, manifest_json, created_by)
		VALUES ('dep_failed', 'test', 'failed', 'sha256:failed', '{}', 'test')
	`); err != nil {
		t.Fatalf("seed failed deployment: %v", err)
	}

	failedDeploymentRun, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{
		WorkspaceID:    "test",
		ModelID:        "olist",
		ServingStateID: "dep_failed",
		TargetType:     analyticsmaterialize.TargetModelTable,
		TargetID:       "olist.orders",
	})
	if err != nil {
		t.Fatalf("create terminal deployment run: %v", err)
	}
	if _, err := repo.MarkRunRunning(ctx, "test", failedDeploymentRun.ID); err != nil {
		t.Fatalf("mark terminal deployment run running: %v", err)
	}
	activeDeploymentRun, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{
		WorkspaceID:    "test",
		ModelID:        "olist",
		ServingStateID: "dep_1",
		TargetType:     analyticsmaterialize.TargetModelTable,
		TargetID:       "olist.customers",
	})
	if err != nil {
		t.Fatalf("create active deployment run: %v", err)
	}
	if _, err := repo.MarkRunRunning(ctx, "test", activeDeploymentRun.ID); err != nil {
		t.Fatalf("mark active deployment run running: %v", err)
	}

	if err := repo.FailRunsForTerminalServingStates(ctx, "refresh did not complete"); err != nil {
		t.Fatalf("fail terminal deployment runs: %v", err)
	}

	storedFailed, err := repo.GetRun(ctx, "test", failedDeploymentRun.ID)
	if err != nil {
		t.Fatalf("get failed deployment run: %v", err)
	}
	if storedFailed.Status != analyticsmaterialize.RunStatusFailed || storedFailed.Error != "refresh did not complete" || storedFailed.FinishedAt == "" {
		t.Fatalf("failed deployment run = %#v, want failed with message and finish time", storedFailed)
	}
	storedActive, err := repo.GetRun(ctx, "test", activeDeploymentRun.ID)
	if err != nil {
		t.Fatalf("get active deployment run: %v", err)
	}
	if storedActive.Status != analyticsmaterialize.RunStatusRunning || storedActive.Error != "" || storedActive.FinishedAt != "" {
		t.Fatalf("active deployment run = %#v, want still running", storedActive)
	}
}

func TestRunRepositoryClaimsExecutableRootJobs(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())

	parent, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{
		WorkspaceID:    "test",
		ModelID:        "olist",
		ServingStateID: "dep_1",
		TargetType:     analyticsmaterialize.TargetSemanticModel,
		TargetID:       "olist",
		JobKind:        analyticsmaterialize.JobKindWorkspaceAssetRefresh,
		PayloadJSON:    `{"assetKey":"olist","assetType":"semantic_model"}`,
	})
	if err != nil {
		t.Fatalf("create parent run: %v", err)
	}
	if _, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{
		WorkspaceID: "test",
		ModelID:     "olist",
		TargetType:  analyticsmaterialize.TargetModelTable,
		TargetID:    "olist.orders",
		ParentRunID: parent.ID,
	}); err != nil {
		t.Fatalf("create child run: %v", err)
	}

	job, ok, err := repo.ClaimNextExecutableJob(ctx, "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	if !ok {
		t.Fatal("expected queued root job")
	}
	if job.RunID != parent.ID || job.Kind != analyticsmaterialize.JobKindWorkspaceAssetRefresh || job.AttemptCount != 1 {
		t.Fatalf("claimed job = %#v, want parent workspace refresh attempt 1", job)
	}
	stored, err := repo.GetRun(ctx, "test", parent.ID)
	if err != nil {
		t.Fatalf("get parent run: %v", err)
	}
	if stored.Status != analyticsmaterialize.RunStatusRunning || stored.StartedAt == "" {
		t.Fatalf("parent run = %#v, want running with start time", stored)
	}
	if _, ok, err := repo.ClaimNextExecutableJob(ctx, "worker-1", time.Minute); err != nil || ok {
		t.Fatalf("second claim ok=%v err=%v, want no child job claimed", ok, err)
	}
}

func TestRunRepositoryReclaimsExpiredJobLease(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())

	run, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "olist"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	job, ok, err := repo.ClaimNextExecutableJob(ctx, "worker-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim ok=%v err=%v", ok, err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, `
		UPDATE refresh_jobs
		SET lease_expires_at = datetime('now', '-1 second')
		WHERE id = ?
	`, job.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	reclaimed, ok, err := repo.ClaimNextExecutableJob(ctx, "worker-2", time.Minute)
	if err != nil {
		t.Fatalf("reclaim job: %v", err)
	}
	if !ok || reclaimed.ID != job.ID || reclaimed.RunID != run.ID || reclaimed.AttemptCount != 2 {
		t.Fatalf("reclaimed job = %#v ok=%v, want same job attempt 2", reclaimed, ok)
	}
	if err := repo.RenewJobLease(ctx, reclaimed.ID, "worker-2", time.Minute); err != nil {
		t.Fatalf("renew lease: %v", err)
	}
	var owner string
	if err := store.SQLDB().QueryRowContext(ctx, `SELECT lease_owner FROM refresh_jobs WHERE id = ?`, reclaimed.ID).Scan(&owner); err != nil {
		t.Fatalf("read lease owner: %v", err)
	}
	if owner != "worker-2" {
		t.Fatalf("lease owner = %q, want worker-2", owner)
	}
}

func TestRunRepositoryReportsDurableQueueStats(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())

	if _, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "queued"}); err != nil {
		t.Fatalf("create queued run: %v", err)
	}
	running, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "running"})
	if err != nil {
		t.Fatalf("create running run: %v", err)
	}
	stale, err := repo.CreateRun(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "stale"})
	if err != nil {
		t.Fatalf("create stale run: %v", err)
	}
	if _, _, err := repo.ClaimNextExecutableJob(ctx, "worker-1", time.Minute); err != nil {
		t.Fatalf("claim queued job: %v", err)
	}
	if _, _, err := repo.ClaimNextExecutableJob(ctx, "worker-1", time.Minute); err != nil {
		t.Fatalf("claim running job: %v", err)
	}
	if _, _, err := repo.ClaimNextExecutableJob(ctx, "worker-1", time.Minute); err != nil {
		t.Fatalf("claim stale job: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, `
		UPDATE refresh_jobs
		SET lease_expires_at = datetime('now', '-1 second')
		WHERE id = (SELECT job_id FROM refresh_job_runs WHERE id = ?)
	`, stale.ID); err != nil {
		t.Fatalf("expire stale lease: %v", err)
	}
	if _, err := repo.MarkRunSucceeded(ctx, "test", running.ID); err != nil {
		t.Fatalf("finish one running run: %v", err)
	}

	stats, err := repo.JobQueueStats(ctx)
	if err != nil {
		t.Fatalf("queue stats: %v", err)
	}
	if stats.QueuedJobs != 0 || stats.RunningJobs != 1 || stats.StaleLeasedJobs != 1 {
		t.Fatalf("queue stats = %#v, want 0 queued, 1 running, 1 stale", stats)
	}
}

func runIDs(runs []analyticsmaterialize.RunRecord) []string {
	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openMaterializationStore(t *testing.T, ctx context.Context) *platform.Store {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, `
		INSERT INTO serving_states (id, workspace_id, status, digest, manifest_json, created_by)
		VALUES ('dep_1', 'test', 'active', 'sha256:test', '{}', 'test')
	`); err != nil {
		t.Fatalf("seed deployment: %v", err)
	}
	return store
}

func seedMaterializationPrincipal(t *testing.T, ctx context.Context, store *platform.Store, id, email, displayName string) {
	t.Helper()
	if _, err := store.SQLDB().ExecContext(ctx, `
		INSERT INTO principals (id, email, display_name)
		VALUES (?, ?, ?)
	`, id, email, displayName); err != nil {
		t.Fatalf("seed principal: %v", err)
	}
}
