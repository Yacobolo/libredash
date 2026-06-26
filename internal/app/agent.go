package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/api"
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
	page, limit, ok := agentPageFromRequest(w, r)
	if !ok {
		return
	}
	conversations, err := service.ListConversationsPage(r.Context(), scope, page)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	nextCursor := ""
	if len(conversations) > limit {
		nextCursor = conversations[limit-1].ID
		conversations = conversations[:limit]
	}
	out := make([]api.AgentConversationResponse, 0, len(conversations))
	for _, conversation := range conversations {
		out = append(out, agentConversationDTO(conversation))
	}
	writeJSON(w, http.StatusOK, pagedResponseWithCursor(out, nextCursor))
}

func (s *Server) getAgentConversation(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	conversation, err := service.GetConversation(r.Context(), scope, chi.URLParam(r, "conversation"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, agentConversationDTO(conversation))
}

func (s *Server) updateAgentConversation(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	var input api.AgentConversationUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	conversation, err := service.UpdateConversation(r.Context(), scope, chi.URLParam(r, "conversation"), input.Title)
	if err != nil {
		writeJSONError(w, err, statusForBadRequestOrNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, agentConversationDTO(conversation))
}

func (s *Server) archiveAgentConversation(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	conversation, err := service.ArchiveConversation(r.Context(), scope, chi.URLParam(r, "conversation"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, agentConversationDTO(conversation))
}

func (s *Server) listAgentMessages(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	page, limit, ok := agentPageFromRequest(w, r)
	if !ok {
		return
	}
	messages, err := service.ListMessagesPage(r.Context(), scope, chi.URLParam(r, "conversation"), page)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	nextCursor := ""
	if len(messages) > limit {
		nextCursor = messages[limit-1].ID
		messages = messages[:limit]
	}
	out := make([]api.AgentMessageResponse, 0, len(messages))
	for _, message := range messages {
		out = append(out, agentMessageDTO(message))
	}
	writeJSON(w, http.StatusOK, pagedResponseWithCursor(out, nextCursor))
}

func (s *Server) listAgentRuns(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	page, limit, ok := agentPageFromRequest(w, r)
	if !ok {
		return
	}
	runs, err := service.ListRunsPage(r.Context(), scope, chi.URLParam(r, "conversation"), page)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	nextCursor := ""
	if len(runs) > limit {
		nextCursor = runs[limit-1].ID
		runs = runs[:limit]
	}
	out := make([]api.AgentRunResponse, 0, len(runs))
	for _, run := range runs {
		out = append(out, agentRunDTO(run))
	}
	writeJSON(w, http.StatusOK, pagedResponseWithCursor(out, nextCursor))
}

func (s *Server) getAgentRun(w http.ResponseWriter, r *http.Request) {
	service, scope, ok := s.agentRequest(w, r)
	if !ok {
		return
	}
	conversationID := chi.URLParam(r, "conversation")
	runID := chi.URLParam(r, "run")
	if conversationID == "" {
		run, err := service.GetRunByID(r.Context(), scope, runID)
		if err != nil {
			writeJSONError(w, err, statusForNotFound(err))
			return
		}
		writeJSON(w, http.StatusOK, agentRunDTO(run))
		return
	}
	run, err := service.GetRun(r.Context(), scope, conversationID, runID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, agentRunDTO(run))
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
	var (
		events []agentapp.Event
		err    error
	)
	page, limit, ok := agentPageFromRequest(w, r)
	if !ok {
		return
	}
	if conversationID := chi.URLParam(r, "conversation"); conversationID != "" {
		events, err = service.ListRunEventsPage(r.Context(), scope, conversationID, chi.URLParam(r, "run"), page)
	} else {
		events, err = service.ListEvents(r.Context(), scope, chi.URLParam(r, "run"))
		if err == nil {
			events = pageAgentEvents(events, page)
		}
	}
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	nextCursor := ""
	if len(events) > limit {
		nextCursor = events[limit-1].ID
		events = events[:limit]
	}
	out := make([]api.AgentEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, agentEventDTO(event))
	}
	writeJSON(w, http.StatusOK, pagedResponseWithCursor(out, nextCursor))
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
		WorkspaceID:   s.workspaceID(chi.URLParam(r, "workspace")),
		PrincipalID:   principal.ID,
		DevAuthBypass: principal.DevBypass,
	}
	if credential, ok := s.auth.APICredential(r); ok {
		scope.Credential = agentCredentialScope(credential)
	}
	return s.agent, scope, true
}

func agentCredentialScope(credential access.APICredential) agentapp.CredentialScope {
	token := credential.Token
	return agentapp.CredentialScope{
		WorkspaceID: token.WorkspaceID,
		Permissions: append([]string(nil), token.Permissions...),
		Restricted:  token.Permissions != nil,
	}
}

func agentConversationDTO(row agentapp.Conversation) api.AgentConversationResponse {
	out := api.AgentConversationResponse{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		PrincipalID: row.PrincipalID,
		Title:       row.Title,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	out.ArchivedAt = row.ArchivedAt
	return out
}

func agentRunDTO(row agentapp.Run) api.AgentRunResponse {
	return api.AgentRunResponse{
		ID:             row.ID,
		ConversationID: row.ConversationID,
		Status:         row.Status,
		Model:          row.Model,
		StopReason:     row.StopReason,
		InputTokens:    row.InputTokens,
		OutputTokens:   row.OutputTokens,
		TotalTokens:    row.TotalTokens,
		Error:          row.Error,
		StartedAt:      row.StartedAt,
		FinishedAt:     row.FinishedAt,
		CreatedAt:      row.CreatedAt,
	}
}

func agentMessageDTO(row agentapp.Message) api.AgentMessageResponse {
	return api.AgentMessageResponse{
		ID:          row.ID,
		RunID:       row.RunID,
		Seq:         row.Seq,
		Role:        row.Role,
		ContentText: row.ContentText,
		Content:     jsonObject(row.ContentJSON),
		ToolCallID:  row.ToolCallID,
		ToolName:    row.ToolName,
		IsError:     row.IsError,
		CreatedAt:   row.CreatedAt,
	}
}

func agentPageFromRequest(w http.ResponseWriter, r *http.Request) (agentapp.Page, int, bool) {
	limit, ok := apiLimitForRequest(w, r)
	if !ok {
		return agentapp.Page{}, 0, false
	}
	query := r.URL.Query()
	pageLimit := limit
	if pageLimit < maxAPILimit {
		pageLimit++
	}
	return agentapp.Page{Limit: pageLimit, After: firstNonEmpty(query.Get("pageToken"), query.Get("after"))}, limit, true
}

func statusForBadRequestOrNotFound(err error) int {
	if errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

func pageAgentEvents(events []agentapp.Event, page agentapp.Page) []agentapp.Event {
	limit := page.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	start := 0
	after := strings.TrimSpace(page.After)
	if after != "" {
		start = len(events)
		for i, event := range events {
			if event.ID == after {
				start = i + 1
				break
			}
		}
	}
	if start >= len(events) {
		return []agentapp.Event{}
	}
	end := start + limit
	if end > len(events) {
		end = len(events)
	}
	return append([]agentapp.Event(nil), events[start:end]...)
}

func agentEventDTO(row agentapp.Event) api.AgentEventResponse {
	return api.AgentEventResponse{
		ID:        row.ID,
		RunID:     row.RunID,
		Seq:       row.Seq,
		EventType: row.EventType,
		Severity:  row.Severity,
		Payload:   jsonObject(row.PayloadJSON),
		CreatedAt: row.CreatedAt,
	}
}

func jsonObject(raw string) map[string]any {
	var out map[string]any
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	if out == nil {
		return map[string]any{}
	}
	return out
}
