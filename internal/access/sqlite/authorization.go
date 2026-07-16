package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
)

func (r *Repository) Authorize(ctx context.Context, principalID string, privilege access.Privilege, object access.ObjectRef) (access.AuthorizationDecision, error) {
	decisions, err := r.AuthorizeBatch(ctx, principalID, []access.AuthorizationCheck{{Privilege: privilege, Object: object}})
	if err != nil {
		return access.AuthorizationDecision{Privilege: privilege, Object: object}, err
	}
	if len(decisions) == 0 {
		return access.AuthorizationDecision{Privilege: privilege, Object: object, Reason: access.ReasonMissingObject}, nil
	}
	return decisions[0], nil
}

func (r *Repository) AuthorizeAny(ctx context.Context, principalID string, privilege access.Privilege, objects []access.ObjectRef) (access.AuthorizationDecision, error) {
	if len(objects) == 0 {
		return access.AuthorizationDecision{Privilege: privilege, Reason: access.ReasonMissingObject}, nil
	}
	checks := make([]access.AuthorizationCheck, 0, len(objects))
	for _, object := range objects {
		checks = append(checks, access.AuthorizationCheck{Privilege: privilege, Object: object})
	}
	decisions, err := r.AuthorizeBatch(ctx, principalID, checks)
	if err != nil {
		return access.AuthorizationDecision{Privilege: privilege}, err
	}
	var last access.AuthorizationDecision
	for _, decision := range decisions {
		if decision.Allowed {
			return decision, nil
		}
		last = decision
	}
	return last, nil
}

func (r *Repository) AuthorizeBatch(ctx context.Context, principalID string, checks []access.AuthorizationCheck) ([]access.AuthorizationDecision, error) {
	out := make([]access.AuthorizationDecision, len(checks))
	principalID = strings.TrimSpace(principalID)
	pending := make([]int, 0, len(checks))
	for i, check := range checks {
		decision := access.AuthorizationDecision{Privilege: check.Privilege, Object: check.Object}
		if principalID == "" {
			decision.Reason = access.ReasonMissingPrincipal
			out[i] = decision
			continue
		}
		if strings.TrimSpace(string(check.Privilege)) == "" {
			decision.Reason = access.ReasonMissingPrivilege
			out[i] = decision
			continue
		}
		if cached, ok := access.CachedAuthorizationDecision(ctx, principalID, check.Privilege, check.Object); ok {
			out[i] = cached
			continue
		}
		out[i] = decision
		pending = append(pending, i)
	}
	if len(pending) == 0 {
		return out, nil
	}
	disabled, err := r.principalDisabled(ctx, principalID)
	if err != nil {
		return nil, err
	}
	if disabled {
		for _, i := range pending {
			out[i].Reason = access.ReasonPrincipalDisabled
			access.StoreAuthorizationDecision(ctx, principalID, out[i])
		}
		return out, nil
	}
	platformDecision, err := r.authorizeByGrant(ctx, principalID, access.PrivilegeManagePlatform, []string{access.PlatformObject().CanonicalID()})
	if err != nil {
		return nil, err
	}
	type objectFacts struct {
		objectID string
		exists   bool
		owner    string
		ancestry []string
	}
	factsByObject := map[string]objectFacts{}
	loadObjectFacts := func(object access.ObjectRef) (objectFacts, error) {
		objectKey := object.CanonicalID()
		if facts, ok := factsByObject[objectKey]; ok {
			return facts, nil
		}
		objectID, exists, err := r.lookupSecurableObjectID(ctx, object)
		if err != nil {
			return objectFacts{}, err
		}
		facts := objectFacts{objectID: objectID, exists: exists}
		if exists {
			owner, err := r.objectOwner(ctx, objectID)
			if err != nil {
				return objectFacts{}, err
			}
			ancestry, err := r.objectAncestry(ctx, objectID)
			if err != nil {
				return objectFacts{}, err
			}
			facts.owner = owner
			facts.ancestry = ancestry
		}
		factsByObject[objectKey] = facts
		return facts, nil
	}
	for _, i := range pending {
		check := checks[i]
		facts, err := loadObjectFacts(check.Object)
		if err != nil {
			return nil, err
		}
		if platformDecision.Allowed {
			out[i].Allowed = true
			out[i].Platform = true
			out[i].GrantID = platformDecision.GrantID
			out[i].GrantObjectID = platformDecision.GrantObjectID
			out[i].SubjectType = platformDecision.SubjectType
			out[i].SubjectID = platformDecision.SubjectID
			out[i].Reason = access.ReasonPlatformAdmin
			access.StoreAuthorizationDecision(ctx, principalID, out[i])
			continue
		}
		if !facts.exists {
			out[i].Reason = access.ReasonUnknownObject
			access.StoreAuthorizationDecision(ctx, principalID, out[i])
			continue
		}
		if facts.owner != "" && facts.owner == principalID {
			out[i].Allowed = true
			out[i].Owner = true
			out[i].Reason = access.ReasonOwner
			access.StoreAuthorizationDecision(ctx, principalID, out[i])
			continue
		}
		grantDecision, err := r.authorizeByGrant(ctx, principalID, check.Privilege, facts.ancestry)
		if err != nil {
			return nil, err
		}
		if grantDecision.Allowed {
			grantDecision.Privilege = check.Privilege
			grantDecision.Object = check.Object
			out[i] = grantDecision
		} else {
			out[i].Reason = access.ReasonNoGrant
		}
		access.StoreAuthorizationDecision(ctx, principalID, out[i])
	}
	return out, nil
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
	privileges := knownPrivileges()
	checks := make([]access.AuthorizationCheck, 0, len(privileges))
	for _, privilege := range privileges {
		checks = append(checks, access.AuthorizationCheck{Privilege: privilege, Object: object})
	}
	decisions, err := r.AuthorizeBatch(ctx, principalID, checks)
	if err != nil {
		return nil, err
	}
	for _, decision := range decisions {
		if decision.Allowed {
			out = append(out, decision)
		}
	}
	return out, nil
}

func (r *Repository) UpsertSecurableObject(ctx context.Context, object access.ObjectRef, ownerPrincipalID string) (access.SecurableObject, error) {
	access.ClearAuthorizationCache(ctx)
	objectID, err := r.ensureSecurableObject(ctx, object)
	if err != nil {
		return access.SecurableObject{}, err
	}
	ownerPrincipalID = strings.TrimSpace(ownerPrincipalID)
	if ownerPrincipalID != "" {
		if err := r.q.InitializeSecurableObjectOwner(ctx, platformdb.InitializeSecurableObjectOwnerParams{
			OwnerPrincipalID: ownerPrincipalID, ID: objectID,
		}); err != nil {
			return access.SecurableObject{}, err
		}
	}
	return r.securableObjectByID(ctx, objectID)
}

func (r *Repository) CreateGrant(ctx context.Context, input access.GrantInput) (access.Grant, error) {
	access.ClearAuthorizationCache(ctx)
	id, err := newID("grant")
	if err != nil {
		return access.Grant{}, err
	}
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
	row, err := r.q.GetScopedGrant(ctx, platformdb.GetScopedGrantParams{
		ID: id, WorkspaceID: workspaceID, WorkspaceObjectID: access.WorkspaceObject(workspaceID).CanonicalID(),
	})
	if err != nil {
		return access.Grant{}, err
	}
	return access.Grant{
		ID: row.ID, ObjectID: row.ObjectID, ObjectType: access.SecurableType(row.ObjectType), WorkspaceID: row.WorkspaceID,
		SubjectType: access.SubjectType(row.SubjectType), SubjectID: row.SubjectID,
		Privilege: access.Privilege(row.Privilege), CreatedAt: row.CreatedAt,
	}, nil
}

func (r *Repository) DeleteGrant(ctx context.Context, workspaceID, id string) error {
	access.ClearAuthorizationCache(ctx)
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("grant id is required")
	}
	return r.q.DeleteScopedGrant(ctx, platformdb.DeleteScopedGrantParams{
		ID: id, WorkspaceID: workspaceID, WorkspaceObjectID: access.WorkspaceObject(workspaceID).CanonicalID(),
	})
}

func (r *Repository) UpsertDataPolicy(ctx context.Context, input access.DataPolicyInput) (access.DataPolicy, error) {
	access.ClearAuthorizationCache(ctx)
	if strings.TrimSpace(input.ID) == "" {
		id, err := newID("datapolicy")
		if err != nil {
			return access.DataPolicy{}, err
		}
		input.ID = id
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
	err = r.q.UpsertDataPolicy(ctx, platformdb.UpsertDataPolicyParams{
		ID: input.ID, WorkspaceID: input.Object.WorkspaceID, ObjectID: objectID,
		SubjectType: string(input.SubjectType), SubjectID: strings.TrimSpace(input.SubjectID),
		PolicyType: input.PolicyType, ExpressionJson: input.ExpressionJSON,
	})
	if err != nil {
		return access.DataPolicy{}, err
	}
	return r.GetDataPolicy(ctx, input.Object.WorkspaceID, input.ID)
}

func (r *Repository) GetDataPolicy(ctx context.Context, workspaceID, id string) (access.DataPolicy, error) {
	row, err := r.q.GetDataPolicy(ctx, platformdb.GetDataPolicyParams{ID: id, WorkspaceID: workspaceID})
	if err != nil {
		return access.DataPolicy{}, err
	}
	return access.DataPolicy{
		ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectID: row.ObjectID, SubjectType: access.SubjectType(row.SubjectType),
		SubjectID: row.SubjectID, PolicyType: row.PolicyType, ExpressionJSON: row.ExpressionJson,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *Repository) ListDataPolicies(ctx context.Context, object access.ObjectRef) ([]access.DataPolicy, error) {
	return r.ListDataPoliciesWithOptions(ctx, object, false)
}

func (r *Repository) ListDataPoliciesWithOptions(ctx context.Context, object access.ObjectRef, includeInherited bool) ([]access.DataPolicy, error) {
	objectID, exists, err := r.lookupSecurableObjectID(ctx, object)
	if err != nil || !exists {
		return []access.DataPolicy{}, err
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
		exists, err := r.q.GroupMemberExists(ctx, platformdb.GroupMemberExistsParams{GroupID: policy.SubjectID, PrincipalID: principalID})
		return exists != 0, err
	default:
		return false, fmt.Errorf("unsupported data policy subject type %q", policy.SubjectType)
	}
}

func (r *Repository) DeleteDataPolicy(ctx context.Context, workspaceID, id string) error {
	access.ClearAuthorizationCache(ctx)
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("data policy id is required")
	}
	return r.q.DeleteDataPolicy(ctx, platformdb.DeleteDataPolicyParams{WorkspaceID: workspaceID, ID: id})
}

func (r *Repository) SetObjectOwner(ctx context.Context, object access.ObjectRef, ownerPrincipalID string) (access.SecurableObject, error) {
	access.ClearAuthorizationCache(ctx)
	ownerPrincipalID = strings.TrimSpace(ownerPrincipalID)
	if ownerPrincipalID == "" {
		return access.SecurableObject{}, fmt.Errorf("owner principal id is required")
	}
	objectID, err := r.ensureSecurableObject(ctx, object)
	if err != nil {
		return access.SecurableObject{}, err
	}
	if err := r.q.SetSecurableObjectOwner(ctx, platformdb.SetSecurableObjectOwnerParams{OwnerPrincipalID: ownerPrincipalID, ID: objectID}); err != nil {
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
	objectID, exists, err := r.lookupSecurableObjectID(ctx, object)
	if err != nil || !exists {
		return []access.GrantView{}, err
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
	decision.Reason = access.ReasonGrant
	return decision, nil
}

func (r *Repository) lookupSecurableObjectID(ctx context.Context, object access.ObjectRef) (string, bool, error) {
	objectID := object.CanonicalID()
	if strings.TrimSpace(objectID) == "" {
		return "", false, fmt.Errorf("securable object id is required")
	}
	found, err := r.q.GetSecurableObjectID(ctx, objectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return found, true, nil
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
	err := r.q.UpsertSecurableObject(ctx, platformdb.UpsertSecurableObjectParams{
		ID: objectID, ObjectType: string(object.Type), WorkspaceID: object.WorkspaceID,
		ParentID: parentID, DisplayName: objectDisplayName(object),
	})
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
		parentID, err := r.q.GetSecurableObjectParent(ctx, current)
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
	if strings.TrimSpace(object.DisplayName) != "" {
		return strings.TrimSpace(object.DisplayName)
	}
	if object.ObjectID != "" {
		return object.ObjectID
	}
	if object.WorkspaceID != "" {
		return object.WorkspaceID
	}
	return string(object.Type)
}

func (r *Repository) objectOwner(ctx context.Context, objectID string) (string, error) {
	owner, err := r.q.GetSecurableObjectOwner(ctx, objectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return owner, err
}

func (r *Repository) securableObjectByID(ctx context.Context, objectID string) (access.SecurableObject, error) {
	row, err := r.q.GetSecurableObject(ctx, objectID)
	if err != nil {
		return access.SecurableObject{}, err
	}
	return access.SecurableObject{
		ID: row.ID, Type: access.SecurableType(row.ObjectType), WorkspaceID: row.WorkspaceID,
		ParentID: row.ParentID, OwnerPrincipalID: row.OwnerPrincipalID, DisplayName: row.DisplayName,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
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
	return r.q.DeleteRoleBindingGrants(ctx, "grant_"+bindingID+"_%")
}

func roleBindingGrantID(bindingID string, privilege access.Privilege) string {
	return "grant_" + bindingID + "_" + strings.ToLower(string(privilege))
}

func (r *Repository) rolePrivileges(ctx context.Context, roleName string) ([]access.Privilege, error) {
	rows, err := r.q.ListRolePrivileges(ctx, roleName)
	if err != nil {
		return nil, err
	}
	privileges := make([]access.Privilege, 0, len(rows))
	for _, value := range rows {
		privileges = append(privileges, access.Privilege(value))
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
	return r.q.UpsertGrant(ctx, platformdb.UpsertGrantParams{
		ID: id, ObjectID: objectID, SubjectType: string(input.SubjectType), SubjectID: subjectID, Privilege: string(input.Privilege),
	})
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
		access.PrivilegeActivateDeployment,
		access.PrivilegeUseAgent,
		access.PrivilegeViewAgent,
		access.PrivilegeManageGrants,
		access.PrivilegeViewAudit,
		access.PrivilegeManageWorkspace,
		access.PrivilegeManagePlatform,
	}
}
