package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/Yacobolo/libredash/pkg/agent"
)

func (s *Server) agentAssetToolDefinitions(scope agentapp.Scope) []agent.ToolDefinition {
	return []agent.ToolDefinition{
		{
			Name:        "list_assets",
			Description: "List bounded logical asset summaries from the active workspace asset catalog.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"type":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":100}},"additionalProperties":false}`),
			Handler: s.agentAssetTool(scope, func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					Type  string `json:"type"`
					Limit int    `json:"limit"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				assets, _, ok, err := s.agentAssetViews(ctx, scope.WorkspaceID)
				if err != nil {
					return nil, err
				}
				if !ok {
					return agentAssetListPayload{Assets: []agentAssetSummary{}}, nil
				}
				return agentAssetList(assets, input.Type, input.Limit), nil
			}),
		},
		{
			Name:        "describe_asset",
			Description: "Describe one logical asset and return its typed payload from the active workspace asset catalog.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"asset_id":{"type":"string"}},"required":["asset_id"],"additionalProperties":false}`),
			Handler: s.agentAssetTool(scope, func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					AssetID string `json:"asset_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				assets, edges, ok, err := s.agentAssetViews(ctx, scope.WorkspaceID)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("no active asset graph for workspace %q", scope.WorkspaceID)
				}
				return agentDescribeAsset(assets, edges, input.AssetID)
			}),
		},
		{
			Name:        "asset_lineage",
			Description: "Return upstream and downstream logical asset IDs for one asset in the active workspace asset catalog.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"asset_id":{"type":"string"}},"required":["asset_id"],"additionalProperties":false}`),
			Handler: s.agentAssetTool(scope, func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					AssetID string `json:"asset_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				assets, edges, ok, err := s.agentAssetViews(ctx, scope.WorkspaceID)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("no active asset graph for workspace %q", scope.WorkspaceID)
				}
				return agentAssetLineage(assets, edges, input.AssetID)
			}),
		},
	}
}

func (s *Server) agentAssetTool(scope agentapp.Scope, fn func(context.Context, json.RawMessage) (any, error)) agent.ToolHandler {
	return agent.ToolHandlerFunc(func(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
		if errResult, ok := s.authorizeAgentPermission(ctx, scope, access.PermissionAssetRead); !ok {
			return errResult, nil
		}
		content, err := fn(ctx, call.Arguments)
		if err != nil {
			return agent.ToolResult{}, err
		}
		return agent.ToolResult{Content: content}, nil
	})
}

func (s *Server) agentAssetViews(ctx context.Context, workspaceID string) ([]workspace.AssetView, []workspace.AssetEdgeView, bool, error) {
	catalog, ok, err := s.workspaceAssetCatalog(ctx, workspaceID)
	if err != nil || !ok {
		return nil, nil, ok, err
	}
	assets := make([]workspace.AssetView, 0, len(catalog.Assets))
	for _, row := range catalog.Assets {
		assets = append(assets, workspace.AssetViewFromCatalogRecord(row))
	}
	edges := make([]workspace.AssetEdgeView, 0, len(catalog.Edges))
	for _, row := range catalog.Edges {
		edges = append(edges, workspace.AssetEdgeViewFromCatalogRecord(row))
	}
	return assets, edges, true, nil
}

type agentAssetListPayload struct {
	Assets []agentAssetSummary `json:"assets"`
}

type agentAssetSummary struct {
	ID            string `json:"id"`
	SnapshotID    string `json:"snapshot_id,omitempty"`
	Type          string `json:"type"`
	Key           string `json:"key"`
	ParentID      string `json:"parent_id,omitempty"`
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	PayloadSchema string `json:"payload_schema"`
	ContentHash   string `json:"content_hash"`
}

type agentAssetDescriptionPayload struct {
	Asset   agentAssetSummary      `json:"asset"`
	Payload map[string]any         `json:"payload"`
	Lineage agentAssetLineageReply `json:"lineage"`
}

type agentAssetLineageReply struct {
	AssetID    string   `json:"asset_id"`
	Upstream   []string `json:"upstream"`
	Downstream []string `json:"downstream"`
}

func agentAssetList(assets []workspace.AssetView, typ string, limit int) agentAssetListPayload {
	if limit <= 0 || limit > 100 {
		limit = maxAgentRows
	}
	typ = strings.TrimSpace(strings.ToLower(typ))
	rows := append([]workspace.AssetView(nil), assets...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	out := make([]agentAssetSummary, 0, min(len(rows), limit))
	for _, asset := range rows {
		if typ != "" && asset.Type != typ {
			continue
		}
		out = append(out, agentSummarizeAsset(asset))
		if len(out) >= limit {
			break
		}
	}
	return agentAssetListPayload{Assets: out}
}

func agentDescribeAsset(assets []workspace.AssetView, edges []workspace.AssetEdgeView, assetID string) (agentAssetDescriptionPayload, error) {
	asset, ok := workspace.AssetByID(assets, assetID)
	if !ok {
		return agentAssetDescriptionPayload{}, fmt.Errorf("asset %q not found", assetID)
	}
	lineage, err := agentAssetLineage(assets, edges, assetID)
	if err != nil {
		return agentAssetDescriptionPayload{}, err
	}
	return agentAssetDescriptionPayload{Asset: agentSummarizeAsset(asset), Payload: asset.Payload, Lineage: lineage}, nil
}

func agentAssetLineage(assets []workspace.AssetView, edges []workspace.AssetEdgeView, assetID string) (agentAssetLineageReply, error) {
	if _, ok := workspace.AssetByID(assets, assetID); !ok {
		return agentAssetLineageReply{}, fmt.Errorf("asset %q not found", assetID)
	}
	upstreamSet := map[string]struct{}{}
	downstreamSet := map[string]struct{}{}
	for _, edge := range edges {
		if edge.ToAssetID == assetID {
			upstreamSet[edge.FromAssetID] = struct{}{}
		}
		if edge.FromAssetID == assetID {
			downstreamSet[edge.ToAssetID] = struct{}{}
		}
	}
	return agentAssetLineageReply{
		AssetID:    assetID,
		Upstream:   sortedAgentAssetIDs(upstreamSet),
		Downstream: sortedAgentAssetIDs(downstreamSet),
	}, nil
}

func agentSummarizeAsset(asset workspace.AssetView) agentAssetSummary {
	return agentAssetSummary{
		ID:            asset.ID,
		SnapshotID:    asset.SnapshotID,
		Type:          asset.Type,
		Key:           asset.Key,
		ParentID:      asset.ParentID,
		Title:         asset.Title,
		Description:   asset.Description,
		PayloadSchema: asset.PayloadSchema,
		ContentHash:   asset.ContentHash,
	}
}

func sortedAgentAssetIDs(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
