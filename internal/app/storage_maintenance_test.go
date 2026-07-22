package app

import (
	"context"
	"testing"
	"time"

	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	"github.com/Yacobolo/leapview/internal/workload"
)

func TestStorageRetentionSkipsWhenMaintenanceCapacityIsUnavailable(t *testing.T) {
	controller, err := workload.New(workload.Config{MaxRunning: 1, Classes: map[workload.Class]workload.Policy{
		workload.Interactive: {MaximumRunning: 1},
		workload.Maintenance: {MaximumRunning: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	held, err := controller.Acquire(context.Background(), workload.Request{Class: workload.Interactive, WorkspaceID: "sales", Operation: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	repo := &retentionProbe{}
	server := &Server{workloads: controller, servingStateRepo: repo, duckLakeCatalogPath: "unused", duckLakeDataPath: "unused"}
	if err := server.reconcileStorageRetention(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if repo.called {
		t.Fatal("storage retention ran while maintenance capacity was unavailable")
	}
	if stats := controller.Stats(); stats.Queued != 0 {
		t.Fatalf("maintenance queued instead of skipping: %#v", stats)
	}
}

type retentionProbe struct {
	servingStateRepository
	called bool
}

func (r *retentionProbe) ReconcileRetention(context.Context, servingstate.Environment, time.Time) error {
	r.called = true
	return nil
}

func (r *retentionProbe) ReferencedDuckLakeSnapshots(context.Context, string) ([]int64, error) {
	r.called = true
	return nil, nil
}
