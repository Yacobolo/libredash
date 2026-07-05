package api

type PublishCreateRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Environment string `json:"environment"`
}

type PublishResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Environment string `json:"environment"`
	Status      string `json:"status"`
	Digest      string `json:"digest"`
	CreatedAt   string `json:"createdAt"`
	ActivatedAt string `json:"activatedAt,omitempty"`
	Error       string `json:"error,omitempty"`
}
