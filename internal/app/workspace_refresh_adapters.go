package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	refreshmodule "github.com/Yacobolo/leapview/internal/refresh/module"
	servingstatemodule "github.com/Yacobolo/leapview/internal/servingstate/module"
	workspacemodule "github.com/Yacobolo/leapview/internal/workspace/module"
)

func workspaceRefreshSupport(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy) refreshmodule.WorkspaceSupport {
	support := refreshmodule.WorkspaceSupport{
		Runs: func() (refreshmodule.RunReader, error) {
			if routes.refreshModule == nil {
				return nil, fmt.Errorf("refresh module is required")
			}
			return routes.refreshModule, nil
		},
		QueuePipeline: func(ctx context.Context, input refreshmodule.QueuePipelineInput) (refreshmodule.QueueAssetResult, error) {
			if routes.refreshModule == nil {
				return refreshmodule.QueueAssetResult{}, fmt.Errorf("refresh module is required")
			}
			return routes.refreshModule.QueuePipelineRefresh(ctx, input)
		},
		Environment: func(r *http.Request) servingstatemodule.Environment {
			return requestServingEnvironment(routes, runtime, platform, policy, r)
		},
		PrincipalID: func(r *http.Request) string {
			principal, _ := routes.accessModule.CurrentPrincipal(r)
			return principal.ID
		},
		DispatchQueued: func() {
			if routes.refreshModule != nil {
				routes.refreshModule.Dispatch(context.Background())
			}
		},
		Broker: runtime.broker,
		AssetCatalog: func(ctx context.Context, workspaceID string) ([]workspacemodule.AssetView, []workspacemodule.AssetEdgeView, bool) {
			assets, edges, err := routes.workspaceModule.WorkspaceAssetsAndEdgesForData(ctx, workspaceID, string(defaultServingEnvironment(routes, runtime, platform, policy)))
			if err != nil || (len(assets) == 0 && len(edges) == 0) {
				return nil, nil, false
			}
			return assets, edges, true
		},
		WorkspaceView: func(r *http.Request, workspaceID string) workspacemodule.WorkspaceView {
			return routes.workspaceModule.WorkspaceResponse(r, workspaceID)
		},
		WorkspaceViewContext: func(ctx context.Context, workspaceID string) workspacemodule.WorkspaceView {
			return routes.workspaceModule.WorkspaceViewContext(ctx, workspaceID)
		},
		Presentation: workspacemodule.RefreshPresentation{},
	}
	if runtime.persistenceConfigured {
		support.DataVersions = routes.refreshModule
	}
	return support
}

func workspaceRefreshService(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, persistence persistenceInputs, workflow workflowInputs) (refreshmodule.Service, error) {
	repo, err := resolveServingStateRepository(routes, runtime, platform, policy, persistence)
	if err != nil {
		return refreshmodule.Service{}, err
	}
	if repo == nil {
		return refreshmodule.Service{}, fmt.Errorf("serving state repository is required")
	}
	hooks := []refreshmodule.CandidateValidationHook{}
	if workflow.managedDataValidation != nil {
		hooks = append(hooks, workflow.managedDataValidation)
	}
	return refreshmodule.Service{
		ServingStates: repo,
		Runtime:       workflow.reloader,
		Publisher: refreshmodule.Publisher{
			Workspace: func() refreshmodule.WorkspaceSupport {
				return workspaceRefreshSupport(routes, runtime, platform, policy)
			},
			SemanticModelVersion: func(ctx context.Context, workspaceID, environment, modelID string) {
				refreshedAt := ""
				if routes.refreshModule != nil {
					if version, ok, err := routes.refreshModule.DataVersion(ctx, workspaceID, environment, modelID); err == nil && ok {
						refreshedAt = version.RefreshedAt.Format(time.RFC3339)
					}
				}
				if routes.dashboardModule != nil {
					routes.dashboardModule.PublishSemanticModelRefresh(workspaceID, environment, modelID, refreshedAt)
				}
			},
		},
		CandidateValidationHooks: hooks,
	}, nil
}
