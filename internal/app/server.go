package app

import (
	"context"
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
	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
	"github.com/gorilla/csrf"
)

type QueryMetrics interface {
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

type workspaceMetrics interface {
	MetricsForWorkspace(workspaceID string) (QueryMetrics, bool)
}

type multiWorkspaceMetrics struct {
	defaultID  string
	workspaces map[string]QueryMetrics
}

func NewMultiWorkspaceMetrics(defaultWorkspaceID string, workspaces map[string]QueryMetrics) QueryMetrics {
	copied := make(map[string]QueryMetrics, len(workspaces))
	for id, metrics := range workspaces {
		copied[id] = metrics
	}
	return multiWorkspaceMetrics{defaultID: defaultWorkspaceID, workspaces: copied}
}

func (m multiWorkspaceMetrics) MetricsForWorkspace(workspaceID string) (QueryMetrics, bool) {
	if workspaceID == "" {
		workspaceID = m.defaultID
	}
	metrics, ok := m.workspaces[workspaceID]
	return metrics, ok
}

func (m multiWorkspaceMetrics) defaultMetrics() QueryMetrics {
	if metrics, ok := m.MetricsForWorkspace(m.defaultID); ok {
		return metrics
	}
	for _, metrics := range m.workspaces {
		return metrics
	}
	return nil
}

type Server struct {
	metrics            QueryMetrics
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
	duckDBDir          string
	defaultWorkspaceID string
	defaultEnvironment string
	rateLimits         RateLimitConfig
	securityHeaders    SecurityHeadersConfig
	requestLogging     bool
	logger             *slog.Logger
	chatTitleMu        sync.Mutex
	pendingChatTitles  map[string]struct{}
}

func New(metrics QueryMetrics) *Server {
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
	DuckDBDir          string
	DefaultWorkspaceID string
	DefaultEnvironment string
	RateLimits         RateLimitConfig
	SecurityHeaders    SecurityHeadersConfig
	RequestLogging     bool
	Logger             *slog.Logger
}

func NewWithOptions(metrics QueryMetrics, options Options) *Server {
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
	server.duckDBDir = options.DuckDBDir
	server.defaultWorkspaceID = options.DefaultWorkspaceID
	server.defaultEnvironment = string(deployment.NormalizeEnvironment(deployment.Environment(options.DefaultEnvironment)))
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
			QueryMetrics: s.metrics,
			refresh:      s.refreshMaterializationsWithRun,
		}
	}
	return dashboardhttp.Handler{
		Metrics: metrics,
		MetricsForWorkspace: func(workspaceID string) (dashboardhttp.Metrics, bool) {
			selected, ok := s.metricsForWorkspace(workspaceID)
			if !ok {
				return nil, false
			}
			if s.store != nil {
				selected = dashboardCommandMetrics{
					QueryMetrics: selected,
					refresh: func(ctx context.Context, modelID string) error {
						return s.refreshMaterializationsWithRunForWorkspace(ctx, workspaceID, modelID)
					},
				}
			}
			return selected, true
		},
		Broker: s.broker,
		CSRFToken: func(r *http.Request) string {
			if s.auth == nil {
				return ""
			}
			return csrf.Token(r)
		},
	}
}

func (s *Server) metricsForWorkspace(workspaceID string) (QueryMetrics, bool) {
	if provider, ok := s.metrics.(workspaceMetrics); ok {
		return provider.MetricsForWorkspace(workspaceID)
	}
	if workspaceID == "" {
		return s.metrics, s.metrics != nil
	}
	if s.metrics == nil {
		return nil, false
	}
	if workspaceID == s.defaultWorkspaceID {
		return s.metrics, true
	}
	catalog := s.metrics.Catalog()
	if catalog.Workspace.ID == "" || catalog.Workspace.ID == workspaceID {
		return s.metrics, true
	}
	return nil, false
}

type dashboardCommandMetrics struct {
	QueryMetrics
	refresh func(context.Context, string) error
}

func (m dashboardCommandMetrics) RefreshMaterializations(ctx context.Context, modelID string) error {
	if m.refresh != nil {
		return m.refresh(ctx, modelID)
	}
	return m.QueryMetrics.RefreshMaterializations(ctx, modelID)
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
	orchestrator := NewRefreshOrchestrator(repo, s.metrics)
	return orchestrator.RefreshSemanticModel(ctx, refreshRunInput{
		WorkspaceID: workspaceID,
		ModelID:     modelID,
		PrincipalID: principal.ID,
	}, refreshPublisher{
		Root:   func() { s.publishModelRefreshPatches(ctx, workspaceID, modelID) },
		Target: func(string) { s.publishModelRefreshPatches(ctx, workspaceID, modelID) },
	})
}
