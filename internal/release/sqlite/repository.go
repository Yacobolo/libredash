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

	"github.com/Yacobolo/libredash/internal/release"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

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
	_, err = tx.ExecContext(ctx, `INSERT INTO api_releases
      (id, project_id, project_digest, request_digest, idempotency_key, status, manifest_json, created_by)
      VALUES (?, ?, ?, ?, ?, 'draft', ?, ?)`, input.ID, input.ProjectID, input.ProjectDigest, input.RequestDigest, input.IdempotencyKey, string(encoded), input.CreatedBy)
	if err != nil {
		existing, getErr := getWith(ctx, tx, input.ProjectID, "", input.IdempotencyKey)
		if getErr == nil {
			if existing.RequestDigest != input.RequestDigest {
				return release.Release{}, release.ErrConflict
			}
			return existing, nil
		}
		return release.Release{}, err
	}
	for _, workspace := range input.Workspaces {
		if _, err := tx.ExecContext(ctx, `INSERT INTO api_release_artifacts (release_id, workspace_id, expected_digest) VALUES (?, ?, ?)`, input.ID, workspace.WorkspaceID, workspace.ArtifactDigest); err != nil {
			return release.Release{}, err
		}
	}
	for _, connection := range input.Connections {
		if _, err := tx.ExecContext(ctx, `INSERT INTO api_release_connections (release_id, connection_id, revision_id) VALUES (?, ?, ?)`, input.ID, connection.ConnectionID, connection.RevisionID); err != nil {
			return release.Release{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return release.Release{}, err
	}
	return r.Get(ctx, input.ProjectID, input.ID)
}

func (r *Repository) Get(ctx context.Context, projectID, releaseID string) (release.Release, error) {
	return getWith(ctx, r.db, strings.TrimSpace(projectID), strings.TrimSpace(releaseID), "")
}

func (r *Repository) List(ctx context.Context, projectID string) ([]release.Release, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM api_releases WHERE project_id = ? ORDER BY created_at DESC, id DESC`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
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
	var status string
	var expected string
	err := r.db.QueryRowContext(ctx, `SELECT r.status, a.expected_digest FROM api_releases r JOIN api_release_artifacts a ON a.release_id = r.id WHERE r.id = ? AND a.workspace_id = ?`, artifact.ReleaseID, artifact.WorkspaceID).Scan(&status, &expected)
	if errors.Is(err, sql.ErrNoRows) {
		return release.ErrNotFound
	}
	if err != nil {
		return err
	}
	if release.Status(status) != release.StatusDraft {
		return release.ErrImmutable
	}
	if artifact.ExpectedDigest != "" && artifact.ExpectedDigest != expected {
		return release.ErrConflict
	}
	result, err := r.db.ExecContext(ctx, `UPDATE api_release_artifacts SET size_bytes = ?, uploaded_at = CURRENT_TIMESTAMP WHERE release_id = ? AND workspace_id = ? AND serving_state_id = ? AND uploaded_at IS NULL`, artifact.SizeBytes, artifact.ReleaseID, artifact.WorkspaceID, artifact.ServingStateID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return release.ErrConflict
	}
	return nil
}

func (r *Repository) AssignArtifactTarget(ctx context.Context, projectID, releaseID, workspaceID, servingStateID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE api_release_artifacts SET serving_state_id = ?
    WHERE release_id = ? AND workspace_id = ? AND serving_state_id IS NULL
      AND EXISTS (SELECT 1 FROM api_releases WHERE id = ? AND project_id = ? AND status = 'draft')`,
		strings.TrimSpace(servingStateID), releaseID, workspaceID, releaseID, projectID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
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
	current, err := getWith(ctx, tx, projectID, releaseID, "")
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
	if _, err := tx.ExecContext(ctx, `UPDATE api_releases SET status = 'validating' WHERE id = ? AND project_id = ? AND status = 'draft'`, releaseID, projectID); err != nil {
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
	current, err := getWith(ctx, tx, projectID, releaseID, "")
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
		if _, err := tx.ExecContext(ctx, `UPDATE api_release_artifacts SET actual_digest = ? WHERE release_id = ? AND workspace_id = ?`, digest, releaseID, artifact.WorkspaceID); err != nil {
			return release.Release{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE api_releases SET status = 'ready', finalized_at = CURRENT_TIMESTAMP WHERE id = ? AND project_id = ? AND status = 'validating'`, releaseID, projectID); err != nil {
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
	result, err := r.db.ExecContext(ctx, `UPDATE api_releases SET status = 'failed', error = ?, finalized_at = CURRENT_TIMESTAMP WHERE id = ? AND project_id = ? AND status = 'validating'`, message, releaseID, projectID)
	if err != nil {
		return release.Release{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return release.Release{}, release.ErrImmutable
	}
	return r.Get(ctx, projectID, releaseID)
}

func (r *Repository) LinkDeployment(ctx context.Context, projectID, deploymentID, releaseID, rollbackOf string) error {
	var rollback any
	if strings.TrimSpace(rollbackOf) != "" {
		rollback = strings.TrimSpace(rollbackOf)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO api_deployment_releases (deployment_id, project_id, release_id, rollback_of) VALUES (?, ?, ?, ?)
    ON CONFLICT(deployment_id) DO UPDATE SET release_id = excluded.release_id
    WHERE api_deployment_releases.project_id = excluded.project_id AND api_deployment_releases.release_id = excluded.release_id`,
		strings.TrimSpace(deploymentID), strings.TrimSpace(projectID), strings.TrimSpace(releaseID), rollback)
	return err
}

func (r *Repository) DeploymentRelease(ctx context.Context, projectID, deploymentID string) (string, string, error) {
	var releaseID, rollbackOf string
	err := r.db.QueryRowContext(ctx, `SELECT release_id, COALESCE(rollback_of, '') FROM api_deployment_releases WHERE project_id = ? AND deployment_id = ?`, strings.TrimSpace(projectID), strings.TrimSpace(deploymentID)).Scan(&releaseID, &rollbackOf)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", release.ErrNotFound
	}
	return releaseID, rollbackOf, err
}

func (r *Repository) ListDeploymentIDs(ctx context.Context, projectID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT deployment_id FROM api_deployment_releases WHERE project_id = ? ORDER BY created_at DESC, deployment_id DESC`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) PriorDeploymentRelease(ctx context.Context, projectID, deploymentID string) (string, error) {
	var releaseID string
	err := r.db.QueryRowContext(ctx, `SELECT prior.release_id
    FROM api_deployment_releases current
    JOIN api_deployment_releases prior ON prior.project_id = current.project_id AND prior.created_at < current.created_at
    WHERE current.project_id = ? AND current.deployment_id = ?
    ORDER BY prior.created_at DESC, prior.deployment_id DESC LIMIT 1`, strings.TrimSpace(projectID), strings.TrimSpace(deploymentID)).Scan(&releaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", release.ErrNotFound
	}
	return releaseID, err
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func getWith(ctx context.Context, q queryer, projectID, releaseID, idempotencyKey string) (release.Release, error) {
	query := `SELECT id, project_id, project_digest, request_digest, idempotency_key, status, manifest_json, created_by, created_at, COALESCE(finalized_at, ''), error FROM api_releases WHERE project_id = ? AND id = ?`
	args := []any{projectID, releaseID}
	if idempotencyKey != "" {
		query = `SELECT id, project_id, project_digest, request_digest, idempotency_key, status, manifest_json, created_by, created_at, COALESCE(finalized_at, ''), error FROM api_releases WHERE project_id = ? AND idempotency_key = ?`
		args = []any{projectID, idempotencyKey}
	}
	var item release.Release
	var status, manifestJSON string
	err := q.QueryRowContext(ctx, query, args...).Scan(&item.ID, &item.ProjectID, &item.ProjectDigest, &item.RequestDigest, &item.IdempotencyKey, &status, &manifestJSON, &item.CreatedBy, &item.CreatedAt, &item.FinalizedAt, &item.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return release.Release{}, release.ErrNotFound
	}
	if err != nil {
		return release.Release{}, err
	}
	item.Status = release.Status(status)
	if err := json.Unmarshal([]byte(manifestJSON), &item.Manifest); err != nil {
		return release.Release{}, err
	}
	rows, err := q.QueryContext(ctx, `SELECT workspace_id, expected_digest, COALESCE(serving_state_id, ''), actual_digest, size_bytes, COALESCE(uploaded_at, '') FROM api_release_artifacts WHERE release_id = ? ORDER BY workspace_id`, item.ID)
	if err != nil {
		return release.Release{}, err
	}
	defer rows.Close()
	for rows.Next() {
		artifact := release.Artifact{ReleaseID: item.ID}
		if err := rows.Scan(&artifact.WorkspaceID, &artifact.ExpectedDigest, &artifact.ServingStateID, &artifact.ActualDigest, &artifact.SizeBytes, &artifact.UploadedAt); err != nil {
			return release.Release{}, err
		}
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
	return item, rows.Err()
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
