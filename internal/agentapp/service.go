package agentapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/pkg/agent"
)

var (
	ErrDisabled = errors.New("agent is not configured")
	ErrBusy     = errors.New("agent conversation already has a running turn")
)

const (
	maxToolArgumentsPreviewBytes = 2000
	maxToolResultPreviewBytes    = 4000
)

func IsBusy(err error) bool {
	return errors.Is(err, ErrBusy)
}

type Scope struct {
	WorkspaceID string
	PrincipalID string
}

type Service struct {
	metrics Metrics
	store   *platform.Store
	config  Config
	model   agent.Model

	mu      sync.Mutex
	running map[string]struct{}
}

func NewService(metrics Metrics, store *platform.Store, config Config) *Service {
	return &Service{
		metrics: metrics,
		store:   store,
		config:  config,
		model:   NewOpenAIModel(config, http.DefaultClient),
		running: map[string]struct{}{},
	}
}

func (s *Service) Enabled() bool {
	return s != nil && s.config.Enabled()
}

func (s *Service) CreateConversation(ctx context.Context, scope Scope, title string) (platformdb.AgentConversation, error) {
	if s.store == nil {
		return platformdb.AgentConversation{}, fmt.Errorf("agent store is required")
	}
	return s.store.CreateAgentConversation(ctx, platform.AgentConversationInput{
		WorkspaceID:  scope.WorkspaceID,
		PrincipalID:  scope.PrincipalID,
		Title:        title,
		MetadataJSON: `{}`,
	})
}

func (s *Service) ListConversations(ctx context.Context, scope Scope) ([]platformdb.AgentConversation, error) {
	return s.store.ListAgentConversations(ctx, scope.WorkspaceID, scope.PrincipalID)
}

func (s *Service) ListMessages(ctx context.Context, scope Scope, conversationID string) ([]platformdb.AgentMessage, error) {
	return s.store.ListAgentMessages(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID)
}

func (s *Service) ListEvents(ctx context.Context, scope Scope, runID string) ([]platformdb.AgentEvent, error) {
	return s.store.ListAgentEvents(ctx, scope.WorkspaceID, scope.PrincipalID, runID)
}

func (s *Service) ConversationEvents(ctx context.Context, scope Scope, conversationID string) ([]api.AgentEventEnvelope, error) {
	if _, err := s.store.GetAgentConversation(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID); err != nil {
		return nil, err
	}
	messages, err := s.store.ListAgentMessages(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID)
	if err != nil {
		return nil, err
	}
	events := make([]api.AgentEventEnvelope, 0, len(messages))
	for _, message := range messages {
		events = append(events, messageEnvelope(conversationID, message))
	}
	runs, err := s.store.ListAgentRuns(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		runEvents, err := s.store.ListAgentEvents(ctx, scope.WorkspaceID, scope.PrincipalID, run.ID)
		if err != nil {
			return nil, err
		}
		for _, event := range runEvents {
			events = append(events, eventEnvelope(conversationID, event))
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].CreatedAt == events[j].CreatedAt {
			return events[i].ID < events[j].ID
		}
		return events[i].CreatedAt < events[j].CreatedAt
	})
	return events, nil
}

func (s *Service) ConversationTranscript(ctx context.Context, scope Scope, conversationID string) ([]api.AgentChatTranscriptItem, error) {
	if _, err := s.store.GetAgentConversation(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID); err != nil {
		return nil, err
	}
	messages, err := s.store.ListAgentMessages(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID)
	if err != nil {
		return nil, err
	}
	return transcriptFromMessages(conversationID, messages), nil
}

type PromptInput struct {
	Scope          Scope
	ConversationID string
	Input          string
	CorrelationID  string
	OnEvent        func(api.AgentEventEnvelope)
}

type PromptResult struct {
	ConversationID string           `json:"conversationId"`
	RunID          string           `json:"runId"`
	StopReason     agent.StopReason `json:"stopReason"`
	Content        string           `json:"content"`
}

func (s *Service) Prompt(ctx context.Context, input PromptInput) (PromptResult, error) {
	if !s.Enabled() {
		return PromptResult{}, ErrDisabled
	}
	if s.store == nil {
		return PromptResult{}, fmt.Errorf("agent store is required")
	}
	if err := s.acquire(input.ConversationID); err != nil {
		return PromptResult{}, err
	}
	defer s.release(input.ConversationID)

	conversation, err := s.store.GetAgentConversation(ctx, input.Scope.WorkspaceID, input.Scope.PrincipalID, input.ConversationID)
	if err != nil {
		return PromptResult{}, err
	}
	initial, err := decodeTranscript(conversation.TranscriptJson)
	if err != nil {
		return PromptResult{}, err
	}
	runID := newID("run")
	run, err := s.store.CreateAgentRun(ctx, platform.AgentRunInput{
		WorkspaceID:    input.Scope.WorkspaceID,
		PrincipalID:    input.Scope.PrincipalID,
		ConversationID: input.ConversationID,
		RunID:          runID,
		Model:          s.config.Model,
		MetadataJSON:   metadataJSON(map[string]any{"base_url": s.config.normalizedBaseURL(), "model": s.config.Model}),
	})
	if err != nil {
		return PromptResult{}, err
	}
	sink := &storeEventSink{store: s.store, scope: input.Scope, conversationID: input.ConversationID, runID: run.ID, onEvent: input.OnEvent}
	def := agent.Definition{
		Name:              "libredash-readonly",
		SystemPrompt:      systemPrompt(),
		Model:             s.model,
		Tools:             s.toolDefinitions(input.Scope),
		InitialTranscript: initial,
		Events:            sink,
		IDGenerator:       fixedRunIDGenerator{runID: run.ID},
	}
	harness, err := agent.New(def)
	if err != nil {
		_ = s.finishRun(ctx, input, run.ID, platform.AgentRunStatusFailed, "", sink.usage, err)
		return PromptResult{}, err
	}
	result, promptErr := harness.Prompt(ctx, agent.PromptRequest{Input: input.Input, CorrelationID: input.CorrelationID})
	transcript := harness.Transcript()
	if err := s.persistNewMessages(ctx, input, run.ID, initial, transcript); err != nil && promptErr == nil {
		promptErr = err
	}
	if err := s.persistTranscript(ctx, input, transcript); err != nil && promptErr == nil {
		promptErr = err
	}
	status := platform.AgentRunStatusCompleted
	if promptErr != nil {
		status = platform.AgentRunStatusFailed
		if errors.Is(promptErr, context.Canceled) {
			status = platform.AgentRunStatusCanceled
		}
	}
	if err := s.finishRun(ctx, input, run.ID, status, result.StopReason, sink.usage, promptErr); err != nil && promptErr == nil {
		promptErr = err
	}
	if promptErr != nil {
		return PromptResult{}, promptErr
	}
	return PromptResult{
		ConversationID: input.ConversationID,
		RunID:          result.RunID,
		StopReason:     result.StopReason,
		Content:        result.FinalMessage.Content,
	}, nil
}

func (s *Service) acquire(conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.running[conversationID]; ok {
		return ErrBusy
	}
	s.running[conversationID] = struct{}{}
	return nil
}

func (s *Service) release(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, conversationID)
}

func (s *Service) persistNewMessages(ctx context.Context, input PromptInput, runID string, initial, transcript []agent.Message) error {
	seen := map[string]struct{}{}
	for _, message := range initial {
		if message.ID != "" {
			seen[message.ID] = struct{}{}
		}
	}
	for _, message := range transcript {
		if message.ID != "" {
			if _, ok := seen[message.ID]; ok {
				continue
			}
			seen[message.ID] = struct{}{}
		}
		if err := s.appendMessage(ctx, input, runID, message); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) appendMessage(ctx context.Context, input PromptInput, runID string, message agent.Message) error {
	if message.Role == agent.RoleSystem {
		return nil
	}
	row, err := s.store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    input.Scope.WorkspaceID,
		PrincipalID:    input.Scope.PrincipalID,
		ConversationID: input.ConversationID,
		RunID:          runID,
		Role:           platformRole(message.Role),
		ContentText:    message.Content,
		ContentJSON:    messageContentJSON(message),
		ToolCallID:     message.ToolCallID,
		ToolName:       message.ToolName,
		IsError:        message.IsError,
	})
	if err == nil && input.OnEvent != nil {
		input.OnEvent(messageEnvelope(input.ConversationID, row))
	}
	return err
}

func (s *Service) persistTranscript(ctx context.Context, input PromptInput, transcript []agent.Message) error {
	bytes, err := json.Marshal(transcript)
	if err != nil {
		return err
	}
	_, err = s.store.UpdateAgentConversationTranscript(ctx, input.Scope.WorkspaceID, input.Scope.PrincipalID, input.ConversationID, string(bytes))
	return err
}

func (s *Service) finishRun(ctx context.Context, input PromptInput, runID, status string, stop agent.StopReason, usage agent.Usage, runErr error) error {
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	_, err := s.store.FinishAgentRun(ctx, platform.AgentRunFinish{
		WorkspaceID:    input.Scope.WorkspaceID,
		PrincipalID:    input.Scope.PrincipalID,
		ConversationID: input.ConversationID,
		RunID:          runID,
		Status:         status,
		StopReason:     string(stop),
		InputTokens:    int64(usage.InputTokens),
		OutputTokens:   int64(usage.OutputTokens),
		TotalTokens:    int64(usage.TotalTokens),
		Error:          errText,
		MetadataJSON:   metadataJSON(map[string]any{"model": s.config.Model}),
	})
	return err
}

type storeEventSink struct {
	store          *platform.Store
	scope          Scope
	conversationID string
	runID          string
	onEvent        func(api.AgentEventEnvelope)
	usage          agent.Usage
	mu             sync.Mutex
}

func (s *storeEventSink) Emit(ctx context.Context, event agent.Event) error {
	if event.Type == agent.EventTypeModelResponse || event.Type == agent.EventTypeCompactionEnd {
		s.mu.Lock()
		s.usage.InputTokens += event.Usage.InputTokens
		s.usage.OutputTokens += event.Usage.OutputTokens
		s.usage.TotalTokens += event.Usage.TotalTokens
		s.mu.Unlock()
	}
	row, err := s.store.AppendAgentEvent(ctx, platform.AgentEventInput{
		WorkspaceID: s.scope.WorkspaceID,
		PrincipalID: s.scope.PrincipalID,
		RunID:       s.runID,
		Sequence:    event.Sequence,
		EventType:   string(event.Type),
		Severity:    string(event.Severity),
		PayloadJSON: eventPayloadJSON(event),
	})
	if err == nil && s.onEvent != nil {
		s.onEvent(eventEnvelope(s.conversationID, row))
	}
	return err
}

type fixedRunIDGenerator struct {
	runID string
}

func (g fixedRunIDGenerator) NewID(prefix string) string {
	if prefix == "run" {
		return g.runID
	}
	return newID(prefix)
}

func decodeTranscript(raw string) ([]agent.Message, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var messages []agent.Message
	if err := json.Unmarshal([]byte(raw), &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func newID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}

func metadataJSON(value map[string]any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func eventPayloadJSON(event agent.Event) string {
	payload := map[string]any{
		"type":           event.Type,
		"turn_id":        event.TurnID,
		"message_id":     event.MessageID,
		"tool_call_id":   event.ToolCallID,
		"tool_name":      event.ToolName,
		"stop_reason":    event.StopReason,
		"finish_reason":  event.FinishReason,
		"usage":          event.Usage,
		"provider":       event.Provider,
		"model":          event.Model,
		"provider_meta":  event.ProviderMetadata,
		"correlation_id": event.CorrelationID,
	}
	if event.Error != nil {
		payload["error"] = event.Error.Error()
	}
	if event.Delta != "" {
		payload["delta"] = event.Delta
	}
	return metadataJSON(payload)
}

func eventPayload(raw string) map[string]any {
	payload := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return payload
	}
	_ = json.Unmarshal([]byte(raw), &payload)
	return payload
}

func eventEnvelope(conversationID string, row platformdb.AgentEvent) api.AgentEventEnvelope {
	return api.AgentEventEnvelope{
		ID:             row.ID,
		ConversationID: conversationID,
		RunID:          row.RunID,
		Seq:            row.Seq,
		Type:           row.EventType,
		Severity:       row.Severity,
		CreatedAt:      row.CreatedAt,
		Payload:        eventPayload(row.PayloadJson),
	}
}

func messageEnvelope(conversationID string, row platformdb.AgentMessage) api.AgentEventEnvelope {
	runID := ""
	if row.RunID.Valid {
		runID = row.RunID.String
	}
	return api.AgentEventEnvelope{
		ID:             "message:" + row.ID,
		ConversationID: conversationID,
		RunID:          runID,
		Seq:            row.Seq,
		Type:           "message_appended",
		Severity:       "info",
		CreatedAt:      row.CreatedAt,
		Payload: map[string]any{
			"message": map[string]any{
				"id":           row.ID,
				"role":         row.Role,
				"content":      row.ContentText,
				"content_json": eventPayload(row.ContentJson),
				"tool_call_id": row.ToolCallID,
				"tool_name":    row.ToolName,
				"is_error":     row.IsError,
			},
		},
	}
}

func messageContentJSON(message agent.Message) string {
	payload := map[string]any{
		"role":          message.Role,
		"content":       message.Content,
		"tool_calls":    message.ToolCalls,
		"tool_call_id":  message.ToolCallID,
		"tool_name":     message.ToolName,
		"is_error":      message.IsError,
		"finish_reason": message.FinishReason,
		"usage":         message.Usage,
	}
	return metadataJSON(payload)
}

func transcriptFromMessages(conversationID string, messages []platformdb.AgentMessage) []api.AgentChatTranscriptItem {
	items := make([]api.AgentChatTranscriptItem, 0, len(messages))
	toolIndex := map[string]int{}
	for _, message := range messages {
		runID := ""
		if message.RunID.Valid {
			runID = message.RunID.String
		}
		switch message.Role {
		case platform.AgentMessageRoleUser:
			items = append(items, api.AgentChatTranscriptItem{
				ID:             message.ID,
				Kind:           "user",
				Text:           message.ContentText,
				ConversationID: conversationID,
				RunID:          runID,
				CreatedAt:      message.CreatedAt,
			})
		case platform.AgentMessageRoleAssistant:
			if strings.TrimSpace(message.ContentText) != "" {
				items = append(items, api.AgentChatTranscriptItem{
					ID:             message.ID,
					Kind:           "assistant",
					Markdown:       message.ContentText,
					Status:         "complete",
					ConversationID: conversationID,
					RunID:          runID,
					CreatedAt:      message.CreatedAt,
				})
			}
			for _, call := range toolCallsFromContentJSON(message.ContentJson) {
				if call.ID == "" {
					continue
				}
				toolIndex[call.ID] = len(items)
				items = append(items, api.AgentChatTranscriptItem{
					ID:             "tool:" + call.ID,
					Kind:           "tool",
					ToolCallID:     call.ID,
					Name:           call.Name,
					Title:          toolTitle(call.Name),
					Status:         "running",
					InputJSON:      formatToolCallPreview(call),
					ArgumentsJSON:  formatJSONPreview(string(call.Arguments), maxToolArgumentsPreviewBytes),
					ConversationID: conversationID,
					RunID:          runID,
					CreatedAt:      message.CreatedAt,
				})
			}
		case platform.AgentMessageRoleTool:
			item := api.AgentChatTranscriptItem{
				ID:             message.ID,
				Kind:           "tool",
				ToolCallID:     message.ToolCallID,
				Name:           message.ToolName,
				Title:          toolTitle(message.ToolName),
				Status:         "complete",
				Summary:        toolSummary(message.ContentText),
				ResultSummary:  toolSummary(message.ContentText),
				ResultJSON:     formatJSONPreview(message.ContentText, maxToolResultPreviewBytes),
				ConversationID: conversationID,
				RunID:          runID,
				CreatedAt:      message.CreatedAt,
			}
			if message.IsError {
				item.Status = "error"
				item.Error = toolErrorSummary(message.ContentText)
				item.Summary = ""
				item.ResultSummary = ""
			}
			if idx, ok := toolIndex[message.ToolCallID]; ok {
				items[idx] = mergeToolTranscriptItem(items[idx], item)
				continue
			}
			items = append(items, item)
		}
	}
	return items
}

type transcriptToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func toolCallsFromContentJSON(raw string) []transcriptToolCall {
	var payload struct {
		ToolCalls []transcriptToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	return payload.ToolCalls
}

func mergeToolTranscriptItem(started, finished api.AgentChatTranscriptItem) api.AgentChatTranscriptItem {
	started.ID = finished.ID
	started.Status = finished.Status
	started.Summary = finished.Summary
	started.ResultSummary = finished.ResultSummary
	started.ResultJSON = finished.ResultJSON
	started.Error = finished.Error
	started.RunID = finished.RunID
	if started.InputJSON == "" {
		started.InputJSON = finished.InputJSON
	}
	if started.ArgumentsJSON == "" {
		started.ArgumentsJSON = finished.ArgumentsJSON
	}
	if started.Name == "" {
		started.Name = finished.Name
	}
	if started.Title == "" {
		started.Title = finished.Title
	}
	return started
}

func toolTitle(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Tool"
	}
	parts := strings.Fields(strings.ReplaceAll(name, "_", " "))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func formatToolCallPreview(call transcriptToolCall) string {
	payload := struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{
		Name:      call.Name,
		Arguments: "{}",
	}
	if len(call.Arguments) > 0 && json.Valid(call.Arguments) {
		payload.Arguments = string(call.Arguments)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return formatJSONPreview(string(raw), maxToolArgumentsPreviewBytes)
}

func formatJSONPreview(raw string, limit int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || limit <= 0 {
		return ""
	}
	var indented bytes.Buffer
	if json.Valid([]byte(raw)) {
		if err := json.Indent(&indented, []byte(raw), "", "  "); err == nil {
			raw = indented.String()
		}
	}
	return truncateDisplayText(raw, limit)
}

func toolSummary(raw string) string {
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return truncateDisplayText(raw, 160)
	}
	for _, key := range []string{"summary", "title", "name", "message"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return truncateDisplayText(value, 160)
		}
	}
	if total, ok := payload["total"].(float64); ok {
		return fmt.Sprintf("Returned %.0f records", total)
	}
	if count, ok := payload["count"].(float64); ok {
		return fmt.Sprintf("Returned %.0f records", count)
	}
	return ""
}

func toolErrorSummary(raw string) string {
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return truncateDisplayText(raw, 200)
	}
	if errPayload, ok := payload["error"].(map[string]any); ok {
		message, _ := errPayload["message"].(string)
		code, _ := errPayload["code"].(string)
		switch {
		case message != "" && code != "":
			return truncateDisplayText(code+": "+message, 200)
		case message != "":
			return truncateDisplayText(message, 200)
		case code != "":
			return truncateDisplayText(code, 200)
		}
	}
	return ""
}

func truncateDisplayText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 1 {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-1]) + "..."
}

func platformRole(role agent.Role) string {
	switch role {
	case agent.RoleUser:
		return platform.AgentMessageRoleUser
	case agent.RoleAssistant:
		return platform.AgentMessageRoleAssistant
	case agent.RoleTool:
		return platform.AgentMessageRoleTool
	case agent.RoleSummary:
		return platform.AgentMessageRoleSummary
	default:
		return string(role)
	}
}

func systemPrompt() string {
	return `You are LibreDash's read-only BI assistant. Answer using only the provided tools and conversation context. You can help users understand dashboards, semantic models, metric views, filters, visuals, and table snapshots they are allowed to access. Do not invent dashboard IDs, metric names, or data values. You cannot write data, deploy changes, edit permissions, run raw SQL, access files, or call external services.`
}
