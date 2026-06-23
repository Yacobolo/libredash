package agentapp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/pkg/agent"
)

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
