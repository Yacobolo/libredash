package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/assetnav"
	"github.com/Yacobolo/libredash/internal/dashboard"
	dashboardstream "github.com/Yacobolo/libredash/internal/dashboard/stream"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacedatastar "github.com/Yacobolo/libredash/internal/workspace/datastar"
	"github.com/go-chi/chi/v5"
	"github.com/starfederation/datastar-go/datastar"
)

type Handler struct {
	WorkspaceID      func(string) string
	Environment      func(*nethttp.Request) string
	ReadModel        ReadModel
	RefreshState     RefreshStateProvider
	RefreshRunner    AssetRefreshRunner
	Broker           Broker
	CSRFToken        func(*nethttp.Request) string
	CurrentRoleLabel func(*nethttp.Request) string
	ChromeOptions    func(*nethttp.Request) []ui.ChromeOption
}

type workspaceAccessSignalPayload struct {
	WorkspaceAccess struct {
		Command ui.WorkspaceAccessCommand `json:"command"`
	} `json:"workspaceAccess"`
	WorkspaceAccessCommand ui.WorkspaceAccessCommand `json:"workspaceAccessCommand"`
}

func (signals workspaceAccessSignalPayload) command() ui.WorkspaceAccessCommand {
	command := signals.WorkspaceAccess.Command
	if command.Email == "" && command.Role == "" && command.PrincipalID == "" && command.BindingID == "" && command.SubjectType == "" && command.SubjectID == "" {
		command = signals.WorkspaceAccessCommand
	}
	return command
}

func (h Handler) WorkspaceCatalog(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaces, err := h.workspaceList(r)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	if err := ui.WorkspacesPage(h.catalogForWorkspacesPage(r, workspaces), workspaces, h.currentRoleLabel(r), h.chromeOptions(r)...).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h Handler) WorkspaceAssets(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	switch r.URL.Query().Get("type") {
	case "connection":
		nethttp.Redirect(w, r, assetnav.ConnectionsHref(r.URL.Query().Get("q")), nethttp.StatusFound)
		return
	case "source":
		nethttp.Redirect(w, r, assetnav.ConnectionsHrefWithType("source", r.URL.Query().Get("q")), nethttp.StatusFound)
		return
	}
	assets, _, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	filtered := workspace.FilterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))
	workspaceView := h.workspaceResponse(r, workspaceID)
	access := h.workspaceAccess(r, workspaceView, h.canManageAccess(r, workspaceID), ui.WorkspaceAccessStatus{})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	if err := ui.WorkspacePage(h.catalogForWorkspace(workspaceID), workspaceView, filtered, r.URL.Query().Get("type"), r.URL.Query().Get("q"), h.currentRoleLabel(r), access, h.csrfToken(r), h.chromeOptions(r)...).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h Handler) WorkspaceAsset(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	assetID := chi.URLParam(r, "asset")
	assets, edges, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	selected, ok := workspace.AssetByID(assets, assetID)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	if selected.Type == string(workspace.AssetTypeConnection) {
		nethttp.Redirect(w, r, assetnav.ConnectionAssetSectionHref(assetID, "details"), nethttp.StatusFound)
		return
	}
	if selected.Type == string(workspace.AssetTypeSource) {
		nethttp.Redirect(w, r, assetnav.CanonicalSourceAssetSectionHref(workspaceID, selected.ID, "details", edges), nethttp.StatusFound)
		return
	}
	nethttp.Redirect(w, r, "/workspaces/"+workspaceID+"/assets/"+assetID+"/details", nethttp.StatusFound)
}

func (h Handler) WorkspaceAssetSection(w nethttp.ResponseWriter, r *nethttp.Request) {
	section := chi.URLParam(r, "section")
	redirectToDetails := false
	if section == "definition" {
		section = "details"
		redirectToDetails = true
	}
	if !ui.ValidWorkspaceAssetSection(section) {
		nethttp.NotFound(w, r)
		return
	}
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	assets, edges, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	assetID := chi.URLParam(r, "asset")
	selected, ok := workspace.AssetByID(assets, assetID)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	if section == "refreshes" && !workspaceAssetRefreshable(selected) {
		nethttp.NotFound(w, r)
		return
	}
	if section == "data" {
		if selected.Type != string(workspace.AssetTypeSemanticModel) && selected.Type != string(workspace.AssetTypeModelTable) && selected.Type != string(workspace.AssetTypeSource) {
			nethttp.NotFound(w, r)
			return
		}
		values := url.Values{}
		values.Set("workspace", workspaceID)
		values.Set("object", assetID)
		nethttp.Redirect(w, r, "/data?"+values.Encode(), nethttp.StatusFound)
		return
	}
	if selected.Type == string(workspace.AssetTypeConnection) {
		nethttp.Redirect(w, r, assetnav.ConnectionAssetSectionHref(assetID, section), nethttp.StatusFound)
		return
	}
	if selected.Type == string(workspace.AssetTypeSource) {
		nethttp.Redirect(w, r, assetnav.CanonicalSourceAssetSectionHref(workspaceID, selected.ID, section, edges), nethttp.StatusFound)
		return
	}
	if redirectToDetails {
		nethttp.Redirect(w, r, "/workspaces/"+workspaceID+"/assets/"+assetID+"/details", nethttp.StatusFound)
		return
	}
	refresh, err := h.assetRefreshState(r.Context(), workspaceID, selected)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	refresh.CSRFToken = h.csrfToken(r)
	versions, err := h.assetVersionsState(r.Context(), workspaceID, h.environment(r), selected, section)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	if err := ui.WorkspaceAssetPageWithRefreshAndVersions(h.catalogForWorkspace(workspaceID), h.workspaceResponse(r, workspaceID), selected, assets, edges, section, h.currentRoleLabel(r), refresh, versions, h.chromeOptions(r)...).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h Handler) Connections(w nethttp.ResponseWriter, r *nethttp.Request) {
	assets, edges, err := h.platformAssetsAndEdges(r)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	activeType := workspace.NormalizeConnectionAssetType(r.URL.Query().Get("type"))
	filtered := workspace.FilterConnectionAssets(assets, activeType, r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	if err := ui.ConnectionsPage(h.catalogForWorkspacesPage(r, nil), "platform", filtered, edges, activeType, r.URL.Query().Get("q"), h.currentRoleLabel(r), h.chromeOptions(r)...).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h Handler) ConnectionSource(w nethttp.ResponseWriter, r *nethttp.Request) {
	connectionID := chi.URLParam(r, "connection")
	sourceID := chi.URLParam(r, "source")
	assets, edges, err := h.platformAssetsAndEdges(r)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	if _, _, ok := connectionSourcePair(assets, edges, connectionID, sourceID); !ok {
		nethttp.NotFound(w, r)
		return
	}
	nethttp.Redirect(w, r, assetnav.ConnectionSourceAssetSectionHref(connectionID, sourceID, "details"), nethttp.StatusFound)
}

func (h Handler) ConnectionSourceSection(w nethttp.ResponseWriter, r *nethttp.Request) {
	section := chi.URLParam(r, "section")
	if section == "definition" {
		nethttp.Redirect(w, r, assetnav.ConnectionSourceAssetSectionHref(chi.URLParam(r, "connection"), chi.URLParam(r, "source"), "details"), nethttp.StatusFound)
		return
	}
	if !ui.ValidWorkspaceAssetSection(section) {
		nethttp.NotFound(w, r)
		return
	}
	if section == "refreshes" {
		nethttp.NotFound(w, r)
		return
	}
	assets, edges, err := h.platformAssetsAndEdges(r)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	connection, source, ok := connectionSourcePair(assets, edges, chi.URLParam(r, "connection"), chi.URLParam(r, "source"))
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	if section == "data" {
		values := url.Values{}
		values.Set("workspace", source.WorkspaceID)
		values.Set("object", source.ID)
		nethttp.Redirect(w, r, "/data?"+values.Encode(), nethttp.StatusFound)
		return
	}
	versions, err := h.assetVersionsState(r.Context(), source.WorkspaceID, h.environment(r), source, section)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	if err := ui.ConnectionSourceAssetPageWithVersions(h.catalogForWorkspacesPage(r, nil), platformAssetWorkspaceView(), connection, source, assets, edges, section, h.currentRoleLabel(r), versions).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h Handler) ConnectionAsset(w nethttp.ResponseWriter, r *nethttp.Request) {
	nethttp.Redirect(w, r, assetnav.ConnectionAssetSectionHref(chi.URLParam(r, "asset"), "details"), nethttp.StatusFound)
}

func (h Handler) ConnectionAssetSection(w nethttp.ResponseWriter, r *nethttp.Request) {
	section := chi.URLParam(r, "section")
	if section == "definition" {
		nethttp.Redirect(w, r, assetnav.ConnectionAssetSectionHref(chi.URLParam(r, "asset"), "details"), nethttp.StatusFound)
		return
	}
	if !ui.ValidWorkspaceAssetSection(section) {
		nethttp.NotFound(w, r)
		return
	}
	if section == "refreshes" || section == "data" {
		nethttp.NotFound(w, r)
		return
	}
	assets, edges, err := h.platformAssetsAndEdges(r)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	selected, ok := workspace.AssetByID(assets, chi.URLParam(r, "asset"))
	if !ok || selected.Type != string(workspace.AssetTypeConnection) {
		nethttp.NotFound(w, r)
		return
	}
	versions, err := h.assetVersionsState(r.Context(), selected.WorkspaceID, h.environment(r), selected, section)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	if err := ui.ConnectionAssetPageWithVersions(h.catalogForWorkspacesPage(r, nil), platformAssetWorkspaceView(), selected, assets, edges, section, h.currentRoleLabel(r), versions).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h Handler) Workspaces(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaces, err := h.workspaceList(r)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	_ = writePagedJSON(w, r, apiWorkspaceDTOs(workspaces))
}

func (h Handler) Assets(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	assets, _, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	filtered := workspace.FilterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))
	if r.URL.Query().Get("include") == "all" {
		filtered = workspace.FilterAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))
	}
	_ = writePagedJSON(w, r, apiAssetSummaryDTOs(filtered))
}

func (h Handler) ActiveDeploymentGraph(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	repo, err := h.workspaceRepository()
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	graph := workspace.AssetGraph{}
	if repo != nil {
		var ok bool
		graph, ok, err = repo.ActiveServingStateGraph(r.Context(), workspace.WorkspaceID(workspaceID), h.environment(r))
		if err != nil {
			writeJSONError(w, err, nethttp.StatusInternalServerError)
			return
		}
		if !ok {
			graph = workspace.AssetGraph{}
		}
	}
	response, err := apiWorkspaceAssetGraphDTO(graph)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	writeJSON(w, nethttp.StatusOK, response)
}

func (h Handler) Asset(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	assetID := firstNonEmpty(chi.URLParam(r, "assetId"), chi.URLParam(r, "asset"))
	assets, _, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	asset, ok := workspace.AssetByID(assets, assetID)
	if !ok {
		writeJSONError(w, fmt.Errorf("asset %q not found", assetID), nethttp.StatusNotFound)
		return
	}
	writeJSON(w, nethttp.StatusOK, apiAssetDTOs([]workspace.AssetView{asset})[0])
}

func (h Handler) AssetEdges(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	_, edges, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	_ = writePagedJSON(w, r, apiAssetEdgeDTOs(edges))
}

func (h Handler) AssetLineage(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	assetID := firstNonEmpty(chi.URLParam(r, "assetId"), chi.URLParam(r, "asset"))
	assets, edges, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	if _, ok := workspace.AssetByID(assets, assetID); !ok {
		writeJSONError(w, fmt.Errorf("asset %q not found", assetID), nethttp.StatusNotFound)
		return
	}
	writeJSON(w, nethttp.StatusOK, api.AssetLineageResponse{
		AssetID:    assetID,
		Upstream:   assetLineageEndpointIDs(edges, assetID, true),
		Downstream: assetLineageEndpointIDs(edges, assetID, false),
	})
}

func (h Handler) Roles(w nethttp.ResponseWriter, r *nethttp.Request) {
	_, roles, err := h.roleBindingsAndRoles(r, h.workspaceID(chi.URLParam(r, "workspace")))
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	_ = writePagedJSON(w, r, apiRoleDTOs(roles))
}

func (h Handler) RoleBindings(w nethttp.ResponseWriter, r *nethttp.Request) {
	repo, err := h.accessRepository()
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	if repo == nil {
		_ = writePagedJSON(w, r, []map[string]any{})
		return
	}
	bindings, err := repo.ListRoleBindings(r.Context(), h.workspaceID(chi.URLParam(r, "workspace")))
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, apiRoleBindingDTO(binding))
	}
	_ = writePagedJSON(w, r, out)
}

func (h Handler) UpsertRoleBinding(w nethttp.ResponseWriter, r *nethttp.Request) {
	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	repo, err := h.accessRepository()
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	if repo == nil {
		writeJSONError(w, errWorkspaceAccessNotConfigured, nethttp.StatusInternalServerError)
		return
	}
	principal, err := repo.SetPrincipalRole(r.Context(), access.PrincipalRoleInput{WorkspaceID: workspaceID, Email: input.Email, DisplayName: input.DisplayName, Role: input.Role})
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	writeJSON(w, nethttp.StatusOK, map[string]string{"principalId": principal.ID})
}

func (h Handler) DeleteRoleBinding(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	repo, err := h.accessRepository()
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	if repo == nil {
		writeJSONError(w, errWorkspaceAccessNotConfigured, nethttp.StatusInternalServerError)
		return
	}
	bindingID := chi.URLParam(r, "binding")
	if bindingID == "" {
		bindingID = chi.URLParam(r, "principal")
	}
	if err := repo.DeleteRoleBinding(r.Context(), workspaceID, bindingID); err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	writeJSON(w, nethttp.StatusOK, map[string]string{"status": "removed"})
}

func (h Handler) AccessUpsert(w nethttp.ResponseWriter, r *nethttp.Request) {
	signals := workspaceAccessSignalPayload{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	command := signals.command()
	status := ui.WorkspaceAccessStatus{Message: "Access updated."}
	repo, err := h.accessRepository()
	if err != nil {
		status = ui.WorkspaceAccessStatus{Error: err.Error()}
	} else if repo == nil {
		status = ui.WorkspaceAccessStatus{Error: errWorkspaceAccessNotConfigured.Error()}
	} else if object, ok := assetAccessObject(r, workspaceID); ok {
		subjectType, subjectID, err := h.resolveAccessSubject(r, repo, command)
		if err != nil {
			status = ui.WorkspaceAccessStatus{Error: err.Error()}
		} else if _, err := repo.CreateGrant(r.Context(), access.GrantInput{
			Object:      object,
			SubjectType: subjectType,
			SubjectID:   subjectID,
			Privilege:   access.Privilege(strings.TrimSpace(firstNonEmpty(command.Privilege, command.Role))),
		}); err != nil {
			status = ui.WorkspaceAccessStatus{Error: err.Error()}
		}
	} else {
		subjectType, subjectID, err := h.resolveAccessSubject(r, repo, command)
		if err != nil {
			status = ui.WorkspaceAccessStatus{Error: err.Error()}
		} else {
			input := access.RoleBindingInput{WorkspaceID: workspaceID, SubjectType: subjectType, SubjectID: subjectID, Role: command.Role}
			if strings.TrimSpace(command.BindingID) != "" {
				_, err = repo.UpdateRoleBinding(r.Context(), workspaceID, command.BindingID, input)
			} else {
				_, err = repo.CreateRoleBinding(r.Context(), input)
			}
			if err != nil {
				status = ui.WorkspaceAccessStatus{Error: err.Error()}
			}
		}
	}
	h.patchWorkspaceAccess(w, r, workspaceID, status)
}

func (h Handler) resolveAccessSubject(r *nethttp.Request, repo access.Repository, command ui.WorkspaceAccessCommand) (access.SubjectType, string, error) {
	subjectType := access.SubjectType(strings.TrimSpace(command.SubjectType))
	if subjectType == "" {
		subjectType = access.SubjectPrincipal
	}
	subjectID := strings.TrimSpace(command.SubjectID)
	switch subjectType {
	case access.SubjectPrincipal:
		if subjectID != "" {
			return subjectType, subjectID, nil
		}
		email := strings.TrimSpace(command.Email)
		if email == "" {
			return "", "", fmt.Errorf("email is required")
		}
		principal, err := repo.UpsertPrincipal(r.Context(), access.PrincipalInput{
			ID:    access.PrincipalIDForEmail(email),
			Email: email,
		})
		if err != nil {
			return "", "", err
		}
		return subjectType, principal.ID, nil
	case access.SubjectGroup, access.SubjectServicePrincipal:
		if subjectID == "" {
			return "", "", fmt.Errorf("subject id is required")
		}
		return subjectType, subjectID, nil
	default:
		return "", "", fmt.Errorf("unsupported subject type %q", command.SubjectType)
	}
}

func (h Handler) AccessRemove(w nethttp.ResponseWriter, r *nethttp.Request) {
	signals := workspaceAccessSignalPayload{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	command := signals.command()
	status := ui.WorkspaceAccessStatus{Message: "Access removed."}
	repo, err := h.accessRepository()
	if err != nil {
		status = ui.WorkspaceAccessStatus{Error: err.Error()}
	} else if repo == nil {
		status = ui.WorkspaceAccessStatus{Error: errWorkspaceAccessNotConfigured.Error()}
	} else if _, ok := assetAccessObject(r, workspaceID); ok {
		if err := repo.DeleteGrant(r.Context(), workspaceID, command.BindingID); err != nil {
			status = ui.WorkspaceAccessStatus{Error: err.Error()}
		}
	} else if err := repo.RemovePrincipalRoles(r.Context(), workspaceID, command.PrincipalID); err != nil {
		status = ui.WorkspaceAccessStatus{Error: err.Error()}
	}
	h.patchWorkspaceAccess(w, r, workspaceID, status)
}

func (h Handler) patchWorkspaceAccess(w nethttp.ResponseWriter, r *nethttp.Request, workspaceID string, status ui.WorkspaceAccessStatus) {
	workspaceView := h.workspaceResponse(r, workspaceID)
	accessResponse := h.workspaceAccess(r, workspaceView, true, status)
	if object, ok := assetAccessObject(r, workspaceID); ok {
		accessResponse = h.objectAccess(r, workspaceView, object, status)
	}
	sse := datastar.NewSSE(w, r)
	_ = sse.MarshalAndPatchSignals(map[string]any{
		"workspaceAccess": ui.WorkspaceAccessSignals(accessResponse, h.csrfToken(r)),
	})
}

func (h Handler) objectAccess(r *nethttp.Request, workspaceView workspace.WorkspaceView, object access.ObjectRef, status ui.WorkspaceAccessStatus) ui.WorkspaceAccessResponse {
	response := ui.WorkspaceAccessResponse{
		Workspace:   workspaceView,
		ObjectType:  string(object.Type),
		ObjectID:    object.ObjectID,
		ObjectTitle: object.ObjectID,
		Mode:        "object",
		Roles:       objectPrivilegeRoleViews(),
		CanManage:   true,
		Status:      status,
	}
	repo, err := h.accessRepository()
	if err != nil {
		response.Status.Error = err.Error()
		return response
	}
	if repo == nil {
		response.Status.Error = errWorkspaceAccessNotConfigured.Error()
		return response
	}
	grants, err := repo.ListGrants(r.Context(), object)
	if err != nil {
		response.Status.Error = err.Error()
		return response
	}
	for _, grant := range grants {
		view := workspace.RoleBindingView{
			ID:          grant.ID,
			WorkspaceID: grant.WorkspaceID,
			SubjectType: string(grant.SubjectType),
			SubjectID:   grant.SubjectID,
			Role:        string(grant.Privilege),
			CreatedAt:   grant.CreatedAt,
		}
		if grant.SubjectType == access.SubjectPrincipal {
			view.PrincipalID = grant.SubjectID
			if principal, err := repo.PrincipalByID(r.Context(), grant.SubjectID); err == nil {
				view.Email = principal.Email
				view.DisplayName = principal.DisplayName
			}
		} else if grant.SubjectType == access.SubjectGroup {
			view.GroupID = grant.SubjectID
			view.GroupName = h.groupDisplayName(r, repo, grant.SubjectID)
		} else if grant.SubjectType == access.SubjectServicePrincipal {
			view.PrincipalID = grant.SubjectID
			if principal, err := repo.PrincipalByID(r.Context(), grant.SubjectID); err == nil {
				view.DisplayName = firstNonEmpty(principal.DisplayName, principal.ID)
			}
		}
		response.Bindings = append(response.Bindings, view)
	}
	return response
}

func (h Handler) groupDisplayName(r *nethttp.Request, repo access.Repository, groupID string) string {
	groups, err := repo.ListAllGroups(r.Context())
	if err != nil {
		return groupID
	}
	for _, group := range groups {
		if group.ID == groupID {
			return firstNonEmpty(group.Name, group.ID)
		}
	}
	return groupID
}

func objectPrivilegeRoleViews() []workspace.RoleView {
	privileges := []access.Privilege{
		access.PrivilegeViewItem,
		access.PrivilegeEditItem,
		access.PrivilegeManageItem,
		access.PrivilegeQueryData,
		access.PrivilegePreviewData,
		access.PrivilegeRefreshData,
		access.PrivilegeUseAgent,
		access.PrivilegeViewAgent,
		access.PrivilegeManageGrants,
	}
	roles := make([]workspace.RoleView, 0, len(privileges))
	for _, privilege := range privileges {
		roles = append(roles, workspace.RoleView{Name: string(privilege), Privileges: []string{string(privilege)}})
	}
	return roles
}

func assetAccessObject(r *nethttp.Request, workspaceID string) (access.ObjectRef, bool) {
	raw := strings.TrimSpace(chi.URLParam(r, "asset"))
	if raw == "" {
		return access.ObjectRef{}, false
	}
	typ, objectID, ok := strings.Cut(raw, ":")
	if !ok || strings.TrimSpace(objectID) == "" {
		return access.ObjectRef{}, false
	}
	switch workspace.AssetType(typ) {
	case workspace.AssetTypeDashboard:
		return access.ItemObjectWithParent(access.SecurableDashboard, workspaceID, objectID, access.WorkspaceObject(workspaceID)), true
	case workspace.AssetTypeSemanticModel:
		return access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, objectID, access.WorkspaceObject(workspaceID)), true
	case workspace.AssetTypeSource:
		return access.ItemObjectWithParent(access.SecurableSource, workspaceID, objectID, access.WorkspaceObject(workspaceID)), true
	case workspace.AssetTypeModelTable:
		return access.ItemObjectWithParent(access.SecurableModelTable, workspaceID, objectID, access.WorkspaceObject(workspaceID)), true
	case workspace.AssetTypeSemanticTable:
		modelID, tableID, ok := strings.Cut(objectID, ".")
		if !ok {
			return access.ItemObject(access.SecurableDataset, workspaceID, objectID), true
		}
		model := access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, modelID, access.WorkspaceObject(workspaceID))
		return access.ItemObjectWithParent(access.SecurableDataset, workspaceID, modelID+"/"+tableID, model), true
	case workspace.AssetTypeField:
		parts := strings.Split(objectID, ".")
		if len(parts) < 3 {
			return access.ItemObject(access.SecurableColumn, workspaceID, objectID), true
		}
		model := access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, parts[0], access.WorkspaceObject(workspaceID))
		table := access.ItemObjectWithParent(access.SecurableDataset, workspaceID, parts[0]+"/"+parts[1], model)
		return access.ItemObjectWithParent(access.SecurableColumn, workspaceID, parts[0]+"/"+parts[1]+"/"+strings.Join(parts[2:], "."), table), true
	case workspace.AssetTypeWorkspaceAgentPolicy:
		return access.ItemObjectWithParent(access.SecurableAgentPolicy, workspaceID, objectID, access.WorkspaceObject(workspaceID)), true
	default:
		return access.ObjectRef{}, false
	}
}

func AssetObjectRefs(r *nethttp.Request, workspaceID string) []access.ObjectRef {
	objects := []access.ObjectRef{}
	if object, ok := assetAccessObject(r, workspaceID); ok {
		objects = append(objects, object)
	}
	if strings.TrimSpace(workspaceID) != "" {
		objects = append(objects, access.WorkspaceObject(workspaceID))
	}
	return objects
}

func (h Handler) AssetUpdatesStream(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	assetID := chi.URLParam(r, "asset")
	section := workspacedatastar.WorkspaceAssetUpdateSection(r)
	assets, edges, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	selected, ok := workspace.AssetByID(assets, assetID)
	if !ok || !workspaceAssetRefreshable(selected) {
		nethttp.NotFound(w, r)
		return
	}
	if h.RefreshState == nil {
		nethttp.Error(w, "workspace refresh state provider is required", nethttp.StatusServiceUnavailable)
		return
	}

	sse := datastar.NewSSE(w, r)
	if err := sse.MarshalAndPatchSignals(h.workspaceAssetRefreshPatch(r, workspaceID, selected, assets, edges, section)); err != nil {
		return
	}
	updates, unsubscribe := h.broker().Subscribe(workspacedatastar.WorkspaceAssetStreamID(workspaceID, assetID, section))
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case patch, ok := <-updates:
			if !ok {
				return
			}
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return
			}
		}
	}
}

func (h Handler) RefreshAsset(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.refreshAsset(w, r)
}

func (h Handler) RefreshAssetMaterializations(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.refreshAsset(w, r)
}

func (h Handler) refreshAsset(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	assetID := chi.URLParam(r, "asset")
	assets, edges, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		nethttp.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	selected, ok := workspace.AssetByID(assets, assetID)
	if !ok || !workspaceAssetRefreshable(selected) {
		nethttp.NotFound(w, r)
		return
	}
	if h.RefreshRunner == nil {
		nethttp.Error(w, "workspace refresh runner is required", nethttp.StatusServiceUnavailable)
		return
	}
	if err := h.RefreshRunner.RefreshAsset(r.Context(), AssetRefreshInput{
		Request:     r,
		WorkspaceID: workspaceID,
		Asset:       selected,
		Assets:      assets,
		Edges:       edges,
	}); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	w.WriteHeader(nethttp.StatusNoContent)
}

func (h Handler) workspaceID(value string) string {
	if h.WorkspaceID != nil {
		return h.WorkspaceID(value)
	}
	return value
}

func (h Handler) environment(r *nethttp.Request) string {
	if h.Environment == nil {
		return ""
	}
	return h.Environment(r)
}

func (h Handler) workspaceRepository() (workspace.Repository, error) {
	return h.ReadModel.workspaceRepository()
}

func (h Handler) accessRepository() (access.Repository, error) {
	return h.ReadModel.accessRepository()
}

func (h Handler) assetsAndEdges(r *nethttp.Request, workspaceID string) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	return h.ReadModel.WorkspaceAssetsAndEdges(r, workspaceID)
}

func (h Handler) platformAssetsAndEdges(r *nethttp.Request) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	return h.ReadModel.PlatformAssetsAndEdges(r)
}

func (h Handler) workspaceList(r *nethttp.Request) ([]workspace.WorkspaceView, error) {
	return h.ReadModel.WorkspaceList(r)
}

func (h Handler) workspaceAssetsAndEdgesForData(ctx context.Context, workspaceID, environment string) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	assets, edges, err := h.ReadModel.WorkspaceAssetsAndEdgesForData(ctx, workspaceID, environment)
	if err != nil {
		return nil, nil, err
	}
	if len(assets) == 0 && len(edges) == 0 {
		return nil, nil, fmt.Errorf("workspace %q assets were not found", workspaceID)
	}
	return assets, edges, nil
}

func (h Handler) metricsForWorkspace(workspaceID string) (Metrics, bool) {
	return h.ReadModel.metricsForWorkspace(workspaceID)
}

func (h Handler) roleBindingsAndRoles(r *nethttp.Request, workspaceID string) ([]workspace.RoleBindingView, []workspace.RoleView, error) {
	return h.ReadModel.RoleBindingsAndRoles(r, workspaceID)
}

func (h Handler) catalogForWorkspacesPage(r *nethttp.Request, workspaces []workspace.WorkspaceView) dashboard.Catalog {
	return h.ReadModel.CatalogForWorkspacesPage(r, workspaces)
}

func (h Handler) catalogForWorkspace(workspaceID string) dashboard.Catalog {
	return h.ReadModel.catalogForWorkspace(workspaceID)
}

func (h Handler) workspaceResponse(r *nethttp.Request, workspaceID string) workspace.WorkspaceView {
	return h.ReadModel.WorkspaceResponse(r, workspaceID)
}

func (h Handler) canManageAccess(r *nethttp.Request, workspaceID string) bool {
	return h.ReadModel.CanManageAccess(r, workspaceID)
}

func (h Handler) workspaceAccess(r *nethttp.Request, view workspace.WorkspaceView, canManage bool, status ui.WorkspaceAccessStatus) ui.WorkspaceAccessResponse {
	return h.ReadModel.WorkspaceAccess(r, view, canManage, status)
}

func (h Handler) assetRefreshState(ctx context.Context, workspaceID string, asset workspace.AssetView) (ui.AssetRefreshState, error) {
	if h.RefreshState == nil || !workspaceAssetRefreshable(asset) {
		return ui.AssetRefreshState{}, nil
	}
	return h.RefreshState.AssetRefreshState(ctx, workspaceID, asset)
}

func (h Handler) assetVersionsState(ctx context.Context, workspaceID, environment string, asset workspace.AssetView, section string) (ui.AssetVersionsState, error) {
	if h.RefreshState == nil {
		return ui.AssetVersionsState{CurrentContentHash: asset.ContentHash}, nil
	}
	return h.RefreshState.AssetVersionsState(ctx, workspaceID, environment, asset, section)
}

func (h Handler) csrfToken(r *nethttp.Request) string {
	if h.CSRFToken == nil {
		return ""
	}
	return h.CSRFToken(r)
}

func (h Handler) currentRoleLabel(r *nethttp.Request) string {
	if h.CurrentRoleLabel == nil {
		return ""
	}
	return h.CurrentRoleLabel(r)
}

func (h Handler) chromeOptions(r *nethttp.Request) []ui.ChromeOption {
	if h.ChromeOptions == nil {
		return nil
	}
	return h.ChromeOptions(r)
}

func (h Handler) broker() Broker {
	if h.Broker != nil {
		return h.Broker
	}
	return noopBroker{}
}

var errWorkspaceAccessNotConfigured = errors.New("workspace access store is not configured")

type noopBroker struct{}

func (noopBroker) Subscribe(string) (<-chan dashboardstream.Patch, func()) {
	ch := make(chan dashboardstream.Patch)
	close(ch)
	return ch, func() {}
}

func (noopBroker) Publish(string, dashboardstream.Patch) {}

func connectionSourcePair(assets []workspace.AssetView, edges []workspace.AssetEdgeView, connectionID, sourceID string) (workspace.AssetView, workspace.AssetView, bool) {
	connection, ok := workspace.AssetByID(assets, connectionID)
	if !ok || connection.Type != string(workspace.AssetTypeConnection) {
		return workspace.AssetView{}, workspace.AssetView{}, false
	}
	source, ok := workspace.AssetByID(assets, sourceID)
	if !ok || source.Type != string(workspace.AssetTypeSource) || assetnav.SourceConnectionID(source.ID, edges) != connection.ID {
		return workspace.AssetView{}, workspace.AssetView{}, false
	}
	return connection, source, true
}

func platformAssetWorkspaceView() workspace.WorkspaceView {
	return workspace.WorkspaceView{ID: "platform", Title: "Global assets", Description: "Global connection and source assets."}
}

func workspaceAssetRefreshable(asset workspace.AssetView) bool {
	return asset.Type == string(workspace.AssetTypeSemanticModel) || asset.Type == string(workspace.AssetTypeModelTable)
}

func (h Handler) workspaceAssetRefreshPatch(r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView, section string) map[string]any {
	refresh, err := h.assetRefreshState(r.Context(), workspaceID, asset)
	if err != nil {
		refresh = ui.AssetRefreshState{Latest: ui.AssetRefreshRun{Status: "failed"}}
	}
	return workspacedatastar.WorkspaceAssetRefreshSignals(h.workspaceResponse(r, workspaceID), asset, assets, edges, refresh, section)
}

func (h Handler) PublishWorkspaceAssetRefreshPatch(r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) {
	for _, section := range workspacedatastar.WorkspaceAssetRefreshSections() {
		h.broker().Publish(workspacedatastar.WorkspaceAssetStreamID(workspaceID, asset.ID, section), h.workspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges, section))
	}
}

func (h Handler) PublishWorkspaceAssetRefreshPatchForTarget(r *nethttp.Request, workspaceID, targetID string, assets []workspace.AssetView, edges []workspace.AssetEdgeView) {
	for _, asset := range assets {
		if asset.Key == targetID && workspaceAssetRefreshable(asset) {
			h.PublishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges)
		}
	}
}

func apiWorkspaceDTOs(rows []workspace.WorkspaceView) []api.WorkspaceResponse {
	out := make([]api.WorkspaceResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.WorkspaceResponse{
			ID:                   row.ID,
			Title:                row.Title,
			Description:          row.Description,
			ActiveServingStateID: row.ActiveServingStateID,
			CreatedAt:            row.CreatedAt,
			UpdatedAt:            row.UpdatedAt,
		})
	}
	return out
}

func apiAssetDTOs(rows []workspace.AssetView) []api.AssetResponse {
	out := make([]api.AssetResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.AssetResponse{
			ID:             row.ID,
			SnapshotID:     row.SnapshotID,
			WorkspaceID:    row.WorkspaceID,
			ServingStateID: row.ServingStateID,
			Type:           row.Type,
			Key:            row.Key,
			ParentID:       row.ParentID,
			Title:          row.Title,
			Description:    row.Description,
			SourceFile:     row.SourceFile,
			PayloadSchema:  row.PayloadSchema,
			Payload:        row.Payload,
			Href:           row.Href,
		})
	}
	return out
}

func apiAssetSummaryDTOs(rows []workspace.AssetView) []api.AssetSummaryResponse {
	out := make([]api.AssetSummaryResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.AssetSummaryResponse{
			ID:             row.ID,
			SnapshotID:     row.SnapshotID,
			WorkspaceID:    row.WorkspaceID,
			ServingStateID: row.ServingStateID,
			Type:           row.Type,
			Key:            row.Key,
			ParentID:       row.ParentID,
			Title:          row.Title,
			Description:    row.Description,
			SourceFile:     row.SourceFile,
			PayloadSchema:  row.PayloadSchema,
			ContentHash:    row.ContentHash,
			Href:           row.Href,
		})
	}
	return out
}

func apiWorkspaceAssetGraphDTO(graph workspace.AssetGraph) (api.WorkspaceAssetGraphResponse, error) {
	assets := make([]api.AssetGraphAssetResponse, 0, len(graph.Assets))
	for _, row := range graph.Assets {
		payload := map[string]any{}
		if row.PayloadJSON != "" {
			if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
				return api.WorkspaceAssetGraphResponse{}, err
			}
		}
		assets = append(assets, api.AssetGraphAssetResponse{
			ID:             string(row.ID),
			SnapshotID:     string(row.SnapshotID),
			WorkspaceID:    string(row.WorkspaceID),
			ServingStateID: string(row.ServingStateID),
			Type:           string(row.Type),
			Key:            row.Key,
			ParentID:       string(row.ParentID),
			Title:          row.Title,
			Description:    row.Description,
			SourceFile:     row.SourceFile,
			PayloadSchema:  row.PayloadSchema,
			Payload:        payload,
			ContentHash:    row.ContentHash,
		})
	}
	return api.WorkspaceAssetGraphResponse{
		Assets: assets,
		Edges:  apiWorkspaceAssetGraphEdgeDTOs(graph.Edges),
	}, nil
}

func apiWorkspaceAssetGraphEdgeDTOs(rows []workspace.AssetEdge) []api.AssetEdgeResponse {
	out := make([]api.AssetEdgeResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.AssetEdgeResponse{
			ID:             string(row.ID),
			WorkspaceID:    string(row.WorkspaceID),
			ServingStateID: string(row.ServingStateID),
			FromAssetID:    string(row.FromAssetID),
			ToAssetID:      string(row.ToAssetID),
			Type:           string(row.Type),
		})
	}
	return out
}

func apiAssetEdgeDTOs(rows []workspace.AssetEdgeView) []api.AssetEdgeResponse {
	out := make([]api.AssetEdgeResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.AssetEdgeResponse{
			ID:             row.ID,
			WorkspaceID:    row.WorkspaceID,
			ServingStateID: row.ServingStateID,
			FromAssetID:    row.FromAssetID,
			ToAssetID:      row.ToAssetID,
			Type:           row.Type,
		})
	}
	return out
}

func assetLineageEndpointIDs(edges []workspace.AssetEdgeView, assetID string, upstream bool) []string {
	values := map[string]struct{}{}
	for _, edge := range edges {
		if upstream && edge.ToAssetID == assetID {
			values[edge.FromAssetID] = struct{}{}
		}
		if !upstream && edge.FromAssetID == assetID {
			values[edge.ToAssetID] = struct{}{}
		}
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func apiRoleDTOs(rows []workspace.RoleView) []api.RoleResponse {
	out := make([]api.RoleResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.RoleResponse{Name: row.Name, Privileges: row.Privileges})
	}
	return out
}

func apiRoleBindingDTO(row access.RoleBinding) map[string]any {
	return map[string]any{"id": row.ID, "workspaceId": row.WorkspaceID, "subjectType": string(row.SubjectType), "subjectId": row.SubjectID, "email": row.Email, "displayName": firstNonEmpty(row.DisplayName, row.GroupName), "role": row.Role, "createdAt": row.CreatedAt}
}

func writePagedJSON[T any](w nethttp.ResponseWriter, r *nethttp.Request, items []T) bool {
	page, nextCursor, ok := pageSliceForRequest(w, r, items)
	if !ok {
		return false
	}
	writeJSON(w, nethttp.StatusOK, pagedResponseWithCursor(page, nextCursor))
	return true
}

type pageResponse struct {
	NextCursor string `json:"nextCursor"`
}

func pagedResponseWithCursor(items any, nextCursor string) map[string]any {
	return map[string]any{"items": items, "page": pageResponse{NextCursor: nextCursor}}
}

func pageSliceForRequest[T any](w nethttp.ResponseWriter, r *nethttp.Request, items []T) ([]T, string, bool) {
	limit, ok := apiLimitForRequest(w, r)
	if !ok {
		return nil, "", false
	}
	start, ok := apiCursorOffsetForRequest(w, r)
	if !ok {
		return nil, "", false
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	nextCursor := ""
	if end < len(items) {
		nextCursor = encodeIndexCursor(end)
	}
	return append([]T(nil), items[start:end]...), nextCursor, true
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

func apiCursorOffsetForRequest(w nethttp.ResponseWriter, r *nethttp.Request) (int, bool) {
	offset, err := decodeIndexCursor(r.URL.Query().Get("pageToken"))
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return 0, false
	}
	return offset, true
}

func decodeIndexCursor(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("invalid page token")
	}
	var offset int
	if _, err := fmt.Sscanf(string(raw), "offset:%d", &offset); err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid page token")
	}
	return offset, nil
}

func encodeIndexCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("offset:%d", offset)))
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

func statusForNotFound(err error) int {
	if err == nil {
		return nethttp.StatusInternalServerError
	}
	return nethttp.StatusNotFound
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
