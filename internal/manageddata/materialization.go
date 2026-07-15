package manageddata

import "context"

// RevisionMaterializer projects an immutable manifest into a local root that
// remains valid until its returned lease is released.
type RevisionMaterializer interface {
	MaterializeRevision(context.Context, string, Manifest) (RevisionLease, error)
}

// RevisionLease owns the lifetime of one materialized revision root.
type RevisionLease interface {
	Root() string
	Release() error
}
