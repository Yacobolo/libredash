package signals

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/agent"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	workspaceview "github.com/Yacobolo/libredash/internal/workspace"
)

const (
	RouteDashboard       RouteKind = "dashboard"
	RouteCatalog         RouteKind = "catalog"
	RouteChat            RouteKind = "chat"
	RouteWorkspace       RouteKind = "workspace"
	RouteWorkspaceAsset  RouteKind = "workspace_asset"
	RouteConnections     RouteKind = "connections"
	RouteConnectionAsset RouteKind = "connection_asset"
	RouteData            RouteKind = "data"
	RouteAdmin           RouteKind = "admin"
	RouteLogin           RouteKind = "login"
)

type AdminStorageData struct {
	CatalogPath        string
	DataPath           string
	Status             string
	DatabaseCount      int
	CatalogSizeBytes   int64
	CatalogSizeLabel   string
	DataSizeBytes      int64
	DataSizeLabel      string
	TotalSizeBytes     int64
	TotalSizeLabel     string
	TotalDataSizeBytes int64
	TotalDataSizeLabel string
	TableCount         int
	SnapshotCount      int
	DataFileCount      int
	Databases          []AdminStorageDatabase
	Tables             []AdminStorageTable
	Snapshots          []AdminStorageSnapshot
	ServingStates      []AdminStorageServingState
	Warnings           []string
}

type AdminStorageDatabase struct {
	ID        string
	Name      string
	Path      string
	ModelID   string
	ModelName string
	SizeBytes int64
	SizeLabel string
}

type AdminStorageTable struct {
	DatabaseID    string
	DatabaseName  string
	DatabasePath  string
	ModelID       string
	ModelName     string
	Schema        string
	Name          string
	Type          string
	TableID       int64
	TableUUID     string
	DuckLakePath  string
	BeginSnapshot int64
	EndSnapshot   int64
	RowCount      int64
	RowCountLabel string
	ColumnCount   int
	FileCount     int
	SizeBytes     int64
	SizeLabel     string
	Columns       []AdminStorageColumn
	Files         []AdminStorageFile
	History       []AdminStorageTableHistory
	ServingStates []AdminStorageServingState
}

type AdminStorageColumn struct {
	ID                  int64
	Name                string
	Type                string
	Ordinal             int
	Nullable            string
	Default             string
	InitialDefault      string
	DefaultValueType    string
	DefaultValueDialect string
	BeginSnapshot       int64
	ContainsNull        string
	ContainsNaN         string
	MinValue            string
	MaxValue            string
	ExtraStats          string
}

type AdminStorageFile struct {
	ID               int64
	Path             string
	Format           string
	RecordCount      int64
	RecordCountLabel string
	SizeBytes        int64
	SizeLabel        string
	BeginSnapshot    int64
	EndSnapshot      int64
}

type AdminStorageTableHistory struct {
	SnapshotID    int64
	Time          string
	SchemaVersion int64
	Source        string
	Changes       string
	Author        string
	Message       string
	ExtraInfo     string
}

type AdminStorageSnapshot struct {
	ID                int64
	Time              string
	SchemaVersion     int64
	Author            string
	Message           string
	Changes           string
	ExtraInfo         string
	Protected         bool
	ServingStateCount int
}

type AdminStorageServingState struct {
	WorkspaceID    string
	Environment    string
	ServingStateID string
	Status         string
	SnapshotID     int64
	Digest         string
	Active         bool
	ActivatedAt    string
}

type WorkspaceAccessResponse struct {
	Workspace   workspaceview.WorkspaceView     `json:"workspace"`
	ObjectType  string                          `json:"objectType,omitempty"`
	ObjectID    string                          `json:"objectId,omitempty"`
	ObjectTitle string                          `json:"objectTitle,omitempty"`
	Mode        string                          `json:"mode,omitempty"`
	Roles       []workspaceview.RoleView        `json:"roles"`
	Bindings    []workspaceview.RoleBindingView `json:"bindings"`
	CanManage   bool                            `json:"canManage"`
	Status      WorkspaceAccessStatus           `json:"status"`
}

type ChatViewState struct {
	Agent   ChatSignal
	Visuals map[string]DashboardVisual
}

func ChatTranscriptItems(items []agent.ChatTranscriptItem) []ChatTranscriptItemSignal {
	out := make([]ChatTranscriptItemSignal, 0, len(items))
	for _, item := range items {
		out = append(out, ChatTranscriptItem(item))
	}
	return out
}

func ChatTranscriptItem(item agent.ChatTranscriptItem) ChatTranscriptItemSignal {
	out := ChatTranscriptItemSignal{
		ID:             item.ID,
		Kind:           item.Kind,
		Text:           optionalValue(item.Text),
		Markdown:       optionalValue(item.Markdown),
		ToolCallID:     optionalValue(item.ToolCallID),
		Name:           optionalValue(item.Name),
		Title:          optionalValue(item.Title),
		Status:         optionalValue(item.Status),
		Summary:        optionalValue(item.Summary),
		ResultSummary:  optionalValue(item.ResultSummary),
		InputJSON:      optionalValue(item.InputJSON),
		InputFormat:    optionalValue(item.InputFormat),
		ArgumentsJSON:  optionalValue(item.ArgumentsJSON),
		ResultJSON:     optionalValue(item.ResultJSON),
		ResultFormat:   optionalValue(item.ResultFormat),
		Error:          optionalValue(item.Error),
		ConversationID: optionalValue(item.ConversationID),
		RunID:          optionalValue(item.RunID),
		CreatedAt:      optionalValue(item.CreatedAt),
	}
	if item.Artifact != nil {
		out.Artifact = &ChatArtifactSignal{
			Type:    item.Artifact.Type,
			ID:      item.Artifact.ID,
			Summary: optionalValue(item.Artifact.Summary),
		}
	}
	return out
}

func DashboardInitialEnvelope(clientID, streamInstanceID string, catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters) DashboardEnvelope {
	activePage = activePage.WithDefaults()
	tableRequest := DefaultTableRequest(report, activePage)
	initialFilters = report.NormalizeFiltersForPage(activePage.ID, initialFilters).WithDefaults()
	modelID, modelTitle := "", ""
	if model != nil {
		modelID = model.Name
		modelTitle = model.Title
	}
	return DashboardEnvelope{
		Chrome: ChromeSignal{Sidebar: sidebarConfig(catalog, "workspaces", report.ID, workspaceDisplayTitle(catalog), report.Title, activePage.Title, modelID, modelTitle, true, "", strings.TrimSpace(catalog.Workspace.ID) != "")},
		Page: DashboardPageSignal{
			Kind:           RouteDashboard,
			Title:          report.Title,
			Description:    optionalValue(report.Description),
			DashboardID:    report.ID,
			DashboardTitle: report.Title,
			PageID:         activePage.ID,
			PageTitle:      activePage.Title,
			HeaderDetail:   ReportPageHeaderDetail(pages, activePage),
			ModelID:        modelID,
			ModelTitle:     modelTitle,
			Canvas:         DashboardPageCanvasFromDashboard(activePage.Canvas),
			Grid:           DashboardPageGridFromDashboard(activePage.Grid),
			Pages:          dashboardPageNav(catalog.Workspace.ID, report.ID, pages, activePage),
			Components:     dashboardComponents(activePage),
		},
		Runtime: RouteRuntimeSignal{
			Kind:             RouteDashboard,
			ClientID:         optionalValue(clientID),
			StreamInstanceID: optionalValue(streamInstanceID),
			DashboardID:      optionalValue(report.ID),
			PageID:           optionalValue(activePage.ID),
			ModelID:          optionalValue(modelID),
		},
		ComponentStatus:     map[string]DashboardComponentStatus{},
		FilterConfig:        ReportFilterConfigsFromReport(report.FilterConfigForPage(activePage.ID)),
		Filters:             DashboardFiltersFromDashboard(initialFilters),
		URLParams:           report.URLParamsFromFiltersForPage(activePage.ID, initialFilters),
		URLParamShape:       report.URLParamShapeForPage(activePage.ID),
		FilterOptions:       map[string][]DashboardFilterOption{},
		InteractionCommand:  DashboardInteractionCommandFromDashboard(dashboard.InteractionCommand{Toggle: true, Mappings: []dashboard.InteractionCommandMapping{}}),
		VisualWindowCommand: DashboardVisualWindowRequestFromDashboard(tableRequest),
		Visuals: DashboardVisualsFromDashboard(
			VisualSignals(report, model, activePage),
			TableSignals(report, activePage, tableRequest),
		),
		Status: DashboardStatusFromDashboard(dashboard.Status{
			Loading:       false,
			Error:         "",
			LastUpdated:   "",
			SetupRequired: false,
		}),
	}
}

func ChatInitialEnvelope(catalog dashboard.Catalog, workspaceID, roleLabel, view string, state ChatViewState) ChatEnvelope {
	chrome := ChromeSignal{Sidebar: SidebarConfigForChat(catalog, workspaceID, roleLabel, view)}
	AttachChatSidebar(&chrome.Sidebar, state.Agent)
	return ChatEnvelope{
		Chrome:  chrome,
		Page:    ChatPage(workspaceID, view, state.Agent),
		Runtime: RouteRuntimeSignal{Kind: RouteChat},
		Agent:   state.Agent,
		Visuals: state.Visuals,
	}
}

func ChatPage(workspaceID, view string, agent ChatSignal) ChatPageSignal {
	if strings.TrimSpace(view) == "" {
		view = "conversation"
	}
	return ChatPageSignal{
		Kind:        RouteChat,
		View:        view,
		Title:       "Chats",
		Description: "Ask read-only questions about dashboards, semantic models, measures, and fields.",
	}
}

func AttachChatSidebar(sidebar *SidebarSignal, agent ChatSignal) {
	if sidebar == nil {
		return
	}
	sidebar.PrimaryAction = &SidebarActionSignal{Label: "New chat", Href: chatPath("new"), Icon: "plus"}
	sidebar.History = &SidebarHistorySignal{
		Label:     "Chats",
		EmptyText: optionalValue("No chats yet."),
		Items:     ChatHistoryItems(agent),
	}
}

func ChatHistoryItems(agent ChatSignal) []SidebarHistoryItemSignal {
	items := make([]SidebarHistoryItemSignal, 0, len(agent.Conversations))
	for _, conversation := range agent.Conversations {
		title := conversation.Title
		if title == "" {
			title = "Conversation"
		}
		items = append(items, SidebarHistoryItemSignal{
			ID:      conversation.ID,
			Title:   title,
			Href:    chatPath(conversation.ID),
			Active:  conversation.ID == agent.ActiveConversationID,
			Pending: conversation.TitlePending,
		})
	}
	return items
}

func WorkspaceAccessSignals(access WorkspaceAccessResponse) WorkspaceAccessSignal {
	roles := make([]any, len(access.Roles))
	for index := range access.Roles {
		roles[index] = access.Roles[index]
	}
	bindings := make([]any, len(access.Bindings))
	for index := range access.Bindings {
		bindings[index] = access.Bindings[index]
	}
	return WorkspaceAccessSignal{
		Workspace:   access.Workspace,
		ObjectType:  optionalValue(access.ObjectType),
		ObjectID:    optionalValue(access.ObjectID),
		ObjectTitle: optionalValue(access.ObjectTitle),
		Mode:        optionalValue(access.Mode),
		Roles:       roles,
		Bindings:    bindings,
		CanManage:   access.CanManage,
		Status:      access.Status,
		Command:     WorkspaceAccessCommand{},
		Search:      "",
	}
}

func SidebarConfigForCatalog(catalog dashboard.Catalog) SidebarSignal {
	modelID, modelTitle := "", ""
	if len(catalog.Models) > 0 {
		modelID = catalog.Models[0].ID
		modelTitle = catalog.Models[0].Title
	}
	return sidebarConfig(catalog, "dashboards", "", "LibreDash", "Dashboards", "Discovery", modelID, modelTitle, false, "", false)
}

func SidebarConfigForWorkspace(catalog dashboard.Catalog, active, roleLabel string) SidebarSignal {
	return sidebarConfig(catalog, active, "", workspaceDisplayTitle(catalog), "Workspace", "Published assets", "", "", false, roleLabel, strings.TrimSpace(catalog.Workspace.ID) != "")
}

func SidebarConfigForChat(catalog dashboard.Catalog, workspaceID, roleLabel, view string) SidebarSignal {
	if strings.TrimSpace(workspaceID) != "" {
		catalog.Workspace.ID = workspaceID
	}
	active := ""
	if strings.TrimSpace(view) == "list" {
		active = "chat"
	}
	config := SidebarConfigForWorkspace(catalog, active, roleLabel)
	return config
}

func sidebarConfig(catalog dashboard.Catalog, active, dashboardID, workspaceTitle, dashboardTitle, pageTitle, modelID, modelTitle string, compact bool, roleLabel string, includeWorkspaceScoped bool) SidebarSignal {
	return SidebarSignal{
		WorkspaceTitle: workspaceTitle,
		Active:         active,
		DashboardID:    optionalValue(dashboardID),
		DashboardTitle: dashboardTitle,
		PageTitle:      pageTitle,
		ModelID:        optionalValue(modelID),
		ModelTitle:     optionalValue(modelTitle),
		Compact:        compact,
		UserRole:       optionalValue(roleLabel),
		Groups:         sidebarGroups(catalog, includeWorkspaceScoped),
	}
}

func DefaultTableRequest(report reportdef.Dashboard, page dashboard.Page) dashboard.TableRequest {
	request := dashboard.TableRequest{Block: "all", Start: 0, Count: dashboard.TableChunkSize}
	for _, name := range pageTableIDs(page) {
		table, ok := report.Tables[name]
		if !ok {
			continue
		}
		if table.KindOrDefault() == "data_table" {
			request.Table = name
			request.Sort = table.DefaultSort
			break
		}
	}
	return request
}

func TableSignals(report reportdef.Dashboard, page dashboard.Page, request dashboard.TableRequest) map[string]dashboard.Table {
	tables := map[string]dashboard.Table{}
	for _, name := range pageTableIDs(page) {
		table, ok := report.Tables[name]
		if !ok {
			continue
		}
		style := table.Style.WithDefaults()
		tables[name] = dashboard.Table{
			Version:       2,
			Kind:          table.KindOrDefault(),
			Title:         table.Title,
			Style:         style,
			Interaction:   interactionSignal("row_selection", table.Interaction.RowSelection),
			Selection:     []dashboard.InteractionSelectionEntry{},
			Columns:       table.Columns,
			Cardinality:   dashboard.TableCardinality{Kind: dashboard.CardinalityUnknown},
			AvailableRows: 0,
			IsCapped:      false,
			RowCap:        dashboard.TableInteractiveRowCap,
			ChunkSize:     dashboard.TableChunkSize,
			RowHeight:     style.RowHeight(),
			ResetVersion:  request.ResetVersion,
			Sort:          table.DefaultSort,
			Blocks: map[string]dashboard.TableBlock{
				"a": {Start: 0, Rows: []map[string]any{}},
				"b": {Start: dashboard.TableChunkSize, Rows: []map[string]any{}},
				"c": {Start: dashboard.TableChunkSize * 2, Rows: []map[string]any{}},
			},
			LoadingBlock: "",
			Error:        "",
		}
	}
	return tables
}

func VisualSignals(report reportdef.Dashboard, model *semanticmodel.Model, page dashboard.Page) map[string]dashboard.Visual {
	visuals := map[string]dashboard.Visual{}
	for _, id := range pageVisualIDs(page) {
		visual, ok := report.Visuals[id]
		if !ok {
			continue
		}
		measureName := ""
		unit := ""
		format := ""
		title := visual.Title
		if model != nil && len(visual.Query.Measures) > 0 {
			measureName = displayField(visual.Query.Measures[0].Field)
		}
		if title == "" {
			title = measureName
		}
		visuals[id] = visualSignal(id, visual, title, unit, format, measureName)
	}
	return visuals
}

func ReportPageHeaderDetail(pages []dashboard.Page, activePage dashboard.Page) string {
	title := displayLabel(activePage.Title, activePage.ID)
	for index, page := range pages {
		if page.ID == activePage.ID {
			return formatReportPageNumber(index, len(pages)) + ". " + title
		}
	}
	return title
}

func ValidateDashboardEnvelope(envelope DashboardEnvelope) error {
	if envelope.Page.Kind != RouteDashboard {
		return fmt.Errorf("dashboard envelope page kind = %q", envelope.Page.Kind)
	}
	if envelope.Runtime.Kind != RouteDashboard {
		return fmt.Errorf("dashboard envelope runtime kind = %q", envelope.Runtime.Kind)
	}
	if envelope.Page.DashboardID == "" || envelope.Page.PageID == "" {
		return fmt.Errorf("dashboard envelope requires dashboardId and pageId")
	}
	usedVisuals := map[string]struct{}{}
	usedFilters := map[string]struct{}{}
	for _, component := range envelope.Page.Components {
		switch {
		case component.Visual != nil && *component.Visual != "":
			usedVisuals[*component.Visual] = struct{}{}
			if _, ok := envelope.Visuals[*component.Visual]; !ok {
				return fmt.Errorf("component %q references missing visual %q", component.ID, *component.Visual)
			}
		case component.Filter != nil && *component.Filter != "":
			usedFilters[*component.Filter] = struct{}{}
			if !filterConfigContains(envelope.FilterConfig, *component.Filter) {
				return fmt.Errorf("component %q references missing filter config %q", component.ID, *component.Filter)
			}
			if _, ok := envelope.Filters.Controls[*component.Filter]; !ok {
				return fmt.Errorf("component %q references missing filter control %q", component.ID, *component.Filter)
			}
		}
	}
	for id := range envelope.Visuals {
		if _, ok := usedVisuals[id]; !ok {
			return fmt.Errorf("unused visual payload %q", id)
		}
	}
	return nil
}

func ValidateChatEnvelope(envelope ChatEnvelope) error {
	if envelope.Page.Kind != RouteChat {
		return fmt.Errorf("chat envelope page kind = %q", envelope.Page.Kind)
	}
	if envelope.Runtime.Kind != RouteChat {
		return fmt.Errorf("chat envelope runtime kind = %q", envelope.Runtime.Kind)
	}
	if envelope.Page.Title == "" {
		return fmt.Errorf("chat envelope requires page title")
	}
	return nil
}

func dashboardPageNav(workspaceID, reportID string, pages []dashboard.Page, activePage dashboard.Page) []DashboardPageNavSignal {
	items := make([]DashboardPageNavSignal, 0, len(pages))
	for _, page := range pages {
		items = append(items, DashboardPageNavSignal{
			ID:     page.ID,
			Title:  page.Title,
			Href:   "/workspaces/" + workspaceID + "/dashboards/" + reportID + "/pages/" + page.ID,
			Active: page.ID == activePage.ID,
		})
	}
	return items
}

func dashboardComponents(page dashboard.Page) []DashboardComponentSignal {
	components := make([]DashboardComponentSignal, 0, len(page.Visuals))
	for _, visual := range page.PlacedVisuals() {
		kind := visual.Kind
		visualID := visual.Visual
		if visual.Table != "" {
			kind = "visual"
			visualID = visual.Table
		} else if visual.Visual != "" {
			kind = "visual"
		} else if visual.Filter != "" {
			kind = "filter"
		}
		components = append(components, DashboardComponentSignal{
			ID:          visual.ID,
			Kind:        kind,
			Visual:      optionalValue(visualID),
			Filter:      optionalValue(visual.Filter),
			Description: optionalValue(visual.Description),
			Placement:   DashboardPagePlacementFromDashboard(visual.Placement),
			X:           visual.X,
			Y:           visual.Y,
			Width:       visual.Width,
			Height:      visual.Height,
			Eyebrow:     optionalValue(visual.Eyebrow),
			Title:       optionalValue(visual.Title),
			Subtitle:    optionalValue(visual.Subtitle),
			Badges:      optionalSlice(visual.Badges),
		})
	}
	return components
}

func sidebarGroups(catalog dashboard.Catalog, includeWorkspaceScoped bool) []SidebarGroupSignal {
	return []SidebarGroupSignal{
		{
			Label: "Navigation",
			Items: []SidebarItemSignal{
				{ID: "dashboards", Label: "Dashboards", Href: "/", Icon: "dashboard", Meta: optionalValue("Reports")},
				{ID: "chat", Label: "Chats", Href: chatPath(), Icon: "chat", Meta: optionalValue("Agent interface")},
				{ID: "workspaces", Label: "Workspaces", Href: "/workspaces", Icon: "catalog", Meta: optionalValue("Published assets")},
				{ID: "data", Label: "Data", Href: "/data", Icon: "cache", Meta: optionalValue("Inspect rows")},
				{ID: "connections", Label: "Connections", Href: "/connections", Icon: "data", Meta: optionalValue("Data access")},
				{ID: "admin", Label: "Admin", Href: "/admin", Icon: "settings", Meta: optionalValue("Read-only administration")},
			},
		},
	}
}

func chatPath(parts ...string) string {
	path := "/chats"
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		path += "/" + url.PathEscape(part)
	}
	return path
}

func workspaceDisplayTitle(catalog dashboard.Catalog) string {
	if strings.TrimSpace(catalog.Workspace.Title) != "" {
		return catalog.Workspace.Title
	}
	if strings.TrimSpace(catalog.Workspace.ID) != "" {
		return catalog.Workspace.ID
	}
	return "LibreDash"
}

func pageVisualIDs(page dashboard.Page) []string {
	seen := map[string]struct{}{}
	ids := []string{}
	for _, item := range page.Visuals {
		if item.Visual == "" {
			continue
		}
		if _, ok := seen[item.Visual]; ok {
			continue
		}
		seen[item.Visual] = struct{}{}
		ids = append(ids, item.Visual)
	}
	sort.Strings(ids)
	return ids
}

func pageTableIDs(page dashboard.Page) []string {
	seen := map[string]struct{}{}
	ids := []string{}
	for _, item := range page.Visuals {
		if item.Table == "" {
			continue
		}
		if _, ok := seen[item.Table]; ok {
			continue
		}
		seen[item.Table] = struct{}{}
		ids = append(ids, item.Table)
	}
	sort.Strings(ids)
	return ids
}

func visualSignal(id string, visual reportdef.Visual, title, unit, format, measure string) dashboard.Visual {
	seriesList := []string{}
	if !visual.Query.Series.IsZero() {
		seriesList = append(seriesList, displayField(visual.Query.Series.Field))
	}
	visualType := visual.Type
	if visualType == "" && visual.KindOrDefault() == "kpi" {
		visualType = "kpi"
	}
	rendererOptions := map[string]map[string]any{}
	if len(visual.RendererOptions) > 0 {
		for key, value := range visual.RendererOptions {
			if nested, ok := value.(map[string]any); ok {
				rendererOptions[key] = nested
				continue
			}
			rendererOptions[key] = map[string]any{"value": value}
		}
	}
	return dashboard.Visual{
		Version:         3,
		ID:              id,
		Kind:            visual.KindOrDefault(),
		Shape:           visual.ShapeOrDefault(),
		Renderer:        visual.RendererOrDefault(),
		Type:            visualType,
		Title:           title,
		Unit:            unit,
		Format:          format,
		Interaction:     interactionSignal("point_selection", visual.Interaction.PointSelection),
		Dimensions:      displayFieldRefs(visual.Query.Dimensions),
		Measure:         measure,
		Measures:        displayFieldRefs(visual.Query.Measures),
		Series:          seriesList,
		Options:         visual.CoreOptions(),
		RendererOptions: rendererOptions,
		Selection:       []dashboard.InteractionSelectionEntry{},
		Data:            []dashboard.Datum{},
	}
}

func interactionSignal(kind string, selection reportdef.SelectionInteraction) dashboard.InteractionConfig {
	mappings := make([]dashboard.InteractionConfigMapping, 0, len(selection.Mappings))
	for _, mapping := range selection.Mappings {
		mappings = append(mappings, dashboard.InteractionConfigMapping{
			Field: mapping.Field,
			Fact:  mapping.Fact,
			Grain: mapping.Grain,
			Value: mapping.Value,
			Label: mapping.Label,
		})
	}
	return dashboard.InteractionConfig{
		Kind:     kind,
		Toggle:   selection.Toggle,
		Mappings: mappings,
		Targets:  append([]string{}, selection.Targets...),
	}
}

func displayFieldRefs(refs []reportdef.FieldRef) []string {
	fields := make([]string, len(refs))
	for i, ref := range refs {
		fields[i] = displayField(ref.Field)
	}
	return fields
}

func displayField(field string) string {
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
}

func displayLabel(label, fallback string) string {
	if strings.TrimSpace(label) != "" {
		return label
	}
	return fallback
}

func formatReportPageNumber(index, pageCount int) string {
	pageNumber := fmt.Sprintf("%d", index+1)
	if pageCount >= 10 {
		width := len(fmt.Sprintf("%d", pageCount))
		if len(pageNumber) < width {
			return strings.Repeat("0", width-len(pageNumber)) + pageNumber
		}
	}
	return pageNumber
}

func filterConfigContains(config []ReportFilterConfig, id string) bool {
	for _, item := range config {
		if item.ID == id {
			return true
		}
	}
	return false
}
