package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func TestRepositoryChecksRBAC(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)

	principal, err := repo.SetPrincipalRole(ctx, access.PrincipalRoleInput{
		WorkspaceID: "test",
		Email:       "owner@example.com",
		DisplayName: "Owner",
		Role:        "owner",
	})
	if err != nil {
		t.Fatalf("set principal role: %v", err)
	}
	allowed, err := repo.HasPermission(ctx, "test", principal.ID, access.PermissionDeploymentActivate)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if !allowed {
		t.Fatal("owner missing deployment activation permission")
	}
}

func TestRepositoryChecksPlatformRolePermissions(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)

	principal, err := repo.SetPlatformRole(ctx, access.PlatformRoleInput{
		PrincipalID: "dev",
		Email:       "dev@localhost",
		DisplayName: "Local Developer",
		Role:        access.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("set platform role: %v", err)
	}
	for _, workspaceID := range []string{"test", "other"} {
		allowed, err := repo.HasPermission(ctx, workspaceID, principal.ID, access.PermissionTokenManage)
		if err != nil {
			t.Fatalf("check permission for %s: %v", workspaceID, err)
		}
		if !allowed {
			t.Fatalf("platform admin missing token manage permission for workspace %s", workspaceID)
		}
	}

	limited, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{ID: "limited", Email: "limited@example.com", DisplayName: "Limited"})
	if err != nil {
		t.Fatalf("upsert limited principal: %v", err)
	}
	allowed, err := repo.HasPermission(ctx, "test", limited.ID, access.PermissionTokenManage)
	if err != nil {
		t.Fatalf("check limited permission: %v", err)
	}
	if allowed {
		t.Fatal("principal without platform or workspace role unexpectedly has permission")
	}
}

func TestRepositoryChecksGroupRolePermissions(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)

	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          "principal_group_member",
		Email:       "member@example.com",
		DisplayName: "Group Member",
	})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	group, err := repo.UpsertGroup(ctx, access.GroupInput{
		WorkspaceID: "test",
		Provider:    "azureadv2",
		ExternalID:  "group-object-id",
		Name:        "Deployers",
	})
	if err != nil {
		t.Fatalf("upsert group: %v", err)
	}
	if err := repo.AddGroupMember(ctx, "test", group.ID, principal.ID); err != nil {
		t.Fatalf("add group member: %v", err)
	}
	binding, err := repo.CreateRoleBinding(ctx, access.RoleBindingInput{
		WorkspaceID: "test",
		SubjectType: access.SubjectGroup,
		SubjectID:   group.ID,
		Role:        access.RoleDeployer,
	})
	if err != nil {
		t.Fatalf("create group role binding: %v", err)
	}
	if binding.SubjectType != access.SubjectGroup || binding.SubjectID != group.ID {
		t.Fatalf("binding subject = %q/%q, want group/%q", binding.SubjectType, binding.SubjectID, group.ID)
	}

	allowed, err := repo.HasPermission(ctx, "test", principal.ID, access.PermissionDeploymentActivate)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if !allowed {
		t.Fatal("group member missing deployment activation permission")
	}
	if err := repo.RemoveGroupMember(ctx, "test", group.ID, principal.ID); err != nil {
		t.Fatalf("remove group member: %v", err)
	}
	allowed, err = repo.HasPermission(ctx, "test", principal.ID, access.PermissionDeploymentActivate)
	if err != nil {
		t.Fatalf("check permission after remove: %v", err)
	}
	if allowed {
		t.Fatal("removed group member still has deployment activation permission")
	}
}

func TestRepositoryManagesRoleBindingsByID(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)

	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          "principal_binding_subject",
		Email:       "subject@example.com",
		DisplayName: "Subject",
	})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	created, err := repo.CreateRoleBinding(ctx, access.RoleBindingInput{
		WorkspaceID: "test",
		SubjectType: access.SubjectPrincipal,
		SubjectID:   principal.ID,
		Role:        access.RoleViewer,
	})
	if err != nil {
		t.Fatalf("create role binding: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created binding id is empty")
	}
	if created.Role != access.RoleViewer {
		t.Fatalf("created role = %q, want viewer", created.Role)
	}

	updated, err := repo.UpdateRoleBinding(ctx, "test", created.ID, access.RoleBindingInput{
		SubjectType: access.SubjectPrincipal,
		SubjectID:   principal.ID,
		Role:        access.RoleEditor,
	})
	if err != nil {
		t.Fatalf("update role binding: %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("updated binding id = %q, want %q", updated.ID, created.ID)
	}
	if updated.Role != access.RoleEditor {
		t.Fatalf("updated role = %q, want editor", updated.Role)
	}
	got, err := repo.GetRoleBinding(ctx, "test", created.ID)
	if err != nil {
		t.Fatalf("get role binding: %v", err)
	}
	if got.SubjectType != access.SubjectPrincipal || got.SubjectID != principal.ID {
		t.Fatalf("got subject = %q/%q, want principal/%q", got.SubjectType, got.SubjectID, principal.ID)
	}
	if err := repo.DeleteRoleBinding(ctx, "test", created.ID); err != nil {
		t.Fatalf("delete role binding: %v", err)
	}
	bindings, err := repo.ListRoleBindings(ctx, "test")
	if err != nil {
		t.Fatalf("list role bindings: %v", err)
	}
	for _, binding := range bindings {
		if binding.ID == created.ID {
			t.Fatal("deleted role binding was listed")
		}
	}
}

func TestRepositoryResolveExternalPrincipalAttachesBootstrappedEmail(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)

	if err := repo.BootstrapAdmin(ctx, "test", "owner@example.com"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	principal, err := repo.ResolveExternalPrincipal(ctx, access.ExternalIdentityInput{
		Provider:    "azureadv2",
		TenantID:    "tenant",
		Subject:     "object-id",
		Email:       "OWNER@example.com",
		DisplayName: "Owner",
	})
	if err != nil {
		t.Fatalf("resolve external principal: %v", err)
	}
	if principal.ID != access.PrincipalIDForEmail("owner@example.com") {
		t.Fatalf("principal id = %q, want bootstrapped email principal", principal.ID)
	}
	allowed, err := repo.HasPermission(ctx, "test", principal.ID, access.PermissionDeploymentActivate)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if !allowed {
		t.Fatal("attached Azure identity did not inherit owner permissions")
	}

	again, err := repo.ResolveExternalPrincipal(ctx, access.ExternalIdentityInput{
		Provider:    "azureadv2",
		TenantID:    "tenant",
		Subject:     "object-id",
		Email:       "owner@example.com",
		DisplayName: "Owner Updated",
	})
	if err != nil {
		t.Fatalf("resolve existing identity: %v", err)
	}
	if again.ID != principal.ID {
		t.Fatalf("existing identity principal = %q, want %q", again.ID, principal.ID)
	}
}

func TestRepositoryBootstrapAdminIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, repo := openAccessRepo(t, ctx)

	for i := 0; i < 2; i++ {
		if err := repo.BootstrapAdmin(ctx, "test", "owner@example.com"); err != nil {
			t.Fatalf("bootstrap admin %d: %v", i, err)
		}
	}
	var count int
	err := store.SQLDB().QueryRowContext(ctx, `
		SELECT count(*)
		FROM role_bindings rb
		JOIN roles r ON r.id = rb.role_id
		WHERE rb.workspace_id = ? AND rb.principal_id = ? AND r.name = ?
	`, "test", access.PrincipalIDForEmail("owner@example.com"), "owner").Scan(&count)
	if err != nil {
		t.Fatalf("count role bindings: %v", err)
	}
	if count != 1 {
		t.Fatalf("owner role bindings = %d, want 1", count)
	}
}

func TestRepositoryResolveExternalPrincipalWithoutEmailCreatesUnprivilegedPrincipal(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)

	principal, err := repo.ResolveExternalPrincipal(ctx, access.ExternalIdentityInput{
		Provider:    "azureadv2",
		TenantID:    "tenant",
		Subject:     "new-object-id",
		DisplayName: "New User",
	})
	if err != nil {
		t.Fatalf("resolve external principal: %v", err)
	}
	allowed, err := repo.HasPermission(ctx, "test", principal.ID, access.PermissionDeploymentActivate)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if allowed {
		t.Fatal("new external principal unexpectedly has deployment activation permission")
	}
}

func TestRepositorySessionsAndAPITokensResolvePrincipals(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)
	principal, err := repo.SetPrincipalRole(ctx, access.PrincipalRoleInput{
		WorkspaceID: "test",
		Email:       "viewer@example.com",
		DisplayName: "Viewer",
		Role:        "viewer",
	})
	if err != nil {
		t.Fatalf("set principal role: %v", err)
	}

	sessionToken, err := repo.CreateSession(ctx, principal.ID, time.Hour)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionPrincipal, err := repo.PrincipalForToken(ctx, sessionToken)
	if err != nil {
		t.Fatalf("principal for session: %v", err)
	}
	if sessionPrincipal.ID != principal.ID {
		t.Fatalf("session principal = %q, want %q", sessionPrincipal.ID, principal.ID)
	}

	apiToken, err := repo.CreateAPIToken(ctx, principal.ID, "test")
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	apiPrincipal, err := repo.PrincipalForAPIToken(ctx, apiToken)
	if err != nil {
		t.Fatalf("principal for api token: %v", err)
	}
	if apiPrincipal.ID != principal.ID {
		t.Fatalf("api token principal = %q, want %q", apiPrincipal.ID, principal.ID)
	}
}

func TestRepositoryListsAndRevokesSessionsByID(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)
	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          "principal_session_owner",
		Email:       "sessions@example.com",
		DisplayName: "Sessions",
	})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	token, err := repo.CreateSession(ctx, principal.ID, time.Hour)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessions, err := repo.ListSessions(ctx, principal.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	if sessions[0].ID == "" || sessions[0].RevokedAt != "" {
		t.Fatalf("session metadata = id %q revoked %q, want id and no revocation", sessions[0].ID, sessions[0].RevokedAt)
	}
	if err := repo.RevokeSession(ctx, sessions[0].ID); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	sessions, err = repo.ListSessions(ctx, principal.ID)
	if err != nil {
		t.Fatalf("list sessions after revoke: %v", err)
	}
	if sessions[0].RevokedAt == "" {
		t.Fatal("revoked session missing revoked_at")
	}
	if _, err := repo.PrincipalForToken(ctx, token); err == nil {
		t.Fatal("revoked session token still resolves")
	}
}

func TestRepositoryListsAndRevokesAPITokens(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)
	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          "principal_api_token_owner",
		Email:       "tokens@example.com",
		DisplayName: "Tokens",
	})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	expiresAt := time.Now().Add(time.Hour).UTC()
	secret, created, err := repo.CreateAPITokenWithMetadata(ctx, access.APITokenInput{
		PrincipalID: principal.ID,
		WorkspaceID: "test",
		Name:        "production",
		Permissions: []string{access.PermissionDashboardView},
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	if secret == "" || created.ID == "" {
		t.Fatal("api token secret or id is empty")
	}
	tokens, err := repo.ListAPITokens(ctx, principal.ID)
	if err != nil {
		t.Fatalf("list api tokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("tokens len = %d, want 1", len(tokens))
	}
	token := tokens[0]
	if token.WorkspaceID != "test" || token.ExpiresAt == "" || token.RevokedAt != "" {
		t.Fatalf("token metadata = workspace %q expires %q revoked %q", token.WorkspaceID, token.ExpiresAt, token.RevokedAt)
	}
	if len(token.Permissions) != 1 || token.Permissions[0] != access.PermissionDashboardView {
		t.Fatalf("token permissions = %#v, want dashboard view", token.Permissions)
	}
	if _, err := repo.PrincipalForAPIToken(ctx, secret); err != nil {
		t.Fatalf("principal for api token: %v", err)
	}
	if err := repo.RevokeAPIToken(ctx, token.ID); err != nil {
		t.Fatalf("revoke api token: %v", err)
	}
	tokens, err = repo.ListAPITokens(ctx, principal.ID)
	if err != nil {
		t.Fatalf("list api tokens after revoke: %v", err)
	}
	if tokens[0].RevokedAt == "" {
		t.Fatal("revoked api token missing revoked_at")
	}
	if _, err := repo.PrincipalForAPIToken(ctx, secret); err == nil {
		t.Fatal("revoked api token still resolves")
	}
}

func TestRepositoryAPITokenCredentialIncludesTokenMetadata(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)
	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          "principal_api_credential",
		Email:       "credential@example.com",
		DisplayName: "Credential",
	})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	secret, created, err := repo.CreateAPITokenWithMetadata(ctx, access.APITokenInput{
		PrincipalID: principal.ID,
		WorkspaceID: "test",
		Name:        "scoped",
		Permissions: []string{access.PermissionWorkspaceRead, access.PermissionTokenManage},
	})
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}

	credential, err := repo.CredentialForAPIToken(ctx, secret)
	if err != nil {
		t.Fatalf("credential for api token: %v", err)
	}
	if credential.Principal.ID != principal.ID {
		t.Fatalf("credential principal = %q, want %q", credential.Principal.ID, principal.ID)
	}
	if credential.Token.ID != created.ID || credential.Token.WorkspaceID != "test" {
		t.Fatalf("credential token metadata = id %q workspace %q, want %q/test", credential.Token.ID, credential.Token.WorkspaceID, created.ID)
	}
	if len(credential.Token.Permissions) != 2 || credential.Token.Permissions[0] != access.PermissionWorkspaceRead || credential.Token.Permissions[1] != access.PermissionTokenManage {
		t.Fatalf("credential token permissions = %#v", credential.Token.Permissions)
	}
}

func TestRepositoryScopedRevocationRejectsForeignOrUnknownIDs(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)
	owner, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{ID: "principal_revoke_owner", Email: "owner@example.com", DisplayName: "Owner"})
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	foreign, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{ID: "principal_revoke_foreign", Email: "foreign@example.com", DisplayName: "Foreign"})
	if err != nil {
		t.Fatalf("upsert foreign: %v", err)
	}
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
	if err := repo.RevokeSessionForPrincipal(ctx, owner.ID, foreignSessions[0].ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("revoke foreign session err = %v, want sql.ErrNoRows", err)
	}
	if _, err := repo.PrincipalForToken(ctx, foreignSessionSecret); err != nil {
		t.Fatalf("foreign session was revoked by owner: %v", err)
	}
	if err := repo.RevokeSessionForPrincipal(ctx, owner.ID, ownerSessions[0].ID); err != nil {
		t.Fatalf("revoke owner session: %v", err)
	}
	if _, err := repo.PrincipalForToken(ctx, ownerSessionSecret); err == nil {
		t.Fatal("owner session still resolves after scoped revoke")
	}

	ownerSecret, ownerToken, err := repo.CreateAPITokenWithMetadata(ctx, access.APITokenInput{PrincipalID: owner.ID, Name: "owner"})
	if err != nil {
		t.Fatalf("create owner api token: %v", err)
	}
	foreignSecret, foreignToken, err := repo.CreateAPITokenWithMetadata(ctx, access.APITokenInput{PrincipalID: foreign.ID, Name: "foreign"})
	if err != nil {
		t.Fatalf("create foreign api token: %v", err)
	}
	if err := repo.RevokeAPITokenForPrincipal(ctx, owner.ID, foreignToken.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("revoke foreign api token err = %v, want sql.ErrNoRows", err)
	}
	if _, err := repo.PrincipalForAPIToken(ctx, foreignSecret); err != nil {
		t.Fatalf("foreign api token was revoked by owner: %v", err)
	}
	if err := repo.RevokeAPITokenForPrincipal(ctx, owner.ID, ownerToken.ID); err != nil {
		t.Fatalf("revoke owner api token: %v", err)
	}
	if _, err := repo.PrincipalForAPIToken(ctx, ownerSecret); err == nil {
		t.Fatal("owner api token still resolves after scoped revoke")
	}
	if err := repo.RevokeAPITokenForPrincipal(ctx, owner.ID, "token_missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("revoke unknown api token err = %v, want sql.ErrNoRows", err)
	}
}

func TestRepositoryListsAuditEvents(t *testing.T) {
	ctx := context.Background()
	_, repo := openAccessRepo(t, ctx)
	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          "principal_audit_actor",
		Email:       "audit@example.com",
		DisplayName: "Audit",
	})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	if err := repo.RecordAuditEvent(ctx, access.AuditEventInput{
		WorkspaceID:  "test",
		PrincipalID:  principal.ID,
		Action:       "role_binding.created",
		TargetType:   "role_binding",
		TargetID:     "binding_1",
		MetadataJSON: `{"role":"viewer"}`,
	}); err != nil {
		t.Fatalf("record audit event: %v", err)
	}
	if err := repo.RecordAuditEvent(ctx, access.AuditEventInput{
		WorkspaceID:  "test",
		PrincipalID:  principal.ID,
		Action:       "session.revoked",
		TargetType:   "session",
		TargetID:     "session_1",
		MetadataJSON: `{}`,
	}); err != nil {
		t.Fatalf("record audit event 2: %v", err)
	}
	events, err := repo.ListAuditEvents(ctx, access.AuditEventFilter{
		WorkspaceID: "test",
		Action:      "role_binding.created",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].PrincipalID != principal.ID || events[0].MetadataJSON != `{"role":"viewer"}` {
		t.Fatalf("event = %#v, want recorded role binding event", events[0])
	}
}

func TestRepositoryFiltersAndPaginatesAuditEvents(t *testing.T) {
	ctx := context.Background()
	store, repo := openAccessRepo(t, ctx)
	alice, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          "principal_audit_alice",
		Email:       "alice@example.com",
		DisplayName: "Alice",
	})
	if err != nil {
		t.Fatalf("upsert alice: %v", err)
	}
	bob, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          "principal_audit_bob",
		Email:       "bob@example.com",
		DisplayName: "Bob",
	})
	if err != nil {
		t.Fatalf("upsert bob: %v", err)
	}
	seed := []struct {
		actor      string
		action     string
		targetType string
		targetID   string
		createdAt  string
	}{
		{actor: alice.ID, action: "role_binding.created", targetType: "role_binding", targetID: "binding_old", createdAt: "2026-01-02 10:00:00"},
		{actor: alice.ID, action: "role_binding.created", targetType: "role_binding", targetID: "binding_mid", createdAt: "2026-01-02 11:00:00"},
		{actor: alice.ID, action: "role_binding.deleted", targetType: "role_binding", targetID: "binding_new", createdAt: "2026-01-02 12:00:00"},
		{actor: bob.ID, action: "role_binding.created", targetType: "role_binding", targetID: "binding_bob", createdAt: "2026-01-02 13:00:00"},
		{actor: alice.ID, action: "session.revoked", targetType: "session", targetID: "session_1", createdAt: "2026-01-02 14:00:00"},
	}
	for _, row := range seed {
		if err := repo.RecordAuditEvent(ctx, access.AuditEventInput{
			WorkspaceID:  "test",
			PrincipalID:  row.actor,
			Action:       row.action,
			TargetType:   row.targetType,
			TargetID:     row.targetID,
			MetadataJSON: `{}`,
		}); err != nil {
			t.Fatalf("record %s: %v", row.targetID, err)
		}
		if _, err := store.SQLDB().ExecContext(ctx, `UPDATE audit_events SET created_at = ? WHERE target_id = ?`, row.createdAt, row.targetID); err != nil {
			t.Fatalf("set created_at for %s: %v", row.targetID, err)
		}
	}

	filtered, err := repo.ListAuditEvents(ctx, access.AuditEventFilter{
		WorkspaceID: "test",
		PrincipalID: alice.ID,
		Action:      "role_binding.created",
		TargetType:  "role_binding",
		From:        "2026-01-02T10:30:00Z",
		To:          "2026-01-02T12:30:00Z",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list filtered audit events: %v", err)
	}
	if len(filtered) != 1 || filtered[0].TargetID != "binding_mid" {
		t.Fatalf("filtered events = %#v, want only binding_mid", filtered)
	}

	targeted, err := repo.ListAuditEvents(ctx, access.AuditEventFilter{
		WorkspaceID: "test",
		TargetType:  "role_binding",
		TargetID:    "binding_new",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list targeted audit events: %v", err)
	}
	if len(targeted) != 1 || targeted[0].Action != "role_binding.deleted" {
		t.Fatalf("targeted events = %#v, want binding_new deletion", targeted)
	}

	firstPage, err := repo.ListAuditEvents(ctx, access.AuditEventFilter{WorkspaceID: "test", Limit: 2})
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if len(firstPage) != 2 || firstPage[0].TargetID != "session_1" || firstPage[1].TargetID != "binding_bob" {
		t.Fatalf("first page = %#v, want session_1 then binding_bob", firstPage)
	}
	nextPage, err := repo.ListAuditEvents(ctx, access.AuditEventFilter{WorkspaceID: "test", Limit: 2, PageToken: auditPageToken(firstPage[1].CreatedAt, firstPage[1].ID)})
	if err != nil {
		t.Fatalf("list next page: %v", err)
	}
	if len(nextPage) != 2 || nextPage[0].TargetID != "binding_new" || nextPage[1].TargetID != "binding_mid" {
		t.Fatalf("next page = %#v, want binding_new then binding_mid", nextPage)
	}
}

func openAccessRepo(t *testing.T, ctx context.Context) (*platform.Store, *Repository) {
	t.Helper()
	store, err := platform.Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := workspacesqlite.NewRepository(store.SQLDB()).Ensure(ctx, workspace.EnsureInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	return store, NewRepository(store.SQLDB())
}
