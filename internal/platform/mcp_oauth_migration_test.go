package platform

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMCPOAuthMigrationCreatesPersistentProtocolState(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "leapview.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, `INSERT INTO oauth_clients
        (id, name, redirect_uris_json, grant_types_json, response_types_json, scopes_json, audience_json, public, token_endpoint_auth_method)
        VALUES ('client-1', 'Claude', '[]', '["authorization_code"]', '["code"]', '["mcp:use"]', '["https://dash.example/mcp"]', 1, 'none')`); err != nil {
		t.Fatalf("insert OAuth client: %v", err)
	}
	if _, err := store.SQLDB().ExecContext(ctx, `INSERT INTO oauth_sessions
        (kind, signature, request_id, request_json) VALUES ('access_token', 'signature-1', 'request-1', '{}')`); err != nil {
		t.Fatalf("insert OAuth session: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var clientCount, sessionCount int
	if err := reopened.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM oauth_clients WHERE id = 'client-1'`).Scan(&clientCount); err != nil {
		t.Fatalf("count OAuth clients: %v", err)
	}
	if err := reopened.SQLDB().QueryRowContext(ctx, `SELECT count(*) FROM oauth_sessions WHERE signature = 'signature-1'`).Scan(&sessionCount); err != nil {
		t.Fatalf("count OAuth sessions: %v", err)
	}
	if clientCount != 1 || sessionCount != 1 {
		t.Fatalf("persisted OAuth state = clients:%d sessions:%d", clientCount, sessionCount)
	}
}
