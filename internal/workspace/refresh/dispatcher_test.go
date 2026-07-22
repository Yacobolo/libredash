package refresh

import (
	"context"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/analytics/materialize"
	"github.com/Yacobolo/leapview/internal/execution"
)

func TestDispatcherMarksUnsupportedJobFailed(t *testing.T) {
	ctx := context.Background()
	queue := &fakeQueueRepository{jobs: []materialize.JobRecord{{
		ID:          "job_1",
		WorkspaceID: "sales",
		RunID:       "run_1",
		Kind:        "unknown",
	}}}

	Dispatcher{
		Runs:         queue,
		Executor:     execution.New(execution.Config{MaxRunningJobs: 1, MaxQueuedJobs: 1}),
		Owner:        "test-owner",
		LeaseTimeout: time.Minute,
	}.Run(ctx)

	if queue.failedRun != "run_1" {
		t.Fatalf("failed run = %q, want run_1", queue.failedRun)
	}
	if queue.failedMessage == "" {
		t.Fatal("failed message is empty")
	}
}

type fakeQueueRepository struct {
	jobs          []materialize.JobRecord
	claimOwner    string
	renewedJob    string
	failedRun     string
	failedMessage string
}

func (r *fakeQueueRepository) ClaimNextExecutableJob(_ context.Context, _, owner string, _ time.Duration) (materialize.JobRecord, bool, error) {
	r.claimOwner = owner
	if len(r.jobs) == 0 {
		return materialize.JobRecord{}, false, nil
	}
	job := r.jobs[0]
	r.jobs = r.jobs[1:]
	return job, true, nil
}

func (r *fakeQueueRepository) RenewJobLease(context.Context, string, string, time.Duration) error {
	return nil
}

func (r *fakeQueueRepository) JobQueueStats(context.Context, string) (materialize.JobQueueStats, error) {
	return materialize.JobQueueStats{}, nil
}

func (r *fakeQueueRepository) CreateRun(context.Context, materialize.RunInput) (materialize.RunRecord, error) {
	return materialize.RunRecord{}, nil
}

func (r *fakeQueueRepository) ListChildRuns(context.Context, string, string) ([]materialize.RunRecord, error) {
	return nil, nil
}

func (r *fakeQueueRepository) MarkRunRunning(context.Context, string, string) (materialize.RunRecord, error) {
	return materialize.RunRecord{}, nil
}

func (r *fakeQueueRepository) MarkRunSucceeded(context.Context, string, string) (materialize.RunRecord, error) {
	return materialize.RunRecord{}, nil
}

func (r *fakeQueueRepository) MarkRunFailed(_ context.Context, _ string, runID, message string) (materialize.RunRecord, error) {
	r.failedRun = runID
	r.failedMessage = message
	return materialize.RunRecord{ID: runID, Status: materialize.RunStatusFailed, Error: message}, nil
}
