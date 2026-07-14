// Package apiadapter maps managed-data domain services to the public HTTP
// contract without exposing persistence identifiers or backend failures.
package apiadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/manageddata"
	managedhttp "github.com/Yacobolo/libredash/internal/manageddata/http"
	"github.com/Yacobolo/libredash/internal/manageddata/rollout"
)

var canonicalRevisionID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Repository is the persistence surface needed to present managed data over
// HTTP. RevisionByDigest must reject ambiguous digests across collections.
type Repository interface {
	CollectionByProjectConnection(context.Context, string, string) (manageddata.Collection, error)
	RevisionByDigest(context.Context, string) (manageddata.Revision, error)
	RevisionByID(context.Context, string) (manageddata.Revision, error)
	ListRevisions(context.Context, string) ([]manageddata.Revision, error)
	UploadSessionIDByRevisionID(context.Context, string) (string, error)
	EnvironmentPointer(context.Context, string, manageddata.Environment) (manageddata.EnvironmentPointer, error)
	ListRollouts(context.Context, string) ([]manageddata.Rollout, error)
	ListServingStateBindings(context.Context, string) ([]manageddata.ServingStateBinding, error)
}

// RolloutService is the domain orchestration surface used by the adapter.
type RolloutService interface {
	Create(context.Context, rollout.CreateRequest) (manageddata.Rollout, error)
	Get(context.Context, rollout.Scope) (manageddata.Rollout, error)
	Activate(context.Context, rollout.Scope) (manageddata.Rollout, error)
	Rollback(context.Context, rollout.Scope, string) (manageddata.Rollout, error)
}

// Adapter implements the managed-data HTTP metadata and rollout contracts.
type Adapter struct {
	repository Repository
	rollouts   RolloutService
}

func New(repository Repository, rollouts RolloutService) (*Adapter, error) {
	if repository == nil || rollouts == nil {
		return nil, fmt.Errorf("managed-data repository and rollout service are required")
	}
	return &Adapter{repository: repository, rollouts: rollouts}, nil
}

func (a *Adapter) CollectionByProjectConnection(ctx context.Context, project, connection string) (manageddata.Collection, error) {
	collection, err := a.repository.CollectionByProjectConnection(ctx, strings.TrimSpace(project), strings.TrimSpace(connection))
	if err != nil {
		return manageddata.Collection{}, publicError(err)
	}
	if collection.ProjectID != strings.TrimSpace(project) || collection.ConnectionName != strings.TrimSpace(connection) || collection.Status != manageddata.CollectionStatusActive {
		return manageddata.Collection{}, managedhttp.ErrNotFound
	}
	return collection, nil
}

// RevisionByID accepts only the public content-addressed revision identity.
func (a *Adapter) RevisionByID(ctx context.Context, publicID string) (managedhttp.RevisionMetadata, error) {
	publicID = strings.TrimSpace(publicID)
	if !canonicalRevisionID.MatchString(publicID) {
		return managedhttp.RevisionMetadata{}, managedhttp.ErrInvalid
	}
	revision, err := a.repository.RevisionByDigest(ctx, publicID)
	if err != nil {
		return managedhttp.RevisionMetadata{}, publicError(err)
	}
	return a.revisionMetadata(ctx, revision)
}

func (a *Adapter) ListRevisions(ctx context.Context, collectionID string) ([]managedhttp.RevisionMetadata, error) {
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
	out := make([]managedhttp.RevisionMetadata, 0, len(rows))
	for _, revision := range rows {
		if revision.CollectionID != collectionID {
			return nil, managedhttp.ErrBackend
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
		return manageddata.EnvironmentPointer{}, managedhttp.ErrNotFound
	}
	revision, err := a.repository.RevisionByID(ctx, pointer.RevisionID)
	if err != nil {
		return manageddata.EnvironmentPointer{}, publicError(err)
	}
	if revision.CollectionID != collectionID || revision.Status != manageddata.RevisionStatusReady || !canonicalRevisionID.MatchString(revision.Digest) {
		return manageddata.EnvironmentPointer{}, managedhttp.ErrBackend
	}
	pointer.RevisionID = revision.Digest
	return pointer, nil
}

func (a *Adapter) List(ctx context.Context, request managedhttp.RolloutListRequest) ([]managedhttp.Rollout, error) {
	collection, err := a.requestCollection(ctx, request.Project, request.Connection, request.CollectionID)
	if err != nil {
		return nil, err
	}
	status, statusFilter, err := domainStatusFilter(request.Status)
	if err != nil {
		return nil, err
	}
	if statusFilter && status == "" {
		return []managedhttp.Rollout{}, nil
	}
	rows, err := a.repository.ListRollouts(ctx, collection.ID)
	if err != nil {
		return nil, publicError(err)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt == rows[j].CreatedAt {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt > rows[j].CreatedAt
	})
	out := make([]managedhttp.Rollout, 0, len(rows))
	for _, row := range rows {
		if row.CollectionID != collection.ID {
			return nil, managedhttp.ErrBackend
		}
		if request.Environment != "" && string(row.Environment) != request.Environment || statusFilter && row.Status != status {
			continue
		}
		mapped, mapErr := a.mapRollout(ctx, row)
		if mapErr != nil {
			return nil, mapErr
		}
		out = append(out, mapped)
	}
	return out, nil
}

func (a *Adapter) Get(ctx context.Context, request managedhttp.RolloutRequest) (managedhttp.Rollout, error) {
	if _, err := a.requestCollection(ctx, request.Project, request.Connection, request.CollectionID); err != nil {
		return managedhttp.Rollout{}, err
	}
	row, err := a.rollouts.Get(ctx, rollout.Scope{Project: request.Project, Connection: request.Connection, RolloutID: request.RolloutID})
	if err != nil {
		return managedhttp.Rollout{}, publicError(err)
	}
	if row.ID != request.RolloutID || row.CollectionID != request.CollectionID {
		return managedhttp.Rollout{}, managedhttp.ErrNotFound
	}
	return a.mapRollout(ctx, row)
}

func (a *Adapter) Create(ctx context.Context, request managedhttp.RolloutCreateRequest) (managedhttp.Rollout, error) {
	collection, err := a.requestCollection(ctx, request.Project, request.Connection, request.CollectionID)
	if err != nil {
		return managedhttp.Rollout{}, err
	}
	if !canonicalRevisionID.MatchString(strings.TrimSpace(request.RevisionID)) || strings.TrimSpace(request.IdempotencyKey) == "" || strings.TrimSpace(request.Actor) == "" {
		return managedhttp.Rollout{}, managedhttp.ErrInvalid
	}
	revision, err := a.scopedRevisionByDigest(ctx, collection.ID, request.RevisionID)
	if err != nil {
		return managedhttp.Rollout{}, err
	}
	environment, err := manageddata.NormalizeEnvironment(request.Environment)
	if err != nil || len(request.Targets) == 0 {
		return managedhttp.Rollout{}, managedhttp.ErrInvalid
	}
	targets := make([]manageddata.RolloutTargetInput, len(request.Targets))
	seenWorkspaces := make(map[string]struct{}, len(request.Targets))
	for i, target := range request.Targets {
		workspace := strings.TrimSpace(target.Workspace)
		servingStateID := strings.TrimSpace(target.ServingStateID)
		if workspace == "" || servingStateID == "" {
			return managedhttp.Rollout{}, managedhttp.ErrInvalid
		}
		if _, exists := seenWorkspaces[workspace]; exists {
			return managedhttp.Rollout{}, managedhttp.ErrInvalid
		}
		seenWorkspaces[workspace] = struct{}{}
		targets[i] = manageddata.RolloutTargetInput{WorkspaceID: workspace, ServingStateID: servingStateID}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].WorkspaceID < targets[j].WorkspaceID })
	id := deterministicRolloutID(request.Project, request.Connection, request.Actor, request.IdempotencyKey)
	create := rollout.CreateRequest{
		ID: id, Project: request.Project, Connection: request.Connection, Environment: environment,
		RevisionID: revision.ID, Targets: targets, Actor: request.Actor,
	}
	row, err := a.rollouts.Create(ctx, create)
	if errors.Is(err, manageddata.ErrConflict) {
		row, err = a.rollouts.Get(ctx, rollout.Scope{Project: request.Project, Connection: request.Connection, RolloutID: id})
		if err == nil && !sameRolloutRequest(row, collection.ID, revision.ID, environment, targets, request.Actor) {
			return managedhttp.Rollout{}, managedhttp.ErrConflict
		}
	}
	if err != nil {
		return managedhttp.Rollout{}, publicError(err)
	}
	if row.ID != id || !sameRolloutRequest(row, collection.ID, revision.ID, environment, targets, request.Actor) {
		return managedhttp.Rollout{}, managedhttp.ErrBackend
	}
	return a.mapRollout(ctx, row)
}

func (a *Adapter) Activate(ctx context.Context, request managedhttp.RolloutRequest) (managedhttp.Rollout, error) {
	if _, err := a.requestCollection(ctx, request.Project, request.Connection, request.CollectionID); err != nil {
		return managedhttp.Rollout{}, err
	}
	row, err := a.rollouts.Activate(ctx, rollout.Scope{Project: request.Project, Connection: request.Connection, RolloutID: request.RolloutID})
	if err != nil {
		return managedhttp.Rollout{}, publicError(err)
	}
	if row.ID != request.RolloutID || row.CollectionID != request.CollectionID {
		return managedhttp.Rollout{}, managedhttp.ErrNotFound
	}
	return a.mapRollout(ctx, row)
}

func (a *Adapter) Rollback(ctx context.Context, request managedhttp.RolloutRollbackRequest) (managedhttp.Rollout, error) {
	if _, err := a.requestCollection(ctx, request.Project, request.Connection, request.CollectionID); err != nil {
		return managedhttp.Rollout{}, err
	}
	row, err := a.rollouts.Rollback(ctx, rollout.Scope{Project: request.Project, Connection: request.Connection, RolloutID: request.RolloutID}, request.Reason)
	if err != nil {
		return managedhttp.Rollout{}, publicError(err)
	}
	if row.CollectionID != request.CollectionID {
		return managedhttp.Rollout{}, managedhttp.ErrNotFound
	}
	mapped, err := a.mapRollout(ctx, row)
	if err != nil {
		return managedhttp.Rollout{}, err
	}
	// Rollback is implemented as a compensating rollout internally. The public
	// resource remains the rollout the operator requested to roll back.
	mapped.ID = request.RolloutID
	mapped.Status = managedhttp.RolloutStatusRolledBack
	mapped.RolledBackAt = row.CompletedAt
	for i := range mapped.Targets {
		mapped.Targets[i].Status = managedhttp.RolloutTargetStatusRolledBack
		mapped.Targets[i].RolledBackAt = row.CompletedAt
	}
	return mapped, nil
}

func (a *Adapter) requestCollection(ctx context.Context, project, connection, expectedID string) (manageddata.Collection, error) {
	collection, err := a.CollectionByProjectConnection(ctx, project, connection)
	if err != nil {
		return manageddata.Collection{}, err
	}
	if collection.ID != strings.TrimSpace(expectedID) {
		return manageddata.Collection{}, managedhttp.ErrNotFound
	}
	return collection, nil
}

func (a *Adapter) revisionMetadata(ctx context.Context, revision manageddata.Revision) (managedhttp.RevisionMetadata, error) {
	if revision.Status != manageddata.RevisionStatusReady || !canonicalRevisionID.MatchString(revision.Digest) {
		return managedhttp.RevisionMetadata{}, managedhttp.ErrNotFound
	}
	uploadID, err := a.repository.UploadSessionIDByRevisionID(ctx, revision.ID)
	if err != nil {
		return managedhttp.RevisionMetadata{}, publicError(err)
	}
	if strings.TrimSpace(uploadID) == "" {
		return managedhttp.RevisionMetadata{}, managedhttp.ErrBackend
	}
	revision.ID = revision.Digest
	return managedhttp.RevisionMetadata{Revision: revision, UploadSessionID: uploadID}, nil
}

func (a *Adapter) scopedRevisionByDigest(ctx context.Context, collectionID, digest string) (manageddata.Revision, error) {
	rows, err := a.repository.ListRevisions(ctx, collectionID)
	if err != nil {
		return manageddata.Revision{}, publicError(err)
	}
	var found *manageddata.Revision
	for i := range rows {
		if rows[i].CollectionID != collectionID {
			return manageddata.Revision{}, managedhttp.ErrBackend
		}
		if rows[i].Digest != digest || rows[i].Status != manageddata.RevisionStatusReady {
			continue
		}
		if found != nil {
			return manageddata.Revision{}, managedhttp.ErrBackend
		}
		copy := rows[i]
		found = &copy
	}
	if found == nil {
		return manageddata.Revision{}, managedhttp.ErrNotFound
	}
	return *found, nil
}

func (a *Adapter) mapRollout(ctx context.Context, row manageddata.Rollout) (managedhttp.Rollout, error) {
	revision, err := a.repository.RevisionByID(ctx, row.RevisionID)
	if err != nil {
		return managedhttp.Rollout{}, publicError(err)
	}
	if revision.CollectionID != row.CollectionID || revision.Status != manageddata.RevisionStatusReady || !canonicalRevisionID.MatchString(revision.Digest) {
		return managedhttp.Rollout{}, managedhttp.ErrBackend
	}
	status, err := publicRolloutStatus(row.Status)
	if err != nil {
		return managedhttp.Rollout{}, err
	}
	targets := append([]manageddata.RolloutTarget(nil), row.Targets...)
	sort.Slice(targets, func(i, j int) bool { return targets[i].WorkspaceID < targets[j].WorkspaceID })
	mappedTargets := make([]managedhttp.RolloutTarget, len(targets))
	for i, target := range targets {
		targetStatus, statusErr := publicTargetStatus(target.Status, row.Status)
		if statusErr != nil {
			return managedhttp.Rollout{}, statusErr
		}
		previous, previousErr := a.previousRevisionDigest(ctx, row.CollectionID, target.PriorServingStateID)
		if previousErr != nil {
			return managedhttp.Rollout{}, previousErr
		}
		mappedTargets[i] = managedhttp.RolloutTarget{
			Workspace: target.WorkspaceID, ServingStateID: target.ServingStateID, Status: targetStatus,
			PreviousRevisionID: previous, ActivatedAt: target.ActivatedAt,
		}
		if row.Status == manageddata.RolloutStatusSuperseded {
			mappedTargets[i].RolledBackAt = row.CompletedAt
		}
	}
	mapped := managedhttp.Rollout{
		ID: row.ID, CollectionID: row.CollectionID, RevisionID: revision.Digest, Environment: string(row.Environment),
		Status: status, Targets: mappedTargets, CreatedAt: row.CreatedAt,
	}
	switch row.Status {
	case manageddata.RolloutStatusActive:
		mapped.ActivatedAt = row.CompletedAt
	case manageddata.RolloutStatusFailed:
		mapped.Error = "managed-data rollout failed"
	case manageddata.RolloutStatusSuperseded:
		mapped.ActivatedAt = row.CompletedAt
		mapped.RolledBackAt = row.CompletedAt
	}
	return mapped, nil
}

func (a *Adapter) previousRevisionDigest(ctx context.Context, collectionID, servingStateID string) (string, error) {
	if strings.TrimSpace(servingStateID) == "" {
		return "", nil
	}
	bindings, err := a.repository.ListServingStateBindings(ctx, servingStateID)
	if err != nil {
		return "", publicError(err)
	}
	for _, binding := range bindings {
		if binding.CollectionID != collectionID {
			continue
		}
		revision, revisionErr := a.repository.RevisionByID(ctx, binding.RevisionID)
		if revisionErr != nil {
			return "", publicError(revisionErr)
		}
		if revision.CollectionID != collectionID || revision.Status != manageddata.RevisionStatusReady || !canonicalRevisionID.MatchString(revision.Digest) {
			return "", managedhttp.ErrBackend
		}
		return revision.Digest, nil
	}
	return "", nil
}

func publicRolloutStatus(status manageddata.RolloutStatus) (managedhttp.RolloutStatus, error) {
	switch status {
	case manageddata.RolloutStatusPending:
		return managedhttp.RolloutStatusDraft, nil
	case manageddata.RolloutStatusActive:
		return managedhttp.RolloutStatusActive, nil
	case manageddata.RolloutStatusFailed:
		return managedhttp.RolloutStatusFailed, nil
	case manageddata.RolloutStatusSuperseded:
		return managedhttp.RolloutStatusRolledBack, nil
	default:
		return "", managedhttp.ErrBackend
	}
}

func publicTargetStatus(status manageddata.TargetStatus, rolloutStatus manageddata.RolloutStatus) (managedhttp.RolloutTargetStatus, error) {
	if rolloutStatus == manageddata.RolloutStatusSuperseded {
		return managedhttp.RolloutTargetStatusRolledBack, nil
	}
	switch status {
	case manageddata.TargetStatusPending:
		return managedhttp.RolloutTargetStatusPending, nil
	case manageddata.TargetStatusActive:
		return managedhttp.RolloutTargetStatusActive, nil
	case manageddata.TargetStatusFailed:
		return managedhttp.RolloutTargetStatusFailed, nil
	default:
		return "", managedhttp.ErrBackend
	}
}

func domainStatusFilter(status managedhttp.RolloutStatus) (manageddata.RolloutStatus, bool, error) {
	switch status {
	case "":
		return "", false, nil
	case managedhttp.RolloutStatusDraft:
		return manageddata.RolloutStatusPending, true, nil
	case managedhttp.RolloutStatusActive:
		return manageddata.RolloutStatusActive, true, nil
	case managedhttp.RolloutStatusFailed:
		return manageddata.RolloutStatusFailed, true, nil
	case managedhttp.RolloutStatusRolledBack:
		return manageddata.RolloutStatusSuperseded, true, nil
	case managedhttp.RolloutStatusActivating, managedhttp.RolloutStatusRollingBack:
		return "", true, nil
	default:
		return "", false, managedhttp.ErrInvalid
	}
}

func sameRolloutRequest(row manageddata.Rollout, collectionID, revisionID string, environment manageddata.Environment, targets []manageddata.RolloutTargetInput, actor string) bool {
	if row.CollectionID != collectionID || row.RevisionID != revisionID || row.Environment != environment || row.CreatedBy != strings.TrimSpace(actor) || len(row.Targets) != len(targets) {
		return false
	}
	actual := append([]manageddata.RolloutTarget(nil), row.Targets...)
	sort.Slice(actual, func(i, j int) bool { return actual[i].WorkspaceID < actual[j].WorkspaceID })
	for i := range targets {
		if actual[i].WorkspaceID != targets[i].WorkspaceID || actual[i].ServingStateID != targets[i].ServingStateID {
			return false
		}
	}
	return true
}

func deterministicRolloutID(project, connection, actor, idempotencyKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(project) + "\x00" + strings.TrimSpace(connection) + "\x00" + strings.TrimSpace(actor) + "\x00" + strings.TrimSpace(idempotencyKey)))
	return "rollout_" + hex.EncodeToString(sum[:])
}

func publicError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, manageddata.ErrNotFound), errors.Is(err, managedhttp.ErrNotFound):
		return managedhttp.ErrNotFound
	case errors.Is(err, manageddata.ErrConflict), errors.Is(err, managedhttp.ErrConflict):
		return managedhttp.ErrConflict
	case errors.Is(err, managedhttp.ErrInvalid):
		return managedhttp.ErrInvalid
	default:
		return managedhttp.ErrBackend
	}
}

var _ managedhttp.Repository = (*Adapter)(nil)
var _ managedhttp.RolloutCoordinator = (*Adapter)(nil)
