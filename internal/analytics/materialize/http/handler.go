package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/go-chi/chi/v5"
)

type Principal struct {
	ID string
}

type Handler struct {
	Repository       func() (materialize.RunRepository, error)
	RunnerConfigured func() bool
	DispatchQueued   func()
	CurrentPrincipal func(*nethttp.Request) (Principal, bool)
	WorkspaceID      func(string) string
	RunCreated       func(context.Context, materialize.RunRecord) error
}

type materializationRunRequest struct {
	ModelID        string `json:"modelId"`
	ServingStateID string `json:"servingStateId,omitempty"`
	TargetType     string `json:"targetType,omitempty"`
	TargetID       string `json:"targetId,omitempty"`
	TriggerType    string `json:"triggerType,omitempty"`
	ParentRunID    string `json:"parentRunId,omitempty"`
	RetryOf        string `json:"retryOf,omitempty"`
}

func (h Handler) CreateRun(w nethttp.ResponseWriter, r *nethttp.Request) {
	repo, workspaceID, ok := h.runRepository(w, r)
	if !ok {
		return
	}
	if h.RunnerConfigured != nil && !h.RunnerConfigured() {
		writeJSONError(w, fmt.Errorf("materialization refresh runner is not configured"), nethttp.StatusServiceUnavailable)
		return
	}
	var input materializationRunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	principalID := ""
	if h.CurrentPrincipal != nil {
		if principal, ok := h.CurrentPrincipal(r); ok {
			principalID = principal.ID
		}
	}
	if input.RetryOf != "" {
		prior, err := repo.GetRun(r.Context(), workspaceID, input.RetryOf)
		if err != nil {
			writeJSONError(w, fmt.Errorf("retryOf does not identify a refresh run in this workspace"), nethttp.StatusUnprocessableEntity)
			return
		}
		if prior.Status == materialize.RunStatusQueued || prior.Status == materialize.RunStatusRunning {
			writeJSONError(w, fmt.Errorf("retryOf refresh run is not terminal"), nethttp.StatusConflict)
			return
		}
		if input.ModelID == "" {
			input.ModelID = prior.ModelID
		}
		if input.ServingStateID == "" {
			input.ServingStateID = prior.ServingStateID
		}
		if input.TargetType == "" {
			input.TargetType = prior.TargetType
		}
		if input.TargetID == "" {
			input.TargetID = prior.TargetID
		}
	}
	run, err := repo.CreateRun(r.Context(), materialize.RunInput{
		WorkspaceID:    workspaceID,
		ModelID:        input.ModelID,
		ServingStateID: input.ServingStateID,
		PrincipalID:    principalID,
		TargetType:     input.TargetType,
		TargetID:       input.TargetID,
		TriggerType:    input.TriggerType,
		ParentRunID:    input.ParentRunID,
		RetryOf:        input.RetryOf,
	})
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	if h.RunCreated != nil {
		if err := h.RunCreated(r.Context(), run); err != nil {
			writeJSONError(w, err, nethttp.StatusServiceUnavailable)
			return
		}
	}
	if h.DispatchQueued != nil {
		h.DispatchQueued()
	}
	w.Header().Set("Location", strings.TrimSuffix(r.URL.Path, "/")+"/"+run.ID)
	writeJSON(w, nethttp.StatusAccepted, run)
}

func (h Handler) ListRuns(w nethttp.ResponseWriter, r *nethttp.Request) {
	repo, workspaceID, ok := h.runRepository(w, r)
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
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	nextCursor := ""
	if len(runs) > limit {
		nextCursor = runs[limit-1].ID
		runs = runs[:limit]
	}
	writeJSON(w, nethttp.StatusOK, pagedResponseWithCursor(runs, nextCursor))
}

func (h Handler) GetRun(w nethttp.ResponseWriter, r *nethttp.Request) {
	repo, workspaceID, ok := h.runRepository(w, r)
	if !ok {
		return
	}
	run, err := repo.GetRun(r.Context(), workspaceID, chi.URLParam(r, "run"))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, nethttp.StatusOK, run)
}

func (h Handler) runRepository(w nethttp.ResponseWriter, r *nethttp.Request) (materialize.RunRepository, string, bool) {
	if h.Repository == nil {
		writeJSONError(w, fmt.Errorf("platform store is required"), nethttp.StatusServiceUnavailable)
		return nil, "", false
	}
	repo, err := h.Repository()
	if err != nil {
		writeJSONError(w, err, nethttp.StatusServiceUnavailable)
		return nil, "", false
	}
	workspaceID := chi.URLParam(r, "workspace")
	if h.WorkspaceID != nil {
		workspaceID = h.WorkspaceID(workspaceID)
	}
	if workspaceID == "" {
		writeJSONError(w, fmt.Errorf("workspace id is required"), nethttp.StatusBadRequest)
		return nil, "", false
	}
	return repo, workspaceID, true
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

func apiLimitForRequest(w nethttp.ResponseWriter, r *nethttp.Request) (int, bool) {
	limit, err := parseAPILimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
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
	if err == sql.ErrNoRows || errors.Is(err, sql.ErrNoRows) {
		return nethttp.StatusNotFound
	}
	return nethttp.StatusInternalServerError
}

func writeJSON(w nethttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w nethttp.ResponseWriter, err error, status int) {
	writeJSON(w, status, api.ErrorResponse{
		Code:      status,
		Message:   err.Error(),
		Details:   map[string]any{},
		RequestID: "",
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
