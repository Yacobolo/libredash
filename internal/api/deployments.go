package api

type DeploymentCreateRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type DeploymentResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Status      string `json:"status"`
	Digest      string `json:"digest"`
	CreatedAt   string `json:"createdAt"`
	ActivatedAt string `json:"activatedAt,omitempty"`
	Error       string `json:"error,omitempty"`
}
