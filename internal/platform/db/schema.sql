CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS deployments (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL DEFAULT 'dev',
  status TEXT NOT NULL,
  digest TEXT NOT NULL DEFAULT '',
  manifest_json TEXT NOT NULL DEFAULT '{}',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  activated_at TEXT,
  error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS workspace_active_deployments (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL,
  deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(workspace_id, environment)
);

CREATE TABLE IF NOT EXISTS platform_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS deployment_artifacts (
  id TEXT PRIMARY KEY,
  deployment_id TEXT NOT NULL UNIQUE REFERENCES deployments(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment TEXT NOT NULL DEFAULT 'dev',
  digest TEXT NOT NULL,
  format TEXT NOT NULL,
  path TEXT NOT NULL,
  manifest_json TEXT NOT NULL DEFAULT '{}',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS assets (
  snapshot_id TEXT PRIMARY KEY,
  logical_asset_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
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
  UNIQUE(deployment_id, logical_asset_id)
);

CREATE TABLE IF NOT EXISTS asset_edges (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  from_logical_asset_id TEXT NOT NULL,
  to_logical_asset_id TEXT NOT NULL,
  edge_type TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS principals (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
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
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  provider TEXT NOT NULL DEFAULT '',
  external_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(workspace_id, provider, external_id)
);

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

CREATE TABLE IF NOT EXISTS roles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  permissions_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS role_permissions (
  role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  permission_name TEXT NOT NULL REFERENCES permissions(name) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(role_id, permission_name)
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
  token_hash TEXT NOT NULL UNIQUE,
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
  token_hash TEXT NOT NULL UNIQUE,
  permissions_json TEXT NOT NULL DEFAULT '[]',
  expires_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_used_at TEXT,
  revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS materialization_jobs (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  deployment_id TEXT REFERENCES deployments(id) ON DELETE SET NULL,
  model_id TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS materialization_job_runs (
  id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES materialization_jobs(id) ON DELETE CASCADE,
  principal_id TEXT REFERENCES principals(id) ON DELETE SET NULL,
  target_type TEXT NOT NULL DEFAULT 'semantic_model',
  target_id TEXT NOT NULL DEFAULT '',
  trigger_type TEXT NOT NULL DEFAULT 'direct',
  parent_run_id TEXT REFERENCES materialization_job_runs(id) ON DELETE SET NULL,
  status TEXT NOT NULL,
  started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TEXT,
  error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS materialization_job_runs_target_idx
  ON materialization_job_runs(target_type, target_id, started_at DESC);
CREATE INDEX IF NOT EXISTS materialization_job_runs_parent_idx
  ON materialization_job_runs(parent_run_id);
CREATE INDEX IF NOT EXISTS materialization_jobs_workspace_created_idx
  ON materialization_jobs(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS materialization_job_runs_target_job_idx
  ON materialization_job_runs(target_type, target_id, job_id);

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  workspace_id TEXT REFERENCES workspaces(id) ON DELETE SET NULL,
  principal_id TEXT REFERENCES principals(id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  target_type TEXT NOT NULL DEFAULT '',
  target_id TEXT NOT NULL DEFAULT '',
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

CREATE INDEX IF NOT EXISTS deployments_workspace_created_idx ON deployments(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS assets_deployment_type_idx ON assets(deployment_id, asset_type);
CREATE INDEX IF NOT EXISTS assets_deployment_logical_idx ON assets(deployment_id, logical_asset_id);
CREATE UNIQUE INDEX IF NOT EXISTS asset_edges_unique_idx
  ON asset_edges(deployment_id, from_logical_asset_id, to_logical_asset_id, edge_type);
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
CREATE INDEX IF NOT EXISTS sessions_token_hash_idx ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS api_tokens_principal_idx ON api_tokens(principal_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_workspace_created_idx ON audit_events(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS query_events_workspace_created_idx ON query_events(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS query_events_principal_created_idx ON query_events(principal_id, created_at DESC);
CREATE INDEX IF NOT EXISTS query_events_surface_created_idx ON query_events(surface, created_at DESC);
CREATE INDEX IF NOT EXISTS query_events_status_created_idx ON query_events(status, created_at DESC);
CREATE INDEX IF NOT EXISTS agent_conversations_owner_updated_idx ON agent_conversations(principal_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS agent_messages_conversation_seq_idx ON agent_messages(conversation_id, seq);
CREATE INDEX IF NOT EXISTS agent_runs_conversation_started_idx ON agent_runs(conversation_id, started_at DESC);
CREATE INDEX IF NOT EXISTS agent_events_run_seq_idx ON agent_events(run_id, seq);
