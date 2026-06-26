package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkspaceSearchReturnsProgressiveDiscoveryResults(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/search?q=orders&limit=20", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Items []map[string]any `json:"items"`
		Page  struct {
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	seen := map[string]map[string]any{}
	for _, item := range response.Items {
		for _, key := range []string{"id", "type", "name", "description"} {
			if _, ok := item[key]; !ok {
				t.Fatalf("search item missing %s: %#v", key, item)
			}
		}
		seen[item["type"].(string)+":"+item["id"].(string)] = item
		for _, forbidden := range []string{"items", "data", "query", "columns", "meta"} {
			if _, ok := item[forbidden]; ok {
				t.Fatalf("search item leaked detailed field %q: %#v", forbidden, item)
			}
		}
	}
	for _, want := range []string{
		"dashboard:executive-sales",
		"visual:visual:executive-sales.overview.orders",
		"table:table:executive-sales.overview.orders",
		"dataset:test.orders",
		"field:test.orders.order_id",
		"measure:test.order_count",
	} {
		if _, ok := seen[want]; !ok {
			t.Fatalf("search results missing %s in %#v", want, response.Items)
		}
	}
	if got := seen["visual:visual:executive-sales.overview.orders"]["dashboardId"]; got != "executive-sales" {
		t.Fatalf("visual dashboardId=%#v", got)
	}
}

func TestWorkspaceSearchDoesNotFallbackToRuntimeCatalogForOtherWorkspaces(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/other/search?q=orders&limit=20", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	for _, item := range response.Items {
		if item["dashboardId"] == "executive-sales" || item["modelId"] == "test" {
			t.Fatalf("search leaked runtime catalog item for another workspace: %#v", item)
		}
	}
}

func TestWorkspaceSearchTypeFilteringPaginationAndErrors(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})

	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/search?types=visual,table&limit=1", nil)
	firstReq.Header.Set("Accept", "application/json")
	firstRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", firstRec.Code, firstRec.Body.String())
	}
	var first struct {
		Items []map[string]any `json:"items"`
		Page  struct {
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first: %v body=%s", err, firstRec.Body.String())
	}
	if len(first.Items) != 1 || first.Page.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	if typ := first.Items[0]["type"]; typ != "visual" && typ != "table" {
		t.Fatalf("unexpected filtered type %#v in %#v", typ, first.Items[0])
	}

	nextReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/search?types=visual,table&limit=1&pageToken="+first.Page.NextCursor, nil)
	nextReq.Header.Set("Accept", "application/json")
	nextRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(nextRec, nextReq)
	if nextRec.Code != http.StatusOK {
		t.Fatalf("next status=%d body=%s", nextRec.Code, nextRec.Body.String())
	}
	if strings.Contains(nextRec.Body.String(), first.Items[0]["id"].(string)) {
		t.Fatalf("next page repeated first item: first=%#v next=%s", first.Items[0], nextRec.Body.String())
	}

	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/search?types=dashboard,unknown", nil)
	badReq.Header.Set("Accept", "application/json")
	badRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("bad status=%d body=%s", badRec.Code, badRec.Body.String())
	}
	assertAPIError(t, badRec, http.StatusBadRequest, "unknown search type")
}
