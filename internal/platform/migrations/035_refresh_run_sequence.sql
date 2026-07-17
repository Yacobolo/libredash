-- +goose Up
ALTER TABLE refresh_job_runs ADD COLUMN created_sequence INTEGER NOT NULL DEFAULT 0;

UPDATE refresh_job_runs
SET created_sequence = rowid
WHERE created_sequence = 0;

CREATE UNIQUE INDEX refresh_job_runs_created_sequence_idx
  ON refresh_job_runs(created_sequence);

-- +goose Down
DROP INDEX IF EXISTS refresh_job_runs_created_sequence_idx;
ALTER TABLE refresh_job_runs DROP COLUMN created_sequence;
