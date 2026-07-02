package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"

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
	cmd.Flags().StringVar(&opts.projectPath, "project", "", "project path override for dev serve")
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
	if !opts.production {
		projectPath := opts.projectPath
		if projectPath == "" {
			projectPath, err = dashboardruntime.DiscoverCatalogPath()
			if err != nil {
				return err
			}
		} else {
			if err := os.Setenv("LIBREDASH_CATALOG_PATH", projectPath); err != nil {
				return err
			}
		}
		var (
			metrics   app.QueryMetrics
			closeFunc func()
		)
		projectMetrics, err := dashboardruntime.NewFromProject(dataDir, projectPath, duckDBDirPath(cfg), dashboardDataRuntimeFactory{})
		if err != nil {
			return fmt.Errorf("initializing DuckDB metrics: %w", err)
		}
		workspaceMetrics := make(map[string]app.QueryMetrics, len(projectMetrics))
		workspaceIDs := make([]string, 0, len(projectMetrics))
		for workspaceID, service := range projectMetrics {
			workspaceIDs = append(workspaceIDs, workspaceID)
			workspaceMetrics[workspaceID] = service
		}
		sort.Strings(workspaceIDs)
		defaultWorkspaceID := opts.workspaceID
		if len(workspaceIDs) > 0 && defaultWorkspaceID == "" {
			defaultWorkspaceID = workspaceIDs[0]
		}
		metrics = app.NewMultiWorkspaceMetrics(defaultWorkspaceID, workspaceMetrics)
		closeFunc = func() {
			for _, service := range projectMetrics {
				_ = service.Close()
			}
		}
		opts.workspaceID = defaultWorkspaceID
		defer closeFunc()
		server, cleanup, err := localDevServer(ctx, metrics, cfg, opts.workspaceID, deployment.NormalizeEnvironment(deployment.Environment(opts.environment)))
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
	environment := deployment.NormalizeEnvironment(deployment.Environment(opts.environment))
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
	defaultWorkspaceID := opts.workspaceID
	if defaultWorkspaceID == "" {
		defaultWorkspaceID = platform.DefaultWorkspaceID
	}
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: workspace.WorkspaceID(defaultWorkspaceID), Title: defaultWorkspaceID}); err != nil {
		return err
	}
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	if err := accessRepo.BootstrapAdmin(ctx, defaultWorkspaceID, cfg.BootstrapEmail); err != nil {
		return err
	}
	deploymentRepo := deploymentsqlite.NewRepository(store.SQLDB())
	agentRepo := agentappsqlite.NewRepository(store.SQLDB())
	summaries, err := workspaceRepo.List(ctx)
	if err != nil {
		return err
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
		return err
	}
	defer registry.Close()
	runtimeMetrics := app.NewDynamicRuntimeMetrics(defaultWorkspaceID, dataDir, func(workspaceID string) app.RuntimeProvider {
		return registry.ProviderForWorkspace(deployment.WorkspaceID(workspaceID))
	})
	assetCatalog := workspace.NewAssetCatalogService(workspaceRepo)
	auth := app.NewAuth(accessRepo, defaultWorkspaceID, app.AuthConfig{
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
		Reloader:           registry,
		ArtifactDir:        cfg.ArtifactDir(),
		DuckDBDir:          cfg.DuckDBDirPath(),
		DefaultWorkspaceID: defaultWorkspaceID,
		DefaultEnvironment: string(environment),
		RateLimits:         rateLimits,
		SecurityHeaders:    app.SecurityHeaders(cfg.HSTSEnabled(cookieSecure)),
		RequestLogging:     cfg.RequestLoggingEnabled(),
		Logger:             slog.Default(),
	})
	slog.Info("LibreDash listening", "url", "http://localhost"+addr)
	return http.ListenAndServe(addr, server.Routes())
}

func localDevServer(ctx context.Context, metrics app.QueryMetrics, cfg config.Config, workspaceID string, environment deployment.Environment) (*app.Server, func(), error) {
	duckDBDir := ""
	if metrics != nil {
		duckDBDir = metrics.DataDir()
	}
	if cfg.DuckDBDir != "" {
		duckDBDir = cfg.DuckDBDirPath()
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
	if err := workspaceRepo.Ensure(ctx, workspace.EnsureInput{ID: workspace.WorkspaceID(workspaceID), Title: workspaceID}); err != nil {
		cleanup()
		return nil, nil, err
	}
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	if err := app.SeedLocalDeveloperPlatformAdmin(ctx, accessRepo); err != nil {
		cleanup()
		return nil, nil, err
	}
	deploymentRepo := deploymentsqlite.NewRepository(store.SQLDB())
	agentRepo := agentappsqlite.NewRepository(store.SQLDB())
	assetCatalog := workspace.NewAssetCatalogService(workspaceRepo)
	auth := app.NewAuth(accessRepo, workspaceID, app.AuthConfig{
		DevBypass:    true,
		CSRFKey:      cfg.CSRFKey,
		CookieSecure: false,
	})
	config := agentConfig(cfg)
	var agent *agentapp.Service
	if config.Enabled() {
		agent = agentapp.NewService(metrics, agentRepo, config)
	}
	server := app.NewWithOptions(metrics, app.Options{
		Store:              store,
		DeploymentRepo:     deploymentRepo,
		WorkspaceRepo:      workspaceRepo,
		AssetCatalog:       assetCatalog,
		AccessRepo:         accessRepo,
		Agent:              agent,
		Auth:               auth,
		ArtifactDir:        cfg.ArtifactDir(),
		DuckDBDir:          duckDBDir,
		DefaultWorkspaceID: workspaceID,
		DefaultEnvironment: string(environment),
	})
	return server, cleanup, nil
}

func duckDBDirPath(cfg config.Config) string {
	if cfg.DuckDBDir != "" {
		return cfg.DuckDBDirPath()
	}
	return filepath.Join(cfg.DataDir, ".duckdb")
}

func agentConfig(cfg config.Config) agentapp.Config {
	return agentapp.Config{
		APIKey:  cfg.AgentAPIKey,
		BaseURL: cfg.AgentBaseURL,
		Model:   cfg.AgentModel,
	}
}
