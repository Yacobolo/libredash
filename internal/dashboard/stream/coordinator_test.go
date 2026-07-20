package stream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dataquery"
)

func TestCoordinatorPublishesStartBeforeWorkCompletes(t *testing.T) {
	events := make(chan RefreshEvent, 4)
	workStarted := make(chan struct{})
	release := make(chan struct{})
	coordinator := NewCoordinator(context.Background(), func(event RefreshEvent) {
		events <- event
	})
	t.Cleanup(coordinator.Close)

	refresh, err := coordinator.Begin(func(current dashboard.Filters) (dashboard.Filters, error) {
		current.Controls["state"] = dashboard.FilterControl{Type: "multi_select", Values: []string{"SP"}}
		return current, nil
	}, func(ctx context.Context, publish RefreshPublisher) {
		close(workStarted)
		select {
		case <-release:
			publish(RefreshEvent{Type: RefreshEventVisual, Target: "orders"})
		case <-ctx.Done():
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if refresh.Generation != 1 || refresh.ID == "" {
		t.Fatalf("refresh = %#v", refresh)
	}

	select {
	case event := <-events:
		if event.Type != RefreshEventStart || event.Generation != 1 || event.Filters.Controls["state"].Values[0] != "SP" {
			t.Fatalf("start event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("start event was not published immediately")
	}
	select {
	case <-workStarted:
	case <-time.After(time.Second):
		t.Fatal("work did not start")
	}
	select {
	case event := <-events:
		t.Fatalf("unexpected event before work release: %#v", event)
	default:
	}

	close(release)
	assertRefreshEvent(t, events, RefreshEventVisual, 1)
	assertRefreshEvent(t, events, RefreshEventComplete, 1)
}

func TestCoordinatorPublishesMonotonicBackendProgressAndCompletesThePlan(t *testing.T) {
	events := make(chan RefreshEvent, 8)
	coordinator := NewCoordinator(context.Background(), func(event RefreshEvent) {
		events <- event
	})
	t.Cleanup(coordinator.Close)

	_, err := coordinator.Begin(nil, func(_ context.Context, publish RefreshPublisher) {
		publish(RefreshEvent{Type: RefreshEventProgress, ProgressPercent: testProgressPercent(0)})
		for completed := 1; completed <= 4; completed++ {
			publish(RefreshEvent{Type: RefreshEventProgress, ProgressPercent: testProgressPercent(float64(completed) * 25)})
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	next := func() RefreshEvent {
		t.Helper()
		select {
		case event := <-events:
			return event
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for progress event")
			return RefreshEvent{}
		}
	}

	start := next()
	if start.Type != RefreshEventStart || start.ProgressPercent == nil || *start.ProgressPercent != 0 {
		t.Fatalf("start event = %#v", start)
	}
	for completed := 0; completed <= 4; completed++ {
		event := next()
		if event.Type != RefreshEventProgress || event.ProgressPercent == nil || *event.ProgressPercent != float64(completed)*25 {
			t.Fatalf("progress %d = %#v", completed, event)
		}
	}
	complete := next()
	if complete.Type != RefreshEventComplete || complete.ProgressPercent == nil || *complete.ProgressPercent != 100 {
		t.Fatalf("complete event = %#v", complete)
	}
}

func testProgressPercent(value float64) *float64 { return &value }

func TestCoordinatorDebounceSkipsSupersededGenerationWork(t *testing.T) {
	coordinator := NewCoordinator(context.Background(), func(RefreshEvent) {})
	t.Cleanup(coordinator.Close)
	var mu sync.Mutex
	invoked := []string{}
	startedAt := time.Now()
	work := func(name string) RefreshWork {
		return func(context.Context, RefreshPublisher) {
			mu.Lock()
			invoked = append(invoked, name)
			mu.Unlock()
		}
	}
	begin := func(name string) error {
		_, err := coordinator.BeginPrepared(func(current dashboard.Filters) (RefreshPreparation, error) {
			return RefreshPreparation{Filters: current, Command: "select"}, nil
		}, func(RefreshPreparation) RefreshWork { return work(name) })
		return err
	}
	if err := begin("stale"); err != nil {
		t.Fatal(err)
	}
	if err := begin("current"); err != nil {
		t.Fatal(err)
	}

	eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(invoked) == 1
	})
	mu.Lock()
	defer mu.Unlock()
	if len(invoked) != 1 || invoked[0] != "current" {
		t.Fatalf("invoked work = %#v, want latest generation only", invoked)
	}
	if elapsed := time.Since(startedAt); elapsed < 25*time.Millisecond {
		t.Fatalf("latest work invoked after %s, want generation debounce", elapsed)
	}
}

func TestCoordinatorLatestGenerationSuppressesCanceledResultsAndCompletion(t *testing.T) {
	var (
		mu     sync.Mutex
		events []RefreshEvent
	)
	coordinator := NewCoordinator(context.Background(), func(event RefreshEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})
	t.Cleanup(coordinator.Close)

	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	_, err := coordinator.Begin(func(current dashboard.Filters) (dashboard.Filters, error) {
		return current, nil
	}, func(_ context.Context, publish RefreshPublisher) {
		close(firstStarted)
		<-firstRelease
		publish(RefreshEvent{Type: RefreshEventVisual, Target: "stale"})
	})
	if err != nil {
		t.Fatal(err)
	}
	<-firstStarted

	secondDone := make(chan struct{})
	_, err = coordinator.Begin(func(current dashboard.Filters) (dashboard.Filters, error) {
		return current, nil
	}, func(_ context.Context, publish RefreshPublisher) {
		publish(RefreshEvent{Type: RefreshEventVisual, Target: "current"})
		close(secondDone)
	})
	if err != nil {
		t.Fatal(err)
	}
	<-secondDone
	close(firstRelease)

	eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, event := range events {
			if event.Type == RefreshEventComplete && event.Generation == 2 {
				return true
			}
		}
		return false
	})

	mu.Lock()
	defer mu.Unlock()
	for _, event := range events {
		if event.Generation == 1 && event.Type != RefreshEventStart {
			t.Fatalf("canceled generation published %#v", event)
		}
		if event.Target == "stale" {
			t.Fatalf("stale result published: %#v", events)
		}
	}
}

func TestCoordinatorsForSeparateStreamInstancesDoNotCancelEachOther(t *testing.T) {
	firstEvents := make(chan RefreshEvent, 4)
	secondEvents := make(chan RefreshEvent, 4)
	first := NewCoordinator(context.Background(), func(event RefreshEvent) { firstEvents <- event })
	second := NewCoordinator(context.Background(), func(event RefreshEvent) { secondEvents <- event })
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	release := make(chan struct{})
	work := func(_ context.Context, publish RefreshPublisher) {
		<-release
		publish(RefreshEvent{Type: RefreshEventVisual, Target: "orders"})
	}
	for _, coordinator := range []*Coordinator{first, second} {
		if _, err := coordinator.Begin(func(current dashboard.Filters) (dashboard.Filters, error) { return current, nil }, work); err != nil {
			t.Fatal(err)
		}
	}
	close(release)

	assertRefreshEvent(t, firstEvents, RefreshEventStart, 1)
	assertRefreshEvent(t, firstEvents, RefreshEventVisual, 1)
	assertRefreshEvent(t, firstEvents, RefreshEventComplete, 1)
	assertRefreshEvent(t, secondEvents, RefreshEventStart, 1)
	assertRefreshEvent(t, secondEvents, RefreshEventVisual, 1)
	assertRefreshEvent(t, secondEvents, RefreshEventComplete, 1)
}

func TestCoordinatorPropagatesRefreshCorrelationID(t *testing.T) {
	correlations := make(chan string, 1)
	coordinator := NewCoordinator(context.Background(), func(RefreshEvent) {})
	t.Cleanup(coordinator.Close)
	refresh, err := coordinator.Begin(nil, func(ctx context.Context, _ RefreshPublisher) {
		correlations <- dataquery.MetadataFromContext(ctx).CorrelationID
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case correlation := <-correlations:
		if correlation != refresh.ID {
			t.Fatalf("correlation = %q, want %q", correlation, refresh.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("work did not receive correlation metadata")
	}
}

func TestCoordinatorSummaryDistinguishesPartialAndRefreshErrors(t *testing.T) {
	for _, test := range []struct {
		name              string
		work              RefreshWork
		wantOutcome       string
		wantTerminalError bool
	}{
		{
			name: "partial",
			work: func(_ context.Context, publish RefreshPublisher) {
				publish(RefreshEvent{Type: RefreshEventVisual, Target: "ok", Queries: 1})
				publish(RefreshEvent{Type: RefreshEventTargetError, Target: "visual:bad", Err: errors.New("bad"), Queries: 1})
			},
			wantOutcome: "partial",
		},
		{
			name: "refresh error",
			work: func(_ context.Context, publish RefreshPublisher) {
				publish(RefreshEvent{Type: RefreshEventTargetError, Target: "refresh", Err: errors.New("refresh failed")})
			},
			wantOutcome:       "error",
			wantTerminalError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			summaries := make(chan RefreshSummary, 1)
			events := make(chan RefreshEvent, 8)
			coordinator := NewCoordinator(context.Background(), func(event RefreshEvent) { events <- event })
			coordinator.SetObserver(func(summary RefreshSummary) { summaries <- summary })
			t.Cleanup(coordinator.Close)
			if _, err := coordinator.Begin(nil, test.work); err != nil {
				t.Fatal(err)
			}
			select {
			case summary := <-summaries:
				if summary.Outcome != test.wantOutcome {
					t.Fatalf("outcome = %q, want %q", summary.Outcome, test.wantOutcome)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for summary")
			}
			terminalError := false
			drain := true
			for drain {
				select {
				case event := <-events:
					if event.Type == RefreshEventComplete && event.Err != nil {
						terminalError = true
					}
				default:
					drain = false
				}
			}
			if terminalError != test.wantTerminalError {
				t.Fatalf("terminal error = %t, want %t", terminalError, test.wantTerminalError)
			}
		})
	}
}

func TestCoordinatorSummaryIncludesTargetsCachesAndCancellationReason(t *testing.T) {
	summaries := make(chan RefreshSummary, 2)
	coordinator := NewCoordinator(context.Background(), func(RefreshEvent) {})
	coordinator.SetObserver(func(summary RefreshSummary) { summaries <- summary })
	t.Cleanup(coordinator.Close)

	release := make(chan struct{})
	if _, err := coordinator.BeginPrepared(func(current dashboard.Filters) (RefreshPreparation, error) {
		return RefreshPreparation{Filters: current, Command: "select", Targets: []string{"visual:a", "visual:b"}}, nil
	}, func(RefreshPreparation) RefreshWork {
		return func(_ context.Context, publish RefreshPublisher) {
			<-release
			publish(RefreshEvent{Type: RefreshEventVisual, Target: "a"})
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Begin(nil, func(_ context.Context, publish RefreshPublisher) {
		publish(RefreshEvent{Type: RefreshEventCacheOutcome, CacheOutcome: dataquery.CacheHit})
		publish(RefreshEvent{Type: RefreshEventVisual, Target: "current"})
		publish(RefreshEvent{Type: RefreshEventTargetError, Target: "visual:bad", Err: errors.New("bad")})
	}); err != nil {
		t.Fatal(err)
	}
	close(release)

	canceled := <-summaries
	if canceled.CancellationReason != "superseded" || canceled.PlannedTargets != 2 || canceled.CancellationCount != 1 {
		t.Fatalf("canceled summary = %#v", canceled)
	}
	complete := <-summaries
	if complete.TargetSuccesses != 1 || complete.TargetErrors != 1 || complete.CacheOutcomes[dataquery.CacheHit] != 1 {
		t.Fatalf("complete summary = %#v", complete)
	}
}

func TestCoordinatorSummarySeparatesTargetWorkSumFromCriticalPath(t *testing.T) {
	summaries := make(chan RefreshSummary, 1)
	coordinator := NewCoordinator(context.Background(), func(RefreshEvent) {})
	coordinator.SetObserver(func(summary RefreshSummary) { summaries <- summary })
	t.Cleanup(coordinator.Close)

	if _, err := coordinator.Begin(nil, func(_ context.Context, publish RefreshPublisher) {
		publish(RefreshEvent{Type: RefreshEventProgress, Duration: 30 * time.Millisecond})
		publish(RefreshEvent{Type: RefreshEventVisual, Target: "a", Duration: 90 * time.Millisecond})
		publish(RefreshEvent{Type: RefreshEventProgress, Duration: 20 * time.Millisecond, StageTimingsMs: map[string]float64{"targetCriticalPath": 35}})
	}); err != nil {
		t.Fatal(err)
	}
	summary := <-summaries
	if got := summary.StageTimingsMs["targetWorkSum"]; got != 50 {
		t.Fatalf("target work sum = %v, want 50", got)
	}
	if got := summary.StageTimingsMs["targetCriticalPath"]; got != 35 {
		t.Fatalf("target critical path = %v, want 35", got)
	}
	if _, legacy := summary.StageTimingsMs["targetExecution"]; legacy {
		t.Fatalf("summary retained misleading targetExecution: %#v", summary.StageTimingsMs)
	}
}

type setupRequiredTestError struct{}

func (setupRequiredTestError) Error() string       { return "source data is missing" }
func (setupRequiredTestError) SetupRequired() bool { return true }

func TestCoordinatorTerminalErrorReflectsTargetFailureSeverity(t *testing.T) {
	tests := []struct {
		name              string
		work              RefreshWork
		wantTerminalError bool
		wantSetupRequired bool
	}{
		{
			name: "ordinary partial failure remains component scoped",
			work: func(_ context.Context, publish RefreshPublisher) {
				publish(RefreshEvent{Type: RefreshEventVisual, Target: "ok"})
				publish(RefreshEvent{Type: RefreshEventTargetError, Target: "visual:bad", Err: errors.New("bad")})
			},
		},
		{
			name: "all targets failed",
			work: func(_ context.Context, publish RefreshPublisher) {
				publish(RefreshEvent{Type: RefreshEventTargetError, Target: "visual:bad", Err: errors.New("bad")})
			},
			wantTerminalError: true,
		},
		{
			name: "setup required survives partial success",
			work: func(_ context.Context, publish RefreshPublisher) {
				publish(RefreshEvent{Type: RefreshEventVisual, Target: "ok"})
				publish(RefreshEvent{Type: RefreshEventTargetError, Target: "visual:missing", Err: setupRequiredTestError{}})
			},
			wantTerminalError: true,
			wantSetupRequired: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := make(chan RefreshEvent, 8)
			coordinator := NewCoordinator(context.Background(), func(event RefreshEvent) { events <- event })
			t.Cleanup(coordinator.Close)
			if _, err := coordinator.Begin(nil, test.work); err != nil {
				t.Fatal(err)
			}
			for {
				select {
				case event := <-events:
					if event.Type != RefreshEventComplete {
						continue
					}
					if (event.Err != nil) != test.wantTerminalError {
						t.Fatalf("terminal error = %v, want present=%t", event.Err, test.wantTerminalError)
					}
					if test.wantSetupRequired {
						var setup interface{ SetupRequired() bool }
						if !errors.As(event.Err, &setup) || !setup.SetupRequired() {
							t.Fatalf("terminal error = %v, want setup-required error", event.Err)
						}
					}
					return
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for completion")
				}
			}
		})
	}
}

func assertRefreshEvent(t *testing.T, events <-chan RefreshEvent, wantType RefreshEventType, wantGeneration uint64) {
	t.Helper()
	select {
	case event := <-events:
		if event.Type != wantType || event.Generation != wantGeneration {
			t.Fatalf("event = %#v, want type=%s generation=%d", event, wantType, wantGeneration)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s generation %d", wantType, wantGeneration)
	}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not satisfied")
		}
		time.Sleep(time.Millisecond)
	}
}
