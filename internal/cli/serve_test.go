package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/platform"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

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
	principal, err := repo.PrincipalByID(ctx, "dev")
	if err != nil {
		t.Fatalf("lookup dev principal: %v", err)
	}
	if principal.Email != "dev@localhost" || principal.DisplayName != "Local Developer" {
		t.Fatalf("dev principal = %#v, want Local Developer", principal)
	}
	allowed, err := repo.HasPermission(ctx, "other", principal.ID, access.PermissionTokenManage)
	if err != nil {
		t.Fatalf("check dev platform permission: %v", err)
	}
	if !allowed {
		t.Fatal("local dev principal missing platform admin permission")
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
