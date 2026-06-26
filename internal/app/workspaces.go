package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	if section == "refreshes" && selected.Type != "semantic_model" {
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
	if !ok || selected.Type != "semantic_model" {
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
	if !ok || selected.Type != "semantic_model" {
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
	repo := materialize.NewSQLRunRepository(s.store.SQLDB())
	principal, _ := currentPrincipal(s, r)
	run, err := repo.CreateRun(r.Context(), materialize.RunInput{WorkspaceID: workspaceID, ModelID: asset.Key, PrincipalID: principal.ID})
	if err != nil {
		return err
	}
	s.publishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges)

	if _, err := repo.MarkRunRunning(r.Context(), workspaceID, run.ID); err != nil {
		return err
	}
	s.publishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges)

	if err := s.metrics.RefreshMaterializations(r.Context(), asset.Key); err != nil {
		if _, finishErr := repo.MarkRunFailed(r.Context(), workspaceID, run.ID, err.Error()); finishErr != nil {
			return finishErr
		}
		s.publishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges)
		return err
	}
	if _, err := repo.MarkRunSucceeded(r.Context(), workspaceID, run.ID); err != nil {
		return err
	}
	s.publishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges)
	return nil
}

func (s *Server) publishWorkspaceAssetRefreshPatch(r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) {
	for _, section := range workspaceAssetRefreshSections() {
		s.broker.Publish(workspaceAssetStreamID(workspaceID, asset.ID, section), s.workspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges, section))
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
		if asset.Type != "semantic_model" || asset.Key != modelID {
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
	if repo, err := s.workspaceRepository(); err == nil && repo != nil {
		graph, ok, err := repo.ActiveDeploymentGraph(ctx, workspace.WorkspaceID(workspaceID))
		if err == nil && ok {
			assets := make([]workspace.AssetView, 0, len(graph.Assets))
			for _, row := range graph.Assets {
				assets = append(assets, workspace.AssetViewFromAsset(row))
			}
			edges := make([]workspace.AssetEdgeView, 0, len(graph.Edges))
			for _, row := range graph.Edges {
				edges = append(edges, workspace.AssetEdgeViewFromAssetEdge(row))
			}
			return assets, edges, true
		}
	}
	return s.workspaceAssetsFromRuntime(workspaceID)
}

func (s *Server) workspaceAssetRefreshPatch(r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView, section string) map[string]any {
	refresh, err := s.assetRefreshState(r, workspaceID, asset)
	if err != nil {
		return map[string]any{"assetRefresh": map[string]any{"status": "failed", "running": false, "lastSuccessful": ""}}
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
	if s.store == nil || asset.Type != "semantic_model" {
		return ui.AssetRefreshState{}, nil
	}
	repo := materialize.NewSQLRunRepository(s.store.SQLDB())
	runs, err := repo.ListModelRuns(ctx, workspaceID, asset.Key, materialize.RunPage{Limit: 50})
	if err != nil {
		return ui.AssetRefreshState{}, err
	}
	state := ui.AssetRefreshState{Runs: uiRefreshRuns(runs)}
	if len(state.Runs) > 0 {
		state.Latest = state.Runs[0]
	}
	if latest, ok, err := repo.LatestSuccessfulModelRun(ctx, workspaceID, asset.Key); err != nil {
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
		Status:               run.Status,
		StartedAt:            run.StartedAt,
		FinishedAt:           run.FinishedAt,
		Error:                run.Error,
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
	_ = writePagedJSON(w, r, apiAssetDTOs(workspace.FilterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))))
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
	assets := make([]workspace.AssetView, 0, len(graph.Assets))
	for _, row := range graph.Assets {
		assets = append(assets, workspace.AssetViewFromAsset(row))
	}
	edges := make([]workspace.AssetEdgeView, 0, len(graph.Edges))
	for _, row := range graph.Edges {
		edges = append(edges, workspace.AssetEdgeViewFromAssetEdge(row))
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

func (s *Server) workspaceAssetsFromRuntime(workspaceID string) ([]workspace.AssetView, []workspace.AssetEdgeView, bool) {
	graph, ok := s.workspaceGraphFromRuntime(workspaceID, "local")
	if !ok {
		return nil, nil, false
	}
	assets := make([]workspace.AssetView, 0, len(graph.Assets))
	for _, row := range graph.Assets {
		assets = append(assets, workspace.AssetViewFromAsset(row))
	}
	edges := make([]workspace.AssetEdgeView, 0, len(graph.Edges))
	for _, row := range graph.Edges {
		edges = append(edges, workspace.AssetEdgeViewFromAssetEdge(row))
	}
	return assets, edges, true
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

func fallbackAssets(catalog dashboard.Catalog, workspaceID string) []workspace.AssetView {
	inputs := []workspace.FallbackAssetInput{}
	for _, report := range catalog.Dashboards {
		inputs = append(inputs, workspace.FallbackAssetInput{ID: "dashboard:" + report.ID, Type: "dashboard", Key: report.ID, Title: report.Title, Description: report.Description})
	}
	for _, model := range catalog.Models {
		inputs = append(inputs, workspace.FallbackAssetInput{ID: "semantic_model:" + model.ID, Type: "semantic_model", Key: model.ID, Title: model.Title, Description: model.Description})
	}
	return workspace.FallbackAssetViews(workspaceID, inputs)
}

func apiWorkspaceDTOs(views []workspace.WorkspaceView) []api.WorkspaceResponse {
	out := make([]api.WorkspaceResponse, 0, len(views))
	for _, view := range views {
		out = append(out, apiWorkspaceDTO(view))
	}
	return out
}

func apiWorkspaceDTO(view workspace.WorkspaceView) api.WorkspaceResponse {
	return api.WorkspaceResponse{
		ID:                 view.ID,
		Title:              view.Title,
		Description:        view.Description,
		ActiveDeploymentID: view.ActiveDeploymentID,
		CreatedAt:          view.CreatedAt,
		UpdatedAt:          view.UpdatedAt,
	}
}

func apiAssetDTOs(views []workspace.AssetView) []api.AssetResponse {
	out := make([]api.AssetResponse, 0, len(views))
	for _, view := range views {
		out = append(out, apiAssetDTO(view))
	}
	return out
}

func apiAssetDTO(view workspace.AssetView) api.AssetResponse {
	return api.AssetResponse{
		ID:           view.ID,
		WorkspaceID:  view.WorkspaceID,
		DeploymentID: view.DeploymentID,
		Type:         view.Type,
		Key:          view.Key,
		ParentID:     view.ParentID,
		Title:        view.Title,
		Description:  view.Description,
		Meta:         view.Meta,
		Href:         view.Href,
	}
}

func apiAssetEdgeDTOs(views []workspace.AssetEdgeView) []api.AssetEdgeResponse {
	out := make([]api.AssetEdgeResponse, 0, len(views))
	for _, view := range views {
		out = append(out, apiAssetEdgeDTO(view))
	}
	return out
}

func apiAssetEdgeDTO(view workspace.AssetEdgeView) api.AssetEdgeResponse {
	return api.AssetEdgeResponse{
		ID:           view.ID,
		WorkspaceID:  view.WorkspaceID,
		DeploymentID: view.DeploymentID,
		FromAssetID:  view.FromAssetID,
		ToAssetID:    view.ToAssetID,
		Type:         view.Type,
	}
}

func apiRoleDTOs(views []workspace.RoleView) []api.RoleResponse {
	out := make([]api.RoleResponse, 0, len(views))
	for _, view := range views {
		out = append(out, api.RoleResponse{Name: view.Name, Permissions: view.Permissions})
	}
	return out
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
