-- name: UpsertWorkspace :exec
INSERT INTO workspaces (id, title, description, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
  title = excluded.title,
  description = excluded.description,
  updated_at = CURRENT_TIMESTAMP;
-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = ?;

-- name: ListWorkspaces :many
SELECT * FROM workspaces ORDER BY created_at;

-- name: ListWorkspacesWithActiveMetadata :many
SELECT
  w.id,
  CASE WHEN a.title IS NOT NULL AND a.title <> '' THEN a.title ELSE w.title END AS title,
  CASE WHEN a.description IS NOT NULL THEN a.description ELSE w.description END AS description,
  COALESCE(active.serving_state_id, '') AS active_serving_state_id,
  w.created_at,
  w.updated_at
FROM workspaces w
LEFT JOIN workspace_active_serving_states active
  ON active.workspace_id = w.id AND active.environment = ?
LEFT JOIN assets a
  ON a.serving_state_id = active.serving_state_id
 AND a.asset_type = 'catalog'
 AND a.logical_asset_id = 'catalog:' || w.id
ORDER BY w.created_at;

-- name: GetWorkspaceWithActiveMetadata :one
SELECT
  w.id,
  CASE WHEN a.title IS NOT NULL AND a.title <> '' THEN a.title ELSE w.title END AS title,
  CASE WHEN a.description IS NOT NULL THEN a.description ELSE w.description END AS description,
  COALESCE(active.serving_state_id, '') AS active_serving_state_id,
  w.created_at,
  w.updated_at
FROM workspaces w
LEFT JOIN workspace_active_serving_states active
  ON active.workspace_id = w.id AND active.environment = ?
LEFT JOIN assets a
  ON a.serving_state_id = active.serving_state_id
 AND a.asset_type = 'catalog'
 AND a.logical_asset_id = 'catalog:' || w.id
WHERE w.id = ?;

-- name: SetActiveServingState :exec
INSERT INTO workspace_active_serving_states (workspace_id, environment, serving_state_id, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(workspace_id, environment) DO UPDATE SET
  serving_state_id = excluded.serving_state_id,
  updated_at = CURRENT_TIMESTAMP;
