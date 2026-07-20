package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/internal/asyncjob"
	asyncjobsqlite "github.com/Yacobolo/leapview/internal/asyncjob/sqlite"
	platformdb "github.com/Yacobolo/leapview/internal/platform/db"
)

type Repository struct {
	db     *sql.DB
	q      *platformdb.Queries
	events asyncjob.Repository
}

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{db: sqlDB, q: platformdb.New(sqlDB), events: asyncjobsqlite.NewRepository(sqlDB)}
}

func (r *Repository) CreateConversation(ctx context.Context, input agent.ConversationInput) (agent.Conversation, error) {
	metadata, err := normalizedJSONObject(input.MetadataJSON)
	if err != nil {
		return agent.Conversation{}, err
	}
	principalID, err := agentPrincipalID(input.PrincipalID)
	if err != nil {
		return agent.Conversation{}, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = agent.ConversationDefaultTitle
	}
	row, err := r.q.CreateAgentConversation(ctx, platformdb.CreateAgentConversationParams{
		ID:             newID("agentconv"),
		PrincipalID:    principalID,
		Title:          title,
		Status:         agent.ConversationStatusActive,
		MetadataJson:   metadata,
		TranscriptJson: "[]",
	})
	if err != nil {
		return agent.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) ListConversations(ctx context.Context, principalID string) ([]agent.Conversation, error) {
	return r.ListConversationsPage(ctx, principalID, agent.Page{})
}

func (r *Repository) ListConversationsPage(ctx context.Context, principalID string, page agent.Page) ([]agent.Conversation, error) {
	principalID, err := agentPrincipalID(principalID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListAgentConversations(ctx, principalID)
	if err != nil {
		return nil, err
	}
	out := make([]agent.Conversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapConversation(row))
	}
	return pageByID(out, page, func(row agent.Conversation) string { return row.ID }), nil
}

func (r *Repository) GetConversation(ctx context.Context, principalID, conversationID string) (agent.Conversation, error) {
	principalID, err := agentPrincipalID(principalID)
	if err != nil {
		return agent.Conversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agent.Conversation{}, fmt.Errorf("conversation id is required")
	}
	row, err := r.q.GetAgentConversation(ctx, platformdb.GetAgentConversationParams{
		ID:          conversationID,
		PrincipalID: principalID,
	})
	if err != nil {
		return agent.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) UpdateConversation(ctx context.Context, input agent.ConversationUpdate) (agent.Conversation, error) {
	principalID, err := agentPrincipalID(input.PrincipalID)
	if err != nil {
		return agent.Conversation{}, err
	}
	if strings.TrimSpace(input.ConversationID) == "" {
		return agent.Conversation{}, fmt.Errorf("conversation id is required")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return agent.Conversation{}, fmt.Errorf("conversation title is required")
	}
	row, err := r.q.UpdateAgentConversationTitle(ctx, platformdb.UpdateAgentConversationTitleParams{
		Title: title, ConversationID: input.ConversationID, PrincipalID: principalID,
	})
	if err != nil {
		return agent.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) ArchiveConversation(ctx context.Context, principalID, conversationID string) (agent.Conversation, error) {
	principalID, err := agentPrincipalID(principalID)
	if err != nil {
		return agent.Conversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agent.Conversation{}, fmt.Errorf("conversation id is required")
	}
	row, err := r.q.ArchiveAgentConversation(ctx, platformdb.ArchiveAgentConversationParams{
		ID:          conversationID,
		PrincipalID: principalID,
	})
	if err != nil {
		return agent.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) UpdateDefaultConversationTitle(ctx context.Context, principalID, conversationID, title string) (agent.Conversation, error) {
	principalID, err := agentPrincipalID(principalID)
	if err != nil {
		return agent.Conversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agent.Conversation{}, fmt.Errorf("conversation id is required")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return agent.Conversation{}, fmt.Errorf("conversation title is required")
	}
	row, err := r.q.UpdateDefaultAgentConversationTitle(ctx, platformdb.UpdateDefaultAgentConversationTitleParams{
		Title:       title,
		ID:          conversationID,
		PrincipalID: principalID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.Conversation{}, agent.ErrNotFound
		}
		return agent.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) UpdateConversationTranscript(ctx context.Context, principalID, conversationID, transcriptJSON string) (agent.Conversation, error) {
	transcript, err := normalizedJSONArray(transcriptJSON)
	if err != nil {
		return agent.Conversation{}, err
	}
	principalID, err = agentPrincipalID(principalID)
	if err != nil {
		return agent.Conversation{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agent.Conversation{}, fmt.Errorf("conversation id is required")
	}
	row, err := r.q.UpdateAgentConversationTranscript(ctx, platformdb.UpdateAgentConversationTranscriptParams{
		TranscriptJson: transcript,
		ID:             conversationID,
		PrincipalID:    principalID,
	})
	if err != nil {
		return agent.Conversation{}, err
	}
	return mapConversation(row), nil
}

func (r *Repository) AppendMessage(ctx context.Context, input agent.MessageInput) (agent.Message, error) {
	content, err := normalizedJSONObject(input.ContentJSON)
	if err != nil {
		return agent.Message{}, err
	}
	if !validMessageRole(input.Role) {
		return agent.Message{}, fmt.Errorf("invalid agent message role %q", input.Role)
	}
	principalID, err := agentPrincipalID(input.PrincipalID)
	if err != nil {
		return agent.Message{}, err
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
		PrincipalID:    principalID,
	})
	if err != nil {
		return agent.Message{}, err
	}
	return mapMessage(row), nil
}

func (r *Repository) ListMessages(ctx context.Context, principalID, conversationID string) ([]agent.Message, error) {
	return r.ListMessagesPage(ctx, principalID, conversationID, agent.Page{})
}

func (r *Repository) ListMessagesPage(ctx context.Context, principalID, conversationID string, page agent.Page) ([]agent.Message, error) {
	principalID, err := agentPrincipalID(principalID)
	if err != nil {
		return nil, err
	}
	if _, err := r.GetConversation(ctx, principalID, conversationID); err != nil {
		return nil, err
	}
	rows, err := r.q.ListAgentMessages(ctx, platformdb.ListAgentMessagesParams{
		ConversationID: conversationID,
		PrincipalID:    principalID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]agent.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapMessage(row))
	}
	return pageByID(out, page, func(row agent.Message) string { return row.ID }), nil
}

func (r *Repository) CreateRun(ctx context.Context, input agent.RunInput) (agent.Run, error) {
	metadata, err := normalizedJSONObject(input.MetadataJSON)
	if err != nil {
		return agent.Run{}, err
	}
	principalID, err := agentPrincipalID(input.PrincipalID)
	if err != nil {
		return agent.Run{}, err
	}
	if _, err := r.GetConversation(ctx, principalID, input.ConversationID); err != nil {
		return agent.Run{}, err
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		runID = newID("agentrun")
	}
	row, err := r.q.CreateAgentRun(ctx, platformdb.CreateAgentRunParams{
		ID:             runID,
		Status:         agent.RunStatusRunning,
		Model:          input.Model,
		MetadataJson:   metadata,
		ConversationID: input.ConversationID,
		PrincipalID:    principalID,
	})
	if err != nil {
		return agent.Run{}, err
	}
	return mapRun(row), nil
}

func (r *Repository) FinishRun(ctx context.Context, input agent.RunFinish) (agent.Run, error) {
	metadata, err := normalizedJSONObject(input.MetadataJSON)
	if err != nil {
		return agent.Run{}, err
	}
	if !validRunStatus(input.Status) || input.Status == agent.RunStatusRunning {
		return agent.Run{}, fmt.Errorf("invalid final agent run status %q", input.Status)
	}
	principalID, err := agentPrincipalID(input.PrincipalID)
	if err != nil {
		return agent.Run{}, err
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
		PrincipalID:    principalID,
	})
	if err != nil {
		return agent.Run{}, err
	}
	return mapRun(row), nil
}

func (r *Repository) ListRuns(ctx context.Context, principalID, conversationID string) ([]agent.Run, error) {
	return r.ListRunsPage(ctx, principalID, conversationID, agent.Page{})
}

func (r *Repository) ListRunsPage(ctx context.Context, principalID, conversationID string, page agent.Page) ([]agent.Run, error) {
	principalID, err := agentPrincipalID(principalID)
	if err != nil {
		return nil, err
	}
	if _, err := r.GetConversation(ctx, principalID, conversationID); err != nil {
		return nil, err
	}
	rows, err := r.q.ListAgentRuns(ctx, platformdb.ListAgentRunsParams{
		ConversationID: conversationID,
		PrincipalID:    principalID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]agent.Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapRun(row))
	}
	return pageByID(out, page, func(row agent.Run) string { return row.ID }), nil
}

func (r *Repository) GetRun(ctx context.Context, principalID, conversationID, runID string) (agent.Run, error) {
	principalID, err := agentPrincipalID(principalID)
	if err != nil {
		return agent.Run{}, err
	}
	if strings.TrimSpace(conversationID) == "" {
		return agent.Run{}, fmt.Errorf("conversation id is required")
	}
	if strings.TrimSpace(runID) == "" {
		return agent.Run{}, fmt.Errorf("run id is required")
	}
	row, err := r.q.GetAgentRunInConversation(ctx, platformdb.GetAgentRunInConversationParams{
		RunID: runID, ConversationID: conversationID, PrincipalID: principalID,
	})
	if err != nil {
		return agent.Run{}, err
	}
	return mapRun(row), nil
}

func (r *Repository) GetRunByID(ctx context.Context, principalID, runID string) (agent.Run, error) {
	principalID, err := agentPrincipalID(principalID)
	if err != nil {
		return agent.Run{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return agent.Run{}, fmt.Errorf("run id is required")
	}
	row, err := r.q.GetAgentRunForPrincipal(ctx, platformdb.GetAgentRunForPrincipalParams{
		RunID: runID, PrincipalID: principalID,
	})
	if err != nil {
		return agent.Run{}, err
	}
	return mapRun(row), nil
}

func (r *Repository) AppendEvent(ctx context.Context, input agent.EventInput) (agent.Event, error) {
	payload, err := normalizedJSONObject(input.PayloadJSON)
	if err != nil {
		return agent.Event{}, err
	}
	principalID, err := agentPrincipalID(input.PrincipalID)
	if err != nil {
		return agent.Event{}, err
	}
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		return agent.Event{}, fmt.Errorf("event type is required")
	}
	severity := strings.TrimSpace(input.Severity)
	if severity == "" {
		severity = "info"
	}
	if input.Sequence <= 0 {
		return agent.Event{}, fmt.Errorf("event sequence is required")
	}
	exists, err := r.agentRunExists(ctx, principalID, input.RunID)
	if err != nil {
		return agent.Event{}, err
	}
	if !exists {
		return agent.Event{}, sql.ErrNoRows
	}
	data, err := json.Marshal(map[string]any{"sequence": input.Sequence, "severity": severity, "payload": json.RawMessage(payload)})
	if err != nil {
		return agent.Event{}, err
	}
	stored, err := r.events.AppendEvent(ctx, "agent_run", input.RunID, eventType, data)
	if err != nil {
		return agent.Event{}, err
	}
	return agent.Event{ID: fmt.Sprintf("%020d", stored.ID), RunID: input.RunID, Seq: input.Sequence, EventType: eventType, Severity: severity, PayloadJSON: payload, CreatedAt: stored.CreatedAt}, nil
}

func (r *Repository) ListEvents(ctx context.Context, principalID, runID string) ([]agent.Event, error) {
	return r.ListEventsPage(ctx, principalID, runID, agent.Page{})
}

func (r *Repository) ListEventsPage(ctx context.Context, principalID, runID string, page agent.Page) ([]agent.Event, error) {
	principalID, err := agentPrincipalID(principalID)
	if err != nil {
		return nil, err
	}
	if exists, err := r.agentRunExists(ctx, principalID, runID); err != nil {
		return nil, err
	} else if !exists {
		return nil, sql.ErrNoRows
	}
	limit := page.Limit
	if limit <= 0 {
		limit = 10000
	}
	if limit > 10000 {
		limit = 10000
	}
	after := int64(0)
	if strings.TrimSpace(page.After) != "" {
		parsed, parseErr := strconv.ParseInt(strings.TrimSpace(page.After), 10, 64)
		if parseErr != nil || parsed < 1 {
			return nil, fmt.Errorf("invalid event cursor")
		}
		after = parsed
	}
	out := []agent.Event{}
	for len(out) < limit {
		batchSize := min(200, limit-len(out))
		rows, err := r.events.ListEvents(ctx, "agent_run", runID, after, batchSize)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			var data struct {
				Sequence int64           `json:"sequence"`
				Severity string          `json:"severity"`
				Payload  json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(row.Data, &data); err != nil {
				return nil, err
			}
			out = append(out, agent.Event{ID: fmt.Sprintf("%020d", row.ID), RunID: runID, Seq: data.Sequence, EventType: row.EventType, Severity: data.Severity, PayloadJSON: string(data.Payload), CreatedAt: row.CreatedAt})
			after = row.ID
		}
		if len(rows) < batchSize {
			break
		}
	}
	return out, nil
}

func (r *Repository) agentRunExists(ctx context.Context, principalID, runID string) (bool, error) {
	exists, err := r.q.AgentRunExistsForPrincipal(ctx, platformdb.AgentRunExistsForPrincipalParams{
		RunID: runID, PrincipalID: principalID,
	})
	return exists != 0, err
}

func mapConversation(row platformdb.AgentConversation) agent.Conversation {
	out := agent.Conversation{
		ID:             row.ID,
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

func mapMessage(row platformdb.AgentMessage) agent.Message {
	out := agent.Message{
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

func mapRun(row platformdb.AgentRun) agent.Run {
	out := agent.Run{
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

func agentPrincipalID(principalID string) (string, error) {
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return "", fmt.Errorf("principal id is required")
	}
	return principalID, nil
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
	case agent.MessageRoleUser, agent.MessageRoleAssistant, agent.MessageRoleTool, agent.MessageRoleSummary:
		return true
	default:
		return false
	}
}

func validRunStatus(status string) bool {
	switch status {
	case agent.RunStatusRunning, agent.RunStatusCompleted, agent.RunStatusFailed, agent.RunStatusCanceled:
		return true
	default:
		return false
	}
}

func pageByID[T any](rows []T, page agent.Page, id func(T) string) []T {
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
