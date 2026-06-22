package app

import (
	"context"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/starfederation/datastar-go/datastar"
)

type chatSignals struct {
	Agent api.AgentChatSignal `json:"agent"`
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	scope := s.chatScope(r)
	signal := s.chatSignal(r.Context(), scope, "", "", false)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ChatPage(s.metrics.Catalog(), csrfToken(r, s.auth), s.currentRoleLabel(r), signal).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) chatTurn(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.chatService(w, r)
	if !ok {
		return
	}
	signals := chatSignals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	input := strings.TrimSpace(signals.Agent.Composer.Value)
	if input == "" {
		http.Error(w, "input is required", http.StatusBadRequest)
		return
	}
	conversationID := strings.TrimSpace(signals.Agent.ActiveConversationID)
	if conversationID == "" {
		conversation, err := service.CreateConversation(r.Context(), scope, "New conversation")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		conversationID = conversation.ID
	}

	sse := datastar.NewSSE(w, r)
	events := append([]api.AgentEventEnvelope{}, signals.Agent.Events...)
	pending := api.AgentEventEnvelope{
		ID:             "pending:user",
		ConversationID: conversationID,
		Type:           "message_appended",
		Severity:       "info",
		Payload: map[string]any{"message": map[string]any{
			"role":    platform.AgentMessageRoleUser,
			"content": input,
		}},
	}
	events = append(events, pending)
	_ = sse.MarshalAndPatchSignals(map[string]any{"agent": s.chatSignalWith(scope, conversationID, events, "", true)})

	emit := func(event api.AgentEventEnvelope) {
		events = append(events, event)
		_ = sse.MarshalAndPatchSignals(map[string]any{"agent": s.chatSignalWith(scope, conversationID, events, "", true)})
	}
	result, err := service.Prompt(r.Context(), agentapp.PromptInput{
		Scope:          scope,
		ConversationID: conversationID,
		Input:          input,
		OnEvent:        emit,
	})
	statusErr := ""
	if err != nil {
		statusErr = err.Error()
		if agentapp.IsBusy(err) {
			statusErr = "A turn is already running for this conversation."
		}
	}
	if result.RunID != "" {
		if refreshed, refreshErr := service.ConversationEvents(r.Context(), scope, conversationID); refreshErr == nil {
			events = refreshed
		}
	}
	_ = sse.MarshalAndPatchSignals(map[string]any{"agent": s.chatSignalWith(scope, conversationID, events, statusErr, false)})
}

func (s *Server) chatSelectConversation(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.chatService(w, r)
	if !ok {
		return
	}
	signals := chatSignals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conversationID := strings.TrimSpace(signals.Agent.ActiveConversationID)
	if conversationID == "" {
		http.Error(w, "conversation id is required", http.StatusBadRequest)
		return
	}
	events, err := service.ConversationEvents(r.Context(), scope, conversationID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	sse := datastar.NewSSE(w, r)
	_ = sse.MarshalAndPatchSignals(map[string]any{"agent": s.chatSignalWith(scope, conversationID, events, "", false)})
}

func (s *Server) chatService(w http.ResponseWriter, r *http.Request) (*agentapp.Service, agentapp.Scope, bool) {
	if s.agent == nil || !s.agent.Enabled() {
		http.Error(w, agentapp.ErrDisabled.Error(), http.StatusServiceUnavailable)
		return nil, agentapp.Scope{}, false
	}
	scope := s.chatScope(r)
	if scope.PrincipalID == "" {
		http.Error(w, "chat requires an authenticated principal", http.StatusUnauthorized)
		return nil, agentapp.Scope{}, false
	}
	return s.agent, scope, true
}

func (s *Server) chatScope(r *http.Request) agentapp.Scope {
	principalID := ""
	if s.auth != nil {
		if principal, ok := s.auth.Principal(r); ok {
			principalID = principal.ID
			if principal.DevBypass && s.store != nil {
				_, _ = s.store.UpsertPrincipal(r.Context(), platform.PrincipalInput{ID: principal.ID, Email: principal.Email, DisplayName: principal.DisplayName})
			}
		}
	}
	return agentapp.Scope{WorkspaceID: s.workspaceID(""), PrincipalID: principalID}
}

func (s *Server) chatSignal(ctx context.Context, scope agentapp.Scope, activeID, statusErr string, running bool) api.AgentChatSignal {
	events := []api.AgentEventEnvelope{}
	if activeID != "" && s.agent != nil && scope.PrincipalID != "" {
		if loaded, err := s.agent.ConversationEvents(ctx, scope, activeID); err == nil {
			events = loaded
		}
	}
	return s.chatSignalWith(scope, activeID, events, statusErr, running)
}

func (s *Server) chatSignalWith(scope agentapp.Scope, activeID string, events []api.AgentEventEnvelope, statusErr string, running bool) api.AgentChatSignal {
	conversations := []api.AgentConversationResponse{}
	if s.agent != nil && scope.PrincipalID != "" {
		if rows, err := s.agent.ListConversations(context.Background(), scope); err == nil {
			for _, row := range rows {
				conversations = append(conversations, agentConversationDTO(row))
			}
		}
	}
	enabled := s.agent != nil && s.agent.Enabled()
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	return api.AgentChatSignal{
		Conversations:        conversations,
		ActiveConversationID: activeID,
		Events:               events,
		Status: api.AgentChatStatus{
			Enabled: enabled,
			Running: running,
			Error:   statusErr,
		},
		Composer: api.AgentComposerSignal{
			Value:       "",
			Disabled:    !enabled || running,
			Placeholder: chatPlaceholder(enabled, running),
		},
	}
}

func chatPlaceholder(enabled, running bool) string {
	if !enabled {
		return "Agent is not configured"
	}
	if running {
		return "Waiting for the current answer..."
	}
	return "Ask about dashboards, metrics, or models..."
}
