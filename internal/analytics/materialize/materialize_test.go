package materialize_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	analyticsmaterialize "github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
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

func TestRunServicePersistsQueuedRunningAndSucceededStates(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())
	runner := &recordingRefreshRunner{}
	service := analyticsmaterialize.RunService{Repo: repo, Runner: runner}

	queued, err := service.Enqueue(ctx, analyticsmaterialize.RunInput{
		WorkspaceID:  "test",
		ModelID:      "model.orders",
		DeploymentID: "dep_1",
	})
	if err != nil {
		t.Fatalf("enqueue run: %v", err)
	}
	if queued.Status != analyticsmaterialize.RunStatusQueued || queued.ModelID != "model.orders" || queued.DeploymentID != "dep_1" {
		t.Fatalf("queued run = %#v", queued)
	}

	finished, err := service.Execute(ctx, "test", queued.ID)
	if err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if finished.Status != analyticsmaterialize.RunStatusSucceeded || finished.FinishedAt == "" || runner.modelID != "model.orders" {
		t.Fatalf("finished run = %#v runner=%#v", finished, runner)
	}
	stored, err := repo.GetRun(ctx, "test", queued.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if stored.Status != analyticsmaterialize.RunStatusSucceeded || stored.ModelID != "model.orders" || stored.DeploymentID != "dep_1" {
		t.Fatalf("stored run = %#v", stored)
	}
}

func TestRunServicePersistsFailedStateWithError(t *testing.T) {
	ctx := context.Background()
	store := openMaterializationStore(t, ctx)
	defer store.Close()
	repo := analyticsmaterialize.NewSQLRunRepository(store.SQLDB())
	service := analyticsmaterialize.RunService{Repo: repo, Runner: failingRefreshRunner{}}
	queued, err := service.Enqueue(ctx, analyticsmaterialize.RunInput{WorkspaceID: "test", ModelID: "model.orders"})
	if err != nil {
		t.Fatalf("enqueue run: %v", err)
	}

	failed, err := service.Execute(ctx, "test", queued.ID)
	if err == nil {
		t.Fatal("execute run unexpectedly succeeded")
	}
	if failed.Status != analyticsmaterialize.RunStatusFailed || failed.Error == "" || failed.FinishedAt == "" {
		t.Fatalf("failed run = %#v err=%v", failed, err)
	}
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

type recordingRefreshRunner struct {
	modelID string
}

func (r *recordingRefreshRunner) RefreshMaterializations(_ context.Context, modelID string) error {
	r.modelID = modelID
	return nil
}

type failingRefreshRunner struct{}

func (failingRefreshRunner) RefreshMaterializations(context.Context, string) error {
	return errors.New("refresh failed")
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
		INSERT INTO deployments (id, workspace_id, status, digest, manifest_json, created_by)
		VALUES ('dep_1', 'test', 'active', 'sha256:test', '{}', 'test')
	`); err != nil {
		t.Fatalf("seed deployment: %v", err)
	}
	return store
}
