package app

import (
	"context"
	"fmt"
	"os"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentfs "github.com/Yacobolo/libredash/internal/deployment/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
)

func (s *Server) workspaceRefreshService(runRepo refresh.RunRepository) (refresh.Service, error) {
	repo, err := s.deploymentRepository()
	if err != nil {
		return refresh.Service{}, err
	}
	if repo == nil {
		return refresh.Service{}, fmt.Errorf("deployment repository is required")
	}
	return refresh.Service{
		Deployments:  repo,
		Runs:         runRepo,
		Artifacts:    appRefreshArtifactLoader{},
		Materializer: appRefreshMaterializer{server: s},
		Runtime:      appRefreshRuntimeHost{reloader: s.reloader},
		Retention:    appRefreshRetention{server: s},
		Publisher:    appRefreshPublisher{server: s},
	}, nil
}

type appRefreshArtifactLoader struct{}

func (appRefreshArtifactLoader) Load(_ context.Context, artifact deployment.Artifact) (refresh.LoadedArtifact, error) {
	root, err := os.MkdirTemp("", "libredash-refresh-artifact-*")
	if err != nil {
		return refresh.LoadedArtifact{}, err
	}
	defer os.RemoveAll(root)
	if err := deploymentfs.ExtractArtifact(artifact.Path, root); err != nil {
		return refresh.LoadedArtifact{}, err
	}
	compiled, _, err := deploymentfs.LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		return refresh.LoadedArtifact{}, err
	}
	return refresh.LoadedArtifact{Definition: compiled.Definition, Graph: compiled.Graph}, nil
}

type appRefreshMaterializer struct {
	server *Server
}

func (m appRefreshMaterializer) Materialize(ctx context.Context, input refresh.MaterializeInput) (int64, error) {
	if m.server == nil {
		return 0, fmt.Errorf("server is required")
	}
	return m.server.executeWorkspaceAssetRefreshPlan(ctx, input.Definition, input.Active, input.Candidate, input.Artifact, input.Environment, input.Plan)
}

type appRefreshRuntimeHost struct {
	reloader runtimeReloader
}

func (h appRefreshRuntimeHost) PrepareDeployment(ctx context.Context, deploymentID string) (deployment.PreparedRuntime, error) {
	if h.reloader == nil {
		return nil, nil
	}
	return h.reloader.PrepareDeployment(ctx, deploymentID)
}

func (h appRefreshRuntimeHost) CommitPrepared(prepared deployment.PreparedRuntime) error {
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
	p.server.publishWorkspaceAssetRefreshPatchesForTarget(ctx, workspaceID, targetType, targetID)
}

type appLegacyRefreshExecutor struct {
	repo    *materialize.SQLRunRepository
	metrics QueryMetrics
	logger  interface {
		WarnContext(context.Context, string, ...any)
	}
}

func (e appLegacyRefreshExecutor) ExecuteLegacyJob(ctx context.Context, job materialize.JobRecord) error {
	orchestrator := NewGenericRefreshOrchestrator(e.repo, e.metrics)
	_, err := orchestrator.ExecuteRun(ctx, job.WorkspaceID, job.RunID, refreshPublisher{})
	if err != nil && e.logger != nil {
		e.logger.WarnContext(ctx, "materialization job failed", "workspace", job.WorkspaceID, "run", job.RunID, "error", err)
	}
	return err
}
