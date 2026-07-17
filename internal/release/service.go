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

type PinValidator interface {
	ValidateServingStatePins(context.Context, string, string, map[string]string) error
}

type Service struct {
	releases    Repository
	states      ServingStateRepository
	workspaces  WorkspaceRepository
	artifacts   ArtifactStore
	validator   ArtifactValidator
	pins        PinValidator
	environment servingstate.Environment
}

type ServiceOptions struct {
	Releases    Repository
	States      ServingStateRepository
	Workspaces  WorkspaceRepository
	Artifacts   ArtifactStore
	Validator   ArtifactValidator
	Pins        PinValidator
	Environment servingstate.Environment
}

func NewService(options ServiceOptions) (*Service, error) {
	if options.Releases == nil || options.States == nil || options.Workspaces == nil || options.Artifacts == nil || options.Validator == nil {
		return nil, fmt.Errorf("release repositories, artifact store, and validator are required")
	}
	return &Service{releases: options.Releases, states: options.States, workspaces: options.Workspaces, artifacts: options.Artifacts, validator: options.Validator, pins: options.Pins, environment: servingstate.NormalizeEnvironment(options.Environment)}, nil
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
	expectedPins := make(map[string]string, len(current.Manifest.Connections))
	for _, pin := range current.Manifest.Connections {
		if pin.ConnectionID == "" || pin.RevisionID == "" {
			return s.failFinalization(ctx, current, ErrInvalid)
		}
		if _, duplicate := expectedPins[pin.ConnectionID]; duplicate {
			return s.failFinalization(ctx, current, ErrInvalid)
		}
		expectedPins[pin.ConnectionID] = pin.RevisionID
	}
	if len(expectedPins) > 0 && s.pins == nil {
		return s.failFinalization(ctx, current, fmt.Errorf("%w: managed-data pin validation is unavailable", ErrConflict))
	}
	for _, artifact := range current.Artifacts {
		state, validateErr := s.validator.Validate(ctx, servingstate.ID(artifact.ServingStateID))
		if validateErr != nil {
			return s.failFinalization(ctx, current, validateErr)
		}
		if state.ProjectID != current.ProjectID || state.ProjectDigest != current.ProjectDigest || state.Digest != artifact.ExpectedDigest {
			mismatch := fmt.Errorf("%w: artifact %q does not match the release manifest", ErrConflict, artifact.WorkspaceID)
			return s.failFinalization(ctx, current, mismatch)
		}
		if s.pins != nil {
			if pinErr := s.pins.ValidateServingStatePins(ctx, artifact.ServingStateID, current.ProjectID, expectedPins); pinErr != nil {
				return s.failFinalization(ctx, current, pinErr)
			}
		}
		digests[artifact.WorkspaceID] = state.Digest
	}
	return s.releases.CompleteFinalization(ctx, projectID, releaseID, digests)
}

func (s *Service) failFinalization(ctx context.Context, current Release, cause error) (Release, error) {
	failed, failErr := s.releases.FailFinalization(ctx, current.ProjectID, current.ID, cause)
	if failErr != nil {
		return Release{}, errorsJoin(cause, failErr)
	}
	return failed, cause
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
