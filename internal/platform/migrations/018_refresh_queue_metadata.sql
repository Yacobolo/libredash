-- +goose Up
ALTER TABLE refresh_jobs ADD COLUMN kind TEXT NOT NULL DEFAULT 'refresh';
ALTER TABLE refresh_jobs ADD COLUMN payload_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE refresh_jobs ADD COLUMN queued_at TEXT NOT NULL DEFAULT '';
ALTER TABLE refresh_jobs ADD COLUMN started_at TEXT;
ALTER TABLE refresh_jobs ADD COLUMN finished_at TEXT;
ALTER TABLE refresh_jobs ADD COLUMN lease_owner TEXT NOT NULL DEFAULT '';
ALTER TABLE refresh_jobs ADD COLUMN lease_expires_at TEXT;
ALTER TABLE refresh_jobs ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE refresh_jobs ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE query_events ADD COLUMN queue_wait_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE query_events ADD COLUMN execution_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE query_events ADD COLUMN execution_state TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS refresh_jobs_claim_idx
  ON refresh_jobs(status, queued_at, id);
CREATE INDEX IF NOT EXISTS refresh_jobs_lease_idx
  ON refresh_jobs(status, lease_expires_at);

UPDATE refresh_jobs
SET queued_at = created_at
WHERE queued_at = '';

-- +goose Down
DROP INDEX IF EXISTS refresh_jobs_lease_idx;
DROP INDEX IF EXISTS refresh_jobs_claim_idx;
