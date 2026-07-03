package app

import (
	"context"
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestServingStateRefreshCandidatePersistsResolvedDataRoot(t *testing.T) {
	ctx := context.Background()
	repo := &servingStateRepo{}
	service := newServingStateService(repo)
	active := servingState{
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

	candidate, err := service.CreateRefreshCandidate(ctx, servingRefreshCandidateInput{
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

type servingStateRepo struct {
	savedArtifact   deployment.Artifact
	savedValidation deployment.Validation
}

func (r *servingStateRepo) ActiveArtifact(context.Context, deployment.WorkspaceID, deployment.Environment) (deployment.Deployment, deployment.Artifact, error) {
	return deployment.Deployment{}, deployment.Artifact{}, deployment.ErrNotFound
}

func (r *servingStateRepo) Create(_ context.Context, input deployment.CreateInput) (deployment.Deployment, error) {
	return deployment.Deployment{
		ID:          "dep_candidate",
		WorkspaceID: input.WorkspaceID,
		Environment: input.Environment,
		Status:      deployment.StatusPending,
	}, nil
}

func (r *servingStateRepo) SaveValidated(_ context.Context, deploymentID deployment.ID, validation deployment.Validation, artifact deployment.Artifact) (deployment.Deployment, error) {
	r.savedValidation = validation
	r.savedArtifact = artifact
	return deployment.Deployment{
		ID:          deploymentID,
		WorkspaceID: artifact.WorkspaceID,
		Environment: artifact.Environment,
		Status:      deployment.StatusValidated,
		Digest:      validation.Digest,
	}, nil
}

func (r *servingStateRepo) ByID(context.Context, deployment.ID) (deployment.Deployment, error) {
	return deployment.Deployment{}, deployment.ErrNotFound
}

func (r *servingStateRepo) ArtifactByDeployment(context.Context, deployment.ID) (deployment.Artifact, error) {
	return deployment.Artifact{}, deployment.ErrNotFound
}

func (r *servingStateRepo) MarkFailed(context.Context, deployment.ID, error) error {
	return nil
}

func (r *servingStateRepo) RecordDuckLakeSnapshot(context.Context, deployment.ID, int64) error {
	return nil
}

func (r *servingStateRepo) Activate(context.Context, deployment.WorkspaceID, deployment.Environment, deployment.ID) (deployment.Deployment, error) {
	return deployment.Deployment{}, nil
}

func (r *servingStateRepo) ActivateWithWorkspacePolicy(context.Context, deployment.WorkspaceID, deployment.Environment, deployment.ID, workspace.AccessPolicy) (deployment.Deployment, error) {
	return deployment.Deployment{}, nil
}

func (r *servingStateRepo) List(context.Context, deployment.WorkspaceID, deployment.Environment) ([]deployment.Deployment, error) {
	return nil, nil
}
