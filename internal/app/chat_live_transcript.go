package app

import (
	"strings"

	agentcap "github.com/Yacobolo/leapview/internal/agent"
	agentcore "github.com/Yacobolo/leapview/pkg/agent"
)

func appendServerUserTranscript(transcript []agentcap.ChatTranscriptItem, conversationID, input string) []agentcap.ChatTranscriptItem {
	if strings.TrimSpace(input) == "" {
		return transcript
	}
	next := append([]agentcap.ChatTranscriptItem{}, transcript...)
	next = append(next, agentcap.ChatTranscriptItem{
		ID:             "live:user",
		Kind:           "user",
		Text:           input,
		ConversationID: conversationID,
	})
	return next
}

func applyLiveTranscriptEvent(transcript []agentcap.ChatTranscriptItem, conversationID string, event agentcap.EventEnvelope) []agentcap.ChatTranscriptItem {
	next := append([]agentcap.ChatTranscriptItem{}, transcript...)
	switch event.Type {
	case string(agentcore.EventTypeMessageDelta):
		delta := stringPayload(event.Payload, "delta")
		if delta == "" {
			return next
		}
		for i := len(next) - 1; i >= 0; i-- {
			if next[i].Kind == "assistant" && next[i].Status == "streaming" && next[i].RunID == event.RunID {
				next[i].Markdown += delta
				return next
			}
		}
		return append(next, agentcap.ChatTranscriptItem{
			ID:             "live:assistant:" + event.RunID,
			Kind:           "assistant",
			Markdown:       delta,
			Status:         "streaming",
			ConversationID: conversationID,
			RunID:          event.RunID,
			CreatedAt:      event.CreatedAt,
		})
	case string(agentcore.EventTypeToolStart):
		callID := stringPayload(event.Payload, "tool_call_id")
		name := stringPayload(event.Payload, "tool_name")
		if callID == "" {
			return next
		}
		if idx := transcriptToolIndex(next, callID); idx >= 0 {
			next[idx].Status = "running"
			return next
		}
		return append(next, agentcap.ChatTranscriptItem{
			ID:             "live:tool:" + callID,
			Kind:           "tool",
			ToolCallID:     callID,
			Name:           name,
			Title:          liveToolTitle(name),
			Status:         "running",
			ConversationID: conversationID,
			RunID:          event.RunID,
			CreatedAt:      event.CreatedAt,
		})
	case string(agentcore.EventTypeToolEnd):
		callID := stringPayload(event.Payload, "tool_call_id")
		if callID == "" {
			return next
		}
		idx := transcriptToolIndex(next, callID)
		if idx < 0 {
			name := stringPayload(event.Payload, "tool_name")
			next = append(next, agentcap.ChatTranscriptItem{
				ID:             "live:tool:" + callID,
				Kind:           "tool",
				ToolCallID:     callID,
				Name:           name,
				Title:          liveToolTitle(name),
				ConversationID: conversationID,
				RunID:          event.RunID,
				CreatedAt:      event.CreatedAt,
			})
			idx = len(next) - 1
		}
		if event.Severity == string(agentcore.SeverityError) || event.Severity == string(agentcore.SeverityWarn) {
			next[idx].Status = "error"
			next[idx].Error = "Tool failed"
			return next
		}
		next[idx].Status = "complete"
		return next
	default:
		return next
	}
}

func transcriptToolIndex(transcript []agentcap.ChatTranscriptItem, callID string) int {
	for i := range transcript {
		if transcript[i].Kind == "tool" && transcript[i].ToolCallID == callID {
			return i
		}
	}
	return -1
}

func stringPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}

func liveToolTitle(name string) string {
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
