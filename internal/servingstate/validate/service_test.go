package validate

import (
	"context"
	"errors"
	"testing"

	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

func TestServiceValidatesPromotesAndSaves(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Status: servingstate.StatusPending},
	}
	artifacts := &fakeArtifacts{}
	validator := &fakeValidator{
		validation: servingstate.Validation{Digest: "digest", ManifestJSON: `{"version":1}`, RootDir: "/tmp/root"},
	}
	service := NewService(repo, artifacts, validator)

	validated, err := service.Validate(ctx, "dep_1")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if validated.Status != servingstate.StatusValidated {
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
		deployment: servingstate.State{ID: "dep_1", WorkspaceID: "test", Status: servingstate.StatusPending},
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
	deployment servingstate.State
	saved      bool
	failed     bool
}

func (r *fakeRepo) ByID(context.Context, servingstate.ID) (servingstate.State, error) {
	return r.deployment, nil
}

func (r *fakeRepo) MarkFailed(context.Context, servingstate.ID, error) error {
	r.failed = true
	return nil
}

func (r *fakeRepo) SaveValidated(_ context.Context, _ servingstate.ID, validation servingstate.Validation, artifact servingstate.Artifact) (servingstate.State, error) {
	r.saved = true
	r.deployment.Status = servingstate.StatusValidated
	r.deployment.Digest = validation.Digest
	r.deployment.ManifestJSON = artifact.ManifestJSON
	return r.deployment, nil
}

type fakeArtifacts struct {
	promotedDigest string
}

func (a *fakeArtifacts) UploadPath(servingstate.ID) string {
	return "upload.tar.gz"
}

func (a *fakeArtifacts) PromoteUploaded(_ context.Context, servingStateID servingstate.ID, digest, manifestJSON string) (servingstate.Artifact, error) {
	a.promotedDigest = digest
	return servingstate.Artifact{ID: "artifact_" + string(servingStateID), ServingStateID: servingStateID, Digest: digest, ManifestJSON: manifestJSON}, nil
}

type fakeValidator struct {
	validation  servingstate.Validation
	err         error
	cleaned     bool
	environment servingstate.Environment
}

func (v *fakeValidator) ValidateArtifact(_ string, _ servingstate.WorkspaceID, environment servingstate.Environment, _ servingstate.ID) (servingstate.Validation, error) {
	v.environment = environment
	if v.err != nil {
		return servingstate.Validation{}, v.err
	}
	return v.validation, nil
}

func (v *fakeValidator) Cleanup(servingstate.Validation) error {
	v.cleaned = true
	return nil
}
