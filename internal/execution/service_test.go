package execution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/dataquery"
)

func TestServiceQueuesReadsAndRejectsWhenFull(t *testing.T) {
	service := New(Config{MaxRunningReads: 1, MaxQueuedReads: 1, ReadQueueWait: time.Second, MaxRunningJobs: 1, MaxQueuedJobs: 1})
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondDone := make(chan error, 1)

	go func() {
		_, _ = service.SubmitRead(context.Background(), dataquery.Query{}, func(context.Context) (dataquery.Result, error) {
			close(firstStarted)
			<-releaseFirst
			return dataquery.Result{}, nil
		})
	}()
	<-firstStarted

	go func() {
		_, err := service.SubmitRead(context.Background(), dataquery.Query{}, func(context.Context) (dataquery.Result, error) {
			return dataquery.Result{}, nil
		})
		secondDone <- err
	}()
	waitForQueueDepth(t, service.waiting, 1)

	result, err := service.SubmitRead(context.Background(), dataquery.Query{}, func(context.Context) (dataquery.Result, error) {
		t.Fatal("third read should not execute")
		return dataquery.Result{}, nil
	})
	if !errors.Is(err, ErrReadQueueFull) {
		t.Fatalf("third read error = %v, want ErrReadQueueFull", err)
	}
	if result.ExecutionState != dataquery.ExecutionRejected {
		t.Fatalf("third read execution state = %q, want rejected", result.ExecutionState)
	}

	close(releaseFirst)
	if err := <-secondDone; err != nil {
		t.Fatalf("queued read error = %v", err)
	}
}

func TestServiceRecordsReadTelemetry(t *testing.T) {
	service := New(Config{MaxRunningReads: 1, MaxQueuedReads: 1, ReadQueueWait: time.Second, MaxRunningJobs: 1, MaxQueuedJobs: 1})
	result, err := service.SubmitRead(context.Background(), dataquery.Query{}, func(context.Context) (dataquery.Result, error) {
		time.Sleep(time.Millisecond)
		return dataquery.Result{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExecutionState != dataquery.ExecutionSucceeded {
		t.Fatalf("execution state = %q, want succeeded", result.ExecutionState)
	}
	if result.ExecutionMS <= 0 {
		t.Fatalf("execution ms = %d, want positive", result.ExecutionMS)
	}
}

func TestSubmitReadFromContextAdmitsOnePhysicalReadAndDoesNotDoubleAdmit(t *testing.T) {
	service := New(Config{MaxRunningReads: 1, MaxQueuedReads: -1, ReadQueueWait: time.Second})
	ctx := WithReadAdmission(context.Background(), service)
	var calls int
	result, err := SubmitReadFromContext(ctx, dataquery.Query{Kind: dataquery.KindSemanticRows}, func(ctx context.Context) (dataquery.Result, error) {
		calls++
		if service.Stats().RunningReads != 1 {
			t.Fatalf("running reads = %d, want 1", service.Stats().RunningReads)
		}
		return SubmitReadFromContext(ctx, dataquery.Query{Kind: dataquery.KindSemanticRows}, func(context.Context) (dataquery.Result, error) {
			calls++
			return dataquery.Result{RowsReturned: 1}, nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.RowsReturned != 1 {
		t.Fatalf("nested admitted result calls=%d result=%#v", calls, result)
	}
	if service.Stats().RunningReads != 0 {
		t.Fatalf("running reads after release = %d, want 0", service.Stats().RunningReads)
	}
}

func TestServiceAppliesReadExecutionTimeout(t *testing.T) {
	service := New(Config{
		MaxRunningReads:      1,
		MaxQueuedReads:       1,
		ReadQueueWait:        time.Second,
		ReadExecutionTimeout: 10 * time.Millisecond,
		MaxRunningJobs:       1,
		MaxQueuedJobs:        1,
	})
	result, err := service.SubmitRead(context.Background(), dataquery.Query{}, func(ctx context.Context) (dataquery.Result, error) {
		<-ctx.Done()
		return dataquery.Result{}, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("read error = %v, want deadline exceeded", err)
	}
	if result.ExecutionState != dataquery.ExecutionTimeout {
		t.Fatalf("execution state = %q, want timeout", result.ExecutionState)
	}
}

func TestServiceCanceledQueuedReadDoesNotExecute(t *testing.T) {
	service := New(Config{MaxRunningReads: 1, MaxQueuedReads: 1, ReadQueueWait: time.Second, MaxRunningJobs: 1, MaxQueuedJobs: 1})
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	go func() {
		_, _ = service.SubmitRead(context.Background(), dataquery.Query{}, func(context.Context) (dataquery.Result, error) {
			close(firstStarted)
			<-releaseFirst
			return dataquery.Result{}, nil
		})
	}()
	<-firstStarted

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := service.SubmitRead(ctx, dataquery.Query{}, func(context.Context) (dataquery.Result, error) {
			t.Fatal("canceled queued read should not execute")
			return dataquery.Result{}, nil
		})
		errCh <- err
	}()
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("queued read error = %v, want context.Canceled", err)
	}
	close(releaseFirst)
}

func TestServiceRunsOneJobAtATime(t *testing.T) {
	service := New(Config{MaxRunningReads: 1, MaxQueuedReads: 1, MaxRunningJobs: 1, MaxQueuedJobs: 2})
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	var mu sync.Mutex
	running := 0
	maxRunning := 0

	run := func(started chan<- struct{}, release <-chan struct{}) error {
		mu.Lock()
		running++
		if running > maxRunning {
			maxRunning = running
		}
		mu.Unlock()
		close(started)
		if release != nil {
			<-release
		}
		mu.Lock()
		running--
		mu.Unlock()
		return nil
	}

	go func() {
		_ = service.SubmitJob(context.Background(), JobRef{RunID: "first"}, func(context.Context) error {
			return run(firstStarted, releaseFirst)
		})
	}()
	<-firstStarted
	go func() {
		_ = service.SubmitJob(context.Background(), JobRef{RunID: "second"}, func(context.Context) error {
			return run(secondStarted, nil)
		})
	}()

	select {
	case <-secondStarted:
		t.Fatal("second job started before first released")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	<-secondStarted

	mu.Lock()
	defer mu.Unlock()
	if maxRunning != 1 {
		t.Fatalf("max running jobs = %d, want 1", maxRunning)
	}
}

func waitForQueueDepth(t *testing.T, queue chan struct{}, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(queue) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("queue depth = %d, want %d", len(queue), want)
}
