// Package apiadapter maps managed-data metadata to transport-neutral control contracts.
package apiadapter

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/manageddata"
	"github.com/Yacobolo/libredash/internal/manageddata/control"
)

var canonicalRevisionID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Repository interface {
	CollectionByProjectConnection(context.Context, string, string) (manageddata.Collection, error)
	RevisionByID(context.Context, string) (manageddata.Revision, error)
	ListRevisions(context.Context, string) ([]manageddata.Revision, error)
	ListUploadSessions(context.Context, string) ([]manageddata.UploadSession, error)
	UploadSessionIDByRevisionID(context.Context, string) (string, error)
	EnvironmentPointer(context.Context, string, manageddata.Environment) (manageddata.EnvironmentPointer, error)
}

func (a *Adapter) ListUploadSessions(ctx context.Context, collectionID string) ([]manageddata.UploadSession, error) {
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return nil, control.ErrInvalid
	}
	rows, err := a.repository.ListUploadSessions(ctx, collectionID)
	if err != nil {
		return nil, publicError(err)
	}
	for _, row := range rows {
		if row.CollectionID != collectionID {
			return nil, control.ErrBackend
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt == rows[j].CreatedAt {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt > rows[j].CreatedAt
	})
	return rows, nil
}

type Adapter struct {
	repository Repository
}

func New(repository Repository) (*Adapter, error) {
	if repository == nil {
		return nil, fmt.Errorf("managed-data repository is required")
	}
	return &Adapter{repository: repository}, nil
}

func (a *Adapter) CollectionByProjectConnection(ctx context.Context, project, connection string) (manageddata.Collection, error) {
	collection, err := a.repository.CollectionByProjectConnection(ctx, strings.TrimSpace(project), strings.TrimSpace(connection))
	if err != nil {
		return manageddata.Collection{}, publicError(err)
	}
	if collection.ProjectID != strings.TrimSpace(project) || collection.ConnectionName != strings.TrimSpace(connection) || collection.Status != manageddata.CollectionStatusActive {
		return manageddata.Collection{}, control.ErrNotFound
	}
	return collection, nil
}

func (a *Adapter) RevisionByID(ctx context.Context, collectionID, publicID string) (control.RevisionMetadata, error) {
	collectionID = strings.TrimSpace(collectionID)
	publicID = strings.TrimSpace(publicID)
	if collectionID == "" || !canonicalRevisionID.MatchString(publicID) {
		return control.RevisionMetadata{}, control.ErrInvalid
	}
	revision, err := a.scopedRevisionByDigest(ctx, collectionID, publicID)
	if err != nil {
		return control.RevisionMetadata{}, err
	}
	return a.revisionMetadata(ctx, revision)
}

func (a *Adapter) ListRevisions(ctx context.Context, collectionID string) ([]control.RevisionMetadata, error) {
	collectionID = strings.TrimSpace(collectionID)
	rows, err := a.repository.ListRevisions(ctx, collectionID)
	if err != nil {
		return nil, publicError(err)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Sequence == rows[j].Sequence {
			return rows[i].Digest > rows[j].Digest
		}
		return rows[i].Sequence > rows[j].Sequence
	})
	out := make([]control.RevisionMetadata, 0, len(rows))
	for _, revision := range rows {
		if revision.CollectionID != collectionID {
			return nil, control.ErrBackend
		}
		if revision.Status != manageddata.RevisionStatusReady {
			continue
		}
		metadata, metadataErr := a.revisionMetadata(ctx, revision)
		if metadataErr != nil {
			return nil, metadataErr
		}
		out = append(out, metadata)
	}
	return out, nil
}

func (a *Adapter) EnvironmentPointer(ctx context.Context, collectionID string, environment manageddata.Environment) (manageddata.EnvironmentPointer, error) {
	pointer, err := a.repository.EnvironmentPointer(ctx, strings.TrimSpace(collectionID), environment)
	if err != nil {
		return manageddata.EnvironmentPointer{}, publicError(err)
	}
	if pointer.CollectionID != collectionID || pointer.Environment != environment {
		return manageddata.EnvironmentPointer{}, control.ErrNotFound
	}
	revision, err := a.repository.RevisionByID(ctx, pointer.RevisionID)
	if err != nil {
		return manageddata.EnvironmentPointer{}, publicError(err)
	}
	if revision.CollectionID != collectionID || revision.Status != manageddata.RevisionStatusReady || !canonicalRevisionID.MatchString(revision.Digest) {
		return manageddata.EnvironmentPointer{}, control.ErrBackend
	}
	pointer.RevisionID = revision.Digest
	return pointer, nil
}

func (a *Adapter) revisionMetadata(ctx context.Context, revision manageddata.Revision) (control.RevisionMetadata, error) {
	if revision.Status != manageddata.RevisionStatusReady || !canonicalRevisionID.MatchString(revision.Digest) {
		return control.RevisionMetadata{}, control.ErrNotFound
	}
	uploadID, err := a.repository.UploadSessionIDByRevisionID(ctx, revision.ID)
	if err != nil {
		return control.RevisionMetadata{}, publicError(err)
	}
	if strings.TrimSpace(uploadID) == "" {
		return control.RevisionMetadata{}, control.ErrBackend
	}
	revision.ID = revision.Digest
	return control.RevisionMetadata{Revision: revision, UploadSessionID: uploadID}, nil
}

func (a *Adapter) scopedRevisionByDigest(ctx context.Context, collectionID, digest string) (manageddata.Revision, error) {
	rows, err := a.repository.ListRevisions(ctx, collectionID)
	if err != nil {
		return manageddata.Revision{}, publicError(err)
	}
	var found *manageddata.Revision
	for index := range rows {
		if rows[index].CollectionID != collectionID {
			return manageddata.Revision{}, control.ErrBackend
		}
		if rows[index].Digest != digest || rows[index].Status != manageddata.RevisionStatusReady {
			continue
		}
		if found != nil {
			return manageddata.Revision{}, control.ErrBackend
		}
		copy := rows[index]
		found = &copy
	}
	if found == nil {
		return manageddata.Revision{}, control.ErrNotFound
	}
	return *found, nil
}

func publicError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, manageddata.ErrNotFound), errors.Is(err, control.ErrNotFound):
		return control.ErrNotFound
	case errors.Is(err, manageddata.ErrConflict), errors.Is(err, control.ErrConflict):
		return control.ErrConflict
	case errors.Is(err, control.ErrInvalid):
		return control.ErrInvalid
	default:
		return control.ErrBackend
	}
}

var _ control.MetadataRepository = (*Adapter)(nil)
