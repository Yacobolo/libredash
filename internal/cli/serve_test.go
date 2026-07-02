package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentsqlite "github.com/Yacobolo/libredash/internal/deployment/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
)

func TestLocalDevServerAlwaysOpensPlatformStore(t *testing.T) {
	home := t.TempDir()
	_, cleanup, err := localDevServer(context.Background(), nil, config.Config{HomeDir: home}, "test", deployment.DefaultEnvironment)
	if err != nil {
		t.Fatalf("local dev server: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(home, "libredash.db")); err != nil {
		t.Fatalf("platform store was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "artifacts")); err != nil {
		t.Fatalf("artifact directory was not created: %v", err)
	}
}

func TestLocalDevServerSeedsPlatformAdminPrincipal(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	_, cleanup, err := localDevServer(ctx, nil, config.Config{HomeDir: home}, "test", deployment.DefaultEnvironment)
	if err != nil {
		t.Fatalf("local dev server: %v", err)
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

func TestLocalDevServerDoesNotCreateDeployments(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	_, cleanup, err := localDevServer(ctx, nil, config.Config{HomeDir: home}, "test", deployment.DefaultEnvironment)
	if err != nil {
		t.Fatalf("local dev server: %v", err)
	}
	defer cleanup()

	store, err := platform.Open(ctx, filepath.Join(home, "libredash.db"))
	if err != nil {
		t.Fatalf("open platform store: %v", err)
	}
	defer store.Close()
	deploymentRepo := deploymentsqlite.NewRepository(store.SQLDB())
	deployments, err := deploymentRepo.List(ctx, deployment.WorkspaceID("test"), deployment.DefaultEnvironment)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deployments) != 0 {
		t.Fatalf("deployments = %#v, want none before explicit deploy", deployments)
	}
}
