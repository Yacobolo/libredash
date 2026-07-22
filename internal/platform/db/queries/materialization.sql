-- name: CreateRefreshJob :exec
INSERT INTO refresh_jobs (id, workspace_id, serving_state_id, model_id, kind, payload_json, status, queued_at)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), NULLIF(CAST(sqlc.arg(serving_state_id) AS TEXT), ''), sqlc.arg(model_id), sqlc.arg(kind), sqlc.arg(payload_json), sqlc.arg(status), CURRENT_TIMESTAMP);

-- name: CreateRefreshJobRun :exec
INSERT INTO refresh_job_runs (
  id, job_id, principal_id, environment, target_type, target_id, trigger_type,
  parent_run_id, retry_of, status, created_sequence
)
VALUES (
  sqlc.arg(id), sqlc.arg(job_id), NULLIF(CAST(sqlc.arg(principal_id) AS TEXT), ''),
  sqlc.arg(environment), sqlc.arg(target_type), sqlc.arg(target_id), sqlc.arg(trigger_type),
  NULLIF(CAST(sqlc.arg(parent_run_id) AS TEXT), ''), NULLIF(CAST(sqlc.arg(retry_of) AS TEXT), ''),
  sqlc.arg(status), COALESCE((SELECT MAX(created_sequence) + 1 FROM refresh_job_runs), 1)
);

-- name: NextExecutableRefreshJob :one
SELECT j.id, j.workspace_id, r.environment, COALESCE(j.serving_state_id, '') AS serving_state_id, j.model_id, j.kind, j.payload_json,
       r.id AS run_id, r.target_type, r.target_id, r.trigger_type, j.attempt_count
FROM refresh_jobs j
JOIN refresh_job_runs r ON r.job_id = j.id
WHERE COALESCE(r.parent_run_id, '') = ''
  AND j.kind = sqlc.arg(refresh_pipeline_kind)
  AND r.environment = sqlc.arg(environment)
  AND (
    (j.status = sqlc.arg(queued_status) AND r.status = sqlc.arg(run_queued_status))
    OR (j.status = sqlc.arg(running_status) AND (j.lease_expires_at IS NULL OR j.lease_expires_at <= CURRENT_TIMESTAMP))
  )
ORDER BY COALESCE(NULLIF(j.queued_at, ''), j.created_at) ASC, j.id ASC
LIMIT 1;

-- name: ListExecutableRefreshJobHeads :many
WITH eligible AS (
  SELECT j.id, j.workspace_id, r.environment, COALESCE(j.serving_state_id, '') AS serving_state_id, j.model_id, j.kind, j.payload_json,
         r.id AS run_id, r.target_type, r.target_id, r.trigger_type, j.attempt_count,
         ROW_NUMBER() OVER (
           PARTITION BY j.workspace_id
           ORDER BY COALESCE(NULLIF(j.queued_at, ''), j.created_at) ASC, j.id ASC
         ) AS workspace_position,
         COALESCE(NULLIF(j.queued_at, ''), j.created_at) AS queue_position
  FROM refresh_jobs j
  JOIN refresh_job_runs r ON r.job_id = j.id
  WHERE COALESCE(r.parent_run_id, '') = ''
    AND j.kind = sqlc.arg(refresh_pipeline_kind)
    AND r.environment = sqlc.arg(environment)
    AND (
      (j.status = sqlc.arg(queued_status) AND r.status = sqlc.arg(run_queued_status))
      OR (j.status = sqlc.arg(running_status) AND (j.lease_expires_at IS NULL OR j.lease_expires_at <= CURRENT_TIMESTAMP))
    )
)
SELECT id, workspace_id, environment, serving_state_id, model_id, kind, payload_json,
       run_id, target_type, target_id, trigger_type, attempt_count
FROM eligible
WHERE workspace_position = 1
ORDER BY queue_position ASC, id ASC
LIMIT sqlc.arg(result_limit);

-- name: ClaimRefreshJob :execresult
UPDATE refresh_jobs
SET status = sqlc.arg(running_status), started_at = COALESCE(started_at, CURRENT_TIMESTAMP), finished_at = NULL,
    lease_owner = sqlc.arg(lease_owner), lease_expires_at = datetime('now', CAST(sqlc.arg(lease_modifier) AS TEXT)),
    attempt_count = attempt_count + 1, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND (
    status = sqlc.arg(queued_status)
    OR (status = sqlc.arg(previous_running_status) AND (lease_expires_at IS NULL OR lease_expires_at <= CURRENT_TIMESTAMP))
  );

-- name: MarkRefreshJobRunClaimed :exec
UPDATE refresh_job_runs
SET status = sqlc.arg(status), started_at = CURRENT_TIMESTAMP, finished_at = NULL, error = ''
WHERE id = sqlc.arg(id);

-- name: RenewRefreshJobLease :exec
UPDATE refresh_jobs
SET lease_expires_at = datetime('now', CAST(sqlc.arg(lease_modifier) AS TEXT)), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND lease_owner = sqlc.arg(lease_owner) AND status = sqlc.arg(status);

-- name: GetRefreshJobQueueStats :one
SELECT
  CAST(COALESCE(SUM(CASE WHEN j.status = sqlc.arg(queued_status) THEN 1 ELSE 0 END), 0) AS INTEGER) AS queued_jobs,
  CAST(COALESCE(SUM(CASE WHEN j.status = sqlc.arg(running_status) AND j.lease_expires_at IS NOT NULL AND j.lease_expires_at > CURRENT_TIMESTAMP THEN 1 ELSE 0 END), 0) AS INTEGER) AS running_jobs,
  CAST(COALESCE(SUM(CASE WHEN j.status = sqlc.arg(stale_running_status) AND (j.lease_expires_at IS NULL OR j.lease_expires_at <= CURRENT_TIMESTAMP) THEN 1 ELSE 0 END), 0) AS INTEGER) AS stale_leased_jobs
FROM refresh_jobs j
JOIN refresh_job_runs r ON r.job_id = j.id
WHERE COALESCE(r.parent_run_id, '') = ''
  AND j.kind = sqlc.arg(refresh_pipeline_kind)
  AND r.environment = sqlc.arg(environment);

-- name: GetMaterializationRun :one
SELECT r.id, j.workspace_id, r.environment, j.serving_state_id, j.model_id, r.principal_id,
       COALESCE(NULLIF(p.display_name, ''), NULLIF(p.email, ''), r.principal_id, '') AS principal_display_name,
       r.target_type, r.target_id, r.trigger_type, r.parent_run_id, r.retry_of, r.status, j.created_at, j.updated_at,
       r.started_at, r.finished_at, r.error
FROM refresh_job_runs r
JOIN refresh_jobs j ON j.id = r.job_id
LEFT JOIN principals p ON p.id = r.principal_id
WHERE r.id = sqlc.arg(run_id) AND j.workspace_id = sqlc.arg(workspace_id);

-- name: ListChildMaterializationRuns :many
SELECT r.id, j.workspace_id, r.environment, j.serving_state_id, j.model_id, r.principal_id,
       COALESCE(NULLIF(p.display_name, ''), NULLIF(p.email, ''), r.principal_id, '') AS principal_display_name,
       r.target_type, r.target_id, r.trigger_type, r.parent_run_id, r.retry_of, r.status, j.created_at, j.updated_at,
       r.started_at, r.finished_at, r.error
FROM refresh_job_runs r
JOIN refresh_jobs j ON j.id = r.job_id
LEFT JOIN principals p ON p.id = r.principal_id
WHERE j.workspace_id = sqlc.arg(workspace_id) AND r.parent_run_id = sqlc.arg(parent_run_id)
ORDER BY r.rowid ASC;

-- name: LatestSuccessfulMaterializationRun :one
SELECT r.id, j.workspace_id, r.environment, j.serving_state_id, j.model_id, r.principal_id,
       COALESCE(NULLIF(p.display_name, ''), NULLIF(p.email, ''), r.principal_id, '') AS principal_display_name,
       r.target_type, r.target_id, r.trigger_type, r.parent_run_id, r.retry_of, r.status, j.created_at, j.updated_at,
       r.started_at, r.finished_at, r.error
FROM refresh_job_runs r
JOIN refresh_jobs j ON j.id = r.job_id
LEFT JOIN principals p ON p.id = r.principal_id
WHERE j.workspace_id = sqlc.arg(workspace_id) AND r.target_type = sqlc.arg(target_type)
  AND r.environment = sqlc.arg(environment)
  AND r.target_id = sqlc.arg(target_id) AND r.status = sqlc.arg(status)
ORDER BY j.created_at DESC, r.rowid DESC
LIMIT 1;

-- name: FailTerminalServingStateRuns :exec
UPDATE refresh_job_runs
SET status = sqlc.arg(failed_status), finished_at = CURRENT_TIMESTAMP,
    error = CASE WHEN error <> '' THEN error ELSE sqlc.arg(error_message) END
WHERE refresh_job_runs.status IN (sqlc.arg(queued_status), sqlc.arg(running_status))
  AND job_id IN (
    SELECT j.id FROM refresh_jobs j
    JOIN serving_states d ON d.id = j.serving_state_id
    WHERE d.environment = sqlc.arg(environment)
      AND d.status IN ('failed', 'delete_scheduled', 'deleted')
  );

-- name: FailTerminalServingStateJobs :exec
UPDATE refresh_jobs
SET status = sqlc.arg(failed_status), updated_at = CURRENT_TIMESTAMP
WHERE refresh_jobs.status IN (sqlc.arg(queued_status), sqlc.arg(running_status))
  AND serving_state_id IN (
    SELECT id FROM serving_states
    WHERE environment = sqlc.arg(environment)
      AND status IN ('failed', 'delete_scheduled', 'deleted')
  );

-- name: MarkMaterializationRunActive :execresult
UPDATE refresh_job_runs
SET status = sqlc.arg(status), finished_at = finished_at, error = sqlc.arg(error_message)
WHERE refresh_job_runs.id = sqlc.arg(run_id)
  AND job_id IN (SELECT refresh_jobs.id FROM refresh_jobs WHERE workspace_id = sqlc.arg(workspace_id));

-- name: MarkMaterializationRunTerminal :execresult
UPDATE refresh_job_runs
SET status = sqlc.arg(status), finished_at = CURRENT_TIMESTAMP, error = sqlc.arg(error_message)
WHERE refresh_job_runs.id = sqlc.arg(run_id)
  AND job_id IN (SELECT refresh_jobs.id FROM refresh_jobs WHERE workspace_id = sqlc.arg(workspace_id));

-- name: UpdateRefreshJobForActiveRun :exec
UPDATE refresh_jobs
SET status = sqlc.arg(new_status), updated_at = CURRENT_TIMESTAMP
WHERE refresh_jobs.id = (SELECT job_id FROM refresh_job_runs WHERE refresh_job_runs.id = sqlc.arg(run_id))
  AND workspace_id = sqlc.arg(workspace_id);
-- name: CompleteRefreshJobSucceeded :exec
UPDATE refresh_jobs
SET status = 'succeeded', updated_at = CURRENT_TIMESTAMP, finished_at = CURRENT_TIMESTAMP,
    lease_owner = '', lease_expires_at = NULL
WHERE refresh_jobs.id = (SELECT job_id FROM refresh_job_runs WHERE refresh_job_runs.id = sqlc.arg(run_id))
  AND workspace_id = sqlc.arg(workspace_id);

-- name: CompleteRefreshJobFailed :exec
UPDATE refresh_jobs
SET status = 'failed', updated_at = CURRENT_TIMESTAMP, finished_at = CURRENT_TIMESTAMP,
    lease_owner = '', lease_expires_at = NULL, last_error = sqlc.arg(error_message)
WHERE refresh_jobs.id = (SELECT job_id FROM refresh_job_runs WHERE refresh_job_runs.id = sqlc.arg(run_id))
  AND workspace_id = sqlc.arg(workspace_id);
-- name: CancelQueuedMaterializationRun :execresult
UPDATE refresh_job_runs
SET status = sqlc.arg(cancelled_status), finished_at = CURRENT_TIMESTAMP, error = ''
WHERE refresh_job_runs.id = sqlc.arg(run_id)
  AND refresh_job_runs.status = sqlc.arg(queued_status)
  AND refresh_job_runs.job_id IN (
    SELECT refresh_jobs.id FROM refresh_jobs WHERE refresh_jobs.workspace_id = sqlc.arg(workspace_id)
  );

-- name: CancelQueuedRefreshJobForRun :exec
UPDATE refresh_jobs
SET status = sqlc.arg(cancelled_status), finished_at = CURRENT_TIMESTAMP,
    lease_owner = '', lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE refresh_jobs.id = (SELECT refresh_job_runs.job_id FROM refresh_job_runs WHERE refresh_job_runs.id = sqlc.arg(run_id))
  AND refresh_jobs.workspace_id = sqlc.arg(workspace_id)
  AND refresh_jobs.status = sqlc.arg(queued_status);

-- name: CancelQueuedChildMaterializationRuns :exec
UPDATE refresh_job_runs
SET status = sqlc.arg(cancelled_status), finished_at = CURRENT_TIMESTAMP, error = ''
WHERE refresh_job_runs.parent_run_id = sqlc.arg(parent_run_id)
  AND refresh_job_runs.status = sqlc.arg(queued_status)
  AND refresh_job_runs.job_id IN (SELECT id FROM refresh_jobs WHERE workspace_id = sqlc.arg(workspace_id));

-- name: CancelQueuedChildRefreshJobs :exec
UPDATE refresh_jobs
SET status = sqlc.arg(cancelled_status), finished_at = CURRENT_TIMESTAMP,
    lease_owner = '', lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE refresh_jobs.workspace_id = sqlc.arg(workspace_id)
  AND refresh_jobs.status = sqlc.arg(queued_status)
  AND refresh_jobs.id IN (SELECT job_id FROM refresh_job_runs WHERE parent_run_id = sqlc.arg(parent_run_id));

-- name: FailCancelledRefreshCandidate :execresult
UPDATE serving_states
SET status = 'failed', error = 'refresh cancelled'
WHERE id = sqlc.arg(serving_state_id)
  AND source = 'refresh'
  AND status = 'validated';

-- name: ListMaterializationRuns :many
SELECT r.id, j.workspace_id, r.environment, j.serving_state_id, j.model_id, r.principal_id,
       COALESCE(NULLIF(p.display_name, ''), NULLIF(p.email, ''), r.principal_id, '') AS principal_display_name,
       r.target_type, r.target_id, r.trigger_type, r.parent_run_id, r.retry_of, r.status,
       j.created_at, j.updated_at, r.started_at, r.finished_at, r.error
FROM refresh_job_runs r
JOIN refresh_jobs j ON j.id = r.job_id
LEFT JOIN principals p ON p.id = r.principal_id
WHERE j.workspace_id = sqlc.arg(workspace_id)
  AND r.environment = sqlc.arg(environment)
  AND COALESCE(r.parent_run_id, '') = ''
  AND r.target_type = 'refresh_pipeline'
  AND (
    CAST(sqlc.arg(cursor_created_at) AS TEXT) = ''
    OR j.created_at < CAST(sqlc.arg(cursor_created_at) AS TEXT)
    OR (j.created_at = CAST(sqlc.arg(cursor_created_at) AS TEXT) AND r.created_sequence < sqlc.arg(cursor_sequence))
  )
ORDER BY j.created_at DESC, r.created_sequence DESC
LIMIT sqlc.arg(limit);

-- name: ListTargetMaterializationRuns :many
SELECT r.id, j.workspace_id, r.environment, j.serving_state_id, j.model_id, r.principal_id,
       COALESCE(NULLIF(p.display_name, ''), NULLIF(p.email, ''), r.principal_id, '') AS principal_display_name,
       r.target_type, r.target_id, r.trigger_type, r.parent_run_id, r.retry_of, r.status,
       j.created_at, j.updated_at, r.started_at, r.finished_at, r.error
FROM refresh_job_runs r
JOIN refresh_jobs j ON j.id = r.job_id
LEFT JOIN principals p ON p.id = r.principal_id
WHERE j.workspace_id = sqlc.arg(workspace_id)
  AND r.environment = sqlc.arg(environment)
  AND r.target_type = sqlc.arg(target_type)
  AND r.target_id = sqlc.arg(target_id)
  AND (
    CAST(sqlc.arg(cursor_created_at) AS TEXT) = ''
    OR j.created_at < CAST(sqlc.arg(cursor_created_at) AS TEXT)
    OR (j.created_at = CAST(sqlc.arg(cursor_created_at) AS TEXT) AND r.created_sequence < sqlc.arg(cursor_sequence))
  )
ORDER BY j.created_at DESC, r.created_sequence DESC
LIMIT sqlc.arg(limit);

-- name: GetMaterializationRunCursor :one
SELECT j.created_at, r.created_sequence
FROM refresh_job_runs r
JOIN refresh_jobs j ON j.id = r.job_id
WHERE r.id = sqlc.arg(run_id)
  AND j.workspace_id = sqlc.arg(workspace_id)
  AND r.environment = sqlc.arg(environment)
  AND (
    CAST(sqlc.arg(target_type) AS TEXT) = ''
    OR (r.target_type = CAST(sqlc.arg(target_type) AS TEXT) AND r.target_id = sqlc.arg(target_id))
  );
