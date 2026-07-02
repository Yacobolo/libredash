package cli

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/agentapp"
	agentappsqlite "github.com/Yacobolo/libredash/internal/agentapp/sqlite"
	"github.com/Yacobolo/libredash/internal/app"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentsqlite "github.com/Yacobolo/libredash/internal/deployment/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
	"github.com/spf13/cobra"
)

func serveCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the LibreDash HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(ctx, opts)
		},
	}
	cfg := config.MustLoad()
	cmd.Flags().StringVar(&opts.addr, "addr", cfg.ListenAddr(), "listen address")
	cmd.Flags().StringVar(&opts.dataDir, "data-dir", cfg.DataDir, "dashboard source data directory")
	cmd.Flags().StringVar(&opts.environment, "environment", string(deployment.DefaultEnvironment), "deployment environment")
	cmd.Flags().BoolVar(&opts.production, "production", cfg.Production, "serve active deployment from the platform DB")
	return cmd
}

func runServe(ctx context.Context, opts *rootOptions) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	addr := opts.addr
	if addr == "" {
		addr = cfg.ListenAddr()
	}
	dataDir := opts.dataDir
	if dataDir == "" {
		dataDir = cfg.DataDir
	}
	if opts.production {
		if err := cfg.ValidateProductionAuth(); err != nil {
			return err
		}
	}
	server, cleanup, err := deploymentBackedServer(ctx, cfg, dataDir, opts.production, deployment.NormalizeEnvironment(deployment.Environment(opts.environment)))
	if err != nil {
		return err
	}
	defer cleanup()
	slog.Info("LibreDash listening", "url", "http://localhost"+addr)
	return http.ListenAndServe(addr, server.Routes())
}

func deploymentBackedServer(ctx context.Context, cfg config.Config, dataDir string, production bool, environment deployment.Environment) (*app.Server, func(), error) {
	cookieSecure, err := cfg.CookieSecure()
	if err != nil {
		return nil, nil, err
	}
	for _, dir := range []string{cfg.ArtifactDir(), cfg.DuckDBDirPath(), cfg.RuntimeDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
	}
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = store.Close() }
	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	if !production {
		if err := app.SeedLocalDeveloperPlatformAdmin(ctx, accessRepo); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	deploymentRepo := deploymentsqlite.NewRepository(store.SQLDB())
	agentRepo := agentappsqlite.NewRepository(store.SQLDB())
	summaries, err := workspaceRepo.List(ctx)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	workspaceIDs := make([]deployment.WorkspaceID, 0, len(summaries))
	for _, summary := range summaries {
		workspaceIDs = append(workspaceIDs, deployment.WorkspaceID(summary.ID))
	}
	registry := runtimehost.NewRegistryWithFactory(runtimehost.RegistryOptions{
		Repo:         deploymentRepo,
		WorkspaceIDs: workspaceIDs,
		Environment:  environment,
		DataDir:      dataDir,
		Factory: deploymentRuntimeFactory{
			dataDir:    dataDir,
			duckDBDir:  cfg.DuckDBDirPath(),
			runtimeDir: cfg.RuntimeDir(),
		},
	})
	if err := registry.Reload(ctx); err != nil {
		cleanup()
		return nil, nil, err
	}
	cleanupWithRegistry := func() {
		_ = registry.Close()
		cleanup()
	}
	runtimeMetrics := app.NewDynamicRuntimeMetrics("", dataDir, func(workspaceID string) app.RuntimeProvider {
		return registry.ProviderForWorkspace(deployment.WorkspaceID(workspaceID))
	})
	assetCatalog := workspace.NewAssetCatalogService(workspaceRepo)
	authConfig := app.AuthConfig{DevBypass: true, CSRFKey: cfg.CSRFKey, CookieSecure: false}
	if production {
		authConfig = app.AuthConfig{
			DevBypass:       cfg.DevAuthBypass,
			APITokenOnly:    cfg.APITokenOnlyAuth,
			AzureClientID:   cfg.AzureClientID,
			AzureSecret:     cfg.AzureSecret,
			AzureCallback:   cfg.AzureCallbackURL,
			AzureTenant:     cfg.AzureTenant,
			CSRFKey:         cfg.CSRFKey,
			CookieSecure:    cookieSecure,
			BootstrapTenant: cfg.AzureTenant,
		}
	}
	auth := app.NewAuth(accessRepo, "", authConfig)
	rateLimits := app.ProductionRateLimitConfig()
	rateLimits.Enabled = production && cfg.RateLimitingEnabled()
	server := app.NewWithOptions(runtimeMetrics, app.Options{
		Store:              store,
		DeploymentRepo:     deploymentRepo,
		WorkspaceRepo:      workspaceRepo,
		AssetCatalog:       assetCatalog,
		AccessRepo:         accessRepo,
		Agent:              agentapp.NewService(runtimeMetrics, agentRepo, agentapp.Config{APIKey: cfg.AgentAPIKey, BaseURL: cfg.AgentBaseURL, Model: cfg.AgentModel}),
		Auth:               auth,
		Reloader:           registry,
		ArtifactDir:        cfg.ArtifactDir(),
		DuckDBDir:          cfg.DuckDBDirPath(),
		DefaultEnvironment: string(environment),
		RateLimits:         rateLimits,
		SecurityHeaders:    app.SecurityHeaders(production && cfg.HSTSEnabled(cookieSecure)),
		RequestLogging:     production && cfg.RequestLoggingEnabled(),
		Logger:             slog.Default(),
	})
	return server, cleanupWithRegistry, nil
}
