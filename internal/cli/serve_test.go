package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/platform"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestServeProductionModeHonorsConfigEnv(t *testing.T) {
	cfg := config.Config{Production: true}
	if !serveProductionMode(cfg, rootOptions{}) {
		t.Fatal("serve production mode ignored config production setting")
	}
}

func TestServeCommandConstructionDoesNotParseEnvironment(t *testing.T) {
	t.Setenv("LIBREDASH_EXEC_MAX_RUNNING_READS", "invalid")
	cmd := serveCommand(context.Background(), &rootOptions{})
	if cmd == nil {
		t.Fatal("serveCommand() returned nil")
	}
}

func TestServeEnvironmentDefaultsToProductionEnvironment(t *testing.T) {
	if got := serveEnvironment(true, ""); got != servingstate.Environment("prod") {
		t.Fatalf("production serve environment = %q, want prod", got)
	}
	if got := serveEnvironment(false, ""); got != servingstate.DefaultEnvironment {
		t.Fatalf("development serve environment = %q, want %q", got, servingstate.DefaultEnvironment)
	}
	if got := serveEnvironment(true, "dev"); got != servingstate.DefaultEnvironment {
		t.Fatalf("explicit production serve environment = %q, want dev", got)
	}
}

func TestServeEnvironmentFlagValueIgnoresSharedCommandDefaults(t *testing.T) {
	if got := serveEnvironmentFlagValue(false, "dev"); got != "" {
		t.Fatalf("unchanged serve environment flag value = %q, want empty", got)
	}
	if got := serveEnvironmentFlagValue(true, "dev"); got != "dev" {
		t.Fatalf("changed serve environment flag value = %q, want dev", got)
	}
}

func TestListenURLHandlesQualifiedAddr(t *testing.T) {
	for _, tt := range []struct {
		addr string
		want string
	}{
		{addr: ":8080", want: "http://localhost:8080"},
		{addr: "127.0.0.1:18080", want: "http://127.0.0.1:18080"},
		{addr: "0.0.0.0:8080", want: "http://0.0.0.0:8080"},
	} {
		if got := listenURL(tt.addr); got != tt.want {
			t.Fatalf("listenURL(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

func TestProductionHTTPServerHasTimeouts(t *testing.T) {
	server := productionHTTPServer(":0", http.NewServeMux())
	if server.ReadHeaderTimeout <= 0 {
		t.Fatal("ReadHeaderTimeout is not configured")
	}
	if server.ReadTimeout <= 0 {
		t.Fatal("ReadTimeout is not configured")
	}
	if server.WriteTimeout <= 0 {
		t.Fatal("WriteTimeout is not configured")
	}
	if server.IdleTimeout <= 0 {
		t.Fatal("IdleTimeout is not configured")
	}
	if server.Shutdown(context.Background()) != nil {
		t.Fatal("empty server shutdown should be a no-op")
	}
}

func TestDefaultHTTPServerShutdownTimeout(t *testing.T) {
	if defaultHTTPServerShutdownTimeout < 5*time.Second {
		t.Fatalf("shutdown timeout = %s, want at least 5s", defaultHTTPServerShutdownTimeout)
	}
}

func TestDeploymentBackedDevServerAlwaysOpensPlatformStore(t *testing.T) {
	home := t.TempDir()
	_, cleanup, err := servingStateBackedServer(context.Background(), config.Config{HomeDir: home}, "", false, servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("deployment-backed dev server: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(home, "libredash.db")); err != nil {
		t.Fatalf("platform store was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "artifacts")); err != nil {
		t.Fatalf("artifact directory was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "data")); err != nil {
		t.Fatalf("DuckLake data directory was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "ducklake")); err != nil {
		t.Fatalf("DuckLake catalog directory was not created: %v", err)
	}
}

func TestDeploymentBackedServerCreatesPrivateStateDirectories(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	restoreUmask := setServeTestUmask(t, 0)
	_, cleanup, err := servingStateBackedServer(context.Background(), config.Config{HomeDir: home}, "", false, servingstate.DefaultEnvironment)
	restoreUmask()
	if err != nil {
		t.Fatalf("deployment-backed dev server: %v", err)
	}
	defer cleanup()

	for _, path := range []string{
		home,
		filepath.Join(home, "artifacts"),
		filepath.Join(home, "data"),
		filepath.Join(home, "duckdb"),
		filepath.Join(home, "ducklake"),
		filepath.Join(home, "runtime"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("mode for %s = %#o, want 0700", path, got)
		}
	}
}

func TestProductionServerAllowsCallbackHostAndRejectsOthers(t *testing.T) {
	home := t.TempDir()
	server, cleanup, err := servingStateBackedServer(context.Background(), config.Config{
		HomeDir:            home,
		Production:         true,
		OIDCIssuerURL:      "https://issuer.example",
		OIDCClientID:       "client-id",
		OIDCSecret:         "client-secret",
		OIDCCallbackURL:    "https://app.example.com/auth/oidc/callback",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}, "", true, servingstate.Environment("prod"))
	if err != nil {
		t.Fatalf("production server: %v", err)
	}
	defer cleanup()
	handler := server.Routes()

	for _, tc := range []struct {
		name string
		host string
		want int
	}{
		{name: "callback host", host: "app.example.com", want: http.StatusOK},
		{name: "unexpected host", host: "evil.example.com", want: http.StatusMisdirectedRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			req.Host = tc.host
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func setServeTestUmask(t *testing.T, mask int) func() {
	t.Helper()
	old := syscall.Umask(mask)
	return func() {
		syscall.Umask(old)
	}
}

func TestDeploymentBackedDevServerRemovesLegacyDuckLakeArtifacts(t *testing.T) {
	home := t.TempDir()
	legacyCatalog := filepath.Join(home, "duckdb", "dev", "catalog.sqlite")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(legacyCatalog), "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyCatalog, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(legacyCatalog), "data", "old.parquet"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, cleanup, err := servingStateBackedServer(context.Background(), config.Config{HomeDir: home}, "", false, servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("deployment-backed dev server: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(legacyCatalog); !os.IsNotExist(err) {
		t.Fatalf("legacy DuckLake catalog exists or stat failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(legacyCatalog), "data")); !os.IsNotExist(err) {
		t.Fatalf("legacy DuckLake data exists or stat failed: %v", err)
	}
}

func TestDeploymentBackedDevServerSeedsPlatformAdminPrincipal(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	_, cleanup, err := servingStateBackedServer(ctx, config.Config{HomeDir: home}, "", false, servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("deployment-backed dev server: %v", err)
	}
	defer cleanup()

	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	repo := accesssqlite.NewRepository(store.SQLDB())
	if err := workspacesqlite.NewRepositoryWithSecurables(store.SQLDB(), repo).Ensure(ctx, workspace.EnsureInput{ID: "other", Title: "Other"}); err != nil {
		t.Fatalf("ensure other workspace: %v", err)
	}
	principal, err := repo.PrincipalByID(ctx, "dev")
	if err != nil {
		t.Fatalf("lookup dev principal: %v", err)
	}
	if principal.Email != "dev@localhost" || principal.DisplayName != "Local Developer" {
		t.Fatalf("dev principal = %#v, want Local Developer", principal)
	}
	decision, err := repo.Authorize(ctx, principal.ID, access.PrivilegeManageGrants, access.WorkspaceObject("other"))
	if err != nil {
		t.Fatalf("check dev platform privilege: %v", err)
	}
	if !decision.Allowed {
		t.Fatal("local dev principal missing platform admin privilege")
	}
}

func TestDeploymentBackedDevServerDoesNotCreateWorkspacesOrDeployments(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	_, cleanup, err := servingStateBackedServer(ctx, config.Config{HomeDir: home}, "", false, servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("deployment-backed dev server: %v", err)
	}
	defer cleanup()

	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	workspaces, err := workspaceRepo.List(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(workspaces) != 0 {
		t.Fatalf("workspaces = %#v, want none before explicit deploy", workspaces)
	}
	servingStateRepo := servingstatesqlite.NewRepository(store.SQLDB())
	deployments, err := servingStateRepo.List(ctx, servingstate.WorkspaceID("test"), servingstate.DefaultEnvironment)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deployments) != 0 {
		t.Fatalf("deployments = %#v, want none before explicit deploy", deployments)
	}
}
