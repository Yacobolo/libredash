package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	agentcore "github.com/Yacobolo/libredash/pkg/agent"
)

type PromptInput struct {
	Scope          Scope
	ConversationID string
	Input          string
	CorrelationID  string
	OnEvent        func(EventEnvelope)
}

type PromptResult struct {
	ConversationID string               `json:"conversationId"`
	RunID          string               `json:"runId"`
	StopReason     agentcore.StopReason `json:"stopReason"`
	Content        string               `json:"content"`
}

type StartedPrompt struct {
	Scope          Scope
	ConversationID string
	RunID          string
	Input          string
	CorrelationID  string

	service      *Service
	systemPrompt string
	initial      []agentcore.Message
	runContext   context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	closed       bool
}

func (s *Service) Prompt(ctx context.Context, input PromptInput) (PromptResult, error) {
	started, err := s.StartPrompt(ctx, input)
	if err != nil {
		return PromptResult{}, err
	}
	return started.Complete(ctx, input.OnEvent)
}

func (s *Service) StartPrompt(ctx context.Context, input PromptInput) (*StartedPrompt, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if policy, ok := s.policyForScope(input.Scope); ok && !policy.Enabled {
		return nil, ErrPolicyDisabled
	}
	if s.repo == nil {
		return nil, fmt.Errorf("agent store is required")
	}
	if strings.TrimSpace(input.Input) == "" {
		return nil, fmt.Errorf("prompt input is required")
	}
	if err := s.acquire(input.ConversationID); err != nil {
		return nil, err
	}
	release := true
	defer func() {
		if release {
			s.release(input.ConversationID)
		}
	}()

	conversation, err := s.repo.GetConversation(ctx, input.Scope.WorkspaceID, input.Scope.PrincipalID, input.ConversationID)
	if err != nil {
		return nil, err
	}
	initial, err := decodeTranscript(conversation.TranscriptJSON)
	if err != nil {
		return nil, err
	}
	systemPrompt, err := s.systemPrompt(ctx)
	if err != nil {
		return nil, err
	}
	runID := newID("run")
	run, err := s.repo.CreateRun(ctx, RunInput{
		WorkspaceID:    input.Scope.WorkspaceID,
		PrincipalID:    input.Scope.PrincipalID,
		ConversationID: input.ConversationID,
		RunID:          runID,
		Model:          s.config.Model,
		MetadataJSON:   metadataJSON(map[string]any{"base_url": s.config.NormalizedBaseURL(), "model": s.config.Model}),
	})
	if err != nil {
		return nil, err
	}
	userMessage := agentcore.Message{
		ID:      newID("msg"),
		Role:    agentcore.RoleUser,
		Content: input.Input,
	}
	if err := s.appendMessage(ctx, PromptInput{
		Scope:          input.Scope,
		ConversationID: input.ConversationID,
	}, run.ID, userMessage); err != nil {
		_ = s.finishRun(ctx, input, run.ID, RunStatusFailed, "", agentcore.Usage{}, err)
		return nil, err
	}
	initial = append(initial, userMessage)
	if err := s.persistTranscript(ctx, input, initial); err != nil {
		_ = s.finishRun(ctx, input, run.ID, RunStatusFailed, "", agentcore.Usage{}, err)
		return nil, err
	}
	runContext, cancel := context.WithCancel(context.Background())
	s.attachRun(input.ConversationID, run.ID, cancel)
	release = false
	return &StartedPrompt{
		Scope:          input.Scope,
		ConversationID: input.ConversationID,
		RunID:          run.ID,
		Input:          input.Input,
		CorrelationID:  input.CorrelationID,
		service:        s,
		systemPrompt:   systemPrompt,
		initial:        initial,
		runContext:     runContext,
		cancel:         cancel,
	}, nil
}

// ResumePrompt reconstructs an already-persisted running prompt for a durable
// worker after process restart. StartPrompt persists the run, user message, and
// transcript before it returns, so no request body or in-memory closure is
// required to continue execution.
func (s *Service) ResumePrompt(ctx context.Context, scope Scope, conversationID, runID, correlationID string) (*StartedPrompt, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if policy, ok := s.policyForScope(scope); ok && !policy.Enabled {
		return nil, ErrPolicyDisabled
	}
	if s.repo == nil {
		return nil, fmt.Errorf("agent store is required")
	}
	conversationID, runID = strings.TrimSpace(conversationID), strings.TrimSpace(runID)
	if conversationID == "" || runID == "" {
		return nil, fmt.Errorf("conversation and run are required")
	}
	if err := s.acquireForResume(conversationID, runID); err != nil {
		return nil, err
	}
	release := true
	defer func() {
		if release {
			s.release(conversationID)
		}
	}()
	run, err := s.repo.GetRun(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID, runID)
	if err != nil {
		return nil, err
	}
	if run.Status != RunStatusRunning {
		return nil, fmt.Errorf("run %q is not resumable from status %q", runID, run.Status)
	}
	conversation, err := s.repo.GetConversation(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID)
	if err != nil {
		return nil, err
	}
	initial, err := decodeTranscript(conversation.TranscriptJSON)
	if err != nil {
		return nil, err
	}
	input := ""
	for index := len(initial) - 1; index >= 0; index-- {
		if initial[index].Role == agentcore.RoleUser {
			input = strings.TrimSpace(initial[index].Content)
			break
		}
	}
	if input == "" {
		return nil, fmt.Errorf("persisted run has no user prompt")
	}
	systemPrompt, err := s.systemPrompt(ctx)
	if err != nil {
		return nil, err
	}
	runContext, cancel := context.WithCancel(context.Background())
	s.attachRun(conversationID, runID, cancel)
	release = false
	return &StartedPrompt{Scope: scope, ConversationID: conversationID, RunID: runID, Input: input, CorrelationID: correlationID, service: s, systemPrompt: systemPrompt, initial: initial, runContext: runContext, cancel: cancel}, nil
}

func (s *Service) acquireForResume(conversationID, runID string) error {
	s.mu.Lock()
	if active, ok := s.running[conversationID]; ok {
		if active.runID != runID {
			s.mu.Unlock()
			return ErrBusy
		}
		if active.cancel != nil {
			active.cancel()
		}
		delete(s.running, conversationID)
	}
	s.mu.Unlock()
	return s.acquire(conversationID)
}

func (s *Service) CompletePrompt(ctx context.Context, started *StartedPrompt, onEvent func(EventEnvelope)) (PromptResult, error) {
	if started == nil {
		return PromptResult{}, fmt.Errorf("started prompt is required")
	}
	return started.Complete(ctx, onEvent)
}

func (p *StartedPrompt) Complete(ctx context.Context, onEvent func(EventEnvelope)) (PromptResult, error) {
	if err := p.claim(); err != nil {
		return PromptResult{}, err
	}
	defer p.release()
	executionContext := p.runContext
	if executionContext == nil {
		executionContext = ctx
	}
	if p.cancel != nil {
		stop := context.AfterFunc(ctx, p.cancel)
		defer stop()
	}
	s := p.service
	input := PromptInput{
		Scope:          p.Scope,
		ConversationID: p.ConversationID,
		Input:          p.Input,
		CorrelationID:  p.CorrelationID,
		OnEvent:        onEvent,
	}

	sink := &storeEventSink{repo: s.repo, scope: input.Scope, conversationID: input.ConversationID, runID: p.RunID, onEvent: input.OnEvent}
	def := agentcore.Definition{
		Name:              "libredash-readonly",
		SystemPrompt:      p.systemPrompt,
		Model:             s.model,
		Tools:             s.toolDefinitions(input.Scope),
		InitialTranscript: p.initial,
		Events:            sink,
		IDGenerator:       fixedRunIDGenerator{runID: p.RunID},
	}
	harness, err := agentcore.New(def)
	if err != nil {
		_ = s.finishRun(context.WithoutCancel(executionContext), input, p.RunID, RunStatusFailed, "", sink.usage, err)
		return PromptResult{}, err
	}
	result, promptErr := promptFromPersistedUser(executionContext, harness, input)
	transcript := harness.Transcript()
	if err := s.persistNewMessages(ctx, input, p.RunID, p.initial, transcript); err != nil && promptErr == nil {
		promptErr = err
	}
	if err := s.persistTranscript(ctx, input, transcript); err != nil && promptErr == nil {
		promptErr = err
	}
	status := RunStatusCompleted
	if promptErr != nil {
		status = RunStatusFailed
		if errors.Is(promptErr, context.Canceled) {
			status = RunStatusCanceled
		}
	}
	if err := s.finishRun(context.WithoutCancel(executionContext), input, p.RunID, status, result.StopReason, sink.usage, promptErr); err != nil && promptErr == nil {
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

func (p *StartedPrompt) Abort(ctx context.Context, runErr error) error {
	if p == nil {
		return nil
	}
	if err := p.claim(); err != nil {
		return nil
	}
	defer p.release()
	if runErr == nil {
		runErr = fmt.Errorf("prompt aborted")
	}
	input := PromptInput{
		Scope:          p.Scope,
		ConversationID: p.ConversationID,
		Input:          p.Input,
		CorrelationID:  p.CorrelationID,
	}
	return p.service.finishRun(ctx, input, p.RunID, RunStatusFailed, "", agentcore.Usage{}, runErr)
}

func (p *StartedPrompt) claim() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("started prompt is already closed")
	}
	p.closed = true
	return nil
}

func (p *StartedPrompt) release() {
	if p.service != nil {
		p.service.release(p.ConversationID)
	}
}

func promptFromPersistedUser(ctx context.Context, harness *agentcore.Agent, input PromptInput) (agentcore.RunResult, error) {
	return harness.Prompt(ctx, agentcore.PromptRequest{Input: input.Input, CorrelationID: input.CorrelationID, InputAlreadyAppended: true})
}

func (s *Service) acquire(conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.running[conversationID]; ok {
		return ErrBusy
	}
	s.running[conversationID] = runningPrompt{}
	return nil
}

func (s *Service) attachRun(conversationID, runID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.running[conversationID]; ok {
		s.running[conversationID] = runningPrompt{runID: runID, cancel: cancel}
	}
}

func (s *Service) release(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if active, ok := s.running[conversationID]; ok && active.cancel != nil {
		active.cancel()
	}
	delete(s.running, conversationID)
}

func (s *Service) persistNewMessages(ctx context.Context, input PromptInput, runID string, initial, transcript []agentcore.Message) error {
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

func (s *Service) appendMessage(ctx context.Context, input PromptInput, runID string, message agentcore.Message) error {
	if message.Role == agentcore.RoleSystem {
		return nil
	}
	row, err := s.repo.AppendMessage(ctx, MessageInput{
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

func (s *Service) persistTranscript(ctx context.Context, input PromptInput, transcript []agentcore.Message) error {
	bytes, err := json.Marshal(compactTranscriptForStorage(transcript))
	if err != nil {
		return err
	}
	_, err = s.repo.UpdateConversationTranscript(ctx, input.Scope.WorkspaceID, input.Scope.PrincipalID, input.ConversationID, string(bytes))
	return err
}

func compactTranscriptForStorage(transcript []agentcore.Message) []agentcore.Message {
	out := make([]agentcore.Message, len(transcript))
	for i, message := range transcript {
		message.DisplayContent = nil
		out[i] = message
	}
	return out
}

func (s *Service) finishRun(ctx context.Context, input PromptInput, runID, status string, stop agentcore.StopReason, usage agentcore.Usage, runErr error) error {
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	_, err := s.repo.FinishRun(ctx, RunFinish{
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

func decodeTranscript(raw string) ([]agentcore.Message, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var messages []agentcore.Message
	if err := json.Unmarshal([]byte(raw), &messages); err != nil {
		return nil, err
	}
	return messages, nil
}
