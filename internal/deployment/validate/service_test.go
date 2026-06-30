package validate

import (
	"context"
	"errors"
	"testing"

	"github.com/Yacobolo/libredash/internal/deployment"
)

func TestServiceValidatesPromotesAndSaves(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Status: deployment.StatusPending},
	}
	artifacts := &fakeArtifacts{}
	validator := &fakeValidator{
		validation: deployment.Validation{Digest: "digest", ManifestJSON: `{"version":1}`, RootDir: "/tmp/root"},
	}
	service := NewService(repo, artifacts, validator)

	validated, err := service.Validate(ctx, "dep_1")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if validated.Status != deployment.StatusValidated {
		t.Fatalf("status = %q, want validated", validated.Status)
	}
	if artifacts.promotedDigest != "digest" || !repo.saved {
		t.Fatalf("promoted digest=%q saved=%v, want digest/true", artifacts.promotedDigest, repo.saved)
	}
	if !validator.cleaned {
		t.Fatal("validator cleanup was not called")
	}
}

func TestServiceMarksFailedWhenValidationFails(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: deployment.Deployment{ID: "dep_1", WorkspaceID: "test", Status: deployment.StatusPending},
	}
	validator := &fakeValidator{err: errors.New("bad bundle")}
	service := NewService(repo, &fakeArtifacts{}, validator)

	if _, err := service.Validate(ctx, "dep_1"); err == nil {
		t.Fatal("expected validation error")
	}
	if !repo.failed {
		t.Fatal("deployment was not marked failed")
	}
	if repo.saved {
		t.Fatal("invalid deployment was saved")
	}
}

type fakeRepo struct {
	deployment deployment.Deployment
	saved      bool
	failed     bool
}

func (r *fakeRepo) ByID(context.Context, deployment.ID) (deployment.Deployment, error) {
	return r.deployment, nil
}

func (r *fakeRepo) MarkFailed(context.Context, deployment.ID, error) error {
	r.failed = true
	return nil
}

func (r *fakeRepo) SaveValidated(_ context.Context, _ deployment.ID, validation deployment.Validation, artifact deployment.Artifact) (deployment.Deployment, error) {
	r.saved = true
	r.deployment.Status = deployment.StatusValidated
	r.deployment.Digest = validation.Digest
	r.deployment.ManifestJSON = artifact.ManifestJSON
	return r.deployment, nil
}

type fakeArtifacts struct {
	promotedDigest string
}

func (a *fakeArtifacts) UploadPath(deployment.ID) string {
	return "upload.tar.gz"
}

func (a *fakeArtifacts) PromoteUploaded(_ context.Context, deploymentID deployment.ID, digest, manifestJSON string) (deployment.Artifact, error) {
	a.promotedDigest = digest
	return deployment.Artifact{ID: "artifact_" + string(deploymentID), DeploymentID: deploymentID, Digest: digest, ManifestJSON: manifestJSON}, nil
}

type fakeValidator struct {
	validation  deployment.Validation
	err         error
	cleaned     bool
	environment deployment.Environment
}

func (v *fakeValidator) ValidateArtifact(_ string, _ deployment.WorkspaceID, environment deployment.Environment, _ deployment.ID) (deployment.Validation, error) {
	v.environment = environment
	if v.err != nil {
		return deployment.Validation{}, v.err
	}
	return v.validation, nil
}

func (v *fakeValidator) Cleanup(deployment.Validation) error {
	v.cleaned = true
	return nil
}
