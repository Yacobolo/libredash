package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	manageddatabinding "github.com/Yacobolo/libredash/internal/manageddata/binding"
	"github.com/Yacobolo/libredash/internal/release"
	"github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/servingstate/validate"
	"github.com/Yacobolo/libredash/internal/staticasset"
)

func (s *Server) releaseService() (*release.Service, error) {
	if s.store == nil {
		return nil, fmt.Errorf("platform store is not configured")
	}
	states, err := s.servingStateRepository()
	if err != nil {
		return nil, err
	}
	workspaces, err := s.workspaceRepository()
	if err != nil || workspaces == nil {
		return nil, fmt.Errorf("workspace repository is not configured: %w", err)
	}
	store := servingstatefs.NewArtifactStore(s.artifactDir)
	hooks := []validate.Hook{}
	if s.managedDataBindingRepo != nil {
		binder, err := manageddatabinding.New(s.managedDataBindingRepo)
		if err != nil {
			return nil, err
		}
		hooks = append(hooks, binder)
	}
	validator := validate.NewService(states, store, servingstatefs.Validator{}, hooks...)
	return release.NewService(release.ServiceOptions{
		Releases: s.releaseRepository(), States: states, Workspaces: workspaces,
		Artifacts: store, Validator: validator, Environment: servingstate.Environment(s.defaultEnvironment),
	})
}

func (a apiGenAdapter) GetCapabilities(w http.ResponseWriter, r *http.Request) {
	writeAPIJSON(w, http.StatusOK, apigenapi.CapabilitiesResponse{
		ApiVersion: "v1", BuildVersion: staticasset.Version(), Authentication: []apigenapi.AuthenticationMode{apigenapi.AuthenticationModeBearer}, Environment: a.server.defaultEnvironment,
		QueryFormats:    []apigenapi.QueryFormat{apigenapi.QueryFormatApplicationJson, apigenapi.QueryFormatApplicationVndApacheArrowStream},
		UploadProtocols: []apigenapi.UploadProtocol{apigenapi.UploadProtocolTus, apigenapi.UploadProtocolS3Multipart},
		VisualShapes: []apigenapi.VisualShape{
			apigenapi.VisualShapeCategoryValue, apigenapi.VisualShapeCategorySeriesValue, apigenapi.VisualShapeCategoryMultiMeasure,
			apigenapi.VisualShapeCategoryDelta, apigenapi.VisualShapeBinnedMeasure, apigenapi.VisualShapeHierarchy,
			apigenapi.VisualShapeSingleValue, apigenapi.VisualShapeMatrix, apigenapi.VisualShapeGraph,
			apigenapi.VisualShapeGeo, apigenapi.VisualShapeOhlc, apigenapi.VisualShapeDistribution,
		},
	})
}

func (a apiGenAdapter) CreateRelease(w http.ResponseWriter, r *http.Request, project string, headers apigenapi.GenCreateReleaseHeaders) {
	principal, ok := currentPrincipal(a.server, r)
	if !ok {
		writeAPIProblem(w, r, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Bearer authentication is required", nil)
		return
	}
	var body apigenapi.ReleaseCreateRequest
	if err := decodeAPIBody(w, r, &body); err != nil {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_JSON", err.Error(), nil)
		return
	}
	service, err := a.server.releaseService()
	if err != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "RELEASE_SERVICE_UNAVAILABLE", "Release service is unavailable", nil)
		return
	}
	input := release.CreateInput{ProjectID: project, ProjectDigest: body.ProjectDigest, IdempotencyKey: headers.IdempotencyKey, CreatedBy: principal.ID}
	for _, item := range body.Workspaces {
		input.Workspaces = append(input.Workspaces, release.WorkspaceManifest{WorkspaceID: item.Workspace, ArtifactDigest: item.ArtifactDigest})
	}
	for _, item := range body.Connections {
		input.Connections = append(input.Connections, release.ConnectionPin{ConnectionID: item.Connection, RevisionID: item.RevisionId})
	}
	created, err := service.Create(r.Context(), input)
	if err != nil {
		writeReleaseError(w, r, err)
		return
	}
	w.Header().Set("Location", releaseLocation(project, created.ID))
	writeAPIJSON(w, http.StatusCreated, releaseResponse(created))
}

func (a apiGenAdapter) ListReleases(w http.ResponseWriter, r *http.Request, project string, _ apigenapi.GenListReleasesParams) {
	service, err := a.server.releaseService()
	if err != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "RELEASE_SERVICE_UNAVAILABLE", "Release service is unavailable", nil)
		return
	}
	rows, err := service.List(r.Context(), project)
	if err != nil {
		writeReleaseError(w, r, err)
		return
	}
	items := make([]apigenapi.ReleaseResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, releaseResponse(row))
	}
	writeAPIJSON(w, http.StatusOK, apigenapi.ReleaseListResponse{Items: items, Page: apigenapi.PageInfo{}})
}

func (a apiGenAdapter) GetRelease(w http.ResponseWriter, r *http.Request, project, releaseID string) {
	service, err := a.server.releaseService()
	if err != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "RELEASE_SERVICE_UNAVAILABLE", "Release service is unavailable", nil)
		return
	}
	row, err := service.Get(r.Context(), project, releaseID)
	if err != nil {
		writeReleaseError(w, r, err)
		return
	}
	w.Header().Set("ETag", strongETag(row.RequestDigest+":"+string(row.Status)))
	writeAPIJSON(w, http.StatusOK, releaseResponse(row))
}

func (a apiGenAdapter) UploadReleaseArtifact(w http.ResponseWriter, r *http.Request, project, releaseID, workspaceID string, headers apigenapi.GenUploadReleaseArtifactHeaders) {
	if headers.ContentType != "application/octet-stream" {
		writeAPIProblem(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Release artifacts require application/octet-stream", nil)
		return
	}
	service, err := a.server.releaseService()
	if err != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "RELEASE_SERVICE_UNAVAILABLE", "Release service is unavailable", nil)
		return
	}
	artifact, err := service.UploadArtifact(r.Context(), project, releaseID, workspaceID, headers.ContentDigest, http.MaxBytesReader(w, r.Body, servingstatefs.MaxUploadBytes))
	if err != nil {
		writeReleaseError(w, r, err)
		return
	}
	w.Header().Set("Location", releaseLocation(project, releaseID)+"/workspaces/"+workspaceID+"/artifact")
	writeAPIJSON(w, http.StatusCreated, apigenapi.ReleaseArtifactResponse{ReleaseId: releaseID, WorkspaceId: workspaceID, Digest: artifact.ExpectedDigest, SizeBytes: artifact.SizeBytes})
}

func (a apiGenAdapter) FinalizeRelease(w http.ResponseWriter, r *http.Request, project, releaseID string, _ apigenapi.GenFinalizeReleaseHeaders) {
	service, err := a.server.releaseService()
	if err != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "RELEASE_SERVICE_UNAVAILABLE", "Release service is unavailable", nil)
		return
	}
	row, err := service.BeginFinalization(r.Context(), project, releaseID)
	if err != nil {
		writeReleaseError(w, r, err)
		return
	}
	w.Header().Set("Location", releaseLocation(project, releaseID))
	writeAPIJSON(w, http.StatusAccepted, releaseResponse(row))
	go a.validateRelease(service, project, releaseID)
}

func (a apiGenAdapter) validateRelease(service *release.Service, project, releaseID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if _, err := service.ValidateFinalization(ctx, project, releaseID); err != nil && a.server.logger != nil {
		a.server.logger.Error("asynchronous release validation failed", "project", project, "release", releaseID, "error", err)
	}
}

func (a apiGenAdapter) ListReleaseEvents(w http.ResponseWriter, r *http.Request, project, releaseID string, params apigenapi.GenListReleaseEventsParams, _ apigenapi.GenListReleaseEventsHeaders) {
	repo := a.server.releaseRepository()
	row, err := repo.Get(r.Context(), project, releaseID)
	if err != nil {
		writeReleaseError(w, r, err)
		return
	}
	writeAsyncEventPage(w, r, releaseEvents(row), params.Limit, params.PageToken, "release:"+project+":"+releaseID, func(ctx context.Context) ([]apigenapi.AsyncEventResponse, error) {
		latest, err := repo.Get(ctx, project, releaseID)
		return releaseEvents(latest), err
	})
}

func releaseResponse(row release.Release) apigenapi.ReleaseResponse {
	result := apigenapi.ReleaseResponse{
		Id: row.ID, ProjectId: row.ProjectID, ProjectDigest: row.ProjectDigest, Status: apigenapi.ReleaseStatus(row.Status),
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, Workspaces: make([]apigenapi.ReleaseWorkspaceManifest, 0, len(row.Manifest.Workspaces)),
		Connections: make([]apigenapi.ReleaseConnectionPin, 0, len(row.Manifest.Connections)),
	}
	for _, item := range row.Manifest.Workspaces {
		mapped := apigenapi.ReleaseWorkspaceManifest{Workspace: item.WorkspaceID, ArtifactDigest: item.ArtifactDigest}
		if item.ServingStateID != "" {
			mapped.ServingStateId = &item.ServingStateID
		}
		result.Workspaces = append(result.Workspaces, mapped)
	}
	for _, item := range row.Manifest.Connections {
		result.Connections = append(result.Connections, apigenapi.ReleaseConnectionPin{Connection: item.ConnectionID, RevisionId: item.RevisionID})
	}
	if row.FinalizedAt != "" {
		result.FinalizedAt = &row.FinalizedAt
	}
	if row.Error != "" {
		result.Error = &row.Error
	}
	return result
}

func releaseLocation(project, releaseID string) string {
	return "/api/v1/projects/" + project + "/releases/" + releaseID
}

func writeReleaseError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
	switch {
	case errors.Is(err, release.ErrInvalid):
		status, code = http.StatusUnprocessableEntity, "INVALID_RELEASE"
	case errors.Is(err, release.ErrNotFound):
		status, code = http.StatusNotFound, "RELEASE_NOT_FOUND"
	case errors.Is(err, release.ErrIncomplete), errors.Is(err, release.ErrConflict), errors.Is(err, release.ErrImmutable):
		status, code = http.StatusConflict, "RELEASE_CONFLICT"
	case errors.Is(err, release.ErrDigest):
		status, code = http.StatusUnprocessableEntity, "CONTENT_DIGEST_MISMATCH"
	}
	detail := err.Error()
	if status == http.StatusInternalServerError {
		detail = "The release request could not be completed"
	}
	writeAPIProblem(w, r, status, code, detail, nil)
}

func decodeAPIBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain exactly one JSON value")
	}
	return nil
}

func writeAPIJSON(w http.ResponseWriter, status int, value any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string, violations []apigenapi.ProblemFieldError) {
	if violations == nil {
		violations = []apigenapi.ProblemFieldError{}
	}
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = w.Header().Get("X-Request-ID")
	}
	w.Header().Set("Content-Type", "application/problem+json")
	writeAPIJSON(w, status, apigenapi.ProblemDetails{
		Type: "https://libredash.dev/problems/" + strings.ToLower(code), Title: http.StatusText(status), Status: int32(status),
		Detail: detail, Instance: r.URL.Path, Code: code, RequestId: requestID, Errors: violations,
	})
}

func strongETag(value string) string {
	sum := sha256.Sum256([]byte(value))
	return strconv.Quote(hex.EncodeToString(sum[:]))
}

func _releaseContext(_ context.Context) {}
