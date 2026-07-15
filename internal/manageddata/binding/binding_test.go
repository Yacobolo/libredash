package binding

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/manageddata"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
)

const (
	digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestBinderResolvesPinsWithinEachCollection(t *testing.T) {
	repo := &fakeRepository{
		collections: map[string]manageddata.Collection{
			"project-a\x00orders":    activeCollection("collection-z", "orders"),
			"project-a\x00customers": activeCollection("collection-a", "customers"),
		},
		revisions: map[string][]manageddata.Revision{
			"collection-z": {{ID: "orders-r2", CollectionID: "collection-z", Digest: digestA, Status: manageddata.RevisionStatusReady}},
			// The same content digest in another collection must resolve to that collection's internal revision.
			"collection-a": {{ID: "customers-r4", CollectionID: "collection-a", Digest: digestA, Status: manageddata.RevisionStatusReady}},
		},
	}
	binder := binderForRepository(repo)

	err := binder.AfterArtifactValidation(t.Context(), servingstate.State{
		ID: "state-1", WorkspaceID: "sales", Environment: "prod",
	}, validatedMetadata("project-a", map[string]string{"customers": digestA, "orders": digestA}))
	if err != nil {
		t.Fatalf("AfterArtifactValidation() error = %v", err)
	}
	want := []manageddata.ServingStateBinding{
		{ServingStateID: "state-1", CollectionID: "collection-a", RevisionID: "customers-r4", Environment: "prod"},
		{ServingStateID: "state-1", CollectionID: "collection-z", RevisionID: "orders-r2", Environment: "prod"},
	}
	if !reflect.DeepEqual(repo.replaced, want) {
		t.Fatalf("bindings = %#v, want %#v", repo.replaced, want)
	}
	if got, want := repo.collectionLookups, []string{"project-a\x00customers", "project-a\x00orders"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("collection lookups = %#v, want %#v", got, want)
	}
}

func TestBinderCanPinBootstrapRevisionWithoutEnvironmentPointer(t *testing.T) {
	repo := validFakeRepository()
	binder := binderForRepository(repo)
	if err := binder.AfterArtifactValidation(t.Context(), servingstate.State{ID: "bootstrap", Environment: "prod"}, validatedMetadata("project-a", map[string]string{"orders": digestA})); err != nil {
		t.Fatalf("AfterArtifactValidation() error = %v", err)
	}
	if repo.listCalls != 1 || len(repo.replaced) != 1 || repo.replaced[0].RevisionID != "revision-1" {
		t.Fatalf("repository result = calls %d bindings %#v", repo.listCalls, repo.replaced)
	}
}

func TestBinderReplacesFullBindingSetWithoutManagedConnections(t *testing.T) {
	repo := &fakeRepository{replaced: []manageddata.ServingStateBinding{{CollectionID: "stale"}}}
	binder := binderForRepository(repo)
	if err := binder.AfterArtifactValidation(t.Context(), servingstate.State{ID: "state-1", Environment: "dev"}, validatedMetadata("project-a", map[string]string{})); err != nil {
		t.Fatal(err)
	}
	if repo.replaceCalls != 1 || len(repo.replaced) != 0 {
		t.Fatalf("replace calls = %d, bindings = %#v", repo.replaceCalls, repo.replaced)
	}
}

func TestBinderRejectsInvalidOrUnavailablePinsBeforeAtomicReplacement(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeRepository, *servingstate.Validation)
		want   error
	}{
		{name: "missing project id", mutate: func(_ *fakeRepository, validation *servingstate.Validation) {
			validation.ProjectID = ""
		}, want: ErrArtifactMetadata},
		{name: "whitespace project id", mutate: func(_ *fakeRepository, validation *servingstate.Validation) {
			validation.ProjectID = " project-a"
		}, want: ErrArtifactMetadata},
		{name: "nil pins", mutate: func(_ *fakeRepository, validation *servingstate.Validation) {
			validation.ManagedDataRevisions = nil
		}, want: ErrArtifactMetadata},
		{name: "missing collection", mutate: func(repo *fakeRepository, _ *servingstate.Validation) {
			delete(repo.collections, "project-a\x00orders")
		}, want: ErrPinnedRevisionUnavailable},
		{name: "archived collection", mutate: func(repo *fakeRepository, _ *servingstate.Validation) {
			collection := repo.collections["project-a\x00orders"]
			collection.Status = manageddata.CollectionStatusArchived
			repo.collections["project-a\x00orders"] = collection
		}, want: ErrPinnedRevisionUnavailable},
		{name: "missing ready revision", mutate: func(repo *fakeRepository, _ *servingstate.Validation) {
			repo.revisions["orders"] = nil
		}, want: ErrPinnedRevisionUnavailable},
		{name: "pending revision", mutate: func(repo *fakeRepository, _ *servingstate.Validation) {
			repo.revisions["orders"][0].Status = manageddata.RevisionStatusPending
		}, want: ErrPinnedRevisionUnavailable},
		{name: "ambiguous revision", mutate: func(repo *fakeRepository, _ *servingstate.Validation) {
			repo.revisions["orders"] = append(repo.revisions["orders"], repo.revisions["orders"][0])
		}, want: ErrArtifactMetadata},
		{name: "wrong collection revision", mutate: func(repo *fakeRepository, _ *servingstate.Validation) {
			repo.revisions["orders"][0].CollectionID = "other"
		}, want: ErrArtifactMetadata},
		{name: "empty connection pin", mutate: func(_ *fakeRepository, validation *servingstate.Validation) {
			validation.ManagedDataRevisions[""] = digestA
		}, want: ErrArtifactMetadata},
		{name: "internal id pin", mutate: func(_ *fakeRepository, validation *servingstate.Validation) {
			validation.ManagedDataRevisions["orders"] = "revision-1"
		}, want: ErrArtifactMetadata},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := validFakeRepository()
			validation := validatedMetadata("project-a", map[string]string{"orders": digestA})
			test.mutate(repo, &validation)
			binder := binderForRepository(repo)
			err := binder.AfterArtifactValidation(t.Context(), servingstate.State{ID: "state-1", Environment: "prod"}, validation)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repo.replaceCalls != 0 {
				t.Fatalf("ReplaceServingStateBindings() calls = %d, want 0", repo.replaceCalls)
			}
		})
	}
}

func TestBinderSanitizesRepositoryErrors(t *testing.T) {
	secret := "s3://private-bucket/object?token=secret"
	tests := []struct {
		name  string
		build func() *Binder
		want  error
	}{
		{name: "collection", build: func() *Binder {
			repo := validFakeRepository()
			repo.collectionErr = errors.New(secret)
			return binderForRepository(repo)
		}, want: ErrRepository},
		{name: "revision", build: func() *Binder {
			repo := validFakeRepository()
			repo.listErr = errors.New(secret)
			return binderForRepository(repo)
		}, want: ErrRepository},
		{name: "replacement", build: func() *Binder {
			repo := validFakeRepository()
			repo.replaceErr = errors.New(secret)
			return binderForRepository(repo)
		}, want: ErrRepository},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.build().AfterArtifactValidation(t.Context(), servingstate.State{ID: "state-1", Environment: "prod"}, validatedMetadata("project-a", map[string]string{"orders": digestA}))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if strings.Contains(err.Error(), "private-bucket") || strings.Contains(err.Error(), "token=secret") {
				t.Fatalf("error exposed sensitive metadata: %v", err)
			}
		})
	}
}

func validFakeRepository() *fakeRepository {
	return &fakeRepository{
		collections: map[string]manageddata.Collection{"project-a\x00orders": activeCollection("orders", "orders")},
		revisions:   map[string][]manageddata.Revision{"orders": {{ID: "revision-1", CollectionID: "orders", Digest: digestA, Status: manageddata.RevisionStatusReady}}},
	}
}

func activeCollection(id, connection string) manageddata.Collection {
	return manageddata.Collection{ID: id, ProjectID: "project-a", ConnectionName: connection, Status: manageddata.CollectionStatusActive}
}

func binderForRepository(repo Repository) *Binder {
	binder, err := New(repo)
	if err != nil {
		panic(err)
	}
	return binder
}

func validatedMetadata(projectID string, pins map[string]string) servingstate.Validation {
	return servingstate.Validation{ProjectID: projectID, ManagedDataRevisions: pins}
}

type fakeRepository struct {
	collections       map[string]manageddata.Collection
	revisions         map[string][]manageddata.Revision
	collectionErr     error
	listErr           error
	replaceErr        error
	collectionLookups []string
	replaced          []manageddata.ServingStateBinding
	listCalls         int
	replaceCalls      int
}

func (r *fakeRepository) CollectionByProjectConnection(_ context.Context, projectID, connectionName string) (manageddata.Collection, error) {
	key := projectID + "\x00" + connectionName
	r.collectionLookups = append(r.collectionLookups, key)
	if r.collectionErr != nil {
		return manageddata.Collection{}, r.collectionErr
	}
	collection, ok := r.collections[key]
	if !ok {
		return manageddata.Collection{}, manageddata.ErrNotFound
	}
	return collection, nil
}

func (r *fakeRepository) ListRevisions(_ context.Context, collectionID string) ([]manageddata.Revision, error) {
	r.listCalls++
	if r.listErr != nil {
		return nil, r.listErr
	}
	return append([]manageddata.Revision(nil), r.revisions[collectionID]...), nil
}

func (r *fakeRepository) ReplaceServingStateBindings(_ context.Context, _ string, bindings []manageddata.ServingStateBinding) error {
	r.replaceCalls++
	r.replaced = append([]manageddata.ServingStateBinding(nil), bindings...)
	return r.replaceErr
}
