package validate

import (
	"context"

	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

type Repository interface {
	ByID(ctx context.Context, id servingstate.ID) (servingstate.State, error)
	MarkFailed(ctx context.Context, servingStateID servingstate.ID, cause error) error
	SaveValidated(ctx context.Context, servingStateID servingstate.ID, validation servingstate.Validation, artifact servingstate.Artifact) (servingstate.State, error)
}

type ArtifactStore interface {
	UploadPath(servingStateID servingstate.ID) string
	PromoteUploaded(ctx context.Context, servingStateID servingstate.ID, digest, manifestJSON string) (servingstate.Artifact, error)
}

type Validator interface {
	ValidateArtifact(path string, workspaceID servingstate.WorkspaceID, environment servingstate.Environment, servingStateID servingstate.ID) (servingstate.Validation, error)
	Cleanup(validation servingstate.Validation) error
}

type Hook interface {
	AfterArtifactValidation(ctx context.Context, candidate servingstate.State, validation servingstate.Validation) error
}

type Service struct {
	repo      Repository
	artifacts ArtifactStore
	validator Validator
	hooks     []Hook
}

func NewService(repo Repository, artifacts ArtifactStore, validator Validator, hooks ...Hook) Service {
	return Service{repo: repo, artifacts: artifacts, validator: validator, hooks: append([]Hook(nil), hooks...)}
}

func (s Service) Validate(ctx context.Context, servingStateID servingstate.ID) (servingstate.State, error) {
	current, err := s.repo.ByID(ctx, servingStateID)
	if err != nil {
		return servingstate.State{}, err
	}
	validation, err := s.validator.ValidateArtifact(s.artifacts.UploadPath(current.ID), current.WorkspaceID, current.Environment, current.ID)
	if err != nil {
		_ = s.repo.MarkFailed(ctx, current.ID, err)
		return servingstate.State{}, err
	}
	defer func() { _ = s.validator.Cleanup(validation) }()
	for _, hook := range s.hooks {
		if hook == nil {
			continue
		}
		if err := hook.AfterArtifactValidation(ctx, current, validation); err != nil {
			_ = s.repo.MarkFailed(ctx, current.ID, err)
			return servingstate.State{}, err
		}
	}

	artifact, err := s.artifacts.PromoteUploaded(ctx, current.ID, validation.Digest, validation.ManifestJSON)
	if err != nil {
		return servingstate.State{}, err
	}
	artifact.WorkspaceID = current.WorkspaceID
	artifact.Environment = current.Environment
	return s.repo.SaveValidated(ctx, current.ID, validation, artifact)
}
