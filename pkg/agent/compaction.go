package agent

import (
	"context"
	"strings"
)

type CompactionConfig struct {
	KeepLastTurns int
	TriggerRatio  float64
	SystemPrompt  string
}

func defaultCompaction(c CompactionConfig) CompactionConfig {
	if c.KeepLastTurns == 0 {
		c.KeepLastTurns = 8
	}
	if c.TriggerRatio == 0 {
		c.TriggerRatio = 0.70
	}
	if c.SystemPrompt == "" {
		c.SystemPrompt = defaultSummaryPrompt
	}
	return c
}

const defaultSummaryPrompt = `Summarize the older conversation for future agent turns. Preserve user goals, decisions, relevant tool results, pending tasks, IDs, names, paths, entities, important failures, and corrections. Exclude irrelevant small talk and duplicate raw tool output.`

func (a *Agent) maybeCompact(ctx context.Context, run *runState, force bool) error {
	if !force && !a.shouldCompact() {
		return nil
	}

	summary, older, keep := a.compactionParts()
	if len(older) == 0 {
		return nil
	}

	_ = run.emit(ctx, Event{Type: EventTypeCompactionStart, Severity: SeverityInfo})
	contextBearingMessages := older
	if summary.Content != "" {
		contextBearingMessages = append([]Message{summary}, older...)
	}
	req := ModelRequest{
		Purpose:       ModelRequestPurposeCompaction,
		RunID:         run.runID,
		CorrelationID: run.correlationID,
		SystemPrompt:  promptWithExternalContextGuidance(a.def.Compaction.SystemPrompt, contextBearingMessages),
		Messages:      a.compactionMessages(summary, older),
		Tools:         nil,
		Limits:        a.def.Limits,
	}
	resp, err := a.def.Model.Complete(ctx, req, noopModelStream{})
	if err != nil {
		_ = run.emit(ctx, Event{Type: EventTypeCompactionError, Severity: SeverityWarn, Error: agentErrorPtr(ErrorCodeCompaction, "compaction failed", err)})
		return nil
	}
	if strings.TrimSpace(resp.Content) == "" {
		_ = run.emit(ctx, Event{Type: EventTypeCompactionError, Severity: SeverityWarn, Error: agentErrorPtr(ErrorCodeCompaction, "compaction returned empty summary", nil)})
		return nil
	}

	summaryKind := MessageKind("")
	if hasExternalContext(older) || summary.Kind == messageKindExternalContextSummary {
		summaryKind = messageKindExternalContextSummary
	}
	next := []Message{{
		ID:      a.def.IDGenerator.NewID("msg"),
		Role:    RoleSummary,
		Kind:    summaryKind,
		Content: resp.Content,
	}}
	next = append(next, keep...)
	a.mu.Lock()
	a.transcript = next
	a.mu.Unlock()
	_ = run.emit(ctx, Event{Type: EventTypeCompactionEnd, Severity: SeverityInfo, Usage: resp.Usage})
	return nil
}

func (a *Agent) shouldCompact() bool {
	estimate := a.estimateModelRequestTokens(a.snapshotTranscript())
	threshold := int(float64(a.def.Limits.ContextWindowTokens) * a.def.Compaction.TriggerRatio)
	return estimate >= threshold
}

func (a *Agent) compactionParts() (summary Message, older []Message, keep []Message) {
	transcript := a.snapshotTranscript()
	if len(transcript) > 0 && transcript[0].Role == RoleSummary {
		summary = transcript[0]
		transcript = transcript[1:]
	}
	turns := splitCompleteTurns(transcript)
	var active []Message
	if len(turns) > 0 && !turnHasAssistant(turns[len(turns)-1]) {
		active = turns[len(turns)-1]
		turns = turns[:len(turns)-1]
	}
	if len(turns) <= a.def.Compaction.KeepLastTurns {
		return summary, nil, transcript
	}
	cut := len(turns) - a.def.Compaction.KeepLastTurns
	for _, turn := range turns[:cut] {
		older = append(older, turn...)
	}
	for _, turn := range turns[cut:] {
		keep = append(keep, turn...)
	}
	keep = append(keep, active...)
	return summary, older, keep
}

func splitCompleteTurns(messages []Message) [][]Message {
	var turns [][]Message
	var current []Message
	for _, message := range messages {
		if message.Kind == MessageKindExternalContext && turnHasAssistant(current) {
			turns = append(turns, current)
			current = nil
		}
		if message.Role == RoleUser && message.Kind != MessageKindExternalContext && turnHasVisibleUser(current) {
			turns = append(turns, current)
			current = nil
		}
		current = append(current, message)
	}
	if len(current) > 0 {
		turns = append(turns, current)
	}
	return turns
}

func turnHasVisibleUser(messages []Message) bool {
	for _, message := range messages {
		if message.Role == RoleUser && message.Kind != MessageKindExternalContext {
			return true
		}
	}
	return false
}

func turnHasAssistant(messages []Message) bool {
	for _, message := range messages {
		if message.Role == RoleAssistant {
			return true
		}
	}
	return false
}

func (a *Agent) compactionMessages(summary Message, older []Message) []Message {
	contextBearingMessages := older
	if summary.Content != "" {
		contextBearingMessages = append([]Message{summary}, older...)
	}
	messages := []Message{{Role: RoleSystem, Content: promptWithExternalContextGuidance(a.def.Compaction.SystemPrompt, contextBearingMessages)}}
	if summary.Content != "" {
		if summary.Kind == messageKindExternalContextSummary {
			messages = append(messages, Message{Role: RoleUser, Kind: MessageKindExternalContext, Content: "<external_context_summary>\n" + summary.Content + "\n</external_context_summary>"})
		} else {
			messages = append(messages, Message{Role: RoleSystem, Content: "Existing summary:\n" + summary.Content})
		}
	}
	for _, message := range older {
		message.DisplayContent = nil
		messages = append(messages, message)
	}
	return cloneMessages(messages)
}

func agentErrorPtr(code ErrorCode, message string, err error) *AgentError {
	return &AgentError{Code: code, Message: message, Err: err}
}
