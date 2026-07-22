package refresh

import (
	"context"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/analytics/materialize"
	"github.com/Yacobolo/leapview/internal/workload"
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
		Runs: queue,
		Admitter: func() workload.Admitter {
			controller, err := workload.New(workload.Config{MaxRunning: 1, MaximumQueued: 1, Classes: map[workload.Class]workload.Policy{workload.Refresh: {MaximumRunning: 1, MaximumQueued: 1, MaximumQueuedPerWorkspace: 1}}})
			if err != nil {
				t.Fatal(err)
			}
			return controller
		}(),
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

func TestDispatcherAdmissionRejectionLeavesDurableJobRetryable(t *testing.T) {
	queue := &fakeQueueRepository{jobs: []materialize.JobRecord{{ID: "job_1", WorkspaceID: "sales", RunID: "run_1", Kind: materialize.JobKindRefreshPipeline}}}
	controller, err := workload.New(workload.Config{MaxRunning: 1, Classes: map[workload.Class]workload.Policy{
		workload.Interactive: {MaximumRunning: 1}, workload.Refresh: {MaximumRunning: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	held, err := controller.Acquire(context.Background(), workload.Request{Class: workload.Interactive, WorkspaceID: "sales", Operation: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	Dispatcher{Runs: queue, Admitter: controller, Owner: "test-owner", LeaseTimeout: time.Minute}.Run(context.Background())
	held.Release()
	if len(queue.jobs) != 1 || queue.claimOwner != "" {
		t.Fatalf("rejected job was claimed: %#v", queue)
	}
	if queue.failedRun != "" {
		t.Fatalf("rejected job was failed: %#v", queue)
	}
}

func TestDispatcherReleasesRefreshPermitBeforeRunFinished(t *testing.T) {
	queue := &fakeQueueRepository{jobs: []materialize.JobRecord{{ID: "job_1", WorkspaceID: "sales", RunID: "run_1", Kind: "unknown"}}}
	controller, err := workload.New(workload.Config{MaxRunning: 1, Classes: map[workload.Class]workload.Policy{
		workload.Refresh: {MaximumRunning: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	runningAtCallback := -1
	Dispatcher{
		Runs: queue, Admitter: controller, Owner: "test-owner", LeaseTimeout: time.Minute,
		RunFinished: func(context.Context, materialize.JobRecord) { runningAtCallback = controller.Stats().Running },
	}.Run(context.Background())
	if runningAtCallback != 0 {
		t.Fatalf("running permits at completion callback = %d, want 0", runningAtCallback)
	}
}

type fakeQueueRepository struct {
	jobs          []materialize.JobRecord
	claimOwner    string
	renewedJob    string
	failedRun     string
	failedMessage string
}

func (r *fakeQueueRepository) ListExecutableJobs(context.Context, string, int) ([]materialize.JobRecord, error) {
	if len(r.jobs) == 0 {
		return nil, nil
	}
	return append([]materialize.JobRecord(nil), r.jobs...), nil
}

func (r *fakeQueueRepository) ClaimExecutableJob(_ context.Context, candidate materialize.JobRecord, owner string, _ time.Duration) (materialize.JobRecord, bool, error) {
	r.claimOwner = owner
	for index, job := range r.jobs {
		if job.ID != candidate.ID {
			continue
		}
		r.jobs = append(r.jobs[:index], r.jobs[index+1:]...)
		return job, true, nil
	}
	return materialize.JobRecord{}, false, nil
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
