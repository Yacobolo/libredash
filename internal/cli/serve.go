package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/agentapp"
	agentappsqlite "github.com/Yacobolo/libredash/internal/agentapp/sqlite"
	"github.com/Yacobolo/libredash/internal/app"
	"github.com/Yacobolo/libredash/internal/config"
	dashboardruntime "github.com/Yacobolo/libredash/internal/dashboard/runtime"
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
	cmd.Flags().StringVar(&opts.localCatalog, "local-catalog", "", "serve a filesystem catalog instead of active deployments")
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
	if !opts.production {
		catalogPath := opts.localCatalog
		if catalogPath != "" {
			if err := os.Setenv("LIBREDASH_CATALOG_PATH", catalogPath); err != nil {
				return err
			}
		}
		metrics, err := dashboardruntime.New(dataDir, dashboardDataRuntimeFactory{})
		if err != nil {
			return fmt.Errorf("initializing DuckDB metrics: %w", err)
		}
		defer metrics.Close()
		server, cleanup, err := localDevServer(ctx, metrics, cfg, opts.workspaceID)
		if err != nil {
			return err
		}
		defer cleanup()
		slog.Info("LibreDash listening", "url", "http://localhost"+addr)
		return http.ListenAndServe(addr, server.Routes())
	}

	if err := cfg.ValidateProductionAuth(); err != nil {
		return err
	}
	cookieSecure, err := cfg.CookieSecure()
	if err != nil {
		return err
	}
	for _, dir := range []string{cfg.ArtifactDir(), cfg.DuckDBDirPath(), cfg.RuntimeDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer store.Close()
	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: workspace.WorkspaceID(opts.workspaceID), Title: opts.workspaceID}); err != nil {
		return err
	}
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	if err := accessRepo.BootstrapAdmin(ctx, opts.workspaceID, cfg.BootstrapEmail); err != nil {
		return err
	}
	deploymentRepo := deploymentsqlite.NewRepository(store.SQLDB())
	agentRepo := agentappsqlite.NewRepository(store.SQLDB())
	manager := runtimehost.NewManagerWithFactory(deploymentRepo, deployment.WorkspaceID(opts.workspaceID), dataDir, deploymentRuntimeFactory{
		dataDir:    dataDir,
		duckDBDir:  cfg.DuckDBDirPath(),
		runtimeDir: cfg.RuntimeDir(),
	})
	if err := manager.Reload(ctx); err != nil {
		return err
	}
	defer manager.Close()
	runtimeMetrics := app.NewRuntimeMetrics(manager, dataDir, opts.workspaceID)
	assetCatalog := workspace.NewAssetCatalogService(workspaceRepo)
	if provider, ok := runtimeMetrics.(workspace.RuntimeAssetGraphProvider); ok {
		assetCatalog.WithRuntimeProvider(provider)
	}
	auth := app.NewAuth(accessRepo, opts.workspaceID, app.AuthConfig{
		DevBypass:       cfg.DevAuthBypass,
		APITokenOnly:    cfg.APITokenOnlyAuth,
		AzureClientID:   cfg.AzureClientID,
		AzureSecret:     cfg.AzureSecret,
		AzureCallback:   cfg.AzureCallbackURL,
		AzureTenant:     cfg.AzureTenant,
		CSRFKey:         cfg.CSRFKey,
		CookieSecure:    cookieSecure,
		BootstrapTenant: cfg.AzureTenant,
	})
	rateLimits := app.ProductionRateLimitConfig()
	rateLimits.Enabled = cfg.RateLimitingEnabled()
	server := app.NewWithOptions(runtimeMetrics, app.Options{
		Store:              store,
		DeploymentRepo:     deploymentRepo,
		WorkspaceRepo:      workspaceRepo,
		AssetCatalog:       assetCatalog,
		AccessRepo:         accessRepo,
		Agent:              agentapp.NewService(runtimeMetrics, agentRepo, agentapp.Config{APIKey: cfg.AgentAPIKey, BaseURL: cfg.AgentBaseURL, Model: cfg.AgentModel}),
		Auth:               auth,
		Reloader:           manager,
		ArtifactDir:        cfg.ArtifactDir(),
		DefaultWorkspaceID: opts.workspaceID,
		RateLimits:         rateLimits,
		SecurityHeaders:    app.SecurityHeaders(cfg.HSTSEnabled(cookieSecure)),
		RequestLogging:     cfg.RequestLoggingEnabled(),
		Logger:             slog.Default(),
	})
	slog.Info("LibreDash listening", "url", "http://localhost"+addr)
	return http.ListenAndServe(addr, server.Routes())
}

func localDevServer(ctx context.Context, metrics *dashboardruntime.Service, cfg config.Config, workspaceID string) (*app.Server, func(), error) {
	config := agentConfig(cfg)
	if !config.Enabled() {
		return app.New(metrics), func() {}, nil
	}

	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = store.Close() }

	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: workspace.WorkspaceID(workspaceID), Title: workspaceID}); err != nil {
		cleanup()
		return nil, nil, err
	}
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	agentRepo := agentappsqlite.NewRepository(store.SQLDB())
	assetCatalog := workspace.NewAssetCatalogService(workspaceRepo).WithRuntimeProvider(metrics)
	auth := app.NewAuth(accessRepo, workspaceID, app.AuthConfig{
		DevBypass:    true,
		CSRFKey:      cfg.CSRFKey,
		CookieSecure: false,
	})
	server := app.NewWithOptions(metrics, app.Options{
		Store:              store,
		WorkspaceRepo:      workspaceRepo,
		AssetCatalog:       assetCatalog,
		AccessRepo:         accessRepo,
		Agent:              agentapp.NewService(metrics, agentRepo, config),
		Auth:               auth,
		DefaultWorkspaceID: workspaceID,
	})
	return server, cleanup, nil
}

func agentConfig(cfg config.Config) agentapp.Config {
	return agentapp.Config{
		APIKey:  cfg.AgentAPIKey,
		BaseURL: cfg.AgentBaseURL,
		Model:   cfg.AgentModel,
	}
}
