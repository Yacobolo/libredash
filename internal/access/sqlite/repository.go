package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Repository struct {
	db *sql.DB
	q  *platformdb.Queries
}

const defaultAPITokenTTL = 90 * 24 * time.Hour

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{db: sqlDB, q: platformdb.New(sqlDB)}
}

func (r *Repository) PrincipalByID(ctx context.Context, id string) (access.Principal, error) {
	row, err := r.q.GetPrincipal(ctx, id)
	if err != nil {
		return access.Principal{}, err
	}
	return mapPrincipal(row), nil
}

func (r *Repository) ListPrincipals(ctx context.Context, filter access.PrincipalFilter) ([]access.Principal, error) {
	if r == nil || r.db == nil {
		return []access.Principal{}, nil
	}
	email := strings.TrimSpace(filter.Email)
	query := strings.TrimSpace(filter.Query)
	rows, err := r.db.QueryContext(ctx, `
SELECT id, kind, email, display_name, disabled_at, created_at, updated_at
FROM principals
WHERE (? = '' OR lower(email) = lower(?))
  AND (? = '' OR lower(email) LIKE '%' || lower(?) || '%' OR lower(display_name) LIKE '%' || lower(?) || '%')
ORDER BY email, id
`, email, email, query, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []access.Principal{}
	for rows.Next() {
		var principal access.Principal
		var disabledAt sql.NullString
		if err := rows.Scan(&principal.ID, &principal.Kind, &principal.Email, &principal.DisplayName, &disabledAt, &principal.CreatedAt, &principal.UpdatedAt); err != nil {
			return nil, err
		}
		if disabledAt.Valid {
			principal.DisabledAt = disabledAt.String
		}
		out = append(out, principal)
	}
	return out, rows.Err()
}

func (r *Repository) principalDisabled(ctx context.Context, principalID string) (bool, error) {
	row, err := r.q.GetPrincipal(ctx, principalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return row.DisabledAt.Valid && row.DisabledAt.String != "", nil
}

func (r *Repository) UpsertPrincipal(ctx context.Context, input access.PrincipalInput) (access.Principal, error) {
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("principal")
	}
	if input.Kind == "" {
		input.Kind = access.PrincipalKindUser
	}
	if err := r.q.UpsertPrincipal(ctx, platformdb.UpsertPrincipalParams{
		ID:          input.ID,
		Kind:        string(input.Kind),
		Email:       input.Email,
		DisplayName: input.DisplayName,
	}); err != nil {
		return access.Principal{}, err
	}
	row, err := r.q.GetPrincipal(ctx, input.ID)
	if err != nil {
		return access.Principal{}, err
	}
	return mapPrincipal(row), nil
}

func (r *Repository) SetPrincipalRole(ctx context.Context, input access.PrincipalRoleInput) (access.Principal, error) {
	email := access.NormalizeEmail(input.Email)
	if email == "" {
		return access.Principal{}, fmt.Errorf("email is required")
	}
	if strings.TrimSpace(input.Role) == "" {
		return access.Principal{}, fmt.Errorf("role is required")
	}
	role, err := r.q.GetRoleByName(ctx, input.Role)
	if err != nil {
		return access.Principal{}, err
	}
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          access.PrincipalIDForEmail(email),
		Email:       email,
		DisplayName: firstNonEmpty(strings.TrimSpace(input.DisplayName), email),
	})
	if err != nil {
		return access.Principal{}, err
	}
	if err := r.q.DeletePrincipalRoleBindings(ctx, platformdb.DeletePrincipalRoleBindingsParams{
		WorkspaceID: input.WorkspaceID,
		PrincipalID: sql.NullString{String: principal.ID, Valid: true},
	}); err != nil {
		return access.Principal{}, err
	}
	bindingID := stableAccessID("rolebinding", input.WorkspaceID, principal.ID+"|"+input.Role)
	if err := r.deleteRoleBindingGrants(ctx, bindingID); err != nil {
		return access.Principal{}, err
	}
	if err := r.q.InsertRoleBinding(ctx, platformdb.InsertRoleBindingParams{
		ID:          bindingID,
		WorkspaceID: input.WorkspaceID,
		RoleID:      role.ID,
		PrincipalID: sql.NullString{String: principal.ID, Valid: true},
	}); err != nil {
		return access.Principal{}, err
	}
	if err := r.syncRoleBindingGrants(ctx, bindingID, input.WorkspaceID, input.Role, access.SubjectPrincipal, principal.ID); err != nil {
		return access.Principal{}, err
	}
	return principal, nil
}

func (r *Repository) SetPlatformRole(ctx context.Context, input access.PlatformRoleInput) (access.Principal, error) {
	principalID := strings.TrimSpace(input.PrincipalID)
	email := access.NormalizeEmail(input.Email)
	if principalID == "" && email == "" {
		return access.Principal{}, fmt.Errorf("principal id or email is required")
	}
	if strings.TrimSpace(input.Role) == "" {
		return access.Principal{}, fmt.Errorf("role is required")
	}
	role, err := r.q.GetRoleByName(ctx, input.Role)
	if err != nil {
		return access.Principal{}, err
	}
	if principalID == "" {
		principalID = access.PrincipalIDForEmail(email)
	}
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          principalID,
		Email:       email,
		DisplayName: firstNonEmpty(strings.TrimSpace(input.DisplayName), email, principalID),
	})
	if err != nil {
		return access.Principal{}, err
	}
	if err := r.q.InsertPlatformRoleBinding(ctx, platformdb.InsertPlatformRoleBindingParams{
		ID:          newID("platformrolebinding"),
		RoleID:      role.ID,
		PrincipalID: principal.ID,
	}); err != nil {
		return access.Principal{}, err
	}
	privileges, err := r.rolePrivileges(ctx, firstNonEmpty(input.Role, access.RolePlatformAdmin))
	if err != nil {
		return access.Principal{}, err
	}
	for _, privilege := range privileges {
		if err := r.upsertGrantWithID(ctx, "grant_platform_"+stableID(principal.ID+"|"+string(privilege)), access.GrantInput{
			Object:      access.PlatformObject(),
			SubjectType: access.SubjectPrincipal,
			SubjectID:   principal.ID,
			Privilege:   privilege,
		}); err != nil {
			return access.Principal{}, err
		}
	}
	return principal, nil
}

func (r *Repository) RemovePrincipalRoles(ctx context.Context, workspaceID, principalID string) error {
	if strings.TrimSpace(principalID) == "" {
		return fmt.Errorf("principal id is required")
	}
	return r.q.DeletePrincipalRoleBindings(ctx, platformdb.DeletePrincipalRoleBindingsParams{
		WorkspaceID: workspaceID,
		PrincipalID: sql.NullString{String: principalID, Valid: true},
	})
}

func (r *Repository) CreateRoleBinding(ctx context.Context, input access.RoleBindingInput) (access.RoleBinding, error) {
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("rolebinding")
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

func (r *Repository) Authorize(ctx context.Context, principalID string, privilege access.Privilege, object access.ObjectRef) (access.AuthorizationDecision, error) {
	decision := access.AuthorizationDecision{Privilege: privilege, Object: object}
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		decision.Reason = "missing_principal"
		return decision, nil
	}
	if strings.TrimSpace(string(privilege)) == "" {
		decision.Reason = "missing_privilege"
		return decision, nil
	}
	if disabled, err := r.principalDisabled(ctx, principalID); err != nil {
		return decision, err
	} else if disabled {
		decision.Reason = "principal_disabled"
		return decision, nil
	}
	objectID, err := r.ensureSecurableObject(ctx, object)
	if err != nil {
		return decision, err
	}
	if owner, err := r.objectOwner(ctx, objectID); err != nil {
		return decision, err
	} else if owner != "" && owner == principalID {
		decision.Allowed = true
		decision.Owner = true
		decision.Reason = "owner"
		return decision, nil
	}
	platformDecision, err := r.authorizeByGrant(ctx, principalID, access.PrivilegeManagePlatform, []string{access.PlatformObject().CanonicalID()})
	if err != nil {
		return decision, err
	}
	if platformDecision.Allowed {
		decision.Allowed = true
		decision.Platform = true
		decision.GrantID = platformDecision.GrantID
		decision.GrantObjectID = platformDecision.GrantObjectID
		decision.SubjectType = platformDecision.SubjectType
		decision.SubjectID = platformDecision.SubjectID
		decision.Reason = "platform_admin"
		return decision, nil
	}
	objectIDs, err := r.objectAncestry(ctx, objectID)
	if err != nil {
		return decision, err
	}
	grantDecision, err := r.authorizeByGrant(ctx, principalID, privilege, objectIDs)
	if err != nil {
		return decision, err
	}
	if grantDecision.Allowed {
		grantDecision.Privilege = privilege
		grantDecision.Object = object
		return grantDecision, nil
	}
	decision.Reason = "no_grant"
	return decision, nil
}

func (r *Repository) AuthorizeAny(ctx context.Context, principalID string, privilege access.Privilege, objects []access.ObjectRef) (access.AuthorizationDecision, error) {
	if len(objects) == 0 {
		return access.AuthorizationDecision{Privilege: privilege, Reason: "missing_object"}, nil
	}
	var last access.AuthorizationDecision
	for _, object := range objects {
		decision, err := r.Authorize(ctx, principalID, privilege, object)
		if err != nil {
			return decision, err
		}
		if decision.Allowed {
			return decision, nil
		}
		last = decision
	}
	return last, nil
}

func (r *Repository) EffectivePrivileges(ctx context.Context, principalID string, object access.ObjectRef) ([]access.Privilege, error) {
	out := []access.Privilege{}
	effective, err := r.EffectiveAccess(ctx, principalID, object)
	if err != nil {
		return nil, err
	}
	for _, decision := range effective {
		out = append(out, decision.Privilege)
	}
	return out, nil
}

func (r *Repository) EffectiveAccess(ctx context.Context, principalID string, object access.ObjectRef) ([]access.AuthorizationDecision, error) {
	out := []access.AuthorizationDecision{}
	for _, privilege := range knownPrivileges() {
		decision, err := r.Authorize(ctx, principalID, privilege, object)
		if err != nil {
			return nil, err
		}
		if decision.Allowed {
			out = append(out, decision)
		}
	}
	return out, nil
}

func (r *Repository) CreateGrant(ctx context.Context, input access.GrantInput) (access.Grant, error) {
	id := newID("grant")
	if err := r.upsertGrantWithID(ctx, id, input); err != nil {
		return access.Grant{}, err
	}
	grants, err := r.ListGrants(ctx, input.Object)
	if err != nil {
		return access.Grant{}, err
	}
	for _, grant := range grants {
		if grant.ID == id {
			return grant, nil
		}
	}
	return access.Grant{}, sql.ErrNoRows
}

func (r *Repository) GetGrant(ctx context.Context, workspaceID, id string) (access.Grant, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT g.id, g.object_id, so.object_type, so.workspace_id, g.subject_type, g.subject_id, g.privilege, g.created_at
FROM grants g
JOIN securable_objects so ON so.id = g.object_id
WHERE g.id = ?
  AND (so.workspace_id = ? OR so.id = ?)
`, id, workspaceID, access.WorkspaceObject(workspaceID).CanonicalID())
	var grant access.Grant
	var objectType, subjectType, privilege string
	if err := row.Scan(&grant.ID, &grant.ObjectID, &objectType, &grant.WorkspaceID, &subjectType, &grant.SubjectID, &privilege, &grant.CreatedAt); err != nil {
		return access.Grant{}, err
	}
	grant.ObjectType = access.SecurableType(objectType)
	grant.SubjectType = access.SubjectType(subjectType)
	grant.Privilege = access.Privilege(privilege)
	return grant, nil
}

func (r *Repository) DeleteGrant(ctx context.Context, workspaceID, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("grant id is required")
	}
	_, err := r.db.ExecContext(ctx, `
DELETE FROM grants
WHERE id = ?
  AND object_id IN (
    SELECT id FROM securable_objects
    WHERE workspace_id = ? OR id = ?
  )
`, id, workspaceID, access.WorkspaceObject(workspaceID).CanonicalID())
	return err
}

func (r *Repository) UpsertDataPolicy(ctx context.Context, input access.DataPolicyInput) (access.DataPolicy, error) {
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("datapolicy")
	}
	if strings.TrimSpace(input.PolicyType) == "" {
		return access.DataPolicy{}, fmt.Errorf("data policy type is required")
	}
	if strings.TrimSpace(input.ExpressionJSON) == "" {
		input.ExpressionJSON = "{}"
	}
	objectID, err := r.ensureSecurableObject(ctx, input.Object)
	if err != nil {
		return access.DataPolicy{}, err
	}
	if input.SubjectType != "" && strings.TrimSpace(input.SubjectID) == "" {
		return access.DataPolicy{}, fmt.Errorf("data policy subject id is required")
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO data_policies (id, workspace_id, object_id, subject_type, subject_id, policy_type, expression_json)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  workspace_id = excluded.workspace_id,
  object_id = excluded.object_id,
  subject_type = excluded.subject_type,
  subject_id = excluded.subject_id,
  policy_type = excluded.policy_type,
  expression_json = excluded.expression_json,
  updated_at = CURRENT_TIMESTAMP
`, input.ID, input.Object.WorkspaceID, objectID, string(input.SubjectType), strings.TrimSpace(input.SubjectID), input.PolicyType, input.ExpressionJSON)
	if err != nil {
		return access.DataPolicy{}, err
	}
	return r.GetDataPolicy(ctx, input.Object.WorkspaceID, input.ID)
}

func (r *Repository) GetDataPolicy(ctx context.Context, workspaceID, id string) (access.DataPolicy, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, workspace_id, object_id, subject_type, subject_id, policy_type, expression_json, created_at, updated_at
FROM data_policies
WHERE id = ? AND workspace_id = ?
`, id, workspaceID)
	var policy access.DataPolicy
	var subjectType string
	if err := row.Scan(&policy.ID, &policy.WorkspaceID, &policy.ObjectID, &subjectType, &policy.SubjectID, &policy.PolicyType, &policy.ExpressionJSON, &policy.CreatedAt, &policy.UpdatedAt); err != nil {
		return access.DataPolicy{}, err
	}
	policy.SubjectType = access.SubjectType(subjectType)
	return policy, nil
}

func (r *Repository) ListDataPolicies(ctx context.Context, object access.ObjectRef) ([]access.DataPolicy, error) {
	return r.ListDataPoliciesWithOptions(ctx, object, false)
}

func (r *Repository) ListDataPoliciesWithOptions(ctx context.Context, object access.ObjectRef, includeInherited bool) ([]access.DataPolicy, error) {
	objectID, err := r.ensureSecurableObject(ctx, object)
	if err != nil {
		return nil, err
	}
	objectIDs := []string{objectID}
	if includeInherited {
		objectIDs, err = r.objectAncestry(ctx, objectID)
		if err != nil {
			return nil, err
		}
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(objectIDs)), ",")
	args := make([]any, 0, len(objectIDs)*2)
	for _, id := range objectIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, workspace_id, object_id, subject_type, subject_id, policy_type, expression_json, created_at, updated_at
FROM data_policies
WHERE object_id IN (`+placeholders+`)
ORDER BY CASE object_id`+grantOrderCase(objectIDs)+` ELSE 999 END, policy_type, id
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	policies := []access.DataPolicy{}
	for rows.Next() {
		var policy access.DataPolicy
		var subjectType string
		if err := rows.Scan(&policy.ID, &policy.WorkspaceID, &policy.ObjectID, &subjectType, &policy.SubjectID, &policy.PolicyType, &policy.ExpressionJSON, &policy.CreatedAt, &policy.UpdatedAt); err != nil {
			return nil, err
		}
		policy.SubjectType = access.SubjectType(subjectType)
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func (r *Repository) ListEffectiveDataPolicies(ctx context.Context, principalID string, object access.ObjectRef, includeInherited bool) ([]access.DataPolicy, error) {
	policies, err := r.ListDataPoliciesWithOptions(ctx, object, includeInherited)
	if err != nil {
		return nil, err
	}
	out := make([]access.DataPolicy, 0, len(policies))
	for _, policy := range policies {
		applies, err := r.dataPolicyAppliesToPrincipal(ctx, policy, principalID)
		if err != nil {
			return nil, err
		}
		if applies {
			out = append(out, policy)
		}
	}
	return out, nil
}

func (r *Repository) dataPolicyAppliesToPrincipal(ctx context.Context, policy access.DataPolicy, principalID string) (bool, error) {
	switch policy.SubjectType {
	case "":
		return true, nil
	case access.SubjectPrincipal, access.SubjectServicePrincipal:
		return strings.TrimSpace(policy.SubjectID) == strings.TrimSpace(principalID), nil
	case access.SubjectGroup:
		var found string
		err := r.db.QueryRowContext(ctx, `
SELECT principal_id
FROM group_members
WHERE group_id = ? AND principal_id = ?
LIMIT 1
`, policy.SubjectID, principalID).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return err == nil, err
	default:
		return false, fmt.Errorf("unsupported data policy subject type %q", policy.SubjectType)
	}
}

func (r *Repository) DeleteDataPolicy(ctx context.Context, workspaceID, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("data policy id is required")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM data_policies WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	return err
}

func (r *Repository) SetObjectOwner(ctx context.Context, object access.ObjectRef, ownerPrincipalID string) (access.SecurableObject, error) {
	ownerPrincipalID = strings.TrimSpace(ownerPrincipalID)
	if ownerPrincipalID == "" {
		return access.SecurableObject{}, fmt.Errorf("owner principal id is required")
	}
	objectID, err := r.ensureSecurableObject(ctx, object)
	if err != nil {
		return access.SecurableObject{}, err
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE securable_objects
SET owner_principal_id = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, ownerPrincipalID, objectID); err != nil {
		return access.SecurableObject{}, err
	}
	return r.securableObjectByID(ctx, objectID)
}

func (r *Repository) ListGrants(ctx context.Context, object access.ObjectRef) ([]access.Grant, error) {
	views, err := r.ListGrantsWithOptions(ctx, object, false)
	if err != nil {
		return nil, err
	}
	grants := make([]access.Grant, 0, len(views))
	for _, view := range views {
		grants = append(grants, view.Grant)
	}
	return grants, nil
}

func (r *Repository) ListGrantsWithOptions(ctx context.Context, object access.ObjectRef, includeInherited bool) ([]access.GrantView, error) {
	objectID, err := r.ensureSecurableObject(ctx, object)
	if err != nil {
		return nil, err
	}
	objectIDs := []string{objectID}
	if includeInherited {
		objectIDs, err = r.objectAncestry(ctx, objectID)
		if err != nil {
			return nil, err
		}
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(objectIDs)), ",")
	args := make([]any, 0, len(objectIDs)*2)
	for _, id := range objectIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT g.id, g.object_id, so.object_type, so.workspace_id, so.parent_id,
       parent.object_type, parent.id,
       g.subject_type, g.subject_id, g.privilege, g.created_at
FROM grants g
JOIN securable_objects so ON so.id = g.object_id
LEFT JOIN securable_objects parent ON parent.id = so.parent_id
WHERE g.object_id IN (`+placeholders+`)
ORDER BY CASE g.object_id`+grantOrderCase(objectIDs)+` ELSE 999 END, g.subject_type, g.subject_id, g.privilege
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := []access.GrantView{}
	for rows.Next() {
		var grant access.GrantView
		var objectType, subjectType, privilege string
		var parentType, parentID sql.NullString
		if err := rows.Scan(&grant.ID, &grant.ObjectID, &objectType, &grant.WorkspaceID, &grant.ParentID, &parentType, &parentID, &subjectType, &grant.SubjectID, &privilege, &grant.CreatedAt); err != nil {
			return nil, err
		}
		grant.ObjectType = access.SecurableType(objectType)
		grant.SubjectType = access.SubjectType(subjectType)
		grant.Privilege = access.Privilege(privilege)
		grant.Inherited = grant.ObjectID != objectID
		grant.ParentType = access.SecurableType(nullString(parentType))
		grant.ParentObject = nullString(parentID)
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func (r *Repository) authorizeByGrant(ctx context.Context, principalID string, privilege access.Privilege, objectIDs []string) (access.AuthorizationDecision, error) {
	decision := access.AuthorizationDecision{Privilege: privilege}
	if len(objectIDs) == 0 {
		return decision, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(objectIDs)), ",")
	args := []any{principalID, string(privilege), principalID}
	for _, id := range objectIDs {
		args = append(args, id)
	}
	query := `
SELECT g.id, g.object_id, g.subject_type, g.subject_id
FROM grants g
LEFT JOIN group_members gm
  ON g.subject_type = 'group'
 AND gm.group_id = g.subject_id
 AND gm.principal_id = ?
WHERE g.privilege = ?
  AND (g.subject_type IN ('principal', 'service_principal') AND g.subject_id = ? OR gm.principal_id IS NOT NULL)
  AND g.object_id IN (` + placeholders + `)
ORDER BY CASE g.object_id`
	for i, id := range objectIDs {
		query += fmt.Sprintf(" WHEN ? THEN %d", i)
		args = append(args, id)
	}
	query += " ELSE 999 END LIMIT 1"
	var grantID, objectID, subjectType, subjectID string
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&grantID, &objectID, &subjectType, &subjectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return decision, nil
		}
		return decision, err
	}
	decision.Allowed = true
	decision.GrantID = grantID
	decision.GrantObjectID = objectID
	decision.SubjectType = access.SubjectType(subjectType)
	decision.SubjectID = subjectID
	decision.Inherited = len(objectIDs) > 0 && objectID != objectIDs[0]
	decision.Reason = "grant"
	return decision, nil
}

func (r *Repository) ensureSecurableObject(ctx context.Context, object access.ObjectRef) (string, error) {
	objectID := object.CanonicalID()
	if strings.TrimSpace(objectID) == "" {
		return "", fmt.Errorf("securable object id is required")
	}
	parentID := ""
	if strings.TrimSpace(object.ParentID) != "" {
		parentID = strings.TrimSpace(object.ParentID)
	} else if parent, ok := object.Parent(); ok {
		parentID = parent.CanonicalID()
		if _, err := r.ensureSecurableObject(ctx, parent); err != nil {
			return "", err
		}
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO securable_objects (id, object_type, workspace_id, parent_id, display_name)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  object_type = excluded.object_type,
  workspace_id = excluded.workspace_id,
  parent_id = excluded.parent_id,
  display_name = COALESCE(NULLIF(excluded.display_name, ''), securable_objects.display_name),
  updated_at = CURRENT_TIMESTAMP
`, objectID, string(object.Type), object.WorkspaceID, parentID, objectDisplayName(object))
	return objectID, err
}

func (r *Repository) objectAncestry(ctx context.Context, objectID string) ([]string, error) {
	ids := []string{}
	seen := map[string]bool{}
	current := objectID
	for current != "" {
		if seen[current] {
			return nil, fmt.Errorf("securable object parent cycle at %q", current)
		}
		seen[current] = true
		ids = append(ids, current)
		var parentID string
		err := r.db.QueryRowContext(ctx, `SELECT parent_id FROM securable_objects WHERE id = ?`, current).Scan(&parentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, err
		}
		current = parentID
	}
	return ids, nil
}

func grantOrderCase(objectIDs []string) string {
	out := ""
	for i, id := range objectIDs {
		out += fmt.Sprintf(" WHEN '%s' THEN %d", strings.ReplaceAll(id, "'", "''"), i)
	}
	return out
}

func objectDisplayName(object access.ObjectRef) string {
	if object.ObjectID != "" {
		return object.ObjectID
	}
	if object.WorkspaceID != "" {
		return object.WorkspaceID
	}
	return string(object.Type)
}

func (r *Repository) objectOwner(ctx context.Context, objectID string) (string, error) {
	var owner string
	err := r.db.QueryRowContext(ctx, `SELECT owner_principal_id FROM securable_objects WHERE id = ?`, objectID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return owner, err
}

func (r *Repository) securableObjectByID(ctx context.Context, objectID string) (access.SecurableObject, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, object_type, workspace_id, parent_id, owner_principal_id, display_name, created_at, updated_at
FROM securable_objects
WHERE id = ?
`, objectID)
	var object access.SecurableObject
	var objectType string
	if err := row.Scan(&object.ID, &objectType, &object.WorkspaceID, &object.ParentID, &object.OwnerPrincipalID, &object.DisplayName, &object.CreatedAt, &object.UpdatedAt); err != nil {
		return access.SecurableObject{}, err
	}
	object.Type = access.SecurableType(objectType)
	return object, nil
}

func (r *Repository) syncRoleBindingGrants(ctx context.Context, bindingID, workspaceID, roleName string, subjectType access.SubjectType, subjectID string) error {
	if err := r.ensureWorkspaceSecurable(ctx, workspaceID); err != nil {
		return err
	}
	privileges, err := r.rolePrivileges(ctx, roleName)
	if err != nil {
		return err
	}
	for _, privilege := range privileges {
		grantID := roleBindingGrantID(bindingID, privilege)
		if err := r.upsertGrantWithID(ctx, grantID, access.GrantInput{
			Object:      access.WorkspaceObject(workspaceID),
			SubjectType: subjectType,
			SubjectID:   subjectID,
			Privilege:   privilege,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) deleteRoleBindingGrants(ctx context.Context, bindingID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM grants WHERE id LIKE ?`, "grant_"+bindingID+"_%")
	return err
}

func roleBindingGrantID(bindingID string, privilege access.Privilege) string {
	return "grant_" + bindingID + "_" + strings.ToLower(string(privilege))
}

func (r *Repository) rolePrivileges(ctx context.Context, roleName string) ([]access.Privilege, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT privilege FROM role_grant_templates WHERE role_name = ? ORDER BY privilege`, roleName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	privileges := []access.Privilege{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		privileges = append(privileges, access.Privilege(value))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(privileges) == 0 {
		return nil, fmt.Errorf("role %q has no grant template", roleName)
	}
	return privileges, nil
}

func (r *Repository) upsertGrantWithID(ctx context.Context, id string, input access.GrantInput) error {
	subjectID := strings.TrimSpace(input.SubjectID)
	if subjectID == "" {
		return fmt.Errorf("grant subject id is required")
	}
	if input.SubjectType == "" {
		return fmt.Errorf("grant subject type is required")
	}
	if input.Privilege == "" {
		return fmt.Errorf("grant privilege is required")
	}
	objectID, err := r.ensureSecurableObject(ctx, input.Object)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO grants (id, object_id, subject_type, subject_id, privilege)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(object_id, subject_type, subject_id, privilege) DO UPDATE SET id = excluded.id
`, id, objectID, string(input.SubjectType), subjectID, string(input.Privilege))
	return err
}

func (r *Repository) ensureWorkspaceSecurable(ctx context.Context, workspaceID string) error {
	_, err := r.ensureSecurableObject(ctx, access.WorkspaceObject(workspaceID))
	return err
}

func knownPrivileges() []access.Privilege {
	return []access.Privilege{
		access.PrivilegeUseWorkspace,
		access.PrivilegeViewItem,
		access.PrivilegeEditItem,
		access.PrivilegeManageItem,
		access.PrivilegeQueryData,
		access.PrivilegePreviewData,
		access.PrivilegeRefreshData,
		access.PrivilegeDeploy,
		access.PrivilegeActivatePublish,
		access.PrivilegeUseAgent,
		access.PrivilegeViewAgent,
		access.PrivilegeManageGrants,
		access.PrivilegeViewAudit,
		access.PrivilegeManageWorkspace,
		access.PrivilegeManagePlatform,
	}
}

func (r *Repository) BootstrapAdmin(ctx context.Context, workspaceID, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          access.PrincipalIDForEmail(email),
		Email:       email,
		DisplayName: email,
	})
	if err != nil {
		return err
	}
	role, err := r.q.GetRoleByName(ctx, access.RoleOwner)
	if err != nil {
		return err
	}
	bindingID := stableAccessID("rolebinding", workspaceID, principal.ID+"|"+access.RoleOwner)
	if err := r.deleteRoleBindingGrants(ctx, bindingID); err != nil {
		return err
	}
	if err := r.q.InsertRoleBinding(ctx, platformdb.InsertRoleBindingParams{
		ID:          bindingID,
		WorkspaceID: workspaceID,
		RoleID:      role.ID,
		PrincipalID: sql.NullString{String: principal.ID, Valid: principal.ID != ""},
	}); err != nil {
		return err
	}
	return r.syncRoleBindingGrants(ctx, bindingID, workspaceID, access.RoleOwner, access.SubjectPrincipal, principal.ID)
}

func (r *Repository) ResolveExternalPrincipal(ctx context.Context, input access.ExternalIdentityInput) (access.Principal, error) {
	input.Email = access.NormalizeEmail(input.Email)
	if input.Provider == "" || input.Subject == "" {
		return access.Principal{}, fmt.Errorf("external identity requires provider and subject")
	}
	identity, err := r.q.GetExternalIdentity(ctx, platformdb.GetExternalIdentityParams{
		Provider: input.Provider,
		TenantID: input.TenantID,
		Subject:  input.Subject,
	})
	if err == nil {
		principal, err := r.q.GetPrincipal(ctx, identity.PrincipalID)
		if err != nil {
			return access.Principal{}, err
		}
		if principal.DisabledAt.Valid && principal.DisabledAt.String != "" {
			return access.Principal{}, sql.ErrNoRows
		}
		return r.UpsertPrincipal(ctx, access.PrincipalInput{
			ID:          identity.PrincipalID,
			Email:       input.Email,
			DisplayName: input.DisplayName,
		})
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return access.Principal{}, err
	}

	var principal access.Principal
	if input.Email != "" {
		row, err := r.q.GetPrincipalByEmail(ctx, input.Email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return access.Principal{}, err
		}
		if err == nil {
			principal = mapPrincipal(row)
			if principal.DisabledAt != "" {
				return access.Principal{}, sql.ErrNoRows
			}
		}
	}
	if principal.ID == "" {
		principal, err = r.UpsertPrincipal(ctx, access.PrincipalInput{
			ID:          "external_" + stableID(input.Provider+"|"+input.TenantID+"|"+input.Subject),
			Email:       input.Email,
			DisplayName: input.DisplayName,
		})
		if err != nil {
			return access.Principal{}, err
		}
	} else {
		principal, err = r.UpsertPrincipal(ctx, access.PrincipalInput{
			ID:          principal.ID,
			Email:       input.Email,
			DisplayName: input.DisplayName,
		})
		if err != nil {
			return access.Principal{}, err
		}
	}

	if err := r.q.UpsertExternalIdentity(ctx, platformdb.UpsertExternalIdentityParams{
		ID:          "identity_" + stableID(input.Provider+"|"+input.TenantID+"|"+input.Subject),
		PrincipalID: principal.ID,
		Provider:    input.Provider,
		TenantID:    input.TenantID,
		Subject:     input.Subject,
		Email:       input.Email,
	}); err != nil {
		return access.Principal{}, err
	}
	return principal, nil
}

func (r *Repository) UpsertSCIMUser(ctx context.Context, input access.SCIMUserInput) (access.SCIMUser, error) {
	id := strings.TrimSpace(input.ID)
	existingSubject := ""
	if id == "" {
		id = "scim_user_" + stableID(firstNonEmpty(input.ExternalID, input.UserName, input.Email))
	} else if identity, err := r.q.GetExternalIdentityByPrincipalProvider(ctx, platformdb.GetExternalIdentityByPrincipalProviderParams{
		PrincipalID: id,
		Provider:    "scim",
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if _, principalErr := r.q.GetPrincipal(ctx, id); principalErr == nil {
				return access.SCIMUser{}, sql.ErrNoRows
			} else if principalErr != nil && !errors.Is(principalErr, sql.ErrNoRows) {
				return access.SCIMUser{}, principalErr
			}
		} else {
			return access.SCIMUser{}, err
		}
	} else {
		existingSubject = identity.Subject
	}
	subject := strings.TrimSpace(firstNonEmpty(input.ExternalID, existingSubject, input.ID, input.UserName, input.Email))
	if subject == "" {
		return access.SCIMUser{}, fmt.Errorf("scim user requires id, external id, userName, or email")
	}
	if id == "" {
		id = "scim_user_" + stableID(subject)
	}
	email := access.NormalizeEmail(firstNonEmpty(input.Email, input.UserName))
	displayName := strings.TrimSpace(firstNonEmpty(input.DisplayName, email, input.UserName, id))
	principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          id,
		Kind:        access.PrincipalKindUser,
		Email:       email,
		DisplayName: displayName,
	})
	if err != nil {
		return access.SCIMUser{}, err
	}
	if err := r.q.UpsertExternalIdentity(ctx, platformdb.UpsertExternalIdentityParams{
		ID:          "identity_" + stableID("scim||"+subject),
		PrincipalID: principal.ID,
		Provider:    "scim",
		TenantID:    "",
		Subject:     subject,
		Email:       email,
	}); err != nil {
		return access.SCIMUser{}, err
	}
	if !input.Active {
		return r.DisableSCIMUser(ctx, principal.ID)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE principals SET disabled_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, principal.ID); err != nil {
		return access.SCIMUser{}, err
	}
	row, err := r.q.GetPrincipal(ctx, principal.ID)
	if err != nil {
		return access.SCIMUser{}, err
	}
	return access.SCIMUser{Principal: mapPrincipal(row), ExternalID: subject}, nil
}

func (r *Repository) ListSCIMUsers(ctx context.Context, filter access.SCIMUserFilter) ([]access.SCIMUser, error) {
	if strings.TrimSpace(filter.ID) != "" {
		row, err := r.q.GetPrincipal(ctx, strings.TrimSpace(filter.ID))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return []access.SCIMUser{}, nil
			}
			return nil, err
		}
		identity, err := r.q.GetExternalIdentityByPrincipalProvider(ctx, platformdb.GetExternalIdentityByPrincipalProviderParams{
			PrincipalID: row.ID,
			Provider:    "scim",
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return []access.SCIMUser{}, nil
			}
			return nil, err
		}
		return []access.SCIMUser{{Principal: mapPrincipal(row), ExternalID: identity.Subject}}, nil
	}
	subject := strings.TrimSpace(firstNonEmpty(filter.ID, filter.ExternalID))
	rows, err := r.q.ListSCIMPrincipals(ctx, platformdb.ListSCIMPrincipalsParams{
		Subject:  subject,
		UserName: strings.TrimSpace(filter.UserName),
	})
	if err != nil {
		return nil, err
	}
	users := make([]access.SCIMUser, 0, len(rows))
	for _, row := range rows {
		identity, err := r.q.GetExternalIdentityByPrincipalProvider(ctx, platformdb.GetExternalIdentityByPrincipalProviderParams{
			PrincipalID: row.ID,
			Provider:    "scim",
		})
		if err != nil {
			return nil, err
		}
		users = append(users, access.SCIMUser{Principal: mapPrincipal(row), ExternalID: identity.Subject})
	}
	return users, nil
}

func (r *Repository) DisableSCIMUser(ctx context.Context, principalID string) (access.SCIMUser, error) {
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return access.SCIMUser{}, fmt.Errorf("principal id is required")
	}
	identity, err := r.q.GetExternalIdentityByPrincipalProvider(ctx, platformdb.GetExternalIdentityByPrincipalProviderParams{
		PrincipalID: principalID,
		Provider:    "scim",
	})
	if err != nil {
		return access.SCIMUser{}, err
	}
	if err := r.q.DisablePrincipal(ctx, principalID); err != nil {
		return access.SCIMUser{}, err
	}
	if err := r.q.DeleteSCIMGroupMembersByPrincipal(ctx, principalID); err != nil {
		return access.SCIMUser{}, err
	}
	if err := r.q.RevokeSessionsByPrincipal(ctx, principalID); err != nil {
		return access.SCIMUser{}, err
	}
	if err := r.q.RevokeAPITokensByPrincipal(ctx, principalID); err != nil {
		return access.SCIMUser{}, err
	}
	row, err := r.q.GetPrincipal(ctx, principalID)
	if err != nil {
		return access.SCIMUser{}, err
	}
	return access.SCIMUser{Principal: mapPrincipal(row), ExternalID: identity.Subject}, nil
}

func (r *Repository) UpsertGroup(ctx context.Context, input access.GroupInput) (access.Group, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" {
		return access.Group{}, fmt.Errorf("workspace id is required")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return access.Group{}, fmt.Errorf("group name is required")
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = newID("group")
	}
	if strings.TrimSpace(input.Provider) == "" && strings.TrimSpace(input.ExternalID) == "" {
		input.Provider = "local"
		input.ExternalID = input.ID
	}
	if err := r.q.UpsertGroup(ctx, platformdb.UpsertGroupParams{
		ID:          input.ID,
		WorkspaceID: input.WorkspaceID,
		Provider:    input.Provider,
		ExternalID:  input.ExternalID,
		Name:        input.Name,
	}); err != nil {
		return access.Group{}, err
	}
	row, err := r.q.GetGroupByProviderExternalID(ctx, platformdb.GetGroupByProviderExternalIDParams{
		WorkspaceID: input.WorkspaceID,
		Provider:    input.Provider,
		ExternalID:  input.ExternalID,
	})
	if err != nil {
		return access.Group{}, err
	}
	return mapGroup(row), nil
}

func (r *Repository) ListGroups(ctx context.Context, workspaceID string) ([]access.Group, error) {
	rows, err := r.q.ListGroupsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	groups := make([]access.Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, mapGroup(row))
	}
	return groups, nil
}

func (r *Repository) ListAllGroups(ctx context.Context) ([]access.Group, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, workspace_id, provider, external_id, name, created_at
FROM groups
ORDER BY workspace_id, name, id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []access.Group{}
	for rows.Next() {
		var group access.Group
		if err := rows.Scan(&group.ID, &group.WorkspaceID, &group.Provider, &group.ExternalID, &group.Name, &group.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (r *Repository) DeleteGroup(ctx context.Context, workspaceID, groupID string) error {
	if strings.TrimSpace(groupID) == "" {
		return fmt.Errorf("group id is required")
	}
	return r.q.DeleteGroup(ctx, platformdb.DeleteGroupParams{
		WorkspaceID: workspaceID,
		ID:          groupID,
	})
}

func (r *Repository) AddGroupMember(ctx context.Context, workspaceID, groupID, principalID string) error {
	if strings.TrimSpace(groupID) == "" || strings.TrimSpace(principalID) == "" {
		return fmt.Errorf("group id and principal id are required")
	}
	return r.q.InsertGroupMember(ctx, platformdb.InsertGroupMemberParams{
		WorkspaceID: workspaceID,
		GroupID:     groupID,
		PrincipalID: principalID,
	})
}

func (r *Repository) RemoveGroupMember(ctx context.Context, workspaceID, groupID, principalID string) error {
	if strings.TrimSpace(groupID) == "" || strings.TrimSpace(principalID) == "" {
		return fmt.Errorf("group id and principal id are required")
	}
	return r.q.DeleteGroupMember(ctx, platformdb.DeleteGroupMemberParams{
		WorkspaceID: workspaceID,
		GroupID:     groupID,
		PrincipalID: principalID,
	})
}

func (r *Repository) ListGroupMembers(ctx context.Context, workspaceID, groupID string) ([]access.GroupMember, error) {
	rows, err := r.q.ListGroupMembers(ctx, platformdb.ListGroupMembersParams{
		WorkspaceID: workspaceID,
		GroupID:     groupID,
	})
	if err != nil {
		return nil, err
	}
	members := make([]access.GroupMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, access.GroupMember{
			GroupID:     row.GroupID,
			WorkspaceID: row.WorkspaceID,
			PrincipalID: row.PrincipalID,
			Email:       row.Email,
			DisplayName: row.DisplayName,
			CreatedAt:   row.CreatedAt,
		})
	}
	return members, nil
}

func (r *Repository) ListGroupMembersByGroup(ctx context.Context, groupID string) ([]access.GroupMember, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT gm.group_id, g.workspace_id, gm.principal_id, p.email, p.display_name, gm.created_at
FROM group_members gm
JOIN groups g ON g.id = gm.group_id
JOIN principals p ON p.id = gm.principal_id
WHERE gm.group_id = ?
ORDER BY p.email, p.display_name, gm.principal_id
`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []access.GroupMember{}
	for rows.Next() {
		var member access.GroupMember
		if err := rows.Scan(&member.GroupID, &member.WorkspaceID, &member.PrincipalID, &member.Email, &member.DisplayName, &member.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (r *Repository) UpsertSCIMGroup(ctx context.Context, input access.SCIMGroupInput) (access.Group, error) {
	externalID := strings.TrimSpace(firstNonEmpty(input.ExternalID, input.ID, input.Name))
	if externalID == "" {
		return access.Group{}, fmt.Errorf("scim group requires external id, id, or display name")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = externalID
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "scim_group_" + stableID(externalID)
	}
	if err := r.q.UpsertGroup(ctx, platformdb.UpsertGroupParams{
		ID:          id,
		WorkspaceID: "",
		Provider:    "scim",
		ExternalID:  externalID,
		Name:        name,
	}); err != nil {
		return access.Group{}, err
	}
	if input.MemberIDs != nil {
		if err := r.q.DeleteSCIMGroupMembers(ctx, id); err != nil {
			return access.Group{}, err
		}
		for _, principalID := range input.MemberIDs {
			principalID = strings.TrimSpace(principalID)
			if principalID == "" {
				continue
			}
			if err := r.q.InsertGroupMember(ctx, platformdb.InsertGroupMemberParams{
				WorkspaceID: "",
				GroupID:     id,
				PrincipalID: principalID,
			}); err != nil {
				return access.Group{}, err
			}
		}
	}
	row, err := r.q.GetSCIMGroup(ctx, id)
	if err != nil {
		return access.Group{}, err
	}
	return mapGroup(row), nil
}

func (r *Repository) ListSCIMGroups(ctx context.Context, filter access.SCIMGroupFilter) ([]access.Group, error) {
	if strings.TrimSpace(filter.ID) != "" {
		row, err := r.q.GetSCIMGroup(ctx, strings.TrimSpace(filter.ID))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return []access.Group{}, nil
			}
			return nil, err
		}
		return []access.Group{mapGroup(row)}, nil
	}
	rows, err := r.q.ListSCIMGroups(ctx, platformdb.ListSCIMGroupsParams{
		ExternalID:  strings.TrimSpace(filter.ExternalID),
		DisplayName: strings.TrimSpace(filter.DisplayName),
	})
	if err != nil {
		return nil, err
	}
	groups := make([]access.Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, mapGroup(row))
	}
	return groups, nil
}

func (r *Repository) DeleteSCIMGroup(ctx context.Context, groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group id is required")
	}
	if err := r.q.DeleteSCIMGroupMembers(ctx, groupID); err != nil {
		return err
	}
	return r.q.DeleteSCIMGroup(ctx, groupID)
}

func (r *Repository) AddSCIMGroupMember(ctx context.Context, groupID, principalID string) error {
	if strings.TrimSpace(groupID) == "" || strings.TrimSpace(principalID) == "" {
		return fmt.Errorf("group id and principal id are required")
	}
	return r.q.InsertGroupMember(ctx, platformdb.InsertGroupMemberParams{
		WorkspaceID: "",
		GroupID:     groupID,
		PrincipalID: principalID,
	})
}

func (r *Repository) RemoveSCIMGroupMember(ctx context.Context, groupID, principalID string) error {
	if strings.TrimSpace(groupID) == "" || strings.TrimSpace(principalID) == "" {
		return fmt.Errorf("group id and principal id are required")
	}
	return r.q.DeleteGroupMember(ctx, platformdb.DeleteGroupMemberParams{
		WorkspaceID: "",
		GroupID:     groupID,
		PrincipalID: principalID,
	})
}

func (r *Repository) ListSCIMGroupMembers(ctx context.Context, groupID string) ([]access.GroupMember, error) {
	rows, err := r.q.ListSCIMGroupMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members := make([]access.GroupMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, access.GroupMember{
			GroupID:     row.GroupID,
			WorkspaceID: row.WorkspaceID,
			PrincipalID: row.PrincipalID,
			Email:       row.Email,
			DisplayName: row.DisplayName,
			CreatedAt:   row.CreatedAt,
		})
	}
	return members, nil
}

func (r *Repository) ReconcileWorkspacePolicy(ctx context.Context, workspaceID string, policy workspace.AccessPolicy) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	bindings, err := r.ListRoleBindings(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		if err := r.DeleteRoleBinding(ctx, workspaceID, binding.ID); err != nil {
			return err
		}
	}
	if _, err := r.db.ExecContext(ctx, `
DELETE FROM grants
WHERE object_id IN (
  SELECT id FROM securable_objects WHERE workspace_id = ? OR id = ?
)
`, workspaceID, access.WorkspaceObject(workspaceID).CanonicalID()); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM data_policies WHERE workspace_id = ?`, workspaceID); err != nil {
		return err
	}
	groups, err := r.ListGroups(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if err := r.DeleteGroup(ctx, workspaceID, group.ID); err != nil {
			return err
		}
	}

	groupIDs := map[string]string{}
	for _, name := range sortedWorkspaceGroupNames(policy.Groups) {
		groupPolicy := policy.Groups[name]
		group, err := r.UpsertGroup(ctx, access.GroupInput{
			ID:          stableAccessID("group", workspaceID, name),
			WorkspaceID: workspaceID,
			Provider:    "local",
			ExternalID:  name,
			Name:        firstNonEmpty(groupPolicy.Name, name),
		})
		if err != nil {
			return err
		}
		groupIDs[name] = group.ID
		for _, member := range groupPolicy.Members {
			principal, err := r.policyPrincipal(ctx, member.PrincipalID, member.Email, member.DisplayName)
			if err != nil {
				return err
			}
			if err := r.AddGroupMember(ctx, workspaceID, group.ID, principal.ID); err != nil {
				return err
			}
		}
	}

	for _, name := range sortedWorkspaceRoleBindingNames(policy.RoleBindings) {
		binding := policy.RoleBindings[name]
		input := access.RoleBindingInput{
			ID:          stableAccessID("rolebinding", workspaceID, name),
			WorkspaceID: workspaceID,
			Role:        binding.Role,
		}
		switch binding.Subject.Kind {
		case string(access.SubjectGroup):
			groupID := groupIDs[binding.Subject.Group]
			if groupID == "" {
				return fmt.Errorf("workspace role binding %q references unknown group %q", name, binding.Subject.Group)
			}
			input.SubjectType = access.SubjectGroup
			input.SubjectID = groupID
		case string(access.SubjectPrincipal):
			principal, err := r.policyPrincipal(ctx, binding.Subject.PrincipalID, binding.Subject.Email, binding.Subject.DisplayName)
			if err != nil {
				return err
			}
			input.SubjectType = access.SubjectPrincipal
			input.SubjectID = principal.ID
		default:
			return fmt.Errorf("workspace role binding %q has unsupported subject kind %q", name, binding.Subject.Kind)
		}
		if _, err := r.CreateRoleBinding(ctx, input); err != nil {
			return err
		}
	}
	for _, name := range sortedWorkspaceGrantNames(policy.Grants) {
		grant := policy.Grants[name]
		subjectType, subjectID, err := r.policySubject(ctx, workspaceID, grant.Subject, groupIDs)
		if err != nil {
			return fmt.Errorf("workspace grant %q: %w", name, err)
		}
		if err := r.upsertGrantWithID(ctx, stableAccessID("grant", workspaceID, name), access.GrantInput{
			Object:      policyObjectRef(workspaceID, grant.Object),
			SubjectType: subjectType,
			SubjectID:   subjectID,
			Privilege:   access.Privilege(grant.Privilege),
		}); err != nil {
			return err
		}
	}
	for _, name := range sortedWorkspaceDataPolicyNames(policy.DataPolicies) {
		dataPolicy := policy.DataPolicies[name]
		var subjectType access.SubjectType
		var subjectID string
		if strings.TrimSpace(dataPolicy.Subject.Kind) != "" {
			var err error
			subjectType, subjectID, err = r.policySubject(ctx, workspaceID, dataPolicy.Subject, groupIDs)
			if err != nil {
				return fmt.Errorf("workspace data policy %q: %w", name, err)
			}
		}
		if _, err := r.UpsertDataPolicy(ctx, access.DataPolicyInput{
			ID:             stableAccessID("datapolicy", workspaceID, name),
			Object:         policyObjectRef(workspaceID, dataPolicy.Object),
			SubjectType:    subjectType,
			SubjectID:      subjectID,
			PolicyType:     dataPolicy.PolicyType,
			ExpressionJSON: dataPolicy.ExpressionJSON,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) policySubject(ctx context.Context, workspaceID string, subject workspace.WorkspaceRoleBindingSubject, groupIDs map[string]string) (access.SubjectType, string, error) {
	switch subject.Kind {
	case string(access.SubjectGroup):
		groupID := groupIDs[subject.Group]
		if groupID == "" {
			return "", "", fmt.Errorf("unknown group %q", subject.Group)
		}
		return access.SubjectGroup, groupID, nil
	case string(access.SubjectPrincipal):
		principal, err := r.policyPrincipal(ctx, subject.PrincipalID, subject.Email, subject.DisplayName)
		if err != nil {
			return "", "", err
		}
		return access.SubjectPrincipal, principal.ID, nil
	case string(access.SubjectServicePrincipal):
		id := strings.TrimSpace(subject.PrincipalID)
		if id == "" {
			return "", "", fmt.Errorf("service principal subject requires principalId")
		}
		principal, err := r.UpsertPrincipal(ctx, access.PrincipalInput{
			ID:          id,
			Kind:        access.PrincipalKindServicePrincipal,
			DisplayName: firstNonEmpty(strings.TrimSpace(subject.DisplayName), id),
		})
		if err != nil {
			return "", "", err
		}
		return access.SubjectServicePrincipal, principal.ID, nil
	default:
		return "", "", fmt.Errorf("unsupported subject kind %q in workspace %q", subject.Kind, workspaceID)
	}
}

func policyObjectRef(workspaceID string, object workspace.WorkspaceSecurableObjectRef) access.ObjectRef {
	typ := access.SecurableType(strings.TrimSpace(object.Type))
	objectID := strings.TrimSpace(object.ID)
	switch typ {
	case access.SecurableWorkspace:
		return access.WorkspaceObject(workspaceID)
	case access.SecurableDataset, access.SecurableTable:
		if modelID, _, ok := strings.Cut(objectID, "/"); ok && strings.TrimSpace(modelID) != "" {
			return access.ItemObjectWithParent(typ, workspaceID, objectID, access.ItemObject(access.SecurableSemanticModel, workspaceID, modelID))
		}
	case access.SecurableColumn:
		parts := strings.Split(objectID, "/")
		if len(parts) >= 3 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			parent := access.ItemObjectWithParent(access.SecurableDataset, workspaceID, parts[0]+"/"+parts[1], access.ItemObject(access.SecurableSemanticModel, workspaceID, parts[0]))
			return access.ItemObjectWithParent(typ, workspaceID, objectID, parent)
		}
	}
	return access.ItemObject(typ, workspaceID, objectID)
}

func (r *Repository) policyPrincipal(ctx context.Context, id, email, displayName string) (access.Principal, error) {
	email = access.NormalizeEmail(email)
	id = strings.TrimSpace(id)
	if id == "" && email != "" {
		id = access.PrincipalIDForEmail(email)
	}
	if id == "" {
		return access.Principal{}, fmt.Errorf("policy principal requires principalId or email")
	}
	return r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          id,
		Email:       email,
		DisplayName: firstNonEmpty(strings.TrimSpace(displayName), email, id),
	})
}

func (r *Repository) CreateSession(ctx context.Context, principalID string, ttl time.Duration) (string, error) {
	token := newSecret()
	fingerprint := secretFingerprint(token)
	verifier, err := newSecretVerifier(token)
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(ttl).UTC().Format(time.RFC3339)
	return token, r.q.CreateSession(ctx, platformdb.CreateSessionParams{
		ID:               newID("session"),
		PrincipalID:      principalID,
		TokenFingerprint: fingerprint,
		TokenVerifier:    verifier,
		ExpiresAt:        expires,
	})
}

func (r *Repository) PrincipalForToken(ctx context.Context, token string) (access.Principal, error) {
	session, err := r.sessionForToken(ctx, token)
	if err != nil {
		return access.Principal{}, err
	}
	row, err := r.q.GetPrincipal(ctx, session.PrincipalID)
	if err != nil {
		return access.Principal{}, err
	}
	principal := mapPrincipal(row)
	if principal.DisabledAt != "" {
		return access.Principal{}, sql.ErrNoRows
	}
	return principal, nil
}

func (r *Repository) DisabledPrincipalForSessionToken(ctx context.Context, token string) (string, string, error) {
	session, err := r.sessionForAuditToken(ctx, token)
	if err != nil {
		return "", "", err
	}
	row, err := r.q.GetPrincipal(ctx, session.PrincipalID)
	if err != nil {
		return "", "", err
	}
	principal := mapPrincipal(row)
	if principal.DisabledAt == "" {
		return "", "", sql.ErrNoRows
	}
	return principal.ID, session.ID, nil
}

func (r *Repository) sessionForAuditToken(ctx context.Context, token string) (platformdb.Session, error) {
	fingerprint := secretFingerprint(token)
	session, err := r.q.GetSessionByTokenFingerprintForAudit(ctx, fingerprint)
	if err != nil {
		return platformdb.Session{}, err
	}
	if !verifySecret(token, session.TokenVerifier) {
		return platformdb.Session{}, sql.ErrNoRows
	}
	return session, nil
}

func (r *Repository) sessionForToken(ctx context.Context, token string) (platformdb.Session, error) {
	fingerprint := secretFingerprint(token)
	session, err := r.q.GetSessionByTokenFingerprint(ctx, fingerprint)
	if err != nil {
		return platformdb.Session{}, err
	}
	if !verifySecret(token, session.TokenVerifier) {
		return platformdb.Session{}, sql.ErrNoRows
	}
	_ = r.q.TouchSession(ctx, session.ID)
	return session, nil
}

func (r *Repository) DeleteSession(ctx context.Context, token string) error {
	fingerprint := secretFingerprint(token)
	return r.q.DeleteSessionByTokenFingerprint(ctx, fingerprint)
}

func (r *Repository) ListSessions(ctx context.Context, principalID string) ([]access.Session, error) {
	rows, err := r.q.ListSessionsByPrincipal(ctx, principalID)
	if err != nil {
		return nil, err
	}
	sessions := make([]access.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, mapSession(row))
	}
	return sessions, nil
}

func (r *Repository) RevokeSession(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("session id is required")
	}
	return r.q.RevokeSession(ctx, id)
}

func (r *Repository) RevokeSessionForPrincipal(ctx context.Context, principalID, id string) error {
	if strings.TrimSpace(principalID) == "" || strings.TrimSpace(id) == "" {
		return fmt.Errorf("principal id and session id are required")
	}
	_, err := r.q.RevokeSessionForPrincipal(ctx, platformdb.RevokeSessionForPrincipalParams{
		PrincipalID: principalID,
		ID:          id,
	})
	return err
}

func (r *Repository) CreateAPIToken(ctx context.Context, principalID, name string) (string, error) {
	token, _, err := r.CreateAPITokenWithMetadata(ctx, access.APITokenInput{
		PrincipalID: principalID,
		Name:        name,
	})
	return token, err
}

func (r *Repository) CreateAPITokenWithMetadata(ctx context.Context, input access.APITokenInput) (string, access.APIToken, error) {
	if strings.TrimSpace(input.PrincipalID) == "" {
		return "", access.APIToken{}, fmt.Errorf("principal id is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return "", access.APIToken{}, fmt.Errorf("token name is required")
	}
	token := newSecret()
	id := newID("token")
	privilegesJSON, err := json.Marshal(input.Privileges)
	if err != nil {
		return "", access.APIToken{}, err
	}
	if input.ExpiresAt.IsZero() {
		input.ExpiresAt = time.Now().Add(defaultAPITokenTTL)
	}
	expiresAt := sql.NullString{}
	if !input.ExpiresAt.IsZero() {
		expiresAt = sql.NullString{String: input.ExpiresAt.UTC().Format(time.RFC3339), Valid: true}
	}
	fingerprint := secretFingerprint(token)
	verifier, err := newSecretVerifier(token)
	if err != nil {
		return "", access.APIToken{}, err
	}
	if err := r.q.CreateAPIToken(ctx, platformdb.CreateAPITokenParams{
		ID:               id,
		PrincipalID:      input.PrincipalID,
		WorkspaceID:      sql.NullString{String: input.WorkspaceID, Valid: strings.TrimSpace(input.WorkspaceID) != ""},
		Name:             input.Name,
		TokenFingerprint: fingerprint,
		TokenVerifier:    verifier,
		PrivilegesJson:   string(privilegesJSON),
		ExpiresAt:        expiresAt,
	}); err != nil {
		return "", access.APIToken{}, err
	}
	tokens, err := r.q.ListAPITokensByPrincipal(ctx, input.PrincipalID)
	if err != nil {
		return "", access.APIToken{}, err
	}
	for _, row := range tokens {
		if row.ID == id {
			return token, mapAPIToken(row), nil
		}
	}
	return token, access.APIToken{ID: id, PrincipalID: input.PrincipalID, WorkspaceID: input.WorkspaceID, Name: input.Name, Privileges: input.Privileges, ExpiresAt: nullString(expiresAt)}, nil
}

func (r *Repository) PrincipalForAPIToken(ctx context.Context, token string) (access.Principal, error) {
	credential, err := r.CredentialForAPIToken(ctx, token)
	if err != nil {
		return access.Principal{}, err
	}
	return credential.Principal, nil
}

func (r *Repository) CredentialForAPIToken(ctx context.Context, token string) (access.APICredential, error) {
	apiToken, err := r.apiTokenForSecret(ctx, token)
	if err != nil {
		return access.APICredential{}, err
	}
	row, err := r.q.GetPrincipal(ctx, apiToken.PrincipalID)
	if err != nil {
		return access.APICredential{}, err
	}
	principal := mapPrincipal(row)
	if principal.DisabledAt != "" {
		return access.APICredential{}, sql.ErrNoRows
	}
	return access.APICredential{
		Principal: principal,
		Token:     mapAPIToken(apiToken),
	}, nil
}

func (r *Repository) DisabledPrincipalForAPIToken(ctx context.Context, token string) (string, string, error) {
	apiToken, err := r.apiTokenForAuditSecret(ctx, token)
	if err != nil {
		return "", "", err
	}
	row, err := r.q.GetPrincipal(ctx, apiToken.PrincipalID)
	if err != nil {
		return "", "", err
	}
	principal := mapPrincipal(row)
	if principal.DisabledAt == "" {
		return "", "", sql.ErrNoRows
	}
	return principal.ID, apiToken.ID, nil
}

func (r *Repository) apiTokenForAuditSecret(ctx context.Context, token string) (platformdb.ApiToken, error) {
	fingerprint := secretFingerprint(token)
	apiToken, err := r.q.GetAPITokenByFingerprintForAudit(ctx, fingerprint)
	if err != nil {
		return platformdb.ApiToken{}, err
	}
	if !verifySecret(token, apiToken.TokenVerifier) {
		return platformdb.ApiToken{}, sql.ErrNoRows
	}
	return apiToken, nil
}

func (r *Repository) apiTokenForSecret(ctx context.Context, token string) (platformdb.ApiToken, error) {
	fingerprint := secretFingerprint(token)
	apiToken, err := r.q.GetAPITokenByFingerprint(ctx, fingerprint)
	if err != nil {
		return platformdb.ApiToken{}, err
	}
	if !verifySecret(token, apiToken.TokenVerifier) {
		return platformdb.ApiToken{}, sql.ErrNoRows
	}
	_ = r.q.TouchAPIToken(ctx, apiToken.ID)
	return apiToken, nil
}

func (r *Repository) ListAPITokens(ctx context.Context, principalID string) ([]access.APIToken, error) {
	rows, err := r.q.ListAPITokensByPrincipal(ctx, principalID)
	if err != nil {
		return nil, err
	}
	tokens := make([]access.APIToken, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, mapAPIToken(row))
	}
	return tokens, nil
}

func (r *Repository) RevokeAPIToken(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("api token id is required")
	}
	return r.q.RevokeAPIToken(ctx, id)
}

func (r *Repository) RevokeAPITokenForPrincipal(ctx context.Context, principalID, id string) error {
	if strings.TrimSpace(principalID) == "" || strings.TrimSpace(id) == "" {
		return fmt.Errorf("principal id and api token id are required")
	}
	_, err := r.q.RevokeAPITokenForPrincipal(ctx, platformdb.RevokeAPITokenForPrincipalParams{
		PrincipalID: principalID,
		ID:          id,
	})
	return err
}

func (r *Repository) CreateServicePrincipal(ctx context.Context, input access.ServicePrincipalInput) (access.Principal, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = newID("sp")
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = id
	}
	return r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          id,
		Kind:        access.PrincipalKindServicePrincipal,
		DisplayName: displayName,
	})
}

func (r *Repository) ListServicePrincipals(ctx context.Context) ([]access.Principal, error) {
	rows, err := r.q.ListServicePrincipals(ctx)
	if err != nil {
		return nil, err
	}
	principals := make([]access.Principal, 0, len(rows))
	for _, row := range rows {
		principals = append(principals, mapPrincipal(row))
	}
	return principals, nil
}

func (r *Repository) UpdateServicePrincipal(ctx context.Context, id string, input access.ServicePrincipalInput) (access.Principal, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return access.Principal{}, fmt.Errorf("service principal id is required")
	}
	existing, err := r.q.GetPrincipal(ctx, id)
	if err != nil {
		return access.Principal{}, err
	}
	if access.PrincipalKind(existing.Kind) != access.PrincipalKindServicePrincipal {
		return access.Principal{}, fmt.Errorf("principal %q is not a service principal", id)
	}
	displayName := firstNonEmpty(strings.TrimSpace(input.DisplayName), existing.DisplayName, id)
	return r.UpsertPrincipal(ctx, access.PrincipalInput{
		ID:          id,
		Kind:        access.PrincipalKindServicePrincipal,
		DisplayName: displayName,
	})
}

func (r *Repository) DeleteServicePrincipal(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("service principal id is required")
	}
	return r.q.DeleteServicePrincipal(ctx, id)
}

func (r *Repository) CreateServicePrincipalSecret(ctx context.Context, servicePrincipalID, name string) (string, access.ServicePrincipalSecret, error) {
	servicePrincipalID = strings.TrimSpace(servicePrincipalID)
	if servicePrincipalID == "" {
		return "", access.ServicePrincipalSecret{}, fmt.Errorf("service principal id is required")
	}
	principal, err := r.q.GetPrincipal(ctx, servicePrincipalID)
	if err != nil {
		return "", access.ServicePrincipalSecret{}, err
	}
	if access.PrincipalKind(principal.Kind) != access.PrincipalKindServicePrincipal {
		return "", access.ServicePrincipalSecret{}, fmt.Errorf("principal %q is not a service principal", servicePrincipalID)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	secret := newSecret()
	fingerprint := secretFingerprint(secret)
	verifier, err := newSecretVerifier(secret)
	if err != nil {
		return "", access.ServicePrincipalSecret{}, err
	}
	row := access.ServicePrincipalSecret{
		ID:                 newID("spsecret"),
		ServicePrincipalID: servicePrincipalID,
		Name:               name,
	}
	if err := r.q.CreateServicePrincipalSecret(ctx, platformdb.CreateServicePrincipalSecretParams{
		ID:                 row.ID,
		ServicePrincipalID: row.ServicePrincipalID,
		Name:               row.Name,
		SecretFingerprint:  fingerprint,
		SecretVerifier:     verifier,
	}); err != nil {
		return "", access.ServicePrincipalSecret{}, err
	}
	return secret, row, nil
}

func (r *Repository) RevokeServicePrincipalSecret(ctx context.Context, servicePrincipalID, secretID string) error {
	if strings.TrimSpace(servicePrincipalID) == "" || strings.TrimSpace(secretID) == "" {
		return fmt.Errorf("service principal id and secret id are required")
	}
	return r.q.RevokeServicePrincipalSecret(ctx, platformdb.RevokeServicePrincipalSecretParams{
		ServicePrincipalID: servicePrincipalID,
		ID:                 secretID,
	})
}

func (r *Repository) PrincipalForServicePrincipalSecret(ctx context.Context, servicePrincipalID, secret string) (access.Principal, error) {
	row, err := r.servicePrincipalSecretForSecret(ctx, servicePrincipalID, secret)
	if err != nil {
		return access.Principal{}, err
	}
	principal, err := r.q.GetPrincipal(ctx, row.ServicePrincipalID)
	if err != nil {
		return access.Principal{}, err
	}
	mapped := mapPrincipal(principal)
	if mapped.DisabledAt != "" {
		return access.Principal{}, sql.ErrNoRows
	}
	return mapped, nil
}

func (r *Repository) servicePrincipalSecretForSecret(ctx context.Context, servicePrincipalID, secret string) (platformdb.ServicePrincipalSecret, error) {
	servicePrincipalID = strings.TrimSpace(servicePrincipalID)
	fingerprint := secretFingerprint(secret)
	row, err := r.q.GetServicePrincipalSecretByFingerprint(ctx, platformdb.GetServicePrincipalSecretByFingerprintParams{
		ServicePrincipalID: servicePrincipalID,
		SecretFingerprint:  fingerprint,
	})
	if err != nil {
		return platformdb.ServicePrincipalSecret{}, err
	}
	if !verifySecret(secret, row.SecretVerifier) {
		return platformdb.ServicePrincipalSecret{}, sql.ErrNoRows
	}
	return row, nil
}

func (r *Repository) RecordAuditEvent(ctx context.Context, input access.AuditEventInput) error {
	if strings.TrimSpace(input.Action) == "" {
		return fmt.Errorf("audit action is required")
	}
	if strings.TrimSpace(input.MetadataJSON) == "" {
		input.MetadataJSON = "{}"
	}
	return r.q.InsertAuditEvent(ctx, platformdb.InsertAuditEventParams{
		ID:            newID("audit"),
		WorkspaceID:   sql.NullString{String: input.WorkspaceID, Valid: strings.TrimSpace(input.WorkspaceID) != ""},
		PrincipalID:   sql.NullString{String: input.PrincipalID, Valid: strings.TrimSpace(input.PrincipalID) != ""},
		Action:        input.Action,
		TargetType:    input.TargetType,
		TargetID:      input.TargetID,
		Privilege:     string(input.Privilege),
		Status:        input.Status,
		RequestID:     input.RequestID,
		CorrelationID: input.CorrelationID,
		MetadataJson:  input.MetadataJSON,
	})
}

func (r *Repository) ListAuditEvents(ctx context.Context, filter access.AuditEventFilter) ([]access.AuditEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if filter.PageToken != "" && filter.CursorTime == "" && filter.CursorID == "" {
		filter.CursorTime, filter.CursorID = decodeAuditPageToken(filter.PageToken)
	}
	from := sqliteAuditTime(filter.From)
	to := sqliteAuditTime(filter.To)
	cursorTime := sqliteAuditTime(filter.CursorTime)
	rows, err := r.q.ListAuditEvents(ctx, platformdb.ListAuditEventsParams{
		Column1:     filter.WorkspaceID,
		WorkspaceID: sql.NullString{String: filter.WorkspaceID, Valid: strings.TrimSpace(filter.WorkspaceID) != ""},
		Column3:     filter.PrincipalID,
		PrincipalID: sql.NullString{String: filter.PrincipalID, Valid: strings.TrimSpace(filter.PrincipalID) != ""},
		Column5:     filter.Action,
		Action:      filter.Action,
		Column7:     filter.TargetType,
		TargetType:  filter.TargetType,
		Column9:     filter.TargetID,
		TargetID:    filter.TargetID,
		Column11:    from,
		CreatedAt:   from,
		Column13:    to,
		CreatedAt_2: to,
		Column15:    cursorTime,
		CreatedAt_3: cursorTime,
		CreatedAt_4: cursorTime,
		ID:          filter.CursorID,
		Limit:       int64(limit),
	})
	if err != nil {
		return nil, err
	}
	events := make([]access.AuditEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, access.AuditEvent{
			ID:            row.ID,
			WorkspaceID:   nullString(row.WorkspaceID),
			PrincipalID:   nullString(row.PrincipalID),
			Action:        row.Action,
			TargetType:    row.TargetType,
			TargetID:      row.TargetID,
			Privilege:     access.Privilege(row.Privilege),
			Status:        row.Status,
			RequestID:     row.RequestID,
			CorrelationID: row.CorrelationID,
			MetadataJSON:  row.MetadataJson,
			CreatedAt:     row.CreatedAt,
		})
	}
	return events, nil
}

func auditPageToken(createdAt, id string) string {
	if createdAt == "" || id == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "\x00" + id))
}

func sqliteAuditTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC().Format("2006-01-02 15:04:05")
		}
	}
	return value
}

func decodeAuditPageToken(token string) (string, string) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", ""
	}
	createdAt, id, ok := strings.Cut(string(raw), "\x00")
	if !ok {
		return "", ""
	}
	return createdAt, id
}

func (r *Repository) roleBindingParts(ctx context.Context, input access.RoleBindingInput) (platformdb.Role, sql.NullString, sql.NullString, error) {
	if strings.TrimSpace(input.Role) == "" {
		return platformdb.Role{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("role is required")
	}
	role, err := r.q.GetRoleByName(ctx, input.Role)
	if err != nil {
		return platformdb.Role{}, sql.NullString{}, sql.NullString{}, err
	}
	subjectID := strings.TrimSpace(input.SubjectID)
	if subjectID == "" {
		return platformdb.Role{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("subject id is required")
	}
	switch input.SubjectType {
	case access.SubjectPrincipal:
		return role, sql.NullString{String: subjectID, Valid: true}, sql.NullString{}, nil
	case access.SubjectServicePrincipal:
		principal, err := r.q.GetPrincipal(ctx, subjectID)
		if err != nil {
			return platformdb.Role{}, sql.NullString{}, sql.NullString{}, err
		}
		if access.PrincipalKind(principal.Kind) != access.PrincipalKindServicePrincipal {
			return platformdb.Role{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("principal %q is not a service principal", subjectID)
		}
		return role, sql.NullString{String: subjectID, Valid: true}, sql.NullString{}, nil
	case access.SubjectGroup:
		return role, sql.NullString{}, sql.NullString{String: subjectID, Valid: true}, nil
	default:
		return platformdb.Role{}, sql.NullString{}, sql.NullString{}, fmt.Errorf("unsupported subject type %q", input.SubjectType)
	}
}

func mapPrincipal(row platformdb.Principal) access.Principal {
	return access.Principal{
		ID:          row.ID,
		Kind:        access.PrincipalKind(row.Kind),
		Email:       row.Email,
		DisplayName: row.DisplayName,
		DisabledAt:  nullString(row.DisabledAt),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func mapGroup(row platformdb.Group) access.Group {
	return access.Group{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		Provider:    row.Provider,
		ExternalID:  row.ExternalID,
		Name:        row.Name,
		CreatedAt:   row.CreatedAt,
	}
}

func mapRoleBinding(row platformdb.GetRoleBindingByIDRow) access.RoleBinding {
	return access.RoleBinding{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		SubjectType: access.SubjectType(row.SubjectType),
		SubjectID:   row.SubjectID,
		PrincipalID: nullString(row.PrincipalID),
		GroupID:     nullString(row.GroupID),
		Email:       nullString(row.Email),
		DisplayName: nullString(row.DisplayName),
		GroupName:   nullString(row.GroupName),
		Role:        row.RoleName,
		CreatedAt:   row.CreatedAt,
	}
}

func mapListedRoleBinding(row platformdb.ListRoleBindingsByWorkspaceRow) access.RoleBinding {
	return access.RoleBinding{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		SubjectType: access.SubjectType(row.SubjectType),
		SubjectID:   row.SubjectID,
		PrincipalID: nullString(row.PrincipalID),
		GroupID:     nullString(row.GroupID),
		Email:       nullString(row.Email),
		DisplayName: nullString(row.DisplayName),
		GroupName:   nullString(row.GroupName),
		Role:        row.RoleName,
		CreatedAt:   row.CreatedAt,
	}
}

func mapSession(row platformdb.Session) access.Session {
	return access.Session{
		ID:          row.ID,
		PrincipalID: row.PrincipalID,
		ExpiresAt:   row.ExpiresAt,
		CreatedAt:   row.CreatedAt,
		LastSeenAt:  row.LastSeenAt,
		RevokedAt:   nullString(row.RevokedAt),
	}
}

func mapAPIToken(row platformdb.ApiToken) access.APIToken {
	var privileges []access.Privilege
	_ = json.Unmarshal([]byte(row.PrivilegesJson), &privileges)
	return access.APIToken{
		ID:          row.ID,
		PrincipalID: row.PrincipalID,
		WorkspaceID: nullString(row.WorkspaceID),
		Name:        row.Name,
		Privileges:  privileges,
		ExpiresAt:   nullString(row.ExpiresAt),
		CreatedAt:   row.CreatedAt,
		LastUsedAt:  nullString(row.LastUsedAt),
		RevokedAt:   nullString(row.RevokedAt),
	}
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stableAccessID(prefix, workspaceID, name string) string {
	return "cac_" + prefix + "_" + stableID(workspaceID+"|"+name)
}

func sortedWorkspaceGroupNames(values map[string]workspace.WorkspaceGroup) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedWorkspaceRoleBindingNames(values map[string]workspace.WorkspaceRoleBinding) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedWorkspaceGrantNames(values map[string]workspace.WorkspaceGrant) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedWorkspaceDataPolicyNames(values map[string]workspace.WorkspaceDataPolicy) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func newID(prefix string) string {
	return prefix + "_" + newSecret()[:24]
}

func newSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		sum := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano)))
		return hex.EncodeToString(sum[:])
	}
	return hex.EncodeToString(b[:])
}

func stableID(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(value)))
	return hex.EncodeToString(sum[:])[:32]
}
