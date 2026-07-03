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
	Agent             AdminAgentData
	Storage           AdminStorageData
	QueryHistory      AdminQueryHistoryData
}

type AdminAgentData struct {
	Enabled      bool
	Model        string
	SystemPrompt string
	CanWrite     bool
	CSRFToken    string
	UpdatePath   string
	Tools        []AdminAgentTool
}

type AdminAgentTool struct {
	Name        string
	Description string
	InputSchema map[string]any
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

type AdminQueryEvent struct {
	ID            string
	WorkspaceID   string
	PrincipalID   string
	Surface       string
	Operation     string
	QueryKind     string
	ModelID       string
	Target        string
	ObjectType    string
	ObjectID      string
	RequestID     string
	CorrelationID string
	Status        string
	DurationMS    int64
	RowsReturned  int
	Error         string
	SQL           string
	PlanText      string
	QueryJSON     string
	CreatedAt     string
}

type AdminQueryHistoryData struct {
	Events      []AdminQueryEvent
	FilterMenus []uisignals.FilterMenuSignal
	Filters     uisignals.AdminQueryHistoryFilters
	NextCursor  string
	HasMore     bool
	Limit       int
	Error       string
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
type AdminStorageFile = uisignals.AdminStorageFile
type AdminStorageTableHistory = uisignals.AdminStorageTableHistory
type AdminStorageSnapshot = uisignals.AdminStorageSnapshot
type AdminStorageDeployment = uisignals.AdminStorageDeployment
type AdminStorageSignal = uisignals.AdminStorageSignal
type AdminStorageSummary = uisignals.AdminStorageSummary
type AdminStorageTableSignal = uisignals.AdminStorageTableSignal
type AdminStorageColumnSignal = uisignals.AdminStorageColumnSignal
type AdminStorageFileSignal = uisignals.AdminStorageFileSignal
type AdminStorageTableHistorySignal = uisignals.AdminStorageTableHistorySignal
type AdminStorageSnapshotSignal = uisignals.AdminStorageSnapshotSignal
type AdminStorageDeploymentSignal = uisignals.AdminStorageDeploymentSignal
type AdminStorageCommand = uisignals.AdminStorageCommand

func AdminPage(catalog dashboard.Catalog, active, roleLabel string, data AdminData, chromeOptions ...ChromeOption) g.Node {
	title := adminPageTitle(active)
	page := adminPageSignal(active, data)
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForWorkspace(catalog, "admin", roleLabel)}
	applyChromeOptions(&chrome, chromeOptions)
	storageSignal := page.Storage
	signals := map[string]any{
		"chrome":    chrome,
		"page":      page,
		"runtime":   uisignals.RouteRuntimeSignal{Kind: uisignals.RouteAdmin},
		"status":    dashboard.Status{},
		"csrfToken": data.CSRFToken,
	}
	if active == "agent" {
		signals["adminAgentCommand"] = map[string]string{"systemPrompt": data.Agent.SystemPrompt}
	}
	if active == "storage" {
		signals["adminStorage"] = storageSignal
		signals["adminStorageCommand"] = AdminStorageCommand{}
	}
	if active == "queries" {
		queryHistory := AdminQueryHistorySignalFromData(data.QueryHistory)
		signals["adminQueryHistory"] = queryHistory
		signals["adminQueryDetail"] = uisignals.AdminQueryDetailSignal{}
		signals["adminQueryHistoryCommand"] = uisignals.AdminQueryHistoryCommand{Action: "load_more", Filters: queryHistory.Filters, PageToken: queryHistory.NextCursor, Limit: queryHistory.Limit}
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
	if active == "agent" {
		adminAttrs = append(adminAttrs,
			g.Attr("agent-prompt", data.Agent.SystemPrompt),
			g.Attr("data-attr:agent-prompt", "$adminAgentCommand.systemPrompt"),
			g.Attr("data-on:ld-agent-system-prompt-save", "$adminAgentCommand = evt.detail; "+patchAction("/api/v1/admin/agent/config")),
		)
	}
	if active == "queries" {
		adminAttrs = append(adminAttrs,
			g.Attr("query-history", jsonString(AdminQueryHistorySignalFromData(data.QueryHistory))),
			g.Attr("query-detail", jsonString(uisignals.AdminQueryDetailSignal{})),
			g.Attr("data-attr:query-history", "JSON.stringify($adminQueryHistory)"),
			g.Attr("data-attr:query-detail", "JSON.stringify($adminQueryDetail)"),
			g.Attr("data-on:ld-query-history-command", "$adminQueryHistoryCommand = evt.detail; evt.detail.action == 'select_detail' ? ($adminQueryDetail = {eventId: evt.detail.eventId, loading: true, error: ''}) : evt.detail.action == 'close_detail' ? ($adminQueryDetail = {eventId: '', loading: false, error: ''}) : ($adminQueryHistory.loading = true, $adminQueryHistory.error = ''); "+postAction("/admin/queries/command")),
		)
	}
	adminChildren := []g.Node{}
	if active == "agent" {
		promptAttrs := []g.Node{
			g.Attr("slot", "agent-prompt"),
			g.Attr("value", data.Agent.SystemPrompt),
			g.Attr("data-attr:value", "$adminAgentCommand.systemPrompt"),
		}
		if !data.Agent.CanWrite {
			promptAttrs = append(promptAttrs, g.Attr("disabled", ""))
		}
		adminChildren = append(adminChildren, g.El("ld-agent-prompt-editor", promptAttrs...))
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
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/admin-page.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/admin-page.js"))),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		),
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				ds.Signals(signals),
				g.If(active == "storage", ds.Init("@get('/admin/storage/updates', {openWhenHidden: true})")),
				g.If(active == "queries", ds.Init("@get('/admin/queries/updates', {openWhenHidden: true})")),
				g.El("ld-app-shell",
					g.Attr("chrome", jsonString(chrome)),
					g.Attr("data-attr:chrome", "JSON.stringify($chrome)"),
					g.El("ld-admin-page", append(adminAttrs, adminChildren...)...),
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
		page.Sections = []uisignals.AdminContentSectionSignal{{Title: "Principals", Table: adminPrincipalsGrid(data.Principals)}}
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
		page.Sections = []uisignals.AdminContentSectionSignal{{Title: "Groups", Table: adminPrincipalGroupsGrid(principal, data.Groups)}}
	case "groups":
		page.HeaderTitle = "Groups"
		page.HeaderDetail = "Workspace groups and their read-only membership summaries."
		page.Sections = []uisignals.AdminContentSectionSignal{{Title: "Groups", Table: adminGroupsGrid(data.Groups)}}
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
		page.Sections = []uisignals.AdminContentSectionSignal{{Title: "Members", Table: adminGroupMembersGrid(group, data.Principals)}}
	case "agent":
		page.HeaderTitle = "Agent"
		page.HeaderDetail = "Platform agent prompt and read-only tool inventory."
		page.Agent = adminAgentSignal(data.Agent)
		page.Metrics = []uisignals.AdminMetricSignal{
			{Label: "Status", Value: configuredLabel(data.Agent.Enabled)},
			{Label: "Model", Value: data.Agent.Model},
			{Label: "Tools", Value: fmt.Sprint(len(data.Agent.Tools))},
		}
	case "storage":
		page.HeaderTitle = "Storage"
		page.HeaderDetail = "Read-only DuckLake catalog and table metadata."
		page.Storage = AdminStorageSignalFromData(data.Storage, AdminStorageCommand{})
		if data.Storage.Status != "" {
			page.Empty = data.Storage.Status
		}
		page.Metrics = []uisignals.AdminMetricSignal{
			{Label: "Catalog path", Value: data.Storage.CatalogPath},
			{Label: "Data path", Value: data.Storage.DataPath},
			{Label: "Snapshots", Value: fmt.Sprint(data.Storage.SnapshotCount)},
			{Label: "Tables", Value: fmt.Sprint(data.Storage.TableCount)},
		}
	case "queries":
		page.HeaderTitle = "Query History"
		page.HeaderDetail = "Product query audit across dashboards, API, agents, and Data Explorer."
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

func AdminQueryHistorySignalFromData(data AdminQueryHistoryData) uisignals.AdminQueryHistorySignal {
	limit := data.Limit
	if limit <= 0 {
		limit = 50
	}
	return uisignals.AdminQueryHistorySignal{
		Table:            adminQueryEventsGrid(data.Events),
		FilterMenus:      data.FilterMenus,
		Filters:          data.Filters,
		NextCursor:       data.NextCursor,
		LoadedCountLabel: queryHistoryCountLabel(len(data.Events)),
		HasMore:          data.HasMore,
		Loading:          false,
		Error:            data.Error,
		Limit:            limit,
	}
}

func AdminQueryDetailSignalFromEvent(event AdminQueryEvent) uisignals.AdminQueryDetailSignal {
	return uisignals.AdminQueryDetailSignal{
		EventID:       event.ID,
		Loading:       false,
		Error:         event.Error,
		Status:        event.Status,
		StatusLabel:   queryEventStatusLabel(event.Status),
		WorkspaceID:   event.WorkspaceID,
		PrincipalID:   event.PrincipalID,
		Surface:       event.Surface,
		Operation:     event.Operation,
		QueryKind:     event.QueryKind,
		ModelID:       event.ModelID,
		Target:        event.Target,
		ObjectType:    event.ObjectType,
		ObjectID:      event.ObjectID,
		RequestID:     event.RequestID,
		CorrelationID: event.CorrelationID,
		DurationMS:    event.DurationMS,
		RowsReturned:  event.RowsReturned,
		QueryError:    event.Error,
		SQL:           event.SQL,
		PlanText:      event.PlanText,
		QueryJSON:     event.QueryJSON,
		CreatedAt:     event.CreatedAt,
	}
}

func queryEventStatusLabel(status string) string {
	switch status {
	case "success":
		return "Success"
	case "canceled":
		return "Canceled"
	case "timeout":
		return "Timeout"
	case "validation_failed":
		return "Validation failed"
	case "":
		return "Unknown"
	default:
		return status
	}
}

func queryHistoryCountLabel(count int) string {
	if count == 1 {
		return "1 query loaded"
	}
	return fmt.Sprintf("%d queries loaded", count)
}

func adminQueryEventSignals(events []AdminQueryEvent) []uisignals.AdminQueryEventSignal {
	out := make([]uisignals.AdminQueryEventSignal, 0, len(events))
	for _, event := range events {
		out = append(out, uisignals.AdminQueryEventSignal{
			ID:            event.ID,
			WorkspaceID:   event.WorkspaceID,
			PrincipalID:   event.PrincipalID,
			Surface:       event.Surface,
			Operation:     event.Operation,
			QueryKind:     event.QueryKind,
			ModelID:       event.ModelID,
			Target:        event.Target,
			ObjectType:    event.ObjectType,
			ObjectID:      event.ObjectID,
			RequestID:     event.RequestID,
			CorrelationID: event.CorrelationID,
			Status:        event.Status,
			DurationMS:    event.DurationMS,
			RowsReturned:  event.RowsReturned,
			Error:         event.Error,
			SQL:           event.SQL,
			PlanText:      event.PlanText,
			QueryJSON:     event.QueryJSON,
			CreatedAt:     event.CreatedAt,
		})
	}
	return out
}

func adminSidebarSignal(active string) uisignals.SubSidebarSignal {
	principalsActive := active == "principals" || active == "principal-detail"
	groupsActive := active == "groups" || active == "group-detail"
	agentActive := active == "agent"
	storageActive := active == "storage"
	queriesActive := active == "queries"
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
			{ID: "agent", Title: "Agent", Href: "/admin/agent", Active: agentActive},
			{ID: "storage", Title: "Storage", Href: "/admin/storage", Active: storageActive},
			{ID: "queries", Title: "Query History", Href: "/admin/queries", Active: queriesActive},
		},
	}
}

func adminAgentSignal(data AdminAgentData) uisignals.AdminAgentSignal {
	tools := make([]uisignals.AdminAgentToolSignal, 0, len(data.Tools))
	for _, tool := range data.Tools {
		tools = append(tools, uisignals.AdminAgentToolSignal{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return uisignals.AdminAgentSignal{
		Enabled:      data.Enabled,
		Model:        data.Model,
		SystemPrompt: data.SystemPrompt,
		CanWrite:     data.CanWrite,
		CSRFToken:    data.CSRFToken,
		UpdatePath:   data.UpdatePath,
		Tools:        tools,
	}
}

func adminPrincipalsGrid(principals []AdminPrincipal) recordTable {
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
	return recordTable{
		Columns: []recordTableColumn{
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

func adminPrincipalGroupsGrid(principal AdminPrincipal, groups []AdminGroup) recordTable {
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
	return recordTable{
		Columns: []recordTableColumn{
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

func adminGroupsGrid(groups []AdminGroup) recordTable {
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
	return recordTable{
		Columns: []recordTableColumn{
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

func adminGroupMembersGrid(group AdminGroup, principals []AdminPrincipal) recordTable {
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
	return recordTable{
		Columns: []recordTableColumn{
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

func adminQueryEventsGrid(events []AdminQueryEvent) recordTable {
	rows := make([]map[string]any, 0, len(events))
	for _, event := range events {
		rows = append(rows, map[string]any{
			"id": event.ID,
			"query": map[string]any{
				"label":           queryEventStatement(event),
				"statusLabel":     event.Status,
				"tone":            queryEventStatusTone(event.Status),
				"icon":            queryEventStatusIcon(event.Status),
				"expandedContent": queryEventExpandedContent(event),
			},
			"started_at":     event.CreatedAt,
			"duration_ms":    map[string]any{"label": fmt.Sprintf("%d ms", event.DurationMS), "value": event.DurationMS},
			"source":         event.Surface,
			"runtime":        queryEventRuntimeLabel(event),
			"principal_id":   event.PrincipalID,
			"rows_returned":  event.RowsReturned,
			"operation":      event.Operation,
			"kind":           event.QueryKind,
			"model":          event.ModelID,
			"target":         event.Target,
			"object":         queryEventObjectLabel(event),
			"request_id":     event.RequestID,
			"correlation_id": event.CorrelationID,
			"error":          event.Error,
		})
	}
	falseValue := false
	return recordTable{
		Columns: []recordTableColumn{
			{ID: "query", Header: "Query", Kind: "query", Width: "560px", Toggleable: &falseValue},
			{ID: "started_at", Header: "Started", Width: "150px"},
			{ID: "duration_ms", Header: "Duration", Kind: "number", Align: "right", Width: "105px"},
			{ID: "source", Header: "Source type", Width: "120px"},
			{ID: "runtime", Header: "Runtime", Kind: "code", Width: "130px"},
			{ID: "principal_id", Header: "User", Kind: "code", Width: "150px"},
			{ID: "rows_returned", Header: "Rows", Kind: "number", Align: "right", Width: "90px"},
			{ID: "operation", Header: "Operation", Kind: "code", Width: "145px"},
			{ID: "kind", Header: "Kind", Kind: "code", Width: "170px"},
			{ID: "model", Header: "Model", Kind: "code", Width: "130px"},
			{ID: "target", Header: "Target", Kind: "code", Width: "150px"},
			{ID: "object", Header: "Object", Kind: "code", Width: "220px"},
			{ID: "request_id", Header: "Request ID", Kind: "code", Width: "170px"},
			{ID: "correlation_id", Header: "Correlation ID", Kind: "code", Width: "170px"},
			{ID: "error", Header: "Error", Kind: "code", Width: "220px"},
		},
		Rows:      rows,
		Empty:     "No query events match these filters.",
		MinWidth:  "1305px",
		Density:   "tight",
		RowAction: "detail",
		ColumnSelector: &uisignals.RecordTableColumnSelector{
			Enabled:        true,
			Label:          "Columns",
			DefaultColumns: []string{"started_at", "duration_ms", "source", "runtime", "principal_id", "rows_returned"},
		},
	}
}

func queryEventStatement(event AdminQueryEvent) string {
	sql := collapseWhitespace(event.SQL)
	if sql != "" {
		return sql
	}
	parts := []string{event.Operation, event.QueryKind, strings.Join(nonEmptyStrings(event.ModelID, event.Target), ".")}
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		if label := collapseWhitespace(part); label != "" {
			labels = append(labels, label)
		}
	}
	if len(labels) > 0 {
		return strings.Join(labels, " · ")
	}
	return event.ID
}

func queryEventExpandedContent(event AdminQueryEvent) string {
	if event.SQL != "" {
		return event.SQL
	}
	return queryEventStatement(event)
}

func queryEventObjectLabel(event AdminQueryEvent) string {
	object := strings.Join(nonEmptyStrings(event.ObjectType, event.ObjectID), ":")
	if object != "" {
		return object
	}
	object = strings.Join(nonEmptyStrings(event.ModelID, event.Target), ":")
	if object != "" {
		return object
	}
	return "-"
}

func queryEventRuntimeLabel(event AdminQueryEvent) string {
	if event.WorkspaceID == "" {
		return "-"
	}
	return event.WorkspaceID
}

func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func queryEventStatusTone(status string) string {
	switch status {
	case "success":
		return "success"
	case "canceled":
		return "muted"
	case "timeout":
		return "attention"
	default:
		return "danger"
	}
}

func queryEventStatusIcon(status string) string {
	switch status {
	case "success":
		return "check"
	case "canceled", "timeout":
		return "clock"
	default:
		return "x"
	}
}

func adminQueryMetrics(events []AdminQueryEvent) []uisignals.AdminMetricSignal {
	failures := 0
	totalDuration := int64(0)
	for _, event := range events {
		if event.Status != "success" {
			failures++
		}
		totalDuration += event.DurationMS
	}
	avg := int64(0)
	if len(events) > 0 {
		avg = totalDuration / int64(len(events))
	}
	return []uisignals.AdminMetricSignal{
		{Label: "Recent events", Value: fmt.Sprint(len(events))},
		{Label: "Failures", Value: fmt.Sprint(failures)},
		{Label: "Average duration", Value: fmt.Sprintf("%d ms", avg)},
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
	case "agent":
		return "Agent"
	case "storage":
		return "Storage"
	case "queries":
		return "Query History"
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
			CatalogPath:        data.CatalogPath,
			DataPath:           data.DataPath,
			CatalogSizeLabel:   data.CatalogSizeLabel,
			DataSizeLabel:      data.DataSizeLabel,
			TotalSizeLabel:     data.TotalSizeLabel,
			TotalDataSizeLabel: data.TotalDataSizeLabel,
			DatabaseCount:      data.DatabaseCount,
			TableCount:         data.TableCount,
			SnapshotCount:      data.SnapshotCount,
			DataFileCount:      data.DataFileCount,
		},
		Status:        data.Status,
		Warnings:      data.Warnings,
		Tables:        tables,
		Snapshots:     adminStorageSnapshotSignals(data.Snapshots),
		Deployments:   adminStorageDeploymentSignals(data.Deployments),
		SelectedKey:   selectedKey,
		SelectedTable: selected,
	}
}

func AdminStorageTableSignalFromTable(table AdminStorageTable) AdminStorageTableSignal {
	columns := make([]AdminStorageColumnSignal, 0, len(table.Columns))
	for _, column := range table.Columns {
		columns = append(columns, AdminStorageColumnSignal{
			ID:                  column.ID,
			Name:                column.Name,
			Type:                column.Type,
			Ordinal:             column.Ordinal,
			Nullable:            column.Nullable,
			Default:             column.Default,
			InitialDefault:      column.InitialDefault,
			DefaultValueType:    column.DefaultValueType,
			DefaultValueDialect: column.DefaultValueDialect,
			BeginSnapshot:       column.BeginSnapshot,
			ContainsNull:        column.ContainsNull,
			ContainsNaN:         column.ContainsNaN,
			MinValue:            column.MinValue,
			MaxValue:            column.MaxValue,
			ExtraStats:          column.ExtraStats,
		})
	}
	files := make([]AdminStorageFileSignal, 0, len(table.Files))
	for _, file := range table.Files {
		files = append(files, AdminStorageFileSignal{
			ID:               file.ID,
			Path:             file.Path,
			Format:           file.Format,
			RecordCount:      file.RecordCount,
			RecordCountLabel: file.RecordCountLabel,
			SizeBytes:        file.SizeBytes,
			SizeLabel:        file.SizeLabel,
			BeginSnapshot:    file.BeginSnapshot,
			EndSnapshot:      file.EndSnapshot,
		})
	}
	history := make([]AdminStorageTableHistorySignal, 0, len(table.History))
	for _, event := range table.History {
		history = append(history, AdminStorageTableHistorySignal{
			SnapshotID:    event.SnapshotID,
			Time:          event.Time,
			SchemaVersion: event.SchemaVersion,
			Source:        event.Source,
			Changes:       event.Changes,
			Author:        event.Author,
			Message:       event.Message,
			ExtraInfo:     event.ExtraInfo,
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
		TableID:       table.TableID,
		TableUUID:     table.TableUUID,
		DuckLakePath:  table.DuckLakePath,
		BeginSnapshot: table.BeginSnapshot,
		EndSnapshot:   table.EndSnapshot,
		RowCount:      table.RowCount,
		RowCountLabel: table.RowCountLabel,
		ColumnCount:   table.ColumnCount,
		FileCount:     table.FileCount,
		SizeBytes:     table.SizeBytes,
		SizeLabel:     table.SizeLabel,
		Columns:       columns,
		Files:         files,
		History:       history,
		Deployments:   adminStorageDeploymentSignals(table.Deployments),
	}
}

func adminStorageSnapshotSignals(snapshots []AdminStorageSnapshot) []AdminStorageSnapshotSignal {
	out := make([]AdminStorageSnapshotSignal, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, AdminStorageSnapshotSignal{
			ID:              snapshot.ID,
			Time:            snapshot.Time,
			SchemaVersion:   snapshot.SchemaVersion,
			Author:          snapshot.Author,
			Message:         snapshot.Message,
			Changes:         snapshot.Changes,
			ExtraInfo:       snapshot.ExtraInfo,
			Protected:       snapshot.Protected,
			DeploymentCount: snapshot.DeploymentCount,
		})
	}
	return out
}

func adminStorageDeploymentSignals(deployments []AdminStorageDeployment) []AdminStorageDeploymentSignal {
	out := make([]AdminStorageDeploymentSignal, 0, len(deployments))
	for _, deployment := range deployments {
		out = append(out, AdminStorageDeploymentSignal{
			WorkspaceID:  deployment.WorkspaceID,
			Environment:  deployment.Environment,
			DeploymentID: deployment.DeploymentID,
			Status:       deployment.Status,
			SnapshotID:   deployment.SnapshotID,
			Digest:       deployment.Digest,
			Active:       deployment.Active,
			ActivatedAt:  deployment.ActivatedAt,
		})
	}
	return out
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
