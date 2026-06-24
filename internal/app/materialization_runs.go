package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/go-chi/chi/v5"
)

type materializationRunRequest struct {
	ModelID      string `json:"modelId"`
	DeploymentID string `json:"deploymentId,omitempty"`
}

func (s *Server) createMaterializationRun(w http.ResponseWriter, r *http.Request) {
	service, workspaceID, ok := s.materializationRunService(w, r)
	if !ok {
		return
	}
	var input materializationRunRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	run, err := service.Enqueue(r.Context(), materialize.RunInput{
		WorkspaceID:  workspaceID,
		ModelID:      input.ModelID,
		DeploymentID: input.DeploymentID,
	})
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	go func() {
		ctx := context.Background()
		if _, err := service.Execute(ctx, workspaceID, run.ID); err != nil && s.logger != nil {
			s.logger.WarnContext(ctx, "async materialization refresh failed", "workspace", workspaceID, "run", run.ID, "error", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) listMaterializationRuns(w http.ResponseWriter, r *http.Request) {
	repo, workspaceID, ok := s.materializationRunRepository(w, r)
	if !ok {
		return
	}
	runs, err := repo.ListRuns(r.Context(), workspaceID, materializationRunPageFromRequest(r))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, pagedResponse(runs))
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

func (s *Server) materializationRunService(w http.ResponseWriter, r *http.Request) (materialize.RunService, string, bool) {
	repo, workspaceID, ok := s.materializationRunRepository(w, r)
	if !ok {
		return materialize.RunService{}, "", false
	}
	if s.metrics == nil {
		writeJSONError(w, fmt.Errorf("materialization refresh runner is not configured"), http.StatusServiceUnavailable)
		return materialize.RunService{}, "", false
	}
	return materialize.RunService{Repo: repo, Runner: s.metrics}, workspaceID, true
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

func materializationRunPageFromRequest(r *http.Request) materialize.RunPage {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return materialize.RunPage{Limit: limit, After: firstNonEmpty(r.URL.Query().Get("pageToken"), r.URL.Query().Get("after"))}
}
