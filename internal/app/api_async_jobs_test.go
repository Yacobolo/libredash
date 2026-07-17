package app

import (
	"context"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/asyncjob"
)

func TestBackgroundLifecycleReclaimsPersistedAPIJobs(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), JobLeaseTimeout: time.Second})
	repo, err := server.asyncRepository()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Enqueue(t.Context(), asyncjob.EnqueueInput{ID: "job-restart", Kind: "test.unsupported", ResourceKind: "test", ResourceID: "resource-1", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	server.StartBackgroundJobs(ctx)
	t.Cleanup(func() {
		cancel()
		stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = server.StopBackgroundJobs(stopCtx)
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, getErr := repo.Get(t.Context(), "job-restart")
		if getErr == nil && job.Status == asyncjob.StatusFailed {
			if job.Attempts != 1 || job.FinishedAt == "" {
				t.Fatalf("failed job = %#v", job)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("persisted API job was not claimed by the background worker")
}
