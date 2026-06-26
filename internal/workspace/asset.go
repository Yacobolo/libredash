package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type Asset struct {
	ID            AssetID
	SnapshotID    AssetSnapshotID
	WorkspaceID   WorkspaceID
	DeploymentID  DeploymentID
	Type          AssetType
	Key           string
	ParentID      AssetID
	Title         string
	Description   string
	PayloadSchema string
	PayloadJSON   string
	ContentHash   string
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

func NewAsset(workspaceID WorkspaceID, deploymentID DeploymentID, typ AssetType, key string, parentID AssetID, title, description, payloadSchema string, payload any) (Asset, error) {
	if err := validatePayloadSchema(typ, payloadSchema); err != nil {
		return Asset{}, err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return Asset{}, err
	}
	id := NewAssetID(typ, key)
	hashBytes, err := json.Marshal(assetHashPayload{
		Type:          typ,
		Key:           key,
		ParentID:      parentID,
		Title:         title,
		Description:   description,
		PayloadSchema: payloadSchema,
		PayloadJSON:   json.RawMessage(payloadBytes),
	})
	if err != nil {
		return Asset{}, err
	}
	sum := sha256.Sum256(hashBytes)
	return Asset{
		ID:            id,
		SnapshotID:    NewAssetSnapshotID(deploymentID, id),
		WorkspaceID:   workspaceID,
		DeploymentID:  deploymentID,
		Type:          typ,
		Key:           key,
		ParentID:      parentID,
		Title:         title,
		Description:   description,
		PayloadSchema: payloadSchema,
		PayloadJSON:   string(payloadBytes),
		ContentHash:   hex.EncodeToString(sum[:]),
	}, nil
}

type assetHashPayload struct {
	Type          AssetType       `json:"type"`
	Key           string          `json:"key"`
	ParentID      AssetID         `json:"parentId,omitempty"`
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	PayloadSchema string          `json:"payloadSchema"`
	PayloadJSON   json.RawMessage `json:"payload"`
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

func ValidateAssetGraphForDeployment(graph AssetGraph, workspaceID WorkspaceID, deploymentID DeploymentID) error {
	assetIDs := make(map[AssetID]struct{}, len(graph.Assets))
	for _, asset := range graph.Assets {
		if asset.ID == "" {
			return fmt.Errorf("asset logical id is required")
		}
		if _, ok := assetIDs[asset.ID]; ok {
			return fmt.Errorf("asset %s is duplicated", asset.ID)
		}
		assetIDs[asset.ID] = struct{}{}
		if asset.WorkspaceID != workspaceID {
			return fmt.Errorf("asset %s workspace = %q, want %q", asset.ID, asset.WorkspaceID, workspaceID)
		}
		if asset.DeploymentID != deploymentID {
			return fmt.Errorf("asset %s deployment = %q, want %q", asset.ID, asset.DeploymentID, deploymentID)
		}
		if want := NewAssetSnapshotID(deploymentID, asset.ID); asset.SnapshotID != want {
			return fmt.Errorf("asset %s snapshot id = %q, want %q", asset.ID, asset.SnapshotID, want)
		}
		if err := validatePayloadSchema(asset.Type, asset.PayloadSchema); err != nil {
			return fmt.Errorf("asset %s: %w", asset.ID, err)
		}
	}
	for _, asset := range graph.Assets {
		if asset.ParentID == "" {
			continue
		}
		if _, ok := assetIDs[asset.ParentID]; !ok {
			return fmt.Errorf("asset %s parent %s is not in graph", asset.ID, asset.ParentID)
		}
	}

	edgeKeys := make(map[assetEdgeKey]struct{}, len(graph.Edges))
	for _, edge := range graph.Edges {
		if edge.WorkspaceID != workspaceID {
			return fmt.Errorf("asset edge %s workspace = %q, want %q", edge.ID, edge.WorkspaceID, workspaceID)
		}
		if edge.DeploymentID != deploymentID {
			return fmt.Errorf("asset edge %s deployment = %q, want %q", edge.ID, edge.DeploymentID, deploymentID)
		}
		if want := NewAssetEdgeID(deploymentID, edge.FromAssetID, edge.ToAssetID, edge.Type); edge.ID != want {
			return fmt.Errorf("asset edge %s id = %q, want %q", edge.Type, edge.ID, want)
		}
		if _, ok := assetIDs[edge.FromAssetID]; !ok {
			return fmt.Errorf("asset edge %s from asset %s is not in graph", edge.ID, edge.FromAssetID)
		}
		if _, ok := assetIDs[edge.ToAssetID]; !ok {
			return fmt.Errorf("asset edge %s to asset %s is not in graph", edge.ID, edge.ToAssetID)
		}
		key := assetEdgeKey{from: edge.FromAssetID, to: edge.ToAssetID, typ: edge.Type}
		if _, ok := edgeKeys[key]; ok {
			return fmt.Errorf("asset edge %s -> %s (%s) is duplicated", edge.FromAssetID, edge.ToAssetID, edge.Type)
		}
		edgeKeys[key] = struct{}{}
	}
	return nil
}

type assetEdgeKey struct {
	from AssetID
	to   AssetID
	typ  AssetEdgeType
}

func validatePayloadSchema(typ AssetType, schema string) error {
	want := PayloadSchemaForAssetType(typ)
	if want == "" {
		return fmt.Errorf("asset %s payload schema is not registered", typ)
	}
	if schema == "" {
		return fmt.Errorf("asset %s payload schema is required", typ)
	}
	if schema != want {
		return fmt.Errorf("asset %s payload schema = %q, want %q", typ, schema, want)
	}
	return nil
}

func PayloadSchemaForAssetType(typ AssetType) string {
	switch typ {
	case AssetTypeCatalog:
		return "catalog.v1"
	case AssetTypeConnection:
		return "connection.v1"
	case AssetTypeSource:
		return "source.v1"
	case AssetTypeModelTable:
		return "model_table.v1"
	case AssetTypeSemanticModel:
		return "semantic_model.v1"
	case AssetTypeSemanticTable:
		return "semantic_table.v1"
	case AssetTypeField:
		return "field.v1"
	case AssetTypeMeasure:
		return "measure.v1"
	case AssetTypeRelationship:
		return "relationship.v1"
	case AssetTypeDashboard:
		return "dashboard.v1"
	case AssetTypePage:
		return "page.v1"
	case AssetTypePageItem:
		return "page_item.v1"
	case AssetTypeFilter:
		return "filter.v1"
	case AssetTypeVisual:
		return "visual.v1"
	case AssetTypeTable:
		return "table.v1"
	default:
		return ""
	}
}
