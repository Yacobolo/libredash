package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/queryaudit"
)

func TestRepositoryRecordsAndFiltersQueryEvents(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, t.TempDir()+"/libredash.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	repo := NewRepository(store.SQLDB())
	events := []queryaudit.EventInput{
		{WorkspaceID: "sales", PrincipalID: "p1", Surface: "api", Operation: "api_query", QueryKind: "semantic_aggregate", ModelID: "sales", Target: "orders", Status: "success", QueueWaitMS: 7, PlanningMS: 3, ConnectionWaitMS: 5, DatabaseMS: 9, ExecutionMS: 11, ExecutionState: "succeeded", RowsReturned: 10, SQL: "select * from orders", QueryJSON: `{"target":"orders"}`},
		{WorkspaceID: "sales", PrincipalID: "p2", Surface: "data_explorer", Operation: "preview_window", QueryKind: "model_table_rows", ModelID: "sales", Target: "customers", Status: "error", Error: "missing table", QueryJSON: `{"target":"customers"}`},
		{WorkspaceID: "operations", PrincipalID: "p1", Surface: "agent", Operation: "agent_query", QueryKind: "semantic_rows", ModelID: "operations", Target: "reviews", Status: "success", QueryJSON: `{"target":"reviews"}`},
	}
	for _, event := range events {
		if err := repo.RecordQueryEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	sales, err := repo.ListQueryEvents(ctx, queryaudit.Filter{WorkspaceID: "sales"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sales) != 2 {
		t.Fatalf("sales events = %d, want 2", len(sales))
	}

	errors, err := repo.ListQueryEvents(ctx, queryaudit.Filter{Status: "error"})
	if err != nil {
		t.Fatal(err)
	}
	if len(errors) != 1 || errors[0].Target != "customers" {
		t.Fatalf("error events = %#v, want customers", errors)
	}

	search, err := repo.ListQueryEvents(ctx, queryaudit.Filter{Search: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if len(search) != 1 || search[0].Target != "orders" {
		t.Fatalf("search events = %#v, want orders", search)
	}
	if search[0].QueueWaitMS != 7 || search[0].PlanningMS != 3 || search[0].ConnectionWaitMS != 5 || search[0].DatabaseMS != 9 || search[0].ExecutionMS != 11 || search[0].ExecutionState != "succeeded" {
		t.Fatalf("execution telemetry = %#v, want admission/planning/connection/database/execution timings", search[0].EventInput)
	}

	multi, err := repo.ListQueryEvents(ctx, queryaudit.Filter{WorkspaceIDs: []string{"sales", "operations"}, Surfaces: []string{"api", "agent"}, Statuses: []string{"success"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(multi) != 2 {
		t.Fatalf("multi-filter events = %d, want 2: %#v", len(multi), multi)
	}
	for _, event := range multi {
		if event.Status != "success" || (event.Surface != "api" && event.Surface != "agent") {
			t.Fatalf("unexpected multi-filter event: %#v", event)
		}
	}

	options, err := repo.ListQueryEventFilterOptions(ctx, "surface", "data", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].Value != "data_explorer" || options[0].Count != 1 {
		t.Fatalf("surface options = %#v, want data_explorer count", options)
	}
	if _, err := repo.ListQueryEventFilterOptions(ctx, "sql", "", 10); err == nil {
		t.Fatal("expected unsupported filter option field error")
	}
}

func TestRepositoryRedactsSecretsBeforePersistingQueryEvents(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, t.TempDir()+"/libredash.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	repo := NewRepository(store.SQLDB())
	input := queryaudit.EventInput{
		WorkspaceID: "sales",
		PrincipalID: "p1",
		Surface:     "api",
		Operation:   "api_query",
		QueryKind:   "semantic_rows",
		ModelID:     "sales",
		Target:      "orders",
		Status:      "success",
		SQL:         "SELECT * FROM quack_query('postgres://user:secret-pass@example.com/db', 'select 1', token => 'secret-token', PASSWORD 'secret-password')",
		PlanText:    "CREATE SECRET prod (TYPE s3, KEY_ID 'public-ish', SECRET 'super-secret', CLIENT_SECRET 'client-secret')",
		QueryJSON:   `{"target":"orders","options":{"api_key":"secret-api-key","note":"keep-me"}}`,
	}
	if err := repo.RecordQueryEvent(ctx, input); err != nil {
		t.Fatal(err)
	}

	events, err := repo.ListQueryEvents(ctx, queryaudit.Filter{WorkspaceID: "sales"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	stored := events[0].SQL + "\n" + events[0].PlanText + "\n" + events[0].QueryJSON
	for _, leaked := range []string{"secret-pass", "secret-token", "secret-password", "super-secret", "client-secret", "secret-api-key"} {
		if strings.Contains(stored, leaked) {
			t.Fatalf("stored query audit leaked %q:\n%s", leaked, stored)
		}
	}
	for _, want := range []string{"[REDACTED]", "keep-me"} {
		if !strings.Contains(stored, want) {
			t.Fatalf("stored query audit missing %q:\n%s", want, stored)
		}
	}
}

func TestRepositoryRejectsQueryEventWithoutPrincipal(t *testing.T) {
	ctx := context.Background()
	store, err := platform.Open(ctx, t.TempDir()+"/libredash.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	repo := NewRepository(store.SQLDB())
	err = repo.RecordQueryEvent(ctx, queryaudit.EventInput{
		WorkspaceID: "sales",
		Surface:     "api",
		Operation:   "api_query",
		QueryKind:   "semantic_rows",
		ModelID:     "sales",
		Target:      "orders",
		Status:      "success",
		QueryJSON:   `{"target":"orders"}`,
	})
	if err == nil {
		t.Fatal("expected missing principal error")
	}
}
