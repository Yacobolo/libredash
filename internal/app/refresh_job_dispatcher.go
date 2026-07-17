package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/execution"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
)

func (s *Server) dispatchQueuedRefreshJobs(ctx context.Context) {
	if s == nil || s.store == nil || s.metrics == nil {
		return
	}
	var ok bool
	ctx, ok = s.refreshDispatchContext(ctx)
	if !ok {
		return
	}
	s.jobDispatchMu.Lock()
	if s.jobDispatching {
		s.jobDispatchMu.Unlock()
		return
	}
	s.jobDispatching = true
	s.jobDispatchWG.Add(1)
	s.jobDispatchMu.Unlock()
	go func() {
		defer s.jobDispatchWG.Done()
		s.runRefreshJobDispatcher(ctx)
	}()
}

func (s *Server) refreshDispatchContext(fallback context.Context) (context.Context, bool) {
	s.backgroundMu.Lock()
	defer s.backgroundMu.Unlock()
	if s.backgroundStopping {
		return nil, false
	}
	if s.backgroundCtx != nil {
		return s.backgroundCtx, true
	}
	if fallback == nil {
		fallback = context.Background()
	}
	return fallback, true
}

func (s *Server) runRefreshJobDispatcher(ctx context.Context) {
	defer func() {
		s.jobDispatchMu.Lock()
		s.jobDispatching = false
		s.jobDispatchMu.Unlock()
	}()
	repo, err := s.refreshRunRepository()
	if err != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "create refresh run repository failed", "error", err)
		}
		return
	}
	service, err := s.workspaceRefreshService(repo)
	if err != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "create refresh dispatcher failed", "error", err)
		}
		return
	}
	dispatcher := refresh.Dispatcher{
		Runs:         repo,
		Service:      service,
		Executor:     s.executionService(),
		Direct:       appDirectRefreshExecutor{repo: repo, metrics: s.metrics, logger: s.logger},
		LeaseTimeout: s.jobLeaseTimeout,
		Logger:       s.logger,
		Owner:        fmt.Sprintf("libredash-%d", time.Now().UnixNano()),
		ExecutionStats: func() execution.Stats {
			return s.executionService().Stats()
		},
		RunFinished: func(ctx context.Context, job materialize.JobRecord) {
			run, err := repo.GetRun(ctx, job.WorkspaceID, job.RunID)
			if err != nil {
				return
			}
			eventType := "refresh." + string(run.Status)
			_ = s.appendAsyncEvent(ctx, "refresh", run.ID, eventType, run)
		},
	}
	dispatcher.Run(ctx)
}
