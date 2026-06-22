package api

type AgentConversationCreateRequest struct {
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
}

type AgentMessageResponse struct {
	ID          string `json:"id"`
	RunID       string `json:"runId,omitempty"`
	Seq         int64  `json:"seq"`
	Role        string `json:"role"`
	ContentText string `json:"contentText,omitempty"`
	ContentJSON string `json:"contentJson,omitempty"`
	ToolCallID  string `json:"toolCallId,omitempty"`
	ToolName    string `json:"toolName,omitempty"`
	IsError     bool   `json:"isError,omitempty"`
	CreatedAt   string `json:"createdAt"`
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
	ID          string `json:"id"`
	RunID       string `json:"runId"`
	Seq         int64  `json:"seq"`
	EventType   string `json:"eventType"`
	Severity    string `json:"severity"`
	PayloadJSON string `json:"payloadJson"`
	CreatedAt   string `json:"createdAt"`
}

type AgentChatSignal struct {
	Conversations        []AgentConversationResponse `json:"conversations"`
	ActiveConversationID string                      `json:"activeConversationId"`
	Events               []AgentEventEnvelope        `json:"events"`
	Status               AgentChatStatus             `json:"status"`
	Composer             AgentComposerSignal         `json:"composer"`
}

type AgentChatStatus struct {
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

type AgentComposerSignal struct {
	Value       string `json:"value"`
	Disabled    bool   `json:"disabled"`
	Placeholder string `json:"placeholder"`
}

type AgentEventEnvelope struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversationId,omitempty"`
	RunID          string         `json:"runId,omitempty"`
	Seq            int64          `json:"seq"`
	Type           string         `json:"type"`
	Severity       string         `json:"severity,omitempty"`
	CreatedAt      string         `json:"createdAt,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
}
