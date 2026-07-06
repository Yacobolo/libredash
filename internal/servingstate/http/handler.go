package http

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"os"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/servingstate/activate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/servingstate/validate"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/go-chi/chi/v5"
)

type RuntimeHost interface {
	Reload(ctx context.Context) error
	PrepareServingState(ctx context.Context, servingStateID string) (servingstate.PreparedRuntime, error)
	CommitPrepared(prepared servingstate.PreparedRuntime) error
}

type Repository interface {
	validate.Repository
	activate.Repository
	servingstatefs.ArtifactRepository
	ActiveArtifact(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) (servingstate.State, servingstate.Artifact, error)
	Create(ctx context.Context, input servingstate.CreateInput) (servingstate.State, error)
	List(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) ([]servingstate.State, error)
}

type Principal struct {
	ID string
}

type Options struct {
	Repository          func() (Repository, error)
	WorkspaceRepository func() (workspace.Repository, error)
	AccessRepository    func() (access.Repository, error)
	Runtime             RuntimeHost
	CurrentPrincipal    func(*stdhttp.Request) (Principal, bool)
	ArtifactDir         string
	DataDir             func() string
	DefaultEnvironment  string
	WorkspaceID         func(string) string
}

type Handler struct {
	options Options
}

func NewHandler(options Options) *Handler {
	return &Handler{options: options}
}

func (h *Handler) Create(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var input api.PublishCreateRequest
	if err := decodeOptionalJSONBody(r, &input); err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	workspaceRepo, err := h.workspaceRepository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	if workspaceRepo == nil {
		writeJSONError(w, fmt.Errorf("workspace repository is not configured"), stdhttp.StatusInternalServerError)
		return
	}
	if err := workspaceRepo.Ensure(r.Context(), workspace.EnsureInput{ID: workspace.WorkspaceID(workspaceID), Title: firstNonEmpty(input.Title, workspaceID), Description: input.Description}); err != nil {
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
	row, err := repo.Create(r.Context(), servingstate.CreateInput{WorkspaceID: servingstate.WorkspaceID(workspaceID), Environment: environment, CreatedBy: createdBy})
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	writeJSON(w, stdhttp.StatusCreated, publishDTO(row))
}

func (h *Handler) UploadArtifact(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	servingStateID := chi.URLParam(r, "publish")
	repo, err := h.repository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	row, err := h.servingStateByIDForRequestWorkspace(r, repo, servingstate.ID(servingStateID))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	if err := os.MkdirAll(h.options.ArtifactDir, 0o755); err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	artifactStore := servingstatefs.NewArtifactStore(h.options.ArtifactDir)
	path := artifactStore.UploadPath(row.ID)
	out, err := os.Create(path)
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	size, copyErr := io.Copy(out, stdhttp.MaxBytesReader(w, r.Body, 128<<20))
	closeErr := out.Close()
	if copyErr != nil {
		writeJSONError(w, copyErr, stdhttp.StatusBadRequest)
		return
	}
	if closeErr != nil {
		writeJSONError(w, closeErr, stdhttp.StatusInternalServerError)
		return
	}
	writeJSON(w, stdhttp.StatusOK, map[string]any{"publishId": row.ID, "sizeBytes": size})
}

func (h *Handler) Validate(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	servingStateID := chi.URLParam(r, "publish")
	repo, err := h.repository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	if _, err := h.servingStateByIDForRequestWorkspace(r, repo, servingstate.ID(servingStateID)); err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	service := validate.NewService(repo, servingstatefs.NewArtifactStore(h.options.ArtifactDir), servingstatefs.Validator{DataDir: h.dataDir()})
	row, err := service.Validate(r.Context(), servingstate.ID(servingStateID))
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusBadRequest)
		return
	}
	writeJSON(w, stdhttp.StatusOK, publishDTO(row))
}

func (h *Handler) Activate(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	servingStateID := chi.URLParam(r, "publish")
	repo, err := h.repository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	current, err := h.servingStateByIDForRequestWorkspace(r, repo, servingstate.ID(servingStateID))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	accessRepo, err := h.accessRepository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	var accessReconciler access.WorkspacePolicyReconciler
	if accessRepo != nil {
		if reconciler, ok := accessRepo.(access.WorkspacePolicyReconciler); ok {
			accessReconciler = reconciler
		}
	}
	service := activate.NewServiceWithAccess(repo, h.options.Runtime, servingstatefs.NewAccessPolicyLoader(repo), accessReconciler)
	row, err := service.Activate(r.Context(), servingstate.ID(servingStateID))
	if err != nil {
		writeJSONError(w, err, statusForActivationError(err))
		return
	}
	if current.Status == servingstate.StatusInactive {
		h.recordRollbackAudit(r, accessRepo, row)
	}
	writeJSON(w, stdhttp.StatusOK, publishDTO(row))
}

func (h *Handler) recordRollbackAudit(r *stdhttp.Request, repo access.Repository, row servingstate.State) {
	if repo == nil {
		return
	}
	principalID := ""
	if h.options.CurrentPrincipal != nil {
		if principal, ok := h.options.CurrentPrincipal(r); ok {
			principalID = principal.ID
		}
	}
	_ = repo.RecordAuditEvent(r.Context(), access.AuditEventInput{
		WorkspaceID:   string(row.WorkspaceID),
		PrincipalID:   principalID,
		Action:        "publish.rolled_back",
		TargetType:    "publish",
		TargetID:      string(row.ID),
		Privilege:     access.PrivilegeActivatePublish,
		Status:        "success",
		RequestID:     r.Header.Get("X-Request-Id"),
		CorrelationID: r.Header.Get("X-Correlation-Id"),
		MetadataJSON:  "{}",
	})
}

func (h *Handler) List(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	workspaceID := h.workspaceID(firstNonEmpty(chi.URLParam(r, "workspace"), r.URL.Query().Get("workspace")))
	repo, err := h.repository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	rows, err := repo.List(r.Context(), servingstate.WorkspaceID(workspaceID), requestServingEnvironment(r, ""))
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
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
	writeJSON(w, stdhttp.StatusOK, pagedResponseWithCursor(page, nextCursor))
}

func (h *Handler) Get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	repo, err := h.repository()
	if err != nil {
		writeJSONError(w, err, stdhttp.StatusInternalServerError)
		return
	}
	row, err := h.servingStateByIDForRequestWorkspace(r, repo, servingstate.ID(chi.URLParam(r, "publish")))
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, stdhttp.StatusOK, publishDTO(row))
}

func (h *Handler) servingStateByIDForRequestWorkspace(r *stdhttp.Request, repo Repository, servingStateID servingstate.ID) (servingstate.State, error) {
	row, err := repo.ByID(r.Context(), servingStateID)
	if err != nil {
		return servingstate.State{}, err
	}
	if workspaceID := chi.URLParam(r, "workspace"); workspaceID != "" && row.WorkspaceID != servingstate.WorkspaceID(h.workspaceID(workspaceID)) {
		return servingstate.State{}, servingstate.ErrNotFound
	}
	if row.Environment != requestServingEnvironment(r, "") {
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

func (h *Handler) accessRepository() (access.Repository, error) {
	if h.options.AccessRepository == nil {
		return nil, nil
	}
	return h.options.AccessRepository()
}

func (h *Handler) workspaceID(candidate string) string {
	if h.options.WorkspaceID != nil {
		return h.options.WorkspaceID(candidate)
	}
	return candidate
}

func (h *Handler) dataDir() string {
	if h.options.DataDir == nil {
		return ""
	}
	return h.options.DataDir()
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

func requestServingEnvironment(r *stdhttp.Request, fallback string) servingstate.Environment {
	if query := r.URL.Query().Get("environment"); query != "" {
		fallback = query
	}
	return servingstate.NormalizeEnvironment(servingstate.Environment(fallback))
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

type pageResponse struct {
	NextCursor string `json:"nextCursor"`
}

func pagedResponseWithCursor(items any, nextCursor string) map[string]any {
	return map[string]any{"items": items, "page": pageResponse{NextCursor: nextCursor}}
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

func statusForActivationError(err error) int {
	if errors.Is(err, servingstate.ErrNotFound) {
		return stdhttp.StatusNotFound
	}
	if errors.Is(err, activate.ErrInvalidStatus) {
		return stdhttp.StatusBadRequest
	}
	return stdhttp.StatusInternalServerError
}

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
		return 50, nil
	}
	var limit int
	if _, err := fmt.Sscanf(value, "%d", &limit); err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit < 1 {
		return 0, fmt.Errorf("limit must be at least 1")
	}
	if limit > 100 {
		return 100, nil
	}
	return limit, nil
}

func decodeCursor(token string) (string, string) {
	if token == "" {
		return "", ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", ""
	}
	for i, b := range raw {
		if b == 0 {
			return string(raw[:i]), string(raw[i+1:])
		}
	}
	return "", ""
}

func encodeCursor(createdAt, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "\x00" + id))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
