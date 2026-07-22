package apiadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/Yacobolo/leapview/internal/deployment"
	"github.com/Yacobolo/leapview/internal/manageddata"
)

func TestCreateProducesStableIdAndRequestDigestForIdempotentReplay(t *testing.T) {
	service := &fakeService{}
	metadata := &fakeMetadata{}
	adapter, err := New(service, metadata)
	if err != nil {
		t.Fatal(err)
	}
	request := CreateRequest{
		Project: "project", Environment: "prod", Actor: "principal", IdempotencyKey: "deploy-1",
		Targets: []TargetRequest{{Workspace: "support", CandidateID: "support_2"}, {Workspace: "sales", CandidateID: "sales_2"}},
	}

	first, err := adapter.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID || first.RequestDigest == "" || first.RequestDigest != second.RequestDigest {
		t.Fatalf("first = %#v, second = %#v", first, second)
	}
	if service.created.Targets[0].WorkspaceID != "sales" || first.Project != "project" || first.Status != StatusPending {
		t.Fatalf("created input = %#v, response = %#v", service.created, first)
	}
}

func TestCreateRejectsDuplicateWorkspaceAsInvalidRequest(t *testing.T) {
	adapter, err := New(&fakeService{}, &fakeMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Create(context.Background(), CreateRequest{
		Project: "project", Environment: "prod", Actor: "principal", IdempotencyKey: "deploy-1",
		Targets: []TargetRequest{{Workspace: "sales", CandidateID: "sales_1"}, {Workspace: "sales", CandidateID: "sales_2"}},
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want invalid request", err)
	}
}

func TestMapResponseExposesPublicManagedRevisionDigests(t *testing.T) {
	service := &fakeService{row: deployment.Deployment{
		ID: "deployment_1", ProjectID: "project", Environment: "prod", RequestDigest: "sha256:request", Status: deployment.StatusActive,
		Targets:     []deployment.Target{{DeploymentID: "deployment_1", WorkspaceID: "sales", ServingStateID: "sales_2", Status: deployment.TargetStatusActive}},
		Connections: []deployment.ConnectionPointer{{DeploymentID: "deployment_1", CollectionID: "orders", RevisionID: "revision_2", PriorRevisionID: "revision_1", PriorGeneration: 1, ActivatedGeneration: 2}},
	}}
	metadata := &fakeMetadata{
		collections: map[string]manageddata.Collection{"orders": {ID: "orders", ProjectID: "project", ConnectionName: "orders"}},
		revisions: map[string]manageddata.Revision{
			"revision_1": {ID: "revision_1", CollectionID: "orders", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: manageddata.RevisionStatusReady},
			"revision_2": {ID: "revision_2", CollectionID: "orders", Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Status: manageddata.RevisionStatusReady},
		},
	}
	adapter, _ := New(service, metadata)

	got, err := adapter.Get(context.Background(), Scope{Project: "project", DeploymentID: "deployment_1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "project" || got.Status != StatusActive || len(got.Connections) != 1 || got.Connections[0].RevisionID != metadata.revisions["revision_2"].Digest {
		t.Fatalf("response = %#v", got)
	}
}

type fakeService struct {
	row     deployment.Deployment
	created deployment.CreateInput
}

func (s *fakeService) Create(_ context.Context, input deployment.CreateInput) (deployment.Deployment, error) {
	s.created = input
	if s.row.ID == "" {
		s.row = deployment.Deployment{ID: input.ID, ProjectID: input.ProjectID, Environment: input.Environment, RequestDigest: input.RequestDigest, Status: deployment.StatusPending, CreatedBy: input.CreatedBy}
		for _, target := range input.Targets {
			s.row.Targets = append(s.row.Targets, deployment.Target{DeploymentID: input.ID, WorkspaceID: target.WorkspaceID, ServingStateID: target.ServingStateID, Status: deployment.TargetStatusPending})
		}
	}
	return s.row, nil
}
func (s *fakeService) Get(context.Context, deployment.Scope) (deployment.Deployment, error) {
	return s.row, nil
}
func (s *fakeService) Activate(context.Context, deployment.Scope) (deployment.Deployment, error) {
	return s.row, nil
}
func (s *fakeService) Cancel(context.Context, deployment.Scope) (deployment.Deployment, error) {
	s.row.Status = deployment.StatusCancelled
	return s.row, nil
}

type fakeMetadata struct {
	collections map[string]manageddata.Collection
	revisions   map[string]manageddata.Revision
}

func (m *fakeMetadata) CollectionByID(_ context.Context, id string) (manageddata.Collection, error) {
	return m.collections[id], nil
}
func (m *fakeMetadata) RevisionByID(_ context.Context, id string) (manageddata.Revision, error) {
	return m.revisions[id], nil
}
