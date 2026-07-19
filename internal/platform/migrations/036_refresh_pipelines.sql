-- +goose Up
DELETE FROM refresh_jobs;

ALTER TABLE refresh_job_runs ADD COLUMN environment TEXT NOT NULL DEFAULT 'dev';

CREATE TABLE refresh_pipeline_schedules (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL,
  pipeline_id TEXT NOT NULL,
  semantic_model_id TEXT NOT NULL,
  artifact_digest TEXT NOT NULL,
  cron TEXT NOT NULL,
  timezone TEXT NOT NULL,
  next_run_at TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (workspace_id, environment, pipeline_id, cron, timezone)
);

CREATE UNIQUE INDEX refresh_pipeline_active_run_idx
  ON refresh_job_runs(environment, target_type, target_id)
  WHERE parent_run_id IS NULL
    AND target_type = 'refresh_pipeline'
    AND status IN ('queued', 'running');

CREATE INDEX refresh_pipeline_schedules_due_idx
  ON refresh_pipeline_schedules(next_run_at, workspace_id, environment, pipeline_id);

CREATE TABLE refresh_pipeline_occurrences (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL,
  pipeline_id TEXT NOT NULL,
  artifact_digest TEXT NOT NULL,
  scheduled_at TEXT NOT NULL,
  run_id TEXT REFERENCES refresh_job_runs(id) ON DELETE SET NULL,
  claimed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY (workspace_id, environment, pipeline_id, scheduled_at)
);

CREATE TABLE semantic_model_data_versions (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL,
  semantic_model_id TEXT NOT NULL,
  snapshot_id INTEGER NOT NULL,
  serving_state_id TEXT NOT NULL REFERENCES serving_states(id) ON DELETE RESTRICT,
  refreshed_at TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('publish', 'refresh')),
  pipeline_id TEXT,
  run_id TEXT REFERENCES refresh_job_runs(id) ON DELETE SET NULL,
  PRIMARY KEY (workspace_id, environment, semantic_model_id)
);

-- +goose Down
DROP TABLE IF EXISTS semantic_model_data_versions;
DROP TABLE IF EXISTS refresh_pipeline_occurrences;
DROP INDEX IF EXISTS refresh_pipeline_schedules_due_idx;
DROP TABLE IF EXISTS refresh_pipeline_schedules;
DROP INDEX IF EXISTS refresh_pipeline_active_run_idx;
ALTER TABLE refresh_job_runs DROP COLUMN environment;
