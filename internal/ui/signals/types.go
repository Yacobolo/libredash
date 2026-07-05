package signals

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/agentapp"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	workspaceview "github.com/Yacobolo/libredash/internal/workspace"
)

type RouteKind string

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

type ChromeSignal struct {
	Sidebar SidebarSignal `json:"sidebar"`
}

type SidebarSignal struct {
	WorkspaceTitle string                `json:"workspaceTitle"`
	Active         string                `json:"active"`
	DashboardID    string                `json:"dashboardId,omitempty"`
	DashboardTitle string                `json:"dashboardTitle"`
	PageTitle      string                `json:"pageTitle"`
	ModelID        string                `json:"modelId,omitempty"`
	ModelTitle     string                `json:"modelTitle,omitempty"`
	Compact        bool                  `json:"compact"`
	UserRole       string                `json:"userRole,omitempty"`
	PrimaryAction  *SidebarActionSignal  `json:"primaryAction,omitempty"`
	History        *SidebarHistorySignal `json:"history,omitempty"`
	Groups         []SidebarGroupSignal  `json:"groups"`
}

type SidebarActionSignal struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Icon  string `json:"icon"`
}

type SidebarHistorySignal struct {
	Label     string                     `json:"label"`
	EmptyText string                     `json:"emptyText,omitempty"`
	Items     []SidebarHistoryItemSignal `json:"items"`
}

type SidebarHistoryItemSignal struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Href    string `json:"href"`
	Active  bool   `json:"active"`
	Pending bool   `json:"pending,omitempty"`
}

type SidebarGroupSignal struct {
	Label string              `json:"label"`
	Items []SidebarItemSignal `json:"items"`
}

type SidebarItemSignal struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Href   string `json:"href"`
	Icon   string `json:"icon"`
	Meta   string `json:"meta,omitempty"`
	Active bool   `json:"active,omitempty"`
}

type RouteRuntimeSignal struct {
	ClientID    string    `json:"clientId,omitempty"`
	DashboardID string    `json:"dashboardId,omitempty"`
	PageID      string    `json:"pageId,omitempty"`
	ModelID     string    `json:"modelId,omitempty"`
	Kind        RouteKind `json:"kind"`
}

type StatusSignal = dashboard.Status

type CatalogPageEnvelope struct {
	Chrome  ChromeSignal       `json:"chrome"`
	Page    CatalogPageSignal  `json:"page"`
	Runtime RouteRuntimeSignal `json:"runtime"`
	Status  StatusSignal       `json:"status"`
}

type CatalogPageSignal struct {
	Kind        RouteKind                `json:"kind"`
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	Dashboards  []CatalogDashboardSignal `json:"dashboards"`
}

type CatalogDashboardSignal struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description,omitempty"`
	SemanticModel string   `json:"semanticModel,omitempty"`
	PageCount     int      `json:"pageCount"`
	Tags          []string `json:"tags,omitempty"`
	Href          string   `json:"href"`
}

type DashboardEnvelope struct {
	Chrome             ChromeSignal                        `json:"chrome"`
	Page               DashboardPageSignal                 `json:"page"`
	Runtime            RouteRuntimeSignal                  `json:"runtime"`
	CSRFToken          string                              `json:"csrfToken"`
	FilterConfig       []reportdef.FilterConfig            `json:"filterConfig"`
	Filters            dashboard.Filters                   `json:"filters"`
	URLParams          map[string]any                      `json:"urlParams"`
	URLParamShape      map[string]any                      `json:"urlParamShape"`
	FilterOptions      map[string][]dashboard.FilterOption `json:"filterOptions"`
	InteractionCommand dashboard.InteractionCommand        `json:"interactionCommand"`
	TableCommand       dashboard.TableRequest              `json:"tableCommand"`
	Tables             map[string]dashboard.Table          `json:"tables"`
	Visuals            map[string]dashboard.Visual         `json:"visuals"`
	Status             dashboard.Status                    `json:"status"`
}

type DashboardPageSignal struct {
	Kind           RouteKind                  `json:"kind"`
	Title          string                     `json:"title"`
	Description    string                     `json:"description,omitempty"`
	DashboardID    string                     `json:"dashboardId"`
	DashboardTitle string                     `json:"dashboardTitle"`
	PageID         string                     `json:"pageId"`
	PageTitle      string                     `json:"pageTitle"`
	HeaderDetail   string                     `json:"headerDetail"`
	ModelID        string                     `json:"modelId"`
	ModelTitle     string                     `json:"modelTitle"`
	Canvas         dashboard.PageCanvas       `json:"canvas"`
	Grid           dashboard.PageGrid         `json:"grid"`
	Pages          []DashboardPageNavSignal   `json:"pages"`
	Components     []DashboardComponentSignal `json:"components"`
}

type DashboardPageNavSignal struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Href   string `json:"href"`
	Active bool   `json:"active"`
}

type DashboardComponentSignal struct {
	ID          string                  `json:"id"`
	Kind        string                  `json:"kind"`
	Visual      string                  `json:"visual,omitempty"`
	Table       string                  `json:"table,omitempty"`
	Filter      string                  `json:"filter,omitempty"`
	Description string                  `json:"description,omitempty"`
	Placement   dashboard.PagePlacement `json:"placement"`
	X           float64                 `json:"x"`
	Y           float64                 `json:"y"`
	Width       float64                 `json:"width"`
	Height      float64                 `json:"height"`
	Eyebrow     string                  `json:"eyebrow,omitempty"`
	Title       string                  `json:"title,omitempty"`
	Subtitle    string                  `json:"subtitle,omitempty"`
	Badges      []string                `json:"badges,omitempty"`
}

type ChatEnvelope struct {
	Chrome    ChromeSignal                `json:"chrome"`
	Page      ChatPageSignal              `json:"page"`
	Runtime   RouteRuntimeSignal          `json:"runtime"`
	CSRFToken string                      `json:"csrfToken"`
	Agent     ChatSignal                  `json:"agent"`
	Visuals   map[string]dashboard.Visual `json:"visuals"`
	Tables    map[string]dashboard.Table  `json:"tables"`
}

type ChatPageSignal struct {
	Kind        RouteKind `json:"kind"`
	View        string    `json:"view"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
}

type SubSidebarSignal struct {
	Label       string                 `json:"label"`
	RailLabel   string                 `json:"railLabel"`
	AriaLabel   string                 `json:"ariaLabel"`
	StorageKey  string                 `json:"storageKey"`
	ActiveID    string                 `json:"activeId"`
	EmptyText   string                 `json:"emptyText,omitempty"`
	Disabled    bool                   `json:"disabled,omitempty"`
	Collapsible bool                   `json:"collapsible"`
	Numbered    bool                   `json:"numbered"`
	Items       []SubSidebarItemSignal `json:"items"`
}

type SubSidebarItemSignal struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Href    string `json:"href"`
	Active  bool   `json:"active"`
	Pending bool   `json:"pending,omitempty"`
}

type WorkspacePageEnvelope struct {
	Chrome          ChromeSignal          `json:"chrome"`
	Page            WorkspacePageSignal   `json:"page"`
	Runtime         RouteRuntimeSignal    `json:"runtime"`
	WorkspaceAccess WorkspaceAccessSignal `json:"workspaceAccess,omitempty"`
	Status          StatusSignal          `json:"status"`
}

type WorkspacePageSignal struct {
	Kind        RouteKind                `json:"kind"`
	Title       string                   `json:"title"`
	Description string                   `json:"description,omitempty"`
	WorkspaceID string                   `json:"workspaceId,omitempty"`
	Cards       []WorkspaceCardSignal    `json:"cards,omitempty"`
	AssetList   WorkspaceAssetListSignal `json:"assetList,omitempty"`
}

type WorkspaceAssetPageEnvelope struct {
	Chrome  ChromeSignal             `json:"chrome"`
	Page    WorkspaceAssetPageSignal `json:"page"`
	Runtime RouteRuntimeSignal       `json:"runtime"`
	Status  StatusSignal             `json:"status"`
}

type WorkspaceAssetPageSignal struct {
	Kind          RouteKind                     `json:"kind"`
	Title         string                        `json:"title"`
	WorkspaceID   string                        `json:"workspaceId"`
	AssetID       string                        `json:"assetId"`
	ActiveSection string                        `json:"activeSection"`
	Asset         WorkspaceAssetSummarySignal   `json:"asset"`
	Breadcrumbs   []WorkspaceBreadcrumbSignal   `json:"breadcrumbs"`
	Actions       []WorkspaceActionSignal       `json:"actions,omitempty"`
	Tabs          []WorkspaceTabSignal          `json:"tabs"`
	Details       WorkspaceAssetDetailsSignal   `json:"details,omitempty"`
	Lineage       WorkspaceAssetLineageSignal   `json:"lineage,omitempty"`
	Refresh       WorkspaceAssetRefreshSignal   `json:"refresh,omitempty"`
	Versions      *WorkspaceAssetVersionsSignal `json:"versions,omitempty"`
}

type ConnectionsPageEnvelope struct {
	Chrome  ChromeSignal          `json:"chrome"`
	Page    ConnectionsPageSignal `json:"page"`
	Runtime RouteRuntimeSignal    `json:"runtime"`
	Status  StatusSignal          `json:"status"`
}

type ConnectionsPageSignal struct {
	Kind        RouteKind                `json:"kind"`
	Title       string                   `json:"title"`
	Description string                   `json:"description,omitempty"`
	WorkspaceID string                   `json:"workspaceId,omitempty"`
	AssetList   WorkspaceAssetListSignal `json:"assetList,omitempty"`
}

type DataExplorerPageEnvelope struct {
	Chrome       ChromeSignal           `json:"chrome"`
	Page         DataExplorerPageSignal `json:"page"`
	DataExplorer DataExplorerSignal     `json:"dataExplorer"`
	Runtime      RouteRuntimeSignal     `json:"runtime"`
	Status       StatusSignal           `json:"status"`
}

type DataExplorerPageSignal struct {
	Kind                RouteKind                     `json:"kind"`
	Title               string                        `json:"title"`
	Description         string                        `json:"description,omitempty"`
	WorkspaceID         string                        `json:"workspaceId,omitempty"`
	SelectedWorkspaceID string                        `json:"selectedWorkspaceId,omitempty"`
	SelectedObject      string                        `json:"selectedObject,omitempty"`
	Workspaces          []DataExplorerWorkspaceSignal `json:"workspaces"`
	Tabs                []WorkspaceTabSignal          `json:"tabs"`
}

type DataExplorerWorkspaceSignal struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Href        string `json:"href"`
	ObjectCount int    `json:"objectCount"`
	Active      bool   `json:"active"`
}

type DataExplorerSignal struct {
	Objects             []DataExplorerObjectSignal `json:"objects"`
	SelectedWorkspaceID string                     `json:"selectedWorkspaceId,omitempty"`
	SelectedKey         string                     `json:"selectedKey,omitempty"`
	SelectedObject      *DataExplorerObjectSignal  `json:"selectedObject,omitempty"`
	Preview             DataPreviewSignal          `json:"preview"`
	Command             DataExplorerCommand        `json:"command"`
	Warnings            []string                   `json:"warnings,omitempty"`
}

type DataExplorerObjectSignal struct {
	Key            string                    `json:"key"`
	WorkspaceID    string                    `json:"workspaceId"`
	WorkspaceTitle string                    `json:"workspaceTitle,omitempty"`
	AssetID        string                    `json:"assetId,omitempty"`
	Layer          string                    `json:"layer"`
	ModelID        string                    `json:"modelId,omitempty"`
	Table          string                    `json:"table,omitempty"`
	Source         string                    `json:"source,omitempty"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description,omitempty"`
	DetailHref     string                    `json:"detailHref,omitempty"`
	ColumnCount    int                       `json:"columnCount"`
	RowCountLabel  string                    `json:"rowCountLabel,omitempty"`
	Columns        []DataPreviewColumnSignal `json:"columns,omitempty"`
}

type DataPreviewSignal struct {
	Columns       []DataPreviewColumnSignal         `json:"columns"`
	TotalRows     int                               `json:"totalRows"`
	AvailableRows int                               `json:"availableRows"`
	ChunkSize     int                               `json:"chunkSize"`
	RowHeight     int                               `json:"rowHeight"`
	ResetVersion  int                               `json:"resetVersion"`
	Blocks        map[string]DataPreviewBlockSignal `json:"blocks"`
	LoadingBlock  string                            `json:"loadingBlock,omitempty"`
	TotalRowLabel string                            `json:"totalRowLabel,omitempty"`
	Sort          DataPreviewSortSignal             `json:"sort"`
	SQL           string                            `json:"sql,omitempty"`
	Error         string                            `json:"error,omitempty"`
}

type DataPreviewBlockSignal struct {
	Start        int                   `json:"start"`
	RequestSeq   int                   `json:"requestSeq"`
	ResetVersion int                   `json:"resetVersion"`
	Sort         DataPreviewSortSignal `json:"sort"`
	Rows         []map[string]any      `json:"rows"`
}

type DataPreviewColumnSignal struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type,omitempty"`
}

type DataPreviewSortSignal struct {
	Column    string `json:"column,omitempty"`
	Direction string `json:"direction,omitempty"`
}

type DataExplorerCommand struct {
	WorkspaceID    string                `json:"workspaceId,omitempty"`
	ObjectKey      string                `json:"objectKey,omitempty"`
	Offset         int                   `json:"offset"`
	Limit          int                   `json:"limit"`
	Block          string                `json:"block,omitempty"`
	Start          int                   `json:"start"`
	Count          int                   `json:"count"`
	RequestSeq     int                   `json:"requestSeq"`
	ResetVersion   int                   `json:"resetVersion"`
	Sort           DataPreviewSortSignal `json:"sort"`
	VisibleColumns []string              `json:"visibleColumns,omitempty"`
	ColumnWidths   map[string]float64    `json:"columnWidths,omitempty"`
}

type WorkspaceCardSignal struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Href         string `json:"href"`
	ServingLabel string `json:"servingLabel"`
}

type WorkspaceAssetListSignal struct {
	WorkspaceID string                        `json:"workspaceId,omitempty"`
	Query       string                        `json:"query,omitempty"`
	ActiveType  string                        `json:"activeType,omitempty"`
	SearchHref  string                        `json:"searchHref"`
	Tabs        []WorkspaceTabSignal          `json:"tabs"`
	Assets      []WorkspaceAssetSummarySignal `json:"assets"`
	Empty       string                        `json:"empty"`
}

type WorkspaceAssetSummarySignal struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	TypeLabel   string `json:"typeLabel"`
	Key         string `json:"key"`
	ParentTitle string `json:"parentTitle,omitempty"`
	ParentHref  string `json:"parentHref,omitempty"`
	DetailHref  string `json:"detailHref"`
	OpenHref    string `json:"openHref"`
}

type WorkspaceTabSignal struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Href   string `json:"href"`
	Active bool   `json:"active"`
	Count  int    `json:"count,omitempty"`
}

type WorkspaceBreadcrumbSignal struct {
	Label   string `json:"label"`
	Href    string `json:"href,omitempty"`
	Current bool   `json:"current,omitempty"`
}

type WorkspaceActionSignal struct {
	Label    string `json:"label"`
	Href     string `json:"href,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Command  string `json:"command,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type WorkspaceAssetDetailsSignal struct {
	Overview           []DefinitionFactSignal         `json:"overview"`
	Sections           []WorkspaceDetailSectionSignal `json:"sections"`
	SemanticModelGraph *SemanticModelGraphSignal      `json:"semanticModelGraph,omitempty"`
}

type WorkspaceDetailSectionSignal struct {
	Title string                 `json:"title"`
	Facts []DefinitionFactSignal `json:"facts,omitempty"`
	Table RecordTableSignal      `json:"table,omitempty"`
	Code  string                 `json:"code,omitempty"`
	Lang  string                 `json:"lang,omitempty"`
}

type DefinitionFactSignal struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Code  bool   `json:"code,omitempty"`
	Wide  bool   `json:"wide,omitempty"`
}

type SemanticModelGraphSignal struct {
	BaseTable string                         `json:"baseTable,omitempty"`
	Nodes     []SemanticModelGraphNodeSignal `json:"nodes"`
	Edges     []SemanticModelGraphEdgeSignal `json:"edges"`
}

type SemanticModelGraphNodeSignal struct {
	ID          string                          `json:"id"`
	Title       string                          `json:"title"`
	Description string                          `json:"description,omitempty"`
	PrimaryKey  string                          `json:"primaryKey,omitempty"`
	Fields      []SemanticModelGraphFieldSignal `json:"fields"`
}

type SemanticModelGraphFieldSignal struct {
	Name          string   `json:"name"`
	Label         string   `json:"label,omitempty"`
	Type          string   `json:"type,omitempty"`
	PrimaryKey    bool     `json:"primaryKey,omitempty"`
	Join          bool     `json:"join,omitempty"`
	Relationships []string `json:"relationships,omitempty"`
}

type SemanticModelGraphEdgeSignal struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Target      string `json:"target"`
	SourceField string `json:"sourceField"`
	TargetField string `json:"targetField"`
	Cardinality string `json:"cardinality"`
	Label       string `json:"label"`
	Active      bool   `json:"active"`
}

type WorkspaceAssetLineageSignal struct {
	Count       int                     `json:"count"`
	Graph       AssetLineageGraphSignal `json:"graph"`
	UsesTable   RecordTableSignal       `json:"usesTable"`
	UsedByTable RecordTableSignal       `json:"usedByTable"`
}

type WorkspaceAssetRefreshSignal struct {
	Status         string             `json:"status"`
	Running        bool               `json:"running"`
	LastSuccessful string             `json:"lastSuccessful"`
	RunsTable      *RecordTableSignal `json:"runsTable,omitempty"`
}

type WorkspaceAssetVersionsSignal struct {
	CurrentContentHash string            `json:"currentContentHash"`
	Table              RecordTableSignal `json:"table"`
}

type AssetLineageGraphSignal struct {
	Nodes []AssetLineageNodeSignal `json:"nodes"`
	Edges []AssetLineageEdgeSignal `json:"edges"`
}

type AssetLineageNodeSignal struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Kind              string `json:"kind"`
	Meta              string `json:"meta,omitempty"`
	Href              string `json:"href,omitempty"`
	Side              string `json:"side"`
	Rank              int    `json:"rank"`
	Selected          bool   `json:"selected,omitempty"`
	VisibleUpstream   int    `json:"visibleUpstreamCount,omitempty"`
	VisibleDownstream int    `json:"visibleDownstreamCount,omitempty"`
	UsesCount         int    `json:"usesCount,omitempty"`
	UsedByCount       int    `json:"usedByCount,omitempty"`
	ContainedCount    int    `json:"containedCount,omitempty"`
	ContainedSummary  string `json:"containedSummary,omitempty"`
}

type AssetLineageEdgeSignal struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Kind   string `json:"kind"`
}

type RecordTableSignal struct {
	Columns        []RecordTableColumnSignal  `json:"columns"`
	Rows           []map[string]any           `json:"rows"`
	Empty          string                     `json:"empty"`
	MinWidth       string                     `json:"minWidth,omitempty"`
	ColumnSelector *RecordTableColumnSelector `json:"columnSelector,omitempty"`
	Density        string                     `json:"density,omitempty"`
	RowAction      string                     `json:"rowAction,omitempty"`
}

type RecordTableColumnSignal struct {
	ID         string `json:"id"`
	Header     string `json:"header"`
	Kind       string `json:"kind,omitempty"`
	Align      string `json:"align,omitempty"`
	HrefKey    string `json:"hrefKey,omitempty"`
	Width      string `json:"width,omitempty"`
	Toggleable *bool  `json:"toggleable,omitempty"`
}

type RecordTableColumnSelector struct {
	Enabled        bool     `json:"enabled"`
	StorageKey     string   `json:"storageKey,omitempty"`
	Label          string   `json:"label,omitempty"`
	DefaultColumns []string `json:"defaultColumns,omitempty"`
}

type RecordTableBadgeSignal struct {
	Label string `json:"label"`
	Tone  string `json:"tone,omitempty"`
}

type AdminPageEnvelope struct {
	Chrome  ChromeSignal       `json:"chrome"`
	Page    AdminPageSignal    `json:"page"`
	Runtime RouteRuntimeSignal `json:"runtime"`
	Status  StatusSignal       `json:"status"`
}

type AdminPageSignal struct {
	Kind         RouteKind                   `json:"kind"`
	Title        string                      `json:"title"`
	Active       string                      `json:"active"`
	Sidebar      SubSidebarSignal            `json:"sidebar"`
	HeaderTitle  string                      `json:"headerTitle"`
	HeaderDetail string                      `json:"headerDetail"`
	Empty        string                      `json:"empty,omitempty"`
	Metrics      []AdminMetricSignal         `json:"metrics,omitempty"`
	Sections     []AdminContentSectionSignal `json:"sections,omitempty"`
	Agent        AdminAgentSignal            `json:"agent,omitempty"`
	Storage      AdminStorageSignal          `json:"storage,omitempty"`
}

type AdminQueryHistorySignal struct {
	Table            RecordTableSignal        `json:"table"`
	FilterMenus      []FilterMenuSignal       `json:"filterMenus,omitempty"`
	Filters          AdminQueryHistoryFilters `json:"filters"`
	NextCursor       string                   `json:"nextCursor"`
	LoadedCountLabel string                   `json:"loadedCountLabel"`
	HasMore          bool                     `json:"hasMore"`
	Loading          bool                     `json:"loading"`
	Error            string                   `json:"error"`
	Limit            int                      `json:"limit"`
}

type AdminQueryHistoryFilters struct {
	Workspaces []string `json:"workspaces,omitempty"`
	Principals []string `json:"principals,omitempty"`
	Surfaces   []string `json:"surfaces,omitempty"`
	Kinds      []string `json:"kinds,omitempty"`
	Statuses   []string `json:"statuses,omitempty"`
	Target     string   `json:"target,omitempty"`
	Search     string   `json:"search,omitempty"`
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
}

type AdminQueryHistoryCommand struct {
	Action     string                   `json:"action"`
	Filters    AdminQueryHistoryFilters `json:"filters"`
	FilterMenu FilterMenuCommand        `json:"filterMenu,omitempty"`
	PageToken  string                   `json:"pageToken,omitempty"`
	Limit      int                      `json:"limit,omitempty"`
	EventID    string                   `json:"eventId,omitempty"`
}

type FilterMenuSignal struct {
	ID           string                   `json:"id"`
	Label        string                   `json:"label"`
	SummaryLabel string                   `json:"summaryLabel,omitempty"`
	Mode         string                   `json:"mode,omitempty"`
	Search       string                   `json:"search,omitempty"`
	Selected     []string                 `json:"selected,omitempty"`
	Options      []FilterMenuOptionSignal `json:"options,omitempty"`
	Loading      bool                     `json:"loading"`
	Error        string                   `json:"error,omitempty"`
	Placeholder  string                   `json:"placeholder,omitempty"`
	EmptyLabel   string                   `json:"emptyLabel,omitempty"`
}

type FilterMenuOptionSignal struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	CountLabel  string `json:"countLabel,omitempty"`
	Selected    bool   `json:"selected"`
	Disabled    bool   `json:"disabled"`
}

type FilterMenuCommand struct {
	MenuID   string   `json:"menuId,omitempty"`
	Action   string   `json:"action,omitempty"`
	Search   string   `json:"search,omitempty"`
	Value    string   `json:"value,omitempty"`
	Selected []string `json:"selected,omitempty"`
}

type AdminQueryDetailSignal struct {
	EventID       string `json:"eventId,omitempty"`
	Loading       bool   `json:"loading"`
	Error         string `json:"error,omitempty"`
	Status        string `json:"status,omitempty"`
	StatusLabel   string `json:"statusLabel,omitempty"`
	WorkspaceID   string `json:"workspaceId,omitempty"`
	PrincipalID   string `json:"principalId,omitempty"`
	Surface       string `json:"surface,omitempty"`
	Operation     string `json:"operation,omitempty"`
	QueryKind     string `json:"queryKind,omitempty"`
	ModelID       string `json:"modelId,omitempty"`
	Target        string `json:"target,omitempty"`
	ObjectType    string `json:"objectType,omitempty"`
	ObjectID      string `json:"objectId,omitempty"`
	RequestID     string `json:"requestId,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
	DurationMS    int64  `json:"durationMs"`
	RowsReturned  int    `json:"rowsReturned"`
	QueryError    string `json:"queryError,omitempty"`
	SQL           string `json:"sql,omitempty"`
	PlanText      string `json:"planText,omitempty"`
	QueryJSON     string `json:"queryJson,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
}

type AdminAgentSignal struct {
	Enabled      bool                   `json:"enabled"`
	Model        string                 `json:"model,omitempty"`
	SystemPrompt string                 `json:"systemPrompt"`
	CanWrite     bool                   `json:"canWrite"`
	CSRFToken    string                 `json:"csrfToken"`
	UpdatePath   string                 `json:"updatePath"`
	Tools        []AdminAgentToolSignal `json:"tools"`
}

type AdminAgentToolSignal struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type AdminMetricSignal struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail,omitempty"`
}

type AdminContentSectionSignal struct {
	Title string                 `json:"title"`
	Facts []DefinitionFactSignal `json:"facts,omitempty"`
	Table RecordTableSignal      `json:"table,omitempty"`
}

type AdminQueryEventSignal struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspaceId"`
	PrincipalID   string `json:"principalId"`
	Surface       string `json:"surface"`
	Operation     string `json:"operation"`
	QueryKind     string `json:"queryKind"`
	ModelID       string `json:"modelId"`
	Target        string `json:"target"`
	ObjectType    string `json:"objectType"`
	ObjectID      string `json:"objectId"`
	RequestID     string `json:"requestId"`
	CorrelationID string `json:"correlationId"`
	Status        string `json:"status"`
	DurationMS    int64  `json:"durationMs"`
	RowsReturned  int    `json:"rowsReturned"`
	Error         string `json:"error"`
	SQL           string `json:"sql"`
	PlanText      string `json:"planText"`
	QueryJSON     string `json:"queryJson"`
	CreatedAt     string `json:"createdAt"`
}

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

type AdminStorageSignal struct {
	Summary       AdminStorageSummary              `json:"summary"`
	Status        string                           `json:"status"`
	Warnings      []string                         `json:"warnings"`
	Tables        []AdminStorageTableSignal        `json:"tables"`
	Snapshots     []AdminStorageSnapshotSignal     `json:"snapshots"`
	ServingStates []AdminStorageServingStateSignal `json:"servingStates"`
	SelectedKey   string                           `json:"selectedKey"`
	SelectedTable *AdminStorageTableSignal         `json:"selectedTable"`
}

type AdminStorageSummary struct {
	CatalogPath        string `json:"catalogPath"`
	DataPath           string `json:"dataPath"`
	CatalogSizeLabel   string `json:"catalogSizeLabel"`
	DataSizeLabel      string `json:"dataSizeLabel"`
	TotalSizeLabel     string `json:"totalSizeLabel"`
	TotalDataSizeLabel string `json:"totalDataSizeLabel"`
	DatabaseCount      int    `json:"databaseCount"`
	TableCount         int    `json:"tableCount"`
	SnapshotCount      int    `json:"snapshotCount"`
	DataFileCount      int    `json:"dataFileCount"`
}

type AdminStorageTableSignal struct {
	Key           string                           `json:"key"`
	DatabaseID    string                           `json:"databaseId"`
	DatabaseName  string                           `json:"databaseName"`
	DatabasePath  string                           `json:"databasePath"`
	ModelID       string                           `json:"modelId"`
	ModelName     string                           `json:"modelName"`
	Schema        string                           `json:"schema"`
	Name          string                           `json:"name"`
	Type          string                           `json:"type"`
	TableID       int64                            `json:"tableId"`
	TableUUID     string                           `json:"tableUuid"`
	DuckLakePath  string                           `json:"duckLakePath"`
	BeginSnapshot int64                            `json:"beginSnapshot"`
	EndSnapshot   int64                            `json:"endSnapshot"`
	RowCount      int64                            `json:"rowCount"`
	RowCountLabel string                           `json:"rowCountLabel"`
	ColumnCount   int                              `json:"columnCount"`
	FileCount     int                              `json:"fileCount"`
	SizeBytes     int64                            `json:"sizeBytes"`
	SizeLabel     string                           `json:"sizeLabel"`
	Columns       []AdminStorageColumnSignal       `json:"columns,omitempty"`
	Files         []AdminStorageFileSignal         `json:"files,omitempty"`
	History       []AdminStorageTableHistorySignal `json:"history,omitempty"`
	ServingStates []AdminStorageServingStateSignal `json:"servingStates,omitempty"`
}

type AdminStorageColumnSignal struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Type                string `json:"type"`
	Ordinal             int    `json:"ordinal"`
	Nullable            string `json:"nullable"`
	Default             string `json:"default"`
	InitialDefault      string `json:"initialDefault"`
	DefaultValueType    string `json:"defaultValueType"`
	DefaultValueDialect string `json:"defaultValueDialect"`
	BeginSnapshot       int64  `json:"beginSnapshot"`
	ContainsNull        string `json:"containsNull"`
	ContainsNaN         string `json:"containsNan"`
	MinValue            string `json:"minValue"`
	MaxValue            string `json:"maxValue"`
	ExtraStats          string `json:"extraStats"`
}

type AdminStorageFileSignal struct {
	ID               int64  `json:"id"`
	Path             string `json:"path"`
	Format           string `json:"format"`
	RecordCount      int64  `json:"recordCount"`
	RecordCountLabel string `json:"recordCountLabel"`
	SizeBytes        int64  `json:"sizeBytes"`
	SizeLabel        string `json:"sizeLabel"`
	BeginSnapshot    int64  `json:"beginSnapshot"`
	EndSnapshot      int64  `json:"endSnapshot"`
}

type AdminStorageTableHistorySignal struct {
	SnapshotID    int64  `json:"snapshotId"`
	Time          string `json:"time"`
	SchemaVersion int64  `json:"schemaVersion"`
	Source        string `json:"source"`
	Changes       string `json:"changes"`
	Author        string `json:"author"`
	Message       string `json:"message"`
	ExtraInfo     string `json:"extraInfo"`
}

type AdminStorageSnapshotSignal struct {
	ID                int64  `json:"id"`
	Time              string `json:"time"`
	SchemaVersion     int64  `json:"schemaVersion"`
	Author            string `json:"author"`
	Message           string `json:"message"`
	Changes           string `json:"changes"`
	ExtraInfo         string `json:"extraInfo"`
	Protected         bool   `json:"protected"`
	ServingStateCount int    `json:"servingStateCount"`
}

type AdminStorageServingStateSignal struct {
	WorkspaceID    string `json:"workspaceId"`
	Environment    string `json:"environment"`
	ServingStateID string `json:"servingStateId"`
	Status         string `json:"status"`
	SnapshotID     int64  `json:"snapshotId"`
	Digest         string `json:"digest"`
	Active         bool   `json:"active"`
	ActivatedAt    string `json:"activatedAt"`
}

type AdminStorageCommand struct {
	DatabaseID string `json:"databaseId"`
	Schema     string `json:"schema"`
	Table      string `json:"table"`
}

type LoginPageEnvelope struct {
	Page    LoginPageSignal    `json:"page"`
	Runtime RouteRuntimeSignal `json:"runtime"`
	Status  StatusSignal       `json:"status"`
}

type LoginPageSignal struct {
	Kind                RouteKind `json:"kind"`
	Title               string    `json:"title"`
	ProviderLabel       string    `json:"providerLabel"`
	BackgroundModuleSrc string    `json:"backgroundModuleSrc"`
}

type WorkspaceAccessResponse struct {
	Workspace workspaceview.WorkspaceView     `json:"workspace"`
	Roles     []workspaceview.RoleView        `json:"roles"`
	Bindings  []workspaceview.RoleBindingView `json:"bindings"`
	CanManage bool                            `json:"canManage"`
	Status    WorkspaceAccessStatus           `json:"status"`
}

type WorkspaceAccessSignal struct {
	WorkspaceAccessResponse
	CSRFToken string                 `json:"csrfToken"`
	Command   WorkspaceAccessCommand `json:"command"`
	Search    string                 `json:"search"`
}

type WorkspaceAccessStatus struct {
	Loading bool   `json:"loading"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

type WorkspaceAccessCommand struct {
	Email       string `json:"email"`
	Role        string `json:"role"`
	PrincipalID string `json:"principalId"`
}

type ChatSignal struct {
	Conversations        []ChatConversationSummary   `json:"conversations"`
	ActiveConversationID string                      `json:"activeConversationId"`
	Transcript           []ChatTranscriptItemSignal  `json:"transcript"`
	Status               ChatStatus                  `json:"status"`
	Composer             ComposerSignal              `json:"composer"`
	Visuals              map[string]dashboard.Visual `json:"-"`
	Tables               map[string]dashboard.Table  `json:"-"`
}

type ChatTranscriptItemSignal struct {
	ID             string              `json:"id"`
	Kind           string              `json:"kind"`
	Text           string              `json:"text,omitempty"`
	Markdown       string              `json:"markdown,omitempty"`
	ToolCallID     string              `json:"toolCallId,omitempty"`
	Name           string              `json:"name,omitempty"`
	Title          string              `json:"title,omitempty"`
	Status         string              `json:"status,omitempty"`
	Summary        string              `json:"summary,omitempty"`
	ResultSummary  string              `json:"resultSummary,omitempty"`
	InputJSON      string              `json:"inputJson,omitempty"`
	InputFormat    string              `json:"inputFormat,omitempty"`
	ArgumentsJSON  string              `json:"argumentsJson,omitempty"`
	ResultJSON     string              `json:"resultJson,omitempty"`
	ResultFormat   string              `json:"resultFormat,omitempty"`
	Artifact       *ChatArtifactSignal `json:"artifact,omitempty"`
	Error          string              `json:"error,omitempty"`
	ConversationID string              `json:"conversationId,omitempty"`
	RunID          string              `json:"runId,omitempty"`
	CreatedAt      string              `json:"createdAt,omitempty"`
}

type ChatArtifactSignal struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Summary string `json:"summary,omitempty"`
}

func ChatTranscriptItems(items []agentapp.ChatTranscriptItem) []ChatTranscriptItemSignal {
	out := make([]ChatTranscriptItemSignal, 0, len(items))
	for _, item := range items {
		out = append(out, ChatTranscriptItem(item))
	}
	return out
}

func ChatTranscriptItem(item agentapp.ChatTranscriptItem) ChatTranscriptItemSignal {
	out := ChatTranscriptItemSignal{
		ID:             item.ID,
		Kind:           item.Kind,
		Text:           item.Text,
		Markdown:       item.Markdown,
		ToolCallID:     item.ToolCallID,
		Name:           item.Name,
		Title:          item.Title,
		Status:         item.Status,
		Summary:        item.Summary,
		ResultSummary:  item.ResultSummary,
		InputJSON:      item.InputJSON,
		InputFormat:    item.InputFormat,
		ArgumentsJSON:  item.ArgumentsJSON,
		ResultJSON:     item.ResultJSON,
		ResultFormat:   item.ResultFormat,
		Error:          item.Error,
		ConversationID: item.ConversationID,
		RunID:          item.RunID,
		CreatedAt:      item.CreatedAt,
	}
	if item.Artifact != nil {
		out.Artifact = &ChatArtifactSignal{
			Kind:    item.Artifact.Kind,
			ID:      item.Artifact.ID,
			Summary: item.Artifact.Summary,
		}
	}
	return out
}

type ChatConversationSummary struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspaceId"`
	PrincipalID     string `json:"principalId"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	MessageCount    int    `json:"messageCount"`
	LastMessageText string `json:"lastMessageText,omitempty"`
	TitlePending    bool   `json:"titlePending,omitempty"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	ArchivedAt      string `json:"archivedAt,omitempty"`
}

type ChatStatus struct {
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

type ComposerSignal struct {
	Value       string `json:"value"`
	Disabled    bool   `json:"disabled"`
	Placeholder string `json:"placeholder"`
}

func DashboardInitialEnvelope(dataDir, clientID, csrfToken string, catalog dashboard.Catalog, report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page, activePage dashboard.Page, initialFilters dashboard.Filters) DashboardEnvelope {
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
			Description:    report.Description,
			DashboardID:    report.ID,
			DashboardTitle: report.Title,
			PageID:         activePage.ID,
			PageTitle:      activePage.Title,
			HeaderDetail:   ReportPageHeaderDetail(pages, activePage),
			ModelID:        modelID,
			ModelTitle:     modelTitle,
			Canvas:         activePage.Canvas,
			Grid:           activePage.Grid,
			Pages:          dashboardPageNav(catalog.Workspace.ID, report.ID, pages, activePage),
			Components:     dashboardComponents(activePage),
		},
		Runtime: RouteRuntimeSignal{
			Kind:        RouteDashboard,
			ClientID:    clientID,
			DashboardID: report.ID,
			PageID:      activePage.ID,
			ModelID:     modelID,
		},
		CSRFToken:          csrfToken,
		FilterConfig:       report.FilterConfigForPage(activePage.ID),
		Filters:            initialFilters,
		URLParams:          report.URLParamsFromFiltersForPage(activePage.ID, initialFilters),
		URLParamShape:      report.URLParamShapeForPage(activePage.ID),
		FilterOptions:      map[string][]dashboard.FilterOption{},
		InteractionCommand: dashboard.InteractionCommand{Toggle: true, Mappings: []dashboard.InteractionCommandMapping{}},
		TableCommand:       tableRequest,
		Tables:             TableSignals(report, activePage, tableRequest),
		Visuals:            VisualSignals(report, model, activePage),
		Status: dashboard.Status{
			Loading:       false,
			Error:         "",
			LastUpdated:   "",
			DataDirectory: dataDir,
			SetupRequired: false,
		},
	}
}

func ChatInitialEnvelope(catalog dashboard.Catalog, workspaceID, csrfToken, roleLabel, view string, agent ChatSignal) ChatEnvelope {
	chrome := ChromeSignal{Sidebar: SidebarConfigForChat(catalog, workspaceID, roleLabel, view)}
	AttachChatSidebar(&chrome.Sidebar, agent)
	return ChatEnvelope{
		Chrome:    chrome,
		Page:      ChatPage(workspaceID, view, agent),
		Runtime:   RouteRuntimeSignal{Kind: RouteChat},
		CSRFToken: csrfToken,
		Agent:     agent,
		Visuals:   agent.Visuals,
		Tables:    agent.Tables,
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
		EmptyText: "No chats yet.",
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

func WorkspaceAccessSignals(access WorkspaceAccessResponse, csrfToken string) WorkspaceAccessSignal {
	return WorkspaceAccessSignal{
		WorkspaceAccessResponse: access,
		CSRFToken:               csrfToken,
		Command:                 WorkspaceAccessCommand{},
		Search:                  "",
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
		DashboardID:    dashboardID,
		DashboardTitle: dashboardTitle,
		PageTitle:      pageTitle,
		ModelID:        modelID,
		ModelTitle:     modelTitle,
		Compact:        compact,
		UserRole:       roleLabel,
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
			TotalRows:     0,
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
	usedTables := map[string]struct{}{}
	usedFilters := map[string]struct{}{}
	for _, component := range envelope.Page.Components {
		switch {
		case component.Visual != "":
			usedVisuals[component.Visual] = struct{}{}
			if _, ok := envelope.Visuals[component.Visual]; !ok {
				return fmt.Errorf("component %q references missing visual %q", component.ID, component.Visual)
			}
		case component.Table != "":
			usedTables[component.Table] = struct{}{}
			if _, ok := envelope.Tables[component.Table]; !ok {
				return fmt.Errorf("component %q references missing table %q", component.ID, component.Table)
			}
		case component.Filter != "":
			usedFilters[component.Filter] = struct{}{}
			if !filterConfigContains(envelope.FilterConfig, component.Filter) {
				return fmt.Errorf("component %q references missing filter config %q", component.ID, component.Filter)
			}
			if _, ok := envelope.Filters.WithDefaults().Controls[component.Filter]; !ok {
				return fmt.Errorf("component %q references missing filter control %q", component.ID, component.Filter)
			}
		}
	}
	for id := range envelope.Visuals {
		if _, ok := usedVisuals[id]; !ok {
			return fmt.Errorf("unused visual payload %q", id)
		}
	}
	for id := range envelope.Tables {
		if _, ok := usedTables[id]; !ok {
			return fmt.Errorf("unused table payload %q", id)
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
		components = append(components, DashboardComponentSignal{
			ID:          visual.ID,
			Kind:        visual.Kind,
			Visual:      visual.Visual,
			Table:       visual.Table,
			Filter:      visual.Filter,
			Description: visual.Description,
			Placement:   visual.Placement,
			X:           visual.X,
			Y:           visual.Y,
			Width:       visual.Width,
			Height:      visual.Height,
			Eyebrow:     visual.Eyebrow,
			Title:       visual.Title,
			Subtitle:    visual.Subtitle,
			Badges:      append([]string{}, visual.Badges...),
		})
	}
	return components
}

func sidebarGroups(catalog dashboard.Catalog, includeWorkspaceScoped bool) []SidebarGroupSignal {
	return []SidebarGroupSignal{
		{
			Label: "Navigation",
			Items: []SidebarItemSignal{
				{ID: "dashboards", Label: "Dashboards", Href: "/", Icon: "dashboard", Meta: "Reports"},
				{ID: "chat", Label: "Chats", Href: chatPath(), Icon: "chat", Meta: "Agent interface"},
				{ID: "workspaces", Label: "Workspaces", Href: "/workspaces", Icon: "catalog", Meta: "Published assets"},
				{ID: "data", Label: "Data", Href: "/data", Icon: "cache", Meta: "Inspect rows"},
				{ID: "connections", Label: "Connections", Href: "/connections", Icon: "data", Meta: "Data access"},
				{ID: "admin", Label: "Admin", Href: "/admin", Icon: "settings", Meta: "Read-only administration"},
			},
		},
	}
}

func chatPath(parts ...string) string {
	path := "/chat"
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

func filterConfigContains(config []reportdef.FilterConfig, id string) bool {
	for _, item := range config {
		if item.ID == id {
			return true
		}
	}
	return false
}
