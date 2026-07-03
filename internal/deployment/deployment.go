package deployment

import (
	"errors"
	"time"

	"github.com/Yacobolo/libredash/internal/workspace"
)

var ErrNotFound = errors.New("deployment not found")

type ID string

type WorkspaceID string

type Environment string

type Status string

const (
	StatusPending         Status = "pending"
	StatusValidated       Status = "validated"
	StatusActive          Status = "active"
	StatusDraining        Status = "draining"
	StatusInactive        Status = "inactive"
	StatusFailed          Status = "failed"
	StatusExpired         Status = "expired"
	StatusDeleteScheduled Status = "delete_scheduled"
	StatusDeleted         Status = "deleted"
)

const DefaultEnvironment Environment = "dev"

type Source string

const (
	SourcePublish Source = "publish"
	SourceRefresh Source = "refresh"
)

type Deployment struct {
	ID                 ID
	WorkspaceID        WorkspaceID
	Environment        Environment
	Status             Status
	Source             Source
	Digest             string
	ManifestJSON       string
	CreatedBy          string
	CreatedAt          string
	ActivatedAt        string
	SupersededAt       string
	CleanupAfter       string
	Error              string
	DuckLakeSnapshotID int64
}

func (d Deployment) CanActivate() bool {
	return d.Status == StatusValidated || d.Status == StatusInactive || d.Status == StatusActive
}

type CreateInput struct {
	WorkspaceID WorkspaceID
	Environment Environment
	CreatedBy   string
	Source      Source
}

type Artifact struct {
	ID           string
	DeploymentID ID
	WorkspaceID  WorkspaceID
	Environment  Environment
	Digest       string
	Format       string
	Path         string
	DataRoot     string
	ManifestJSON string
	SizeBytes    int64
	CreatedAt    string
}

type SnapshotLeaseInput struct {
	WorkspaceID        WorkspaceID
	Environment        Environment
	DeploymentID       ID
	DuckLakeSnapshotID int64
	OwnerID            string
	ExpiresAt          time.Time
}

type Validation struct {
	Digest       string
	ManifestJSON string
	RootDir      string
	DataRoot     string
	Graph        workspace.AssetGraph
}

type PreparedRuntime interface {
	Close() error
}

func NormalizeEnvironment(value Environment) Environment {
	if value == "" {
		return DefaultEnvironment
	}
	return value
}

func NormalizeSource(value Source) Source {
	if value == "" {
		return SourcePublish
	}
	return value
}
