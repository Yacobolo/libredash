package app

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/pkg/agent"
	"github.com/go-chi/chi/v5"
	"github.com/starfederation/datastar-go/datastar"
)

type chatSignals struct {
	Agent chatTurnAgentSignal `json:"agent"`
}

type chatTurnAgentSignal struct {
	ActiveConversationID string                 `json:"activeConversationId"`
	Composer             chatTurnComposerSignal `json:"composer"`
}

type chatTurnComposerSignal struct {
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
	s.renderChat(w, r, s.chatSignalWith(scope, conversationID, transcript, "", false))
}

func (s *Server) renderChat(w http.ResponseWriter, r *http.Request, signal api.AgentChatSignal) {
	_ = ensureClientID(w, r)
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
	streamConversations := s.chatConversations(scope)
	conversationID := strings.TrimSpace(signals.Agent.ActiveConversationID)
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

	streamActiveID := strings.TrimSpace(signals.Agent.ActiveConversationID)
	emit := func(event api.AgentEventEnvelope) {
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
	_ = sse.MarshalAndPatchSignals(map[string]any{"agent": s.chatSignalWith(scope, conversationID, transcript, statusErr, false)})
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
	updates, unsubscribe := s.broker.subscribe(chatStreamID(scope, chatClientID(r)))
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

func chatSignalWithConversations(conversations []api.AgentConversationResponse, activeID string, transcript []api.AgentChatTranscriptItem, statusErr string, running, enabled bool) api.AgentChatSignal {
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	if conversations == nil {
		conversations = []api.AgentConversationResponse{}
	}
	return api.AgentChatSignal{
		Conversations:        conversations,
		ActiveConversationID: activeID,
		Transcript:           transcript,
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
	transcript := []api.AgentChatTranscriptItem{}
	if activeID != "" && s.agent != nil && scope.PrincipalID != "" {
		if loaded, err := s.agent.ConversationTranscript(ctx, scope, activeID); err == nil {
			transcript = loaded
		}
	}
	return s.chatSignalWith(scope, activeID, transcript, statusErr, running)
}

func (s *Server) chatSignalWith(scope agentapp.Scope, activeID string, transcript []api.AgentChatTranscriptItem, statusErr string, running bool) api.AgentChatSignal {
	conversations := s.chatConversations(scope)
	enabled := s.agent != nil && s.agent.Enabled()
	if !enabled && statusErr == "" {
		statusErr = "Agent is not configured"
	}
	return api.AgentChatSignal{
		Conversations:        conversations,
		ActiveConversationID: activeID,
		Transcript:           transcript,
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

func (s *Server) chatConversations(scope agentapp.Scope) []api.AgentConversationResponse {
	conversations := []api.AgentConversationResponse{}
	if s.agent == nil || scope.PrincipalID == "" {
		return conversations
	}
	rows, err := s.agent.ListConversations(context.Background(), scope)
	if err != nil {
		return conversations
	}
	for _, row := range rows {
		out := agentConversationDTO(row)
		out.TitlePending = s.isChatTitlePending(row.ID)
		conversations = append(conversations, out)
	}
	return conversations
}

func (s *Server) generateConversationTitleAsync(scope agentapp.Scope, conversationID, clientID string) {
	if s.agent == nil {
		s.clearChatTitlePending(conversationID)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = s.agent.GenerateConversationTitle(ctx, scope, conversationID)
		s.clearChatTitlePending(conversationID)
		s.broker.publish(chatStreamID(scope, clientID), signalPatch{
			"agent": map[string]any{
				"conversations": s.chatConversations(scope),
			},
		})
	}()
}

func (s *Server) queueMissingChatTitle(ctx context.Context, scope agentapp.Scope, conversationID, clientID string) {
	if s.agent == nil || s.isChatTitlePending(conversationID) {
		return
	}
	ok, err := s.agent.ConversationNeedsGeneratedTitle(ctx, scope, conversationID)
	if err != nil || !ok {
		return
	}
	s.markChatTitlePending(conversationID)
	s.generateConversationTitleAsync(scope, conversationID, clientID)
}

func (s *Server) markChatTitlePending(conversationID string) {
	if conversationID == "" {
		return
	}
	s.chatTitleMu.Lock()
	defer s.chatTitleMu.Unlock()
	if s.pendingChatTitles == nil {
		s.pendingChatTitles = map[string]struct{}{}
	}
	s.pendingChatTitles[conversationID] = struct{}{}
}

func (s *Server) clearChatTitlePending(conversationID string) {
	if conversationID == "" {
		return
	}
	s.chatTitleMu.Lock()
	defer s.chatTitleMu.Unlock()
	delete(s.pendingChatTitles, conversationID)
}

func (s *Server) isChatTitlePending(conversationID string) bool {
	s.chatTitleMu.Lock()
	defer s.chatTitleMu.Unlock()
	_, ok := s.pendingChatTitles[conversationID]
	return ok
}

func chatStreamID(scope agentapp.Scope, clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "chat:" + clientID + ":" + scope.WorkspaceID + ":" + scope.PrincipalID
}

func chatClientID(r *http.Request) string {
	if cookie, err := r.Cookie("ld_client_id"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return "default"
}

func appendServerUserTranscript(transcript []api.AgentChatTranscriptItem, conversationID, input string) []api.AgentChatTranscriptItem {
	if strings.TrimSpace(input) == "" {
		return transcript
	}
	next := append([]api.AgentChatTranscriptItem{}, transcript...)
	next = append(next, api.AgentChatTranscriptItem{
		ID:             "live:user",
		Kind:           "user",
		Text:           input,
		ConversationID: conversationID,
	})
	return next
}

func applyLiveTranscriptEvent(transcript []api.AgentChatTranscriptItem, conversationID string, event api.AgentEventEnvelope) []api.AgentChatTranscriptItem {
	next := append([]api.AgentChatTranscriptItem{}, transcript...)
	switch event.Type {
	case string(agent.EventTypeMessageDelta):
		delta := stringPayload(event.Payload, "delta")
		if delta == "" {
			return next
		}
		for i := len(next) - 1; i >= 0; i-- {
			if next[i].Kind == "assistant" && next[i].Status == "streaming" && next[i].RunID == event.RunID {
				next[i].Markdown += delta
				return next
			}
		}
		return append(next, api.AgentChatTranscriptItem{
			ID:             "live:assistant:" + event.RunID,
			Kind:           "assistant",
			Markdown:       delta,
			Status:         "streaming",
			ConversationID: conversationID,
			RunID:          event.RunID,
			CreatedAt:      event.CreatedAt,
		})
	case string(agent.EventTypeToolStart):
		callID := stringPayload(event.Payload, "tool_call_id")
		name := stringPayload(event.Payload, "tool_name")
		if callID == "" {
			return next
		}
		if idx := transcriptToolIndex(next, callID); idx >= 0 {
			next[idx].Status = "running"
			return next
		}
		return append(next, api.AgentChatTranscriptItem{
			ID:             "live:tool:" + callID,
			Kind:           "tool",
			ToolCallID:     callID,
			Name:           name,
			Title:          liveToolTitle(name),
			Status:         "running",
			ConversationID: conversationID,
			RunID:          event.RunID,
			CreatedAt:      event.CreatedAt,
		})
	case string(agent.EventTypeToolEnd):
		callID := stringPayload(event.Payload, "tool_call_id")
		if callID == "" {
			return next
		}
		idx := transcriptToolIndex(next, callID)
		if idx < 0 {
			name := stringPayload(event.Payload, "tool_name")
			next = append(next, api.AgentChatTranscriptItem{
				ID:             "live:tool:" + callID,
				Kind:           "tool",
				ToolCallID:     callID,
				Name:           name,
				Title:          liveToolTitle(name),
				ConversationID: conversationID,
				RunID:          event.RunID,
				CreatedAt:      event.CreatedAt,
			})
			idx = len(next) - 1
		}
		if event.Severity == string(agent.SeverityError) || event.Severity == string(agent.SeverityWarn) {
			next[idx].Status = "error"
			next[idx].Error = "Tool failed"
			return next
		}
		next[idx].Status = "complete"
		return next
	default:
		return next
	}
}

func transcriptToolIndex(transcript []api.AgentChatTranscriptItem, callID string) int {
	for i := range transcript {
		if transcript[i].Kind == "tool" && transcript[i].ToolCallID == callID {
			return i
		}
	}
	return -1
}

func stringPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}

func liveToolTitle(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Tool"
	}
	parts := strings.Fields(strings.ReplaceAll(name, "_", " "))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
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
