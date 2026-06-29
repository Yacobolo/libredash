package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
)

func TestRefreshVisibilityStreamsAndPersistsSemanticModelRuns(t *testing.T) {
	h := newStoreBackedHarness(t)
	ctx := context.Background()
	workspaceID := "libredash"
	semanticAssetID := integrationAssetID(t, h.store, workspaceID, "semantic_model", "olist")
	ordersAssetID := integrationAssetID(t, h.store, workspaceID, "model_table", "olist.orders")

	details := h.getAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+semanticAssetID+"/details")
	for _, want := range []string{"Refresh status", "refresh-materializations", "/updates?section=details"} {
		if !strings.Contains(details, want) {
			t.Fatalf("semantic model details page did not contain %q", want)
		}
	}
	refreshes := h.getAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+semanticAssetID+"/refreshes")
	for _, want := range []string{"Triggered by", "Trigger", "Run ID"} {
		if !strings.Contains(refreshes, want) {
			t.Fatalf("semantic model refreshes page did not contain %q", want)
		}
	}

	semanticStream := h.openAssetUpdatesStream(t, workspaceID, semanticAssetID, "refreshes")
	ordersStream := h.openAssetUpdatesStream(t, workspaceID, ordersAssetID, "refreshes")
	_ = semanticStream.nextPatch(t)
	_ = ordersStream.nextPatch(t)

	if got := h.postAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+semanticAssetID+"/refresh-materializations"); got != http.StatusNoContent {
		t.Fatalf("semantic model refresh status = %d, want %d", got, http.StatusNoContent)
	}
	requireAssetRefreshStatus(t, semanticStream, materialize.RunStatusRunning)
	requireAssetRefreshStatus(t, semanticStream, materialize.RunStatusSucceeded)
	requireStreamText(t, ordersStream, "Semantic model")

	repo := materialize.NewSQLRunRepository(h.store.SQLDB())
	modelRuns, err := repo.ListTargetRuns(ctx, workspaceID, materialize.TargetSemanticModel, "olist", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list semantic model runs: %v", err)
	}
	if len(modelRuns) != 1 {
		t.Fatalf("semantic model runs = %#v, want one parent run", modelRuns)
	}
	parent := modelRuns[0]
	if parent.Status != materialize.RunStatusSucceeded || parent.TriggerType != materialize.TriggerDirect || parent.PrincipalID != "dev" {
		t.Fatalf("semantic model run = %#v, want succeeded direct run attributed to dev", parent)
	}

	tableRuns, err := repo.ListTargetRuns(ctx, workspaceID, materialize.TargetModelTable, "olist.orders", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list orders table runs: %v", err)
	}
	if len(tableRuns) != 1 {
		t.Fatalf("orders table runs = %#v, want one child run", tableRuns)
	}
	child := tableRuns[0]
	if child.Status != materialize.RunStatusSucceeded || child.TriggerType != materialize.TriggerSemanticModel || child.ParentRunID != parent.ID || child.PrincipalID != "dev" {
		t.Fatalf("orders table run = %#v, want semantic model child run attributed to dev", child)
	}

	semanticRefreshes := h.getAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+semanticAssetID+"/refreshes")
	for _, want := range []string{"Local Developer", "Direct", shortRunID(parent.ID)} {
		if !strings.Contains(semanticRefreshes, want) {
			t.Fatalf("semantic model refreshes page did not contain %q after refresh", want)
		}
	}
	ordersRefreshes := h.getAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+ordersAssetID+"/refreshes")
	for _, want := range []string{"Local Developer", "Semantic model", shortRunID(child.ID)} {
		if !strings.Contains(ordersRefreshes, want) {
			t.Fatalf("orders refreshes page did not contain %q after refresh", want)
		}
	}
}

func TestRefreshVisibilityRecordsDirectModelTableDependencyRuns(t *testing.T) {
	catalogPath := writeDependentRefreshCatalog(t)
	h := newStoreBackedHarness(t,
		withCatalog(catalogPath),
		withOlistFixture(writeDependentRefreshFixture),
	)
	ctx := context.Background()
	workspaceID := "dep-refresh"
	summaryAssetID := integrationAssetID(t, h.store, workspaceID, "model_table", "olist.order_summary")
	ordersAssetID := integrationAssetID(t, h.store, workspaceID, "model_table", "olist.orders")

	details := h.getAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+summaryAssetID+"/details")
	for _, want := range []string{"Refresh status", "refresh-materializations", "/updates?section=details"} {
		if !strings.Contains(details, want) {
			t.Fatalf("model table details page did not contain %q", want)
		}
	}
	refreshes := h.getAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+summaryAssetID+"/refreshes")
	for _, want := range []string{"Triggered by", "Trigger", "Run ID"} {
		if !strings.Contains(refreshes, want) {
			t.Fatalf("model table refreshes page did not contain %q", want)
		}
	}

	summaryStream := h.openAssetUpdatesStream(t, workspaceID, summaryAssetID, "refreshes")
	_ = summaryStream.nextPatch(t)

	if got := h.postAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+summaryAssetID+"/refresh-materializations"); got != http.StatusNoContent {
		t.Fatalf("model table refresh status = %d, want %d", got, http.StatusNoContent)
	}
	requireStreamPatch(t, summaryStream, func(patch map[string]any) bool {
		refresh := mapAt(mapAt(patch, "page"), "refresh")
		return stringValue(refresh["status"]) == materialize.RunStatusSucceeded && strings.Contains(patchJSON(t, patch), "Direct")
	})

	repo := materialize.NewSQLRunRepository(h.store.SQLDB())
	selectedRuns, err := repo.ListTargetRuns(ctx, workspaceID, materialize.TargetModelTable, "olist.order_summary", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list selected table runs: %v", err)
	}
	if len(selectedRuns) != 1 {
		t.Fatalf("selected table runs = %#v, want one direct run", selectedRuns)
	}
	selected := selectedRuns[0]
	if selected.Status != materialize.RunStatusSucceeded || selected.TriggerType != materialize.TriggerDirect || selected.ParentRunID != "" || selected.PrincipalID != "dev" {
		t.Fatalf("selected table run = %#v, want succeeded direct root run attributed to dev", selected)
	}
	dependencyRuns, err := repo.ListTargetRuns(ctx, workspaceID, materialize.TargetModelTable, "olist.orders", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list dependency table runs: %v", err)
	}
	if len(dependencyRuns) != 1 {
		t.Fatalf("dependency table runs = %#v, want one dependency run", dependencyRuns)
	}
	dependency := dependencyRuns[0]
	if dependency.Status != materialize.RunStatusSucceeded || dependency.TriggerType != materialize.TriggerDependency || dependency.ParentRunID != selected.ID || dependency.PrincipalID != "dev" {
		t.Fatalf("dependency table run = %#v, want dependency run attributed to dev and linked to selected run", dependency)
	}

	summaryRefreshes := h.getAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+summaryAssetID+"/refreshes")
	for _, want := range []string{"Local Developer", "Direct", shortRunID(selected.ID)} {
		if !strings.Contains(summaryRefreshes, want) {
			t.Fatalf("summary refreshes page did not contain %q after refresh", want)
		}
	}
	ordersRefreshes := h.getAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+ordersAssetID+"/refreshes")
	for _, want := range []string{"Local Developer", "Dependency", shortRunID(dependency.ID)} {
		if !strings.Contains(ordersRefreshes, want) {
			t.Fatalf("orders refreshes page did not contain %q after dependency refresh", want)
		}
	}
}

func requireAssetRefreshStatus(t *testing.T, stream *streamClient, status string) map[string]any {
	t.Helper()
	return requireStreamPatch(t, stream, func(patch map[string]any) bool {
		refresh := mapAt(mapAt(patch, "page"), "refresh")
		return stringValue(refresh["status"]) == status
	})
}

func requireStreamText(t *testing.T, stream *streamClient, text string) map[string]any {
	t.Helper()
	return requireStreamPatch(t, stream, func(patch map[string]any) bool {
		return strings.Contains(patchJSON(t, patch), text)
	})
}

func requireStreamPatch(t *testing.T, stream *streamClient, match func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case patch, ok := <-stream.patches:
			if !ok {
				t.Fatal("asset updates stream closed before matching patch")
			}
			if match(patch) {
				return patch
			}
		case err := <-stream.errs:
			if err != nil {
				t.Fatalf("read asset updates stream: %v", err)
			}
		case <-deadline:
			t.Fatal("timed out waiting for matching asset updates patch")
		}
	}
}

func patchJSON(t *testing.T, patch map[string]any) string {
	t.Helper()
	out, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}
	return string(out)
}

func shortRunID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func writeDependentRefreshCatalog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFixture(t, dir, "catalog.yaml", `workspace:
  id: dep-refresh
  title: Dependent Refresh Workspace
semantic_models:
- id: olist
  title: Dependent Olist
  path: model.yaml
dashboards:
- id: dependent-dashboard
  title: Dependent Dashboard
  path: dashboard.yaml
`)
	writeFixture(t, dir, "model.yaml", `name: olist
title: Dependent Olist
description: Small model used by refresh visibility integration tests.
default_connection: local
connections:
  local:
    kind: local
    defaults:
      options:
        header: true
sources:
  orders:
    connection: local
    path: orders.csv
    fields:
      order_id: {}
      status: {}
      revenue: {}
models:
  orders:
    source: orders
    primary_key: order_id
    fields:
      order_id:
        label: Order ID
      status:
        label: Status
      revenue:
        label: Revenue
  order_summary:
    primary_key: status
    transform:
      sql: |
        SELECT status, COUNT(*) AS order_count
        FROM model.orders
        GROUP BY status
    fields:
      status:
        label: Status
      order_count:
        label: Orders
semantic_models:
  olist:
    base_table: order_summary
    tables:
      - orders
      - order_summary
    measures:
      defaults:
        table: order_summary
        grain: status
      order_count:
        label: Orders
        expression: SUM(order_summary.order_count)
        format: integer
`)
	writeFixture(t, dir, "dashboard.yaml", `id: dependent-dashboard
title: Dependent Dashboard
description: Small dashboard used by refresh visibility integration tests.
semantic_model: olist
visuals:
  total_orders:
    kind: kpi
    shape: single_value
    query:
      measures:
        order_count:
pages:
- id: overview
  title: Overview
  canvas:
    width: 800
    height: 400
  grid:
    columns: 12
    row_height: 48
    gap: 16
    padding: 16
  visuals:
  - id: total-orders
    kind: kpi_card
    visual: total_orders
    placement:
      col: 1
      row: 1
      col_span: 3
      row_span: 2
`)
	return filepath.Join(dir, "catalog.yaml")
}

func writeDependentRefreshFixture(t *testing.T, dir string) {
	t.Helper()
	writeFixture(t, dir, "orders.csv", `order_id,status,revenue
o1,delivered,10
o2,shipped,20
`)
}
