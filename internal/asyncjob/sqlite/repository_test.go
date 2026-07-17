package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/asyncjob"
	"github.com/Yacobolo/libredash/internal/platform"
)

func TestRepositoryPersistsClaimsReclaimsAndOrderedEvents(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewRepository(store.SQLDB())

	created, err := repo.Enqueue(t.Context(), asyncjob.EnqueueInput{
		ID: "job-1", Kind: "release.finalize", ResourceKind: "release", ResourceID: "release-1", Payload: []byte(`{"project":"project-a"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != asyncjob.StatusQueued {
		t.Fatalf("status = %q", created.Status)
	}

	claimed, ok, err := repo.Claim(t.Context(), "worker-a", time.Minute)
	if err != nil || !ok || claimed.ID != created.ID || claimed.Attempts != 1 {
		t.Fatalf("claim = %#v, %v, %v", claimed, ok, err)
	}
	if _, ok, err := repo.Claim(t.Context(), "worker-b", time.Minute); err != nil || ok {
		t.Fatalf("leased job claimed twice: ok=%v err=%v", ok, err)
	}
	if _, err := store.SQLDB().ExecContext(t.Context(), `UPDATE api_async_jobs SET lease_expires_at = datetime('now', '-1 second') WHERE id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}
	reclaimed, ok, err := repo.Claim(t.Context(), "worker-b", time.Minute)
	if err != nil || !ok || reclaimed.Attempts != 2 {
		t.Fatalf("reclaim = %#v, %v, %v", reclaimed, ok, err)
	}

	for _, eventType := range []string{"release.validating", "release.ready"} {
		if _, err := repo.AppendEvent(t.Context(), "release", "release-1", eventType, []byte(`{"status":"ok"}`)); err != nil {
			t.Fatal(err)
		}
	}
	events, err := repo.ListEvents(t.Context(), "release", "release-1", 0, 50)
	if err != nil || len(events) != 2 || events[0].ID != 1 || events[1].ID != 2 {
		t.Fatalf("events = %#v, err=%v", events, err)
	}

	if err := repo.Complete(t.Context(), reclaimed.ID, "worker-b"); err != nil {
		t.Fatal(err)
	}
	finished, err := repo.Get(t.Context(), reclaimed.ID)
	if err != nil || finished.Status != asyncjob.StatusSucceeded || finished.FinishedAt == "" {
		t.Fatalf("finished = %#v, err=%v", finished, err)
	}
}

func TestRepositoryRejectsIdempotentJobIDWithDifferentPayload(t *testing.T) {
	store, err := platform.Open(context.Background(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewRepository(store.SQLDB())
	input := asyncjob.EnqueueInput{ID: "job-1", Kind: "release.finalize", ResourceKind: "release", ResourceID: "release-1", Payload: []byte(`{"a":1}`)}
	if _, err := repo.Enqueue(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	input.Payload = []byte(`{"a":2}`)
	if _, err := repo.Enqueue(t.Context(), input); err == nil {
		t.Fatal("different payload reused the same durable job ID")
	}
}

func TestRepositoryAppendsConcurrentEventsWithContiguousResourceSequence(t *testing.T) {
	store, err := platform.Open(t.Context(), filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repo := NewRepository(store.SQLDB())

	const count = 20
	errors := make(chan error, count)
	var group sync.WaitGroup
	for index := 0; index < count; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, appendErr := repo.AppendEvent(t.Context(), "deployment", "deploy-1", "deployment.progress", []byte(`{"status":"running"}`))
			errors <- appendErr
		}()
	}
	group.Wait()
	close(errors)
	for appendErr := range errors {
		if appendErr != nil {
			t.Fatalf("append concurrent event: %v", appendErr)
		}
	}
	events, err := repo.ListEvents(t.Context(), "deployment", "deploy-1", 0, count)
	if err != nil || len(events) != count {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	for index, event := range events {
		if event.ID != int64(index+1) {
			t.Fatalf("event %d ID=%d", index, event.ID)
		}
	}
}
