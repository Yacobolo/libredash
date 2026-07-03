package refresh

import (
	"context"
	"fmt"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type DeploymentRepository interface {
	ActiveArtifact(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment) (deployment.Deployment, deployment.Artifact, error)
	Create(ctx context.Context, input deployment.CreateInput) (deployment.Deployment, error)
	SaveValidated(ctx context.Context, deploymentID deployment.ID, validation deployment.Validation, artifact deployment.Artifact) (deployment.Deployment, error)
	ByID(ctx context.Context, id deployment.ID) (deployment.Deployment, error)
	ArtifactByDeployment(ctx context.Context, deploymentID deployment.ID) (deployment.Artifact, error)
	RecordDuckLakeSnapshot(ctx context.Context, deploymentID deployment.ID, snapshotID int64) error
	Activate(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment, deploymentID deployment.ID) (deployment.Deployment, error)
	MarkFailed(ctx context.Context, deploymentID deployment.ID, cause error) error
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
	Load(ctx context.Context, artifact deployment.Artifact) (LoadedArtifact, error)
}

type Materializer interface {
	Materialize(ctx context.Context, input MaterializeInput) (int64, error)
}

type MaterializeInput struct {
	Definition  *workspace.Definition
	Active      deployment.Deployment
	Candidate   deployment.Deployment
	Artifact    deployment.Artifact
	Environment deployment.Environment
	Plan        Plan
}

type RuntimeHost interface {
	PrepareDeployment(ctx context.Context, deploymentID string) (deployment.PreparedRuntime, error)
	CommitPrepared(prepared deployment.PreparedRuntime) error
	Reload(ctx context.Context) error
}

type RetentionRunner interface {
	Run(ctx context.Context, dryRun bool) error
}

type Publisher interface {
	PublishRefreshTarget(ctx context.Context, workspaceID, targetType, targetID string)
}

type Service struct {
	Deployments  DeploymentRepository
	Runs         RunRepository
	Artifacts    ArtifactLoader
	Materializer Materializer
	Runtime      RuntimeHost
	Retention    RetentionRunner
	Publisher    Publisher
}

type ServingState struct {
	Deployment deployment.Deployment
	Artifact   deployment.Artifact
}

type QueueAssetInput struct {
	WorkspaceID string
	Environment deployment.Environment
	PrincipalID string
	Asset       workspace.AssetView
	DataRoot    string
}

type QueueAssetResult struct {
	Run            materialize.RunRecord
	DependencyRuns []materialize.RunRecord
	DeploymentID   deployment.ID
}

func (s Service) QueueAssetRefresh(ctx context.Context, input QueueAssetInput) (QueueAssetResult, error) {
	if s.Deployments == nil {
		return QueueAssetResult{}, fmt.Errorf("deployment repository is required")
	}
	if s.Runs == nil {
		return QueueAssetResult{}, fmt.Errorf("refresh run repository is required")
	}
	if s.Artifacts == nil {
		return QueueAssetResult{}, fmt.Errorf("artifact loader is required")
	}
	environment := deployment.NormalizeEnvironment(input.Environment)
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
		WorkspaceID:  input.WorkspaceID,
		ModelID:      plan.ModelID,
		DeploymentID: string(candidate.Deployment.ID),
		PrincipalID:  input.PrincipalID,
		TargetType:   plan.TargetType,
		TargetID:     plan.TargetID,
		TriggerType:  materialize.TriggerDirect,
		JobKind:      materialize.JobKindWorkspaceAssetRefresh,
		PayloadJSON:  fmt.Sprintf(`{"assetKey":%q,"assetType":%q}`, input.Asset.Key, input.Asset.Type),
	})
	if err != nil {
		_ = s.MarkFailed(ctx, candidate, err)
		return QueueAssetResult{}, err
	}
	dependencyRuns = make([]materialize.RunRecord, 0, len(plan.DependencyTables))
	for _, table := range plan.DependencyTables {
		run, err := s.Runs.CreateRun(ctx, materialize.RunInput{
			WorkspaceID:  input.WorkspaceID,
			ModelID:      WorkspaceRefreshModelID,
			DeploymentID: string(candidate.Deployment.ID),
			PrincipalID:  input.PrincipalID,
			TargetType:   materialize.TargetModelTable,
			TargetID:     input.WorkspaceID + "." + table,
			TriggerType:  plan.ChildTrigger,
			ParentRunID:  rootRun.ID,
			JobKind:      materialize.JobKindChildRun,
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
	return QueueAssetResult{Run: rootRun, DependencyRuns: dependencyRuns, DeploymentID: candidate.Deployment.ID}, nil
}

func (s Service) ExecuteClaimedJob(ctx context.Context, job materialize.JobRecord) error {
	if s.Deployments == nil {
		return fmt.Errorf("deployment repository is required")
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
	if job.DeploymentID == "" {
		return fmt.Errorf("workspace refresh job deployment id is required")
	}
	candidateDeployment, err := s.Deployments.ByID(ctx, deployment.ID(job.DeploymentID))
	if err != nil {
		return err
	}
	if candidateDeployment.Status == deployment.StatusActive && candidateDeployment.DuckLakeSnapshotID > 0 {
		_, _ = s.Runs.MarkRunSucceeded(ctx, job.WorkspaceID, job.RunID)
		return nil
	}
	candidateArtifact, err := s.Deployments.ArtifactByDeployment(ctx, candidateDeployment.ID)
	if err != nil {
		return err
	}
	activeState, err := s.Active(ctx, job.WorkspaceID, candidateDeployment.Environment)
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
	candidate := ServingState{Deployment: candidateDeployment, Artifact: candidateArtifact}
	snapshotID, err := s.Materializer.Materialize(ctx, MaterializeInput{
		Definition:  loaded.Definition,
		Active:      activeState.Deployment,
		Candidate:   candidate.Deployment,
		Artifact:    candidate.Artifact,
		Environment: candidateDeployment.Environment,
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
	var prepared deployment.PreparedRuntime
	if s.Runtime != nil {
		prepared, err = s.Runtime.PrepareDeployment(ctx, string(candidateDeployment.ID))
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
	Environment   deployment.Environment
	CreatedBy     string
	Active        ServingState
	ArtifactGraph workspace.AssetGraph
}

func (s Service) Active(ctx context.Context, workspaceID string, environment deployment.Environment) (ServingState, error) {
	active, artifact, err := s.Deployments.ActiveArtifact(ctx, deployment.WorkspaceID(workspaceID), environment)
	if err != nil {
		return ServingState{}, err
	}
	return ServingState{Deployment: active, Artifact: artifact}, nil
}

func (s Service) CreateRefreshCandidate(ctx context.Context, input RefreshCandidateInput) (ServingState, error) {
	active := input.Active
	workspaceID := deployment.WorkspaceID(input.WorkspaceID)
	environment := deployment.NormalizeEnvironment(input.Environment)
	created, err := s.Deployments.Create(ctx, deployment.CreateInput{
		WorkspaceID: workspaceID,
		Environment: environment,
		CreatedBy:   input.CreatedBy,
		Source:      deployment.SourceRefresh,
	})
	if err != nil {
		return ServingState{}, err
	}
	candidateArtifact := deployment.Artifact{
		ID:           "artifact_" + string(created.ID),
		DeploymentID: created.ID,
		WorkspaceID:  workspaceID,
		Environment:  environment,
		Digest:       active.Artifact.Digest,
		Format:       active.Artifact.Format,
		Path:         active.Artifact.Path,
		DataRoot:     active.Artifact.DataRoot,
		ManifestJSON: active.Artifact.ManifestJSON,
		SizeBytes:    active.Artifact.SizeBytes,
		CreatedAt:    active.Artifact.CreatedAt,
	}
	validated, err := s.Deployments.SaveValidated(ctx, created.ID, deployment.Validation{
		Digest:       active.Deployment.Digest,
		ManifestJSON: active.Deployment.ManifestJSON,
		Graph:        RetargetAssetGraph(input.ArtifactGraph, workspace.WorkspaceID(input.WorkspaceID), workspace.DeploymentID(created.ID)),
		DataRoot:     active.Artifact.DataRoot,
	}, candidateArtifact)
	if err != nil {
		_ = s.Deployments.MarkFailed(ctx, created.ID, err)
		return ServingState{}, err
	}
	return ServingState{Deployment: validated, Artifact: candidateArtifact}, nil
}

func (s Service) RecordSnapshot(ctx context.Context, candidate ServingState, snapshotID int64) error {
	if snapshotID <= 0 {
		return fmt.Errorf("serving state snapshot id must be positive")
	}
	return s.Deployments.RecordDuckLakeSnapshot(ctx, candidate.Deployment.ID, snapshotID)
}

func (s Service) Activate(ctx context.Context, candidate ServingState) (deployment.Deployment, error) {
	return s.Deployments.Activate(ctx, candidate.Deployment.WorkspaceID, candidate.Deployment.Environment, candidate.Deployment.ID)
}

func (s Service) MarkFailed(ctx context.Context, state ServingState, cause error) error {
	if state.Deployment.ID == "" || cause == nil {
		return nil
	}
	return s.Deployments.MarkFailed(ctx, state.Deployment.ID, cause)
}

func RetargetAssetGraph(graph workspace.AssetGraph, workspaceID workspace.WorkspaceID, deploymentID workspace.DeploymentID) workspace.AssetGraph {
	out := workspace.AssetGraph{
		Assets: make([]workspace.Asset, 0, len(graph.Assets)),
		Edges:  make([]workspace.AssetEdge, 0, len(graph.Edges)),
	}
	for _, asset := range graph.Assets {
		asset.WorkspaceID = workspaceID
		asset.DeploymentID = deploymentID
		asset.SnapshotID = workspace.NewAssetSnapshotID(deploymentID, asset.ID)
		out.Assets = append(out.Assets, asset)
	}
	for _, edge := range graph.Edges {
		edge.WorkspaceID = workspaceID
		edge.DeploymentID = deploymentID
		edge.ID = workspace.NewAssetEdgeID(deploymentID, edge.FromAssetID, edge.ToAssetID, edge.Type)
		out.Edges = append(out.Edges, edge)
	}
	return out
}
