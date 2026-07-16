package sqlite

import (
	"context"
	"database/sql"
)

type ProjectRecord struct {
	ID, CreatedAt, UpdatedAt, LatestReleaseID, ActiveDeploymentID string
}

type WorkspaceRecord struct {
	ID, Title, Description, ActiveServingStateID string
}

type ConnectionRecord struct {
	ID, Title, Description, ActiveRevisionID string
}

func (r *Repository) ListProjects(ctx context.Context) ([]ProjectRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT project_id, MIN(created_at), MAX(updated_at) FROM (
      SELECT project_id, created_at, COALESCE(finalized_at, created_at) AS updated_at FROM api_releases
      UNION ALL SELECT project_id, created_at, updated_at FROM managed_data_collections
    ) GROUP BY project_id ORDER BY project_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ProjectRecord{}
	for rows.Next() {
		var item ProjectRecord
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		r.populateProjectPointers(ctx, &item)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) GetProject(ctx context.Context, projectID string) (ProjectRecord, error) {
	item := ProjectRecord{ID: projectID}
	err := r.db.QueryRowContext(ctx, `SELECT MIN(created_at), MAX(updated_at) FROM (
      SELECT created_at, COALESCE(finalized_at, created_at) AS updated_at FROM api_releases WHERE project_id = ?
      UNION ALL SELECT created_at, updated_at FROM managed_data_collections WHERE project_id = ?
    )`, projectID, projectID).Scan(&item.CreatedAt, &item.UpdatedAt)
	if err != nil || item.CreatedAt == "" {
		return ProjectRecord{}, sql.ErrNoRows
	}
	r.populateProjectPointers(ctx, &item)
	return item, nil
}

func (r *Repository) populateProjectPointers(ctx context.Context, item *ProjectRecord) {
	_ = r.db.QueryRowContext(ctx, `SELECT id FROM api_releases WHERE project_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`, item.ID).Scan(&item.LatestReleaseID)
	_ = r.db.QueryRowContext(ctx, `SELECT d.id FROM project_deployments d JOIN api_deployment_releases l ON l.deployment_id = d.id WHERE l.project_id = ? AND d.status = 'active' ORDER BY d.activated_at DESC LIMIT 1`, item.ID).Scan(&item.ActiveDeploymentID)
}

func (r *Repository) ListProjectWorkspaces(ctx context.Context, projectID, environment string) ([]WorkspaceRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT a.workspace_id, COALESCE(w.title, a.workspace_id), COALESCE(w.description, ''),
    COALESCE(active.serving_state_id, '') FROM api_release_artifacts a JOIN api_releases rel ON rel.id = a.release_id
    LEFT JOIN workspaces w ON w.id = a.workspace_id
    LEFT JOIN workspace_active_serving_states active ON active.workspace_id = a.workspace_id AND active.environment = ?
    WHERE rel.project_id = ? ORDER BY a.workspace_id`, environment, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []WorkspaceRecord{}
	for rows.Next() {
		var item WorkspaceRecord
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.ActiveServingStateID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ListConnections(ctx context.Context, projectID, environment string) ([]ConnectionRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.connection_name, c.name, c.description, COALESCE(rev.digest, '')
    FROM managed_data_collections c LEFT JOIN managed_data_environment_pointers ptr ON ptr.collection_id = c.id AND ptr.environment = ?
    LEFT JOIN managed_data_revisions rev ON rev.id = ptr.revision_id WHERE c.project_id = ? AND c.status = 'active' ORDER BY c.connection_name`, environment, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ConnectionRecord{}
	for rows.Next() {
		var item ConnectionRecord
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.ActiveRevisionID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) GetConnection(ctx context.Context, projectID, connectionID, environment string) (ConnectionRecord, error) {
	item := ConnectionRecord{ID: connectionID}
	err := r.db.QueryRowContext(ctx, `SELECT c.name, c.description, COALESCE(rev.digest, '') FROM managed_data_collections c
    LEFT JOIN managed_data_environment_pointers ptr ON ptr.collection_id = c.id AND ptr.environment = ?
    LEFT JOIN managed_data_revisions rev ON rev.id = ptr.revision_id
    WHERE c.project_id = ? AND c.connection_name = ? AND c.status = 'active'`, environment, projectID, connectionID).
		Scan(&item.Title, &item.Description, &item.ActiveRevisionID)
	return item, err
}
