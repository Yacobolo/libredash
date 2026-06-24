package sqlite

import (
	"context"
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
