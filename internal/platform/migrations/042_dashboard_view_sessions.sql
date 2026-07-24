-- +goose Up

CREATE TABLE dashboard_view_sessions (
  id TEXT PRIMARY KEY,
  key_json TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  state_json TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX dashboard_view_sessions_expiry_idx
  ON dashboard_view_sessions(expires_at);

-- +goose Down

DROP INDEX dashboard_view_sessions_expiry_idx;
DROP TABLE dashboard_view_sessions;
