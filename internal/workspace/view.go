package workspace

import (
	"encoding/json"
	"strings"
)

type WorkspaceView struct {
	ID                 string
	Title              string
	Description        string
	ActiveDeploymentID string
	CreatedAt          string
	UpdatedAt          string
}

type AssetView struct {
	ID           string
	WorkspaceID  string
	DeploymentID string
	Type         string
	Key          string
	ParentID     string
	Title        string
	Description  string
	Meta         map[string]any
	Href         string
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

type FallbackAssetInput struct {
	ID          string
	Type        string
	Key         string
	Title       string
	Description string
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

func AssetViewFromAsset(row Asset) AssetView {
	return AssetView{
		ID:           string(row.ID),
		WorkspaceID:  string(row.WorkspaceID),
		DeploymentID: string(row.DeploymentID),
		Type:         string(row.Type),
		Key:          row.Key,
		ParentID:     string(row.ParentID),
		Title:        row.Title,
		Description:  row.Description,
		Meta:         SafeAssetMeta(string(row.Type), row.ContentJSON),
		Href:         AssetHref(string(row.Type), row.Key),
	}
}

func AssetEdgeViewFromAssetEdge(row AssetEdge) AssetEdgeView {
	return AssetEdgeView{
		ID:           string(row.ID),
		WorkspaceID:  string(row.WorkspaceID),
		DeploymentID: string(row.DeploymentID),
		FromAssetID:  string(row.FromAssetID),
		ToAssetID:    string(row.ToAssetID),
		Type:         string(row.Type),
	}
}

func FallbackAssetViews(workspaceID string, inputs []FallbackAssetInput) []AssetView {
	assets := make([]AssetView, 0, len(inputs))
	for _, input := range inputs {
		assets = append(assets, AssetView{
			ID:          input.ID,
			WorkspaceID: workspaceID,
			Type:        input.Type,
			Key:         input.Key,
			Title:       input.Title,
			Description: input.Description,
			Href:        AssetHref(input.Type, input.Key),
		})
	}
	return assets
}

func SafeAssetMeta(assetType, raw string) map[string]any {
	var content map[string]any
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return nil
	}
	authConfigured := hasConfiguredAuth(content["auth"]) || hasConfiguredAuth(content["Auth"])
	content = scrubAssetSecrets(content).(map[string]any)
	switch assetType {
	case string(AssetTypeConnection):
		content["credentials_configured"] = authConfigured
	case string(AssetTypeSource):
		return pickMeta(content, "format", "Format", "path", "Path", "connection", "Connection", "object", "Object", "options", "Options", "fields", "Fields", "schema", "Schema")
	case string(AssetTypeModelTable):
		return pickMeta(content,
			"source", "Source",
			"sources", "Sources",
			"source_dependencies", "SourceDependencies",
			"transform", "Transform",
			"sql", "SQL",
			"primary_key", "PrimaryKey",
			"grain", "Grain",
			"dimensions", "Dimensions",
			"fields", "Fields",
			"schema", "Schema",
		)
	case string(AssetTypeMeasure):
		return pickMeta(content, "expression", "Expression", "unit", "Unit", "format", "Format")
	case string(AssetTypeField):
		return pickMeta(content, "expr", "Expr", "where", "Where", "order_expr", "OrderExpr")
	case string(AssetTypeDashboard):
		return pickMeta(content, "semantic_model", "SemanticModel", "tags", "Tags")
	}
	return content
}

func scrubAssetSecrets(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			if strings.EqualFold(key, "auth") {
				if hasConfiguredAuth(nested) {
					out["credentials_configured"] = true
				}
				continue
			}
			out[key] = scrubAssetSecrets(nested)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, nested := range typed {
			out = append(out, scrubAssetSecrets(nested))
		}
		return out
	default:
		return value
	}
}

func hasConfiguredAuth(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		return len(typed) > 0
	case nil:
		return false
	default:
		return true
	}
}

func pickMeta(content map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		if value, ok := content[key]; ok {
			out[key] = value
		}
	}
	return out
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
