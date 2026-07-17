package http

import (
	"context"
	"net/http"
	"sort"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/admin/storage"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/queryaudit"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Principal struct {
	ID          string
	Email       string
	DisplayName string
	DevBypass   bool
}

type AgentDetailsProvider func(context.Context) (api.AdminAgentResponse, error)
type CSRFTokenProvider func(*http.Request) string
type CurrentPrincipalProvider func(*http.Request) (Principal, bool)

type ReadModel struct {
	AccessRepository     func() (access.Repository, error)
	AgentDetails         AgentDetailsProvider
	StorageService       storage.Service
	QueryAuditRepository QueryAuditRepositoryProvider
	CSRFToken            CSRFTokenProvider
	CurrentPrincipal     CurrentPrincipalProvider
	DefaultWorkspaceID   string
	AuthConfigured       bool
	AccessConfigured     bool
}

func (m ReadModel) Data(r *http.Request) (ui.AdminData, error) {
	data := ui.AdminData{
		Workspace:         workspace.WorkspaceView{ID: "platform", Title: "Platform"},
		CSRFToken:         m.csrfToken(r),
		AuthConfigured:    m.AuthConfigured,
		AccessConfigured:  m.AccessConfigured,
		AccessStatusLabel: "Configured",
	}
	var err error
	data.Agent, err = m.agentData(r)
	if err != nil {
		return data, err
	}
	repo, err := m.accessRepository()
	if err != nil {
		return data, err
	}
	if repo == nil {
		data.AccessConfigured = false
		data.AccessStatusLabel = "Access store is not configured"
		data.RoleCount = len(defaultRoleViews())
		data.Storage = m.StorageService.Data(r.Context())
		return data, nil
	}
	principals, err := m.principalsData(r, repo)
	if err != nil {
		return data, err
	}
	groups, err := repo.ListAllGroups(r.Context())
	if err != nil {
		return data, err
	}
	bindings, roles, err := m.roleBindingsAndRoles(r, repo)
	if err != nil {
		return data, err
	}
	membersByGroup := map[string][]ui.AdminPrincipalRef{}
	groupsByID := map[string]access.Group{}
	for _, group := range groups {
		groupsByID[group.ID] = group
		members := groupMembersData(r, repo, group.ID)
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
	data.Storage = m.StorageService.Data(r.Context())
	data.QueryHistory = m.QueryHistoryData(r, uisignals.AdminQueryHistoryFilters{}, "", 50)
	data.PrincipalCount = len(data.Principals)
	data.GroupCount = len(data.Groups)
	return data, nil
}

func (m ReadModel) agentData(r *http.Request) (ui.AdminAgentData, error) {
	details, err := m.agentDetails(r.Context())
	if err != nil {
		return ui.AdminAgentData{}, err
	}
	data := ui.AdminAgentData{
		Enabled:      details.Enabled,
		Model:        details.Model,
		SystemPrompt: details.SystemPrompt,
		CSRFToken:    m.csrfToken(r),
		UpdatePath:   "/admin/agent/config",
		CanWrite:     true,
	}
	for _, tool := range details.Tools {
		data.Tools = append(data.Tools, ui.AdminAgentTool{
			Name:         tool.Name,
			Description:  tool.Description,
			Effect:       tool.Effect,
			Defaults:     tool.Defaults,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
		})
	}
	if !m.AuthConfigured {
		return data, nil
	}
	principal, ok := m.currentPrincipal(r)
	if !ok || principal.DevBypass {
		return data, nil
	}
	repo, err := m.accessRepository()
	if err != nil || repo == nil {
		return data, err
	}
	decision, err := repo.Authorize(r.Context(), principal.ID, access.PrivilegeManageGrants, access.WorkspaceObject(m.DefaultWorkspaceID))
	if err != nil {
		return data, err
	}
	data.CanWrite = decision.Allowed
	return data, nil
}

func (m ReadModel) principalsData(r *http.Request, repo access.Repository) ([]ui.AdminPrincipal, error) {
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

func groupMembersData(r *http.Request, repo access.Repository, groupID string) []ui.AdminPrincipalRef {
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

func (m ReadModel) roleBindingsAndRoles(r *http.Request, repo access.Repository) ([]workspace.RoleBindingView, []workspace.RoleView, error) {
	if repo == nil {
		return nil, defaultRoleViews(), nil
	}
	roleRows, err := repo.ListRoles(r.Context())
	if err != nil {
		return nil, nil, err
	}
	bindingRows, err := repo.ListAllRoleBindings(r.Context())
	if err != nil {
		return nil, nil, err
	}
	bindings := make([]workspace.RoleBindingView, 0, len(bindingRows))
	for _, row := range bindingRows {
		bindings = append(bindings, roleBindingView(row))
	}
	return bindings, roleViews(roleRows), nil
}

func (m ReadModel) QueryHistoryData(r *http.Request, filters uisignals.AdminQueryHistoryFilters, pageToken string, limit int) ui.AdminQueryHistoryData {
	repo, err := m.queryAuditRepository()
	if err != nil || repo == nil {
		return ui.AdminQueryHistoryData{Filters: filters, Limit: normalizeQueryHistoryLimit(limit), Error: queryHistoryErrorText(err)}
	}
	filters = normalizeQueryHistoryFilters(filters)
	events, nextCursor, hasMore, err := queryHistoryPage(r, repo, filters, pageToken, limit)
	if err != nil {
		return ui.AdminQueryHistoryData{Filters: filters, Limit: normalizeQueryHistoryLimit(limit), Error: err.Error()}
	}
	return ui.AdminQueryHistoryData{
		Events:      events,
		FilterMenus: m.queryHistoryFilterMenus(r, repo, filters, "", ""),
		Filters:     filters,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
		Limit:       normalizeQueryHistoryLimit(limit),
	}
}

func (m ReadModel) PrincipalLabels(r *http.Request, values []string) map[string]string {
	labels := map[string]string{}
	current, hasCurrent := m.currentPrincipal(r)
	for _, value := range values {
		if value == "" {
			continue
		}
		if hasCurrent && value == current.ID {
			identity := firstNonEmpty(current.Email, current.DisplayName, current.ID)
			labels[value] = "Me (" + identity + ")"
			continue
		}
		labels[value] = value
	}
	return labels
}

func (m ReadModel) accessRepository() (access.Repository, error) {
	if m.AccessRepository == nil {
		return nil, nil
	}
	return m.AccessRepository()
}

func (m ReadModel) queryAuditRepository() (queryaudit.Repository, error) {
	if m.QueryAuditRepository == nil {
		return nil, nil
	}
	return m.QueryAuditRepository()
}

func (m ReadModel) agentDetails(ctx context.Context) (api.AdminAgentResponse, error) {
	if m.AgentDetails == nil {
		return api.AdminAgentResponse{}, nil
	}
	return m.AgentDetails(ctx)
}

func (m ReadModel) currentPrincipal(r *http.Request) (Principal, bool) {
	if m.CurrentPrincipal == nil {
		return Principal{}, false
	}
	return m.CurrentPrincipal(r)
}

func (m ReadModel) csrfToken(r *http.Request) string {
	if m.CSRFToken == nil {
		return ""
	}
	return m.CSRFToken(r)
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

func defaultRoleViews() []workspace.RoleView {
	return roleViews(access.DefaultRoles())
}

func roleViews(rows []access.Role) []workspace.RoleView {
	roles := make([]workspace.RoleView, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, workspace.RoleView{Name: row.Name, Privileges: privilegeStrings(row.Privileges)})
	}
	return roles
}

func privilegeStrings(values []access.Privilege) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func roleBindingView(row access.RoleBinding) workspace.RoleBindingView {
	return workspace.RoleBindingView{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		SubjectType: string(row.SubjectType),
		SubjectID:   row.SubjectID,
		PrincipalID: row.PrincipalID,
		GroupID:     row.GroupID,
		Email:       row.Email,
		DisplayName: firstNonEmpty(row.DisplayName, row.GroupName),
		GroupName:   row.GroupName,
		Role:        row.Role,
		CreatedAt:   row.CreatedAt,
	}
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

func adminPrincipalSortKey(row ui.AdminPrincipal) string {
	return firstNonEmpty(row.Email, row.DisplayName, row.ID)
}

func adminPrincipalRefSortKey(row ui.AdminPrincipalRef) string {
	return firstNonEmpty(row.Email, row.DisplayName, row.ID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
