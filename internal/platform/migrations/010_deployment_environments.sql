-- +goose Up
-- Add deployment environments and active-deployment pointers for stores that
-- applied the original platform migration before multi-environment deployments.

ALTER TABLE deployments ADD COLUMN environment TEXT NOT NULL DEFAULT 'dev';

CREATE TABLE IF NOT EXISTS workspace_active_deployments (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL,
  deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(workspace_id, environment)
);

ALTER TABLE deployment_artifacts ADD COLUMN environment TEXT NOT NULL DEFAULT 'dev';

INSERT OR IGNORE INTO workspace_active_deployments (workspace_id, environment, deployment_id, updated_at)
SELECT workspace_id, environment, id, COALESCE(activated_at, CURRENT_TIMESTAMP)
FROM deployments
WHERE status = 'active';
