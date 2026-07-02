package app

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/agentconfig"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/pkg/agent"
	"github.com/starfederation/datastar-go/datastar"
)

type adminAgentCommandSignals struct {
	SystemPrompt      string `json:"systemPrompt"`
	AdminAgentCommand struct {
		SystemPrompt string `json:"systemPrompt"`
	} `json:"adminAgentCommand"`
}

func (s *Server) getAdminAgentConfig(w http.ResponseWriter, r *http.Request) {
	details, err := s.adminAgentDetails(r.Context())
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (s *Server) updateAdminAgentConfig(w http.ResponseWriter, r *http.Request) {
	var signals adminAgentCommandSignals
	if err := datastar.ReadSignals(r, &signals); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	systemPrompt := signals.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = signals.AdminAgentCommand.SystemPrompt
	}
	prompt, err := agentconfig.NormalizeSystemPrompt(systemPrompt)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	if s.store == nil {
		writeJSONError(w, agentapp.ErrDisabled, http.StatusServiceUnavailable)
		return
	}
	if err := s.store.UpsertSetting(r.Context(), agentconfig.SystemPromptSettingKey, prompt); err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	details, err := s.adminAgentDetails(r.Context())
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (s *Server) adminAgentDetails(ctx context.Context) (api.AdminAgentResponse, error) {
	prompt, err := s.agentSystemPrompt(ctx)
	if err != nil {
		return api.AdminAgentResponse{}, err
	}
	out := api.AdminAgentResponse{
		Enabled:      s.agent != nil && s.agent.Enabled(),
		SystemPrompt: prompt,
	}
	if s.agent != nil {
		out.Model = s.agent.Model()
		out.Tools = adminAgentToolDTOs(s.agent.ToolDefinitions(agentapp.Scope{WorkspaceID: s.defaultWorkspaceID, PrincipalID: "admin", DevAuthBypass: true}))
	}
	return out, nil
}

func (s *Server) agentSystemPrompt(ctx context.Context) (string, error) {
	if s.store == nil {
		return agentconfig.DefaultSystemPrompt, nil
	}
	prompt, err := s.store.GetSetting(ctx, agentconfig.SystemPromptSettingKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return agentconfig.DefaultSystemPrompt, nil
		}
		return "", err
	}
	return agentconfig.NormalizeSystemPrompt(prompt)
}

func adminAgentToolDTOs(tools []agent.ToolDefinition) []api.AdminAgentToolResponse {
	out := make([]api.AdminAgentToolResponse, 0, len(tools))
	for _, tool := range tools {
		out = append(out, api.AdminAgentToolResponse{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: jsonObject(string(tool.InputSchema)),
		})
	}
	return out
}
