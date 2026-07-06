package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpdatesLoginNoopStreamDoesNotRequireAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?route=login", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "event: datastar-patch-signals") || !strings.Contains(body, `"status"`) {
		t.Fatalf("login noop updates did not stream a signal patch:\n%s", body)
	}
}

func TestUpdatesRejectsUnknownRoute(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/updates?route=missing", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUpdatesRequiresRouteQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/updates?dashboard=executive-sales&datastar=%7B%22runtime%22%3A%7B%22kind%22%3A%22dashboard%22%7D%7D", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "updates route is required") {
		t.Fatalf("body = %q, want controlled missing route error", rec.Body.String())
	}
}

func TestLegacyUpdateRoutesAreNotRegistered(t *testing.T) {
	server := New(fakeMetrics{})
	for _, path := range []string{
		"/data/updates",
		"/chat/updates",
		"/admin/storage/updates",
		"/admin/queries/updates",
		"/workspaces/test-workspace/updates",
		"/workspaces/test-workspace/assets/model_table:olist.orders/updates",
		"/workspaces/test-workspace/chat/updates",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		server.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d body=%s", path, rec.Code, http.StatusNotFound, rec.Body.String())
		}
	}
}
