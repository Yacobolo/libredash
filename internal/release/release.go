// Package release models immutable, project-wide deployment releases.
package release

import "errors"

var (
	ErrInvalid    = errors.New("invalid release")
	ErrNotFound   = errors.New("release not found")
	ErrConflict   = errors.New("release conflict")
	ErrIncomplete = errors.New("release artifacts are incomplete")
	ErrImmutable  = errors.New("release is immutable")
	ErrDigest     = errors.New("content digest mismatch")
)

type Status string

const (
	StatusDraft      Status = "draft"
	StatusValidating Status = "validating"
	StatusReady      Status = "ready"
	StatusFailed     Status = "failed"
)

type WorkspaceManifest struct {
	WorkspaceID    string `json:"workspace"`
	ArtifactDigest string `json:"artifactDigest"`
	ServingStateID string `json:"servingStateId,omitempty"`
}

type ConnectionPin struct {
	ConnectionID string `json:"connection"`
	RevisionID   string `json:"revisionId"`
}

type Manifest struct {
	Workspaces  []WorkspaceManifest `json:"workspaces"`
	Connections []ConnectionPin     `json:"connections"`
}

type CreateInput struct {
	ID             string
	ProjectID      string
	ProjectDigest  string
	RequestDigest  string
	IdempotencyKey string
	CreatedBy      string
	Workspaces     []WorkspaceManifest
	Connections    []ConnectionPin
}

type Artifact struct {
	ReleaseID      string
	WorkspaceID    string
	ExpectedDigest string
	ServingStateID string
	ActualDigest   string
	SizeBytes      int64
	UploadedAt     string
}

type Release struct {
	ID             string
	ProjectID      string
	ProjectDigest  string
	RequestDigest  string
	IdempotencyKey string
	Status         Status
	Manifest       Manifest
	Artifacts      []Artifact
	CreatedBy      string
	CreatedAt      string
	FinalizedAt    string
	Error          string
}
