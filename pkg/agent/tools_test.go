package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestToolValidationFailuresBecomeToolResults(t *testing.T) {
	tests := []struct {
		name     string
		call     ToolCall
		wantCode string
	}{
		{
			name:     "unknown tool",
			call:     ToolCall{ID: "call_1", Name: "missing", Arguments: json.RawMessage(`{}`)},
			wantCode: "unknown_tool",
		},
		{
			name:     "malformed json",
			call:     ToolCall{ID: "call_1", Name: "lookup", Arguments: json.RawMessage(`{`)},
			wantCode: "invalid_tool_arguments",
		},
		{
			name:     "schema mismatch",
			call:     ToolCall{ID: "call_1", Name: "lookup", Arguments: json.RawMessage(`{"id":7}`)},
			wantCode: "invalid_tool_arguments",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := &fakeModel{responses: []ModelResponse{
				{ToolCalls: []ToolCall{tc.call}, FinishReason: FinishReasonToolCalls},
				{Content: "repaired", FinishReason: FinishReasonStop},
			}}
			a := mustAgent(t, Definition{
				Name:         "test",
				SystemPrompt: "x",
				Model:        model,
				Tools: []ToolDefinition{{
					Name:        "lookup",
					Description: "lookup",
					InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}`),
					Handler:     noopTool(),
				}},
			})

			_, err := a.Prompt(context.Background(), PromptRequest{Input: "go"})
			if err != nil {
				t.Fatalf("Prompt returned error: %v", err)
			}
			transcript := a.Transcript()
			var tool Message
			for _, message := range transcript {
				if message.Role == RoleTool {
					tool = message
					break
				}
			}
			if !tool.IsError {
				t.Fatalf("tool result IsError = false, want true: %#v", tool)
			}
			if !strings.Contains(tool.Content, tc.wantCode) {
				t.Fatalf("tool result = %s, want code %s", tool.Content, tc.wantCode)
			}
		})
	}
}

func TestToolOutputValidationAndHandlerFailures(t *testing.T) {
	tests := []struct {
		name         string
		handler      ToolHandler
		limit        int
		displayLimit int
		wantCode     string
	}{
		{
			name: "handler error",
			handler: ToolHandlerFunc(func(context.Context, ToolCall) (ToolResult, error) {
				return ToolResult{}, errors.New("service unavailable")
			}),
			wantCode: "tool_execution_failed",
		},
		{
			name: "panic",
			handler: ToolHandlerFunc(func(context.Context, ToolCall) (ToolResult, error) {
				panic("bad tool")
			}),
			wantCode: "tool_panic",
		},
		{
			name: "nil content",
			handler: ToolHandlerFunc(func(context.Context, ToolCall) (ToolResult, error) {
				return ToolResult{}, nil
			}),
			wantCode: "tool_result_invalid",
		},
		{
			name: "not serializable",
			handler: ToolHandlerFunc(func(context.Context, ToolCall) (ToolResult, error) {
				return ToolResult{Content: map[string]any{"bad": func() {}}}, nil
			}),
			wantCode: "tool_result_invalid",
		},
		{
			name: "too large",
			handler: ToolHandlerFunc(func(context.Context, ToolCall) (ToolResult, error) {
				return ToolResult{Content: map[string]any{"value": strings.Repeat("x", 100)}}, nil
			}),
			limit:    12,
			wantCode: "tool_output_too_large",
		},
		{
			name: "display too large",
			handler: ToolHandlerFunc(func(context.Context, ToolCall) (ToolResult, error) {
				return ToolResult{
					Content:        map[string]any{"ok": true},
					DisplayContent: map[string]any{"rows": strings.Repeat("row-data", 100)},
				}, nil
			}),
			displayLimit: 24,
			wantCode:     "tool_display_output_too_large",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limits := Limits{}
			if tc.limit > 0 {
				limits.MaxToolResultBytes = tc.limit
			}
			if tc.displayLimit > 0 {
				limits.MaxToolDisplayBytes = tc.displayLimit
			}
			model := &fakeModel{responses: []ModelResponse{
				{ToolCalls: []ToolCall{{ID: "call_1", Name: "work", Arguments: json.RawMessage(`{}`)}}, FinishReason: FinishReasonToolCalls},
				{Content: "done", FinishReason: FinishReasonStop},
			}}
			a := mustAgent(t, Definition{
				Name:         "test",
				SystemPrompt: "x",
				Model:        model,
				Limits:       limits,
				Tools: []ToolDefinition{{
					Name:        "work",
					Description: "work",
					InputSchema: json.RawMessage(`{"type":"object"}`),
					Handler:     tc.handler,
				}},
			})

			_, err := a.Prompt(context.Background(), PromptRequest{Input: "go"})
			if err != nil {
				t.Fatalf("Prompt returned error: %v", err)
			}
			tool := onlyToolMessage(t, a.Transcript())
			if !tool.IsError || !strings.Contains(tool.Content, tc.wantCode) {
				t.Fatalf("tool content = %s, IsError=%v, want %s", tool.Content, tool.IsError, tc.wantCode)
			}
			if strings.Contains(tool.Content, "row-data") || tool.DisplayContent != nil {
				t.Fatalf("tool error leaked display payload: %#v", tool)
			}
		})
	}
}

func TestToolExecutionIsBoundedParallelAndOrdered(t *testing.T) {
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var running int32
	var maxRunning int32
	handler := ToolHandlerFunc(func(ctx context.Context, call ToolCall) (ToolResult, error) {
		current := atomic.AddInt32(&running, 1)
		for {
			old := atomic.LoadInt32(&maxRunning)
			if current <= old || atomic.CompareAndSwapInt32(&maxRunning, old, current) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-ctx.Done():
			atomic.AddInt32(&running, -1)
			return ToolResult{}, ctx.Err()
		case <-release:
			atomic.AddInt32(&running, -1)
			return ToolResult{Content: map[string]any{"id": call.ID}}, nil
		}
	})
	model := &fakeModel{responses: []ModelResponse{
		{ToolCalls: []ToolCall{
			{ID: "call_1", Name: "work", Arguments: json.RawMessage(`{}`)},
			{ID: "call_2", Name: "work", Arguments: json.RawMessage(`{}`)},
			{ID: "call_3", Name: "work", Arguments: json.RawMessage(`{}`)},
		}, FinishReason: FinishReasonToolCalls},
		{Content: "done", FinishReason: FinishReasonStop},
	}}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: "x",
		Model:        model,
		Limits:       Limits{MaxConcurrentTools: 2, ToolTimeout: time.Second},
		Tools: []ToolDefinition{{
			Name:        "work",
			Description: "work",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler:     handler,
		}},
	})

	done := make(chan error, 1)
	go func() {
		_, err := a.Prompt(context.Background(), PromptRequest{Input: "go"})
		done <- err
	}()
	<-started
	<-started
	if atomic.LoadInt32(&maxRunning) != 2 {
		t.Fatalf("max running = %d, want 2", maxRunning)
	}
	close(release)
	<-started
	if err := <-done; err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}

	transcript := a.Transcript()
	gotIDs := make([]string, 0, 3)
	for _, message := range transcript {
		if message.Role == RoleTool {
			gotIDs = append(gotIDs, message.ToolCallID)
		}
	}
	if strings.Join(gotIDs, ",") != "call_1,call_2,call_3" {
		t.Fatalf("tool result order = %v", gotIDs)
	}
}

func TestToolDisplayContentIsNotSentBackToModel(t *testing.T) {
	model := &fakeModel{responses: []ModelResponse{
		{ToolCalls: []ToolCall{{ID: "call_1", Name: "visual", Arguments: json.RawMessage(`{}`)}}, FinishReason: FinishReasonToolCalls},
		{Content: "done", FinishReason: FinishReasonStop},
	}}
	display := map[string]any{
		"kind": "table",
		"patch": map[string]any{
			"tables": map[string]any{
				"agent_table_1": map[string]any{"rows": strings.Repeat("row-data", 1000)},
			},
		},
	}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: "x",
		Model:        model,
		Tools: []ToolDefinition{{
			Name:        "visual",
			Description: "visual",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler: ToolHandlerFunc(func(context.Context, ToolCall) (ToolResult, error) {
				return ToolResult{
					Content:        map[string]any{"ok": true, "id": "agent_table_1"},
					DisplayContent: display,
				}, nil
			}),
		}},
	})

	if _, err := a.Prompt(context.Background(), PromptRequest{Input: "go"}); err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	tool := onlyToolMessage(t, a.Transcript())
	if tool.DisplayContent == nil {
		t.Fatalf("tool transcript missing display content: %#v", tool)
	}
	if !strings.Contains(tool.Content, `"ok":true`) || strings.Contains(tool.Content, "row-data") {
		t.Fatalf("tool model content should be compact: %s", tool.Content)
	}
	if len(model.requests) != 2 {
		t.Fatalf("model requests = %d, want 2", len(model.requests))
	}
	last := model.requests[1].Messages[len(model.requests[1].Messages)-1]
	if last.Role != RoleTool || last.DisplayContent != nil {
		t.Fatalf("second model request leaked display content: %#v", last)
	}
	if strings.Contains(last.Content, "row-data") {
		t.Fatalf("second model request leaked display rows: %s", last.Content)
	}
}

func TestToolDisplayContentDoesNotAffectTokenEstimate(t *testing.T) {
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: "x",
		Model:        &fakeModel{responses: []ModelResponse{{Content: "ok", FinishReason: FinishReasonStop}}},
	})
	base := []Message{{Role: RoleTool, ToolCallID: "call_1", ToolName: "visual", Content: `{"ok":true}`}}
	withDisplay := []Message{{Role: RoleTool, ToolCallID: "call_1", ToolName: "visual", Content: `{"ok":true}`, DisplayContent: map[string]any{"rows": strings.Repeat("row-data", 1000)}}}
	if got, want := a.estimateModelInputTokens(withDisplay), a.estimateModelInputTokens(base); got != want {
		t.Fatalf("token estimate with display = %d, want %d", got, want)
	}
}

func TestToolFatalResultStopsRunAfterAppendingResult(t *testing.T) {
	model := &fakeModel{responses: []ModelResponse{
		{ToolCalls: []ToolCall{{ID: "call_1", Name: "fatal", Arguments: json.RawMessage(`{}`)}}, FinishReason: FinishReasonToolCalls},
		{Content: "should not call", FinishReason: FinishReasonStop},
	}}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: "x",
		Model:        model,
		Tools: []ToolDefinition{{
			Name:        "fatal",
			Description: "fatal",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler: ToolHandlerFunc(func(context.Context, ToolCall) (ToolResult, error) {
				return ToolResult{Content: map[string]any{"error": "stop"}, Fatal: true}, nil
			}),
		}},
	})

	result, err := a.Prompt(context.Background(), PromptRequest{Input: "go"})
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if result.StopReason != StopReasonFatalToolError {
		t.Fatalf("StopReason = %s, want fatal_tool_error", result.StopReason)
	}
	if len(model.requests) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.requests))
	}
	tool := onlyToolMessage(t, a.Transcript())
	if tool.ToolCallID != "call_1" || !strings.Contains(tool.Content, "stop") {
		t.Fatalf("tool result = %#v, want appended fatal result", tool)
	}
}

func TestFatalToolErrorStopsRunAfterAppendingErrorResult(t *testing.T) {
	model := &fakeModel{responses: []ModelResponse{
		{ToolCalls: []ToolCall{{ID: "call_1", Name: "fatal", Arguments: json.RawMessage(`{}`)}}, FinishReason: FinishReasonToolCalls},
		{Content: "should not call", FinishReason: FinishReasonStop},
	}}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: "x",
		Model:        model,
		Tools: []ToolDefinition{{
			Name:        "fatal",
			Description: "fatal",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler: ToolHandlerFunc(func(context.Context, ToolCall) (ToolResult, error) {
				return ToolResult{}, FatalToolError(errors.New("stop now"))
			}),
		}},
	})

	result, err := a.Prompt(context.Background(), PromptRequest{Input: "go"})
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if result.StopReason != StopReasonFatalToolError {
		t.Fatalf("StopReason = %s, want fatal_tool_error", result.StopReason)
	}
	tool := onlyToolMessage(t, a.Transcript())
	if !tool.IsError || !strings.Contains(tool.Content, "tool_execution_failed") {
		t.Fatalf("tool result = %#v, want execution error", tool)
	}
}

func onlyToolMessage(t *testing.T, messages []Message) Message {
	t.Helper()
	for _, message := range messages {
		if message.Role == RoleTool {
			return message
		}
	}
	t.Fatal("no tool message found")
	return Message{}
}
