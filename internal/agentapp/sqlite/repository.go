package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/agentapp"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
)

type Repository struct {
	db *sql.DB
	q  *platformdb.Queries
}

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{db: sqlDB, q: platformdb.New(sqlDB)}
}

func (r *Repository) CreateConversation(ctx context.Context, input agentapp.ConversationInput) (agentapp.Conversation, error) {
	metadata, err := normalizedJSONObject(input.MetadataJSON)
	if err != nil {
		return agentapp.Conversation{}, err
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return agentapp.Conversation{}, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = agentapp.ConversationDefaultTitle
	}
	row, err := r.q.CreateAgentConversation(ctx, platformdb.CreateAgentConversationParams{
		ID:             newID("agentconv"),
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
		Title:          title,
		Status:         agentapp.ConversationStatusActive,
		MetadataJson:   metadata,
		TranscriptJson: "[]",
	})
	if err != nil {
		return agentapp.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) ListConversations(ctx context.Context, workspaceID, principalID string) ([]agentapp.Conversation, error) {
	return r.ListConversationsPage(ctx, workspaceID, principalID, agentapp.Page{})
}

func (r *Repository) ListConversationsPage(ctx context.Context, workspaceID, principalID string, page agentapp.Page) ([]agentapp.Conversation, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListAgentConversations(ctx, platformdb.ListAgentConversationsParams{
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]agentapp.Conversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapConversation(row))
	}
	return pageByID(out, page, func(row agentapp.Conversation) string { return row.ID }), nil
}

func (r *Repository) GetConversation(ctx context.Context, workspaceID, principalID, conversationID string) (agentapp.Conversation, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return agentapp.Conversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agentapp.Conversation{}, fmt.Errorf("conversation id is required")
	}
	row, err := r.q.GetAgentConversation(ctx, platformdb.GetAgentConversationParams{
		ID:          conversationID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
	if err != nil {
		return agentapp.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) UpdateConversation(ctx context.Context, input agentapp.ConversationUpdate) (agentapp.Conversation, error) {
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return agentapp.Conversation{}, err
	}
	if strings.TrimSpace(input.ConversationID) == "" {
		return agentapp.Conversation{}, fmt.Errorf("conversation id is required")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return agentapp.Conversation{}, fmt.Errorf("conversation title is required")
	}
	row := platformdb.AgentConversation{ID: input.ConversationID}
	err = r.db.QueryRowContext(ctx, `
		UPDATE agent_conversations
		SET title = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ? AND principal_id = ? AND status = 'active'
		RETURNING id, workspace_id, principal_id, title, status, metadata_json, transcript_json, created_at, updated_at, archived_at
	`, title, input.ConversationID, workspaceID, principalID).Scan(
		&row.ID,
		&row.WorkspaceID,
		&row.PrincipalID,
		&row.Title,
		&row.Status,
		&row.MetadataJson,
		&row.TranscriptJson,
		&row.CreatedAt,
		&row.UpdatedAt,
		&row.ArchivedAt,
	)
	if err != nil {
		return agentapp.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) ArchiveConversation(ctx context.Context, workspaceID, principalID, conversationID string) (agentapp.Conversation, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return agentapp.Conversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agentapp.Conversation{}, fmt.Errorf("conversation id is required")
	}
	row, err := r.q.ArchiveAgentConversation(ctx, platformdb.ArchiveAgentConversationParams{
		ID:          conversationID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
	if err != nil {
		return agentapp.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) UpdateDefaultConversationTitle(ctx context.Context, workspaceID, principalID, conversationID, title string) (agentapp.Conversation, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return agentapp.Conversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agentapp.Conversation{}, fmt.Errorf("conversation id is required")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return agentapp.Conversation{}, fmt.Errorf("conversation title is required")
	}
	row, err := r.q.UpdateDefaultAgentConversationTitle(ctx, platformdb.UpdateDefaultAgentConversationTitleParams{
		Title:       title,
		ID:          conversationID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
	if err != nil {
		return agentapp.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) UpdateConversationTranscript(ctx context.Context, workspaceID, principalID, conversationID, transcriptJSON string) (agentapp.Conversation, error) {
	transcript, err := normalizedJSONArray(transcriptJSON)
	if err != nil {
		return agentapp.Conversation{}, err
	}
	workspaceID, principalID, err = agentScope(workspaceID, principalID)
	if err != nil {
		return agentapp.Conversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agentapp.Conversation{}, fmt.Errorf("conversation id is required")
	}
	row, err := r.q.UpdateAgentConversationTranscript(ctx, platformdb.UpdateAgentConversationTranscriptParams{
		TranscriptJson: transcript,
		ID:             conversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
	if err != nil {
		return agentapp.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) AppendMessage(ctx context.Context, input agentapp.MessageInput) (agentapp.Message, error) {
	content, err := normalizedJSONObject(input.ContentJSON)
	if err != nil {
		return agentapp.Message{}, err
	}
	if !validMessageRole(input.Role) {
		return agentapp.Message{}, fmt.Errorf("invalid agent message role %q", input.Role)
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return agentapp.Message{}, err
	}
	row, err := r.q.AppendAgentMessage(ctx, platformdb.AppendAgentMessageParams{
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
	if err != nil {
		return agentapp.Message{}, err
	}
	return mapMessage(row), nil
}

func (r *Repository) ListMessages(ctx context.Context, workspaceID, principalID, conversationID string) ([]agentapp.Message, error) {
	return r.ListMessagesPage(ctx, workspaceID, principalID, conversationID, agentapp.Page{})
}

func (r *Repository) ListMessagesPage(ctx context.Context, workspaceID, principalID, conversationID string, page agentapp.Page) ([]agentapp.Message, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return nil, err
	}
	if _, err := r.GetConversation(ctx, workspaceID, principalID, conversationID); err != nil {
		return nil, err
	}
	rows, err := r.q.ListAgentMessages(ctx, platformdb.ListAgentMessagesParams{
		ConversationID: conversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]agentapp.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapMessage(row))
	}
	return pageByID(out, page, func(row agentapp.Message) string { return row.ID }), nil
}

func (r *Repository) CreateRun(ctx context.Context, input agentapp.RunInput) (agentapp.Run, error) {
	metadata, err := normalizedJSONObject(input.MetadataJSON)
	if err != nil {
		return agentapp.Run{}, err
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return agentapp.Run{}, err
	}
	if _, err := r.GetConversation(ctx, workspaceID, principalID, input.ConversationID); err != nil {
		return agentapp.Run{}, err
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		runID = newID("agentrun")
	}
	row, err := r.q.CreateAgentRun(ctx, platformdb.CreateAgentRunParams{
		ID:             runID,
		Status:         agentapp.RunStatusRunning,
		Model:          input.Model,
		MetadataJson:   metadata,
		ConversationID: input.ConversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
	if err != nil {
		return agentapp.Run{}, err
	}
	return mapRun(row), nil
}

func (r *Repository) FinishRun(ctx context.Context, input agentapp.RunFinish) (agentapp.Run, error) {
	metadata, err := normalizedJSONObject(input.MetadataJSON)
	if err != nil {
		return agentapp.Run{}, err
	}
	if !validRunStatus(input.Status) || input.Status == agentapp.RunStatusRunning {
		return agentapp.Run{}, fmt.Errorf("invalid final agent run status %q", input.Status)
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return agentapp.Run{}, err
	}
	row, err := r.q.FinishAgentRun(ctx, platformdb.FinishAgentRunParams{
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
	if err != nil {
		return agentapp.Run{}, err
	}
	return mapRun(row), nil
}

func (r *Repository) ListRuns(ctx context.Context, workspaceID, principalID, conversationID string) ([]agentapp.Run, error) {
	return r.ListRunsPage(ctx, workspaceID, principalID, conversationID, agentapp.Page{})
}

func (r *Repository) ListRunsPage(ctx context.Context, workspaceID, principalID, conversationID string, page agentapp.Page) ([]agentapp.Run, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return nil, err
	}
	if _, err := r.GetConversation(ctx, workspaceID, principalID, conversationID); err != nil {
		return nil, err
	}
	rows, err := r.q.ListAgentRuns(ctx, platformdb.ListAgentRunsParams{
		ConversationID: conversationID,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]agentapp.Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapRun(row))
	}
	return pageByID(out, page, func(row agentapp.Run) string { return row.ID }), nil
}

func (r *Repository) GetRun(ctx context.Context, workspaceID, principalID, conversationID, runID string) (agentapp.Run, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return agentapp.Run{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agentapp.Run{}, fmt.Errorf("conversation id is required")
	}
	if strings.TrimSpace(runID) == "" {
		return agentapp.Run{}, fmt.Errorf("run id is required")
	}
	var row platformdb.AgentRun
	err = r.db.QueryRowContext(ctx, `
		SELECT r.id, r.conversation_id, r.status, r.model, r.stop_reason, r.input_tokens, r.output_tokens, r.total_tokens, r.error, r.started_at, r.finished_at, r.metadata_json
		FROM agent_runs r
		JOIN agent_conversations c ON c.id = r.conversation_id
		WHERE r.id = ? AND c.id = ? AND c.workspace_id = ? AND c.principal_id = ?
	`, runID, conversationID, workspaceID, principalID).Scan(
		&row.ID,
		&row.ConversationID,
		&row.Status,
		&row.Model,
		&row.StopReason,
		&row.InputTokens,
		&row.OutputTokens,
		&row.TotalTokens,
		&row.Error,
		&row.StartedAt,
		&row.FinishedAt,
		&row.MetadataJson,
	)
	if err != nil {
		return agentapp.Run{}, err
	}
	return mapRun(row), nil
}

func (r *Repository) GetRunByID(ctx context.Context, workspaceID, principalID, runID string) (agentapp.Run, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return agentapp.Run{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return agentapp.Run{}, fmt.Errorf("run id is required")
	}
	var row platformdb.AgentRun
	err = r.db.QueryRowContext(ctx, `
		SELECT r.id, r.conversation_id, r.status, r.model, r.stop_reason, r.input_tokens, r.output_tokens, r.total_tokens, r.error, r.started_at, r.finished_at, r.metadata_json
		FROM agent_runs r
		JOIN agent_conversations c ON c.id = r.conversation_id
		WHERE r.id = ? AND c.workspace_id = ? AND c.principal_id = ?
	`, runID, workspaceID, principalID).Scan(
		&row.ID,
		&row.ConversationID,
		&row.Status,
		&row.Model,
		&row.StopReason,
		&row.InputTokens,
		&row.OutputTokens,
		&row.TotalTokens,
		&row.Error,
		&row.StartedAt,
		&row.FinishedAt,
		&row.MetadataJson,
	)
	if err != nil {
		return agentapp.Run{}, err
	}
	return mapRun(row), nil
}

func (r *Repository) AppendEvent(ctx context.Context, input agentapp.EventInput) (agentapp.Event, error) {
	payload, err := normalizedJSONObject(input.PayloadJSON)
	if err != nil {
		return agentapp.Event{}, err
	}
	workspaceID, principalID, err := agentScope(input.WorkspaceID, input.PrincipalID)
	if err != nil {
		return agentapp.Event{}, err
	}
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		return agentapp.Event{}, fmt.Errorf("event type is required")
	}
	severity := strings.TrimSpace(input.Severity)
	if severity == "" {
		severity = "info"
	}
	if input.Sequence <= 0 {
		return agentapp.Event{}, fmt.Errorf("event sequence is required")
	}
	row, err := r.q.AppendAgentEvent(ctx, platformdb.AppendAgentEventParams{
		ID:          newID("agentevt"),
		Seq:         input.Sequence,
		EventType:   eventType,
		Severity:    severity,
		PayloadJson: payload,
		RunID:       input.RunID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
	if err != nil {
		return agentapp.Event{}, err
	}
	return mapEvent(row), nil
}

func (r *Repository) ListEvents(ctx context.Context, workspaceID, principalID, runID string) ([]agentapp.Event, error) {
	return r.ListEventsPage(ctx, workspaceID, principalID, runID, agentapp.Page{})
}

func (r *Repository) ListEventsPage(ctx context.Context, workspaceID, principalID, runID string, page agentapp.Page) ([]agentapp.Event, error) {
	workspaceID, principalID, err := agentScope(workspaceID, principalID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListAgentEvents(ctx, platformdb.ListAgentEventsParams{
		RunID:       runID,
		WorkspaceID: workspaceID,
		PrincipalID: principalID,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		exists, err := r.agentRunExists(ctx, workspaceID, principalID, runID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, sql.ErrNoRows
		}
	}
	out := make([]agentapp.Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapEvent(row))
	}
	return pageByID(out, page, func(row agentapp.Event) string { return row.ID }), nil
}

func (r *Repository) agentRunExists(ctx context.Context, workspaceID, principalID, runID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM agent_runs r
			JOIN agent_conversations c ON c.id = r.conversation_id
			WHERE r.id = ? AND c.workspace_id = ? AND c.principal_id = ?
		)
	`, runID, workspaceID, principalID).Scan(&exists)
	return exists, err
}

func mapConversation(row platformdb.AgentConversation) agentapp.Conversation {
	out := agentapp.Conversation{
		ID:             row.ID,
		WorkspaceID:    row.WorkspaceID,
		PrincipalID:    row.PrincipalID,
		Title:          row.Title,
		Status:         row.Status,
		MetadataJSON:   row.MetadataJson,
		TranscriptJSON: row.TranscriptJson,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.ArchivedAt.Valid {
		out.ArchivedAt = row.ArchivedAt.String
	}
	return out
}

func mapMessage(row platformdb.AgentMessage) agentapp.Message {
	out := agentapp.Message{
		ID:             row.ID,
		ConversationID: row.ConversationID,
		Seq:            row.Seq,
		Role:           row.Role,
		ContentText:    row.ContentText,
		ContentJSON:    row.ContentJson,
		ToolCallID:     row.ToolCallID,
		ToolName:       row.ToolName,
		IsError:        row.IsError,
		CreatedAt:      row.CreatedAt,
	}
	if row.RunID.Valid {
		out.RunID = row.RunID.String
	}
	return out
}

func mapRun(row platformdb.AgentRun) agentapp.Run {
	out := agentapp.Run{
		ID:             row.ID,
		ConversationID: row.ConversationID,
		Status:         row.Status,
		Model:          row.Model,
		StopReason:     row.StopReason,
		InputTokens:    row.InputTokens,
		OutputTokens:   row.OutputTokens,
		TotalTokens:    row.TotalTokens,
		Error:          row.Error,
		StartedAt:      row.StartedAt,
		MetadataJSON:   row.MetadataJson,
		CreatedAt:      row.StartedAt,
	}
	if row.FinishedAt.Valid {
		out.FinishedAt = row.FinishedAt.String
	}
	return out
}

func mapEvent(row platformdb.AgentEvent) agentapp.Event {
	return agentapp.Event{
		ID:          row.ID,
		RunID:       row.RunID,
		Seq:         row.Seq,
		EventType:   row.EventType,
		Severity:    row.Severity,
		PayloadJSON: row.PayloadJson,
		CreatedAt:   row.CreatedAt,
	}
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

func validMessageRole(role string) bool {
	switch role {
	case agentapp.MessageRoleUser, agentapp.MessageRoleAssistant, agentapp.MessageRoleTool, agentapp.MessageRoleSummary:
		return true
	default:
		return false
	}
}

func validRunStatus(status string) bool {
	switch status {
	case agentapp.RunStatusRunning, agentapp.RunStatusCompleted, agentapp.RunStatusFailed, agentapp.RunStatusCanceled:
		return true
	default:
		return false
	}
}

func pageByID[T any](rows []T, page agentapp.Page, id func(T) string) []T {
	limit := page.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	start := 0
	after := strings.TrimSpace(page.After)
	if after != "" {
		start = len(rows)
		for i, row := range rows {
			if id(row) == after {
				start = i + 1
				break
			}
		}
	}
	if start >= len(rows) {
		return []T{}
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	return append([]T(nil), rows[start:end]...)
}

func newID(prefix string) string {
	return prefix + "_" + newSecret()[:24]
}

func newSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		sum := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano)))
		return hex.EncodeToString(sum[:])
	}
	return hex.EncodeToString(b[:])
}
