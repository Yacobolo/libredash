package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/agentapp"
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
	assetCatalog       workspace.AssetCatalogReader
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
	AssetCatalog       workspace.AssetCatalogReader
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
	server.assetCatalog = options.AssetCatalog
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

func (s *Server) upsertAuthenticatedPrincipal(ctx context.Context, principal Principal) error {
	repo, err := s.accessRepository()
	if err != nil || repo == nil {
		return err
	}
	_, err = repo.UpsertPrincipal(ctx, accessPrincipalInput(principal))
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
	return dashboardhttp.Handler{
		Metrics: s.metrics,
		Broker:  s.broker,
		CSRFToken: func(r *http.Request) string {
			if s.auth == nil {
				return ""
			}
			return csrf.Token(r)
		},
	}
}
