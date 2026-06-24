-- +goose Up
-- Production access primitives: explicit permissions, group membership,
-- binding subjects, revocable sessions/tokens, and audit listing support.

CREATE TABLE IF NOT EXISTS group_members (
  group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(group_id, principal_id)
);

CREATE TABLE IF NOT EXISTS permissions (
  name TEXT PRIMARY KEY,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS role_permissions (
  role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  permission_name TEXT NOT NULL REFERENCES permissions(name) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(role_id, permission_name)
);

ALTER TABLE sessions ADD COLUMN revoked_at TEXT;
ALTER TABLE api_tokens ADD COLUMN workspace_id TEXT REFERENCES workspaces(id) ON DELETE SET NULL;
ALTER TABLE api_tokens ADD COLUMN permissions_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE api_tokens ADD COLUMN revoked_at TEXT;

INSERT OR IGNORE INTO permissions (name)
VALUES
  ('workspace:read'),
  ('asset:read'),
  ('deployment:read'),
  ('deployment:write'),
  ('deployment:activate'),
  ('rbac:read'),
  ('rbac:write'),
  ('agent:use'),
  ('agent:read'),
  ('materialization:run'),
  ('audit:read'),
  ('token:manage');

INSERT OR IGNORE INTO roles (id, name, permissions_json)
VALUES
  ('role_owner', 'owner', '["workspace:read","asset:read","deployment:read","deployment:write","deployment:activate","rbac:read","rbac:write","agent:use","agent:read","materialization:run","audit:read","token:manage"]'),
  ('role_admin', 'admin', '["workspace:read","asset:read","deployment:read","deployment:write","deployment:activate","rbac:read","rbac:write","agent:use","agent:read","materialization:run","audit:read","token:manage"]'),
  ('role_deployer', 'deployer', '["workspace:read","asset:read","deployment:read","deployment:write","deployment:activate","materialization:run"]'),
  ('role_editor', 'editor', '["workspace:read","asset:read","agent:use","agent:read","materialization:run"]'),
  ('role_viewer', 'viewer', '["workspace:read","asset:read","agent:use","agent:read"]');

INSERT OR IGNORE INTO role_permissions (role_id, permission_name)
SELECT r.id, p.name
FROM roles r
JOIN permissions p ON p.name IN (
  'workspace:read',
  'asset:read',
  'deployment:read',
  'deployment:write',
  'deployment:activate',
  'rbac:read',
  'rbac:write',
  'agent:use',
  'agent:read',
  'materialization:run',
  'audit:read',
  'token:manage'
)
WHERE r.name IN ('owner', 'admin');

INSERT OR IGNORE INTO role_permissions (role_id, permission_name)
SELECT r.id, p.name
FROM roles r
JOIN permissions p ON p.name IN (
  'workspace:read',
  'asset:read',
  'deployment:read',
  'deployment:write',
  'deployment:activate',
  'materialization:run'
)
WHERE r.name = 'deployer';

INSERT OR IGNORE INTO role_permissions (role_id, permission_name)
SELECT r.id, p.name
FROM roles r
JOIN permissions p ON p.name IN ('workspace:read', 'asset:read', 'agent:use', 'agent:read', 'materialization:run')
WHERE r.name = 'editor';

INSERT OR IGNORE INTO role_permissions (role_id, permission_name)
SELECT r.id, p.name
FROM roles r
JOIN permissions p ON p.name IN ('workspace:read', 'asset:read', 'agent:use', 'agent:read')
WHERE r.name = 'viewer';

CREATE INDEX IF NOT EXISTS group_members_principal_idx ON group_members(workspace_id, principal_id);
CREATE INDEX IF NOT EXISTS api_tokens_principal_idx ON api_tokens(principal_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_workspace_created_idx ON audit_events(workspace_id, created_at DESC);
