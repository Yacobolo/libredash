package refresh

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Yacobolo/leapview/internal/analytics/materialize"
	"github.com/Yacobolo/leapview/internal/workload"
)

type QueueRepository interface {
	RunRepository
	ListExecutableJobs(ctx context.Context, environment string, limit int) ([]materialize.JobRecord, error)
	ClaimExecutableJob(ctx context.Context, candidate materialize.JobRecord, owner string, lease time.Duration) (materialize.JobRecord, bool, error)
	RenewJobLease(ctx context.Context, jobID, owner string, lease time.Duration) error
	JobQueueStats(ctx context.Context, environment string) (materialize.JobQueueStats, error)
}

type Dispatcher struct {
	Runs          QueueRepository
	Service       Service
	Admitter      workload.Admitter
	LeaseTimeout  time.Duration
	Logger        *slog.Logger
	Owner         string
	Environment   string
	WorkloadStats func() workload.Stats
	RunFinished   func(context.Context, materialize.JobRecord)
}

func (d Dispatcher) Run(ctx context.Context) {
	if d.Runs == nil {
		return
	}
	if d.Admitter == nil {
		d.Admitter, _ = workload.New(workload.DefaultConfig())
	}
	owner := d.Owner
	if owner == "" {
		owner = fmt.Sprintf("leapview-%d", time.Now().UnixNano())
	}
	for {
		queueStats, _ := d.Runs.JobQueueStats(ctx, d.Environment)
		candidates, err := d.Runs.ListExecutableJobs(ctx, d.Environment, 16)
		if err != nil {
			if d.Logger != nil {
				d.Logger.WarnContext(ctx, "list refresh job candidates failed", "error", err)
			}
			return
		}
		if len(candidates) == 0 {
			return
		}
		finished := make(chan bool, len(candidates))
		for _, candidate := range candidates {
			candidate := candidate
			go func() { finished <- d.dispatchCandidate(ctx, owner, candidate, queueStats) }()
		}
		claimed := false
		for range candidates {
			claimed = <-finished || claimed
		}
		if !claimed {
			return
		}
	}
}

func (d Dispatcher) dispatchCandidate(ctx context.Context, owner string, candidate materialize.JobRecord, queueStats materialize.JobQueueStats) bool {
	lease, err := d.admitter().Acquire(ctx, workload.Request{Class: workload.Refresh, WorkspaceID: candidate.WorkspaceID, Operation: "materialization.refresh"})
	if err != nil {
		if d.Logger != nil {
			d.Logger.InfoContext(ctx, "refresh admission deferred", "workspace", candidate.WorkspaceID, "run", candidate.RunID, "error", err)
		}
		return false
	}
	defer lease.Release()
	job, ok, err := d.Runs.ClaimExecutableJob(lease.Context(), candidate, owner, d.leaseTimeout())
	if err != nil {
		if d.Logger != nil {
			d.Logger.WarnContext(ctx, "claim refresh job failed", "job", candidate.ID, "error", err)
		}
		return false
	}
	if !ok {
		return false
	}
	if d.Logger != nil {
		stats := workload.Stats{}
		if d.WorkloadStats != nil {
			stats = d.WorkloadStats()
		}
		d.Logger.InfoContext(ctx, "dispatch refresh job",
			"workspace", job.WorkspaceID, "run", job.RunID, "kind", job.Kind,
			"queued_jobs", queueStats.QueuedJobs, "running_jobs", queueStats.RunningJobs,
			"stale_leased_jobs", queueStats.StaleLeasedJobs,
			"workload_running", stats.Running, "workload_queued", stats.Queued,
		)
	}
	stopRenew := d.renewJobLease(lease.Context(), job.ID, owner)
	err = d.executeClaimedJob(lease.Context(), job)
	stopRenew()
	if err != nil {
		_, _ = d.Runs.MarkRunFailed(context.Background(), job.WorkspaceID, job.RunID, err.Error())
	}
	lease.Release()
	d.notifyRunFinished(job)
	return true
}

func (d Dispatcher) notifyRunFinished(job materialize.JobRecord) {
	if d.RunFinished != nil {
		d.RunFinished(context.Background(), job)
	}
}

func (d Dispatcher) executeClaimedJob(ctx context.Context, job materialize.JobRecord) error {
	switch job.Kind {
	case materialize.JobKindRefreshPipeline:
		return d.Service.ExecuteClaimedJob(ctx, job)
	default:
		err := fmt.Errorf("unsupported refresh job kind %q", job.Kind)
		_, _ = d.Runs.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, err.Error())
		return err
	}
}

func (d Dispatcher) renewJobLease(ctx context.Context, jobID, owner string) func() {
	interval := d.leaseTimeout() / 2
	if interval <= 0 {
		interval = time.Second
	}
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				if err := d.Runs.RenewJobLease(context.Background(), jobID, owner, d.leaseTimeout()); err != nil && d.Logger != nil {
					d.Logger.WarnContext(ctx, "renew refresh job lease failed", "job", jobID, "error", err)
				}
			}
		}
	}()
	return func() {
		close(done)
	}
}

func (d Dispatcher) admitter() workload.Admitter {
	if d.Admitter != nil {
		return d.Admitter
	}
	controller, _ := workload.New(workload.DefaultConfig())
	return controller
}

func (d Dispatcher) leaseTimeout() time.Duration {
	if d.LeaseTimeout > 0 {
		return d.LeaseTimeout
	}
	return 2 * time.Minute
}
