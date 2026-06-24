package integration

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	dashboardruntime "github.com/Yacobolo/libredash/internal/dashboard/runtime"
)

func TestCommandsPublishReloadPatchesToOpenStream(t *testing.T) {
	h := newHarness(t)

	tests := []struct {
		name    string
		path    string
		signals map[string]any
		assert  func(t *testing.T, patches []map[string]any)
	}{
		{
			name: "/commands/select",
			path: "/commands/select",
			signals: mergeSignals(runtimeSignals("cmd-select", "overview"), map[string]any{
				"interactionCommand": pointSelectionCommand("orders", "orders.status", "delivered"),
				"tableCommand":       tableCommand("orders_table", "all", 0, 50, 3, 0),
			}),
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()
				requireStatusLoading(t, patches, true)
				requireSelection(t, patches, "orders", "orders.status", "delivered")
				requireTable(t, patches, "orders_table")
			},
		},
		{
			name: "/commands/clear-selection",
			path: "/commands/clear-selection",
			signals: mergeSignals(runtimeSignals("cmd-clear", "overview"), map[string]any{
				"filters": map[string]any{
					"selections": []map[string]any{selectionSignal("orders", "orders.status", "delivered")},
				},
				"tableCommand": tableCommand("orders_table", "all", 0, 50, 4, 0),
			}),
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()
				requireStatusLoading(t, patches, true)
				requireNoSelection(t, patches)
				requireTable(t, patches, "orders_table")
			},
		},
		{
			name: "/commands/reset-filters",
			path: "/commands/reset-filters",
			signals: mergeSignals(runtimeSignals("cmd-reset", "overview"), map[string]any{
				"filters": map[string]any{
					"controls": map[string]any{
						"state": map[string]any{
							"type":     "multi_select",
							"operator": "in",
							"values":   []string{"SP"},
						},
					},
				},
				"tableCommand": tableCommand("orders_table", "all", 50, 50, 5, 2),
			}),
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()
				requireStatusLoading(t, patches, true)
				requireFilterValues(t, patches, "state")
				requireTableResetVersion(t, patches, "orders_table", 3)
			},
		},
		{
			name: "/commands/refresh-materializations",
			path: "/commands/refresh-materializations",
			signals: mergeSignals(runtimeSignals("cmd-refresh", "overview"), map[string]any{
				"tableCommand": tableCommand("orders_table", "all", 0, 50, 6, 0),
			}),
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()
				requireStatusLoading(t, patches, true)
				requireVisual(t, patches, "total_orders")
				requireTable(t, patches, "orders_table")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := h.openUpdatesStream(t, "executive-sales", "overview", runtimeSignals(clientIDFromSignals(tt.signals), "overview"))
			drainInitialSnapshot(t, stream)

			if got := h.postCommand(t, tt.path, tt.signals); got != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", got, http.StatusNoContent)
			}

			tt.assert(t, nextPatches(t, stream, 3))
		})
	}
}

func TestTableWindowCommandPublishesOnlyRequestedTablePatch(t *testing.T) {
	h := newHarness(t)
	stream := h.openUpdatesStream(t, "executive-sales", "overview", runtimeSignals("cmd-table", "overview"))
	drainInitialSnapshot(t, stream)

	status := h.postCommand(t, "/commands/table-window", mergeSignals(runtimeSignals("cmd-table", "overview"), map[string]any{
		"tableCommand": tableCommand("orders_table", "a", 0, 1, 7, 0),
	}))
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", status, http.StatusNoContent)
	}

	patch := stream.nextPatch(t)
	requireTableBlock(t, []map[string]any{patch}, "orders_table", "a", 0, 7)
	if hasKey(patch, "status") || hasKey(patch, "visuals") || hasKey(patch, "filters") {
		t.Fatalf("table-window command streamed non-table patch: %#v", patch)
	}
	stream.expectNoPatch(t, 150*time.Millisecond)
}

func TestTableWindowCommandDoesNotPublishCanceledTablePatch(t *testing.T) {
	h := newHarness(t, withMetricsWrapper(func(metrics *dashboardruntime.Service) integrationMetrics {
		return canceledTableWindowMetrics{integrationMetrics: metrics}
	}))
	stream := h.openUpdatesStream(t, "executive-sales", "overview", runtimeSignals("cmd-table-canceled", "overview"))
	drainInitialSnapshot(t, stream)

	status := h.postCommand(t, "/commands/table-window", mergeSignals(runtimeSignals("cmd-table-canceled", "overview"), map[string]any{
		"tableCommand": tableCommand("orders_table", "a", 0, 1, 8, 0),
	}))
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", status, http.StatusNoContent)
	}

	stream.expectNoPatch(t, 500*time.Millisecond)
}

func TestRefreshMaterializationsCommandPublishesErrorPatch(t *testing.T) {
	h := newHarness(t, withOlistFixture(func(t *testing.T, dir string) {}))
	stream := h.openUpdatesStream(t, "executive-sales", "overview", runtimeSignals("cmd-refresh-error", "overview"))
	drainInitialSnapshot(t, stream)

	signals := runtimeSignals("cmd-refresh-error", "overview")
	runtime := signals["runtime"].(map[string]any)
	runtime["modelId"] = "olist"
	status := h.postCommand(t, "/commands/refresh-materializations", signals)
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", status, http.StatusNoContent)
	}

	patches := nextPatches(t, stream, 2)
	requireStatusLoading(t, patches, true)
	requireStatusError(t, patches, true)
}

func TestCommandRejectsMalformedDatastarBody(t *testing.T) {
	h := newHarness(t)
	req, err := http.NewRequest(http.MethodPost, h.serverURL(t)+"/commands/select", strings.NewReader("{not-json"))
	if err != nil {
		t.Fatalf("create command request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /commands/select: %v", err)
	}
	defer res.Body.Close()
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(res.Body)

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body:\n%s", res.StatusCode, http.StatusBadRequest, body.String())
	}
}

type canceledTableWindowMetrics struct {
	integrationMetrics
}

func (m canceledTableWindowMetrics) QueryTable(_ context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryTablePage(context.Background(), dashboardID, "", filters, request)
}

func (m canceledTableWindowMetrics) QueryTablePage(_ context.Context, _ string, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return dashboard.EmptyTable(request.WithDefaults(), context.Canceled), nil
}

func drainInitialSnapshot(t *testing.T, stream *streamClient) []map[string]any {
	t.Helper()
	patches := []map[string]any{}
	for len(patches) < 6 {
		patch := stream.nextPatch(t)
		patches = append(patches, patch)
		if _, ok := patch["tables"]; ok {
			return patches
		}
	}
	t.Fatalf("initial stream did not include tables patch: %#v", patches)
	return nil
}

func nextPatches(t *testing.T, stream *streamClient, count int) []map[string]any {
	t.Helper()
	patches := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		patches = append(patches, stream.nextPatch(t))
	}
	return patches
}

func runtimeSignals(clientID, pageID string) map[string]any {
	return map[string]any{
		"runtime": map[string]any{
			"clientId":    clientID,
			"dashboardId": "executive-sales",
			"pageId":      pageID,
		},
	}
}

func clientIDFromSignals(signals map[string]any) string {
	runtime, _ := signals["runtime"].(map[string]any)
	clientID, _ := runtime["clientId"].(string)
	return clientID
}

func pointSelectionCommand(sourceID, field, value string) map[string]any {
	return map[string]any{
		"sourceKind":      "visual",
		"sourceId":        sourceID,
		"interactionKind": "point_selection",
		"action":          "set",
		"toggle":          true,
		"mappings": []map[string]any{{
			"field": field,
			"value": value,
			"label": value,
		}},
	}
}

func selectionSignal(sourceID, field, value string) map[string]any {
	return map[string]any{
		"id":              "visual:" + sourceID + ":point_selection",
		"sourceKind":      "visual",
		"sourceId":        sourceID,
		"interactionKind": "point_selection",
		"entries": []map[string]any{{
			"mappings": []map[string]any{{
				"field": field,
				"value": value,
				"label": value,
			}},
		}},
	}
}

func tableCommand(table, block string, start, count, requestSeq, resetVersion int) map[string]any {
	return map[string]any{
		"table":        table,
		"block":        block,
		"start":        start,
		"count":        count,
		"requestSeq":   requestSeq,
		"resetVersion": resetVersion,
	}
}

func mergeSignals(base map[string]any, extra map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}
