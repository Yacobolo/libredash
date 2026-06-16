package platform

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Yacobolo/libredash/internal/platform/db"
)

func TestStoreMigratesSeedsRolesAndChecksRBAC(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "libredash.db")
	store, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	store, err = Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	}
	defer store.Close()

	if err := store.EnsureWorkspace(ctx, WorkspaceInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	principal, err := store.UpsertPrincipal(ctx, PrincipalInput{Email: "owner@example.com", DisplayName: "Owner"})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	if err := store.BindRole(ctx, "test", principal.ID, "owner"); err != nil {
		t.Fatalf("bind role: %v", err)
	}
	allowed, err := store.HasPermission(ctx, "test", principal.ID, PermissionDeploymentActivate)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if !allowed {
		t.Fatal("owner missing deployment activation permission")
	}
}

func TestStoreDeploymentActivation(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.EnsureWorkspace(ctx, WorkspaceInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	deployment, err := store.CreateDeployment(ctx, "test", "tester")
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if err := store.ValidateDeployment(ctx, deployment.ID, "digest", "{}", zeroArtifact(deployment.ID, "test"), nil, nil); err != nil {
		t.Fatalf("validate deployment: %v", err)
	}
	if err := store.ActivateDeployment(ctx, "test", deployment.ID); err != nil {
		t.Fatalf("activate deployment: %v", err)
	}
	active, _, err := store.ActiveArtifact(ctx, "test")
	if err != nil {
		t.Fatalf("active artifact: %v", err)
	}
	if active.ID != deployment.ID {
		t.Fatalf("active deployment = %q, want %q", active.ID, deployment.ID)
	}
}

func TestResolveExternalPrincipalAttachesBootstrappedEmail(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.EnsureWorkspace(ctx, WorkspaceInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := store.BootstrapAdmin(ctx, "test", "owner@example.com"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	principal, err := store.ResolveExternalPrincipal(ctx, ExternalIdentityInput{
		Provider:    "azureadv2",
		TenantID:    "tenant",
		Subject:     "object-id",
		Email:       "OWNER@example.com",
		DisplayName: "Owner",
	})
	if err != nil {
		t.Fatalf("resolve external principal: %v", err)
	}
	if principal.ID != PrincipalIDForEmail("owner@example.com") {
		t.Fatalf("principal id = %q, want bootstrapped email principal", principal.ID)
	}
	allowed, err := store.HasPermission(ctx, "test", principal.ID, PermissionDeploymentActivate)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if !allowed {
		t.Fatal("attached Azure identity did not inherit owner permissions")
	}

	again, err := store.ResolveExternalPrincipal(ctx, ExternalIdentityInput{
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

func TestBootstrapAdminIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.EnsureWorkspace(ctx, WorkspaceInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := store.BootstrapAdmin(ctx, "test", "owner@example.com"); err != nil {
			t.Fatalf("bootstrap admin %d: %v", i, err)
		}
	}
	var count int
	err = store.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM role_bindings rb
		JOIN roles r ON r.id = rb.role_id
		WHERE rb.workspace_id = ? AND rb.principal_id = ? AND r.name = ?
	`, "test", PrincipalIDForEmail("owner@example.com"), "owner").Scan(&count)
	if err != nil {
		t.Fatalf("count role bindings: %v", err)
	}
	if count != 1 {
		t.Fatalf("owner role bindings = %d, want 1", count)
	}
}

func TestResolveExternalPrincipalWithoutEmailCreatesUnprivilegedPrincipal(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "libredash.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.EnsureWorkspace(ctx, WorkspaceInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	principal, err := store.ResolveExternalPrincipal(ctx, ExternalIdentityInput{
		Provider:    "azureadv2",
		TenantID:    "tenant",
		Subject:     "new-object-id",
		DisplayName: "New User",
	})
	if err != nil {
		t.Fatalf("resolve external principal: %v", err)
	}
	allowed, err := store.HasPermission(ctx, "test", principal.ID, PermissionDeploymentActivate)
	if err != nil {
		t.Fatalf("check permission: %v", err)
	}
	if allowed {
		t.Fatal("new external principal unexpectedly has deployment activation permission")
	}
}

func zeroArtifact(deploymentID, workspaceID string) db.InsertDeploymentArtifactParams {
	return db.InsertDeploymentArtifactParams{
		ID:           "artifact_" + deploymentID,
		DeploymentID: deploymentID,
		WorkspaceID:  workspaceID,
		Digest:       "digest",
		Format:       "tar.gz",
		Path:         "artifact.tar.gz",
		ManifestJson: "{}",
	}
}
