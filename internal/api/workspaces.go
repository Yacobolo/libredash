package api

type WorkspaceResponse struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	ActiveDeploymentID string `json:"activeDeploymentId,omitempty"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

type AssetResponse struct {
	ID            string         `json:"id"`
	SnapshotID    string         `json:"snapshotId"`
	WorkspaceID   string         `json:"workspaceId"`
	DeploymentID  string         `json:"deploymentId"`
	Type          string         `json:"type"`
	Key           string         `json:"key"`
	ParentID      string         `json:"parentId,omitempty"`
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	SourceFile    string         `json:"sourceFile,omitempty"`
	PayloadSchema string         `json:"payloadSchema"`
	Payload       map[string]any `json:"payload"`
	Href          string         `json:"href,omitempty"`
}

type AssetSummaryResponse struct {
	ID            string `json:"id"`
	SnapshotID    string `json:"snapshotId"`
	WorkspaceID   string `json:"workspaceId"`
	DeploymentID  string `json:"deploymentId"`
	Type          string `json:"type"`
	Key           string `json:"key"`
	ParentID      string `json:"parentId,omitempty"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	SourceFile    string `json:"sourceFile,omitempty"`
	PayloadSchema string `json:"payloadSchema"`
	ContentHash   string `json:"contentHash"`
	Href          string `json:"href,omitempty"`
}

type AssetEdgeResponse struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspaceId"`
	DeploymentID string `json:"deploymentId"`
	FromAssetID  string `json:"fromAssetId"`
	ToAssetID    string `json:"toAssetId"`
	Type         string `json:"type"`
}

type AssetLineageResponse struct {
	AssetID    string   `json:"assetId"`
	Upstream   []string `json:"upstream"`
	Downstream []string `json:"downstream"`
}

type RoleResponse struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

type RoleBindingResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	PrincipalID string `json:"principalId"`
	GroupID     string `json:"groupId,omitempty"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	GroupName   string `json:"groupName,omitempty"`
	Role        string `json:"role"`
	CreatedAt   string `json:"createdAt"`
}
