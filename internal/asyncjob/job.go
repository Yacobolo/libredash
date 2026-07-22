// Package asyncjob defines durable, leased background work shared by public API resources.
package asyncjob

import (
	"context"
	"errors"
	"time"
)

var (
	ErrConflict = errors.New("async job conflicts with persisted work")
	ErrNotFound = errors.New("async job not found")
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type EnqueueInput struct {
	ID            string
	Kind          string
	WorkloadClass string
	WorkspaceID   string
	ResourceKind  string
	ResourceID    string
	Payload       []byte
}

type Job struct {
	ID, Kind, WorkloadClass, WorkspaceID, ResourceKind, ResourceID string
	Payload                                                        []byte
	Status                                                         Status
	Attempts                                                       int
	LeaseOwner, LeaseExpiresAt                                     string
	CreatedAt, StartedAt, FinishedAt                               string
	ErrorJSON                                                      string
}

type Event struct {
	ID                    int64
	ResourceKind          string
	ResourceID, EventType string
	Data                  []byte
	CreatedAt             string
}

// Repository is the durable boundary used by async producers, workers, and
// event consumers. Storage adapters implement it without exposing their
// database handle to application composition.
type Repository interface {
	Enqueue(context.Context, EnqueueInput) (Job, error)
	Get(context.Context, string) (Job, error)
	Candidates(context.Context, string, int) ([]Job, error)
	ClaimByID(context.Context, string, string, string, time.Duration) (Job, bool, error)
	Renew(context.Context, string, string, time.Duration) error
	Complete(context.Context, string, string) error
	Fail(context.Context, string, string, []byte) error
	Cancel(context.Context, string) error
	CancelClaimed(context.Context, string, string) error
	AppendEvent(context.Context, string, string, string, []byte) (Event, error)
	ListEvents(context.Context, string, string, int64, int) ([]Event, error)
}
