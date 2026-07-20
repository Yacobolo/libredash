package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	agentconfig "github.com/Yacobolo/leapview/internal/agent/config"
	agentcore "github.com/Yacobolo/leapview/pkg/agent"
)

var (
	ErrDisabled          = errors.New("agent is not configured")
	ErrBusy              = errors.New("agent conversation already has a running turn")
	ErrRunNotCancellable = errors.New("agent run is not cancellable")
)

const (
	maxToolArgumentsPreviewBytes = 2000
	maxToolResultPreviewBytes    = 4000
)

func IsBusy(err error) bool {
	return errors.Is(err, ErrBusy)
}

type Scope struct {
	WorkspaceID   string
	PrincipalID   string
	Credential    CredentialScope
	DevAuthBypass bool
}

type CredentialScope struct {
	WorkspaceID string
	Privileges  []string
	Restricted  bool
}

type ToolProvider func(scope Scope) []agentcore.ToolDefinition

type SystemPromptProvider func(ctx context.Context) (string, error)

type Service struct {
	metrics any
	repo    Repository
	config  Config
	model   agentcore.Model

	toolProviders        []ToolProvider
	systemPromptProvider SystemPromptProvider

	mu      sync.Mutex
	running map[string]runningPrompt
}

type runningPrompt struct {
	runID  string
	cancel context.CancelFunc
}

type ServiceOption func(*Service)

func WithModel(model agentcore.Model) ServiceOption {
	return func(s *Service) {
		s.model = model
	}
}

func NewService(metrics any, repo Repository, config Config, options ...ServiceOption) *Service {
	s := &Service{
		metrics: metrics,
		repo:    repo,
		config:  config,
		running: map[string]runningPrompt{},
	}
	for _, option := range options {
		option(s)
	}
	return s
}

func (s *Service) SetModel(model agentcore.Model) {
	s.model = model
}

func (s *Service) ConfigureDefaultModel(factory func(Config) agentcore.Model) {
	if s == nil || s.model != nil || factory == nil || !s.config.Enabled() {
		return
	}
	s.model = factory(s.config)
}

func (s *Service) SetToolProviders(providers ...ToolProvider) {
	s.toolProviders = append([]ToolProvider(nil), providers...)
}

func (s *Service) AppendToolProviders(providers ...ToolProvider) {
	s.toolProviders = append(s.toolProviders, providers...)
}

func (s *Service) SetSystemPromptProvider(provider SystemPromptProvider) {
	s.systemPromptProvider = provider
}

func (s *Service) Enabled() bool {
	return s != nil && s.config.Enabled()
}

func (s *Service) ConversationRunning(conversationID string) bool {
	if s == nil || strings.TrimSpace(conversationID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.running[conversationID]
	return ok
}

func (s *Service) Model() string {
	if s == nil {
		return ""
	}
	return s.config.Model
}

func (s *Service) CreateConversation(ctx context.Context, scope Scope, title string) (Conversation, error) {
	if s.repo == nil {
		return Conversation{}, fmt.Errorf("agent store is required")
	}
	return s.repo.CreateConversation(ctx, ConversationInput{
		PrincipalID:  scope.PrincipalID,
		Title:        title,
		MetadataJSON: `{}`,
	})
}

func (s *Service) ListConversations(ctx context.Context, scope Scope) ([]Conversation, error) {
	return s.repo.ListConversations(ctx, scope.PrincipalID)
}

func (s *Service) ListConversationsPage(ctx context.Context, scope Scope, page Page) ([]Conversation, error) {
	return s.repo.ListConversationsPage(ctx, scope.PrincipalID, normalizePage(page))
}

func (s *Service) GetConversation(ctx context.Context, scope Scope, conversationID string) (Conversation, error) {
	return s.repo.GetConversation(ctx, scope.PrincipalID, conversationID)
}

func (s *Service) UpdateConversation(ctx context.Context, scope Scope, conversationID, title string) (Conversation, error) {
	return s.repo.UpdateConversation(ctx, ConversationUpdate{
		PrincipalID:    scope.PrincipalID,
		ConversationID: conversationID,
		Title:          title,
	})
}

func (s *Service) ArchiveConversation(ctx context.Context, scope Scope, conversationID string) (Conversation, error) {
	return s.repo.ArchiveConversation(ctx, scope.PrincipalID, conversationID)
}

func (s *Service) ListMessages(ctx context.Context, scope Scope, conversationID string) ([]Message, error) {
	return s.repo.ListMessages(ctx, scope.PrincipalID, conversationID)
}

func (s *Service) ListMessagesPage(ctx context.Context, scope Scope, conversationID string, page Page) ([]Message, error) {
	return s.repo.ListMessagesPage(ctx, scope.PrincipalID, conversationID, normalizePage(page))
}

func (s *Service) ListRunsPage(ctx context.Context, scope Scope, conversationID string, page Page) ([]Run, error) {
	return s.repo.ListRunsPage(ctx, scope.PrincipalID, conversationID, normalizePage(page))
}

func (s *Service) GetRun(ctx context.Context, scope Scope, conversationID, runID string) (Run, error) {
	return s.repo.GetRun(ctx, scope.PrincipalID, conversationID, runID)
}

func (s *Service) CancelRun(ctx context.Context, scope Scope, conversationID, runID string) error {
	run, err := s.GetRun(ctx, scope, conversationID, runID)
	if err != nil {
		return err
	}
	if run.Status != RunStatusRunning {
		return ErrRunNotCancellable
	}
	s.mu.Lock()
	active, ok := s.running[conversationID]
	if !ok || active.runID != runID || active.cancel == nil {
		s.mu.Unlock()
		return ErrRunNotCancellable
	}
	cancel := active.cancel
	s.mu.Unlock()
	cancel()
	return nil
}

func (s *Service) CancelPersistedRun(ctx context.Context, scope Scope, conversationID, runID string) error {
	run, err := s.GetRun(ctx, scope, conversationID, runID)
	if err != nil {
		return err
	}
	if run.Status != RunStatusRunning {
		return ErrRunNotCancellable
	}
	s.release(conversationID)
	return s.finishRun(ctx, PromptInput{Scope: scope, ConversationID: conversationID}, runID, RunStatusCanceled, "", agentcore.Usage{}, context.Canceled)
}

func (s *Service) GetRunByID(ctx context.Context, scope Scope, runID string) (Run, error) {
	return s.repo.GetRunByID(ctx, scope.PrincipalID, runID)
}

func (s *Service) ListEvents(ctx context.Context, scope Scope, runID string) ([]Event, error) {
	return s.repo.ListEvents(ctx, scope.PrincipalID, runID)
}

func (s *Service) ListRunEventsPage(ctx context.Context, scope Scope, conversationID, runID string, page Page) ([]Event, error) {
	if _, err := s.repo.GetRun(ctx, scope.PrincipalID, conversationID, runID); err != nil {
		return nil, err
	}
	return s.repo.ListEventsPage(ctx, scope.PrincipalID, runID, normalizePage(page))
}

func (s *Service) ConversationEvents(ctx context.Context, scope Scope, conversationID string) ([]EventEnvelope, error) {
	if _, err := s.repo.GetConversation(ctx, scope.PrincipalID, conversationID); err != nil {
		return nil, err
	}
	messages, err := s.repo.ListMessages(ctx, scope.PrincipalID, conversationID)
	if err != nil {
		return nil, err
	}
	events := make([]EventEnvelope, 0, len(messages))
	for _, message := range messages {
		events = append(events, messageEnvelope(conversationID, message))
	}
	runs, err := s.repo.ListRuns(ctx, scope.PrincipalID, conversationID)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		runEvents, err := s.repo.ListEvents(ctx, scope.PrincipalID, run.ID)
		if err != nil {
			return nil, err
		}
		for _, event := range runEvents {
			events = append(events, eventEnvelope(conversationID, event))
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].CreatedAt == events[j].CreatedAt {
			return events[i].ID < events[j].ID
		}
		return events[i].CreatedAt < events[j].CreatedAt
	})
	return events, nil
}

func normalizePage(page Page) Page {
	if page.Limit <= 0 || page.Limit > 100 {
		page.Limit = 100
	}
	return page
}

func (s *Service) ConversationTranscript(ctx context.Context, scope Scope, conversationID string) ([]ChatTranscriptItem, error) {
	state, err := s.ConversationTranscriptState(ctx, scope, conversationID)
	if err != nil {
		return nil, err
	}
	return state.Transcript, nil
}

func (s *Service) ConversationTranscriptState(ctx context.Context, scope Scope, conversationID string) (ChatTranscriptState, error) {
	if _, err := s.repo.GetConversation(ctx, scope.PrincipalID, conversationID); err != nil {
		return ChatTranscriptState{}, err
	}
	messages, err := s.repo.ListMessages(ctx, scope.PrincipalID, conversationID)
	if err != nil {
		return ChatTranscriptState{}, err
	}
	return transcriptStateFromMessages(conversationID, messages), nil
}

func (s *Service) systemPrompt(ctx context.Context) (string, error) {
	if s != nil && s.systemPromptProvider != nil {
		prompt, err := s.systemPromptProvider(ctx)
		if err != nil {
			return "", err
		}
		return agentconfig.NormalizeSystemPrompt(prompt)
	}
	return agentconfig.DefaultSystemPrompt, nil
}
