package api

type SearchResult struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DashboardID string `json:"dashboardId,omitempty"`
	PageID      string `json:"pageId,omitempty"`
	VisualID    string `json:"visualId,omitempty"`
	TableID     string `json:"tableId,omitempty"`
	FilterID    string `json:"filterId,omitempty"`
	ModelID     string `json:"modelId,omitempty"`
	DatasetID   string `json:"datasetId,omitempty"`
	FieldID     string `json:"fieldId,omitempty"`
	AssetID     string `json:"assetId,omitempty"`
}

type SearchResponse struct {
	Items []SearchResult `json:"items"`
	Page  PageInfo       `json:"page"`
}
