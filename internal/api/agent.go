package api

type AgentConversationCreateRequest struct {
	Title string `json:"title"`
}

type AgentConversationUpdateRequest struct {
	Title string `json:"title"`
}

type AgentConversationResponse struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspaceId"`
	PrincipalID     string `json:"principalId"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	ArchivedAt      string `json:"archivedAt,omitempty"`
	MessageCount    int    `json:"messageCount,omitempty"`
	LastMessageText string `json:"lastMessageText,omitempty"`
	TitlePending    bool   `json:"titlePending,omitempty"`
}

type AgentRunResponse struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	Status         string `json:"status"`
	Model          string `json:"model,omitempty"`
	StopReason     string `json:"stopReason,omitempty"`
	InputTokens    int64  `json:"inputTokens,omitempty"`
	OutputTokens   int64  `json:"outputTokens,omitempty"`
	TotalTokens    int64  `json:"totalTokens,omitempty"`
	Error          string `json:"error,omitempty"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt,omitempty"`
	CreatedAt      string `json:"createdAt"`
}

type AgentMessageResponse struct {
	ID          string         `json:"id"`
	RunID       string         `json:"runId,omitempty"`
	Seq         int64          `json:"seq"`
	Role        string         `json:"role"`
	ContentText string         `json:"contentText,omitempty"`
	Content     map[string]any `json:"content,omitempty"`
	ToolCallID  string         `json:"toolCallId,omitempty"`
	ToolName    string         `json:"toolName,omitempty"`
	IsError     bool           `json:"isError,omitempty"`
	CreatedAt   string         `json:"createdAt"`
}

type AgentTurnRequest struct {
	Input         string `json:"input"`
	CorrelationID string `json:"correlationId,omitempty"`
}

type AgentTurnResponse struct {
	ConversationID string `json:"conversationId"`
	RunID          string `json:"runId"`
	StopReason     string `json:"stopReason"`
	Content        string `json:"content"`
}

type AgentEventResponse struct {
	ID        string         `json:"id"`
	RunID     string         `json:"runId"`
	Seq       int64          `json:"seq"`
	EventType string         `json:"eventType"`
	Severity  string         `json:"severity"`
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"createdAt"`
}

type AdminAgentToolResponse struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type AdminAgentResponse struct {
	Enabled      bool                     `json:"enabled"`
	Model        string                   `json:"model,omitempty"`
	SystemPrompt string                   `json:"systemPrompt"`
	Tools        []AdminAgentToolResponse `json:"tools"`
}

type AdminAgentConfigPatchRequest struct {
	SystemPrompt string `json:"systemPrompt"`
}
