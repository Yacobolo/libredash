package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type Asset struct {
	ID           string
	WorkspaceID  string
	DeploymentID string
	Type         string
	Key          string
	ParentID     string
	Title        string
	Description  string
	ContentJSON  string
	ContentHash  string
}

type AssetEdge struct {
	ID           string
	WorkspaceID  string
	DeploymentID string
	FromAssetID  string
	ToAssetID    string
	Type         string
}

func NewAsset(workspaceID, deploymentID, typ, key, parentID, title, description string, content any) (Asset, error) {
	bytes, err := json.Marshal(content)
	if err != nil {
		return Asset{}, err
	}
	sum := sha256.Sum256(bytes)
	return Asset{
		ID:           "asset_" + stableID(deploymentID+"|"+typ+"|"+key),
		WorkspaceID:  workspaceID,
		DeploymentID: deploymentID,
		Type:         typ,
		Key:          key,
		ParentID:     parentID,
		Title:        title,
		Description:  description,
		ContentJSON:  string(bytes),
		ContentHash:  hex.EncodeToString(sum[:]),
	}, nil
}

func NewAssetEdge(workspaceID, deploymentID, fromID, toID, typ string) AssetEdge {
	return AssetEdge{
		ID:           "edge_" + stableID(deploymentID+"|"+fromID+"|"+toID+"|"+typ),
		WorkspaceID:  workspaceID,
		DeploymentID: deploymentID,
		FromAssetID:  fromID,
		ToAssetID:    toID,
		Type:         typ,
	}
}
