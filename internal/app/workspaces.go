package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/assetnav"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/starfederation/datastar-go/datastar"
)

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
	filtered := workspace.FilterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))
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
	activeType := workspace.NormalizeConnectionAssetType(r.URL.Query().Get("type"))
	filtered := workspace.FilterConnectionAssets(assets, activeType, r.URL.Query().Get("q"))
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
	var selected workspace.AssetView
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
	var selected workspace.AssetView
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
	if section == "refreshes" && !workspaceAssetRefreshable(selected) {
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
	refresh, err := s.assetRefreshState(r, workspaceID, selected)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	refresh.CSRFToken = csrfToken(r, s.auth)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspaceAssetPageWithRefresh(s.metrics.Catalog(), workspace, selected, assets, edges, section, s.currentRoleLabel(r), refresh).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) refreshWorkspaceAssetMaterializations(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assetID := chi.URLParam(r, "asset")
	assets, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	selected, ok := workspace.AssetByID(assets, assetID)
	if !ok || !workspaceAssetRefreshable(selected) {
		http.NotFound(w, r)
		return
	}
	if s.store == nil {
		http.Error(w, "platform store is required", http.StatusServiceUnavailable)
		return
	}
	if err := s.refreshWorkspaceAssetWithPatches(r, workspaceID, selected, assets, edges); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) workspaceAssetUpdates(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assetID := chi.URLParam(r, "asset")
	section := workspaceAssetUpdateSection(r)
	assets, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	selected, ok := workspace.AssetByID(assets, assetID)
	if !ok || !workspaceAssetRefreshable(selected) {
		http.NotFound(w, r)
		return
	}
	if s.store == nil {
		http.Error(w, "platform store is required", http.StatusServiceUnavailable)
		return
	}

	sse := datastar.NewSSE(w, r)
	if err := sse.MarshalAndPatchSignals(s.workspaceAssetRefreshPatch(r, workspaceID, selected, assets, edges, section)); err != nil {
		return
	}
	updates, unsubscribe := s.broker.Subscribe(workspaceAssetStreamID(workspaceID, assetID, section))
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

func (s *Server) refreshWorkspaceAssetWithPatches(r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	switch asset.Type {
	case string(workspace.AssetTypeSemanticModel):
		return s.refreshSemanticModelAssetWithPatches(r.Context(), r, workspaceID, asset, assets, edges)
	case string(workspace.AssetTypeModelTable):
		return s.refreshModelTableAssetWithPatches(r.Context(), r, workspaceID, asset, assets, edges)
	default:
		return http.ErrMissingFile
	}
}

func (s *Server) refreshSemanticModelAssetWithPatches(ctx context.Context, r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	repo := materialize.NewSQLRunRepository(s.store.SQLDB())
	principal, _ := currentPrincipal(s, r)
	orchestrator := NewRefreshOrchestrator(repo, s.metrics)
	return orchestrator.RefreshSemanticModel(ctx, refreshRunInput{
		WorkspaceID: workspaceID,
		ModelID:     asset.Key,
		PrincipalID: principal.ID,
	}, refreshPublisher{
		Root: func() { s.publishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges) },
		Target: func(targetID string) {
			s.publishWorkspaceAssetRefreshPatchForTarget(r, workspaceID, targetID, assets, edges)
		},
	})
}

func (s *Server) refreshModelTableAssetWithPatches(ctx context.Context, r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	repo := materialize.NewSQLRunRepository(s.store.SQLDB())
	principal, _ := currentPrincipal(s, r)
	modelID, tableName := modelTableTargetParts(asset.Key)
	if modelID == "" || tableName == "" {
		return errors.New("model table asset key is invalid")
	}
	orchestrator := NewRefreshOrchestrator(repo, s.metrics)
	return orchestrator.RefreshModelTable(ctx, refreshRunInput{
		WorkspaceID: workspaceID,
		ModelID:     modelID,
		PrincipalID: principal.ID,
		TargetID:    asset.Key,
	}, tableName, refreshPublisher{
		Root: func() { s.publishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges) },
		Target: func(targetID string) {
			s.publishWorkspaceAssetRefreshPatchForTarget(r, workspaceID, targetID, assets, edges)
		},
	})
}

func (s *Server) publishWorkspaceAssetRefreshPatch(r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) {
	for _, section := range workspaceAssetRefreshSections() {
		s.broker.Publish(workspaceAssetStreamID(workspaceID, asset.ID, section), s.workspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges, section))
	}
}

func (s *Server) publishWorkspaceAssetRefreshPatchForTarget(r *http.Request, workspaceID, targetID string, assets []workspace.AssetView, edges []workspace.AssetEdgeView) {
	for _, asset := range assets {
		if asset.Key == targetID && workspaceAssetRefreshable(asset) {
			s.publishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges)
		}
	}
}

func (s *Server) publishModelRefreshPatches(ctx context.Context, workspaceID, modelID string) {
	assets, edges, ok := s.workspaceAssetsAndEdgesForRefresh(ctx, workspaceID)
	if !ok {
		return
	}
	view := catalogWorkspaceView(s.metrics.Catalog())
	view.ID = workspaceID
	for _, asset := range assets {
		if asset.Type == string(workspace.AssetTypeSemanticModel) && asset.Key != modelID {
			continue
		}
		if asset.Type == string(workspace.AssetTypeModelTable) {
			assetModelID, _ := modelTableTargetParts(asset.Key)
			if assetModelID != modelID {
				continue
			}
		}
		if !workspaceAssetRefreshable(asset) {
			continue
		}
		refresh, err := s.assetRefreshStateForContext(ctx, workspaceID, asset)
		if err != nil {
			continue
		}
		for _, section := range workspaceAssetRefreshSections() {
			s.broker.Publish(workspaceAssetStreamID(workspaceID, asset.ID, section), ui.WorkspaceAssetRefreshSignals(view, asset, assets, edges, refresh, section))
		}
	}
}

func (s *Server) workspaceAssetsAndEdgesForRefresh(ctx context.Context, workspaceID string) ([]workspace.AssetView, []workspace.AssetEdgeView, bool) {
	catalog, ok, err := s.workspaceAssetCatalog(ctx, workspaceID)
	if err != nil || !ok {
		return nil, nil, false
	}
	assets := make([]workspace.AssetView, 0, len(catalog.Assets))
	for _, row := range catalog.Assets {
		assets = append(assets, workspace.AssetViewFromCatalogRecord(row))
	}
	edges := make([]workspace.AssetEdgeView, 0, len(catalog.Edges))
	for _, row := range catalog.Edges {
		edges = append(edges, workspace.AssetEdgeViewFromCatalogRecord(row))
	}
	return assets, edges, true
}

func (s *Server) workspaceAssetRefreshPatch(r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView, section string) map[string]any {
	refresh, err := s.assetRefreshState(r, workspaceID, asset)
	if err != nil {
		refresh = ui.AssetRefreshState{Latest: ui.AssetRefreshRun{Status: "failed"}}
	}
	return ui.WorkspaceAssetRefreshSignals(s.workspaceResponse(r, workspaceID), asset, assets, edges, refresh, section)
}

func workspaceAssetStreamID(workspaceID, assetID, section string) string {
	return "workspace-asset:" + workspaceID + ":" + assetID + ":" + section
}

func workspaceAssetRefreshSections() []string {
	return []string{"details", "refreshes", "lineage"}
}

func workspaceAssetUpdateSection(r *http.Request) string {
	switch strings.TrimSpace(r.URL.Query().Get("section")) {
	case "refreshes":
		return "refreshes"
	case "lineage":
		return "lineage"
	default:
		return "details"
	}
}

func (s *Server) assetRefreshState(r *http.Request, workspaceID string, asset workspace.AssetView) (ui.AssetRefreshState, error) {
	return s.assetRefreshStateForContext(r.Context(), workspaceID, asset)
}

func (s *Server) assetRefreshStateForContext(ctx context.Context, workspaceID string, asset workspace.AssetView) (ui.AssetRefreshState, error) {
	if s.store == nil || !workspaceAssetRefreshable(asset) {
		return ui.AssetRefreshState{}, nil
	}
	repo := materialize.NewSQLRunRepository(s.store.SQLDB())
	targetType := materialize.TargetSemanticModel
	if asset.Type == string(workspace.AssetTypeModelTable) {
		targetType = materialize.TargetModelTable
	}
	runs, err := repo.ListTargetRuns(ctx, workspaceID, targetType, asset.Key, materialize.RunPage{Limit: 50})
	if err != nil {
		return ui.AssetRefreshState{}, err
	}
	state := ui.AssetRefreshState{Runs: uiRefreshRuns(runs)}
	if len(state.Runs) > 0 {
		state.Latest = state.Runs[0]
	}
	if latest, ok, err := repo.LatestSuccessfulTargetRun(ctx, workspaceID, targetType, asset.Key); err != nil {
		return ui.AssetRefreshState{}, err
	} else if ok {
		state.LatestSuccessful = uiRefreshRun(latest)
	}
	return state, nil
}

func uiRefreshRuns(runs []materialize.RunRecord) []ui.AssetRefreshRun {
	out := make([]ui.AssetRefreshRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, uiRefreshRun(run))
	}
	return out
}

func uiRefreshRun(run materialize.RunRecord) ui.AssetRefreshRun {
	return ui.AssetRefreshRun{
		ID:                   run.ID,
		ModelID:              run.ModelID,
		DeploymentID:         run.DeploymentID,
		PrincipalID:          run.PrincipalID,
		PrincipalDisplayName: run.PrincipalDisplayName,
		TargetType:           run.TargetType,
		TargetID:             run.TargetID,
		TriggerType:          run.TriggerType,
		ParentRunID:          run.ParentRunID,
		Status:               run.Status,
		StartedAt:            run.StartedAt,
		FinishedAt:           run.FinishedAt,
		Error:                run.Error,
	}
}

func workspaceAssetRefreshable(asset workspace.AssetView) bool {
	return asset.Type == string(workspace.AssetTypeSemanticModel) || asset.Type == string(workspace.AssetTypeModelTable)
}

func modelTableTargetParts(key string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(key), ".", 2)
	if len(parts) != 2 {
		return "", strings.TrimSpace(key)
	}
	return parts[0], parts[1]
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
	if section == "refreshes" {
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

func connectionSourcePair(assets []workspace.AssetView, edges []workspace.AssetEdgeView, connectionID, sourceID string) (workspace.AssetView, workspace.AssetView, bool) {
	connection, ok := workspace.AssetByID(assets, connectionID)
	if !ok || connection.Type != "connection" {
		return workspace.AssetView{}, workspace.AssetView{}, false
	}
	source, ok := workspace.AssetByID(assets, sourceID)
	if !ok || source.Type != "source" || assetnav.SourceConnectionID(source.ID, edges) != connection.ID {
		return workspace.AssetView{}, workspace.AssetView{}, false
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
	if section == "refreshes" {
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
	var selected workspace.AssetView
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
	_ = writePagedJSON(w, r, apiWorkspaceDTOs(workspaces))
}

func (s *Server) apiWorkspaceAssets(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assets, _, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	_ = writePagedJSON(w, r, apiAssetSummaryDTOs(workspace.FilterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))))
}

func (s *Server) apiWorkspaceAsset(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assetID := firstNonEmpty(chi.URLParam(r, "assetId"), chi.URLParam(r, "asset"))
	assets, _, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	asset, ok := workspace.AssetByID(assets, assetID)
	if !ok {
		writeJSONError(w, fmt.Errorf("asset %q not found", assetID), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, apiAssetDTOs([]workspace.AssetView{asset})[0])
}

func (s *Server) apiWorkspaceAssetEdges(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	_, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	_ = writePagedJSON(w, r, apiAssetEdgeDTOs(edges))
}

func (s *Server) apiWorkspaceAssetLineage(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	assetID := firstNonEmpty(chi.URLParam(r, "assetId"), chi.URLParam(r, "asset"))
	assets, edges, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	if _, ok := workspace.AssetByID(assets, assetID); !ok {
		writeJSONError(w, fmt.Errorf("asset %q not found", assetID), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, api.AssetLineageResponse{
		AssetID:    assetID,
		Upstream:   assetLineageEndpointIDs(edges, assetID, true),
		Downstream: assetLineageEndpointIDs(edges, assetID, false),
	})
}

func (s *Server) apiWorkspaceRoles(w http.ResponseWriter, r *http.Request) {
	_, roles, err := s.roleBindingsAndRoles(r, s.workspaceID(chi.URLParam(r, "workspace")))
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	_ = writePagedJSON(w, r, apiRoleDTOs(roles))
}

func (s *Server) apiRoleBindings(w http.ResponseWriter, r *http.Request) {
	repo, err := s.accessRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if repo == nil {
		_ = writePagedJSON(w, r, []map[string]any{})
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
	_ = writePagedJSON(w, r, out)
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

func (s *Server) workspaceList(r *http.Request) ([]workspace.WorkspaceView, error) {
	repo, err := s.workspaceRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return []workspace.WorkspaceView{catalogWorkspaceView(s.metrics.Catalog())}, nil
	}
	rows, err := repo.List(r.Context())
	if err != nil {
		return nil, err
	}
	out := make([]workspace.WorkspaceView, 0, len(rows))
	for _, row := range rows {
		out = append(out, workspace.WorkspaceViewFromSummary(row))
	}
	return out, nil
}

func (s *Server) workspaceResponse(r *http.Request, workspaceID string) workspace.WorkspaceView {
	if repo, _ := s.workspaceRepository(); repo != nil {
		if row, err := repo.ByID(r.Context(), workspace.WorkspaceID(workspaceID)); err == nil {
			return workspace.WorkspaceViewFromSummary(row)
		}
	}
	view := catalogWorkspaceView(s.metrics.Catalog())
	view.ID = workspaceID
	return view
}

func (s *Server) workspaceAssetsAndEdges(r *http.Request, workspaceID string) ([]workspace.AssetView, []workspace.AssetEdgeView, error) {
	catalog, ok, err := s.workspaceAssetCatalog(r.Context(), workspaceID)
	if err != nil || !ok {
		return nil, nil, err
	}
	assets := make([]workspace.AssetView, 0, len(catalog.Assets))
	for _, row := range catalog.Assets {
		assets = append(assets, workspace.AssetViewFromCatalogRecord(row))
	}
	edges := make([]workspace.AssetEdgeView, 0, len(catalog.Edges))
	for _, row := range catalog.Edges {
		edges = append(edges, workspace.AssetEdgeViewFromCatalogRecord(row))
	}
	return assets, edges, nil
}

func (s *Server) workspaceAssetCatalog(ctx context.Context, workspaceID string) (workspace.AssetCatalog, bool, error) {
	reader, err := s.workspaceAssetCatalogReader()
	if err != nil || reader == nil {
		return workspace.AssetCatalog{}, false, err
	}
	return reader.ActiveAssetCatalog(ctx, workspace.WorkspaceID(workspaceID))
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
	if provider, ok := s.metrics.(workspace.RuntimeAssetGraphProvider); ok {
		service.WithRuntimeProvider(provider)
	}
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

func apiWorkspaceDTOs(rows []workspace.WorkspaceView) []api.WorkspaceResponse {
	out := make([]api.WorkspaceResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.WorkspaceResponse{
			ID:                 row.ID,
			Title:              row.Title,
			Description:        row.Description,
			ActiveDeploymentID: row.ActiveDeploymentID,
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		})
	}
	return out
}

func apiAssetDTOs(rows []workspace.AssetView) []api.AssetResponse {
	out := make([]api.AssetResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.AssetResponse{
			ID:            row.ID,
			SnapshotID:    row.SnapshotID,
			WorkspaceID:   row.WorkspaceID,
			DeploymentID:  row.DeploymentID,
			Type:          row.Type,
			Key:           row.Key,
			ParentID:      row.ParentID,
			Title:         row.Title,
			Description:   row.Description,
			PayloadSchema: row.PayloadSchema,
			Payload:       row.Payload,
			Href:          row.Href,
		})
	}
	return out
}

func apiAssetSummaryDTOs(rows []workspace.AssetView) []api.AssetSummaryResponse {
	out := make([]api.AssetSummaryResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.AssetSummaryResponse{
			ID:            row.ID,
			SnapshotID:    row.SnapshotID,
			WorkspaceID:   row.WorkspaceID,
			DeploymentID:  row.DeploymentID,
			Type:          row.Type,
			Key:           row.Key,
			ParentID:      row.ParentID,
			Title:         row.Title,
			Description:   row.Description,
			PayloadSchema: row.PayloadSchema,
			Href:          row.Href,
		})
	}
	return out
}

func apiAssetEdgeDTOs(rows []workspace.AssetEdgeView) []api.AssetEdgeResponse {
	out := make([]api.AssetEdgeResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, api.AssetEdgeResponse{
			ID:           row.ID,
			WorkspaceID:  row.WorkspaceID,
			DeploymentID: row.DeploymentID,
			FromAssetID:  row.FromAssetID,
			ToAssetID:    row.ToAssetID,
			Type:         row.Type,
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
		out = append(out, api.RoleResponse{Name: row.Name, Permissions: row.Permissions})
	}
	return out
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
		return "Local workspace"
	}
	principal, ok := s.auth.Principal(r)
	if !ok {
		return "Workspace access"
	}
	if principal.DevBypass {
		return "Platform admin"
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
