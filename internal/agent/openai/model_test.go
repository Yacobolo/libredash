package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentapp "github.com/Yacobolo/libredash/internal/agent"
	agentcore "github.com/Yacobolo/libredash/pkg/agent"
)

func TestNewModelUsesBoundedDefaultHTTPClient(t *testing.T) {
	model := NewModel(agentapp.Config{APIKey: "test-key", BaseURL: "https://api.example.com", Model: "test-model"}, nil)
	if model.client == nil {
		t.Fatal("default HTTP client is nil")
	}
	if model.client == http.DefaultClient {
		t.Fatal("default HTTP client should not use the unbounded process-global client")
	}
	if model.client.Timeout != DefaultHTTPTimeout {
		t.Fatalf("default HTTP timeout = %s, want %s", model.client.Timeout, DefaultHTTPTimeout)
	}
}

func TestOpenAIModelConvertsChatCompletionPayloads(t *testing.T) {
	var got openAIChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeJSON(t, w, openAIChatResponse{
			ID: "chatcmpl_test",
			Choices: []openAIChoice{{
				Index: 0,
				Message: openAIMessage{
					Role:    "assistant",
					Content: "I will check.",
					ToolCalls: []openAIToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: openAIFunctionCall{
							Name:      "list_dashboards",
							Arguments: `{}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
			Usage: openAIUsage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18},
		})
	}))
	defer server.Close()

	model := NewModel(agentapp.Config{APIKey: "test-key", BaseURL: server.URL, Model: "deepseek-v4-flash"}, server.Client())
	resp, err := model.Complete(context.Background(), agentcore.ModelRequest{
		Purpose: agentcore.ModelRequestPurposeTurn,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: "system"},
			{Role: agentcore.RoleUser, Content: "hello"},
			{Role: agentcore.RoleTool, ToolCallID: "call_previous", ToolName: "list_dashboards", Content: `{"dashboards":[]}`},
		},
		Tools: []agentcore.ToolSpec{{
			Name:        "list_dashboards",
			Description: "List dashboards.",
			InputSchema: []byte(`{"type":"object","additionalProperties":false}`),
		}},
		Limits: agentcore.Limits{ReserveOutputTokens: 123},
	}, nil)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if got.Model != "deepseek-v4-flash" || got.MaxTokens != 123 {
		t.Fatalf("request model/max = %s/%d", got.Model, got.MaxTokens)
	}
	if got.Thinking == nil || got.Thinking.Type != "disabled" {
		t.Fatalf("deepseek v4 request should disable thinking: %#v", got.Thinking)
	}
	if len(got.Messages) != 3 || got.Messages[2].Role != "tool" || got.Messages[2].ToolCallID != "call_previous" {
		t.Fatalf("messages = %#v", got.Messages)
	}
	if len(got.Tools) != 1 || got.Tools[0].Function.Name != "list_dashboards" {
		t.Fatalf("tools = %#v", got.Tools)
	}
	if resp.Content != "I will check." || resp.FinishReason != agentcore.FinishReasonToolCalls {
		t.Fatalf("response = %#v", resp)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "list_dashboards" {
		t.Fatalf("tool calls = %#v", resp.ToolCalls)
	}
	if resp.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
	if resp.ProviderMetadata["id"] != "chatcmpl_test" || resp.ProviderMetadata["model"] != "deepseek-v4-flash" {
		t.Fatalf("metadata = %#v", resp.ProviderMetadata)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
