package sqlite

import (
	"context"
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
		{WorkspaceID: "sales", PrincipalID: "p1", Surface: "api", Operation: "api_query", QueryKind: "semantic_aggregate", ModelID: "sales", Target: "orders", Status: "success", RowsReturned: 10, SQL: "select * from orders", QueryJSON: `{"target":"orders"}`},
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
