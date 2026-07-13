package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agent"
	agentconfig "github.com/Yacobolo/libredash/internal/agent/config"
	"github.com/Yacobolo/libredash/internal/api"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/ui"
	agentcore "github.com/Yacobolo/libredash/pkg/agent"
	"github.com/Yacobolo/libredash/pkg/pagestream"
	"github.com/go-chi/chi/v5"
)

type Principal struct {
	ID            string
	DevAuthBypass bool
}

type Settings interface {
	GetSetting(ctx context.Context, key string) (string, error)
	UpsertSetting(ctx context.Context, key, value string) error
}

type Options struct {
	Service                *agent.Service
	Settings               Settings
	CurrentPrincipal       func(*stdhttp.Request) (Principal, bool)
	CurrentCredential      func(*stdhttp.Request) (access.APICredential, bool)
	WorkspaceID            func(string) string
	DefaultWorkspace       string
	Broker                 *pagestream.Broker
	CatalogForWorkspace    func(string) dashboard.Catalog
	CSRFToken              func(*stdhttp.Request) string
	CurrentRoleLabel       func(*stdhttp.Request) string
	ChatSignal             func(context.Context, agent.Scope, string, string, bool) ui.ChatSignal
	ChatSignalWith         func(context.Context, agent.Scope, string, []agent.ChatTranscriptItem, agent.ChatArtifactSignals, string, bool) ui.ChatSignal
	QueueMissingTitle      func(context.Context, agent.Scope, string, string)
	ExecuteStartedChatTurn func(context.Context, *agent.Service, agent.Scope, *agent.StartedPrompt, ChatTurnExecution) (agent.PromptResult, error)
}

type Handler struct {
	options Options
}

func NewHandler(options Options) *Handler {
	return &Handler{options: options}
}

func (h *Handler) CreateConversation(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
	if !ok {
		return
	}
	var input api.AgentConversationCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	conversation, err := service.CreateConversation(r.Context(), scope, input.Title)
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	writeJSON(w, stdhttp.StatusCreated, agentConversationDTO(conversation))
}

func (h *Handler) ListConversations(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
	if !ok {
		return
	}
	page, limit, ok := agentPageFromRequest(w, r)
	if !ok {
		return
	}
	conversations, err := service.ListConversationsPage(r.Context(), scope, page)
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
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
	writeJSON(w, stdhttp.StatusOK, pagedResponseWithCursor(out, nextCursor))
}

func (h *Handler) GetConversation(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
	if !ok {
		return
	}
	conversation, err := service.GetConversation(r.Context(), scope, chi.URLParam(r, "conversation"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, stdhttp.StatusOK, agentConversationDTO(conversation))
}

func (h *Handler) UpdateConversation(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
	if !ok {
		return
	}
	var input api.AgentConversationUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	conversation, err := service.UpdateConversation(r.Context(), scope, chi.URLParam(r, "conversation"), input.Title)
	if err != nil {
		writeJSONError(w, err, statusForBadRequestOrNotFound(err))
		return
	}
	writeJSON(w, stdhttp.StatusOK, agentConversationDTO(conversation))
}

func (h *Handler) ArchiveConversation(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
	if !ok {
		return
	}
	conversation, err := service.ArchiveConversation(r.Context(), scope, chi.URLParam(r, "conversation"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, stdhttp.StatusOK, agentConversationDTO(conversation))
}

func (h *Handler) ListMessages(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
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
	writeJSON(w, stdhttp.StatusOK, pagedResponseWithCursor(out, nextCursor))
}

func (h *Handler) ListRuns(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
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
	writeJSON(w, stdhttp.StatusOK, pagedResponseWithCursor(out, nextCursor))
}

func (h *Handler) GetRun(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
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
		writeJSON(w, stdhttp.StatusOK, agentRunDTO(run))
		return
	}
	run, err := service.GetRun(r.Context(), scope, conversationID, runID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, stdhttp.StatusOK, agentRunDTO(run))
}

func (h *Handler) CreateTurn(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
	if !ok {
		return
	}
	var input api.AgentTurnRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	if strings.TrimSpace(input.Input) == "" {
		writeJSONError(w, fmt.Errorf("input is required"), stdhttp.StatusBadRequest)
		return
	}
	result, err := service.Prompt(r.Context(), agent.PromptInput{
		Scope:          scope,
		ConversationID: chi.URLParam(r, "conversation"),
		Input:          input.Input,
		CorrelationID:  input.CorrelationID,
	})
	if err != nil {
		status := stdhttp.StatusInternalServerError
		if errors.Is(err, agent.ErrDisabled) || errors.Is(err, agent.ErrPolicyDisabled) {
			status = stdhttp.StatusServiceUnavailable
		} else if agent.IsBusy(err) {
			status = stdhttp.StatusConflict
		} else if errors.Is(err, sql.ErrNoRows) {
			status = stdhttp.StatusNotFound
		}
		writeJSONError(w, err, status)
		return
	}
	writeJSON(w, stdhttp.StatusOK, api.AgentTurnResponse{
		ConversationID: result.ConversationID,
		RunID:          result.RunID,
		StopReason:     string(result.StopReason),
		Content:        result.Content,
	})
}

func (h *Handler) ListEvents(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	service, scope, ok := h.agentRequest(w, r)
	if !ok {
		return
	}
	var (
		events []agent.Event
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
	writeJSON(w, stdhttp.StatusOK, pagedResponseWithCursor(out, nextCursor))
}

func (h *Handler) GetAdminConfig(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	details, err := h.AdminDetails(r.Context())
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	writeJSON(w, stdhttp.StatusOK, details)
}

func (h *Handler) UpdateAdminConfig(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var signals adminAgentCommandSignals
	if err := pagestream.ReadSignals(r, &signals); err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	systemPrompt := signals.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = signals.AdminAgentCommand.SystemPrompt
	}
	prompt, err := agentconfig.NormalizeSystemPrompt(systemPrompt)
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	if h.options.Settings == nil {
		writeJSONError(w, agent.ErrDisabled, stdhttp.StatusServiceUnavailable)
		return
	}
	if err := h.options.Settings.UpsertSetting(r.Context(), agentconfig.SystemPromptSettingKey, prompt); err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	details, err := h.AdminDetails(r.Context())
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	writeJSON(w, stdhttp.StatusOK, details)
}

func (h *Handler) AdminDetails(ctx context.Context) (api.AdminAgentResponse, error) {
	prompt, err := h.SystemPrompt(ctx)
	if err != nil {
		return api.AdminAgentResponse{}, err
	}
	out := api.AdminAgentResponse{
		Enabled:      h.options.Service != nil && h.options.Service.Enabled(),
		SystemPrompt: prompt,
	}
	if h.options.Service != nil {
		out.Model = h.options.Service.Model()
		out.Tools = adminAgentToolDTOs(h.options.Service.ToolDefinitions(agent.Scope{WorkspaceID: h.options.DefaultWorkspace, PrincipalID: "admin", DevAuthBypass: true}))
	}
	return out, nil
}

func (h *Handler) agentRequest(w stdhttp.ResponseWriter, r *stdhttp.Request) (*agent.Service, agent.Scope, bool) {
	if h.options.Service == nil || !h.options.Service.Enabled() {
		writeJSONError(w, agent.ErrDisabled, stdhttp.StatusServiceUnavailable)
		return nil, agent.Scope{}, false
	}
	if h.options.CurrentPrincipal == nil {
		writeJSONError(w, fmt.Errorf("agent API requires authentication"), stdhttp.StatusUnauthorized)
		return nil, agent.Scope{}, false
	}
	principal, ok := h.options.CurrentPrincipal(r)
	if !ok {
		writeJSONError(w, fmt.Errorf("agent API requires an authenticated principal"), stdhttp.StatusUnauthorized)
		return nil, agent.Scope{}, false
	}
	scope := agent.Scope{
		WorkspaceID:   h.workspaceID(chi.URLParam(r, "workspace")),
		PrincipalID:   principal.ID,
		DevAuthBypass: principal.DevAuthBypass,
	}
	if h.options.CurrentCredential != nil {
		if credential, ok := h.options.CurrentCredential(r); ok {
			scope.Credential = agentCredentialScope(credential)
		}
	}
	return h.options.Service, scope, true
}

func (h *Handler) SystemPrompt(ctx context.Context) (string, error) {
	if h.options.Settings == nil {
		return agentconfig.DefaultSystemPrompt, nil
	}
	prompt, err := h.options.Settings.GetSetting(ctx, agentconfig.SystemPromptSettingKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return agentconfig.DefaultSystemPrompt, nil
		}
		return "", err
	}
	return agentconfig.NormalizeSystemPrompt(prompt)
}

func (h *Handler) workspaceID(candidate string) string {
	if h.options.WorkspaceID != nil {
		return h.options.WorkspaceID(candidate)
	}
	return candidate
}

type adminAgentCommandSignals struct {
	SystemPrompt      string `json:"systemPrompt"`
	AdminAgentCommand struct {
		SystemPrompt string `json:"systemPrompt"`
	} `json:"adminAgentCommand"`
}

func agentCredentialScope(credential access.APICredential) agent.CredentialScope {
	token := credential.Token
	return agent.CredentialScope{
		WorkspaceID: token.WorkspaceID,
		Privileges:  privilegeStrings(token.Privileges),
		Restricted:  token.Privileges != nil,
	}
}

func privilegeStrings(values []access.Privilege) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func agentConversationDTO(row agent.Conversation) api.AgentConversationResponse {
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

func agentRunDTO(row agent.Run) api.AgentRunResponse {
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

func agentMessageDTO(row agent.Message) api.AgentMessageResponse {
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

func agentEventDTO(row agent.Event) api.AgentEventResponse {
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

func agentPageFromRequest(w stdhttp.ResponseWriter, r *stdhttp.Request) (agent.Page, int, bool) {
	limit, ok := apiLimitForRequest(w, r)
	if !ok {
		return agent.Page{}, 0, false
	}
	query := r.URL.Query()
	pageLimit := limit
	if pageLimit < maxAPILimit {
		pageLimit++
	}
	return agent.Page{Limit: pageLimit, After: firstNonEmpty(query.Get("pageToken"), query.Get("after"))}, limit, true
}

func pageAgentEvents(events []agent.Event, page agent.Page) []agent.Event {
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
		return []agent.Event{}
	}
	end := start + limit
	if end > len(events) {
		end = len(events)
	}
	return append([]agent.Event(nil), events[start:end]...)
}

func adminAgentToolDTOs(tools []agentcore.ToolDefinition) []api.AdminAgentToolResponse {
	contracts := apigenapi.GetAPIGenToolContracts()
	out := make([]api.AdminAgentToolResponse, 0, len(tools))
	for _, tool := range tools {
		dto := api.AdminAgentToolResponse{
			Name:         tool.Name,
			Description:  tool.Description,
			Effect:       "read",
			Defaults:     map[string]any{},
			InputSchema:  jsonObject(string(tool.InputSchema)),
			OutputSchema: map[string]any{},
		}
		if contract, ok := contracts[tool.Name]; ok {
			dto.Effect = string(contract.Effect)
			dto.OutputSchema = jsonObject(string(contract.OutputSchema))
			for _, binding := range contract.Bindings {
				if binding.Argument != "" && binding.Default != nil {
					dto.Defaults[binding.Argument] = binding.Default
				}
			}
		}
		out = append(out, dto)
	}
	return out
}

type pageResponse struct {
	NextCursor string `json:"nextCursor"`
}

func pagedResponseWithCursor(items any, nextCursor string) map[string]any {
	return map[string]any{"items": items, "page": pageResponse{NextCursor: nextCursor}}
}

const (
	defaultAPILimit = 50
	maxAPILimit     = 100
)

func apiLimitForRequest(w stdhttp.ResponseWriter, r *stdhttp.Request) (int, bool) {
	limit, err := parseAPILimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return 0, false
	}
	return limit, true
}

func parseAPILimit(value string) (int, error) {
	if value == "" {
		return defaultAPILimit, nil
	}
	var limit int
	if _, err := fmt.Sscanf(value, "%d", &limit); err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit < 1 {
		return 0, fmt.Errorf("limit must be at least 1")
	}
	if limit > maxAPILimit {
		return maxAPILimit, nil
	}
	return limit, nil
}

func statusForNotFound(err error) int {
	if err == sql.ErrNoRows {
		return stdhttp.StatusNotFound
	}
	return stdhttp.StatusInternalServerError
}

func statusForBadRequestOrNotFound(err error) int {
	if errors.Is(err, sql.ErrNoRows) {
		return stdhttp.StatusNotFound
	}
	return stdhttp.StatusBadRequest
}

func writeJSON(w stdhttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w stdhttp.ResponseWriter, err error, status int) {
	writeJSON(w, status, api.ErrorResponse{
		Code:      status,
		Message:   err.Error(),
		Details:   map[string]any{},
		RequestID: "",
	})
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
