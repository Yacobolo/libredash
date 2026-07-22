package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/access/http/mcpoauth"
	accesssqlite "github.com/Yacobolo/leapview/internal/access/sqlite"
	"github.com/Yacobolo/leapview/internal/agent"
	agentopenai "github.com/Yacobolo/leapview/internal/agent/openai"
	analyticsducklake "github.com/Yacobolo/leapview/internal/analytics/ducklake"
	queryauthz "github.com/Yacobolo/leapview/internal/analytics/query/authz"
	"github.com/Yacobolo/leapview/internal/analytics/resultcache"
	apiidempotencysqlite "github.com/Yacobolo/leapview/internal/apiidempotency/sqlite"
	"github.com/Yacobolo/leapview/internal/asyncjob"
	asyncjobsqlite "github.com/Yacobolo/leapview/internal/asyncjob/sqlite"
	cursorsigningsqlite "github.com/Yacobolo/leapview/internal/cursorsigning/sqlite"
	dashboardhttp "github.com/Yacobolo/leapview/internal/dashboard/http"
	"github.com/Yacobolo/leapview/internal/dashboard/publication"
	publicationsqlite "github.com/Yacobolo/leapview/internal/dashboard/publication/sqlite"
	dashboardstream "github.com/Yacobolo/leapview/internal/dashboard/stream"
	deploymenthttp "github.com/Yacobolo/leapview/internal/deployment/http"
	manageddatabinding "github.com/Yacobolo/leapview/internal/manageddata/binding"
	"github.com/Yacobolo/leapview/internal/manageddata/control"
	manageddatahttp "github.com/Yacobolo/leapview/internal/manageddata/http"
	manageddatasqlite "github.com/Yacobolo/leapview/internal/manageddata/sqlite"
	"github.com/Yacobolo/leapview/internal/platform"
	queryauditsqlite "github.com/Yacobolo/leapview/internal/queryaudit/sqlite"
	"github.com/Yacobolo/leapview/internal/queryruntime"
	"github.com/Yacobolo/leapview/internal/refreshpipeline"
	refreshpipelinesqlite "github.com/Yacobolo/leapview/internal/refreshpipeline/sqlite"
	releasesqlite "github.com/Yacobolo/leapview/internal/release/sqlite"
	"github.com/Yacobolo/leapview/internal/runtimehost"
	productsearch "github.com/Yacobolo/leapview/internal/search"
	searchsqlite "github.com/Yacobolo/leapview/internal/search/sqlite"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/leapview/internal/servingstate/sqlite"
	"github.com/Yacobolo/leapview/internal/staticasset"
	"github.com/Yacobolo/leapview/internal/ui"
	"github.com/Yacobolo/leapview/internal/workload"
	"github.com/Yacobolo/leapview/internal/workspace"
	workspacesqlite "github.com/Yacobolo/leapview/internal/workspace/sqlite"
	agentcore "github.com/Yacobolo/leapview/pkg/agent"
	"github.com/Yacobolo/leapview/pkg/pagestream"
	"github.com/gorilla/csrf"
)

type QueryMetrics = queryruntime.Metrics
type workspaceMetrics = queryruntime.WorkspaceMetrics

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
		return nil, false
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
	metrics                         QueryMetrics
	workloads                       *workload.Controller
	broker                          *pagestream.Broker
	publicationBroker               dashboardhttp.SignalBroker
	pageStreamTrace                 *pagestream.TraceStore
	dashboardRefreshes              *dashboardstream.Registry
	publicationStreams              publication.StreamRegistry
	publicationRepo                 *publicationsqlite.Repository
	publicationService              *publication.Service
	store                           *platform.Store
	servingStateRepo                servingStateRepository
	managedDataBindingRepo          manageddatabinding.Repository
	managedDataResolver             runtimehost.ManagedDataResolver
	refreshPipelineRepo             refreshpipeline.Repository
	refreshPipelineClock            refreshpipeline.Clock
	workspaceRepo                   workspace.Repository
	assetCatalogMu                  sync.Mutex
	assetCatalog                    workspace.AssetCatalogReader
	search                          *productsearch.Service
	accessRepo                      access.Repository
	asyncJobs                       asyncjob.Repository
	agent                           *agent.Service
	auth                            *Auth
	reloader                        runtimeReloader
	artifactDir                     string
	duckDBDir                       string
	duckLakeCatalogPath             string
	duckLakeDataPath                string
	duckDBEnvironment               *analyticsducklake.Environment
	queryResultCache                *resultcache.Pool
	defaultWorkspaceID              string
	defaultEnvironment              string
	scimBearerToken                 string
	metricsBearerToken              string
	allowedHosts                    []string
	rateLimits                      RateLimitConfig
	securityHeaders                 SecurityHeadersConfig
	requestBodyLimit                RequestBodyLimitConfig
	requestLogging                  bool
	telemetry                       *httpTelemetry
	logger                          *slog.Logger
	jobLeaseTimeout                 time.Duration
	jobDispatchMu                   sync.Mutex
	jobDispatching                  bool
	apiJobDispatching               bool
	jobDispatchWG                   sync.WaitGroup
	backgroundMu                    sync.Mutex
	backgroundCtx                   context.Context
	backgroundCancel                context.CancelFunc
	backgroundStopping              bool
	chatTitleMu                     sync.Mutex
	pendingChatTitles               map[string]struct{}
	managedDataOptions              manageddatahttp.Options
	deploymentOptions               deploymenthttp.Options
	managedDataTus                  http.Handler
	managedDataExpirer              managedDataUploadExpirer
	managedDataExpireInterval       time.Duration
	managedDataMaintenanceStarted   bool
	publicationMonitorStarted       bool
	refreshPipelineSchedulerStarted bool
	apiIdempotencyMu                sync.Mutex
	apiIdempotency                  map[string]*apiIdempotencyRecord
	apiIdempotencyStore             *apiidempotencysqlite.Store
	mcpOAuth                        *mcpoauth.Service
	mcpOAuthResource                mcpoauth.ResourceServer
	mcpOAuthInitErr                 error
	publicURL                       string
}

func New(metrics QueryMetrics) *Server {
	logger := slog.Default()
	var trace *pagestream.TraceStore
	if !staticasset.Production() {
		trace = pagestream.NewTraceStore(pagestream.TraceOptions{
			CapacityPerStream: 512,
			MaxStreams:        32,
			IncludePayloads:   true,
		})
	}
	return &Server{
		metrics:            metrics,
		broker:             pagestream.NewBroker(pagestream.WithTraceStore(trace)),
		pageStreamTrace:    trace,
		dashboardRefreshes: dashboardstream.NewRegistry(),
		publicationStreams: publication.NewMemoryStreamRegistry(),
		requestBodyLimit:   DefaultRequestBodyLimitConfig(),
		telemetry:          newHTTPTelemetry(),
		logger:             logger,
		pendingChatTitles:  map[string]struct{}{},
		apiIdempotency:     map[string]*apiIdempotencyRecord{},
	}
}

type Options struct {
	Store                     *platform.Store
	ServingStateRepo          servingStateRepository
	ManagedDataBindingRepo    manageddatabinding.Repository
	ManagedDataResolver       runtimehost.ManagedDataResolver
	WorkspaceRepo             workspace.Repository
	AssetCatalog              workspace.AssetCatalogReader
	AccessRepo                access.Repository
	Agent                     *agent.Service
	Auth                      *Auth
	Reloader                  runtimeReloader
	ArtifactDir               string
	DuckDBDir                 string
	DuckLakeCatalogPath       string
	DuckLakeDataPath          string
	DuckDBEnvironment         *analyticsducklake.Environment
	QueryResultCache          *resultcache.Pool
	DefaultWorkspaceID        string
	DefaultEnvironment        string
	SCIMBearerToken           string
	MetricsBearerToken        string
	AllowedHosts              []string
	RateLimits                RateLimitConfig
	SecurityHeaders           SecurityHeadersConfig
	RequestBodyLimit          RequestBodyLimitConfig
	RequestLogging            bool
	Logger                    *slog.Logger
	Workload                  *workload.Controller
	JobLeaseTimeout           time.Duration
	ManagedData               manageddatahttp.Options
	Deployment                deploymenthttp.Options
	ManagedDataTus            http.Handler
	ManagedDataExpirer        managedDataUploadExpirer
	ManagedDataExpireInterval time.Duration
	MCPOAuth                  MCPOAuthConfig
	PublicURL                 string
	RefreshPipelineClock      refreshpipeline.Clock
}

type MCPOAuthConfig struct {
	PublicURL string
	IssuerURL string
}

func (s *Server) AnalyticalFatal() <-chan struct{} {
	if s == nil || s.duckDBEnvironment == nil {
		return nil
	}
	return s.duckDBEnvironment.Fatal()
}

func (s *Server) AnalyticalHealth() error {
	if s == nil || s.duckDBEnvironment == nil {
		return nil
	}
	return s.duckDBEnvironment.Healthy()
}

func (s *Server) StopWorkloadAdmission() {
	if s != nil && s.workloads != nil {
		s.workloads.Close()
	}
}

func NewWithOptions(metrics QueryMetrics, options Options) *Server {
	telemetry := newHTTPTelemetry()
	if options.DuckDBEnvironment != nil || options.QueryResultCache != nil {
		telemetry.registry.MustRegister(newAnalyticalCollector(options.DuckDBEnvironment, options.QueryResultCache))
	}
	controller := options.Workload
	if controller == nil {
		controller, _ = workload.New(workload.DefaultConfig(), workload.WithObserver(telemetry))
	} else {
		controller.SetObserver(telemetry)
	}
	if metrics != nil {
		metrics = workloadMetrics{QueryMetrics: metrics, admitter: controller, defaultWorkspaceID: options.DefaultWorkspaceID}
	}
	dataAccessRepo := options.AccessRepo
	if dataAccessRepo == nil && options.Auth != nil && options.Store != nil {
		dataAccessRepo = accesssqlite.NewRepository(options.Store.SQLDB())
	}
	if metrics != nil && dataAccessRepo != nil {
		metrics = queryauthz.New(metrics, queryauthz.Options{
			Repo:               dataAccessRepo,
			DefaultWorkspaceID: options.DefaultWorkspaceID,
			PrincipalFromContext: func(ctx context.Context) (queryauthz.Principal, bool) {
				principal, ok := principalFromContext(ctx)
				return queryauthz.Principal{ID: principal.ID, DevBypass: principal.DevBypass}, ok
			},
			CredentialFromContext: apiCredentialFromContext,
			TokenAllows:           apiTokenAllows,
		})
	}
	if metrics != nil && options.Store != nil {
		metrics = queryAuditMetrics{
			QueryMetrics:       metrics,
			recorder:           queryauditsqlite.NewRepository(options.Store.SQLDB()),
			defaultWorkspaceID: options.DefaultWorkspaceID,
		}
	}
	servingStateRepo := options.ServingStateRepo
	managedDataBindingRepo := options.ManagedDataBindingRepo
	if options.Store != nil {
		if servingStateRepo == nil {
			servingStateRepo = servingstatesqlite.NewRepository(options.Store.SQLDB())
		}
		if managedDataBindingRepo == nil {
			managedDataBindingRepo = manageddatasqlite.NewRepository(options.Store.SQLDB())
		}
	}
	server := New(metrics)
	server.telemetry = telemetry
	server.publicationBroker = server.broker
	server.refreshPipelineClock = options.RefreshPipelineClock
	if server.refreshPipelineClock == nil {
		server.refreshPipelineClock = refreshpipeline.RealClock{}
	}
	server.workloads = controller
	server.store = options.Store
	if options.Store != nil {
		server.asyncJobs = asyncjobsqlite.NewRepository(options.Store.SQLDB())
		server.apiIdempotencyStore = apiidempotencysqlite.NewStore(options.Store.SQLDB())
		server.refreshPipelineRepo = refreshpipelinesqlite.NewRepository(options.Store.SQLDB())
		server.publicationRepo = publicationsqlite.NewRepository(options.Store.SQLDB())
		server.publicationStreams = publicationsqlite.NewStreamRegistry(options.Store.SQLDB())
		server.publicationBroker = publicationsqlite.NewBroker(options.Store.SQLDB(), server.pageStreamTrace, server.logger)
		server.publicationService = publication.NewService(server.publicationRepo, server.publicationStreams.ClosePublication)
		if err := cursorsigningsqlite.Configure(context.Background(), options.Store.SQLDB()); err != nil {
			server.logger.ErrorContext(context.Background(), "configure cursor signing failed", "error", err)
		}
		server.search = productsearch.NewService(searchsqlite.New(options.Store.SQLDB()), appSearchAuthorizer{server: server})
	}
	server.servingStateRepo = servingStateRepo
	server.managedDataBindingRepo = managedDataBindingRepo
	server.managedDataResolver = options.ManagedDataResolver
	server.workspaceRepo = options.WorkspaceRepo
	server.assetCatalog = options.AssetCatalog
	server.accessRepo = options.AccessRepo
	if server.accessRepo == nil && dataAccessRepo != nil {
		server.accessRepo = dataAccessRepo
	}
	server.agent = options.Agent
	server.auth = options.Auth
	if server.store != nil && server.auth != nil && server.accessRepo != nil {
		publicURL := strings.TrimSuffix(strings.TrimSpace(options.MCPOAuth.PublicURL), "/")
		if publicURL == "" {
			publicURL = "http://localhost:8080"
		}
		if issuerURL := strings.TrimSpace(options.MCPOAuth.IssuerURL); issuerURL != "" {
			server.mcpOAuthResource, server.mcpOAuthInitErr = mcpoauth.NewExternal(server.accessRepo, mcpoauth.ExternalConfig{
				IssuerURL: issuerURL, ResourceURL: publicURL + "/mcp",
			})
		} else {
			server.mcpOAuth, server.mcpOAuthInitErr = mcpoauth.New(server.store.SQLDB(), server.accessRepo, mcpoauth.Config{
				IssuerURL: publicURL, ResourceURL: publicURL + "/mcp", Secret: server.auth.mcpOAuthSecret(),
			})
			server.mcpOAuthResource = server.mcpOAuth
		}
	}
	server.reloader = options.Reloader
	server.artifactDir = options.ArtifactDir
	server.duckDBDir = options.DuckDBDir
	server.duckLakeCatalogPath = options.DuckLakeCatalogPath
	server.duckLakeDataPath = options.DuckLakeDataPath
	server.duckDBEnvironment = options.DuckDBEnvironment
	server.queryResultCache = options.QueryResultCache
	server.defaultWorkspaceID = options.DefaultWorkspaceID
	server.defaultEnvironment = string(servingstate.NormalizeEnvironment(servingstate.Environment(options.DefaultEnvironment)))
	server.publicURL = strings.TrimSuffix(strings.TrimSpace(options.PublicURL), "/")
	if server.publicURL == "" {
		server.publicURL = strings.TrimSuffix(strings.TrimSpace(options.MCPOAuth.PublicURL), "/")
	}
	server.scimBearerToken = options.SCIMBearerToken
	server.metricsBearerToken = options.MetricsBearerToken
	server.allowedHosts = append([]string(nil), options.AllowedHosts...)
	server.rateLimits = options.RateLimits
	server.securityHeaders = options.SecurityHeaders
	server.requestBodyLimit = options.RequestBodyLimit
	if !server.requestBodyLimit.Enabled && server.requestBodyLimit.MaxBytes == 0 {
		server.requestBodyLimit = DefaultRequestBodyLimitConfig()
	}
	server.requestLogging = options.RequestLogging
	server.managedDataOptions = options.ManagedData
	server.deploymentOptions = options.Deployment
	server.managedDataTus = options.ManagedDataTus
	server.managedDataExpirer = options.ManagedDataExpirer
	server.managedDataExpireInterval = options.ManagedDataExpireInterval
	server.jobLeaseTimeout = options.JobLeaseTimeout
	if server.jobLeaseTimeout <= 0 {
		server.jobLeaseTimeout = 2 * time.Minute
	}
	if options.Logger != nil {
		server.logger = options.Logger
		if server.pageStreamTrace != nil {
			server.pageStreamTrace.SetLogger(options.Logger)
		}
	}
	if server.mcpOAuthInitErr != nil {
		server.logger.ErrorContext(context.Background(), "initialize MCP OAuth failed", "error", server.mcpOAuthInitErr)
	}
	if err := server.registerDefaultWorkspaceSecurable(context.Background()); err != nil {
		server.logger.ErrorContext(context.Background(), "register default workspace securable failed", "workspace", server.defaultWorkspaceID, "error", err)
	}
	if err := server.registerStoredWorkspaceSecurables(context.Background()); err != nil {
		server.logger.ErrorContext(context.Background(), "register stored workspace securables failed", "error", err)
	}
	if server.agent != nil {
		server.agent.ConfigureDefaultModel(func(config agent.Config) agentcore.Model {
			return agentopenai.NewModel(config, nil)
		})
	}
	server.configureAgentTools()
	return server
}

func (s *Server) StartBackgroundJobs(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.backgroundMu.Lock()
	if s.backgroundCancel == nil {
		s.backgroundCtx, s.backgroundCancel = context.WithCancel(ctx)
		s.backgroundStopping = false
	}
	backgroundCtx := s.backgroundCtx
	startManagedDataMaintenance := s.managedDataExpirer != nil && !s.managedDataMaintenanceStarted
	if startManagedDataMaintenance {
		s.managedDataMaintenanceStarted = true
	}
	startPublicationMonitor := s.publicationRepo != nil && !s.publicationMonitorStarted
	if startPublicationMonitor {
		s.publicationMonitorStarted = true
	}
	s.backgroundMu.Unlock()
	s.dispatchQueuedRefreshJobs(backgroundCtx)
	s.dispatchQueuedAsyncJobs(backgroundCtx)
	s.startRefreshPipelineScheduler(backgroundCtx)
	if startManagedDataMaintenance {
		s.startManagedDataMaintenance(backgroundCtx)
	}
	if startPublicationMonitor {
		s.startPublicationMonitor(backgroundCtx)
	}
}

func (s *Server) StopBackgroundJobs(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.backgroundMu.Lock()
	cancel := s.backgroundCancel
	if cancel == nil {
		s.backgroundMu.Unlock()
		return nil
	}
	s.backgroundStopping = true
	cancel()
	s.backgroundMu.Unlock()

	done := make(chan struct{})
	go func() {
		s.jobDispatchWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		s.backgroundMu.Lock()
		s.backgroundCtx = nil
		s.backgroundCancel = nil
		s.backgroundStopping = false
		s.managedDataMaintenanceStarted = false
		s.publicationMonitorStarted = false
		s.refreshPipelineSchedulerStarted = false
		s.backgroundMu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type managedDataUploadExpirer interface {
	ExpireUploads(context.Context) (control.ExpireResult, error)
}

func (s *Server) startManagedDataMaintenance(ctx context.Context) {
	interval := s.managedDataExpireInterval
	if interval <= 0 {
		interval = time.Hour
	}
	s.jobDispatchWG.Add(1)
	go func() {
		defer s.jobDispatchWG.Done()
		run := func() {
			lease, err := s.workloadController().Acquire(ctx, workload.Request{Class: workload.Maintenance, Operation: "managed_data.collect"})
			if err != nil {
				s.logger.DebugContext(ctx, "managed-data maintenance skipped", "error", err)
				return
			}
			defer lease.Release()
			result, err := s.managedDataExpirer.ExpireUploads(lease.Context())
			if err != nil {
				s.logger.WarnContext(ctx, "managed-data upload expiration failed", "error", err)
				return
			}
			if result.Expired > 0 {
				s.logger.InfoContext(ctx, "expired managed-data upload sessions", "count", result.Expired)
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func (s *Server) workspaceRepository() (workspace.Repository, error) {
	if s.workspaceRepo != nil {
		return s.workspaceRepo, nil
	}
	if s.store == nil {
		return nil, nil
	}
	var securables workspacesqlite.SecurableRegistrar
	if accessRepo, err := s.accessRepository(); err == nil {
		securables = accessRepo
	} else if s.logger != nil {
		s.logger.ErrorContext(context.Background(), "create access repository for workspace securable registration failed", "error", err)
	}
	s.workspaceRepo = workspacesqlite.NewRepositoryWithSecurables(s.store.SQLDB(), securables)
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

func (s *Server) authorizeListObject(ctx context.Context, principalID string, object access.ObjectRef) (bool, error) {
	if s.auth == nil {
		return true, nil
	}
	repo, err := s.accessRepository()
	if err != nil {
		return false, err
	}
	if repo == nil || strings.TrimSpace(principalID) == "" {
		return false, nil
	}
	decision, err := repo.Authorize(ctx, principalID, access.PrivilegeViewItem, object)
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

func (s *Server) releaseRepository() *releasesqlite.Repository {
	if s.store == nil {
		return nil
	}
	return releasesqlite.NewRepository(s.store.SQLDB())
}

func (s *Server) registerDefaultWorkspaceSecurable(ctx context.Context) error {
	if strings.TrimSpace(s.defaultWorkspaceID) == "" {
		return nil
	}
	repo, err := s.accessRepository()
	if err != nil {
		return err
	}
	if repo == nil {
		return nil
	}
	_, err = repo.UpsertSecurableObject(ctx, access.WorkspaceObject(s.defaultWorkspaceID), "")
	return err
}

func (s *Server) registerStoredWorkspaceSecurables(ctx context.Context) error {
	workspaceRepo, err := s.workspaceRepository()
	if err != nil {
		return err
	}
	accessRepo, err := s.accessRepository()
	if err != nil {
		return err
	}
	if workspaceRepo == nil || accessRepo == nil {
		return nil
	}
	workspaces, err := workspaceRepo.List(ctx)
	if err != nil {
		return err
	}
	for _, row := range workspaces {
		object := access.WorkspaceObject(string(row.ID))
		object.DisplayName = row.Title
		if _, err := accessRepo.UpsertSecurableObject(ctx, object, ""); err != nil {
			return err
		}
	}
	return nil
}

func principalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

func apiCredentialFromContext(ctx context.Context) (access.APICredential, bool) {
	credential, ok := ctx.Value(apiCredentialContextKey{}).(access.APICredential)
	return credential, ok
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
		Role:        access.RolePlatformAdmin,
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
	if err := ui.CatalogPageForCatalogs(s.workspaceHTTPReadModel().CatalogsForVisibleWorkspaces(r), s.chatChromeOption(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.LoginPage(s.loginPageOptions(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) loginPageOptions(r *http.Request) ui.LoginPageOptions {
	opts := ui.LoginPageOptions{
		LocalAuth:     s.auth != nil && s.auth.localAuth,
		SSOAuth:       s.auth == nil || s.auth.configured,
		ProviderLabel: "Sign in with Azure Active Directory",
	}
	if s.auth != nil {
		opts.CSRFToken = csrf.Token(r)
		if principal, _, ok := s.auth.authenticate(r); ok {
			opts.MustChangePassword = s.auth.mustChangeLocalPassword(r, principal.ID)
		}
	}
	return opts
}

func (s *Server) dashboardHTTP() dashboardhttp.Handler {
	var metrics dashboardhttp.Metrics = s.metrics
	if s.store != nil {
		metrics = dashboardCommandMetrics{
			QueryMetrics: s.metrics,
		}
	}
	return dashboardhttp.Handler{
		Metrics: metrics,
		AnalyticalContext: func(ctx context.Context) context.Context {
			return workload.WithAdmitter(ctx, s.workloadController())
		},
		MetricsForWorkspace: func(workspaceID string) (dashboardhttp.Metrics, bool) {
			selected, ok := s.metricsForWorkspace(workspaceID)
			if !ok {
				return nil, false
			}
			if s.store != nil {
				selected = dashboardCommandMetrics{
					QueryMetrics: selected,
				}
			}
			return selected, true
		},
		Broker:       s.broker,
		Coordinators: s.dashboardRefreshes,
		Logger:       s.logger,
		RefreshStarted: func(refresh dashboardstream.Refresh) {
			s.telemetry.dashboardRefreshStarted(refresh.Command)
		},
		RefreshFinished:      s.telemetry.dashboardRefreshFinished,
		RefreshEventObserved: s.telemetry.dashboardRefreshEventObserved,
		CacheObserved:        s.telemetry.dashboardCacheObserved,
		CurrentPrincipalID: func(r *http.Request) string {
			principal, ok := principalFromContext(r.Context())
			if !ok {
				return ""
			}
			return principal.ID
		},
		AuthorizeListObject: s.authorizeListObject,
		CSRFToken: func(r *http.Request) string {
			if s.auth == nil {
				return ""
			}
			return csrf.Token(r)
		},
		ChromeDecorators: s.dashboardChromeDecorators,
		Environment:      func(r *http.Request) string { return string(s.requestServingEnvironment(r)) },
		DataRefreshedAt: func(ctx context.Context, workspaceID, environment, modelID string) string {
			if s.refreshPipelineRepo == nil {
				return ""
			}
			version, ok, err := s.refreshPipelineRepo.DataVersion(ctx, workspaceID, environment, modelID)
			if err != nil || !ok {
				return ""
			}
			return version.RefreshedAt.Format(time.RFC3339)
		},
		AgentBootstrap: func(r *http.Request, workspaceID string) ui.ChatViewState {
			return s.agentHTTPHandler().DashboardBootstrap(r, workspaceID)
		},
	}
}

func (s *Server) metricsForWorkspace(workspaceID string) (QueryMetrics, bool) {
	if workspaceID == "" {
		return nil, false
	}
	if provider, ok := s.metrics.(workspaceMetrics); ok {
		return provider.MetricsForWorkspace(workspaceID)
	}
	if s.metrics == nil {
		return nil, false
	}
	if s.defaultWorkspaceID != "" && workspaceID == s.defaultWorkspaceID {
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
}
