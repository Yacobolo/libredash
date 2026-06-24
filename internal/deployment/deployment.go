package deployment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var ErrNotFound = errors.New("deployment not found")

type ID string

type WorkspaceID string

type Status string

const (
	StatusPending   Status = "pending"
	StatusValidated Status = "validated"
	StatusActive    Status = "active"
	StatusInactive  Status = "inactive"
	StatusFailed    Status = "failed"
)

type Deployment struct {
	ID           ID
	WorkspaceID  WorkspaceID
	Status       Status
	Digest       string
	ManifestJSON string
	CreatedBy    string
	CreatedAt    string
	ActivatedAt  string
	Error        string
}

func (d Deployment) CanActivate() bool {
	return d.Status == StatusValidated || d.Status == StatusInactive || d.Status == StatusActive
}

type CreateInput struct {
	WorkspaceID WorkspaceID
	CreatedBy   string
}

type Artifact struct {
	ID           string
	DeploymentID ID
	WorkspaceID  WorkspaceID
	Digest       string
	Format       string
	Path         string
	ManifestJSON string
	SizeBytes    int64
	CreatedAt    string
}

type Asset struct {
	ID             string
	WorkspaceID    WorkspaceID
	DeploymentID   ID
	Type           string
	Key            string
	ParentID       string
	Title          string
	Description    string
	ContentJSON    string
	ContentHash    string
	ContentVersion int
}

type AssetEdge struct {
	ID           string
	WorkspaceID  WorkspaceID
	DeploymentID ID
	FromAssetID  string
	ToAssetID    string
	Type         string
}

type Validation struct {
	Digest       string
	ManifestJSON string
	RootDir      string
	Assets       []Asset
	Edges        []AssetEdge
}

type PreparedRuntime interface {
	Close() error
}

func NewAsset(workspaceID WorkspaceID, deploymentID ID, typ, key, parentID, title, description string, content any) (Asset, error) {
	bytes, err := json.Marshal(content)
	if err != nil {
		return Asset{}, err
	}
	sum := sha256.Sum256(bytes)
	return Asset{
		ID:           "asset_" + stableID(string(deploymentID)+"|"+typ+"|"+key),
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

func NewAssetEdge(workspaceID WorkspaceID, deploymentID ID, fromID, toID, typ string) AssetEdge {
	return AssetEdge{
		ID:           "edge_" + stableID(string(deploymentID)+"|"+fromID+"|"+toID+"|"+typ),
		WorkspaceID:  workspaceID,
		DeploymentID: deploymentID,
		FromAssetID:  fromID,
		ToAssetID:    toID,
		Type:         typ,
	}
}

func stableID(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(value)))
	return hex.EncodeToString(sum[:])[:32]
}
