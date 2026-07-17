package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/Yacobolo/libredash/internal/manageddata/control"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func (a apiGenAdapter) ListProjects(w http.ResponseWriter, r *http.Request, params apigenapi.GenListProjectsParams) {
	repo := a.server.releaseRepository()
	if repo == nil {
		writeAPIJSON(w, http.StatusOK, apigenapi.ProjectListResponse{Items: []apigenapi.ProjectResponse{}, Page: apigenapi.PageInfo{}})
		return
	}
	rows, err := repo.ListProjects(r.Context())
	if err != nil {
		writeAPIProblem(w, r, http.StatusInternalServerError, "PROJECT_LIST_FAILED", "Projects could not be loaded", nil)
		return
	}
	items := []apigenapi.ProjectResponse{}
	for _, row := range rows {
		item := apigenapi.ProjectResponse{Id: row.ID, Title: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
		if row.LatestReleaseID != "" {
			item.LatestReleaseId = &row.LatestReleaseID
		}
		if row.ActiveDeploymentID != "" {
			item.ActiveDeploymentId = &row.ActiveDeploymentID
		}
		items = append(items, item)
	}
	page, next, err := keysetPage(items, params.Limit, params.PageToken, func(item apigenapi.ProjectResponse) string { return item.Id })
	if err != nil {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", err.Error(), nil)
		return
	}
	writeAPIJSON(w, http.StatusOK, apigenapi.ProjectListResponse{Items: page, Page: apigenapi.PageInfo{NextCursor: next}})
}

func (a apiGenAdapter) GetProject(w http.ResponseWriter, r *http.Request, projectID string) {
	repo := a.server.releaseRepository()
	if repo == nil {
		writeAPIProblem(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found", nil)
		return
	}
	row, err := repo.GetProject(r.Context(), projectID)
	if err != nil {
		writeAPIProblem(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found", nil)
		return
	}
	item := apigenapi.ProjectResponse{Id: projectID, Title: projectID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.LatestReleaseID != "" {
		item.LatestReleaseId = &row.LatestReleaseID
	}
	if row.ActiveDeploymentID != "" {
		item.ActiveDeploymentId = &row.ActiveDeploymentID
	}
	writeAPIJSON(w, http.StatusOK, item)
}

func (a apiGenAdapter) ListProjectWorkspaces(w http.ResponseWriter, r *http.Request, projectID string, params apigenapi.GenListProjectWorkspacesParams) {
	repo := a.server.releaseRepository()
	if repo == nil {
		writeAPIProblem(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found", nil)
		return
	}
	rows, err := repo.ListProjectWorkspaces(r.Context(), projectID, a.server.defaultEnvironment)
	if err != nil {
		writeAPIProblem(w, r, http.StatusInternalServerError, "PROJECT_WORKSPACES_FAILED", "Project workspaces could not be loaded", nil)
		return
	}
	items := []apigenapi.ProjectWorkspaceResponse{}
	for _, row := range rows {
		item := apigenapi.ProjectWorkspaceResponse{Id: row.ID, Title: row.Title}
		if row.Description != "" {
			item.Description = &row.Description
		}
		if row.ActiveServingStateID != "" {
			item.ActiveServingStateId = &row.ActiveServingStateID
		}
		items = append(items, item)
	}
	page, next, err := keysetPage(items, params.Limit, params.PageToken, func(item apigenapi.ProjectWorkspaceResponse) string { return item.Id })
	if err != nil {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", err.Error(), nil)
		return
	}
	writeAPIJSON(w, http.StatusOK, apigenapi.ProjectWorkspaceListResponse{Items: page, Page: apigenapi.PageInfo{NextCursor: next}})
}

func (a apiGenAdapter) ListManagedConnections(w http.ResponseWriter, r *http.Request, projectID string, params apigenapi.GenListManagedConnectionsParams) {
	repo := a.server.releaseRepository()
	if repo == nil {
		writeAPIJSON(w, http.StatusOK, apigenapi.ManagedConnectionListResponse{Items: []apigenapi.ManagedConnectionResponse{}, Page: apigenapi.PageInfo{}})
		return
	}
	rows, err := repo.ListConnections(r.Context(), projectID, a.server.defaultEnvironment)
	if err != nil {
		writeAPIProblem(w, r, http.StatusInternalServerError, "CONNECTION_LIST_FAILED", "Connections could not be loaded", nil)
		return
	}
	items := []apigenapi.ManagedConnectionResponse{}
	for _, row := range rows {
		item := apigenapi.ManagedConnectionResponse{Id: row.ID, ProjectId: projectID, Title: row.Title}
		if row.Description != "" {
			item.Description = &row.Description
		}
		if row.ActiveRevisionID != "" {
			item.ActiveRevisionId = &row.ActiveRevisionID
		}
		items = append(items, item)
	}
	page, next, err := keysetPage(items, params.Limit, params.PageToken, func(item apigenapi.ManagedConnectionResponse) string { return item.Id })
	if err != nil {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_CURSOR", err.Error(), nil)
		return
	}
	writeAPIJSON(w, http.StatusOK, apigenapi.ManagedConnectionListResponse{Items: page, Page: apigenapi.PageInfo{NextCursor: next}})
}

func (a apiGenAdapter) GetManagedConnection(w http.ResponseWriter, r *http.Request, projectID, connectionID string) {
	repo := a.server.releaseRepository()
	if repo == nil {
		writeAPIProblem(w, r, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Connection not found", nil)
		return
	}
	row, err := repo.GetConnection(r.Context(), projectID, connectionID, a.server.defaultEnvironment)
	if err != nil {
		writeAPIProblem(w, r, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Connection not found", nil)
		return
	}
	item := apigenapi.ManagedConnectionResponse{Id: connectionID, ProjectId: projectID, Title: row.Title}
	if row.Description != "" {
		item.Description = &row.Description
	}
	if row.ActiveRevisionID != "" {
		item.ActiveRevisionId = &row.ActiveRevisionID
	}
	writeAPIJSON(w, http.StatusOK, item)
}

func (a apiGenAdapter) ListManagedDataUploadSessionEvents(w http.ResponseWriter, r *http.Request, projectID, connectionID, sessionID string, params apigenapi.GenListManagedDataUploadSessionEventsParams, _ apigenapi.GenListManagedDataUploadSessionEventsHeaders) {
	if a.server.managedDataOptions.Uploads == nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "UPLOAD_SERVICE_UNAVAILABLE", "Managed-data uploads are unavailable", nil)
		return
	}
	_, err := a.server.managedDataOptions.Uploads.RecoverUpload(r.Context(), control.UploadRequest{Project: projectID, Connection: connectionID, UploadID: sessionID})
	if err != nil {
		writeAPIProblem(w, r, http.StatusNotFound, "UPLOAD_SESSION_NOT_FOUND", "Upload session not found", nil)
		return
	}
	eventsRepo, repoErr := a.server.asyncRepository()
	if repoErr != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "ASYNC_EVENT_STORE_UNAVAILABLE", "Upload events are unavailable", nil)
		return
	}
	writeStoredAsyncEventPage(w, r, eventsRepo, "upload", sessionID, params.Limit, params.PageToken, "upload:"+projectID+":"+connectionID+":"+sessionID)
}

func (a apiGenAdapter) GetWorkspace(w http.ResponseWriter, r *http.Request, workspaceID string) {
	repo, err := a.server.workspaceRepository()
	if err != nil || repo == nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "WORKSPACE_SERVICE_UNAVAILABLE", "Workspace service is unavailable", nil)
		return
	}
	row, err := repo.ByID(r.Context(), workspace.WorkspaceID(workspaceID))
	if err != nil {
		writeAPIProblem(w, r, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "Workspace not found", nil)
		return
	}
	item := apigenapi.WorkspaceResponse{Id: string(row.ID), Title: row.Title, Description: row.Description, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.ActiveServingStateID != "" {
		value := string(row.ActiveServingStateID)
		item.ActiveServingStateId = &value
	}
	writeAPIJSON(w, http.StatusOK, item)
}

func (a apiGenAdapter) CancelRefreshRun(w http.ResponseWriter, r *http.Request, workspaceID, runID string, _ apigenapi.GenCancelRefreshRunHeaders) {
	repo, err := a.server.refreshRunRepository()
	if err != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "REFRESH_SERVICE_UNAVAILABLE", "Refresh service is unavailable", nil)
		return
	}
	row, err := repo.CancelRun(r.Context(), a.server.workspaceID(workspaceID), runID)
	if err != nil {
		if errors.Is(err, materialize.ErrRunNotCancellable) {
			writeAPIProblem(w, r, http.StatusConflict, "REFRESH_NOT_CANCELLABLE", "Only queued refresh runs can be cancelled", nil)
			return
		}
		writeAPIProblem(w, r, http.StatusNotFound, "REFRESH_RUN_NOT_FOUND", "Refresh run not found", nil)
		return
	}
	if err := a.server.appendAsyncEvent(r.Context(), "refresh", runID, "refresh.cancelled", row); err != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "ASYNC_EVENT_STORE_UNAVAILABLE", "Refresh cancellation could not be recorded", nil)
		return
	}
	w.Header().Set("Location", "/api/v1/workspaces/"+workspaceID+"/refresh-runs/"+runID)
	writeAPIJSON(w, http.StatusAccepted, row)
}

func (a apiGenAdapter) ListRefreshRunEvents(w http.ResponseWriter, r *http.Request, workspaceID, runID string, params apigenapi.GenListRefreshRunEventsParams, _ apigenapi.GenListRefreshRunEventsHeaders) {
	repo, err := a.server.refreshRunRepository()
	if err != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "REFRESH_SERVICE_UNAVAILABLE", "Refresh service is unavailable", nil)
		return
	}
	_, err = repo.GetRun(r.Context(), a.server.workspaceID(workspaceID), runID)
	if err != nil {
		writeAPIProblem(w, r, http.StatusNotFound, "REFRESH_RUN_NOT_FOUND", "Refresh run not found", nil)
		return
	}
	eventsRepo, repoErr := a.server.asyncRepository()
	if repoErr != nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "ASYNC_EVENT_STORE_UNAVAILABLE", "Refresh events are unavailable", nil)
		return
	}
	writeStoredAsyncEventPage(w, r, eventsRepo, "refresh", runID, params.Limit, params.PageToken, "refresh:"+workspaceID+":"+runID)
}

func _normalizeProjectID(value string) string { return strings.TrimSpace(value) }
