package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
)

func TestRefreshVisibilityStreamsAndPersistsSemanticModelRuns(t *testing.T) {
	h := newStoreBackedHarness(t)
	ctx := context.Background()
	workspaceID := "sales"
	semanticAssetID := integrationAssetID(t, h.store, workspaceID, "semantic_model", "sales.sales")
	ordersAssetID := integrationAssetID(t, h.store, workspaceID, "model_table", "sales.orders")

	details := h.getAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+semanticAssetID+"/details")
	for _, want := range []string{"Refresh status", "Refresh data", "/updates?section=details"} {
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

	if got := h.postAuthenticated(t, "/workspaces/"+workspaceID+"/assets/"+semanticAssetID+"/refresh"); got != http.StatusNoContent {
		t.Fatalf("semantic model refresh status = %d, want %d", got, http.StatusNoContent)
	}
	requireAssetRefreshStatus(t, semanticStream, materialize.RunStatusRunning)
	requireAssetRefreshStatus(t, semanticStream, materialize.RunStatusSucceeded)
	requireStreamText(t, ordersStream, "Semantic model")

	repo := materialize.NewSQLRunRepository(h.store.SQLDB())
	modelRuns, err := repo.ListTargetRuns(ctx, workspaceID, materialize.TargetSemanticModel, "sales.sales", materialize.RunPage{Limit: 10})
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

	tableRuns, err := repo.ListTargetRuns(ctx, workspaceID, materialize.TargetModelTable, "sales.orders", materialize.RunPage{Limit: 10})
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
