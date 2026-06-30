package activate

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentfs "github.com/Yacobolo/libredash/internal/deployment/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
)

var ErrInvalidStatus = errors.New("deployment cannot be activated")

type Repository interface {
	ByID(ctx context.Context, id deployment.ID) (deployment.Deployment, error)
	Activate(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment, deploymentID deployment.ID) (deployment.Deployment, error)
	ActivateWithWorkspacePolicy(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment, deploymentID deployment.ID, policy workspace.AccessPolicy) (deployment.Deployment, error)
}

type ArtifactRepository interface {
	ArtifactByDeployment(ctx context.Context, deploymentID deployment.ID) (deployment.Artifact, error)
}

type RuntimeHost interface {
	PrepareDeployment(ctx context.Context, deploymentID string) (deployment.PreparedRuntime, error)
	CommitPrepared(prepared deployment.PreparedRuntime) error
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

func (s Service) Activate(ctx context.Context, deploymentID deployment.ID) (deployment.Deployment, error) {
	current, err := s.repo.ByID(ctx, deploymentID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	if !current.CanActivate() {
		return deployment.Deployment{}, fmt.Errorf("%w: deployment %s has status %q, want validated", ErrInvalidStatus, deploymentID, current.Status)
	}

	var policy *workspace.AccessPolicy
	if s.access != nil && s.artifacts != nil {
		loaded, err := s.accessPolicy(ctx, current)
		if err != nil {
			return deployment.Deployment{}, err
		}
		policy = &loaded
	}
	var prepared deployment.PreparedRuntime
	if s.runtime != nil {
		prepared, err = s.runtime.PrepareDeployment(ctx, string(deploymentID))
		if err != nil {
			return deployment.Deployment{}, err
		}
	}

	var activated deployment.Deployment
	if policy != nil {
		activated, err = s.repo.ActivateWithWorkspacePolicy(ctx, current.WorkspaceID, current.Environment, current.ID, *policy)
	} else {
		activated, err = s.repo.Activate(ctx, current.WorkspaceID, current.Environment, current.ID)
	}
	if err != nil {
		if prepared != nil {
			_ = prepared.Close()
		}
		return deployment.Deployment{}, err
	}
	if prepared != nil {
		if err := s.runtime.CommitPrepared(prepared); err != nil {
			_ = prepared.Close()
			return deployment.Deployment{}, err
		}
	}
	return activated, nil
}

func (s Service) accessPolicy(ctx context.Context, current deployment.Deployment) (workspace.AccessPolicy, error) {
	artifact, err := s.artifacts.ArtifactByDeployment(ctx, current.ID)
	if err != nil {
		return workspace.AccessPolicy{}, err
	}
	root, err := os.MkdirTemp("", "libredash-activate-*")
	if err != nil {
		return workspace.AccessPolicy{}, err
	}
	defer os.RemoveAll(root)
	if err := deploymentfs.ExtractArtifact(artifact.Path, root); err != nil {
		return workspace.AccessPolicy{}, err
	}
	compiled, _, err := deploymentfs.LoadCompiledWorkspaceArtifact(root)
	if err != nil {
		return workspace.AccessPolicy{}, err
	}
	if compiled.WorkspaceID != string(current.WorkspaceID) {
		return workspace.AccessPolicy{}, fmt.Errorf("compiled artifact workspace = %q, want %q", compiled.WorkspaceID, current.WorkspaceID)
	}
	if compiled.DeploymentID != string(current.ID) {
		return workspace.AccessPolicy{}, fmt.Errorf("compiled artifact deployment = %q, want %q", compiled.DeploymentID, current.ID)
	}
	if deployment.Environment(compiled.Environment) != deployment.NormalizeEnvironment(current.Environment) {
		return workspace.AccessPolicy{}, fmt.Errorf("compiled artifact environment = %q, want %q", compiled.Environment, deployment.NormalizeEnvironment(current.Environment))
	}
	if err := deploymentfs.ValidateCompiledWorkspaceArtifact(compiled); err != nil {
		return workspace.AccessPolicy{}, err
	}
	return compiled.Definition.Access, nil
}
