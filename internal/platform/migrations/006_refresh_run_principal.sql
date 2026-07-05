-- +goose Up
-- Forward-only migration: platform migrations do not rebuild SQLite tables for rollback.

ALTER TABLE refresh_job_runs
ADD COLUMN principal_id TEXT REFERENCES principals(id) ON DELETE SET NULL;
