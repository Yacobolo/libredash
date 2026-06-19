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

-- name: SetWorkspaceActiveDeployment :exec
UPDATE workspaces
SET active_deployment_id = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: CreateDeployment :exec
INSERT INTO deployments (id, workspace_id, status, created_by)
VALUES (?, ?, ?, ?);

-- name: GetDeployment :one
SELECT * FROM deployments WHERE id = ?;

-- name: GetActiveDeployment :one
SELECT d.*
FROM deployments d
JOIN workspaces w ON w.active_deployment_id = d.id
WHERE w.id = ?;

-- name: ListDeployments :many
SELECT * FROM deployments
WHERE workspace_id = ?
ORDER BY created_at DESC;

-- name: UpdateDeploymentValidated :exec
UPDATE deployments
SET status = ?, digest = ?, manifest_json = ?, error = ''
WHERE id = ?;

-- name: UpdateDeploymentStatus :exec
UPDATE deployments
SET status = ?, error = ?
WHERE id = ?;

-- name: MarkDeploymentActive :exec
UPDATE deployments
SET status = 'active', activated_at = CURRENT_TIMESTAMP, error = ''
WHERE id = ?;

-- name: MarkOtherDeploymentsInactive :exec
UPDATE deployments
SET status = 'inactive'
WHERE workspace_id = ? AND id <> ? AND status = 'active';

-- name: InsertDeploymentArtifact :exec
INSERT INTO deployment_artifacts (id, deployment_id, workspace_id, digest, format, path, manifest_json, size_bytes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(deployment_id) DO UPDATE SET
  digest = excluded.digest,
  format = excluded.format,
  path = excluded.path,
  manifest_json = excluded.manifest_json,
  size_bytes = excluded.size_bytes;

-- name: GetArtifactByDeployment :one
SELECT * FROM deployment_artifacts WHERE deployment_id = ?;

-- name: ClearAssetsForDeployment :exec
DELETE FROM assets WHERE deployment_id = ?;

-- name: InsertAsset :exec
INSERT INTO assets (id, workspace_id, deployment_id, asset_type, asset_key, parent_asset_id, title, description, content_json, content_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertAssetEdge :exec
INSERT INTO asset_edges (id, workspace_id, deployment_id, from_asset_id, to_asset_id, edge_type)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListAssetsByDeployment :many
SELECT * FROM assets WHERE deployment_id = ? ORDER BY asset_type, asset_key;

-- name: ListAssetEdgesByDeployment :many
SELECT * FROM asset_edges WHERE deployment_id = ? ORDER BY edge_type, from_asset_id, to_asset_id;

-- name: UpsertPrincipal :exec
INSERT INTO principals (id, email, display_name, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
  email = excluded.email,
  display_name = excluded.display_name,
  updated_at = CURRENT_TIMESTAMP;

-- name: GetPrincipal :one
SELECT * FROM principals WHERE id = ?;

-- name: GetPrincipalByEmail :one
SELECT * FROM principals WHERE lower(email) = lower(?) AND email <> '' LIMIT 1;

-- name: UpsertExternalIdentity :exec
INSERT INTO external_identities (id, principal_id, provider, tenant_id, subject, email, updated_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(provider, tenant_id, subject) DO UPDATE SET
  principal_id = excluded.principal_id,
  email = excluded.email,
  updated_at = CURRENT_TIMESTAMP;

-- name: GetExternalIdentity :one
SELECT * FROM external_identities
WHERE provider = ? AND tenant_id = ? AND subject = ?;

-- name: UpsertRole :exec
INSERT INTO roles (id, name, permissions_json)
VALUES (?, ?, ?)
ON CONFLICT(name) DO UPDATE SET permissions_json = excluded.permissions_json;

-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = ?;

-- name: ListRoles :many
SELECT * FROM roles ORDER BY name;

-- name: InsertRoleBinding :exec
INSERT OR IGNORE INTO role_bindings (id, workspace_id, role_id, principal_id, group_id)
VALUES (?, ?, ?, ?, ?);

-- name: ListRoleBindingsByWorkspace :many
SELECT
  rb.id,
  rb.workspace_id,
  rb.principal_id,
  p.email,
  p.display_name,
  r.name AS role_name,
  rb.created_at
FROM role_bindings rb
JOIN roles r ON r.id = rb.role_id
LEFT JOIN principals p ON p.id = rb.principal_id
WHERE rb.workspace_id = ? AND rb.principal_id IS NOT NULL
ORDER BY p.email, r.name;

-- name: DeletePrincipalRoleBindings :exec
DELETE FROM role_bindings
WHERE workspace_id = ? AND principal_id = ?;

-- name: ListPrincipalRolePermissions :many
SELECT r.permissions_json
FROM role_bindings rb
JOIN roles r ON r.id = rb.role_id
WHERE rb.workspace_id = ? AND rb.principal_id = ?;

-- name: CreateSession :exec
INSERT INTO sessions (id, principal_id, token_hash, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetSessionByTokenHash :one
SELECT * FROM sessions WHERE token_hash = ? AND expires_at > CURRENT_TIMESTAMP;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: CreateAPIToken :exec
INSERT INTO api_tokens (id, principal_id, name, token_hash, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetAPITokenByHash :one
SELECT * FROM api_tokens
WHERE token_hash = ? AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP);

-- name: TouchAPIToken :exec
UPDATE api_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: InsertAuditEvent :exec
INSERT INTO audit_events (id, workspace_id, principal_id, action, target_type, target_id, metadata_json)
VALUES (?, ?, ?, ?, ?, ?, ?);
