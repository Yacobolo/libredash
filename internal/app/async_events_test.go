package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/Yacobolo/libredash/internal/asyncjob"
	asyncjobsqlite "github.com/Yacobolo/libredash/internal/asyncjob/sqlite"
)

func appendTestAsyncEvent(t *testing.T, repo asyncjob.Repository, kind, id, event string, sequence int) asyncjob.Event {
	t.Helper()
	row, err := repo.AppendEvent(context.Background(), kind, id, event, []byte(fmt.Sprintf(`{"sequence":%d}`, sequence)))
	if err != nil {
		t.Fatalf("append event %d: %v", sequence, err)
	}
	return row
}

func TestAsyncEventSSEReplaysPersistedEventsAfterLastEventIDAndClosesAtTerminalEvent(t *testing.T) {
	repo := asyncjobsqlite.NewRepository(testStore(t).SQLDB())
	first := appendTestAsyncEvent(t, repo, "release", "rel-a", "release.created", 1)
	appendTestAsyncEvent(t, repo, "release", "rel-a", "release.artifact_uploaded", 2)
	appendTestAsyncEvent(t, repo, "release", "rel-a", "release.ready", 3)
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", fmt.Sprintf("%020d", first.ID))
	rec := httptest.NewRecorder()

	writeStoredAsyncEventPage(rec, req, repo, "release", "rel-a", nil, nil, "release:project-a:rel-a")

	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE response status=%d headers=%v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	if strings.Contains(body, "release.created") || !strings.Contains(body, "release.artifact_uploaded") || !strings.Contains(body, "release.ready") {
		t.Fatalf("unexpected replay body: %s", body)
	}
}

func TestAsyncEventSSERejectsUnknownLastEventID(t *testing.T) {
	repo := asyncjobsqlite.NewRepository(testStore(t).SQLDB())
	appendTestAsyncEvent(t, repo, "release", "rel-a", "release.ready", 1)
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", "99")
	rec := httptest.NewRecorder()

	writeStoredAsyncEventPage(rec, req, repo, "release", "rel-a", nil, nil, "release:project-a:rel-a")

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_LAST_EVENT_ID") {
		t.Fatalf("response status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAsyncEventHistoryPagesDirectlyBeyondTwoHundredRecords(t *testing.T) {
	repo := asyncjobsqlite.NewRepository(testStore(t).SQLDB())
	for index := 1; index <= 205; index++ {
		appendTestAsyncEvent(t, repo, "refresh", "run-a", "refresh.progress", index)
	}
	limit := int32(200)
	firstReq := httptest.NewRequest(http.MethodGet, "/events", nil)
	firstRec := httptest.NewRecorder()
	writeStoredAsyncEventPage(firstRec, firstReq, repo, "refresh", "run-a", &limit, nil, "refresh:sales:run-a")
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", firstRec.Code, firstRec.Body.String())
	}
	var first apigenapi.AsyncEventListResponse
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(first.Items) != 200 || first.Page.NextCursor == nil || *first.Page.NextCursor == "" {
		t.Fatalf("first page count=%d cursor=%v", len(first.Items), first.Page.NextCursor)
	}
	if first.Items[0].ResourceType != "refresh" || first.Items[0].ResourceId != "run-a" {
		t.Fatalf("event envelope = %#v", first.Items[0])
	}
	secondReq := httptest.NewRequest(http.MethodGet, "/events", nil)
	secondRec := httptest.NewRecorder()
	writeStoredAsyncEventPage(secondRec, secondReq, repo, "refresh", "run-a", &limit, first.Page.NextCursor, "refresh:sales:run-a")
	var second apigenapi.AsyncEventListResponse
	if err := json.Unmarshal(secondRec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second page: %v body=%s", err, secondRec.Body.String())
	}
	if secondRec.Code != http.StatusOK || len(second.Items) != 5 || second.Page.NextCursor != nil {
		t.Fatalf("second page status=%d count=%d cursor=%v", secondRec.Code, len(second.Items), second.Page.NextCursor)
	}
}

func TestAsyncEventResponsePromotesCommonProgressAndErrorFields(t *testing.T) {
	rows := []asyncjob.Event{{
		ID: 1, ResourceKind: "refresh", ResourceID: "run-a", EventType: "refresh.failed", CreatedAt: "2026-07-16T12:00:00Z",
		Data: []byte(`{"progress":{"current":7,"total":10,"percent":70},"error":{"code":"QUERY_FAILED","detail":"warehouse unavailable"},"stage":"load"}`),
	}}
	events, err := asyncEventResponses(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Progress == nil || events[0].Progress.Current == nil || *events[0].Progress.Current != 7 {
		t.Fatalf("progress envelope = %#v", events)
	}
	if events[0].Error == nil || events[0].Error.Code != "QUERY_FAILED" || events[0].Data["stage"] != "load" {
		t.Fatalf("error/data envelope = %#v", events[0])
	}
	if _, duplicated := events[0].Data["progress"]; duplicated {
		t.Fatalf("progress duplicated in domain data: %#v", events[0].Data)
	}
}

func TestAsyncEventResponseNormalizesSQLiteTimestamp(t *testing.T) {
	events, err := asyncEventResponses([]asyncjob.Event{{
		ID: 1, ResourceKind: "refresh", ResourceID: "run-a", EventType: "refresh.succeeded",
		CreatedAt: "2026-07-19 06:00:00.123", Data: []byte(`{"status":"succeeded"}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].CreatedAt != "2026-07-19T06:00:00.123Z" {
		t.Fatalf("event timestamp = %#v, want RFC3339", events)
	}
}

func TestAsyncEventPaginationRejectsCursorForAnotherResource(t *testing.T) {
	repo := asyncjobsqlite.NewRepository(testStore(t).SQLDB())
	appendTestAsyncEvent(t, repo, "release", "rel-a", "release.created", 1)
	appendTestAsyncEvent(t, repo, "release", "rel-a", "release.ready", 2)
	limit := int32(1)
	firstReq := httptest.NewRequest(http.MethodGet, "/events", nil)
	firstRec := httptest.NewRecorder()
	writeStoredAsyncEventPage(firstRec, firstReq, repo, "release", "rel-a", &limit, nil, "release:project-a:rel-a")
	var first apigenapi.AsyncEventListResponse
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil || first.Page.NextCursor == nil {
		t.Fatalf("first page cursor=%v error=%v body=%s", first.Page.NextCursor, err, firstRec.Body.String())
	}
	secondReq := httptest.NewRequest(http.MethodGet, "/events", nil)
	secondRec := httptest.NewRecorder()
	writeStoredAsyncEventPage(secondRec, secondReq, repo, "release", "rel-a", &limit, first.Page.NextCursor, "release:project-a:rel-b")
	if secondRec.Code != http.StatusBadRequest || !strings.Contains(secondRec.Body.String(), "INVALID_CURSOR") {
		t.Fatalf("response status=%d body=%s", secondRec.Code, secondRec.Body.String())
	}
}

func TestAsyncEventProtocolTimingValues(t *testing.T) {
	if asyncHeartbeatInterval != 15*time.Second {
		t.Fatalf("heartbeat interval = %s", asyncHeartbeatInterval)
	}
	if asyncStreamAuthorizationLifetime > 5*time.Minute {
		t.Fatalf("authorization lifetime = %s", asyncStreamAuthorizationLifetime)
	}
}
