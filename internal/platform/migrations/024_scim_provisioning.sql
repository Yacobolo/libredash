-- +goose Up
ALTER TABLE principals ADD COLUMN disabled_at TEXT;

PRAGMA foreign_keys = OFF;

CREATE TABLE groups_new (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  external_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(workspace_id, provider, external_id)
);

INSERT INTO groups_new (id, workspace_id, provider, external_id, name, created_at)
SELECT id, workspace_id, provider, external_id, name, created_at
FROM groups;

DROP TABLE groups;
ALTER TABLE groups_new RENAME TO groups;

CREATE TABLE group_members_new (
  group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL DEFAULT '',
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(group_id, principal_id)
);

INSERT INTO group_members_new (group_id, workspace_id, principal_id, created_at)
SELECT group_id, workspace_id, principal_id, created_at
FROM group_members;

DROP TABLE group_members;
ALTER TABLE group_members_new RENAME TO group_members;

PRAGMA foreign_keys = ON;

CREATE INDEX IF NOT EXISTS group_members_principal_idx ON group_members(workspace_id, principal_id);

-- +goose Down
DROP INDEX IF EXISTS group_members_principal_idx;
