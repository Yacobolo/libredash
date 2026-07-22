-- +goose Up
ALTER TABLE api_async_jobs ADD COLUMN workload_class TEXT NOT NULL DEFAULT 'control'
  CHECK (workload_class IN ('background', 'control'));
ALTER TABLE api_async_jobs ADD COLUMN workspace_id TEXT NOT NULL DEFAULT '_node';

UPDATE api_async_jobs
SET workload_class = 'background',
    workspace_id = COALESCE(NULLIF(json_extract(payload_json, '$.Scope.WorkspaceID'), ''), '_global')
WHERE job_kind = 'agent.run';

DROP INDEX api_async_jobs_claim_idx;
CREATE INDEX api_async_jobs_claim_idx
  ON api_async_jobs(workload_class, workspace_id, status, lease_expires_at, created_at, id);

-- +goose Down
DROP INDEX api_async_jobs_claim_idx;
CREATE INDEX api_async_jobs_claim_idx ON api_async_jobs(status, lease_expires_at, created_at, id);
ALTER TABLE api_async_jobs DROP COLUMN workspace_id;
ALTER TABLE api_async_jobs DROP COLUMN workload_class;
