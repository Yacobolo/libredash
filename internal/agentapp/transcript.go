package agentapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/pkg/agent"
)

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
