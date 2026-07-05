package refresh

import (
	"context"
	"fmt"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type ServingStateRepository interface {
	ActiveArtifact(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) (servingstate.State, servingstate.Artifact, error)
	Create(ctx context.Context, input servingstate.CreateInput) (servingstate.State, error)
	SaveValidated(ctx context.Context, servingStateID servingstate.ID, validation servingstate.Validation, artifact servingstate.Artifact) (servingstate.State, error)
	ByID(ctx context.Context, id servingstate.ID) (servingstate.State, error)
	ArtifactByServingState(ctx context.Context, servingStateID servingstate.ID) (servingstate.Artifact, error)
	RecordDuckLakeSnapshot(ctx context.Context, servingStateID servingstate.ID, snapshotID int64) error
	Activate(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment, servingStateID servingstate.ID) (servingstate.State, error)
	MarkFailed(ctx context.Context, servingStateID servingstate.ID, cause error) error
}

type RunRepository interface {
	CreateRun(ctx context.Context, input materialize.RunInput) (materialize.RunRecord, error)
	ListChildRuns(ctx context.Context, workspaceID, parentRunID string) ([]materialize.RunRecord, error)
	MarkRunRunning(ctx context.Context, workspaceID, runID string) (materialize.RunRecord, error)
	MarkRunSucceeded(ctx context.Context, workspaceID, runID string) (materialize.RunRecord, error)
	MarkRunFailed(ctx context.Context, workspaceID, runID, message string) (materialize.RunRecord, error)
}

type LoadedArtifact struct {
	Definition *workspace.Definition
	Graph      workspace.AssetGraph
}

type ArtifactLoader interface {
	Load(ctx context.Context, artifact servingstate.Artifact) (LoadedArtifact, error)
}

type Materializer interface {
	Materialize(ctx context.Context, input MaterializeInput) (int64, error)
}

type MaterializeInput struct {
	Definition  *workspace.Definition
	Active      servingstate.State
	Candidate   servingstate.State
	Artifact    servingstate.Artifact
	Environment servingstate.Environment
	Plan        Plan
}

type RuntimeHost interface {
	PrepareServingState(ctx context.Context, servingStateID string) (servingstate.PreparedRuntime, error)
	CommitPrepared(prepared servingstate.PreparedRuntime) error
	Reload(ctx context.Context) error
}

type RetentionRunner interface {
	Run(ctx context.Context, dryRun bool) error
}

type Publisher interface {
	PublishRefreshTarget(ctx context.Context, workspaceID, targetType, targetID string)
}

type Service struct {
	ServingStates ServingStateRepository
	Runs          RunRepository
	Artifacts     ArtifactLoader
	Materializer  Materializer
	Runtime       RuntimeHost
	Retention     RetentionRunner
	Publisher     Publisher
}

type ServingState struct {
	State    servingstate.State
	Artifact servingstate.Artifact
}

type QueueAssetInput struct {
	WorkspaceID string
	Environment servingstate.Environment
	PrincipalID string
	Asset       workspace.AssetView
	DataRoot    string
}

type QueueAssetResult struct {
	Run            materialize.RunRecord
	DependencyRuns []materialize.RunRecord
	ServingStateID servingstate.ID
}

func (s Service) QueueAssetRefresh(ctx context.Context, input QueueAssetInput) (QueueAssetResult, error) {
	if s.ServingStates == nil {
		return QueueAssetResult{}, fmt.Errorf("serving state repository is required")
	}
	if s.Runs == nil {
		return QueueAssetResult{}, fmt.Errorf("refresh run repository is required")
	}
	if s.Artifacts == nil {
		return QueueAssetResult{}, fmt.Errorf("artifact loader is required")
	}
	environment := servingstate.NormalizeEnvironment(input.Environment)
	active, err := s.Active(ctx, input.WorkspaceID, environment)
	if err != nil {
		return QueueAssetResult{}, err
	}
	if input.DataRoot != "" {
		active.Artifact.DataRoot = input.DataRoot
	}
	loaded, err := s.Artifacts.Load(ctx, active.Artifact)
	if err != nil {
		return QueueAssetResult{}, err
	}
	if loaded.Definition == nil {
		return QueueAssetResult{}, fmt.Errorf("compiled workspace definition is required")
	}
	plan, err := PlanForAsset(loaded.Definition, input.WorkspaceID, input.Asset)
	if err != nil {
		return QueueAssetResult{}, err
	}
	candidate := ServingState{}
	rootRun := materialize.RunRecord{}
	dependencyRuns := []materialize.RunRecord(nil)
	finalized := false
	defer func() {
		if finalized {
			return
		}
		cause := fmt.Errorf("refresh did not complete")
		background := context.Background()
		if rootRun.ID != "" {
			_, _ = s.Runs.MarkRunFailed(background, input.WorkspaceID, rootRun.ID, cause.Error())
		}
		for _, run := range dependencyRuns {
			if run.ID != "" {
				_, _ = s.Runs.MarkRunFailed(background, input.WorkspaceID, run.ID, cause.Error())
			}
		}
		_ = s.MarkFailed(background, candidate, cause)
	}()
	candidate, err = s.CreateRefreshCandidate(ctx, RefreshCandidateInput{
		WorkspaceID:   input.WorkspaceID,
		Environment:   environment,
		CreatedBy:     input.PrincipalID,
		Active:        active,
		ArtifactGraph: loaded.Graph,
	})
	if err != nil {
		return QueueAssetResult{}, err
	}
	rootRun, err = s.Runs.CreateRun(ctx, materialize.RunInput{
		WorkspaceID:    input.WorkspaceID,
		ModelID:        plan.ModelID,
		ServingStateID: string(candidate.State.ID),
		PrincipalID:    input.PrincipalID,
		TargetType:     plan.TargetType,
		TargetID:       plan.TargetID,
		TriggerType:    materialize.TriggerDirect,
		JobKind:        materialize.JobKindWorkspaceAssetRefresh,
		PayloadJSON:    fmt.Sprintf(`{"assetKey":%q,"assetType":%q}`, input.Asset.Key, input.Asset.Type),
	})
	if err != nil {
		_ = s.MarkFailed(ctx, candidate, err)
		return QueueAssetResult{}, err
	}
	dependencyRuns = make([]materialize.RunRecord, 0, len(plan.DependencyTables))
	for _, table := range plan.DependencyTables {
		run, err := s.Runs.CreateRun(ctx, materialize.RunInput{
			WorkspaceID:    input.WorkspaceID,
			ModelID:        WorkspaceRefreshModelID,
			ServingStateID: string(candidate.State.ID),
			PrincipalID:    input.PrincipalID,
			TargetType:     materialize.TargetModelTable,
			TargetID:       input.WorkspaceID + "." + table,
			TriggerType:    plan.ChildTrigger,
			ParentRunID:    rootRun.ID,
			JobKind:        materialize.JobKindChildRun,
		})
		if err != nil {
			_, _ = s.Runs.MarkRunFailed(ctx, input.WorkspaceID, rootRun.ID, err.Error())
			_ = s.MarkFailed(ctx, candidate, err)
			return QueueAssetResult{}, err
		}
		dependencyRuns = append(dependencyRuns, run)
	}
	s.publish(ctx, input.WorkspaceID, plan.TargetType, plan.TargetID)
	for _, run := range dependencyRuns {
		s.publish(ctx, input.WorkspaceID, run.TargetType, run.TargetID)
	}
	finalized = true
	return QueueAssetResult{Run: rootRun, DependencyRuns: dependencyRuns, ServingStateID: candidate.State.ID}, nil
}

func (s Service) ExecuteClaimedJob(ctx context.Context, job materialize.JobRecord) error {
	if s.ServingStates == nil {
		return fmt.Errorf("serving state repository is required")
	}
	if s.Runs == nil {
		return fmt.Errorf("refresh run repository is required")
	}
	if s.Artifacts == nil {
		return fmt.Errorf("artifact loader is required")
	}
	if s.Materializer == nil {
		return fmt.Errorf("refresh materializer is required")
	}
	if job.ServingStateID == "" {
		return fmt.Errorf("workspace refresh job serving state id is required")
	}
	candidateState, err := s.ServingStates.ByID(ctx, servingstate.ID(job.ServingStateID))
	if err != nil {
		return err
	}
	if candidateState.Status == servingstate.StatusActive && candidateState.DuckLakeSnapshotID > 0 {
		_, _ = s.Runs.MarkRunSucceeded(ctx, job.WorkspaceID, job.RunID)
		return nil
	}
	candidateArtifact, err := s.ServingStates.ArtifactByServingState(ctx, candidateState.ID)
	if err != nil {
		return err
	}
	activeState, err := s.Active(ctx, job.WorkspaceID, candidateState.Environment)
	if err != nil {
		return err
	}
	loaded, err := s.Artifacts.Load(ctx, candidateArtifact)
	if err != nil {
		return err
	}
	if loaded.Definition == nil {
		return fmt.Errorf("compiled workspace definition is required")
	}
	asset := workspace.AssetView{Type: AssetTypeForRefreshTarget(job.TargetType), Key: job.TargetID, Title: job.TargetID}
	plan, err := PlanForAsset(loaded.Definition, job.WorkspaceID, asset)
	if err != nil {
		return err
	}
	childRuns, err := s.Runs.ListChildRuns(ctx, job.WorkspaceID, job.RunID)
	if err != nil {
		return err
	}
	for _, child := range childRuns {
		_, _ = s.Runs.MarkRunRunning(ctx, job.WorkspaceID, child.ID)
		s.publish(ctx, job.WorkspaceID, child.TargetType, child.TargetID)
	}
	s.publish(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
	candidate := ServingState{State: candidateState, Artifact: candidateArtifact}
	snapshotID, err := s.Materializer.Materialize(ctx, MaterializeInput{
		Definition:  loaded.Definition,
		Active:      activeState.State,
		Candidate:   candidate.State,
		Artifact:    candidate.Artifact,
		Environment: candidateState.Environment,
		Plan:        plan,
	})
	if err != nil {
		s.failJob(ctx, job, childRuns, candidate, err)
		return err
	}
	if err := s.RecordSnapshot(ctx, candidate, snapshotID); err != nil {
		s.failJob(ctx, job, childRuns, candidate, err)
		return err
	}
	var prepared servingstate.PreparedRuntime
	if s.Runtime != nil {
		prepared, err = s.Runtime.PrepareServingState(ctx, string(candidateState.ID))
		if err != nil {
			s.failJob(ctx, job, childRuns, candidate, err)
			return err
		}
	}
	if _, err := s.Activate(ctx, candidate); err != nil {
		if prepared != nil {
			_ = prepared.Close()
		}
		s.failJob(ctx, job, childRuns, candidate, err)
		return err
	}
	if prepared != nil {
		if err := s.Runtime.CommitPrepared(prepared); err != nil {
			_ = prepared.Close()
			_, _ = s.Runs.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, err.Error())
			s.publish(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
			return err
		}
	} else if s.Runtime != nil {
		_ = s.Runtime.Reload(ctx)
	}
	if s.Retention != nil {
		_ = s.Retention.Run(ctx, false)
	}
	for _, child := range childRuns {
		_, _ = s.Runs.MarkRunSucceeded(ctx, job.WorkspaceID, child.ID)
		s.publish(ctx, job.WorkspaceID, child.TargetType, child.TargetID)
	}
	_, err = s.Runs.MarkRunSucceeded(ctx, job.WorkspaceID, job.RunID)
	s.publish(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
	return err
}

func (s Service) failJob(ctx context.Context, job materialize.JobRecord, childRuns []materialize.RunRecord, candidate ServingState, cause error) {
	_, _ = s.Runs.MarkRunFailed(ctx, job.WorkspaceID, job.RunID, cause.Error())
	for _, child := range childRuns {
		_, _ = s.Runs.MarkRunFailed(ctx, job.WorkspaceID, child.ID, cause.Error())
		s.publish(ctx, job.WorkspaceID, child.TargetType, child.TargetID)
	}
	_ = s.MarkFailed(ctx, candidate, cause)
	s.publish(ctx, job.WorkspaceID, job.TargetType, job.TargetID)
}

func (s Service) publish(ctx context.Context, workspaceID, targetType, targetID string) {
	if s.Publisher != nil {
		s.Publisher.PublishRefreshTarget(ctx, workspaceID, targetType, targetID)
	}
}

type RefreshCandidateInput struct {
	WorkspaceID   string
	Environment   servingstate.Environment
	CreatedBy     string
	Active        ServingState
	ArtifactGraph workspace.AssetGraph
}

func (s Service) Active(ctx context.Context, workspaceID string, environment servingstate.Environment) (ServingState, error) {
	active, artifact, err := s.ServingStates.ActiveArtifact(ctx, servingstate.WorkspaceID(workspaceID), environment)
	if err != nil {
		return ServingState{}, err
	}
	return ServingState{State: active, Artifact: artifact}, nil
}

func (s Service) CreateRefreshCandidate(ctx context.Context, input RefreshCandidateInput) (ServingState, error) {
	active := input.Active
	workspaceID := servingstate.WorkspaceID(input.WorkspaceID)
	environment := servingstate.NormalizeEnvironment(input.Environment)
	created, err := s.ServingStates.Create(ctx, servingstate.CreateInput{
		WorkspaceID: workspaceID,
		Environment: environment,
		CreatedBy:   input.CreatedBy,
		Source:      servingstate.SourceRefresh,
	})
	if err != nil {
		return ServingState{}, err
	}
	candidateArtifact := servingstate.Artifact{
		ID:             "artifact_" + string(created.ID),
		ServingStateID: created.ID,
		WorkspaceID:    workspaceID,
		Environment:    environment,
		Digest:         active.Artifact.Digest,
		Format:         active.Artifact.Format,
		Path:           active.Artifact.Path,
		DataRoot:       active.Artifact.DataRoot,
		ManifestJSON:   active.Artifact.ManifestJSON,
		SizeBytes:      active.Artifact.SizeBytes,
		CreatedAt:      active.Artifact.CreatedAt,
	}
	validated, err := s.ServingStates.SaveValidated(ctx, created.ID, servingstate.Validation{
		Digest:       active.State.Digest,
		ManifestJSON: active.State.ManifestJSON,
		Graph:        RetargetAssetGraph(input.ArtifactGraph, workspace.WorkspaceID(input.WorkspaceID), workspace.ServingStateID(created.ID)),
		DataRoot:     active.Artifact.DataRoot,
	}, candidateArtifact)
	if err != nil {
		_ = s.ServingStates.MarkFailed(ctx, created.ID, err)
		return ServingState{}, err
	}
	return ServingState{State: validated, Artifact: candidateArtifact}, nil
}

func (s Service) RecordSnapshot(ctx context.Context, candidate ServingState, snapshotID int64) error {
	if snapshotID <= 0 {
		return fmt.Errorf("serving state snapshot id must be positive")
	}
	return s.ServingStates.RecordDuckLakeSnapshot(ctx, candidate.State.ID, snapshotID)
}

func (s Service) Activate(ctx context.Context, candidate ServingState) (servingstate.State, error) {
	return s.ServingStates.Activate(ctx, candidate.State.WorkspaceID, candidate.State.Environment, candidate.State.ID)
}

func (s Service) MarkFailed(ctx context.Context, state ServingState, cause error) error {
	if state.State.ID == "" || cause == nil {
		return nil
	}
	return s.ServingStates.MarkFailed(ctx, state.State.ID, cause)
}

func RetargetAssetGraph(graph workspace.AssetGraph, workspaceID workspace.WorkspaceID, servingStateID workspace.ServingStateID) workspace.AssetGraph {
	out := workspace.AssetGraph{
		Assets: make([]workspace.Asset, 0, len(graph.Assets)),
		Edges:  make([]workspace.AssetEdge, 0, len(graph.Edges)),
	}
	for _, asset := range graph.Assets {
		asset.WorkspaceID = workspaceID
		asset.ServingStateID = servingStateID
		asset.SnapshotID = workspace.NewAssetSnapshotID(servingStateID, asset.ID)
		out.Assets = append(out.Assets, asset)
	}
	for _, edge := range graph.Edges {
		edge.WorkspaceID = workspaceID
		edge.ServingStateID = servingStateID
		edge.ID = workspace.NewAssetEdgeID(servingStateID, edge.FromAssetID, edge.ToAssetID, edge.Type)
		out.Edges = append(out.Edges, edge)
	}
	return out
}
