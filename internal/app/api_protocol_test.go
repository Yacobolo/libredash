package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAPIGenResponseBufferNormalizesLegacyErrorsAsProblemDetails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/sales/assets", nil)
	req.Header.Set("X-Request-ID", "req_problem")
	recorder := httptest.NewRecorder()
	buffer := newAPIGenResponseBuffer(recorder, req)
	buffer.Header().Set("Content-Type", "application/json")
	buffer.WriteHeader(http.StatusUnprocessableEntity)
	_, _ = buffer.Write([]byte(`{"code":422,"message":"invalid field","details":{"field":"name"}}`))
	buffer.flush()

	if recorder.Code != http.StatusUnprocessableEntity || recorder.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("response = %d %q body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	for key, want := range map[string]any{
		"status":    float64(422),
		"detail":    "invalid field",
		"instance":  "/api/v1/workspaces/sales/assets",
		"requestId": "req_problem",
	} {
		if problem[key] != want {
			t.Errorf("problem[%s] = %#v, want %#v", key, problem[key], want)
		}
	}
	for _, key := range []string{"type", "title", "code", "errors"} {
		if _, ok := problem[key]; !ok {
			t.Errorf("problem missing %s: %#v", key, problem)
		}
	}
}

func TestPublicProtocolIdempotencyReplaysAndRejectsDigestReuse(t *testing.T) {
	server := New(fakeMetrics{})
	calls := 0
	handler := server.publicProtocolMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/api/v1/principals/p-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p-1"}`))
	}))

	request := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/principals", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer token-a")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "principal-create")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	first := request(`{"email":"a@example.com"}`)
	second := request(`{"email":"a@example.com"}`)
	if first.Code != http.StatusCreated || second.Code != first.Code || second.Body.String() != first.Body.String() || calls != 1 {
		t.Fatalf("replay first=%d/%s second=%d/%s calls=%d", first.Code, first.Body.String(), second.Code, second.Body.String(), calls)
	}
	if second.Header().Get("Idempotency-Replayed") != "true" || second.Header().Get("Location") != first.Header().Get("Location") {
		t.Fatalf("replay headers = %#v", second.Header())
	}
	conflict := request(`{"email":"different@example.com"}`)
	if conflict.Code != http.StatusConflict || calls != 1 || conflict.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("conflict=%d body=%s calls=%d", conflict.Code, conflict.Body.String(), calls)
	}
}

func TestPublicProtocolRequiresIdempotencyKeyForMutationsOnly(t *testing.T) {
	server := New(fakeMetrics{})
	handler := server.publicProtocolMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/v1/principals", http.StatusBadRequest},
		{"/api/v1/workspaces/sales/semantic-models/orders/query", http.StatusNoContent},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(`{}`))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("POST %s = %d, want %d body=%s", tc.path, rec.Code, tc.want, rec.Body.String())
		}
	}
}

func TestPublicListCursorsAreSignedBoundAndUnwrapped(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/sales/assets?limit=1", nil)
	recorder := httptest.NewRecorder()
	buffer := newAPIGenResponseBuffer(recorder, req)
	buffer.Header().Set("Content-Type", "application/json")
	_, _ = buffer.Write([]byte(`{"items":[{"id":"a"}],"page":{"nextCursor":"raw-row-id"}}`))
	buffer.flush()
	var response struct {
		Page struct {
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || !strings.HasPrefix(response.Page.NextCursor, "g1.") {
		t.Fatalf("signed cursor response=%s err=%v", recorder.Body.String(), err)
	}

	server := New(fakeMetrics{})
	seen := ""
	handler := server.publicProtocolMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Query().Get("pageToken")
		w.WriteHeader(http.StatusNoContent)
	}))
	next := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/sales/assets?limit=1&pageToken="+url.QueryEscape(response.Page.NextCursor), nil)
	nextRec := httptest.NewRecorder()
	handler.ServeHTTP(nextRec, next)
	if nextRec.Code != http.StatusNoContent || seen != "raw-row-id" {
		t.Fatalf("cursor unwrap status=%d seen=%q body=%s", nextRec.Code, seen, nextRec.Body.String())
	}

	cross := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/other/assets?limit=1&pageToken="+url.QueryEscape(response.Page.NextCursor), nil)
	crossRec := httptest.NewRecorder()
	handler.ServeHTTP(crossRec, cross)
	if crossRec.Code != http.StatusBadRequest {
		t.Fatalf("cross-resource cursor status=%d body=%s", crossRec.Code, crossRec.Body.String())
	}
}
