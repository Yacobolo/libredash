package app

import workspacedatastar "github.com/Yacobolo/leapview/internal/workspace/datastar"

func workspaceAssetStreamID(workspaceID, assetID, section string) string {
	return workspacedatastar.WorkspaceAssetStreamID(workspaceID, assetID, section)
}
