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
  COALESCE(active.deployment_id, '') AS active_deployment_id,
  w.created_at,
  w.updated_at
FROM workspaces w
LEFT JOIN workspace_active_deployments active
  ON active.workspace_id = w.id AND active.environment = ?
LEFT JOIN assets a
  ON a.deployment_id = active.deployment_id
 AND a.asset_type = 'catalog'
 AND a.logical_asset_id = 'catalog:' || w.id
ORDER BY w.created_at;

-- name: GetWorkspaceWithActiveMetadata :one
SELECT
  w.id,
  CASE WHEN a.title IS NOT NULL AND a.title <> '' THEN a.title ELSE w.title END AS title,
  CASE WHEN a.description IS NOT NULL THEN a.description ELSE w.description END AS description,
  COALESCE(active.deployment_id, '') AS active_deployment_id,
  w.created_at,
  w.updated_at
FROM workspaces w
LEFT JOIN workspace_active_deployments active
  ON active.workspace_id = w.id AND active.environment = ?
LEFT JOIN assets a
  ON a.deployment_id = active.deployment_id
 AND a.asset_type = 'catalog'
 AND a.logical_asset_id = 'catalog:' || w.id
WHERE w.id = ?;

-- name: SetActiveDeployment :exec
INSERT INTO workspace_active_deployments (workspace_id, environment, deployment_id, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(workspace_id, environment) DO UPDATE SET
  deployment_id = excluded.deployment_id,
  updated_at = CURRENT_TIMESTAMP;

-- name: CreateDeployment :exec
INSERT INTO deployments (id, workspace_id, environment, status, created_by)
VALUES (?, ?, ?, ?, ?);

-- name: GetDeployment :one
SELECT * FROM deployments WHERE id = ?;

-- name: GetActiveDeployment :one
SELECT d.*
FROM deployments d
JOIN workspace_active_deployments active ON active.deployment_id = d.id
WHERE active.workspace_id = ? AND active.environment = ?;

-- name: ListDeployments :many
SELECT * FROM deployments
WHERE workspace_id = ? AND environment = ?
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
WHERE workspace_id = ? AND environment = ? AND id <> ? AND status = 'active';

-- name: InsertDeploymentArtifact :exec
INSERT INTO deployment_artifacts (id, deployment_id, workspace_id, environment, digest, format, path, manifest_json, size_bytes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(deployment_id) DO UPDATE SET
  environment = excluded.environment,
  digest = excluded.digest,
  format = excluded.format,
  path = excluded.path,
  manifest_json = excluded.manifest_json,
  size_bytes = excluded.size_bytes;

-- name: GetArtifactByDeployment :one
SELECT * FROM deployment_artifacts WHERE deployment_id = ?;

-- name: ClearAssetsForDeployment :exec
DELETE FROM assets WHERE deployment_id = ?;

-- name: ClearAssetEdgesForDeployment :exec
DELETE FROM asset_edges WHERE deployment_id = ?;

-- name: InsertAsset :exec
INSERT INTO assets (snapshot_id, logical_asset_id, workspace_id, deployment_id, asset_type, asset_key, parent_logical_asset_id, title, description, source_file, payload_schema, payload_json, content_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertAssetEdge :exec
INSERT INTO asset_edges (id, workspace_id, deployment_id, from_logical_asset_id, to_logical_asset_id, edge_type)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListAssetsByDeployment :many
SELECT * FROM assets WHERE deployment_id = ? ORDER BY asset_type, asset_key;

-- name: ListAssetEdgesByDeployment :many
SELECT * FROM asset_edges WHERE deployment_id = ? ORDER BY edge_type, from_logical_asset_id, to_logical_asset_id;

-- name: ListAssetVersions :many
SELECT
  d.id AS deployment_id,
  d.workspace_id,
  d.environment,
  d.status,
  d.digest,
  d.created_by,
  d.created_at,
  d.activated_at,
  a.snapshot_id,
  a.logical_asset_id,
  a.content_hash
FROM deployments d
JOIN assets a ON a.deployment_id = d.id
WHERE d.workspace_id = ?
  AND d.environment = ?
  AND a.logical_asset_id = ?
  AND d.status IN ('active', 'inactive', 'validated')
ORDER BY
  COALESCE(d.activated_at, d.created_at) DESC,
  d.created_at DESC,
  d.id DESC;

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

-- name: UpsertPermission :exec
INSERT INTO permissions (name)
VALUES (?)
ON CONFLICT(name) DO NOTHING;

-- name: ClearRolePermissions :exec
DELETE FROM role_permissions WHERE role_id = ?;

-- name: InsertRolePermission :exec
INSERT OR IGNORE INTO role_permissions (role_id, permission_name)
VALUES (?, ?);

-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = ?;

-- name: ListRoles :many
SELECT * FROM roles ORDER BY name;

-- name: ListPermissions :many
SELECT * FROM permissions ORDER BY name;

-- name: UpsertGroup :exec
INSERT INTO groups (id, workspace_id, provider, external_id, name)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, provider, external_id) DO UPDATE SET
  name = excluded.name;

-- name: GetGroup :one
SELECT * FROM groups
WHERE workspace_id = ? AND id = ?;

-- name: GetGroupByProviderExternalID :one
SELECT * FROM groups
WHERE workspace_id = ? AND provider = ? AND external_id = ?;

-- name: ListGroupsByWorkspace :many
SELECT * FROM groups
WHERE workspace_id = ?
ORDER BY name, id;

-- name: DeleteGroup :exec
DELETE FROM groups
WHERE workspace_id = ? AND id = ?;

-- name: InsertGroupMember :exec
INSERT OR IGNORE INTO group_members (workspace_id, group_id, principal_id)
VALUES (?, ?, ?);

-- name: DeleteGroupMember :exec
DELETE FROM group_members
WHERE workspace_id = ? AND group_id = ? AND principal_id = ?;

-- name: ListGroupMembers :many
SELECT
  gm.group_id,
  gm.workspace_id,
  gm.principal_id,
  p.email,
  p.display_name,
  gm.created_at
FROM group_members gm
JOIN principals p ON p.id = gm.principal_id
WHERE gm.workspace_id = ? AND gm.group_id = ?
ORDER BY p.email, p.display_name, gm.principal_id;

-- name: InsertRoleBinding :exec
INSERT OR IGNORE INTO role_bindings (id, workspace_id, role_id, principal_id, group_id)
VALUES (?, ?, ?, ?, ?);

-- name: InsertPlatformRoleBinding :exec
INSERT OR IGNORE INTO platform_role_bindings (id, role_id, principal_id)
VALUES (?, ?, ?);

-- name: GetRoleBindingByID :one
SELECT
  rb.id,
  rb.workspace_id,
  CASE WHEN NULLIF(rb.principal_id, '') IS NOT NULL THEN 'principal' ELSE 'group' END AS subject_type,
  COALESCE(NULLIF(rb.principal_id, ''), rb.group_id, '') AS subject_id,
  rb.principal_id,
  rb.group_id,
  p.email,
  p.display_name,
  g.name AS group_name,
  r.name AS role_name,
  rb.created_at
FROM role_bindings rb
JOIN roles r ON r.id = rb.role_id
LEFT JOIN principals p ON p.id = NULLIF(rb.principal_id, '')
LEFT JOIN groups g ON g.id = rb.group_id
WHERE rb.workspace_id = ? AND rb.id = ?;

-- name: UpdateRoleBindingByID :exec
UPDATE role_bindings
SET role_id = ?, principal_id = ?, group_id = ?
WHERE workspace_id = ? AND id = ?;

-- name: DeleteRoleBindingByID :exec
DELETE FROM role_bindings
WHERE workspace_id = ? AND id = ?;

-- name: ListRoleBindingsByWorkspace :many
SELECT
  rb.id,
  rb.workspace_id,
  CASE WHEN NULLIF(rb.principal_id, '') IS NOT NULL THEN 'principal' ELSE 'group' END AS subject_type,
  COALESCE(NULLIF(rb.principal_id, ''), rb.group_id, '') AS subject_id,
  rb.principal_id,
  rb.group_id,
  p.email,
  p.display_name,
  g.name AS group_name,
  r.name AS role_name,
  rb.created_at
FROM role_bindings rb
JOIN roles r ON r.id = rb.role_id
LEFT JOIN principals p ON p.id = NULLIF(rb.principal_id, '')
LEFT JOIN groups g ON g.id = rb.group_id
WHERE rb.workspace_id = ?
ORDER BY subject_type, p.email, g.name, r.name;

-- name: DeletePrincipalRoleBindings :exec
DELETE FROM role_bindings
WHERE workspace_id = ? AND principal_id = ?;

-- name: ListPrincipalRolePermissions :many
SELECT DISTINCT permission_name
FROM (
  SELECT rp.permission_name
  FROM role_bindings rb
  JOIN role_permissions rp ON rp.role_id = rb.role_id
  LEFT JOIN group_members gm
    ON gm.workspace_id = rb.workspace_id
   AND gm.group_id = rb.group_id
  WHERE rb.workspace_id = ?
    AND (
      rb.principal_id = ?
      OR gm.principal_id = ?
    )
  UNION
  SELECT rp.permission_name
  FROM platform_role_bindings prb
  JOIN role_permissions rp ON rp.role_id = prb.role_id
  WHERE prb.principal_id = ?
)
ORDER BY permission_name;

-- name: CreateSession :exec
INSERT INTO sessions (id, principal_id, token_hash, expires_at)
VALUES (?, ?, ?, ?);

-- name: GetSessionByTokenHash :one
SELECT * FROM sessions
WHERE token_hash = ? AND expires_at > CURRENT_TIMESTAMP AND revoked_at IS NULL;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: ListSessionsByPrincipal :many
SELECT * FROM sessions
WHERE principal_id = ?
ORDER BY created_at DESC;

-- name: RevokeSession :exec
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
WHERE id = ?;

-- name: RevokeSessionForPrincipal :one
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
WHERE principal_id = ? AND id = ?
RETURNING *;

-- name: CreateAPIToken :exec
INSERT INTO api_tokens (id, principal_id, workspace_id, name, token_hash, permissions_json, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetAPITokenByHash :one
SELECT * FROM api_tokens
WHERE token_hash = ?
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP);

-- name: TouchAPIToken :exec
UPDATE api_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: ListAPITokensByPrincipal :many
SELECT * FROM api_tokens
WHERE principal_id = ?
ORDER BY created_at DESC;

-- name: RevokeAPIToken :exec
UPDATE api_tokens
SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
WHERE id = ?;

-- name: RevokeAPITokenForPrincipal :one
UPDATE api_tokens
SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
WHERE principal_id = ? AND id = ?
RETURNING *;

-- name: InsertAuditEvent :exec
INSERT INTO audit_events (id, workspace_id, principal_id, action, target_type, target_id, metadata_json)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListAuditEvents :many
SELECT * FROM audit_events
WHERE (? = '' OR workspace_id = ?)
  AND (? = '' OR principal_id = ?)
  AND (? = '' OR action = ?)
  AND (? = '' OR target_type = ?)
  AND (? = '' OR target_id = ?)
  AND (? = '' OR created_at >= ?)
  AND (? = '' OR created_at <= ?)
  AND (? = '' OR created_at < ? OR (created_at = ? AND id < ?))
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: CreateAgentConversation :one
INSERT INTO agent_conversations (id, workspace_id, principal_id, title, status, metadata_json, transcript_json)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(principal_id), sqlc.arg(title), sqlc.arg(status), sqlc.arg(metadata_json), sqlc.arg(transcript_json))
RETURNING *;

-- name: ListAgentConversations :many
SELECT * FROM agent_conversations
WHERE principal_id = sqlc.arg(principal_id)
  AND status = 'active'
ORDER BY updated_at DESC, created_at DESC;

-- name: GetAgentConversation :one
SELECT * FROM agent_conversations
WHERE id = sqlc.arg(id)
  AND principal_id = sqlc.arg(principal_id);

-- name: ArchiveAgentConversation :one
UPDATE agent_conversations
SET status = 'archived',
    archived_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: UpdateAgentConversationTranscript :one
UPDATE agent_conversations
SET transcript_json = sqlc.arg(transcript_json),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: UpdateDefaultAgentConversationTitle :one
UPDATE agent_conversations
SET title = sqlc.arg(title),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND principal_id = sqlc.arg(principal_id)
  AND status = 'active'
  AND title = 'New conversation'
RETURNING *;

-- name: AppendAgentMessage :one
INSERT INTO agent_messages (id, conversation_id, run_id, seq, role, content_text, content_json, tool_call_id, tool_name, is_error)
SELECT
  sqlc.arg(id),
  c.id,
  NULLIF(sqlc.arg(run_id), ''),
  COALESCE((SELECT MAX(seq) + 1 FROM agent_messages WHERE conversation_id = c.id), 1),
  sqlc.arg(role),
  sqlc.arg(content_text),
  sqlc.arg(content_json),
  sqlc.arg(tool_call_id),
  sqlc.arg(tool_name),
  sqlc.arg(is_error)
FROM agent_conversations c
WHERE c.id = sqlc.arg(conversation_id)
  AND c.principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: ListAgentMessages :many
SELECT m.*
FROM agent_messages m
JOIN agent_conversations c ON c.id = m.conversation_id
WHERE c.id = sqlc.arg(conversation_id)
  AND c.principal_id = sqlc.arg(principal_id)
ORDER BY m.seq;

-- name: CreateAgentRun :one
INSERT INTO agent_runs (id, conversation_id, status, model, metadata_json)
SELECT
  sqlc.arg(id),
  c.id,
  sqlc.arg(status),
  sqlc.arg(model),
  sqlc.arg(metadata_json)
FROM agent_conversations c
WHERE c.id = sqlc.arg(conversation_id)
  AND c.principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: ListAgentRuns :many
SELECT r.*
FROM agent_runs r
JOIN agent_conversations c ON c.id = r.conversation_id
WHERE c.id = sqlc.arg(conversation_id)
  AND c.principal_id = sqlc.arg(principal_id)
ORDER BY r.started_at DESC;

-- name: FinishAgentRun :one
UPDATE agent_runs
SET status = sqlc.arg(status),
    stop_reason = sqlc.arg(stop_reason),
    input_tokens = sqlc.arg(input_tokens),
    output_tokens = sqlc.arg(output_tokens),
    total_tokens = sqlc.arg(total_tokens),
    error = sqlc.arg(error),
    finished_at = CURRENT_TIMESTAMP,
    metadata_json = sqlc.arg(metadata_json)
WHERE agent_runs.id = sqlc.arg(id)
  AND conversation_id IN (
    SELECT agent_conversations.id
    FROM agent_conversations
    WHERE agent_conversations.id = sqlc.arg(conversation_id)
      AND principal_id = sqlc.arg(principal_id)
  )
RETURNING *;

-- name: AppendAgentEvent :one
INSERT INTO agent_events (id, run_id, seq, event_type, severity, payload_json)
SELECT
  sqlc.arg(id),
  r.id,
  sqlc.arg(seq),
  sqlc.arg(event_type),
  sqlc.arg(severity),
  sqlc.arg(payload_json)
FROM agent_runs r
JOIN agent_conversations c ON c.id = r.conversation_id
WHERE r.id = sqlc.arg(run_id)
  AND c.principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: ListAgentEvents :many
SELECT e.*
FROM agent_events e
JOIN agent_runs r ON r.id = e.run_id
JOIN agent_conversations c ON c.id = r.conversation_id
WHERE r.id = sqlc.arg(run_id)
  AND c.principal_id = sqlc.arg(principal_id)
ORDER BY e.seq;

-- name: InsertQueryEvent :exec
INSERT INTO query_events (
  id,
  workspace_id,
  principal_id,
  surface,
  operation,
  query_kind,
  model_id,
  target,
  object_type,
  object_id,
  request_id,
  correlation_id,
  status,
  duration_ms,
  rows_returned,
  bytes_estimate,
  error,
  sql_text,
  plan_text,
  query_json
)
VALUES (
  sqlc.arg(id),
  sqlc.arg(workspace_id),
  sqlc.arg(principal_id),
  sqlc.arg(surface),
  sqlc.arg(operation),
  sqlc.arg(query_kind),
  sqlc.arg(model_id),
  sqlc.arg(target),
  sqlc.arg(object_type),
  sqlc.arg(object_id),
  sqlc.arg(request_id),
  sqlc.arg(correlation_id),
  sqlc.arg(status),
  sqlc.arg(duration_ms),
  sqlc.arg(rows_returned),
  sqlc.arg(bytes_estimate),
  sqlc.arg(error),
  sqlc.arg(sql_text),
  sqlc.arg(plan_text),
  sqlc.arg(query_json)
);

-- name: GetQueryEvent :one
SELECT *
FROM query_events
WHERE id = sqlc.arg(id);

-- name: ListQueryEvents :many
SELECT *
FROM query_events
WHERE (sqlc.arg(workspace_id) = '' OR workspace_id = sqlc.arg(workspace_id))
  AND (sqlc.arg(principal_id) = '' OR principal_id = sqlc.arg(principal_id))
  AND (sqlc.arg(surface) = '' OR surface = sqlc.arg(surface))
  AND (sqlc.arg(operation) = '' OR operation = sqlc.arg(operation))
  AND (sqlc.arg(query_kind) = '' OR query_kind = sqlc.arg(query_kind))
  AND (sqlc.arg(model_id) = '' OR model_id = sqlc.arg(model_id))
  AND (sqlc.arg(target) = '' OR target = sqlc.arg(target))
  AND (sqlc.arg(status) = '' OR status = sqlc.arg(status))
  AND (sqlc.arg(from_time) = '' OR created_at >= sqlc.arg(from_time))
  AND (sqlc.arg(to_time) = '' OR created_at <= sqlc.arg(to_time))
  AND (
    sqlc.arg(search) = ''
    OR target LIKE '%' || sqlc.arg(search) || '%'
    OR sql_text LIKE '%' || sqlc.arg(search) || '%'
    OR query_json LIKE '%' || sqlc.arg(search) || '%'
  )
  AND (
    sqlc.arg(cursor_time) = ''
    OR created_at < sqlc.arg(cursor_time)
    OR (created_at = sqlc.arg(cursor_time) AND id < sqlc.arg(cursor_id))
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit);
