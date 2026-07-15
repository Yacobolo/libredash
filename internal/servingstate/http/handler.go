package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"

	"github.com/Yacobolo/libredash/internal/api"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	manageddatabinding "github.com/Yacobolo/libredash/internal/manageddata/binding"
	"github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/servingstate/validate"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Repository interface {
	validate.Repository
	Create(ctx context.Context, input servingstate.CreateInput) (servingstate.State, error)
}

type Principal struct {
	ID string
}

type Options struct {
	Repository          func() (Repository, error)
	BindingRepository   func() (manageddatabinding.Repository, error)
	WorkspaceRepository func() (workspace.Repository, error)
	CurrentPrincipal    func(*stdhttp.Request) (Principal, bool)
	ArtifactDir         string
	DefaultEnvironment  string
	WorkspaceID         func(string) string
}

type Handler struct {
	options Options
}

func NewHandler(options Options) *Handler {
	return &Handler{options: options}
}

func (h *Handler) CreateCandidate(w stdhttp.ResponseWriter, r *stdhttp.Request, projectID, workspaceName string) {
	var input apigenapi.DeploymentCandidateCreateRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	if input.Environment == "" {
		writeJSONError(w, fmt.Errorf("environment is required"), stdhttp.StatusBadRequest)
		return
	}
	workspaceID := h.workspaceID(workspaceName)
	workspaceRepo, err := h.workspaceRepository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	if workspaceRepo == nil {
		writeJSONError(w, fmt.Errorf("workspace repository is not configured"), stdhttp.StatusInternalServerError)
		return
	}
	title := workspaceID
	if input.Title != nil && *input.Title != "" {
		title = *input.Title
	}
	description := ""
	if input.Description != nil {
		description = *input.Description
	}
	if err := workspaceRepo.Ensure(r.Context(), workspace.EnsureInput{ID: workspace.WorkspaceID(workspaceID), Title: title, Description: description}); err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	repo, err := h.repository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	createdBy := ""
	if h.options.CurrentPrincipal != nil {
		if principal, ok := h.options.CurrentPrincipal(r); ok {
			createdBy = principal.ID
		}
	}
	environment := requestServingEnvironment(r, input.Environment)
	row, err := repo.Create(r.Context(), servingstate.CreateInput{WorkspaceID: servingstate.WorkspaceID(workspaceID), ProjectID: projectID, Environment: environment, CreatedBy: createdBy})
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	writeJSON(w, stdhttp.StatusCreated, candidateDTO(row))
}

func (h *Handler) UploadCandidateArtifact(w stdhttp.ResponseWriter, r *stdhttp.Request, projectID, workspace string, servingStateID servingstate.ID) {
	repo, err := h.repository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	row, err := h.servingStateByIDForScope(r.Context(), repo, servingStateID, projectID, workspace)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	artifactStore := servingstatefs.NewArtifactStore(h.options.ArtifactDir)
	size, copyErr := artifactStore.SaveUpload(r.Context(), row.ID, stdhttp.MaxBytesReader(w, r.Body, servingstatefs.MaxUploadBytes))
	if copyErr != nil {
		var maxBytesErr *stdhttp.MaxBytesError
		if errors.As(copyErr, &maxBytesErr) {
			writeJSONError(w, copyErr, stdhttp.StatusRequestEntityTooLarge)
			return
		}
		writeJSONError(w, copyErr, stdhttp.StatusInternalServerError)
		return
	}
	writeJSON(w, stdhttp.StatusOK, map[string]any{"candidateId": row.ID, "sizeBytes": size})
}

func (h *Handler) ValidateCandidate(w stdhttp.ResponseWriter, r *stdhttp.Request, projectID, workspace string, servingStateID servingstate.ID) {
	repo, err := h.repository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	if _, err := h.servingStateByIDForScope(r.Context(), repo, servingStateID, projectID, workspace); err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	if h.options.BindingRepository == nil {
		writeJSONError(w, fmt.Errorf("managed data binding repository is not configured"), stdhttp.StatusInternalServerError)
		return
	}
	bindingRepository, err := h.options.BindingRepository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	binder, err := manageddatabinding.New(bindingRepository)
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	service := validate.NewService(repo, servingstatefs.NewArtifactStore(h.options.ArtifactDir), servingstatefs.Validator{}, binder)
	row, err := service.Validate(r.Context(), servingStateID)
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	writeJSON(w, stdhttp.StatusOK, candidateDTO(row))
}

func (h *Handler) servingStateByIDForScope(ctx context.Context, repo Repository, servingStateID servingstate.ID, projectID, workspaceID string) (servingstate.State, error) {
	row, err := repo.ByID(ctx, servingStateID)
	if err != nil {
		return servingstate.State{}, err
	}
	if workspaceID != "" && row.WorkspaceID != servingstate.WorkspaceID(h.workspaceID(workspaceID)) {
		return servingstate.State{}, servingstate.ErrNotFound
	}
	if projectID != "" && row.ProjectID != projectID {
		return servingstate.State{}, servingstate.ErrNotFound
	}
	return row, nil
}

func (h *Handler) repository() (Repository, error) {
	if h.options.Repository == nil {
		return nil, fmt.Errorf("serving state repository is not configured")
	}
	return h.options.Repository()
}

func (h *Handler) workspaceRepository() (workspace.Repository, error) {
	if h.options.WorkspaceRepository == nil {
		return nil, nil
	}
	return h.options.WorkspaceRepository()
}

func (h *Handler) workspaceID(candidate string) string {
	if h.options.WorkspaceID != nil {
		return h.options.WorkspaceID(candidate)
	}
	return candidate
}

func candidateDTO(row servingstate.State) apigenapi.DeploymentCandidateResponse {
	out := apigenapi.DeploymentCandidateResponse{
		Id:          string(row.ID),
		Project:     row.ProjectID,
		Workspace:   string(row.WorkspaceID),
		Environment: string(servingstate.NormalizeEnvironment(row.Environment)),
		Status:      string(row.Status),
		Digest:      row.Digest,
		CreatedAt:   row.CreatedAt,
	}
	if row.Error != "" {
		out.Error = &row.Error
	}
	return out
}

func requestServingEnvironment(r *stdhttp.Request, fallback string) servingstate.Environment {
	if query := r.URL.Query().Get("environment"); query != "" {
		fallback = query
	}
	return servingstate.NormalizeEnvironment(servingstate.Environment(fallback))
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

func decodeOptionalJSONBody(r *stdhttp.Request, dst any) error {
	if r.Body == nil || r.Body == stdhttp.NoBody {
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
		return stdhttp.StatusNotFound
	}
	return stdhttp.StatusInternalServerError
}
