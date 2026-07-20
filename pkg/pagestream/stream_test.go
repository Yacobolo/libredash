package pagestream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/testutil/ssetest"
)

func TestSignalStreamPatchSendsOnePatchSignalsEventPerCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/updates", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	stream := NewSignalStream(rec, req)
	if err := stream.Patch(SignalPatch{"status": "loading"}); err != nil {
		t.Fatalf("patch loading: %v", err)
	}
	if err := stream.Patch(SignalPatch{"status": "ready"}); err != nil {
		t.Fatalf("patch ready: %v", err)
	}

	patches := ssetest.PatchSignals(t, rec.Body.String())
	if len(patches) != 2 || patches[0]["status"] != "loading" || patches[1]["status"] != "ready" {
		t.Fatalf("stream patches = %#v", patches)
	}
}

func TestSignalStreamTracesDirectPatchesWithoutTracingForwardedPatchesTwice(t *testing.T) {
	store := NewTraceStore(TraceOptions{CapacityPerStream: 16, MaxStreams: 2, IncludePayloads: true})
	req := httptest.NewRequest(http.MethodGet, "/updates", nil)
	rec := httptest.NewRecorder()
	stream := NewSignalStream(rec, req, WithStreamTrace(store, "dashboard:page", "dashboard.bootstrap"))
	if err := stream.Patch(SignalPatch{"status": "bootstrap"}); err != nil {
		t.Fatalf("patch bootstrap: %v", err)
	}
	events := store.Events(TraceQuery{StreamID: "dashboard:page", Limit: 10})
	if len(events) != 2 || events[0].Stage != TraceStagePublished || events[1].Stage != TraceStageDelivered || events[1].Origin != "dashboard.bootstrap" {
		t.Fatalf("direct trace events = %#v", events)
	}

	broker := NewBroker(WithTraceStore(store))
	updates, unsubscribe := broker.Subscribe("dashboard:page")
	defer unsubscribe()
	broker.PublishEnvelope("dashboard:page", Envelope{
		Signals: SignalPatch{"status": "async"},
		Trace:   TraceMetadata{Origin: "dashboard.refresh"},
	})
	select {
	case patch := <-updates:
		if err := stream.writeForwarded(patch); err != nil {
			t.Fatalf("write forwarded: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for forwarded patch")
	}
	waitFor(t, time.Second, func() bool {
		return len(store.Events(TraceQuery{StreamID: "dashboard:page", Limit: 10})) >= 4
	})
	if events := store.Events(TraceQuery{StreamID: "dashboard:page", Limit: 10}); len(events) != 4 {
		t.Fatalf("forwarded patch was traced twice: %#v", events)
	}
}

func TestPatchResponseSendsOnePatchSignalsEvent(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/command", nil)
	rec := httptest.NewRecorder()

	if err := PatchResponse(rec, req, SignalPatch{"status": "updated"}); err != nil {
		t.Fatalf("patch response: %v", err)
	}

	patches := ssetest.PatchSignals(t, rec.Body.String())
	if len(patches) != 1 || patches[0]["status"] != "updated" {
		t.Fatalf("patch response patches = %#v", patches)
	}
}

func TestRedirectSendsDatastarRedirectEvent(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/command", nil)
	rec := httptest.NewRecorder()

	if err := Redirect(rec, req, "/chat/abc"); err != nil {
		t.Fatalf("redirect: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: datastar-patch-elements") || !strings.Contains(body, "window.location") || !strings.Contains(body, "/chat/abc") {
		t.Fatalf("redirect response body = %q", body)
	}
}

func TestSignalStreamForwardRelaysBrokerPatchesAndCleansUp(t *testing.T) {
	broker := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/updates", nil).WithContext(ctx)
	rec := newSynchronizedRecorder()
	done := make(chan struct{})

	go func() {
		defer close(done)
		stream := NewSignalStream(rec, req)
		if err := stream.Forward(ctx, broker, "client:page"); err != nil {
			t.Errorf("forward: %v", err)
		}
	}()

	waitFor(t, time.Second, func() bool {
		return broker.SubscriberCount("client:page") == 1
	})
	broker.Publish("client:page", SignalPatch{"status": "broker"})
	waitFor(t, time.Second, func() bool {
		return strings.Contains(rec.BodyString(), "broker")
	})

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
	if got := broker.SubscriberCount("client:page"); got != 0 {
		t.Fatalf("subscriber count after cancellation = %d, want 0", got)
	}

	patches := ssetest.PatchSignals(t, rec.BodyString())
	if len(patches) != 1 || patches[0]["status"] != "broker" {
		t.Fatalf("stream patches = %#v", patches)
	}
}

type synchronizedRecorder struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
}

func newSynchronizedRecorder() *synchronizedRecorder {
	return &synchronizedRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (r *synchronizedRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.Write(p)
}

func (r *synchronizedRecorder) WriteHeader(statusCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ResponseRecorder.WriteHeader(statusCode)
}

func (r *synchronizedRecorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ResponseRecorder.Flush()
}

func (r *synchronizedRecorder) BodyString() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Body.String()
}

func TestSignalStreamForwardRequiresBrokerAndStreamID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/updates", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	if err := NewSignalStream(rec, req).Forward(ctx, nil, "client:page"); err == nil {
		t.Fatal("Forward with nil broker returned nil error")
	}
	if err := NewSignalStream(rec, req).Forward(ctx, NewBroker(), ""); err == nil {
		t.Fatal("Forward with empty stream id returned nil error")
	}
}

func TestSignalStreamWaitStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/updates", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		defer close(done)
		NewSignalStream(rec, req).Wait(ctx)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
