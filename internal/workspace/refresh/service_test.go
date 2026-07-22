package refresh

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/refreshpipeline"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	"github.com/Yacobolo/leapview/internal/workspace"
)

func TestServiceExecuteClaimedJobActivatesAfterMaterializeAndPrepare(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	publisher := &fakePublisher{}
	materializer := &fakeMaterializer{snapshotID: 42}
	runtime := &fakeRuntimeHost{}
	retention := &fakeRetention{}
	service := Service{
		ServingStates: repo,
		Runs:          repo,
		Artifacts:     fakeArtifactLoader{definition: refreshTestDefinition()},
		Materializer:  materializer,
		Runtime:       runtime,
		Retention:     retention,
		Publisher:     publisher,
	}

	err := service.ExecuteClaimedJob(ctx, materialize.JobRecord{
		ID:             "job_1",
		WorkspaceID:    "sales",
		ServingStateID: "dep_candidate",
		RunID:          "run_root",
		ModelID:        "sales",
		TargetType:     materialize.TargetRefreshPipeline,
		TargetID:       "sales.sales-refresh",
		Kind:           materialize.JobKindRefreshPipeline,
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
		ServingStates: repo,
		Runs:          repo,
		Artifacts:     fakeArtifactLoader{definition: refreshTestDefinition()},
		Materializer:  &fakeMaterializer{err: errors.New("materialize failed")},
		Runtime:       &fakeRuntimeHost{},
	}

	err := service.ExecuteClaimedJob(ctx, materialize.JobRecord{
		ID:             "job_1",
		WorkspaceID:    "sales",
		ServingStateID: "dep_candidate",
		RunID:          "run_root",
		ModelID:        "sales",
		TargetType:     materialize.TargetRefreshPipeline,
		TargetID:       "sales.sales-refresh",
		Kind:           materialize.JobKindRefreshPipeline,
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
		ServingStates: repo,
		Runs:          repo,
		Artifacts:     fakeArtifactLoader{definition: refreshTestDefinition()},
		Materializer:  &fakeMaterializer{snapshotID: 42},
		Runtime:       &fakeRuntimeHost{prepareErr: errors.New("prepare failed")},
	}

	err := service.ExecuteClaimedJob(ctx, materialize.JobRecord{
		ID:             "job_1",
		WorkspaceID:    "sales",
		ServingStateID: "dep_candidate",
		RunID:          "run_root",
		ModelID:        "sales",
		TargetType:     materialize.TargetRefreshPipeline,
		TargetID:       "sales.sales-refresh",
		Kind:           materialize.JobKindRefreshPipeline,
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

func TestServiceExecuteClaimedJobRuntimeActivationFailureDoesNotPublishOrActivate(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	wantErr := errors.New("runtime activation failed")
	runtime := &fakeRuntimeHost{activateErr: wantErr}
	service := Service{
		ServingStates: repo,
		Runs:          repo,
		Artifacts:     fakeArtifactLoader{definition: refreshTestDefinition()},
		Materializer:  &fakeMaterializer{snapshotID: 42},
		Runtime:       runtime,
	}

	err := service.ExecuteClaimedJob(ctx, materialize.JobRecord{
		ID: "job_1", WorkspaceID: "sales", ServingStateID: "dep_candidate", RunID: "run_root", ModelID: "sales",
		TargetType: materialize.TargetRefreshPipeline, TargetID: "sales.sales-refresh", Kind: materialize.JobKindRefreshPipeline,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("execute claimed job error = %v, want %v", err, wantErr)
	}
	if repo.activatedDeployment != "" {
		t.Fatalf("activated deployment = %s, want none", repo.activatedDeployment)
	}
	if runtime.committed {
		t.Fatal("runtime was published after atomic activation failed")
	}
	if repo.failedDeployment != "dep_candidate" {
		t.Fatalf("failed deployment = %s, want dep_candidate", repo.failedDeployment)
	}
}

func TestServiceExecuteClaimedJobRequiresAtomicRuntimeHost(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	service := Service{
		ServingStates: repo,
		Runs:          repo,
		Artifacts:     fakeArtifactLoader{definition: refreshTestDefinition()},
		Materializer:  &fakeMaterializer{snapshotID: 42},
	}

	err := service.ExecuteClaimedJob(ctx, materialize.JobRecord{
		ID: "job_1", WorkspaceID: "sales", ServingStateID: "dep_candidate", RunID: "run_root", ModelID: "sales",
		TargetType: materialize.TargetRefreshPipeline, TargetID: "sales.sales-refresh", Kind: materialize.JobKindRefreshPipeline,
	})
	if err == nil || !strings.Contains(err.Error(), "runtime host is required") {
		t.Fatalf("execute claimed job error = %v, want required runtime host", err)
	}
	if repo.activatedDeployment != "" {
		t.Fatalf("activated deployment = %s, want none", repo.activatedDeployment)
	}
}

func TestServiceQueuePipelineRefreshCreatesFullSemanticModelRun(t *testing.T) {
	repo := newFakeRepo()
	service := Service{
		ServingStates: repo,
		Runs:          repo,
		Artifacts:     fakeArtifactLoader{definition: refreshTestDefinition()},
	}
	result, err := service.QueuePipelineRefresh(t.Context(), QueuePipelineInput{
		WorkspaceID: "sales", Environment: servingstate.DefaultEnvironment, PrincipalID: "principal",
		PipelineID: "sales-refresh", TriggerType: materialize.TriggerManual,
	})
	if err != nil {
		t.Fatalf("QueuePipelineRefresh() error = %v", err)
	}
	if result.Run.TargetType != materialize.TargetRefreshPipeline || result.Run.TargetID != "sales.sales-refresh" {
		t.Fatalf("root run = %#v", result.Run)
	}
	if len(repo.createdRuns) != 3 {
		t.Fatalf("created runs = %#v, want pipeline root plus both model-table tasks", repo.createdRuns)
	}
	if repo.createdRuns[0].ModelID != "sales" || repo.createdRuns[0].TriggerType != materialize.TriggerManual {
		t.Fatalf("root input = %#v", repo.createdRuns[0])
	}
}

func TestServiceQueuePipelineRefreshPinsCandidateManagedDataRevisions(t *testing.T) {
	repo := newFakeRepo()
	hook := &fakeCandidateValidationHook{}
	service := Service{
		ServingStates: repo,
		Runs:          repo,
		Artifacts: fakeArtifactLoader{
			definition:           refreshTestDefinition(),
			managedDataRevisions: map[string]string{"olist": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		},
		CandidateValidationHooks: []CandidateValidationHook{hook},
	}

	_, err := service.QueuePipelineRefresh(t.Context(), QueuePipelineInput{
		WorkspaceID: "sales", Environment: servingstate.DefaultEnvironment, PrincipalID: "principal",
		PipelineID: "sales-refresh", TriggerType: materialize.TriggerManual,
	})
	if err != nil {
		t.Fatalf("QueuePipelineRefresh() error = %v", err)
	}
	want := map[string]string{"olist": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if hook.candidate.ID != "dep_candidate" || !reflect.DeepEqual(hook.validation.ManagedDataRevisions, want) {
		t.Fatalf("candidate hook = (%q, %#v), want dep_candidate and %#v", hook.candidate.ID, hook.validation.ManagedDataRevisions, want)
	}
	if !reflect.DeepEqual(repo.savedValidation.ManagedDataRevisions, want) {
		t.Fatalf("saved managed-data revisions = %#v, want %#v", repo.savedValidation.ManagedDataRevisions, want)
	}
}

func TestServiceQueuePipelineRefreshFailsCandidateWhenManagedDataPinningFails(t *testing.T) {
	repo := newFakeRepo()
	service := Service{
		ServingStates: repo,
		Runs:          repo,
		Artifacts: fakeArtifactLoader{
			definition:           refreshTestDefinition(),
			managedDataRevisions: map[string]string{"olist": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		},
		CandidateValidationHooks: []CandidateValidationHook{&fakeCandidateValidationHook{err: errors.New("pin failed")}},
	}

	_, err := service.QueuePipelineRefresh(t.Context(), QueuePipelineInput{
		WorkspaceID: "sales", Environment: servingstate.DefaultEnvironment,
		PipelineID: "sales-refresh", TriggerType: materialize.TriggerManual,
	})
	if err == nil || !strings.Contains(err.Error(), "pin failed") {
		t.Fatalf("QueuePipelineRefresh() error = %v, want pin failure", err)
	}
	if repo.failedDeployment != "dep_candidate" || len(repo.createdRuns) != 0 {
		t.Fatalf("failed candidate = %q created runs = %#v, want failed candidate and no runs", repo.failedDeployment, repo.createdRuns)
	}
}

func TestServiceQueuePipelineRefreshRejectsSupersededScheduledArtifact(t *testing.T) {
	repo := newFakeRepo()
	service := Service{
		ServingStates: repo,
		Runs:          repo,
		Artifacts:     fakeArtifactLoader{definition: refreshTestDefinition()},
	}
	_, err := service.QueuePipelineRefresh(t.Context(), QueuePipelineInput{
		WorkspaceID: "sales", Environment: servingstate.DefaultEnvironment,
		PipelineID: "sales-refresh", TriggerType: materialize.TriggerSchedule,
		ArtifactDigest: "sha256:superseded",
	})
	if err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("QueuePipelineRefresh() error = %v, want superseded artifact", err)
	}
	if len(repo.createdRuns) != 0 {
		t.Fatalf("created runs = %#v, want none", repo.createdRuns)
	}
}

func TestServiceCreateRefreshCandidateCopiesActiveArtifactMetadata(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	service := Service{ServingStates: repo}
	active := ServingState{
		State: servingstate.State{
			ID:                "dep_active",
			WorkspaceID:       "movielens",
			ProjectID:         "movie-project",
			ProjectDigest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ProjectWorkspaces: []string{"movielens", "ratings"},
			AccessPolicyJSON:  `{"groups":{"readers":{"name":"Readers"}}}`,
			Environment:       servingstate.DefaultEnvironment,
			Digest:            "artifact-digest",
			ManifestJSON:      "{}",
		},
		Artifact: servingstate.Artifact{
			ServingStateID: "dep_active",
			WorkspaceID:    "movielens",
			Environment:    servingstate.DefaultEnvironment,
			Digest:         "artifact-digest",
			Format:         "tar.gz",
			Path:           "/tmp/artifact.tgz",
		},
	}

	candidate, err := service.CreateRefreshCandidate(ctx, RefreshCandidateInput{
		WorkspaceID:   "movielens",
		Environment:   servingstate.DefaultEnvironment,
		CreatedBy:     "tester",
		Active:        active,
		ArtifactGraph: workspace.AssetGraph{},
	})
	if err != nil {
		t.Fatalf("create refresh candidate: %v", err)
	}

	if repo.savedArtifact.Path != active.Artifact.Path || candidate.Artifact.Path != active.Artifact.Path {
		t.Fatalf("candidate artifact path = %q, want %q", candidate.Artifact.Path, active.Artifact.Path)
	}
	if repo.savedValidation.ProjectID != active.State.ProjectID {
		t.Fatalf("candidate project = %q, want %q", repo.savedValidation.ProjectID, active.State.ProjectID)
	}
	if repo.savedValidation.ProjectDigest != active.State.ProjectDigest || !reflect.DeepEqual(repo.savedValidation.ProjectWorkspaces, active.State.ProjectWorkspaces) {
		t.Fatalf("candidate project provenance = (%q, %#v), want (%q, %#v)", repo.savedValidation.ProjectDigest, repo.savedValidation.ProjectWorkspaces, active.State.ProjectDigest, active.State.ProjectWorkspaces)
	}
	if group := repo.savedValidation.AccessPolicy.Groups["readers"]; group.Name != "Readers" {
		t.Fatalf("candidate access policy = %#v, want active policy", repo.savedValidation.AccessPolicy)
	}
}

func refreshTestDefinition() *workspace.Definition {
	return &workspace.Definition{RefreshPipelines: map[string]refreshpipeline.Definition{
		"sales-refresh": {ID: "sales-refresh", Name: "sales-refresh", SemanticModel: "sales"},
	}, Models: map[string]*semanticmodel.Model{
		"sales": {
			Name: "sales",
			Tables: map[string]semanticmodel.Table{
				"customers": {},
				"orders":    {ModelDependencies: []string{"customers"}},
			},
		},
	}}
}

type fakeRepo struct {
	activeDeployment           servingstate.State
	activeArtifact             servingstate.Artifact
	candidateState             servingstate.State
	candidateArtifact          servingstate.Artifact
	recordedSnapshotDeployment servingstate.ID
	recordedSnapshot           int64
	activatedDeployment        servingstate.ID
	failedDeployment           servingstate.ID
	runStatuses                map[string]string
	createdRuns                []materialize.RunInput
	savedArtifact              servingstate.Artifact
	savedValidation            servingstate.Validation
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		activeDeployment: servingstate.State{
			ID:                "dep_active",
			WorkspaceID:       "sales",
			ProjectID:         "project",
			ProjectDigest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ProjectWorkspaces: []string{"sales"},
			AccessPolicyJSON:  "{}",
			Environment:       servingstate.DefaultEnvironment,
			Status:            servingstate.StatusActive,
			Digest:            "digest",
			ManifestJSON:      "{}",
		},
		activeArtifact: servingstate.Artifact{
			ServingStateID: "dep_active",
			WorkspaceID:    "sales",
			Environment:    servingstate.DefaultEnvironment,
			Digest:         "digest",
			Format:         "tar.gz",
			Path:           "/tmp/artifact.tar.gz",
			ManifestJSON:   "{}",
		},
		candidateState: servingstate.State{
			ID:           "dep_candidate",
			WorkspaceID:  "sales",
			Environment:  servingstate.DefaultEnvironment,
			Status:       servingstate.StatusValidated,
			Digest:       "digest",
			ManifestJSON: "{}",
		},
		candidateArtifact: servingstate.Artifact{
			ServingStateID: "dep_candidate",
			WorkspaceID:    "sales",
			Environment:    servingstate.DefaultEnvironment,
			Digest:         "digest",
			Format:         "tar.gz",
			Path:           "/tmp/artifact.tar.gz",
			ManifestJSON:   "{}",
		},
		runStatuses: map[string]string{
			"run_root":  materialize.RunStatusRunning,
			"run_child": materialize.RunStatusQueued,
		},
	}
}

func (r *fakeRepo) ActiveArtifact(context.Context, servingstate.WorkspaceID, servingstate.Environment) (servingstate.State, servingstate.Artifact, error) {
	return r.activeDeployment, r.activeArtifact, nil
}

func (r *fakeRepo) Create(context.Context, servingstate.CreateInput) (servingstate.State, error) {
	return servingstate.State{ID: "dep_candidate", WorkspaceID: "sales", Environment: servingstate.DefaultEnvironment, Status: servingstate.StatusPending}, nil
}

func (r *fakeRepo) SaveValidated(_ context.Context, servingStateID servingstate.ID, validation servingstate.Validation, artifact servingstate.Artifact) (servingstate.State, error) {
	r.savedValidation = validation
	r.savedArtifact = artifact
	r.candidateState.ID = servingStateID
	r.candidateState.WorkspaceID = artifact.WorkspaceID
	r.candidateState.Environment = artifact.Environment
	r.candidateState.Digest = validation.Digest
	r.candidateArtifact = artifact
	return r.candidateState, nil
}

func (r *fakeRepo) ByID(context.Context, servingstate.ID) (servingstate.State, error) {
	return r.candidateState, nil
}

func (r *fakeRepo) ArtifactByServingState(context.Context, servingstate.ID) (servingstate.Artifact, error) {
	return r.candidateArtifact, nil
}

func (r *fakeRepo) RecordDuckLakeSnapshot(_ context.Context, servingStateID servingstate.ID, snapshotID int64) error {
	r.recordedSnapshotDeployment = servingStateID
	r.recordedSnapshot = snapshotID
	r.candidateState.DuckLakeSnapshotID = snapshotID
	return nil
}

func (r *fakeRepo) Activate(_ context.Context, _ servingstate.WorkspaceID, _ servingstate.Environment, servingStateID servingstate.ID) (servingstate.State, error) {
	r.activatedDeployment = servingStateID
	return r.candidateState, nil
}

func (r *fakeRepo) MarkFailed(_ context.Context, servingStateID servingstate.ID, _ error) error {
	r.failedDeployment = servingStateID
	return nil
}

func (r *fakeRepo) CreateRun(_ context.Context, input materialize.RunInput) (materialize.RunRecord, error) {
	r.createdRuns = append(r.createdRuns, input)
	id := "run_root"
	if input.ParentRunID != "" {
		id = "run_child"
	}
	r.runStatuses[id] = materialize.RunStatusQueued
	return materialize.RunRecord{ID: id, WorkspaceID: input.WorkspaceID, ServingStateID: input.ServingStateID, TargetType: input.TargetType, TargetID: input.TargetID, TriggerType: input.TriggerType, ParentRunID: input.ParentRunID}, nil
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
	definition           *workspace.Definition
	managedDataRevisions map[string]string
}

func (l fakeArtifactLoader) Load(context.Context, servingstate.Artifact) (LoadedArtifact, error) {
	return LoadedArtifact{Definition: l.definition, Graph: workspace.AssetGraph{}, ManagedDataRevisions: l.managedDataRevisions}, nil
}

type fakeCandidateValidationHook struct {
	candidate  servingstate.State
	validation servingstate.Validation
	err        error
}

func (h *fakeCandidateValidationHook) AfterArtifactValidation(_ context.Context, candidate servingstate.State, validation servingstate.Validation) error {
	h.candidate = candidate
	h.validation = validation
	return h.err
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
	prepared    bool
	committed   bool
	prepareErr  error
	activateErr error
}

func (h *fakeRuntimeHost) PrepareServingState(context.Context, string) (servingstate.PreparedRuntime, error) {
	if h.prepareErr != nil {
		return nil, h.prepareErr
	}
	h.prepared = true
	return fakePrepared{}, nil
}

func (h *fakeRuntimeHost) ActivatePrepared(_ servingstate.PreparedRuntime, activate func() error) error {
	if h.activateErr != nil {
		return h.activateErr
	}
	if err := activate(); err != nil {
		return err
	}
	h.committed = true
	return nil
}

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

func (p *fakePublisher) PublishRefreshTarget(_ context.Context, _, _, _, targetID string) {
	p.targets = append(p.targets, targetID)
}
