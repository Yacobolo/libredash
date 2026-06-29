package ui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	workspaceview "github.com/Yacobolo/libredash/internal/workspace"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

type AdminData struct {
	Workspace         workspaceview.WorkspaceView
	CSRFToken         string
	AuthConfigured    bool
	RBACConfigured    bool
	RBACStatusLabel   string
	PrincipalCount    int
	GroupCount        int
	BindingCount      int
	RoleCount         int
	Principals        []AdminPrincipal
	SelectedPrincipal *AdminPrincipal
	Groups            []AdminGroup
	SelectedGroup     *AdminGroup
	Storage           AdminStorageData
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

type AdminStorageData = uisignals.AdminStorageData
type AdminStorageDatabase = uisignals.AdminStorageDatabase
type AdminStorageTable = uisignals.AdminStorageTable
type AdminStorageColumn = uisignals.AdminStorageColumn
type AdminStorageSignal = uisignals.AdminStorageSignal
type AdminStorageSummary = uisignals.AdminStorageSummary
type AdminStorageTableSignal = uisignals.AdminStorageTableSignal
type AdminStorageColumnSignal = uisignals.AdminStorageColumnSignal
type AdminStorageCommand = uisignals.AdminStorageCommand

func AdminPage(catalog dashboard.Catalog, active, roleLabel string, data AdminData) g.Node {
	title := adminPageTitle(active)
	page := adminPageSignal(active, data)
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForWorkspace(catalog, "admin", roleLabel)}
	storageSignal := page.Storage
	signals := map[string]any{
		"chrome":  chrome,
		"page":    page,
		"runtime": uisignals.RouteRuntimeSignal{Kind: uisignals.RouteAdmin},
		"status":  dashboard.Status{},
	}
	if active == "storage" {
		signals["adminStorage"] = storageSignal
		signals["adminStorageCommand"] = AdminStorageCommand{}
	}
	adminAttrs := []g.Node{
		g.Attr("slot", "page"),
		g.Attr("page", jsonString(page)),
		g.Attr("data-attr:page", "JSON.stringify($page)"),
	}
	if active == "storage" {
		adminAttrs = append(adminAttrs,
			g.Attr("storage", jsonString(storageSignal)),
			g.Attr("data-attr:storage", "JSON.stringify($adminStorage)"),
			g.Attr("data-on:ld-storage-table-select", "$adminStorageCommand = evt.detail; "+postAction("/admin/storage/select-table")),
		)
	}
	return c.HTML5(c.HTML5Props{
		Title:    "Admin - " + title,
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/admin-page.js"))),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		),
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				ds.Signals(signals),
				g.If(active == "storage", ds.Init("@get('/admin/storage/updates', {openWhenHidden: true})")),
				g.El("ld-app-shell",
					g.Attr("chrome", jsonString(chrome)),
					g.Attr("data-attr:chrome", "JSON.stringify($chrome)"),
					g.El("ld-admin-page", adminAttrs...),
				),
				inspectorElement(),
			),
		},
	})
}

func adminPageSignal(active string, data AdminData) uisignals.AdminPageSignal {
	page := uisignals.AdminPageSignal{
		Kind:    uisignals.RouteAdmin,
		Title:   adminPageTitle(active),
		Active:  active,
		Sidebar: adminSidebarSignal(active),
	}
	switch active {
	case "principals":
		page.HeaderTitle = "Principals"
		page.HeaderDetail = "Users and service principals known to LibreDash."
		page.Sections = []uisignals.AdminContentSectionSignal{{Title: "Principals", Grid: adminPrincipalsGrid(data.Principals)}}
	case "principal-detail":
		page.HeaderTitle = "Principals"
		page.HeaderDetail = "Read-only principal access."
		if data.SelectedPrincipal == nil {
			page.Empty = "Principal not found."
			return page
		}
		principal := *data.SelectedPrincipal
		name := adminDisplayLabel(principal.DisplayName, principal.Email, principal.ID)
		page.HeaderTitle = "Principals / " + name
		page.HeaderDetail = "Read-only principal identity and group memberships."
		page.Metrics = []uisignals.AdminMetricSignal{
			{Label: "Email", Value: principal.Email},
			{Label: "Principal ID", Value: principal.ID},
			{Label: "Direct roles", Value: strings.Join(principal.DirectRoles, ", ")},
			{Label: "Group count", Value: fmt.Sprint(len(principal.Groups))},
			{Label: "Created", Value: principal.CreatedAt},
			{Label: "Updated", Value: principal.UpdatedAt},
		}
		page.Sections = []uisignals.AdminContentSectionSignal{{Title: "Groups", Grid: adminPrincipalGroupsGrid(principal, data.Groups)}}
	case "groups":
		page.HeaderTitle = "Groups"
		page.HeaderDetail = "Workspace groups and their read-only membership summaries."
		page.Sections = []uisignals.AdminContentSectionSignal{{Title: "Groups", Grid: adminGroupsGrid(data.Groups)}}
	case "group-detail":
		page.HeaderTitle = "Groups"
		page.HeaderDetail = "Read-only group membership."
		if data.SelectedGroup == nil {
			page.Empty = "Group not found."
			return page
		}
		group := *data.SelectedGroup
		name := adminDisplayLabel(group.Name, group.ExternalID, group.ID)
		page.HeaderTitle = "Groups / " + name
		page.HeaderDetail = "Read-only group membership and role assignments."
		page.Metrics = []uisignals.AdminMetricSignal{
			{Label: "Provider", Value: group.Provider},
			{Label: "External ID", Value: group.ExternalID},
			{Label: "Group ID", Value: group.ID},
			{Label: "Roles", Value: strings.Join(group.Roles, ", ")},
			{Label: "Member count", Value: fmt.Sprint(len(group.Members))},
		}
		page.Sections = []uisignals.AdminContentSectionSignal{{Title: "Members", Grid: adminGroupMembersGrid(group, data.Principals)}}
	case "storage":
		page.HeaderTitle = "Storage"
		page.HeaderDetail = "Read-only DuckDB database and table inventory."
		page.Storage = AdminStorageSignalFromData(data.Storage, AdminStorageCommand{})
		if data.Storage.Status != "" {
			page.Empty = data.Storage.Status
		}
		page.Metrics = []uisignals.AdminMetricSignal{
			{Label: "DuckDB directory", Value: data.Storage.DuckDBDir},
			{Label: "Database files", Value: fmt.Sprint(data.Storage.DatabaseCount)},
			{Label: "Total size", Value: data.Storage.TotalSizeLabel},
			{Label: "Tables and views", Value: fmt.Sprint(data.Storage.TableCount)},
		}
	default:
		page.HeaderTitle = "General"
		page.HeaderDetail = "Read-only workspace administration."
		if !data.RBACConfigured {
			page.Empty = data.RBACStatusLabel
		}
		page.Metrics = []uisignals.AdminMetricSignal{
			{Label: "Workspace", Value: data.Workspace.Title, Detail: data.Workspace.ID},
			{Label: "Auth", Value: configuredLabel(data.AuthConfigured)},
			{Label: "RBAC", Value: data.RBACStatusLabel},
			{Label: "Principals", Value: fmt.Sprint(data.PrincipalCount)},
			{Label: "Groups", Value: fmt.Sprint(data.GroupCount)},
			{Label: "Role bindings", Value: fmt.Sprint(data.BindingCount)},
			{Label: "Roles", Value: fmt.Sprint(data.RoleCount)},
		}
	}
	return page
}

func adminSidebarSignal(active string) uisignals.SubSidebarSignal {
	principalsActive := active == "principals" || active == "principal-detail"
	groupsActive := active == "groups" || active == "group-detail"
	storageActive := active == "storage"
	return uisignals.SubSidebarSignal{
		Label:       "Admin",
		RailLabel:   "Admin",
		AriaLabel:   "Admin navigation",
		StorageKey:  "libredash-admin-sidebar-collapsed",
		ActiveID:    active,
		Numbered:    false,
		Collapsible: false,
		Items: []uisignals.SubSidebarItemSignal{
			{ID: "general", Title: "General", Href: "/admin", Active: active == "general"},
			{ID: "principals", Title: "Principals", Href: "/admin/principals", Active: principalsActive},
			{ID: "groups", Title: "Groups", Href: "/admin/groups", Active: groupsActive},
			{ID: "storage", Title: "Storage", Href: "/admin/storage", Active: storageActive},
		},
	}
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
	case "storage":
		return "Storage"
	default:
		return "General"
	}
}

func AdminStorageSignalFromData(data AdminStorageData, command AdminStorageCommand) AdminStorageSignal {
	tables := make([]AdminStorageTableSignal, 0, len(data.Tables))
	var selected *AdminStorageTableSignal
	for _, table := range data.Tables {
		signalTable := AdminStorageTableSignalFromTable(table)
		tables = append(tables, signalTable)
		if selected == nil && adminStorageCommandMatches(command, table) {
			copy := signalTable
			selected = &copy
		}
	}
	if selected == nil && len(tables) > 0 {
		copy := tables[0]
		selected = &copy
	}
	selectedKey := ""
	if selected != nil {
		selectedKey = selected.Key
	}
	return AdminStorageSignal{
		Summary: AdminStorageSummary{
			DuckDBDir:      data.DuckDBDir,
			DatabaseCount:  data.DatabaseCount,
			TotalSizeLabel: data.TotalSizeLabel,
			TableCount:     data.TableCount,
		},
		Status:        data.Status,
		Warnings:      data.Warnings,
		Tables:        tables,
		SelectedKey:   selectedKey,
		SelectedTable: selected,
	}
}

func AdminStorageTableSignalFromTable(table AdminStorageTable) AdminStorageTableSignal {
	columns := make([]AdminStorageColumnSignal, 0, len(table.Columns))
	for _, column := range table.Columns {
		columns = append(columns, AdminStorageColumnSignal{
			Name:     column.Name,
			Type:     column.Type,
			Ordinal:  column.Ordinal,
			Nullable: column.Nullable,
			Default:  column.Default,
		})
	}
	return AdminStorageTableSignal{
		Key:           AdminStorageTableKey(table.DatabaseID, table.Schema, table.Name),
		DatabaseID:    table.DatabaseID,
		DatabaseName:  table.DatabaseName,
		DatabasePath:  table.DatabasePath,
		ModelID:       table.ModelID,
		ModelName:     table.ModelName,
		Schema:        table.Schema,
		Name:          table.Name,
		Type:          table.Type,
		RowCountLabel: table.RowCountLabel,
		ColumnCount:   table.ColumnCount,
		SizeLabel:     table.SizeLabel,
		Columns:       columns,
	}
}

func AdminStorageTableKey(databaseID, schemaName, tableName string) string {
	return databaseID + "\x00" + schemaName + "\x00" + tableName
}

func adminStorageCommandMatches(command AdminStorageCommand, table AdminStorageTable) bool {
	return command.DatabaseID == table.DatabaseID && command.Schema == table.Schema && command.Table == table.Name
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
