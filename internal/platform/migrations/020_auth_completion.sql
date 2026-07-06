-- +goose Up
-- Follow-up auth-spec storage for service-principal credentials and complete
-- audit provenance fields.

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

CREATE INDEX IF NOT EXISTS service_principal_secrets_principal_idx
  ON service_principal_secrets(service_principal_id, created_at DESC);

ALTER TABLE audit_events ADD COLUMN privilege TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN status TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN request_id TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN correlation_id TEXT NOT NULL DEFAULT '';
