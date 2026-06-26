package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/assetnav"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/starfederation/datastar-go/datastar"
)

type workspaceAssetProvider interface {
	WorkspaceAssets(workspaceID, deploymentID string) ([]workspace.Asset, []workspace.AssetEdge, bool)
}

var errWorkspaceRBACNotConfigured = errors.New("Workspace RBAC store is not configured.")

func (s *Server) workspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := s.workspaceList(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspacesPage(s.metrics.Catalog(), workspaces, s.currentRoleLabel(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) workspaceAssets(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	switch r.URL.Query().Get("type") {
	case "connection":
		http.Redirect(w, r, assetnav.ConnectionsHref(r.URL.Query().Get("q")), http.StatusFound)
		return
	case "source":
		http.Redirect(w, r, assetnav.ConnectionsHrefWithType("source", r.URL.Query().Get("q")), http.StatusFound)
		return
	}
	assets, _, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	filtered := filterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))
	workspace := s.workspaceResponse(r, workspaceID)
	canManage := s.canManageWorkspaceAccess(r, workspaceID)
	access := s.workspaceAccessResponse(r, workspace, canManage, ui.WorkspaceAccessStatus{})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspacePage(s.metrics.Catalog(), workspace, filtered, r.URL.Query().Get("type"), r.URL.Query().Get("q"), s.currentRoleLabel(r), access, csrfToken(r, s.auth)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID("")
	assets, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	activeType := normalizeConnectionAssetType(r.URL.Query().Get("type"))
	filtered := filterConnectionAssets(assets, activeType, r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ConnectionsPage(s.metrics.Catalog(), workspaceID, filtered, edges, activeType, r.URL.Query().Get("q"), s.currentRoleLabel(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) workspaceAsset(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assetID := chi.URLParam(r, "asset")
	assets, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	var selected api.AssetResponse
	for _, asset := range assets {
		if asset.ID == assetID {
			selected = asset
			break
		}
	}
	if selected.ID == "" {
		http.NotFound(w, r)
		return
	}
	if selected.Type == "connection" {
		http.Redirect(w, r, assetnav.ConnectionAssetSectionHref(assetID, "details"), http.StatusFound)
		return
	}
	if selected.Type == "source" {
		http.Redirect(w, r, assetnav.CanonicalSourceAssetSectionHref(workspaceID, selected.ID, "details", edges), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/workspaces/"+workspaceID+"/assets/"+assetID+"/details", http.StatusFound)
}

func (s *Server) workspaceAssetSection(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	redirectToDetails := false
	if section == "definition" {
		section = "details"
		redirectToDetails = true
	}
	if !ui.ValidWorkspaceAssetSection(section) {
		http.NotFound(w, r)
		return
	}
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assets, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	assetID := chi.URLParam(r, "asset")
	var selected api.AssetResponse
	for _, asset := range assets {
		if asset.ID == assetID {
			selected = asset
			break
		}
	}
	if selected.ID == "" {
		http.NotFound(w, r)
		return
	}
	if selected.Type == "connection" {
		http.Redirect(w, r, assetnav.ConnectionAssetSectionHref(assetID, section), http.StatusFound)
		return
	}
	if selected.Type == "source" {
		http.Redirect(w, r, assetnav.CanonicalSourceAssetSectionHref(workspaceID, selected.ID, section, edges), http.StatusFound)
		return
	}
	if redirectToDetails {
		http.Redirect(w, r, "/workspaces/"+workspaceID+"/assets/"+assetID+"/details", http.StatusFound)
		return
	}
	workspace := s.workspaceResponse(r, workspaceID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspaceAssetPage(s.metrics.Catalog(), workspace, selected, assets, edges, section, s.currentRoleLabel(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) connectionAsset(w http.ResponseWriter, r *http.Request) {
	assetID := chi.URLParam(r, "asset")
	http.Redirect(w, r, assetnav.ConnectionAssetSectionHref(assetID, "details"), http.StatusFound)
}

func (s *Server) connectionSourceAsset(w http.ResponseWriter, r *http.Request) {
	connectionID := chi.URLParam(r, "connection")
	sourceID := chi.URLParam(r, "source")
	workspaceID := s.workspaceID("")
	assets, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	if _, _, ok := connectionSourcePair(assets, edges, connectionID, sourceID); !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, assetnav.ConnectionSourceAssetSectionHref(connectionID, sourceID, "details"), http.StatusFound)
}

func (s *Server) connectionSourceAssetSection(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	if section == "definition" {
		http.Redirect(w, r, assetnav.ConnectionSourceAssetSectionHref(chi.URLParam(r, "connection"), chi.URLParam(r, "source"), "details"), http.StatusFound)
		return
	}
	if !ui.ValidWorkspaceAssetSection(section) {
		http.NotFound(w, r)
		return
	}
	workspaceID := s.workspaceID("")
	assets, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	connection, source, ok := connectionSourcePair(assets, edges, chi.URLParam(r, "connection"), chi.URLParam(r, "source"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	workspace := s.workspaceResponse(r, workspaceID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ConnectionSourceAssetPage(s.metrics.Catalog(), workspace, connection, source, assets, edges, section, s.currentRoleLabel(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func connectionSourcePair(assets []api.AssetResponse, edges []api.AssetEdgeResponse, connectionID, sourceID string) (api.AssetResponse, api.AssetResponse, bool) {
	connection, ok := assetByID(assets, connectionID)
	if !ok || connection.Type != "connection" {
		return api.AssetResponse{}, api.AssetResponse{}, false
	}
	source, ok := assetByID(assets, sourceID)
	if !ok || source.Type != "source" || assetnav.SourceConnectionID(source.ID, edges) != connection.ID {
		return api.AssetResponse{}, api.AssetResponse{}, false
	}
	return connection, source, true
}

func (s *Server) connectionAssetSection(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	if section == "definition" {
		http.Redirect(w, r, assetnav.ConnectionAssetSectionHref(chi.URLParam(r, "asset"), "details"), http.StatusFound)
		return
	}
	if !ui.ValidWorkspaceAssetSection(section) {
		http.NotFound(w, r)
		return
	}
	workspaceID := s.workspaceID("")
	assets, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	assetID := chi.URLParam(r, "asset")
	var selected api.AssetResponse
	for _, asset := range assets {
		if asset.ID == assetID {
			selected = asset
			break
		}
	}
	if selected.ID == "" || selected.Type != "connection" {
		http.NotFound(w, r)
		return
	}
	workspace := s.workspaceResponse(r, workspaceID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ConnectionAssetPage(s.metrics.Catalog(), workspace, selected, assets, edges, section, s.currentRoleLabel(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) workspacePermissions(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	bindings, roles, err := s.roleBindingsAndRoles(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspacePermissionsPage(s.metrics.Catalog(), s.workspaceResponse(r, workspaceID), bindings, roles, csrfToken(r, s.auth), s.currentRoleLabel(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) updateWorkspacePermission(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if repo == nil {
		http.Error(w, errWorkspaceRBACNotConfigured.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := repo.SetPrincipalRole(r.Context(), access.PrincipalRoleInput{WorkspaceID: workspaceID, Email: r.FormValue("email"), DisplayName: r.FormValue("displayName"), Role: r.FormValue("role")}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/workspaces/"+workspaceID+"/permissions", http.StatusFound)
}

func (s *Server) removeWorkspacePermission(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	repo, err := s.accessRepository()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if repo == nil {
		http.Error(w, errWorkspaceRBACNotConfigured.Error(), http.StatusInternalServerError)
		return
	}
	if err := repo.RemovePrincipalRoles(r.Context(), workspaceID, r.FormValue("principalId")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/workspaces/"+workspaceID+"/permissions", http.StatusFound)
}

type workspaceAccessSignalPayload struct {
	WorkspaceAccess struct {
		Command ui.WorkspaceAccessCommand `json:"command"`
	} `json:"workspaceAccess"`
	WorkspaceAccessCommand ui.WorkspaceAccessCommand `json:"workspaceAccessCommand"`
}

func (signals workspaceAccessSignalPayload) command() ui.WorkspaceAccessCommand {
	command := signals.WorkspaceAccess.Command
	if command.Email == "" && command.Role == "" && command.PrincipalID == "" {
		command = signals.WorkspaceAccessCommand
	}
	return command
}

func (s *Server) upsertWorkspaceAccess(w http.ResponseWriter, r *http.Request) {
	signals := workspaceAccessSignalPayload{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	command := signals.command()
	status := ui.WorkspaceAccessStatus{Message: "Access updated."}
	repo, err := s.accessRepository()
	if err != nil {
		status = ui.WorkspaceAccessStatus{Error: err.Error()}
	} else if repo == nil {
		status = ui.WorkspaceAccessStatus{Error: errWorkspaceRBACNotConfigured.Error()}
	} else if _, err := repo.SetPrincipalRole(r.Context(), access.PrincipalRoleInput{WorkspaceID: workspaceID, Email: command.Email, Role: command.Role}); err != nil {
		status = ui.WorkspaceAccessStatus{Error: err.Error()}
	}
	s.patchWorkspaceAccess(w, r, workspaceID, status)
}

func (s *Server) removeWorkspaceAccess(w http.ResponseWriter, r *http.Request) {
	signals := workspaceAccessSignalPayload{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	command := signals.command()
	status := ui.WorkspaceAccessStatus{Message: "Access removed."}
	repo, err := s.accessRepository()
	if err != nil {
		status = ui.WorkspaceAccessStatus{Error: err.Error()}
	} else if repo == nil {
		status = ui.WorkspaceAccessStatus{Error: errWorkspaceRBACNotConfigured.Error()}
	} else if err := repo.RemovePrincipalRoles(r.Context(), workspaceID, command.PrincipalID); err != nil {
		status = ui.WorkspaceAccessStatus{Error: err.Error()}
	}
	s.patchWorkspaceAccess(w, r, workspaceID, status)
}

func (s *Server) patchWorkspaceAccess(w http.ResponseWriter, r *http.Request, workspaceID string, status ui.WorkspaceAccessStatus) {
	workspace := s.workspaceResponse(r, workspaceID)
	access := s.workspaceAccessResponse(r, workspace, true, status)
	sse := datastar.NewSSE(w, r)
	_ = sse.MarshalAndPatchSignals(map[string]any{
		"workspaceAccess": ui.WorkspaceAccessSignals(access, csrfToken(r, s.auth)),
	})
}

func (s *Server) apiWorkspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := s.workspaceList(r)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, pagedResponse(workspaces))
}

func (s *Server) apiWorkspaceAssets(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assets, _, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, pagedResponse(filterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))))
}

func (s *Server) apiWorkspaceAssetEdges(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	_, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, pagedResponse(edges))
}

func (s *Server) apiWorkspaceRoles(w http.ResponseWriter, r *http.Request) {
	_, roles, err := s.roleBindingsAndRoles(r, s.workspaceID(chi.URLParam(r, "workspace")))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, pagedResponse(roles))
}

func (s *Server) apiRoleBindings(w http.ResponseWriter, r *http.Request) {
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if repo == nil {
		writeJSON(w, http.StatusOK, pagedResponse([]map[string]any{}))
		return
	}
	bindings, err := repo.ListRoleBindings(r.Context(), s.workspaceID(chi.URLParam(r, "workspace")))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, apiRoleBindingDTO(binding))
	}
	writeJSON(w, http.StatusOK, pagedResponse(out))
}

func (s *Server) apiUpsertRoleBinding(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if repo == nil {
		writeJSONError(w, errWorkspaceRBACNotConfigured, http.StatusInternalServerError)
		return
	}
	principal, err := repo.SetPrincipalRole(r.Context(), access.PrincipalRoleInput{WorkspaceID: workspaceID, Email: input.Email, DisplayName: input.DisplayName, Role: input.Role})
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"principalId": principal.ID})
}

func (s *Server) apiDeleteRoleBinding(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if repo == nil {
		writeJSONError(w, errWorkspaceRBACNotConfigured, http.StatusInternalServerError)
		return
	}
	bindingID := chi.URLParam(r, "binding")
	if bindingID == "" {
		bindingID = chi.URLParam(r, "principal")
	}
	if err := repo.DeleteRoleBinding(r.Context(), workspaceID, bindingID); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) workspaceList(r *http.Request) ([]api.WorkspaceResponse, error) {
	repo, err := s.workspaceRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return []api.WorkspaceResponse{catalogWorkspaceResponse(s.metrics.Catalog())}, nil
	}
	rows, err := repo.List(r.Context())
	if err != nil {
		return nil, err
	}
	out := make([]api.WorkspaceResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, workspaceDTO(row))
	}
	return out, nil
}

func (s *Server) workspaceResponse(r *http.Request, workspaceID string) api.WorkspaceResponse {
	if repo, _ := s.workspaceRepository(); repo != nil {
		if row, err := repo.ByID(r.Context(), workspace.WorkspaceID(workspaceID)); err == nil {
			return workspaceDTO(row)
		}
	}
	workspace := catalogWorkspaceResponse(s.metrics.Catalog())
	workspace.ID = workspaceID
	return workspace
}

func (s *Server) workspaceAssetsAndEdges(r *http.Request, workspaceID string) ([]api.AssetResponse, []api.AssetEdgeResponse, error) {
	catalog, ok, err := s.workspaceAssetCatalog(r.Context(), workspaceID)
	if err != nil || !ok {
		return nil, nil, err
	}
	assets := make([]api.AssetResponse, 0, len(catalog.Assets))
	for _, row := range catalog.Assets {
		assets = append(assets, assetDTOFromCatalog(row))
	}
	edges := make([]api.AssetEdgeResponse, 0, len(catalog.Edges))
	for _, row := range catalog.Edges {
		edges = append(edges, assetEdgeDTOFromCatalog(row))
	}
	return assets, edges, nil
}

func (s *Server) workspaceAssetCatalog(ctx context.Context, workspaceID string) (workspace.AssetCatalog, bool, error) {
	if s.store != nil {
		repo, err := s.workspaceRepository()
		if err != nil {
			return workspace.AssetCatalog{}, false, err
		}
		catalog, ok, err := workspace.NewAssetCatalogService(repo).ActiveAssetCatalog(ctx, workspace.WorkspaceID(workspaceID))
		if err != nil || ok {
			return catalog, ok, err
		}
	}
	return s.workspaceAssetCatalogFromRuntime(workspaceID)
}

func (s *Server) workspaceGraphFromRuntime(workspaceID, deploymentID string) (workspace.AssetGraph, bool) {
	provider, ok := s.metrics.(workspaceAssetProvider)
	if !ok || deploymentID == "" {
		return workspace.AssetGraph{}, false
	}
	assets, edges, ok := provider.WorkspaceAssets(workspaceID, deploymentID)
	if !ok {
		return workspace.AssetGraph{}, false
	}
	return workspace.AssetGraph{Assets: assets, Edges: edges}, true
}

func (s *Server) workspaceAssetCatalogFromRuntime(workspaceID string) (workspace.AssetCatalog, bool, error) {
	graph, ok := s.workspaceGraphFromRuntime(workspaceID, "local")
	if !ok {
		return workspace.AssetCatalog{}, false, nil
	}
	catalog, err := workspace.DecodeAssetCatalog(graph)
	return catalog, err == nil, err
}

func (s *Server) roleBindingsAndRoles(r *http.Request, workspaceID string) ([]api.RoleBindingResponse, []api.RoleResponse, error) {
	repo, err := s.accessRepository()
	if err != nil {
		return nil, nil, err
	}
	if repo == nil {
		return nil, defaultWorkspaceRoles(), nil
	}
	bindingRows, err := repo.ListRoleBindings(r.Context(), workspaceID)
	if err != nil {
		return nil, nil, err
	}
	roleRows, err := repo.ListRoles(r.Context())
	if err != nil {
		return nil, nil, err
	}
	bindings := make([]api.RoleBindingResponse, 0, len(bindingRows))
	for _, row := range bindingRows {
		bindings = append(bindings, roleBindingDTO(row))
	}
	roles := make([]api.RoleResponse, 0, len(roleRows))
	for _, row := range roleRows {
		roles = append(roles, roleDTO(row))
	}
	return bindings, roles, nil
}

func (s *Server) workspaceAccessResponse(r *http.Request, workspace api.WorkspaceResponse, canManage bool, status ui.WorkspaceAccessStatus) ui.WorkspaceAccessResponse {
	bindings, roles, err := s.roleBindingsAndRoles(r, workspace.ID)
	if err != nil && status.Error == "" {
		status.Error = err.Error()
	}
	return ui.WorkspaceAccessResponse{
		Workspace: workspace,
		Roles:     roles,
		Bindings:  bindings,
		CanManage: canManage,
		Status:    status,
	}
}

func (s *Server) canManageWorkspaceAccess(r *http.Request, workspaceID string) bool {
	if s.auth == nil {
		return true
	}
	repo, err := s.accessRepository()
	if err != nil || repo == nil {
		return false
	}
	principal, ok := s.auth.Principal(r)
	if !ok {
		return false
	}
	if principal.DevBypass {
		return true
	}
	allowed, err := repo.HasPermission(r.Context(), workspaceID, principal.ID, access.PermissionRBACManage)
	return err == nil && allowed
}

func defaultWorkspaceRoles() []api.RoleResponse {
	return []api.RoleResponse{
		{Name: access.RoleViewer},
		{Name: access.RoleEditor},
		{Name: access.RoleDeployer},
		{Name: access.RoleAdmin},
		{Name: access.RoleOwner},
	}
}

func workspaceDTO(row workspace.Summary) api.WorkspaceResponse {
	activeDeploymentID := ""
	if row.ActiveDeploymentID != "" {
		activeDeploymentID = string(row.ActiveDeploymentID)
	}
	return api.WorkspaceResponse{
		ID:                 string(row.ID),
		Title:              row.Title,
		Description:        row.Description,
		ActiveDeploymentID: activeDeploymentID,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

func catalogWorkspaceResponse(catalog dashboard.Catalog) api.WorkspaceResponse {
	return api.WorkspaceResponse{
		ID:          catalog.Workspace.ID,
		Title:       catalog.Workspace.Title,
		Description: catalog.Workspace.Description,
	}
}

func assetDTOFromCatalog(row workspace.AssetRecord) api.AssetResponse {
	return api.AssetResponse{
		ID:            string(row.ID),
		SnapshotID:    string(row.SnapshotID),
		WorkspaceID:   string(row.WorkspaceID),
		DeploymentID:  string(row.DeploymentID),
		Type:          string(row.Type),
		Key:           row.Key,
		ParentID:      string(row.ParentID),
		Title:         row.Title,
		Description:   row.Description,
		PayloadSchema: row.PayloadSchema,
		Payload:       row.Payload,
		Href:          assetHref(string(row.Type), row.Key),
	}
}

func assetEdgeDTOFromCatalog(row workspace.AssetEdgeRecord) api.AssetEdgeResponse {
	return api.AssetEdgeResponse{
		ID:           string(row.ID),
		WorkspaceID:  string(row.WorkspaceID),
		DeploymentID: string(row.DeploymentID),
		FromAssetID:  string(row.FromAssetID),
		ToAssetID:    string(row.ToAssetID),
		Type:         string(row.Type),
	}
}

func roleBindingDTO(row access.RoleBinding) api.RoleBindingResponse {
	return api.RoleBindingResponse{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		SubjectType: string(row.SubjectType),
		SubjectID:   row.SubjectID,
		PrincipalID: row.PrincipalID,
		GroupID:     row.GroupID,
		Email:       row.Email,
		DisplayName: firstNonEmpty(row.DisplayName, row.GroupName),
		GroupName:   row.GroupName,
		Role:        row.Role,
		CreatedAt:   row.CreatedAt,
	}
}

func roleDTO(row access.Role) api.RoleResponse {
	return api.RoleResponse{Name: row.Name, Permissions: row.Permissions}
}

func assetHref(assetType, key string) string {
	switch assetType {
	case "dashboard":
		return "/dashboards/" + key
	default:
		return ""
	}
}

func filterAssets(assets []api.AssetResponse, typ, query string) []api.AssetResponse {
	typ = strings.TrimSpace(typ)
	query = strings.ToLower(strings.TrimSpace(query))
	if typ == "" && query == "" {
		return assets
	}
	out := make([]api.AssetResponse, 0, len(assets))
	for _, asset := range assets {
		if typ != "" && asset.Type != typ {
			continue
		}
		haystack := strings.ToLower(asset.Type + " " + asset.Key + " " + asset.Title + " " + asset.Description)
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		out = append(out, asset)
	}
	return out
}

func filterWorkspaceAssets(assets []api.AssetResponse, typ, query string) []api.AssetResponse {
	typ = strings.TrimSpace(typ)
	query = strings.TrimSpace(query)
	if typ != "" || query != "" {
		return filterAssets(assets, typ, query)
	}
	out := make([]api.AssetResponse, 0, len(assets))
	for _, asset := range assets {
		if isWorkspaceLandingAsset(asset.Type) {
			out = append(out, asset)
		}
	}
	return out
}

func filterConnectionAssets(assets []api.AssetResponse, typ, query string) []api.AssetResponse {
	typ = normalizeConnectionAssetType(typ)
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]api.AssetResponse, 0, len(assets))
	for _, asset := range assets {
		if asset.Type != "connection" && asset.Type != "source" {
			continue
		}
		if typ != "" && asset.Type != typ {
			continue
		}
		haystack := strings.ToLower(asset.Type + " " + asset.Key + " " + asset.Title + " " + asset.Description)
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		out = append(out, asset)
	}
	return out
}

func normalizeConnectionAssetType(typ string) string {
	switch strings.TrimSpace(typ) {
	case "connection", "source":
		return strings.TrimSpace(typ)
	default:
		return ""
	}
}

func assetByID(assets []api.AssetResponse, id string) (api.AssetResponse, bool) {
	for _, asset := range assets {
		if asset.ID == id {
			return asset, true
		}
	}
	return api.AssetResponse{}, false
}

func isWorkspaceLandingAsset(typ string) bool {
	switch typ {
	case "model_table", "semantic_model", "dashboard":
		return true
	default:
		return false
	}
}

func csrfToken(r *http.Request, auth *Auth) string {
	if auth == nil {
		return ""
	}
	return csrf.Token(r)
}

func (s *Server) currentRoleLabel(r *http.Request) string {
	if s.auth == nil {
		return "Local workspace"
	}
	principal, ok := s.auth.Principal(r)
	if !ok {
		return "Workspace access"
	}
	if principal.DevBypass {
		return "Developer access"
	}
	repo, err := s.accessRepository()
	if err != nil || repo == nil {
		return "Workspace access"
	}
	rows, err := repo.ListRoleBindings(r.Context(), s.workspaceID(""))
	if err != nil {
		return "Workspace access"
	}
	for _, row := range rows {
		if row.PrincipalID == principal.ID {
			return row.Role
		}
	}
	return "Workspace access"
}
