package release

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Repository interface {
	Create(context.Context, CreateInput) (Release, error)
	Get(context.Context, string, string) (Release, error)
	List(context.Context, string) ([]Release, error)
	AssignArtifactTarget(context.Context, string, string, string, string) error
	RecordArtifact(context.Context, Artifact) error
	BeginFinalization(context.Context, string, string) (Release, error)
	CompleteFinalization(context.Context, string, string, map[string]string) (Release, error)
	FailFinalization(context.Context, string, string, error) (Release, error)
}

type ServingStateRepository interface {
	Create(context.Context, servingstate.CreateInput) (servingstate.State, error)
}

type WorkspaceRepository interface {
	Ensure(context.Context, workspace.EnsureInput) error
}

type ArtifactStore interface {
	SaveUpload(context.Context, servingstate.ID, io.Reader) (int64, error)
}

type ArtifactValidator interface {
	Validate(context.Context, servingstate.ID) (servingstate.State, error)
}

type Service struct {
	releases    Repository
	states      ServingStateRepository
	workspaces  WorkspaceRepository
	artifacts   ArtifactStore
	validator   ArtifactValidator
	environment servingstate.Environment
}

type ServiceOptions struct {
	Releases    Repository
	States      ServingStateRepository
	Workspaces  WorkspaceRepository
	Artifacts   ArtifactStore
	Validator   ArtifactValidator
	Environment servingstate.Environment
}

func NewService(options ServiceOptions) (*Service, error) {
	if options.Releases == nil || options.States == nil || options.Workspaces == nil || options.Artifacts == nil || options.Validator == nil {
		return nil, fmt.Errorf("release repositories, artifact store, and validator are required")
	}
	return &Service{releases: options.Releases, states: options.States, workspaces: options.Workspaces, artifacts: options.Artifacts, validator: options.Validator, environment: servingstate.NormalizeEnvironment(options.Environment)}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Release, error) {
	input.ID = stableID("rel", input.ProjectID, input.IdempotencyKey)
	manifest := Manifest{Workspaces: input.Workspaces, Connections: input.Connections}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return Release{}, err
	}
	sum := sha256.Sum256(encoded)
	input.RequestDigest = "sha256:" + hex.EncodeToString(sum[:])
	created, err := s.releases.Create(ctx, input)
	if err != nil {
		return Release{}, err
	}
	for _, artifact := range created.Artifacts {
		if artifact.ServingStateID != "" {
			continue
		}
		if err := s.workspaces.Ensure(ctx, workspace.EnsureInput{ID: workspace.WorkspaceID(artifact.WorkspaceID), Title: artifact.WorkspaceID}); err != nil {
			return Release{}, err
		}
		state, err := s.states.Create(ctx, servingstate.CreateInput{WorkspaceID: servingstate.WorkspaceID(artifact.WorkspaceID), ProjectID: created.ProjectID, Environment: s.environment, CreatedBy: created.CreatedBy})
		if err != nil {
			return Release{}, err
		}
		if err := s.releases.AssignArtifactTarget(ctx, created.ProjectID, created.ID, artifact.WorkspaceID, string(state.ID)); err != nil {
			return Release{}, err
		}
	}
	return s.releases.Get(ctx, created.ProjectID, created.ID)
}

func (s *Service) Get(ctx context.Context, projectID, releaseID string) (Release, error) {
	return s.releases.Get(ctx, projectID, releaseID)
}

func (s *Service) List(ctx context.Context, projectID string) ([]Release, error) {
	return s.releases.List(ctx, projectID)
}

func (s *Service) UploadArtifact(ctx context.Context, projectID, releaseID, workspaceID, contentDigest string, source io.Reader) (Artifact, error) {
	current, err := s.releases.Get(ctx, projectID, releaseID)
	if err != nil {
		return Artifact{}, err
	}
	if current.Status != StatusDraft {
		return Artifact{}, ErrImmutable
	}
	var target Artifact
	found := false
	for _, artifact := range current.Artifacts {
		if artifact.WorkspaceID == workspaceID {
			target, found = artifact, true
			break
		}
	}
	if !found || target.ServingStateID == "" {
		return Artifact{}, ErrNotFound
	}
	hash := sha256.New()
	size, err := s.artifacts.SaveUpload(ctx, servingstate.ID(target.ServingStateID), io.TeeReader(source, hash))
	if err != nil {
		return Artifact{}, err
	}
	actualContentDigest := "sha-256=:" + base64.StdEncoding.EncodeToString(hash.Sum(nil)) + ":"
	if strings.TrimSpace(contentDigest) != actualContentDigest {
		return Artifact{}, ErrDigest
	}
	target.SizeBytes = size
	if err := s.releases.RecordArtifact(ctx, target); err != nil {
		return Artifact{}, err
	}
	return target, nil
}

func (s *Service) Finalize(ctx context.Context, projectID, releaseID string) (Release, error) {
	if _, err := s.BeginFinalization(ctx, projectID, releaseID); err != nil {
		return Release{}, err
	}
	return s.ValidateFinalization(ctx, projectID, releaseID)
}

func (s *Service) BeginFinalization(ctx context.Context, projectID, releaseID string) (Release, error) {
	return s.releases.BeginFinalization(ctx, projectID, releaseID)
}

func (s *Service) ValidateFinalization(ctx context.Context, projectID, releaseID string) (Release, error) {
	current, err := s.releases.Get(ctx, projectID, releaseID)
	if err != nil {
		return Release{}, err
	}
	if current.Status != StatusValidating {
		return Release{}, ErrConflict
	}
	digests := make(map[string]string, len(current.Artifacts))
	for _, artifact := range current.Artifacts {
		state, validateErr := s.validator.Validate(ctx, servingstate.ID(artifact.ServingStateID))
		if validateErr != nil {
			failed, failErr := s.releases.FailFinalization(ctx, projectID, releaseID, validateErr)
			if failErr != nil {
				return Release{}, errorsJoin(validateErr, failErr)
			}
			return failed, validateErr
		}
		if state.ProjectID != current.ProjectID || state.ProjectDigest != current.ProjectDigest || state.Digest != artifact.ExpectedDigest {
			mismatch := fmt.Errorf("%w: artifact %q does not match the release manifest", ErrConflict, artifact.WorkspaceID)
			failed, failErr := s.releases.FailFinalization(ctx, projectID, releaseID, mismatch)
			if failErr != nil {
				return Release{}, errorsJoin(mismatch, failErr)
			}
			return failed, mismatch
		}
		digests[artifact.WorkspaceID] = state.Digest
	}
	return s.releases.CompleteFinalization(ctx, projectID, releaseID, digests)
}

func stableID(prefix string, values ...string) string {
	hash := sha256.New()
	for _, value := range values {
		_, _ = hash.Write([]byte(strings.TrimSpace(value)))
		_, _ = hash.Write([]byte{0})
	}
	return prefix + "_" + hex.EncodeToString(hash.Sum(nil))[:24]
}

func errorsJoin(primary, secondary error) error {
	return fmt.Errorf("%v; persist failure: %w", primary, secondary)
}
