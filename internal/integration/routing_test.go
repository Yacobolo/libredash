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
	otherPage := h.openUpdatesStream(t, "executive-sales", "chart-line", runtimeSignals("route-target", "chart-line"))
	drainInitialSnapshot(t, target)
	drainInitialSnapshot(t, otherClient)
	drainInitialSnapshot(t, otherPage)

	status := h.postCommand(t, "/commands/select", mergeSignals(runtimeSignals("route-target", "overview"), map[string]any{
		"interactionCommand": pointSelectionCommand("orders", "orders.status", "delivered"),
		"tableCommand":       tableCommand("orders_table", "all", 0, 50, 12, 0),
	}))
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", status, http.StatusNoContent)
	}

	requireStatusLoading(t, []map[string]any{target.nextPatch(t)}, true)
	otherClient.expectNoPatch(t, 150*time.Millisecond)
	otherPage.expectNoPatch(t, 150*time.Millisecond)
}

func TestUpdatesQueryParamsTakePrecedenceOverRuntimeSignalIDs(t *testing.T) {
	h := newHarness(t)

	patches := h.getUpdatesSignals(t, "executive-sales", "overview", map[string]any{
		"runtime": map[string]any{
			"clientId":    "route-precedence",
			"dashboardId": "executive-sales",
			"pageId":      "chart-line",
		},
	})

	requireVisual(t, patches, "total_orders")
	requireTable(t, patches, "orders_table")
	requirePatch(t, patches, func(patch map[string]any) bool {
		visuals := mapAt(patch, "visuals")
		return len(visuals) > 0 && !hasKey(visuals, "revenue_line")
	})
}
