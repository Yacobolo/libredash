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


-- name: UpdateAgentConversationTitle :one
UPDATE agent_conversations
SET title = sqlc.arg(title), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(conversation_id) AND workspace_id = sqlc.arg(workspace_id)
  AND principal_id = sqlc.arg(principal_id) AND status = 'active'
RETURNING id, workspace_id, principal_id, title, status, metadata_json, transcript_json, created_at, updated_at, archived_at;

-- name: GetAgentRunInConversation :one
SELECT r.id, r.conversation_id, r.status, r.model, r.stop_reason, r.input_tokens, r.output_tokens,
       r.total_tokens, r.error, r.started_at, r.finished_at, r.metadata_json
FROM agent_runs r
JOIN agent_conversations c ON c.id = r.conversation_id
WHERE r.id = sqlc.arg(run_id) AND c.id = sqlc.arg(conversation_id)
  AND c.workspace_id = sqlc.arg(workspace_id) AND c.principal_id = sqlc.arg(principal_id);

-- name: GetAgentRunForPrincipal :one
SELECT r.id, r.conversation_id, r.status, r.model, r.stop_reason, r.input_tokens, r.output_tokens,
       r.total_tokens, r.error, r.started_at, r.finished_at, r.metadata_json
FROM agent_runs r
JOIN agent_conversations c ON c.id = r.conversation_id
WHERE r.id = sqlc.arg(run_id) AND c.workspace_id = sqlc.arg(workspace_id)
  AND c.principal_id = sqlc.arg(principal_id);

-- name: AgentRunExistsForPrincipal :one
SELECT EXISTS (
  SELECT 1 FROM agent_runs r
  JOIN agent_conversations c ON c.id = r.conversation_id
  WHERE r.id = sqlc.arg(run_id) AND c.workspace_id = sqlc.arg(workspace_id)
    AND c.principal_id = sqlc.arg(principal_id)
);
-- name: DeleteAsyncEventsForArchivedAgentRuns :exec
DELETE FROM api_async_events
WHERE resource_kind = 'agent_run'
  AND resource_id IN (
    SELECT r.id FROM agent_runs r
    JOIN agent_conversations c ON c.id = r.conversation_id
    WHERE c.archived_at IS NOT NULL
      AND c.archived_at <> ''
      AND c.archived_at < sqlc.arg(cutoff)
  );
