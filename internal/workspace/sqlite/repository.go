package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/Yacobolo/libredash/internal/deployment"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Repository struct {
	q *platformdb.Queries
}

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{q: platformdb.New(sqlDB)}
}

func (r *Repository) Ensure(ctx context.Context, input workspace.EnsureInput) error {
	id := strings.TrimSpace(string(input.ID))
	if id == "" {
		id = "libredash"
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = id
	}
	return r.q.UpsertWorkspace(ctx, platformdb.UpsertWorkspaceParams{
		ID:          id,
		Title:       title,
		Description: input.Description,
	})
}

func (r *Repository) List(ctx context.Context) ([]workspace.Summary, error) {
	rows, err := r.q.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	workspaces := make([]workspace.Summary, 0, len(rows))
	for _, row := range rows {
		workspaces = append(workspaces, mapWorkspace(row))
	}
	return workspaces, nil
}

func (r *Repository) ByID(ctx context.Context, id workspace.WorkspaceID) (workspace.Summary, error) {
	row, err := r.q.GetWorkspace(ctx, string(id))
	if err != nil {
		return workspace.Summary{}, err
	}
	return mapWorkspace(row), nil
}

func (r *Repository) ActiveDeploymentGraph(ctx context.Context, id workspace.WorkspaceID, environment string) (workspace.AssetGraph, bool, error) {
	activeDeployment, err := r.q.GetActiveDeployment(ctx, platformdb.GetActiveDeploymentParams{
		WorkspaceID: string(id),
		Environment: string(deployment.NormalizeEnvironment(deployment.Environment(environment))),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workspace.AssetGraph{}, false, nil
		}
		return workspace.AssetGraph{}, false, err
	}
	assetRows, err := r.q.ListAssetsByDeployment(ctx, activeDeployment.ID)
	if err != nil {
		return workspace.AssetGraph{}, false, err
	}
	edgeRows, err := r.q.ListAssetEdgesByDeployment(ctx, activeDeployment.ID)
	if err != nil {
		return workspace.AssetGraph{}, false, err
	}
	graph := workspace.AssetGraph{
		Assets: make([]workspace.Asset, 0, len(assetRows)),
		Edges:  make([]workspace.AssetEdge, 0, len(edgeRows)),
	}
	for _, row := range assetRows {
		graph.Assets = append(graph.Assets, mapAsset(row))
	}
	for _, row := range edgeRows {
		graph.Edges = append(graph.Edges, mapAssetEdge(row))
	}
	return graph, true, nil
}

func mapWorkspace(row platformdb.Workspace) workspace.Summary {
	out := workspace.Summary{
		ID:          workspace.WorkspaceID(row.ID),
		Title:       row.Title,
		Description: row.Description,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	return out
}

func mapAsset(row platformdb.Asset) workspace.Asset {
	return workspace.Asset{
		ID:            workspace.AssetID(row.LogicalAssetID),
		SnapshotID:    workspace.AssetSnapshotID(row.SnapshotID),
		WorkspaceID:   workspace.WorkspaceID(row.WorkspaceID),
		DeploymentID:  workspace.DeploymentID(row.DeploymentID),
		Type:          workspace.AssetType(row.AssetType),
		Key:           row.AssetKey,
		ParentID:      workspace.AssetID(row.ParentLogicalAssetID),
		Title:         row.Title,
		Description:   row.Description,
		SourceFile:    row.SourceFile,
		PayloadSchema: row.PayloadSchema,
		PayloadJSON:   row.PayloadJson,
		ContentHash:   row.ContentHash,
	}
}

func mapAssetEdge(row platformdb.AssetEdge) workspace.AssetEdge {
	return workspace.AssetEdge{
		ID:           workspace.AssetEdgeID(row.ID),
		WorkspaceID:  workspace.WorkspaceID(row.WorkspaceID),
		DeploymentID: workspace.DeploymentID(row.DeploymentID),
		FromAssetID:  workspace.AssetID(row.FromLogicalAssetID),
		ToAssetID:    workspace.AssetID(row.ToLogicalAssetID),
		Type:         workspace.AssetEdgeType(row.EdgeType),
	}
}
