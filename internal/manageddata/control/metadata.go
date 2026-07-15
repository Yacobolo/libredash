package control

import (
	"context"

	"github.com/Yacobolo/libredash/internal/manageddata"
)

// RevisionMetadata carries the upload provenance associated with a public
// revision. Implementations must scope revision lookup to its collection.
type RevisionMetadata struct {
	Revision        manageddata.Revision
	UploadSessionID string
}

// MetadataRepository exposes managed-data collection and revision metadata to
// delivery adapters without coupling the use case to a transport package.
type MetadataRepository interface {
	CollectionByProjectConnection(context.Context, string, string) (manageddata.Collection, error)
	RevisionByID(context.Context, string, string) (RevisionMetadata, error)
	ListRevisions(context.Context, string) ([]RevisionMetadata, error)
	EnvironmentPointer(context.Context, string, manageddata.Environment) (manageddata.EnvironmentPointer, error)
}
