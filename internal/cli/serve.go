package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/Yacobolo/libredash/internal/app"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/data"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/runtime"
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
		metrics, err := data.NewDuckDBMetrics(dataDir)
		if err != nil {
			return fmt.Errorf("initializing DuckDB metrics: %w", err)
		}
		defer metrics.Close()
		server := app.New(metrics)
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
	if err := store.EnsureWorkspace(ctx, platform.WorkspaceInput{ID: opts.workspaceID, Title: opts.workspaceID}); err != nil {
		return err
	}
	if err := store.BootstrapAdmin(ctx, opts.workspaceID, cfg.BootstrapEmail); err != nil {
		return err
	}
	manager := runtime.NewManager(store, opts.workspaceID, dataDir, cfg.DuckDBDirPath(), cfg.RuntimeDir())
	if err := manager.Reload(ctx); err != nil {
		return err
	}
	defer manager.Close()
	auth := app.NewAuth(store, opts.workspaceID, app.AuthConfig{
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
	server := app.NewWithOptions(manager, app.Options{
		Store:              store,
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
