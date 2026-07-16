package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/release"
)

func TestReleaseEventsDescribePersistedLifecycle(t *testing.T) {
	events := releaseEvents(release.Release{
		ID: "rel-a", ProjectID: "project-a", Status: release.StatusReady,
		CreatedAt: "2026-01-01T00:00:00Z", FinalizedAt: "2026-01-01T00:02:00Z",
		Artifacts: []release.Artifact{{WorkspaceID: "sales", ActualDigest: "sha256:a", UploadedAt: "2026-01-01T00:01:00Z"}},
	})
	if len(events) != 3 || events[0].Event != "release.created" || events[1].Event != "release.artifact_uploaded" || events[2].Event != "release.ready" {
		t.Fatalf("events = %#v", events)
	}
	for index, event := range events {
		if event.Id == "" || event.CreatedAt == "" || event.Data == nil {
			t.Fatalf("event %d is incomplete: %#v", index, event)
		}
	}
}

func TestAsyncEventSSEReplaysAfterLastEventIDAndClosesAtTerminalEvent(t *testing.T) {
	events := releaseEvents(release.Release{
		ID: "rel-a", ProjectID: "project-a", Status: release.StatusReady,
		CreatedAt: "2026-01-01T00:00:00Z", FinalizedAt: "2026-01-01T00:02:00Z",
		Artifacts: []release.Artifact{{WorkspaceID: "sales", ActualDigest: "sha256:a", UploadedAt: "2026-01-01T00:01:00Z"}},
	})
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", events[0].Id)
	rec := httptest.NewRecorder()

	writeAsyncEventPage(rec, req, events, nil, nil, "release:project-a:rel-a")

	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE response status=%d headers=%v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	if strings.Contains(body, "release.created") || !strings.Contains(body, "release.artifact_uploaded") || !strings.Contains(body, "release.ready") {
		t.Fatalf("unexpected replay body: %s", body)
	}
}

func TestAsyncEventSSERejectsUnknownLastEventID(t *testing.T) {
	events := releaseEvents(release.Release{ID: "rel-a", ProjectID: "project-a", Status: release.StatusReady, CreatedAt: "2026-01-01T00:00:00Z", FinalizedAt: "2026-01-01T00:01:00Z"})
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-ID", "missing")
	rec := httptest.NewRecorder()

	writeAsyncEventPage(rec, req, events, nil, nil, "release:project-a:rel-a")

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_LAST_EVENT_ID") {
		t.Fatalf("response status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAsyncEventHeartbeatIntervalIsProtocolValue(t *testing.T) {
	if asyncHeartbeatInterval != 15*time.Second {
		t.Fatalf("heartbeat interval = %s", asyncHeartbeatInterval)
	}
}

func TestAsyncEventPaginationRejectsCursorForAnotherResource(t *testing.T) {
	events := releaseEvents(release.Release{ID: "rel-a", ProjectID: "project-a", Status: release.StatusReady, CreatedAt: "2026-01-01T00:00:00Z", FinalizedAt: "2026-01-01T00:01:00Z"})
	_, cursor, err := pageAsyncEvents(events, 1, "", "release:project-a:rel-a")
	if err != nil || cursor == "" {
		t.Fatalf("first page cursor = %q, error = %v", cursor, err)
	}
	if _, _, err := pageAsyncEvents(events, 1, cursor, "release:project-a:rel-b"); err == nil {
		t.Fatal("expected cursor scope mismatch")
	}
}
