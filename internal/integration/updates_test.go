package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/testutil/ssetest"
)

func TestUpdatesStreamsRealRuntimeSignals(t *testing.T) {
	h := newHarness(t)

	tests := []struct {
		name    string
		pageID  string
		signals map[string]any
		query   url.Values
		assert  func(t *testing.T, patches []map[string]any)
	}{
		{
			name:    "overview filtered to SP",
			pageID:  "overview",
			signals: map[string]any{},
			query: url.Values{
				"state":    []string{"SP"},
				"category": []string{"ignored"},
			},
			assert: func(t *testing.T, patches []map[string]any) {
				t.Helper()

				requireFirstStatusLoading(t, patches)
				requireStatusLoading(t, patches, true)
				requireStatusLoading(t, patches, false)
				requireFilterValues(t, patches, "state", "SP")
				requireFilterOptions(t, patches, "state")
				requireVisual(t, patches, "total_orders")
				requireTable(t, patches, "orders_table")
				requireNoTopLevelSignal(t, patches, "kpis")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := h.getUpdatesSignalsWithQuery(t, "executive-sales", tt.pageID, tt.signals, tt.query)
			tt.assert(t, patches)
		})
	}
}

func TestUpdatesStreamsSetupRequiredPatchForMissingData(t *testing.T) {
	h := newHarness(t, withOlistFixture(func(t *testing.T, dir string) {}))

	patches := h.getUpdatesSignals(t, "executive-sales", "overview", map[string]any{})

	requireFirstStatusLoading(t, patches)
	requirePatch(t, patches, func(patch map[string]any) bool {
		status := mapAt(patch, "status")
		return status["setupRequired"] == true && strings.TrimSpace(stringValue(status["error"])) != ""
	})
}

func TestUpdatesIgnoresMalformedDatastarSignals(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, h.workspaceUpdatesPath()+"?route=dashboard&workspace="+h.workspaceIDOrDefault()+"&dashboard=executive-sales&page=overview&datastar=%7Bnot-json", nil)
	rec := httptest.NewRecorder()

	h.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body:\n%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}
	requireVisual(t, ssetest.PatchSignals(t, rec.Body.String()), "total_orders")
}

func requireFirstStatusLoading(t *testing.T, patches []map[string]any) {
	t.Helper()
	if len(patches) == 0 {
		t.Fatal("no patches streamed")
	}
	status := mapAt(patches[0], "status")
	if status["loading"] != true {
		t.Fatalf("first patch status = %#v, want loading=true; patch=%#v", status, patches[0])
	}
}

func requireStatusLoading(t *testing.T, patches []map[string]any, loading bool) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		status := mapAt(patch, "status")
		return status["loading"] == loading
	})
}

func requireStatusError(t *testing.T, patches []map[string]any, setupRequired bool) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		status := mapAt(patch, "status")
		return stringValue(status["error"]) != "" && status["setupRequired"] == setupRequired
	})
}

func requireFilterValues(t *testing.T, patches []map[string]any, filterID string, want ...string) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		filter := mapAt(patch, "filters", "controls", filterID)
		values, ok := filter["values"].([]any)
		if len(want) == 0 && !ok {
			return true
		}
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

func requireFilterOptions(t *testing.T, patches []map[string]any, filterID string) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		options, ok := patch["filterOptions"].(map[string]any)
		if !ok {
			return false
		}
		values, ok := options[filterID].([]any)
		return ok && len(values) > 0
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
		visual := mapAt(patch, "visuals", tableID)
		return visual["type"] == "table" || visual["type"] == "matrix" || visual["type"] == "pivot"
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

func requireNoSelection(t *testing.T, patches []map[string]any) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		selections, ok := mapAt(patch, "filters")["selections"].([]any)
		return ok && len(selections) == 0
	})
}

func requireSelection(t *testing.T, patches []map[string]any, sourceID, field, value string) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		selections, ok := mapAt(patch, "filters")["selections"].([]any)
		if !ok {
			return false
		}
		for _, rawSelection := range selections {
			selection, ok := rawSelection.(map[string]any)
			if !ok || selection["sourceId"] != sourceID {
				continue
			}
			entries, ok := selection["entries"].([]any)
			if !ok {
				continue
			}
			for _, rawEntry := range entries {
				entry, ok := rawEntry.(map[string]any)
				if !ok {
					continue
				}
				mappings, ok := entry["mappings"].([]any)
				if !ok {
					continue
				}
				for _, rawMapping := range mappings {
					mapping, ok := rawMapping.(map[string]any)
					if ok && mapping["field"] == field && mapping["value"] == value {
						return true
					}
				}
			}
		}
		return false
	})
}

func requireTableBlock(t *testing.T, patches []map[string]any, tableID, blockID string, start, requestSeq int) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		block := tableBlock(patch, tableID, blockID)
		return numberValue(block["start"]) == float64(start) && numberValue(block["requestSeq"]) == float64(requestSeq)
	})
}

func requireTableResetVersion(t *testing.T, patches []map[string]any, tableID string, resetVersion int) {
	t.Helper()
	requirePatch(t, patches, func(patch map[string]any) bool {
		table := mapAt(patch, "visuals", tableID)
		return numberValue(table["resetVersion"]) == float64(resetVersion)
	})
}

func requireNoTopLevelSignal(t *testing.T, patches []map[string]any, signal string) {
	t.Helper()
	for _, patch := range patches {
		if hasKey(patch, signal) {
			t.Fatalf("patch streamed unexpected top-level signal %q: %#v", signal, patch)
		}
	}
}

func tableBlock(patch map[string]any, tableID, blockID string) map[string]any {
	return mapAt(patch, "visuals", tableID, "blocks", blockID)
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

func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func numberValue(value any) float64 {
	number, _ := value.(float64)
	return number
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
