-- +goose Up
ALTER TABLE assets ADD COLUMN content_version INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE assets DROP COLUMN content_version;
