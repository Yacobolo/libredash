package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/assetnav"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacerefresh "github.com/Yacobolo/libredash/internal/workspace/refresh"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/starfederation/datastar-go/datastar"
)

var errWorkspaceRBACNotConfigured = errors.New("Workspace RBAC store is not configured.")

type activeWorkspaceMetadataRepository interface {
	ListWithActiveMetadata(context.Context, string) ([]workspace.Summary, error)
	ByIDWithActiveMetadata(context.Context, workspace.WorkspaceID, string) (workspace.Summary, error)
}

func (s *Server) workspaces(w http.ResponseWriter, r *http.Request) {
	workspaces, err := s.workspaceList(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspacesPage(s.catalogForWorkspacesPage(r, workspaces), workspaces, s.currentRoleLabel(r), s.chatChromeOption(r)).Render(w); err != nil {
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
	if err := ui.WorkspacePage(s.catalogForWorkspace(workspaceID), workspace, filtered, r.URL.Query().Get("type"), r.URL.Query().Get("q"), s.currentRoleLabel(r), access, csrfToken(r, s.auth), s.chatChromeOption(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	assets, edges, err := s.platformConnectionAssetsAndEdges(r)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	activeType := workspace.NormalizeConnectionAssetType(r.URL.Query().Get("type"))
	filtered := workspace.FilterConnectionAssets(assets, activeType, r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ConnectionsPage(s.catalogForWorkspacesPage(r, nil), "platform", filtered, edges, activeType, r.URL.Query().Get("q"), s.currentRoleLabel(r), s.chatChromeOption(r)).Render(w); err != nil {
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
	if section == "data" {
		if selected.Type != "semantic_model" && selected.Type != "model_table" && selected.Type != "source" {
			http.NotFound(w, r)
			return
		}
		values := url.Values{}
		values.Set("workspace", workspaceID)
		values.Set("object", assetID)
		http.Redirect(w, r, "/data?"+values.Encode(), http.StatusFound)
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
	versions, err := s.assetVersionsStateForSection(r.Context(), workspaceID, string(s.requestDeploymentEnvironment(r)), selected, section)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.WorkspaceAssetPageWithRefreshAndVersions(s.catalogForWorkspace(workspaceID), workspace, selected, assets, edges, section, s.currentRoleLabel(r), refresh, versions, s.chatChromeOption(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) refreshWorkspaceAssetMaterializations(w http.ResponseWriter, r *http.Request) {
	s.refreshWorkspaceAsset(w, r)
}

func (s *Server) refreshWorkspaceAsset(w http.ResponseWriter, r *http.Request) {
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
	if err := s.refreshWorkspaceAssetDeploymentWithPatches(r, workspaceID, selected, assets, edges); err != nil {
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

func (s *Server) refreshWorkspaceAssetDeploymentWithPatches(r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	ctx := r.Context()
	runRepo := materialize.NewSQLRunRepository(s.store.SQLDB())
	service, err := s.workspaceRefreshService(runRepo)
	if err != nil {
		return err
	}
	environment := s.requestDeploymentEnvironment(r)
	activeState, err := service.Active(ctx, workspaceID, environment)
	if err != nil {
		return err
	}
	artifact := activeState.Artifact
	if strings.TrimSpace(artifact.Path) == "" {
		return s.refreshWorkspaceAssetWithPatches(r, workspaceID, asset, assets, edges)
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		if os.IsNotExist(err) {
			return s.refreshWorkspaceAssetWithPatches(r, workspaceID, asset, assets, edges)
		}
		return err
	}
	principal, _ := currentPrincipal(s, r)
	if _, err := service.QueueAssetRefresh(ctx, workspacerefresh.QueueAssetInput{
		WorkspaceID: workspaceID,
		Environment: environment,
		PrincipalID: principal.ID,
		Asset:       asset,
		DataRoot:    s.dataDirForWorkspace(workspaceID, artifact),
	}); err != nil {
		return err
	}
	s.dispatchQueuedMaterializationJobs(context.Background())
	return nil
}

func (s *Server) executeWorkspaceAssetRefreshPlan(ctx context.Context, definition *workspace.Definition, active, refreshed deployment.Deployment, artifact deployment.Artifact, environment deployment.Environment, plan workspacerefresh.Plan) (int64, error) {
	runtime, err := s.openWorkspaceRefreshRuntime(ctx, definition, active, refreshed, artifact, environment, plan)
	if err != nil {
		return 0, err
	}
	defer runtime.Close()
	if err := runtime.RefreshWorkspaceTables(ctx, plan.Tables); err != nil {
		return 0, err
	}
	snapshotID := runtime.DuckLakeSnapshotID()
	if snapshotID <= 0 {
		return 0, fmt.Errorf("refresh did not produce a DuckLake snapshot")
	}
	return snapshotID, nil
}

func (s *Server) openWorkspaceRefreshRuntime(ctx context.Context, definition *workspace.Definition, active, refreshed deployment.Deployment, artifact deployment.Artifact, environment deployment.Environment, plan workspacerefresh.Plan) (*analyticsduckdb.WorkspaceRuntime, error) {
	dataDir := s.dataDirForWorkspace(string(refreshed.WorkspaceID), artifact)
	dbDir := s.duckDBDir
	if strings.TrimSpace(dbDir) == "" {
		dbDir = filepath.Join(".libredash", "duckdb")
	}
	dbDir = filepath.Join(dbDir, string(deployment.NormalizeEnvironment(environment)))
	return analyticsduckdb.OpenWorkspaceMaterializeRuntime(ctx, analyticsduckdb.WorkspaceRuntimeConfig{
		Models:             definition.Models,
		DataDir:            dataDir,
		DBDir:              dbDir,
		CatalogPath:        s.duckLakeCatalogPath,
		DuckLakeDataPath:   s.duckLakeDataPath,
		DeploymentID:       string(refreshed.ID),
		WorkspaceID:        string(refreshed.WorkspaceID),
		Environment:        string(deployment.NormalizeEnvironment(environment)),
		TargetType:         plan.TargetType,
		TargetID:           plan.TargetID,
		SemanticDigest:     refreshed.Digest,
		ArtifactDigest:     artifact.Digest,
		SkipInitialRefresh: true,
	})
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

func (s *Server) refreshSemanticModelAssetWithPatches(ctx context.Context, r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	repo := materialize.NewSQLRunRepository(s.store.SQLDB())
	principal, _ := currentPrincipal(s, r)
	orchestrator := NewRefreshOrchestrator(repo, s.metrics)
	return orchestrator.RefreshSemanticModel(ctx, refreshRunInput{
		WorkspaceID: workspaceID,
		ModelID:     semanticModelTargetID(asset),
		PrincipalID: principal.ID,
		TargetID:    asset.Key,
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
	view := catalogWorkspaceView(s.catalogForWorkspace(workspaceID))
	view.ID = workspaceID
	for _, asset := range assets {
		if asset.Type == string(workspace.AssetTypeSemanticModel) && semanticModelTargetID(asset) != modelID {
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

func (s *Server) publishWorkspaceAssetRefreshPatchesForTarget(ctx context.Context, workspaceID, targetType, targetID string) {
	assets, edges, ok := s.workspaceAssetsAndEdgesForRefresh(ctx, workspaceID)
	if !ok {
		return
	}
	view := catalogWorkspaceView(s.catalogForWorkspace(workspaceID))
	view.ID = workspaceID
	for _, asset := range assets {
		if !workspaceAssetRefreshable(asset) {
			continue
		}
		if assetRefreshTargetID(asset) != targetID {
			continue
		}
		if targetType == materialize.TargetModelTable && asset.Type != string(workspace.AssetTypeModelTable) {
			continue
		}
		if targetType == materialize.TargetSemanticModel && asset.Type != string(workspace.AssetTypeSemanticModel) {
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
	catalog, ok, err := s.workspaceAssetCatalog(ctx, workspaceID, string(s.defaultDeploymentEnvironment()))
	if err != nil || !ok {
		return nil, nil, false
	}
	return assetCatalogViews(catalog), assetCatalogEdgeViews(catalog), true
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
	targetID := assetRefreshTargetID(asset)
	runs, err := repo.ListTargetRuns(ctx, workspaceID, targetType, targetID, materialize.RunPage{Limit: 50})
	if err != nil {
		return ui.AssetRefreshState{}, err
	}
	state := ui.AssetRefreshState{Runs: uiRefreshRuns(runs)}
	if len(state.Runs) > 0 {
		state.Latest = state.Runs[0]
	}
	if latest, ok, err := repo.LatestSuccessfulTargetRun(ctx, workspaceID, targetType, targetID); err != nil {
		return ui.AssetRefreshState{}, err
	} else if ok {
		state.LatestSuccessful = uiRefreshRun(latest)
	}
	return state, nil
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

func assetRefreshTargetID(asset workspace.AssetView) string {
	return asset.Key
}

func semanticModelTargetID(asset workspace.AssetView) string {
	if name, ok := asset.Payload["Name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if name, ok := asset.Payload["name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return asset.Key
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
	assets, edges, err := s.platformConnectionAssetsAndEdges(r)
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
	assets, edges, err := s.platformConnectionAssetsAndEdges(r)
	if err != nil {
		http.Error(w, err.Error(), statusForNotFound(err))
		return
	}
	connection, source, ok := connectionSourcePair(assets, edges, chi.URLParam(r, "connection"), chi.URLParam(r, "source"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if section == "data" {
		values := url.Values{}
		values.Set("workspace", source.WorkspaceID)
		values.Set("object", source.ID)
		http.Redirect(w, r, "/data?"+values.Encode(), http.StatusFound)
		return
	}
	workspace := platformAssetWorkspaceView()
	versions, err := s.assetVersionsStateForSection(r.Context(), source.WorkspaceID, string(s.requestDeploymentEnvironment(r)), source, section)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ConnectionSourceAssetPageWithVersions(s.catalogForWorkspacesPage(r, nil), workspace, connection, source, assets, edges, section, s.currentRoleLabel(r), versions).Render(w); err != nil {
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
	if section == "data" {
		http.NotFound(w, r)
		return
	}
	assets, edges, err := s.platformConnectionAssetsAndEdges(r)
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
	workspace := platformAssetWorkspaceView()
	versions, err := s.assetVersionsStateForSection(r.Context(), selected.WorkspaceID, string(s.requestDeploymentEnvironment(r)), selected, section)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ConnectionAssetPageWithVersions(s.catalogForWorkspacesPage(r, nil), workspace, selected, assets, edges, section, s.currentRoleLabel(r), versions).Render(w); err != nil {
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
	if err := ui.WorkspacePermissionsPage(s.catalogForWorkspace(workspaceID), s.workspaceResponse(r, workspaceID), bindings, roles, csrfToken(r, s.auth), s.currentRoleLabel(r)).Render(w); err != nil {
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
	filtered := workspace.FilterWorkspaceAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))
	if r.URL.Query().Get("include") == "all" {
		filtered = workspace.FilterAssets(assets, r.URL.Query().Get("type"), r.URL.Query().Get("q"))
	}
	_ = writePagedJSON(w, r, apiAssetSummaryDTOs(filtered))
}

func (s *Server) apiWorkspaceActiveDeploymentGraph(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	repo, err := s.workspaceRepository()
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	graph := workspace.AssetGraph{}
	if repo != nil {
		var ok bool
		graph, ok, err = repo.ActiveDeploymentGraph(r.Context(), workspace.WorkspaceID(workspaceID), string(s.requestDeploymentEnvironment(r)))
		if err != nil {
			writeJSONError(w, err, http.StatusInternalServerError)
			return
		}
		if !ok {
			graph = workspace.AssetGraph{}
		}
	}
	response, err := apiWorkspaceAssetGraphDTO(graph)
	if err != nil {
		writeJSONError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, response)
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
			SourceFile:    row.SourceFile,
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
			SourceFile:    row.SourceFile,
			PayloadSchema: row.PayloadSchema,
			ContentHash:   row.ContentHash,
			Href:          row.Href,
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
			ID:            string(row.ID),
			SnapshotID:    string(row.SnapshotID),
			WorkspaceID:   string(row.WorkspaceID),
			DeploymentID:  string(row.DeploymentID),
			Type:          string(row.Type),
			Key:           row.Key,
			ParentID:      string(row.ParentID),
			Title:         row.Title,
			Description:   row.Description,
			SourceFile:    row.SourceFile,
			PayloadSchema: row.PayloadSchema,
			Payload:       payload,
			ContentHash:   row.ContentHash,
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
			ID:           string(row.ID),
			WorkspaceID:  string(row.WorkspaceID),
			DeploymentID: string(row.DeploymentID),
			FromAssetID:  string(row.FromAssetID),
			ToAssetID:    string(row.ToAssetID),
			Type:         string(row.Type),
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
