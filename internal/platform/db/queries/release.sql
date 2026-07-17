-- API v1 releases and project discovery.

-- name: CreateAPIRelease :exec
INSERT INTO api_releases (id, project_id, project_digest, request_digest, idempotency_key, status, manifest_json, created_by)
VALUES (?, ?, ?, ?, ?, 'draft', ?, ?);

-- name: CreateAPIReleaseArtifact :exec
INSERT INTO api_release_artifacts (release_id, workspace_id, expected_digest) VALUES (?, ?, ?);

-- name: CreateAPIReleaseConnection :exec
INSERT INTO api_release_connections (release_id, connection_id, revision_id) VALUES (?, ?, ?);

-- name: GetAPIReleaseByID :one
SELECT id, project_id, project_digest, request_digest, idempotency_key, status, manifest_json, created_by,
  created_at, COALESCE(finalized_at, '') AS finalized_at, error
FROM api_releases WHERE project_id = ? AND id = ?;

-- name: GetAPIReleaseByIdempotencyKey :one
SELECT id, project_id, project_digest, request_digest, idempotency_key, status, manifest_json, created_by,
  created_at, COALESCE(finalized_at, '') AS finalized_at, error
FROM api_releases WHERE project_id = ? AND idempotency_key = ?;

-- name: ListAPIReleaseIDs :many
SELECT id FROM api_releases WHERE project_id = ? ORDER BY created_at DESC, id DESC;

-- name: GetAPIReleaseArtifacts :many
SELECT workspace_id, expected_digest, COALESCE(serving_state_id, '') AS serving_state_id,
  actual_digest, size_bytes, COALESCE(uploaded_at, '') AS uploaded_at
FROM api_release_artifacts WHERE release_id = ? ORDER BY workspace_id;

-- name: GetAPIReleaseArtifactUploadState :one
SELECT r.status, a.expected_digest FROM api_releases r
JOIN api_release_artifacts a ON a.release_id = r.id
WHERE r.id = ? AND a.workspace_id = ?;

-- name: RecordAPIReleaseArtifact :execrows
UPDATE api_release_artifacts SET size_bytes = ?, uploaded_at = CURRENT_TIMESTAMP
WHERE release_id = ? AND workspace_id = ? AND serving_state_id = ? AND uploaded_at IS NULL;

-- name: AssignAPIReleaseArtifactTarget :execrows
UPDATE api_release_artifacts SET serving_state_id = ?
WHERE release_id = ? AND workspace_id = ? AND serving_state_id IS NULL
  AND EXISTS (SELECT 1 FROM api_releases WHERE id = ? AND project_id = ? AND status = 'draft');

-- name: MarkAPIReleaseValidating :execrows
UPDATE api_releases SET status = 'validating' WHERE id = ? AND project_id = ? AND status = 'draft';

-- name: SetAPIReleaseArtifactDigest :exec
UPDATE api_release_artifacts SET actual_digest = ? WHERE release_id = ? AND workspace_id = ?;

-- name: MarkAPIReleaseReady :execrows
UPDATE api_releases SET status = 'ready', finalized_at = CURRENT_TIMESTAMP
WHERE id = ? AND project_id = ? AND status = 'validating';

-- name: MarkAPIReleaseFailed :execrows
UPDATE api_releases SET status = 'failed', error = ?, finalized_at = CURRENT_TIMESTAMP
WHERE id = ? AND project_id = ? AND status = 'validating';

-- name: LinkAPIReleaseDeployment :exec
INSERT INTO api_deployment_releases (deployment_id, project_id, release_id, rollback_of) VALUES (?, ?, ?, ?)
ON CONFLICT(deployment_id) DO UPDATE SET release_id = excluded.release_id
WHERE api_deployment_releases.project_id = excluded.project_id
  AND api_deployment_releases.release_id = excluded.release_id;

-- name: GetAPIReleaseDeployment :one
SELECT release_id, COALESCE(rollback_of, '') AS rollback_of
FROM api_deployment_releases WHERE project_id = ? AND deployment_id = ?;

-- name: ListAPIReleaseDeploymentIDs :many
SELECT deployment_id FROM api_deployment_releases
WHERE project_id = ? ORDER BY created_at DESC, deployment_id DESC;

-- name: GetPriorAPIReleaseDeployment :one
SELECT prior.release_id FROM api_deployment_releases current
JOIN api_deployment_releases prior ON prior.project_id = current.project_id AND prior.created_at < current.created_at
WHERE current.project_id = ? AND current.deployment_id = ?
ORDER BY prior.created_at DESC, prior.deployment_id DESC LIMIT 1;

-- name: ListAPIProjects :many
SELECT project_id, CAST(MIN(created_at) AS TEXT) AS created_at, CAST(MAX(updated_at) AS TEXT) AS updated_at FROM (
  SELECT project_id, created_at, COALESCE(finalized_at, created_at) AS updated_at FROM api_releases
  UNION ALL SELECT project_id, created_at, updated_at FROM managed_data_collections
) GROUP BY project_id ORDER BY project_id;

-- name: GetAPIProject :one
SELECT CAST(COALESCE(MIN(created_at), '') AS TEXT) AS created_at,
  CAST(COALESCE(MAX(updated_at), '') AS TEXT) AS updated_at FROM (
  SELECT created_at, COALESCE(finalized_at, created_at) AS updated_at FROM api_releases
  WHERE api_releases.project_id = sqlc.arg(project_id)
  UNION ALL SELECT created_at, updated_at FROM managed_data_collections
  WHERE managed_data_collections.project_id = sqlc.arg(project_id)
);

-- name: GetLatestAPIProjectReleaseID :one
SELECT id FROM api_releases WHERE project_id = ? ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: GetActiveAPIProjectDeploymentID :one
SELECT d.id FROM project_deployments d JOIN api_deployment_releases l ON l.deployment_id = d.id
WHERE l.project_id = ? AND d.status = 'active' ORDER BY d.activated_at DESC LIMIT 1;

-- name: ListAPIProjectWorkspaces :many
SELECT DISTINCT a.workspace_id, COALESCE(w.title, a.workspace_id) AS title,
  COALESCE(w.description, '') AS description, COALESCE(active.serving_state_id, '') AS active_serving_state_id
FROM api_release_artifacts a JOIN api_releases rel ON rel.id = a.release_id
LEFT JOIN workspaces w ON w.id = a.workspace_id
LEFT JOIN workspace_active_serving_states active ON active.workspace_id = a.workspace_id AND active.environment = ?
WHERE rel.project_id = ? ORDER BY a.workspace_id;

-- name: ListAPIProjectConnections :many
SELECT c.connection_name, c.name, c.description, COALESCE(rev.digest, '') AS active_revision_id
FROM managed_data_collections c
LEFT JOIN managed_data_environment_pointers ptr ON ptr.collection_id = c.id AND ptr.environment = ?
LEFT JOIN managed_data_revisions rev ON rev.id = ptr.revision_id
WHERE c.project_id = ? AND c.status = 'active' ORDER BY c.connection_name;

-- name: GetAPIProjectConnection :one
SELECT c.name, c.description, COALESCE(rev.digest, '') AS active_revision_id
FROM managed_data_collections c
LEFT JOIN managed_data_environment_pointers ptr ON ptr.collection_id = c.id AND ptr.environment = ?
LEFT JOIN managed_data_revisions rev ON rev.id = ptr.revision_id
WHERE c.project_id = ? AND c.connection_name = ? AND c.status = 'active';
