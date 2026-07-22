package agent

import (
	"strings"

	agentcore "github.com/Yacobolo/leapview/pkg/agent"
)

const dashboardTurnContextSurface = "dashboard"

const MaxTurnReferences = 12

// TurnContext is server-resolved product context for one user turn. It is
// deliberately separate from Scope: Scope controls authorization, while this
// value describes the dashboard state the user is asking about.
type TurnContext struct {
	Surface        string          `json:"surface"`
	WorkspaceID    string          `json:"workspaceId,omitempty"`
	DashboardID    string          `json:"dashboardId,omitempty"`
	DashboardTitle string          `json:"dashboardTitle,omitempty"`
	PageID         string          `json:"pageId,omitempty"`
	PageTitle      string          `json:"pageTitle,omitempty"`
	ModelID        string          `json:"modelId,omitempty"`
	Generation     int64           `json:"generation,omitempty"`
	Filters        map[string]any  `json:"filters,omitempty"`
	References     []TurnReference `json:"references,omitempty"`
}

type TurnReference struct {
	Reference   TurnReferenceKey        `json:"reference"`
	Name        string                  `json:"name,omitempty"`
	Description string                  `json:"description,omitempty"`
	Workspace   TurnReferenceWorkspace  `json:"workspace"`
	Hierarchy   []string                `json:"hierarchy,omitempty"`
	Href        string                  `json:"href,omitempty"`
	Locations   []TurnReferenceLocation `json:"locations,omitempty"`
	Context     []string                `json:"context,omitempty"`

	// The fields below are derived server-side and enrich model context. They
	// are never trusted when supplied by a client.
	ComponentID string `json:"componentId,omitempty"`
	VisualID    string `json:"visualId,omitempty"`
	VisualType  string `json:"visualType,omitempty"`
	DashboardID string `json:"dashboardId,omitempty"`
	PageID      string `json:"pageId,omitempty"`
	TableID     string `json:"tableId,omitempty"`
	FilterID    string `json:"filterId,omitempty"`
	ModelID     string `json:"modelId,omitempty"`
	DatasetID   string `json:"datasetId,omitempty"`
	FieldID     string `json:"fieldId,omitempty"`
	AssetID     string `json:"assetId,omitempty"`
}

type TurnReferenceKey struct {
	WorkspaceID string `json:"workspaceId"`
	Type        string `json:"type"`
	ID          string `json:"id"`
}

type TurnReferenceWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TurnReferenceLocation struct {
	DashboardID   string `json:"dashboardId,omitempty"`
	DashboardName string `json:"dashboardName,omitempty"`
	PageID        string `json:"pageId,omitempty"`
	PageName      string `json:"pageName,omitempty"`
	Href          string `json:"href"`
}

func (c TurnContext) normalized() TurnContext {
	c.Surface = strings.ToLower(strings.TrimSpace(c.Surface))
	c.WorkspaceID = strings.TrimSpace(c.WorkspaceID)
	c.DashboardID = strings.TrimSpace(c.DashboardID)
	c.DashboardTitle = strings.TrimSpace(c.DashboardTitle)
	c.PageID = strings.TrimSpace(c.PageID)
	c.PageTitle = strings.TrimSpace(c.PageTitle)
	c.ModelID = strings.TrimSpace(c.ModelID)
	refs := make([]TurnReference, 0, len(c.References))
	seen := map[string]struct{}{}
	for _, ref := range c.References {
		ref.Reference.Type = strings.ToLower(strings.TrimSpace(ref.Reference.Type))
		ref.Reference.ID = strings.TrimSpace(ref.Reference.ID)
		ref.Reference.WorkspaceID = strings.TrimSpace(ref.Reference.WorkspaceID)
		ref.Name = strings.TrimSpace(ref.Name)
		ref.Description = strings.TrimSpace(ref.Description)
		ref.Workspace.ID = strings.TrimSpace(ref.Workspace.ID)
		ref.Workspace.Name = strings.TrimSpace(ref.Workspace.Name)
		hierarchy := make([]string, 0, len(ref.Hierarchy))
		for _, part := range ref.Hierarchy {
			if part = strings.TrimSpace(part); part != "" {
				hierarchy = append(hierarchy, part)
			}
		}
		ref.Hierarchy = hierarchy
		ref.Href = strings.TrimSpace(ref.Href)
		ref.ComponentID = strings.TrimSpace(ref.ComponentID)
		ref.VisualID = strings.TrimSpace(ref.VisualID)
		ref.VisualType = strings.ToLower(strings.TrimSpace(ref.VisualType))
		ref.DashboardID = strings.TrimSpace(ref.DashboardID)
		ref.PageID = strings.TrimSpace(ref.PageID)
		ref.TableID = strings.TrimSpace(ref.TableID)
		ref.FilterID = strings.TrimSpace(ref.FilterID)
		ref.ModelID = strings.TrimSpace(ref.ModelID)
		ref.DatasetID = strings.TrimSpace(ref.DatasetID)
		ref.FieldID = strings.TrimSpace(ref.FieldID)
		ref.AssetID = strings.TrimSpace(ref.AssetID)
		if ref.Reference.Type == "" || ref.Reference.ID == "" || ref.Reference.WorkspaceID == "" {
			continue
		}
		key := ref.Reference.WorkspaceID + ":" + ref.Reference.Type + ":" + ref.Reference.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, ref)
	}
	c.References = refs
	return c
}

func turnContextItems(context *TurnContext) []agentcore.ContextItem {
	if context == nil {
		return nil
	}
	normalized := context.normalized()
	if normalized.Surface != dashboardTurnContextSurface && (normalized.Surface != "chat" || len(normalized.References) == 0) {
		return nil
	}
	return []agentcore.ContextItem{{Key: "leapview_context", Value: normalized}}
}
