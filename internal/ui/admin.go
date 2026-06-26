package ui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	workspaceview "github.com/Yacobolo/libredash/internal/workspace"
	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

const adminMainClass = "grid min-w-0 min-h-svh content-start grid-cols-[minmax(0,1fr)] gap-3 bg-app px-4 py-4 max-sm:min-h-0 max-sm:p-3"

type AdminData struct {
	Workspace         workspaceview.WorkspaceView
	AuthConfigured    bool
	RBACConfigured    bool
	RBACStatusLabel   string
	PrincipalCount    int
	GroupCount        int
	BindingCount      int
	RoleCount         int
	Roles             []workspaceview.RoleView
	Principals        []AdminPrincipal
	SelectedPrincipal *AdminPrincipal
	Groups            []AdminGroup
	SelectedGroup     *AdminGroup
}

type AdminPrincipal struct {
	ID          string
	Email       string
	DisplayName string
	CreatedAt   string
	UpdatedAt   string
	DirectRoles []string
	Groups      []AdminGroupRef
}

type AdminGroupRef struct {
	ID         string
	Name       string
	ExternalID string
}

type AdminGroup struct {
	ID         string
	Name       string
	Provider   string
	ExternalID string
	CreatedAt  string
	Roles      []string
	Members    []AdminPrincipalRef
}

type AdminPrincipalRef struct {
	ID          string
	Email       string
	DisplayName string
}

func AdminPage(catalog dashboard.Catalog, active, roleLabel string, data AdminData) g.Node {
	title := adminPageTitle(active)
	return workspaceDocument("Admin - "+title, catalog, "admin", roleLabel, nil,
		h.Div(h.Class("grid min-h-svh min-w-0 content-start grid-cols-[max-content_minmax(0,1fr)] bg-app max-sm:grid-cols-1"),
			adminSubSidebar(active),
			adminContent(active, data),
		),
		h.Script(h.Type("module"), h.Src(staticAsset("/static/sub-sidebar.js"))),
		h.Script(h.Type("module"), h.Src(staticAsset("/static/data-grid.js"))),
	)
}

func adminSubSidebar(active string) g.Node {
	principalsActive := active == "principals" || active == "principal-detail"
	groupsActive := active == "groups" || active == "group-detail"
	items := []map[string]any{
		{"id": "general", "title": "General", "href": "/admin", "active": active == "general"},
		{"id": "principals", "title": "Principals", "href": "/admin/principals", "active": principalsActive},
		{"id": "groups", "title": "Groups", "href": "/admin/groups", "active": groupsActive},
	}
	return g.El("ld-sub-sidebar",
		h.Class("border-r border-outline-variant max-sm:border-b max-sm:border-r-0"),
		g.Attr("config", jsonString(map[string]any{
			"label":       "Admin",
			"railLabel":   "Admin",
			"ariaLabel":   "Admin navigation",
			"storageKey":  "libredash-admin-sidebar-collapsed",
			"activeId":    active,
			"numbered":    false,
			"collapsible": false,
			"items":       items,
		})),
	)
}

func adminContent(active string, data AdminData) g.Node {
	switch active {
	case "principals":
		return adminPrincipalsContent(data)
	case "principal-detail":
		return adminPrincipalDetailContent(data)
	case "groups":
		return adminGroupsContent(data)
	case "group-detail":
		return adminGroupDetailContent(data)
	default:
		return adminGeneralContent(data)
	}
}

func adminGeneralContent(data AdminData) g.Node {
	return h.Section(h.Class(adminMainClass), h.Aria("label", "Admin general"),
		workspaceHeader("Admin", "General", "Read-only workspace administration.", nil),
		g.If(!data.RBACConfigured, emptyState(data.RBACStatusLabel)),
		h.Div(h.Class("grid max-w-workspace-detail content-start items-start grid-cols-[repeat(auto-fit,minmax(10rem,1fr))] gap-3"),
			adminMetricCard("Workspace", data.Workspace.Title, data.Workspace.ID),
			adminMetricCard("Auth", configuredLabel(data.AuthConfigured), ""),
			adminMetricCard("RBAC", data.RBACStatusLabel, ""),
			adminMetricCard("Principals", fmt.Sprint(data.PrincipalCount), ""),
			adminMetricCard("Groups", fmt.Sprint(data.GroupCount), ""),
			adminMetricCard("Role bindings", fmt.Sprint(data.BindingCount), ""),
			adminMetricCard("Roles", fmt.Sprint(data.RoleCount), ""),
		),
	)
}

func adminPrincipalsContent(data AdminData) g.Node {
	return h.Section(h.Class(adminMainClass), h.Aria("label", "Admin principals"),
		workspaceHeader("Admin", "Principals", "Users and service principals known to LibreDash.", nil),
		h.Div(h.Class(workspacePanelClass),
			g.El("ld-data-grid", g.Attr("grid", jsonString(adminPrincipalsGrid(data.Principals)))),
		),
	)
}

func adminPrincipalDetailContent(data AdminData) g.Node {
	if data.SelectedPrincipal == nil {
		return h.Section(h.Class(adminMainClass), h.Aria("label", "Admin principal detail"),
			workspaceHeader("Admin", "Principals", "Read-only principal access.", adminBackToPrincipalsAction()),
			emptyState("Principal not found."),
		)
	}
	principal := *data.SelectedPrincipal
	name := adminDisplayLabel(principal.DisplayName, principal.Email, principal.ID)
	return h.Section(h.Class(adminMainClass), h.Aria("label", "Admin principal detail"),
		workspaceHeader("Admin", "Principals / "+name, "Read-only principal identity and group memberships.", adminBackToPrincipalsAction()),
		h.Div(h.Class("grid max-w-workspace-detail content-start items-start grid-cols-[repeat(auto-fit,minmax(14rem,1fr))] gap-3"),
			adminMetricCard("Email", principal.Email, ""),
			adminMetricCard("Principal ID", principal.ID, ""),
			adminMetricCard("Direct roles", strings.Join(principal.DirectRoles, ", "), ""),
			adminMetricCard("Group count", fmt.Sprint(len(principal.Groups)), ""),
			adminMetricCard("Created", principal.CreatedAt, ""),
			adminMetricCard("Updated", principal.UpdatedAt, ""),
		),
		h.Section(h.Class("grid min-w-0 content-start gap-3"), h.Aria("label", "Principal groups"),
			h.H2(h.Class("m-0 text-body-sm font-semibold text-fg-default"), g.Text("Groups")),
			h.Div(h.Class(workspacePanelClass),
				g.El("ld-data-grid", g.Attr("grid", jsonString(adminPrincipalGroupsGrid(principal, data.Groups)))),
			),
		),
	)
}

func adminGroupsContent(data AdminData) g.Node {
	return h.Section(h.Class(adminMainClass), h.Aria("label", "Admin groups"),
		workspaceHeader("Admin", "Groups", "Workspace groups and their read-only membership summaries.", nil),
		h.Div(h.Class(workspacePanelClass),
			g.El("ld-data-grid", g.Attr("grid", jsonString(adminGroupsGrid(data.Groups)))),
		),
	)
}

func adminGroupDetailContent(data AdminData) g.Node {
	if data.SelectedGroup == nil {
		return h.Section(h.Class(adminMainClass), h.Aria("label", "Admin group detail"),
			workspaceHeader("Admin", "Groups", "Read-only group membership.", adminBackToGroupsAction()),
			emptyState("Group not found."),
		)
	}
	group := *data.SelectedGroup
	name := adminDisplayLabel(group.Name, group.ExternalID, group.ID)
	return h.Section(h.Class(adminMainClass), h.Aria("label", "Admin group detail"),
		workspaceHeader("Admin", "Groups / "+name, "Read-only group membership and role assignments.", adminBackToGroupsAction()),
		h.Div(h.Class("grid max-w-workspace-detail content-start items-start grid-cols-[repeat(auto-fit,minmax(10rem,1fr))] gap-3"),
			adminMetricCard("Provider", group.Provider, ""),
			adminMetricCard("External ID", group.ExternalID, ""),
			adminMetricCard("Group ID", group.ID, ""),
			adminMetricCard("Roles", strings.Join(group.Roles, ", "), ""),
			adminMetricCard("Member count", fmt.Sprint(len(group.Members)), ""),
		),
		h.Section(h.Class("grid min-w-0 content-start gap-3"), h.Aria("label", "Group members"),
			h.H2(h.Class("m-0 text-body-sm font-semibold text-fg-default"), g.Text("Members")),
			h.Div(h.Class(workspacePanelClass),
				g.El("ld-data-grid", g.Attr("grid", jsonString(adminGroupMembersGrid(group, data.Principals)))),
			),
		),
	)
}

func adminBackToPrincipalsAction() g.Node {
	return h.A(h.Class(metricActionButtonClass), h.Href("/admin/principals"), h.Title("Back to principals"), h.Aria("label", "Back to principals"), lucide.ArrowLeft(metricActionIconAttrs()...))
}

func adminBackToGroupsAction() g.Node {
	return h.A(h.Class(metricActionButtonClass), h.Href("/admin/groups"), h.Title("Back to groups"), h.Aria("label", "Back to groups"), lucide.ArrowLeft(metricActionIconAttrs()...))
}

func adminMetricCard(label, value, detail string) g.Node {
	if strings.TrimSpace(value) == "" {
		value = "-"
	}
	return h.Div(h.Class(workspacePanelClass+" content-start p-4"),
		h.P(h.Class(eyebrowClass), g.Text(label)),
		h.P(h.Class("m-0 truncate text-title-sm font-semibold text-fg-default"), g.Text(value)),
		g.If(detail != "", h.P(h.Class("m-0 mt-1 truncate text-caption font-medium text-fg-muted"), g.Text(detail))),
	)
}

func adminPrincipalsGrid(principals []AdminPrincipal) metricGrid {
	rows := make([]map[string]any, 0, len(principals))
	for _, principal := range principals {
		rows = append(rows, map[string]any{
			"name":        adminDisplayLabel(principal.DisplayName, principal.Email, principal.ID),
			"name_href":   adminPrincipalHref(principal.ID),
			"email":       principal.Email,
			"id":          principal.ID,
			"roles":       principal.DirectRoles,
			"group_count": len(principal.Groups),
			"updated_at":  principal.UpdatedAt,
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "name_href", Width: "150px"},
			{ID: "email", Header: "Email", Width: "190px"},
			{ID: "roles", Header: "Direct roles", Kind: "tags", Width: "135px"},
			{ID: "group_count", Header: "Group count", Kind: "number", Align: "right", Width: "120px"},
			{ID: "id", Header: "Principal ID", Kind: "code", Width: "190px"},
			{ID: "updated_at", Header: "Updated", Width: "150px"},
		},
		Rows:     rows,
		Empty:    "No principals found.",
		MinWidth: "935px",
	}
}

func adminPrincipalGroupsGrid(principal AdminPrincipal, groups []AdminGroup) metricGrid {
	groupsByID := make(map[string]AdminGroup, len(groups))
	for _, group := range groups {
		groupsByID[group.ID] = group
	}
	rows := make([]map[string]any, 0, len(principal.Groups))
	for _, ref := range principal.Groups {
		group := groupsByID[ref.ID]
		rows = append(rows, map[string]any{
			"name":         adminDisplayLabel(group.Name, ref.Name, group.ExternalID, ref.ExternalID, ref.ID),
			"name_href":    adminGroupHref(ref.ID),
			"provider":     group.Provider,
			"external_id":  adminDisplayLabel(group.ExternalID, ref.ExternalID),
			"roles":        group.Roles,
			"member_count": len(group.Members),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "name_href", Width: "180px"},
			{ID: "provider", Header: "Provider", Width: "120px"},
			{ID: "external_id", Header: "External ID", Kind: "code", Width: "180px"},
			{ID: "roles", Header: "Roles", Kind: "tags", Width: "160px"},
			{ID: "member_count", Header: "Member count", Kind: "number", Align: "right", Width: "130px"},
		},
		Rows:     rows,
		Empty:    "No groups found.",
		MinWidth: "800px",
	}
}

func adminGroupsGrid(groups []AdminGroup) metricGrid {
	rows := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		rows = append(rows, map[string]any{
			"name":         adminDisplayLabel(group.Name, group.ExternalID, group.ID),
			"name_href":    adminGroupHref(group.ID),
			"provider":     group.Provider,
			"external_id":  group.ExternalID,
			"id":           group.ID,
			"roles":        group.Roles,
			"member_count": len(group.Members),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "name_href", Width: "180px"},
			{ID: "provider", Header: "Provider", Width: "120px"},
			{ID: "external_id", Header: "External ID", Kind: "code", Width: "180px"},
			{ID: "roles", Header: "Roles", Kind: "tags", Width: "180px"},
			{ID: "member_count", Header: "Member count", Kind: "number", Align: "right", Width: "130px"},
			{ID: "id", Header: "Group ID", Kind: "code", Width: "220px"},
		},
		Rows:     rows,
		Empty:    "No groups found.",
		MinWidth: "1010px",
	}
}

func adminGroupMembersGrid(group AdminGroup, principals []AdminPrincipal) metricGrid {
	principalsByID := make(map[string]AdminPrincipal, len(principals))
	for _, principal := range principals {
		principalsByID[principal.ID] = principal
	}
	rows := make([]map[string]any, 0, len(group.Members))
	for _, member := range group.Members {
		principal := principalsByID[member.ID]
		rows = append(rows, map[string]any{
			"name":         adminDisplayLabel(member.DisplayName, principal.DisplayName, member.Email, principal.Email, member.ID),
			"email":        adminDisplayLabel(member.Email, principal.Email),
			"id":           member.ID,
			"direct_roles": principal.DirectRoles,
			"updated_at":   principal.UpdatedAt,
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Width: "150px"},
			{ID: "email", Header: "Email", Width: "190px"},
			{ID: "id", Header: "Principal ID", Kind: "code", Width: "180px"},
			{ID: "direct_roles", Header: "Direct roles", Kind: "tags", Width: "130px"},
			{ID: "updated_at", Header: "Updated", Width: "150px"},
		},
		Rows:     rows,
		Empty:    "No members found.",
		MinWidth: "840px",
	}
}

func adminGroupRefLabels(values []AdminGroupRef) []string {
	tags := make([]string, 0, len(values))
	for _, value := range values {
		tags = append(tags, adminDisplayLabel(value.Name, value.ExternalID, value.ID))
	}
	return tags
}

func adminPrincipalLabels(values []AdminPrincipalRef) []string {
	labels := make([]string, 0, len(values))
	for _, value := range values {
		labels = append(labels, adminPrincipalLabel(value))
	}
	return labels
}

func adminGroupHref(groupID string) string {
	return "/admin/groups/" + url.PathEscape(groupID)
}

func adminPrincipalHref(principalID string) string {
	return "/admin/principals/" + url.PathEscape(principalID)
}

func adminPageTitle(active string) string {
	switch active {
	case "principals":
		return "Principals"
	case "principal-detail":
		return "Principal"
	case "groups":
		return "Groups"
	case "group-detail":
		return "Group"
	default:
		return "General"
	}
}

func configuredLabel(configured bool) string {
	if configured {
		return "Configured"
	}
	return "Not configured"
}

func adminDisplayLabel(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "-"
}

func adminPrincipalLabel(member AdminPrincipalRef) string {
	if strings.TrimSpace(member.DisplayName) != "" && strings.TrimSpace(member.Email) != "" && member.DisplayName != member.Email {
		return member.DisplayName + " <" + member.Email + ">"
	}
	return adminDisplayLabel(member.DisplayName, member.Email, member.ID)
}
