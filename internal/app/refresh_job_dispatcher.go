package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Yacobolo/libredash/internal/execution"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
)

func (s *Server) dispatchQueuedRefreshJobs(ctx context.Context) {
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
	go s.runRefreshJobDispatcher(ctx)
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
	}
	dispatcher.Run(ctx)
}
