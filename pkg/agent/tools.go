package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/sync/errgroup"
)

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     ToolHandler
}

type ToolHandler interface {
	Run(ctx context.Context, call ToolCall) (ToolResult, error)
}

type ToolHandlerFunc func(ctx context.Context, call ToolCall) (ToolResult, error)

func (f ToolHandlerFunc) Run(ctx context.Context, call ToolCall) (ToolResult, error) {
	return f(ctx, call)
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ToolResult struct {
	Content        any
	DisplayContent any
	IsError        bool
	Fatal          bool
}

type compiledTool struct {
	def    ToolDefinition
	schema *jsonschema.Schema
}

type toolExecutionResult struct {
	message Message
	fatal   error
}

func (a *Agent) executeToolCalls(ctx context.Context, run *runState, turnID string, calls []ToolCall) ([]Message, error) {
	results := make([]toolExecutionResult, len(calls))
	seen := map[string]struct{}{}
	valid := make([]int, 0, len(calls))
	for i, call := range calls {
		if call.ID == "" {
			results[i] = toolExecutionResult{message: toolErrorMessage(call, "invalid_tool_arguments", "Tool call ID is required.", nil, true)}
			continue
		}
		if _, ok := seen[call.ID]; ok {
			results[i] = toolExecutionResult{message: toolErrorMessage(call, "invalid_tool_arguments", "Tool call ID must be unique within an assistant message.", nil, true)}
			continue
		}
		seen[call.ID] = struct{}{}
		if errMsg, ok := a.validateToolCall(call); !ok {
			results[i] = toolExecutionResult{message: errMsg}
			continue
		}
		valid = append(valid, i)
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(a.def.Limits.MaxConcurrentTools)
	var mu sync.Mutex
	for _, index := range valid {
		index := index
		group.Go(func() error {
			call := calls[index]
			tool := a.tools[call.Name]
			_ = run.emit(groupCtx, Event{
				Type:       EventTypeToolStart,
				Severity:   SeverityInfo,
				TurnID:     turnID,
				ToolCallID: call.ID,
				ToolName:   call.Name,
			})
			result := a.runOneTool(groupCtx, call, tool)
			_ = run.emit(groupCtx, Event{
				Type:       EventTypeToolEnd,
				Severity:   eventSeverityForToolResult(result.message),
				TurnID:     turnID,
				ToolCallID: call.ID,
				ToolName:   call.Name,
			})
			mu.Lock()
			results[index] = result
			mu.Unlock()
			return nil
		})
	}
	_ = group.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	messages := make([]Message, len(results))
	for i, result := range results {
		messages[i] = result.message
		if result.fatal != nil {
			return messages, result.fatal
		}
	}
	return messages, nil
}

func eventSeverityForToolResult(message Message) Severity {
	if message.IsError {
		return SeverityWarn
	}
	return SeverityInfo
}

func (a *Agent) validateToolCall(call ToolCall) (Message, bool) {
	tool, ok := a.tools[call.Name]
	if !ok {
		return toolErrorMessage(call, "unknown_tool", fmt.Sprintf("Tool %q is not configured.", call.Name), nil, true), false
	}
	if !json.Valid(call.Arguments) {
		return toolErrorMessage(call, "invalid_tool_arguments", "Tool arguments must be valid JSON.", nil, true), false
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(call.Arguments))
	if err != nil {
		return toolErrorMessage(call, "invalid_tool_arguments", "Tool arguments must be valid JSON.", []string{err.Error()}, true), false
	}
	if err := tool.schema.Validate(instance); err != nil {
		return toolErrorMessage(call, "invalid_tool_arguments", "Tool arguments did not match the schema.", []string{err.Error()}, true), false
	}
	return Message{}, true
}

func (a *Agent) runOneTool(ctx context.Context, call ToolCall, tool *compiledTool) (result toolExecutionResult) {
	toolCtx, cancel := context.WithTimeout(ctx, a.def.Limits.ToolTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			result.message = toolErrorMessage(call, "tool_panic", "Tool handler panicked.", []string{fmt.Sprint(recovered), string(debug.Stack())}, false)
		}
	}()

	toolResult, err := tool.def.Handler.Run(toolCtx, call)
	if err != nil {
		var fatal fatalToolError
		if ctxErr := toolCtx.Err(); ctxErr != nil {
			result.message = toolErrorMessage(call, "tool_timeout", "Tool execution timed out or was canceled.", []string{ctxErr.Error()}, true)
			return result
		}
		result.message = toolErrorMessage(call, "tool_execution_failed", "Tool execution failed.", []string{err.Error()}, true)
		if isFatalToolError(err, &fatal) {
			result.fatal = NewError(ErrorCodeTool, "fatal tool error", fatal.err)
		}
		return result
	}
	if toolCtx.Err() != nil {
		result.message = toolErrorMessage(call, "tool_timeout", "Tool execution timed out or was canceled.", []string{toolCtx.Err().Error()}, true)
		return result
	}
	if toolResult.Content == nil {
		result.message = toolErrorMessage(call, "tool_result_invalid", "Tool returned no JSON-serializable result.", nil, false)
		return result
	}
	body, err := json.Marshal(toolResult.Content)
	if err != nil {
		result.message = toolErrorMessage(call, "tool_result_invalid", "Tool output was not JSON-serializable.", []string{err.Error()}, false)
		return result
	}
	if toolResult.DisplayContent != nil {
		displayBody, err := json.Marshal(toolResult.DisplayContent)
		if err != nil {
			result.message = toolErrorMessage(call, "tool_result_invalid", "Tool display output was not JSON-serializable.", []string{err.Error()}, false)
			return result
		}
		if len(displayBody) > a.def.Limits.MaxToolDisplayBytes {
			result.message = toolErrorMessage(call, "tool_display_output_too_large", "Tool display output exceeded the configured size limit.", nil, false)
			return result
		}
	}
	if len(body) > a.def.Limits.MaxToolResultBytes {
		result.message = toolErrorMessage(call, "tool_output_too_large", "Tool output exceeded the configured size limit.", nil, false)
		return result
	}
	result.message = Message{
		ID:             a.def.IDGenerator.NewID("msg"),
		Role:           RoleTool,
		Content:        string(body),
		DisplayContent: toolResult.DisplayContent,
		ToolCallID:     call.ID,
		ToolName:       call.Name,
		IsError:        toolResult.IsError,
	}
	if toolResult.Fatal {
		result.fatal = NewError(ErrorCodeTool, "fatal tool result", nil)
	}
	return result
}

func isFatalToolError(err error, target *fatalToolError) bool {
	if err == nil {
		return false
	}
	if v, ok := err.(fatalToolError); ok {
		*target = v
		return true
	}
	type unwrapper interface{ Unwrap() error }
	if wrapped, ok := err.(unwrapper); ok {
		return isFatalToolError(wrapped.Unwrap(), target)
	}
	return false
}

func toolErrorMessage(call ToolCall, code, message string, details []string, retryable bool) Message {
	payload := map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   message,
			"retryable": retryable,
		},
	}
	if len(details) > 0 {
		payload["error"].(map[string]any)["details"] = details
	}
	body, _ := json.Marshal(payload)
	return Message{
		Role:       RoleTool,
		Content:    string(body),
		ToolCallID: call.ID,
		ToolName:   call.Name,
		IsError:    true,
	}
}
