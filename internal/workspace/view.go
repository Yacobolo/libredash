package workspace

import "strings"

type WorkspaceView struct {
	ID                 string
	Title              string
	Description        string
	ActiveDeploymentID string
	CreatedAt          string
	UpdatedAt          string
}

type AssetView struct {
	ID            string
	SnapshotID    string
	WorkspaceID   string
	DeploymentID  string
	Type          string
	Key           string
	ParentID      string
	Title         string
	Description   string
	PayloadSchema string
	Payload       map[string]any
	ContentHash   string
	Href          string
}

type AssetEdgeView struct {
	ID           string
	WorkspaceID  string
	DeploymentID string
	FromAssetID  string
	ToAssetID    string
	Type         string
}

type RoleView struct {
	Name        string
	Permissions []string
}

type RoleBindingView struct {
	ID          string
	WorkspaceID string
	SubjectType string
	SubjectID   string
	PrincipalID string
	GroupID     string
	Email       string
	DisplayName string
	GroupName   string
	Role        string
	CreatedAt   string
}

func WorkspaceViewFromSummary(row Summary) WorkspaceView {
	activeDeploymentID := ""
	if row.ActiveDeploymentID != "" {
		activeDeploymentID = string(row.ActiveDeploymentID)
	}
	return WorkspaceView{
		ID:                 string(row.ID),
		Title:              row.Title,
		Description:        row.Description,
		ActiveDeploymentID: activeDeploymentID,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func AssetViewFromCatalogRecord(row AssetRecord) AssetView {
	return AssetView{
		ID:            string(row.ID),
		SnapshotID:    string(row.SnapshotID),
		WorkspaceID:   string(row.WorkspaceID),
		DeploymentID:  string(row.DeploymentID),
		Type:          string(row.Type),
		Key:           row.Key,
		ParentID:      string(row.ParentID),
		Title:         row.Title,
		Description:   row.Description,
		PayloadSchema: row.PayloadSchema,
		Payload:       row.Payload,
		ContentHash:   row.ContentHash,
		Href:          AssetHref(string(row.Type), row.Key),
	}
}

func AssetEdgeViewFromCatalogRecord(row AssetEdgeRecord) AssetEdgeView {
	return AssetEdgeView{
		ID:           string(row.ID),
		WorkspaceID:  string(row.WorkspaceID),
		DeploymentID: string(row.DeploymentID),
		FromAssetID:  string(row.FromAssetID),
		ToAssetID:    string(row.ToAssetID),
		Type:         string(row.Type),
	}
}

func AssetHref(assetType, key string) string {
	switch assetType {
	case string(AssetTypeDashboard):
		return "/dashboards/" + key
	default:
		return ""
	}
}

func FilterAssets(assets []AssetView, typ, query string) []AssetView {
	typ = strings.TrimSpace(typ)
	query = strings.ToLower(strings.TrimSpace(query))
	if typ == "" && query == "" {
		return assets
	}
	out := make([]AssetView, 0, len(assets))
	for _, asset := range assets {
		if typ != "" && asset.Type != typ {
			continue
		}
		haystack := strings.ToLower(asset.Type + " " + asset.Key + " " + asset.Title + " " + asset.Description)
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		out = append(out, asset)
	}
	return out
}

func FilterWorkspaceAssets(assets []AssetView, typ, query string) []AssetView {
	typ = strings.TrimSpace(typ)
	query = strings.TrimSpace(query)
	if typ != "" || query != "" {
		return FilterAssets(assets, typ, query)
	}
	out := make([]AssetView, 0, len(assets))
	for _, asset := range assets {
		if IsWorkspaceLandingAsset(asset.Type) {
			out = append(out, asset)
		}
	}
	return out
}

func FilterConnectionAssets(assets []AssetView, typ, query string) []AssetView {
	typ = NormalizeConnectionAssetType(typ)
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]AssetView, 0, len(assets))
	for _, asset := range assets {
		if asset.Type != string(AssetTypeConnection) && asset.Type != string(AssetTypeSource) {
			continue
		}
		if typ != "" && asset.Type != typ {
			continue
		}
		haystack := strings.ToLower(asset.Type + " " + asset.Key + " " + asset.Title + " " + asset.Description)
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		out = append(out, asset)
	}
	return out
}

func NormalizeConnectionAssetType(typ string) string {
	switch strings.TrimSpace(typ) {
	case string(AssetTypeConnection), string(AssetTypeSource):
		return strings.TrimSpace(typ)
	default:
		return ""
	}
}

func AssetByID(assets []AssetView, id string) (AssetView, bool) {
	for _, asset := range assets {
		if asset.ID == id {
			return asset, true
		}
	}
	return AssetView{}, false
}

func IsWorkspaceLandingAsset(typ string) bool {
	switch typ {
	case string(AssetTypeModelTable), string(AssetTypeSemanticModel), string(AssetTypeDashboard):
		return true
	default:
		return false
	}
}

func SourceConnectionID(sourceID string, edges []AssetEdgeView) string {
	for _, edge := range edges {
		if edge.Type == string(AssetEdgeUsesConnection) && edge.FromAssetID == sourceID {
			return edge.ToAssetID
		}
	}
	return ""
}
