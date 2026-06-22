package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/platform/db"
)

const (
	AgentConversationDefaultTitle   = "New conversation"
	AgentConversationStatusActive   = "active"
	AgentConversationStatusArchived = "archived"

	AgentRunStatusRunning   = "running"
	AgentRunStatusCompleted = "completed"
	AgentRunStatusFailed    = "failed"
	AgentRunStatusCanceled  = "canceled"

	AgentMessageRoleUser      = "user"
	AgentMessageRoleAssistant = "assistant"
	AgentMessageRoleTool      = "tool"
	AgentMessageRoleSummary   = "summary"
)

type AgentConversationInput struct {
	WorkspaceID  string
	PrincipalID  string
	Title        string
	MetadataJSON string
}

type AgentMessageInput struct {
	WorkspaceID    string
	PrincipalID    string
	ConversationID string
	RunID          string
	Role           string
	ContentText    string
	ContentJSON    string
	ToolCallID     string
	ToolName       string
	IsError        bool
}

type AgentRunInput struct {
	WorkspaceID    string
	PrincipalID    string
	ConversationID string
	RunID          string
	Model          string
	MetadataJSON   string
}

type AgentRunFinish struct {
	WorkspaceID    string
	PrincipalID    string
	ConversationID string
	RunID          string
	Status         string
	StopReason     string
	InputTokens    int64
	OutputTokens   int64
	TotalTokens    int64
	Error          string
	MetadataJSON   string
}

type AgentEventInput struct {
	WorkspaceID string
	PrincipalID string
	RunID       string
	Sequence    int64
	EventType   string
	Severity    string
	PayloadJSON string
}

func (s *Store) CreateAgentConversation(ctx context.Context, input AgentConversationInput) (db.AgentConversation, error) {
	metadata, err := normalizedJSONObject(input.MetadataJSON)
	if err != nil {
		return db.AgentConversation{}, err
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return db.AgentConversation{}, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = AgentConversationDefaultTitle
	}
	return s.q.CreateAgentConversation(ctx, db.CreateAgentConversationParams{
		ID:             newID("agentconv"),
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
		Title:          title,
		Status:         AgentConversationStatusActive,
		MetadataJson:   metadata,
		TranscriptJson: "[]",
	})
}

func (s *Store) ListAgentConversations(ctx context.Context, workspaceID, principalID string) ([]db.AgentConversation, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return nil, err
	}
	return s.q.ListAgentConversations(ctx, db.ListAgentConversationsParams{
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
}

func (s *Store) GetAgentConversation(ctx context.Context, workspaceID, principalID, conversationID string) (db.AgentConversation, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return db.AgentConversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return db.AgentConversation{}, fmt.Errorf("conversation id is required")
	}
	return s.q.GetAgentConversation(ctx, db.GetAgentConversationParams{
		ID:          conversationID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
}

func (s *Store) ArchiveAgentConversation(ctx context.Context, workspaceID, principalID, conversationID string) (db.AgentConversation, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return db.AgentConversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return db.AgentConversation{}, fmt.Errorf("conversation id is required")
	}
	return s.q.ArchiveAgentConversation(ctx, db.ArchiveAgentConversationParams{
		ID:          conversationID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
}

func (s *Store) UpdateDefaultAgentConversationTitle(ctx context.Context, workspaceID, principalID, conversationID, title string) (db.AgentConversation, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return db.AgentConversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return db.AgentConversation{}, fmt.Errorf("conversation id is required")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return db.AgentConversation{}, fmt.Errorf("conversation title is required")
	}
	return s.q.UpdateDefaultAgentConversationTitle(ctx, db.UpdateDefaultAgentConversationTitleParams{
		Title:       title,
		ID:          conversationID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
}

func (s *Store) UpdateAgentConversationTranscript(ctx context.Context, workspaceID, principalID, conversationID, transcriptJSON string) (db.AgentConversation, error) {
	transcript, err := normalizedJSONArray(transcriptJSON)
	if err != nil {
		return db.AgentConversation{}, err
	}
	workspaceID, principalID, err = agentScope(workspaceID, principalID)
	if err != nil {
		return db.AgentConversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return db.AgentConversation{}, fmt.Errorf("conversation id is required")
	}
	return s.q.UpdateAgentConversationTranscript(ctx, db.UpdateAgentConversationTranscriptParams{
		TranscriptJson: transcript,
		ID:             conversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
}

func (s *Store) AppendAgentMessage(ctx context.Context, input AgentMessageInput) (db.AgentMessage, error) {
	content, err := normalizedJSONObject(input.ContentJSON)
	if err != nil {
		return db.AgentMessage{}, err
	}
	if !validAgentMessageRole(input.Role) {
		return db.AgentMessage{}, fmt.Errorf("invalid agent message role %q", input.Role)
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return db.AgentMessage{}, err
	}
	return s.q.AppendAgentMessage(ctx, db.AppendAgentMessageParams{
		ID:             newID("agentmsg"),
		RunID:          input.RunID,
		Role:           input.Role,
		ContentText:    input.ContentText,
		ContentJson:    content,
		ToolCallID:     input.ToolCallID,
		ToolName:       input.ToolName,
		IsError:        input.IsError,
		ConversationID: input.ConversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
}

func (s *Store) ListAgentMessages(ctx context.Context, workspaceID, principalID, conversationID string) ([]db.AgentMessage, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return nil, err
	}
	if _, err := s.GetAgentConversation(ctx, workspaceID, principalID, conversationID); err != nil {
		return nil, err
	}
	return s.q.ListAgentMessages(ctx, db.ListAgentMessagesParams{
		ConversationID: conversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
}

func (s *Store) CreateAgentRun(ctx context.Context, input AgentRunInput) (db.AgentRun, error) {
	metadata, err := normalizedJSONObject(input.MetadataJSON)
	if err != nil {
		return db.AgentRun{}, err
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return db.AgentRun{}, err
	}
	if _, err := s.GetAgentConversation(ctx, workspaceID, principalID, input.ConversationID); err != nil {
		return db.AgentRun{}, err
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		runID = newID("agentrun")
	}
	return s.q.CreateAgentRun(ctx, db.CreateAgentRunParams{
		ID:             runID,
		Status:         AgentRunStatusRunning,
		Model:          input.Model,
		MetadataJson:   metadata,
		ConversationID: input.ConversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
}

func (s *Store) FinishAgentRun(ctx context.Context, input AgentRunFinish) (db.AgentRun, error) {
	metadata, err := normalizedJSONObject(input.MetadataJSON)
	if err != nil {
		return db.AgentRun{}, err
	}
	if !validAgentRunStatus(input.Status) || input.Status == AgentRunStatusRunning {
		return db.AgentRun{}, fmt.Errorf("invalid final agent run status %q", input.Status)
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return db.AgentRun{}, err
	}
	return s.q.FinishAgentRun(ctx, db.FinishAgentRunParams{
		Status:         input.Status,
		StopReason:     input.StopReason,
		InputTokens:    input.InputTokens,
		OutputTokens:   input.OutputTokens,
		TotalTokens:    input.TotalTokens,
		Error:          input.Error,
		MetadataJson:   metadata,
		ID:             input.RunID,
		ConversationID: input.ConversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
}

func (s *Store) ListAgentRuns(ctx context.Context, workspaceID, principalID, conversationID string) ([]db.AgentRun, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return nil, err
	}
	if _, err := s.GetAgentConversation(ctx, workspaceID, principalID, conversationID); err != nil {
		return nil, err
	}
	return s.q.ListAgentRuns(ctx, db.ListAgentRunsParams{
		ConversationID: conversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
}

func (s *Store) AppendAgentEvent(ctx context.Context, input AgentEventInput) (db.AgentEvent, error) {
	payload, err := normalizedJSONObject(input.PayloadJSON)
	if err != nil {
		return db.AgentEvent{}, err
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return db.AgentEvent{}, err
	}
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		return db.AgentEvent{}, fmt.Errorf("event type is required")
	}
	severity := strings.TrimSpace(input.Severity)
	if severity == "" {
		severity = "info"
	}
	if input.Sequence <= 0 {
		return db.AgentEvent{}, fmt.Errorf("event sequence is required")
	}
	return s.q.AppendAgentEvent(ctx, db.AppendAgentEventParams{
		ID:          newID("agentevt"),
		Seq:         input.Sequence,
		EventType:   eventType,
		Severity:    severity,
		PayloadJson: payload,
		RunID:       input.RunID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
}

func (s *Store) ListAgentEvents(ctx context.Context, workspaceID, principalID, runID string) ([]db.AgentEvent, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return nil, err
	}
	events, err := s.q.ListAgentEvents(ctx, db.ListAgentEventsParams{
		RunID:       runID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		if exists, err := s.agentRunExists(ctx, workspaceID, principalID, runID); err != nil {
			return nil, err
		} else if !exists {
			return nil, sql.ErrNoRows
		}
	}
	return events, nil
}

func (s *Store) agentRunExists(ctx context.Context, workspaceID, principalID, runID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM agent_runs r
			JOIN agent_conversations c ON c.id = r.conversation_id
			WHERE r.id = ? AND c.workspace_id = ? AND c.principal_id = ?
		)
	`, runID, workspaceID, principalID).Scan(&exists)
	return exists, err
}

func agentScope(workspaceID, principalID string) (string, string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	principalID = strings.TrimSpace(principalID)
	if workspaceID == "" {
		return "", "", fmt.Errorf("workspace id is required")
	}
	if principalID == "" {
		return "", "", fmt.Errorf("principal id is required")
	}
	return workspaceID, principalID, nil
}

func normalizedJSONObject(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}", nil
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("invalid JSON object")
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", err
	}
	if _, ok := value.(map[string]any); !ok {
		return "", fmt.Errorf("JSON value must be an object")
	}
	return raw, nil
}

func normalizedJSONArray(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "[]", nil
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("invalid JSON array")
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", err
	}
	if _, ok := value.([]any); !ok {
		return "", fmt.Errorf("JSON value must be an array")
	}
	return raw, nil
}

func validAgentMessageRole(role string) bool {
	switch role {
	case AgentMessageRoleUser, AgentMessageRoleAssistant, AgentMessageRoleTool, AgentMessageRoleSummary:
		return true
	default:
		return false
	}
}

func validAgentRunStatus(status string) bool {
	switch status {
	case AgentRunStatusRunning, AgentRunStatusCompleted, AgentRunStatusFailed, AgentRunStatusCanceled:
		return true
	default:
		return false
	}
}
