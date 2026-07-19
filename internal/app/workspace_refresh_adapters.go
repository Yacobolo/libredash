package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	manageddatabinding "github.com/Yacobolo/libredash/internal/manageddata/binding"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacehttp "github.com/Yacobolo/libredash/internal/workspace/http"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
	"github.com/Yacobolo/libredash/pkg/pagestream"
)

func (s *Server) workspaceRefreshSupport() workspacehttp.Support {
	return workspacehttp.Support{
		Runs: func() (workspacehttp.RunRepository, error) {
			return s.refreshRunRepository()
		},
		Service: func(repo workspacehttp.RunRepository) (refresh.Service, error) {
			return s.workspaceRefreshService(repo)
		},
		Environment: func(r *http.Request) servingstate.Environment {
			return s.requestServingEnvironment(r)
		},
		PrincipalID: func(r *http.Request) string {
			principal, _ := currentPrincipal(s, r)
			return principal.ID
		},
		DispatchQueued: func() {
			s.dispatchQueuedRefreshJobs(context.Background())
		},
		Broker: s.broker,
		AssetCatalog: func(ctx context.Context, workspaceID string) ([]workspace.AssetView, []workspace.AssetEdgeView, bool) {
			assets, edges, err := s.workspaceHTTPReadModel().WorkspaceAssetsAndEdgesForData(ctx, workspaceID, string(s.defaultServingEnvironment()))
			if err != nil || (len(assets) == 0 && len(edges) == 0) {
				return nil, nil, false
			}
			return assets, edges, true
		},
		WorkspaceView: func(r *http.Request, workspaceID string) workspace.WorkspaceView {
			return s.workspaceHTTPReadModel().WorkspaceResponse(r, workspaceID)
		},
		WorkspaceViewContext: func(ctx context.Context, workspaceID string) workspace.WorkspaceView {
			return s.workspaceHTTPReadModel().WorkspaceViewContext(ctx, workspaceID)
		},
		WorkspaceVersions: s.assetVersionsStateForSection,
		DataVersions:      s.refreshPipelineRepo,
	}
}

func (s *Server) workspaceRefreshService(runRepo refresh.RunRepository) (refresh.Service, error) {
	repo, err := s.servingStateRepository()
	if err != nil {
		return refresh.Service{}, err
	}
	if repo == nil {
		return refresh.Service{}, fmt.Errorf("serving state repository is required")
	}
	hooks := []refresh.CandidateValidationHook{}
	if s.managedDataBindingRepo != nil {
		binder, err := manageddatabinding.New(s.managedDataBindingRepo)
		if err != nil {
			return refresh.Service{}, err
		}
		hooks = append(hooks, binder)
	}
	return refresh.Service{
		ServingStates: repo,
		Runs:          runRepo,
		Artifacts:     appRefreshArtifactLoader{},
		Materializer: analyticsduckdb.WorkspaceRefreshMaterializer{
			DuckDBDir:       s.duckDBDir,
			DuckLakeCatalog: s.duckLakeCatalogPath,
			DuckLakeData:    s.duckLakeDataPath,
			ManagedData:     s.managedDataResolver,
		},
		Runtime:                  appRefreshRuntimeHost{reloader: s.reloader},
		Retention:                appRefreshRetention{server: s},
		Publisher:                appRefreshPublisher{server: s},
		DataVersions:             s.refreshPipelineRepo,
		CandidateValidationHooks: hooks,
	}, nil
}

type appRefreshArtifactLoader struct{}

func (appRefreshArtifactLoader) Load(_ context.Context, artifact servingstate.Artifact) (refresh.LoadedArtifact, error) {
	root, err := os.MkdirTemp("", "libredash-refresh-artifact-*")
	if err != nil {
		return refresh.LoadedArtifact{}, err
	}
	defer os.RemoveAll(root)
	if err := servingstatefs.ExtractArtifact(artifact.Path, root); err != nil {
		return refresh.LoadedArtifact{}, err
	}
	compiled, _, err := servingstatefs.LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		return refresh.LoadedArtifact{}, err
	}
	return refresh.LoadedArtifact{
		Definition: compiled.Definition, Graph: compiled.Graph,
		ManagedDataRevisions: compiled.ManagedDataRevisions,
	}, nil
}

type appRefreshRuntimeHost struct {
	reloader runtimeReloader
}

func (h appRefreshRuntimeHost) PrepareServingState(ctx context.Context, servingStateID string) (servingstate.PreparedRuntime, error) {
	if h.reloader == nil {
		return nil, nil
	}
	return h.reloader.PrepareServingState(ctx, servingStateID)
}

func (h appRefreshRuntimeHost) CommitPrepared(prepared servingstate.PreparedRuntime) error {
	if h.reloader == nil || prepared == nil {
		return nil
	}
	return h.reloader.CommitPrepared(prepared)
}

func (h appRefreshRuntimeHost) CommitPreparedWithActivation(prepared servingstate.PreparedRuntime, activate func() error) error {
	if h.reloader == nil || prepared == nil {
		return activate()
	}
	if atomic, ok := h.reloader.(interface {
		CommitPreparedWithActivation(servingstate.PreparedRuntime, func() error) error
	}); ok {
		return atomic.CommitPreparedWithActivation(prepared, activate)
	}
	if err := activate(); err != nil {
		return err
	}
	return h.reloader.CommitPrepared(prepared)
}

func (h appRefreshRuntimeHost) Reload(ctx context.Context) error {
	if h.reloader == nil {
		return nil
	}
	return h.reloader.Reload(ctx)
}

type appRefreshRetention struct {
	server *Server
}

func (r appRefreshRetention) Run(ctx context.Context, dryRun bool) error {
	if r.server == nil {
		return nil
	}
	return r.server.reconcileStorageRetention(ctx, dryRun)
}

type appRefreshPublisher struct {
	server *Server
}

func (p appRefreshPublisher) PublishRefreshTarget(ctx context.Context, workspaceID, environment, targetType, targetID string) {
	if p.server == nil {
		return
	}
	p.server.workspaceRefreshSupport().PublishWorkspaceAssetRefreshPatchesForTarget(ctx, workspaceID, environment, targetType, targetID)
}

func (p appRefreshPublisher) PublishSemanticModelVersion(ctx context.Context, workspaceID, environment, modelID string) {
	if p.server == nil {
		return
	}
	refreshedAt := ""
	if p.server.refreshPipelineRepo != nil {
		if version, ok, err := p.server.refreshPipelineRepo.DataVersion(ctx, workspaceID, environment, modelID); err == nil && ok {
			refreshedAt = version.RefreshedAt.Format(time.RFC3339)
		}
	}
	for _, streamID := range p.server.dashboardRefreshes.RefreshSemanticModel(workspaceID, environment, modelID) {
		p.server.broker.Publish(streamID, pagestream.SignalPatch{"status": map[string]any{"lastUpdated": refreshedAt}})
	}
}
