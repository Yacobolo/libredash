-- +goose Up
-- Fingerprint/verifier columns are created directly by baseline development migrations.
CREATE UNIQUE INDEX IF NOT EXISTS sessions_token_fingerprint_unique_idx
  ON sessions(token_fingerprint)
  WHERE token_fingerprint IS NOT NULL AND token_fingerprint <> '';
CREATE UNIQUE INDEX IF NOT EXISTS api_tokens_token_fingerprint_unique_idx
  ON api_tokens(token_fingerprint)
  WHERE token_fingerprint IS NOT NULL AND token_fingerprint <> '';
CREATE UNIQUE INDEX IF NOT EXISTS service_principal_secrets_fingerprint_unique_idx
  ON service_principal_secrets(service_principal_id, secret_fingerprint)
  WHERE secret_fingerprint IS NOT NULL AND secret_fingerprint <> '';

-- +goose Down
DROP INDEX IF EXISTS service_principal_secrets_fingerprint_unique_idx;
DROP INDEX IF EXISTS api_tokens_token_fingerprint_unique_idx;
DROP INDEX IF EXISTS sessions_token_fingerprint_unique_idx;
