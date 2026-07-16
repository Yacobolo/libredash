-- name: GetPlatformSetting :one
SELECT value FROM platform_settings WHERE key = sqlc.arg(key);

-- name: UpsertPlatformSetting :exec
INSERT INTO platform_settings (key, value)
VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP;

-- name: InsertPlatformSettingIfMissing :exec
INSERT INTO platform_settings (key, value)
VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT(key) DO NOTHING;
