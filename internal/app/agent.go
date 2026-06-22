package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/go-chi/chi/v5"
)

func (s *Server) createAgentConversation(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	var input api.AgentConversationCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	conversation, err := service.CreateConversation(r.Context(), scope, input.Title)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, agentConversationDTO(conversation))
}

func (s *Server) listAgentConversations(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	conversations, err := service.ListConversations(r.Context(), scope)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	out := make([]api.AgentConversationResponse, 0, len(conversations))
	for _, conversation := range conversations {
		out = append(out, agentConversationDTO(conversation))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listAgentMessages(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	messages, err := service.ListMessages(r.Context(), scope, chi.URLParam(r, "conversation"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	out := make([]api.AgentMessageResponse, 0, len(messages))
	for _, message := range messages {
		out = append(out, agentMessageDTO(message))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createAgentTurn(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	var input api.AgentTurnRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(input.Input) == "" {
		writeJSONError(w, fmt.Errorf("input is required"), http.StatusBadRequest)
		return
	}
	result, err := service.Prompt(r.Context(), agentapp.PromptInput{
		Scope:          scope,
		ConversationID: chi.URLParam(r, "conversation"),
		Input:          input.Input,
		CorrelationID:  input.CorrelationID,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, agentapp.ErrDisabled) {
			status = http.StatusServiceUnavailable
		} else if agentapp.IsBusy(err) {
			status = http.StatusConflict
		} else if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeJSONError(w, err, status)
		return
	}
	writeJSON(w, http.StatusOK, api.AgentTurnResponse{
		ConversationID: result.ConversationID,
		RunID:          result.RunID,
		StopReason:     string(result.StopReason),
		Content:        result.Content,
	})
}

func (s *Server) listAgentEvents(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	events, err := service.ListEvents(r.Context(), scope, chi.URLParam(r, "run"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	out := make([]api.AgentEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, agentEventDTO(event))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) agentRequest(w http.ResponseWriter, r *http.Request) (*agentapp.Service, agentapp.Scope, bool) {
	if s.agent == nil || !s.agent.Enabled() {
		writeJSONError(w, agentapp.ErrDisabled, http.StatusServiceUnavailable)
		return nil, agentapp.Scope{}, false
	}
	if s.auth == nil {
		writeJSONError(w, fmt.Errorf("agent API requires authentication"), http.StatusUnauthorized)
		return nil, agentapp.Scope{}, false
	}
	principal, ok := s.auth.Principal(r)
	if !ok {
		writeJSONError(w, fmt.Errorf("agent API requires an authenticated principal"), http.StatusUnauthorized)
		return nil, agentapp.Scope{}, false
	}
	scope := agentapp.Scope{
		WorkspaceID: s.workspaceID(chi.URLParam(r, "workspace")),
		PrincipalID: principal.ID,
	}
	if principal.DevBypass && s.store != nil {
		_, _ = s.store.UpsertPrincipal(r.Context(), platformPrincipalInput(principal))
	}
	return s.agent, scope, true
}

func platformPrincipalInput(principal Principal) platform.PrincipalInput {
	return platform.PrincipalInput{ID: principal.ID, Email: principal.Email, DisplayName: principal.DisplayName}
}

func agentConversationDTO(row platformdb.AgentConversation) api.AgentConversationResponse {
	out := api.AgentConversationResponse{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		PrincipalID: row.PrincipalID,
		Title:       row.Title,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.ArchivedAt.Valid {
		out.ArchivedAt = row.ArchivedAt.String
	}
	return out
}

func agentMessageDTO(row platformdb.AgentMessage) api.AgentMessageResponse {
	runID := ""
	if row.RunID.Valid {
		runID = row.RunID.String
	}
	return api.AgentMessageResponse{
		ID:          row.ID,
		RunID:       runID,
		Seq:         row.Seq,
		Role:        row.Role,
		ContentText: row.ContentText,
		ContentJSON: row.ContentJson,
		ToolCallID:  row.ToolCallID,
		ToolName:    row.ToolName,
		IsError:     row.IsError,
		CreatedAt:   row.CreatedAt,
	}
}

func agentEventDTO(row platformdb.AgentEvent) api.AgentEventResponse {
	return api.AgentEventResponse{
		ID:          row.ID,
		RunID:       row.RunID,
		Seq:         row.Seq,
		EventType:   row.EventType,
		Severity:    row.Severity,
		PayloadJSON: row.PayloadJson,
		CreatedAt:   row.CreatedAt,
	}
}
