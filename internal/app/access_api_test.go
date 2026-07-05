package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/platform"
)

func TestAPITokenWorkspaceAndPermissionAllowlistAreEnforced(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPrincipal(t, ctx, store, "token-owner@example.com", "Token Owner", access.RoleOwner)
	token, _ := testScopedAPIToken(t, ctx, store, access.APITokenInput{
		PrincipalID: owner.ID,
		WorkspaceID: "test",
		Name:        "workspace-read-only",
		Permissions: []string{access.PermissionWorkspaceRead},
	})
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, ArtifactDir: t.TempDir(), DefaultWorkspaceID: "test"})

	deploymentsReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/publishes", nil)
	deploymentsReq.Header.Set("Authorization", "Bearer "+token)
	deploymentsReq.Header.Set("Accept", "application/json")
	deploymentsRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(deploymentsRec, deploymentsReq)
	if deploymentsRec.Code != http.StatusForbidden {
		t.Fatalf("deployment list status = %d, want %d body=%s", deploymentsRec.Code, http.StatusForbidden, deploymentsRec.Body.String())
	}

	foreignWorkspaceReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/other/assets", nil)
	foreignWorkspaceReq.Header.Set("Authorization", "Bearer "+token)
	foreignWorkspaceReq.Header.Set("Accept", "application/json")
	foreignWorkspaceRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(foreignWorkspaceRec, foreignWorkspaceReq)
	if foreignWorkspaceRec.Code != http.StatusForbidden {
		t.Fatalf("foreign workspace status = %d, want %d body=%s", foreignWorkspaceRec.Code, http.StatusForbidden, foreignWorkspaceRec.Body.String())
	}

	permissionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/permissions?workspace=test", nil)
	permissionsReq.Header.Set("Authorization", "Bearer "+token)
	permissionsReq.Header.Set("Accept", "application/json")
	permissionsRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(permissionsRec, permissionsReq)
	if permissionsRec.Code != http.StatusOK {
		t.Fatalf("permissions status = %d, want %d body=%s", permissionsRec.Code, http.StatusOK, permissionsRec.Body.String())
	}
	var permissionsBody struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(permissionsRec.Body.Bytes(), &permissionsBody); err != nil {
		t.Fatalf("decode permissions: %v", err)
	}
	if !hasString(permissionsBody.Permissions, access.PermissionWorkspaceRead) {
		t.Fatalf("permissions = %#v, want workspace read", permissionsBody.Permissions)
	}
	if hasString(permissionsBody.Permissions, access.PermissionPublishRead) {
		t.Fatalf("permissions = %#v, token allowlist leaked deployment read", permissionsBody.Permissions)
	}

	emptyAllowlistToken, _ := testScopedAPIToken(t, ctx, store, access.APITokenInput{
		PrincipalID: owner.ID,
		WorkspaceID: "test",
		Name:        "empty-allowlist",
		Permissions: []string{},
	})
	emptyAllowlistReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/permissions?workspace=test", nil)
	emptyAllowlistReq.Header.Set("Authorization", "Bearer "+emptyAllowlistToken)
	emptyAllowlistReq.Header.Set("Accept", "application/json")
	emptyAllowlistRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(emptyAllowlistRec, emptyAllowlistReq)
	if emptyAllowlistRec.Code != http.StatusForbidden {
		t.Fatalf("empty allowlist status = %d, want %d body=%s", emptyAllowlistRec.Code, http.StatusForbidden, emptyAllowlistRec.Body.String())
	}
}

func TestCurrentAPITokenRevocationIsScopedToAuthenticatedPrincipal(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	owner := testPlatformPrincipal(t, ctx, store, "token-revoke-owner@example.com", "Token Owner", access.RoleAdmin)
	foreign := testPlatformPrincipal(t, ctx, store, "token-revoke-foreign@example.com", "Token Foreign", access.RoleAdmin)
	authSecret, _ := testScopedAPIToken(t, ctx, store, access.APITokenInput{
		PrincipalID: owner.ID,
		WorkspaceID: "test",
		Name:        "auth",
		Permissions: []string{access.PermissionTokenManage},
	})
	_, ownerToken := testScopedAPIToken(t, ctx, store, access.APITokenInput{PrincipalID: owner.ID, Name: "owned"})
	foreignSecret, foreignToken := testScopedAPIToken(t, ctx, store, access.APITokenInput{PrincipalID: foreign.ID, Name: "foreign"})
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, ArtifactDir: t.TempDir(), DefaultWorkspaceID: "test"})

	for _, id := range []string{foreignToken.ID, "token_missing"} {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/api-tokens/"+id, nil)
		req.Header.Set("Authorization", "Bearer "+authSecret)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("revoke api token %q status = %d, want %d body=%s", id, rec.Code, http.StatusNotFound, rec.Body.String())
		}
	}
	if _, err := testAccessRepository(store).PrincipalForAPIToken(ctx, foreignSecret); err != nil {
		t.Fatalf("foreign token was revoked by owner: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/api-tokens/"+ownerToken.ID, nil)
	req.Header.Set("Authorization", "Bearer "+authSecret)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke owned api token status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestCurrentSessionRevocationIsScopedToAuthenticatedPrincipal(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	repo := testAccessRepository(store)
	owner := testPlatformPrincipal(t, ctx, store, "session-revoke-owner@example.com", "Session Owner", access.RoleAdmin)
	foreign := testPlatformPrincipal(t, ctx, store, "session-revoke-foreign@example.com", "Session Foreign", access.RoleAdmin)
	authSecret, _ := testScopedAPIToken(t, ctx, store, access.APITokenInput{
		PrincipalID: owner.ID,
		WorkspaceID: "test",
		Name:        "auth",
		Permissions: []string{access.PermissionWorkspaceRead},
	})
	ownerSessionSecret, err := repo.CreateSession(ctx, owner.ID, time.Hour)
	if err != nil {
		t.Fatalf("create owner session: %v", err)
	}
	foreignSessionSecret, err := repo.CreateSession(ctx, foreign.ID, time.Hour)
	if err != nil {
		t.Fatalf("create foreign session: %v", err)
	}
	ownerSessions, err := repo.ListSessions(ctx, owner.ID)
	if err != nil {
		t.Fatalf("list owner sessions: %v", err)
	}
	foreignSessions, err := repo.ListSessions(ctx, foreign.ID)
	if err != nil {
		t.Fatalf("list foreign sessions: %v", err)
	}
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, ArtifactDir: t.TempDir(), DefaultWorkspaceID: "test"})

	for _, id := range []string{foreignSessions[0].ID, "session_missing"} {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/sessions/"+id, nil)
		req.Header.Set("Authorization", "Bearer "+authSecret)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("revoke session %q status = %d, want %d body=%s", id, rec.Code, http.StatusNotFound, rec.Body.String())
		}
	}
	if _, err := repo.PrincipalForToken(ctx, foreignSessionSecret); err != nil {
		t.Fatalf("foreign session was revoked by owner: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/sessions/"+ownerSessions[0].ID, nil)
	req.Header.Set("Authorization", "Bearer "+authSecret)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke owned session status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if _, err := repo.PrincipalForToken(ctx, ownerSessionSecret); err == nil {
		t.Fatal("owner session still resolves after revocation")
	}
}

func testScopedAPIToken(t *testing.T, ctx context.Context, store *platform.Store, input access.APITokenInput) (string, access.APIToken) {
	t.Helper()
	secret, token, err := testAccessRepository(store).CreateAPITokenWithMetadata(ctx, input)
	if err != nil {
		t.Fatalf("create scoped api token: %v", err)
	}
	return secret, token
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
