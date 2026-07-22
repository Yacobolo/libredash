package http

import (
	"context"
	nethttp "net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/ui"
	"github.com/Yacobolo/leapview/pkg/pagestream"
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

type ChatTurnEmitter func(ui.ChatViewState) error

type ChatTurnExecution struct {
	EmitInitialRunning bool
	GenerateTitle      bool
	ClientID           string
	LiveConversations  []ui.ChatConversationSummary
	Emit               ChatTurnEmitter
}

func (h *Handler) Chat(w nethttp.ResponseWriter, r *nethttp.Request) {
	scope := h.chatScope(r)
	h.renderChat(w, r, "list", h.chatSignal(r.Context(), scope, "", "", false))
}

func (h *Handler) ChatNew(w nethttp.ResponseWriter, r *nethttp.Request) {
	scope := h.chatScope(r)
	h.renderChat(w, r, "new", h.chatSignal(r.Context(), scope, "", "", false))
}

func (h *Handler) ChatConversation(w nethttp.ResponseWriter, r *nethttp.Request) {
	scope := h.chatScope(r)
	conversationID := strings.TrimSpace(chi.URLParam(r, "conversation"))
	if conversationID == "updates" {
		nethttp.NotFound(w, r)
		return
	}
	if h.options.Service == nil || !h.options.Service.Enabled() {
		h.renderChat(w, r, "conversation", h.chatSignal(r.Context(), scope, "", "", false))
		return
	}
	if scope.PrincipalID == "" {
		nethttp.Error(w, "chat requires an authenticated principal", nethttp.StatusUnauthorized)
		return
	}
	state, err := h.options.Service.ConversationTranscriptState(r.Context(), scope, conversationID)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	if h.options.QueueMissingTitle != nil {
		h.options.QueueMissingTitle(r.Context(), scope, conversationID, chatClientID(r))
	}
	h.renderChat(w, r, "conversation", h.chatSignalWith(r.Context(), scope, conversationID, state.Transcript, state.Artifacts, "", h.options.Service.ConversationRunning(conversationID)))
}

func (h *Handler) ChatTurn(w nethttp.ResponseWriter, r *nethttp.Request) {
	service, scope, ok := h.chatService(w, r)
	if !ok {
		return
	}
	clientID := chatClientID(r)
	signals := chatTurnCommandSignals{}
	if err := pagestream.ReadSignals(r, &signals); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	input := strings.TrimSpace(signals.Agent.Composer.Value)
	if input == "" {
		nethttp.Error(w, "input is required", nethttp.StatusBadRequest)
		return
	}
	activeConversationID := strings.TrimSpace(signals.Agent.ActiveConversationID)
	if activeConversationID == "" {
		h.startDraftChatTurn(w, r, service, scope, clientID, input)
		return
	}
	h.runChatTurn(w, r, service, scope, clientID, activeConversationID, input)
}

func (h *Handler) ChatUpdates(w nethttp.ResponseWriter, r *nethttp.Request) {
	scope := h.chatScope(r)
	signal, view := h.chatBootstrapSignal(r, scope)
	workspaceID := ""
	catalog := dashboard.Catalog{}
	streamID := chatStreamID(scope, chatClientID(r))
	var trace *pagestream.TraceStore
	if h.options.Broker != nil {
		trace = h.options.Broker.TraceStore()
	}
	updates := pagestream.NewSignalStream(w, r, pagestream.WithStreamTrace(trace, streamID, "chat.bootstrap"))
	if err := updates.Patch(ui.ChatBootstrapSignals(catalog, workspaceID, h.currentRoleLabel(r), view, signal)); err != nil {
		return
	}
	if h.options.Service == nil || !h.options.Service.Enabled() || scope.PrincipalID == "" || h.options.Broker == nil {
		updates.Wait(r.Context())
		return
	}
	_ = updates.Forward(r.Context(), h.options.Broker, streamID)
}

func (h *Handler) renderChat(w nethttp.ResponseWriter, r *nethttp.Request, view string, signal ui.ChatViewState) {
	_ = pagestream.EnsureClientID(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	workspaceID := ""
	catalog := dashboard.Catalog{}
	if err := ui.ChatPage(catalog, workspaceID, h.csrfToken(r), h.currentRoleLabel(r), view, signal).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h *Handler) startDraftChatTurn(w nethttp.ResponseWriter, r *nethttp.Request, service *agent.Service, scope agent.Scope, clientID, input string) {
	conversation, err := service.CreateConversation(r.Context(), scope, "New conversation")
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	started, err := service.StartPrompt(r.Context(), agent.PromptInput{
		Scope:          scope,
		ConversationID: conversation.ID,
		Input:          input,
	})
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	go h.completeDraftChatTurn(service, scope, clientID, started)
	_ = pagestream.Redirect(w, r, chatRoutePath(conversation.ID))
}

func (h *Handler) completeDraftChatTurn(service *agent.Service, scope agent.Scope, clientID string, started *agent.StartedPrompt) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if h.options.ExecuteStartedChatTurn == nil {
		return
	}
	_, _ = h.options.ExecuteStartedChatTurn(ctx, service, scope, started, ChatTurnExecution{
		EmitInitialRunning: true,
		GenerateTitle:      true,
		ClientID:           clientID,
		Emit: func(signal ui.ChatViewState) error {
			if h.options.Broker != nil {
				h.options.Broker.Publish(chatStreamID(scope, clientID), chatSignalPatch(signal))
			}
			return nil
		},
	})
}

func (h *Handler) runChatTurn(w nethttp.ResponseWriter, r *nethttp.Request, service *agent.Service, scope agent.Scope, clientID, activeConversationID, input string) {
	conversationID := strings.TrimSpace(activeConversationID)
	state, err := service.ConversationTranscriptState(r.Context(), scope, conversationID)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	transcript := state.Transcript
	streamArtifacts := state.Artifacts
	streamID := chatStreamID(scope, clientID)
	var trace *pagestream.TraceStore
	if h.options.Broker != nil {
		trace = h.options.Broker.TraceStore()
	}
	updates := pagestream.NewSignalStream(w, r, pagestream.WithStreamTrace(trace, streamID, "chat.turn"))
	started, err := service.StartPrompt(r.Context(), agent.PromptInput{
		Scope:          scope,
		ConversationID: conversationID,
		Input:          input,
	})
	if err != nil {
		_ = updates.Patch(chatSignalPatch(h.chatSignalWith(r.Context(), scope, conversationID, transcript, streamArtifacts, chatTurnStatusError(err), false)))
		return
	}
	if h.options.ExecuteStartedChatTurn == nil {
		nethttp.Error(w, "chat turn executor is not configured", nethttp.StatusServiceUnavailable)
		return
	}
	_, _ = h.options.ExecuteStartedChatTurn(r.Context(), service, scope, started, ChatTurnExecution{
		LiveConversations: h.chatConversations(r.Context(), scope),
		Emit: func(signal ui.ChatViewState) error {
			return updates.Patch(chatSignalPatch(signal))
		},
	})
}

func (h *Handler) chatBootstrapSignal(r *nethttp.Request, scope agent.Scope) (ui.ChatViewState, string) {
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	if view == "" {
		view = "list"
	}
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation"))
	if conversationID == "" || h.options.Service == nil || !h.options.Service.Enabled() || scope.PrincipalID == "" {
		return h.chatSignal(r.Context(), scope, "", "", false), view
	}
	state, err := h.options.Service.ConversationTranscriptState(r.Context(), scope, conversationID)
	if err != nil {
		return h.chatSignal(r.Context(), scope, "", "", false), view
	}
	return h.chatSignalWith(r.Context(), scope, conversationID, state.Transcript, state.Artifacts, "", h.options.Service.ConversationRunning(conversationID)), view
}

func (h *Handler) chatService(w nethttp.ResponseWriter, r *nethttp.Request) (*agent.Service, agent.Scope, bool) {
	if h.options.Service == nil || !h.options.Service.Enabled() {
		nethttp.Error(w, agent.ErrDisabled.Error(), nethttp.StatusServiceUnavailable)
		return nil, agent.Scope{}, false
	}
	scope := h.chatScope(r)
	if scope.PrincipalID == "" {
		nethttp.Error(w, "chat requires an authenticated principal", nethttp.StatusUnauthorized)
		return nil, agent.Scope{}, false
	}
	return h.options.Service, scope, true
}

func (h *Handler) chatScope(r *nethttp.Request) agent.Scope {
	principalID := ""
	devBypass := false
	if h.options.CurrentPrincipal != nil {
		if principal, ok := h.options.CurrentPrincipal(r); ok {
			principalID = principal.ID
			devBypass = principal.DevAuthBypass
		}
	}
	scope := agent.Scope{PrincipalID: principalID, DevAuthBypass: devBypass}
	if h.options.CurrentCredential != nil {
		if credential, ok := h.options.CurrentCredential(r); ok {
			scope.Credential = agentCredentialScope(credential)
		}
	}
	return scope
}

func (h *Handler) chatSignal(ctx context.Context, scope agent.Scope, activeID, statusErr string, running bool) ui.ChatViewState {
	if h.options.ChatSignal != nil {
		return h.options.ChatSignal(ctx, scope, activeID, statusErr, running)
	}
	return ui.ChatViewState{}
}

func (h *Handler) chatSignalWith(ctx context.Context, scope agent.Scope, activeID string, transcript []agent.ChatTranscriptItem, artifacts agent.ChatArtifactSignals, statusErr string, running bool) ui.ChatViewState {
	if h.options.ChatSignalWith != nil {
		return h.options.ChatSignalWith(ctx, scope, activeID, transcript, artifacts, statusErr, running)
	}
	return ui.ChatViewState{}
}

func (h *Handler) chatConversations(ctx context.Context, scope agent.Scope) []ui.ChatConversationSummary {
	signal := h.chatSignal(ctx, scope, "", "", false)
	return signal.Agent.Conversations
}

func (h *Handler) csrfToken(r *nethttp.Request) string {
	if h.options.CSRFToken == nil {
		return ""
	}
	return h.options.CSRFToken(r)
}

func (h *Handler) currentRoleLabel(r *nethttp.Request) string {
	if h.options.CurrentRoleLabel == nil {
		return ""
	}
	return h.options.CurrentRoleLabel(r)
}

func chatTurnStatusError(err error) string {
	if err == nil {
		return ""
	}
	if agent.IsBusy(err) {
		return "A turn is already running for this conversation."
	}
	return err.Error()
}

func chatSignalPatch(signal ui.ChatViewState) pagestream.SignalPatch {
	return pagestream.SignalPatch{
		"agent":   signal.Agent,
		"visuals": signal.Visuals,
	}
}

func chatRoutePath(parts ...string) string {
	path := "/chats"
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		path += "/" + url.PathEscape(part)
	}
	return path
}

func chatClientID(r *nethttp.Request) string {
	if cookie, err := r.Cookie("lv_client_id"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return "default"
}

func chatStreamID(scope agent.Scope, clientID string) string {
	if strings.TrimSpace(clientID) == "" {
		clientID = "default"
	}
	return "chat:" + clientID + ":" + scope.PrincipalID
}
