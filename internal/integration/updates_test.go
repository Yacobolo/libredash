package integration

import "testing"

func TestUpdatesStreamsRealRuntimeSignals(t *testing.T) {
	h := newHarness(t)

	tests := []struct {
		name    string
		pageID  string
		signals map[string]any
		assert  func(t *testing.T, patches []map[string]any)
	}{
		{
			name:   "overview filtered to SP",
			pageID: "overview",
			signals: map[string]any{
				"filters": map[string]any{
					"controls": map[string]any{
						"state": map[string]any{
							"type":     "multi_select",
							"operator": "in",
							"values":   []string{"SP"},
						},
						"category": map[string]any{
							"type":     "text",
							"operator": "contains",
							"value":    "ignored",
						},
					},
				},
			},
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()

				requireStatusLoading(t, patches, true)
				requireStatusLoading(t, patches, false)
				requireFilterValues(t, patches, "state", "SP")
				requireVisual(t, patches, "total_orders")
				requireTable(t, patches, "orders_table")
				requireNoFilter(t, patches, "category")
			},
		},
		{
			name:   "chart line page scoped",
			pageID: "chart-line",
			signals: map[string]any{
				"runtime": map[string]any{
					"clientId":    "integration-client",
					"dashboardId": "executive-sales",
					"pageId":      "chart-line",
				},
			},
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()

				requirePatch(t, patches, func(patch map[string]any) bool {
					visuals := mapAt(patch, "visuals")
					return len(visuals) > 0 &&
						hasKey(visuals, "revenue_line") &&
						!hasKey(visuals, "total_orders") &&
						!hasKey(visuals, "orders") &&
						!hasKey(patch, "kpis")
				})
				requireNoTopLevelSignal(t, patches, "kpis")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := h.getUpdatesSignals(t, "executive-sales", tt.pageID, tt.signals)
			tt.assert(t, patches)
		})
	}
}

func requireStatusLoading(t *testing.T, patches []map[string]any, loading bool) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		status := mapAt(patch, "status")
		return status["loading"] == loading
	})
}

func requireFilterValues(t *testing.T, patches []map[string]any, filterID string, want ...string) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		filter := mapAt(patch, "filters", "controls", filterID)
		values, ok := filter["values"].([]any)
		if !ok || len(values) != len(want) {
			return false
		}
		for i := range want {
			if values[i] != want[i] {
				return false
			}
		}
		return true
	})
}

func requireVisual(t *testing.T, patches []map[string]any, visualID string) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		return hasKey(mapAt(patch, "visuals"), visualID)
	})
}

func requireTable(t *testing.T, patches []map[string]any, tableID string) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		return hasKey(mapAt(patch, "tables"), tableID)
	})
}

func requireNoFilter(t *testing.T, patches []map[string]any, filterID string) {
	t.Helper()
	for _, patch := range patches {
		if hasKey(mapAt(patch, "filters", "controls"), filterID) {
			t.Fatalf("patch streamed unexpected filter %q: %#v", filterID, patch)
		}
	}
}

func requireNoTopLevelSignal(t *testing.T, patches []map[string]any, signal string) {
	t.Helper()
	for _, patch := range patches {
		if hasKey(patch, signal) {
			t.Fatalf("patch streamed unexpected top-level signal %q: %#v", signal, patch)
		}
	}
}

func requirePatch(t *testing.T, patches []map[string]any, match func(map[string]any) bool) map[string]any {
	t.Helper()
	for _, patch := range patches {
		if match(patch) {
			return patch
		}
	}
	t.Fatalf("no patch matched predicate; patches: %#v", patches)
	return nil
}

func hasKey(source map[string]any, key string) bool {
	if source == nil {
		return false
	}
	_, ok := source[key]
	return ok
}

func mapAt(source map[string]any, path ...string) map[string]any {
	current := source
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}
