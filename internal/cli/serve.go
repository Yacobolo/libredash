package cli

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/agentapp"
	agentappsqlite "github.com/Yacobolo/libredash/internal/agentapp/sqlite"
	analyticsducklake "github.com/Yacobolo/libredash/internal/analytics/ducklake"
	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/app"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentsqlite "github.com/Yacobolo/libredash/internal/deployment/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	storagemaintenance "github.com/Yacobolo/libredash/internal/storage/maintenance"
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
	server.StartBackgroundJobs(ctx)
	slog.Info("LibreDash listening", "url", "http://localhost"+addr)
	return http.ListenAndServe(addr, server.Routes())
}

func deploymentBackedServer(ctx context.Context, cfg config.Config, dataDir string, production bool, environment deployment.Environment) (*app.Server, func(), error) {
	cookieSecure, err := cfg.CookieSecure()
	if err != nil {
		return nil, nil, err
	}
	for _, dir := range []string{cfg.ArtifactDir(), cfg.DuckDBDirPath(), cfg.RuntimeDir(), cfg.DuckLakeDataDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
	}
	if err := analyticsducklake.MigrateSQLiteCatalogDataPath(ctx, cfg.DBPath(), cfg.DuckLakeDataDir()); err != nil {
		return nil, nil, err
	}
	if err := removeLegacyDuckLakeArtifacts(cfg.DuckDBDirPath()); err != nil {
		return nil, nil, err
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
	if err := materialize.NewSQLRunRepository(store.SQLDB()).FailRunsForTerminalDeployments(ctx, "refresh did not complete"); err != nil {
		cleanup()
		return nil, nil, err
	}
	if _, err := storagemaintenance.Run(ctx, deploymentRepo, storagemaintenance.Options{
		RootDir:     cfg.HomeDir,
		CatalogPath: cfg.DBPath(),
		DataPath:    cfg.DuckLakeDataDir(),
		DryRun:      false,
	}); err != nil {
		cleanup()
		return nil, nil, err
	}
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
	var registry *runtimehost.Registry
	registry = runtimehost.NewRegistryWithFactory(runtimehost.RegistryOptions{
		Repo:         deploymentRepo,
		WorkspaceIDs: workspaceIDs,
		Environment:  environment,
		DataDir:      dataDir,
		OnDrained: func(deployment.ID, int64) {
			go func() {
				protected := []int64(nil)
				if registry != nil {
					protected = registry.LeasedSnapshots()
				}
				if _, err := storagemaintenance.Run(context.Background(), deploymentRepo, storagemaintenance.Options{
					RootDir:                      cfg.HomeDir,
					CatalogPath:                  cfg.DBPath(),
					DataPath:                     cfg.DuckLakeDataDir(),
					AdditionalProtectedSnapshots: protected,
					DryRun:                       false,
				}); err != nil {
					slog.Default().Warn("storage retention cleanup failed after runtime drain", "error", err)
				}
			}()
		},
		Factory: deploymentRuntimeFactory{
			dataDir:          dataDir,
			duckDBDir:        cfg.DuckDBDirPath(),
			runtimeDir:       cfg.RuntimeDir(),
			catalogPath:      cfg.DBPath(),
			duckLakeDataPath: cfg.DuckLakeDataDir(),
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
		Store:               store,
		DeploymentRepo:      deploymentRepo,
		WorkspaceRepo:       workspaceRepo,
		AssetCatalog:        assetCatalog,
		AccessRepo:          accessRepo,
		Agent:               agentapp.NewService(runtimeMetrics, agentRepo, agentapp.Config{APIKey: cfg.AgentAPIKey, BaseURL: cfg.AgentBaseURL, Model: cfg.AgentModel}),
		Auth:                auth,
		Reloader:            registry,
		ArtifactDir:         cfg.ArtifactDir(),
		DuckDBDir:           cfg.DuckDBDirPath(),
		DuckLakeCatalogPath: cfg.DBPath(),
		DuckLakeDataPath:    cfg.DuckLakeDataDir(),
		DefaultEnvironment:  string(environment),
		RateLimits:          rateLimits,
		SecurityHeaders:     app.SecurityHeaders(production && cfg.HSTSEnabled(cookieSecure)),
		RequestLogging:      production && cfg.RequestLoggingEnabled(),
		Logger:              slog.Default(),
	})
	return server, cleanupWithRegistry, nil
}

func removeLegacyDuckLakeArtifacts(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "catalog.sqlite" {
			return nil
		}
		dir := filepath.Dir(path)
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if err := os.RemoveAll(filepath.Join(dir, "data")); err != nil {
			return err
		}
		return nil
	})
}
