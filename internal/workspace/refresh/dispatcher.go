package refresh

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/execution"
)

type QueueRepository interface {
	RunRepository
	ClaimNextExecutableJob(ctx context.Context, owner string, lease time.Duration) (materialize.JobRecord, bool, error)
	RenewJobLease(ctx context.Context, jobID, owner string, lease time.Duration) error
	JobQueueStats(ctx context.Context) (materialize.JobQueueStats, error)
}

type LegacyExecutor interface {
	ExecuteLegacyJob(ctx context.Context, job materialize.JobRecord) error
}

type Dispatcher struct {
	Runs           QueueRepository
	Service        Service
	Executor       *execution.Service
	Legacy         LegacyExecutor
	LeaseTimeout   time.Duration
	Logger         *slog.Logger
	Owner          string
	ExecutionStats func() execution.Stats
}

func (d Dispatcher) Run(ctx context.Context) {
	if d.Runs == nil {
		return
	}
	owner := d.Owner
	if owner == "" {
		owner = fmt.Sprintf("libredash-%d", time.Now().UnixNano())
	}
	for {
		queueStats, _ := d.Runs.JobQueueStats(ctx)
		job, ok, err := d.Runs.ClaimNextExecutableJob(ctx, owner, d.leaseTimeout())
		if err != nil {
			if d.Logger != nil {
				d.Logger.WarnContext(ctx, "claim refresh job failed", "error", err)
			}
			return
		}
		if !ok {
			return
		}
		if d.Logger != nil {
			stats := execution.Stats{}
			if d.ExecutionStats != nil {
				stats = d.ExecutionStats()
			}
			d.Logger.InfoContext(ctx, "dispatch refresh job",
				"workspace", job.WorkspaceID,
				"run", job.RunID,
				"kind", job.Kind,
				"queued_jobs", queueStats.QueuedJobs,
				"running_jobs", queueStats.RunningJobs,
				"stale_leased_jobs", queueStats.StaleLeasedJobs,
				"running_reads", stats.RunningReads,
				"queued_reads", stats.QueuedReads,
				"running_writes", stats.RunningJobs,
			)
		}
		err = d.executionService().SubmitJob(ctx, execution.JobRef{WorkspaceID: job.WorkspaceID, RunID: job.RunID, Kind: job.Kind}, func(ctx context.Context) error {
			stopRenew := d.renewJobLease(ctx, job.ID, owner)
			defer stopRenew()
			return d.executeClaimedJob(ctx, job)
		})
		if err != nil {
			_, _ = d.Runs.MarkRunFailed(context.Background(), job.WorkspaceID, job.RunID, err.Error())
			return
		}
	}
}

func (d Dispatcher) executeClaimedJob(ctx context.Context, job materialize.JobRecord) error {
	switch job.Kind {
	case materialize.JobKindRefresh:
		if d.Legacy == nil {
			err := fmt.Errorf("legacy refresh executor is required")
			_, _ = d.Runs.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, err.Error())
			return err
		}
		return d.Legacy.ExecuteLegacyJob(ctx, job)
	case materialize.JobKindWorkspaceAssetRefresh:
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

func (d Dispatcher) executionService() *execution.Service {
	if d.Executor != nil {
		return d.Executor
	}
	return execution.New(execution.DefaultConfig())
}

func (d Dispatcher) leaseTimeout() time.Duration {
	if d.LeaseTimeout > 0 {
		return d.LeaseTimeout
	}
	return 2 * time.Minute
}
