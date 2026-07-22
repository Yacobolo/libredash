package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	agentapp "github.com/Yacobolo/leapview/internal/agent"
	agentcore "github.com/Yacobolo/leapview/pkg/agent"
)

const DefaultHTTPTimeout = 5 * time.Minute

type OpenAIModel struct {
	config agentapp.Config
	client *http.Client
}

func NewModel(config agentapp.Config, client *http.Client) *OpenAIModel {
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPTimeout}
	}
	return &OpenAIModel{config: config, client: client}
}

func (m *OpenAIModel) Complete(ctx context.Context, req agentcore.ModelRequest, stream agentcore.ModelStream) (agentcore.ModelResponse, error) {
	if !m.config.Enabled() {
		return agentcore.ModelResponse{}, agentapp.ErrDisabled
	}
	body := openAIChatRequest{
		Model:     m.config.Model,
		Messages:  openAIMessages(req.Messages),
		Tools:     openAITools(req.Tools),
		MaxTokens: req.Limits.ReserveOutputTokens,
	}
	if disableThinkingForRequest(m.config) {
		body.Thinking = &openAIThinking{Type: "disabled"}
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return agentcore.ModelResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.config.NormalizedBaseURL()+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return agentcore.ModelResponse{}, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.config.APIKey)

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return agentcore.ModelResponse{}, err
	}
	defer resp.Body.Close()
	bytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		if isContextLimitResponse(resp.StatusCode, string(bytes)) {
			return agentcore.ModelResponse{}, agentcore.ErrContextLength
		}
		return agentcore.ModelResponse{}, fmt.Errorf("chat completion failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bytes)))
	}
	var decoded openAIChatResponse
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		return agentcore.ModelResponse{}, err
	}
	if len(decoded.Choices) == 0 {
		return agentcore.ModelResponse{}, errors.New("chat completion returned no choices")
	}
	choice := decoded.Choices[0]
	out := agentcore.ModelResponse{
		Content:      choice.Message.Content,
		FinishReason: agentcore.NormalizeFinishReason(agentcore.FinishReason(choice.FinishReason)),
		Usage: agentcore.Usage{
			InputTokens:  decoded.Usage.PromptTokens,
			OutputTokens: decoded.Usage.CompletionTokens,
			TotalTokens:  decoded.Usage.TotalTokens,
		},
		ProviderMetadata: map[string]any{
			"id":    decoded.ID,
			"model": m.config.Model,
		},
	}
	for _, call := range choice.Message.ToolCalls {
		if call.Type != "" && call.Type != "function" {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, agentcore.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}
	if out.FinishReason == agentcore.FinishReasonUnknown && len(out.ToolCalls) > 0 {
		out.FinishReason = agentcore.FinishReasonToolCalls
	}
	if req.Purpose == agentcore.ModelRequestPurposeTurn && out.Content != "" && stream != nil {
		_ = stream.Delta(ctx, out.Content)
	}
	return out, nil
}

func openAIMessages(messages []agentcore.Message) []openAIMessage {
	out := make([]openAIMessage, 0, len(messages))
	for _, message := range messages {
		msg := openAIMessage{
			Role:       string(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
		}
		for _, call := range message.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, openAIToolCall{
				ID:   call.ID,
				Type: "function",
				Function: openAIFunctionCall{
					Name:      call.Name,
					Arguments: string(call.Arguments),
				},
			})
		}
		out = append(out, msg)
	}
	return out
}

func openAITools(tools []agentcore.ToolSpec) []openAITool {
	out := make([]openAITool, 0, len(tools))
	for _, tool := range tools {
		params := json.RawMessage(`{"type":"object"}`)
		if len(tool.InputSchema) > 0 {
			params = append(json.RawMessage(nil), tool.InputSchema...)
		}
		out = append(out, openAITool{
			Type: "function",
			Function: openAIToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

func isContextLimitResponse(status int, body string) bool {
	if status != http.StatusBadRequest && status != http.StatusRequestEntityTooLarge {
		return false
	}
	body = strings.ToLower(body)
	return strings.Contains(body, "context") || strings.Contains(body, "maximum context") || strings.Contains(body, "token")
}

// DeepSeek V4 enables reasoning by default; short metadata calls need normal content.
func disableThinkingForRequest(config agentapp.Config) bool {
	model := strings.ToLower(strings.TrimSpace(config.Model))
	return strings.HasPrefix(model, "deepseek-v4")
}

type openAIChatRequest struct {
	Model      string          `json:"model"`
	Messages   []openAIMessage `json:"messages"`
	Tools      []openAITool    `json:"tools,omitempty"`
	ToolChoice string          `json:"tool_choice,omitempty"`
	MaxTokens  int             `json:"max_tokens,omitempty"`
	Thinking   *openAIThinking `json:"thinking,omitempty"`
}

type openAIThinking struct {
	Type string `json:"type"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	ID      string         `json:"id"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
