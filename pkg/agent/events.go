package agent

import (
	"context"
	"sync/atomic"
	"time"
)

type EventSink interface {
	// Emit receives best-effort lifecycle events. Returned errors are ignored by
	// the harness so observability sinks cannot break agent execution.
	Emit(ctx context.Context, event Event) error
}

type EventSinkFunc func(ctx context.Context, event Event) error

func (f EventSinkFunc) Emit(ctx context.Context, event Event) error {
	return f(ctx, event)
}

type noopEventSink struct{}

func (noopEventSink) Emit(context.Context, Event) error { return nil }

type EventType string

const (
	EventTypeAgentStart      EventType = "agent_start"
	EventTypeAgentEnd        EventType = "agent_end"
	EventTypeTurnStart       EventType = "turn_start"
	EventTypeTurnEnd         EventType = "turn_end"
	EventTypeModelRequest    EventType = "model_request"
	EventTypeModelResponse   EventType = "model_response"
	EventTypeModelRetry      EventType = "model_retry"
	EventTypeMessageDelta    EventType = "message_delta"
	EventTypeMessageEnd      EventType = "message_end"
	EventTypeToolStart       EventType = "tool_start"
	EventTypeToolEnd         EventType = "tool_end"
	EventTypeCompactionStart EventType = "compaction_start"
	EventTypeCompactionEnd   EventType = "compaction_end"
	EventTypeCompactionError EventType = "compaction_error"
	EventTypeError           EventType = "error"
	EventTypeAbort           EventType = "abort"
)

type Severity string

const (
	SeverityDebug Severity = "debug"
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Event struct {
	Type          EventType
	Severity      Severity
	Time          time.Time
	Sequence      int64
	RunID         string
	TurnID        string
	MessageID     string
	ToolCallID    string
	ToolName      string
	CorrelationID string
	Delta         string
	StopReason    StopReason
	FinishReason  FinishReason
	Error         *AgentError
	Usage         Usage
	Provider      string
	Model         string

	ProviderMetadata map[string]any
}

type runState struct {
	agent         *Agent
	runID         string
	correlationID string
	seq           atomic.Int64
}

func (r *runState) emit(ctx context.Context, event Event) error {
	if event.Severity == "" {
		event.Severity = SeverityInfo
	}
	event.RunID = r.runID
	event.CorrelationID = r.correlationID
	event.Sequence = r.seq.Add(1)
	event.Time = r.agent.def.Clock.Now()
	return r.agent.def.Events.Emit(ctx, event)
}

type eventModelStream struct {
	run    *runState
	turnID string
}

func (s eventModelStream) Delta(ctx context.Context, text string) error {
	return s.run.emit(ctx, Event{
		Type:     EventTypeMessageDelta,
		Severity: SeverityInfo,
		TurnID:   s.turnID,
		Delta:    text,
	})
}
