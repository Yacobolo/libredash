-- Public API idempotency and cursor signing.

-- name: DeleteExpiredAPIIdempotencyRecord :exec
DELETE FROM api_idempotency_records WHERE scope = ? AND expires_at <= ?;

-- name: CreateAPIIdempotencyRecord :execrows
INSERT INTO api_idempotency_records
  (scope, request_digest, state, owner_id, lease_expires_at, created_at, updated_at, expires_at)
VALUES (?, ?, 'pending', ?, ?, ?, ?, ?) ON CONFLICT(scope) DO NOTHING;

-- name: ReclaimAPIIdempotencyRecord :execrows
UPDATE api_idempotency_records SET owner_id = sqlc.arg(owner_id),
  lease_expires_at = sqlc.arg(new_lease_expires_at), updated_at = sqlc.arg(updated_at)
WHERE scope = sqlc.arg(scope) AND request_digest = sqlc.arg(request_digest)
  AND state = 'pending' AND lease_expires_at <= sqlc.arg(now);

-- name: GetAPIIdempotencyRecord :one
SELECT request_digest, state, owner_id, lease_expires_at, response_status,
  response_headers_json, response_body FROM api_idempotency_records WHERE scope = ?;

-- name: RenewAPIIdempotencyRecord :execrows
UPDATE api_idempotency_records SET lease_expires_at = ?, updated_at = ?
WHERE scope = ? AND request_digest = ? AND owner_id = ? AND state = 'pending';

-- name: CompleteAPIIdempotencyRecord :execrows
UPDATE api_idempotency_records SET state = 'completed', response_status = ?, response_headers_json = ?,
  response_body = ?, updated_at = ?
WHERE scope = ? AND request_digest = ? AND owner_id = ? AND state = 'pending';

-- name: AbandonAPIIdempotencyRecord :execrows
DELETE FROM api_idempotency_records
WHERE scope = ? AND request_digest = ? AND owner_id = ? AND state = 'pending';

-- name: CreateInitialAPICursorSigningKey :exec
INSERT OR IGNORE INTO api_cursor_signing_keys(key_id, secret, active, created_at)
SELECT 'v1', ?, 1, ? WHERE NOT EXISTS (SELECT 1 FROM api_cursor_signing_keys WHERE active = 1);

-- name: ListAPICursorSigningKeys :many
SELECT key_id, secret, active FROM api_cursor_signing_keys
WHERE retired_at IS NULL ORDER BY created_at, key_id;
