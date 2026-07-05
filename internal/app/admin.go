package app

import (
	"net/http"
	"sort"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func (s *Server) currentAdminRoleLabel(r *http.Request) string {
	if s.auth == nil {
		return "Local platform"
	}
	principal, ok := s.auth.Principal(r)
	if ok && principal.DevBypass {
		return "Platform admin"
	}
	return "Platform access"
}

func (s *Server) adminData(r *http.Request) (ui.AdminData, error) {
	data := ui.AdminData{
		Workspace:       workspace.WorkspaceView{ID: "platform", Title: "Platform"},
		CSRFToken:       csrfToken(r, s.auth),
		AuthConfigured:  s.auth != nil,
		RBACConfigured:  s.store != nil,
		RBACStatusLabel: "Configured",
	}
	var err error
	data.Agent, err = s.adminAgentData(r)
	if err != nil {
		return data, err
	}
	repo, err := s.accessRepository()
	if err != nil {
		return data, err
	}
	if repo == nil {
		data.RBACConfigured = false
		data.RBACStatusLabel = "RBAC store is not configured"
		data.RoleCount = len(defaultWorkspaceRoles())
		data.Storage = s.storageReadModel().Data(r.Context())
		return data, nil
	}
	principals, err := s.adminPrincipalsData(r)
	if err != nil {
		return data, err
	}
	groups, err := s.adminGroupsData(r)
	if err != nil {
		return data, err
	}
	bindings, roles, err := s.adminRoleBindingsAndRoles(r)
	if err != nil {
		return data, err
	}
	membersByGroup := map[string][]ui.AdminPrincipalRef{}
	groupsByID := map[string]access.Group{}
	for _, group := range groups {
		groupsByID[group.ID] = group
		members := s.adminGroupMembersData(r, group.ID)
		for _, member := range members {
			membersByGroup[group.ID] = append(membersByGroup[group.ID], ui.AdminPrincipalRef{
				ID:          member.ID,
				Email:       member.Email,
				DisplayName: member.DisplayName,
			})
		}
	}
	data.RoleCount = len(roles)
	data.BindingCount = len(bindings)
	data.Principals = buildAdminPrincipals(principals, bindings, groupsByID, membersByGroup)
	data.Groups = buildAdminGroups(groups, bindings, membersByGroup)
	data.Storage = s.storageReadModel().Data(r.Context())
	data.QueryHistory = s.adminHTTPHandler().QueryHistoryData(r, uisignals.AdminQueryHistoryFilters{}, "", 50)
	data.PrincipalCount = len(data.Principals)
	data.GroupCount = len(data.Groups)
	return data, nil
}

func (s *Server) adminAgentData(r *http.Request) (ui.AdminAgentData, error) {
	details, err := s.agentHTTPHandler().AdminDetails(r.Context())
	if err != nil {
		return ui.AdminAgentData{}, err
	}
	data := ui.AdminAgentData{
		Enabled:      details.Enabled,
		Model:        details.Model,
		SystemPrompt: details.SystemPrompt,
		CSRFToken:    csrfToken(r, s.auth),
		UpdatePath:   "/api/v1/admin/agent/config",
		CanWrite:     true,
	}
	for _, tool := range details.Tools {
		data.Tools = append(data.Tools, ui.AdminAgentTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	if s.auth == nil {
		return data, nil
	}
	principal, ok := s.auth.Principal(r)
	if !ok || principal.DevBypass {
		return data, nil
	}
	repo, err := s.accessRepository()
	if err != nil || repo == nil {
		return data, err
	}
	allowed, err := repo.HasPermission(r.Context(), s.defaultWorkspaceID, principal.ID, access.PermissionRBACWrite)
	if err != nil {
		return data, err
	}
	data.CanWrite = allowed
	return data, nil
}

func (s *Server) adminGroupsData(r *http.Request) ([]access.Group, error) {
	repo, err := s.accessRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, nil
	}
	return repo.ListAllGroups(r.Context())
}

func (s *Server) adminGroupMembersData(r *http.Request, groupID string) []ui.AdminPrincipalRef {
	repo, err := s.accessRepository()
	if err != nil || repo == nil {
		return nil
	}
	rows, err := repo.ListGroupMembersByGroup(r.Context(), groupID)
	if err != nil {
		return nil
	}
	members := make([]ui.AdminPrincipalRef, 0, len(rows))
	for _, row := range rows {
		members = append(members, ui.AdminPrincipalRef{
			ID:          row.PrincipalID,
			Email:       row.Email,
			DisplayName: row.DisplayName,
		})
	}
	return members
}

func (s *Server) adminRoleBindingsAndRoles(r *http.Request) ([]workspace.RoleBindingView, []workspace.RoleView, error) {
	repo, err := s.accessRepository()
	if err != nil {
		return nil, nil, err
	}
	if repo == nil {
		return nil, defaultWorkspaceRoles(), nil
	}
	roleRows, err := repo.ListRoles(r.Context())
	if err != nil {
		return nil, nil, err
	}
	bindings, err := s.adminRoleBindingsData(r)
	if err != nil {
		return nil, nil, err
	}
	roles := make([]workspace.RoleView, 0, len(roleRows))
	for _, row := range roleRows {
		roles = append(roles, roleView(row))
	}
	return bindings, roles, nil
}

func (s *Server) adminRoleBindingsData(r *http.Request) ([]workspace.RoleBindingView, error) {
	repo, err := s.accessRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, nil
	}
	rows, err := repo.ListAllRoleBindings(r.Context())
	if err != nil {
		return nil, err
	}
	bindings := make([]workspace.RoleBindingView, 0, len(rows))
	for _, row := range rows {
		bindings = append(bindings, roleBindingView(row))
	}
	return bindings, nil
}

func (s *Server) adminPrincipalsData(r *http.Request) ([]ui.AdminPrincipal, error) {
	repo, err := s.accessRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return []ui.AdminPrincipal{}, nil
	}
	rows, err := repo.ListPrincipals(r.Context(), access.PrincipalFilter{
		Email: r.URL.Query().Get("email"),
		Query: r.URL.Query().Get("q"),
	})
	if err != nil {
		return nil, err
	}
	principals := make([]ui.AdminPrincipal, 0, len(rows))
	for _, row := range rows {
		principals = append(principals, ui.AdminPrincipal{
			ID:          row.ID,
			Email:       row.Email,
			DisplayName: row.DisplayName,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		})
	}
	sort.SliceStable(principals, func(i, j int) bool {
		return adminPrincipalSortKey(principals[i]) < adminPrincipalSortKey(principals[j])
	})
	return principals, nil
}

func buildAdminPrincipals(principals []ui.AdminPrincipal, bindings []workspace.RoleBindingView, groupsByID map[string]access.Group, membersByGroup map[string][]ui.AdminPrincipalRef) []ui.AdminPrincipal {
	byID := make(map[string]*ui.AdminPrincipal, len(principals))
	out := make([]ui.AdminPrincipal, 0, len(principals))
	for _, principal := range principals {
		row := principal
		byID[row.ID] = &row
		out = append(out, row)
	}
	for _, binding := range bindings {
		if binding.SubjectType == string(access.SubjectPrincipal) && binding.PrincipalID != "" {
			if principal := byID[binding.PrincipalID]; principal != nil {
				principal.DirectRoles = appendUnique(principal.DirectRoles, binding.Role)
			}
		}
	}
	for groupID, members := range membersByGroup {
		group := groupsByID[groupID]
		for _, member := range members {
			if principal := byID[member.ID]; principal != nil {
				principal.Groups = append(principal.Groups, ui.AdminGroupRef{
					ID:         group.ID,
					Name:       group.Name,
					ExternalID: group.ExternalID,
				})
			}
		}
	}
	for i := range out {
		if principal := byID[out[i].ID]; principal != nil {
			sort.Strings(principal.DirectRoles)
			sort.SliceStable(principal.Groups, func(i, j int) bool {
				return principal.Groups[i].Name < principal.Groups[j].Name
			})
			out[i] = *principal
		}
	}
	return out
}

func buildAdminGroups(groups []access.Group, bindings []workspace.RoleBindingView, membersByGroup map[string][]ui.AdminPrincipalRef) []ui.AdminGroup {
	out := make([]ui.AdminGroup, 0, len(groups))
	byID := make(map[string]*ui.AdminGroup, len(groups))
	for _, group := range groups {
		row := ui.AdminGroup{
			ID:         group.ID,
			Name:       group.Name,
			Provider:   group.Provider,
			ExternalID: group.ExternalID,
			CreatedAt:  group.CreatedAt,
			Members:    membersByGroup[group.ID],
		}
		sort.SliceStable(row.Members, func(i, j int) bool {
			return adminPrincipalRefSortKey(row.Members[i]) < adminPrincipalRefSortKey(row.Members[j])
		})
		byID[row.ID] = &row
		out = append(out, row)
	}
	for _, binding := range bindings {
		if binding.SubjectType == string(access.SubjectGroup) && binding.GroupID != "" {
			if group := byID[binding.GroupID]; group != nil {
				group.Roles = appendUnique(group.Roles, binding.Role)
			}
		}
	}
	for i := range out {
		if group := byID[out[i].ID]; group != nil {
			sort.Strings(group.Roles)
			out[i] = *group
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func stringMapValue(row map[string]any, key string) string {
	if value, ok := row[key].(string); ok {
		return value
	}
	return ""
}

func adminPrincipalSortKey(row ui.AdminPrincipal) string {
	return firstNonEmpty(row.Email, row.DisplayName, row.ID)
}

func adminPrincipalRefSortKey(row ui.AdminPrincipalRef) string {
	return firstNonEmpty(row.Email, row.DisplayName, row.ID)
}
