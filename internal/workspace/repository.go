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
}
