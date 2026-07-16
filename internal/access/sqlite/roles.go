package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Yacobolo/libredash/internal/access"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"strings"
)

func (r *Repository) CreateRoleBinding(ctx context.Context, input access.RoleBindingInput) (access.RoleBinding, error) {
	access.ClearAuthorizationCache(ctx)
	if strings.TrimSpace(input.ID) == "" {
		id, err := newID("rolebinding")
		if err != nil {
			return access.RoleBinding{}, err
		}
		input.ID = id
	}
	if strings.TrimSpace(input.WorkspaceID) == "" {
		return access.RoleBinding{}, fmt.Errorf("workspace id is required")
	}
	role, principalID, groupID, err := r.roleBindingParts(ctx, input)
	if err != nil {
		return access.RoleBinding{}, err
	}
	if err := r.q.InsertRoleBinding(ctx, platformdb.InsertRoleBindingParams{
		ID:          input.ID,
		WorkspaceID: input.WorkspaceID,
		RoleID:      role.ID,
		PrincipalID: principalID,
		GroupID:     groupID,
	}); err != nil {
		return access.RoleBinding{}, err
	}
	if err := r.syncRoleBindingGrants(ctx, input.ID, input.WorkspaceID, input.Role, input.SubjectType, input.SubjectID); err != nil {
		return access.RoleBinding{}, err
	}
	return r.GetRoleBinding(ctx, input.WorkspaceID, input.ID)
}

func (r *Repository) GetRoleBinding(ctx context.Context, workspaceID, id string) (access.RoleBinding, error) {
	row, err := r.q.GetRoleBindingByID(ctx, platformdb.GetRoleBindingByIDParams{
		WorkspaceID: workspaceID,
		ID:          id,
	})
	if err != nil {
		return access.RoleBinding{}, err
	}
	return mapRoleBinding(row), nil
}

func (r *Repository) UpdateRoleBinding(ctx context.Context, workspaceID, id string, input access.RoleBindingInput) (access.RoleBinding, error) {
	access.ClearAuthorizationCache(ctx)
	input.WorkspaceID = workspaceID
	role, principalID, groupID, err := r.roleBindingParts(ctx, input)
	if err != nil {
		return access.RoleBinding{}, err
	}
	if err := r.q.UpdateRoleBindingByID(ctx, platformdb.UpdateRoleBindingByIDParams{
		RoleID:      role.ID,
		PrincipalID: principalID,
		GroupID:     groupID,
		WorkspaceID: workspaceID,
		ID:          id,
	}); err != nil {
		return access.RoleBinding{}, err
	}
	if err := r.deleteRoleBindingGrants(ctx, id); err != nil {
		return access.RoleBinding{}, err
	}
	if err := r.syncRoleBindingGrants(ctx, id, workspaceID, input.Role, input.SubjectType, input.SubjectID); err != nil {
		return access.RoleBinding{}, err
	}
	return r.GetRoleBinding(ctx, workspaceID, id)
}

func (r *Repository) DeleteRoleBinding(ctx context.Context, workspaceID, id string) error {
	access.ClearAuthorizationCache(ctx)
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("role binding id is required")
	}
	if err := r.deleteRoleBindingGrants(ctx, id); err != nil {
		return err
	}
	return r.q.DeleteRoleBindingByID(ctx, platformdb.DeleteRoleBindingByIDParams{
		WorkspaceID: workspaceID,
		ID:          id,
	})
}

func (r *Repository) ListRoleBindings(ctx context.Context, workspaceID string) ([]access.RoleBinding, error) {
	rows, err := r.q.ListRoleBindingsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	bindings := make([]access.RoleBinding, 0, len(rows))
	for _, row := range rows {
		bindings = append(bindings, mapListedRoleBinding(row))
	}
	return bindings, nil
}

func (r *Repository) ListAllRoleBindings(ctx context.Context) ([]access.RoleBinding, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT rb.id, rb.workspace_id, COALESCE(p.id, ''), COALESCE(g.id, ''), COALESCE(p.email, ''), COALESCE(p.display_name, ''), COALESCE(g.name, ''), roles.name, rb.created_at
FROM role_bindings rb
JOIN roles ON roles.id = rb.role_id
LEFT JOIN principals p ON p.id = rb.principal_id
LEFT JOIN groups g ON g.id = rb.group_id
ORDER BY rb.workspace_id, rb.created_at, rb.id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := []access.RoleBinding{}
	for rows.Next() {
		var binding access.RoleBinding
		if err := rows.Scan(&binding.ID, &binding.WorkspaceID, &binding.PrincipalID, &binding.GroupID, &binding.Email, &binding.DisplayName, &binding.GroupName, &binding.Role, &binding.CreatedAt); err != nil {
			return nil, err
		}
		if binding.GroupID != "" {
			binding.SubjectType = access.SubjectGroup
			binding.SubjectID = binding.GroupID
		} else {
			binding.SubjectType = access.SubjectPrincipal
			binding.SubjectID = binding.PrincipalID
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (r *Repository) ListRoles(ctx context.Context) ([]access.Role, error) {
	rows, err := r.q.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	roles := make([]access.Role, 0, len(rows))
	for _, row := range rows {
		var privileges []access.Privilege
		_ = json.Unmarshal([]byte(row.PrivilegesJson), &privileges)
		roles = append(roles, access.Role{Name: row.Name, Privileges: privileges})
	}
	return roles, nil
}
