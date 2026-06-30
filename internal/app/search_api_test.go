package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/deployment"
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

func TestWorkspaceSearchUsesRouteWorkspaceRuntimeCatalog(t *testing.T) {
	server := NewWithOptions(NewMultiWorkspaceMetrics("sales", map[string]QueryMetrics{
		"sales":      workspaceSearchMetrics{workspaceID: "sales", dashboardID: "executive-sales", title: "Executive Sales Dashboard"},
		"operations": workspaceSearchMetrics{workspaceID: "operations", dashboardID: "fulfillment-operations", title: "Fulfillment Operations"},
	}), Options{Store: testStore(t), DefaultWorkspaceID: "sales"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/operations/search?types=dashboard&q=fulfillment&limit=20", nil)
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
	if len(response.Items) == 0 {
		t.Fatalf("expected operations dashboard search result")
	}
	if got := response.Items[0]["dashboardId"]; got != "fulfillment-operations" {
		t.Fatalf("dashboardId=%#v, want fulfillment-operations; items=%#v", got, response.Items)
	}
	for _, item := range response.Items {
		if item["dashboardId"] == "executive-sales" {
			t.Fatalf("operations search leaked sales dashboard: %#v", item)
		}
	}
}

func TestWorkspaceSearchDoesNotLeakDefaultWorkspaceRuntimeDocuments(t *testing.T) {
	server := NewWithOptions(NewMultiWorkspaceMetrics("sales", map[string]QueryMetrics{
		"sales":      workspaceSearchMetrics{workspaceID: "sales", dashboardID: "executive-sales", title: "Executive Sales Dashboard"},
		"operations": workspaceSearchMetrics{workspaceID: "operations", dashboardID: "fulfillment-operations", title: "Fulfillment Operations"},
	}), Options{Store: testStore(t), DefaultWorkspaceID: "sales"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/operations/search?types=dashboard,visual,table&limit=20", nil)
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
	seenTypes := map[string]bool{}
	for _, item := range response.Items {
		seenTypes[item["type"].(string)] = true
		if item["dashboardId"] == "executive-sales" || strings.Contains(item["id"].(string), "executive-sales") {
			t.Fatalf("operations search leaked sales runtime document: %#v", item)
		}
	}
	for _, typ := range []string{"dashboard", "visual", "table"} {
		if !seenTypes[typ] {
			t.Fatalf("missing %s search result in %#v", typ, response.Items)
		}
	}
}

func TestWorkspaceSearchDefaultWorkspaceStillReturnsRichRuntimeResults(t *testing.T) {
	server := NewWithOptions(NewMultiWorkspaceMetrics("sales", map[string]QueryMetrics{
		"sales":      workspaceSearchMetrics{workspaceID: "sales", dashboardID: "executive-sales", title: "Executive Sales Dashboard"},
		"operations": workspaceSearchMetrics{workspaceID: "operations", dashboardID: "fulfillment-operations", title: "Fulfillment Operations"},
	}), Options{Store: testStore(t), DefaultWorkspaceID: "sales"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/sales/search?types=dashboard,visual,table&limit=20", nil)
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
	seen := map[string]bool{}
	for _, item := range response.Items {
		seen[item["type"].(string)+":"+item["dashboardId"].(string)] = true
	}
	for _, want := range []string{"dashboard:executive-sales", "visual:executive-sales", "table:executive-sales"} {
		if !seen[want] {
			t.Fatalf("missing %s in %#v", want, response.Items)
		}
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

func TestWorkspaceSearchUsesAssetGraphFallbackWithoutRuntimeDocs(t *testing.T) {
	store := testStore(t)
	seedEnvironmentAssetDeployment(t, store, "asset-only", deployment.DefaultEnvironment, "Graph Only Dashboard", "Graph Only Connection")
	server := NewWithOptions(NewMultiWorkspaceMetrics("sales", map[string]QueryMetrics{
		"sales": workspaceSearchMetrics{workspaceID: "sales", dashboardID: "executive-sales", title: "Executive Sales Dashboard"},
	}), Options{Store: store, DefaultWorkspaceID: "sales"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/asset-only/search?types=asset&q=Graph%20Only&limit=20", nil)
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
	found := false
	for _, item := range response.Items {
		if item["dashboardId"] == "executive-sales" {
			t.Fatalf("asset-only fallback leaked default runtime document: %#v", item)
		}
		if item["type"] == "asset" && item["name"] == "Graph Only Dashboard" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing graph-backed asset search result in %#v", response.Items)
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

type workspaceSearchMetrics struct {
	fakeMetrics
	workspaceID string
	dashboardID string
	title       string
}

func (m workspaceSearchMetrics) Catalog() dashboard.Catalog {
	return dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{ID: m.workspaceID, Title: m.workspaceID},
		Models: []dashboard.CatalogModel{{
			ID:          "test",
			Title:       "Test Model",
			Description: "Fixture model",
		}},
		Dashboards: []dashboard.CatalogDashboard{{
			ID:            m.dashboardID,
			Title:         m.title,
			Description:   "Fixture report",
			SemanticModel: "test",
			PageCount:     2,
		}},
	}
}

func (m workspaceSearchMetrics) DefaultDashboardID() string {
	return m.dashboardID
}

func (m workspaceSearchMetrics) ModelIDForDashboard(dashboardID string) string {
	if dashboardID == m.dashboardID {
		return "test"
	}
	return ""
}

func (m workspaceSearchMetrics) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	if dashboardID != m.dashboardID {
		return reportdef.Dashboard{}, nil, false
	}
	report, model, ok := fakeMetrics{}.Report("executive-sales")
	if !ok {
		return reportdef.Dashboard{}, nil, false
	}
	report.ID = m.dashboardID
	report.Title = m.title
	return report, model, true
}

func (m workspaceSearchMetrics) Pages(dashboardID string) []dashboard.Page {
	if dashboardID != m.dashboardID {
		return nil
	}
	pages := fakeMetrics{}.Pages("executive-sales")
	return append([]dashboard.Page(nil), pages...)
}
