package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Yacobolo/leapview/internal/analytics/materialize"
	materializehttp "github.com/Yacobolo/leapview/internal/analytics/materialize/http"
	"github.com/Yacobolo/leapview/internal/workload"
	"github.com/Yacobolo/leapview/internal/workspace/refresh"
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
		Admitter:     s.workloadController(),
		LeaseTimeout: s.jobLeaseTimeout,
		Logger:       s.logger,
		Owner:        fmt.Sprintf("leapview-%d", time.Now().UnixNano()),
		Environment:  string(s.defaultServingEnvironment()),
		WorkloadStats: func() workload.Stats {
			return s.workloadController().Stats()
		},
		RunFinished: func(ctx context.Context, job materialize.JobRecord) {
			run, err := repo.GetRun(ctx, job.WorkspaceID, job.RunID)
			if err != nil {
				return
			}
			if run.Status == materialize.RunStatusSucceeded {
				_ = s.reconcileStorageRetention(ctx, false)
			}
			response, ok := materializehttp.PipelineRunResponseFor(run)
			if !ok {
				return
			}
			eventType := "refresh." + string(run.Status)
			_ = s.appendAsyncEvent(ctx, "refresh", run.ID, eventType, response)
		},
	}
	dispatcher.Run(ctx)
}
