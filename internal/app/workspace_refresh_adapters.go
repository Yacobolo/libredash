package app

import (
	"context"
	"fmt"
	"net/http"
	"os"

	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	materializesqlite "github.com/Yacobolo/libredash/internal/analytics/materialize/sqlite"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacehttp "github.com/Yacobolo/libredash/internal/workspace/http"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
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
		DirectRunner: appRefreshRunner{metrics: s.metrics},
		ModelLookup:  refreshModelLookup(s.metrics),
		Broker:       s.broker,
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
	return refresh.Service{
		ServingStates: repo,
		Runs:          runRepo,
		Artifacts:     appRefreshArtifactLoader{},
		Materializer: analyticsduckdb.WorkspaceRefreshMaterializer{
			DuckDBDir:       s.duckDBDir,
			DuckLakeCatalog: s.duckLakeCatalogPath,
			DuckLakeData:    s.duckLakeDataPath,
		},
		Runtime:   appRefreshRuntimeHost{reloader: s.reloader},
		Retention: appRefreshRetention{server: s},
		Publisher: appRefreshPublisher{server: s},
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
	return refresh.LoadedArtifact{Definition: compiled.Definition, Graph: compiled.Graph}, nil
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

func (p appRefreshPublisher) PublishRefreshTarget(ctx context.Context, workspaceID, targetType, targetID string) {
	if p.server == nil {
		return
	}
	p.server.workspaceRefreshSupport().PublishWorkspaceAssetRefreshPatchesForTarget(ctx, workspaceID, targetType, targetID)
}

type appDirectRefreshExecutor struct {
	repo    *materializesqlite.SQLRunRepository
	metrics QueryMetrics
	logger  interface {
		WarnContext(context.Context, string, ...any)
	}
}

func (e appDirectRefreshExecutor) ExecuteDirectJob(ctx context.Context, job materialize.JobRecord) error {
	orchestrator := materialize.NewGenericRefreshOrchestrator(e.repo, appRefreshRunner{metrics: e.metrics}, refreshModelLookup(e.metrics))
	_, err := orchestrator.ExecuteRun(ctx, job.WorkspaceID, job.RunID, materialize.RefreshPublisher{})
	if err != nil && e.logger != nil {
		e.logger.WarnContext(ctx, "refresh job failed", "workspace", job.WorkspaceID, "run", job.RunID, "error", err)
	}
	return err
}
