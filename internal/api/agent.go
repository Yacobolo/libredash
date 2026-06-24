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

type AgentChatSignal struct {
	Conversations        []AgentConversationResponse `json:"conversations"`
	ActiveConversationID string                      `json:"activeConversationId"`
	Transcript           []AgentChatTranscriptItem   `json:"transcript"`
	Status               AgentChatStatus             `json:"status"`
	Composer             AgentComposerSignal         `json:"composer"`
}

type AgentChatTranscriptItem struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Text           string `json:"text,omitempty"`
	Markdown       string `json:"markdown,omitempty"`
	ToolCallID     string `json:"toolCallId,omitempty"`
	Name           string `json:"name,omitempty"`
	Title          string `json:"title,omitempty"`
	Status         string `json:"status,omitempty"`
	Summary        string `json:"summary,omitempty"`
	ResultSummary  string `json:"resultSummary,omitempty"`
	InputJSON      string `json:"inputJson,omitempty"`
	ArgumentsJSON  string `json:"argumentsJson,omitempty"`
	ResultJSON     string `json:"resultJson,omitempty"`
	Error          string `json:"error,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	RunID          string `json:"runId,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
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
