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

type materializationRunRequest struct {
	ModelID      string `json:"modelId"`
	DeploymentID string `json:"deploymentId,omitempty"`
	TargetType   string `json:"targetType,omitempty"`
	TargetID     string `json:"targetId,omitempty"`
	TriggerType  string `json:"triggerType,omitempty"`
	ParentRunID  string `json:"parentRunId,omitempty"`
}

func (s *Server) createMaterializationRun(w http.ResponseWriter, r *http.Request) {
	repo, workspaceID, ok := s.materializationRunRepository(w, r)
	if !ok {
		return
	}
	if s.metrics == nil {
		writeJSONError(w, fmt.Errorf("materialization refresh runner is not configured"), http.StatusServiceUnavailable)
		return
	}
	var input materializationRunRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	principal, _ := currentPrincipal(s, r)
	run, err := repo.CreateRun(r.Context(), materialize.RunInput{
		WorkspaceID:  workspaceID,
		ModelID:      input.ModelID,
		DeploymentID: input.DeploymentID,
		PrincipalID:  principal.ID,
		TargetType:   input.TargetType,
		TargetID:     input.TargetID,
		TriggerType:  input.TriggerType,
		ParentRunID:  input.ParentRunID,
	})
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	s.dispatchQueuedMaterializationJobs(context.Background())
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) executionService() *execution.Service {
	if s.executor == nil {
		s.executor = execution.New(execution.DefaultConfig())
	}
	return s.executor
}

func (s *Server) listMaterializationRuns(w http.ResponseWriter, r *http.Request) {
	repo, workspaceID, ok := s.materializationRunRepository(w, r)
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
	writeJSON(w, http.StatusOK, pagedResponseWithCursor(runs, nextCursor))
}

func (s *Server) getMaterializationRun(w http.ResponseWriter, r *http.Request) {
	repo, workspaceID, ok := s.materializationRunRepository(w, r)
	if !ok {
		return
	}
	run, err := repo.GetRun(r.Context(), workspaceID, chi.URLParam(r, "run"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) materializationRunRepository(w http.ResponseWriter, r *http.Request) (*materialize.SQLRunRepository, string, bool) {
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
