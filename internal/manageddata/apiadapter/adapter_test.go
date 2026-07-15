package apiadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/Yacobolo/libredash/internal/manageddata/control"
)

const (
	digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestRepositoryRevisionIDsAreCanonicalDigests(t *testing.T) {
	adapter := mustNew(t, fixtureRepository())
	for _, test := range []struct {
		id      string
		wantErr error
	}{
		{id: "revision_a", wantErr: control.ErrInvalid},
		{id: "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", wantErr: control.ErrInvalid},
		{id: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", wantErr: control.ErrNotFound},
	} {
		_, err := adapter.RevisionByID(context.Background(), "collection_a", test.id)
		if !errors.Is(err, test.wantErr) {
			t.Fatalf("RevisionByID(%q) error = %v, want %v", test.id, err, test.wantErr)
		}
	}
	got, err := adapter.RevisionByID(context.Background(), "collection_a", digestA)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision.ID != digestA || got.UploadSessionID != "upload_a" {
		t.Fatalf("revision = %#v", got)
	}
}

func TestListRevisionsIsDeterministicAndIncludesProvenance(t *testing.T) {
	repository := fixtureRepository()
	repository.uploads["revision_b"] = "upload_b"
	adapter := mustNew(t, repository)
	got, err := adapter.ListRevisions(context.Background(), "collection_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Revision.ID != digestB || got[0].UploadSessionID != "upload_b" || got[1].Revision.ID != digestA {
		t.Fatalf("revisions = %#v", got)
	}
}

func TestEnvironmentPointerExposesDeploymentAndPublicRevision(t *testing.T) {
	adapter := mustNew(t, fixtureRepository())
	got, err := adapter.EnvironmentPointer(context.Background(), "collection_a", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.RevisionID != digestA || got.DeploymentID != "deployment_a" {
		t.Fatalf("pointer = %#v", got)
	}
}

func mustNew(t *testing.T, repository *fakeRepository) *Adapter {
	t.Helper()
	adapter, err := New(repository)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func fixtureRepository() *fakeRepository {
	return &fakeRepository{
		collections: map[string]manageddata.Collection{
			"project_a\x00orders": {ID: "collection_a", ProjectID: "project_a", ConnectionName: "orders", Status: manageddata.CollectionStatusActive},
		},
		revisions: []manageddata.Revision{
			{ID: "revision_a", CollectionID: "collection_a", Sequence: 1, Digest: digestA, Status: manageddata.RevisionStatusReady},
			{ID: "revision_b", CollectionID: "collection_a", Sequence: 2, Digest: digestB, Status: manageddata.RevisionStatusReady},
		},
		uploads: map[string]string{"revision_a": "upload_a"},
		pointer: manageddata.EnvironmentPointer{CollectionID: "collection_a", Environment: "prod", RevisionID: "revision_a", DeploymentID: "deployment_a"},
	}
}

type fakeRepository struct {
	collections map[string]manageddata.Collection
	revisions   []manageddata.Revision
	uploads     map[string]string
	pointer     manageddata.EnvironmentPointer
}

func (r *fakeRepository) CollectionByProjectConnection(_ context.Context, project, connection string) (manageddata.Collection, error) {
	value, ok := r.collections[project+"\x00"+connection]
	if !ok {
		return manageddata.Collection{}, manageddata.ErrNotFound
	}
	return value, nil
}

func (r *fakeRepository) RevisionByID(_ context.Context, id string) (manageddata.Revision, error) {
	for _, revision := range r.revisions {
		if revision.ID == id {
			return revision, nil
		}
	}
	return manageddata.Revision{}, manageddata.ErrNotFound
}

func (r *fakeRepository) ListRevisions(_ context.Context, collectionID string) ([]manageddata.Revision, error) {
	var result []manageddata.Revision
	for _, revision := range r.revisions {
		if revision.CollectionID == collectionID {
			result = append(result, revision)
		}
	}
	return result, nil
}

func (r *fakeRepository) UploadSessionIDByRevisionID(_ context.Context, revisionID string) (string, error) {
	id, ok := r.uploads[revisionID]
	if !ok {
		return "", manageddata.ErrNotFound
	}
	return id, nil
}

func (r *fakeRepository) EnvironmentPointer(context.Context, string, manageddata.Environment) (manageddata.EnvironmentPointer, error) {
	return r.pointer, nil
}

func TestInterfaceConformance(t *testing.T) {
	var _ control.MetadataRepository = (*Adapter)(nil)
}
