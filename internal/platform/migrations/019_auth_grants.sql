-- +goose Up
-- Canonical Fabric/Unity-Catalog-style grant engine. The product is still in
-- development, so legacy role-permission tables remain only as inert metadata.

ALTER TABLE principals ADD COLUMN kind TEXT NOT NULL DEFAULT 'user';

CREATE TABLE IF NOT EXISTS securable_objects (
  id TEXT PRIMARY KEY,
  object_type TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  parent_id TEXT NOT NULL DEFAULT '',
  owner_principal_id TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS securable_objects_workspace_idx
  ON securable_objects(workspace_id, object_type);

CREATE TABLE IF NOT EXISTS grants (
  id TEXT PRIMARY KEY,
  object_id TEXT NOT NULL REFERENCES securable_objects(id) ON DELETE CASCADE,
  subject_type TEXT NOT NULL,
  subject_id TEXT NOT NULL,
  privilege TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(object_id, subject_type, subject_id, privilege)
);

CREATE INDEX IF NOT EXISTS grants_subject_idx
  ON grants(subject_type, subject_id, privilege);

CREATE TABLE IF NOT EXISTS role_grant_templates (
  role_name TEXT NOT NULL,
  privilege TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(role_name, privilege)
);

CREATE TABLE IF NOT EXISTS data_policies (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  object_id TEXT NOT NULL REFERENCES securable_objects(id) ON DELETE CASCADE,
  policy_type TEXT NOT NULL,
  expression_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
