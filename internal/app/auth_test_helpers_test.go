package app

import (
	"context"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
)

func testAuth(store *platform.Store, workspaceID string, cfg AuthConfig) *Auth {
	repo := accesssqlite.NewRepository(store.SQLDB())
	if cfg.DevBypass {
		_, _ = repo.SetPlatformRole(context.Background(), access.PlatformRoleInput{
			PrincipalID: "dev",
			Email:       "dev@localhost",
			DisplayName: "Local Developer",
			Role:        access.RolePlatformAdmin,
		})
	}
	return NewAuth(repo, workspaceID, cfg)
}
