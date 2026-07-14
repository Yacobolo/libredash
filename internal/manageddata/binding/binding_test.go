package binding

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/manageddata"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestBinderPinsDistinctManagedConnectionsDeterministically(t *testing.T) {
	repo := &fakeRepository{
		collections: map[string]manageddata.Collection{
			"project-a\x00orders":    {ID: "collection-z", ProjectID: "project-a", ConnectionName: "orders", Status: manageddata.CollectionStatusActive},
			"project-a\x00customers": {ID: "collection-a", ProjectID: "project-a", ConnectionName: "customers", Status: manageddata.CollectionStatusActive},
		},
		pointers: map[string]manageddata.EnvironmentPointer{
			"collection-z\x00prod": {CollectionID: "collection-z", Environment: "prod", RevisionID: "orders-r2", RolloutID: "rollout-orders", Generation: 2},
			"collection-a\x00prod": {CollectionID: "collection-a", Environment: "prod", RevisionID: "customers-r4", RolloutID: "rollout-customers", Generation: 4},
		},
		revisions: map[string]manageddata.Revision{
			"orders-r2":    {ID: "orders-r2", CollectionID: "collection-z", Status: manageddata.RevisionStatusReady},
			"customers-r4": {ID: "customers-r4", CollectionID: "collection-a", Status: manageddata.RevisionStatusReady},
		},
	}
	artifact := compiledArtifact("project-a", map[string]map[string]string{
		"sales": {
			"orders": "managed",
			"local":  "local",
		},
		"service": {
			"customers": "managed",
			"orders":    "managed",
		},
	})
	binder := newBinder(repo, func(string) (servingstatefs.CompiledWorkspaceArtifact, error) { return artifact, nil })

	err := binder.AfterArtifactValidation(t.Context(), servingstate.State{
		ID: "state-1", WorkspaceID: "sales", Environment: "prod",
	}, servingstate.Validation{RootDir: "/validated/artifact"})
	if err != nil {
		t.Fatalf("AfterArtifactValidation() error = %v", err)
	}
	if repo.replaceCalls != 1 {
		t.Fatalf("ReplaceServingStateBindings() calls = %d, want 1", repo.replaceCalls)
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

func TestBinderRejectsArtifactWithoutProject(t *testing.T) {
	repo := &fakeRepository{replaced: []manageddata.ServingStateBinding{{CollectionID: "stale"}}}
	binder := newBinder(repo, func(string) (servingstatefs.CompiledWorkspaceArtifact, error) {
		return compiledArtifact("", map[string]map[string]string{"sales": {"local": "local"}}), nil
	})

	err := binder.AfterArtifactValidation(t.Context(), servingstate.State{ID: "state-1", Environment: "dev"}, servingstate.Validation{RootDir: "/artifact"})
	if !errors.Is(err, ErrArtifactMetadata) {
		t.Fatalf("error = %v, want ErrArtifactMetadata", err)
	}
	if repo.replaceCalls != 0 {
		t.Fatalf("ReplaceServingStateBindings() calls = %d, want 0", repo.replaceCalls)
	}
}

func TestBinderRejectsManagedLegacyArtifactWithoutProject(t *testing.T) {
	repo := &fakeRepository{}
	binder := newBinder(repo, func(string) (servingstatefs.CompiledWorkspaceArtifact, error) {
		return compiledArtifact("", map[string]map[string]string{"sales": {"orders": "managed"}}), nil
	})

	err := binder.AfterArtifactValidation(t.Context(), servingstate.State{ID: "state-1", Environment: "prod"}, servingstate.Validation{RootDir: "/artifact"})
	if !errors.Is(err, ErrArtifactMetadata) {
		t.Fatalf("error = %v, want ErrArtifactMetadata", err)
	}
	if repo.replaceCalls != 0 {
		t.Fatalf("ReplaceServingStateBindings() calls = %d, want 0", repo.replaceCalls)
	}
}

func TestBinderRejectsInconsistentConnectionMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(servingstatefs.CompiledWorkspaceArtifact)
	}{
		{
			name: "kind differs",
			mutate: func(artifact servingstatefs.CompiledWorkspaceArtifact) {
				artifact.Definition.Models["second"].Connections["orders"] = semanticmodel.Connection{Kind: "local"}
			},
		},
		{
			name: "managed metadata differs",
			mutate: func(artifact servingstatefs.CompiledWorkspaceArtifact) {
				artifact.Definition.Models["second"].Connections["orders"] = semanticmodel.Connection{Kind: "managed", Description: "different"}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{}
			artifact := compiledArtifact("project-a", map[string]map[string]string{
				"first":  {"orders": "managed"},
				"second": {"orders": "managed"},
			})
			test.mutate(artifact)
			binder := newBinder(repo, func(string) (servingstatefs.CompiledWorkspaceArtifact, error) { return artifact, nil })

			err := binder.AfterArtifactValidation(t.Context(), servingstate.State{ID: "state-1", Environment: "prod"}, servingstate.Validation{RootDir: "/artifact"})
			if !errors.Is(err, ErrArtifactMetadata) {
				t.Fatalf("error = %v, want ErrArtifactMetadata", err)
			}
		})
	}
}

func TestBinderValidatesAllMetadataBeforeReplacingBindings(t *testing.T) {
	validRepo := func() *fakeRepository {
		return &fakeRepository{
			collections: map[string]manageddata.Collection{
				"project-a\x00orders": {ID: "orders", ProjectID: "project-a", ConnectionName: "orders", Status: manageddata.CollectionStatusActive},
			},
			pointers: map[string]manageddata.EnvironmentPointer{
				"orders\x00prod": {CollectionID: "orders", Environment: "prod", RevisionID: "revision-1", RolloutID: "rollout-1", Generation: 1},
			},
			revisions: map[string]manageddata.Revision{
				"revision-1": {ID: "revision-1", CollectionID: "orders", Status: manageddata.RevisionStatusReady},
			},
		}
	}
	artifact := compiledArtifact("project-a", map[string]map[string]string{"sales": {"orders": "managed"}})
	tests := []struct {
		name   string
		mutate func(*fakeRepository)
		want   error
	}{
		{name: "missing collection", mutate: func(repo *fakeRepository) { delete(repo.collections, "project-a\x00orders") }, want: ErrCurrentRevisionUnavailable},
		{name: "archived collection", mutate: func(repo *fakeRepository) {
			collection := repo.collections["project-a\x00orders"]
			collection.Status = manageddata.CollectionStatusArchived
			repo.collections["project-a\x00orders"] = collection
		}, want: ErrCurrentRevisionUnavailable},
		{name: "mismatched collection identity", mutate: func(repo *fakeRepository) {
			collection := repo.collections["project-a\x00orders"]
			collection.ProjectID = "other-project"
			repo.collections["project-a\x00orders"] = collection
		}, want: ErrArtifactMetadata},
		{name: "missing pointer", mutate: func(repo *fakeRepository) { delete(repo.pointers, "orders\x00prod") }, want: ErrCurrentRevisionUnavailable},
		{name: "mismatched pointer", mutate: func(repo *fakeRepository) {
			pointer := repo.pointers["orders\x00prod"]
			pointer.CollectionID = "other"
			repo.pointers["orders\x00prod"] = pointer
		}, want: ErrArtifactMetadata},
		{name: "missing revision", mutate: func(repo *fakeRepository) { delete(repo.revisions, "revision-1") }, want: ErrCurrentRevisionUnavailable},
		{name: "pending revision", mutate: func(repo *fakeRepository) {
			revision := repo.revisions["revision-1"]
			revision.Status = manageddata.RevisionStatusPending
			repo.revisions["revision-1"] = revision
		}, want: ErrCurrentRevisionUnavailable},
		{name: "mismatched revision", mutate: func(repo *fakeRepository) {
			revision := repo.revisions["revision-1"]
			revision.CollectionID = "other"
			repo.revisions["revision-1"] = revision
		}, want: ErrArtifactMetadata},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := validRepo()
			test.mutate(repo)
			binder := newBinder(repo, func(string) (servingstatefs.CompiledWorkspaceArtifact, error) { return artifact, nil })

			err := binder.AfterArtifactValidation(t.Context(), servingstate.State{ID: "state-1", Environment: "prod"}, servingstate.Validation{RootDir: "/artifact"})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if repo.replaceCalls != 0 {
				t.Fatalf("ReplaceServingStateBindings() calls = %d, want 0", repo.replaceCalls)
			}
		})
	}
}

func TestBinderSanitizesLoaderAndRepositoryErrors(t *testing.T) {
	secret := "s3://private-bucket/object?token=secret"
	tests := []struct {
		name  string
		build func() *Binder
		want  error
	}{
		{
			name: "loader",
			build: func() *Binder {
				return newBinder(&fakeRepository{}, func(string) (servingstatefs.CompiledWorkspaceArtifact, error) {
					return servingstatefs.CompiledWorkspaceArtifact{}, errors.New(secret)
				})
			},
			want: ErrArtifactMetadata,
		},
		{
			name: "repository",
			build: func() *Binder {
				repo := validFakeRepository()
				repo.collectionErr = errors.New(secret)
				return binderForRepository(repo)
			},
			want: ErrRepository,
		},
		{
			name: "pointer repository",
			build: func() *Binder {
				repo := validFakeRepository()
				repo.pointerErr = errors.New(secret)
				return binderForRepository(repo)
			},
			want: ErrRepository,
		},
		{
			name: "revision repository",
			build: func() *Binder {
				repo := validFakeRepository()
				repo.revisionErr = errors.New(secret)
				return binderForRepository(repo)
			},
			want: ErrRepository,
		},
		{
			name: "replacement repository",
			build: func() *Binder {
				repo := validFakeRepository()
				repo.replaceErr = errors.New(secret)
				return binderForRepository(repo)
			},
			want: ErrRepository,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.build().AfterArtifactValidation(t.Context(), servingstate.State{ID: "state-1", Environment: "prod"}, servingstate.Validation{RootDir: secret})
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
		collections: map[string]manageddata.Collection{
			"project-a\x00orders": {ID: "orders", ProjectID: "project-a", ConnectionName: "orders", Status: manageddata.CollectionStatusActive},
		},
		pointers: map[string]manageddata.EnvironmentPointer{
			"orders\x00prod": {CollectionID: "orders", Environment: "prod", RevisionID: "revision-1", RolloutID: "rollout-1", Generation: 1},
		},
		revisions: map[string]manageddata.Revision{
			"revision-1": {ID: "revision-1", CollectionID: "orders", Status: manageddata.RevisionStatusReady},
		},
	}
}

func binderForRepository(repo Repository) *Binder {
	return newBinder(repo, func(string) (servingstatefs.CompiledWorkspaceArtifact, error) {
		return compiledArtifact("project-a", map[string]map[string]string{"sales": {"orders": "managed"}}), nil
	})
}

func compiledArtifact(projectID string, models map[string]map[string]string) servingstatefs.CompiledWorkspaceArtifact {
	definition := &workspace.Definition{Models: make(map[string]*semanticmodel.Model, len(models))}
	for modelName, connections := range models {
		model := &semanticmodel.Model{Connections: make(map[string]semanticmodel.Connection, len(connections))}
		for connectionName, kind := range connections {
			model.Connections[connectionName] = semanticmodel.Connection{Kind: kind}
		}
		definition.Models[modelName] = model
	}
	return servingstatefs.CompiledWorkspaceArtifact{Version: 1, ProjectID: projectID, Definition: definition}
}

type fakeRepository struct {
	collections       map[string]manageddata.Collection
	pointers          map[string]manageddata.EnvironmentPointer
	revisions         map[string]manageddata.Revision
	collectionErr     error
	pointerErr        error
	revisionErr       error
	replaceErr        error
	collectionLookups []string
	replaced          []manageddata.ServingStateBinding
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

func (r *fakeRepository) EnvironmentPointer(_ context.Context, collectionID string, environment manageddata.Environment) (manageddata.EnvironmentPointer, error) {
	if r.pointerErr != nil {
		return manageddata.EnvironmentPointer{}, r.pointerErr
	}
	pointer, ok := r.pointers[collectionID+"\x00"+string(environment)]
	if !ok {
		return manageddata.EnvironmentPointer{}, manageddata.ErrNotFound
	}
	return pointer, nil
}

func (r *fakeRepository) RevisionByID(_ context.Context, revisionID string) (manageddata.Revision, error) {
	if r.revisionErr != nil {
		return manageddata.Revision{}, r.revisionErr
	}
	revision, ok := r.revisions[revisionID]
	if !ok {
		return manageddata.Revision{}, manageddata.ErrNotFound
	}
	return revision, nil
}

func (r *fakeRepository) ReplaceServingStateBindings(_ context.Context, _ string, bindings []manageddata.ServingStateBinding) error {
	r.replaceCalls++
	r.replaced = append([]manageddata.ServingStateBinding(nil), bindings...)
	return r.replaceErr
}
