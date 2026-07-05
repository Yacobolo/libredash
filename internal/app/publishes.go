package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/api"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/servingstate/activate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	"github.com/Yacobolo/libredash/internal/servingstate/validate"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/go-chi/chi/v5"
)

type runtimeReloader interface {
	Reload(ctx context.Context) error
	PrepareServingState(ctx context.Context, servingStateID string) (servingstate.PreparedRuntime, error)
	CommitPrepared(prepared servingstate.PreparedRuntime) error
}

type servingStateRepository interface {
	validate.Repository
	activate.Repository
	activate.ArtifactRepository
	ActiveArtifact(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) (servingstate.State, servingstate.Artifact, error)
	Create(ctx context.Context, input servingstate.CreateInput) (servingstate.State, error)
	List(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) ([]servingstate.State, error)
}

func (s *Server) createPublish(w http.ResponseWriter, r *http.Request) {
	var input api.PublishCreateRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	workspaceRepo, err := s.workspaceRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if workspaceRepo == nil {
		writeJSONError(w, fmt.Errorf("workspace repository is not configured"), http.StatusInternalServerError)
		return
	}
	if err := workspaceRepo.Ensure(r.Context(), workspace.EnsureInput{ID: workspace.WorkspaceID(workspaceID), Title: firstNonEmpty(input.Title, workspaceID), Description: input.Description}); err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	repo, err := s.servingStateRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	createdBy := ""
	if s.auth != nil {
		if principal, ok := s.auth.Principal(r); ok {
			createdBy = principal.ID
		}
	}
	environment := requestServingEnvironment(r, input.Environment)
	state, err := repo.Create(r.Context(), servingstate.CreateInput{WorkspaceID: servingstate.WorkspaceID(workspaceID), Environment: environment, CreatedBy: createdBy})
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, publishDTO(state))
}

func (s *Server) uploadPublishArtifact(w http.ResponseWriter, r *http.Request) {
	servingStateID := chi.URLParam(r, "publish")
	repo, err := s.servingStateRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	state, err := s.servingStateByIDForRequestWorkspace(r, repo, servingstate.ID(servingStateID))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	if err := os.MkdirAll(s.artifactDir, 0o755); err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	artifactStore := servingstatefs.NewArtifactStore(s.artifactDir)
	path := artifactStore.UploadPath(state.ID)
	out, err := os.Create(path)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	size, copyErr := io.Copy(out, http.MaxBytesReader(w, r.Body, 128<<20))
	closeErr := out.Close()
	if copyErr != nil {
		writeJSONError(w, copyErr, http.StatusBadRequest)
		return
	}
	if closeErr != nil {
		writeJSONError(w, closeErr, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"publishId": state.ID, "sizeBytes": size})
}

func (s *Server) validatePublish(w http.ResponseWriter, r *http.Request) {
	servingStateID := chi.URLParam(r, "publish")
	repo, err := s.servingStateRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if _, err := s.servingStateByIDForRequestWorkspace(r, repo, servingstate.ID(servingStateID)); err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	dataDir := ""
	if s.metrics != nil {
		dataDir = s.metrics.DataDir()
	}
	service := validate.NewService(repo, servingstatefs.NewArtifactStore(s.artifactDir), servingstatefs.Validator{DataDir: dataDir})
	state, err := service.Validate(r.Context(), servingstate.ID(servingStateID))
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, publishDTO(state))
}

func (s *Server) activatePublish(w http.ResponseWriter, r *http.Request) {
	servingStateID := chi.URLParam(r, "publish")
	repo, err := s.servingStateRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if _, err := s.servingStateByIDForRequestWorkspace(r, repo, servingstate.ID(servingStateID)); err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	accessRepo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	var accessReconciler access.WorkspacePolicyReconciler
	if accessRepo != nil {
		if reconciler, ok := accessRepo.(access.WorkspacePolicyReconciler); ok {
			accessReconciler = reconciler
		}
	}
	service := activate.NewServiceWithAccess(repo, s.reloader, repo, accessReconciler)
	state, err := service.Activate(r.Context(), servingstate.ID(servingStateID))
	if err != nil {
		writeJSONError(w, err, statusForActivationError(err))
		return
	}
	writeJSON(w, http.StatusOK, publishDTO(state))
}

func (s *Server) listPublishes(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(firstNonEmpty(chi.URLParam(r, "workspace"), r.URL.Query().Get("workspace")))
	repo, err := s.servingStateRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	rows, err := repo.List(r.Context(), servingstate.WorkspaceID(workspaceID), requestServingEnvironment(r, ""))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	response := make([]api.PublishResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, publishDTO(row))
	}
	limit, ok := apiLimitForRequest(w, r)
	if !ok {
		return
	}
	page, nextCursor := pagePublishes(response, limit, r.URL.Query().Get("pageToken"))
	writeJSON(w, http.StatusOK, pagedResponseWithCursor(page, nextCursor))
}

func (s *Server) getPublish(w http.ResponseWriter, r *http.Request) {
	repo, err := s.servingStateRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	state, err := s.servingStateByIDForRequestWorkspace(r, repo, servingstate.ID(chi.URLParam(r, "publish")))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, publishDTO(state))
}

func (s *Server) servingStateByIDForRequestWorkspace(r *http.Request, repo servingStateRepository, servingStateID servingstate.ID) (servingstate.State, error) {
	row, err := repo.ByID(r.Context(), servingStateID)
	if err != nil {
		return servingstate.State{}, err
	}
	if workspaceID := chi.URLParam(r, "workspace"); workspaceID != "" && row.WorkspaceID != servingstate.WorkspaceID(s.workspaceID(workspaceID)) {
		return servingstate.State{}, servingstate.ErrNotFound
	}
	if row.Environment != requestServingEnvironment(r, "") {
		return servingstate.State{}, servingstate.ErrNotFound
	}
	return row, nil
}

func (s *Server) workspaceID(candidate string) string {
	return candidate
}

func (s *Server) servingStateRepository() (servingStateRepository, error) {
	if s.servingStateRepo != nil {
		return s.servingStateRepo, nil
	}
	if s.store == nil {
		return nil, fmt.Errorf("serving state repository is not configured")
	}
	s.servingStateRepo = servingstatesqlite.NewRepository(s.store.SQLDB())
	return s.servingStateRepo, nil
}

func publishDTO(row servingstate.State) api.PublishResponse {
	out := api.PublishResponse{
		ID:          string(row.ID),
		WorkspaceID: string(row.WorkspaceID),
		Environment: string(servingstate.NormalizeEnvironment(row.Environment)),
		Status:      string(row.Status),
		Digest:      row.Digest,
		CreatedAt:   row.CreatedAt,
		Error:       row.Error,
	}
	out.ActivatedAt = row.ActivatedAt
	return out
}

func requestServingEnvironment(r *http.Request, fallback string) servingstate.Environment {
	if query := r.URL.Query().Get("environment"); query != "" {
		fallback = query
	}
	return servingstate.NormalizeEnvironment(servingstate.Environment(fallback))
}

func (s *Server) defaultServingEnvironment() servingstate.Environment {
	return servingstate.NormalizeEnvironment(servingstate.Environment(s.defaultEnvironment))
}

func (s *Server) requestServingEnvironment(r *http.Request) servingstate.Environment {
	return requestServingEnvironment(r, string(s.defaultServingEnvironment()))
}

func pagePublishes(rows []api.PublishResponse, limit int, pageToken string) ([]api.PublishResponse, string) {
	cursorCreatedAt, cursorID := decodeCursor(pageToken)
	start := 0
	if cursorCreatedAt != "" && cursorID != "" {
		for i, row := range rows {
			if row.CreatedAt == cursorCreatedAt && row.ID == cursorID {
				start = i + 1
				break
			}
		}
	}
	if start > len(rows) {
		start = len(rows)
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	nextCursor := ""
	if end < len(rows) && end > start {
		last := rows[end-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return rows[start:end], nextCursor
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, err error, status int) {
	writeJSON(w, status, api.ErrorResponse{
		Code:      status,
		Message:   err.Error(),
		Details:   map[string]any{},
		RequestID: "",
	})
}

func decodeOptionalJSONBody(r *http.Request, dst any) error {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("malformed JSON: %w", err)
	}
	return fmt.Errorf("malformed JSON: multiple JSON values")
}

func statusForNotFound(err error) int {
	if err == sql.ErrNoRows || errors.Is(err, servingstate.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func statusForActivationError(err error) int {
	if errors.Is(err, servingstate.ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, activate.ErrInvalidStatus) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
