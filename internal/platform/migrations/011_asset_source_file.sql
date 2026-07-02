-- +goose Up
-- Preserve source file metadata for asset detail/debug surfaces.

ALTER TABLE assets ADD COLUMN source_file TEXT NOT NULL DEFAULT '';
