package agentapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/pkg/agent"
)

type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.Model) != ""
}

func (c Config) normalizedBaseURL() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "https://api.openai.com/v1"
	}
	return strings.TrimRight(c.BaseURL, "/")
}

type OpenAIModel struct {
	config Config
	client *http.Client
}

func NewOpenAIModel(config Config, client *http.Client) *OpenAIModel {
	if client == nil {
		client = http.DefaultClient
	}
	return &OpenAIModel{config: config, client: client}
}

func (m *OpenAIModel) Complete(ctx context.Context, req agent.ModelRequest, stream agent.ModelStream) (agent.ModelResponse, error) {
	if !m.config.Enabled() {
		return agent.ModelResponse{}, ErrDisabled
	}
	body := openAIChatRequest{
		Model:     m.config.Model,
		Messages:  openAIMessages(req.Messages),
		Tools:     openAITools(req.Tools),
		MaxTokens: req.Limits.ReserveOutputTokens,
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return agent.ModelResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.config.normalizedBaseURL()+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return agent.ModelResponse{}, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.config.APIKey)

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return agent.ModelResponse{}, err
	}
	defer resp.Body.Close()
	bytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		if isContextLimitResponse(resp.StatusCode, string(bytes)) {
			return agent.ModelResponse{}, agent.ErrContextLength
		}
		return agent.ModelResponse{}, fmt.Errorf("chat completion failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bytes)))
	}
	var decoded openAIChatResponse
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		return agent.ModelResponse{}, err
	}
	if len(decoded.Choices) == 0 {
		return agent.ModelResponse{}, errors.New("chat completion returned no choices")
	}
	choice := decoded.Choices[0]
	out := agent.ModelResponse{
		Content:      choice.Message.Content,
		FinishReason: agent.NormalizeFinishReason(agent.FinishReason(choice.FinishReason)),
		Usage: agent.Usage{
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
		out.ToolCalls = append(out.ToolCalls, agent.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}
	if out.FinishReason == agent.FinishReasonUnknown && len(out.ToolCalls) > 0 {
		out.FinishReason = agent.FinishReasonToolCalls
	}
	if req.Purpose == agent.ModelRequestPurposeTurn && out.Content != "" && stream != nil {
		_ = stream.Delta(ctx, out.Content)
	}
	return out, nil
}

func openAIMessages(messages []agent.Message) []openAIMessage {
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

func openAITools(tools []agent.ToolSpec) []openAITool {
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

type openAIChatRequest struct {
	Model      string          `json:"model"`
	Messages   []openAIMessage `json:"messages"`
	Tools      []openAITool    `json:"tools,omitempty"`
	ToolChoice string          `json:"tool_choice,omitempty"`
	MaxTokens  int             `json:"max_tokens,omitempty"`
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
