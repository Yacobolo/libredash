-- +goose Up
-- Forward-only migration: platform migrations do not rebuild SQLite tables for rollback.

CREATE INDEX IF NOT EXISTS materialization_jobs_workspace_created_idx
  ON materialization_jobs(workspace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS materialization_job_runs_target_job_idx
  ON materialization_job_runs(target_type, target_id, job_id);
