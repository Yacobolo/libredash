-- +goose Up
-- Add internal serving-state lifecycle metadata. Stale-state cleanup is
-- lease-gated, so no fixed cleanup timestamp is part of the schema.
ALTER TABLE serving_states ADD COLUMN source TEXT NOT NULL DEFAULT 'publish';
ALTER TABLE serving_states ADD COLUMN superseded_at TEXT;
