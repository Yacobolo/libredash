package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	accessmodule "github.com/Yacobolo/leapview/internal/access/module"
	adminmodule "github.com/Yacobolo/leapview/internal/admin/module"
	agentmodule "github.com/Yacobolo/leapview/internal/agent/module"
	analyticsmodule "github.com/Yacobolo/leapview/internal/analytics/module"
	"github.com/Yacobolo/leapview/internal/api"
	apiapigenruntime "github.com/Yacobolo/leapview/internal/api/apigenruntime"
	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	apihttpmiddleware "github.com/Yacobolo/leapview/internal/api/httpmiddleware"
	apiprotocol "github.com/Yacobolo/leapview/internal/api/protocol"
	"github.com/Yacobolo/leapview/internal/catalog"
	dashboardmodule "github.com/Yacobolo/leapview/internal/dashboard/module"
	deploymentmodule "github.com/Yacobolo/leapview/internal/deployment/module"
	manageddatamodule "github.com/Yacobolo/leapview/internal/manageddata/module"
	"github.com/Yacobolo/leapview/internal/observability"
	"github.com/Yacobolo/leapview/internal/platform/jobs"
	jobsmodule "github.com/Yacobolo/leapview/internal/platform/jobs/module"
	platformlifecycle "github.com/Yacobolo/leapview/internal/platform/lifecycle"
	refreshmodule "github.com/Yacobolo/leapview/internal/refresh/module"
	releasemodule "github.com/Yacobolo/leapview/internal/release/module"
	runtimehostmodule "github.com/Yacobolo/leapview/internal/runtimehost/module"
	servingstatemodule "github.com/Yacobolo/leapview/internal/servingstate/module"
	"github.com/Yacobolo/leapview/internal/staticasset"
	"github.com/Yacobolo/leapview/internal/ui"
	uitransport "github.com/Yacobolo/leapview/internal/ui/transport"
	workloadmodule "github.com/Yacobolo/leapview/internal/workload/module"
	workspacemodule "github.com/Yacobolo/leapview/internal/workspace/module"
	"github.com/Yacobolo/leapview/pkg/pagestream"
)

type QueryMetrics = dashboardmodule.Metrics
type workspaceMetrics = dashboardmodule.WorkspaceMetrics

type capabilityRoutes struct {
	accessModule      *accessmodule.Module
	workspaceModule   *workspacemodule.Module
	managedDataModule *manageddatamodule.Module
	deploymentModule  *deploymentmodule.Module
	dashboardModule   *dashboardmodule.Module
	dashboardAssets   dashboardmodule.Assets
	agentModule       *agentmodule.Module
	releaseModule     *releasemodule.Module
	refreshModule     *refreshmodule.Module
	adminModule       *adminmodule.Module
}

type runtimeServices struct {
	analyticsModule       *analyticsmodule.Module
	metrics               QueryMetrics
	workloads             workloadControl
	broker                *pagestream.Broker
	pageStreamTrace       *pagestream.TraceStore
	pageStreams           *uitransport.PageStream
	persistenceConfigured bool
	platformHealth        platformHealth
	storageRetention      *servingstatemodule.Retention
	queryAuditProvider    adminmodule.QueryAuditReaderProvider
	queryAuditEvents      http.HandlerFunc
}

type platformServices struct {
	asyncJobs     jobs.Repository
	jobModule     *jobsmodule.Module
	auth          *accessmodule.Auth
	telemetry     *observability.Telemetry
	health        *observability.Health
	logger        *slog.Logger
	workers       *platformlifecycle.Group
	apiProtocol   *apiprotocol.Protocol
	apiGenHandler *apiapigenruntime.Handler
}

type httpPolicy struct {
	defaultWorkspaceID string
	defaultEnvironment string
	scimBearerToken    string
	metricsBearerToken string
	allowedHosts       []string
	rateLimits         apihttpmiddleware.RateLimitConfig
	securityHeaders    apihttpmiddleware.SecurityHeadersConfig
	requestBodyLimit   apihttpmiddleware.RequestBodyLimitConfig
	requestLogging     bool
	managedDataTus     http.Handler
}

type persistenceInputs struct {
	agentSettings         agentmodule.Settings
	adminDatabase         *sql.DB
	servingStateRepo      servingStateRepository
	workspaceReadModel    workspacemodule.ReadModel
	workspaceDirectory    workspacemodule.Directory
	workspaceAssetCatalog workspacemodule.AssetCatalogReader
	accessRepo            accessmodule.Repository
}

type workflowInputs struct {
	managedDataValidation refreshmodule.CandidateValidationHook
	managedDataResolver   runtimehostmodule.ManagedDataResolver
	refreshPipelineClock  refreshmodule.Clock
	agent                 *agentmodule.Service
	agentConfig           agentmodule.ModelConfig
	reloader              runtimeReloader
	deploymentConfig      deploymentmodule.Config
}

type storageInputs struct {
	duckLakeCatalogPath string
	duckLakeDataPath    string
	jobLeaseTimeout     time.Duration
	publicURL           string
}

func newCompositionSurfaces(metrics QueryMetrics) (*capabilityRoutes, *runtimeServices, *platformServices, *httpPolicy) {
	logger := slog.Default()
	var trace *pagestream.TraceStore
	if !staticasset.Production() {
		trace = pagestream.NewTraceStore(pagestream.TraceOptions{
			CapacityPerStream: 512,
			MaxStreams:        32,
			IncludePayloads:   true,
		})
	}
	routes := &capabilityRoutes{}
	runtime := &runtimeServices{
		metrics: metrics, broker: pagestream.NewBroker(pagestream.WithTraceStore(trace)),
		pageStreamTrace: trace,
	}
	platform := &platformServices{telemetry: observability.New(), logger: logger}
	policy := &httpPolicy{requestBodyLimit: apihttpmiddleware.DefaultRequestBodyLimitConfig()}
	return routes, runtime, platform, policy
}

type dataAssemblyInputs struct {
	Database           *sql.DB
	PlatformHealth     platformHealth
	AdminDatabase      *sql.DB
	ServingStateRepo   servingStateRepository
	StorageRetention   *servingstatemodule.Retention
	WorkspaceReadModel workspacemodule.ReadModel
	WorkspaceDirectory workspacemodule.Directory
	AssetCatalog       workspacemodule.AssetCatalogReader
	AccessRepo         accessmodule.Repository
}

type capabilityAssemblyInputs struct {
	ReleaseModule     *releasemodule.Module
	JobModule         *jobsmodule.Module
	AccessModule      *accessmodule.Module
	Agent             *agentmodule.Service
	ManagedDataModule *manageddatamodule.Module
	AnalyticsModule   *analyticsmodule.Module
	DashboardAssets   dashboardmodule.Assets
}

type workflowAssemblyInputs struct {
	AgentSettings         agentmodule.Settings
	ManagedDataValidation refreshmodule.CandidateValidationHook
	ManagedDataResolver   runtimehostmodule.ManagedDataResolver
	AgentConfig           agentmodule.ModelConfig
	Auth                  *accessmodule.Auth
	Reloader              runtimeReloader
	Workload              workloadControl
	DeploymentConfig      deploymentmodule.Config
	RefreshPipelineClock  refreshmodule.Clock
	QueryAudit            *analyticsmodule.QueryAuditSurface
}

type runtimeAssemblyInputs struct {
	DuckDBDir           string
	DuckLakeCatalogPath string
	DuckLakeDataPath    string
	DefaultWorkspaceID  string
	DefaultEnvironment  string
	SCIMBearerToken     string
	MetricsBearerToken  string
	AllowedHosts        []string
}

type httpAssemblyInputs struct {
	RateLimits       apihttpmiddleware.RateLimitConfig
	SecurityHeaders  apihttpmiddleware.SecurityHeadersConfig
	RequestBodyLimit apihttpmiddleware.RequestBodyLimitConfig
	RequestLogging   bool
	Logger           *slog.Logger
	JobLeaseTimeout  time.Duration
	ManagedDataTus   http.Handler
	MCPOAuth         MCPOAuthConfig
	PublicURL        string
}

type MCPOAuthConfig struct {
	PublicURL string
	IssuerURL string
}

type platformHealth interface {
	Ping(context.Context) error
}

type workloadControl interface {
	workloadmodule.Admitter
	Stats() workloadmodule.Stats
	SetObserver(workloadmodule.Observer)
	Close()
}

func AnalyticalFatal(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy) <-chan struct{} {
	if runtime == nil || runtime.analyticsModule == nil {
		return nil
	}
	return runtime.analyticsModule.Fatal()
}

func AnalyticalHealth(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy) error {
	if runtime == nil || runtime.analyticsModule == nil {
		return nil
	}
	return runtime.analyticsModule.Healthy()
}

func StopWorkloadAdmission(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy) {
	if runtime != nil && runtime.workloads != nil {
		runtime.workloads.Close()
	}
}

func buildApplicationSurfaces(
	ctx context.Context,
	metrics QueryMetrics,
	data dataAssemblyInputs,
	capabilities capabilityAssemblyInputs,
	workflow workflowAssemblyInputs,
	runtimeConfig runtimeAssemblyInputs,
	httpConfig httpAssemblyInputs,
) (*capabilityRoutes, *runtimeServices, *platformServices, *httpPolicy, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	telemetry := observability.New()
	if capabilities.AnalyticsModule != nil {
		telemetry.Register(capabilities.AnalyticsModule.Collector())
	}
	controller := workflow.Workload
	ownsController := false
	workloadTelemetry := workloadmodule.NewTelemetryObserver(telemetry)
	if controller == nil {
		var err error
		controller, err = workloadmodule.Build(ctx, workloadmodule.Config{
			Policy: workloadmodule.DefaultConfig(), Observer: workloadTelemetry,
		})
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("build workload module: %w", err)
		}
		ownsController = true
	} else {
		controller.SetObserver(workloadTelemetry)
	}
	fail := func(err error) (*capabilityRoutes, *runtimeServices, *platformServices, *httpPolicy, error) {
		if ownsController && controller != nil {
			controller.Close()
		}
		return nil, nil, nil, nil, err
	}
	if metrics != nil {
		metrics = dashboardmodule.WithAdmission(metrics, controller, runtimeConfig.DefaultWorkspaceID)
	}
	dataAccessRepo := data.AccessRepo
	workspaceReadModel := data.WorkspaceReadModel
	var dataAuthorization accessmodule.DataAuthorizationService = dataAccessRepo
	if capabilities.AccessModule != nil {
		dataAuthorization = capabilities.AccessModule.DataAuthorizationService()
	}
	if metrics != nil && dataAuthorization != nil && (data.AccessRepo != nil || workflow.Auth != nil || capabilities.AccessModule != nil) {
		metrics = dashboardmodule.WithQueryAuthorization(metrics, dashboardmodule.QueryAuthorizationConfig{
			Repository:         dataAuthorization,
			DefaultWorkspaceID: runtimeConfig.DefaultWorkspaceID,
			PrincipalFromContext: func(ctx context.Context) (dashboardmodule.QueryPrincipal, bool) {
				principal, ok := accessmodule.PrincipalFromContext(ctx)
				return dashboardmodule.QueryPrincipal{ID: principal.ID, DevBypass: principal.DevBypass || workflow.Auth == nil}, ok
			},
			CredentialFromContext: accessmodule.APICredentialFromContext,
			TokenAllows:           accessmodule.TokenAllows,
		})
	}
	var queryAuditProvider adminmodule.QueryAuditReaderProvider
	var queryAuditRecorder dashboardmodule.QueryAuditRecorder
	var queryAuditEvents http.HandlerFunc
	if workflow.QueryAudit != nil {
		queryAuditProvider = adminmodule.QueryAuditReaderProvider(workflow.QueryAudit.Provider())
		queryAuditRecorder = workflow.QueryAudit.Recorder()
		queryAuditEvents = workflow.QueryAudit.Events(func(value string) string { return value })
	}
	if capabilities.AnalyticsModule != nil {
		if capabilities.AnalyticsModule.QueryAuditReader() != nil {
			queryAuditProvider = adminmodule.QueryAuditReaderProvider(capabilities.AnalyticsModule.QueryAuditProvider())
		}
		if capabilities.AnalyticsModule.QueryAuditRecorder() != nil {
			queryAuditRecorder = capabilities.AnalyticsModule.QueryAuditRecorder()
		}
	}
	if metrics != nil && queryAuditRecorder != nil {
		metrics = dashboardmodule.WithQueryAudit(metrics, queryAuditRecorder, runtimeConfig.DefaultWorkspaceID, func(ctx context.Context) (string, bool) {
			principal, ok := accessmodule.PrincipalFromContext(ctx)
			return principal.ID, ok
		})
	}
	servingStateRepo := data.ServingStateRepo
	routes, runtime, platform, policy := newCompositionSurfaces(metrics)
	persistence := persistenceInputs{}
	moduleWorkflow := workflowInputs{}
	storage := storageInputs{}
	runtime.queryAuditEvents = queryAuditEvents
	if runtime.queryAuditEvents == nil {
		runtime.queryAuditEvents = analyticsmodule.NewQueryAuditEvents(nil, func(value string) string { return workspaceID(routes, runtime, platform, policy, value) })
	}
	if capabilities.AnalyticsModule != nil && capabilities.AnalyticsModule.QueryAuditReader() != nil {
		runtime.queryAuditEvents = capabilities.AnalyticsModule.QueryAuditEvents(func(value string) string { return workspaceID(routes, runtime, platform, policy, value) })
	}
	platform.telemetry = telemetry
	moduleWorkflow.refreshPipelineClock = workflow.RefreshPipelineClock
	runtime.queryAuditProvider = queryAuditProvider
	if moduleWorkflow.refreshPipelineClock == nil {
		moduleWorkflow.refreshPipelineClock = refreshmodule.NewRealClock()
	}
	runtime.workloads = controller
	runtime.persistenceConfigured = data.Database != nil
	runtime.platformHealth = data.PlatformHealth
	persistence.agentSettings = workflow.AgentSettings
	persistence.adminDatabase = data.AdminDatabase
	if data.Database != nil {
		platform.jobModule = capabilities.JobModule
		if platform.jobModule == nil {
			var err error
			platform.jobModule, err = jobsmodule.Build(ctx, jobsmodule.Config{
				Database: data.Database, Admission: runtime.workloads,
				LeaseTimeout: httpConfig.JobLeaseTimeout, Logger: httpConfig.Logger,
			})
			if err != nil {
				return fail(fmt.Errorf("build platform jobs module: %w", err))
			}
		}
		platform.asyncJobs = platform.jobModule
		if err := configureAPIProtocol(routes, runtime, platform, policy, ctx, data.Database); err != nil {
			return fail(fmt.Errorf("build API protocol: %w", err))
		}
	}
	if platform.apiProtocol == nil {
		if err := configureAPIProtocol(routes, runtime, platform, policy, ctx, nil); err != nil {
			return fail(fmt.Errorf("build API protocol: %w", err))
		}
	}
	persistence.servingStateRepo = servingStateRepo
	retentionStates, _ := servingStateRepo.(servingstatemodule.RetentionRepository)
	runtime.storageRetention = data.StorageRetention
	if runtime.storageRetention == nil {
		runtime.storageRetention = servingstatemodule.NewRetention(servingstatemodule.RetentionConfig{
			States: retentionStates, Snapshots: capabilities.AnalyticsModule.RetentionSnapshots(),
			Admission: controller, Environment: runtimeConfig.DefaultEnvironment,
			CatalogPath: runtimeConfig.DuckLakeCatalogPath, DataPath: runtimeConfig.DuckLakeDataPath,
			ProtectedSnapshots: func() []int64 {
				if provider, ok := workflow.Reloader.(interface{ LeasedSnapshots() []int64 }); ok {
					return provider.LeasedSnapshots()
				}
				return nil
			},
		})
	}
	moduleWorkflow.managedDataValidation = workflow.ManagedDataValidation
	moduleWorkflow.managedDataResolver = workflow.ManagedDataResolver
	runtime.analyticsModule = capabilities.AnalyticsModule
	routes.dashboardAssets = capabilities.DashboardAssets
	persistence.workspaceReadModel = workspaceReadModel
	persistence.workspaceDirectory = data.WorkspaceDirectory
	persistence.workspaceAssetCatalog = data.AssetCatalog
	routes.releaseModule = capabilities.ReleaseModule
	persistence.accessRepo = data.AccessRepo
	moduleWorkflow.agent = capabilities.Agent
	moduleWorkflow.agentConfig = workflow.AgentConfig
	platform.auth = workflow.Auth
	routes.accessModule = capabilities.AccessModule
	moduleWorkflow.reloader = workflow.Reloader
	storage.duckLakeCatalogPath = runtimeConfig.DuckLakeCatalogPath
	storage.duckLakeDataPath = runtimeConfig.DuckLakeDataPath
	policy.defaultWorkspaceID = runtimeConfig.DefaultWorkspaceID
	policy.defaultEnvironment = string(servingstatemodule.NormalizeEnvironment(servingstatemodule.Environment(runtimeConfig.DefaultEnvironment)))
	storage.publicURL = strings.TrimSuffix(strings.TrimSpace(httpConfig.PublicURL), "/")
	policy.scimBearerToken = runtimeConfig.SCIMBearerToken
	policy.metricsBearerToken = runtimeConfig.MetricsBearerToken
	policy.allowedHosts = append([]string(nil), runtimeConfig.AllowedHosts...)
	policy.rateLimits = httpConfig.RateLimits
	policy.securityHeaders = httpConfig.SecurityHeaders
	policy.requestBodyLimit = httpConfig.RequestBodyLimit
	if !policy.requestBodyLimit.Enabled && policy.requestBodyLimit.MaxBytes == 0 {
		policy.requestBodyLimit = apihttpmiddleware.DefaultRequestBodyLimitConfig()
	}
	policy.requestLogging = httpConfig.RequestLogging
	routes.managedDataModule = capabilities.ManagedDataModule
	moduleWorkflow.deploymentConfig = workflow.DeploymentConfig
	policy.managedDataTus = httpConfig.ManagedDataTus
	storage.jobLeaseTimeout = httpConfig.JobLeaseTimeout
	if storage.jobLeaseTimeout <= 0 {
		storage.jobLeaseTimeout = 2 * time.Minute
	}
	if httpConfig.Logger != nil {
		platform.logger = httpConfig.Logger
		if runtime.pageStreamTrace != nil {
			runtime.pageStreamTrace.SetLogger(httpConfig.Logger)
		}
	}
	if err := configureRefreshModule(routes, runtime, platform, policy, ctx, data.Database, persistence, moduleWorkflow, storage); err != nil {
		return fail(err)
	}
	if err := configureModules(routes, runtime, platform, policy, ctx, data.Database, persistence, moduleWorkflow, storage); err != nil {
		return fail(err)
	}
	if platform.asyncJobs != nil {
		handlers := make([]jobs.Handler, 0, 4)
		if routes.releaseModule != nil {
			handlers = append(handlers, routes.releaseModule.JobHandlers(platform.asyncJobs)...)
		}
		if routes.deploymentModule != nil {
			handlers = append(handlers, routes.deploymentModule.JobHandlers()...)
		}
		if routes.managedDataModule != nil && routes.managedDataModule.HasFinalizeJobs() {
			handlers = append(handlers, routes.managedDataModule.JobHandlers(platform.asyncJobs)...)
		}
		if routes.agentModule != nil {
			handlers = append(handlers, routes.agentModule.JobHandlers(platform.asyncJobs)...)
		}
		if err := platform.jobModule.RegisterHandlers(handlers); err != nil {
			return fail(fmt.Errorf("register async job handlers: %w", err))
		}
	}
	return routes, runtime, platform, policy, nil
}

func configureModules(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, ctx context.Context, database *sql.DB, persistence persistenceInputs, moduleWorkflow workflowInputs, storage storageInputs) error {
	if routes == nil || runtime == nil || platform == nil || policy == nil {
		return errors.New("runtime router is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var apiDispatcher *apiGenDispatcher
	if routes.accessModule == nil {
		var err error
		routes.accessModule, err = accessmodule.Build(ctx, accessmodule.Config{
			Database: database, ExistingAuth: platform.auth, WorkspaceID: policy.defaultWorkspaceID,
			WorkspaceIDs: func(ctx context.Context) ([]string, error) {
				if persistence.workspaceDirectory != nil {
					return persistence.workspaceDirectory.WorkspaceIDs(ctx)
				}
				repository, err := workspaceReadModel(routes, runtime, platform, policy, persistence)
				if err != nil || repository == nil {
					return nil, err
				}
				rows, err := repository.List(ctx)
				if err != nil {
					return nil, err
				}
				ids := make([]string, 0, len(rows))
				for _, row := range rows {
					ids = append(ids, string(row.ID))
				}
				return ids, nil
			},
		})
		if err != nil {
			return fmt.Errorf("build access module: %w", err)
		}
	}
	if routes.workspaceModule == nil {
		refreshSupport := workspaceRefreshSupport(routes, runtime, platform, policy)
		var err error
		routes.workspaceModule, err = workspacemodule.Build(ctx, workspacemodule.Config{
			Database:      database,
			Directory:     persistence.workspaceDirectory,
			ReadModel:     persistence.workspaceReadModel,
			AccessService: routes.accessModule.WorkspaceAccessService(),
			AssetCatalog:  persistence.workspaceAssetCatalog,
			WorkspaceID: func(value string) string {
				return workspaceID(routes, runtime, platform, policy, value)
			},
			Environment: func(r *http.Request) string {
				return string(requestServingEnvironment(routes, runtime, platform, policy, r))
			},
			MetricsForWorkspace: func(workspaceID string) (QueryMetrics, bool) {
				return metricsForWorkspace(routes, runtime, platform, policy, workspaceID)
			},
			RootMetrics: runtime.metrics,
			CurrentPrincipal: func(r *http.Request) (workspacemodule.Principal, bool) {
				principal, ok := routes.accessModule.CurrentPrincipal(r)
				return workspacemodule.Principal{
					ID: principal.ID, Email: principal.Email,
					DisplayName: principal.DisplayName, DevBypass: principal.DevBypass,
				}, ok
			},
			AuthConfigured:     platform.auth != nil,
			RuntimeEnvironment: policy.defaultEnvironment,
			DefaultWorkspaceID: policy.defaultWorkspaceID,
			RefreshState:       refreshSupport,
			RefreshRunner: workspacemodule.AssetRefreshFunc(func(ctx context.Context, input workspacemodule.AssetRefreshInput) error {
				return refreshSupport.RefreshAsset(ctx, input.Request, input.WorkspaceID, input.Asset, input.Assets, input.Edges)
			}),
			Broker:           runtime.broker,
			CSRFToken:        routes.accessModule.CSRFToken,
			CurrentRoleLabel: routes.accessModule.CurrentRoleLabel,
			ChromeOptions: func(r *http.Request) []ui.ChromeOption {
				return []ui.ChromeOption{routes.agentModule.ChromeOption(r)}
			},
			CurrentCredential: func(r *http.Request) (accessmodule.APICredential, bool) {
				return accessmodule.APICredentialFromContext(r.Context())
			},
			AuthorizeObject: routes.accessModule.AuthorizeObject,
		})
		if err != nil {
			return fmt.Errorf("build workspace module: %w", err)
		}
		persistence.workspaceAssetCatalog = nil
	}
	if routes.deploymentModule == nil {
		config := moduleWorkflow.deploymentConfig
		config.Logger = platform.logger
		config.InstanceEnvironment = policy.defaultEnvironment
		config.CurrentPrincipal = func(r *http.Request) (deploymentmodule.Principal, bool) {
			principal, ok := routes.accessModule.CurrentPrincipal(r)
			return deploymentmodule.Principal{ID: principal.ID}, ok
		}
		config.Jobs = deploymentmodule.JobConfig{
			Reconcile: func(ctx context.Context) error {
				if routes.refreshModule == nil {
					return nil
				}
				return routes.refreshModule.Reconcile(ctx)
			},
			Events: platform.asyncJobs,
			Logger: platform.logger,
		}
		config.API = deploymentmodule.APIConfig{Releases: routes.releaseModule.DeploymentLinkage(), Jobs: platform.asyncJobs}
		config.PublicationAuthorization = deploymentmodule.PublicationAuthorizationConfig{
			States: persistence.servingStateRepo, AuthorizeObject: routes.accessModule.AuthorizeObject,
			Bypass: func(actor string) bool {
				return (platform.auth == nil || platform.auth.DevBypass()) && actor == accessmodule.LocalDeveloperPrincipal().ID
			},
		}
		var err error
		routes.deploymentModule, err = deploymentmodule.Build(ctx, config)
		if err != nil {
			return fmt.Errorf("build deployment module: %w", err)
		}
	}
	if routes.dashboardModule == nil {
		var err error
		routes.dashboardModule, err = dashboardmodule.Build(ctx, dashboardmodule.Config{
			Database: database,
			HTTP: dashboardmodule.HTTPConfig{
				Metrics: runtime.metrics,
				MetricsForWorkspace: func(workspaceID string) (QueryMetrics, bool) {
					return metricsForWorkspace(routes, runtime, platform, policy, workspaceID)
				},
				Admission: workloadController(routes, runtime, platform, policy), Broker: runtime.broker, Logger: platform.logger,
				Telemetry: platform.telemetry,
				CurrentPrincipalID: func(r *http.Request) string {
					principal, ok := accessmodule.PrincipalFromContext(r.Context())
					if !ok {
						return ""
					}
					return principal.ID
				},
				AuthorizeListObject: func(ctx context.Context, principalID string, object accessmodule.ObjectRef) (bool, error) {
					return authorizeListObject(routes, runtime, platform, policy, ctx, principalID, object)
				},
				CSRFToken:        routes.accessModule.CSRFToken,
				ChatChromeSignal: routes.agentModule.ChromeSignal,
				Environment: func(r *http.Request) string {
					return string(requestServingEnvironment(routes, runtime, platform, policy, r))
				},
				DataRefreshedAt: func(ctx context.Context, workspaceID, environment, modelID string) string {
					if routes.refreshModule == nil {
						return ""
					}
					version, ok, err := routes.refreshModule.DataVersion(ctx, workspaceID, environment, modelID)
					if err != nil || !ok {
						return ""
					}
					return version.RefreshedAt.Format(time.RFC3339)
				},
				AgentBootstrap: func(r *http.Request, workspaceID string) ui.ChatViewState {
					return routes.agentModule.HTTP().DashboardBootstrap(r, workspaceID)
				},
			},
			Semantic: dashboardmodule.SemanticConfig{
				Metrics: runtime.metrics,
				MetricsForWorkspace: func(workspaceID string) (QueryMetrics, bool) {
					return metricsForWorkspace(routes, runtime, platform, policy, workspaceID)
				},
				CurrentPrincipalID: func(r *http.Request) string {
					principal, ok := accessmodule.PrincipalFromContext(r.Context())
					if !ok {
						return ""
					}
					return principal.ID
				},
				AuthorizeListObject: func(ctx context.Context, principalID string, object accessmodule.ObjectRef) (bool, error) {
					return authorizeListObject(routes, runtime, platform, policy, ctx, principalID, object)
				},
			},
			PublicTelemetry: dashboardmodule.PublicTelemetry{
				DocumentObserved: platform.telemetry.PublicDocumentObserved,
				StreamStarted:    platform.telemetry.PublicStreamStarted,
				CommandObserved:  platform.telemetry.PublicCommandObserved,
			},
			Logger:    platform.logger,
			Trace:     runtime.pageStreamTrace,
			PublicURL: storage.publicURL,
			CurrentActor: func(r *http.Request) string {
				principal, ok := accessmodule.PrincipalFromContext(r.Context())
				if !ok {
					return ""
				}
				return principal.ID
			},
			RuntimeMetrics: runtime.metrics, DefaultWorkspaceID: policy.defaultWorkspaceID,
			ServingSnapshot: func(ctx context.Context, requestedWorkspaceID string) (string, error) {
				if routes.workspaceModule == nil {
					return "", nil
				}
				return routes.workspaceModule.ActiveServingStateID(ctx, workspaceID(routes, runtime, platform, policy, requestedWorkspaceID))
			},
		})
		if err != nil {
			return fmt.Errorf("build dashboard module: %w", err)
		}
	}
	if routes.agentModule == nil {
		var err error
		routes.agentModule, err = agentmodule.Build(ctx, agentmodule.Config{
			Database: database, Model: moduleWorkflow.agentConfig,
			Service: moduleWorkflow.agent, Jobs: platform.asyncJobs, DefaultWorkspaceID: policy.defaultWorkspaceID,
			RunWorkloadClass: string(workloadmodule.BackgroundClass), GlobalWorkspaceID: workloadmodule.GlobalWorkspace,
			Search: routes.workspaceModule,
			Environment: func(r *http.Request) string {
				return string(requestServingEnvironment(routes, runtime, platform, policy, r))
			},
			DashboardMetrics: func(workspaceID string) (QueryMetrics, bool) {
				return metricsForWorkspace(routes, runtime, platform, policy, workspaceID)
			},
			AuthorizeAnyObject:       routes.accessModule.AuthorizeAnyObject,
			SkipContextAuthorization: platform.auth == nil,
			RecordAudit:              routes.accessModule.RecordAudit,
			EnableSystemPrompt:       runtime.persistenceConfigured,
			Logger:                   platform.logger,
			MCPProtect:               routes.accessModule.ProtectMCP,
			MCPScope: func(r *http.Request) (agentmodule.Scope, bool) {
				identity, ok := routes.accessModule.MCPIdentity(r)
				if !ok {
					return agentmodule.Scope{}, false
				}
				scope := agentmodule.Scope{
					PrincipalID: identity.PrincipalID, DevAuthBypass: identity.DevBypass,
					Credential: agentmodule.CredentialScope{
						WorkspaceID: identity.Credential.Token.WorkspaceID,
						Restricted:  identity.Restricted,
					},
				}
				for _, privilege := range identity.Credential.Token.Privileges {
					scope.Credential.Privileges = append(scope.Credential.Privileges, string(privilege))
				}
				return scope, true
			},
			DispatchAPIGen: func(scope agentmodule.Scope, operationID string, writer http.ResponseWriter, request *http.Request) bool {
				principal := accessmodule.Principal{ID: scope.PrincipalID, DevBypass: scope.DevAuthBypass}
				if platform.auth == nil {
					principal = accessmodule.LocalDeveloperPrincipal()
				}
				ctx := accessmodule.WithPrincipal(request.Context(), principal)
				if scope.Credential.Restricted || scope.Credential.WorkspaceID != "" || len(scope.Credential.Privileges) > 0 {
					ctx = accessmodule.WithAPICredential(ctx, accessmodule.AgentAPICredential(
						scope.PrincipalID, scope.Credential.WorkspaceID, scope.Credential.Privileges,
					))
				}
				request = request.WithContext(ctx)
				if apiDispatcher == nil {
					return false
				}
				return apigenapi.DispatchAPIGenOperation(operationID, apiDispatcher, apiprotocol.TransportErrorResponder{Logger: platform.logger}, writer, request)
			},
			HTTP: agentmodule.HTTPConfig{
				Settings: persistence.agentSettings, Broker: runtime.broker,
				CSRFToken:        routes.accessModule.CSRFToken,
				CurrentRoleLabel: routes.accessModule.CurrentRoleLabel,
				CurrentPrincipal: func(r *http.Request) (agentmodule.Principal, bool) {
					if platform.auth == nil {
						return agentmodule.Principal{}, false
					}
					principal, ok := platform.auth.Principal(r)
					return agentmodule.Principal{ID: principal.ID, DevAuthBypass: principal.DevBypass}, ok
				},
				CurrentCredential: func(r *http.Request) (accessmodule.APICredential, bool) {
					if platform.auth == nil {
						return accessmodule.APICredential{}, false
					}
					return platform.auth.APICredential(r)
				},
			},
		})
		if err != nil {
			return fmt.Errorf("build agent module: %w", err)
		}
	}
	if routes.refreshModule == nil {
		if err := configureRefreshModule(routes, runtime, platform, policy, ctx, nil, persistence, moduleWorkflow, storage); err != nil {
			return err
		}
	}
	if routes.adminModule == nil {
		var accessReader adminmodule.AccessReader
		if reader := routes.accessModule.AdminReader(); reader != nil {
			accessReader = reader
		}
		currentAdminPrincipal := func(r *http.Request) (adminmodule.Principal, bool) {
			principal, ok := routes.accessModule.CurrentPrincipal(r)
			return adminmodule.Principal{
				ID: principal.ID, Email: principal.Email, DisplayName: principal.DisplayName, DevBypass: principal.DevBypass,
			}, ok
		}
		var err error
		routes.adminModule, err = adminmodule.Build(ctx, adminmodule.Config{
			Catalog: func() catalog.Catalog {
				return runtime.metrics.Catalog()
			},
			Access: accessReader,
			AgentDetails: func(ctx context.Context) (api.AdminAgentResponse, error) {
				return routes.agentModule.HTTP().AdminDetails(ctx)
			},
			QueryAuditReader: runtime.queryAuditProvider,
			CSRFToken:        routes.accessModule.CSRFToken,
			CurrentPrincipal: currentAdminPrincipal,
			CurrentCredential: func(r *http.Request) (accessmodule.APICredential, bool) {
				if platform.auth == nil {
					return accessmodule.APICredential{}, false
				}
				return platform.auth.APICredential(r)
			},
			AuthorizeAnyWorkspace: routes.accessModule.AuthorizeAnyWorkspace,
			Publications:          routes.dashboardModule,
			DefaultWorkspaceID:    policy.defaultWorkspaceID,
			AuthConfigured:        platform.auth != nil,
			AccessConfigured:      accessReader != nil,
			Storage: adminmodule.StorageConfig{
				CatalogPath: storage.duckLakeCatalogPath, DataPath: storage.duckLakeDataPath,
				Environment: policy.defaultEnvironment, ControlPlane: persistence.adminDatabase,
				Analytics: runtime.analyticsModule.AdminResources(), Admitter: workloadController(routes, runtime, platform, policy),
			},
			CurrentRoleLabel: func(r *http.Request) string {
				principal, ok := currentAdminPrincipal(r)
				return adminmodule.RoleLabel(platform.auth != nil, principal, ok)
			},
			ChromeOption: routes.agentModule.ChromeOption,
			EnsureClientID: func(w http.ResponseWriter, r *http.Request) {
				_ = pagestream.EnsureClientID(w, r)
			},
			Broker: runtime.broker,
		})
		if err != nil {
			return fmt.Errorf("build admin module: %w", err)
		}
	}
	if routes.managedDataModule == nil {
		var err error
		routes.managedDataModule, err = manageddatamodule.Build(ctx, manageddatamodule.Config{
			Disabled:    true,
			Environment: policy.defaultEnvironment, Jobs: platform.asyncJobs,
			CurrentPrincipal: func(r *http.Request) (manageddatamodule.Principal, bool) {
				if platform.auth == nil {
					return manageddatamodule.Principal{}, false
				}
				principal, ok := platform.auth.Principal(r)
				return manageddatamodule.Principal{ID: principal.ID}, ok
			},
		})
		if err != nil {
			return fmt.Errorf("build managed data module: %w", err)
		}
	}
	objects, err := routes.workspaceModule.SecurableObjects(ctx, policy.defaultWorkspaceID)
	if err != nil {
		return fmt.Errorf("resolve workspace securables: %w", err)
	}
	if err := routes.accessModule.RegisterSecurables(ctx, objects); err != nil {
		return fmt.Errorf("register workspace securables: %w", err)
	}
	apiDispatcher = &apiGenDispatcher{
		accessModule: routes.accessModule, agentModule: routes.agentModule,
		dashboardModule: routes.dashboardModule, deploymentModule: routes.deploymentModule,
		managedDataModule: routes.managedDataModule, refreshModule: routes.refreshModule,
		releaseModule: routes.releaseModule, workspaceModule: routes.workspaceModule,
		defaultEnvironment: policy.defaultEnvironment, managedDataTus: policy.managedDataTus,
		queryAuditEvents: runtime.queryAuditEvents,
	}
	apiGenAuthorizer, err := routes.accessModule.APIGenAuthorizer(accessmodule.APIGenObjectResolvers{
		Dashboard:      dashboardmodule.DashboardObjectRefs,
		SemanticModel:  dashboardmodule.SemanticDatasetObjectRefs,
		WorkspaceAsset: workspacemodule.AssetObjectRefs,
	})
	if err != nil {
		return fmt.Errorf("build APIGen authorizer: %w", err)
	}
	platform.apiGenHandler, err = apiapigenruntime.Build(
		apiGenAuthorizer,
		apiDispatcher,
		apiprotocol.TransportErrorResponder{Logger: platform.logger},
	)
	if err != nil {
		return fmt.Errorf("build APIGen transport: %w", err)
	}
	configurePageStream(routes, runtime, platform, policy)
	platform.health = observability.NewHealth(observability.HealthConfig{
		Platform: func(ctx context.Context) error {
			if runtime.platformHealth == nil {
				return errors.New("platform store is missing")
			}
			return runtime.platformHealth.Ping(ctx)
		},
		Analytics: func() error {
			if runtime.analyticsModule == nil {
				return nil
			}
			return runtime.analyticsModule.Healthy()
		},
		Checks: map[string]func(context.Context) error{
			"mapAssets": func(ctx context.Context) error {
				if routes.dashboardAssets == nil {
					return nil
				}
				return routes.dashboardAssets.Verify(ctx)
			},
		},
		ActiveWorkspaces: routes.workspaceModule.ActiveRuntimeWorkspaces,
		RuntimeReady:     routes.dashboardModule.RuntimeReady,
	})
	platform.workers = platformlifecycle.New(
		platformlifecycle.Component{Start: routes.refreshModule.Start, Stop: routes.refreshModule.Stop},
		platformlifecycle.Component{
			Start: func(ctx context.Context) error { routes.managedDataModule.Start(ctx); return nil },
			Stop:  routes.managedDataModule.Stop,
		},
		platformlifecycle.Component{Start: routes.dashboardModule.Start, Stop: routes.dashboardModule.Stop},
		platformlifecycle.Component{Start: platform.jobModule.Start, Stop: platform.jobModule.Stop},
	)
	return nil
}

func StartBackgroundJobs(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, ctx context.Context) error {
	if platform == nil || platform.workers == nil {
		return nil
	}
	return platform.workers.Start(ctx)
}

func StopBackgroundJobs(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, ctx context.Context) error {
	if platform == nil || platform.workers == nil {
		return nil
	}
	return platform.workers.Stop(ctx)
}

func workspaceReadModel(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, persistence persistenceInputs) (workspacemodule.ReadModel, error) {
	return persistence.workspaceReadModel, nil
}

func authorizeListObject(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, ctx context.Context, principalID string, object accessmodule.ObjectRef) (bool, error) {
	if platform.auth == nil {
		return true, nil
	}
	if strings.TrimSpace(principalID) == "" {
		return false, nil
	}
	return routes.accessModule.AuthorizeObject(ctx, principalID, accessmodule.PrivilegeViewItem, object)
}

func metricsForWorkspace(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, workspaceID string) (QueryMetrics, bool) {
	if workspaceID == "" {
		return nil, false
	}
	if provider, ok := runtime.metrics.(workspaceMetrics); ok {
		return provider.MetricsForWorkspace(workspaceID)
	}
	if runtime.metrics == nil {
		return nil, false
	}
	if policy.defaultWorkspaceID != "" && workspaceID == policy.defaultWorkspaceID {
		return runtime.metrics, true
	}
	catalog := runtime.metrics.Catalog()
	if catalog.Workspace.ID == "" || catalog.Workspace.ID == workspaceID {
		return runtime.metrics, true
	}
	return nil, false
}
