package http

import (
	"context"
	"errors"
	nethttp "net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/analytics/materialize"
	"github.com/Yacobolo/leapview/internal/refreshpipeline"
	"github.com/Yacobolo/leapview/internal/servingstate"
	"github.com/Yacobolo/leapview/internal/ui"
	"github.com/Yacobolo/leapview/internal/workspace"
	workspacedatastar "github.com/Yacobolo/leapview/internal/workspace/datastar"
	"github.com/Yacobolo/leapview/internal/workspace/refresh"
	"github.com/Yacobolo/leapview/pkg/pagestream"
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
	Broker         interface {
		Publish(string, pagestream.SignalPatch)
	}
	AssetCatalog         func(context.Context, string) ([]workspace.AssetView, []workspace.AssetEdgeView, bool)
	WorkspaceView        func(*nethttp.Request, string) workspace.WorkspaceView
	WorkspaceViewContext func(context.Context, string) workspace.WorkspaceView
	WorkspaceVersions    func(context.Context, string, string, workspace.AssetView, string) (ui.AssetVersionsState, error)
	DataVersions         refreshpipeline.Repository
}

func (s Support) RefreshAsset(_ context.Context, input AssetRefreshInput) error {
	return s.queueAssetRefreshWithPatches(input.Request, input.WorkspaceID, input.Asset, input.Assets, input.Edges)
}

func (s Support) AssetRefreshState(ctx context.Context, workspaceID, environment string, asset workspace.AssetView) (ui.AssetRefreshState, error) {
	if !workspaceAssetRefreshable(asset) {
		return ui.AssetRefreshState{}, nil
	}
	repo, err := s.runRepository()
	if err != nil {
		return ui.AssetRefreshState{}, err
	}
	targetType := materialize.TargetRefreshPipeline
	targetID := assetRefreshTargetID(asset)
	environment = string(servingstate.NormalizeEnvironment(servingstate.Environment(environment)))
	runs, err := repo.ListTargetRuns(ctx, workspaceID, targetType, targetID, materialize.RunPage{Limit: 50, Environment: environment})
	if err != nil {
		return ui.AssetRefreshState{}, err
	}
	state := ui.AssetRefreshState{Runs: uiRefreshRuns(runs)}
	pipelineID := strings.TrimPrefix(asset.Key, workspaceID+".")
	if s.DataVersions != nil {
		nextRun, ok, err := s.DataVersions.NextRun(ctx, workspaceID, environment, pipelineID)
		if err != nil {
			return ui.AssetRefreshState{}, err
		}
		if ok {
			state.NextRun = nextRun
		}
	}
	if len(state.Runs) > 0 {
		state.Latest = state.Runs[0]
	}
	if latest, ok, err := repo.LatestSuccessfulTargetRun(ctx, workspaceID, environment, targetType, targetID); err != nil {
		return ui.AssetRefreshState{}, err
	} else if ok {
		state.LatestSuccessful = uiRefreshRun(latest)
	}
	if s.DataVersions != nil {
		modelID := refreshPipelineModelID(asset)
		if version, ok, err := s.DataVersions.DataVersion(ctx, workspaceID, environment, modelID); err != nil {
			return ui.AssetRefreshState{}, err
		} else if ok {
			state.DataVersion = ui.AssetDataVersion{
				SnapshotID: version.SnapshotID, ServingStateID: version.ServingStateID,
				RefreshedAt: version.RefreshedAt, Source: version.Source,
			}
		}
	}
	return state, nil
}

func refreshPipelineModelID(asset workspace.AssetView) string {
	for _, key := range []string{"semanticModel", "SemanticModel"} {
		if value, ok := asset.Payload[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	pipelineID := strings.TrimPrefix(asset.Key, workspaceID+".")
	if _, err := service.QueuePipelineRefresh(ctx, refresh.QueuePipelineInput{
		WorkspaceID: workspaceID,
		Environment: environment,
		PrincipalID: s.principalID(r),
		PipelineID:  pipelineID,
		TriggerType: materialize.TriggerManual,
	}); err != nil {
		return err
	}
	if s.DispatchQueued != nil {
		s.DispatchQueued()
	}
	return nil
}

func (s Support) PublishWorkspaceAssetRefreshPatch(r *nethttp.Request, workspaceID string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView) {
	for _, section := range workspacedatastar.WorkspaceAssetRefreshSections() {
		s.publish(workspacedatastar.WorkspaceAssetStreamID(workspaceID, asset.ID, section), s.workspaceAssetRefreshPatch(r, workspaceID, asset, assets, edges, section))
	}
}

func (s Support) PublishWorkspaceAssetRefreshPatchesForTarget(ctx context.Context, workspaceID, environment, targetType, targetID string) {
	if targetType != materialize.TargetRefreshPipeline {
		return
	}
	assets, edges, ok := s.workspaceAssetsAndEdges(ctx, workspaceID)
	if !ok {
		return
	}
	for _, asset := range assets {
		if assetRefreshTargetID(asset) != targetID {
			continue
		}
		if asset.Type != string(workspace.AssetTypeRefreshPipeline) {
			continue
		}
		s.publishAssetRefreshSignals(ctx, nil, workspaceID, environment, asset, assets, edges, "")
	}
}

func (s Support) publishAssetRefreshSignals(ctx context.Context, r *nethttp.Request, workspaceID, environment string, asset workspace.AssetView, assets []workspace.AssetView, edges []workspace.AssetEdgeView, onlySection string) {
	if !workspaceAssetRefreshable(asset) {
		return
	}
	refresh, err := s.AssetRefreshState(ctx, workspaceID, environment, asset)
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
	environment := string(servingstate.DefaultEnvironment)
	if s.Environment != nil {
		environment = string(s.Environment(r))
	}
	refresh, err := s.AssetRefreshState(r.Context(), workspaceID, environment, asset)
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
		PrincipalDisplayName: run.PrincipalDisplayName,
		TriggerType:          run.TriggerType,
		Status:               run.Status,
		StartedAt:            run.StartedAt,
		FinishedAt:           run.FinishedAt,
		Error:                run.Error,
	}
}

func assetRefreshTargetID(asset workspace.AssetView) string {
	return asset.Key
}
