package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
)

func BenchmarkAuthorizeDirectGrant(b *testing.B) {
	ctx := context.Background()
	_, repo := openAccessRepo(b, ctx)
	principal := benchmarkPrincipal(b, ctx, repo, "bench_direct")
	object := access.ItemObject(access.SecurableDashboard, "test", "exec")
	benchmarkGrant(b, ctx, repo, object, principal.ID, access.PrivilegeViewItem)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decision, err := repo.Authorize(ctx, principal.ID, access.PrivilegeViewItem, object)
		if err != nil || !decision.Allowed {
			b.Fatalf("decision = %#v err=%v, want allowed", decision, err)
		}
	}
}

func BenchmarkAuthorizeGroupGrantFanout(b *testing.B) {
	ctx := context.Background()
	_, repo := openAccessRepo(b, ctx)
	principal := benchmarkPrincipal(b, ctx, repo, "bench_group_member")
	for i := 0; i < 250; i++ {
		group, err := repo.UpsertGroup(ctx, access.GroupInput{WorkspaceID: "test", Name: fmt.Sprintf("Group %03d", i)})
		if err != nil {
			b.Fatalf("upsert group: %v", err)
		}
		if err := repo.AddGroupMember(ctx, "test", group.ID, principal.ID); err != nil {
			b.Fatalf("add group member: %v", err)
		}
	}
	targetGroup, err := repo.UpsertGroup(ctx, access.GroupInput{WorkspaceID: "test", Name: "Target"})
	if err != nil {
		b.Fatalf("upsert target group: %v", err)
	}
	if err := repo.AddGroupMember(ctx, "test", targetGroup.ID, principal.ID); err != nil {
		b.Fatalf("add target group member: %v", err)
	}
	object := access.ItemObject(access.SecurableDashboard, "test", "grouped")
	if _, err := repo.CreateGrant(ctx, access.GrantInput{Object: object, SubjectType: access.SubjectGroup, SubjectID: targetGroup.ID, Privilege: access.PrivilegeViewItem}); err != nil {
		b.Fatalf("create grant: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decision, err := repo.Authorize(ctx, principal.ID, access.PrivilegeViewItem, object)
		if err != nil || !decision.Allowed {
			b.Fatalf("decision = %#v err=%v, want allowed", decision, err)
		}
	}
}

func BenchmarkAuthorizeInheritedGrantDepth(b *testing.B) {
	ctx := context.Background()
	_, repo := openAccessRepo(b, ctx)
	principal := benchmarkPrincipal(b, ctx, repo, "bench_inherited")
	workspaceObject := access.WorkspaceObject("test")
	parent := workspaceObject
	for i := 0; i < 20; i++ {
		parent = access.ItemObjectWithParent(access.SecurableDataset, "test", fmt.Sprintf("model/table_%02d", i), parent)
		if _, err := repo.UpsertSecurableObject(ctx, parent, ""); err != nil {
			b.Fatalf("upsert securable: %v", err)
		}
	}
	benchmarkGrant(b, ctx, repo, workspaceObject, principal.ID, access.PrivilegeQueryData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decision, err := repo.Authorize(ctx, principal.ID, access.PrivilegeQueryData, parent)
		if err != nil || !decision.Allowed || !decision.Inherited {
			b.Fatalf("decision = %#v err=%v, want inherited allowed", decision, err)
		}
	}
}

func BenchmarkAuthorizeAny(b *testing.B) {
	ctx := context.Background()
	_, repo := openAccessRepo(b, ctx)
	principal := benchmarkPrincipal(b, ctx, repo, "bench_any")
	objects := make([]access.ObjectRef, 0, 25)
	for i := 0; i < 25; i++ {
		object := access.ItemObject(access.SecurableDashboard, "test", fmt.Sprintf("dash_%02d", i))
		if _, err := repo.UpsertSecurableObject(ctx, object, ""); err != nil {
			b.Fatalf("upsert object: %v", err)
		}
		objects = append(objects, object)
	}
	benchmarkGrant(b, ctx, repo, objects[len(objects)-1], principal.ID, access.PrivilegeViewItem)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decision, err := repo.AuthorizeAny(ctx, principal.ID, access.PrivilegeViewItem, objects)
		if err != nil || !decision.Allowed {
			b.Fatalf("decision = %#v err=%v, want allowed", decision, err)
		}
	}
}

func BenchmarkEffectiveAccess(b *testing.B) {
	ctx := context.Background()
	_, repo := openAccessRepo(b, ctx)
	principal := benchmarkPrincipal(b, ctx, repo, "bench_effective")
	object := access.ItemObject(access.SecurableDashboard, "test", "effective")
	for _, privilege := range []access.Privilege{access.PrivilegeViewItem, access.PrivilegeQueryData, access.PrivilegeUseAgent, access.PrivilegeViewAgent} {
		benchmarkGrant(b, ctx, repo, object, principal.ID, privilege)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decisions, err := repo.EffectiveAccess(ctx, principal.ID, object)
		if err != nil || len(decisions) != 4 {
			b.Fatalf("decisions = %#v err=%v, want 4 decisions", decisions, err)
		}
	}
}

func benchmarkPrincipal(tb testing.TB, ctx context.Context, repo *Repository, id string) access.Principal {
	tb.Helper()
	principal, err := repo.UpsertPrincipal(ctx, access.PrincipalInput{ID: id, Email: id + "@example.com", DisplayName: id})
	if err != nil {
		tb.Fatalf("upsert principal: %v", err)
	}
	return principal
}

func benchmarkGrant(tb testing.TB, ctx context.Context, repo *Repository, object access.ObjectRef, principalID string, privilege access.Privilege) {
	tb.Helper()
	if _, err := repo.CreateGrant(ctx, access.GrantInput{Object: object, SubjectType: access.SubjectPrincipal, SubjectID: principalID, Privilege: privilege}); err != nil {
		tb.Fatalf("create grant: %v", err)
	}
}
