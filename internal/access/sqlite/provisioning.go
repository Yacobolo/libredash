package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Yacobolo/leapview/internal/access"
	platformdb "github.com/Yacobolo/leapview/internal/platform/db"
	"github.com/Yacobolo/leapview/internal/workspace"
	"strings"
)

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
	access.ClearAuthorizationCache(ctx)
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
	access.ClearAuthorizationCache(ctx)
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
	if err := r.q.EnablePrincipal(ctx, principal.ID); err != nil {
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
	access.ClearAuthorizationCache(ctx)
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
	access.ClearAuthorizationCache(ctx)
	if strings.TrimSpace(input.WorkspaceID) == "" {
		return access.Group{}, fmt.Errorf("workspace id is required")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return access.Group{}, fmt.Errorf("group name is required")
	}
	if strings.TrimSpace(input.ID) == "" {
		id, err := newID("group")
		if err != nil {
			return access.Group{}, err
		}
		input.ID = id
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
	rows, err := r.q.ListAllGroups(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]access.Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, mapGroup(row))
	}
	return groups, nil
}

func (r *Repository) DeleteGroup(ctx context.Context, workspaceID, groupID string) error {
	access.ClearAuthorizationCache(ctx)
	if strings.TrimSpace(groupID) == "" {
		return fmt.Errorf("group id is required")
	}
	return r.q.DeleteGroup(ctx, platformdb.DeleteGroupParams{
		WorkspaceID: workspaceID,
		ID:          groupID,
	})
}

func (r *Repository) AddGroupMember(ctx context.Context, workspaceID, groupID, principalID string) error {
	access.ClearAuthorizationCache(ctx)
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
	access.ClearAuthorizationCache(ctx)
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
	rows, err := r.q.ListGroupMembersByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	members := make([]access.GroupMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, access.GroupMember{
			GroupID: row.GroupID, WorkspaceID: row.WorkspaceID, PrincipalID: row.PrincipalID,
			Email: row.Email, DisplayName: row.DisplayName, CreatedAt: row.CreatedAt,
		})
	}
	return members, nil
}

func (r *Repository) UpsertSCIMGroup(ctx context.Context, input access.SCIMGroupInput) (access.Group, error) {
	access.ClearAuthorizationCache(ctx)
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
	access.ClearAuthorizationCache(ctx)
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
	access.ClearAuthorizationCache(ctx)
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
	access.ClearAuthorizationCache(ctx)
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
	if err := r.q.DeleteWorkspaceGrants(ctx, platformdb.DeleteWorkspaceGrantsParams{
		WorkspaceID: workspaceID, WorkspaceObjectID: access.WorkspaceObject(workspaceID).CanonicalID(),
	}); err != nil {
		return err
	}
	if err := r.q.DeleteWorkspaceDataPolicies(ctx, workspaceID); err != nil {
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
	case access.SecurableSemanticField:
		if modelID, _, ok := strings.Cut(objectID, "/"); ok && strings.TrimSpace(modelID) != "" {
			return access.ItemObjectWithParent(typ, workspaceID, objectID, access.ItemObject(access.SecurableSemanticModel, workspaceID, modelID))
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
