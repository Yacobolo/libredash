-- +goose Up
-- Legacy string permission tables were replaced by role_grant_templates,
-- securable_objects, and grants. Roles keep privileges_json only as product
-- bundle metadata for display and role template seeding.
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;

-- +goose Down
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
