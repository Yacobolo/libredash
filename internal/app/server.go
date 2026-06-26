package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	dashboardhttp "github.com/Yacobolo/libredash/internal/dashboard/http"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	dashboardstream "github.com/Yacobolo/libredash/internal/dashboard/stream"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
	"github.com/gorilla/csrf"
)

type queryMetrics interface {
	Catalog() dashboard.Catalog
	DefaultDashboardID() string
	ModelIDForDashboard(dashboardID string) string
	Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool)
	SemanticModel(modelID string) (*semanticmodel.Model, bool)
	DefaultFilters(dashboardID string) dashboard.Filters
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error)
	PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error)
	RefreshMaterializations(ctx context.Context, modelID string) error
	DataDir() string
	Pages(dashboardID string) []dashboard.Page
}

type Server struct {
	metrics            queryMetrics
	broker             *dashboardstream.Broker
	store              *platform.Store
	deploymentRepo     deploymentRepository
	workspaceRepo      workspace.Repository
	accessRepo         access.Repository
	agent              *agentapp.Service
	auth               *Auth
	reloader           runtimeReloader
	artifactDir        string
	defaultWorkspaceID string
	rateLimits         RateLimitConfig
	securityHeaders    SecurityHeadersConfig
	requestLogging     bool
	logger             *slog.Logger
	chatTitleMu        sync.Mutex
	pendingChatTitles  map[string]struct{}
}

func New(metrics queryMetrics) *Server {
	return &Server{metrics: metrics, broker: dashboardstream.NewBroker(), logger: slog.Default(), pendingChatTitles: map[string]struct{}{}}
}

type Options struct {
	Store              *platform.Store
	DeploymentRepo     deploymentRepository
	WorkspaceRepo      workspace.Repository
	AccessRepo         access.Repository
	Agent              *agentapp.Service
	Auth               *Auth
	Reloader           runtimeReloader
	ArtifactDir        string
	DefaultWorkspaceID string
	RateLimits         RateLimitConfig
	SecurityHeaders    SecurityHeadersConfig
	RequestLogging     bool
	Logger             *slog.Logger
}

func NewWithOptions(metrics queryMetrics, options Options) *Server {
	server := New(metrics)
	server.store = options.Store
	server.deploymentRepo = options.DeploymentRepo
	server.workspaceRepo = options.WorkspaceRepo
	server.accessRepo = options.AccessRepo
	server.agent = options.Agent
	server.auth = options.Auth
	server.reloader = options.Reloader
	server.artifactDir = options.ArtifactDir
	server.defaultWorkspaceID = options.DefaultWorkspaceID
	server.rateLimits = options.RateLimits
	server.securityHeaders = options.SecurityHeaders
	server.requestLogging = options.RequestLogging
	if options.Logger != nil {
		server.logger = options.Logger
	}
	server.configureAgentTools()
	return server
}

func (s *Server) workspaceRepository() (workspace.Repository, error) {
	if s.workspaceRepo != nil {
		return s.workspaceRepo, nil
	}
	if s.store == nil {
		return nil, nil
	}
	s.workspaceRepo = workspacesqlite.NewRepository(s.store.SQLDB())
	return s.workspaceRepo, nil
}

func (s *Server) accessRepository() (access.Repository, error) {
	if s.accessRepo != nil {
		return s.accessRepo, nil
	}
	if s.store == nil {
		return nil, nil
	}
	s.accessRepo = accesssqlite.NewRepository(s.store.SQLDB())
	return s.accessRepo, nil
}

func principalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

func localDeveloperPrincipal() Principal {
	return Principal{ID: "dev", Email: "dev@localhost", DisplayName: "Local Developer", DevBypass: true}
}

func SeedLocalDeveloperPlatformAdmin(ctx context.Context, repo access.Repository) error {
	if repo == nil {
		return nil
	}
	principal := localDeveloperPrincipal()
	_, err := repo.SetPlatformRole(ctx, access.PlatformRoleInput{
		PrincipalID: principal.ID,
		Email:       principal.Email,
		DisplayName: principal.DisplayName,
		Role:        access.RoleAdmin,
	})
	return err
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.CatalogPage(s.metrics.Catalog()).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.LoginPage().Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) dashboardHTTP() dashboardhttp.Handler {
	var metrics dashboardhttp.Metrics = s.metrics
	if s.store != nil {
		metrics = dashboardCommandMetrics{
			queryMetrics: s.metrics,
			refresh:      s.refreshMaterializationsWithRun,
		}
	}
	return dashboardhttp.Handler{
		Metrics: metrics,
		Broker:  s.broker,
		CSRFToken: func(r *http.Request) string {
			if s.auth == nil {
				return ""
			}
			return csrf.Token(r)
		},
	}
}

type dashboardCommandMetrics struct {
	queryMetrics
	refresh func(context.Context, string) error
}

func (m dashboardCommandMetrics) RefreshMaterializations(ctx context.Context, modelID string) error {
	if m.refresh != nil {
		return m.refresh(ctx, modelID)
	}
	return m.queryMetrics.RefreshMaterializations(ctx, modelID)
}

func (s *Server) refreshMaterializationsWithRun(ctx context.Context, modelID string) error {
	return s.refreshMaterializationsWithRunForWorkspace(ctx, s.workspaceID(""), modelID)
}

func (s *Server) refreshMaterializationsWithRunForWorkspace(ctx context.Context, workspaceID, modelID string) error {
	if s.store == nil {
		return s.metrics.RefreshMaterializations(ctx, modelID)
	}
	repo := materialize.NewSQLRunRepository(s.store.SQLDB())
	principal, _ := principalFromContext(ctx)
	run, err := repo.CreateRun(ctx, materialize.RunInput{
		WorkspaceID: workspaceID,
		ModelID:     modelID,
		PrincipalID: principal.ID,
		TargetType:  materialize.TargetSemanticModel,
		TargetID:    modelID,
		TriggerType: materialize.TriggerDirect,
	})
	if err != nil {
		return err
	}
	s.publishModelRefreshPatches(ctx, workspaceID, modelID)
	if _, err := repo.MarkRunRunning(ctx, workspaceID, run.ID); err != nil {
		return err
	}
	s.publishModelRefreshPatches(ctx, workspaceID, modelID)
	model, ok := s.metrics.SemanticModel(modelID)
	if !ok {
		err := fmt.Errorf("unknown semantic model %q", modelID)
		if _, finishErr := repo.MarkRunFailed(ctx, workspaceID, run.ID, err.Error()); finishErr != nil {
			return finishErr
		}
		s.publishModelRefreshPatches(ctx, workspaceID, modelID)
		return err
	}
	order, err := materialize.ModelTableOrder(model)
	if err != nil {
		if _, finishErr := repo.MarkRunFailed(ctx, workspaceID, run.ID, err.Error()); finishErr != nil {
			return finishErr
		}
		s.publishModelRefreshPatches(ctx, workspaceID, modelID)
		return err
	}
	for _, tableName := range order {
		targetID := modelID + "." + tableName
		tableRun, err := repo.CreateRun(ctx, materialize.RunInput{
			WorkspaceID: workspaceID,
			ModelID:     modelID,
			PrincipalID: principal.ID,
			TargetType:  materialize.TargetModelTable,
			TargetID:    targetID,
			TriggerType: materialize.TriggerSemanticModel,
			ParentRunID: run.ID,
		})
		if err != nil {
			return err
		}
		s.publishModelRefreshPatches(ctx, workspaceID, modelID)
		if _, err := repo.MarkRunRunning(ctx, workspaceID, tableRun.ID); err != nil {
			return err
		}
		s.publishModelRefreshPatches(ctx, workspaceID, modelID)
		if err := s.refreshModelTables(ctx, modelID, []string{tableName}); err != nil {
			if _, finishErr := repo.MarkRunFailed(ctx, workspaceID, tableRun.ID, err.Error()); finishErr != nil {
				return finishErr
			}
			if _, finishErr := repo.MarkRunFailed(ctx, workspaceID, run.ID, err.Error()); finishErr != nil {
				return finishErr
			}
			s.publishModelRefreshPatches(ctx, workspaceID, modelID)
			return err
		}
		if _, err := repo.MarkRunSucceeded(ctx, workspaceID, tableRun.ID); err != nil {
			return err
		}
		s.publishModelRefreshPatches(ctx, workspaceID, modelID)
	}
	if _, err := repo.MarkRunSucceeded(ctx, workspaceID, run.ID); err != nil {
		return err
	}
	s.publishModelRefreshPatches(ctx, workspaceID, modelID)
	return nil
}
