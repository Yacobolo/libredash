-- +goose Up
-- Complete auth-spec follow-ups: subject-scoped data policies.

ALTER TABLE data_policies ADD COLUMN subject_type TEXT NOT NULL DEFAULT '';
ALTER TABLE data_policies ADD COLUMN subject_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS data_policies_subject_idx
  ON data_policies(workspace_id, subject_type, subject_id);

-- +goose Down
DROP INDEX IF EXISTS data_policies_subject_idx;
