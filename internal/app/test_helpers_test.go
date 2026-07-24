package app

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	accessmodule "github.com/Yacobolo/leapview/internal/access/module"
	agentmodule "github.com/Yacobolo/leapview/internal/agent/module"
	analyticsmodule "github.com/Yacobolo/leapview/internal/analytics/module"
	apihttpmiddleware "github.com/Yacobolo/leapview/internal/api/httpmiddleware"
	dashboardmodule "github.com/Yacobolo/leapview/internal/dashboard/module"
	deploymentmodule "github.com/Yacobolo/leapview/internal/deployment/module"
	manageddatamodule "github.com/Yacobolo/leapview/internal/manageddata/module"
	jobsmodule "github.com/Yacobolo/leapview/internal/platform/jobs/module"
	refreshmodule "github.com/Yacobolo/leapview/internal/refresh/module"
	releasemodule "github.com/Yacobolo/leapview/internal/release/module"
	"github.com/Yacobolo/leapview/internal/runtimehost"
	runtimehostmodule "github.com/Yacobolo/leapview/internal/runtimehost/module"
	servingstatemodule "github.com/Yacobolo/leapview/internal/servingstate/module"
	workspacemodule "github.com/Yacobolo/leapview/internal/workspace/module"
)

// assemblyConfig is deliberately test-only. Focused capability tests use it
// while they are moved beside their owners; production has no general
// dependency bag.
type assemblyConfig struct {
	Database              *sql.DB
	PlatformHealth        platformHealth
	AgentSettings         agentmodule.Settings
	AdminDatabase         *sql.DB
	ServingStateRepo      servingStateRepository
	StorageRetention      *servingstatemodule.Retention
	ManagedDataValidation refreshmodule.CandidateValidationHook
	ManagedDataResolver   runtimehostmodule.ManagedDataResolver
	WorkspaceRepo         workspacemodule.Repository
	WorkspaceDirectory    workspacemodule.Directory
	AssetCatalog          workspacemodule.AssetCatalogReader
	ReleaseModule         *releasemodule.Module
	JobModule             *jobsmodule.Module
	AccessRepo            accessmodule.Repository
	AccessModule          *accessmodule.Module
	Agent                 *agentmodule.Service
	AgentConfig           agentmodule.ModelConfig
	Auth                  *accessmodule.Auth
	Reloader              runtimeReloader
	DuckDBDir             string
	DuckLakeCatalogPath   string
	DuckLakeDataPath      string
	DefaultWorkspaceID    string
	DefaultEnvironment    string
	SCIMBearerToken       string
	MetricsBearerToken    string
	AllowedHosts          []string
	RateLimits            apihttpmiddleware.RateLimitConfig
	SecurityHeaders       apihttpmiddleware.SecurityHeadersConfig
	RequestBodyLimit      apihttpmiddleware.RequestBodyLimitConfig
	RequestLogging        bool
	Logger                *slog.Logger
	Workload              workloadControl
	JobLeaseTimeout       time.Duration
	ManagedDataModule     *manageddatamodule.Module
	DeploymentConfig      deploymentmodule.Config
	ManagedDataTus        http.Handler
	MCPOAuth              MCPOAuthConfig
	PublicURL             string
	RefreshPipelineClock  refreshmodule.Clock
	AnalyticsModule       *analyticsmodule.Module
	DashboardAssets       dashboardmodule.Assets
	QueryAudit            *analyticsmodule.QueryAuditSurface
}

// appTestHarness is a test fixture facade for legacy app-package tests.
// Production composition exposes only the final handler and lifecycle.
type appTestHarness struct {
	routes   capabilityRoutes
	runtime  runtimeServices
	platform platformServices
	policy   httpPolicy
}

func (s *appTestHarness) Routes() http.Handler {
	return Routes(&s.routes, &s.runtime, &s.platform, &s.policy)
}

func (s *appTestHarness) StartBackgroundJobs(ctx context.Context) error {
	return StartBackgroundJobs(&s.routes, &s.runtime, &s.platform, &s.policy, ctx)
}

func (s *appTestHarness) StopBackgroundJobs(ctx context.Context) error {
	return StopBackgroundJobs(&s.routes, &s.runtime, &s.platform, &s.policy, ctx)
}

func (s *appTestHarness) workloadController() workloadControl {
	return workloadController(&s.routes, &s.runtime, &s.platform, &s.policy)
}

func (s *appTestHarness) workspaceID(value string) string {
	return workspaceID(&s.routes, &s.runtime, &s.platform, &s.policy, value)
}

func (s *appTestHarness) requestServingEnvironment(r *http.Request) servingstatemodule.Environment {
	return requestServingEnvironment(&s.routes, &s.runtime, &s.platform, &s.policy, r)
}

func (s *appTestHarness) publicProtocolMiddleware(next http.Handler) http.Handler {
	return publicProtocolMiddleware(&s.routes, &s.runtime, &s.platform, &s.policy, next)
}

func (s *appTestHarness) metricsForWorkspace(workspaceID string) (QueryMetrics, bool) {
	return metricsForWorkspace(&s.routes, &s.runtime, &s.platform, &s.policy, workspaceID)
}

func assembleRuntime(metrics QueryMetrics, options assemblyConfig) *appTestHarness {
	server, err := assembleRuntimeChecked(context.Background(), metrics, options)
	if err != nil {
		panic(err)
	}
	return server
}

func newAppTestHarness(metrics QueryMetrics) *appTestHarness {
	return assembleRuntime(metrics, assemblyConfig{})
}

func apiGenDispatcherForTest(server *appTestHarness) apiGenDispatcher {
	return apiGenDispatcher{
		accessModule: server.routes.accessModule, agentModule: server.routes.agentModule,
		dashboardModule: server.routes.dashboardModule, deploymentModule: server.routes.deploymentModule,
		managedDataModule: server.routes.managedDataModule, refreshModule: server.routes.refreshModule,
		releaseModule: server.routes.releaseModule, workspaceModule: server.routes.workspaceModule,
		defaultEnvironment: server.policy.defaultEnvironment, managedDataTus: server.policy.managedDataTus,
		queryAuditEvents: server.runtime.queryAuditEvents,
	}
}

func assembleRuntimeChecked(ctx context.Context, metrics QueryMetrics, options assemblyConfig) (*appTestHarness, error) {
	if options.AccessModule == nil {
		var err error
		options.AccessModule, err = accessmodule.Build(ctx, accessmodule.Config{
			Database: options.Database, WorkspaceID: options.DefaultWorkspaceID,
			ExistingAuth: options.Auth, Auth: accessmodule.AuthConfig{Disabled: options.Auth == nil},
		})
		if err != nil {
			return nil, err
		}
	}
	routes, runtime, platform, policy, err := buildApplicationSurfaces(ctx, metrics,
		dataAssemblyInputs{
			Database: options.Database, PlatformHealth: options.PlatformHealth,
			AdminDatabase: options.AdminDatabase, ServingStateRepo: options.ServingStateRepo,
			StorageRetention: options.StorageRetention, WorkspaceReadModel: options.WorkspaceRepo,
			WorkspaceDirectory: options.WorkspaceDirectory, AssetCatalog: options.AssetCatalog,
			AccessRepo: options.AccessRepo,
		},
		capabilityAssemblyInputs{
			ReleaseModule: options.ReleaseModule, JobModule: options.JobModule,
			AccessModule: options.AccessModule, Agent: options.Agent,
			ManagedDataModule: options.ManagedDataModule, AnalyticsModule: options.AnalyticsModule,
			DashboardAssets: options.DashboardAssets,
		},
		workflowAssemblyInputs{
			AgentSettings: options.AgentSettings, ManagedDataValidation: options.ManagedDataValidation,
			ManagedDataResolver: options.ManagedDataResolver, AgentConfig: options.AgentConfig,
			Auth: options.Auth, Reloader: options.Reloader, Workload: options.Workload,
			DeploymentConfig: options.DeploymentConfig, RefreshPipelineClock: options.RefreshPipelineClock,
			QueryAudit: options.QueryAudit,
		},
		runtimeAssemblyInputs{
			DuckDBDir: options.DuckDBDir, DuckLakeCatalogPath: options.DuckLakeCatalogPath,
			DuckLakeDataPath: options.DuckLakeDataPath, DefaultWorkspaceID: options.DefaultWorkspaceID,
			DefaultEnvironment: options.DefaultEnvironment, SCIMBearerToken: options.SCIMBearerToken,
			MetricsBearerToken: options.MetricsBearerToken, AllowedHosts: options.AllowedHosts,
		},
		httpAssemblyInputs{
			RateLimits: options.RateLimits, SecurityHeaders: options.SecurityHeaders,
			RequestBodyLimit: options.RequestBodyLimit, RequestLogging: options.RequestLogging,
			Logger: options.Logger, JobLeaseTimeout: options.JobLeaseTimeout,
			ManagedDataTus: options.ManagedDataTus, MCPOAuth: options.MCPOAuth,
			PublicURL: options.PublicURL,
		},
	)
	if err != nil {
		return nil, err
	}
	return &appTestHarness{
		routes: *routes, runtime: *runtime, platform: *platform, policy: *policy,
	}, nil
}

func NewRuntimeMetrics(provider runtimehost.Provider, workspaceID string) QueryMetrics {
	return dashboardmodule.NewRuntimeMetrics(provider, workspaceID)
}

func NewDynamicRuntimeMetrics(defaultWorkspaceID string, factory func(string) runtimehost.Provider) QueryMetrics {
	return dashboardmodule.NewDynamicRuntimeMetrics(defaultWorkspaceID, factory)
}

func NewMultiWorkspaceMetrics(defaultWorkspaceID string, workspaces map[string]QueryMetrics) QueryMetrics {
	return dashboardmodule.NewMultiWorkspaceMetrics(defaultWorkspaceID, workspaces)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
