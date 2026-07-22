package sqlite

import (
	"context"
	"database/sql"

	platformdb "github.com/Yacobolo/leapview/internal/platform/db"
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
	rows, err := r.q.ListAPIProjects(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ProjectRecord, 0, len(rows))
	for _, row := range rows {
		item := ProjectRecord{ID: row.ProjectID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
		r.populateProjectPointers(ctx, &item)
		result = append(result, item)
	}
	return result, nil
}

func (r *Repository) GetProject(ctx context.Context, projectID string) (ProjectRecord, error) {
	item := ProjectRecord{ID: projectID}
	row, err := r.q.GetAPIProject(ctx, projectID)
	item.CreatedAt, item.UpdatedAt = row.CreatedAt, row.UpdatedAt
	if err != nil || item.CreatedAt == "" {
		return ProjectRecord{}, sql.ErrNoRows
	}
	r.populateProjectPointers(ctx, &item)
	return item, nil
}

func (r *Repository) populateProjectPointers(ctx context.Context, item *ProjectRecord) {
	item.LatestReleaseID, _ = r.q.GetLatestAPIProjectReleaseID(ctx, item.ID)
	item.ActiveDeploymentID, _ = r.q.GetActiveAPIProjectDeploymentID(ctx, item.ID)
}

func (r *Repository) ListProjectWorkspaces(ctx context.Context, projectID, environment string) ([]WorkspaceRecord, error) {
	rows, err := r.q.ListAPIProjectWorkspaces(ctx, platformdb.ListAPIProjectWorkspacesParams{Environment: environment, ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	result := make([]WorkspaceRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, WorkspaceRecord{ID: row.WorkspaceID, Title: row.Title, Description: row.Description, ActiveServingStateID: row.ActiveServingStateID})
	}
	return result, nil
}

func (r *Repository) ListConnections(ctx context.Context, projectID, environment string) ([]ConnectionRecord, error) {
	rows, err := r.q.ListAPIProjectConnections(ctx, platformdb.ListAPIProjectConnectionsParams{Environment: environment, ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	result := make([]ConnectionRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, ConnectionRecord{ID: row.ConnectionName, Title: row.Name, Description: row.Description, ActiveRevisionID: row.ActiveRevisionID})
	}
	return result, nil
}

func (r *Repository) GetConnection(ctx context.Context, projectID, connectionID, environment string) (ConnectionRecord, error) {
	item := ConnectionRecord{ID: connectionID}
	row, err := r.q.GetAPIProjectConnection(ctx, platformdb.GetAPIProjectConnectionParams{Environment: environment, ProjectID: projectID, ConnectionName: connectionID})
	item.Title, item.Description, item.ActiveRevisionID = row.Name, row.Description, row.ActiveRevisionID
	return item, err
}
