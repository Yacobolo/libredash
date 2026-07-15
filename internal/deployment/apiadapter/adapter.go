// Package apiadapter translates public project deployment requests to the deployment domain.
package apiadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/deployment"
	"github.com/Yacobolo/libredash/internal/manageddata"
)

var ErrInvalid = errors.New("invalid deployment request")

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

type TargetRequest struct {
	Workspace   string `json:"workspace"`
	CandidateID string `json:"candidateId"`
}

type CreateRequest struct {
	Project        string
	Environment    string
	Targets        []TargetRequest
	Actor          string
	IdempotencyKey string
}

type Scope struct {
	Project      string
	DeploymentID string
}

type ActivateRequest struct {
	Scope
	Actor          string
	IdempotencyKey string
}

type Deployment struct {
	ID            string
	Project       string
	Environment   string
	RequestDigest string
	Status        Status
	CreatedAt     string
	ActivatedAt   string
	Error         string
	Targets       []Target
	Connections   []Connection
}

type Target struct {
	Workspace        string
	CandidateID      string
	PriorCandidateID string
	Status           TargetStatus
	ActivatedAt      string
	Error            string
}

type Connection struct {
	Connection          string
	RevisionID          string
	PriorRevisionID     string
	PriorGeneration     int64
	ActivatedGeneration int64
}

type Service interface {
	Create(context.Context, deployment.CreateInput) (deployment.Deployment, error)
	Get(context.Context, deployment.Scope) (deployment.Deployment, error)
	Activate(context.Context, deployment.Scope) (deployment.Deployment, error)
}

type Metadata interface {
	CollectionByID(context.Context, string) (manageddata.Collection, error)
	RevisionByID(context.Context, string) (manageddata.Revision, error)
}

type Adapter struct {
	service  Service
	metadata Metadata
}

func New(service Service, metadata Metadata) (*Adapter, error) {
	if service == nil || metadata == nil {
		return nil, fmt.Errorf("deployment service and managed-data metadata are required")
	}
	return &Adapter{service: service, metadata: metadata}, nil
}

func (a *Adapter) Create(ctx context.Context, request CreateRequest) (Deployment, error) {
	request.Project = strings.TrimSpace(request.Project)
	request.Environment = strings.TrimSpace(request.Environment)
	request.Actor = strings.TrimSpace(request.Actor)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.Project == "" || request.Environment == "" || request.Actor == "" || request.IdempotencyKey == "" || len(request.Targets) == 0 {
		return Deployment{}, fmt.Errorf("%w: project, environment, actor, idempotency key, and targets are required", ErrInvalid)
	}

	targets := make([]TargetRequest, 0, len(request.Targets))
	workspaces := make(map[string]struct{}, len(request.Targets))
	states := make(map[string]struct{}, len(request.Targets))
	for _, target := range request.Targets {
		target.Workspace = strings.TrimSpace(target.Workspace)
		target.CandidateID = strings.TrimSpace(target.CandidateID)
		if target.Workspace == "" || target.CandidateID == "" {
			return Deployment{}, fmt.Errorf("%w: target workspace and candidate are required", ErrInvalid)
		}
		if _, duplicate := workspaces[target.Workspace]; duplicate {
			return Deployment{}, fmt.Errorf("%w: duplicate workspace target %q", ErrInvalid, target.Workspace)
		}
		if _, duplicate := states[target.CandidateID]; duplicate {
			return Deployment{}, fmt.Errorf("%w: duplicate candidate target %q", ErrInvalid, target.CandidateID)
		}
		workspaces[target.Workspace] = struct{}{}
		states[target.CandidateID] = struct{}{}
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Workspace == targets[j].Workspace {
			return targets[i].CandidateID < targets[j].CandidateID
		}
		return targets[i].Workspace < targets[j].Workspace
	})
	digest, err := requestDigest(request.Project, request.Environment, targets)
	if err != nil {
		return Deployment{}, err
	}
	input := deployment.CreateInput{
		ID:            stableID(request.Project, request.Actor, request.IdempotencyKey),
		ProjectID:     request.Project,
		Environment:   request.Environment,
		RequestDigest: digest,
		CreatedBy:     request.Actor,
		Targets:       make([]deployment.TargetInput, 0, len(targets)),
	}
	for _, target := range targets {
		input.Targets = append(input.Targets, deployment.TargetInput{WorkspaceID: target.Workspace, ServingStateID: target.CandidateID})
	}
	row, err := a.service.Create(ctx, input)
	if err != nil {
		return Deployment{}, err
	}
	return a.mapDeployment(ctx, row)
}

func (a *Adapter) Get(ctx context.Context, scope Scope) (Deployment, error) {
	row, err := a.service.Get(ctx, deployment.Scope{ProjectID: strings.TrimSpace(scope.Project), DeploymentID: strings.TrimSpace(scope.DeploymentID)})
	if err != nil {
		return Deployment{}, err
	}
	return a.mapDeployment(ctx, row)
}

func (a *Adapter) Activate(ctx context.Context, request ActivateRequest) (Deployment, error) {
	if strings.TrimSpace(request.IdempotencyKey) == "" || strings.TrimSpace(request.Actor) == "" {
		return Deployment{}, fmt.Errorf("%w: actor and idempotency key are required", ErrInvalid)
	}
	row, err := a.service.Activate(ctx, deployment.Scope{ProjectID: strings.TrimSpace(request.Project), DeploymentID: strings.TrimSpace(request.DeploymentID)})
	if err != nil {
		return Deployment{}, err
	}
	return a.mapDeployment(ctx, row)
}

func (a *Adapter) mapDeployment(ctx context.Context, row deployment.Deployment) (Deployment, error) {
	result := Deployment{
		ID: row.ID, Project: row.ProjectID, Environment: row.Environment, RequestDigest: row.RequestDigest,
		Status: Status(row.Status), CreatedAt: row.CreatedAt, ActivatedAt: row.ActivatedAt, Error: row.Error,
		Targets: make([]Target, 0, len(row.Targets)), Connections: make([]Connection, 0, len(row.Connections)),
	}
	for _, target := range row.Targets {
		result.Targets = append(result.Targets, Target{
			Workspace: target.WorkspaceID, CandidateID: target.ServingStateID, PriorCandidateID: target.PriorServingStateID,
			Status: TargetStatus(target.Status), ActivatedAt: target.ActivatedAt, Error: target.Error,
		})
	}
	for _, pointer := range row.Connections {
		collection, err := a.metadata.CollectionByID(ctx, pointer.CollectionID)
		if err != nil {
			return Deployment{}, fmt.Errorf("load managed collection %q: %w", pointer.CollectionID, err)
		}
		revision, err := a.metadata.RevisionByID(ctx, pointer.RevisionID)
		if err != nil {
			return Deployment{}, fmt.Errorf("load managed revision %q: %w", pointer.RevisionID, err)
		}
		connection := Connection{Connection: collection.ConnectionName, RevisionID: revision.Digest, PriorGeneration: pointer.PriorGeneration, ActivatedGeneration: pointer.ActivatedGeneration}
		if pointer.PriorRevisionID != "" {
			prior, err := a.metadata.RevisionByID(ctx, pointer.PriorRevisionID)
			if err != nil {
				return Deployment{}, fmt.Errorf("load prior managed revision %q: %w", pointer.PriorRevisionID, err)
			}
			connection.PriorRevisionID = prior.Digest
		}
		result.Connections = append(result.Connections, connection)
	}
	sort.Slice(result.Targets, func(i, j int) bool { return result.Targets[i].Workspace < result.Targets[j].Workspace })
	sort.Slice(result.Connections, func(i, j int) bool { return result.Connections[i].Connection < result.Connections[j].Connection })
	return result, nil
}

func stableID(project, actor, key string) string {
	sum := sha256.Sum256([]byte(project + "\x00" + actor + "\x00" + key))
	return "deployment_" + hex.EncodeToString(sum[:16])
}

func requestDigest(project, environment string, targets []TargetRequest) (string, error) {
	payload := struct {
		Project     string          `json:"project"`
		Environment string          `json:"environment"`
		Targets     []TargetRequest `json:"targets"`
	}{Project: project, Environment: environment, Targets: targets}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode deployment request: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
