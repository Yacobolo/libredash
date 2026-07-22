package release

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Yacobolo/leapview/internal/servingstate"
)

func TestValidateFinalizationRequiresEveryArtifactToMatchReleaseConnectionPins(t *testing.T) {
	pinErr := errors.New("artifact pins disagree with release manifest")
	repo := &serviceTestReleaseRepository{current: Release{
		ID: "release-1", ProjectID: "project-a", ProjectDigest: "sha256:project", Status: StatusValidating,
		Manifest:  Manifest{Connections: []ConnectionPin{{ConnectionID: "orders", RevisionID: "sha256:orders"}}},
		Artifacts: []Artifact{{WorkspaceID: "sales", ServingStateID: "state-1", ExpectedDigest: "sha256:artifact"}},
	}}
	pins := &serviceTestPinValidator{err: pinErr}
	service := &Service{
		releases: repo,
		validator: serviceTestArtifactValidator{state: servingstate.State{
			ID: "state-1", ProjectID: "project-a", ProjectDigest: "sha256:project", Digest: "sha256:artifact",
		}},
		pins: pins,
	}

	got, err := service.ValidateFinalization(t.Context(), "project-a", "release-1")
	if !errors.Is(err, pinErr) || got.Status != StatusFailed {
		t.Fatalf("ValidateFinalization() = status %q, error %v", got.Status, err)
	}
	if repo.completed {
		t.Fatal("release became ready despite mismatched managed-data pins")
	}
	want := map[string]string{"orders": "sha256:orders"}
	if pins.stateID != "state-1" || pins.projectID != "project-a" || !reflect.DeepEqual(pins.expected, want) {
		t.Fatalf("pin validation = state %q project %q pins %#v, want state-1 project-a %#v", pins.stateID, pins.projectID, pins.expected, want)
	}
}

type serviceTestReleaseRepository struct {
	current   Release
	completed bool
}

func (r *serviceTestReleaseRepository) Create(context.Context, CreateInput) (Release, error) {
	return Release{}, nil
}
func (r *serviceTestReleaseRepository) Get(context.Context, string, string) (Release, error) {
	return r.current, nil
}
func (r *serviceTestReleaseRepository) List(context.Context, string) ([]Release, error) {
	return nil, nil
}
func (r *serviceTestReleaseRepository) AssignArtifactTarget(context.Context, string, string, string, string) error {
	return nil
}
func (r *serviceTestReleaseRepository) RecordArtifact(context.Context, Artifact) error { return nil }
func (r *serviceTestReleaseRepository) BeginFinalization(context.Context, string, string) (Release, error) {
	return r.current, nil
}
func (r *serviceTestReleaseRepository) CompleteFinalization(context.Context, string, string, map[string]string) (Release, error) {
	r.completed = true
	r.current.Status = StatusReady
	return r.current, nil
}
func (r *serviceTestReleaseRepository) FailFinalization(_ context.Context, _, _ string, cause error) (Release, error) {
	r.current.Status = StatusFailed
	r.current.Error = cause.Error()
	return r.current, nil
}

type serviceTestArtifactValidator struct{ state servingstate.State }

func (v serviceTestArtifactValidator) Validate(context.Context, servingstate.ID) (servingstate.State, error) {
	return v.state, nil
}

type serviceTestPinValidator struct {
	stateID, projectID string
	expected           map[string]string
	err                error
}

func (v *serviceTestPinValidator) ValidateServingStatePins(_ context.Context, stateID, projectID string, expected map[string]string) error {
	v.stateID, v.projectID = stateID, projectID
	v.expected = make(map[string]string, len(expected))
	for key, value := range expected {
		v.expected[key] = value
	}
	return v.err
}

// Compile-time guards keep the service fakes aligned with the real interfaces.
var _ Repository = (*serviceTestReleaseRepository)(nil)
var _ ArtifactValidator = serviceTestArtifactValidator{}
