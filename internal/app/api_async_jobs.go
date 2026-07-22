package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/internal/asyncjob"
	"github.com/Yacobolo/leapview/internal/deployment/apiadapter"
	"github.com/Yacobolo/leapview/internal/manageddata/control"
	"github.com/Yacobolo/leapview/internal/workload"
)

const (
	apiJobReleaseFinalize    = "release.finalize"
	apiJobDeploymentActivate = "deployment.activate"
	apiJobUploadFinalize     = "upload.finalize"
	apiJobAgentRun           = "agent.run"
)

type releaseFinalizeJob struct{ Project, Release string }
type deploymentActivateJob struct{ Project, Deployment, Actor, IdempotencyKey string }
type uploadFinalizeJob struct{ Project, Connection, UploadSession string }
type agentRunJob struct {
	Scope                            agent.Scope
	Conversation, Run, CorrelationID string
}

func (s *Server) asyncRepository() (asyncjob.Repository, error) {
	if s == nil || s.asyncJobs == nil {
		return nil, fmt.Errorf("async job repository is required")
	}
	return s.asyncJobs, nil
}

func (s *Server) enqueueAsyncJob(ctx context.Context, input asyncjob.EnqueueInput) error {
	repo, err := s.asyncRepository()
	if err != nil {
		return err
	}
	if _, err := repo.Enqueue(ctx, input); err != nil {
		return err
	}
	s.dispatchQueuedAsyncJobs(context.Background())
	return nil
}

func (s *Server) enqueueAsyncJobPayload(ctx context.Context, id, kind, resourceKind, resourceID string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	class := workload.Control
	workspaceID := workload.NodeWorkspace
	if agentJob, ok := payload.(agentRunJob); ok {
		class = workload.Background
		workspaceID = agentJob.Scope.WorkspaceID
		if workspaceID == "" {
			workspaceID = agentJob.Scope.Credential.WorkspaceID
		}
		if workspaceID == "" {
			workspaceID = s.defaultWorkspaceID
		}
		if workspaceID == "" {
			workspaceID = workload.GlobalWorkspace
		}
	}
	return s.enqueueAsyncJob(ctx, asyncjob.EnqueueInput{ID: id, Kind: kind, WorkloadClass: string(class), WorkspaceID: workspaceID, ResourceKind: resourceKind, ResourceID: resourceID, Payload: encoded})
}

func (s *Server) appendAsyncEvent(ctx context.Context, resourceKind, resourceID, eventType string, data any) error {
	repo, err := s.asyncRepository()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = repo.AppendEvent(ctx, resourceKind, resourceID, eventType, encoded)
	return err
}

func (s *Server) dispatchQueuedAsyncJobs(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	ctx, ok := s.asyncDispatchContext()
	if !ok {
		return
	}
	s.jobDispatchMu.Lock()
	if s.apiJobDispatching {
		s.jobDispatchMu.Unlock()
		return
	}
	s.apiJobDispatching = true
	s.jobDispatchWG.Add(1)
	s.jobDispatchMu.Unlock()
	go func() {
		defer s.jobDispatchWG.Done()
		defer func() { s.jobDispatchMu.Lock(); s.apiJobDispatching = false; s.jobDispatchMu.Unlock() }()
		s.runAsyncJobDispatcher(ctx)
	}()
}

func (s *Server) asyncDispatchContext() (context.Context, bool) {
	s.backgroundMu.Lock()
	defer s.backgroundMu.Unlock()
	if s.backgroundStopping || s.backgroundCtx == nil {
		return nil, false
	}
	return s.backgroundCtx, true
}

func (s *Server) runAsyncJobDispatcher(ctx context.Context) {
	repo, err := s.asyncRepository()
	if err != nil {
		return
	}
	owner := fmt.Sprintf("leapview-api-%d", time.Now().UnixNano())
	var pumps sync.WaitGroup
	for _, class := range []workload.Class{workload.Control, workload.Background} {
		class := class
		pumps.Add(1)
		go func() { defer pumps.Done(); s.runAsyncJobPump(ctx, repo, owner, class) }()
	}
	pumps.Wait()
}

func (s *Server) runAsyncJobPump(ctx context.Context, repo asyncjob.Repository, owner string, class workload.Class) {
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	for {
		candidates, listErr := repo.Candidates(ctx, string(class), 16)
		if listErr != nil {
			if s.logger != nil {
				s.logger.WarnContext(ctx, "list API async job candidates failed", "class", class, "error", listErr)
			}
		} else if len(candidates) > 0 {
			var batch sync.WaitGroup
			for _, candidate := range candidates {
				candidate := candidate
				batch.Add(1)
				go func() { defer batch.Done(); s.dispatchAsyncCandidate(ctx, repo, owner, class, candidate) }()
			}
			batch.Wait()
		}
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
		}
	}
}

func (s *Server) dispatchAsyncCandidate(ctx context.Context, repo asyncjob.Repository, owner string, class workload.Class, candidate asyncjob.Job) {
	lease, err := s.workloadController().Acquire(ctx, workload.Request{Class: class, WorkspaceID: candidate.WorkspaceID, Operation: candidate.Kind})
	if err != nil {
		if s.logger != nil {
			s.logger.InfoContext(ctx, "async job admission deferred", "class", class, "workspace", candidate.WorkspaceID, "job", candidate.ID, "error", err)
		}
		return
	}
	defer lease.Release()
	job, ok, err := repo.ClaimByID(lease.Context(), candidate.ID, string(class), owner, s.jobLeaseTimeout)
	if err != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "claim API async job failed", "class", class, "job", candidate.ID, "error", err)
		}
		return
	}
	if !ok {
		return
	}
	s.executeClaimedAsyncJob(lease.Context(), repo, owner, job)
}

func (s *Server) executeClaimedAsyncJob(parent context.Context, repo asyncjob.Repository, owner string, job asyncjob.Job) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	done := make(chan struct{})
	go func() {
		interval := s.jobLeaseTimeout / 2
		if interval < time.Second {
			interval = time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := repo.Renew(context.WithoutCancel(ctx), job.ID, owner, s.jobLeaseTimeout); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	err := s.executeAsyncJob(ctx, job)
	close(done)
	if err == nil {
		_ = repo.Complete(context.WithoutCancel(ctx), job.ID, owner)
		return
	}
	if ctx.Err() != nil {
		_ = repo.CancelClaimed(context.WithoutCancel(ctx), job.ID, owner)
		return
	}
	problem, _ := json.Marshal(map[string]any{"code": "ASYNC_JOB_FAILED", "detail": err.Error()})
	_ = repo.Fail(context.WithoutCancel(ctx), job.ID, owner, problem)
	if s.logger != nil {
		s.logger.ErrorContext(ctx, "API async job failed", "kind", job.Kind, "resource", job.ResourceID, "error", err)
	}
}

func (s *Server) cancelQueuedAsyncJob(ctx context.Context, id string) (bool, error) {
	repo, err := s.asyncRepository()
	if err != nil {
		return false, err
	}
	err = repo.Cancel(ctx, id)
	if err == nil {
		return true, nil
	}
	if err == asyncjob.ErrConflict {
		return false, nil
	}
	return false, err
}

func (s *Server) executeAsyncJob(ctx context.Context, job asyncjob.Job) error {
	switch job.Kind {
	case apiJobReleaseFinalize:
		var payload releaseFinalizeJob
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		service, err := s.releaseService()
		if err != nil {
			return err
		}
		row, err := service.ValidateFinalization(ctx, payload.Project, payload.Release)
		event := "release.ready"
		if err != nil {
			event = "release.failed"
		}
		_ = s.appendAsyncEvent(context.WithoutCancel(ctx), "release", payload.Release, event, releaseResponse(row))
		return err
	case apiJobDeploymentActivate:
		var payload deploymentActivateJob
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		pending, err := s.deploymentOptions.Coordinator.Get(ctx, apiadapter.Scope{Project: payload.Project, DeploymentID: payload.Deployment})
		if err != nil {
			return err
		}
		targets := make([]apiadapter.TargetRequest, 0, len(pending.Targets))
		for _, target := range pending.Targets {
			targets = append(targets, apiadapter.TargetRequest{Workspace: target.Workspace, CandidateID: target.CandidateID})
		}
		principal := Principal{ID: payload.Actor, DevBypass: s.auth == nil || s.auth.devBypass}
		if err := s.authorizePublicationDeployment(ctx, principal, pending.Environment, targets); err != nil {
			_ = s.appendAsyncEvent(context.WithoutCancel(ctx), "deployment", payload.Deployment, "deployment.failed", map[string]any{"deploymentId": payload.Deployment, "status": "failed"})
			return err
		}
		row, err := s.deploymentOptions.Coordinator.Activate(ctx, apiadapter.ActivateRequest{Scope: apiadapter.Scope{Project: payload.Project, DeploymentID: payload.Deployment}, Actor: payload.Actor, IdempotencyKey: payload.IdempotencyKey})
		if err == nil && s.refreshPipelineRepo != nil {
			if reconcileErr := s.reconcileRefreshPipelineSchedules(ctx, s.refreshPipelineRepo); reconcileErr != nil {
				s.logger.WarnContext(ctx, "reconcile refresh pipelines after deployment activation failed", "error", reconcileErr)
			}
		}
		event := "deployment.active"
		if err != nil {
			event = "deployment.failed"
		}
		_ = s.appendAsyncEvent(context.WithoutCancel(ctx), "deployment", payload.Deployment, event, map[string]any{"deploymentId": payload.Deployment, "status": row.Status})
		return err
	case apiJobUploadFinalize:
		var payload uploadFinalizeJob
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		result, err := s.managedDataOptions.Uploads.CompleteFinalizeUpload(ctx, control.UploadRequest{Project: payload.Project, Connection: payload.Connection, UploadID: payload.UploadSession})
		event := "upload_session.completed"
		if err != nil {
			event = "upload_session.failed"
		}
		_ = s.appendAsyncEvent(context.WithoutCancel(ctx), "upload", payload.UploadSession, event, map[string]any{"uploadSessionId": payload.UploadSession, "status": result.Upload.Status})
		return err
	case apiJobAgentRun:
		var payload agentRunJob
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		if s.agent == nil {
			return fmt.Errorf("agent service is unavailable")
		}
		started, err := s.agent.ResumePrompt(ctx, payload.Scope, payload.Conversation, payload.Run, payload.CorrelationID)
		if err != nil {
			return err
		}
		_, err = started.Complete(ctx, nil)
		event := "agent_run.completed"
		if err != nil {
			event = "agent_run.failed"
		}
		_ = s.appendAsyncEvent(context.WithoutCancel(ctx), "agent_run", payload.Run, event, map[string]any{"runId": payload.Run, "conversationId": payload.Conversation})
		return err
	default:
		return fmt.Errorf("unsupported API async job kind %q", job.Kind)
	}
}
