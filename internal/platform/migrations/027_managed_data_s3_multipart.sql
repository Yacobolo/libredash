-- +goose Up

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

-- +goose Down
-- Forward-only: multipart state is retained to avoid orphaning provider uploads.
