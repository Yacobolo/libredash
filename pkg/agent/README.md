# pkg/agent

`pkg/agent` is a small embedded agent harness for Go applications. It owns the
agent loop, transcript state, tool-call validation, bounded parallel tool
execution, automatic compaction, cancellation, and lifecycle events.

It intentionally does not ship built-in tools or a concrete model provider. The
host application supplies an OpenAI-compatible model adapter and tailor-made
service tools.

## When To Use It

Use this package when an application already owns:

- domain services and permissions
- curated actions the model may call
- UI or API routes for user prompts
- persistence or audit history, if needed

Do not use it as a standalone agent platform. The harness has no filesystem,
shell, browser, SQL, HTTP, MCP, or LibreDash-specific tools.

## Basic Flow

1. Implement `agent.Model` with an OpenAI-compatible provider adapter.
2. Define tools as Go structs with JSON Schema input schemas and handlers.
3. Construct one `agent.Agent` from `agent.Definition`.
4. Call `Prompt(ctx, agent.PromptRequest{Input: ...})`.
5. Stream or persist lifecycle events from `EventSink`.

## Minimal Example

```go
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Yacobolo/libredash/pkg/agent"
)

type DashboardService interface {
	ListDashboards(ctx context.Context) ([]string, error)
}

func NewWorkspaceAgent(service DashboardService, model agent.Model) (*agent.Agent, error) {
	return agent.New(agent.Definition{
		Name: "workspace-assistant",
		SystemPrompt: `You help users understand and operate this BI workspace.
Use tools for workspace facts. Do not invent dashboard IDs.`,
		Model:  model,
		Events: agent.EventSinkFunc(logAgentEvent),
		Tools: []agent.ToolDefinition{
			{
				Name:        "list_dashboards",
				Description: "List dashboards available in the current workspace.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {},
					"additionalProperties": false
				}`),
				Handler: agent.ToolHandlerFunc(func(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
					dashboards, err := service.ListDashboards(ctx)
					if err != nil {
						return agent.ToolResult{}, err
					}
					return agent.ToolResult{
						Content: map[string]any{"dashboards": dashboards},
					}, nil
				}),
			},
		},
	})
}

func Ask(ctx context.Context, a *agent.Agent, input string) (string, error) {
	result, err := a.Prompt(ctx, agent.PromptRequest{
		Input:         input,
		CorrelationID: "request-id-from-your-app",
	})
	if err != nil {
		return "", err
	}
	if result.StopReason != agent.StopReasonCompleted {
		return "", fmt.Errorf("agent stopped: %s", result.StopReason)
	}
	return result.FinalMessage.Content, nil
}

func logAgentEvent(ctx context.Context, event agent.Event) error {
	log.Printf("agent event type=%s run=%s turn=%s stop=%s", event.Type, event.RunID, event.TurnID, event.StopReason)
	return nil
}
```

## Model Adapter

`pkg/agent` only defines the model interface:

```go
type Model interface {
	Complete(ctx context.Context, req ModelRequest, stream ModelStream) (ModelResponse, error)
}
```

Your adapter should translate `ModelRequest` into the provider's
OpenAI-compatible chat/tool-call request shape, then translate the response back
into `ModelResponse`.

Important adapter behavior:

- Use `req.Purpose` to distinguish normal turns from compaction.
- For `ModelRequestPurposeTurn`, call `stream.Delta(ctx, text)` as provider
  deltas arrive.
- For `ModelRequestPurposeCompaction`, do not emit normal assistant deltas.
- Pass `req.Tools` to turn requests; compaction requests receive no tools.
- Normalize provider finish reasons where possible. The harness also normalizes
  common values such as `length` to `truncated`.
- Return `agent.ErrContextLength` or an `agent.ErrorCodeLimit` error when the
  provider rejects a request for context length so the harness can compact and
  retry once.

Provider SDKs such as `github.com/openai/openai-go` belong in the embedding
application or an adapter package, not in this core package.

## Tools

Tools are declarative specs plus host-owned handlers:

```go
agent.ToolDefinition{
	Name:        "describe_model",
	Description: "Describe a semantic model by ID.",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"model_id": {"type": "string"}
		},
		"required": ["model_id"],
		"additionalProperties": false
	}`),
	Handler: agent.ToolHandlerFunc(func(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
		var input struct {
			ModelID string `json:"model_id"`
		}
		if err := json.Unmarshal(call.Arguments, &input); err != nil {
			return agent.ToolResult{}, err
		}

		// Call application services here.
		return agent.ToolResult{
			Content: map[string]any{
				"model_id": input.ModelID,
				"tables":   []string{"orders", "customers"},
			},
		}, nil
	}),
}
```

The harness validates tool calls before handlers run:

- unknown tools
- missing or duplicate tool-call IDs
- malformed JSON arguments
- JSON Schema failures
- non-serializable outputs
- outputs that remain overlarge after truncation

Successful tool results are normalized, truncated, and serialized as TOON by
default before they are sent back to the model. Set
`Definition.ToolOutput.Format` to `ToolOutputJSON` only when an embedding
application needs JSON model-visible tool results. Validation and ordinary
handler failures use the same formatter and become model-visible tool-result
messages, allowing the model to repair the next call. Fatal tool outcomes stop
the run after appending the tool result:

```go
return agent.ToolResult{
	Content: map[string]any{"draft_id": draft.ID},
	Fatal:   true,
}, nil
```

or:

```go
return agent.ToolResult{}, agent.FatalToolError(err)
```

## Events

Events are best-effort lifecycle notifications. `EventSink.Emit` errors are
ignored by the harness so UI, logging, or tracing failures do not break agent
execution.

Useful event types include:

- `agent_start`, `agent_end`
- `turn_start`, `turn_end`
- `model_request`, `model_response`, `model_retry`
- `message_delta`, `message_end`
- `tool_start`, `tool_end`
- `compaction_start`, `compaction_end`, `compaction_error`
- `error`, `abort`

Every event includes run ID, sequence number, timestamp, severity, and optional
correlation ID. Turn-scoped events include turn ID. Model lifecycle events can
include provider metadata supplied by the adapter.

## Compaction

Compaction is always enabled in V1. The harness keeps the last configured
complete turns verbatim and summarizes older turns with the same configured
model using `ModelRequestPurposeCompaction`.

Defaults:

- keep last 8 complete turns
- compact when estimated request size reaches 70% of the context window
- reserve 4096 output tokens

A complete turn is a user message, the following assistant message, and any tool
results produced by that assistant message. Compaction does not split tool calls
from their results.

## Limits

Default limits are intentionally conservative:

- `MaxTurns`: 16
- `MaxToolCalls`: 64 per run
- `MaxConcurrentTools`: 4 per assistant turn
- `ToolTimeout`: 30 seconds
- `MaxToolResultBytes`: 64 KiB after tool-output formatting and truncation
- `ContextWindowTokens`: 128000
- `ReserveOutputTokens`: 4096

Set limits in `agent.Definition` when constructing the harness.

Default tool-output policy:

- `Format`: `ToolOutputTOON`
- `MaxStringChars`: 2000
- `MaxArrayItems`: 50
- `MaxObjectDepth`: 8

## Boundaries

Keep these outside `pkg/agent`:

- OpenAI, DeepSeek, or gateway SDK configuration
- API keys, auth, rate limiting, and billing policy
- LibreDash-specific tools and prompts
- durable transcript persistence
- approval workflows
- raw filesystem, shell, browser, SQL, or network tools
