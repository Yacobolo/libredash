package refresh

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestServiceExecuteClaimedJobActivatesAfterMaterializeAndPrepare(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	publisher := &fakePublisher{}
	materializer := &fakeMaterializer{snapshotID: 42}
	runtime := &fakeRuntimeHost{}
	retention := &fakeRetention{}
	service := Service{
		Deployments:  repo,
		Runs:         repo,
		Artifacts:    fakeArtifactLoader{definition: refreshTestDefinition()},
		Materializer: materializer,
		Runtime:      runtime,
		Retention:    retention,
		Publisher:    publisher,
	}

	err := service.ExecuteClaimedJob(ctx, materialize.JobRecord{
		ID:           "job_1",
		WorkspaceID:  "sales",
		DeploymentID: "dep_candidate",
		RunID:        "run_root",
		TargetType:   materialize.TargetModelTable,
		TargetID:     "sales.orders",
		Kind:         materialize.JobKindWorkspaceAssetRefresh,
	})
	if err != nil {
		t.Fatalf("execute claimed job: %v", err)
	}

	if repo.recordedSnapshotDeployment != "dep_candidate" || repo.recordedSnapshot != 42 {
		t.Fatalf("recorded snapshot = %s/%d, want dep_candidate/42", repo.recordedSnapshotDeployment, repo.recordedSnapshot)
	}
	if repo.activatedDeployment != "dep_candidate" {
		t.Fatalf("activated deployment = %s, want dep_candidate", repo.activatedDeployment)
	}
	if !runtime.prepared || !runtime.committed {
		t.Fatalf("runtime prepared/committed = %v/%v, want true/true", runtime.prepared, runtime.committed)
	}
	if !retention.ran {
		t.Fatal("retention was not reconciled")
	}
	if repo.runStatuses["run_root"] != materialize.RunStatusSucceeded || repo.runStatuses["run_child"] != materialize.RunStatusSucceeded {
		t.Fatalf("run statuses = %#v, want root and child succeeded", repo.runStatuses)
	}
	if got, want := materializer.tables, []string{"customers", "orders"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("materialized tables = %#v, want %#v", got, want)
	}
}

func TestServiceExecuteClaimedJobMaterializeFailureDoesNotActivate(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	service := Service{
		Deployments:  repo,
		Runs:         repo,
		Artifacts:    fakeArtifactLoader{definition: refreshTestDefinition()},
		Materializer: &fakeMaterializer{err: errors.New("materialize failed")},
		Runtime:      &fakeRuntimeHost{},
	}

	err := service.ExecuteClaimedJob(ctx, materialize.JobRecord{
		ID:           "job_1",
		WorkspaceID:  "sales",
		DeploymentID: "dep_candidate",
		RunID:        "run_root",
		TargetType:   materialize.TargetModelTable,
		TargetID:     "sales.orders",
		Kind:         materialize.JobKindWorkspaceAssetRefresh,
	})
	if err == nil {
		t.Fatal("execute claimed job error = nil, want materialize failure")
	}
	if repo.activatedDeployment != "" {
		t.Fatalf("activated deployment = %s, want none", repo.activatedDeployment)
	}
	if repo.failedDeployment != "dep_candidate" {
		t.Fatalf("failed deployment = %s, want dep_candidate", repo.failedDeployment)
	}
	if repo.runStatuses["run_root"] != materialize.RunStatusFailed || repo.runStatuses["run_child"] != materialize.RunStatusFailed {
		t.Fatalf("run statuses = %#v, want root and child failed", repo.runStatuses)
	}
}

func TestServiceExecuteClaimedJobRuntimePrepareFailureDoesNotActivate(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	service := Service{
		Deployments:  repo,
		Runs:         repo,
		Artifacts:    fakeArtifactLoader{definition: refreshTestDefinition()},
		Materializer: &fakeMaterializer{snapshotID: 42},
		Runtime:      &fakeRuntimeHost{prepareErr: errors.New("prepare failed")},
	}

	err := service.ExecuteClaimedJob(ctx, materialize.JobRecord{
		ID:           "job_1",
		WorkspaceID:  "sales",
		DeploymentID: "dep_candidate",
		RunID:        "run_root",
		TargetType:   materialize.TargetModelTable,
		TargetID:     "sales.orders",
		Kind:         materialize.JobKindWorkspaceAssetRefresh,
	})
	if err == nil {
		t.Fatal("execute claimed job error = nil, want prepare failure")
	}
	if repo.activatedDeployment != "" {
		t.Fatalf("activated deployment = %s, want none", repo.activatedDeployment)
	}
	if repo.recordedSnapshot != 42 {
		t.Fatalf("recorded snapshot = %d, want 42 before prepare", repo.recordedSnapshot)
	}
	if repo.runStatuses["run_root"] != materialize.RunStatusFailed {
		t.Fatalf("root run status = %s, want failed", repo.runStatuses["run_root"])
	}
}

func TestServiceQueueAssetRefreshCreatesDependencyRuns(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	service := Service{
		Deployments: repo,
		Runs:        repo,
		Artifacts:   fakeArtifactLoader{definition: refreshTestDefinition()},
		Publisher:   &fakePublisher{},
	}

	result, err := service.QueueAssetRefresh(ctx, QueueAssetInput{
		WorkspaceID: "sales",
		Environment: deployment.DefaultEnvironment,
		PrincipalID: "principal",
		Asset:       workspace.AssetView{Type: string(workspace.AssetTypeModelTable), Key: "sales.orders"},
	})
	if err != nil {
		t.Fatalf("queue asset refresh: %v", err)
	}
	if result.DeploymentID != "dep_candidate" {
		t.Fatalf("deployment id = %s, want dep_candidate", result.DeploymentID)
	}
	if len(repo.createdRuns) != 2 {
		t.Fatalf("created runs = %#v, want root plus dependency", repo.createdRuns)
	}
	root := repo.createdRuns[0]
	child := repo.createdRuns[1]
	if root.JobKind != materialize.JobKindWorkspaceAssetRefresh || root.TargetID != "sales.orders" || root.ParentRunID != "" {
		t.Fatalf("root run = %#v, want workspace asset root", root)
	}
	if child.JobKind != materialize.JobKindChildRun || child.TargetID != "sales.customers" || child.ParentRunID != result.Run.ID {
		t.Fatalf("child run = %#v, want dependency child", child)
	}
}

func TestServiceCreateRefreshCandidatePersistsResolvedDataRoot(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	service := Service{Deployments: repo}
	active := ServingState{
		Deployment: deployment.Deployment{
			ID:           "dep_active",
			WorkspaceID:  "movielens",
			Environment:  deployment.DefaultEnvironment,
			Digest:       "artifact-digest",
			ManifestJSON: "{}",
		},
		Artifact: deployment.Artifact{
			DeploymentID: "dep_active",
			WorkspaceID:  "movielens",
			Environment:  deployment.DefaultEnvironment,
			Digest:       "artifact-digest",
			Format:       "tar.gz",
			Path:         "/tmp/artifact.tgz",
			DataRoot:     ".data/movielens",
		},
	}

	candidate, err := service.CreateRefreshCandidate(ctx, RefreshCandidateInput{
		WorkspaceID:   "movielens",
		Environment:   deployment.DefaultEnvironment,
		CreatedBy:     "tester",
		Active:        active,
		ArtifactGraph: workspace.AssetGraph{},
	})
	if err != nil {
		t.Fatalf("create refresh candidate: %v", err)
	}

	if repo.savedArtifact.DataRoot != ".data/movielens" {
		t.Fatalf("saved artifact data root = %q, want .data/movielens", repo.savedArtifact.DataRoot)
	}
	if repo.savedValidation.DataRoot != ".data/movielens" {
		t.Fatalf("saved validation data root = %q, want .data/movielens", repo.savedValidation.DataRoot)
	}
	if candidate.Artifact.DataRoot != ".data/movielens" {
		t.Fatalf("candidate artifact data root = %q, want .data/movielens", candidate.Artifact.DataRoot)
	}
}

func refreshTestDefinition() *workspace.Definition {
	return &workspace.Definition{Models: map[string]*semanticmodel.Model{
		"sales": {
			Name: "sales",
			Tables: map[string]semanticmodel.Table{
				"customers": {Kind: "dimension"},
				"orders":    {Kind: "fact", ModelDependencies: []string{"customers"}},
			},
		},
	}}
}

type fakeRepo struct {
	activeDeployment           deployment.Deployment
	activeArtifact             deployment.Artifact
	candidateDeployment        deployment.Deployment
	candidateArtifact          deployment.Artifact
	recordedSnapshotDeployment deployment.ID
	recordedSnapshot           int64
	activatedDeployment        deployment.ID
	failedDeployment           deployment.ID
	runStatuses                map[string]string
	createdRuns                []materialize.RunInput
	savedArtifact              deployment.Artifact
	savedValidation            deployment.Validation
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		activeDeployment: deployment.Deployment{
			ID:           "dep_active",
			WorkspaceID:  "sales",
			Environment:  deployment.DefaultEnvironment,
			Status:       deployment.StatusActive,
			Digest:       "digest",
			ManifestJSON: "{}",
		},
		activeArtifact: deployment.Artifact{
			DeploymentID: "dep_active",
			WorkspaceID:  "sales",
			Environment:  deployment.DefaultEnvironment,
			Digest:       "digest",
			Format:       "tar.gz",
			Path:         "/tmp/artifact.tar.gz",
			DataRoot:     ".data/sales",
			ManifestJSON: "{}",
		},
		candidateDeployment: deployment.Deployment{
			ID:           "dep_candidate",
			WorkspaceID:  "sales",
			Environment:  deployment.DefaultEnvironment,
			Status:       deployment.StatusValidated,
			Digest:       "digest",
			ManifestJSON: "{}",
		},
		candidateArtifact: deployment.Artifact{
			DeploymentID: "dep_candidate",
			WorkspaceID:  "sales",
			Environment:  deployment.DefaultEnvironment,
			Digest:       "digest",
			Format:       "tar.gz",
			Path:         "/tmp/artifact.tar.gz",
			DataRoot:     ".data/sales",
			ManifestJSON: "{}",
		},
		runStatuses: map[string]string{
			"run_root":  materialize.RunStatusRunning,
			"run_child": materialize.RunStatusQueued,
		},
	}
}

func (r *fakeRepo) ActiveArtifact(context.Context, deployment.WorkspaceID, deployment.Environment) (deployment.Deployment, deployment.Artifact, error) {
	return r.activeDeployment, r.activeArtifact, nil
}

func (r *fakeRepo) Create(context.Context, deployment.CreateInput) (deployment.Deployment, error) {
	return deployment.Deployment{ID: "dep_candidate", WorkspaceID: "sales", Environment: deployment.DefaultEnvironment, Status: deployment.StatusPending}, nil
}

func (r *fakeRepo) SaveValidated(_ context.Context, deploymentID deployment.ID, validation deployment.Validation, artifact deployment.Artifact) (deployment.Deployment, error) {
	r.savedValidation = validation
	r.savedArtifact = artifact
	r.candidateDeployment.ID = deploymentID
	r.candidateDeployment.WorkspaceID = artifact.WorkspaceID
	r.candidateDeployment.Environment = artifact.Environment
	r.candidateDeployment.Digest = validation.Digest
	r.candidateArtifact = artifact
	return r.candidateDeployment, nil
}

func (r *fakeRepo) ByID(context.Context, deployment.ID) (deployment.Deployment, error) {
	return r.candidateDeployment, nil
}

func (r *fakeRepo) ArtifactByDeployment(context.Context, deployment.ID) (deployment.Artifact, error) {
	return r.candidateArtifact, nil
}

func (r *fakeRepo) RecordDuckLakeSnapshot(_ context.Context, deploymentID deployment.ID, snapshotID int64) error {
	r.recordedSnapshotDeployment = deploymentID
	r.recordedSnapshot = snapshotID
	r.candidateDeployment.DuckLakeSnapshotID = snapshotID
	return nil
}

func (r *fakeRepo) Activate(_ context.Context, _ deployment.WorkspaceID, _ deployment.Environment, deploymentID deployment.ID) (deployment.Deployment, error) {
	r.activatedDeployment = deploymentID
	return r.candidateDeployment, nil
}

func (r *fakeRepo) MarkFailed(_ context.Context, deploymentID deployment.ID, _ error) error {
	r.failedDeployment = deploymentID
	return nil
}

func (r *fakeRepo) CreateRun(_ context.Context, input materialize.RunInput) (materialize.RunRecord, error) {
	r.createdRuns = append(r.createdRuns, input)
	id := "run_root"
	if input.ParentRunID != "" {
		id = "run_child"
	}
	r.runStatuses[id] = materialize.RunStatusQueued
	return materialize.RunRecord{ID: id, WorkspaceID: input.WorkspaceID, DeploymentID: input.DeploymentID, TargetType: input.TargetType, TargetID: input.TargetID, TriggerType: input.TriggerType, ParentRunID: input.ParentRunID}, nil
}

func (r *fakeRepo) ListChildRuns(context.Context, string, string) ([]materialize.RunRecord, error) {
	return []materialize.RunRecord{{ID: "run_child", WorkspaceID: "sales", TargetType: materialize.TargetModelTable, TargetID: "sales.customers"}}, nil
}

func (r *fakeRepo) MarkRunRunning(_ context.Context, _ string, runID string) (materialize.RunRecord, error) {
	r.runStatuses[runID] = materialize.RunStatusRunning
	return materialize.RunRecord{ID: runID, Status: materialize.RunStatusRunning}, nil
}

func (r *fakeRepo) MarkRunSucceeded(_ context.Context, _ string, runID string) (materialize.RunRecord, error) {
	r.runStatuses[runID] = materialize.RunStatusSucceeded
	return materialize.RunRecord{ID: runID, Status: materialize.RunStatusSucceeded}, nil
}

func (r *fakeRepo) MarkRunFailed(_ context.Context, _ string, runID, _ string) (materialize.RunRecord, error) {
	r.runStatuses[runID] = materialize.RunStatusFailed
	return materialize.RunRecord{ID: runID, Status: materialize.RunStatusFailed}, nil
}

type fakeArtifactLoader struct {
	definition *workspace.Definition
}

func (l fakeArtifactLoader) Load(context.Context, deployment.Artifact) (LoadedArtifact, error) {
	return LoadedArtifact{Definition: l.definition, Graph: workspace.AssetGraph{}}, nil
}

type fakeMaterializer struct {
	snapshotID int64
	err        error
	tables     []string
}

func (m *fakeMaterializer) Materialize(_ context.Context, input MaterializeInput) (int64, error) {
	m.tables = append([]string(nil), input.Plan.Tables...)
	if m.err != nil {
		return 0, m.err
	}
	return m.snapshotID, nil
}

type fakeRuntimeHost struct {
	prepared   bool
	committed  bool
	prepareErr error
}

func (h *fakeRuntimeHost) PrepareDeployment(context.Context, string) (deployment.PreparedRuntime, error) {
	if h.prepareErr != nil {
		return nil, h.prepareErr
	}
	h.prepared = true
	return fakePrepared{}, nil
}

func (h *fakeRuntimeHost) CommitPrepared(deployment.PreparedRuntime) error {
	h.committed = true
	return nil
}

func (h *fakeRuntimeHost) Reload(context.Context) error { return nil }

type fakePrepared struct{}

func (fakePrepared) Close() error { return nil }

type fakeRetention struct {
	ran bool
}

func (r *fakeRetention) Run(context.Context, bool) error {
	r.ran = true
	return nil
}

type fakePublisher struct {
	targets []string
}

func (p *fakePublisher) PublishRefreshTarget(_ context.Context, _, _, targetID string) {
	p.targets = append(p.targets, targetID)
}
