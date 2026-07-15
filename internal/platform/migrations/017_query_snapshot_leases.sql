-- +goose Up
-- Persist query snapshot leases so cleanup can protect snapshots even when
-- invoked outside the serving process.
CREATE TABLE IF NOT EXISTS query_snapshot_leases (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL,
  serving_state_id TEXT NOT NULL REFERENCES serving_states(id) ON DELETE CASCADE,
  ducklake_snapshot_id INTEGER NOT NULL,
  owner_id TEXT NOT NULL DEFAULT '',
  acquired_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TEXT NOT NULL,
  released_at TEXT
);

CREATE INDEX IF NOT EXISTS query_snapshot_leases_live_idx
  ON query_snapshot_leases(ducklake_snapshot_id, expires_at)
  WHERE released_at IS NULL;
