-- +goose Up
-- Make chat conversations principal-scoped. Keep workspace_id as legacy metadata,
-- but remove workspace ownership/cascade semantics.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS agent_conversations_global (
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

INSERT INTO agent_conversations_global (
  id,
  workspace_id,
  principal_id,
  title,
  status,
  metadata_json,
  transcript_json,
  created_at,
  updated_at,
  archived_at
)
SELECT
  id,
  workspace_id,
  principal_id,
  title,
  status,
  metadata_json,
  transcript_json,
  created_at,
  updated_at,
  archived_at
FROM agent_conversations;

DROP TABLE agent_conversations;
ALTER TABLE agent_conversations_global RENAME TO agent_conversations;

PRAGMA foreign_keys = ON;

CREATE INDEX IF NOT EXISTS agent_conversations_owner_updated_idx ON agent_conversations(principal_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS agent_messages_conversation_seq_idx ON agent_messages(conversation_id, seq);
CREATE INDEX IF NOT EXISTS agent_runs_conversation_started_idx ON agent_runs(conversation_id, started_at DESC);
CREATE INDEX IF NOT EXISTS agent_events_run_seq_idx ON agent_events(run_id, seq);
