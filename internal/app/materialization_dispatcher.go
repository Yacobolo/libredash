package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentfs "github.com/Yacobolo/libredash/internal/deployment/filesystem"
	"github.com/Yacobolo/libredash/internal/execution"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func (s *Server) dispatchQueuedMaterializationJobs(ctx context.Context) {
	if s == nil || s.store == nil || s.metrics == nil {
		return
	}
	s.jobDispatchMu.Lock()
	if s.jobDispatching {
		s.jobDispatchMu.Unlock()
		return
	}
	s.jobDispatching = true
	s.jobDispatchMu.Unlock()
	go s.runMaterializationDispatcher(ctx)
}

func (s *Server) runMaterializationDispatcher(ctx context.Context) {
	defer func() {
		s.jobDispatchMu.Lock()
		s.jobDispatching = false
		s.jobDispatchMu.Unlock()
	}()
	repo := materialize.NewSQLRunRepository(s.store.SQLDB())
	owner := fmt.Sprintf("libredash-%d", time.Now().UnixNano())
	for {
		queueStats, _ := repo.JobQueueStats(ctx)
		executionStats := s.executionService().Stats()
		job, ok, err := repo.ClaimNextExecutableJob(ctx, owner, s.jobLeaseTimeout)
		if err != nil {
			if s.logger != nil {
				s.logger.WarnContext(ctx, "claim materialization job failed", "error", err)
			}
			return
		}
		if !ok {
			return
		}
		if s.logger != nil {
			s.logger.InfoContext(ctx, "dispatch materialization job",
				"workspace", job.WorkspaceID,
				"run", job.RunID,
				"kind", job.Kind,
				"queued_jobs", queueStats.QueuedJobs,
				"running_jobs", queueStats.RunningJobs,
				"stale_leased_jobs", queueStats.StaleLeasedJobs,
				"running_reads", executionStats.RunningReads,
				"queued_reads", executionStats.QueuedReads,
				"running_writes", executionStats.RunningJobs,
			)
		}
		err = s.executionService().SubmitJob(ctx, execution.JobRef{WorkspaceID: job.WorkspaceID, RunID: job.RunID, Kind: job.Kind}, func(ctx context.Context) error {
			stopRenew := s.renewMaterializationJobLease(ctx, repo, job.ID, owner)
			defer stopRenew()
			return s.executeClaimedMaterializationJob(ctx, repo, job)
		})
		if err != nil {
			_, _ = repo.MarkRunFailed(context.Background(), job.WorkspaceID, job.RunID, err.Error())
			return
		}
	}
}

func (s *Server) renewMaterializationJobLease(ctx context.Context, repo *materialize.SQLRunRepository, jobID, owner string) func() {
	interval := s.jobLeaseTimeout / 2
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
				if err := repo.RenewJobLease(context.Background(), jobID, owner, s.jobLeaseTimeout); err != nil && s.logger != nil {
					s.logger.WarnContext(ctx, "renew materialization job lease failed", "job", jobID, "error", err)
				}
			}
		}
	}()
	return func() {
		close(done)
	}
}

func (s *Server) executeClaimedMaterializationJob(ctx context.Context, repo *materialize.SQLRunRepository, job materialize.JobRecord) error {
	switch job.Kind {
	case materialize.JobKindMaterialization:
		orchestrator := NewGenericRefreshOrchestrator(repo, s.metrics)
		_, err := orchestrator.ExecuteRun(ctx, job.WorkspaceID, job.RunID, refreshPublisher{})
		if err != nil && s.logger != nil {
			s.logger.WarnContext(ctx, "materialization job failed", "workspace", job.WorkspaceID, "run", job.RunID, "error", err)
		}
		return err
	case materialize.JobKindWorkspaceAssetRefresh:
		return s.executeClaimedWorkspaceAssetRefresh(ctx, repo, job)
	default:
		err := fmt.Errorf("unsupported materialization job kind %q", job.Kind)
		_, _ = repo.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, err.Error())
		return err
	}
}

func (s *Server) executeClaimedWorkspaceAssetRefresh(ctx context.Context, repo *materialize.SQLRunRepository, job materialize.JobRecord) error {
	deploymentRepo, err := s.deploymentRepository()
	if err != nil {
		return err
	}
	if deploymentRepo == nil {
		return fmt.Errorf("deployment repository is required")
	}
	if job.DeploymentID == "" {
		return fmt.Errorf("workspace refresh job deployment id is required")
	}
	candidateDeployment, err := deploymentRepo.ByID(ctx, deployment.ID(job.DeploymentID))
	if err != nil {
		return err
	}
	if candidateDeployment.Status == deployment.StatusActive && candidateDeployment.DuckLakeSnapshotID > 0 {
		_, _ = repo.MarkRunSucceeded(ctx, job.WorkspaceID, job.RunID)
		return nil
	}
	candidateArtifact, err := deploymentRepo.ArtifactByDeployment(ctx, candidateDeployment.ID)
	if err != nil {
		return err
	}
	serving := newServingStateService(deploymentRepo)
	activeState, err := serving.Active(ctx, job.WorkspaceID, candidateDeployment.Environment)
	if err != nil {
		return err
	}
	root, err := os.MkdirTemp("", "libredash-replay-refresh-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if err := deploymentfs.ExtractArtifact(candidateArtifact.Path, root); err != nil {
		return err
	}
	compiled, _, err := deploymentfs.LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		return err
	}
	if compiled.Definition == nil {
		return fmt.Errorf("compiled workspace definition is required")
	}
	asset := workspace.AssetView{
		Type:  workspaceAssetTypeForRefreshTarget(job.TargetType),
		Key:   job.TargetID,
		Title: job.TargetID,
	}
	plan, err := workspaceAssetRefreshPlanForAsset(compiled.Definition, job.WorkspaceID, asset)
	if err != nil {
		return err
	}
	childRuns, err := repo.ListChildRuns(ctx, job.WorkspaceID, job.RunID)
	if err != nil {
		return err
	}
	for _, child := range childRuns {
		_, _ = repo.MarkRunRunning(ctx, job.WorkspaceID, child.ID)
		s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, child.TargetType, child.TargetID)
	}
	s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
	candidate := servingState{Deployment: candidateDeployment, Artifact: candidateArtifact}
	snapshotID, err := s.executeWorkspaceAssetRefreshPlan(ctx, compiled.Definition, activeState.Deployment, candidate.Deployment, candidate.Artifact, candidateDeployment.Environment, plan)
	if err != nil {
		_, _ = repo.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, err.Error())
		for _, child := range childRuns {
			_, _ = repo.MarkRunFailed(ctx, job.WorkspaceID, child.ID, err.Error())
			s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, child.TargetType, child.TargetID)
		}
		_ = serving.MarkFailed(ctx, candidate, err)
		s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
		return err
	}
	if err := serving.RecordSnapshot(ctx, candidate, snapshotID); err != nil {
		_, _ = repo.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, err.Error())
		_ = serving.MarkFailed(ctx, candidate, err)
		s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
		return err
	}
	var prepared deployment.PreparedRuntime
	if s.reloader != nil {
		prepared, err = s.reloader.PrepareDeployment(ctx, string(candidateDeployment.ID))
		if err != nil {
			_, _ = repo.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, err.Error())
			_ = serving.MarkFailed(ctx, candidate, err)
			s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
			return err
		}
	}
	if _, err := serving.Activate(ctx, candidate); err != nil {
		if prepared != nil {
			_ = prepared.Close()
		}
		_, _ = repo.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, err.Error())
		_ = serving.MarkFailed(ctx, candidate, err)
		s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
		return err
	}
	if prepared != nil {
		if err := s.reloader.CommitPrepared(prepared); err != nil {
			_ = prepared.Close()
			_, _ = repo.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, err.Error())
			s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
			return err
		}
	} else if s.reloader != nil {
		_ = s.reloader.Reload(ctx)
	}
	if err := s.reconcileStorageRetention(ctx, false); err != nil && s.logger != nil {
		s.logger.WarnContext(ctx, "storage retention reconciliation failed", "workspace", job.WorkspaceID, "environment", candidateDeployment.Environment, "error", err)
	}
	for _, child := range childRuns {
		_, _ = repo.MarkRunSucceeded(ctx, job.WorkspaceID, child.ID)
		s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, child.TargetType, child.TargetID)
	}
	_, err = repo.MarkRunSucceeded(ctx, job.WorkspaceID, job.RunID)
	s.publishWorkspaceAssetRefreshPatchesForTarget(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
	return err
}

func workspaceAssetTypeForRefreshTarget(targetType string) string {
	switch targetType {
	case materialize.TargetModelTable:
		return string(workspace.AssetTypeModelTable)
	case materialize.TargetSemanticModel:
		return string(workspace.AssetTypeSemanticModel)
	default:
		return targetType
	}
}
