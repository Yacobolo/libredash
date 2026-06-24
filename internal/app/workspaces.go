package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/api"
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
	if r.URL.Query().Get("type") == "connection" {
		http.Redirect(w, r, connectionsHref(r.URL.Query().Get("q")), http.StatusFound)
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
	access := s.workspaceAccessResponse(r, workspace, canManage, api.WorkspaceAccessStatus{})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspacePage(s.metrics.Catalog(), workspace, filtered, r.URL.Query().Get("type"), r.URL.Query().Get("q"), s.currentRoleLabel(r), access, csrfToken(r, s.auth)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID("")
	assets, _, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	filtered := filterAssets(assets, "connection", r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ConnectionsPage(s.metrics.Catalog(), workspaceID, filtered, r.URL.Query().Get("q"), s.currentRoleLabel(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) workspaceAsset(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assetID := chi.URLParam(r, "asset")
	assets, _, err := s.workspaceAssetsAndEdges(r, workspaceID)
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
		http.Redirect(w, r, connectionAssetSectionHref(assetID, "details"), http.StatusFound)
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
		http.Redirect(w, r, connectionAssetSectionHref(assetID, section), http.StatusFound)
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
	http.Redirect(w, r, connectionAssetSectionHref(assetID, "details"), http.StatusFound)
}

func (s *Server) connectionAssetSection(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	if section == "definition" {
		http.Redirect(w, r, connectionAssetSectionHref(chi.URLParam(r, "asset"), "details"), http.StatusFound)
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
		Command api.WorkspaceAccessCommand `json:"command"`
	} `json:"workspaceAccess"`
	WorkspaceAccessCommand api.WorkspaceAccessCommand `json:"workspaceAccessCommand"`
}

func (signals workspaceAccessSignalPayload) command() api.WorkspaceAccessCommand {
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
	status := api.WorkspaceAccessStatus{Message: "Access updated."}
	repo, err := s.accessRepository()
	if err != nil {
		status = api.WorkspaceAccessStatus{Error: err.Error()}
	} else if repo == nil {
		status = api.WorkspaceAccessStatus{Error: errWorkspaceRBACNotConfigured.Error()}
	} else if _, err := repo.SetPrincipalRole(r.Context(), access.PrincipalRoleInput{WorkspaceID: workspaceID, Email: command.Email, Role: command.Role}); err != nil {
		status = api.WorkspaceAccessStatus{Error: err.Error()}
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
	status := api.WorkspaceAccessStatus{Message: "Access removed."}
	repo, err := s.accessRepository()
	if err != nil {
		status = api.WorkspaceAccessStatus{Error: err.Error()}
	} else if repo == nil {
		status = api.WorkspaceAccessStatus{Error: errWorkspaceRBACNotConfigured.Error()}
	} else if err := repo.RemovePrincipalRoles(r.Context(), workspaceID, command.PrincipalID); err != nil {
		status = api.WorkspaceAccessStatus{Error: err.Error()}
	}
	s.patchWorkspaceAccess(w, r, workspaceID, status)
}

func (s *Server) patchWorkspaceAccess(w http.ResponseWriter, r *http.Request, workspaceID string, status api.WorkspaceAccessStatus) {
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
	writeJSON(w, http.StatusOK, workspaces)
}

func (s *Server) apiWorkspaceAssets(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assets, _, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, filterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q")))
}

func (s *Server) apiWorkspaceAssetEdges(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	_, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	writeJSON(w, http.StatusOK, edges)
}

func (s *Server) apiWorkspaceRoles(w http.ResponseWriter, r *http.Request) {
	_, roles, err := s.roleBindingsAndRoles(r, s.workspaceID(chi.URLParam(r, "workspace")))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (s *Server) apiRoleBindings(w http.ResponseWriter, r *http.Request) {
	bindings, _, err := s.roleBindingsAndRoles(r, s.workspaceID(chi.URLParam(r, "workspace")))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, bindings)
}

func (s *Server) apiUpsertRoleBinding(w http.ResponseWriter, r *http.Request) {
	var input api.RoleBindingUpsertRequest
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
	if err := repo.RemovePrincipalRoles(r.Context(), workspaceID, chi.URLParam(r, "principal")); err != nil {
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
	if s.store == nil {
		if assets, edges, ok := s.workspaceAssetsFromRuntime(workspaceID); ok {
			return assets, edges, nil
		}
		return fallbackAssets(s.metrics.Catalog(), workspaceID), nil, nil
	}
	repo, err := s.workspaceRepository()
	if err != nil {
		return nil, nil, err
	}
	graph, ok, err := repo.ActiveDeploymentGraph(r.Context(), workspace.WorkspaceID(workspaceID))
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		if assets, edges, ok := s.workspaceAssetsFromRuntime(workspaceID); ok {
			return assets, edges, nil
		}
		return nil, nil, nil
	}
	if staleActiveLineageGraph(graph) {
		if refreshed, ok := s.workspaceGraphFromRuntime(workspaceID, string(activeGraphDeploymentID(graph))); ok && !staleActiveLineageGraph(refreshed) {
			if err := repo.ReplaceActiveDeploymentGraph(r.Context(), workspace.WorkspaceID(workspaceID), refreshed); err == nil {
				graph = refreshed
			}
		}
	}
	assets := make([]api.AssetResponse, 0, len(graph.Assets))
	for _, row := range graph.Assets {
		assets = append(assets, assetDTOFromWorkspace(row))
	}
	edges := make([]api.AssetEdgeResponse, 0, len(graph.Edges))
	for _, row := range graph.Edges {
		edges = append(edges, assetEdgeDTOFromWorkspace(row))
	}
	return assets, edges, nil
}

func activeGraphDeploymentID(graph workspace.AssetGraph) workspace.DeploymentID {
	for _, asset := range graph.Assets {
		if asset.DeploymentID != "" {
			return asset.DeploymentID
		}
	}
	for _, edge := range graph.Edges {
		if edge.DeploymentID != "" {
			return edge.DeploymentID
		}
	}
	return ""
}

func staleActiveLineageGraph(graph workspace.AssetGraph) bool {
	hasLegacyModel := false
	hasSemanticTable := false
	hasRelationship := false
	hasPageItem := false
	hasRelationshipDefinition := false
	hasPageItemDefinition := false
	assets := map[workspace.AssetID]workspace.Asset{}
	for _, asset := range graph.Assets {
		assets[asset.ID] = asset
		if asset.ContentVersion < workspace.CurrentAssetContentVersion {
			return true
		}
		if assetHasLegacyGeneratedDescription(asset) {
			return true
		}
		if assetHasDocumentedFieldsWithoutSchema(asset) {
			return true
		}
		switch asset.Type {
		case "semantic_model", "dashboard":
			hasLegacyModel = true
		case "semantic_table":
			hasSemanticTable = true
		case "relationship":
			hasRelationship = true
		case "page_item":
			hasPageItem = true
		}
		if asset.Type == "semantic_model" && assetContentHasItems(asset.ContentJSON, "Relationships", "relationships") {
			hasRelationshipDefinition = true
		}
		if asset.Type == "dashboard" && dashboardContentHasPageItems(asset.ContentJSON) {
			hasPageItemDefinition = true
		}
	}
	if !hasLegacyModel {
		return false
	}
	if !hasSemanticTable {
		return true
	}
	if hasRelationshipDefinition && !hasRelationship {
		return true
	}
	if hasPageItemDefinition && !hasPageItem {
		return true
	}
	for _, edge := range graph.Edges {
		from := assets[edge.FromAssetID]
		if from.Type != "dashboard" {
			continue
		}
		switch edge.Type {
		case workspace.AssetEdgeUsesField, workspace.AssetEdgeUsesMeasure, workspace.AssetEdgeUsesModelTable:
			return true
		}
	}
	return false
}

func assetHasDocumentedFieldsWithoutSchema(asset workspace.Asset) bool {
	if asset.Type != workspace.AssetTypeSource && asset.Type != workspace.AssetTypeModelTable {
		return false
	}
	if assetContentHasItems(asset.ContentJSON, "Schema", "schema") {
		return false
	}
	if asset.Type == workspace.AssetTypeModelTable {
		return assetContentHasItems(asset.ContentJSON, "Dimensions", "dimensions", "Fields", "fields")
	}
	return assetContentHasItems(asset.ContentJSON, "Fields", "fields")
}

func assetHasLegacyGeneratedDescription(asset workspace.Asset) bool {
	description := strings.TrimSpace(asset.Description)
	if description == "" {
		return false
	}
	switch asset.Type {
	case workspace.AssetTypeConnection:
		var content map[string]any
		if err := json.Unmarshal([]byte(asset.ContentJSON), &content); err != nil {
			return false
		}
		kind, _ := content["Kind"].(string)
		if kind == "" {
			kind, _ = content["kind"].(string)
		}
		return kind != "" && description == kind+" connection"
	case workspace.AssetTypeSource:
		var content map[string]any
		if err := json.Unmarshal([]byte(asset.ContentJSON), &content); err != nil {
			return false
		}
		format, _ := content["Format"].(string)
		if format == "" {
			format, _ = content["format"].(string)
		}
		path, _ := content["Path"].(string)
		if path == "" {
			path, _ = content["path"].(string)
		}
		object, _ := content["Object"].(string)
		if object == "" {
			object, _ = content["object"].(string)
		}
		if object != "" && description == "object: "+object {
			return true
		}
		if format == "" || path == "" {
			return false
		}
		return description == format+" file: "+path || description == format+" table: "+path
	default:
		return false
	}
}

func assetContentHasItems(raw string, keys ...string) bool {
	var content map[string]any
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return false
	}
	for _, key := range keys {
		switch value := content[key].(type) {
		case []any:
			if len(value) > 0 {
				return true
			}
		case map[string]any:
			if len(value) > 0 {
				return true
			}
		}
	}
	return false
}

func dashboardContentHasPageItems(raw string) bool {
	var content map[string]any
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return false
	}
	for _, key := range []string{"Pages", "pages"} {
		pages, ok := content[key].([]any)
		if !ok {
			continue
		}
		for _, item := range pages {
			page, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"Visuals", "visuals", "Items", "items"} {
				if items, ok := page[key].([]any); ok && len(items) > 0 {
					return true
				}
			}
		}
	}
	return false
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

func (s *Server) workspaceAssetsFromRuntime(workspaceID string) ([]api.AssetResponse, []api.AssetEdgeResponse, bool) {
	graph, ok := s.workspaceGraphFromRuntime(workspaceID, "local")
	if !ok {
		return nil, nil, false
	}
	assets := make([]api.AssetResponse, 0, len(graph.Assets))
	for _, row := range graph.Assets {
		assets = append(assets, assetDTOFromWorkspace(row))
	}
	edges := make([]api.AssetEdgeResponse, 0, len(graph.Edges))
	for _, row := range graph.Edges {
		edges = append(edges, assetEdgeDTOFromWorkspace(row))
	}
	return assets, edges, true
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

func (s *Server) workspaceAccessResponse(r *http.Request, workspace api.WorkspaceResponse, canManage bool, status api.WorkspaceAccessStatus) api.WorkspaceAccessResponse {
	bindings, roles, err := s.roleBindingsAndRoles(r, workspace.ID)
	if err != nil && status.Error == "" {
		status.Error = err.Error()
	}
	return api.WorkspaceAccessResponse{
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

func assetDTOFromWorkspace(row workspace.Asset) api.AssetResponse {
	return api.AssetResponse{
		ID:           string(row.ID),
		WorkspaceID:  string(row.WorkspaceID),
		DeploymentID: string(row.DeploymentID),
		Type:         string(row.Type),
		Key:          row.Key,
		ParentID:     string(row.ParentID),
		Title:        row.Title,
		Description:  row.Description,
		Meta:         safeAssetMeta(string(row.Type), row.ContentJSON),
		Href:         assetHref(string(row.Type), row.Key),
	}
}

func assetEdgeDTOFromWorkspace(row workspace.AssetEdge) api.AssetEdgeResponse {
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
		PrincipalID: row.PrincipalID,
		Email:       row.Email,
		DisplayName: row.DisplayName,
		Role:        row.Role,
		CreatedAt:   row.CreatedAt,
	}
}

func roleDTO(row access.Role) api.RoleResponse {
	return api.RoleResponse{Name: row.Name, Permissions: row.Permissions}
}

func safeAssetMeta(assetType, raw string) map[string]any {
	var content map[string]any
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return nil
	}
	authConfigured := hasConfiguredAuth(content["auth"]) || hasConfiguredAuth(content["Auth"])
	content = scrubAssetSecrets(content).(map[string]any)
	switch assetType {
	case "connection":
		content["credentials_configured"] = authConfigured
	case "source":
		return pickMeta(content, "format", "Format", "path", "Path", "connection", "Connection", "object", "Object", "options", "Options", "fields", "Fields", "schema", "Schema")
	case "model_table":
		return pickMeta(content,
			"source", "Source",
			"sources", "Sources",
			"source_dependencies", "SourceDependencies",
			"transform", "Transform",
			"sql", "SQL",
			"primary_key", "PrimaryKey",
			"grain", "Grain",
			"dimensions", "Dimensions",
			"fields", "Fields",
			"schema", "Schema",
		)
	case "measure":
		return pickMeta(content, "expression", "Expression", "unit", "Unit", "format", "Format")
	case "field":
		return pickMeta(content, "expr", "Expr", "where", "Where", "order_expr", "OrderExpr")
	case "dashboard":
		return pickMeta(content, "semantic_model", "SemanticModel", "tags", "Tags")
	}
	return content
}

func scrubAssetSecrets(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			if strings.EqualFold(key, "auth") {
				if hasConfiguredAuth(nested) {
					out["credentials_configured"] = true
				}
				continue
			}
			out[key] = scrubAssetSecrets(nested)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, nested := range typed {
			out = append(out, scrubAssetSecrets(nested))
		}
		return out
	default:
		return value
	}
}

func hasConfiguredAuth(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		return len(typed) > 0
	case nil:
		return false
	default:
		return true
	}
}

func pickMeta(content map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		if value, ok := content[key]; ok {
			out[key] = value
		}
	}
	return out
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

func connectionsHref(query string) string {
	href := "/connections"
	if strings.TrimSpace(query) == "" {
		return href
	}
	values := url.Values{}
	values.Set("q", query)
	return href + "?" + values.Encode()
}

func connectionAssetSectionHref(assetID, section string) string {
	return "/connections/" + assetID + "/" + section
}

func isWorkspaceLandingAsset(typ string) bool {
	switch typ {
	case "model_table", "semantic_model", "dashboard":
		return true
	default:
		return false
	}
}

func fallbackAssets(catalog dashboard.Catalog, workspaceID string) []api.AssetResponse {
	assets := []api.AssetResponse{}
	for _, report := range catalog.Dashboards {
		assets = append(assets, api.AssetResponse{ID: "dashboard:" + report.ID, WorkspaceID: workspaceID, Type: "dashboard", Key: report.ID, Title: report.Title, Description: report.Description, Href: "/dashboards/" + report.ID})
	}
	for _, model := range catalog.Models {
		assets = append(assets, api.AssetResponse{ID: "semantic_model:" + model.ID, WorkspaceID: workspaceID, Type: "semantic_model", Key: model.ID, Title: model.Title, Description: model.Description})
	}
	return assets
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
