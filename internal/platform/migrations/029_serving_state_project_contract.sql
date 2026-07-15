-- +goose Up
ALTER TABLE serving_states ADD COLUMN project_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE serving_states ADD COLUMN project_workspaces_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE serving_states ADD COLUMN access_policy_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE serving_states DROP COLUMN access_policy_json;
ALTER TABLE serving_states DROP COLUMN project_workspaces_json;
ALTER TABLE serving_states DROP COLUMN project_digest;
