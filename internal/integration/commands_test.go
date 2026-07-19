package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/consumer"
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
				"interactionCommand":  ordersRowSelectionCommand("delivered"),
				"visualWindowCommand": visualWindowCommand("orders_table", "all", 0, 50, 3, 0),
			}),
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()
				requireStatusLoading(t, patches, true)
				requireSelection(t, patches, "orders_table", "orders.status", "delivered")
				requireVisual(t, patches, "category_revenue")
			},
		},
		{
			name: "/commands/clear-selection",
			path: "/commands/clear-selection",
			signals: mergeSignals(runtimeSignals("cmd-clear", "overview"), map[string]any{
				"filters": map[string]any{
					"selections": []map[string]any{selectionSignal("orders_table", "orders.status", "delivered")},
				},
				"visualWindowCommand": visualWindowCommand("orders_table", "all", 0, 50, 4, 0),
			}),
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()
				requireStatusLoading(t, patches, true)
				requireNoSelection(t, patches)
				requireVisual(t, patches, "category_revenue")
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
				"visualWindowCommand": visualWindowCommand("orders_table", "all", 50, 50, 5, 2),
			}),
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()
				requireStatusLoading(t, patches, true)
				requireFilterValues(t, patches, "state")
				requireTableResetVersion(t, patches, "orders_table", 3)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := h.openUpdatesStream(t, "executive-sales", "overview", runtimeSignals(clientIDFromSignals(tt.signals), "overview"))
			drainInitialSnapshot(t, stream)
			if tt.path == "/commands/clear-selection" {
				primeSignals := mergeSignals(runtimeSignals(clientIDFromSignals(tt.signals), "overview"), map[string]any{
					"interactionCommand": ordersRowSelectionCommand("delivered"),
				})
				if got := h.postCommand(t, "/commands/select", primeSignals); got != http.StatusOK {
					t.Fatalf("prime selection status = %d, want %d", got, http.StatusOK)
				}
				_ = nextRefreshPatches(t, stream)
			}

			if got := h.postCommand(t, tt.path, tt.signals); got != http.StatusOK {
				t.Fatalf("status = %d, want %d", got, http.StatusOK)
			}

			tt.assert(t, nextRefreshPatches(t, stream))
		})
	}
}

func TestVisualWindowCommandPublishesOnlyRequestedVisualPatch(t *testing.T) {
	h := newHarness(t)
	stream := h.openUpdatesStream(t, "executive-sales", "overview", runtimeSignals("cmd-table", "overview"))
	drainInitialSnapshot(t, stream)

	status := h.postCommand(t, "/commands/visual-window", mergeSignals(runtimeSignals("cmd-table", "overview"), map[string]any{
		"visualWindowCommand": visualWindowCommand("orders_table", "a", 0, 1, 7, 0),
	}))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}

	patches := nextRefreshPatches(t, stream)
	requireTableBlock(t, patches, "orders_table", "a", 0, 7)
	for _, patch := range patches {
		if hasKey(patch, "filterOptions") {
			t.Fatalf("visual-window command streamed non-target data: %#v", patch)
		}
		for visualID := range mapAt(patch, "visuals") {
			if visualID != "orders_table" {
				t.Fatalf("visual-window command streamed non-target visual %q: %#v", visualID, patch)
			}
		}
	}
	stream.expectNoPatch(t, 150*time.Millisecond)
}

func TestVisualWindowCommandDoesNotPublishCanceledVisualPatch(t *testing.T) {
	h := newHarness(t, withMetricsWrapper(func(metrics *dashboardruntime.Service) integrationMetrics {
		return canceledVisualWindowMetrics{integrationMetrics: metrics}
	}))
	stream := h.openUpdatesStream(t, "executive-sales", "overview", runtimeSignals("cmd-table-canceled", "overview"))
	drainInitialSnapshot(t, stream)

	status := h.postCommand(t, "/commands/visual-window", mergeSignals(runtimeSignals("cmd-table-canceled", "overview"), map[string]any{
		"visualWindowCommand": visualWindowCommand("orders_table", "a", 0, 1, 8, 0),
	}))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}

	patches := nextRefreshPatches(t, stream)
	for _, patch := range patches {
		if hasKey(mapAt(patch, "visuals"), "orders_table") {
			t.Fatalf("canceled visual-window command streamed visual data: %#v", patch)
		}
	}
	requireStatusLoading(t, patches, false)
}

func TestRefreshMaterializationsCommandIsRemoved(t *testing.T) {
	h := newHarness(t, withOlistFixture(func(t *testing.T, dir string) {}))
	signals := runtimeSignals("cmd-refresh-error", "overview")
	runtime := signals["runtime"].(map[string]any)
	runtime["modelId"] = "olist"
	encodedSignals, err := json.Marshal(signals)
	if err != nil {
		t.Fatalf("marshal Datastar signals: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, h.serverURL(t)+h.workspaceCommandPath("/commands/refresh-materializations"), bytes.NewReader(encodedSignals))
	if err != nil {
		t.Fatalf("create removed command request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST removed command: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusNotFound)
	}
}

func TestCommandRejectsMalformedDatastarBody(t *testing.T) {
	h := newHarness(t)
	req, err := http.NewRequest(http.MethodPost, h.serverURL(t)+h.workspaceCommandPath("/commands/select"), strings.NewReader("{not-json"))
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

type canceledVisualWindowMetrics struct {
	integrationMetrics
}

func (m canceledVisualWindowMetrics) ExecuteConsumersPage(ctx context.Context, request consumer.Request, publish consumer.Publisher) error {
	if request.Command != "visual_window" {
		return m.integrationMetrics.ExecuteConsumersPage(ctx, request, publish)
	}
	for _, target := range request.Targets {
		publish(consumer.Result{Target: target, Err: context.Canceled})
	}
	return nil
}

func (m canceledVisualWindowMetrics) QueryTable(_ context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryTablePage(context.Background(), dashboardID, "", filters, request)
}

func (m canceledVisualWindowMetrics) QueryTablePage(_ context.Context, _ string, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return dashboard.EmptyTable(request.WithDefaults(), context.Canceled), nil
}

func drainInitialSnapshot(t *testing.T, stream *streamClient) []map[string]any {
	t.Helper()
	patches := []map[string]any{}
	quiet := time.NewTimer(150 * time.Millisecond)
	defer quiet.Stop()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	seenSnapshotTable := false
	for {
		select {
		case patch, ok := <-stream.patches:
			if !ok {
				return patches
			}
			patches = append(patches, patch)
			if tableHasSnapshot(patch, "orders_table") {
				seenSnapshotTable = true
			}
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(150 * time.Millisecond)
		case err := <-stream.errs:
			if err != nil {
				t.Fatalf("read initial updates stream: %v", err)
			}
			return patches
		case <-quiet.C:
			if seenSnapshotTable {
				return patches
			}
			quiet.Reset(150 * time.Millisecond)
		case <-deadline.C:
			t.Fatalf("initial stream did not include populated tables patch: %#v", patches)
		}
	}
}

func tableHasSnapshot(patch map[string]any, tableID string) bool {
	table := mapAt(patch, "visuals", tableID)
	if _, ok := table["availableRows"]; !ok {
		return false
	}
	return hasKey(table, "blocks")
}

func nextRefreshPatches(t *testing.T, stream *streamClient) []map[string]any {
	t.Helper()
	patches := []map[string]any{}
	var generation float64
	for {
		patch := stream.nextPatch(t)
		patches = append(patches, patch)
		status := mapAt(patch, "status")
		loading, hasLoading := status["loading"].(bool)
		currentGeneration, _ := status["generation"].(float64)
		if hasLoading && loading && currentGeneration > 0 && generation == 0 {
			generation = currentGeneration
		}
		if hasLoading && !loading && generation > 0 && currentGeneration == generation {
			return patches
		}
	}
}

func runtimeSignals(clientID, pageID string) map[string]any {
	return map[string]any{
		"runtime": map[string]any{
			"clientId":         clientID,
			"dashboardId":      "executive-sales",
			"pageId":           pageID,
			"streamInstanceId": clientID + "-stream",
		},
	}
}

func clientIDFromSignals(signals map[string]any) string {
	runtime, _ := signals["runtime"].(map[string]any)
	clientID, _ := runtime["clientId"].(string)
	return clientID
}

func streamInstanceIDFromSignals(signals map[string]any) string {
	runtime, _ := signals["runtime"].(map[string]any)
	streamInstanceID, _ := runtime["streamInstanceId"].(string)
	return streamInstanceID
}

func ordersRowSelectionCommand(status string) map[string]any {
	return map[string]any{
		"sourceKind":      "visual",
		"sourceId":        "orders_table",
		"interactionKind": "row_selection",
		"action":          "set",
		"toggle":          true,
		"mappings": []map[string]any{
			{
				"field": "orders.order_id",
				"fact":  "orders",
				"value": "fixture-order-id",
				"label": "fixture-order-id",
			},
			{
				"field": "orders.status",
				"fact":  "orders",
				"value": status,
				"label": status,
			},
			{
				"field": "orders.category",
				"fact":  "orders",
				"value": "fixture-category",
				"label": "fixture-category",
			},
		},
	}
}

func selectionSignal(sourceID, field, value string) map[string]any {
	return map[string]any{
		"id":              "visual:" + sourceID + ":row_selection",
		"sourceKind":      "visual",
		"sourceId":        sourceID,
		"interactionKind": "row_selection",
		"entries": []map[string]any{{
			"mappings": []map[string]any{{
				"field": field,
				"fact":  "orders",
				"value": value,
				"label": value,
			}},
		}},
	}
}

func visualWindowCommand(visual, block string, start, count, requestSeq, resetVersion int) map[string]any {
	return map[string]any{
		"visual":       visual,
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
