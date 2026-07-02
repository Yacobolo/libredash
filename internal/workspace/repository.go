package workspace

import "context"

type Summary struct {
	ID                 WorkspaceID
	Title              string
	Description        string
	ActiveDeploymentID DeploymentID
	CreatedAt          string
	UpdatedAt          string
}

type AssetVersion struct {
	DeploymentID DeploymentID
	WorkspaceID  WorkspaceID
	Environment  string
	Status       string
	Digest       string
	CreatedBy    string
	CreatedAt    string
	ActivatedAt  string
	SnapshotID   AssetSnapshotID
	AssetID      AssetID
	ContentHash  string
}

type EnsureInput struct {
	ID          WorkspaceID
	Title       string
	Description string
}

type Repository interface {
	Ensure(ctx context.Context, input EnsureInput) error
	List(ctx context.Context) ([]Summary, error)
	ByID(ctx context.Context, id WorkspaceID) (Summary, error)
	ActiveDeploymentGraph(ctx context.Context, id WorkspaceID, environment string) (AssetGraph, bool, error)
	AssetVersions(ctx context.Context, workspaceID WorkspaceID, environment string, assetID AssetID) ([]AssetVersion, error)
}
