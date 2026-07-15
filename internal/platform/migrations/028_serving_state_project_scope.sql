-- +goose Up
ALTER TABLE serving_states ADD COLUMN project_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE serving_states DROP COLUMN project_id;
