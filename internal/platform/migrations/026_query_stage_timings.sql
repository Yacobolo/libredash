-- +goose Up
-- Keep stable, separately attributable query stages for admission, planning,
-- reader acquisition, and database execution. Existing rows retain zero values.
ALTER TABLE query_events ADD COLUMN planning_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE query_events ADD COLUMN connection_wait_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE query_events ADD COLUMN database_ms INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot safely drop these columns on every supported deployment version.
