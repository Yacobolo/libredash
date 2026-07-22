package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/analytics/materialize"
	materializehttp "github.com/Yacobolo/leapview/internal/analytics/materialize/http"
	materializesqlite "github.com/Yacobolo/leapview/internal/analytics/materialize/sqlite"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	"github.com/Yacobolo/leapview/internal/workload"
	"github.com/Yacobolo/leapview/internal/workspace/refresh"
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
		Environment: func(*http.Request) string { return string(s.defaultServingEnvironment()) },
		RunCreated: func(ctx context.Context, run materialize.RunRecord) error {
			response, ok := materializehttp.PipelineRunResponseFor(run)
			if !ok {
				return fmt.Errorf("refresh service returned a non-pipeline run")
			}
			return s.appendAsyncEvent(ctx, "refresh", run.ID, "refresh.queued", response)
		},
		AuthorizePipelineView: s.authorizeRefreshPipelineVisibility,
		AuthorizePipelineRun:  s.authorizeRefreshPipelineExecution,
		QueuePipeline: func(ctx context.Context, workspaceID, environment, pipelineID, principalID, retryOf string) (materialize.RunRecord, error) {
			repo, err := s.refreshRunRepository()
			if err != nil {
				return materialize.RunRecord{}, err
			}
			service, err := s.workspaceRefreshService(repo)
			if err != nil {
				return materialize.RunRecord{}, err
			}
			trigger := materialize.TriggerManual
			if retryOf != "" {
				trigger = materialize.TriggerRetry
			}
			result, err := service.QueuePipelineRefresh(ctx, refresh.QueuePipelineInput{
				WorkspaceID: workspaceID, Environment: servingstate.Environment(environment), PrincipalID: principalID,
				PipelineID: pipelineID, TriggerType: trigger, RetryOf: retryOf,
			})
			return result.Run, err
		},
	}
}

func (s *Server) authorizeRefreshPipelineVisibility(r *http.Request, workspaceID, pipelineID string) (bool, error) {
	return s.authorizeRefreshPipelinePrivilege(r, workspaceID, pipelineID, access.PrivilegeViewItem)
}

func (s *Server) authorizeRefreshPipelineExecution(r *http.Request, workspaceID, pipelineID string) (bool, error) {
	return s.authorizeRefreshPipelinePrivilege(r, workspaceID, pipelineID, access.PrivilegeRefreshData)
}

func (s *Server) authorizeRefreshPipelinePrivilege(r *http.Request, workspaceID, pipelineID string, privilege access.Privilege) (bool, error) {
	principal, ok := currentPrincipal(s, r)
	if !ok {
		return false, nil
	}
	if principal.DevBypass {
		return true, nil
	}
	if credential, ok := apiCredentialFromContext(r.Context()); ok && !apiTokenAllows(credential.Token, workspaceID, privilege) {
		return false, nil
	}
	modelID, found, err := s.refreshPipelineSemanticModel(r.Context(), workspaceID, strings.TrimSpace(pipelineID))
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	repo, err := s.accessRepository()
	if err != nil {
		return false, err
	}
	if repo == nil {
		return true, nil
	}
	object := access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, modelID, access.WorkspaceObject(workspaceID))
	decision, err := repo.Authorize(r.Context(), principal.ID, privilege, object)
	return decision.Allowed, err
}

func (s *Server) refreshRunRepository() (*materializesqlite.SQLRunRepository, error) {
	if s.store == nil {
		return nil, fmt.Errorf("platform store is required")
	}
	return materializesqlite.NewSQLRunRepository(s.store.SQLDB()), nil
}

func (s *Server) workloadController() *workload.Controller {
	if s.workloads == nil {
		s.workloads, _ = workload.New(workload.DefaultConfig())
	}
	return s.workloads
}
