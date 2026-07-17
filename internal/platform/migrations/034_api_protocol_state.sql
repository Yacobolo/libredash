-- +goose Up
CREATE TABLE api_idempotency_records (
  scope TEXT PRIMARY KEY,
  request_digest TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('pending', 'completed')),
  response_status INTEGER,
  response_headers_json TEXT,
  response_body BLOB,
  owner_id TEXT NOT NULL,
  lease_expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE INDEX api_idempotency_records_expiry_idx
  ON api_idempotency_records(expires_at);

CREATE TABLE api_cursor_signing_keys (
  key_id TEXT PRIMARY KEY,
  secret BLOB NOT NULL CHECK(length(secret) >= 32),
  active INTEGER NOT NULL DEFAULT 0 CHECK(active IN (0, 1)),
  created_at TEXT NOT NULL,
  retired_at TEXT
);

CREATE UNIQUE INDEX api_cursor_signing_keys_active_idx
  ON api_cursor_signing_keys(active) WHERE active = 1;

DROP INDEX IF EXISTS agent_events_run_seq_idx;
DROP TABLE IF EXISTS agent_events;

-- +goose Down
CREATE TABLE agent_events (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL,
  event_type TEXT NOT NULL,
  severity TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(run_id, seq)
);
CREATE INDEX agent_events_run_seq_idx ON agent_events(run_id, seq);
DROP INDEX IF EXISTS api_cursor_signing_keys_active_idx;
DROP TABLE IF EXISTS api_cursor_signing_keys;
DROP INDEX IF EXISTS api_idempotency_records_expiry_idx;
DROP TABLE IF EXISTS api_idempotency_records;
