package platform

import (
	"context"
	"path/filepath"
	"testing"
)

func TestS3MultipartMigrationAddsConstrainedDurableState(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer store.Close()

	for _, table := range []string{"managed_data_s3_multipart_uploads", "managed_data_s3_multipart_parts"} {
		var count int
		if err := store.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatalf("inspect table %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("table %s count = %d, want 1", table, count)
		}
	}

	_, err = store.SQLDB().ExecContext(ctx, `
		INSERT INTO managed_data_s3_multipart_uploads
			(id, upload_session_id, logical_path, sha256, size_bytes, status, idempotency_identity)
		VALUES ('bad', 'missing-session', 'data.csv', ?, 1, 'unknown', ?)
	`, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err == nil {
		t.Fatal("invalid multipart state unexpectedly satisfied migration constraints")
	}
}
