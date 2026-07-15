CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS serving_states (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL DEFAULT '',
  project_digest TEXT NOT NULL DEFAULT '',
  project_workspaces_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(project_workspaces_json)),
  access_policy_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(access_policy_json)),
  environment TEXT NOT NULL DEFAULT 'dev',
  status TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'publish',
  digest TEXT NOT NULL DEFAULT '',
  manifest_json TEXT NOT NULL DEFAULT '{}',
  ducklake_snapshot_id INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  activated_at TEXT,
  superseded_at TEXT,
  error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS workspace_active_serving_states (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL,
  serving_state_id TEXT NOT NULL REFERENCES serving_states(id) ON DELETE CASCADE,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(workspace_id, environment)
);

CREATE TABLE IF NOT EXISTS platform_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS serving_state_artifacts (
  id TEXT PRIMARY KEY,
  serving_state_id TEXT NOT NULL UNIQUE REFERENCES serving_states(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL DEFAULT 'dev',
  digest TEXT NOT NULL,
  format TEXT NOT NULL,
  path TEXT NOT NULL,
  manifest_json TEXT NOT NULL DEFAULT '{}',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

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

CREATE TABLE IF NOT EXISTS assets (
  snapshot_id TEXT PRIMARY KEY,
  logical_asset_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  serving_state_id TEXT NOT NULL REFERENCES serving_states(id) ON DELETE CASCADE,
  asset_type TEXT NOT NULL,
  asset_key TEXT NOT NULL,
  parent_logical_asset_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  source_file TEXT NOT NULL DEFAULT '',
  payload_schema TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  content_hash TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(serving_state_id, logical_asset_id)
);

CREATE TABLE IF NOT EXISTS asset_edges (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  serving_state_id TEXT NOT NULL REFERENCES serving_states(id) ON DELETE CASCADE,
  from_logical_asset_id TEXT NOT NULL,
  to_logical_asset_id TEXT NOT NULL,
  edge_type TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS principals (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL DEFAULT 'user',
  email TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  disabled_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS external_identities (
  id TEXT PRIMARY KEY,
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  tenant_id TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(provider, tenant_id, subject)
);

CREATE TABLE IF NOT EXISTS groups (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  external_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(workspace_id, provider, external_id)
);

CREATE TABLE IF NOT EXISTS group_members (
  group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL DEFAULT '',
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(group_id, principal_id)
);

CREATE TABLE IF NOT EXISTS roles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  privileges_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS role_bindings (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  principal_id TEXT REFERENCES principals(id) ON DELETE CASCADE,
  group_id TEXT REFERENCES groups(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS platform_role_bindings (
  id TEXT PRIMARY KEY,
  role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  token_fingerprint TEXT NOT NULL UNIQUE,
  token_verifier TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS oauth_states (
  id TEXT PRIMARY KEY,
  state_hash TEXT NOT NULL UNIQUE,
  redirect_url TEXT NOT NULL DEFAULT '/',
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS api_tokens (
  id TEXT PRIMARY KEY,
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  workspace_id TEXT REFERENCES workspaces(id) ON DELETE SET NULL,
  name TEXT NOT NULL,
  token_fingerprint TEXT NOT NULL UNIQUE,
  token_verifier TEXT NOT NULL,
  privileges_json TEXT NOT NULL DEFAULT '[]',
  expires_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_used_at TEXT,
  revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS service_principal_secrets (
  id TEXT PRIMARY KEY,
  service_principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  secret_fingerprint TEXT NOT NULL,
  secret_verifier TEXT NOT NULL,
  expires_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS local_user_credentials (
  principal_id TEXT PRIMARY KEY REFERENCES principals(id) ON DELETE CASCADE,
  password_verifier TEXT NOT NULL,
  must_change_password INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  password_changed_at TEXT
);

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
  subject_type TEXT NOT NULL DEFAULT '',
  subject_id TEXT NOT NULL DEFAULT '',
  policy_type TEXT NOT NULL,
  expression_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS refresh_jobs (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  serving_state_id TEXT REFERENCES serving_states(id) ON DELETE SET NULL,
  model_id TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'refresh',
  payload_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL,
  queued_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  started_at TEXT,
  finished_at TEXT,
  lease_owner TEXT NOT NULL DEFAULT '',
  lease_expires_at TEXT,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS refresh_job_runs (
  id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES refresh_jobs(id) ON DELETE CASCADE,
  principal_id TEXT REFERENCES principals(id) ON DELETE SET NULL,
  target_type TEXT NOT NULL DEFAULT 'semantic_model',
  target_id TEXT NOT NULL DEFAULT '',
  trigger_type TEXT NOT NULL DEFAULT 'direct',
  parent_run_id TEXT REFERENCES refresh_job_runs(id) ON DELETE SET NULL,
  status TEXT NOT NULL,
  started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TEXT,
  error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS refresh_job_runs_target_idx
  ON refresh_job_runs(target_type, target_id, started_at DESC);
CREATE INDEX IF NOT EXISTS refresh_job_runs_parent_idx
  ON refresh_job_runs(parent_run_id);
CREATE INDEX IF NOT EXISTS refresh_jobs_workspace_created_idx
  ON refresh_jobs(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS refresh_jobs_claim_idx
  ON refresh_jobs(status, queued_at, id);
CREATE INDEX IF NOT EXISTS refresh_jobs_lease_idx
  ON refresh_jobs(status, lease_expires_at);
CREATE INDEX IF NOT EXISTS refresh_job_runs_target_job_idx
  ON refresh_job_runs(target_type, target_id, job_id);

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  workspace_id TEXT REFERENCES workspaces(id) ON DELETE SET NULL,
  principal_id TEXT REFERENCES principals(id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  target_type TEXT NOT NULL DEFAULT '',
  target_id TEXT NOT NULL DEFAULT '',
  privilege TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  correlation_id TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS query_events (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  principal_id TEXT NOT NULL DEFAULT '',
  surface TEXT NOT NULL DEFAULT '',
  operation TEXT NOT NULL DEFAULT '',
  query_kind TEXT NOT NULL DEFAULT '',
  model_id TEXT NOT NULL DEFAULT '',
  target TEXT NOT NULL DEFAULT '',
  object_type TEXT NOT NULL DEFAULT '',
  object_id TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT '',
  correlation_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  queue_wait_ms INTEGER NOT NULL DEFAULT 0,
  execution_ms INTEGER NOT NULL DEFAULT 0,
  execution_state TEXT NOT NULL DEFAULT '',
  rows_returned INTEGER NOT NULL DEFAULT 0,
  bytes_estimate INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  sql_text TEXT NOT NULL DEFAULT '',
  plan_text TEXT NOT NULL DEFAULT '',
  query_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_conversations (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  title TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  transcript_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  archived_at TEXT
);

CREATE TABLE IF NOT EXISTS agent_runs (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES agent_conversations(id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  model TEXT NOT NULL DEFAULT '',
  stop_reason TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS agent_messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES agent_conversations(id) ON DELETE CASCADE,
  run_id TEXT REFERENCES agent_runs(id) ON DELETE SET NULL,
  seq INTEGER NOT NULL,
  role TEXT NOT NULL,
  content_text TEXT NOT NULL DEFAULT '',
  content_json TEXT NOT NULL DEFAULT '{}',
  tool_call_id TEXT NOT NULL DEFAULT '',
  tool_name TEXT NOT NULL DEFAULT '',
  is_error BOOLEAN NOT NULL DEFAULT false,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(conversation_id, seq)
);

CREATE TABLE IF NOT EXISTS agent_events (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL,
  event_type TEXT NOT NULL,
  severity TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(run_id, seq)
);

CREATE INDEX IF NOT EXISTS serving_states_workspace_created_idx ON serving_states(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS assets_serving_state_type_idx ON assets(serving_state_id, asset_type);
CREATE INDEX IF NOT EXISTS assets_serving_state_logical_idx ON assets(serving_state_id, logical_asset_id);
CREATE UNIQUE INDEX IF NOT EXISTS asset_edges_unique_idx
  ON asset_edges(serving_state_id, from_logical_asset_id, to_logical_asset_id, edge_type);
CREATE INDEX IF NOT EXISTS role_bindings_principal_idx ON role_bindings(workspace_id, principal_id);
CREATE INDEX IF NOT EXISTS group_members_principal_idx ON group_members(workspace_id, principal_id);
CREATE UNIQUE INDEX IF NOT EXISTS role_bindings_principal_unique_idx
  ON role_bindings(workspace_id, role_id, principal_id)
  WHERE principal_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS role_bindings_group_unique_idx
  ON role_bindings(workspace_id, role_id, group_id)
  WHERE group_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS platform_role_bindings_principal_unique_idx
  ON platform_role_bindings(role_id, principal_id);
CREATE UNIQUE INDEX IF NOT EXISTS sessions_token_fingerprint_unique_idx
  ON sessions(token_fingerprint)
  WHERE token_fingerprint IS NOT NULL AND token_fingerprint <> '';
CREATE INDEX IF NOT EXISTS api_tokens_principal_idx ON api_tokens(principal_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS api_tokens_token_fingerprint_unique_idx
  ON api_tokens(token_fingerprint)
  WHERE token_fingerprint IS NOT NULL AND token_fingerprint <> '';
CREATE INDEX IF NOT EXISTS service_principal_secrets_principal_idx ON service_principal_secrets(service_principal_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS service_principal_secrets_fingerprint_unique_idx
  ON service_principal_secrets(service_principal_id, secret_fingerprint)
  WHERE secret_fingerprint IS NOT NULL AND secret_fingerprint <> '';
CREATE INDEX IF NOT EXISTS audit_events_workspace_created_idx ON audit_events(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS query_events_workspace_created_idx ON query_events(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS query_events_principal_created_idx ON query_events(principal_id, created_at DESC);
CREATE INDEX IF NOT EXISTS query_events_surface_created_idx ON query_events(surface, created_at DESC);
CREATE INDEX IF NOT EXISTS query_events_status_created_idx ON query_events(status, created_at DESC);
CREATE INDEX IF NOT EXISTS agent_conversations_owner_updated_idx ON agent_conversations(principal_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS agent_messages_conversation_seq_idx ON agent_messages(conversation_id, seq);
CREATE INDEX IF NOT EXISTS agent_runs_conversation_started_idx ON agent_runs(conversation_id, started_at DESC);
CREATE INDEX IF NOT EXISTS agent_events_run_seq_idx ON agent_events(run_id, seq);

CREATE TABLE IF NOT EXISTS managed_data_collections (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  connection_name TEXT NOT NULL COLLATE NOCASE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'archived')),
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  archived_at TEXT,
  UNIQUE(project_id, connection_name),
  CHECK(length(id) BETWEEN 1 AND 128),
  CHECK(length(trim(project_id)) > 0),
  CHECK(length(trim(connection_name)) > 0),
  CHECK(length(trim(name)) > 0),
  CHECK((status = 'archived') = (archived_at IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS managed_data_revisions (
  id TEXT PRIMARY KEY,
  collection_id TEXT NOT NULL REFERENCES managed_data_collections(id) ON DELETE RESTRICT,
  sequence INTEGER NOT NULL CHECK(sequence > 0),
  digest TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'ready', 'failed')),
  manifest_json TEXT NOT NULL,
  file_count INTEGER NOT NULL CHECK(file_count >= 0),
  size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ready_at TEXT,
  error TEXT NOT NULL DEFAULT '',
  UNIQUE(collection_id, id),
  UNIQUE(collection_id, sequence),
  UNIQUE(collection_id, digest),
  CHECK(length(digest) = 71 AND substr(digest, 1, 7) = 'sha256:'),
  CHECK(json_valid(manifest_json)),
  CHECK((status = 'ready') = (ready_at IS NOT NULL)),
  CHECK(status <> 'failed' OR length(error) > 0)
);

CREATE TRIGGER IF NOT EXISTS managed_data_ready_revision_immutable
BEFORE UPDATE ON managed_data_revisions
WHEN OLD.status = 'ready'
BEGIN
  SELECT RAISE(ABORT, 'ready managed data revisions are immutable');
END;

CREATE TABLE IF NOT EXISTS managed_data_revision_files (
  revision_id TEXT NOT NULL REFERENCES managed_data_revisions(id) ON DELETE CASCADE,
  logical_path TEXT NOT NULL COLLATE NOCASE,
  size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
  sha256 TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  media_type TEXT NOT NULL DEFAULT '',
  etag TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(revision_id, logical_path),
  CHECK(length(logical_path) > 0),
  CHECK(length(sha256) = 64),
  CHECK(length(storage_key) > 0)
);

CREATE TABLE IF NOT EXISTS managed_data_upload_sessions (
  id TEXT PRIMARY KEY,
  collection_id TEXT NOT NULL REFERENCES managed_data_collections(id) ON DELETE RESTRICT,
  base_revision_id TEXT,
  revision_id TEXT,
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'committing', 'complete', 'aborted', 'expired', 'failed')),
  manifest_json TEXT NOT NULL,
  expected_file_count INTEGER NOT NULL CHECK(expected_file_count >= 0),
  expected_size_bytes INTEGER NOT NULL CHECK(expected_size_bytes >= 0),
  uploaded_file_count INTEGER NOT NULL DEFAULT 0 CHECK(uploaded_file_count >= 0),
  uploaded_size_bytes INTEGER NOT NULL DEFAULT 0 CHECK(uploaded_size_bytes >= 0),
  storage_backend TEXT NOT NULL,
  staging_prefix TEXT NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TEXT NOT NULL,
  completed_at TEXT,
  error TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(collection_id, base_revision_id) REFERENCES managed_data_revisions(collection_id, id) ON DELETE RESTRICT,
  FOREIGN KEY(collection_id, revision_id) REFERENCES managed_data_revisions(collection_id, id) ON DELETE RESTRICT,
  CHECK(json_valid(manifest_json)),
  CHECK(length(storage_backend) > 0),
  CHECK(length(staging_prefix) > 0),
  CHECK((status = 'complete') = (revision_id IS NOT NULL)),
  CHECK((status = 'complete') = (completed_at IS NOT NULL)),
  CHECK(status <> 'failed' OR length(error) > 0)
);

CREATE INDEX IF NOT EXISTS managed_data_upload_sessions_expiry_idx
  ON managed_data_upload_sessions(status, expires_at);

CREATE TABLE IF NOT EXISTS managed_data_s3_multipart_uploads (
  id TEXT PRIMARY KEY,
  upload_session_id TEXT NOT NULL REFERENCES managed_data_upload_sessions(id) ON DELETE CASCADE,
  logical_path TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
  object_key TEXT NOT NULL DEFAULT '',
  provider_upload_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'creating'
    CHECK(status IN ('creating', 'open', 'completing', 'completed', 'aborting', 'aborted', 'failed')),
  existing INTEGER NOT NULL DEFAULT 0 CHECK(existing IN (0, 1)),
  idempotency_identity TEXT NOT NULL,
  completion_identity TEXT NOT NULL DEFAULT '',
  completion_request_hash TEXT NOT NULL DEFAULT '',
  abort_identity TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TEXT,
  aborted_at TEXT,
  error TEXT NOT NULL DEFAULT '',
  UNIQUE(upload_session_id, idempotency_identity),
  CHECK(length(logical_path) > 0),
  CHECK(length(sha256) = 64 AND sha256 = lower(sha256)),
  CHECK(length(idempotency_identity) = 64),
  CHECK(completion_identity = '' OR length(completion_identity) = 64),
  CHECK(completion_request_hash = '' OR length(completion_request_hash) = 64),
  CHECK(abort_identity = '' OR length(abort_identity) = 64),
  CHECK(status = 'creating' OR length(object_key) > 0 OR status IN ('aborting', 'aborted')),
  CHECK(existing = 0 OR status = 'completed'),
  CHECK((status = 'completed') = (completed_at IS NOT NULL)),
  CHECK((status = 'aborted') = (aborted_at IS NOT NULL)),
  CHECK(status <> 'failed' OR length(error) > 0),
  CHECK(length(error) <= 2048)
);

CREATE INDEX IF NOT EXISTS managed_data_s3_multipart_recovery_idx
  ON managed_data_s3_multipart_uploads(status, updated_at, id);

CREATE TABLE IF NOT EXISTS managed_data_s3_multipart_parts (
  multipart_upload_id TEXT NOT NULL REFERENCES managed_data_s3_multipart_uploads(id) ON DELETE CASCADE,
  part_number INTEGER NOT NULL CHECK(part_number BETWEEN 1 AND 10000),
  size_bytes INTEGER NOT NULL CHECK(size_bytes > 0),
  sha256 TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(multipart_upload_id, part_number),
  CHECK(sha256 = '' OR (length(sha256) = 64 AND sha256 = lower(sha256)))
);

CREATE INDEX IF NOT EXISTS managed_data_revisions_collection_created_idx
  ON managed_data_revisions(collection_id, sequence DESC);

CREATE TABLE IF NOT EXISTS project_deployments (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  environment TEXT NOT NULL,
  request_digest TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'active', 'failed', 'superseded')),
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  activated_at TEXT,
  error TEXT NOT NULL DEFAULT '',
  CHECK(length(trim(project_id)) > 0),
  CHECK(length(environment) BETWEEN 1 AND 128),
  CHECK(length(request_digest) > 0),
  CHECK((status IN ('active', 'superseded')) = (activated_at IS NOT NULL)),
  CHECK(status <> 'failed' OR length(error) > 0)
);

CREATE TABLE IF NOT EXISTS project_deployment_targets (
  deployment_id TEXT NOT NULL REFERENCES project_deployments(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
  serving_state_id TEXT NOT NULL REFERENCES serving_states(id) ON DELETE RESTRICT,
  prior_serving_state_id TEXT REFERENCES serving_states(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'active', 'failed')),
  activated_at TEXT,
  error TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(deployment_id, workspace_id),
  UNIQUE(deployment_id, serving_state_id),
  CHECK(serving_state_id <> COALESCE(prior_serving_state_id, '')),
  CHECK((status = 'active') = (activated_at IS NOT NULL)),
  CHECK(status <> 'failed' OR length(error) > 0)
);

CREATE INDEX IF NOT EXISTS project_deployment_targets_serving_state_idx
  ON project_deployment_targets(serving_state_id);

CREATE TABLE IF NOT EXISTS project_deployment_connections (
  deployment_id TEXT NOT NULL REFERENCES project_deployments(id) ON DELETE CASCADE,
  collection_id TEXT NOT NULL,
  revision_id TEXT NOT NULL,
  prior_revision_id TEXT,
  prior_generation INTEGER NOT NULL DEFAULT 0 CHECK(prior_generation >= 0),
  activated_generation INTEGER CHECK(activated_generation > 0),
  PRIMARY KEY(deployment_id, collection_id),
  FOREIGN KEY(collection_id, revision_id) REFERENCES managed_data_revisions(collection_id, id) ON DELETE RESTRICT,
  FOREIGN KEY(collection_id, prior_revision_id) REFERENCES managed_data_revisions(collection_id, id) ON DELETE RESTRICT,
  CHECK((prior_generation = 0) = (prior_revision_id IS NULL))
);

CREATE TABLE IF NOT EXISTS managed_data_environment_pointers (
  collection_id TEXT NOT NULL,
  environment TEXT NOT NULL,
  revision_id TEXT NOT NULL,
  deployment_id TEXT NOT NULL REFERENCES project_deployments(id) ON DELETE RESTRICT,
  generation INTEGER NOT NULL CHECK(generation > 0),
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(collection_id, environment),
  FOREIGN KEY(collection_id, revision_id) REFERENCES managed_data_revisions(collection_id, id) ON DELETE RESTRICT,
  CHECK(length(environment) BETWEEN 1 AND 128)
);

CREATE INDEX IF NOT EXISTS managed_data_environment_pointers_revision_idx
  ON managed_data_environment_pointers(revision_id);

CREATE INDEX IF NOT EXISTS project_deployments_project_environment_idx
  ON project_deployments(project_id, environment, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS managed_data_serving_state_bindings (
  serving_state_id TEXT NOT NULL REFERENCES serving_states(id) ON DELETE CASCADE,
  collection_id TEXT NOT NULL,
  revision_id TEXT NOT NULL,
  environment TEXT NOT NULL,
  bound_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(serving_state_id, collection_id),
  FOREIGN KEY(collection_id, revision_id) REFERENCES managed_data_revisions(collection_id, id) ON DELETE RESTRICT,
  CHECK(length(environment) BETWEEN 1 AND 128)
);

CREATE INDEX IF NOT EXISTS managed_data_serving_state_bindings_revision_idx
  ON managed_data_serving_state_bindings(revision_id);
