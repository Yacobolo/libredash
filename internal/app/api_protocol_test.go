package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	apiidempotencysqlite "github.com/Yacobolo/leapview/internal/apiidempotency/sqlite"
	"github.com/Yacobolo/leapview/internal/cursorsigning"
	"github.com/Yacobolo/leapview/internal/workspace"
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

func TestAPIGenTransportErrorsUseProblemDetailsWithoutLeakingCause(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects?limit=bad", nil)
	req.Header.Set("X-Request-ID", "req_transport")
	recorder := httptest.NewRecorder()
	apiGenTransportErrorResponder{}.RespondTransportError(req.Context(), recorder, req, apigenapi.GenTransportError{
		OperationID: "listProjects", Kind: "query_parameter", StatusCode: http.StatusBadRequest,
		Code: "INVALID_REQUEST", PublicDetail: "Invalid query parameter.", Cause: errors.New("secret parser detail"),
	})

	if recorder.Code != http.StatusBadRequest || recorder.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("response = %d %q body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "secret parser detail") {
		t.Fatalf("transport cause leaked to client: %s", recorder.Body.String())
	}
	var problem apigenapi.ProblemDetails
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "INVALID_REQUEST" || problem.RequestId != "req_transport" || problem.Instance != "/api/v1/projects" || problem.Detail != "Invalid query parameter." {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestAPIGenTransportErrorsIdentifyInvalidParameterWithoutLeakingValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects?limit=secret-value", nil)
	recorder := httptest.NewRecorder()
	apiGenTransportErrorResponder{}.RespondTransportError(req.Context(), recorder, req, apigenapi.GenTransportError{
		OperationID: "listProjects", Kind: "query_parameter", StatusCode: http.StatusBadRequest,
		Code: "INVALID_REQUEST", PublicDetail: "Invalid query parameter.", Cause: errors.New(`invalid query parameter "limit": invalid integer "secret-value"`),
	})

	if strings.Contains(recorder.Body.String(), "secret-value") {
		t.Fatalf("transport value leaked to client: %s", recorder.Body.String())
	}
	var problem apigenapi.ProblemDetails
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Detail != `Invalid query parameter "limit".` || len(problem.Errors) != 1 || problem.Errors[0].Field != "limit" {
		t.Fatalf("problem = %#v", problem)
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

func TestPublicProtocolIdempotencyReplaysAfterServerRestart(t *testing.T) {
	store := testStore(t)
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/api/v1/principals/p-restart")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p-restart"}`))
	})
	request := func(server *Server) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/principals", bytes.NewBufferString(`{"email":"restart@example.com"}`))
		req.Header.Set("Authorization", "Bearer token-a")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "restart-safe")
		rec := httptest.NewRecorder()
		server.publicProtocolMiddleware(next).ServeHTTP(rec, req)
		return rec
	}

	first := request(NewWithOptions(fakeMetrics{}, Options{Store: store}))
	second := request(NewWithOptions(fakeMetrics{}, Options{Store: store}))
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated || calls != 1 {
		t.Fatalf("first=%d second=%d calls=%d firstBody=%s secondBody=%s", first.Code, second.Code, calls, first.Body.String(), second.Body.String())
	}
	if second.Header().Get("Idempotency-Replayed") != "true" || second.Header().Get("Location") != first.Header().Get("Location") {
		t.Fatalf("restart replay headers = %#v", second.Header())
	}
}

func TestDurableIdempotencyReclaimsExpiredPendingLease(t *testing.T) {
	store := testStore(t)
	db := store.SQLDB()
	now := time.Now().UTC()
	if _, err := db.ExecContext(context.Background(), `INSERT INTO api_idempotency_records(
		scope, request_digest, state, owner_id, lease_expires_at, created_at, updated_at, expires_at
	) VALUES (?, ?, 'pending', ?, ?, ?, ?, ?)`,
		"stale-scope", "same-digest", "dead-server", now.Add(-time.Minute).Format(time.RFC3339Nano),
		now.Add(-time.Hour).Format(time.RFC3339Nano), now.Add(-time.Minute).Format(time.RFC3339Nano), now.Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed stale lease: %v", err)
	}
	record, execute, err := apiidempotencysqlite.NewStore(db).Claim(context.Background(), "stale-scope", "same-digest", "replacement-server", apiIdempotencyLease, apiIdempotencyLifetime)
	if err != nil {
		t.Fatalf("reclaim stale lease: %v", err)
	}
	if !execute || record.Owner != "replacement-server" || record.Digest != "same-digest" || !record.LeaseExpires.After(now) {
		t.Fatalf("reclaimed record = %#v execute=%v", record, execute)
	}
}

func TestDurableIdempotencyDoesNotReplayTransientServerFailures(t *testing.T) {
	store := testStore(t)
	server := NewWithOptions(fakeMetrics{}, Options{Store: store})
	calls := 0
	handler := server.publicProtocolMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/releases", bytes.NewBufferString(`{"projectDigest":"sha256:test"}`))
		req.Header.Set("Authorization", "Bearer dev")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "retry-transient")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	first, second := request(), request()
	if first.Code != http.StatusServiceUnavailable || second.Code != http.StatusCreated || calls != 2 {
		t.Fatalf("first=%d second=%d calls=%d", first.Code, second.Code, calls)
	}
	if second.Header().Get("Idempotency-Replayed") != "" {
		t.Fatalf("successful retry was incorrectly replayed: %#v", second.Header())
	}
}

func TestPublicProtocolMapsStreamedBodyLimitTo413(t *testing.T) {
	server := New(fakeMetrics{})
	handler := requestBodyLimit(RequestBodyLimitConfig{Enabled: true, MaxBytes: 4})(server.publicProtocolMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/principals", strings.NewReader(`{"email":"long@example.com"}`))
	req.ContentLength = -1
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "too-large")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "CONTENT_TOO_LARGE") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
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

func TestPublicProtocolAlwaysRequiresBearerCredentials(t *testing.T) {
	server := New(fakeMetrics{})
	handler := server.publicProtocolMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, tc := range []struct {
		name          string
		authorization string
		want          int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "browser scheme", authorization: "Basic ZGV2OmRldg==", want: http.StatusUnauthorized},
		{name: "empty bearer", authorization: "Bearer", want: http.StatusUnauthorized},
		{name: "bearer", authorization: "Bearer dev", want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
			req.Header.Set("Authorization", tc.authorization)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusUnauthorized && rec.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("content type = %q body=%s", rec.Header().Get("Content-Type"), rec.Body.String())
			}
		})
	}
}

func TestPublicProtocolValidatesConfiguredDevelopmentBearer(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Auth: NewAuth(nil, "", AuthConfig{DevBypass: true, DevAPIToken: "local-secret"})})
	handler := server.publicProtocolMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for token, want := range map[string]int{"wrong": http.StatusUnauthorized, "local-secret": http.StatusNoContent} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("token %q status = %d, want %d body=%s", token, rec.Code, want, rec.Body.String())
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
	next.Header.Set("Authorization", "Bearer token")
	nextRec := httptest.NewRecorder()
	handler.ServeHTTP(nextRec, next)
	if nextRec.Code != http.StatusNoContent || seen != "raw-row-id" {
		t.Fatalf("cursor unwrap status=%d seen=%q body=%s", nextRec.Code, seen, nextRec.Body.String())
	}

	cross := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/other/assets?limit=1&pageToken="+url.QueryEscape(response.Page.NextCursor), nil)
	cross.Header.Set("Authorization", "Bearer token")
	crossRec := httptest.NewRecorder()
	handler.ServeHTTP(crossRec, cross)
	if crossRec.Code != http.StatusBadRequest {
		t.Fatalf("cross-resource cursor status=%d body=%s", crossRec.Code, crossRec.Body.String())
	}
}

func TestPublicListCursorRejectsUnavailableServingSnapshot(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), WorkspaceRepo: apiSnapshotWorkspaceRepository{summary: workspace.Summary{ID: "sales", ActiveServingStateID: "state-new"}}})
	first := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/sales/assets?limit=1", nil)
	first.Header.Set(apiCursorSnapshotHeader, "state-old")
	cursor := signAPIPageCursor(first, "last-asset")
	handler := server.publicProtocolMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	next := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/sales/assets?limit=1&pageToken="+url.QueryEscape(cursor), nil)
	next.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, next)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "SNAPSHOT_UNAVAILABLE") {
		t.Fatalf("snapshot change status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicListCursorSurvivesServerRestartFromDurableKeyRing(t *testing.T) {
	store := testStore(t)
	server := NewWithOptions(fakeMetrics{}, Options{Store: store})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/sales/assets?limit=1", nil)
	cursor := signAPIPageCursor(request, "asset-a")
	if err := cursorsigning.Configure("transient", map[string][]byte{"transient": bytes.Repeat([]byte{9}, 32)}); err != nil {
		t.Fatal(err)
	}
	server = NewWithOptions(fakeMetrics{}, Options{Store: store})
	handler := server.publicProtocolMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("pageToken"); got != "asset-a" {
			t.Fatalf("unwrapped cursor = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	next := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/sales/assets?limit=1&pageToken="+url.QueryEscape(cursor), nil)
	next.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, next)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
