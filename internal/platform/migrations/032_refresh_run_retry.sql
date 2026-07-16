-- +goose Up
ALTER TABLE refresh_job_runs ADD COLUMN retry_of TEXT REFERENCES refresh_job_runs(id) ON DELETE SET NULL;
CREATE INDEX refresh_job_runs_retry_idx ON refresh_job_runs(retry_of);

-- +goose Down
DROP INDEX IF EXISTS refresh_job_runs_retry_idx;
ALTER TABLE refresh_job_runs DROP COLUMN retry_of;
