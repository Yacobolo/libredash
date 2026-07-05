package datastar

import (
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func WorkspaceAccessSignals(access ui.WorkspaceAccessResponse, csrfToken string) map[string]any {
	return map[string]any{
		"workspaceAccess": ui.WorkspaceAccessSignals(access, csrfToken),
	}
}

func WorkspaceAssetStreamID(workspaceID, assetID, section string) string {
	return "workspace-asset:" + workspaceID + ":" + assetID + ":" + section
}

func WorkspaceAssetRefreshSections() []string {
	return []string{"details", "refreshes", "lineage"}
}

func WorkspaceAssetUpdateSection(r *http.Request) string {
	switch strings.TrimSpace(r.URL.Query().Get("section")) {
	case "refreshes":
		return "refreshes"
	case "lineage":
		return "lineage"
	default:
		return "details"
	}
}

func WorkspaceAssetRefreshSignals(view workspace.WorkspaceView, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView, refresh ui.AssetRefreshState, section string) map[string]any {
	return ui.WorkspaceAssetRefreshSignals(view, asset, assets, edges, refresh, section)
}
