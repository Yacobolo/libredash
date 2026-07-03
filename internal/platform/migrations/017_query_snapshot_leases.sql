-- +goose Up
-- Persist query snapshot leases so cleanup can protect snapshots even when
-- invoked outside the serving process.
ALTER TABLE deployment_artifacts ADD COLUMN data_root TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS query_snapshot_leases (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL,
  deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  ducklake_snapshot_id INTEGER NOT NULL,
  owner_id TEXT NOT NULL DEFAULT '',
  acquired_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TEXT NOT NULL,
  released_at TEXT
);

CREATE INDEX IF NOT EXISTS query_snapshot_leases_live_idx
  ON query_snapshot_leases(ducklake_snapshot_id, expires_at)
  WHERE released_at IS NULL;
