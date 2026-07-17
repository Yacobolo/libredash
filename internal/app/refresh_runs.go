package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	materializehttp "github.com/Yacobolo/libredash/internal/analytics/materialize/http"
	materializesqlite "github.com/Yacobolo/libredash/internal/analytics/materialize/sqlite"
	"github.com/Yacobolo/libredash/internal/execution"
)

func (s *Server) refreshRunHTTP() materializehttp.Handler {
	return materializehttp.Handler{
		Repository: func() (materialize.RunRepository, error) {
			return s.refreshRunRepository()
		},
		RunnerConfigured: func() bool {
			return s.metrics != nil
		},
		DispatchQueued: func() {
			s.dispatchQueuedRefreshJobs(context.Background())
		},
		CurrentPrincipal: func(r *http.Request) (materializehttp.Principal, bool) {
			principal, ok := currentPrincipal(s, r)
			return materializehttp.Principal{ID: principal.ID}, ok
		},
		WorkspaceID: s.workspaceID,
		RunCreated: func(ctx context.Context, run materialize.RunRecord) error {
			return s.appendAsyncEvent(ctx, "refresh", run.ID, "refresh.queued", run)
		},
	}
}

func (s *Server) refreshRunRepository() (*materializesqlite.SQLRunRepository, error) {
	if s.store == nil {
		return nil, fmt.Errorf("platform store is required")
	}
	return materializesqlite.NewSQLRunRepository(s.store.SQLDB()), nil
}

func (s *Server) executionService() *execution.Service {
	if s.executor == nil {
		s.executor = execution.New(execution.DefaultConfig())
	}
	return s.executor
}
