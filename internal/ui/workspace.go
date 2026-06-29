package ui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/assetnav"
	"github.com/Yacobolo/libredash/internal/dashboard"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	workspaceview "github.com/Yacobolo/libredash/internal/workspace"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

func WorkspacesPage(catalog dashboard.Catalog, workspaces []workspaceview.WorkspaceView, roleLabel string) g.Node {
	page := workspaceCatalogPageSignal(workspaces)
	return workspaceRouteDocument("LibreDash Workspaces", catalog, "workspaces", roleLabel, page, uisignals.RouteWorkspace,
		g.El("ld-workspace-page",
			g.Attr("slot", "page"),
			g.Attr("page", jsonString(page)),
			g.Attr("data-attr:page", "JSON.stringify($page)"),
		),
		nil,
	)
}

func WorkspacePage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, assets []workspaceview.AssetView, activeType, query, roleLabel string, access WorkspaceAccessResponse, csrfToken string) g.Node {
	page := workspacePageSignal(workspace, assets, nil, activeType, query)
	attrs := []g.Node{
		g.Attr("slot", "page"),
		g.Attr("page", jsonString(page)),
		g.Attr("data-attr:page", "JSON.stringify($page)"),
	}
	accessAttrs, extraSignals := workspaceAccessRouteBridge(workspace.ID, access, csrfToken)
	attrs = append(attrs, accessAttrs...)
	return workspaceRouteDocument(workspace.Title, catalog, "workspaces", roleLabel, page, uisignals.RouteWorkspace,
		g.El("ld-workspace-page", attrs...),
		extraSignals,
	)
}

func ConnectionsPage(catalog dashboard.Catalog, workspaceID string, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeType, query, roleLabel string) g.Node {
	page := connectionsPageSignal(workspaceID, assets, edges, activeType, query)
	return workspaceRouteDocument("Connections", catalog, "connections", roleLabel, page, uisignals.RouteConnections,
		g.El("ld-connections-page",
			g.Attr("slot", "page"),
			g.Attr("page", jsonString(page)),
			g.Attr("data-attr:page", "JSON.stringify($page)"),
		),
		nil,
	)
}

type workspaceAccessSignalState struct {
	WorkspaceAccessResponse
	CSRFToken string                 `json:"csrfToken"`
	Command   WorkspaceAccessCommand `json:"command"`
	Search    string                 `json:"search"`
}

func WorkspaceAccessSignals(access WorkspaceAccessResponse, csrfToken string) workspaceAccessSignalState {
	return workspaceAccessSignalState{
		WorkspaceAccessResponse: access,
		CSRFToken:               csrfToken,
		Command:                 WorkspaceAccessCommand{},
		Search:                  "",
	}
}

func workspaceAccessRouteBridge(workspaceID string, access WorkspaceAccessResponse, csrfToken string) ([]g.Node, map[string]any) {
	if !access.CanManage {
		return nil, nil
	}
	accessSignal := WorkspaceAccessSignals(access, csrfToken)
	upsert := "$workspaceAccess.status = {loading: true, error: '', message: ''}; $workspaceAccess.command = evt.detail; " + postActionWithCSRFSignal("/workspaces/"+workspaceID+"/access/upsert", "$workspaceAccess.csrfToken")
	remove := "$workspaceAccess.status = {loading: true, error: '', message: ''}; $workspaceAccess.command = evt.detail; " + postActionWithCSRFSignal("/workspaces/"+workspaceID+"/access/remove", "$workspaceAccess.csrfToken")
	return []g.Node{
		g.Attr("workspaceaccess", jsonString(accessSignal)),
		g.Attr("data-attr:workspaceaccess", "JSON.stringify($workspaceAccess)"),
		g.Attr("data-on:ld-workspace-access-search__debounce.200ms", "$workspaceAccess.search = evt.detail.search"),
		g.Attr("data-on:ld-workspace-access-upsert", upsert),
		g.Attr("data-on:ld-workspace-access-remove", remove),
	}, map[string]any{"workspaceAccess": accessSignal}
}

func workspaceCatalogPageSignal(workspaces []workspaceview.WorkspaceView) uisignals.WorkspacePageSignal {
	return uisignals.WorkspacePageSignal{
		Kind:        uisignals.RouteWorkspace,
		Title:       "Workspaces",
		Description: "View published BI workspaces. Authoring lives in Git.",
		Cards:       workspaceCardSignals(workspaces),
	}
}

func workspacePageSignal(workspace workspaceview.WorkspaceView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeType, query string) uisignals.WorkspacePageSignal {
	return uisignals.WorkspacePageSignal{
		Kind:        uisignals.RouteWorkspace,
		Title:       workspace.Title,
		Description: workspace.Description,
		WorkspaceID: workspace.ID,
		AssetList: workspaceAssetListSignal(
			workspace.ID,
			assets,
			edges,
			activeType,
			query,
			workspaceAssetListTabs(workspace.ID, activeType, query),
			"No assets match this view.",
			"/workspaces/"+workspace.ID,
		),
	}
}

func connectionsPageSignal(workspaceID string, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeType, query string) uisignals.ConnectionsPageSignal {
	return uisignals.ConnectionsPageSignal{
		Kind:        uisignals.RouteConnections,
		Title:       "Connections",
		Description: "Connection-scoped data assets used by published semantic models.",
		WorkspaceID: workspaceID,
		AssetList: workspaceAssetListSignal(
			workspaceID,
			assets,
			edges,
			activeType,
			query,
			connectionAssetListTabs(activeType, query),
			"No connection assets match this view.",
			"/connections",
		),
	}
}

func workspaceCardSignals(workspaces []workspaceview.WorkspaceView) []uisignals.WorkspaceCardSignal {
	cards := make([]uisignals.WorkspaceCardSignal, 0, len(workspaces))
	for _, workspace := range workspaces {
		description := workspace.Description
		if strings.TrimSpace(description) == "" {
			description = "Published workspace assets."
		}
		cards = append(cards, uisignals.WorkspaceCardSignal{
			ID:              workspace.ID,
			Title:           workspace.Title,
			Description:     description,
			Href:            "/workspaces/" + workspace.ID,
			DeploymentLabel: activeDeploymentLabel(workspace),
		})
	}
	return cards
}

func workspaceAssetListSignal(workspaceID string, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeType, query string, tabs []uisignals.WorkspaceTabSignal, empty, searchHref string) uisignals.WorkspaceAssetListSignal {
	items := make([]uisignals.WorkspaceAssetSummarySignal, 0, len(assets))
	sortedAssets := sortedWorkspaceAssetList(assets)
	assetIndex := assetsByID(sortedAssets)
	for _, asset := range sortedAssets {
		items = append(items, workspaceAssetSummarySignal(workspaceID, asset, assetIndex, edges))
	}
	return uisignals.WorkspaceAssetListSignal{
		WorkspaceID: workspaceID,
		Query:       query,
		ActiveType:  activeType,
		SearchHref:  searchHref,
		Tabs:        tabs,
		Assets:      items,
		Empty:       empty,
	}
}

func sortedWorkspaceAssetList(assets []workspaceview.AssetView) []workspaceview.AssetView {
	out := append([]workspaceview.AssetView(nil), assets...)
	sort.SliceStable(out, func(i, j int) bool {
		leftPriority := workspaceAssetTypePriority(out[i].Type)
		rightPriority := workspaceAssetTypePriority(out[j].Type)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		leftTitle := strings.ToLower(assetTitle(out[i]))
		rightTitle := strings.ToLower(assetTitle(out[j]))
		if leftTitle != rightTitle {
			return leftTitle < rightTitle
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func workspaceAssetTypePriority(typ string) int {
	switch typ {
	case "dashboard":
		return 0
	case "model_table":
		return 1
	case "semantic_model":
		return 2
	case "connection":
		return 3
	case "source":
		return 4
	default:
		return 10
	}
}

func workspaceAssetListTabs(workspaceID, activeType, query string) []uisignals.WorkspaceTabSignal {
	types := []string{"", "model_table", "semantic_model", "dashboard"}
	tabs := make([]uisignals.WorkspaceTabSignal, 0, len(types))
	for _, typ := range types {
		label := "All"
		if typ != "" {
			label = assetTypeLabel(typ)
		}
		tabs = append(tabs, uisignals.WorkspaceTabSignal{ID: typ, Label: label, Href: workspaceAssetHref(workspaceID, typ, query), Active: typ == activeType})
	}
	return tabs
}

func connectionAssetListTabs(activeType, query string) []uisignals.WorkspaceTabSignal {
	types := []string{"", "connection", "source"}
	tabs := make([]uisignals.WorkspaceTabSignal, 0, len(types))
	for _, typ := range types {
		label := "All"
		if typ != "" {
			label = assetTypeLabel(typ)
		}
		tabs = append(tabs, uisignals.WorkspaceTabSignal{ID: typ, Label: label, Href: connectionAssetListHref(typ, query), Active: typ == activeType})
	}
	return tabs
}

func workspaceAssetSummarySignal(workspaceID string, asset workspaceview.AssetView, assetIndex map[string]workspaceview.AssetView, edges []workspaceview.AssetEdgeView) uisignals.WorkspaceAssetSummarySignal {
	detailHref := assetnav.CanonicalAssetSectionHref(workspaceID, asset, "details", edges)
	openHref := detailHref
	if asset.Href != "" {
		openHref = asset.Href
	}
	parentTitle := emptyDash("")
	parentHref := ""
	if asset.Type == "source" {
		if connection, ok := assetIndex[assetnav.SourceConnectionID(asset.ID, edges)]; ok && connection.Type == "connection" {
			parentTitle = assetTitle(connection)
			parentHref = assetnav.ConnectionAssetSectionHref(connection.ID, "details")
		}
	} else if parent, ok := assetIndex[asset.ParentID]; ok {
		parentTitle = assetTitle(parent)
		parentHref = assetnav.WorkspaceAssetSectionHref(workspaceID, parent.ID, "details")
	}
	return uisignals.WorkspaceAssetSummarySignal{
		ID:          asset.ID,
		Title:       assetTitle(asset),
		Description: asset.Description,
		Type:        asset.Type,
		TypeLabel:   assetTypeLabel(asset.Type),
		Key:         asset.Key,
		ParentTitle: parentTitle,
		ParentHref:  parentHref,
		DetailHref:  detailHref,
		OpenHref:    openHref,
	}
}

func workspaceAssetPageSignal(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection string, lineage assetLineageModel) uisignals.WorkspaceAssetPageSignal {
	return workspaceAssetPageSignalWithRefresh(workspace, asset, assets, edges, activeSection, lineage, AssetRefreshState{})
}

func workspaceAssetPageSignalWithRefresh(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection string, lineage assetLineageModel, refresh AssetRefreshState) uisignals.WorkspaceAssetPageSignal {
	page := baseWorkspaceAssetPageSignalWithRefresh(workspace, asset, assets, edges, activeSection, lineage, refresh)
	page.Kind = uisignals.RouteWorkspaceAsset
	page.Breadcrumbs = []uisignals.WorkspaceBreadcrumbSignal{
		{Label: "Workspaces", Href: "/workspaces"},
		{Label: workspace.Title, Href: "/workspaces/" + workspace.ID},
		{Label: assetTitle(asset), Current: true},
	}
	page.Actions = []uisignals.WorkspaceActionSignal{{Label: "Back to workspace", Href: "/workspaces/" + workspace.ID, Icon: "back"}}
	if assetRefreshable(asset.Type) {
		page.Actions = append([]uisignals.WorkspaceActionSignal{{
			Label:    "Refresh materializations",
			Icon:     "refresh",
			Command:  "refresh-materializations",
			Disabled: assetRefreshSignal(refresh).Running,
		}}, page.Actions...)
	}
	if asset.Href != "" {
		page.Actions = append(page.Actions, uisignals.WorkspaceActionSignal{Label: "Open asset", Href: asset.Href, Icon: "open"})
	}
	page.Tabs = []uisignals.WorkspaceTabSignal{
		{ID: "details", Label: "Details", Href: assetnav.WorkspaceAssetSectionHref(workspace.ID, asset.ID, "details"), Active: activeSection == "details"},
	}
	if assetRefreshable(asset.Type) {
		page.Tabs = append(page.Tabs, uisignals.WorkspaceTabSignal{ID: "refreshes", Label: "Refreshes", Href: assetnav.WorkspaceAssetSectionHref(workspace.ID, asset.ID, "refreshes"), Active: activeSection == "refreshes"})
	}
	page.Tabs = append(page.Tabs, uisignals.WorkspaceTabSignal{ID: "lineage", Label: "Lineage", Href: assetnav.WorkspaceAssetSectionHref(workspace.ID, asset.ID, "lineage"), Active: activeSection == "lineage", Count: lineage.Count})
	return page
}

func connectionAssetPageSignal(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection string, lineage assetLineageModel) uisignals.WorkspaceAssetPageSignal {
	page := baseWorkspaceAssetPageSignal(workspace, asset, assets, edges, activeSection, lineage)
	page.Kind = uisignals.RouteConnectionAsset
	page.Breadcrumbs = []uisignals.WorkspaceBreadcrumbSignal{
		{Label: "Connections", Href: "/connections"},
		{Label: assetTitle(asset), Current: true},
	}
	page.Actions = []uisignals.WorkspaceActionSignal{{Label: "Back to connections", Href: "/connections", Icon: "back"}}
	page.Tabs = []uisignals.WorkspaceTabSignal{
		{ID: "details", Label: "Details", Href: assetnav.ConnectionAssetSectionHref(asset.ID, "details"), Active: activeSection == "details"},
		{ID: "lineage", Label: "Lineage", Href: assetnav.ConnectionAssetSectionHref(asset.ID, "lineage"), Active: activeSection == "lineage", Count: lineage.Count},
	}
	return page
}

func connectionSourceAssetPageSignal(workspace workspaceview.WorkspaceView, connection workspaceview.AssetView, source workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection string, lineage assetLineageModel) uisignals.WorkspaceAssetPageSignal {
	page := baseWorkspaceAssetPageSignal(workspace, source, assets, edges, activeSection, lineage)
	page.Kind = uisignals.RouteConnectionAsset
	page.Breadcrumbs = []uisignals.WorkspaceBreadcrumbSignal{
		{Label: "Connections", Href: "/connections"},
		{Label: assetTitle(connection), Href: assetnav.ConnectionAssetSectionHref(connection.ID, "details")},
		{Label: "Sources", Href: "/connections?type=source"},
		{Label: assetTitle(source), Current: true},
	}
	page.Actions = []uisignals.WorkspaceActionSignal{{Label: "Back to sources", Href: "/connections?type=source", Icon: "back"}}
	page.Tabs = []uisignals.WorkspaceTabSignal{
		{ID: "details", Label: "Details", Href: assetnav.ConnectionSourceAssetSectionHref(connection.ID, source.ID, "details"), Active: activeSection == "details"},
		{ID: "lineage", Label: "Lineage", Href: assetnav.ConnectionSourceAssetSectionHref(connection.ID, source.ID, "lineage"), Active: activeSection == "lineage", Count: lineage.Count},
	}
	return page
}

func baseWorkspaceAssetPageSignal(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection string, lineage assetLineageModel) uisignals.WorkspaceAssetPageSignal {
	return baseWorkspaceAssetPageSignalWithRefresh(workspace, asset, assets, edges, activeSection, lineage, AssetRefreshState{})
}

func baseWorkspaceAssetPageSignalWithRefresh(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection string, lineage assetLineageModel, refresh AssetRefreshState) uisignals.WorkspaceAssetPageSignal {
	activeSection = normalizeWorkspaceAssetSection(activeSection)
	page := uisignals.WorkspaceAssetPageSignal{
		Title:         assetTitle(asset),
		WorkspaceID:   workspace.ID,
		AssetID:       asset.ID,
		ActiveSection: activeSection,
		Asset:         workspaceAssetSummarySignal(workspace.ID, asset, assetsByID(assets), edges),
	}
	if assetRefreshable(asset.Type) {
		page.Refresh = assetRefreshSignal(refresh)
	}
	if activeSection == "details" {
		page.Details = workspaceAssetDetailsSignalWithRefresh(workspace, asset, assets, edges, refresh)
	}
	if activeSection == "lineage" {
		page.Lineage = uisignals.WorkspaceAssetLineageSignal{
			Count:      lineage.Count,
			Graph:      lineage.Graph,
			UsesGrid:   lineage.Uses,
			UsedByGrid: lineage.UsedBy,
		}
	}
	if activeSection == "refreshes" && assetRefreshable(asset.Type) {
		runsGrid := assetRefreshesGrid(refresh)
		page.Refresh.RunsGrid = &runsGrid
	}
	return page
}

func workspaceAssetDetailsSignal(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) uisignals.WorkspaceAssetDetailsSignal {
	return workspaceAssetDetailsSignalWithRefresh(workspace, asset, assets, edges, AssetRefreshState{})
}

func workspaceAssetDetailsSignalWithRefresh(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, refresh AssetRefreshState) uisignals.WorkspaceAssetDetailsSignal {
	model := assetDetailModelForAssetWithRefresh(workspace, asset, assets, edges, refresh)
	sections := make([]uisignals.WorkspaceDetailSectionSignal, 0, len(model.Sections))
	for _, section := range model.Sections {
		sections = append(sections, uisignals.WorkspaceDetailSectionSignal{
			Title: section.Title,
			Facts: definitionFactSignals(section.Facts),
			Grid:  section.Grid,
			Code:  section.Code,
			Lang:  section.Lang,
		})
	}
	return uisignals.WorkspaceAssetDetailsSignal{
		Overview: definitionFactSignals(model.Overview),
		Sections: sections,
	}
}

func definitionFactSignals(facts []definitionFact) []uisignals.DefinitionFactSignal {
	out := make([]uisignals.DefinitionFactSignal, 0, len(facts))
	for _, fact := range facts {
		if strings.TrimSpace(fact.Value) == "" {
			continue
		}
		out = append(out, uisignals.DefinitionFactSignal{Label: fact.Label, Value: fact.Value, Code: fact.Code, Wide: fact.Wide})
	}
	return out
}

func WorkspaceAssetPage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection, roleLabel string) g.Node {
	return WorkspaceAssetPageWithRefresh(catalog, workspace, asset, assets, edges, activeSection, roleLabel, AssetRefreshState{})
}

func WorkspaceAssetPageWithRefresh(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection, roleLabel string, refresh AssetRefreshState) g.Node {
	activeSection = normalizeWorkspaceAssetSection(activeSection)
	lineage := assetLineage(workspace.ID, asset, assets, edges)
	page := workspaceAssetPageSignalWithRefresh(workspace, asset, assets, edges, activeSection, lineage, refresh)
	extraSignals := map[string]any{}
	attrs := []g.Node{
		g.Attr("slot", "page"),
		g.Attr("page", jsonString(page)),
		g.Attr("data-attr:page", "JSON.stringify($page)"),
	}
	if assetRefreshable(asset.Type) {
		refreshPath := "/workspaces/" + workspace.ID + "/assets/" + asset.ID + "/refresh-materializations"
		updatesURL := "/workspaces/" + workspace.ID + "/assets/" + asset.ID + "/updates?section=" + activeSection
		extraSignals["csrfToken"] = refresh.CSRFToken
		attrs = append(attrs,
			g.Attr("data-on:ld-refresh-materializations", "$page.refresh.status = 'running'; $page.refresh.running = true; "+postActionWithCSRFSignal(refreshPath, "$csrfToken")),
		)
		extraHeadInit := ds.Init("@get('" + updatesURL + "', {openWhenHidden: true})")
		return workspaceAssetRouteDocument(asset, catalog, "workspaces", roleLabel, page, uisignals.RouteWorkspaceAsset, g.El("ld-workspace-asset-page", attrs...), extraSignals, activeSection, extraHeadInit)
	}
	return workspaceAssetRouteDocument(asset, catalog, "workspaces", roleLabel, page, uisignals.RouteWorkspaceAsset, g.El("ld-workspace-asset-page", attrs...), nil, activeSection)
}

func workspaceAssetRouteDocument(asset workspaceview.AssetView, catalog dashboard.Catalog, active, roleLabel string, page any, routeKind uisignals.RouteKind, routeRoot g.Node, extraSignals map[string]any, activeSection string, bodyExtras ...g.Node) g.Node {
	extraHead := []g.Node{}
	if activeSection == "lineage" {
		extraHead = append(extraHead,
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/asset-lineage-graph.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/asset-lineage-graph.js"))),
		)
	}
	return workspaceRouteDocumentWithBodyExtras(asset.Title, catalog, active, roleLabel, page, routeKind, routeRoot, extraSignals, bodyExtras, extraHead...)
}

func ConnectionAssetPage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection, roleLabel string) g.Node {
	activeSection = normalizeWorkspaceAssetSection(activeSection)
	lineage := assetLineage(workspace.ID, asset, assets, edges)
	page := connectionAssetPageSignal(workspace, asset, assets, edges, activeSection, lineage)
	extraHead := []g.Node{}
	if activeSection == "lineage" {
		extraHead = append(extraHead,
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/asset-lineage-graph.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/asset-lineage-graph.js"))),
		)
	}
	return workspaceRouteDocument(asset.Title, catalog, "connections", roleLabel, page, uisignals.RouteConnectionAsset,
		g.El("ld-workspace-asset-page",
			g.Attr("slot", "page"),
			g.Attr("page", jsonString(page)),
			g.Attr("data-attr:page", "JSON.stringify($page)"),
		),
		nil,
		extraHead...,
	)
}

func ConnectionSourceAssetPage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, connection workspaceview.AssetView, source workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection, roleLabel string) g.Node {
	activeSection = normalizeWorkspaceAssetSection(activeSection)
	lineage := assetLineage(workspace.ID, source, assets, edges)
	page := connectionSourceAssetPageSignal(workspace, connection, source, assets, edges, activeSection, lineage)
	extraHead := []g.Node{}
	if activeSection == "lineage" {
		extraHead = append(extraHead,
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/asset-lineage-graph.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/asset-lineage-graph.js"))),
		)
	}
	return workspaceRouteDocument(source.Title, catalog, "connections", roleLabel, page, uisignals.RouteConnectionAsset,
		g.El("ld-workspace-asset-page",
			g.Attr("slot", "page"),
			g.Attr("page", jsonString(page)),
			g.Attr("data-attr:page", "JSON.stringify($page)"),
		),
		nil,
		extraHead...,
	)
}

func WorkspacePermissionsPage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, bindings []workspaceview.RoleBindingView, roles []workspaceview.RoleView, csrfToken, roleLabel string) g.Node {
	page := uisignals.WorkspacePageSignal{
		Kind:        uisignals.RouteWorkspace,
		Title:       workspace.Title,
		Description: "Assign workspace roles. BI assets remain authored in Git.",
		WorkspaceID: workspace.ID,
	}
	access := WorkspaceAccessResponse{
		Workspace: workspace,
		Roles:     roles,
		Bindings:  bindings,
		CanManage: true,
	}
	attrs := []g.Node{
		g.Attr("slot", "page"),
		g.Attr("page", jsonString(page)),
		g.Attr("data-attr:page", "JSON.stringify($page)"),
	}
	accessAttrs, extraSignals := workspaceAccessRouteBridge(workspace.ID, access, csrfToken)
	attrs = append(attrs, accessAttrs...)
	return workspaceRouteDocument("Workspace permissions", catalog, "settings", roleLabel, page, uisignals.RouteWorkspace,
		g.El("ld-workspace-page", attrs...),
		extraSignals,
	)
}

func workspaceRouteDocument(title string, catalog dashboard.Catalog, active, roleLabel string, page any, routeKind uisignals.RouteKind, routeRoot g.Node, extraSignals map[string]any, extraHead ...g.Node) g.Node {
	return workspaceRouteDocumentWithBodyExtras(title, catalog, active, roleLabel, page, routeKind, routeRoot, extraSignals, nil, extraHead...)
}

func workspaceRouteDocumentWithBodyExtras(title string, catalog dashboard.Catalog, active, roleLabel string, page any, routeKind uisignals.RouteKind, routeRoot g.Node, extraSignals map[string]any, bodyExtras []g.Node, extraHead ...g.Node) g.Node {
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForWorkspace(catalog, active, roleLabel)}
	signals := map[string]any{
		"chrome":  chrome,
		"page":    page,
		"runtime": uisignals.RouteRuntimeSignal{Kind: routeKind},
		"status":  dashboard.Status{},
	}
	for key, value := range extraSignals {
		signals[key] = value
	}
	head := []g.Node{
		h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
		h.Script(h.Type("module"), h.Src(staticAsset("/static/workspace-page.js"))),
		inspectorScript(),
		h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
	}
	head = append(head, extraHead...)
	mainChildren := []g.Node{ds.Signals(signals)}
	mainChildren = append(mainChildren, bodyExtras...)
	mainChildren = append(mainChildren,
		g.El("ld-app-shell",
			g.Attr("chrome", jsonString(chrome)),
			g.Attr("data-attr:chrome", "JSON.stringify($chrome)"),
			routeRoot,
		),
		inspectorElement(),
	)
	return c.HTML5(c.HTML5Props{
		Title:    title,
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(head...),
		Body: []g.Node{
			h.Main(append([]g.Node{h.Class(appRootClass)}, mainChildren...)...),
		},
	})
}

func activeDeploymentLabel(workspace workspaceview.WorkspaceView) string {
	if workspace.ActiveDeploymentID == "" {
		return "Local catalog"
	}
	return "Published deployment"
}

func connectionAssetListHref(typ, query string) string {
	href := "/connections"
	values := url.Values{}
	if typ != "" {
		values.Set("type", typ)
	}
	if strings.TrimSpace(query) != "" {
		values.Set("q", query)
	}
	if encoded := values.Encode(); encoded != "" {
		href += "?" + encoded
	}
	return href
}

func workspaceAssetHref(workspaceID, typ, query string) string {
	href := "/workspaces/" + workspaceID
	values := url.Values{}
	if typ != "" {
		values.Set("type", typ)
	}
	if strings.TrimSpace(query) != "" {
		values.Set("q", query)
	}
	if encoded := values.Encode(); encoded != "" {
		href += "?" + encoded
	}
	return href
}

func ValidWorkspaceAssetSection(section string) bool {
	switch section {
	case "details", "lineage", "refreshes":
		return true
	default:
		return false
	}
}

type AssetRefreshState struct {
	CSRFToken        string
	UpdatesURL       string
	Runs             []AssetRefreshRun
	Latest           AssetRefreshRun
	LatestSuccessful AssetRefreshRun
}

type AssetRefreshRun struct {
	ID                   string
	ModelID              string
	DeploymentID         string
	PrincipalID          string
	PrincipalDisplayName string
	TargetType           string
	TargetID             string
	TriggerType          string
	ParentRunID          string
	Status               string
	StartedAt            string
	FinishedAt           string
	Error                string
}

func WorkspaceAssetRefreshSignals(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, refresh AssetRefreshState, activeSection string) map[string]any {
	lineage := assetLineage(workspace.ID, asset, assets, edges)
	return map[string]any{
		"page": workspaceAssetPageSignalWithRefresh(workspace, asset, assets, edges, activeSection, lineage, refresh),
	}
}

func assetRefreshSignal(refresh AssetRefreshState) uisignals.WorkspaceAssetRefreshSignal {
	status := strings.TrimSpace(refresh.Latest.Status)
	if status == "" {
		status = "not refreshed"
	}
	return uisignals.WorkspaceAssetRefreshSignal{
		Status:         status,
		Running:        status == "queued" || status == "running",
		LastSuccessful: refresh.LatestSuccessful.FinishedAt,
	}
}

func assetRefreshesGrid(refresh AssetRefreshState) metricGrid {
	rows := make([]map[string]any, 0, len(refresh.Runs))
	for _, run := range refresh.Runs {
		rows = append(rows, map[string]any{
			"status":       refreshStatusGridValue(run.Status),
			"started":      emptyDash(run.StartedAt),
			"duration":     emptyDash(refreshRunDuration(run)),
			"triggered_by": emptyDash(run.PrincipalDisplayName),
			"trigger":      refreshTriggerLabel(run.TriggerType),
			"run":          emptyDash(shortRefreshRunID(run.ID)),
			"error":        emptyDash(run.Error),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "status", Header: "Status", Kind: "status", Width: "140px"},
			{ID: "started", Header: "Started", Width: "180px"},
			{ID: "duration", Header: "Duration", Width: "110px"},
			{ID: "triggered_by", Header: "Triggered by", Width: "130px"},
			{ID: "trigger", Header: "Trigger", Width: "130px"},
			{ID: "run", Header: "Run ID", Kind: "code", Width: "160px"},
			{ID: "error", Header: "Error"},
		},
		Rows:     rows,
		Empty:    "No refresh runs have been recorded for this asset.",
		MinWidth: "1040px",
	}
}

func refreshTriggerLabel(trigger string) string {
	switch strings.TrimSpace(trigger) {
	case "direct":
		return "Direct"
	case "semantic_model":
		return "Semantic model"
	case "dependency":
		return "Dependency"
	default:
		return "-"
	}
}

func refreshStatusGridValue(status string) any {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "not refreshed"
	}
	return metricGridBadge{Label: status, Tone: refreshStatusBadgeTone(status)}
}

func refreshStatusBadgeTone(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded":
		return "success"
	case "running", "queued":
		return "accent"
	case "failed":
		return "danger"
	default:
		return "muted"
	}
}

func shortRefreshRunID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 18 {
		return id
	}
	return id[:18]
}

func refreshRunDuration(run AssetRefreshRun) string {
	started, ok := parseRefreshTime(run.StartedAt)
	if !ok {
		return ""
	}
	finished, ok := parseRefreshTime(run.FinishedAt)
	if !ok || finished.Before(started) {
		return ""
	}
	return finished.Sub(started).Round(time.Second).String()
}

func parseRefreshTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func assetRefreshable(assetType string) bool {
	return assetType == "semantic_model" || assetType == "model_table"
}

func normalizeWorkspaceAssetSection(section string) string {
	if ValidWorkspaceAssetSection(section) {
		return section
	}
	return "details"
}

type assetLineageModel struct {
	Count  int
	Graph  assetLineageGraph
	Uses   metricGrid
	UsedBy metricGrid
}

type assetLineageGraph = uisignals.AssetLineageGraphSignal
type assetLineageNode = uisignals.AssetLineageNodeSignal
type assetLineageEdge = uisignals.AssetLineageEdgeSignal

func assetLineage(workspaceID string, selected workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) assetLineageModel {
	byID := assetsByID(assets)
	outgoing := edgesByFromAsset(edges)
	incoming := edgesByToAsset(edges)
	graph := assetLineageGraph{
		Nodes: []assetLineageNode{lineageNode(workspaceID, selected, 0, true, edges)},
	}
	nodeIndex := map[string]int{selected.ID: 0}
	seenEdges := map[string]struct{}{}

	addNode := func(asset workspaceview.AssetView, rank int, selected bool) {
		if asset.ID == "" {
			return
		}
		if !selected && isLineageHiddenContextAsset(asset) {
			return
		}
		if existing, ok := nodeIndex[asset.ID]; ok {
			node := graph.Nodes[existing]
			if !node.Selected && absInt(rank) < absInt(node.Rank) {
				node.Rank = rank
				node.Side = lineageSideForRank(rank)
				graph.Nodes[existing] = node
			}
			return
		}
		nodeIndex[asset.ID] = len(graph.Nodes)
		graph.Nodes = append(graph.Nodes, lineageNode(workspaceID, asset, rank, selected, edges))
	}
	addEdge := func(edge workspaceview.AssetEdgeView) {
		if edge.FromAssetID == "" || edge.ToAssetID == "" {
			return
		}
		if _, ok := nodeIndex[edge.FromAssetID]; !ok {
			return
		}
		if _, ok := nodeIndex[edge.ToAssetID]; !ok {
			return
		}
		key := lineageEdgeKey(edge)
		if _, ok := seenEdges[key]; ok {
			return
		}
		graph.Edges = append(graph.Edges, assetLineageEdge{
			ID:     key,
			Source: edge.FromAssetID,
			Target: edge.ToAssetID,
			Label:  labelFromKey(edge.Type),
			Kind:   edge.Type,
		})
		seenEdges[key] = struct{}{}
	}

	type lineageWalkConfig struct {
		edges  func(string) []workspaceview.AssetEdgeView
		peerID func(workspaceview.AssetEdgeView) string
		rank   func(int) int
	}
	var walkDependencyEdges func(assetID string, depth int, visiting map[string]struct{}, config lineageWalkConfig)
	walkDependencyEdges = func(assetID string, depth int, visiting map[string]struct{}, config lineageWalkConfig) {
		if _, ok := visiting[assetID]; ok {
			return
		}
		visiting[assetID] = struct{}{}
		defer delete(visiting, assetID)
		for _, edge := range sortedLineageEdges(config.edges(assetID), byID) {
			if !isLineageDependencyEdge(edge) {
				continue
			}
			asset, ok := byID[config.peerID(edge)]
			if !ok {
				continue
			}
			addNode(asset, config.rank(depth), false)
			addEdge(edge)
			walkDependencyEdges(asset.ID, depth+1, visiting, config)
		}
	}

	upstreamWalk := lineageWalkConfig{
		edges: func(assetID string) []workspaceview.AssetEdgeView {
			return outgoing[assetID]
		},
		peerID: func(edge workspaceview.AssetEdgeView) string {
			return edge.ToAssetID
		},
		rank: func(depth int) int {
			return -depth
		},
	}
	downstreamWalk := lineageWalkConfig{
		edges: func(assetID string) []workspaceview.AssetEdgeView {
			return incoming[assetID]
		},
		peerID: func(edge workspaceview.AssetEdgeView) string {
			return edge.FromAssetID
		},
		rank: func(depth int) int {
			return depth
		},
	}

	for _, rootID := range lineageDependencyRootIDs(selected, outgoing, byID) {
		if rootID != selected.ID {
			addNode(byID[rootID], 1, false)
		}
		walkDependencyEdges(rootID, 1, map[string]struct{}{}, upstreamWalk)
		if rootID != selected.ID {
			walkDependencyEdges(rootID, 1, map[string]struct{}{}, downstreamWalk)
		}
	}
	walkDependencyEdges(selected.ID, 1, map[string]struct{}{}, downstreamWalk)
	addContainsContext(selected.ID, &graph, nodeIndex, byID, edges, addNode, addEdge)

	sortLineageNodes(graph.Nodes)
	sortLineageGraphEdges(graph.Edges)
	collapsedGraph := collapsedAssetLineageGraph(workspaceID, selected, graph, byID, edges)
	enrichAssetLineageGraph(collapsedGraph, byID, edges)
	usesRows, usedByRows := lineageTablesFromGraph(workspaceID, selected, collapsedGraph, byID, edges)
	return assetLineageModel{
		Count:  len(usesRows) + len(usedByRows),
		Graph:  collapsedGraph,
		Uses:   lineageTable(usesRows, "This asset does not reference other assets."),
		UsedBy: lineageTable(usedByRows, "No assets reference this asset."),
	}
}

func enrichAssetLineageGraph(graph assetLineageGraph, assets map[string]workspaceview.AssetView, edges []workspaceview.AssetEdgeView) {
	nodeIndex := map[string]int{}
	for index, node := range graph.Nodes {
		nodeIndex[node.ID] = index
	}
	for _, edge := range graph.Edges {
		if sourceIndex, ok := nodeIndex[edge.Source]; ok {
			graph.Nodes[sourceIndex].VisibleDownstream++
		}
		if targetIndex, ok := nodeIndex[edge.Target]; ok {
			graph.Nodes[targetIndex].VisibleUpstream++
		}
	}

	containsByParent := map[string]map[string]int{}
	for _, edge := range edges {
		if isLineageDependencyEdge(edge) {
			if index, ok := nodeIndex[edge.FromAssetID]; ok {
				graph.Nodes[index].UsesCount++
			}
			if index, ok := nodeIndex[edge.ToAssetID]; ok {
				graph.Nodes[index].UsedByCount++
			}
			continue
		}
		if !isContainsEdge(edge) {
			continue
		}
		child, ok := assets[edge.ToAssetID]
		if !ok {
			continue
		}
		if _, ok := containsByParent[edge.FromAssetID]; !ok {
			containsByParent[edge.FromAssetID] = map[string]int{}
		}
		containsByParent[edge.FromAssetID][child.Type]++
	}
	for index, node := range graph.Nodes {
		contains := containsByParent[node.ID]
		if len(contains) == 0 {
			continue
		}
		count := 0
		for _, value := range contains {
			count += value
		}
		graph.Nodes[index].ContainedCount = count
		graph.Nodes[index].ContainedSummary = lineageContainedSummary(contains)
	}
}

func lineageContainedSummary(counts map[string]int) string {
	types := make([]string, 0, len(counts))
	for typ := range counts {
		types = append(types, typ)
	}
	sort.Slice(types, func(i, j int) bool {
		return assetTypeLabel(types[i]) < assetTypeLabel(types[j])
	})
	parts := make([]string, 0, len(types))
	for _, typ := range types {
		parts = append(parts, fmt.Sprintf("%d %s", counts[typ], pluralAssetTypeLabel(typ, counts[typ])))
	}
	return strings.Join(parts, ", ")
}

func pluralAssetTypeLabel(typ string, count int) string {
	label := strings.ToLower(assetTypeLabel(typ))
	if count == 1 {
		return label
	}
	if strings.HasSuffix(label, "y") && len(label) > 1 {
		return strings.TrimSuffix(label, "y") + "ies"
	}
	return label + "s"
}

func collapsedAssetLineageGraph(workspaceID string, selected workspaceview.AssetView, graph assetLineageGraph, assets map[string]workspaceview.AssetView, edges []workspaceview.AssetEdgeView) assetLineageGraph {
	if selected.Type == "catalog" {
		return graph
	}
	selectedAnchor, selectedAnchorOK := lineageVisibleAnchor(selected, assets)
	out := assetLineageGraph{}
	nodeIndex := map[string]int{}
	addNode := func(asset workspaceview.AssetView) {
		if asset.ID == "" {
			return
		}
		if _, ok := nodeIndex[asset.ID]; ok {
			return
		}
		selectedNode := selectedAnchorOK && asset.ID == selectedAnchor.ID
		nodeIndex[asset.ID] = len(out.Nodes)
		out.Nodes = append(out.Nodes, lineageNode(workspaceID, asset, lineageVisualLayer(asset.Type), selectedNode, edges))
	}

	type collapsedEdge struct {
		source string
		target string
		kind   string
		label  string
	}
	candidates := []collapsedEdge{}
	for _, node := range graph.Nodes {
		asset, ok := assets[node.ID]
		if !ok {
			continue
		}
		if anchor, ok := lineageVisibleAnchor(asset, assets); ok {
			addNode(anchor)
		}
	}
	for _, edge := range graph.Edges {
		if !isLineageDependencyEdge(workspaceview.AssetEdgeView{Type: edge.Kind}) {
			continue
		}
		consumer, consumerOK := assets[edge.Source]
		provider, providerOK := assets[edge.Target]
		if !consumerOK || !providerOK {
			continue
		}
		source, sourceOK := lineageVisibleAnchor(provider, assets)
		target, targetOK := lineageVisibleAnchor(consumer, assets)
		if !sourceOK || !targetOK || source.ID == target.ID {
			continue
		}
		policy := lineageProjectionEdge(source.Type, target.Type, edge.Kind)
		addNode(source)
		addNode(target)
		candidates = append(candidates, collapsedEdge{
			source: source.ID,
			target: target.ID,
			kind:   policy.kind,
			label:  policy.label,
		})
	}

	seenEdges := map[string]struct{}{}
	for _, edge := range candidates {
		source := assets[edge.source]
		target := assets[edge.target]
		if lineageVisualLayer(source.Type) >= lineageVisualLayer(target.Type) {
			continue
		}
		key := edge.source + "|" + edge.target + "|" + edge.kind
		if _, ok := seenEdges[key]; ok {
			continue
		}
		seenEdges[key] = struct{}{}
		out.Edges = append(out.Edges, assetLineageEdge{
			ID:     key,
			Source: edge.source,
			Target: edge.target,
			Label:  edge.label,
			Kind:   edge.kind,
		})
	}
	sortLineageNodes(out.Nodes)
	sortLineageGraphEdges(out.Edges)
	return out
}

func edgesByFromAsset(edges []workspaceview.AssetEdgeView) map[string][]workspaceview.AssetEdgeView {
	out := map[string][]workspaceview.AssetEdgeView{}
	for _, edge := range edges {
		out[edge.FromAssetID] = append(out[edge.FromAssetID], edge)
	}
	return out
}

func edgesByToAsset(edges []workspaceview.AssetEdgeView) map[string][]workspaceview.AssetEdgeView {
	out := map[string][]workspaceview.AssetEdgeView{}
	for _, edge := range edges {
		out[edge.ToAssetID] = append(out[edge.ToAssetID], edge)
	}
	return out
}

func sortLineageRows(rows []map[string]any) {
	sort.Slice(rows, func(i, j int) bool {
		return fmt.Sprint(rows[i]["relation"], rows[i]["type"], rows[i]["asset"]) < fmt.Sprint(rows[j]["relation"], rows[j]["type"], rows[j]["asset"])
	})
}

func lineageTablesFromGraph(workspaceID string, selected workspaceview.AssetView, graph assetLineageGraph, assets map[string]workspaceview.AssetView, edges []workspaceview.AssetEdgeView) ([]map[string]any, []map[string]any) {
	anchorID := selected.ID
	if selected.Type != "catalog" {
		if anchor, ok := lineageVisibleAnchor(selected, assets); ok {
			anchorID = anchor.ID
		}
	}
	usesRows := []map[string]any{}
	usedByRows := []map[string]any{}
	hasDependencyEdges := false
	for _, edge := range graph.Edges {
		if edge.Kind != "contains" {
			hasDependencyEdges = true
			break
		}
	}
	for _, edge := range graph.Edges {
		if hasDependencyEdges {
			if edge.Kind == "contains" {
				continue
			}
			if edge.Target == anchorID {
				if peer, ok := assets[edge.Source]; ok {
					usesRows = append(usesRows, lineageGraphTableRow(workspaceID, edge, peer, edges))
				}
			}
			if edge.Source == anchorID {
				if peer, ok := assets[edge.Target]; ok {
					usedByRows = append(usedByRows, lineageGraphTableRow(workspaceID, edge, peer, edges))
				}
			}
			continue
		}
		if edge.Source == anchorID {
			if peer, ok := assets[edge.Target]; ok {
				usesRows = append(usesRows, lineageGraphTableRow(workspaceID, edge, peer, edges))
			}
		}
		if edge.Target == anchorID {
			if peer, ok := assets[edge.Source]; ok {
				usedByRows = append(usedByRows, lineageGraphTableRow(workspaceID, edge, peer, edges))
			}
		}
	}
	sortLineageRows(usesRows)
	sortLineageRows(usedByRows)
	return usesRows, usedByRows
}

func lineageGraphTableRow(workspaceID string, edge assetLineageEdge, peer workspaceview.AssetView, edges []workspaceview.AssetEdgeView) map[string]any {
	return map[string]any{
		"relation":  firstNonEmpty(edge.Label, labelFromKey(edge.Kind)),
		"asset":     assetTitle(peer),
		"assetHref": lineageAssetHref(workspaceID, peer, edges),
		"type":      assetTypeLabel(peer.Type),
		"key":       peer.Key,
	}
}

func lineageTable(rows []map[string]any, empty string) metricGrid {
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "relation", Header: "Relationship", Width: "190px"},
			{ID: "asset", Header: "Asset", Kind: "link", HrefKey: "assetHref", Width: "260px"},
			{ID: "type", Header: "Type", Width: "150px"},
			{ID: "key", Header: "Key", Kind: "code"},
		},
		Rows:     rows,
		Empty:    empty,
		MinWidth: "760px",
	}
}

func lineageNode(workspaceID string, asset workspaceview.AssetView, rank int, selected bool, edges []workspaceview.AssetEdgeView) assetLineageNode {
	return assetLineageNode{
		ID:       asset.ID,
		Label:    assetTitle(asset),
		Kind:     asset.Type,
		Meta:     asset.Key,
		Href:     lineageAssetHref(workspaceID, asset, edges),
		Side:     lineageSideForRank(rank),
		Rank:     rank,
		Selected: selected,
	}
}

func lineageSideForRank(rank int) string {
	switch {
	case rank < 0:
		return "upstream"
	case rank > 0:
		return "downstream"
	default:
		return "selected"
	}
}

func lineageDependencyRootIDs(selected workspaceview.AssetView, outgoing map[string][]workspaceview.AssetEdgeView, assets map[string]workspaceview.AssetView) []string {
	rootIDs := []string{selected.ID}
	if !isRollupLineageAsset(selected.Type) {
		return rootIDs
	}
	seen := map[string]struct{}{selected.ID: {}}
	var walk func(string)
	walk = func(assetID string) {
		for _, edge := range sortedLineageEdges(outgoing[assetID], assets) {
			if !isContainsEdge(edge) {
				continue
			}
			if _, ok := seen[edge.ToAssetID]; ok {
				continue
			}
			if _, ok := assets[edge.ToAssetID]; !ok {
				continue
			}
			seen[edge.ToAssetID] = struct{}{}
			rootIDs = append(rootIDs, edge.ToAssetID)
			walk(edge.ToAssetID)
		}
	}
	walk(selected.ID)
	return rootIDs
}

func isLineageHiddenContextAsset(asset workspaceview.AssetView) bool {
	return asset.Type == "catalog"
}

func lineageVisibleAnchor(asset workspaceview.AssetView, assets map[string]workspaceview.AssetView) (workspaceview.AssetView, bool) {
	current := asset
	seen := map[string]struct{}{}
	for current.ID != "" {
		if isLineageVisibleGraphAsset(current.Type) {
			return current, true
		}
		if _, ok := seen[current.ID]; ok {
			return workspaceview.AssetView{}, false
		}
		seen[current.ID] = struct{}{}
		parent, ok := assets[current.ParentID]
		if !ok {
			return workspaceview.AssetView{}, false
		}
		current = parent
	}
	return workspaceview.AssetView{}, false
}

func isLineageVisibleGraphAsset(typ string) bool {
	return lineageVisualLayer(typ) >= 0
}

type lineageProjectionLayerPolicy struct {
	assetType string
	layer     int
}

var lineageProjectionLayers = []lineageProjectionLayerPolicy{
	{assetType: "connection", layer: 0},
	{assetType: "source", layer: 1},
	{assetType: "model_table", layer: 2},
	{assetType: "semantic_model", layer: 3},
	{assetType: "dashboard", layer: 4},
}

func lineageVisualLayer(typ string) int {
	for _, policy := range lineageProjectionLayers {
		if typ == policy.assetType {
			return policy.layer
		}
	}
	return -1
}

type lineageProjectionEdgeKey struct {
	sourceType string
	targetType string
}

type lineageProjectionEdgePolicy struct {
	key   lineageProjectionEdgeKey
	kind  string
	label string
}

var lineageProjectionEdges = []lineageProjectionEdgePolicy{
	{
		key:   lineageProjectionEdgeKey{sourceType: "connection", targetType: "source"},
		kind:  "lineage_connection_source",
		label: "Provides source",
	},
	{
		key:   lineageProjectionEdgeKey{sourceType: "source", targetType: "model_table"},
		kind:  "lineage_source_model_table",
		label: "Feeds model table",
	},
	{
		key:   lineageProjectionEdgeKey{sourceType: "model_table", targetType: "semantic_model"},
		kind:  "lineage_model_table_semantic_model",
		label: "Feeds semantic model",
	},
	{
		key:   lineageProjectionEdgeKey{sourceType: "semantic_model", targetType: "dashboard"},
		kind:  "lineage_semantic_model_dashboard",
		label: "Powers dashboard",
	},
}

func lineageProjectionEdge(sourceType, targetType, fallback string) lineageProjectionEdgePolicy {
	key := lineageProjectionEdgeKey{sourceType: sourceType, targetType: targetType}
	for _, policy := range lineageProjectionEdges {
		if policy.key == key {
			return policy
		}
	}
	return lineageProjectionEdgePolicy{
		key:   key,
		kind:  fallback,
		label: labelFromKey(fallback),
	}
}

func lineageCollapsedEdgeKind(sourceType, targetType, fallback string) string {
	return lineageProjectionEdge(sourceType, targetType, fallback).kind
}

func lineageCollapsedEdgeLabel(sourceType, targetType, fallback string) string {
	return lineageProjectionEdge(sourceType, targetType, fallback).label
}

func isRollupLineageAsset(typ string) bool {
	switch typ {
	case "dashboard", "page", "semantic_model":
		return true
	default:
		return false
	}
}

func addContainsContext(selectedID string, graph *assetLineageGraph, nodeIndex map[string]int, assets map[string]workspaceview.AssetView, edges []workspaceview.AssetEdgeView, addNode func(workspaceview.AssetView, int, bool), addEdge func(workspaceview.AssetEdgeView)) {
	containsEdges := make([]workspaceview.AssetEdgeView, 0)
	for _, edge := range edges {
		if isContainsEdge(edge) {
			containsEdges = append(containsEdges, edge)
		}
	}
	containsEdges = sortedLineageEdges(containsEdges, assets)
	for _, edge := range containsEdges {
		fromIndex, fromOK := nodeIndex[edge.FromAssetID]
		toIndex, toOK := nodeIndex[edge.ToAssetID]
		if !fromOK && !toOK {
			continue
		}
		if fromOK && toOK {
			addEdge(edge)
			continue
		}
		if fromOK && edge.FromAssetID == selectedID {
			asset, ok := assets[edge.ToAssetID]
			if !ok {
				continue
			}
			addNode(asset, graph.Nodes[fromIndex].Rank+1, false)
			addEdge(edge)
			continue
		}
		if toOK {
			asset, ok := assets[edge.FromAssetID]
			if !ok {
				continue
			}
			addNode(asset, graph.Nodes[toIndex].Rank-1, false)
			addEdge(edge)
		}
	}
}

func isLineageDependencyEdge(edge workspaceview.AssetEdgeView) bool {
	return !isContainsEdge(edge)
}

func isContainsEdge(edge workspaceview.AssetEdgeView) bool {
	return edge.Type == "contains"
}

func lineageEdgeKey(edge workspaceview.AssetEdgeView) string {
	if edge.ID != "" {
		return edge.ID
	}
	return edge.FromAssetID + "|" + edge.ToAssetID + "|" + edge.Type
}

func sortedLineageEdges(edges []workspaceview.AssetEdgeView, assets map[string]workspaceview.AssetView) []workspaceview.AssetEdgeView {
	out := append([]workspaceview.AssetEdgeView(nil), edges...)
	sort.SliceStable(out, func(i, j int) bool {
		return lineageEdgeSortKey(out[i], assets) < lineageEdgeSortKey(out[j], assets)
	})
	return out
}

func lineageEdgeSortKey(edge workspaceview.AssetEdgeView, assets map[string]workspaceview.AssetView) string {
	from := assets[edge.FromAssetID]
	to := assets[edge.ToAssetID]
	return edge.Type + ":" + assetTitle(from) + ":" + assetTitle(to) + ":" + lineageEdgeKey(edge)
}

func sortLineageNodes(nodes []assetLineageNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		left := nodes[i]
		right := nodes[j]
		if left.Rank != right.Rank {
			return left.Rank < right.Rank
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		return left.ID < right.ID
	})
}

func sortLineageGraphEdges(edges []assetLineageEdge) {
	sort.SliceStable(edges, func(i, j int) bool {
		left := edges[i]
		right := edges[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if left.Target != right.Target {
			return left.Target < right.Target
		}
		return left.ID < right.ID
	})
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func lineageAssetHref(workspaceID string, asset workspaceview.AssetView, edges []workspaceview.AssetEdgeView) string {
	return assetnav.CanonicalAssetSectionHref(workspaceID, asset, "details", edges)
}

type assetDetailModel struct {
	Overview []definitionFact
	Sections []assetDetailSection
}

type assetDetailSection struct {
	Title  string
	Signal string
	Grid   metricGrid
	Facts  []definitionFact
	Code   string
	Lang   string
}

func assetDetailUsesCodeBlock(asset workspaceview.AssetView) bool {
	return asset.Type == "model_table" && modelTableSQL(asset.Payload) != ""
}

func assetDetailModelForAsset(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) assetDetailModel {
	return assetDetailModelForAssetWithRefresh(workspace, asset, assets, edges, AssetRefreshState{})
}

func assetDetailModelForAssetWithRefresh(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, refresh AssetRefreshState) assetDetailModel {
	model := assetDetailModel{
		Overview: commonAssetOverviewFacts(asset, assets, shouldShowParentFact(asset.Type)),
	}
	switch asset.Type {
	case "semantic_model":
		semanticModelDetailModel(&model, workspace, asset, assets, refresh)
	case "model_table":
		modelTableDetailModel(&model, workspace, asset, assets, refresh)
	case "dashboard":
		dashboardDetailModel(&model, asset, assets)
	case "connection":
		connectionDetailModel(&model, workspace, asset, assets, edges)
	case "source":
		sourceDetailModel(&model, asset)
	case "measure":
		model.Overview = append(model.Overview, metricLeafFacts(asset)...)
	case "field":
		model.Overview = append(model.Overview, metricLeafFacts(asset)...)
	default:
		model.Overview = append(model.Overview, metaFacts(asset.Payload)...)
	}
	return model
}

func commonAssetOverviewFacts(asset workspaceview.AssetView, assets []workspaceview.AssetView, includeParent bool) []definitionFact {
	facts := []definitionFact{
		{Label: "Type", Value: assetTypeLabel(asset.Type)},
		{Label: "Key", Value: asset.Key, Code: true},
	}
	if includeParent {
		facts = append(facts, definitionFact{Label: "Parent", Value: assetParentTitle(asset.ParentID, assets)})
	}
	facts = append(facts, definitionFact{Label: "Description", Value: asset.Description, Wide: true})
	return facts
}

func shouldShowParentFact(typ string) bool {
	switch typ {
	case "catalog", "connection", "dashboard", "model_table", "semantic_model":
		return false
	default:
		return true
	}
}

func semanticModelDetailModel(model *assetDetailModel, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, refresh AssetRefreshState) {
	meta := asset.Payload
	modelTableMeta := metaMap(meta, "Tables", "tables", "Models", "models")
	modelTables := sortedMapKeys(modelTableMeta)
	measures := sortedMapKeys(metaMap(meta, "Measures", "measures"))
	relationships := metaSlice(meta, "Relationships", "relationships")

	model.Overview = append(model.Overview,
		refreshOverviewFacts(refresh)...,
	)
	model.Sections = append(model.Sections,
		assetDetailSection{Title: fmt.Sprintf("Model tables (%d)", len(modelTables)), Signal: "assetDetailsSemanticModelTablesGrid", Grid: semanticModelTablesGrid(workspace.ID, asset, assets, meta, refresh)},
		assetDetailSection{Title: fmt.Sprintf("Measures (%d)", len(measures)), Signal: "assetDetailsSemanticMeasuresGrid", Grid: semanticMeasuresGrid(workspace.ID, asset, assets, meta)},
		assetDetailSection{Title: fmt.Sprintf("Relationships (%d)", len(relationships)), Signal: "assetDetailsSemanticRelationshipsGrid", Grid: semanticRelationshipsGrid(workspace.ID, asset, assets, meta)},
	)
}

func refreshOverviewFacts(refresh AssetRefreshState) []definitionFact {
	status := strings.TrimSpace(refresh.Latest.Status)
	if status == "" {
		status = "not refreshed"
	}
	return []definitionFact{
		{Label: "Refresh status", Value: status},
		{Label: "Last refreshed", Value: emptyDash(refresh.LatestSuccessful.FinishedAt)},
	}
}

func semanticFieldCount(tables map[string]any) int {
	count := 0
	for _, tableValue := range tables {
		table := asMap(tableValue)
		count += len(metaMap(table, "Dimensions", "dimensions", "Fields", "fields"))
	}
	return count
}

func assetParentTitle(parentID string, assets []workspaceview.AssetView) string {
	if parentID == "" {
		return ""
	}
	for _, asset := range assets {
		if asset.ID == parentID {
			return assetTitle(asset)
		}
	}
	return parentID
}

func semanticConnectionsGrid(workspaceID string, parent workspaceview.AssetView, assets []workspaceview.AssetView, meta map[string]any) metricGrid {
	connections := metaMap(meta, "Connections", "connections")
	rows := make([]map[string]any, 0, len(connections))
	for _, name := range sortedMapKeys(connections) {
		connection := asMap(connections[name])
		child := semanticAssetByName(parent.Key, "connection", name, assets)
		rows = append(rows, map[string]any{
			"name":        name,
			"nameHref":    childHref(workspaceID, child),
			"kind":        emptyDash(metaString(connection, "Kind", "kind")),
			"credentials": metricGridBadgeValue(boolLabel(metaBool(connection, "credentials_configured")), "success"),
			"defaults":    compactJSON(metaValue(connection, "Defaults", "defaults", "Options", "options")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "180px"},
			{ID: "kind", Header: "Kind", Width: "120px"},
			{ID: "credentials", Header: "Credentials", Kind: "badge", Width: "120px"},
			{ID: "defaults", Header: "Defaults / options", Kind: "expression"},
		},
		Rows:     rows,
		Empty:    "No connections are defined for this semantic model.",
		MinWidth: "760px",
	}
}

func semanticSourcesGrid(workspaceID string, parent workspaceview.AssetView, assets []workspaceview.AssetView, meta map[string]any) metricGrid {
	sources := metaMap(meta, "Sources", "sources")
	rows := make([]map[string]any, 0, len(sources))
	for _, name := range sortedMapKeys(sources) {
		source := asMap(sources[name])
		child := semanticAssetByName(parent.Key, "source", name, assets)
		rows = append(rows, map[string]any{
			"name":       name,
			"nameHref":   childHref(workspaceID, child),
			"connection": emptyDash(metaString(source, "Connection", "connection")),
			"format":     metricGridBadgeValue(metaString(source, "Format", "format"), "accent"),
			"path":       emptyDash(firstNonEmpty(metaString(source, "Path", "path"), metaString(source, "Object", "object"))),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "180px"},
			{ID: "connection", Header: "Connection", Kind: "code", Width: "150px"},
			{ID: "format", Header: "Format", Kind: "badge", Width: "110px"},
			{ID: "path", Header: "Path / object", Kind: "expression"},
		},
		Rows:     rows,
		Empty:    "No sources are defined for this semantic model.",
		MinWidth: "820px",
	}
}

func semanticModelTablesGrid(workspaceID string, parent workspaceview.AssetView, assets []workspaceview.AssetView, meta map[string]any, refresh AssetRefreshState) metricGrid {
	tables := metaMap(meta, "Tables", "tables", "Models", "models")
	measureCounts := semanticMeasureCountsByTable(metaMap(meta, "Measures", "measures"))
	rows := make([]map[string]any, 0, len(tables))
	lastRefreshed := emptyDash(refresh.LatestSuccessful.FinishedAt)
	refreshStatus := "not refreshed"
	if strings.TrimSpace(refresh.LatestSuccessful.Status) != "" {
		refreshStatus = refresh.LatestSuccessful.Status
	}
	for _, name := range sortedMapKeys(tables) {
		table := asMap(tables[name])
		child := semanticAssetByName(parent.Key, "model_table", name, assets)
		rows = append(rows, map[string]any{
			"name":           name,
			"nameHref":       childHref(workspaceID, child),
			"primary_key":    emptyDash(metaString(table, "PrimaryKey", "primary_key")),
			"fields":         len(metaMap(table, "Dimensions", "dimensions", "Fields", "fields")),
			"measures":       measureCounts[name],
			"last_refreshed": lastRefreshed,
			"refresh_status": refreshStatusGridValue(refreshStatus),
			"description":    emptyDash(metaString(table, "Description", "description")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "180px"},
			{ID: "primary_key", Header: "Primary key", Kind: "code", Width: "150px"},
			{ID: "fields", Header: "Fields", Width: "100px"},
			{ID: "measures", Header: "Measures", Width: "110px"},
			{ID: "last_refreshed", Header: "Last refreshed", Width: "180px"},
			{ID: "refresh_status", Header: "Refresh status", Kind: "status", Width: "130px"},
			{ID: "description", Header: "Description"},
		},
		Rows:     rows,
		Empty:    "No model tables are defined for this semantic model.",
		MinWidth: "1120px",
	}
}

func semanticMeasureCountsByTable(measures map[string]any) map[string]int {
	counts := map[string]int{}
	defaultTable := metaString(asMap(metaValue(measures, "Defaults", "defaults")), "Table", "table")
	for _, name := range sortedMapKeys(measures) {
		if strings.EqualFold(name, "defaults") {
			continue
		}
		measure := asMap(measures[name])
		table := firstNonEmpty(metaString(measure, "Table", "table"), defaultTable)
		if table == "" {
			continue
		}
		counts[table]++
	}
	return counts
}

func modelTableDetailModel(model *assetDetailModel, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, refresh AssetRefreshState) {
	modelKey, tableName := modelTableKeyParts(asset)
	fields := modelTableFields(asset.Payload)
	sources := modelTableSourceNames(asset.Payload)
	mode := "Unspecified"
	if modelTableSQL(asset.Payload) != "" {
		mode = "Transform"
	} else if metaString(asset.Payload, "Source", "source") != "" {
		mode = "Direct source"
	}
	semanticModel := assetByTypeKey("semantic_model", modelKey, assets)
	model.Overview = append(model.Overview,
		definitionFact{Label: "Semantic model", Value: assetTitle(semanticModel)},
		definitionFact{Label: "Primary key", Value: metaString(asset.Payload, "PrimaryKey", "primary_key"), Code: true},
		definitionFact{Label: "Grain", Value: metaString(asset.Payload, "Grain", "grain"), Code: true},
		definitionFact{Label: "Fields", Value: fmt.Sprint(len(fields))},
		definitionFact{Label: "Input sources", Value: fmt.Sprint(len(sources))},
		definitionFact{Label: "Mode", Value: mode},
	)
	model.Overview = append(model.Overview, refreshOverviewFacts(refresh)...)
	model.Sections = append(model.Sections,
		assetDetailSection{Title: fmt.Sprintf("Fields (%d)", len(fields)), Signal: "assetDetailsModelTableFieldsGrid", Grid: modelTableFieldsGrid(workspace.ID, modelKey, tableName, fields, metaMap(asset.Payload, "Schema", "schema"), assets)},
	)
	if sql := modelTableSQL(asset.Payload); sql != "" {
		model.Sections = append(model.Sections, assetDetailSection{Title: "SQL", Lang: "sql", Code: sql})
	}
}

func modelTableKeyParts(asset workspaceview.AssetView) (string, string) {
	parts := strings.SplitN(asset.Key, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", asset.Key
}

func modelTableFields(meta map[string]any) map[string]any {
	return metaMap(meta, "Dimensions", "dimensions", "Fields", "fields")
}

func sourceDetailModel(model *assetDetailModel, asset workspaceview.AssetView) {
	fields := metaMap(asset.Payload, "Fields", "fields")
	schema := metaMap(asset.Payload, "Schema", "schema")
	columns := modelTableSchemaColumns(fields, schema)
	model.Overview = append(model.Overview, sourceFacts(asset)...)
	model.Overview = append(model.Overview, definitionFact{Label: "Fields", Value: fmt.Sprint(len(columns))})
	model.Sections = append(model.Sections,
		assetDetailSection{Title: fmt.Sprintf("Fields (%d)", len(columns)), Signal: "assetDetailsSourceFieldsGrid", Grid: sourceFieldsGrid(fields, schema)},
	)
}

func modelTableSourceNames(meta map[string]any) []string {
	if source := metaString(meta, "Source", "source"); source != "" {
		return []string{source}
	}
	for _, value := range []any{
		metaValue(meta, "SourceDependencies", "source_dependencies"),
		metaValue(meta, "Sources", "sources"),
	} {
		sources := stringSlice(value)
		if len(sources) > 0 {
			sort.Strings(sources)
			return sources
		}
	}
	return nil
}

func modelTableSQL(meta map[string]any) string {
	return firstNonEmpty(
		metaString(metaMap(meta, "Transform", "transform"), "SQL", "sql"),
		metaString(meta, "SQL", "sql"),
	)
}

func modelTableFieldsGrid(workspaceID, modelKey, tableName string, fields, schema map[string]any, assets []workspaceview.AssetView) metricGrid {
	schemaColumns := modelTableSchemaColumns(fields, schema)
	rows := make([]map[string]any, 0, len(schemaColumns))
	for _, column := range schemaColumns {
		name := metaString(column, "Name", "name")
		field := asMap(fields[name])
		child := assetByTypeKey("field", modelKey+"."+tableName+"."+name, assets)
		key := ""
		if metaBool(column, "PrimaryKey", "primaryKey") {
			key = "Primary key"
		}
		rows = append(rows, map[string]any{
			"name":          name,
			"nameHref":      childHref(workspaceID, child),
			"label":         firstNonEmpty(metaString(field, "Label", "label"), labelFromKey(name)),
			"physical_type": metricGridBadgeValue(metaString(column, "PhysicalType", "physicalType"), "muted"),
			"nullable":      nullableLabel(column, "Nullable", "nullable"),
			"key":           key,
			"description":   emptyDash(metaString(field, "Description", "description")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "170px"},
			{ID: "label", Header: "Label", Width: "180px"},
			{ID: "physical_type", Header: "Physical type", Kind: "badge", Width: "140px"},
			{ID: "nullable", Header: "Nullable", Width: "100px"},
			{ID: "key", Header: "Key", Width: "130px"},
			{ID: "description", Header: "Description"},
		},
		Rows:     rows,
		Empty:    "No schema is available for this model table.",
		MinWidth: "900px",
	}
}

func sourceFieldsGrid(fields, schema map[string]any) metricGrid {
	schemaColumns := modelTableSchemaColumns(fields, schema)
	rows := make([]map[string]any, 0, len(schemaColumns))
	for _, column := range schemaColumns {
		name := metaString(column, "Name", "name")
		field := asMap(fields[name])
		rows = append(rows, map[string]any{
			"name":          name,
			"description":   emptyDash(metaString(field, "Description", "description")),
			"physical_type": metricGridBadgeValue(metaString(column, "PhysicalType", "physicalType"), "muted"),
			"nullable":      nullableLabel(column, "Nullable", "nullable"),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "code", Width: "170px"},
			{ID: "description", Header: "Description"},
			{ID: "physical_type", Header: "Physical type", Kind: "badge", Width: "140px"},
			{ID: "nullable", Header: "Nullable", Width: "100px"},
		},
		Rows:     rows,
		Empty:    "No schema is available for this source.",
		MinWidth: "900px",
	}
}

func modelTableSchemaColumns(fields map[string]any, schema map[string]any) []map[string]any {
	if schema != nil {
		if raw := metaSlice(schema, "Columns", "columns"); len(raw) > 0 {
			columns := make([]map[string]any, 0, len(raw))
			for _, item := range raw {
				columns = append(columns, asMap(item))
			}
			sort.Slice(columns, func(i, j int) bool {
				return metaInt(columns[i], "Ordinal", "ordinal") < metaInt(columns[j], "Ordinal", "ordinal")
			})
			return columns
		}
	}
	columns := make([]map[string]any, 0, len(fields))
	for _, name := range sortedMapKeys(fields) {
		columns = append(columns, map[string]any{"name": name})
	}
	return columns
}

func semanticFieldsGrid(workspaceID string, parent workspaceview.AssetView, assets []workspaceview.AssetView, meta map[string]any) metricGrid {
	tables := metaMap(meta, "Tables", "tables", "Models", "models")
	rows := []map[string]any{}
	for _, tableName := range sortedMapKeys(tables) {
		table := asMap(tables[tableName])
		fields := metaMap(table, "Dimensions", "dimensions", "Fields", "fields")
		for _, fieldName := range sortedMapKeys(fields) {
			field := asMap(fields[fieldName])
			key := parent.Key + "." + tableName + "." + fieldName
			child := assetByTypeKey("field", key, assets)
			rows = append(rows, map[string]any{
				"name":       fieldName,
				"nameHref":   childHref(workspaceID, child),
				"table":      tableName,
				"expression": firstNonEmpty(metaString(field, "Expr", "expr", "Expression", "expression"), tableName+"."+fieldName),
				"type":       metricGridBadgeValue(metaString(field, "Type", "type"), "muted"),
				"filter":     emptyDash(metaString(field, "Where", "where")),
				"order":      emptyDash(metaString(field, "OrderExpr", "order_expr")),
			})
		}
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "170px"},
			{ID: "table", Header: "Model table", Kind: "code", Width: "150px"},
			{ID: "expression", Header: "Expression", Kind: "expression", Width: "260px"},
			{ID: "type", Header: "Type", Kind: "badge", Width: "110px"},
			{ID: "filter", Header: "Filter", Kind: "expression", Width: "220px"},
			{ID: "order", Header: "Order", Kind: "expression", Width: "190px"},
		},
		Rows:     rows,
		Empty:    "No fields are defined for this semantic model.",
		MinWidth: "1100px",
	}
}

func semanticMeasuresGrid(workspaceID string, parent workspaceview.AssetView, assets []workspaceview.AssetView, meta map[string]any) metricGrid {
	measures := metaMap(meta, "Measures", "measures")
	rows := make([]map[string]any, 0, len(measures))
	for _, name := range sortedMapKeys(measures) {
		measure := asMap(measures[name])
		child := childAssetByName(parent.ID, "measure", name, assets)
		rows = append(rows, map[string]any{
			"name":       name,
			"nameHref":   childHref(workspaceID, child),
			"table":      emptyDash(metaString(measure, "Table", "table")),
			"expression": firstNonEmpty(metaString(measure, "Expression", "expression"), metaString(measure, "Expr", "expr")),
			"grain":      metricGridBadgeValue(metaString(measure, "Grain", "grain"), "muted"),
			"format":     metricGridBadgeValue(metaString(measure, "Format", "format"), "accent"),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "160px"},
			{ID: "table", Header: "Table", Kind: "code", Width: "140px"},
			{ID: "expression", Header: "Expression", Kind: "expression"},
			{ID: "grain", Header: "Grain", Kind: "badge", Width: "110px"},
			{ID: "format", Header: "Format", Kind: "badge", Width: "100px"},
		},
		Rows:     rows,
		Empty:    "No measures are defined for this semantic model.",
		MinWidth: "900px",
	}
}

func semanticRelationshipsGrid(workspaceID string, parent workspaceview.AssetView, assets []workspaceview.AssetView, meta map[string]any) metricGrid {
	relationships := metaSlice(meta, "Relationships", "relationships")
	rows := make([]map[string]any, 0, len(relationships))
	for _, item := range relationships {
		relationship := asMap(item)
		id := metaString(relationship, "ID", "id")
		child := semanticAssetByName(parent.Key, "relationship", id, assets)
		fromTable, fromField := splitSemanticFieldRef(metaString(relationship, "From", "from"))
		toTable, toField := splitSemanticFieldRef(metaString(relationship, "To", "to"))
		rows = append(rows, map[string]any{
			"id":          id,
			"idHref":      childHref(workspaceID, child),
			"from_table":  emptyDash(fromTable),
			"from_field":  emptyDash(fromField),
			"to_table":    emptyDash(toTable),
			"to_field":    emptyDash(toField),
			"cardinality": metricGridBadgeValue(metaString(relationship, "Cardinality", "cardinality"), "muted"),
			"active":      metricGridBadgeValue(boolLabel(metaBool(relationship, "Active", "active")), "success"),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "id", Header: "ID", Kind: "link", HrefKey: "idHref", Width: "180px"},
			{ID: "from_table", Header: "From table", Kind: "code", Width: "140px"},
			{ID: "from_field", Header: "From field", Kind: "code", Width: "160px"},
			{ID: "to_table", Header: "To table", Kind: "code", Width: "140px"},
			{ID: "to_field", Header: "To field", Kind: "code", Width: "160px"},
			{ID: "cardinality", Header: "Cardinality", Kind: "badge", Width: "140px"},
			{ID: "active", Header: "Active", Kind: "badge", Width: "90px"},
		},
		Rows:     rows,
		Empty:    "No relationships are defined for this semantic model.",
		MinWidth: "1010px",
	}
}

func splitSemanticFieldRef(ref string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(ref), ".", 2)
	if len(parts) != 2 {
		return strings.TrimSpace(ref), ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func dashboardDetailModel(model *assetDetailModel, asset workspaceview.AssetView, assets []workspaceview.AssetView) {
	pages := childrenByType(asset.ID, "page", assets)
	filters := childrenByType(asset.ID, "filter", assets)
	visuals := childrenByType(asset.ID, "visual", assets)
	tables := childrenByType(asset.ID, "table", assets)
	model.Overview = append(model.Overview,
		definitionFact{Label: "Semantic model", Value: metaString(asset.Payload, "SemanticModel", "semantic_model")},
		definitionFact{Label: "Tags", Value: strings.Join(stringSlice(metaValue(asset.Payload, "Tags", "tags")), ", ")},
	)
	model.Sections = append(model.Sections,
		assetDetailSection{Title: fmt.Sprintf("Pages (%d)", len(pages)), Signal: "assetDetailsPagesGrid", Grid: dashboardPagesGrid(asset, pages)},
		assetDetailSection{Title: fmt.Sprintf("Filters (%d)", len(filters)), Signal: "assetDetailsFiltersGrid", Grid: dashboardFiltersGrid(asset, filters)},
		assetDetailSection{Title: fmt.Sprintf("Visuals (%d)", len(visuals)), Signal: "assetDetailsVisualsGrid", Grid: dashboardVisualsGrid(asset, visuals)},
		assetDetailSection{Title: fmt.Sprintf("Tables (%d)", len(tables)), Signal: "assetDetailsTablesGrid", Grid: dashboardTablesGrid(asset, tables)},
	)
}

func dashboardPagesGrid(parent workspaceview.AssetView, pages []workspaceview.AssetView) metricGrid {
	rows := make([]map[string]any, 0, len(pages))
	for _, page := range pages {
		key := assetChildName(parent, page)
		rows = append(rows, map[string]any{
			"page":        assetTitle(page),
			"pageHref":    assetnav.WorkspaceAssetSectionHref(parent.WorkspaceID, page.ID, "details"),
			"key":         key,
			"description": emptyDash(page.Description),
			"runtime":     "Open",
			"runtimeHref": "/dashboards/" + parent.Key + "/pages/" + key,
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "page", Header: "Page", Kind: "link", HrefKey: "pageHref", Width: "220px"},
			{ID: "key", Header: "Key", Kind: "code", Width: "190px"},
			{ID: "description", Header: "Description"},
			{ID: "runtime", Header: "Runtime", Kind: "link", HrefKey: "runtimeHref", Width: "110px"},
		},
		Rows:     rows,
		Empty:    "No pages are defined for this dashboard.",
		MinWidth: "860px",
	}
}

func dashboardFiltersGrid(parent workspaceview.AssetView, filters []workspaceview.AssetView) metricGrid {
	sortAssetChildren(parent, filters)
	rows := make([]map[string]any, 0, len(filters))
	for _, filter := range filters {
		rows = append(rows, map[string]any{
			"filter":     assetTitle(filter),
			"filterHref": assetnav.WorkspaceAssetSectionHref(parent.WorkspaceID, filter.ID, "details"),
			"key":        assetChildName(parent, filter),
			"field":      emptyDash(metaString(filter.Payload, "Dimension", "dimension", "Field", "field")),
			"type":       emptyDash(metaString(filter.Payload, "Type", "type", "Kind", "kind")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "filter", Header: "Filter", Kind: "link", HrefKey: "filterHref", Width: "190px"},
			{ID: "key", Header: "Key", Kind: "code", Width: "160px"},
			{ID: "field", Header: "Field", Kind: "code", Width: "220px"},
			{ID: "type", Header: "Type", Width: "120px"},
		},
		Rows:     rows,
		Empty:    "No filters are defined for this dashboard.",
		MinWidth: "820px",
	}
}

func dashboardVisualsGrid(parent workspaceview.AssetView, visuals []workspaceview.AssetView) metricGrid {
	sortAssetChildren(parent, visuals)
	rows := make([]map[string]any, 0, len(visuals))
	for _, visual := range visuals {
		query := metaMap(visual.Payload, "Query", "query")
		rows = append(rows, map[string]any{
			"visual":     assetTitle(visual),
			"visualHref": assetnav.WorkspaceAssetSectionHref(parent.WorkspaceID, visual.ID, "details"),
			"key":        assetChildName(parent, visual),
			"type":       emptyDash(firstNonEmpty(metaString(visual.Payload, "Shape", "shape"), metaString(visual.Payload, "Type", "type"), metaString(visual.Payload, "Kind", "kind"))),
			"measures":   emptyDash(strings.Join(stringSlice(metaValue(query, "Measures", "measures")), ", ")),
			"dimensions": emptyDash(strings.Join(stringSlice(metaValue(query, "Dimensions", "dimensions")), ", ")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "visual", Header: "Visual", Kind: "link", HrefKey: "visualHref", Width: "230px"},
			{ID: "key", Header: "Key", Kind: "code", Width: "180px"},
			{ID: "type", Header: "Type", Width: "120px"},
			{ID: "measures", Header: "Measures", Kind: "expression", Width: "220px"},
			{ID: "dimensions", Header: "Dimensions", Kind: "expression"},
		},
		Rows:     rows,
		Empty:    "No visuals are defined for this dashboard.",
		MinWidth: "1040px",
	}
}

func dashboardTablesGrid(parent workspaceview.AssetView, tables []workspaceview.AssetView) metricGrid {
	sortAssetChildren(parent, tables)
	rows := make([]map[string]any, 0, len(tables))
	for _, table := range tables {
		rows = append(rows, map[string]any{
			"table":     assetTitle(table),
			"tableHref": assetnav.WorkspaceAssetSectionHref(parent.WorkspaceID, table.ID, "details"),
			"key":       assetChildName(parent, table),
			"baseTable": emptyDash(metaString(metaMap(table.Payload, "Query", "query"), "Table", "table")),
			"rows":      emptyDash(strings.Join(stringSlice(metaValue(table.Payload, "Rows", "rows")), ", ")),
			"measures":  emptyDash(strings.Join(stringSlice(metaValue(table.Payload, "Measures", "measures")), ", ")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "table", Header: "Table", Kind: "link", HrefKey: "tableHref", Width: "220px"},
			{ID: "key", Header: "Key", Kind: "code", Width: "170px"},
			{ID: "baseTable", Header: "Base table", Kind: "code", Width: "140px"},
			{ID: "rows", Header: "Rows", Kind: "expression", Width: "280px"},
			{ID: "measures", Header: "Measures", Kind: "expression"},
		},
		Rows:     rows,
		Empty:    "No tables are defined for this dashboard.",
		MinWidth: "920px",
	}
}

func connectionDetailModel(model *assetDetailModel, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) {
	sources := sourcesUsingConnection(asset.ID, assets, edges)
	model.Overview = append(model.Overview, connectionFacts(asset)...)
	model.Overview = append(model.Overview, definitionFact{Label: "Sources", Value: fmt.Sprint(len(sources))})
	model.Sections = append(model.Sections,
		assetDetailSection{
			Title:  fmt.Sprintf("Sources (%d)", len(sources)),
			Signal: "assetDetailsConnectionSourcesGrid",
			Grid:   childAssetGrid(workspace.ID, sources, edges, "No sources use this connection."),
		},
	)
}

func sourcesUsingConnection(connectionID string, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) []workspaceview.AssetView {
	byID := assetsByID(assets)
	sources := []workspaceview.AssetView{}
	seen := map[string]struct{}{}
	for _, edge := range edges {
		if edge.Type != "uses_connection" || edge.ToAssetID != connectionID {
			continue
		}
		source, ok := byID[edge.FromAssetID]
		if !ok || source.Type != "source" {
			continue
		}
		if _, ok := seen[source.ID]; ok {
			continue
		}
		seen[source.ID] = struct{}{}
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool {
		return assetTitle(sources[i]) < assetTitle(sources[j])
	})
	return sources
}

func connectionFacts(asset workspaceview.AssetView) []definitionFact {
	return []definitionFact{
		{Label: "Kind", Value: metaString(asset.Payload, "Kind", "kind")},
		{Label: "Scope", Value: metaString(asset.Payload, "Scope", "scope")},
		{Label: "Root", Value: metaString(asset.Payload, "Root", "root")},
		{Label: "Path", Value: metaString(asset.Payload, "Path", "path")},
		{Label: "Credentials", Value: boolLabel(metaBool(asset.Payload, "credentials_configured"))},
		{Label: "Options", Value: compactJSON(metaValue(asset.Payload, "Options", "options"))},
	}
}

func sourceFacts(asset workspaceview.AssetView) []definitionFact {
	return []definitionFact{
		{Label: "Connection", Value: metaString(asset.Payload, "Connection", "connection")},
		{Label: "Format", Value: metaString(asset.Payload, "Format", "format")},
		{Label: "Path", Value: metaString(asset.Payload, "Path", "path")},
		{Label: "Object", Value: metaString(asset.Payload, "Object", "object")},
		{Label: "Options", Value: compactJSON(metaValue(asset.Payload, "Options", "options"))},
	}
}

func metricLeafFacts(asset workspaceview.AssetView) []definitionFact {
	facts := []definitionFact{}
	for _, key := range []string{"Expression", "expression", "Expr", "expr", "Where", "where", "OrderExpr", "order_expr", "Unit", "unit", "Format", "format"} {
		if value := metaString(asset.Payload, key); strings.TrimSpace(value) != "" {
			facts = append(facts, definitionFact{Label: labelFromKey(key), Value: value, Code: strings.Contains(strings.ToLower(key), "expr") || strings.EqualFold(key, "expression")})
		}
	}
	return facts
}

type definitionFact struct {
	Label string
	Value string
	Code  bool
	Wide  bool
}

func childAssetGrid(workspaceID string, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, empty string) metricGrid {
	sort.Slice(assets, func(i, j int) bool {
		return assetTitle(assets[i]) < assetTitle(assets[j])
	})
	rows := make([]map[string]any, 0, len(assets))
	for _, asset := range assets {
		rows = append(rows, map[string]any{
			"name":        assetTitle(asset),
			"nameHref":    assetnav.CanonicalAssetSectionHref(workspaceID, asset, "details", edges),
			"key":         asset.Key,
			"type":        assetTypeLabel(asset.Type),
			"description": emptyDash(asset.Description),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "220px"},
			{ID: "key", Header: "Key", Kind: "code", Width: "220px"},
			{ID: "type", Header: "Type", Width: "150px"},
			{ID: "description", Header: "Description"},
		},
		Rows:     rows,
		Empty:    empty,
		MinWidth: "860px",
	}
}

func childDependencyGrid(workspaceID, assetID string, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) metricGrid {
	byID := assetsByID(assets)
	rows := []map[string]any{}
	for _, edge := range edges {
		if edge.FromAssetID != assetID && edge.ToAssetID != assetID {
			continue
		}
		peerID := edge.ToAssetID
		direction := metricGridBadge{Label: "Outgoing", Tone: "accent"}
		if edge.ToAssetID == assetID {
			peerID = edge.FromAssetID
			direction = metricGridBadge{Label: "Incoming", Tone: "muted"}
		}
		peer, ok := byID[peerID]
		if !ok {
			continue
		}
		rows = append(rows, map[string]any{
			"direction": direction,
			"relation":  labelFromKey(edge.Type),
			"asset":     assetTitle(peer),
			"assetHref": assetnav.CanonicalAssetSectionHref(workspaceID, peer, "details", edges),
			"type":      assetTypeLabel(peer.Type),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return fmt.Sprint(rows[i]["relation"], rows[i]["asset"]) < fmt.Sprint(rows[j]["relation"], rows[j]["asset"])
	})
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "direction", Header: "Direction", Kind: "badge", Width: "120px"},
			{ID: "relation", Header: "Relationship", Width: "180px"},
			{ID: "asset", Header: "Asset", Kind: "link", HrefKey: "assetHref", Width: "240px"},
			{ID: "type", Header: "Type", Width: "140px"},
		},
		Rows:     rows,
		Empty:    "No direct dependencies for this asset.",
		MinWidth: "720px",
	}
}

func metaFacts(meta map[string]any) []definitionFact {
	keys := make([]string, 0, len(meta))
	for key := range meta {
		if assetDefinitionDetailKey(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	facts := make([]definitionFact, 0, len(keys))
	for _, key := range keys {
		facts = append(facts, definitionFact{Label: labelFromKey(key), Value: assetDefinitionValue(meta[key]), Code: looksLikeCodeKey(key)})
	}
	return facts
}

func assetDefinitionDetailKey(key string) bool {
	switch strings.ToLower(key) {
	case "description", "id", "name", "title", "auth":
		return true
	default:
		return false
	}
}

func assetDefinitionValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		if data, err := json.MarshalIndent(typed, "", "  "); err == nil {
			return string(data)
		}
		return fmt.Sprint(value)
	}
}

func looksLikeCodeKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "expr") || strings.Contains(key, "sql")
}

func asMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func metaValue(meta map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := meta[key]; ok {
			return value
		}
	}
	return nil
}

func metaMap(meta map[string]any, keys ...string) map[string]any {
	return asMap(metaValue(meta, keys...))
}

func metaSlice(meta map[string]any, keys ...string) []any {
	if typed, ok := metaValue(meta, keys...).([]any); ok {
		return typed
	}
	return nil
}

func metaString(meta map[string]any, keys ...string) string {
	value := metaValue(meta, keys...)
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%g", typed)
	default:
		return compactJSON(typed)
	}
}

func metaBool(meta map[string]any, keys ...string) bool {
	switch typed := metaValue(meta, keys...).(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || strings.EqualFold(typed, "yes")
	default:
		return false
	}
}

func metaInt(meta map[string]any, keys ...string) int {
	switch typed := metaValue(meta, keys...).(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func compactJSON(value any) string {
	if value == nil {
		return ""
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	text := string(bytes)
	if text == "null" || text == "{}" || text == "[]" {
		return ""
	}
	return text
}

func boolLabel(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func nullableLabel(meta map[string]any, keys ...string) string {
	value := metaValue(meta, keys...)
	if value == nil {
		return "-"
	}
	switch typed := value.(type) {
	case bool:
		return boolLabel(typed)
	case string:
		if strings.EqualFold(typed, "true") || strings.EqualFold(typed, "yes") {
			return "Yes"
		}
		if strings.EqualFold(typed, "false") || strings.EqualFold(typed, "no") {
			return "No"
		}
	}
	return "-"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func childAssetByName(parentID, typ, name string, assets []workspaceview.AssetView) workspaceview.AssetView {
	for _, asset := range assets {
		if asset.ParentID != parentID || asset.Type != typ {
			continue
		}
		if asset.Title == name || asset.Key == name || strings.HasSuffix(asset.Key, "."+name) {
			return asset
		}
	}
	return workspaceview.AssetView{}
}

func semanticAssetByName(modelKey, typ, name string, assets []workspaceview.AssetView) workspaceview.AssetView {
	key := modelKey + "." + name
	if asset := assetByTypeKey(typ, key, assets); asset.ID != "" {
		return asset
	}
	for _, asset := range assets {
		if asset.Type != typ {
			continue
		}
		if asset.Title == name || asset.Key == name || strings.HasSuffix(asset.Key, "."+name) {
			return asset
		}
	}
	return workspaceview.AssetView{}
}

func assetByTypeKey(typ, key string, assets []workspaceview.AssetView) workspaceview.AssetView {
	for _, asset := range assets {
		if asset.Type == typ && asset.Key == key {
			return asset
		}
	}
	return workspaceview.AssetView{}
}

func childrenByType(parentID, typ string, assets []workspaceview.AssetView) []workspaceview.AssetView {
	out := []workspaceview.AssetView{}
	for _, asset := range assets {
		if asset.ParentID == parentID && asset.Type == typ {
			out = append(out, asset)
		}
	}
	return out
}

func metricChildName(parent, child workspaceview.AssetView) string {
	return assetChildName(parent, child)
}

func assetChildName(parent, child workspaceview.AssetView) string {
	prefix := parent.Key + "."
	if strings.HasPrefix(child.Key, prefix) {
		return strings.TrimPrefix(child.Key, prefix)
	}
	if child.Key != "" {
		return child.Key
	}
	return assetTitle(child)
}

func sortAssetChildren(parent workspaceview.AssetView, children []workspaceview.AssetView) {
	sort.Slice(children, func(i, j int) bool {
		left := assetChildName(parent, children[i])
		right := assetChildName(parent, children[j])
		if left == right {
			return assetTitle(children[i]) < assetTitle(children[j])
		}
		return left < right
	})
}

func childHref(workspaceID string, asset workspaceview.AssetView) string {
	if asset.ID == "" {
		return ""
	}
	return assetnav.CanonicalAssetSectionHref(workspaceID, asset, "details", nil)
}

func assetsByID(assets []workspaceview.AssetView) map[string]workspaceview.AssetView {
	byID := map[string]workspaceview.AssetView{}
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	return byID
}

func dependentAssetNames(assetID, edgeType string, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) []string {
	byID := assetsByID(assets)
	names := []string{}
	for _, edge := range edges {
		if edge.FromAssetID != assetID || edge.Type != edgeType {
			continue
		}
		if asset, ok := byID[edge.ToAssetID]; ok {
			names = append(names, assetTitle(asset))
		}
	}
	sort.Strings(names)
	return names
}

func assetTitle(asset workspaceview.AssetView) string {
	return displayLabel(asset.Title, asset.Key)
}

func assetTypeLabel(typ string) string {
	switch typ {
	case "semantic_model":
		return "Semantic model"
	case "semantic_table":
		return "Semantic table"
	case "model_table":
		return "Model table"
	case "page_item":
		return "Page item"
	default:
		return strings.Title(strings.ReplaceAll(typ, "_", " "))
	}
}

func labelFromKey(key string) string {
	switch key {
	case "reads_source":
		return "Reads source"
	case "uses_connection":
		return "Uses connection"
	case "uses_field":
		return "Uses field"
	case "filters_field":
		return "Filters field"
	case "uses_filter":
		return "Uses filter"
	case "uses_model_table":
		return "Uses model table"
	case "uses_measure":
		return "Uses measure"
	case "uses_semantic_model":
		return "Uses semantic model"
	case "uses_semantic_table":
		return "Uses semantic table"
	case "uses_table":
		return "Uses table"
	case "uses_visual":
		return "Uses visual"
	}
	return strings.Title(strings.ReplaceAll(key, "_", " "))
}
