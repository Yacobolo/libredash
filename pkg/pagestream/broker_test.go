package pagestream

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

func TestBrokerPublishSubscribeAndUnsubscribe(t *testing.T) {
	broker := NewBroker()
	updates, unsubscribe := broker.Subscribe("client:page")

	if got := broker.SubscriberCount("client:page"); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}

	broker.Publish("client:page", SignalPatch{"status": "ready"})
	select {
	case patch := <-updates:
		if patch["status"] != "ready" {
			t.Fatalf("patch = %#v", patch)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broker patch")
	}

	unsubscribe()
	if got := broker.SubscriberCount("client:page"); got != 0 {
		t.Fatalf("subscriber count after unsubscribe = %d, want 0", got)
	}
}

func TestBrokerPublishDoesNotBlockWhenSubscriberChannelIsFull(t *testing.T) {
	broker := NewBroker()
	_, unsubscribe := broker.Subscribe("client:page")
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 32; i++ {
			broker.Publish("client:page", SignalPatch{"seq": i})
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publish blocked on a full subscriber channel")
	}
}

func TestBrokerCoalescesBurstWithoutDroppingMergeSignals(t *testing.T) {
	broker := NewBroker()
	updates, unsubscribe := broker.Subscribe("client:page")
	defer unsubscribe()

	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("visual-%03d", i)
		broker.PublishEnvelope("client:page", Envelope{
			Signals: SignalPatch{"visuals": map[string]any{id: i}},
			Delivery: DeliveryMetadata{
				CoalesceGroup: "visual-results",
				MergeRoots:    []string{"visuals"},
			},
		})
	}

	got := map[string]any{}
	deadline := time.After(time.Second)
	for len(got) < 100 {
		select {
		case patch := <-updates:
			visuals, ok := patch["visuals"].(map[string]any)
			if !ok {
				t.Fatalf("visuals patch = %#v", patch["visuals"])
			}
			for id, value := range visuals {
				got[id] = value
			}
		case <-deadline:
			t.Fatalf("received %d/100 visual patches", len(got))
		}
	}
}

func TestBrokerCoalescesReplaceSignalsToNewestValue(t *testing.T) {
	broker := NewBroker()
	updates, unsubscribe := broker.Subscribe("client:page")
	defer unsubscribe()

	broker.PublishEnvelope("client:page", Envelope{
		Signals:  SignalPatch{"status": map[string]any{"generation": 1}},
		Delivery: DeliveryMetadata{CoalesceGroup: "status"},
	})
	for generation := 2; generation <= 100; generation++ {
		broker.PublishEnvelope("client:page", Envelope{
			Signals:  SignalPatch{"status": map[string]any{"generation": generation}},
			Delivery: DeliveryMetadata{CoalesceGroup: "status"},
		})
	}

	latest := 0
	deadline := time.After(time.Second)
	for latest < 100 {
		select {
		case patch := <-updates:
			status := patch["status"].(map[string]any)
			latest = status["generation"].(int)
		case <-deadline:
			t.Fatalf("latest generation = %d, want 100", latest)
		}
	}
}

func TestBrokerPreservesLoadingFeedbackBeforeImmediateCompletion(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	broker := NewBroker()
	updates, unsubscribe := broker.Subscribe("client:page")
	defer unsubscribe()

	// Publish both phases before the subscriber drains its mailbox. Completion
	// must not replace the observable loading feedback.
	broker.Publish("client:page", SignalPatch{"status": map[string]any{"loading": true, "generation": 1}})
	broker.Publish("client:page", SignalPatch{"status": map[string]any{"loading": false, "generation": 1}})

	for index, wantLoading := range []bool{true, false} {
		select {
		case patch := <-updates:
			status, ok := patch["status"].(map[string]any)
			if !ok || status["loading"] != wantLoading {
				t.Fatalf("phase %d patch = %#v, want loading=%t", index, patch, wantLoading)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for loading phase %d", index)
		}
	}
}

func TestBrokerPreservesDashboardRefreshPercentageMilestones(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)

	broker := NewBroker()
	updates, unsubscribe := broker.Subscribe("client:page")
	defer unsubscribe()

	progressPatch := func(percent int) SignalPatch {
		return SignalPatch{"status": map[string]any{
			"loading":         true,
			"generation":      int64(7),
			"progressPercent": percent,
		}}
	}

	for percent := 0; percent <= 100; percent += 25 {
		broker.PublishEnvelope("client:page", Envelope{
			Signals:  progressPatch(percent),
			Delivery: DeliveryMetadata{Generation: 7, Boundary: true},
		})
	}

	for wantPercent := 0; wantPercent <= 100; wantPercent += 25 {
		select {
		case patch := <-updates:
			status := patch["status"].(map[string]any)
			if status["progressPercent"] != wantPercent {
				t.Fatalf("milestone %d patch = %#v", wantPercent, patch)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for milestone %d", wantPercent)
		}
	}
}

func TestBrokerPreservesNestedRunningFeedbackBeforeImmediateCompletion(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	broker := NewBroker()
	updates, unsubscribe := broker.Subscribe("workspace:asset")
	defer unsubscribe()

	broker.Publish("workspace:asset", SignalPatch{"page": map[string]any{
		"refresh": map[string]any{"running": true, "status": "running"},
	}})
	broker.Publish("workspace:asset", SignalPatch{"page": map[string]any{
		"refresh": map[string]any{"running": false, "status": "succeeded"},
	}})

	for index, wantRunning := range []bool{true, false} {
		select {
		case patch := <-updates:
			page := patch["page"].(map[string]any)
			refresh := page["refresh"].(map[string]any)
			if refresh["running"] != wantRunning {
				t.Fatalf("phase %d patch = %#v, want running=%t", index, patch, wantRunning)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for running phase %d", index)
		}
	}
}

func TestBrokerDropsPendingOlderGenerationWhenNewGenerationStarts(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)

	broker := NewBroker()
	updates, unsubscribe := broker.Subscribe("client:page")
	defer unsubscribe()

	broker.PublishEnvelope("client:page", Envelope{
		Signals:  SignalPatch{"status": map[string]any{"loading": true, "generation": int64(1)}},
		Delivery: DeliveryMetadata{Generation: 1, Boundary: true},
	})
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for generation 1 start")
	}

	// Let a generation 1 component result reach the slow subscriber's output
	// buffer, but do not consume it. A generation 2 start must evict that result
	// rather than merge it into generation 2 or deliver it afterward.
	broker.PublishEnvelope("client:page", Envelope{Signals: SignalPatch{
		"visuals":           map[string]any{"old-visual": "generation-1"},
		"tables":            map[string]any{"old-table": "generation-1"},
		"filterOptionPages": map[string]any{"old-filter": "generation-1"},
		"componentStatus": map[string]any{
			"visual:old-visual": map[string]any{"generation": int64(1), "loading": false},
		},
	}, Delivery: DeliveryMetadata{Generation: 1, CoalesceGroup: "dashboard-results"}})
	deadline := time.Now().Add(time.Second)
	for len(updates) == 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if len(updates) == 0 {
		t.Fatal("generation 1 component result never reached the subscriber buffer")
	}

	broker.PublishEnvelope("client:page", Envelope{Signals: SignalPatch{
		"status": map[string]any{"loading": true, "generation": int64(2)},
		"componentStatus": map[string]any{
			"visual:new-visual": map[string]any{"generation": int64(2), "loading": true},
		},
	}, Delivery: DeliveryMetadata{Generation: 2, Boundary: true}})

	select {
	case patch := <-updates:
		status, ok := patch["status"].(map[string]any)
		if !ok || status["generation"] != int64(2) {
			t.Fatalf("first patch after generation 2 starts = %#v", patch)
		}
		for _, staleKey := range []string{"visuals", "tables", "filterOptionPages"} {
			if _, exists := patch[staleKey]; exists {
				t.Fatalf("generation 2 start contains stale %s: %#v", staleKey, patch)
			}
		}
		componentStatus := patch["componentStatus"].(map[string]any)
		if _, exists := componentStatus["visual:old-visual"]; exists {
			t.Fatalf("generation 2 start contains stale component status: %#v", patch)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for generation 2 start")
	}

	broker.PublishEnvelope("client:page", Envelope{
		Signals:  SignalPatch{"status": map[string]any{"loading": false, "generation": int64(2)}},
		Delivery: DeliveryMetadata{Generation: 2, Boundary: true},
	})
	select {
	case patch := <-updates:
		status, ok := patch["status"].(map[string]any)
		if !ok || status["generation"] != int64(2) || status["loading"] != false {
			t.Fatalf("generation 2 completion = %#v", patch)
		}
		for _, staleKey := range []string{"visuals", "tables", "filterOptionPages"} {
			if _, exists := patch[staleKey]; exists {
				t.Fatalf("generation 2 completion contains stale %s: %#v", staleKey, patch)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for generation 2 completion")
	}
}
