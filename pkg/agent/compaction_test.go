package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCompactionKeepsLastTurnsAndUsesNoTools(t *testing.T) {
	model := &fakeModel{responses: []ModelResponse{
		{Content: "summary one", FinishReason: FinishReasonStop},
		{Content: "answer", FinishReason: FinishReasonStop},
		{Content: "summary final", FinishReason: FinishReasonStop},
	}}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: "x",
		Model:        model,
		Limits:       Limits{ContextWindowTokens: 100, ReserveOutputTokens: 10},
		Compaction:   CompactionConfig{KeepLastTurns: 1, TriggerRatio: 0.01},
		Tools: []ToolDefinition{{
			Name:        "noop",
			Description: "noop",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler:     noopTool(),
		}},
	})
	a.transcript = []Message{
		{Role: RoleUser, Content: "old user"},
		{Role: RoleAssistant, Content: "old assistant"},
		{Role: RoleUser, Content: "recent user"},
		{Role: RoleAssistant, Content: "recent assistant", ToolCalls: []ToolCall{{ID: "call_1", Name: "noop", Arguments: json.RawMessage(`{}`)}}},
		{Role: RoleTool, ToolCallID: "call_1", ToolName: "noop", Content: `{"ok":true}`},
	}

	result, err := a.Prompt(context.Background(), PromptRequest{Input: "new"})
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if result.StopReason != StopReasonCompleted {
		t.Fatalf("StopReason = %s, want completed", result.StopReason)
	}
	if len(model.requests) < 2 {
		t.Fatalf("model requests = %d, want compaction and turn", len(model.requests))
	}
	if model.requests[0].Purpose != ModelRequestPurposeCompaction {
		t.Fatalf("first purpose = %s, want compaction", model.requests[0].Purpose)
	}
	if len(model.requests[0].Tools) != 0 {
		t.Fatalf("compaction tools = %d, want 0", len(model.requests[0].Tools))
	}
	if model.requests[1].Purpose != ModelRequestPurposeTurn {
		t.Fatalf("second purpose = %s, want turn", model.requests[1].Purpose)
	}
	if got := roles(model.requests[1].Messages); !strings.HasPrefix(got, "system,system,") {
		t.Fatalf("turn request roles = %s, want system then converted summary", got)
	}

	transcript := a.Transcript()
	if transcript[0].Role != RoleSummary || transcript[0].Content != "summary final" {
		t.Fatalf("first transcript message = %#v, want summary", transcript[0])
	}
	if containsToolResultFor(transcript, "call_1") && !toolResultFollowsAssistant(transcript, "call_1") {
		t.Fatalf("compaction split existing tool result: %#v", transcript)
	}
}

func TestContextOverflowRetriesOnceAfterCompaction(t *testing.T) {
	model := &fakeModel{
		responses: []ModelResponse{
			{Content: "summary", FinishReason: FinishReasonStop},
			{Content: "after retry", FinishReason: FinishReasonStop},
		},
		errs: []error{ErrContextLength},
	}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: "x",
		Model:        model,
		Limits:       Limits{ContextWindowTokens: 1000, ReserveOutputTokens: 10},
		Compaction:   CompactionConfig{KeepLastTurns: 1, TriggerRatio: 1},
	})
	a.transcript = []Message{
		{Role: RoleUser, Content: "older"},
		{Role: RoleAssistant, Content: "older"},
		{Role: RoleUser, Content: "old"},
		{Role: RoleAssistant, Content: "old"},
	}

	result, err := a.Prompt(context.Background(), PromptRequest{Input: "new"})
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if result.StopReason != StopReasonCompleted {
		t.Fatalf("StopReason = %s, want completed", result.StopReason)
	}
	if len(model.requests) != 3 {
		t.Fatalf("requests = %d, want failed turn, compaction, retry", len(model.requests))
	}
	if model.requests[0].Purpose != ModelRequestPurposeTurn ||
		model.requests[1].Purpose != ModelRequestPurposeCompaction ||
		model.requests[2].Purpose != ModelRequestPurposeTurn {
		t.Fatalf("request purposes = %s,%s,%s", model.requests[0].Purpose, model.requests[1].Purpose, model.requests[2].Purpose)
	}
}

func TestHardContextLimitStopsWithoutDroppingActiveMessage(t *testing.T) {
	model := &fakeModel{responses: []ModelResponse{{Content: "unused", FinishReason: FinishReasonStop}}}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: strings.Repeat("x", 500),
		Model:        model,
		Limits:       Limits{ContextWindowTokens: 50, ReserveOutputTokens: 10, HardInputLimitTokens: 20},
		Compaction:   CompactionConfig{KeepLastTurns: 1, TriggerRatio: 0.01},
	})

	result, err := a.Prompt(context.Background(), PromptRequest{Input: "new"})
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if result.StopReason != StopReasonContextLimit {
		t.Fatalf("StopReason = %s, want context_limit", result.StopReason)
	}
	if len(model.requests) != 0 {
		t.Fatalf("model calls = %d, want 0", len(model.requests))
	}
	if got := roles(a.Transcript()); got != "user" {
		t.Fatalf("transcript roles = %s, want active user kept", got)
	}
}

func TestCompactionFailureEmitsEventAndContinuesUntilHardLimit(t *testing.T) {
	events := &recordingEvents{}
	model := &fakeModel{
		responses: []ModelResponse{{Content: "answer", FinishReason: FinishReasonStop}},
		errs:      []error{errors.New("summarizer down")},
	}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: "x",
		Model:        model,
		Events:       events,
		Limits:       Limits{ContextWindowTokens: 1000, ReserveOutputTokens: 10},
		Compaction:   CompactionConfig{KeepLastTurns: 1, TriggerRatio: 0.01},
	})
	a.transcript = []Message{
		{Role: RoleUser, Content: strings.Repeat("older", 50)},
		{Role: RoleAssistant, Content: "older"},
		{Role: RoleUser, Content: strings.Repeat("old", 50)},
		{Role: RoleAssistant, Content: "old"},
	}

	_, err := a.Prompt(context.Background(), PromptRequest{Input: "new"})
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if !strings.Contains(eventTypes(events.events), string(EventTypeCompactionError)) {
		t.Fatalf("events = %s, want compaction_error", eventTypes(events.events))
	}
}

func TestCompactionCanRepeatDuringOneLongRun(t *testing.T) {
	model := &fakeModel{responses: []ModelResponse{
		{Content: "summary one", FinishReason: FinishReasonStop},
		{ToolCalls: []ToolCall{{ID: "call_1", Name: "noop", Arguments: json.RawMessage(`{}`)}}, FinishReason: FinishReasonToolCalls},
		{Content: "summary two", FinishReason: FinishReasonStop},
		{Content: "done", FinishReason: FinishReasonStop},
	}}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: "x",
		Model:        model,
		Limits:       Limits{ContextWindowTokens: 1000, ReserveOutputTokens: 10},
		Compaction:   CompactionConfig{KeepLastTurns: 1, TriggerRatio: 0.01},
		Tools: []ToolDefinition{{
			Name:        "noop",
			Description: "noop",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler:     noopTool(),
		}},
	})
	a.transcript = []Message{
		{Role: RoleUser, Content: "one"},
		{Role: RoleAssistant, Content: "one"},
		{Role: RoleUser, Content: "two"},
		{Role: RoleAssistant, Content: "two"},
	}

	result, err := a.Prompt(context.Background(), PromptRequest{Input: "three"})
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if result.StopReason != StopReasonCompleted {
		t.Fatalf("StopReason = %s, want completed", result.StopReason)
	}
	var compactions int
	for _, req := range model.requests {
		if req.Purpose == ModelRequestPurposeCompaction {
			compactions++
		}
	}
	if compactions != 2 {
		t.Fatalf("compactions = %d, want 2", compactions)
	}
	transcript := a.Transcript()
	if transcript[0].Role != RoleSummary || transcript[0].Content != "summary two" {
		t.Fatalf("first transcript message = %#v, want second summary", transcript[0])
	}
}

func TestHardInputLimitDoesNotDoubleCountReserve(t *testing.T) {
	model := &fakeModel{responses: []ModelResponse{{Content: "ok", FinishReason: FinishReasonStop}}}
	a := mustAgent(t, Definition{
		Name:         "test",
		SystemPrompt: strings.Repeat("x", 24),
		Model:        model,
		Limits:       Limits{ContextWindowTokens: 100, ReserveOutputTokens: 90, HardInputLimitTokens: 10},
		Compaction:   CompactionConfig{KeepLastTurns: 1, TriggerRatio: 1},
	})

	result, err := a.Prompt(context.Background(), PromptRequest{Input: "tiny"})
	if err != nil {
		t.Fatalf("Prompt returned error: %v", err)
	}
	if result.StopReason != StopReasonCompleted {
		t.Fatalf("StopReason = %s, want completed", result.StopReason)
	}
	if len(model.requests) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.requests))
	}
}

func containsToolResultFor(messages []Message, id string) bool {
	for _, message := range messages {
		if message.Role == RoleTool && message.ToolCallID == id {
			return true
		}
	}
	return false
}

func toolResultFollowsAssistant(messages []Message, id string) bool {
	for i, message := range messages {
		if message.Role != RoleAssistant {
			continue
		}
		for _, call := range message.ToolCalls {
			if call.ID == id {
				return i+1 < len(messages) && messages[i+1].Role == RoleTool && messages[i+1].ToolCallID == id
			}
		}
	}
	return false
}
