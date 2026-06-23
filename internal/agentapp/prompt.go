package agentapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/pkg/agent"
)

type PromptInput struct {
	Scope          Scope
	ConversationID string
	Input          string
	CorrelationID  string
	OnEvent        func(api.AgentEventEnvelope)
}

type PromptResult struct {
	ConversationID string           `json:"conversationId"`
	RunID          string           `json:"runId"`
	StopReason     agent.StopReason `json:"stopReason"`
	Content        string           `json:"content"`
}

func (s *Service) Prompt(ctx context.Context, input PromptInput) (PromptResult, error) {
	if !s.Enabled() {
		return PromptResult{}, ErrDisabled
	}
	if s.store == nil {
		return PromptResult{}, fmt.Errorf("agent store is required")
	}
	if err := s.acquire(input.ConversationID); err != nil {
		return PromptResult{}, err
	}
	defer s.release(input.ConversationID)

	conversation, err := s.store.GetAgentConversation(ctx, input.Scope.WorkspaceID, input.Scope.PrincipalID, input.ConversationID)
	if err != nil {
		return PromptResult{}, err
	}
	initial, err := decodeTranscript(conversation.TranscriptJson)
	if err != nil {
		return PromptResult{}, err
	}
	runID := newID("run")
	run, err := s.store.CreateAgentRun(ctx, platform.AgentRunInput{
		WorkspaceID:    input.Scope.WorkspaceID,
		PrincipalID:    input.Scope.PrincipalID,
		ConversationID: input.ConversationID,
		RunID:          runID,
		Model:          s.config.Model,
		MetadataJSON:   metadataJSON(map[string]any{"base_url": s.config.normalizedBaseURL(), "model": s.config.Model}),
	})
	if err != nil {
		return PromptResult{}, err
	}
	sink := &storeEventSink{store: s.store, scope: input.Scope, conversationID: input.ConversationID, runID: run.ID, onEvent: input.OnEvent}
	def := agent.Definition{
		Name:              "libredash-readonly",
		SystemPrompt:      systemPrompt(),
		Model:             s.model,
		Tools:             s.toolDefinitions(input.Scope),
		InitialTranscript: initial,
		Events:            sink,
		IDGenerator:       fixedRunIDGenerator{runID: run.ID},
	}
	harness, err := agent.New(def)
	if err != nil {
		_ = s.finishRun(ctx, input, run.ID, platform.AgentRunStatusFailed, "", sink.usage, err)
		return PromptResult{}, err
	}
	result, promptErr := harness.Prompt(ctx, agent.PromptRequest{Input: input.Input, CorrelationID: input.CorrelationID})
	transcript := harness.Transcript()
	if err := s.persistNewMessages(ctx, input, run.ID, initial, transcript); err != nil && promptErr == nil {
		promptErr = err
	}
	if err := s.persistTranscript(ctx, input, transcript); err != nil && promptErr == nil {
		promptErr = err
	}
	status := platform.AgentRunStatusCompleted
	if promptErr != nil {
		status = platform.AgentRunStatusFailed
		if errors.Is(promptErr, context.Canceled) {
			status = platform.AgentRunStatusCanceled
		}
	}
	if err := s.finishRun(ctx, input, run.ID, status, result.StopReason, sink.usage, promptErr); err != nil && promptErr == nil {
		promptErr = err
	}
	if promptErr != nil {
		return PromptResult{}, promptErr
	}
	return PromptResult{
		ConversationID: input.ConversationID,
		RunID:          result.RunID,
		StopReason:     result.StopReason,
		Content:        result.FinalMessage.Content,
	}, nil
}

func (s *Service) acquire(conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.running[conversationID]; ok {
		return ErrBusy
	}
	s.running[conversationID] = struct{}{}
	return nil
}

func (s *Service) release(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, conversationID)
}

func (s *Service) persistNewMessages(ctx context.Context, input PromptInput, runID string, initial, transcript []agent.Message) error {
	seen := map[string]struct{}{}
	for _, message := range initial {
		if message.ID != "" {
			seen[message.ID] = struct{}{}
		}
	}
	for _, message := range transcript {
		if message.ID != "" {
			if _, ok := seen[message.ID]; ok {
				continue
			}
			seen[message.ID] = struct{}{}
		}
		if err := s.appendMessage(ctx, input, runID, message); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) appendMessage(ctx context.Context, input PromptInput, runID string, message agent.Message) error {
	if message.Role == agent.RoleSystem {
		return nil
	}
	row, err := s.store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    input.Scope.WorkspaceID,
		PrincipalID:    input.Scope.PrincipalID,
		ConversationID: input.ConversationID,
		RunID:          runID,
		Role:           platformRole(message.Role),
		ContentText:    message.Content,
		ContentJSON:    messageContentJSON(message),
		ToolCallID:     message.ToolCallID,
		ToolName:       message.ToolName,
		IsError:        message.IsError,
	})
	if err == nil && input.OnEvent != nil {
		input.OnEvent(messageEnvelope(input.ConversationID, row))
	}
	return err
}

func (s *Service) persistTranscript(ctx context.Context, input PromptInput, transcript []agent.Message) error {
	bytes, err := json.Marshal(transcript)
	if err != nil {
		return err
	}
	_, err = s.store.UpdateAgentConversationTranscript(ctx, input.Scope.WorkspaceID, input.Scope.PrincipalID, input.ConversationID, string(bytes))
	return err
}

func (s *Service) finishRun(ctx context.Context, input PromptInput, runID, status string, stop agent.StopReason, usage agent.Usage, runErr error) error {
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	_, err := s.store.FinishAgentRun(ctx, platform.AgentRunFinish{
		WorkspaceID:    input.Scope.WorkspaceID,
		PrincipalID:    input.Scope.PrincipalID,
		ConversationID: input.ConversationID,
		RunID:          runID,
		Status:         status,
		StopReason:     string(stop),
		InputTokens:    int64(usage.InputTokens),
		OutputTokens:   int64(usage.OutputTokens),
		TotalTokens:    int64(usage.TotalTokens),
		Error:          errText,
		MetadataJSON:   metadataJSON(map[string]any{"model": s.config.Model}),
	})
	return err
}

type fixedRunIDGenerator struct {
	runID string
}

func (g fixedRunIDGenerator) NewID(prefix string) string {
	if prefix == "run" {
		return g.runID
	}
	return newID(prefix)
}

func decodeTranscript(raw string) ([]agent.Message, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var messages []agent.Message
	if err := json.Unmarshal([]byte(raw), &messages); err != nil {
		return nil, err
	}
	return messages, nil
}
