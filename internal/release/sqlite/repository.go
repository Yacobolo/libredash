// Package sqlite persists API releases in the platform SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/release"
)

type Repository struct {
	db *sql.DB
	q  *platformdb.Queries
}

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db, q: platformdb.New(db)} }

func (r *Repository) Create(ctx context.Context, input release.CreateInput) (release.Release, error) {
	if err := normalizeCreate(&input); err != nil {
		return release.Release{}, err
	}
	manifest := release.Manifest{Workspaces: input.Workspaces, Connections: input.Connections}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return release.Release{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return release.Release{}, err
	}
	defer tx.Rollback()
	qtx := r.q.WithTx(tx)
	err = qtx.CreateAPIRelease(ctx, platformdb.CreateAPIReleaseParams{ID: input.ID, ProjectID: input.ProjectID, ProjectDigest: input.ProjectDigest,
		RequestDigest: input.RequestDigest, IdempotencyKey: input.IdempotencyKey, ManifestJson: string(encoded), CreatedBy: input.CreatedBy})
	if err != nil {
		existing, getErr := getWith(ctx, qtx, input.ProjectID, "", input.IdempotencyKey)
		if getErr == nil {
			if existing.RequestDigest != input.RequestDigest {
				return release.Release{}, release.ErrConflict
			}
			return existing, nil
		}
		return release.Release{}, err
	}
	for _, workspace := range input.Workspaces {
		if err := qtx.CreateAPIReleaseArtifact(ctx, platformdb.CreateAPIReleaseArtifactParams{ReleaseID: input.ID, WorkspaceID: workspace.WorkspaceID, ExpectedDigest: workspace.ArtifactDigest}); err != nil {
			return release.Release{}, err
		}
	}
	for _, connection := range input.Connections {
		if err := qtx.CreateAPIReleaseConnection(ctx, platformdb.CreateAPIReleaseConnectionParams{ReleaseID: input.ID, ConnectionID: connection.ConnectionID, RevisionID: connection.RevisionID}); err != nil {
			return release.Release{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return release.Release{}, err
	}
	return r.Get(ctx, input.ProjectID, input.ID)
}

func (r *Repository) Get(ctx context.Context, projectID, releaseID string) (release.Release, error) {
	return getWith(ctx, r.q, strings.TrimSpace(projectID), strings.TrimSpace(releaseID), "")
}

func (r *Repository) List(ctx context.Context, projectID string) ([]release.Release, error) {
	ids, err := r.q.ListAPIReleaseIDs(ctx, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	result := make([]release.Release, 0, len(ids))
	for _, id := range ids {
		item, err := r.Get(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *Repository) RecordArtifact(ctx context.Context, artifact release.Artifact) error {
	if strings.TrimSpace(artifact.ReleaseID) == "" || strings.TrimSpace(artifact.WorkspaceID) == "" || strings.TrimSpace(artifact.ServingStateID) == "" || artifact.SizeBytes < 0 {
		return release.ErrInvalid
	}
	state, err := r.q.GetAPIReleaseArtifactUploadState(ctx, platformdb.GetAPIReleaseArtifactUploadStateParams{ID: artifact.ReleaseID, WorkspaceID: artifact.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return release.ErrNotFound
	}
	if err != nil {
		return err
	}
	if release.Status(state.Status) != release.StatusDraft {
		return release.ErrImmutable
	}
	if artifact.ExpectedDigest != "" && artifact.ExpectedDigest != state.ExpectedDigest {
		return release.ErrConflict
	}
	changed, err := r.q.RecordAPIReleaseArtifact(ctx, platformdb.RecordAPIReleaseArtifactParams{SizeBytes: artifact.SizeBytes, ReleaseID: artifact.ReleaseID, WorkspaceID: artifact.WorkspaceID, ServingStateID: sql.NullString{String: artifact.ServingStateID, Valid: true}})
	if err != nil {
		return err
	}
	if changed != 1 {
		return release.ErrConflict
	}
	return nil
}

func (r *Repository) AssignArtifactTarget(ctx context.Context, projectID, releaseID, workspaceID, servingStateID string) error {
	changed, err := r.q.AssignAPIReleaseArtifactTarget(ctx, platformdb.AssignAPIReleaseArtifactTargetParams{ServingStateID: sql.NullString{String: strings.TrimSpace(servingStateID), Valid: true}, ReleaseID: releaseID, WorkspaceID: workspaceID, ID: releaseID, ProjectID: projectID})
	if err != nil {
		return err
	}
	if changed != 1 {
		return release.ErrConflict
	}
	return nil
}

func (r *Repository) BeginFinalization(ctx context.Context, projectID, releaseID string) (release.Release, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return release.Release{}, err
	}
	defer tx.Rollback()
	qtx := r.q.WithTx(tx)
	current, err := getWith(ctx, qtx, projectID, releaseID, "")
	if err != nil {
		return release.Release{}, err
	}
	if current.Status == release.StatusValidating {
		return current, nil
	}
	if current.Status != release.StatusDraft {
		return release.Release{}, release.ErrImmutable
	}
	for _, artifact := range current.Artifacts {
		if artifact.ServingStateID == "" || artifact.UploadedAt == "" {
			return release.Release{}, release.ErrIncomplete
		}
	}
	if _, err := qtx.MarkAPIReleaseValidating(ctx, platformdb.MarkAPIReleaseValidatingParams{ID: releaseID, ProjectID: projectID}); err != nil {
		return release.Release{}, err
	}
	if err := tx.Commit(); err != nil {
		return release.Release{}, err
	}
	return r.Get(ctx, projectID, releaseID)
}

func (r *Repository) CompleteFinalization(ctx context.Context, projectID, releaseID string, digests map[string]string) (release.Release, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return release.Release{}, err
	}
	defer tx.Rollback()
	qtx := r.q.WithTx(tx)
	current, err := getWith(ctx, qtx, projectID, releaseID, "")
	if err != nil {
		return release.Release{}, err
	}
	if current.Status != release.StatusValidating {
		return release.Release{}, release.ErrImmutable
	}
	for _, artifact := range current.Artifacts {
		digest, ok := digests[artifact.WorkspaceID]
		if !ok || digest != artifact.ExpectedDigest {
			return release.Release{}, release.ErrConflict
		}
		if err := qtx.SetAPIReleaseArtifactDigest(ctx, platformdb.SetAPIReleaseArtifactDigestParams{ActualDigest: digest, ReleaseID: releaseID, WorkspaceID: artifact.WorkspaceID}); err != nil {
			return release.Release{}, err
		}
	}
	if _, err := qtx.MarkAPIReleaseReady(ctx, platformdb.MarkAPIReleaseReadyParams{ID: releaseID, ProjectID: projectID}); err != nil {
		return release.Release{}, err
	}
	if err := tx.Commit(); err != nil {
		return release.Release{}, err
	}
	return r.Get(ctx, projectID, releaseID)
}

func (r *Repository) FailFinalization(ctx context.Context, projectID, releaseID string, cause error) (release.Release, error) {
	message := "release validation failed"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		message = cause.Error()
	}
	changed, err := r.q.MarkAPIReleaseFailed(ctx, platformdb.MarkAPIReleaseFailedParams{Error: message, ID: releaseID, ProjectID: projectID})
	if err != nil {
		return release.Release{}, err
	}
	if changed != 1 {
		return release.Release{}, release.ErrImmutable
	}
	return r.Get(ctx, projectID, releaseID)
}

func (r *Repository) LinkDeployment(ctx context.Context, projectID, deploymentID, releaseID, rollbackOf string) error {
	rollbackValue := strings.TrimSpace(rollbackOf)
	return r.q.LinkAPIReleaseDeployment(ctx, platformdb.LinkAPIReleaseDeploymentParams{DeploymentID: strings.TrimSpace(deploymentID), ProjectID: strings.TrimSpace(projectID), ReleaseID: strings.TrimSpace(releaseID), RollbackOf: sql.NullString{String: rollbackValue, Valid: rollbackValue != ""}})
}

func (r *Repository) DeploymentRelease(ctx context.Context, projectID, deploymentID string) (string, string, error) {
	row, err := r.q.GetAPIReleaseDeployment(ctx, platformdb.GetAPIReleaseDeploymentParams{ProjectID: strings.TrimSpace(projectID), DeploymentID: strings.TrimSpace(deploymentID)})
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", release.ErrNotFound
	}
	return row.ReleaseID, row.RollbackOf, err
}

func (r *Repository) ListDeploymentIDs(ctx context.Context, projectID string) ([]string, error) {
	return r.q.ListAPIReleaseDeploymentIDs(ctx, strings.TrimSpace(projectID))
}

func (r *Repository) PriorDeploymentRelease(ctx context.Context, projectID, deploymentID string) (string, error) {
	releaseID, err := r.q.GetPriorAPIReleaseDeployment(ctx, platformdb.GetPriorAPIReleaseDeploymentParams{ProjectID: strings.TrimSpace(projectID), DeploymentID: strings.TrimSpace(deploymentID)})
	if errors.Is(err, sql.ErrNoRows) {
		return "", release.ErrNotFound
	}
	return releaseID, err
}

func getWith(ctx context.Context, q *platformdb.Queries, projectID, releaseID, idempotencyKey string) (release.Release, error) {
	var raw releaseRow
	var err error
	if idempotencyKey != "" {
		row, queryErr := q.GetAPIReleaseByIdempotencyKey(ctx, platformdb.GetAPIReleaseByIdempotencyKeyParams{ProjectID: projectID, IdempotencyKey: idempotencyKey})
		err = queryErr
		raw = releaseRow{row.ID, row.ProjectID, row.ProjectDigest, row.RequestDigest, row.IdempotencyKey, row.Status, row.ManifestJson, row.CreatedBy, row.CreatedAt, row.FinalizedAt, row.Error}
	} else {
		row, queryErr := q.GetAPIReleaseByID(ctx, platformdb.GetAPIReleaseByIDParams{ProjectID: projectID, ID: releaseID})
		err = queryErr
		raw = releaseRow{row.ID, row.ProjectID, row.ProjectDigest, row.RequestDigest, row.IdempotencyKey, row.Status, row.ManifestJson, row.CreatedBy, row.CreatedAt, row.FinalizedAt, row.Error}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return release.Release{}, release.ErrNotFound
	}
	if err != nil {
		return release.Release{}, err
	}
	item := release.Release{ID: raw.id, ProjectID: raw.projectID, ProjectDigest: raw.projectDigest, RequestDigest: raw.requestDigest,
		IdempotencyKey: raw.idempotencyKey, Status: release.Status(raw.status), CreatedBy: raw.createdBy, CreatedAt: raw.createdAt, FinalizedAt: raw.finalizedAt, Error: raw.errorText}
	if err := json.Unmarshal([]byte(raw.manifestJSON), &item.Manifest); err != nil {
		return release.Release{}, err
	}
	rows, err := q.GetAPIReleaseArtifacts(ctx, item.ID)
	if err != nil {
		return release.Release{}, err
	}
	for _, row := range rows {
		artifact := release.Artifact{ReleaseID: item.ID, WorkspaceID: row.WorkspaceID, ExpectedDigest: row.ExpectedDigest,
			ServingStateID: row.ServingStateID, ActualDigest: row.ActualDigest, SizeBytes: row.SizeBytes, UploadedAt: row.UploadedAt}
		item.Artifacts = append(item.Artifacts, artifact)
	}
	for i := range item.Manifest.Workspaces {
		for _, artifact := range item.Artifacts {
			if artifact.WorkspaceID == item.Manifest.Workspaces[i].WorkspaceID {
				item.Manifest.Workspaces[i].ServingStateID = artifact.ServingStateID
				break
			}
		}
	}
	return item, nil
}

type releaseRow struct {
	id, projectID, projectDigest, requestDigest, idempotencyKey, status, manifestJSON string
	createdBy, createdAt, finalizedAt, errorText                                      string
}

func normalizeCreate(input *release.CreateInput) error {
	input.ID = strings.TrimSpace(input.ID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.ProjectDigest = strings.TrimSpace(input.ProjectDigest)
	input.RequestDigest = strings.TrimSpace(input.RequestDigest)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	if input.ID == "" || input.ProjectID == "" || input.ProjectDigest == "" || input.RequestDigest == "" || input.IdempotencyKey == "" || input.CreatedBy == "" || len(input.Workspaces) == 0 {
		return release.ErrInvalid
	}
	sort.Slice(input.Workspaces, func(i, j int) bool { return input.Workspaces[i].WorkspaceID < input.Workspaces[j].WorkspaceID })
	sort.Slice(input.Connections, func(i, j int) bool { return input.Connections[i].ConnectionID < input.Connections[j].ConnectionID })
	seen := map[string]struct{}{}
	for i := range input.Workspaces {
		input.Workspaces[i].WorkspaceID = strings.TrimSpace(input.Workspaces[i].WorkspaceID)
		input.Workspaces[i].ArtifactDigest = strings.TrimSpace(input.Workspaces[i].ArtifactDigest)
		if input.Workspaces[i].WorkspaceID == "" || input.Workspaces[i].ArtifactDigest == "" {
			return release.ErrInvalid
		}
		if _, ok := seen[input.Workspaces[i].WorkspaceID]; ok {
			return release.ErrInvalid
		}
		seen[input.Workspaces[i].WorkspaceID] = struct{}{}
	}
	seen = map[string]struct{}{}
	for i := range input.Connections {
		input.Connections[i].ConnectionID = strings.TrimSpace(input.Connections[i].ConnectionID)
		input.Connections[i].RevisionID = strings.TrimSpace(input.Connections[i].RevisionID)
		if input.Connections[i].ConnectionID == "" || input.Connections[i].RevisionID == "" {
			return release.ErrInvalid
		}
		if _, ok := seen[input.Connections[i].ConnectionID]; ok {
			return release.ErrInvalid
		}
		seen[input.Connections[i].ConnectionID] = struct{}{}
	}
	if len(input.Workspaces) > 200 || len(input.Connections) > 200 {
		return fmt.Errorf("%w: manifest exceeds 200 resources", release.ErrInvalid)
	}
	return nil
}
