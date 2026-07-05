package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacedatastar "github.com/Yacobolo/libredash/internal/workspace/datastar"
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

func (s *Server) runWorkspaceAssetRefreshWithPatches(r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	switch asset.Type {
	case string(workspace.AssetTypeSemanticModel):
		return s.refreshSemanticModelAssetWithPatches(r.Context(), r, workspaceID, asset, assets, edges)
	case string(workspace.AssetTypeModelTable):
		return s.refreshModelTableAssetWithPatches(r.Context(), r, workspaceID, asset, assets, edges)
	default:
		return http.ErrMissingFile
	}
}

func (s *Server) queueWorkspaceAssetRefreshWithPatches(r *http.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	ctx := r.Context()
	runRepo, err := s.materializationRunRepository()
	if err != nil {
		return err
	}
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
		return s.runWorkspaceAssetRefreshWithPatches(r, workspaceID, asset, assets, edges)
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		if os.IsNotExist(err) {
			return s.runWorkspaceAssetRefreshWithPatches(r, workspaceID, asset, assets, edges)
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
	runtime, err := s.openRefreshRuntime(ctx, definition, active, refreshed, artifact, environment, plan)
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

func (s *Server) openRefreshRuntime(ctx context.Context, definition *workspace.Definition, active, refreshed deployment.Deployment, artifact deployment.Artifact, environment deployment.Environment, plan workspacerefresh.Plan) (*analyticsduckdb.WorkspaceRuntime, error) {
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
	repo, err := s.materializationRunRepository()
	if err != nil {
		return err
	}
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
	repo, err := s.materializationRunRepository()
	if err != nil {
		return err
	}
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
	for _, section := range workspacedatastar.WorkspaceAssetRefreshSections() {
		s.broker.Publish(workspacedatastar.WorkspaceAssetStreamID(workspaceID, asset.ID, section), s.workspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges, section))
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
		for _, section := range workspacedatastar.WorkspaceAssetRefreshSections() {
			s.broker.Publish(workspacedatastar.WorkspaceAssetStreamID(workspaceID, asset.ID, section), workspacedatastar.WorkspaceAssetRefreshSignals(view, asset, assets, edges, refresh, section))
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
		for _, section := range workspacedatastar.WorkspaceAssetRefreshSections() {
			s.broker.Publish(workspacedatastar.WorkspaceAssetStreamID(workspaceID, asset.ID, section), workspacedatastar.WorkspaceAssetRefreshSignals(view, asset, assets, edges, refresh, section))
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
	return workspacedatastar.WorkspaceAssetRefreshSignals(s.workspaceResponse(r, workspaceID), asset, assets, edges, refresh, section)
}

func (s *Server) assetRefreshState(r *http.Request, workspaceID string, asset workspace.AssetView) (ui.AssetRefreshState, error) {
	return s.assetRefreshStateForContext(r.Context(), workspaceID, asset)
}

func (s *Server) assetRefreshStateForContext(ctx context.Context, workspaceID string, asset workspace.AssetView) (ui.AssetRefreshState, error) {
	if s.store == nil || !workspaceAssetRefreshable(asset) {
		return ui.AssetRefreshState{}, nil
	}
	repo, err := s.materializationRunRepository()
	if err != nil {
		return ui.AssetRefreshState{}, err
	}
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
