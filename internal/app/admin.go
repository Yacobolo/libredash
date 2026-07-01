package app

import (
	"database/sql"
	"net/http"
	"sort"

	"github.com/Yacobolo/libredash/internal/access"
	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/go-chi/chi/v5"
)

func (s *Server) adminGeneral(w http.ResponseWriter, r *http.Request) {
	s.renderAdminPage(w, r, "general")
}

func (s *Server) adminPrincipals(w http.ResponseWriter, r *http.Request) {
	s.renderAdminPage(w, r, "principals")
}

func (s *Server) adminPrincipalDetail(w http.ResponseWriter, r *http.Request) {
	data, err := s.adminData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	principalID := chi.URLParam(r, "principal")
	for i := range data.Principals {
		if data.Principals[i].ID == principalID {
			data.SelectedPrincipal = &data.Principals[i]
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if err := ui.AdminPage(s.metrics.Catalog(), "principal-detail", s.currentAdminRoleLabel(r), data, s.chatChromeOption(r)).Render(w); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) adminGroups(w http.ResponseWriter, r *http.Request) {
	s.renderAdminPage(w, r, "groups")
}

func (s *Server) adminStorage(w http.ResponseWriter, r *http.Request) {
	_ = lddatastar.EnsureClientID(w, r)
	s.renderAdminPage(w, r, "storage")
}

func (s *Server) adminGroupDetail(w http.ResponseWriter, r *http.Request) {
	data, err := s.adminData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	groupID := chi.URLParam(r, "group")
	for i := range data.Groups {
		if data.Groups[i].ID == groupID {
			data.SelectedGroup = &data.Groups[i]
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if err := ui.AdminPage(s.metrics.Catalog(), "group-detail", s.currentAdminRoleLabel(r), data, s.chatChromeOption(r)).Render(w); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) renderAdminPage(w http.ResponseWriter, r *http.Request, active string) {
	data, err := s.adminData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.AdminPage(s.metrics.Catalog(), active, s.currentAdminRoleLabel(r), data, s.chatChromeOption(r)).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

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
	repo, err := s.accessRepository()
	if err != nil {
		return data, err
	}
	if repo == nil {
		data.RBACConfigured = false
		data.RBACStatusLabel = "RBAC store is not configured"
		data.RoleCount = len(defaultWorkspaceRoles())
		data.Storage = s.adminStorageData(r)
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
	data.Storage = s.adminStorageData(r)
	data.PrincipalCount = len(data.Principals)
	data.GroupCount = len(data.Groups)
	return data, nil
}

func (s *Server) adminGroupsData(r *http.Request) ([]access.Group, error) {
	if s.store == nil {
		return nil, nil
	}
	rows, err := s.store.SQLDB().QueryContext(r.Context(), `
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

func (s *Server) adminGroupMembersData(r *http.Request, groupID string) []ui.AdminPrincipalRef {
	if s.store == nil {
		return nil
	}
	rows, err := s.store.SQLDB().QueryContext(r.Context(), `
SELECT gm.principal_id, p.email, p.display_name
FROM group_members gm
JOIN principals p ON p.id = gm.principal_id
WHERE gm.group_id = ?
ORDER BY p.email, p.display_name, gm.principal_id
`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	members := []ui.AdminPrincipalRef{}
	for rows.Next() {
		var member ui.AdminPrincipalRef
		if err := rows.Scan(&member.ID, &member.Email, &member.DisplayName); err != nil {
			return nil
		}
		members = append(members, member)
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
	if s.store == nil {
		return nil, nil
	}
	rows, err := s.store.SQLDB().QueryContext(r.Context(), `
SELECT
  rb.id,
  rb.workspace_id,
  CASE WHEN NULLIF(rb.principal_id, '') IS NOT NULL THEN 'principal' ELSE 'group' END AS subject_type,
  COALESCE(NULLIF(rb.principal_id, ''), rb.group_id, '') AS subject_id,
  rb.principal_id,
  rb.group_id,
  p.email,
  p.display_name,
  g.name AS group_name,
  r.name AS role_name,
  rb.created_at
FROM role_bindings rb
JOIN roles r ON r.id = rb.role_id
LEFT JOIN principals p ON p.id = NULLIF(rb.principal_id, '')
LEFT JOIN groups g ON g.id = rb.group_id
ORDER BY rb.workspace_id, subject_type, p.email, g.name, r.name
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := []workspace.RoleBindingView{}
	for rows.Next() {
		var binding workspace.RoleBindingView
		var principalID, groupID, email, displayName, groupName sql.NullString
		if err := rows.Scan(&binding.ID, &binding.WorkspaceID, &binding.SubjectType, &binding.SubjectID, &principalID, &groupID, &email, &displayName, &groupName, &binding.Role, &binding.CreatedAt); err != nil {
			return nil, err
		}
		binding.PrincipalID = adminNullString(principalID)
		binding.GroupID = adminNullString(groupID)
		binding.Email = adminNullString(email)
		binding.DisplayName = firstNonEmpty(adminNullString(displayName), adminNullString(groupName))
		binding.GroupName = adminNullString(groupName)
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (s *Server) adminPrincipalsData(r *http.Request) ([]ui.AdminPrincipal, error) {
	rows, err := s.queryPrincipals(r)
	if err != nil {
		return nil, err
	}
	principals := make([]ui.AdminPrincipal, 0, len(rows))
	for _, row := range rows {
		principals = append(principals, ui.AdminPrincipal{
			ID:          stringMapValue(row, "id"),
			Email:       stringMapValue(row, "email"),
			DisplayName: stringMapValue(row, "displayName"),
			CreatedAt:   stringMapValue(row, "createdAt"),
			UpdatedAt:   stringMapValue(row, "updatedAt"),
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

func adminNullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
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
