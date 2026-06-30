package workspace

import (
	"context"
	"encoding/json"
	"fmt"
)

type AssetCatalog struct {
	Assets []AssetRecord
	Edges  []AssetEdgeRecord
}

type AssetRecord struct {
	ID            AssetID
	SnapshotID    AssetSnapshotID
	WorkspaceID   WorkspaceID
	DeploymentID  DeploymentID
	Type          AssetType
	Key           string
	ParentID      AssetID
	Title         string
	Description   string
	SourceFile    string
	PayloadSchema string
	Payload       map[string]any
	ContentHash   string
}

type AssetEdgeRecord struct {
	ID           AssetEdgeID
	WorkspaceID  WorkspaceID
	DeploymentID DeploymentID
	FromAssetID  AssetID
	ToAssetID    AssetID
	Type         AssetEdgeType
}

type AssetCatalogService struct {
	repo    Repository
	runtime RuntimeAssetGraphProvider
}

func NewAssetCatalogService(repo Repository) *AssetCatalogService {
	return &AssetCatalogService{repo: repo}
}

type RuntimeAssetGraphProvider interface {
	WorkspaceAssets(workspaceID, deploymentID string) ([]Asset, []AssetEdge, bool)
}

type AssetCatalogReader interface {
	ActiveAssetCatalog(ctx context.Context, id WorkspaceID, environment string) (AssetCatalog, bool, error)
}

func (s *AssetCatalogService) WithRuntimeProvider(provider RuntimeAssetGraphProvider) *AssetCatalogService {
	s.runtime = provider
	return s
}

func (s *AssetCatalogService) ActiveAssetCatalog(ctx context.Context, id WorkspaceID, environment string) (AssetCatalog, bool, error) {
	if s == nil {
		return AssetCatalog{}, false, nil
	}
	if s.repo != nil {
		graph, ok, err := s.repo.ActiveDeploymentGraph(ctx, id, environment)
		if err != nil {
			return AssetCatalog{}, false, err
		}
		if ok {
			catalog, err := DecodeAssetCatalog(graph)
			return catalog, true, err
		}
	}
	if s.runtime == nil {
		return AssetCatalog{}, false, nil
	}
	assets, edges, ok := s.runtime.WorkspaceAssets(string(id), "local")
	if !ok {
		return AssetCatalog{}, false, nil
	}
	catalog, err := DecodeAssetCatalog(AssetGraph{Assets: assets, Edges: edges})
	return catalog, true, err
}

func DecodeAssetCatalog(graph AssetGraph) (AssetCatalog, error) {
	catalog := AssetCatalog{
		Assets: make([]AssetRecord, 0, len(graph.Assets)),
		Edges:  make([]AssetEdgeRecord, 0, len(graph.Edges)),
	}
	for _, asset := range graph.Assets {
		payload := map[string]any{}
		if asset.PayloadJSON != "" {
			if err := json.Unmarshal([]byte(asset.PayloadJSON), &payload); err != nil {
				return AssetCatalog{}, fmt.Errorf("decode asset %s payload: %w", asset.ID, err)
			}
		}
		catalog.Assets = append(catalog.Assets, AssetRecord{
			ID:            asset.ID,
			SnapshotID:    asset.SnapshotID,
			WorkspaceID:   asset.WorkspaceID,
			DeploymentID:  asset.DeploymentID,
			Type:          asset.Type,
			Key:           asset.Key,
			ParentID:      asset.ParentID,
			Title:         asset.Title,
			Description:   asset.Description,
			SourceFile:    asset.SourceFile,
			PayloadSchema: asset.PayloadSchema,
			Payload:       payload,
			ContentHash:   asset.ContentHash,
		})
	}
	for _, edge := range graph.Edges {
		catalog.Edges = append(catalog.Edges, AssetEdgeRecord{
			ID:           edge.ID,
			WorkspaceID:  edge.WorkspaceID,
			DeploymentID: edge.DeploymentID,
			FromAssetID:  edge.FromAssetID,
			ToAssetID:    edge.ToAssetID,
			Type:         edge.Type,
		})
	}
	return catalog, nil
}
