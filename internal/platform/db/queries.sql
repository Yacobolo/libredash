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

-- name: CreateServingState :exec
INSERT INTO serving_states (id, workspace_id, environment, status, source, created_by)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetServingState :one
SELECT * FROM serving_states WHERE id = ?;

-- name: GetActiveServingState :one
SELECT d.*
FROM serving_states d
JOIN workspace_active_serving_states active ON active.serving_state_id = d.id
WHERE active.workspace_id = ? AND active.environment = ?;

-- name: ListServingStates :many
SELECT * FROM serving_states
WHERE workspace_id = ? AND environment = ?
ORDER BY created_at DESC;

-- name: ListReferencedDuckLakeSnapshots :many
SELECT DISTINCT ducklake_snapshot_id
FROM serving_states
WHERE ducklake_snapshot_id > 0
  AND status = 'active'
ORDER BY ducklake_snapshot_id;

-- name: ListActiveDuckLakeSnapshots :many
SELECT DISTINCT ducklake_snapshot_id
FROM serving_states
WHERE ducklake_snapshot_id > 0
  AND status = 'active'
ORDER BY ducklake_snapshot_id;

-- name: ListLeasedDuckLakeSnapshots :many
SELECT DISTINCT ducklake_snapshot_id
FROM query_snapshot_leases
WHERE ducklake_snapshot_id > 0
  AND released_at IS NULL
  AND expires_at > CURRENT_TIMESTAMP
ORDER BY ducklake_snapshot_id;

-- name: ExpireInactiveServingStates :exec
UPDATE serving_states
SET status = 'expired', error = ''
WHERE status = 'inactive';

-- name: MarkOtherServingStatesDraining :exec
UPDATE serving_states
SET status = 'draining',
    superseded_at = CURRENT_TIMESTAMP,
    error = ''
WHERE workspace_id = ?
  AND environment = ?
  AND id <> ?
  AND status = 'active';

-- name: MarkDrainingServingStatesDeleteScheduled :exec
UPDATE serving_states
SET status = 'delete_scheduled', error = ''
WHERE status = 'draining';

-- name: ScheduleExpiredServingStateDeletion :exec
UPDATE serving_states
SET status = 'delete_scheduled', error = ''
WHERE status = 'expired';

-- name: MarkDeleteScheduledServingStatesDeleted :exec
UPDATE serving_states
SET status = 'deleted', error = ''
WHERE status = 'delete_scheduled';

-- name: UpdateServingStateValidated :exec
UPDATE serving_states
SET status = ?, digest = ?, manifest_json = ?, error = ''
WHERE id = ?;

-- name: UpdateServingStateDuckLakeSnapshot :exec
UPDATE serving_states
SET ducklake_snapshot_id = ?
WHERE id = ?;

-- name: UpdateServingStateStatus :exec
UPDATE serving_states
SET status = ?, error = ?
WHERE id = ?;

-- name: MarkServingStateActive :exec
UPDATE serving_states
SET status = 'active', activated_at = CURRENT_TIMESTAMP, error = ''
WHERE id = ?;

-- name: MarkOtherServingStatesInactive :exec
UPDATE serving_states
SET status = 'inactive'
WHERE workspace_id = ? AND environment = ? AND id <> ? AND status = 'active';

-- name: InsertServingStateArtifact :exec
INSERT INTO serving_state_artifacts (id, serving_state_id, workspace_id, environment, digest, format, path, data_root, manifest_json, size_bytes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(serving_state_id) DO UPDATE SET
  environment = excluded.environment,
  digest = excluded.digest,
  format = excluded.format,
  path = excluded.path,
  data_root = excluded.data_root,
  manifest_json = excluded.manifest_json,
  size_bytes = excluded.size_bytes;

-- name: GetArtifactByServingState :one
SELECT * FROM serving_state_artifacts WHERE serving_state_id = ?;

-- name: CreateQuerySnapshotLease :exec
INSERT INTO query_snapshot_leases (id, workspace_id, environment, serving_state_id, ducklake_snapshot_id, owner_id, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ReleaseQuerySnapshotLease :exec
UPDATE query_snapshot_leases
SET released_at = COALESCE(released_at, CURRENT_TIMESTAMP)
WHERE id = ?;

-- name: ExtendQuerySnapshotLease :exec
UPDATE query_snapshot_leases
SET expires_at = ?
WHERE id = ?
  AND released_at IS NULL;

-- name: ReleaseExpiredQuerySnapshotLeases :exec
UPDATE query_snapshot_leases
SET released_at = CURRENT_TIMESTAMP
WHERE released_at IS NULL
  AND expires_at <= CURRENT_TIMESTAMP;

-- name: ClearAssetsForServingState :exec
DELETE FROM assets WHERE serving_state_id = ?;

-- name: ClearAssetEdgesForServingState :exec
DELETE FROM asset_edges WHERE serving_state_id = ?;

-- name: InsertAsset :exec
INSERT INTO assets (snapshot_id, logical_asset_id, workspace_id, serving_state_id, asset_type, asset_key, parent_logical_asset_id, title, description, source_file, payload_schema, payload_json, content_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertAssetEdge :exec
INSERT INTO asset_edges (id, workspace_id, serving_state_id, from_logical_asset_id, to_logical_asset_id, edge_type)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListAssetsByServingState :many
SELECT * FROM assets WHERE serving_state_id = ? ORDER BY asset_type, asset_key;

-- name: ListAssetEdgesByServingState :many
SELECT * FROM asset_edges WHERE serving_state_id = ? ORDER BY edge_type, from_logical_asset_id, to_logical_asset_id;

-- name: ListAssetVersions :many
SELECT
  d.id AS serving_state_id,
  d.workspace_id,
  d.environment,
  d.status,
  d.digest,
  d.created_by,
  d.created_at,
  d.activated_at,
  a.snapshot_id,
  a.logical_asset_id,
  a.source_file,
  a.content_hash
FROM serving_states d
JOIN assets a ON a.serving_state_id = d.id
WHERE d.workspace_id = ?
  AND d.environment = ?
  AND a.logical_asset_id = ?
  AND d.source = 'publish'
  AND d.status IN ('active', 'draining', 'inactive', 'validated')
  AND NOT EXISTS (
    SELECT 1
    FROM serving_states newer
    JOIN assets newer_asset ON newer_asset.serving_state_id = newer.id
    WHERE newer.workspace_id = d.workspace_id
      AND newer.environment = d.environment
      AND newer.source = 'publish'
      AND newer.status IN ('active', 'draining', 'inactive', 'validated')
      AND newer_asset.logical_asset_id = a.logical_asset_id
      AND newer_asset.content_hash = a.content_hash
      AND (
        COALESCE(newer.activated_at, newer.created_at) > COALESCE(d.activated_at, d.created_at)
        OR (
          COALESCE(newer.activated_at, newer.created_at) = COALESCE(d.activated_at, d.created_at)
          AND newer.created_at > d.created_at
        )
        OR (
          COALESCE(newer.activated_at, newer.created_at) = COALESCE(d.activated_at, d.created_at)
          AND newer.created_at = d.created_at
          AND newer.id > d.id
        )
      )
  )
ORDER BY
  COALESCE(d.activated_at, d.created_at) DESC,
  d.created_at DESC,
  d.id DESC;

-- name: UpsertPrincipal :exec
INSERT INTO principals (id, kind, email, display_name, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
  kind = excluded.kind,
  email = excluded.email,
  display_name = excluded.display_name,
  updated_at = CURRENT_TIMESTAMP;

-- name: GetPrincipal :one
SELECT * FROM principals WHERE id = ?;

-- name: GetPrincipalByEmail :one
SELECT * FROM principals WHERE lower(email) = lower(?) AND email <> '' LIMIT 1;

-- name: DisablePrincipal :exec
UPDATE principals
SET disabled_at = COALESCE(disabled_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: ListSCIMPrincipals :many
SELECT p.*
FROM principals p
JOIN external_identities ei ON ei.principal_id = p.id
WHERE p.kind = 'user'
  AND ei.provider = 'scim'
  AND (sqlc.arg(subject) = '' OR ei.subject = sqlc.arg(subject))
  AND (
    sqlc.arg(user_name) = ''
    OR lower(p.email) = lower(sqlc.arg(user_name))
    OR lower(ei.email) = lower(sqlc.arg(user_name))
  )
ORDER BY p.email, p.display_name, p.id;

-- name: GetExternalIdentityByPrincipalProvider :one
SELECT * FROM external_identities
WHERE principal_id = ? AND provider = ?
LIMIT 1;

-- name: ListServicePrincipals :many
SELECT * FROM principals
WHERE kind = 'service_principal'
ORDER BY display_name, id;

-- name: DeleteServicePrincipal :exec
DELETE FROM principals
WHERE id = ? AND kind = 'service_principal';

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
INSERT INTO roles (id, name, privileges_json)
VALUES (?, ?, ?)
ON CONFLICT(name) DO UPDATE SET privileges_json = excluded.privileges_json;

-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = ?;

-- name: ListRoles :many
SELECT * FROM roles ORDER BY name;

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
   OR (workspace_id = '' AND provider = 'scim')
ORDER BY name, id;

-- name: GetSCIMGroup :one
SELECT * FROM groups
WHERE provider = 'scim' AND id = ?;

-- name: ListSCIMGroups :many
SELECT * FROM groups
WHERE workspace_id = ''
  AND provider = 'scim'
  AND (sqlc.arg(external_id) = '' OR external_id = sqlc.arg(external_id))
  AND (sqlc.arg(display_name) = '' OR lower(name) = lower(sqlc.arg(display_name)))
ORDER BY name, id;

-- name: DeleteGroup :exec
DELETE FROM groups
WHERE workspace_id = ? AND id = ?;

-- name: DeleteSCIMGroup :exec
DELETE FROM groups
WHERE workspace_id = '' AND provider = 'scim' AND id = ?;

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

-- name: ListSCIMGroupMembers :many
SELECT
  gm.group_id,
  gm.workspace_id,
  gm.principal_id,
  p.email,
  p.display_name,
  gm.created_at
FROM group_members gm
JOIN principals p ON p.id = gm.principal_id
WHERE gm.workspace_id = '' AND gm.group_id = ?
ORDER BY p.email, p.display_name, gm.principal_id;

-- name: DeleteSCIMGroupMembers :exec
DELETE FROM group_members
WHERE workspace_id = '' AND group_id = ?;

-- name: DeleteSCIMGroupMembersByPrincipal :exec
DELETE FROM group_members
WHERE workspace_id = '' AND principal_id = ?;

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
  CASE
    WHEN NULLIF(rb.principal_id, '') IS NOT NULL AND p.kind = 'service_principal' THEN 'service_principal'
    WHEN NULLIF(rb.principal_id, '') IS NOT NULL THEN 'principal'
    ELSE 'group'
  END AS subject_type,
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
  CASE
    WHEN NULLIF(rb.principal_id, '') IS NOT NULL AND p.kind = 'service_principal' THEN 'service_principal'
    WHEN NULLIF(rb.principal_id, '') IS NOT NULL THEN 'principal'
    ELSE 'group'
  END AS subject_type,
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

-- name: CreateSession :exec
INSERT INTO sessions (id, principal_id, token_fingerprint, token_verifier, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSessionByTokenFingerprint :one
SELECT * FROM sessions
WHERE token_fingerprint = ? AND datetime(expires_at) > CURRENT_TIMESTAMP AND revoked_at IS NULL;

-- name: GetSessionByTokenFingerprintForAudit :one
SELECT * FROM sessions
WHERE token_fingerprint = ?
ORDER BY created_at DESC
LIMIT 1;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteSessionByTokenFingerprint :exec
DELETE FROM sessions WHERE token_fingerprint = ?;

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

-- name: RevokeSessionsByPrincipal :exec
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
WHERE principal_id = ? AND revoked_at IS NULL;

-- name: CreateAPIToken :exec
INSERT INTO api_tokens (id, principal_id, workspace_id, name, token_fingerprint, token_verifier, privileges_json, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetAPITokenByFingerprint :one
SELECT * FROM api_tokens
WHERE token_fingerprint = ?
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR datetime(expires_at) > CURRENT_TIMESTAMP);

-- name: GetAPITokenByFingerprintForAudit :one
SELECT * FROM api_tokens
WHERE token_fingerprint = ?
ORDER BY created_at DESC
LIMIT 1;

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

-- name: RevokeAPITokensByPrincipal :exec
UPDATE api_tokens
SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
WHERE principal_id = ? AND revoked_at IS NULL;

-- name: CreateServicePrincipalSecret :exec
INSERT INTO service_principal_secrets (id, service_principal_id, name, secret_fingerprint, secret_verifier, expires_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetServicePrincipalSecretByFingerprint :one
SELECT s.*
FROM service_principal_secrets s
JOIN principals p ON p.id = s.service_principal_id
WHERE p.kind = 'service_principal'
  AND s.service_principal_id = ?
  AND s.secret_fingerprint = ?
  AND s.revoked_at IS NULL
  AND (s.expires_at IS NULL OR datetime(s.expires_at) > CURRENT_TIMESTAMP);

-- name: RevokeServicePrincipalSecret :exec
UPDATE service_principal_secrets
SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
WHERE service_principal_id = ? AND id = ?;

-- name: InsertAuditEvent :exec
INSERT INTO audit_events (id, workspace_id, principal_id, action, target_type, target_id, privilege, status, request_id, correlation_id, metadata_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

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
WHERE workspace_id = sqlc.arg(workspace_id)
  AND principal_id = sqlc.arg(principal_id)
  AND status = 'active'
ORDER BY updated_at DESC, created_at DESC;

-- name: GetAgentConversation :one
SELECT * FROM agent_conversations
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND principal_id = sqlc.arg(principal_id);

-- name: ArchiveAgentConversation :one
UPDATE agent_conversations
SET status = 'archived',
    archived_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: UpdateAgentConversationTranscript :one
UPDATE agent_conversations
SET transcript_json = sqlc.arg(transcript_json),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: UpdateDefaultAgentConversationTitle :one
UPDATE agent_conversations
SET title = sqlc.arg(title),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
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
  AND c.workspace_id = sqlc.arg(workspace_id)
  AND c.principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: ListAgentMessages :many
SELECT m.*
FROM agent_messages m
JOIN agent_conversations c ON c.id = m.conversation_id
WHERE c.id = sqlc.arg(conversation_id)
  AND c.workspace_id = sqlc.arg(workspace_id)
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
  AND c.workspace_id = sqlc.arg(workspace_id)
  AND c.principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: ListAgentRuns :many
SELECT r.*
FROM agent_runs r
JOIN agent_conversations c ON c.id = r.conversation_id
WHERE c.id = sqlc.arg(conversation_id)
  AND c.workspace_id = sqlc.arg(workspace_id)
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
      AND workspace_id = sqlc.arg(workspace_id)
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
  AND c.workspace_id = sqlc.arg(workspace_id)
  AND c.principal_id = sqlc.arg(principal_id)
RETURNING *;

-- name: ListAgentEvents :many
SELECT e.*
FROM agent_events e
JOIN agent_runs r ON r.id = e.run_id
JOIN agent_conversations c ON c.id = r.conversation_id
WHERE r.id = sqlc.arg(run_id)
  AND c.workspace_id = sqlc.arg(workspace_id)
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
  queue_wait_ms,
  execution_ms,
  execution_state,
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
  sqlc.arg(queue_wait_ms),
  sqlc.arg(execution_ms),
  sqlc.arg(execution_state),
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
