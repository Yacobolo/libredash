package agentapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/pkg/agent"
)

var (
	ErrDisabled = errors.New("agent is not configured")
	ErrBusy     = errors.New("agent conversation already has a running turn")
)

const (
	maxToolArgumentsPreviewBytes = 2000
	maxToolResultPreviewBytes    = 4000
)

func IsBusy(err error) bool {
	return errors.Is(err, ErrBusy)
}

type Scope struct {
	WorkspaceID string
	PrincipalID string
}

type Service struct {
	metrics Metrics
	store   *platform.Store
	config  Config
	model   agent.Model

	mu      sync.Mutex
	running map[string]struct{}
}

func NewService(metrics Metrics, store *platform.Store, config Config) *Service {
	return &Service{
		metrics: metrics,
		store:   store,
		config:  config,
		model:   NewOpenAIModel(config, http.DefaultClient),
		running: map[string]struct{}{},
	}
}

func (s *Service) Enabled() bool {
	return s != nil && s.config.Enabled()
}

func (s *Service) CreateConversation(ctx context.Context, scope Scope, title string) (platformdb.AgentConversation, error) {
	if s.store == nil {
		return platformdb.AgentConversation{}, fmt.Errorf("agent store is required")
	}
	return s.store.CreateAgentConversation(ctx, platform.AgentConversationInput{
		WorkspaceID:  scope.WorkspaceID,
		PrincipalID:  scope.PrincipalID,
		Title:        title,
		MetadataJSON: `{}`,
	})
}

func (s *Service) ListConversations(ctx context.Context, scope Scope) ([]platformdb.AgentConversation, error) {
	return s.store.ListAgentConversations(ctx, scope.WorkspaceID, scope.PrincipalID)
}

func (s *Service) ListMessages(ctx context.Context, scope Scope, conversationID string) ([]platformdb.AgentMessage, error) {
	return s.store.ListAgentMessages(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID)
}

func (s *Service) ListEvents(ctx context.Context, scope Scope, runID string) ([]platformdb.AgentEvent, error) {
	return s.store.ListAgentEvents(ctx, scope.WorkspaceID, scope.PrincipalID, runID)
}

func (s *Service) ConversationEvents(ctx context.Context, scope Scope, conversationID string) ([]api.AgentEventEnvelope, error) {
	if _, err := s.store.GetAgentConversation(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID); err != nil {
		return nil, err
	}
	messages, err := s.store.ListAgentMessages(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID)
	if err != nil {
		return nil, err
	}
	events := make([]api.AgentEventEnvelope, 0, len(messages))
	for _, message := range messages {
		events = append(events, messageEnvelope(conversationID, message))
	}
	runs, err := s.store.ListAgentRuns(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		runEvents, err := s.store.ListAgentEvents(ctx, scope.WorkspaceID, scope.PrincipalID, run.ID)
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

func (s *Service) ConversationTranscript(ctx context.Context, scope Scope, conversationID string) ([]api.AgentChatTranscriptItem, error) {
	if _, err := s.store.GetAgentConversation(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID); err != nil {
		return nil, err
	}
	messages, err := s.store.ListAgentMessages(ctx, scope.WorkspaceID, scope.PrincipalID, conversationID)
	if err != nil {
		return nil, err
	}
	return transcriptFromMessages(conversationID, messages), nil
}

func systemPrompt() string {
	return `You are LibreDash's read-only BI assistant. Answer using only the provided tools and conversation context. You can help users understand dashboards, semantic models, metric views, filters, visuals, and table snapshots they are allowed to access. Use progressive disclosure: start with compact summaries, then drill into specific pages, metric views, or tables only when needed. Do not invent dashboard IDs, metric names, or data values. You cannot write data, deploy changes, edit permissions, run raw SQL, access files, or call external services.`
}
