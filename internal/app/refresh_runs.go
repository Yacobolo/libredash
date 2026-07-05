package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/execution"
	"github.com/go-chi/chi/v5"
)

type refreshRunRequest struct {
	ModelID        string `json:"modelId"`
	ServingStateID string `json:"servingStateId,omitempty"`
	TargetType     string `json:"targetType,omitempty"`
	TargetID       string `json:"targetId,omitempty"`
	TriggerType    string `json:"triggerType,omitempty"`
	ParentRunID    string `json:"parentRunId,omitempty"`
}

func (s *Server) createRefreshRun(w http.ResponseWriter, r *http.Request) {
	repo, workspaceID, ok := s.refreshRunRepository(w, r)
	if !ok {
		return
	}
	if s.metrics == nil {
		writeJSONError(w, fmt.Errorf("refresh runner is not configured"), http.StatusServiceUnavailable)
		return
	}
	var input refreshRunRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	principal, _ := currentPrincipal(s, r)
	run, err := repo.CreateRun(r.Context(), materialize.RunInput{
		WorkspaceID:    workspaceID,
		ModelID:        input.ModelID,
		ServingStateID: input.ServingStateID,
		PrincipalID:    principal.ID,
		TargetType:     input.TargetType,
		TargetID:       input.TargetID,
		TriggerType:    input.TriggerType,
		ParentRunID:    input.ParentRunID,
	})
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	s.dispatchQueuedRefreshJobs(context.Background())
	writeJSON(w, http.StatusAccepted, refreshRunDTO(run))
}

func (s *Server) executionService() *execution.Service {
	if s.executor == nil {
		s.executor = execution.New(execution.DefaultConfig())
	}
	return s.executor
}

func (s *Server) listRefreshRuns(w http.ResponseWriter, r *http.Request) {
	repo, workspaceID, ok := s.refreshRunRepository(w, r)
	if !ok {
		return
	}
	limit, ok := apiLimitForRequest(w, r)
	if !ok {
		return
	}
	runs, err := repo.ListRuns(r.Context(), workspaceID, materialize.RunPage{
		Limit: limit + 1,
		After: firstNonEmpty(r.URL.Query().Get("pageToken"), r.URL.Query().Get("after")),
	})
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	nextCursor := ""
	if len(runs) > limit {
		nextCursor = runs[limit-1].ID
		runs = runs[:limit]
	}
	items := make([]refreshRunResponse, 0, len(runs))
	for _, run := range runs {
		items = append(items, refreshRunDTO(run))
	}
	writeJSON(w, http.StatusOK, pagedResponseWithCursor(items, nextCursor))
}

func (s *Server) getRefreshRun(w http.ResponseWriter, r *http.Request) {
	repo, workspaceID, ok := s.refreshRunRepository(w, r)
	if !ok {
		return
	}
	run, err := repo.GetRun(r.Context(), workspaceID, chi.URLParam(r, "run"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, refreshRunDTO(run))
}

type refreshRunResponse struct {
	ID                   string `json:"id"`
	WorkspaceID          string `json:"workspaceId"`
	ModelID              string `json:"modelId"`
	ServingStateID       string `json:"servingStateId,omitempty"`
	PrincipalID          string `json:"principalId,omitempty"`
	PrincipalDisplayName string `json:"principalDisplayName,omitempty"`
	TargetType           string `json:"targetType"`
	TargetID             string `json:"targetId"`
	TriggerType          string `json:"triggerType"`
	ParentRunID          string `json:"parentRunId,omitempty"`
	Status               string `json:"status"`
	Error                string `json:"error,omitempty"`
	CreatedAt            string `json:"createdAt"`
	StartedAt            string `json:"startedAt,omitempty"`
	FinishedAt           string `json:"finishedAt,omitempty"`
}

func refreshRunDTO(run materialize.RunRecord) refreshRunResponse {
	return refreshRunResponse{
		ID:                   run.ID,
		WorkspaceID:          run.WorkspaceID,
		ModelID:              run.ModelID,
		ServingStateID:       run.ServingStateID,
		PrincipalID:          run.PrincipalID,
		PrincipalDisplayName: run.PrincipalDisplayName,
		TargetType:           run.TargetType,
		TargetID:             run.TargetID,
		TriggerType:          run.TriggerType,
		ParentRunID:          run.ParentRunID,
		Status:               run.Status,
		Error:                run.Error,
		CreatedAt:            run.CreatedAt,
		StartedAt:            run.StartedAt,
		FinishedAt:           run.FinishedAt,
	}
}

func (s *Server) refreshRunRepository(w http.ResponseWriter, r *http.Request) (*materialize.SQLRunRepository, string, bool) {
	if s.store == nil {
		writeJSONError(w, fmt.Errorf("platform store is required"), http.StatusServiceUnavailable)
		return nil, "", false
	}
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	if workspaceID == "" {
		writeJSONError(w, fmt.Errorf("workspace id is required"), http.StatusBadRequest)
		return nil, "", false
	}
	return materialize.NewSQLRunRepository(s.store.SQLDB()), workspaceID, true
}
