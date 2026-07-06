-- +goose Up
-- Production access primitives: group membership,
-- binding subjects, revocable sessions/tokens, and audit listing support.

CREATE TABLE IF NOT EXISTS group_members (
  group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(group_id, principal_id)
);

ALTER TABLE sessions ADD COLUMN revoked_at TEXT;
ALTER TABLE api_tokens ADD COLUMN workspace_id TEXT REFERENCES workspaces(id) ON DELETE SET NULL;
ALTER TABLE api_tokens ADD COLUMN privileges_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE api_tokens ADD COLUMN revoked_at TEXT;

INSERT OR IGNORE INTO roles (id, name, privileges_json)
VALUES
  ('role_owner', 'owner', '["USE_WORKSPACE","VIEW_ITEM","EDIT_ITEM","MANAGE_ITEM","QUERY_DATA","PREVIEW_DATA","REFRESH_DATA","DEPLOY","ACTIVATE_PUBLISH","USE_AGENT","VIEW_AGENT","MANAGE_GRANTS","VIEW_AUDIT","MANAGE_WORKSPACE"]'),
  ('role_admin', 'admin', '["USE_WORKSPACE","VIEW_ITEM","EDIT_ITEM","MANAGE_ITEM","QUERY_DATA","PREVIEW_DATA","REFRESH_DATA","DEPLOY","ACTIVATE_PUBLISH","USE_AGENT","VIEW_AGENT","MANAGE_GRANTS","VIEW_AUDIT","MANAGE_WORKSPACE"]'),
  ('role_deployer', 'deployer', '["USE_WORKSPACE","VIEW_ITEM","QUERY_DATA","REFRESH_DATA","DEPLOY","ACTIVATE_PUBLISH"]'),
  ('role_editor', 'editor', '["USE_WORKSPACE","VIEW_ITEM","EDIT_ITEM","REFRESH_DATA","DEPLOY","USE_AGENT","VIEW_AGENT"]'),
  ('role_viewer', 'viewer', '["USE_WORKSPACE","VIEW_ITEM","QUERY_DATA","USE_AGENT","VIEW_AGENT"]');

CREATE INDEX IF NOT EXISTS group_members_principal_idx ON group_members(workspace_id, principal_id);
CREATE INDEX IF NOT EXISTS api_tokens_principal_idx ON api_tokens(principal_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_workspace_created_idx ON audit_events(workspace_id, created_at DESC);
