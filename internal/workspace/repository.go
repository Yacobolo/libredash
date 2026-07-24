package workspace

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("workspace not found")

type Summary struct {
	ID                   WorkspaceID
	Title                string
	Description          string
	ActiveServingStateID ServingStateID
	CreatedAt            string
	UpdatedAt            string
}

type AssetVersion struct {
	ServingStateID ServingStateID
	WorkspaceID    WorkspaceID
	Environment    string
	Status         string
	Digest         string
	CreatedBy      string
	CreatedAt      string
	ActivatedAt    string
	SnapshotID     AssetSnapshotID
	AssetID        AssetID
	SourceFile     string
	ContentHash    string
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
	ActiveServingStateGraph(ctx context.Context, id WorkspaceID, environment string) (AssetGraph, bool, error)
	AssetVersions(ctx context.Context, workspaceID WorkspaceID, environment string, assetID AssetID) ([]AssetVersion, error)
}
