package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/execution"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
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
		Legacy:       appLegacyRefreshExecutor{repo: repo, metrics: s.metrics, logger: s.logger},
		LeaseTimeout: s.jobLeaseTimeout,
		Logger:       s.logger,
		Owner:        fmt.Sprintf("libredash-%d", time.Now().UnixNano()),
		ExecutionStats: func() execution.Stats {
			return s.executionService().Stats()
		},
	}
	dispatcher.Run(ctx)
}
