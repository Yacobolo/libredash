package pagestream

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestTraceStoreRecordsPublishedAndDeliveredSanitizedEnvelopes(t *testing.T) {
	var logs bytes.Buffer
	store := NewTraceStore(TraceOptions{
		CapacityPerStream: 16,
		MaxStreams:        2,
		Logger:            slog.New(slog.NewJSONHandler(&logs, nil)),
		IncludePayloads:   true,
	})
	broker := NewBroker(WithTraceStore(store))
	updates, unsubscribe := broker.Subscribe("dashboard:page")
	defer unsubscribe()

	broker.PublishEnvelope("dashboard:page", Envelope{
		Signals: SignalPatch{
			"status":   map[string]any{"loading": true},
			"password": "do-not-record",
		},
		Delivery: DeliveryMetadata{Generation: 3, Boundary: true},
		Trace:    TraceMetadata{Origin: "dashboard.refresh", CorrelationID: "refresh-3"},
	})
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for traced patch")
	}
	waitFor(t, time.Second, func() bool {
		return len(store.Events(TraceQuery{StreamID: "dashboard:page", Limit: 10})) >= 2
	})

	events := store.Events(TraceQuery{StreamID: "dashboard:page", Limit: 10})
	if len(events) != 2 || events[0].Stage != TraceStagePublished || events[1].Stage != TraceStageDelivered {
		t.Fatalf("events = %#v, want published and delivered", events)
	}
	delivered := events[1]
	if delivered.Sequence != 1 || delivered.Generation != 3 || delivered.CorrelationID != "refresh-3" || delivered.Origin != "dashboard.refresh" {
		t.Fatalf("delivered metadata = %#v", delivered)
	}
	if delivered.Bytes == 0 || delivered.Digest == "" || delivered.QueueMilliseconds < 0 {
		t.Fatalf("delivered diagnostics = %#v", delivered)
	}
	if got := delivered.Payload["password"]; got != "[REDACTED]" {
		t.Fatalf("sanitized password = %#v", got)
	}
	if strings.Contains(logs.String(), "do-not-record") || !strings.Contains(logs.String(), `"stage":"delivered"`) {
		t.Fatalf("trace logs = %s", logs.String())
	}
}

func TestTraceStoreIsBoundedAndSupportsIncrementalQueries(t *testing.T) {
	store := NewTraceStore(TraceOptions{CapacityPerStream: 2, MaxStreams: 1, IncludePayloads: true})
	store.Record(TraceRecord{StreamID: "one", Stage: TraceStagePublished, Signals: SignalPatch{"value": 1}})
	first := store.Events(TraceQuery{StreamID: "one", Limit: 10})
	store.Record(TraceRecord{StreamID: "one", Stage: TraceStagePublished, Signals: SignalPatch{"value": 2}})
	store.Record(TraceRecord{StreamID: "one", Stage: TraceStagePublished, Signals: SignalPatch{"value": 3}})

	events := store.Events(TraceQuery{StreamID: "one", After: first[0].ID, Limit: 10})
	if len(events) != 2 || events[0].Payload["value"] != float64(2) || events[1].Payload["value"] != float64(3) {
		t.Fatalf("bounded incremental events = %#v", events)
	}

	store.Record(TraceRecord{StreamID: "two", Stage: TraceStagePublished, Signals: SignalPatch{"value": 4}})
	if events := store.Events(TraceQuery{StreamID: "one", Limit: 10}); len(events) != 0 {
		t.Fatalf("evicted stream events = %#v", events)
	}
}

func TestTraceStoreIncrementalLimitReturnsOldestUnreadEventsFirst(t *testing.T) {
	store := NewTraceStore(TraceOptions{CapacityPerStream: 8, MaxStreams: 1, IncludePayloads: true})
	for value := 1; value <= 3; value++ {
		store.Record(TraceRecord{StreamID: "one", Stage: TraceStagePublished, Signals: SignalPatch{"value": value}})
	}
	first := store.Events(TraceQuery{StreamID: "one", Limit: 2})
	if len(first) != 2 || first[0].Payload["value"] != float64(1) || first[1].Payload["value"] != float64(2) {
		t.Fatalf("first page = %#v", first)
	}
	second := store.Events(TraceQuery{StreamID: "one", After: first[1].ID, Limit: 2})
	if len(second) != 1 || second[0].Payload["value"] != float64(3) {
		t.Fatalf("second page = %#v", second)
	}
}

func TestTraceStoreBuildsEffectiveDeliveredSignalHistory(t *testing.T) {
	store := NewTraceStore(TraceOptions{CapacityPerStream: 16, SignalCapacityPerStream: 32, MaxStreams: 2})

	store.Record(TraceRecord{
		StreamID: "dashboard:page:tab-1", Stage: TraceStagePublished,
		Signals: SignalPatch{"status": map[string]any{"progressPercent": 0}},
	})
	first := store.Record(TraceRecord{
		StreamID: "dashboard:page:tab-1", Stage: TraceStageDelivered,
		Signals:    SignalPatch{"status": map[string]any{"progressPercent": 0, "loading": true}},
		Generation: 4, Origin: "dashboard.refresh", CorrelationID: "refresh-4",
	})
	store.Record(TraceRecord{
		StreamID: "dashboard:page:tab-1", Stage: TraceStageDelivered,
		Signals:    SignalPatch{"status": map[string]any{"progressPercent": 0}},
		Generation: 4, Origin: "dashboard.refresh", CorrelationID: "refresh-4",
	})
	second := store.Record(TraceRecord{
		StreamID: "dashboard:page:tab-1", Stage: TraceStageDelivered,
		Signals:    SignalPatch{"status": map[string]any{"progressPercent": 25}},
		Generation: 4, Origin: "dashboard.refresh", CorrelationID: "refresh-4",
	})
	third := store.Record(TraceRecord{
		StreamID: "dashboard:page:tab-1", Stage: TraceStageDelivered,
		Signals:    SignalPatch{"status": map[string]any{"progressPercent": nil}},
		Generation: 4, Origin: "dashboard.refresh", CorrelationID: "refresh-4",
	})

	snapshot, ok := store.SignalSnapshot("dashboard:page:tab-1")
	if !ok {
		t.Fatal("expected delivered signal snapshot")
	}
	status := snapshot.State["status"].(map[string]any)
	if status["loading"] != true {
		t.Fatalf("status = %#v", status)
	}
	if _, exists := status["progressPercent"]; exists {
		t.Fatalf("deleted progressPercent remained in state: %#v", status)
	}
	if len(snapshot.Leaves) != 1 || snapshot.Leaves[0].Path != "/status/loading" {
		t.Fatalf("leaves = %#v", snapshot.Leaves)
	}

	changes := store.SignalChanges(SignalHistoryQuery{
		StreamID: "dashboard:page:tab-1", Path: "/status/progressPercent", Limit: 10,
	})
	if len(changes) != 3 {
		t.Fatalf("progress history = %#v, want set 0, set 25, removed", changes)
	}
	if changes[0].Operation != SignalChangeSet || changes[0].Value != float64(0) || changes[0].TraceEventID != first.ID {
		t.Fatalf("first change = %#v", changes[0])
	}
	if changes[1].Operation != SignalChangeSet || changes[1].Value != float64(25) || changes[1].TraceEventID != second.ID {
		t.Fatalf("second change = %#v", changes[1])
	}
	if changes[2].Operation != SignalChangeRemoved || changes[2].TraceEventID != third.ID {
		t.Fatalf("third change = %#v", changes[2])
	}
	for _, change := range changes {
		if change.DisplayPath != "status.progressPercent" || change.Generation != 4 || change.Origin != "dashboard.refresh" || change.CorrelationID != "refresh-4" {
			t.Fatalf("change metadata = %#v", change)
		}
	}
}

func TestTraceStoreSignalHistoryIsSanitizedAtomicBoundedAndStreamScoped(t *testing.T) {
	store := NewTraceStore(TraceOptions{CapacityPerStream: 8, SignalCapacityPerStream: 8, MaxStreams: 2})
	store.Record(TraceRecord{
		StreamID: "one", Stage: TraceStageDelivered,
		Signals: SignalPatch{
			"rows": []any{map[string]any{"id": 1}, map[string]any{"id": 2}},
			"auth": map[string]any{"accessToken": "private"},
			"a/b":  1,
		},
	})
	store.Record(TraceRecord{StreamID: "two", Stage: TraceStageDelivered, Signals: SignalPatch{"value": 9}})

	snapshot, ok := store.SignalSnapshot("one")
	if !ok {
		t.Fatal("expected stream one snapshot")
	}
	if got := snapshot.State["auth"].(map[string]any)["accessToken"]; got != "[REDACTED]" {
		t.Fatalf("sanitized state token = %#v", got)
	}
	rows := store.SignalChanges(SignalHistoryQuery{StreamID: "one", Path: "/rows", Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("array must be one atomic history value: %#v", rows)
	}
	escaped := store.SignalChanges(SignalHistoryQuery{StreamID: "one", Path: "/a~1b", Limit: 10})
	if len(escaped) != 1 || escaped[0].DisplayPath != `["a/b"]` {
		t.Fatalf("escaped-key history = %#v", escaped)
	}
	if changes := store.SignalChanges(SignalHistoryQuery{StreamID: "one", Limit: 10}); len(changes) != 3 {
		t.Fatalf("signal changes = %#v", changes)
	}
	if changes := store.SignalChanges(SignalHistoryQuery{StreamID: "two", Limit: 10}); len(changes) != 1 || changes[0].Path != "/value" {
		t.Fatalf("stream two changes = %#v", changes)
	}

	bounded := NewTraceStore(TraceOptions{SignalCapacityPerStream: 2, MaxStreams: 1})
	for value := 1; value <= 3; value++ {
		bounded.Record(TraceRecord{StreamID: "bounded", Stage: TraceStageDelivered, Signals: SignalPatch{"value": value}})
	}
	changes := bounded.SignalChanges(SignalHistoryQuery{StreamID: "bounded", Path: "/value", Limit: 10})
	if len(changes) != 2 || changes[0].Value != float64(2) || changes[1].Value != float64(3) {
		t.Fatalf("bounded signal history = %#v", changes)
	}
}
