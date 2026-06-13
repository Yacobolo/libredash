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
