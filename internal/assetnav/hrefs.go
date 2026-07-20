package assetnav

import (
	"net/url"
	"strings"

	"github.com/Yacobolo/leapview/internal/workspace"
)

func ConnectionsHref(query string) string {
	return ConnectionsHrefWithType("", query)
}

func ConnectionsHrefWithType(typ, query string) string {
	params := []string{}
	if typ = strings.TrimSpace(typ); typ != "" {
		params = append(params, "type="+url.QueryEscape(typ))
	}
	if query = strings.TrimSpace(query); query != "" {
		params = append(params, "q="+url.QueryEscape(query))
	}
	if len(params) == 0 {
		return "/connections"
	}
	return "/connections?" + strings.Join(params, "&")
}

func WorkspaceAssetSectionHref(workspaceID, assetID, section string) string {
	return "/workspaces/" + workspaceID + "/assets/" + assetID + "/" + section
}

func ConnectionAssetSectionHref(assetID, section string) string {
	return "/connections/" + assetID + "/" + section
}

func ConnectionSourceAssetSectionHref(connectionID, sourceID, section string) string {
	return "/connections/" + connectionID + "/sources/" + sourceID + "/" + section
}

func CanonicalAssetSectionHref(workspaceID string, asset workspace.AssetView, section string, edges []workspace.AssetEdgeView) string {
	switch asset.Type {
	case "connection":
		return ConnectionAssetSectionHref(asset.ID, section)
	case "source":
		return CanonicalSourceAssetSectionHref(workspaceID, asset.ID, section, edges)
	default:
		return WorkspaceAssetSectionHref(workspaceID, asset.ID, section)
	}
}

func CanonicalSourceAssetSectionHref(workspaceID, sourceID, section string, edges []workspace.AssetEdgeView) string {
	if connectionID := SourceConnectionID(sourceID, edges); connectionID != "" {
		return ConnectionSourceAssetSectionHref(connectionID, sourceID, section)
	}
	return WorkspaceAssetSectionHref(workspaceID, sourceID, section)
}

func SourceConnectionID(sourceID string, edges []workspace.AssetEdgeView) string {
	return workspace.SourceConnectionID(sourceID, edges)
}
