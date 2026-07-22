package resource

import (
	"context"
	"database/sql"
)

// Lease pins the process-owned analytical engine connection for one logical
// operation without exposing database/sql across capability boundaries.
type Lease interface {
	Context() context.Context
	Release()
}

// Provider is implemented by analytical databases that enforce
// operation-scoped connection ownership.
type Provider interface {
	Acquire(context.Context) (Lease, error)
}

// Session is the narrow database/sql surface required by the approved DuckDB
// analytical adapters. It exposes a pinned client, never the owning pool.
type Session interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type SessionProvider interface {
	Session(context.Context) (Session, error)
}
