-- +goose Up
-- Add serving state environments and active-serving state pointers for stores that
-- applied the original platform migration before multi-environment serving_states.

ALTER TABLE serving_states ADD COLUMN environment TEXT NOT NULL DEFAULT 'dev';

CREATE TABLE IF NOT EXISTS workspace_active_serving_states (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL,
  serving_state_id TEXT NOT NULL REFERENCES serving_states(id) ON DELETE CASCADE,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(workspace_id, environment)
);

ALTER TABLE serving_state_artifacts ADD COLUMN environment TEXT NOT NULL DEFAULT 'dev';

INSERT OR IGNORE INTO workspace_active_serving_states (workspace_id, environment, serving_state_id, updated_at)
SELECT workspace_id, environment, id, COALESCE(activated_at, CURRENT_TIMESTAMP)
FROM serving_states
WHERE status = 'active';
