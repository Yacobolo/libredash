package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/refreshpipeline"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
)

func (s *Server) startRefreshPipelineScheduler(ctx context.Context) {
	if s.refreshPipelineRepo == nil || s.servingStateRepo == nil || s.refreshPipelineSchedulerStarted {
		return
	}
	s.refreshPipelineSchedulerStarted = true
	s.jobDispatchWG.Add(1)
	go func() {
		defer s.jobDispatchWG.Done()
		repository := s.refreshPipelineRepo
		scheduler := refreshpipeline.Scheduler{
			Repository:  repository,
			Clock:       s.refreshPipelineClock,
			Environment: string(s.defaultServingEnvironment()),
			Trigger: func(ctx context.Context, occurrence refreshpipeline.Occurrence) (string, error) {
				runs, err := s.refreshRunRepository()
				if err != nil {
					return "", err
				}
				service, err := s.workspaceRefreshService(runs)
				if err != nil {
					return "", err
				}
				result, err := service.QueuePipelineRefresh(ctx, refresh.QueuePipelineInput{
					WorkspaceID: occurrence.WorkspaceID, Environment: servingstate.Environment(occurrence.Environment),
					PipelineID: occurrence.PipelineID, TriggerType: materialize.TriggerSchedule, ArtifactDigest: occurrence.ArtifactDigest,
					Occurrence: &occurrence,
				})
				if err == nil {
					s.dispatchQueuedRefreshJobs(ctx)
				}
				return result.Run.ID, err
			},
		}
		if err := s.reconcileRefreshPipelineSchedules(ctx, repository); err != nil {
			s.logger.WarnContext(ctx, "reconcile refresh pipeline schedules failed", "error", err)
		}
		dispatch := func() {
			if err := scheduler.DispatchDue(ctx); err != nil {
				s.logger.WarnContext(ctx, "dispatch scheduled refresh pipelines failed", "error", err)
			}
		}
		dispatch()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				dispatch()
			}
		}
	}()
}

func (s *Server) reconcileRefreshPipelineSchedules(ctx context.Context, repository refreshpipeline.Repository) error {
	states, err := s.servingStateRepository()
	if err != nil {
		return err
	}
	scopes, err := states.ListActiveScopes(ctx)
	if err != nil {
		return err
	}
	clock := s.refreshPipelineClock
	if clock == nil {
		clock = refreshpipeline.RealClock{}
	}
	var reconcileErrors []error
	for _, scope := range activeRefreshPipelineScopes(scopes, s.defaultServingEnvironment()) {
		workspaceID := string(scope.WorkspaceID)
		environment := scope.Environment
		state, artifact, err := states.ActiveArtifact(ctx, scope.WorkspaceID, environment)
		if err != nil {
			reconcileErrors = append(reconcileErrors, err)
			continue
		}
		loaded, err := (appRefreshArtifactLoader{}).Load(ctx, artifact)
		if err != nil {
			reconcileErrors = append(reconcileErrors, err)
			continue
		}
		pipelines := make([]refreshpipeline.Definition, 0, len(loaded.Definition.RefreshPipelines))
		for _, pipeline := range loaded.Definition.RefreshPipelines {
			pipelines = append(pipelines, pipeline)
		}
		sort.Slice(pipelines, func(i, j int) bool { return pipelines[i].ID < pipelines[j].ID })
		if err := repository.Reconcile(ctx, refreshpipeline.ReconcileInput{
			WorkspaceID: workspaceID, Environment: string(environment), ArtifactDigest: artifact.Digest,
			Pipelines: pipelines, Now: clock.Now(),
		}); err != nil {
			reconcileErrors = append(reconcileErrors, err)
			continue
		}
		if state.Source == servingstate.SourcePublish && state.DuckLakeSnapshotID > 0 {
			refreshedAt, err := parseServingStateTime(state.ActivatedAt)
			if err != nil {
				reconcileErrors = append(reconcileErrors, err)
				continue
			}
			for modelID := range loaded.Definition.Models {
				current, ok, err := repository.DataVersion(ctx, workspaceID, string(environment), modelID)
				if err != nil {
					reconcileErrors = append(reconcileErrors, err)
					continue
				}
				if ok && current.ServingStateID == string(state.ID) {
					continue
				}
				if err := repository.SaveDataVersion(ctx, refreshpipeline.DataVersion{
					WorkspaceID: workspaceID, Environment: string(environment), SemanticModel: modelID,
					SnapshotID: state.DuckLakeSnapshotID, ServingStateID: string(state.ID), RefreshedAt: refreshedAt,
					Source: refreshpipeline.DataVersionSourcePublish,
				}); err != nil {
					reconcileErrors = append(reconcileErrors, err)
					continue
				}
				(appRefreshPublisher{server: s}).PublishSemanticModelVersion(ctx, workspaceID, string(environment), modelID)
			}
		}
	}
	return errors.Join(reconcileErrors...)
}

func activeRefreshPipelineScopes(scopes []servingstate.ActiveScope, environment servingstate.Environment) []servingstate.ActiveScope {
	environment = servingstate.NormalizeEnvironment(environment)
	out := make([]servingstate.ActiveScope, 0, len(scopes))
	for _, scope := range scopes {
		if servingstate.NormalizeEnvironment(scope.Environment) == environment {
			out = append(out, scope)
		}
	}
	return out
}

func parseServingStateTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid serving-state activation time %q", value)
}
