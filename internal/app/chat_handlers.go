package app

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/pkg/pagestream"
	"github.com/go-chi/chi/v5"
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

type chatTurnEmitter func(ui.ChatSignal) error

type chatTurnExecution struct {
	emitInitialRunning bool
	generateTitle      bool
	clientID           string
	liveConversations  []ui.ChatConversationSummary
	emit               chatTurnEmitter
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	scope := s.chatScope(r)
	s.renderChat(w, r, "list", s.chatSignal(r.Context(), scope, "", "", false))
}

func (s *Server) legacyChatRedirect(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/updates") {
		http.NotFound(w, r)
		return
	}
	status := http.StatusFound
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		status = http.StatusTemporaryRedirect
	}
	http.Redirect(w, r, legacyGlobalChatPath(chi.URLParam(r, "conversation"), strings.TrimSuffix(r.URL.Path, "/")), status)
}

func (s *Server) chatNew(w http.ResponseWriter, r *http.Request) {
	scope := s.chatScope(r)
	s.renderChat(w, r, "new", s.chatSignal(r.Context(), scope, "", "", false))
}

func (s *Server) chatConversation(w http.ResponseWriter, r *http.Request) {
	scope := s.chatScope(r)
	conversationID := strings.TrimSpace(chi.URLParam(r, "conversation"))
	if conversationID == "updates" {
		http.NotFound(w, r)
		return
	}
	if s.agent == nil || !s.agent.Enabled() {
		s.renderChat(w, r, "conversation", s.chatSignal(r.Context(), scope, "", "", false))
		return
	}
	if scope.PrincipalID == "" {
		http.Error(w, "chat requires an authenticated principal", http.StatusUnauthorized)
		return
	}
	state, err := s.agent.ConversationTranscriptState(r.Context(), scope, conversationID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	s.queueMissingChatTitle(r.Context(), scope, conversationID, chatClientID(r))
	s.renderChat(w, r, "conversation", s.chatSignalWith(r.Context(), scope, conversationID, state.Transcript, state.Artifacts, "", s.agent.ConversationRunning(conversationID)))
}

func (s *Server) renderChat(w http.ResponseWriter, r *http.Request, view string, signal ui.ChatSignal) {
	_ = pagestream.EnsureClientID(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	workspaceID := s.chatDefaultWorkspaceID()
	catalog := s.catalogForWorkspace(workspaceID)
	if err := ui.ChatPage(catalog, workspaceID, csrfToken(r, s.auth), s.currentRoleLabel(r), view, signal).Render(w); err != nil {
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
	if err := pagestream.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	input := strings.TrimSpace(signals.Agent.Composer.Value)
	if input == "" {
		http.Error(w, "input is required", http.StatusBadRequest)
		return
	}
	activeConversationID := strings.TrimSpace(signals.Agent.ActiveConversationID)
	if activeConversationID == "" {
		s.startDraftChatTurn(w, r, service, scope, clientID, input)
		return
	}
	s.runChatTurn(w, r, service, scope, clientID, activeConversationID, input)
}

func (s *Server) startDraftChatTurn(w http.ResponseWriter, r *http.Request, service *agentapp.Service, scope agentapp.Scope, clientID, input string) {
	conversation, err := service.CreateConversation(r.Context(), scope, "New conversation")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	started, err := service.StartPrompt(r.Context(), agentapp.PromptInput{
		Scope:          scope,
		ConversationID: conversation.ID,
		Input:          input,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	go s.completeDraftChatTurn(service, scope, clientID, started)
	if err := pagestream.Redirect(w, r, chatRoutePath(scope.WorkspaceID, conversation.ID)); err != nil {
		return
	}
}

func (s *Server) completeDraftChatTurn(service *agentapp.Service, scope agentapp.Scope, clientID string, started *agentapp.StartedPrompt) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_, _ = s.executeStartedChatTurn(ctx, service, scope, started, chatTurnExecution{
		emitInitialRunning: true,
		generateTitle:      true,
		clientID:           clientID,
		emit: func(signal ui.ChatSignal) error {
			s.broker.Publish(chatStreamID(scope, clientID), chatSignalPatch(signal))
			return nil
		},
	})
}

func (s *Server) runChatTurn(w http.ResponseWriter, r *http.Request, service *agentapp.Service, scope agentapp.Scope, clientID, activeConversationID, input string) {
	streamConversations := s.chatConversations(r.Context(), scope)
	conversationID := strings.TrimSpace(activeConversationID)

	state, err := service.ConversationTranscriptState(r.Context(), scope, conversationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	transcript := state.Transcript
	streamArtifacts := state.Artifacts
	updates := pagestream.NewSignalStream(w, r)

	started, err := service.StartPrompt(r.Context(), agentapp.PromptInput{
		Scope:          scope,
		ConversationID: conversationID,
		Input:          input,
	})
	if err != nil {
		_ = updates.Patch(chatSignalPatch(s.chatSignalWith(r.Context(), scope, conversationID, transcript, streamArtifacts, chatTurnStatusError(err), false)))
		return
	}
	_, _ = s.executeStartedChatTurn(r.Context(), service, scope, started, chatTurnExecution{
		liveConversations: streamConversations,
		emit: func(signal ui.ChatSignal) error {
			return updates.Patch(chatSignalPatch(signal))
		},
	})
}

func (s *Server) executeStartedChatTurn(ctx context.Context, service *agentapp.Service, scope agentapp.Scope, started *agentapp.StartedPrompt, execution chatTurnExecution) (agentapp.PromptResult, error) {
	state, err := service.ConversationTranscriptState(ctx, scope, started.ConversationID)
	if err != nil {
		_ = started.Abort(ctx, err)
		return agentapp.PromptResult{}, err
	}
	transcript := state.Transcript
	streamArtifacts := state.Artifacts
	emit := func(signal ui.ChatSignal) {
		if execution.emit != nil {
			_ = execution.emit(signal)
		}
	}
	liveSignal := func(statusErr string, running bool) ui.ChatSignal {
		conversations := execution.liveConversations
		if conversations == nil {
			conversations = s.chatConversations(ctx, scope)
		}
		return chatSignalWithConversations(conversations, started.ConversationID, transcript, streamArtifacts, statusErr, running, true)
	}
	finalSignal := func(statusErr string, running bool) ui.ChatSignal {
		return s.chatSignalWith(ctx, scope, started.ConversationID, transcript, streamArtifacts, statusErr, running)
	}
	if execution.emitInitialRunning {
		emit(finalSignal("", true))
	}
	result, err := started.Complete(ctx, func(event agentapp.EventEnvelope) {
		transcript = applyLiveTranscriptEvent(transcript, started.ConversationID, event)
		emit(liveSignal("", true))
	})
	statusErr := chatTurnStatusError(err)
	if result.RunID != "" {
		if refreshed, refreshErr := service.ConversationTranscriptState(ctx, scope, started.ConversationID); refreshErr == nil {
			transcript = refreshed.Transcript
			streamArtifacts = refreshed.Artifacts
		}
	}
	shouldGenerateTitle := execution.generateTitle && err == nil && result.RunID != ""
	if shouldGenerateTitle {
		s.markChatTitlePending(started.ConversationID)
	}
	emit(finalSignal(statusErr, false))
	if shouldGenerateTitle {
		s.generateConversationTitleAsync(scope, started.ConversationID, execution.clientID)
	}
	return result, err
}

func chatTurnStatusError(err error) string {
	if err == nil {
		return ""
	}
	if agentapp.IsBusy(err) {
		return "A turn is already running for this conversation."
	}
	return err.Error()
}

func (s *Server) chatUpdates(w http.ResponseWriter, r *http.Request) {
	_, scope, ok := s.chatService(w, r)
	if !ok {
		return
	}
	updates := pagestream.NewSignalStream(w, r)
	signal, view := s.chatBootstrapSignal(r, scope)
	workspaceID := s.chatDefaultWorkspaceID()
	catalog := s.catalogForWorkspace(workspaceID)
	if err := updates.Patch(ui.ChatBootstrapSignals(catalog, workspaceID, s.currentRoleLabel(r), view, signal)); err != nil {
		return
	}
	_ = updates.Forward(r.Context(), s.broker, chatStreamID(scope, chatClientID(r)))
}

func (s *Server) chatBootstrapUpdates(w http.ResponseWriter, r *http.Request) {
	scope := s.chatScope(r)
	signal, view := s.chatBootstrapSignal(r, scope)
	workspaceID := s.chatDefaultWorkspaceID()
	s.patchAndWait(w, r, ui.ChatBootstrapSignals(s.catalogForWorkspace(workspaceID), workspaceID, s.currentRoleLabel(r), view, signal))
}

func (s *Server) chatBootstrapSignal(r *http.Request, scope agentapp.Scope) (ui.ChatSignal, string) {
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	if view == "" {
		view = "list"
	}
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation"))
	if conversationID == "" || s.agent == nil || !s.agent.Enabled() || scope.PrincipalID == "" {
		return s.chatSignal(r.Context(), scope, "", "", false), view
	}
	state, err := s.agent.ConversationTranscriptState(r.Context(), scope, conversationID)
	if err != nil {
		return s.chatSignal(r.Context(), scope, "", "", false), view
	}
	return s.chatSignalWith(r.Context(), scope, conversationID, state.Transcript, state.Artifacts, "", s.agent.ConversationRunning(conversationID)), view
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
		}
	} else if principal, ok := principalFromContext(r.Context()); ok {
		principalID = principal.ID
		devBypass = principal.DevBypass
	}
	return agentapp.Scope{WorkspaceID: s.chatDefaultWorkspaceID(), PrincipalID: principalID, DevAuthBypass: devBypass}
}

func (s *Server) catalogForWorkspace(workspaceID string) dashboard.Catalog {
	if metrics, ok := s.metricsForWorkspace(workspaceID); ok && metrics != nil {
		return metrics.Catalog()
	}
	if s.metrics == nil {
		return dashboard.Catalog{Workspace: dashboard.CatalogWorkspace{ID: workspaceID}}
	}
	return s.metrics.Catalog()
}

func chatRoutePath(workspaceID string, parts ...string) string {
	path := "/chat"
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		path += "/" + url.PathEscape(part)
	}
	return path
}

func (s *Server) chatDefaultWorkspaceID() string {
	if strings.TrimSpace(s.defaultWorkspaceID) != "" {
		return s.defaultWorkspaceID
	}
	return s.workspaceID("")
}

func legacyGlobalChatPath(conversationID, path string) string {
	switch {
	case strings.HasSuffix(path, "/new"):
		return chatRoutePath("", "new")
	case strings.HasSuffix(path, "/turns"):
		return chatRoutePath("", "turns")
	case strings.TrimSpace(conversationID) != "":
		return chatRoutePath("", conversationID)
	default:
		return chatRoutePath("")
	}
}

func chatClientID(r *http.Request) string {
	if cookie, err := r.Cookie("ld_client_id"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return "default"
}
