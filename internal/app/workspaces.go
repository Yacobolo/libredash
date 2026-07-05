package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/gorilla/csrf"
)

type activeWorkspaceMetadataRepository interface {
	ListWithActiveMetadata(context.Context, string) ([]workspace.Summary, error)
	ByIDWithActiveMetadata(context.Context, workspace.WorkspaceID, string) (workspace.Summary, error)
}

func (s *Server) dataDirForWorkspace(workspaceID string, artifact deployment.Artifact) string {
	if strings.TrimSpace(artifact.DataRoot) != "" {
		return artifact.DataRoot
	}
	dataDir := ""
	if workspaceMetrics, ok := s.metrics.(workspaceMetrics); ok {
		if metrics, ok := workspaceMetrics.MetricsForWorkspace(workspaceID); ok && metrics != nil {
			dataDir = metrics.DataDir()
		}
	}
	if strings.TrimSpace(dataDir) == "" && s.metrics != nil {
		dataDir = s.metrics.DataDir()
	}
	workspaceDataDir := filepath.Join(".data", workspaceID)
	if info, err := os.Stat(workspaceDataDir); err == nil && info.IsDir() {
		return workspaceDataDir
	}
	return dataDir
}

func (s *Server) assetVersionsStateForSection(ctx context.Context, workspaceID, environment string, asset workspace.AssetView, section string) (ui.AssetVersionsState, error) {
	state := ui.AssetVersionsState{CurrentDeploymentID: asset.DeploymentID}
	if section != "versions" {
		return state, nil
	}
	if s.store == nil {
		return state, nil
	}
	repo, err := s.workspaceRepository()
	if err != nil || repo == nil {
		return state, err
	}
	versions, err := repo.AssetVersions(ctx, workspace.WorkspaceID(workspaceID), environment, workspace.AssetID(asset.ID))
	if err != nil {
		return state, err
	}
	state.Versions = make([]ui.AssetVersionState, 0, len(versions))
	for _, version := range versions {
		state.Versions = append(state.Versions, ui.AssetVersionState{
			DeploymentID: string(version.DeploymentID),
			Status:       version.Status,
			Digest:       version.Digest,
			CreatedBy:    version.CreatedBy,
			CreatedAt:    version.CreatedAt,
			ActivatedAt:  version.ActivatedAt,
			ContentHash:  version.ContentHash,
		})
	}
	return state, nil
}

func (s *Server) workspaceList(r *http.Request) ([]workspace.WorkspaceView, error) {
	repo, err := s.workspaceRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return []workspace.WorkspaceView{catalogWorkspaceView(s.metrics.Catalog())}, nil
	}
	rows, err := listWorkspaceRows(r, repo, string(s.requestDeploymentEnvironment(r)))
	if err != nil {
		return nil, err
	}
	out := make([]workspace.WorkspaceView, 0, len(rows))
	for _, row := range rows {
		view := workspace.WorkspaceViewFromSummary(row)
		if !s.canReadWorkspace(r, view.ID) {
			continue
		}
		out = append(out, view)
	}
	return out, nil
}

func listWorkspaceRows(r *http.Request, repo workspace.Repository, environment string) ([]workspace.Summary, error) {
	if activeRepo, ok := repo.(activeWorkspaceMetadataRepository); ok {
		return activeRepo.ListWithActiveMetadata(r.Context(), environment)
	}
	return repo.List(r.Context())
}

func (s *Server) canReadWorkspace(r *http.Request, workspaceID string) bool {
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
	allowed, err := repo.HasPermission(r.Context(), workspaceID, principal.ID, access.PermissionWorkspaceRead)
	return err == nil && allowed
}

func (s *Server) catalogForWorkspacesPage(r *http.Request, workspaces []workspace.WorkspaceView) dashboard.Catalog {
	if len(workspaces) == 0 {
		var err error
		workspaces, err = s.workspaceList(r)
		if err != nil {
			workspaces = nil
		}
	}
	if len(workspaces) > 0 {
		return s.catalogForWorkspace(workspaces[0].ID)
	}
	if s.metrics == nil {
		return dashboard.Catalog{}
	}
	return s.metrics.Catalog()
}

func (s *Server) catalogsForVisibleWorkspaces(r *http.Request) []dashboard.Catalog {
	workspaces, err := s.workspaceList(r)
	if err != nil || len(workspaces) == 0 {
		if s.metrics == nil {
			return nil
		}
		return []dashboard.Catalog{s.metrics.Catalog()}
	}
	catalogs := make([]dashboard.Catalog, 0, len(workspaces))
	for _, row := range workspaces {
		metrics, ok := s.metricsForWorkspace(row.ID)
		if !ok || metrics == nil {
			continue
		}
		catalogs = append(catalogs, metrics.Catalog())
	}
	if len(catalogs) == 0 && s.metrics != nil {
		catalogs = append(catalogs, s.metrics.Catalog())
	}
	return catalogs
}

func (s *Server) workspaceResponse(r *http.Request, workspaceID string) workspace.WorkspaceView {
	if repo, _ := s.workspaceRepository(); repo != nil {
		var row workspace.Summary
		var err error
		if activeRepo, ok := repo.(activeWorkspaceMetadataRepository); ok {
			row, err = activeRepo.ByIDWithActiveMetadata(r.Context(), workspace.WorkspaceID(workspaceID), string(s.requestDeploymentEnvironment(r)))
		} else {
			row, err = repo.ByID(r.Context(), workspace.WorkspaceID(workspaceID))
		}
		if err == nil {
			return workspace.WorkspaceViewFromSummary(row)
		}
	}
	view := catalogWorkspaceView(s.catalogForWorkspace(workspaceID))
	view.ID = workspaceID
	return view
}

func (s *Server) workspaceAssetsAndEdges(r *http.Request, workspaceID string) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	catalog, ok, err := s.workspaceAssetCatalog(r.Context(), workspaceID, string(s.requestDeploymentEnvironment(r)))
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return []workspace.AssetView{}, []workspace.AssetEdgeView{}, nil
	}
	return assetCatalogViews(catalog), assetCatalogEdgeViews(catalog), nil
}

func (s *Server) platformConnectionAssetsAndEdges(r *http.Request) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	repo, err := s.workspaceRepository()
	if err != nil || repo == nil {
		return nil, nil, err
	}
	rows, err := repo.List(r.Context())
	if err != nil {
		return nil, nil, err
	}
	environment := string(s.requestDeploymentEnvironment(r))
	assetsByID := map[string]workspace.AssetView{}
	edgeKeys := map[string]workspace.AssetEdgeView{}
	for _, row := range rows {
		catalog, ok, err := s.workspaceAssetCatalog(r.Context(), string(row.ID), environment)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		assets := assetCatalogViews(catalog)
		edges := assetCatalogEdgeViews(catalog)
		localGlobal := map[string]struct{}{}
		for _, asset := range assets {
			if asset.Type != string(workspace.AssetTypeConnection) && asset.Type != string(workspace.AssetTypeSource) {
				continue
			}
			if _, exists := assetsByID[asset.ID]; !exists {
				assetsByID[asset.ID] = asset
			}
			localGlobal[asset.ID] = struct{}{}
		}
		for _, edge := range edges {
			if edge.Type != string(workspace.AssetEdgeUsesConnection) {
				continue
			}
			if _, ok := localGlobal[edge.FromAssetID]; !ok {
				continue
			}
			if _, ok := localGlobal[edge.ToAssetID]; !ok {
				continue
			}
			key := edge.FromAssetID + "|" + edge.ToAssetID + "|" + edge.Type
			if _, exists := edgeKeys[key]; !exists {
				edgeKeys[key] = edge
			}
		}
	}
	assets := make([]workspace.AssetView, 0, len(assetsByID))
	for _, asset := range assetsByID {
		assets = append(assets, asset)
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].ID < assets[j].ID })
	edges := make([]workspace.AssetEdgeView, 0, len(edgeKeys))
	for _, edge := range edgeKeys {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Type != edges[j].Type {
			return edges[i].Type < edges[j].Type
		}
		if edges[i].FromAssetID != edges[j].FromAssetID {
			return edges[i].FromAssetID < edges[j].FromAssetID
		}
		return edges[i].ToAssetID < edges[j].ToAssetID
	})
	return assets, edges, nil
}

func assetCatalogViews(catalog workspace.AssetCatalog) []workspace.AssetView {
	assets := make([]workspace.AssetView, 0, len(catalog.Assets))
	for _, row := range catalog.Assets {
		assets = append(assets, workspace.AssetViewFromCatalogRecord(row))
	}
	return assets
}

func assetCatalogEdgeViews(catalog workspace.AssetCatalog) []workspace.AssetEdgeView {
	edges := make([]workspace.AssetEdgeView, 0, len(catalog.Edges))
	for _, row := range catalog.Edges {
		edges = append(edges, workspace.AssetEdgeViewFromCatalogRecord(row))
	}
	return edges
}

func platformAssetWorkspaceView() workspace.WorkspaceView {
	return workspace.WorkspaceView{ID: "platform", Title: "Global assets", Description: "Global connection and source assets."}
}

func (s *Server) workspaceAssetCatalog(ctx context.Context, workspaceID, environment string) (workspace.AssetCatalog, bool, error) {
	reader, err := s.workspaceAssetCatalogReader()
	if err != nil || reader == nil {
		return workspace.AssetCatalog{}, false, err
	}
	return reader.ActiveAssetCatalog(ctx, workspace.WorkspaceID(workspaceID), environment)
}

func (s *Server) workspaceAssetCatalogReader() (workspace.AssetCatalogReader, error) {
	if s.assetCatalog != nil {
		return s.assetCatalog, nil
	}
	repo, err := s.workspaceRepository()
	if err != nil {
		return nil, err
	}
	service := workspace.NewAssetCatalogService(repo)
	s.assetCatalog = service
	return s.assetCatalog, nil
}

func (s *Server) roleBindingsAndRoles(r *http.Request, workspaceID string) ([]workspace.RoleBindingView, []workspace.RoleView, error) {
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
	bindings := make([]workspace.RoleBindingView, 0, len(bindingRows))
	for _, row := range bindingRows {
		bindings = append(bindings, roleBindingView(row))
	}
	roles := make([]workspace.RoleView, 0, len(roleRows))
	for _, row := range roleRows {
		roles = append(roles, roleView(row))
	}
	return bindings, roles, nil
}

func (s *Server) workspaceAccessResponse(r *http.Request, workspaceView workspace.WorkspaceView, canManage bool, status ui.WorkspaceAccessStatus) ui.WorkspaceAccessResponse {
	bindings, roles, err := s.roleBindingsAndRoles(r, workspaceView.ID)
	if err != nil && status.Error == "" {
		status.Error = err.Error()
	}
	return ui.WorkspaceAccessResponse{
		Workspace: workspaceView,
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
	allowed, err := repo.HasPermission(r.Context(), workspaceID, principal.ID, access.PermissionRBACManage)
	return err == nil && allowed
}

func defaultWorkspaceRoles() []workspace.RoleView {
	return []workspace.RoleView{
		{Name: access.RoleViewer},
		{Name: access.RoleEditor},
		{Name: access.RoleDeployer},
		{Name: access.RoleAdmin},
		{Name: access.RoleOwner},
	}
}

func catalogWorkspaceView(catalog dashboard.Catalog) workspace.WorkspaceView {
	return workspace.WorkspaceView{
		ID:          catalog.Workspace.ID,
		Title:       catalog.Workspace.Title,
		Description: catalog.Workspace.Description,
	}
}

func roleBindingView(row access.RoleBinding) workspace.RoleBindingView {
	return workspace.RoleBindingView{
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

func roleView(row access.Role) workspace.RoleView {
	return workspace.RoleView{Name: row.Name, Permissions: row.Permissions}
}

func csrfToken(r *http.Request, auth *Auth) string {
	if auth == nil {
		return ""
	}
	return csrf.Token(r)
}

func (s *Server) currentRoleLabel(r *http.Request) string {
	if s.auth == nil {
		return "Local"
	}
	principal, ok := s.auth.Principal(r)
	if !ok {
		return "Signed out"
	}
	if principal.DevBypass {
		return "Platform admin"
	}
	return "Platform access"
}
