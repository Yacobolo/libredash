package validate

import (
	"context"

	"github.com/Yacobolo/libredash/internal/deployment"
)

type Repository interface {
	ByID(ctx context.Context, id deployment.ID) (deployment.Deployment, error)
	MarkFailed(ctx context.Context, deploymentID deployment.ID, cause error) error
	SaveValidated(ctx context.Context, deploymentID deployment.ID, validation deployment.Validation, artifact deployment.Artifact) (deployment.Deployment, error)
}

type ArtifactStore interface {
	UploadPath(deploymentID deployment.ID) string
	PromoteUploaded(ctx context.Context, deploymentID deployment.ID, digest, manifestJSON string) (deployment.Artifact, error)
}

type Validator interface {
	ValidateArtifact(path string, workspaceID deployment.WorkspaceID, environment deployment.Environment, deploymentID deployment.ID) (deployment.Validation, error)
	Cleanup(validation deployment.Validation) error
}

type Service struct {
	repo      Repository
	artifacts ArtifactStore
	validator Validator
}

func NewService(repo Repository, artifacts ArtifactStore, validator Validator) Service {
	return Service{repo: repo, artifacts: artifacts, validator: validator}
}

func (s Service) Validate(ctx context.Context, deploymentID deployment.ID) (deployment.Deployment, error) {
	current, err := s.repo.ByID(ctx, deploymentID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	validation, err := s.validator.ValidateArtifact(s.artifacts.UploadPath(current.ID), current.WorkspaceID, current.Environment, current.ID)
	if err != nil {
		_ = s.repo.MarkFailed(ctx, current.ID, err)
		return deployment.Deployment{}, err
	}
	defer func() { _ = s.validator.Cleanup(validation) }()

	artifact, err := s.artifacts.PromoteUploaded(ctx, current.ID, validation.Digest, validation.ManifestJSON)
	if err != nil {
		return deployment.Deployment{}, err
	}
	artifact.WorkspaceID = current.WorkspaceID
	artifact.Environment = current.Environment
	return s.repo.SaveValidated(ctx, current.ID, validation, artifact)
}
