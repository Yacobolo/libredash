package integration

import (
	"net/http"
	"testing"
	"time"
)

func TestCommandPatchesAreScopedToClientAndPage(t *testing.T) {
	h := newHarness(t)
	target := h.openUpdatesStream(t, "executive-sales", "overview", runtimeSignals("route-target", "overview"))
	otherClient := h.openUpdatesStream(t, "executive-sales", "overview", runtimeSignals("route-other", "overview"))
	drainInitialStreamPatches(t, target)
	drainInitialStreamPatches(t, otherClient)

	status := h.postCommand(t, "/commands/select", mergeSignals(runtimeSignals("route-target", "overview"), map[string]any{
		"interactionCommand":  ordersRowSelectionCommand("delivered"),
		"visualWindowCommand": visualWindowCommand("orders_table", "all", 0, 50, 12, 0),
	}))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}

	requireStatusLoading(t, nextRefreshPatches(t, target), true)
	otherClient.expectNoPatch(t, 150*time.Millisecond)
}

func TestUpdatesQueryParamsTakePrecedenceOverRuntimeSignalIDs(t *testing.T) {
	h := newHarness(t)

	patches := h.getUpdatesSignals(t, "executive-sales", "overview", map[string]any{
		"runtime": map[string]any{
			"clientId":    "route-precedence",
			"dashboardId": "executive-sales",
			"pageId":      "missing",
		},
	})

	requireVisual(t, patches, "total_orders")
	requireTable(t, patches, "orders_table")
	requirePatch(t, patches, func(patch map[string]any) bool {
		visuals := mapAt(patch, "visuals")
		return len(visuals) > 0 && hasKey(visuals, "revenue_by_month")
	})
}

func drainInitialStreamPatches(t *testing.T, stream *streamClient) {
	t.Helper()
	_ = drainInitialSnapshot(t, stream)
}
