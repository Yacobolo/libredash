package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/deployment/apiadapter"
	"github.com/Yacobolo/libredash/internal/release"
)

func (a apiGenAdapter) CreateDeployment(w http.ResponseWriter, r *http.Request, project string, headers apigenapi.GenCreateDeploymentHeaders) {
	var body apigenapi.DeploymentCreateRequest
	if err := decodeAPIBody(w, r, &body); err != nil {
		writeAPIProblem(w, r, http.StatusBadRequest, "INVALID_JSON", err.Error(), nil)
		return
	}
	a.createDeployment(w, r, project, body.ReleaseId, headers.IdempotencyKey, "")
}

func (a apiGenAdapter) createDeployment(w http.ResponseWriter, r *http.Request, project, releaseID, idempotencyKey, rollbackOf string) {
	principal, ok := currentPrincipal(a.server, r)
	if !ok {
		writeAPIProblem(w, r, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Bearer authentication is required", nil)
		return
	}
	if a.server.store == nil || a.server.deploymentOptions.Coordinator == nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "DEPLOYMENT_SERVICE_UNAVAILABLE", "Deployment service is unavailable", nil)
		return
	}
	releases := a.server.releaseRepository()
	targetRelease, err := releases.Get(r.Context(), project, releaseID)
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	if targetRelease.Status != release.StatusReady {
		writeAPIProblem(w, r, http.StatusConflict, "RELEASE_NOT_READY", "Only ready releases can be deployed", nil)
		return
	}
	targets := make([]apiadapter.TargetRequest, 0, len(targetRelease.Artifacts))
	for _, artifact := range targetRelease.Artifacts {
		if artifact.ServingStateID == "" {
			writeAPIProblem(w, r, http.StatusConflict, "RELEASE_INCOMPLETE", "Release is missing a workspace artifact", nil)
			return
		}
		targets = append(targets, apiadapter.TargetRequest{Workspace: artifact.WorkspaceID, CandidateID: artifact.ServingStateID})
	}
	created, err := a.server.deploymentOptions.Coordinator.Create(r.Context(), apiadapter.CreateRequest{
		Project: project, Environment: a.server.defaultEnvironment, Targets: targets, Actor: principal.ID, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	if err := releases.LinkDeployment(r.Context(), project, created.ID, releaseID, rollbackOf); err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	w.Header().Set("Location", deploymentLocation(project, created.ID))
	writeAPIJSON(w, http.StatusAccepted, deploymentResponse(created, releaseID, principal.ID))
	go a.activateDeployment(project, created.ID, principal.ID, idempotencyKey+":cutover")
}

func (a apiGenAdapter) activateDeployment(project, deploymentID, actor, idempotencyKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	_, err := a.server.deploymentOptions.Coordinator.Activate(ctx, apiadapter.ActivateRequest{
		Scope: apiadapter.Scope{Project: project, DeploymentID: deploymentID}, Actor: actor, IdempotencyKey: idempotencyKey,
	})
	if err != nil && a.server.logger != nil {
		a.server.logger.Error("asynchronous deployment failed", "project", project, "deployment", deploymentID, "error", err)
	}
}

func (a apiGenAdapter) GetDeployment(w http.ResponseWriter, r *http.Request, project, deploymentID string) {
	if a.server.store == nil || a.server.deploymentOptions.Coordinator == nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "DEPLOYMENT_SERVICE_UNAVAILABLE", "Deployment service is unavailable", nil)
		return
	}
	releases := a.server.releaseRepository()
	releaseID, _, err := releases.DeploymentRelease(r.Context(), project, deploymentID)
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	row, err := a.server.deploymentOptions.Coordinator.Get(r.Context(), apiadapter.Scope{Project: project, DeploymentID: deploymentID})
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	writeAPIJSON(w, http.StatusOK, deploymentResponse(row, releaseID, ""))
}

func (a apiGenAdapter) ListDeployments(w http.ResponseWriter, r *http.Request, project string, _ apigenapi.GenListDeploymentsParams) {
	if a.server.store == nil || a.server.deploymentOptions.Coordinator == nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "DEPLOYMENT_SERVICE_UNAVAILABLE", "Deployment service is unavailable", nil)
		return
	}
	releases := a.server.releaseRepository()
	ids, err := releases.ListDeploymentIDs(r.Context(), project)
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	items := make([]apigenapi.DeploymentResponse, 0, len(ids))
	for _, id := range ids {
		releaseID, _, err := releases.DeploymentRelease(r.Context(), project, id)
		if err != nil {
			continue
		}
		row, err := a.server.deploymentOptions.Coordinator.Get(r.Context(), apiadapter.Scope{Project: project, DeploymentID: id})
		if err != nil {
			continue
		}
		items = append(items, deploymentResponse(row, releaseID, ""))
	}
	writeAPIJSON(w, http.StatusOK, apigenapi.DeploymentListResponse{Items: items, Page: apigenapi.PageInfo{}})
}

func (a apiGenAdapter) CancelDeployment(w http.ResponseWriter, r *http.Request, project, deploymentID string, _ apigenapi.GenCancelDeploymentHeaders) {
	if a.server.deploymentOptions.Coordinator == nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "DEPLOYMENT_SERVICE_UNAVAILABLE", "Deployment service is unavailable", nil)
		return
	}
	releaseID, _, err := a.server.releaseRepository().DeploymentRelease(r.Context(), project, deploymentID)
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	row, err := a.server.deploymentOptions.Coordinator.Cancel(r.Context(), apiadapter.Scope{Project: project, DeploymentID: deploymentID})
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	w.Header().Set("Location", deploymentLocation(project, deploymentID))
	writeAPIJSON(w, http.StatusAccepted, deploymentResponse(row, releaseID, ""))
}

func (a apiGenAdapter) RollbackDeployment(w http.ResponseWriter, r *http.Request, project, deploymentID string, headers apigenapi.GenRollbackDeploymentHeaders) {
	if a.server.store == nil {
		writeAPIProblem(w, r, http.StatusServiceUnavailable, "DEPLOYMENT_SERVICE_UNAVAILABLE", "Deployment service is unavailable", nil)
		return
	}
	releases := a.server.releaseRepository()
	releaseID, err := releases.PriorDeploymentRelease(r.Context(), project, deploymentID)
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	a.createDeployment(w, r, project, releaseID, headers.IdempotencyKey, deploymentID)
}

func (a apiGenAdapter) ListDeploymentEvents(w http.ResponseWriter, r *http.Request, project, deploymentID string, params apigenapi.GenListDeploymentEventsParams, _ apigenapi.GenListDeploymentEventsHeaders) {
	releases := a.server.releaseRepository()
	releaseID, _, err := releases.DeploymentRelease(r.Context(), project, deploymentID)
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	row, err := a.server.deploymentOptions.Coordinator.Get(r.Context(), apiadapter.Scope{Project: project, DeploymentID: deploymentID})
	if err != nil {
		writeDeploymentError(w, r, err)
		return
	}
	writeAsyncEventPage(w, r, deploymentEvents(row, releaseID), params.Limit, params.PageToken, "deployment:"+project+":"+deploymentID, func(ctx context.Context) ([]apigenapi.AsyncEventResponse, error) {
		latest, err := a.server.deploymentOptions.Coordinator.Get(ctx, apiadapter.Scope{Project: project, DeploymentID: deploymentID})
		return deploymentEvents(latest, releaseID), err
	})
}

func deploymentResponse(row apiadapter.Deployment, releaseID, actor string) apigenapi.DeploymentResponse {
	status := apigenapi.DeploymentStatus(row.Status)
	if row.Status == apiadapter.StatusPending {
		status = apigenapi.DeploymentStatusQueued
	}
	result := apigenapi.DeploymentResponse{
		Id: row.ID, ProjectId: row.Project, ReleaseId: releaseID, Environment: row.Environment, Status: status,
		CreatedBy: actor, CreatedAt: row.CreatedAt, Targets: make([]apigenapi.DeploymentTargetResponse, 0, len(row.Targets)),
		Connections: make([]apigenapi.DeploymentConnectionResponse, 0, len(row.Connections)),
	}
	for _, target := range row.Targets {
		stateID := target.CandidateID
		mapped := apigenapi.DeploymentTargetResponse{WorkspaceId: target.Workspace, ServingStateId: &stateID, Status: string(target.Status)}
		if target.PriorCandidateID != "" {
			mapped.PriorServingStateId = &target.PriorCandidateID
		}
		if target.Error != "" {
			mapped.Error = &target.Error
		}
		result.Targets = append(result.Targets, mapped)
	}
	for _, connection := range row.Connections {
		mapped := apigenapi.DeploymentConnectionResponse{ConnectionId: connection.Connection, RevisionId: connection.RevisionID}
		if connection.PriorRevisionID != "" {
			mapped.PriorRevisionId = &connection.PriorRevisionID
		}
		result.Connections = append(result.Connections, mapped)
	}
	if row.ActivatedAt != "" {
		result.StartedAt = &row.ActivatedAt
		result.FinishedAt = &row.ActivatedAt
	}
	if row.Error != "" {
		result.Error = &row.Error
	}
	return result
}

func deploymentLocation(project, deploymentID string) string {
	return "/api/v1/projects/" + project + "/deployments/" + deploymentID
}

func writeDeploymentError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
	switch {
	case errors.Is(err, release.ErrNotFound), errors.Is(err, deployment.ErrNotFound):
		status, code = http.StatusNotFound, "DEPLOYMENT_NOT_FOUND"
	case errors.Is(err, release.ErrConflict), errors.Is(err, deployment.ErrConflict):
		status, code = http.StatusConflict, "DEPLOYMENT_CONFLICT"
	case errors.Is(err, apiadapter.ErrInvalid):
		status, code = http.StatusUnprocessableEntity, "INVALID_DEPLOYMENT"
	}
	detail := err.Error()
	if status == http.StatusInternalServerError {
		detail = "The deployment request could not be completed"
	}
	writeAPIProblem(w, r, status, code, detail, nil)
}
