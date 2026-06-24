package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Repository struct {
	db *sql.DB
	q  *platformdb.Queries
}

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{db: sqlDB, q: platformdb.New(sqlDB)}
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

func (r *Repository) ActiveDeploymentGraph(ctx context.Context, id workspace.WorkspaceID) (workspace.AssetGraph, bool, error) {
	deployment, err := r.q.GetActiveDeployment(ctx, string(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workspace.AssetGraph{}, false, nil
		}
		return workspace.AssetGraph{}, false, err
	}
	assetRows, err := r.q.ListAssetsByDeployment(ctx, deployment.ID)
	if err != nil {
		return workspace.AssetGraph{}, false, err
	}
	edgeRows, err := r.q.ListAssetEdgesByDeployment(ctx, deployment.ID)
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

func (r *Repository) ReplaceActiveDeploymentGraph(ctx context.Context, id workspace.WorkspaceID, graph workspace.AssetGraph) error {
	deployment, err := r.q.GetActiveDeployment(ctx, string(id))
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	if err := q.ClearAssetsForDeployment(ctx, deployment.ID); err != nil {
		return err
	}
	for _, asset := range graph.Assets {
		if string(asset.DeploymentID) != deployment.ID {
			continue
		}
		if err := q.InsertAsset(ctx, platformdb.InsertAssetParams{
			ID:             string(asset.ID),
			WorkspaceID:    string(asset.WorkspaceID),
			DeploymentID:   string(asset.DeploymentID),
			AssetType:      string(asset.Type),
			AssetKey:       asset.Key,
			ParentAssetID:  sql.NullString{String: string(asset.ParentID), Valid: asset.ParentID != ""},
			Title:          asset.Title,
			Description:    asset.Description,
			ContentJson:    asset.ContentJSON,
			ContentHash:    asset.ContentHash,
			ContentVersion: int64(asset.ContentVersion),
		}); err != nil {
			return err
		}
	}
	for _, edge := range graph.Edges {
		if string(edge.DeploymentID) != deployment.ID {
			continue
		}
		if err := q.InsertAssetEdge(ctx, platformdb.InsertAssetEdgeParams{
			ID:           string(edge.ID),
			WorkspaceID:  string(edge.WorkspaceID),
			DeploymentID: string(edge.DeploymentID),
			FromAssetID:  string(edge.FromAssetID),
			ToAssetID:    string(edge.ToAssetID),
			EdgeType:     string(edge.Type),
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func mapWorkspace(row platformdb.Workspace) workspace.Summary {
	out := workspace.Summary{
		ID:          workspace.WorkspaceID(row.ID),
		Title:       row.Title,
		Description: row.Description,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.ActiveDeploymentID.Valid {
		out.ActiveDeploymentID = workspace.DeploymentID(row.ActiveDeploymentID.String)
	}
	return out
}

func mapAsset(row platformdb.Asset) workspace.Asset {
	parentID := workspace.AssetID("")
	if row.ParentAssetID.Valid {
		parentID = workspace.AssetID(row.ParentAssetID.String)
	}
	return workspace.Asset{
		ID:             workspace.AssetID(row.ID),
		WorkspaceID:    workspace.WorkspaceID(row.WorkspaceID),
		DeploymentID:   workspace.DeploymentID(row.DeploymentID),
		Type:           workspace.AssetType(row.AssetType),
		Key:            row.AssetKey,
		ParentID:       parentID,
		Title:          row.Title,
		Description:    row.Description,
		ContentJSON:    row.ContentJson,
		ContentHash:    row.ContentHash,
		ContentVersion: int(row.ContentVersion),
	}
}

func mapAssetEdge(row platformdb.AssetEdge) workspace.AssetEdge {
	return workspace.AssetEdge{
		ID:           workspace.AssetEdgeID(row.ID),
		WorkspaceID:  workspace.WorkspaceID(row.WorkspaceID),
		DeploymentID: workspace.DeploymentID(row.DeploymentID),
		FromAssetID:  workspace.AssetID(row.FromAssetID),
		ToAssetID:    workspace.AssetID(row.ToAssetID),
		Type:         workspace.AssetEdgeType(row.EdgeType),
	}
}
