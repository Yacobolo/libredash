-- +goose Up
ALTER TABLE materialization_job_runs
  ADD COLUMN target_type TEXT NOT NULL DEFAULT 'semantic_model';

ALTER TABLE materialization_job_runs
  ADD COLUMN target_id TEXT NOT NULL DEFAULT '';

ALTER TABLE materialization_job_runs
  ADD COLUMN trigger_type TEXT NOT NULL DEFAULT 'direct';

ALTER TABLE materialization_job_runs
  ADD COLUMN parent_run_id TEXT REFERENCES materialization_job_runs(id) ON DELETE SET NULL;

UPDATE materialization_job_runs
SET target_id = COALESCE((
  SELECT model_id
  FROM materialization_jobs
  WHERE materialization_jobs.id = materialization_job_runs.job_id
), '')
WHERE target_id = '';

CREATE INDEX IF NOT EXISTS materialization_job_runs_target_idx
  ON materialization_job_runs(target_type, target_id, started_at DESC);

CREATE INDEX IF NOT EXISTS materialization_job_runs_parent_idx
  ON materialization_job_runs(parent_run_id);

-- +goose Down
DROP INDEX IF EXISTS materialization_job_runs_parent_idx;
DROP INDEX IF EXISTS materialization_job_runs_target_idx;
