package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	accessmodule "github.com/Yacobolo/leapview/internal/access/module"
	agentmodule "github.com/Yacobolo/leapview/internal/agent/module"
	analyticsmodule "github.com/Yacobolo/leapview/internal/analytics/module"
	apihttpmiddleware "github.com/Yacobolo/leapview/internal/api/httpmiddleware"
	"github.com/Yacobolo/leapview/internal/config"
	dashboardmodule "github.com/Yacobolo/leapview/internal/dashboard/module"
	deploymentmodule "github.com/Yacobolo/leapview/internal/deployment/module"
	manageddatamodule "github.com/Yacobolo/leapview/internal/manageddata/module"
	"github.com/Yacobolo/leapview/internal/platform"
	jobsmodule "github.com/Yacobolo/leapview/internal/platform/jobs/module"
	"github.com/Yacobolo/leapview/internal/platform/transaction"
	refreshmodule "github.com/Yacobolo/leapview/internal/refresh/module"
	releasemodule "github.com/Yacobolo/leapview/internal/release/module"
	runtimehostmodule "github.com/Yacobolo/leapview/internal/runtimehost/module"
	"github.com/Yacobolo/leapview/internal/securefs"
	servingstatemodule "github.com/Yacobolo/leapview/internal/servingstate/module"
	workloadmodule "github.com/Yacobolo/leapview/internal/workload/module"
	workspacemodule "github.com/Yacobolo/leapview/internal/workspace/module"
)

// assemble constructs the complete process exactly once. CLI and other process
// entrypoints provide configuration but never construct capability adapters.
func assemble(ctx context.Context, cfg config.Config) (http.Handler, Lifecycle, cleanupFunc, error) {
	production := cfg.Production
	environment := servingstatemodule.NormalizeEnvironment(servingstatemodule.Environment(cfg.Environment))
	if strings.TrimSpace(cfg.Environment) == "" {
		if production {
			environment = servingstatemodule.Environment("prod")
		} else {
			environment = servingstatemodule.DefaultEnvironment
		}
	}
	return buildRuntime(ctx, cfg, production, environment)
}

func buildRuntime(ctx context.Context, cfg config.Config, production bool, environment servingstatemodule.Environment) (http.Handler, Lifecycle, cleanupFunc, error) {
	dashboardAssets, err := dashboardmodule.BuildAssets(ctx, cfg.MapAssetDir)
	if err != nil {
		return nil, nil, nil, err
	}
	cookieSecure, err := cfg.CookieSecure()
	if err != nil {
		return nil, nil, nil, err
	}
	var allowedHosts []string
	if production {
		allowedHosts, err = cfg.ProductionAllowedHosts()
	} else {
		allowedHosts, err = cfg.AllowedHostList()
	}
	if err != nil {
		return nil, nil, nil, err
	}
	duckLakeCatalogPath := cfg.DuckLakeCatalogPath()
	for _, dir := range []string{cfg.HomeDir, cfg.ArtifactDir(), cfg.DuckDBDirPath(), cfg.RuntimeDir(), cfg.DuckLakeDataDir(), filepath.Dir(duckLakeCatalogPath)} {
		if err := securefs.EnsurePrivateDir(dir); err != nil {
			return nil, nil, nil, err
		}
	}
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, nil, nil, err
	}
	cleanup := &cleanupStack{}
	cleanup.Push("sqlite", func(context.Context) error { return store.Close() })
	fail := func(err error) (http.Handler, Lifecycle, cleanupFunc, error) {
		cleanupErr := cleanup.Close(context.WithoutCancel(ctx))
		return nil, nil, nil, errors.Join(err, cleanupErr)
	}
	if err := store.BindInstanceEnvironment(ctx, string(environment)); err != nil {
		return fail(err)
	}
	workloadConfig := cfg.WorkloadConfig()
	analyticsModule, err := analyticsmodule.Build(ctx, analyticsmodule.Config{
		Database: store.SQLDB(), RootDir: cfg.DuckDBDirPath(),
		CatalogPath: duckLakeCatalogPath, DataPath: cfg.DuckLakeDataDir(),
		MaxConnections: workloadConfig.MaxRunning, MemoryMaxBytes: cfg.DuckDBNodeMemoryMaxBytes,
		TempMaxBytes: cfg.DuckDBNodeTempMaxBytes, MaxThreads: cfg.DuckDBNodeMaxThreads,
		TempDir:             cfg.DuckDBTempDirPath(),
		RuntimeCacheEntries: cfg.QueryCacheRuntimeMaxEntries, RuntimeCacheBytes: cfg.QueryCacheRuntimeMaxBytes,
		WorkspaceCacheEntries: cfg.QueryCacheWorkspaceMaxEntries, WorkspaceCacheBytes: cfg.QueryCacheWorkspaceMaxBytes,
		NodeCacheEntries: cfg.QueryCacheNodeMaxEntries, NodeCacheBytes: cfg.QueryCacheNodeMaxBytes,
	})
	if err != nil {
		return fail(err)
	}
	cleanup.Push("analytics", func(context.Context) error { return analyticsModule.Close() })
	analyticsRuntimeResources := analyticsModule.RuntimeResources()
	var workspaceDirectory workspacemodule.Directory
	accessModule, err := accessmodule.Build(ctx, accessmodule.Config{
		Database: store.SQLDB(), Auth: accessAuthConfig(cfg, production, cookieSecure),
		WorkspaceID: platform.DefaultWorkspaceID,
		PublicURL:   firstConfigured(cfg.PublicURL, configuredListenURL(cfg.ListenAddr())), MCPIssuerURL: cfg.MCPOAuthIssuerURL,
		WorkspaceIDs: func(ctx context.Context) ([]string, error) {
			if workspaceDirectory == nil {
				return nil, nil
			}
			return workspaceDirectory.WorkspaceIDs(ctx)
		},
	})
	if err != nil {
		return fail(err)
	}
	accessSecurables := accessModule.Securables()
	workspaceDirectory, err = workspacemodule.BuildDirectory(store.SQLDB(), accessSecurables)
	if err != nil {
		return fail(err)
	}
	if !production {
		if err := accessModule.SeedLocalDeveloperPlatformAdmin(ctx); err != nil {
			return fail(err)
		}
	}
	servingStateRepo, err := servingstatemodule.Build(ctx, servingstatemodule.Config{Database: store.SQLDB()})
	if err != nil {
		return fail(err)
	}
	workloadController, err := workloadmodule.Build(ctx, workloadmodule.Config{Policy: workloadConfig})
	if err != nil {
		return fail(err)
	}
	cleanup.Push("workload", func(context.Context) error {
		workloadController.Close()
		return nil
	})
	jobModule, err := jobsmodule.Build(ctx, jobsmodule.Config{
		Database: store.SQLDB(), Admission: workloadController,
		LeaseTimeout: cfg.RefreshJobLeaseTimeout, Logger: slog.Default(),
	})
	if err != nil {
		return fail(err)
	}
	managedDataModule, err := manageddatamodule.Build(ctx, manageddatamodule.Config{
		Database: store.SQLDB(), Product: cfg, ServingStates: servingStateRepo,
		Environment: string(environment),
		CurrentPrincipal: func(r *http.Request) (manageddatamodule.Principal, bool) {
			auth := accessModule.Auth()
			if auth == nil {
				return manageddatamodule.Principal{}, false
			}
			principal, ok := auth.Principal(r)
			return manageddatamodule.Principal{ID: principal.ID}, ok
		},
		Jobs: jobModule,
		Worker: manageddatamodule.MaintenanceWorkerConfig{
			Interval: cfg.ManagedDataGCInterval,
			Acquire: func(ctx context.Context) (manageddatamodule.MaintenanceLease, error) {
				return workloadController.Acquire(ctx, workloadmodule.MaintenanceRequest("managed_data.collect"))
			},
			Logger: slog.Default(),
		},
	})
	if err != nil {
		return fail(err)
	}
	releaseModule, err := releasemodule.Build(ctx, releasemodule.Config{
		Database: store.SQLDB(),
		States:   servingStateRepo, Workspaces: workspaceDirectory,
		ManagedDataPins: managedDataModule.BindingValidation(), ManagedDataHook: managedDataModule.BindingValidation(),
		ArtifactDirectory: cfg.ArtifactDir(), Environment: environment,
		API: releasemodule.APIConfig{
			CurrentPrincipal: func(r *http.Request) (releasemodule.Principal, bool) {
				auth := accessModule.Auth()
				if auth == nil {
					principal := accessmodule.LocalDeveloperPrincipal()
					return releasemodule.Principal{ID: principal.ID}, true
				}
				principal, ok := auth.Principal(r)
				return releasemodule.Principal{ID: principal.ID}, ok
			},
			Jobs: jobModule,
		},
	})
	if err != nil {
		return fail(err)
	}
	managedDataResolution := managedDataModule.RuntimeResolution()
	if managedDataResolution == nil {
		return fail(errors.New("managed-data runtime resolver is required"))
	}
	managedDataResolver := runtimehostmodule.NewManagedDataResolver(managedDataResolution)
	if err := refreshmodule.Recover(ctx, store.SQLDB(), string(environment)); err != nil {
		return fail(err)
	}
	retention := servingstatemodule.NewRetention(servingstatemodule.RetentionConfig{
		States: servingStateRepo, Snapshots: analyticsModule.RetentionSnapshots(),
		Admission: workloadController, Environment: string(environment),
		CatalogPath: duckLakeCatalogPath, DataPath: cfg.DuckLakeDataDir(),
	})
	if err := retention.Run(ctx, false); err != nil {
		return fail(err)
	}
	workspaceIDValues, err := workspaceDirectory.WorkspaceIDs(ctx)
	if err != nil {
		return fail(err)
	}
	workspaceIDs := make([]servingstatemodule.WorkspaceID, 0, len(workspaceIDValues))
	for _, workspaceID := range workspaceIDValues {
		workspaceIDs = append(workspaceIDs, servingstatemodule.WorkspaceID(workspaceID))
	}
	runtimeHostModule, err := runtimehostmodule.Build(ctx, runtimehostmodule.Config{
		States:       servingStateRepo,
		WorkspaceIDs: workspaceIDs,
		Environment:  environment,
		ManagedData:  managedDataResolver,
		OnDrained: func(_ servingstatemodule.ID, _ int64, protected []int64) {
			go func() {
				if err := retention.RunWithProtected(context.Background(), false, protected); err != nil {
					slog.Default().Warn("storage retention cleanup failed after runtime drain", "error", err)
				}
			}()
		},
		Factory: runtimehostmodule.NewFactory(runtimehostmodule.FactoryConfig{
			DuckDBDir: cfg.DuckDBDirPath(), RuntimeDir: cfg.RuntimeDir(),
			DashboardRuntime: dashboardmodule.NewRuntimeFactory(dashboardmodule.RuntimeFactoryConfig{
				Resources: analyticsRuntimeResources,
				MaxRows:   cfg.QueryResultMaxRows, MaxBytes: cfg.QueryResultMaxBytes,
			}),
		}),
	})
	if err != nil {
		return fail(err)
	}
	cleanup.Push("runtime-host", func(context.Context) error { return runtimeHostModule.Close() })
	deploymentRuntime, err := deploymentmodule.NewRuntime(runtimeHostModule)
	if err != nil {
		return fail(err)
	}
	deploymentConfig := deploymentmodule.Config{
		Database: store.SQLDB(), States: servingStateRepo, Runtime: deploymentRuntime,
		ManagedData: managedDataResolver, DeploymentMetadata: managedDataModule.DeploymentMetadata(),
		ActivationHooks: deploymentmodule.ActivationHooks{
			ApplyAccessSnapshot: accessmodule.ApplySnapshot,
			ReconcilePublications: func(ctx context.Context, tx transaction.Transaction, input deploymentmodule.PublicationActivationInput) error {
				return dashboardmodule.ReconcilePublications(ctx, tx, dashboardmodule.PublicationActivationInput{
					ProjectID: input.ProjectID, WorkspaceID: input.WorkspaceID,
					ServingStateID: input.ServingStateID, ActorID: input.ActorID,
					Publications: input.Publications,
				})
			},
		},
	}
	runtimeMetrics := dashboardmodule.NewDynamicRuntimeMetrics("", func(workspaceID string) runtimehostmodule.Provider {
		return runtimeHostModule.ProviderForWorkspace(servingstatemodule.WorkspaceID(workspaceID))
	})
	auth := accessModule.Auth()
	rateLimits := apihttpmiddleware.ProductionRateLimitConfig()
	rateLimits.Enabled = production && cfg.RateLimitingEnabled()
	rateLimits.UseRealIP = cfg.RateLimitingUsesRealIP()
	routes, runtime, platformServices, policy, err := buildApplicationSurfaces(ctx, runtimeMetrics,
		dataAssemblyInputs{
			Database: store.SQLDB(), PlatformHealth: store, AdminDatabase: store.SQLDB(),
			ServingStateRepo: servingStateRepo, StorageRetention: retention,
			WorkspaceDirectory: workspaceDirectory,
		},
		capabilityAssemblyInputs{
			AnalyticsModule: analyticsModule, DashboardAssets: dashboardAssets,
			ReleaseModule: releaseModule, JobModule: jobModule,
			AccessModule: accessModule, ManagedDataModule: managedDataModule,
		},
		workflowAssemblyInputs{
			AgentSettings: store,
			AgentConfig:   agentmodule.ModelConfig{APIKey: cfg.AgentAPIKey, BaseURL: cfg.AgentBaseURL, Model: cfg.AgentModel},
			Auth:          auth, Reloader: runtimeHostModule, Workload: workloadController,
			ManagedDataValidation: managedDataModule.BindingValidation(),
			ManagedDataResolver:   managedDataResolver,
			DeploymentConfig:      deploymentConfig,
		},
		runtimeAssemblyInputs{
			DuckLakeCatalogPath: duckLakeCatalogPath, DuckLakeDataPath: cfg.DuckLakeDataDir(),
			DefaultEnvironment: string(environment), SCIMBearerToken: cfg.SCIMBearerToken,
			MetricsBearerToken: cfg.MetricsBearerToken, AllowedHosts: allowedHosts,
		},
		httpAssemblyInputs{
			PublicURL:       firstConfigured(cfg.PublicURL, configuredListenURL(cfg.ListenAddr())),
			RateLimits:      rateLimits,
			SecurityHeaders: apihttpmiddleware.SecurityHeaders(production && cfg.HSTSEnabled(cookieSecure)),
			RequestLogging:  production && cfg.RequestLoggingEnabled(), Logger: slog.Default(),
			JobLeaseTimeout: cfg.RefreshJobLeaseTimeout, ManagedDataTus: managedDataModule.TusHandler(),
		},
	)
	if err != nil {
		return fail(err)
	}
	handler := Routes(routes, runtime, platformServices, policy)
	lifecycle := newRuntimeLifecycle(platformServices.workers, runtime.analyticsModule, runtime.workloads)
	return handler, lifecycle, cleanup.Close, nil
}

func firstConfigured(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func configuredListenURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}

func accessAuthConfig(cfg config.Config, production, cookieSecure bool) accessmodule.AuthConfig {
	if !production {
		return accessmodule.AuthConfig{DevBypass: true, DevAPIToken: cfg.DevAPIToken, CSRFKey: cfg.CSRFKey}
	}
	providers := []accessmodule.OIDCProviderConfig{}
	if cfg.OIDCConfigured() {
		providers = append(providers, accessmodule.OIDCProviderConfig{
			ID: cfg.OIDCProviderID, IssuerURL: cfg.OIDCIssuerURL, ClientID: cfg.OIDCClientID,
			ClientSecret: cfg.OIDCSecret, RedirectURL: cfg.OIDCCallbackURL, Scopes: cfg.OIDCScopesList(),
		})
	}
	return accessmodule.AuthConfig{
		DevBypass: cfg.DevAuthBypass, DevAPIToken: cfg.DevAPIToken, APITokenOnly: cfg.APITokenOnlyAuth,
		LocalAuth: cfg.LocalAuth, AzureClientID: cfg.AzureClientID, AzureSecret: cfg.AzureSecret,
		AzureCallback: cfg.AzureCallbackURL, AzureTenant: cfg.AzureTenant, CSRFKey: cfg.CSRFKey,
		CookieSecure: cookieSecure, BootstrapTenant: cfg.AzureTenant, OIDCProviders: providers,
	}
}
