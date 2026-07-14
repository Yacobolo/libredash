// Package binding pins project-global managed-data revisions to serving states.
package binding

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/manageddata"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
)

var (
	ErrArtifactMetadata           = errors.New("managed data artifact metadata is inconsistent")
	ErrCurrentRevisionUnavailable = errors.New("managed data current revision is unavailable")
	ErrRepository                 = errors.New("managed data binding repository failure")
)

// Repository is the metadata surface needed to pin current revisions.
// ReplaceServingStateBindings must replace the complete set atomically.
type Repository interface {
	CollectionByProjectConnection(context.Context, string, string) (manageddata.Collection, error)
	EnvironmentPointer(context.Context, string, manageddata.Environment) (manageddata.EnvironmentPointer, error)
	RevisionByID(context.Context, string) (manageddata.Revision, error)
	ReplaceServingStateBindings(context.Context, string, []manageddata.ServingStateBinding) error
}

type artifactLoader func(string) (servingstatefs.CompiledWorkspaceArtifact, error)

// Binder resolves project-global managed-data pointers during publish validation.
type Binder struct {
	repository Repository
	load       artifactLoader
}

func New(repository Repository) (*Binder, error) {
	if repository == nil {
		return nil, fmt.Errorf("managed data binding repository is required")
	}
	return newBinder(repository, loadCompiledArtifact), nil
}

func newBinder(repository Repository, load artifactLoader) *Binder {
	return &Binder{repository: repository, load: load}
}

func loadCompiledArtifact(root string) (servingstatefs.CompiledWorkspaceArtifact, error) {
	compiled, _, err := servingstatefs.LoadCompiledWorkspaceArtifact(root)
	return compiled, err
}

// AfterArtifactValidation implements the serving-state publish validation hook.
func (b *Binder) AfterArtifactValidation(ctx context.Context, candidate servingstate.State, validation servingstate.Validation) error {
	if b == nil || b.repository == nil || b.load == nil {
		return ErrRepository
	}
	if strings.TrimSpace(string(candidate.ID)) == "" || strings.TrimSpace(validation.RootDir) == "" {
		return ErrArtifactMetadata
	}
	compiled, err := b.load(validation.RootDir)
	if err != nil {
		return ErrArtifactMetadata
	}
	connections, err := managedConnections(compiled)
	if err != nil {
		return err
	}
	if compiled.ProjectID != strings.TrimSpace(compiled.ProjectID) || len(connections) > 0 && compiled.ProjectID == "" {
		return ErrArtifactMetadata
	}

	environment, err := manageddata.NormalizeEnvironment(string(servingstate.NormalizeEnvironment(candidate.Environment)))
	if err != nil {
		return ErrArtifactMetadata
	}
	bindings := make([]manageddata.ServingStateBinding, 0, len(connections))
	for _, connection := range connections {
		binding, bindErr := b.currentBinding(ctx, candidate.ID, compiled.ProjectID, connection, environment)
		if bindErr != nil {
			return bindErr
		}
		bindings = append(bindings, binding)
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].CollectionID != bindings[j].CollectionID {
			return bindings[i].CollectionID < bindings[j].CollectionID
		}
		return bindings[i].RevisionID < bindings[j].RevisionID
	})
	if err := b.repository.ReplaceServingStateBindings(ctx, string(candidate.ID), bindings); err != nil {
		return repositoryError(err)
	}
	return nil
}

func managedConnections(compiled servingstatefs.CompiledWorkspaceArtifact) ([]string, error) {
	if compiled.Definition == nil {
		return nil, ErrArtifactMetadata
	}
	connectionsByName := map[string]semanticmodel.Connection{}
	for _, model := range compiled.Definition.Models {
		if model == nil {
			return nil, ErrArtifactMetadata
		}
		for name, connection := range model.Connections {
			authoredName := name
			name = strings.TrimSpace(authoredName)
			kind := strings.TrimSpace(connection.Kind)
			if name == "" || name != authoredName || kind == "" || kind != connection.Kind {
				return nil, ErrArtifactMetadata
			}
			if existing, ok := connectionsByName[name]; ok && !reflect.DeepEqual(existing, connection) {
				return nil, ErrArtifactMetadata
			}
			connectionsByName[name] = connection
		}
	}
	connections := make([]string, 0, len(connectionsByName))
	for name, connection := range connectionsByName {
		if connection.Kind == "managed" {
			connections = append(connections, name)
		}
	}
	sort.Strings(connections)
	return connections, nil
}

func (b *Binder) currentBinding(ctx context.Context, servingStateID servingstate.ID, projectID, connectionName string, environment manageddata.Environment) (manageddata.ServingStateBinding, error) {
	collection, err := b.repository.CollectionByProjectConnection(ctx, projectID, connectionName)
	if err != nil {
		if errors.Is(err, manageddata.ErrNotFound) {
			return manageddata.ServingStateBinding{}, ErrCurrentRevisionUnavailable
		}
		return manageddata.ServingStateBinding{}, repositoryError(err)
	}
	if collection.ID == "" || collection.ProjectID != projectID || collection.ConnectionName != connectionName {
		return manageddata.ServingStateBinding{}, ErrArtifactMetadata
	}
	if collection.Status != manageddata.CollectionStatusActive {
		return manageddata.ServingStateBinding{}, ErrCurrentRevisionUnavailable
	}

	pointer, err := b.repository.EnvironmentPointer(ctx, collection.ID, environment)
	if err != nil {
		if errors.Is(err, manageddata.ErrNotFound) {
			return manageddata.ServingStateBinding{}, ErrCurrentRevisionUnavailable
		}
		return manageddata.ServingStateBinding{}, repositoryError(err)
	}
	if pointer.CollectionID != collection.ID || pointer.Environment != environment || pointer.RevisionID == "" || pointer.RolloutID == "" || pointer.Generation <= 0 {
		return manageddata.ServingStateBinding{}, ErrArtifactMetadata
	}

	revision, err := b.repository.RevisionByID(ctx, pointer.RevisionID)
	if err != nil {
		if errors.Is(err, manageddata.ErrNotFound) {
			return manageddata.ServingStateBinding{}, ErrCurrentRevisionUnavailable
		}
		return manageddata.ServingStateBinding{}, repositoryError(err)
	}
	if revision.ID != pointer.RevisionID || revision.CollectionID != collection.ID {
		return manageddata.ServingStateBinding{}, ErrArtifactMetadata
	}
	if revision.Status != manageddata.RevisionStatusReady {
		return manageddata.ServingStateBinding{}, ErrCurrentRevisionUnavailable
	}
	return manageddata.ServingStateBinding{
		ServingStateID: string(servingStateID),
		CollectionID:   collection.ID,
		RevisionID:     revision.ID,
		Environment:    environment,
	}, nil
}

func repositoryError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrRepository
}
