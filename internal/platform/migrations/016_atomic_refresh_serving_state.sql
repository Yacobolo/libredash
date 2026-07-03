-- +goose Up
-- Add internal serving-state lifecycle metadata. Deployments remain the
-- compatibility table name; cleanup_after is legacy nullable metadata.
ALTER TABLE deployments ADD COLUMN source TEXT NOT NULL DEFAULT 'publish';
ALTER TABLE deployments ADD COLUMN superseded_at TEXT;
ALTER TABLE deployments ADD COLUMN cleanup_after TEXT;
