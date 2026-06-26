package app

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/Yacobolo/libredash/internal/agentapp"
	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/starfederation/datastar-go/datastar"
)

type chatTurnCommandSignals struct {
	Agent chatTurnCommandAgentSignal `json:"agent"`
}

type chatTurnCommandAgentSignal struct {
	ActiveConversationID string                        `json:"activeConversationId"`
	Composer             chatTurnCommandComposerSignal `json:"composer"`
}

type chatTurnCommandComposerSignal struct {
	Value string `json:"value"`
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	scope := s.chatScope(r)
	if s.agent == nil || !s.agent.Enabled() || scope.PrincipalID == "" {
		s.renderChat(w, r, s.chatSignal(r.Context(), scope, "", "", false))
		return
	}
	conversations, err := s.agent.ListConversations(r.Context(), scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(conversations) == 0 {
		http.Redirect(w, r, "/chat/new", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/chat/"+conversations[0].ID, http.StatusFound)
}

func (s *Server) chatNew(w http.ResponseWriter, r *http.Request) {
	scope := s.chatScope(r)
	s.renderChat(w, r, s.chatSignal(r.Context(), scope, "", "", false))
}

func (s *Server) chatConversation(w http.ResponseWriter, r *http.Request) {
	scope := s.chatScope(r)
	conversationID := strings.TrimSpace(chi.URLParam(r, "conversation"))
	if s.agent == nil || !s.agent.Enabled() {
		s.renderChat(w, r, s.chatSignal(r.Context(), scope, "", "", false))
		return
	}
	if scope.PrincipalID == "" {
		http.Error(w, "chat requires an authenticated principal", http.StatusUnauthorized)
		return
	}
	transcript, err := s.agent.ConversationTranscript(r.Context(), scope, conversationID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	s.queueMissingChatTitle(r.Context(), scope, conversationID, chatClientID(r))
	s.renderChat(w, r, s.chatSignalWith(r.Context(), scope, conversationID, transcript, "", false))
}

func (s *Server) renderChat(w http.ResponseWriter, r *http.Request, signal ui.ChatSignal) {
	_ = lddatastar.EnsureClientID(w, r)
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
	clientID := chatClientID(r)
	signals := chatTurnCommandSignals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	input := strings.TrimSpace(signals.Agent.Composer.Value)
	if input == "" {
		http.Error(w, "input is required", http.StatusBadRequest)
		return
	}
	s.runChatTurn(w, r, service, scope, clientID, strings.TrimSpace(signals.Agent.ActiveConversationID), input)
}

func (s *Server) runChatTurn(w http.ResponseWriter, r *http.Request, service *agentapp.Service, scope agentapp.Scope, clientID, activeConversationID, input string) {
	streamConversations := s.chatConversations(r.Context(), scope)
	conversationID := strings.TrimSpace(activeConversationID)
	createdConversation := false
	if conversationID == "" {
		conversation, err := service.CreateConversation(r.Context(), scope, "New conversation")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		conversationID = conversation.ID
		createdConversation = true
	}

	transcript, err := service.ConversationTranscript(r.Context(), scope, conversationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	transcript = appendServerUserTranscript(transcript, conversationID, input)
	sse := datastar.NewSSE(w, r)
	if createdConversation {
		_ = sse.ReplaceURL(url.URL{Path: "/chat/" + conversationID})
	}

	streamActiveID := strings.TrimSpace(activeConversationID)
	emit := func(event agentapp.EventEnvelope) {
		transcript = applyLiveTranscriptEvent(transcript, conversationID, event)
		_ = sse.MarshalAndPatchSignals(map[string]any{"agent": chatSignalWithConversations(streamConversations, streamActiveID, transcript, "", true, true)})
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
		if refreshed, refreshErr := service.ConversationTranscript(r.Context(), scope, conversationID); refreshErr == nil {
			transcript = refreshed
		}
	}
	shouldGenerateTitle := createdConversation && err == nil && result.RunID != ""
	if shouldGenerateTitle {
		s.markChatTitlePending(conversationID)
	}
	_ = sse.MarshalAndPatchSignals(map[string]any{"agent": s.chatSignalWith(r.Context(), scope, conversationID, transcript, statusErr, false)})
	if shouldGenerateTitle {
		s.generateConversationTitleAsync(scope, conversationID, clientID)
	}
}

func (s *Server) chatUpdates(w http.ResponseWriter, r *http.Request) {
	_, scope, ok := s.chatService(w, r)
	if !ok {
		return
	}
	sse := datastar.NewSSE(w, r)
	updates, unsubscribe := s.broker.Subscribe(chatStreamID(scope, chatClientID(r)))
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case patch := <-updates:
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return
			}
		}
	}
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
	devBypass := false
	if s.auth != nil {
		if principal, ok := s.auth.Principal(r); ok {
			principalID = principal.ID
			devBypass = principal.DevBypass
			if principal.DevBypass {
				_ = s.upsertAuthenticatedPrincipal(r.Context(), principal)
			}
		}
	}
	return agentapp.Scope{WorkspaceID: s.workspaceID(""), PrincipalID: principalID, DevAuthBypass: devBypass}
}

func chatClientID(r *http.Request) string {
	if cookie, err := r.Cookie("ld_client_id"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return "default"
}
