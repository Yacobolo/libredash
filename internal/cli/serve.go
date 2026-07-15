package cli

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	oidcauth "github.com/Yacobolo/libredash/internal/access/oidc"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/agent"
	agentsqlite "github.com/Yacobolo/libredash/internal/agent/sqlite"
	materializesqlite "github.com/Yacobolo/libredash/internal/analytics/materialize/sqlite"
	"github.com/Yacobolo/libredash/internal/app"
	"github.com/Yacobolo/libredash/internal/config"
	projectdeployment "github.com/Yacobolo/libredash/internal/deployment"
	deploymentapiadapter "github.com/Yacobolo/libredash/internal/deployment/apiadapter"
	deploymenthttp "github.com/Yacobolo/libredash/internal/deployment/http"
	deploymentsqlite "github.com/Yacobolo/libredash/internal/deployment/sqlite"
	"github.com/Yacobolo/libredash/internal/execution"
	manageddataapiadapter "github.com/Yacobolo/libredash/internal/manageddata/apiadapter"
	manageddatahttp "github.com/Yacobolo/libredash/internal/manageddata/http"
	manageddataresolver "github.com/Yacobolo/libredash/internal/manageddata/resolver"
	"github.com/Yacobolo/libredash/internal/manageddata/s3multipart"
	manageddatasqlite "github.com/Yacobolo/libredash/internal/manageddata/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	"github.com/Yacobolo/libredash/internal/securefs"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	storagemaintenance "github.com/Yacobolo/libredash/internal/storage/maintenance"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
	"github.com/spf13/cobra"
)

const defaultHTTPServerShutdownTimeout = 15 * time.Second

func serveCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the LibreDash HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.environment = serveEnvironmentFlagValue(cmd.Flags().Changed("environment"), opts.environment)
			return runServe(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.addr, "addr", "", "listen address; defaults to the configured address")
	cmd.Flags().StringVar(&opts.environment, "environment", "", "serving environment; defaults to prod in production and dev otherwise")
	cmd.Flags().BoolVar(&opts.production, "production", false, "serve active serving state from the platform DB")
	return cmd
}

func runServe(ctx context.Context, opts *rootOptions) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	production := serveProductionMode(cfg, *opts)
	cfg.Production = production
	addr := opts.addr
	if addr == "" {
		addr = cfg.ListenAddr()
	}
	if err := cfg.Validate(config.ProfileServe); err != nil {
		return err
	}
	environment := serveEnvironment(production, opts.environment)
	server, cleanup, err := servingStateBackedServer(ctx, cfg, production, environment)
	if err != nil {
		return err
	}
	defer cleanup()
	serveCtx, stopServe := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopServe()
	server.StartBackgroundJobs(serveCtx)
	slog.Info("LibreDash listening", "url", listenURL(addr), "environment", environment)
	err = runHTTPServer(serveCtx, productionHTTPServer(addr, server.Routes()))
	stopServe()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultHTTPServerShutdownTimeout)
	defer cancel()
	if stopErr := server.StopBackgroundJobs(shutdownCtx); err == nil && stopErr != nil {
		err = stopErr
	}
	return err
}

func serveProductionMode(cfg config.Config, opts rootOptions) bool {
	return opts.production || cfg.Production
}

func serveEnvironment(production bool, value string) servingstate.Environment {
	if strings.TrimSpace(value) != "" {
		return servingstate.NormalizeEnvironment(servingstate.Environment(value))
	}
	if production {
		return servingstate.Environment("prod")
	}
	return servingstate.DefaultEnvironment
}

func serveEnvironmentFlagValue(changed bool, value string) string {
	if !changed {
		return ""
	}
	return value
}

func listenURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}

func productionHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
}

func runHTTPServer(ctx context.Context, server *http.Server) error {
	if server == nil {
		return errors.New("http server is required")
	}
	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()
	select {
	case err := <-errCh:
		return err
	case <-signalCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultHTTPServerShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	}
}

func servingStateBackedServer(ctx context.Context, cfg config.Config, production bool, environment servingstate.Environment) (*app.Server, func(), error) {
	cookieSecure, err := cfg.CookieSecure()
	if err != nil {
		return nil, nil, err
	}
	var allowedHosts []string
	if production {
		allowedHosts, err = cfg.ProductionAllowedHosts()
	} else {
		allowedHosts, err = cfg.AllowedHostList()
	}
	if err != nil {
		return nil, nil, err
	}
	duckLakeCatalogPath := cfg.DuckLakeCatalogPath()
	for _, dir := range []string{cfg.HomeDir, cfg.ArtifactDir(), cfg.DuckDBDirPath(), cfg.RuntimeDir(), cfg.DuckLakeDataDir(), filepath.Dir(duckLakeCatalogPath)} {
		if err := securefs.EnsurePrivateDir(dir); err != nil {
			return nil, nil, err
		}
	}
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = store.Close() }
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	workspaceRepo := workspacesqlite.NewRepositoryWithSecurables(store.SQLDB(), accessRepo)
	if !production {
		if err := app.SeedLocalDeveloperPlatformAdmin(ctx, accessRepo); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	servingStateRepo := servingstatesqlite.NewRepository(store.SQLDB())
	managedDataRepo := manageddatasqlite.NewRepository(store.SQLDB())
	managedDataStorage, err := newManagedDataStorage(ctx, cfg)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	managedDataControl, err := newManagedDataControl(managedDataRepo, managedDataStorage, cfg)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	managedDataCollector, err := newManagedDataCollector(store.SQLDB(), managedDataStorage, cfg)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	managedDataRuntimeCollector, err := newManagedDataRuntimeCollector(managedDataStorage, cfg)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	managedDataResolver, err := manageddataresolver.New(managedDataRepo, servingStateRepo, managedDataStorage.materializer)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	var managedDataMultipart s3multipart.Coordinator
	var managedDataMultipartService *s3multipart.Service
	if managedDataStorage.s3 != nil {
		multipartService, multipartErr := s3multipart.New(managedDataRepo, managedDataStorage.s3, s3multipart.Config{Backend: "s3"})
		if multipartErr != nil {
			cleanup()
			return nil, nil, multipartErr
		}
		managedDataMultipart = multipartService
		managedDataMultipartService = multipartService
	}
	if err := materializesqlite.NewSQLRunRepository(store.SQLDB()).FailRunsForTerminalServingStates(ctx, "refresh did not complete"); err != nil {
		cleanup()
		return nil, nil, err
	}
	if _, err := storagemaintenance.Run(ctx, servingStateRepo, storagemaintenance.Options{
		RootDir:     cfg.HomeDir,
		CatalogPath: duckLakeCatalogPath,
		DataPath:    cfg.DuckLakeDataDir(),
		DryRun:      false,
	}); err != nil {
		cleanup()
		return nil, nil, err
	}
	agentRepo := agentsqlite.NewRepository(store.SQLDB())
	summaries, err := workspaceRepo.List(ctx)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	workspaceIDs := make([]servingstate.WorkspaceID, 0, len(summaries))
	for _, summary := range summaries {
		workspaceIDs = append(workspaceIDs, servingstate.WorkspaceID(summary.ID))
	}
	var registry *runtimehost.Registry
	registry = runtimehost.NewRegistryWithFactory(runtimehost.RegistryOptions{
		Repo:         servingStateRepo,
		WorkspaceIDs: workspaceIDs,
		Environment:  environment,
		ManagedData:  managedDataResolver,
		OnDrained: func(servingstate.ID, int64) {
			go func() {
				protected := []int64(nil)
				if registry != nil {
					protected = registry.LeasedSnapshots()
				}
				if _, err := storagemaintenance.Run(context.Background(), servingStateRepo, storagemaintenance.Options{
					RootDir:                      cfg.HomeDir,
					CatalogPath:                  duckLakeCatalogPath,
					DataPath:                     cfg.DuckLakeDataDir(),
					AdditionalProtectedSnapshots: protected,
					DryRun:                       false,
				}); err != nil {
					slog.Default().Warn("storage retention cleanup failed after runtime drain", "error", err)
				}
			}()
		},
		Factory: servingStateRuntimeFactory{
			duckDBDir:        cfg.DuckDBDirPath(),
			runtimeDir:       cfg.RuntimeDir(),
			catalogPath:      duckLakeCatalogPath,
			duckLakeDataPath: cfg.DuckLakeDataDir(),
		},
	})
	if err := registry.Reload(ctx); err != nil {
		cleanup()
		return nil, nil, err
	}
	deploymentRuntime, err := projectdeployment.NewRegistryRuntime(registry)
	if err != nil {
		_ = registry.Close()
		cleanup()
		return nil, nil, err
	}
	deploymentService, err := projectdeployment.New(deploymentsqlite.NewRepository(store.SQLDB()), servingStateRepo, deploymentRuntime, managedDataResolver)
	if err != nil {
		_ = registry.Close()
		cleanup()
		return nil, nil, err
	}
	deploymentAPI, err := deploymentapiadapter.New(deploymentService, managedDataRepo)
	if err != nil {
		_ = registry.Close()
		cleanup()
		return nil, nil, err
	}
	managedDataAPI, err := manageddataapiadapter.New(managedDataRepo)
	if err != nil {
		_ = registry.Close()
		cleanup()
		return nil, nil, err
	}
	cleanupWithRegistry := func() {
		_ = registry.Close()
		cleanup()
	}
	runtimeMetrics := app.NewDynamicRuntimeMetrics("", func(workspaceID string) app.RuntimeProvider {
		return registry.ProviderForWorkspace(servingstate.WorkspaceID(workspaceID))
	})
	assetCatalog := workspace.NewAssetCatalogService(workspaceRepo)
	authConfig := app.AuthConfig{DevBypass: true, CSRFKey: cfg.CSRFKey, CookieSecure: false}
	if production {
		oidcProviders := []oidcauth.Config{}
		if cfg.OIDCConfigured() {
			oidcProviders = append(oidcProviders, oidcauth.Config{
				ID:           cfg.OIDCProviderID,
				IssuerURL:    cfg.OIDCIssuerURL,
				ClientID:     cfg.OIDCClientID,
				ClientSecret: cfg.OIDCSecret,
				RedirectURL:  cfg.OIDCCallbackURL,
				Scopes:       cfg.OIDCScopesList(),
			})
		}
		authConfig = app.AuthConfig{
			DevBypass:       cfg.DevAuthBypass,
			APITokenOnly:    cfg.APITokenOnlyAuth,
			LocalAuth:       cfg.LocalAuth,
			AzureClientID:   cfg.AzureClientID,
			AzureSecret:     cfg.AzureSecret,
			AzureCallback:   cfg.AzureCallbackURL,
			AzureTenant:     cfg.AzureTenant,
			CSRFKey:         cfg.CSRFKey,
			CookieSecure:    cookieSecure,
			BootstrapTenant: cfg.AzureTenant,
			OIDCProviders:   oidcProviders,
		}
	}
	auth := app.NewAuth(accessRepo, "", authConfig)
	rateLimits := app.ProductionRateLimitConfig()
	rateLimits.Enabled = production && cfg.RateLimitingEnabled()
	rateLimits.UseRealIP = cfg.RateLimitingUsesRealIP()
	server := app.NewWithOptions(runtimeMetrics, app.Options{
		Store:               store,
		ServingStateRepo:    servingStateRepo,
		WorkspaceRepo:       workspaceRepo,
		AssetCatalog:        assetCatalog,
		AccessRepo:          accessRepo,
		Agent:               agent.NewService(runtimeMetrics, agentRepo, agent.Config{APIKey: cfg.AgentAPIKey, BaseURL: cfg.AgentBaseURL, Model: cfg.AgentModel}),
		Auth:                auth,
		Reloader:            registry,
		ArtifactDir:         cfg.ArtifactDir(),
		DuckDBDir:           cfg.DuckDBDirPath(),
		DuckLakeCatalogPath: duckLakeCatalogPath,
		DuckLakeDataPath:    cfg.DuckLakeDataDir(),
		DefaultEnvironment:  string(environment),
		RateLimits:          rateLimits,
		SecurityHeaders:     app.SecurityHeaders(production && cfg.HSTSEnabled(cookieSecure)),
		RequestLogging:      production && cfg.RequestLoggingEnabled(),
		Logger:              slog.Default(),
		SCIMBearerToken:     cfg.SCIMBearerToken,
		MetricsBearerToken:  cfg.MetricsBearerToken,
		AllowedHosts:        allowedHosts,
		Executor:            execution.New(cfg.ExecutionConfig()),
		JobLeaseTimeout:     cfg.ExecJobLeaseTimeout,
		ManagedData: manageddatahttp.Options{
			Repository: managedDataAPI, Uploads: managedDataControl,
			Multipart: managedDataMultipart,
		},
		Deployment:     deploymenthttp.Options{Coordinator: deploymentAPI},
		ManagedDataTus: managedDataStorage.tus,
		ManagedDataExpirer: managedDataMaintenance{
			uploads: managedDataControl, multipart: managedDataMultipartService,
			uploadTTL: cfg.ManagedDataUploadSessionTTL, collector: managedDataCollector, runtime: managedDataRuntimeCollector,
		},
		ManagedDataExpireInterval: cfg.ManagedDataGCInterval,
	})
	return server, cleanupWithRegistry, nil
}
