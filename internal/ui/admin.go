package ui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Yacobolo/leapview/internal/dashboard"
	uiactions "github.com/Yacobolo/leapview/internal/ui/actions"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
	workspaceview "github.com/Yacobolo/leapview/internal/workspace"
	"github.com/Yacobolo/leapview/pkg/pagestream"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type AdminData struct {
	Workspace         workspaceview.WorkspaceView
	CSRFToken         string
	AuthConfigured    bool
	AccessConfigured  bool
	AccessStatusLabel string
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
	Name         string
	Description  string
	Effect       string
	Defaults     map[string]any
	InputSchema  map[string]any
	OutputSchema map[string]any
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
	ID               string
	WorkspaceID      string
	PrincipalID      string
	Surface          string
	Operation        string
	QueryKind        string
	ModelID          string
	Target           string
	ObjectType       string
	ObjectID         string
	RequestID        string
	CorrelationID    string
	Status           string
	DurationMS       int64
	PlanningMS       int64
	ConnectionWaitMS int64
	DatabaseMS       int64
	RowsReturned     int
	Error            string
	SQL              string
	PlanText         string
	QueryJSON        string
	CreatedAt        string
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
type AdminStorageServingState = uisignals.AdminStorageServingState
type AdminStorageSignal = uisignals.AdminStorageSignal
type AdminStorageSummary = uisignals.AdminStorageSummary
type AdminStorageTableSignal = uisignals.AdminStorageTableSignal
type AdminStorageColumnSignal = uisignals.AdminStorageColumnSignal
type AdminStorageFileSignal = uisignals.AdminStorageFileSignal
type AdminStorageTableHistorySignal = uisignals.AdminStorageTableHistorySignal
type AdminStorageSnapshotSignal = uisignals.AdminStorageSnapshotSignal
type AdminStorageServingStateSignal = uisignals.AdminStorageServingStateSignal
type AdminStorageCommand = uisignals.AdminStorageCommand

func AdminPage(catalog dashboard.Catalog, active, roleLabel string, data AdminData, chromeOptions ...ChromeOption) g.Node {
	title := adminPageTitle(active)
	page := adminPageSignal(active, data)
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForWorkspace(catalog, "admin", roleLabel)}
	applyChromeOptions(&chrome, chromeOptions)
	storageSignal := page.Storage
	adminUpdatesURL := updatesURL(uisignals.RouteAdmin, "section", active)
	if active == "principal-detail" && data.SelectedPrincipal != nil {
		adminUpdatesURL = updatesURL(uisignals.RouteAdmin, "section", active, "principal", data.SelectedPrincipal.ID)
	}
	if active == "group-detail" && data.SelectedGroup != nil {
		adminUpdatesURL = updatesURL(uisignals.RouteAdmin, "section", active, "group", data.SelectedGroup.ID)
	}
	_ = chrome
	_ = storageSignal
	adminAttrs := []g.Node{
		g.Attr("slot", "page"),
		g.Attr("section", active),
	}
	if active == "storage" {
		adminAttrs = append(adminAttrs,
			g.Attr("data-on:lv-storage-table-select", "$adminStorageCommand = evt.detail; "+uiactions.Post("/admin/storage/select-table")),
		)
	}
	if active == "agent" {
		adminAttrs = append(adminAttrs,
			g.Attr("data-on:lv-agent-system-prompt-save", "$adminAgentCommand = evt.detail; "+uiactions.Patch("/admin/agent/config")),
		)
	}
	if active == "queries" {
		adminAttrs = append(adminAttrs,
			g.Attr("data-on:lv-query-history-command", "$adminQueryHistoryCommand = evt.detail; evt.detail.action == 'select_detail' ? ($adminQueryDetail = {eventId: evt.detail.eventId, loading: true, error: ''}) : evt.detail.action == 'close_detail' ? ($adminQueryDetail = {eventId: '', loading: false, error: ''}) : ($adminQueryHistory.loading = true, $adminQueryHistory.error = ''); "+uiactions.Post("/admin/queries/command")),
		)
	}
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:             "Admin - " + title,
		DatastarScriptURL: datastarScriptURL(),
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			csrfMeta(data.CSRFToken),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/admin-page.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/admin-page.js"))),
			inspectorScript(),
		),
		MainAttrs:  []g.Node{h.Class(appRootClass)},
		UpdatesURL: adminUpdatesURL,
		Body: []g.Node{
			g.El("lv-app-shell",
				g.El("lv-admin-page", adminAttrs...),
			),
			inspectorElement(),
		},
	})
}

func AdminBootstrapSignals(catalog dashboard.Catalog, active, roleLabel string, data AdminData, chromeOptions ...ChromeOption) map[string]any {
	page := adminPageSignal(active, data)
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForWorkspace(catalog, "admin", roleLabel)}
	applyChromeOptions(&chrome, chromeOptions)
	signals := map[string]any{
		"chrome":  chrome,
		"page":    page,
		"runtime": uisignals.RouteRuntimeSignal{Kind: uisignals.RouteAdmin},
		"status":  dashboard.Status{},
	}
	if active == "agent" {
		signals["adminAgentCommand"] = map[string]string{"systemPrompt": data.Agent.SystemPrompt}
	}
	if active == "storage" {
		signals["adminStorage"] = page.Storage
		signals["adminStorageCommand"] = AdminStorageCommand{}
	}
	if active == "queries" {
		queryHistory := AdminQueryHistorySignalFromData(data.QueryHistory)
		signals["adminQueryHistory"] = queryHistory
		signals["adminQueryDetail"] = uisignals.AdminQueryDetailSignal{}
		signals["adminQueryHistoryCommand"] = uisignals.AdminQueryHistoryCommand{Action: "load_more", Filters: queryHistory.Filters, PageToken: uisignals.Optional(queryHistory.NextCursor), Limit: uisignals.Pointer(queryHistory.Limit)}
	}
	return signals
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
		page.HeaderDetail = "Users and service principals known to LeapView."
		page.Sections = uisignals.OptionalSlice([]uisignals.AdminContentSectionSignal{{Title: "Principals", Table: uisignals.Pointer(adminPrincipalsGrid(data.Principals))}})
	case "principal-detail":
		page.HeaderTitle = "Principals"
		page.HeaderDetail = "Read-only principal access."
		if data.SelectedPrincipal == nil {
			page.Empty = uisignals.Pointer("Principal not found.")
			return page
		}
		principal := *data.SelectedPrincipal
		name := adminDisplayLabel(principal.DisplayName, principal.Email, principal.ID)
		page.HeaderTitle = "Principals / " + name
		page.HeaderDetail = "Read-only principal identity and group memberships."
		page.Metrics = uisignals.OptionalSlice([]uisignals.AdminMetricSignal{
			{Label: "Email", Value: principal.Email},
			{Label: "Principal ID", Value: principal.ID},
			{Label: "Direct roles", Value: strings.Join(principal.DirectRoles, ", ")},
			{Label: "Group count", Value: fmt.Sprint(len(principal.Groups))},
			{Label: "Created", Value: principal.CreatedAt},
			{Label: "Updated", Value: principal.UpdatedAt},
		})
		page.Sections = uisignals.OptionalSlice([]uisignals.AdminContentSectionSignal{{Title: "Groups", Table: uisignals.Pointer(adminPrincipalGroupsGrid(principal, data.Groups))}})
	case "groups":
		page.HeaderTitle = "Groups"
		page.HeaderDetail = "Workspace groups and their read-only membership summaries."
		page.Sections = uisignals.OptionalSlice([]uisignals.AdminContentSectionSignal{{Title: "Groups", Table: uisignals.Pointer(adminGroupsGrid(data.Groups))}})
	case "group-detail":
		page.HeaderTitle = "Groups"
		page.HeaderDetail = "Read-only group membership."
		if data.SelectedGroup == nil {
			page.Empty = uisignals.Pointer("Group not found.")
			return page
		}
		group := *data.SelectedGroup
		name := adminDisplayLabel(group.Name, group.ExternalID, group.ID)
		page.HeaderTitle = "Groups / " + name
		page.HeaderDetail = "Read-only group membership and role assignments."
		page.Metrics = uisignals.OptionalSlice([]uisignals.AdminMetricSignal{
			{Label: "Provider", Value: group.Provider},
			{Label: "External ID", Value: group.ExternalID},
			{Label: "Group ID", Value: group.ID},
			{Label: "Roles", Value: strings.Join(group.Roles, ", ")},
			{Label: "Member count", Value: fmt.Sprint(len(group.Members))},
		})
		page.Sections = uisignals.OptionalSlice([]uisignals.AdminContentSectionSignal{{Title: "Members", Table: uisignals.Pointer(adminGroupMembersGrid(group, data.Principals))}})
	case "agent":
		page.HeaderTitle = "Agent"
		page.HeaderDetail = "Platform agent prompt and read-only tool inventory."
		page.Agent = uisignals.Pointer(adminAgentSignal(data.Agent))
		page.Metrics = uisignals.OptionalSlice([]uisignals.AdminMetricSignal{
			{Label: "Status", Value: configuredLabel(data.Agent.Enabled)},
			{Label: "Model", Value: data.Agent.Model},
			{Label: "Tools", Value: fmt.Sprint(len(data.Agent.Tools))},
		})
	case "storage":
		page.HeaderTitle = "Storage"
		page.HeaderDetail = "Read-only DuckLake catalog and table metadata."
		page.Storage = uisignals.Pointer(AdminStorageSignalFromData(data.Storage, AdminStorageCommand{}))
		if data.Storage.Status != "" {
			page.Empty = uisignals.Pointer(data.Storage.Status)
		}
		page.Metrics = uisignals.OptionalSlice([]uisignals.AdminMetricSignal{
			{Label: "Catalog path", Value: data.Storage.CatalogPath},
			{Label: "Data path", Value: data.Storage.DataPath},
			{Label: "Snapshots", Value: fmt.Sprint(data.Storage.SnapshotCount)},
			{Label: "Tables", Value: fmt.Sprint(data.Storage.TableCount)},
		})
	case "queries":
		page.HeaderTitle = "Query History"
		page.HeaderDetail = "Product query audit across dashboards, API, agents, and Data Explorer."
	default:
		page.HeaderTitle = "General"
		page.HeaderDetail = "Read-only workspace administration."
		if !data.AccessConfigured {
			page.Empty = uisignals.Optional(data.AccessStatusLabel)
		}
		page.Metrics = uisignals.OptionalSlice([]uisignals.AdminMetricSignal{
			{Label: "Workspace", Value: data.Workspace.Title, Detail: uisignals.Optional(data.Workspace.ID)},
			{Label: "Auth", Value: configuredLabel(data.AuthConfigured)},
			{Label: "Access", Value: data.AccessStatusLabel},
			{Label: "Principals", Value: fmt.Sprint(data.PrincipalCount)},
			{Label: "Groups", Value: fmt.Sprint(data.GroupCount)},
			{Label: "Role bindings", Value: fmt.Sprint(data.BindingCount)},
			{Label: "Roles", Value: fmt.Sprint(data.RoleCount)},
		})
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
		FilterMenus:      uisignals.OptionalSlice(data.FilterMenus),
		Filters:          data.Filters,
		NextCursor:       data.NextCursor,
		LoadedCountLabel: queryHistoryCountLabel(len(data.Events)),
		HasMore:          data.HasMore,
		Loading:          false,
		Error:            data.Error,
		Limit:            int64(limit),
	}
}

func AdminQueryDetailSignalFromEvent(event AdminQueryEvent) uisignals.AdminQueryDetailSignal {
	return uisignals.AdminQueryDetailSignal{
		EventID:          uisignals.Optional(event.ID),
		Loading:          false,
		Error:            uisignals.Optional(event.Error),
		Status:           uisignals.Optional(event.Status),
		StatusLabel:      uisignals.Optional(queryEventStatusLabel(event.Status)),
		WorkspaceID:      uisignals.Optional(event.WorkspaceID),
		PrincipalID:      uisignals.Optional(event.PrincipalID),
		Surface:          uisignals.Optional(event.Surface),
		Operation:        uisignals.Optional(event.Operation),
		QueryKind:        uisignals.Optional(event.QueryKind),
		ModelID:          uisignals.Optional(event.ModelID),
		Target:           uisignals.Optional(event.Target),
		ObjectType:       uisignals.Optional(event.ObjectType),
		ObjectID:         uisignals.Optional(event.ObjectID),
		RequestID:        uisignals.Optional(event.RequestID),
		CorrelationID:    uisignals.Optional(event.CorrelationID),
		DurationMS:       event.DurationMS,
		PlanningMS:       event.PlanningMS,
		ConnectionWaitMS: event.ConnectionWaitMS,
		DatabaseMS:       event.DatabaseMS,
		RowsReturned:     int64(event.RowsReturned),
		QueryError:       uisignals.Optional(event.Error),
		SQL:              uisignals.Optional(event.SQL),
		PlanText:         uisignals.Optional(event.PlanText),
		QueryJSON:        uisignals.Optional(event.QueryJSON),
		CreatedAt:        uisignals.Optional(event.CreatedAt),
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
			ID:               event.ID,
			WorkspaceID:      event.WorkspaceID,
			PrincipalID:      event.PrincipalID,
			Surface:          event.Surface,
			Operation:        event.Operation,
			QueryKind:        event.QueryKind,
			ModelID:          event.ModelID,
			Target:           event.Target,
			ObjectType:       event.ObjectType,
			ObjectID:         event.ObjectID,
			RequestID:        event.RequestID,
			CorrelationID:    event.CorrelationID,
			Status:           event.Status,
			DurationMS:       event.DurationMS,
			PlanningMS:       event.PlanningMS,
			ConnectionWaitMS: event.ConnectionWaitMS,
			DatabaseMS:       event.DatabaseMS,
			RowsReturned:     int64(event.RowsReturned),
			Error:            event.Error,
			SQL:              event.SQL,
			PlanText:         event.PlanText,
			QueryJSON:        event.QueryJSON,
			CreatedAt:        event.CreatedAt,
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
		StorageKey:  "leapview-admin-sidebar-collapsed",
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
			Name:         tool.Name,
			Description:  tool.Description,
			Effect:       tool.Effect,
			Defaults:     tool.Defaults,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
		})
	}
	return uisignals.AdminAgentSignal{
		Enabled:      data.Enabled,
		Model:        uisignals.Optional(data.Model),
		SystemPrompt: data.SystemPrompt,
		CanWrite:     data.CanWrite,
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
			{ID: "name", Header: "Name", Kind: uisignals.Pointer("link"), HrefKey: uisignals.Pointer("name_href"), Width: uisignals.Pointer("150px")},
			{ID: "email", Header: "Email", Width: uisignals.Pointer("190px")},
			{ID: "roles", Header: "Direct roles", Kind: uisignals.Pointer("tags"), Width: uisignals.Pointer("135px")},
			{ID: "group_count", Header: "Group count", Kind: uisignals.Pointer("number"), Align: uisignals.Pointer("right"), Width: uisignals.Pointer("120px")},
			{ID: "id", Header: "Principal ID", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("190px")},
			{ID: "updated_at", Header: "Updated", Width: uisignals.Pointer("150px")},
		},
		Rows:     rows,
		Empty:    "No principals found.",
		MinWidth: uisignals.Pointer("935px"),
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
			{ID: "name", Header: "Name", Kind: uisignals.Pointer("link"), HrefKey: uisignals.Pointer("name_href"), Width: uisignals.Pointer("180px")},
			{ID: "provider", Header: "Provider", Width: uisignals.Pointer("120px")},
			{ID: "external_id", Header: "External ID", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("180px")},
			{ID: "roles", Header: "Roles", Kind: uisignals.Pointer("tags"), Width: uisignals.Pointer("160px")},
			{ID: "member_count", Header: "Member count", Kind: uisignals.Pointer("number"), Align: uisignals.Pointer("right"), Width: uisignals.Pointer("130px")},
		},
		Rows:     rows,
		Empty:    "No groups found.",
		MinWidth: uisignals.Pointer("800px"),
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
			{ID: "name", Header: "Name", Kind: uisignals.Pointer("link"), HrefKey: uisignals.Pointer("name_href"), Width: uisignals.Pointer("180px")},
			{ID: "provider", Header: "Provider", Width: uisignals.Pointer("120px")},
			{ID: "external_id", Header: "External ID", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("180px")},
			{ID: "roles", Header: "Roles", Kind: uisignals.Pointer("tags"), Width: uisignals.Pointer("180px")},
			{ID: "member_count", Header: "Member count", Kind: uisignals.Pointer("number"), Align: uisignals.Pointer("right"), Width: uisignals.Pointer("130px")},
			{ID: "id", Header: "Group ID", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("220px")},
		},
		Rows:     rows,
		Empty:    "No groups found.",
		MinWidth: uisignals.Pointer("1010px"),
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
			{ID: "name", Header: "Name", Width: uisignals.Pointer("150px")},
			{ID: "email", Header: "Email", Width: uisignals.Pointer("190px")},
			{ID: "id", Header: "Principal ID", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("180px")},
			{ID: "direct_roles", Header: "Direct roles", Kind: uisignals.Pointer("tags"), Width: uisignals.Pointer("130px")},
			{ID: "updated_at", Header: "Updated", Width: uisignals.Pointer("150px")},
		},
		Rows:     rows,
		Empty:    "No members found.",
		MinWidth: uisignals.Pointer("840px"),
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
			{ID: "query", Header: "Query", Kind: uisignals.Pointer("query"), Width: uisignals.Pointer("560px"), Toggleable: &falseValue},
			{ID: "started_at", Header: "Started", Width: uisignals.Pointer("150px")},
			{ID: "duration_ms", Header: "Duration", Kind: uisignals.Pointer("number"), Align: uisignals.Pointer("right"), Width: uisignals.Pointer("105px")},
			{ID: "source", Header: "Source type", Width: uisignals.Pointer("120px")},
			{ID: "runtime", Header: "Runtime", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("130px")},
			{ID: "principal_id", Header: "User", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("150px")},
			{ID: "rows_returned", Header: "Rows", Kind: uisignals.Pointer("number"), Align: uisignals.Pointer("right"), Width: uisignals.Pointer("90px")},
			{ID: "operation", Header: "Operation", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("145px")},
			{ID: "kind", Header: "Kind", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("170px")},
			{ID: "model", Header: "Model", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("130px")},
			{ID: "target", Header: "Target", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("150px")},
			{ID: "object", Header: "Object", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("220px")},
			{ID: "request_id", Header: "Request ID", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("170px")},
			{ID: "correlation_id", Header: "Correlation ID", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("170px")},
			{ID: "error", Header: "Error", Kind: uisignals.Pointer("code"), Width: uisignals.Pointer("220px")},
		},
		Rows:      rows,
		Empty:     "No query events match these filters.",
		MinWidth:  uisignals.Pointer("1305px"),
		Density:   uisignals.Pointer("tight"),
		RowAction: uisignals.Pointer("detail"),
		ColumnSelector: &uisignals.RecordTableColumnSelector{
			Enabled:        true,
			Label:          uisignals.Pointer("Columns"),
			DefaultColumns: uisignals.Pointer([]string{"started_at", "duration_ms", "source", "runtime", "principal_id", "rows_returned"}),
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
			DatabaseCount:      int64(data.DatabaseCount),
			TableCount:         int64(data.TableCount),
			SnapshotCount:      int64(data.SnapshotCount),
			DataFileCount:      int64(data.DataFileCount),
		},
		Status:        data.Status,
		Warnings:      data.Warnings,
		Tables:        tables,
		Snapshots:     adminStorageSnapshotSignals(data.Snapshots),
		ServingStates: adminStorageServingStateSignals(data.ServingStates),
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
			Ordinal:             int64(column.Ordinal),
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
		ColumnCount:   int64(table.ColumnCount),
		FileCount:     int64(table.FileCount),
		SizeBytes:     table.SizeBytes,
		SizeLabel:     table.SizeLabel,
		Columns:       uisignals.OptionalSlice(columns),
		Files:         uisignals.OptionalSlice(files),
		History:       uisignals.OptionalSlice(history),
		ServingStates: uisignals.OptionalSlice(adminStorageServingStateSignals(table.ServingStates)),
	}
}

func adminStorageSnapshotSignals(snapshots []AdminStorageSnapshot) []AdminStorageSnapshotSignal {
	out := make([]AdminStorageSnapshotSignal, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, AdminStorageSnapshotSignal{
			ID:                snapshot.ID,
			Time:              snapshot.Time,
			SchemaVersion:     snapshot.SchemaVersion,
			Author:            snapshot.Author,
			Message:           snapshot.Message,
			Changes:           snapshot.Changes,
			ExtraInfo:         snapshot.ExtraInfo,
			Protected:         snapshot.Protected,
			ServingStateCount: int64(snapshot.ServingStateCount),
		})
	}
	return out
}

func adminStorageServingStateSignals(servingStates []AdminStorageServingState) []AdminStorageServingStateSignal {
	out := make([]AdminStorageServingStateSignal, 0, len(servingStates))
	for _, servingState := range servingStates {
		out = append(out, AdminStorageServingStateSignal{
			WorkspaceID:    servingState.WorkspaceID,
			Environment:    servingState.Environment,
			ServingStateID: servingState.ServingStateID,
			Status:         servingState.Status,
			SnapshotID:     servingState.SnapshotID,
			Digest:         servingState.Digest,
			Active:         servingState.Active,
			ActivatedAt:    servingState.ActivatedAt,
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
