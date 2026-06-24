package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type Asset struct {
	ID             AssetID
	WorkspaceID    WorkspaceID
	DeploymentID   DeploymentID
	Type           AssetType
	Key            string
	ParentID       AssetID
	Title          string
	Description    string
	ContentJSON    string
	ContentHash    string
	ContentVersion int
}

type AssetEdge struct {
	ID           AssetEdgeID
	WorkspaceID  WorkspaceID
	DeploymentID DeploymentID
	FromAssetID  AssetID
	ToAssetID    AssetID
	Type         AssetEdgeType
}

type AssetGraph struct {
	Assets []Asset
	Edges  []AssetEdge
}

const CurrentAssetContentVersion = 2

func NewAsset(workspaceID WorkspaceID, deploymentID DeploymentID, typ AssetType, key string, parentID AssetID, title, description string, content any) (Asset, error) {
	bytes, err := json.Marshal(content)
	if err != nil {
		return Asset{}, err
	}
	sum := sha256.Sum256(bytes)
	return Asset{
		ID:             NewAssetID(deploymentID, typ, key),
		WorkspaceID:    workspaceID,
		DeploymentID:   deploymentID,
		Type:           typ,
		Key:            key,
		ParentID:       parentID,
		Title:          title,
		Description:    description,
		ContentJSON:    string(bytes),
		ContentHash:    hex.EncodeToString(sum[:]),
		ContentVersion: CurrentAssetContentVersion,
	}, nil
}

func NewAssetEdge(workspaceID WorkspaceID, deploymentID DeploymentID, fromID, toID AssetID, typ AssetEdgeType) AssetEdge {
	return AssetEdge{
		ID:           NewAssetEdgeID(deploymentID, fromID, toID, typ),
		WorkspaceID:  workspaceID,
		DeploymentID: deploymentID,
		FromAssetID:  fromID,
		ToAssetID:    toID,
		Type:         typ,
	}
}
