-- +goose Up
CREATE TABLE api_releases (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  project_digest TEXT NOT NULL,
  request_digest TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'validating', 'ready', 'failed')),
  manifest_json TEXT NOT NULL CHECK(json_valid(manifest_json)),
  created_by TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finalized_at TEXT,
  error TEXT NOT NULL DEFAULT '',
  UNIQUE(project_id, idempotency_key),
  CHECK(length(trim(project_id)) > 0),
  CHECK(length(project_digest) > 0),
  CHECK(length(request_digest) > 0),
  CHECK((status IN ('ready', 'failed')) = (finalized_at IS NOT NULL)),
  CHECK(status <> 'failed' OR length(error) > 0)
);
CREATE INDEX api_releases_project_created_idx ON api_releases(project_id, created_at DESC, id DESC);
CREATE TABLE api_release_artifacts (
  release_id TEXT NOT NULL REFERENCES api_releases(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL,
  expected_digest TEXT NOT NULL,
  serving_state_id TEXT REFERENCES serving_states(id) ON DELETE RESTRICT,
  actual_digest TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL DEFAULT 0 CHECK(size_bytes >= 0),
  uploaded_at TEXT,
  PRIMARY KEY(release_id, workspace_id),
  UNIQUE(release_id, serving_state_id),
  CHECK(length(trim(workspace_id)) > 0),
  CHECK(length(expected_digest) > 0),
  CHECK(uploaded_at IS NULL OR serving_state_id IS NOT NULL)
);
CREATE TABLE api_release_connections (
  release_id TEXT NOT NULL REFERENCES api_releases(id) ON DELETE CASCADE,
  connection_id TEXT NOT NULL,
  revision_id TEXT NOT NULL,
  PRIMARY KEY(release_id, connection_id),
  CHECK(length(trim(connection_id)) > 0),
  CHECK(length(trim(revision_id)) > 0)
);
CREATE TABLE api_deployment_releases (
  deployment_id TEXT PRIMARY KEY REFERENCES project_deployments(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  release_id TEXT NOT NULL REFERENCES api_releases(id) ON DELETE RESTRICT,
  rollback_of TEXT REFERENCES project_deployments(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE api_async_events (
  resource_kind TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  event_id INTEGER NOT NULL CHECK(event_id > 0),
  event_type TEXT NOT NULL,
  data_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(data_json)),
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(resource_kind, resource_id, event_id)
);

-- +goose Down
DROP TABLE api_async_events;
DROP TABLE api_deployment_releases;
DROP TABLE api_release_connections;
DROP TABLE api_release_artifacts;
DROP TABLE api_releases;
