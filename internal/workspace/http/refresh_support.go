package http

import (
	"context"
	"errors"
	nethttp "net/http"
	"os"
	"strings"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacedatastar "github.com/Yacobolo/libredash/internal/workspace/datastar"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
	"github.com/Yacobolo/libredash/pkg/pagestream"
)

type RunRepository interface {
	materialize.RunRepository
	refresh.RunRepository
}

type ServiceFactory func(RunRepository) (refresh.Service, error)

type Support struct {
	Runs           func() (RunRepository, error)
	Service        ServiceFactory
	Environment    func(*nethttp.Request) servingstate.Environment
	PrincipalID    func(*nethttp.Request) string
	DispatchQueued func()
	DirectRunner   materialize.RefreshRunner
	ModelLookup    materialize.ModelLookup
	Broker         interface {
		Publish(string, pagestream.SignalPatch)
	}
	AssetCatalog         func(context.Context, string) ([]workspace.AssetView, []workspace.AssetEdgeView, bool)
	WorkspaceView        func(*nethttp.Request, string) workspace.WorkspaceView
	WorkspaceViewContext func(context.Context, string) workspace.WorkspaceView
	WorkspaceVersions    func(context.Context, string, string, workspace.AssetView, string) (ui.AssetVersionsState, error)
}

func (s Support) RefreshAsset(_ context.Context, input AssetRefreshInput) error {
	return s.queueAssetRefreshWithPatches(input.Request, input.WorkspaceID, input.Asset, input.Assets, input.Edges)
}

func (s Support) AssetRefreshState(ctx context.Context, workspaceID string, asset workspace.AssetView) (ui.AssetRefreshState, error) {
	if !workspaceAssetRefreshable(asset) {
		return ui.AssetRefreshState{}, nil
	}
	repo, err := s.runRepository()
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

func (s Support) AssetVersionsState(ctx context.Context, workspaceID, environment string, asset workspace.AssetView, section string) (ui.AssetVersionsState, error) {
	if s.WorkspaceVersions == nil {
		return ui.AssetVersionsState{CurrentContentHash: asset.ContentHash}, nil
	}
	return s.WorkspaceVersions(ctx, workspaceID, environment, asset, section)
}

func (s Support) queueAssetRefreshWithPatches(r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	ctx := r.Context()
	runRepo, err := s.runRepository()
	if err != nil {
		return err
	}
	if s.Service == nil {
		return errors.New("workspace refresh service is required")
	}
	service, err := s.Service(runRepo)
	if err != nil {
		return err
	}
	environment := servingstate.DefaultEnvironment
	if s.Environment != nil {
		environment = s.Environment(r)
	}
	activeState, err := service.Active(ctx, workspaceID, environment)
	if err != nil {
		return err
	}
	artifact := activeState.Artifact
	if strings.TrimSpace(artifact.Path) == "" {
		return s.runAssetRefreshWithPatches(r, workspaceID, asset, assets, edges)
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		if os.IsNotExist(err) {
			return s.runAssetRefreshWithPatches(r, workspaceID, asset, assets, edges)
		}
		return err
	}
	if _, err := service.QueueAssetRefresh(ctx, refresh.QueueAssetInput{
		WorkspaceID: workspaceID,
		Environment: environment,
		PrincipalID: s.principalID(r),
		Asset:       asset,
	}); err != nil {
		return err
	}
	if s.DispatchQueued != nil {
		s.DispatchQueued()
	}
	return nil
}

func (s Support) runAssetRefreshWithPatches(r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	switch asset.Type {
	case string(workspace.AssetTypeSemanticModel):
		return s.refreshSemanticModelAssetWithPatches(r.Context(), r, workspaceID, asset, assets, edges)
	case string(workspace.AssetTypeModelTable):
		return s.refreshModelTableAssetWithPatches(r.Context(), r, workspaceID, asset, assets, edges)
	default:
		return nethttp.ErrMissingFile
	}
}

func (s Support) refreshSemanticModelAssetWithPatches(ctx context.Context, r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	repo, err := s.runRepository()
	if err != nil {
		return err
	}
	orchestrator := materialize.NewRefreshOrchestrator(repo, s.DirectRunner, s.ModelLookup)
	return orchestrator.RefreshSemanticModel(ctx, materialize.RefreshRunInput{
		WorkspaceID: workspaceID,
		ModelID:     semanticModelTargetID(asset),
		PrincipalID: s.principalID(r),
		TargetID:    asset.Key,
	}, materialize.RefreshPublisher{
		Root: func() { s.PublishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges) },
		Target: func(targetID string) {
			s.PublishWorkspaceAssetRefreshPatchForTarget(r, workspaceID, targetID, assets, edges)
		},
	})
}

func (s Support) refreshModelTableAssetWithPatches(ctx context.Context, r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) error {
	repo, err := s.runRepository()
	if err != nil {
		return err
	}
	modelID, tableName := modelTableTargetParts(asset.Key)
	if modelID == "" || tableName == "" {
		return errors.New("model table asset key is invalid")
	}
	orchestrator := materialize.NewRefreshOrchestrator(repo, s.DirectRunner, s.ModelLookup)
	return orchestrator.RefreshModelTable(ctx, materialize.RefreshRunInput{
		WorkspaceID: workspaceID,
		ModelID:     modelID,
		PrincipalID: s.principalID(r),
		TargetID:    asset.Key,
	}, tableName, materialize.RefreshPublisher{
		Root: func() { s.PublishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges) },
		Target: func(targetID string) {
			s.PublishWorkspaceAssetRefreshPatchForTarget(r, workspaceID, targetID, assets, edges)
		},
	})
}

func (s Support) PublishWorkspaceAssetRefreshPatch(r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) {
	for _, section := range workspacedatastar.WorkspaceAssetRefreshSections() {
		s.publish(workspacedatastar.WorkspaceAssetStreamID(workspaceID, asset.ID, section), s.workspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges, section))
	}
}

func (s Support) PublishWorkspaceAssetRefreshPatchForTarget(r *nethttp.Request, workspaceID, targetID string, assets []workspace.AssetView, edges []workspace.AssetEdgeView) {
	for _, asset := range assets {
		if asset.Key == targetID && workspaceAssetRefreshable(asset) {
			s.PublishWorkspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges)
		}
	}
}

func (s Support) PublishModelRefreshPatches(ctx context.Context, workspaceID, modelID string) {
	assets, edges, ok := s.workspaceAssetsAndEdges(ctx, workspaceID)
	if !ok {
		return
	}
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
		s.publishAssetRefreshSignals(ctx, nil, workspaceID, asset, assets, edges, "")
	}
}

func (s Support) PublishWorkspaceAssetRefreshPatchesForTarget(ctx context.Context, workspaceID, targetType, targetID string) {
	assets, edges, ok := s.workspaceAssetsAndEdges(ctx, workspaceID)
	if !ok {
		return
	}
	for _, asset := range assets {
		if assetRefreshTargetID(asset) != targetID {
			continue
		}
		if targetType == materialize.TargetModelTable && asset.Type != string(workspace.AssetTypeModelTable) {
			continue
		}
		if targetType == materialize.TargetSemanticModel && asset.Type != string(workspace.AssetTypeSemanticModel) {
			continue
		}
		s.publishAssetRefreshSignals(ctx, nil, workspaceID, asset, assets, edges, "")
	}
}

func (s Support) publishAssetRefreshSignals(ctx context.Context, r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView, onlySection string) {
	if !workspaceAssetRefreshable(asset) {
		return
	}
	refresh, err := s.AssetRefreshState(ctx, workspaceID, asset)
	if err != nil {
		return
	}
	sections := workspacedatastar.WorkspaceAssetRefreshSections()
	if onlySection != "" {
		sections = []string{onlySection}
	}
	view := workspace.WorkspaceView{ID: workspaceID}
	if s.WorkspaceView != nil && r != nil {
		view = s.WorkspaceView(r, workspaceID)
	} else if s.WorkspaceViewContext != nil {
		view = s.WorkspaceViewContext(ctx, workspaceID)
	}
	for _, section := range sections {
		s.publish(workspacedatastar.WorkspaceAssetStreamID(workspaceID, asset.ID, section), pagestream.SignalPatch(workspacedatastar.WorkspaceAssetRefreshSignals(view, asset, assets, edges, refresh, section)))
	}
}

func (s Support) workspaceAssetRefreshPatch(r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView, section string) pagestream.SignalPatch {
	refresh, err := s.AssetRefreshState(r.Context(), workspaceID, asset)
	if err != nil {
		refresh = ui.AssetRefreshState{Latest: ui.AssetRefreshRun{Status: "failed"}}
	}
	view := workspace.WorkspaceView{ID: workspaceID}
	if s.WorkspaceView != nil {
		view = s.WorkspaceView(r, workspaceID)
	}
	return pagestream.SignalPatch(workspacedatastar.WorkspaceAssetRefreshSignals(view, asset, assets, edges, refresh, section))
}

func (s Support) workspaceAssetsAndEdges(ctx context.Context, workspaceID string) ([]workspace.AssetView, []workspace.AssetEdgeView, bool) {
	if s.AssetCatalog == nil {
		return nil, nil, false
	}
	return s.AssetCatalog(ctx, workspaceID)
}

func (s Support) runRepository() (RunRepository, error) {
	if s.Runs == nil {
		return nil, errors.New("materialization run repository is required")
	}
	return s.Runs()
}

func (s Support) principalID(r *nethttp.Request) string {
	if s.PrincipalID == nil {
		return ""
	}
	return s.PrincipalID(r)
}

func (s Support) publish(streamID string, patch pagestream.SignalPatch) {
	if s.Broker == nil {
		return
	}
	s.Broker.Publish(streamID, patch)
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
		ServingStateID:       run.ServingStateID,
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
