package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/app"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/data"
	"github.com/Yacobolo/libredash/internal/deploy"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/runtime"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	addr         string
	dataDir      string
	localCatalog string
	production   bool
	workspaceID  string
	target       string
	token        string
	catalog      string
	deployment   string
}

func Execute(ctx context.Context) error {
	opts := &rootOptions{}
	root := &cobra.Command{
		Use:   "libredash",
		Short: "LibreDash BI-as-code server and deployment CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(ctx, opts)
		},
	}
	root.PersistentFlags().StringVar(&opts.workspaceID, "workspace", platform.DefaultWorkspaceID, "workspace id")
	root.AddCommand(serveCommand(ctx, opts))
	root.AddCommand(deployCommand(ctx, opts))
	root.AddCommand(loginCommand(opts))
	root.AddCommand(deploymentsCommand(ctx, opts))
	root.AddCommand(rollbackCommand(ctx, opts))
	root.AddCommand(adminCommand(ctx, opts))
	return root.ExecuteContext(ctx)
}

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

func deployCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a dashboard-as-code catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	cmd.Flags().StringVar(&opts.token, "token", "", "API token")
	cmd.Flags().StringVar(&opts.catalog, "catalog", filepath.Join("dashboards", "catalog.yaml"), "catalog path")
	return cmd
}

func loginCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store a LibreDash API token for a target",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.target == "" || opts.token == "" {
				return fmt.Errorf("login requires --target and --token for v1 CLI authentication")
			}
			config, err := loadClientConfig()
			if err != nil {
				return err
			}
			if config.Targets == nil {
				config.Targets = map[string]clientTarget{}
			}
			config.Targets[opts.target] = clientTarget{Token: opts.token}
			return saveClientConfig(config)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	cmd.Flags().StringVar(&opts.token, "token", "", "API token")
	return cmd
}

func deploymentsCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "deployments", Short: "Inspect deployments"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List deployments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploymentsList(ctx, opts)
		},
	}
	list.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	list.Flags().StringVar(&opts.token, "token", "", "API token")
	rollback := &cobra.Command{
		Use:   "rollback",
		Short: "Activate a previous deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(ctx, opts)
		},
	}
	rollback.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	rollback.Flags().StringVar(&opts.token, "token", "", "API token")
	rollback.Flags().StringVar(&opts.deployment, "deployment", "", "deployment id")
	parent.AddCommand(list, rollback)
	return parent
}

func rollbackCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Activate a previous deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	cmd.Flags().StringVar(&opts.token, "token", "", "API token")
	cmd.Flags().StringVar(&opts.deployment, "deployment", "", "deployment id")
	return cmd
}

func adminCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "admin", Short: "Administrative utilities"}
	bootstrap := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap an owner principal and API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.MustLoad()
			store, err := platform.Open(ctx, cfg.DBPath())
			if err != nil {
				return err
			}
			defer store.Close()
			email := cfg.BootstrapEmail
			if email == "" {
				email = "admin@localhost"
			}
			if err := store.EnsureWorkspace(ctx, platform.WorkspaceInput{ID: opts.workspaceID, Title: opts.workspaceID}); err != nil {
				return err
			}
			principal, err := store.UpsertPrincipal(ctx, platform.PrincipalInput{ID: platform.PrincipalIDForEmail(email), Email: email, DisplayName: email})
			if err != nil {
				return err
			}
			if err := store.BindRole(ctx, opts.workspaceID, principal.ID, "owner"); err != nil {
				return err
			}
			token, err := store.CreateAPIToken(ctx, principal.ID, "bootstrap")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), token)
			return nil
		},
	}
	parent.AddCommand(bootstrap)
	return parent
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
		log.Printf("LibreDash listening on http://localhost%s", addr)
		return http.ListenAndServe(addr, server.Routes())
	}

	if !cfg.AzureConfigured() && !cfg.DevAuthBypass && !cfg.APITokenOnlyAuth {
		return fmt.Errorf("production serve requires Azure auth env vars, LIBREDASH_DEV_AUTH_BYPASS, or LIBREDASH_API_TOKEN_ONLY_AUTH")
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
		BootstrapTenant: cfg.AzureTenant,
	})
	server := app.NewWithOptions(manager, app.Options{
		Store:              store,
		Auth:               auth,
		Reloader:           manager,
		ArtifactDir:        cfg.ArtifactDir(),
		DefaultWorkspaceID: opts.workspaceID,
	})
	log.Printf("LibreDash listening on http://localhost%s", addr)
	return http.ListenAndServe(addr, server.Routes())
}

func runDeploy(ctx context.Context, opts *rootOptions) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	manifest, digest, err := deploy.PackCatalog(opts.catalog, &buf)
	if err != nil {
		return err
	}
	createBody, _ := json.Marshal(map[string]any{
		"workspaceId": opts.workspaceID,
		"title":       manifest.WorkspaceTitle,
	})
	var created api.DeploymentResponse
	if err := doJSON(ctx, http.MethodPost, target+"/api/deployments", token, bytes.NewReader(createBody), &created); err != nil {
		return err
	}
	if err := doJSON(ctx, http.MethodPut, target+"/api/deployments/"+created.ID+"/artifact", token, bytes.NewReader(buf.Bytes()), nil); err != nil {
		return err
	}
	var validated api.DeploymentResponse
	if err := doJSON(ctx, http.MethodPost, target+"/api/deployments/"+created.ID+"/validate", token, nil, &validated); err != nil {
		return err
	}
	var activated api.DeploymentResponse
	if err := doJSON(ctx, http.MethodPost, target+"/api/deployments/"+created.ID+"/activate", token, nil, &activated); err != nil {
		return err
	}
	fmt.Printf("deployed %s digest=%s localDigest=%s status=%s\n", activated.ID, activated.Digest, digest, activated.Status)
	return nil
}

func runDeploymentsList(ctx context.Context, opts *rootOptions) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	u, _ := url.Parse(target + "/api/deployments")
	q := u.Query()
	q.Set("workspace", opts.workspaceID)
	u.RawQuery = q.Encode()
	var rows []api.DeploymentResponse
	if err := doJSON(ctx, http.MethodGet, u.String(), token, nil, &rows); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tDIGEST\tCREATED\tACTIVATED")
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", row.ID, row.Status, shortDigest(row.Digest), row.CreatedAt, row.ActivatedAt)
	}
	return tw.Flush()
}

func runRollback(ctx context.Context, opts *rootOptions) error {
	if opts.deployment == "" {
		return fmt.Errorf("rollback requires --deployment")
	}
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	var row api.DeploymentResponse
	if err := doJSON(ctx, http.MethodPost, target+"/api/deployments/"+opts.deployment+"/rollback", token, nil, &row); err != nil {
		return err
	}
	fmt.Printf("activated %s status=%s\n", row.ID, row.Status)
	return nil
}

func doJSON(ctx context.Context, method, endpoint, token string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", method, endpoint, strings.TrimSpace(string(bytes)))
	}
	if out == nil || len(bytes) == 0 {
		return nil
	}
	return json.Unmarshal(bytes, out)
}

type clientConfig struct {
	Targets map[string]clientTarget `json:"targets"`
}

type clientTarget struct {
	Token string `json:"token"`
}

func clientTargetAndToken(opts *rootOptions) (string, string, error) {
	cfg := config.MustLoad()
	target := strings.TrimRight(opts.target, "/")
	if target == "" {
		target = strings.TrimRight(cfg.Target, "/")
	}
	token := opts.token
	if token == "" {
		token = cfg.APIToken
	}
	config, _ := loadClientConfig()
	if target != "" && token == "" {
		token = config.Targets[target].Token
	}
	if target == "" {
		return "", "", fmt.Errorf("target is required")
	}
	if token == "" {
		return "", "", fmt.Errorf("API token is required; use --token, LIBREDASH_API_TOKEN, or libredash login --target %s --token <token>", target)
	}
	return target, token, nil
}

func loadClientConfig() (clientConfig, error) {
	path := clientConfigPath()
	bytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return clientConfig{Targets: map[string]clientTarget{}}, nil
	}
	if err != nil {
		return clientConfig{}, err
	}
	var config clientConfig
	if err := json.Unmarshal(bytes, &config); err != nil {
		return clientConfig{}, err
	}
	if config.Targets == nil {
		config.Targets = map[string]clientTarget{}
	}
	return config, nil
}

func saveClientConfig(config clientConfig) error {
	path := clientConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o600)
}

func clientConfigPath() string {
	return config.MustLoad().ClientConfigPath()
}

func shortDigest(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func init() {
	http.DefaultClient.Timeout = 5 * time.Minute
}
