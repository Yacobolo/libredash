package http

import (
	"context"
	nethttp "net/http"
	"sort"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Principal struct {
	ID          string
	Email       string
	DisplayName string
	DevBypass   bool
}

type PrincipalProvider func(*nethttp.Request) (Principal, bool)

type ReadModel struct {
	WorkspaceRepository func() (workspace.Repository, error)
	AccessRepository    func() (access.Repository, error)
	AssetCatalogReader  func() (AssetCatalogReader, error)
	MetricsForWorkspace func(string) (Metrics, bool)
	CatalogForWorkspace func(string) dashboard.Catalog
	RootCatalog         func() dashboard.Catalog
	Environment         func(*nethttp.Request) string
	CurrentPrincipal    PrincipalProvider
	AuthConfigured      bool
}

type activeWorkspaceMetadataRepository interface {
	ListWithActiveMetadata(context.Context, string) ([]workspace.Summary, error)
	ByIDWithActiveMetadata(context.Context, workspace.WorkspaceID, string) (workspace.Summary, error)
}

func (m ReadModel) WorkspaceList(r *nethttp.Request) ([]workspace.WorkspaceView, error) {
	repo, err := m.workspaceRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return []workspace.WorkspaceView{CatalogWorkspaceView(m.rootCatalog())}, nil
	}
	rows, err := m.listWorkspaceRows(r.Context(), repo, m.environment(r))
	if err != nil {
		return nil, err
	}
	out := make([]workspace.WorkspaceView, 0, len(rows))
	for _, row := range rows {
		view := workspace.WorkspaceViewFromSummary(row)
		if !m.CanReadWorkspace(r, view.ID) {
			continue
		}
		out = append(out, view)
	}
	return out, nil
}

func (m ReadModel) CatalogsForVisibleWorkspaces(r *nethttp.Request) []dashboard.Catalog {
	workspaces, err := m.WorkspaceList(r)
	if err != nil || len(workspaces) == 0 {
		return []dashboard.Catalog{m.rootCatalog()}
	}
	catalogs := make([]dashboard.Catalog, 0, len(workspaces))
	for _, row := range workspaces {
		metrics, ok := m.metricsForWorkspace(row.ID)
		if !ok || metrics == nil {
			continue
		}
		catalogs = append(catalogs, metrics.Catalog())
	}
	if len(catalogs) == 0 {
		catalogs = append(catalogs, m.rootCatalog())
	}
	return catalogs
}

func (m ReadModel) CatalogForWorkspacesPage(r *nethttp.Request, workspaces []workspace.WorkspaceView) dashboard.Catalog {
	if len(workspaces) == 0 {
		var err error
		workspaces, err = m.WorkspaceList(r)
		if err != nil {
			workspaces = nil
		}
	}
	if len(workspaces) > 0 {
		return m.catalogForWorkspace(workspaces[0].ID)
	}
	return m.rootCatalog()
}

func (m ReadModel) WorkspaceResponse(r *nethttp.Request, workspaceID string) workspace.WorkspaceView {
	repo, _ := m.workspaceRepository()
	if repo != nil {
		var row workspace.Summary
		var err error
		if activeRepo, ok := repo.(activeWorkspaceMetadataRepository); ok {
			row, err = activeRepo.ByIDWithActiveMetadata(r.Context(), workspace.WorkspaceID(workspaceID), m.environment(r))
		} else {
			row, err = repo.ByID(r.Context(), workspace.WorkspaceID(workspaceID))
		}
		if err == nil {
			return workspace.WorkspaceViewFromSummary(row)
		}
	}
	return m.WorkspaceViewContext(r.Context(), workspaceID)
}

func (m ReadModel) WorkspaceViewContext(_ context.Context, workspaceID string) workspace.WorkspaceView {
	view := CatalogWorkspaceView(m.catalogForWorkspace(workspaceID))
	view.ID = workspaceID
	return view
}

func (m ReadModel) WorkspaceAssetsAndEdges(r *nethttp.Request, workspaceID string) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	return m.WorkspaceAssetsAndEdgesForData(r.Context(), workspaceID, m.environment(r))
}

func (m ReadModel) WorkspaceAssetsAndEdgesForData(ctx context.Context, workspaceID, environment string) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	catalog, ok, err := m.activeAssetCatalog(ctx, workspaceID, environment)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return []workspace.AssetView{}, []workspace.AssetEdgeView{}, nil
	}
	return AssetCatalogViews(catalog), AssetCatalogEdgeViews(catalog), nil
}

func (m ReadModel) PlatformAssetsAndEdges(r *nethttp.Request) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	repo, err := m.workspaceRepository()
	if err != nil || repo == nil {
		return nil, nil, err
	}
	rows, err := repo.List(r.Context())
	if err != nil {
		return nil, nil, err
	}
	environment := m.environment(r)
	assetsByID := map[string]workspace.AssetView{}
	edgeKeys := map[string]workspace.AssetEdgeView{}
	for _, row := range rows {
		catalog, ok, err := m.activeAssetCatalog(r.Context(), string(row.ID), environment)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		assets := AssetCatalogViews(catalog)
		edges := AssetCatalogEdgeViews(catalog)
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

func (m ReadModel) RoleBindingsAndRoles(r *nethttp.Request, workspaceID string) ([]workspace.RoleBindingView, []workspace.RoleView, error) {
	repo, err := m.accessRepository()
	if err != nil {
		return nil, nil, err
	}
	if repo == nil {
		return nil, defaultWorkspaceRoleViews(), nil
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
	return bindings, roleViews(roleRows), nil
}

func (m ReadModel) WorkspaceAccess(r *nethttp.Request, workspaceView workspace.WorkspaceView, canManage bool, status ui.WorkspaceAccessStatus) ui.WorkspaceAccessResponse {
	bindings, roles, err := m.RoleBindingsAndRoles(r, workspaceView.ID)
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

func (m ReadModel) CanManageAccess(r *nethttp.Request, workspaceID string) bool {
	if !m.AuthConfigured {
		return true
	}
	repo, err := m.accessRepository()
	if err != nil || repo == nil {
		return false
	}
	principal, ok := m.currentPrincipal(r)
	if !ok {
		return false
	}
	decision, err := repo.Authorize(r.Context(), principal.ID, access.PrivilegeManageGrants, access.WorkspaceObject(workspaceID))
	return err == nil && decision.Allowed
}

func (m ReadModel) CanReadWorkspace(r *nethttp.Request, workspaceID string) bool {
	if !m.AuthConfigured {
		return true
	}
	repo, err := m.accessRepository()
	if err != nil || repo == nil {
		return false
	}
	principal, ok := m.currentPrincipal(r)
	if !ok {
		return false
	}
	decision, err := repo.Authorize(r.Context(), principal.ID, access.PrivilegeUseWorkspace, access.WorkspaceObject(workspaceID))
	return err == nil && decision.Allowed
}

func (m ReadModel) CanReadObject(r *nethttp.Request, object access.ObjectRef) (bool, error) {
	if !m.AuthConfigured {
		return true, nil
	}
	repo, err := m.accessRepository()
	if err != nil || repo == nil {
		return false, err
	}
	principal, ok := m.currentPrincipal(r)
	if !ok {
		return false, nil
	}
	decision, err := repo.Authorize(r.Context(), principal.ID, access.PrivilegeViewItem, object)
	return decision.Allowed, err
}

func (m ReadModel) activeAssetCatalog(ctx context.Context, workspaceID, environment string) (workspace.AssetCatalog, bool, error) {
	reader, err := m.assetCatalogReader()
	if err != nil || reader == nil {
		return workspace.AssetCatalog{}, false, err
	}
	return reader.ActiveAssetCatalog(ctx, workspace.WorkspaceID(workspaceID), environment)
}

func (m ReadModel) listWorkspaceRows(ctx context.Context, repo workspace.Repository, environment string) ([]workspace.Summary, error) {
	if activeRepo, ok := repo.(activeWorkspaceMetadataRepository); ok {
		return activeRepo.ListWithActiveMetadata(ctx, environment)
	}
	return repo.List(ctx)
}

func (m ReadModel) workspaceRepository() (workspace.Repository, error) {
	if m.WorkspaceRepository == nil {
		return nil, nil
	}
	return m.WorkspaceRepository()
}

func (m ReadModel) accessRepository() (access.Repository, error) {
	if m.AccessRepository == nil {
		return nil, nil
	}
	return m.AccessRepository()
}

func (m ReadModel) assetCatalogReader() (AssetCatalogReader, error) {
	if m.AssetCatalogReader == nil {
		return nil, nil
	}
	return m.AssetCatalogReader()
}

func (m ReadModel) metricsForWorkspace(workspaceID string) (Metrics, bool) {
	if m.MetricsForWorkspace == nil {
		return nil, false
	}
	return m.MetricsForWorkspace(workspaceID)
}

func (m ReadModel) catalogForWorkspace(workspaceID string) dashboard.Catalog {
	if m.CatalogForWorkspace == nil {
		return dashboard.Catalog{Workspace: dashboard.CatalogWorkspace{ID: workspaceID}}
	}
	return m.CatalogForWorkspace(workspaceID)
}

func (m ReadModel) rootCatalog() dashboard.Catalog {
	if m.RootCatalog == nil {
		return dashboard.Catalog{}
	}
	return m.RootCatalog()
}

func (m ReadModel) environment(r *nethttp.Request) string {
	if m.Environment == nil {
		return ""
	}
	return m.Environment(r)
}

func (m ReadModel) currentPrincipal(r *nethttp.Request) (Principal, bool) {
	if m.CurrentPrincipal == nil {
		return Principal{}, false
	}
	return m.CurrentPrincipal(r)
}

func AssetCatalogViews(catalog workspace.AssetCatalog) []workspace.AssetView {
	assets := make([]workspace.AssetView, 0, len(catalog.Assets))
	for _, row := range catalog.Assets {
		assets = append(assets, workspace.AssetViewFromCatalogRecord(row))
	}
	return assets
}

func AssetCatalogEdgeViews(catalog workspace.AssetCatalog) []workspace.AssetEdgeView {
	edges := make([]workspace.AssetEdgeView, 0, len(catalog.Edges))
	for _, row := range catalog.Edges {
		edges = append(edges, workspace.AssetEdgeViewFromCatalogRecord(row))
	}
	return edges
}

func CatalogWorkspaceView(catalog dashboard.Catalog) workspace.WorkspaceView {
	return workspace.WorkspaceView{
		ID:          catalog.Workspace.ID,
		Title:       catalog.Workspace.Title,
		Description: catalog.Workspace.Description,
	}
}

func defaultWorkspaceRoleViews() []workspace.RoleView {
	return roleViews(access.DefaultRoles())
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

func roleViews(rows []access.Role) []workspace.RoleView {
	roles := make([]workspace.RoleView, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, workspace.RoleView{Name: row.Name, Privileges: privilegeStrings(row.Privileges)})
	}
	return roles
}

func privilegeStrings(values []access.Privilege) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}
