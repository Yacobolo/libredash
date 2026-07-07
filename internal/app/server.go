package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/agent"
	agentopenai "github.com/Yacobolo/libredash/internal/agent/openai"
	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	queryauthz "github.com/Yacobolo/libredash/internal/analytics/query/authz"
	dashboardhttp "github.com/Yacobolo/libredash/internal/dashboard/http"
	"github.com/Yacobolo/libredash/internal/execution"
	"github.com/Yacobolo/libredash/internal/platform"
	queryauditsqlite "github.com/Yacobolo/libredash/internal/queryaudit/sqlite"
	"github.com/Yacobolo/libredash/internal/queryruntime"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
	agentcore "github.com/Yacobolo/libredash/pkg/agent"
	"github.com/Yacobolo/libredash/pkg/pagestream"
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
	metrics             QueryMetrics
	executor            *execution.Service
	broker              *pagestream.Broker
	store               *platform.Store
	servingStateRepo    servingStateRepository
	workspaceRepo       workspace.Repository
	assetCatalog        workspace.AssetCatalogReader
	accessRepo          access.Repository
	agent               *agent.Service
	auth                *Auth
	reloader            runtimeReloader
	artifactDir         string
	duckDBDir           string
	duckLakeCatalogPath string
	duckLakeDataPath    string
	defaultWorkspaceID  string
	defaultEnvironment  string
	scimBearerToken     string
	metricsBearerToken  string
	allowedHosts        []string
	rateLimits          RateLimitConfig
	securityHeaders     SecurityHeadersConfig
	requestBodyLimit    RequestBodyLimitConfig
	requestLogging      bool
	telemetry           *httpTelemetry
	logger              *slog.Logger
	jobLeaseTimeout     time.Duration
	jobDispatchMu       sync.Mutex
	jobDispatching      bool
	jobDispatchWG       sync.WaitGroup
	backgroundMu        sync.Mutex
	backgroundCtx       context.Context
	backgroundCancel    context.CancelFunc
	backgroundStopping  bool
	chatTitleMu         sync.Mutex
	pendingChatTitles   map[string]struct{}
}

func New(metrics QueryMetrics) *Server {
	return &Server{
		metrics:           metrics,
		broker:            pagestream.NewBroker(),
		requestBodyLimit:  DefaultRequestBodyLimitConfig(),
		telemetry:         newHTTPTelemetry(),
		logger:            slog.Default(),
		pendingChatTitles: map[string]struct{}{},
	}
}

type Options struct {
	Store               *platform.Store
	ServingStateRepo    servingStateRepository
	WorkspaceRepo       workspace.Repository
	AssetCatalog        workspace.AssetCatalogReader
	AccessRepo          access.Repository
	Agent               *agent.Service
	Auth                *Auth
	Reloader            runtimeReloader
	ArtifactDir         string
	DuckDBDir           string
	DuckLakeCatalogPath string
	DuckLakeDataPath    string
	DefaultWorkspaceID  string
	DefaultEnvironment  string
	SCIMBearerToken     string
	MetricsBearerToken  string
	AllowedHosts        []string
	RateLimits          RateLimitConfig
	SecurityHeaders     SecurityHeadersConfig
	RequestBodyLimit    RequestBodyLimitConfig
	RequestLogging      bool
	Logger              *slog.Logger
	Executor            *execution.Service
	JobLeaseTimeout     time.Duration
}

func NewWithOptions(metrics QueryMetrics, options Options) *Server {
	executor := options.Executor
	if executor == nil {
		executor = execution.New(executionConfigFromEnv())
	}
	if metrics != nil {
		metrics = executionMetrics{QueryMetrics: metrics, executor: executor, defaultWorkspaceID: options.DefaultWorkspaceID}
	}
	dataAccessRepo := options.AccessRepo
	if dataAccessRepo == nil && options.Auth != nil && options.Store != nil {
		dataAccessRepo = accesssqlite.NewRepository(options.Store.SQLDB())
	}
	if metrics != nil && dataAccessRepo != nil && options.Auth != nil {
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
	server := New(metrics)
	server.executor = executor
	server.store = options.Store
	server.servingStateRepo = options.ServingStateRepo
	server.workspaceRepo = options.WorkspaceRepo
	server.assetCatalog = options.AssetCatalog
	server.accessRepo = options.AccessRepo
	if server.accessRepo == nil && dataAccessRepo != nil {
		server.accessRepo = dataAccessRepo
	}
	server.agent = options.Agent
	server.auth = options.Auth
	server.reloader = options.Reloader
	server.artifactDir = options.ArtifactDir
	server.duckDBDir = options.DuckDBDir
	server.duckLakeCatalogPath = options.DuckLakeCatalogPath
	server.duckLakeDataPath = options.DuckLakeDataPath
	server.defaultWorkspaceID = options.DefaultWorkspaceID
	server.defaultEnvironment = string(servingstate.NormalizeEnvironment(servingstate.Environment(options.DefaultEnvironment)))
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
	server.jobLeaseTimeout = options.JobLeaseTimeout
	if server.jobLeaseTimeout <= 0 {
		server.jobLeaseTimeout = durationEnv("LIBREDASH_EXEC_JOB_LEASE_TIMEOUT", 2*time.Minute)
	}
	if options.Logger != nil {
		server.logger = options.Logger
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
	s.backgroundMu.Unlock()
	s.dispatchQueuedRefreshJobs(backgroundCtx)
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
		s.backgroundMu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func executionConfigFromEnv() execution.Config {
	defaults := execution.DefaultConfig()
	return execution.Config{
		MaxRunningReads:      intEnv("LIBREDASH_EXEC_MAX_RUNNING_READS", defaults.MaxRunningReads),
		MaxQueuedReads:       intEnv("LIBREDASH_EXEC_MAX_QUEUED_READS", defaults.MaxQueuedReads),
		ReadQueueWait:        durationEnv("LIBREDASH_EXEC_READ_QUEUE_TIMEOUT", defaults.ReadQueueWait),
		ReadExecutionTimeout: durationEnv("LIBREDASH_EXEC_READ_TIMEOUT", defaults.ReadExecutionTimeout),
		MaxRunningJobs:       intEnv("LIBREDASH_EXEC_MAX_RUNNING_WRITES", defaults.MaxRunningJobs),
		MaxQueuedJobs:        intEnv("LIBREDASH_EXEC_MAX_QUEUED_WRITES", defaults.MaxQueuedJobs),
	}
}

func intEnv(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
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
		CurrentPrincipalID: func(r *http.Request) string {
			principal, ok := principalFromContext(r.Context())
			if !ok {
				return ""
			}
			return principal.ID
		},
		CSRFToken: func(r *http.Request) string {
			if s.auth == nil {
				return ""
			}
			return csrf.Token(r)
		},
		ChromeDecorators: s.dashboardChromeDecorators,
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
	refresh func(context.Context, string) error
}

func (m dashboardCommandMetrics) RefreshMaterializations(ctx context.Context, modelID string) error {
	if m.refresh != nil {
		return m.refresh(ctx, modelID)
	}
	return m.QueryMetrics.RefreshMaterializations(ctx, modelID)
}

func (s *Server) refreshMaterializationsWithRun(ctx context.Context, modelID string) error {
	return s.refreshMaterializationsWithRunForWorkspace(ctx, "", modelID)
}

func (s *Server) refreshMaterializationsWithRunForWorkspace(ctx context.Context, workspaceID, modelID string) error {
	if s.store == nil {
		return s.metrics.RefreshMaterializations(ctx, modelID)
	}
	repo, err := s.refreshRunRepository()
	if err != nil {
		return err
	}
	principal, _ := principalFromContext(ctx)
	orchestrator := materialize.NewRefreshOrchestrator(repo, appRefreshRunner{metrics: s.metrics}, refreshModelLookup(s.metrics))
	return orchestrator.RefreshSemanticModel(ctx, materialize.RefreshRunInput{
		WorkspaceID: workspaceID,
		ModelID:     modelID,
		PrincipalID: principal.ID,
	}, materialize.RefreshPublisher{
		Root:   func() { s.workspaceRefreshSupport().PublishModelRefreshPatches(ctx, workspaceID, modelID) },
		Target: func(string) { s.workspaceRefreshSupport().PublishModelRefreshPatches(ctx, workspaceID, modelID) },
	})
}
