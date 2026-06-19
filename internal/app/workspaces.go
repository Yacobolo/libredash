package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
)

type workspaceAssetProvider interface {
	WorkspaceAssets(workspaceID, deploymentID string) ([]platform.Asset, []platform.AssetEdge, bool)
}

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
	assets, _, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	filtered := filterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))
	workspace := s.workspaceResponse(r, workspaceID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspacePage(s.metrics.Catalog(), workspace, filtered, r.URL.Query().Get("type"), r.URL.Query().Get("q"), s.currentRoleLabel(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/workspaces/"+s.workspaceID("")+"?type=connection", http.StatusFound)
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
	http.Redirect(w, r, "/workspaces/"+workspaceID+"/assets/"+assetID+"/details", http.StatusFound)
}

func (s *Server) workspaceAssetSection(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	if section == "definition" {
		workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
		assetID := chi.URLParam(r, "asset")
		http.Redirect(w, r, "/workspaces/"+workspaceID+"/assets/"+assetID+"/details", http.StatusFound)
		return
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
	workspace := s.workspaceResponse(r, workspaceID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspaceAssetPage(s.metrics.Catalog(), workspace, selected, assets, edges, section, s.currentRoleLabel(r)).Render(w); err != nil {
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
	if _, err := s.store.SetPrincipalRole(r.Context(), workspaceID, r.FormValue("email"), r.FormValue("displayName"), r.FormValue("role")); err != nil {
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
	if err := s.store.RemovePrincipalRoles(r.Context(), workspaceID, r.FormValue("principalId")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/workspaces/"+workspaceID+"/permissions", http.StatusFound)
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
	principal, err := s.store.SetPrincipalRole(r.Context(), workspaceID, input.Email, input.DisplayName, input.Role)
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"principalId": principal.ID})
}

func (s *Server) apiDeleteRoleBinding(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	if err := s.store.RemovePrincipalRoles(r.Context(), workspaceID, chi.URLParam(r, "principal")); err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) workspaceList(r *http.Request) ([]api.WorkspaceResponse, error) {
	if s.store == nil {
		return []api.WorkspaceResponse{catalogWorkspaceResponse(s.metrics.Catalog())}, nil
	}
	rows, err := s.store.Queries().ListWorkspaces(r.Context())
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
	if s.store != nil {
		if row, err := s.store.Queries().GetWorkspace(r.Context(), workspaceID); err == nil {
			return workspaceDTO(row)
		}
	}
	workspace := catalogWorkspaceResponse(s.metrics.Catalog())
	workspace.ID = workspaceID
	return workspace
}

func (s *Server) workspaceAssetsAndEdges(r *http.Request, workspaceID string) ([]api.AssetResponse, []api.AssetEdgeResponse, error) {
	if s.store == nil {
		if provider, ok := s.metrics.(workspaceAssetProvider); ok {
			assetRows, edgeRows, ok := provider.WorkspaceAssets(workspaceID, "local")
			if ok {
				assets := make([]api.AssetResponse, 0, len(assetRows))
				for _, row := range assetRows {
					assets = append(assets, assetDTOFromPlatform(row))
				}
				edges := make([]api.AssetEdgeResponse, 0, len(edgeRows))
				for _, row := range edgeRows {
					edges = append(edges, assetEdgeDTOFromPlatform(row))
				}
				return assets, edges, nil
			}
		}
		return fallbackAssets(s.metrics.Catalog(), workspaceID), nil, nil
	}
	deployment, err := s.store.Queries().GetActiveDeployment(r.Context(), workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	assetRows, err := s.store.Queries().ListAssetsByDeployment(r.Context(), deployment.ID)
	if err != nil {
		return nil, nil, err
	}
	edgeRows, err := s.store.Queries().ListAssetEdgesByDeployment(r.Context(), deployment.ID)
	if err != nil {
		return nil, nil, err
	}
	assets := make([]api.AssetResponse, 0, len(assetRows))
	for _, row := range assetRows {
		assets = append(assets, assetDTO(row))
	}
	edges := make([]api.AssetEdgeResponse, 0, len(edgeRows))
	for _, row := range edgeRows {
		edges = append(edges, assetEdgeDTO(row))
	}
	return assets, edges, nil
}

func (s *Server) roleBindingsAndRoles(r *http.Request, workspaceID string) ([]api.RoleBindingResponse, []api.RoleResponse, error) {
	if s.store == nil {
		return nil, nil, nil
	}
	bindingRows, err := s.store.Queries().ListRoleBindingsByWorkspace(r.Context(), workspaceID)
	if err != nil {
		return nil, nil, err
	}
	roleRows, err := s.store.Queries().ListRoles(r.Context())
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

func workspaceDTO(row platformdb.Workspace) api.WorkspaceResponse {
	activeDeploymentID := ""
	if row.ActiveDeploymentID.Valid {
		activeDeploymentID = row.ActiveDeploymentID.String
	}
	return api.WorkspaceResponse{
		ID:                 row.ID,
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

func assetDTO(row platformdb.Asset) api.AssetResponse {
	parentID := ""
	if row.ParentAssetID.Valid {
		parentID = row.ParentAssetID.String
	}
	meta := safeAssetMeta(row.AssetType, row.ContentJson)
	return api.AssetResponse{
		ID:           row.ID,
		WorkspaceID:  row.WorkspaceID,
		DeploymentID: row.DeploymentID,
		Type:         row.AssetType,
		Key:          row.AssetKey,
		ParentID:     parentID,
		Title:        row.Title,
		Description:  row.Description,
		Meta:         meta,
		Href:         assetHref(row.AssetType, row.AssetKey),
	}
}

func assetDTOFromPlatform(row platform.Asset) api.AssetResponse {
	return api.AssetResponse{
		ID:           row.ID,
		WorkspaceID:  row.WorkspaceID,
		DeploymentID: row.DeploymentID,
		Type:         row.Type,
		Key:          row.Key,
		ParentID:     row.ParentID,
		Title:        row.Title,
		Description:  row.Description,
		Meta:         safeAssetMeta(row.Type, row.ContentJSON),
		Href:         assetHref(row.Type, row.Key),
	}
}

func assetEdgeDTO(row platformdb.AssetEdge) api.AssetEdgeResponse {
	return api.AssetEdgeResponse{
		ID:           row.ID,
		WorkspaceID:  row.WorkspaceID,
		DeploymentID: row.DeploymentID,
		FromAssetID:  row.FromAssetID,
		ToAssetID:    row.ToAssetID,
		Type:         row.EdgeType,
	}
}

func assetEdgeDTOFromPlatform(row platform.AssetEdge) api.AssetEdgeResponse {
	return api.AssetEdgeResponse{
		ID:           row.ID,
		WorkspaceID:  row.WorkspaceID,
		DeploymentID: row.DeploymentID,
		FromAssetID:  row.FromAssetID,
		ToAssetID:    row.ToAssetID,
		Type:         row.Type,
	}
}

func roleBindingDTO(row platformdb.ListRoleBindingsByWorkspaceRow) api.RoleBindingResponse {
	return api.RoleBindingResponse{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		PrincipalID: nullString(row.PrincipalID),
		Email:       nullString(row.Email),
		DisplayName: nullString(row.DisplayName),
		Role:        row.RoleName,
		CreatedAt:   row.CreatedAt,
	}
}

func roleDTO(row platformdb.Role) api.RoleResponse {
	var permissions []string
	_ = json.Unmarshal([]byte(row.PermissionsJson), &permissions)
	return api.RoleResponse{Name: row.Name, Permissions: permissions}
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
		return pickMeta(content, "format", "Format", "path", "Path", "connection", "Connection", "object", "Object", "options", "Options")
	case "metric_view":
		return pickMeta(content, "semantic_model", "SemanticModel", "dataset", "Dataset", "timeseries", "Timeseries")
	case "measure":
		return pickMeta(content, "expression", "Expression", "unit", "Unit", "format", "Format")
	case "dimension":
		return pickMeta(content, "expr", "Expr", "where", "Where", "order_expr", "OrderExpr")
	case "dashboard":
		return pickMeta(content, "metrics_views", "MetricViews", "tags", "Tags")
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
	case "semantic_model":
		return "/models/" + key
	case "metric_view":
		return "/metrics/" + key + "/measures"
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

func isWorkspaceLandingAsset(typ string) bool {
	switch typ {
	case "dashboard", "semantic_model", "metric_view":
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
		assets = append(assets, api.AssetResponse{ID: "semantic_model:" + model.ID, WorkspaceID: workspaceID, Type: "semantic_model", Key: model.ID, Title: model.Title, Description: model.Description, Href: "/models/" + model.ID})
	}
	for _, view := range catalog.MetricViews {
		assets = append(assets, api.AssetResponse{ID: "metric_view:" + view.ID, WorkspaceID: workspaceID, Type: "metric_view", Key: view.ID, Title: view.Title, Description: view.Description, Href: "/metrics/" + view.ID + "/measures"})
	}
	return assets
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
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
	if s.store == nil {
		return "Workspace access"
	}
	rows, err := s.store.Queries().ListRoleBindingsByWorkspace(r.Context(), s.workspaceID(""))
	if err != nil {
		return "Workspace access"
	}
	for _, row := range rows {
		if row.PrincipalID.Valid && row.PrincipalID.String == principal.ID {
			return row.RoleName
		}
	}
	return "Workspace access"
}
