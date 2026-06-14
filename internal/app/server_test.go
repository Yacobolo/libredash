package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

type fakeMetrics struct{}

func (fakeMetrics) DataDir() string {
	return ".data/olist"
}

func (fakeMetrics) Pages() []dashboard.Page {
	return []dashboard.Page{
		{
			ID:     "overview",
			Title:  "Overview",
			Width:  1366,
			Height: 940,
			Visuals: []dashboard.PageVisual{
				{ID: "header", Kind: "header", X: 0, Y: 0, Width: 100, Height: 40, Title: "Test"},
			},
		},
		{
			ID:     "operations",
			Title:  "Operations",
			Width:  1366,
			Height: 940,
		},
	}
}

func (fakeMetrics) ModelGraph() dashboard.ModelGraph {
	return dashboard.ModelGraph{
		Name:  "test",
		Title: "Test Model",
		Stats: dashboard.ModelStats{Sources: 1, CacheTables: 1, Relationships: 1},
		Nodes: []dashboard.ModelNode{
			{ID: "source:orders", Label: "orders", Kind: "source"},
			{ID: "cache:orders_enriched", Label: "orders_enriched", Kind: "cache"},
		},
		Edges: []dashboard.ModelEdge{
			{ID: "orders_cache", Source: "source:orders", Target: "cache:orders_enriched", Kind: "materialization"},
		},
	}
}

func (fakeMetrics) QueryDashboard(_ context.Context, filters dashboard.Filters) (dashboard.Patch, error) {
	return dashboard.Patch{
		Filters: filters.WithDefaults(),
		Status: dashboard.Status{
			Loading:       false,
			LastUpdated:   "12:00:00",
			DataDirectory: ".data/olist",
		},
		KPIs: []dashboard.KPI{{Label: "Orders", Value: "1", Note: "test", Tone: "ink"}},
		Charts: map[string]dashboard.Chart{
			"orders": {Title: "Orders", Unit: "orders", Data: []dashboard.Point{{Label: "delivered", Value: 1}}},
		},
	}, nil
}

func TestPageRouteRendersRequestedYamlPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/pages/operations", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="page-tab active" href="/pages/operations"`) {
		t.Fatalf("operations page tab was not active:\n%s", body)
	}
}

func TestUnknownPageRouteReturnsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/pages/missing", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestModelRouteRendersSemanticModelGraph(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/model", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<ld-model-graph`) {
		t.Fatalf("body does not render model graph component:\n%s", body)
	}
	if !strings.Contains(body, `Test Model`) {
		t.Fatalf("body does not include model title:\n%s", body)
	}
}

func (fakeMetrics) QueryTable(_ context.Context, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	request = request.WithDefaults()
	return dashboard.Table{
		Title: "Orders",
		Columns: []dashboard.TableColumn{
			{Key: "order_id", Label: "Order"},
		},
		Rows: []map[string]any{
			{"order_id": "o1"},
		},
		TotalRows: 1,
		Window:    dashboard.TableWindow{Offset: request.Offset, Limit: request.Limit},
		Sort:      request.Sort,
	}, nil
}

func (fakeMetrics) RefreshCache(_ context.Context) error {
	return nil
}

func TestUpdatesStreamsDatastarPatchSignals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?datastar=%7B%22filters%22%3A%7B%22state%22%3A%22SP%22%7D%7D", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: datastar-patch-signals") {
		t.Fatalf("body does not contain Datastar patch signal event:\n%s", body)
	}
	if !strings.Contains(body, `"state":"SP"`) {
		t.Fatalf("body does not include decoded filter state:\n%s", body)
	}
}

func TestRefreshCacheCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"dateRange":"2018"},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","offset":0,"limit":25}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/refresh-cache", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestChartSelectCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"dateRange":"2018","visualSelections":[]},"runtime":{"clientId":"test-client"},"chartCommand":{"visualId":"orders","field":"status","value":"delivered","label":"delivered"},"tableCommand":{"table":"orders","offset":0,"limit":25}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/chart-select", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestClearSelectionCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"visualSelections":[{"visualId":"orders","field":"status","values":["delivered"]}]},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","offset":0,"limit":25}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/clear-selection", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestTableWindowCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"state":"SP"},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","offset":10,"limit":25,"sort":{"key":"revenue","direction":"desc"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/table-window", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}
