package app

import (
	"net/http"
	"sort"

	"github.com/Yacobolo/libredash/internal/access"
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
	workspaceID := s.workspaceID("")
	data, err := s.adminData(r, workspaceID)
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
			if err := ui.AdminPage(s.metrics.Catalog(), "principal-detail", s.currentRoleLabel(r), data).Render(w); err != nil {
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

func (s *Server) adminGroupDetail(w http.ResponseWriter, r *http.Request) {
	workspaceID := s.workspaceID("")
	data, err := s.adminData(r, workspaceID)
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
			if err := ui.AdminPage(s.metrics.Catalog(), "group-detail", s.currentRoleLabel(r), data).Render(w); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) renderAdminPage(w http.ResponseWriter, r *http.Request, active string) {
	workspaceID := s.workspaceID("")
	data, err := s.adminData(r, workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.AdminPage(s.metrics.Catalog(), active, s.currentRoleLabel(r), data).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) adminData(r *http.Request, workspaceID string) (ui.AdminData, error) {
	data := ui.AdminData{
		Workspace:       s.workspaceResponse(r, workspaceID),
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
		data.Roles = defaultWorkspaceRoles()
		data.RoleCount = len(data.Roles)
		return data, nil
	}
	principals, err := s.adminPrincipalsData(r)
	if err != nil {
		return data, err
	}
	groups, err := repo.ListGroups(r.Context(), workspaceID)
	if err != nil {
		return data, err
	}
	bindings, roles, err := s.roleBindingsAndRoles(r, workspaceID)
	if err != nil {
		return data, err
	}
	membersByGroup := map[string][]ui.AdminPrincipalRef{}
	groupsByID := map[string]access.Group{}
	for _, group := range groups {
		groupsByID[group.ID] = group
		members, err := repo.ListGroupMembers(r.Context(), workspaceID, group.ID)
		if err != nil {
			return data, err
		}
		for _, member := range members {
			membersByGroup[group.ID] = append(membersByGroup[group.ID], ui.AdminPrincipalRef{
				ID:          member.PrincipalID,
				Email:       member.Email,
				DisplayName: member.DisplayName,
			})
		}
	}
	data.Roles = roles
	data.RoleCount = len(roles)
	data.BindingCount = len(bindings)
	data.Principals = buildAdminPrincipals(principals, bindings, groupsByID, membersByGroup)
	data.Groups = buildAdminGroups(groups, bindings, membersByGroup)
	data.PrincipalCount = len(data.Principals)
	data.GroupCount = len(data.Groups)
	return data, nil
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
