// Package binding pins project-global managed-data revisions to serving states.
package binding

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Yacobolo/leapview/internal/manageddata"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

var (
	ErrArtifactMetadata          = errors.New("managed data artifact metadata is inconsistent")
	ErrPinnedRevisionUnavailable = errors.New("managed data pinned revision is unavailable")
	ErrRepository                = errors.New("managed data binding repository failure")
)

// Repository is the metadata surface needed to resolve artifact-owned pins.
// ReplaceServingStateBindings must replace the complete set atomically.
type Repository interface {
	CollectionByProjectConnection(context.Context, string, string) (manageddata.Collection, error)
	ListRevisions(context.Context, string) ([]manageddata.Revision, error)
	ReplaceServingStateBindings(context.Context, string, []manageddata.ServingStateBinding) error
	ListServingStateBindings(context.Context, string) ([]manageddata.ServingStateBinding, error)
}

// Binder resolves project-global revision pins during publish validation.
type Binder struct {
	repository Repository
}

func New(repository Repository) (*Binder, error) {
	if repository == nil {
		return nil, fmt.Errorf("managed data binding repository is required")
	}
	return &Binder{repository: repository}, nil
}

// AfterArtifactValidation implements the serving-state publish validation hook.
func (b *Binder) AfterArtifactValidation(ctx context.Context, candidate servingstate.State, validation servingstate.Validation) error {
	if b == nil || b.repository == nil {
		return ErrRepository
	}
	projectID := strings.TrimSpace(validation.ProjectID)
	if strings.TrimSpace(string(candidate.ID)) == "" || projectID == "" || projectID != validation.ProjectID || validation.ManagedDataRevisions == nil {
		return ErrArtifactMetadata
	}

	environment, err := manageddata.NormalizeEnvironment(string(servingstate.NormalizeEnvironment(candidate.Environment)))
	if err != nil {
		return ErrArtifactMetadata
	}
	connections := make([]string, 0, len(validation.ManagedDataRevisions))
	for connection, digest := range validation.ManagedDataRevisions {
		if connection == "" || connection != strings.TrimSpace(connection) || manageddata.ValidateRevisionID(digest) != nil {
			return ErrArtifactMetadata
		}
		connections = append(connections, connection)
	}
	sort.Strings(connections)
	bindings := make([]manageddata.ServingStateBinding, 0, len(connections))
	for _, connection := range connections {
		binding, bindErr := b.pinnedBinding(ctx, candidate.ID, projectID, connection, validation.ManagedDataRevisions[connection], environment)
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

// ValidateServingStatePins proves that the artifact-owned bindings written by
// AfterArtifactValidation exactly match the release manifest. Release pins are
// content digests, while serving-state bindings use internal revision IDs, so
// each expected digest must first be resolved within its project connection.
func (b *Binder) ValidateServingStatePins(ctx context.Context, servingStateID, projectID string, expected map[string]string) error {
	servingStateID = strings.TrimSpace(servingStateID)
	projectID = strings.TrimSpace(projectID)
	if b == nil || b.repository == nil {
		return ErrRepository
	}
	if servingStateID == "" || projectID == "" || expected == nil {
		return ErrArtifactMetadata
	}
	actual, err := b.repository.ListServingStateBindings(ctx, servingStateID)
	if err != nil {
		return repositoryError(err)
	}
	if len(actual) != len(expected) {
		return ErrArtifactMetadata
	}
	actualByCollection := make(map[string]manageddata.ServingStateBinding, len(actual))
	for _, binding := range actual {
		if binding.ServingStateID != servingStateID || binding.CollectionID == "" || binding.RevisionID == "" {
			return ErrArtifactMetadata
		}
		if _, duplicate := actualByCollection[binding.CollectionID]; duplicate {
			return ErrArtifactMetadata
		}
		actualByCollection[binding.CollectionID] = binding
	}
	for connection, digest := range expected {
		if connection == "" || connection != strings.TrimSpace(connection) || manageddata.ValidateRevisionID(digest) != nil {
			return ErrArtifactMetadata
		}
		resolved, resolveErr := b.pinnedBinding(ctx, servingstate.ID(servingStateID), projectID, connection, digest, manageddata.Environment("dev"))
		if resolveErr != nil {
			return resolveErr
		}
		binding, ok := actualByCollection[resolved.CollectionID]
		if !ok || binding.RevisionID != resolved.RevisionID {
			return ErrArtifactMetadata
		}
	}
	return nil
}

func (b *Binder) pinnedBinding(ctx context.Context, servingStateID servingstate.ID, projectID, connectionName, digest string, environment manageddata.Environment) (manageddata.ServingStateBinding, error) {
	collection, err := b.repository.CollectionByProjectConnection(ctx, projectID, connectionName)
	if err != nil {
		if errors.Is(err, manageddata.ErrNotFound) {
			return manageddata.ServingStateBinding{}, ErrPinnedRevisionUnavailable
		}
		return manageddata.ServingStateBinding{}, repositoryError(err)
	}
	if collection.ID == "" || collection.ProjectID != projectID || collection.ConnectionName != connectionName {
		return manageddata.ServingStateBinding{}, ErrArtifactMetadata
	}
	if collection.Status != manageddata.CollectionStatusActive {
		return manageddata.ServingStateBinding{}, ErrPinnedRevisionUnavailable
	}

	revisions, err := b.repository.ListRevisions(ctx, collection.ID)
	if err != nil {
		return manageddata.ServingStateBinding{}, repositoryError(err)
	}
	var match manageddata.Revision
	matches := 0
	for _, revision := range revisions {
		if revision.CollectionID != collection.ID {
			return manageddata.ServingStateBinding{}, ErrArtifactMetadata
		}
		if revision.Digest == digest && revision.Status == manageddata.RevisionStatusReady {
			match = revision
			matches++
		}
	}
	if matches > 1 {
		return manageddata.ServingStateBinding{}, ErrArtifactMetadata
	}
	if matches == 0 {
		return manageddata.ServingStateBinding{}, ErrPinnedRevisionUnavailable
	}
	return manageddata.ServingStateBinding{
		ServingStateID: string(servingStateID),
		CollectionID:   collection.ID,
		RevisionID:     match.ID,
		Environment:    environment,
	}, nil
}

func repositoryError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrRepository
}
