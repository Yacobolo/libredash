package activate

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Yacobolo/libredash/internal/access"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
)

var ErrInvalidStatus = errors.New("serving state cannot be activated")

type Repository interface {
	ByID(ctx context.Context, id servingstate.ID) (servingstate.State, error)
	RecordDuckLakeSnapshot(ctx context.Context, servingStateID servingstate.ID, snapshotID int64) error
	Activate(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment, servingStateID servingstate.ID) (servingstate.State, error)
	ActivateWithWorkspacePolicy(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment, servingStateID servingstate.ID, policy workspace.AccessPolicy) (servingstate.State, error)
}

type ArtifactRepository interface {
	ArtifactByServingState(ctx context.Context, servingStateID servingstate.ID) (servingstate.Artifact, error)
}

type RuntimeHost interface {
	PrepareServingState(ctx context.Context, servingStateID string) (servingstate.PreparedRuntime, error)
	CommitPrepared(prepared servingstate.PreparedRuntime) error
}

type preparedDuckLakeSnapshot interface {
	DuckLakeSnapshotID() int64
}

type Service struct {
	repo      Repository
	runtime   RuntimeHost
	artifacts ArtifactRepository
	access    access.WorkspacePolicyReconciler
}

func NewService(repo Repository, runtime RuntimeHost) Service {
	return Service{repo: repo, runtime: runtime}
}

func NewServiceWithAccess(repo Repository, runtime RuntimeHost, artifacts ArtifactRepository, accessReconciler access.WorkspacePolicyReconciler) Service {
	return Service{repo: repo, runtime: runtime, artifacts: artifacts, access: accessReconciler}
}

func (s Service) Activate(ctx context.Context, servingStateID servingstate.ID) (servingstate.State, error) {
	current, err := s.repo.ByID(ctx, servingStateID)
	if err != nil {
		return servingstate.State{}, err
	}
	if !current.CanActivate() {
		return servingstate.State{}, fmt.Errorf("%w: serving state %s has status %q, want validated", ErrInvalidStatus, servingStateID, current.Status)
	}

	var policy *workspace.AccessPolicy
	if s.access != nil && s.artifacts != nil {
		loaded, err := s.accessPolicy(ctx, current)
		if err != nil {
			return servingstate.State{}, err
		}
		policy = &loaded
	}
	var prepared servingstate.PreparedRuntime
	if s.runtime != nil {
		prepared, err = s.runtime.PrepareServingState(ctx, string(servingStateID))
		if err != nil {
			return servingstate.State{}, err
		}
	}
	if snapshot, ok := prepared.(preparedDuckLakeSnapshot); ok && snapshot.DuckLakeSnapshotID() > 0 {
		if err := s.repo.RecordDuckLakeSnapshot(ctx, current.ID, snapshot.DuckLakeSnapshotID()); err != nil {
			if prepared != nil {
				_ = prepared.Close()
			}
			return servingstate.State{}, err
		}
	}

	var activated servingstate.State
	if policy != nil {
		activated, err = s.repo.ActivateWithWorkspacePolicy(ctx, current.WorkspaceID, current.Environment, current.ID, *policy)
	} else {
		activated, err = s.repo.Activate(ctx, current.WorkspaceID, current.Environment, current.ID)
	}
	if err != nil {
		if prepared != nil {
			_ = prepared.Close()
		}
		return servingstate.State{}, err
	}
	if prepared != nil {
		if err := s.runtime.CommitPrepared(prepared); err != nil {
			_ = prepared.Close()
			return servingstate.State{}, err
		}
	}
	return activated, nil
}

func (s Service) accessPolicy(ctx context.Context, current servingstate.State) (workspace.AccessPolicy, error) {
	artifact, err := s.artifacts.ArtifactByServingState(ctx, current.ID)
	if err != nil {
		return workspace.AccessPolicy{}, err
	}
	root, err := os.MkdirTemp("", "libredash-activate-*")
	if err != nil {
		return workspace.AccessPolicy{}, err
	}
	defer os.RemoveAll(root)
	if err := servingstatefs.ExtractArtifact(artifact.Path, root); err != nil {
		return workspace.AccessPolicy{}, err
	}
	compiled, _, err := servingstatefs.LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		return workspace.AccessPolicy{}, err
	}
	if compiled.WorkspaceID != string(current.WorkspaceID) {
		return workspace.AccessPolicy{}, fmt.Errorf("compiled artifact workspace = %q, want %q", compiled.WorkspaceID, current.WorkspaceID)
	}
	if compiled.ServingStateID != string(current.ID) {
		return workspace.AccessPolicy{}, fmt.Errorf("compiled artifact serving state = %q, want %q", compiled.ServingStateID, current.ID)
	}
	if servingstate.Environment(compiled.Environment) != servingstate.NormalizeEnvironment(current.Environment) {
		return workspace.AccessPolicy{}, fmt.Errorf("compiled artifact environment = %q, want %q", compiled.Environment, servingstate.NormalizeEnvironment(current.Environment))
	}
	if err := servingstatefs.ValidateCompiledWorkspaceArtifact(compiled); err != nil {
		return workspace.AccessPolicy{}, err
	}
	return compiled.Definition.Access, nil
}
