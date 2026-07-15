// Package deployment coordinates project-scoped atomic serving-state cutovers.
package deployment

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound = errors.New("deployment not found")
	ErrConflict = errors.New("deployment conflict")
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusActive     Status = "active"
	StatusFailed     Status = "failed"
	StatusSuperseded Status = "superseded"
)

type TargetStatus string

const (
	TargetStatusPending TargetStatus = "pending"
	TargetStatusActive  TargetStatus = "active"
	TargetStatusFailed  TargetStatus = "failed"
)

type Deployment struct {
	ID            string
	ProjectID     string
	Environment   string
	RequestDigest string
	Status        Status
	CreatedBy     string
	CreatedAt     string
	ActivatedAt   string
	Error         string
	Targets       []Target
	Connections   []ConnectionPointer
}

type Target struct {
	DeploymentID        string
	WorkspaceID         string
	ServingStateID      string
	PriorServingStateID string
	Status              TargetStatus
	ActivatedAt         string
	Error               string
}

type ConnectionPointer struct {
	DeploymentID        string
	CollectionID        string
	RevisionID          string
	PriorRevisionID     string
	PriorGeneration     int64
	ActivatedGeneration int64
}

type TargetInput struct {
	WorkspaceID    string
	ServingStateID string
}

type CreateInput struct {
	ID            string
	ProjectID     string
	Environment   string
	RequestDigest string
	Targets       []TargetInput
	CreatedBy     string
}

type Scope struct {
	ProjectID    string
	DeploymentID string
}

func validateCreate(input CreateInput) error {
	if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.Environment) == "" || strings.TrimSpace(input.RequestDigest) == "" {
		return fmt.Errorf("deployment id, project, environment, and request digest are required")
	}
	if len(input.Targets) == 0 {
		return fmt.Errorf("deployment requires at least one workspace target")
	}
	workspaces := make(map[string]struct{}, len(input.Targets))
	states := make(map[string]struct{}, len(input.Targets))
	for _, target := range input.Targets {
		workspaceID := strings.TrimSpace(target.WorkspaceID)
		servingStateID := strings.TrimSpace(target.ServingStateID)
		if workspaceID == "" || servingStateID == "" {
			return fmt.Errorf("deployment target workspace and serving state are required")
		}
		if _, duplicate := workspaces[workspaceID]; duplicate {
			return fmt.Errorf("deployment has duplicate workspace target %q", workspaceID)
		}
		if _, duplicate := states[servingStateID]; duplicate {
			return fmt.Errorf("deployment has duplicate serving state target %q", servingStateID)
		}
		workspaces[workspaceID] = struct{}{}
		states[servingStateID] = struct{}{}
	}
	return nil
}
