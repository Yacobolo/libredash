package app

import (
	"context"
	"net/http"

	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacehttp "github.com/Yacobolo/libredash/internal/workspace/http"
)

func (s *Server) workspaceHTTPHandler() workspacehttp.Handler {
	return workspacehttp.Handler{
		WorkspaceID:          s.workspaceID,
		Environment:          func(r *http.Request) string { return string(s.requestDeploymentEnvironment(r)) },
		WorkspaceRepository:  s.workspaceRepository,
		AccessRepository:     s.accessRepository,
		WorkspaceList:        s.workspaceList,
		WorkspaceAssetsEdges: s.workspaceAssetsAndEdges,
		PlatformAssetsEdges:  s.platformConnectionAssetsAndEdges,
		MetricsForWorkspace:  s.workspaceHTTPMetrics,
		CatalogForWorkspaces: s.catalogForWorkspacesPage,
		RoleBindingsAndRoles: s.roleBindingsAndRoles,
		CatalogForWorkspace:  s.catalogForWorkspace,
		WorkspaceResponse:    s.workspaceResponse,
		CanManageAccess:      s.canManageWorkspaceAccess,
		WorkspaceAccess:      s.workspaceAccessResponse,
		RefreshState:         workspaceRefreshHTTPAdapter{server: s},
		RefreshRunner:        workspaceRefreshHTTPAdapter{server: s},
		Broker:               s.broker,
		CSRFToken:            func(r *http.Request) string { return csrfToken(r, s.auth) },
		CurrentRoleLabel:     s.currentRoleLabel,
		ChromeOptions:        func(r *http.Request) []ui.ChromeOption { return []ui.ChromeOption{s.chatChromeOption(r)} },
	}
}

func (s *Server) workspaceHTTPMetrics(workspaceID string) (workspacehttp.Metrics, bool) {
	metrics, ok := s.metricsForWorkspace(workspaceID)
	if !ok {
		return nil, false
	}
	return metrics, true
}

type workspaceRefreshHTTPAdapter struct {
	server *Server
}

func (a workspaceRefreshHTTPAdapter) AssetRefreshState(ctx context.Context, workspaceID string, asset workspace.AssetView) (ui.AssetRefreshState, error) {
	return a.server.assetRefreshStateForContext(ctx, workspaceID, asset)
}

func (a workspaceRefreshHTTPAdapter) AssetVersionsState(ctx context.Context, workspaceID, environment string, asset workspace.AssetView, section string) (ui.AssetVersionsState, error) {
	return a.server.assetVersionsStateForSection(ctx, workspaceID, environment, asset, section)
}

func (a workspaceRefreshHTTPAdapter) RefreshAsset(_ context.Context, input workspacehttp.AssetRefreshInput) error {
	return a.server.queueWorkspaceAssetRefreshWithPatches(input.Request, input.WorkspaceID, input.Asset, input.Assets, input.Edges)
}
