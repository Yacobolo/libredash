package cli

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/agent"
	agentsqlite "github.com/Yacobolo/libredash/internal/agent/sqlite"
	analyticsducklake "github.com/Yacobolo/libredash/internal/analytics/ducklake"
	materializesqlite "github.com/Yacobolo/libredash/internal/analytics/materialize/sqlite"
	"github.com/Yacobolo/libredash/internal/app"
	oidcauth "github.com/Yacobolo/libredash/internal/access/oidc"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
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
	cmd.Flags().StringVar(&opts.environment, "environment", string(servingstate.DefaultEnvironment), "serving environment")
	cmd.Flags().BoolVar(&opts.production, "production", cfg.Production, "serve active serving state from the platform DB")
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
	server, cleanup, err := servingStateBackedServer(ctx, cfg, dataDir, opts.production, servingstate.NormalizeEnvironment(servingstate.Environment(opts.environment)))
	if err != nil {
		return err
	}
	defer cleanup()
	server.StartBackgroundJobs(ctx)
	slog.Info("LibreDash listening", "url", "http://localhost"+addr)
	return http.ListenAndServe(addr, server.Routes())
}

func servingStateBackedServer(ctx context.Context, cfg config.Config, dataDir string, production bool, environment servingstate.Environment) (*app.Server, func(), error) {
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
	servingStateRepo := servingstatesqlite.NewRepository(store.SQLDB())
	if err := materializesqlite.NewSQLRunRepository(store.SQLDB()).FailRunsForTerminalServingStates(ctx, "refresh did not complete"); err != nil {
		cleanup()
		return nil, nil, err
	}
	if _, err := storagemaintenance.Run(ctx, servingStateRepo, storagemaintenance.Options{
		RootDir:     cfg.HomeDir,
		CatalogPath: cfg.DBPath(),
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
		DataDir:      dataDir,
		OnDrained: func(servingstate.ID, int64) {
			go func() {
				protected := []int64(nil)
				if registry != nil {
					protected = registry.LeasedSnapshots()
				}
				if _, err := storagemaintenance.Run(context.Background(), servingStateRepo, storagemaintenance.Options{
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
		Factory: servingStateRuntimeFactory{
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
		DuckLakeCatalogPath: cfg.DBPath(),
		DuckLakeDataPath:    cfg.DuckLakeDataDir(),
		DefaultEnvironment:  string(environment),
		RateLimits:          rateLimits,
		SecurityHeaders:     app.SecurityHeaders(production && cfg.HSTSEnabled(cookieSecure)),
		RequestLogging:      production && cfg.RequestLoggingEnabled(),
		Logger:              slog.Default(),
		SCIMBearerToken:     cfg.SCIMBearerToken,
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
