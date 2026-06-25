package ui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/assetnav"
	"github.com/Yacobolo/libredash/internal/dashboard"
	workspaceview "github.com/Yacobolo/libredash/internal/workspace"
	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

const (
	workspaceMainClass  = "grid min-w-0 min-h-svh content-start gap-3 bg-app px-4 py-4 max-sm:min-h-0 max-sm:p-3"
	workspacePanelClass = "min-w-0 overflow-hidden rounded-default border border-outline-muted bg-panel"
	assetRowClass       = "border-b border-outline-muted last:border-b-0 hover:bg-control-hover"
)

func WorkspacesPage(catalog dashboard.Catalog, workspaces []workspaceview.WorkspaceView, roleLabel string) g.Node {
	return workspaceDocument("LibreDash Workspaces", catalog, "workspaces", roleLabel, nil,
		h.Section(h.Class(catalogMainClass), h.Aria("label", "LibreDash workspaces"),
			workspaceHeader("", "Workspaces", "View published BI workspaces. Authoring lives in Git.", nil),
			h.Div(h.Class("grid grid-cols-catalog-grid items-start justify-start gap-4"),
				g.Map(workspaces, workspaceCard),
			),
		),
	)
}

func WorkspacePage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, assets []workspaceview.AssetView, activeType, query, roleLabel string, access WorkspaceAccessResponse, csrfToken string) g.Node {
	extraHead := []g.Node{}
	if access.CanManage {
		extraHead = append(extraHead, h.Script(h.Type("module"), h.Src(staticAsset("/static/workspace-access-control.js"))))
	}
	return workspaceDocument(workspace.Title, catalog, "workspaces", roleLabel, workspacePageSignals(access, csrfToken),
		h.Section(h.Class(workspaceMainClass), h.Aria("label", "Workspace assets"),
			workspaceHeader(
				"Workspace",
				workspace.Title,
				workspace.Description,
				workspaceAccessControl(workspace.ID, access.CanManage),
			),
			assetToolbar(workspace.ID, activeType, query, assets),
			h.Div(h.Class(workspacePanelClass),
				g.If(len(assets) == 0, h.Div(h.Class("p-3"), emptyState("No assets match this view."))),
				g.If(len(assets) > 0, assetTable(workspace.ID, assets, nil)),
			),
		),
		extraHead...,
	)
}

func ConnectionsPage(catalog dashboard.Catalog, workspaceID string, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeType, query, roleLabel string) g.Node {
	return workspaceDocument("Connections", catalog, "connections", roleLabel, nil,
		h.Section(h.Class(workspaceMainClass), h.Aria("label", "Connections and sources"),
			workspaceHeader("Data access", "Connections", "Connection-scoped data assets used by published semantic models.", nil),
			connectionToolbar(activeType, query),
			h.Div(h.Class(workspacePanelClass),
				g.If(len(assets) == 0, h.Div(h.Class("p-3"), emptyState("No connection assets match this view."))),
				g.If(len(assets) > 0, assetTable(workspaceID, assets, edges)),
			),
		),
	)
}

func workspacePageSignals(access WorkspaceAccessResponse, csrfToken string) map[string]any {
	return map[string]any{
		"workspaceAccess": WorkspaceAccessSignals(access, csrfToken),
	}
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

func workspaceAccessControl(workspaceID string, canManage bool) g.Node {
	if !canManage {
		return nil
	}
	upsert := "$workspaceAccess.status = {loading: true, error: '', message: ''}; $workspaceAccess.command = evt.detail; " + postActionWithCSRFSignal("/workspaces/"+workspaceID+"/access/upsert", "$workspaceAccess.csrfToken")
	remove := "$workspaceAccess.status = {loading: true, error: '', message: ''}; $workspaceAccess.command = evt.detail; " + postActionWithCSRFSignal("/workspaces/"+workspaceID+"/access/remove", "$workspaceAccess.csrfToken")
	return g.El("ld-workspace-access-control",
		g.Attr("data-attr:access", "$workspaceAccess"),
		g.Attr("data-attr:search", "$workspaceAccess.search"),
		g.Attr("data-on:ld-workspace-access-search__debounce.200ms", "$workspaceAccess.search = evt.detail.search"),
		g.Attr("data-on:ld-workspace-access-upsert", upsert),
		g.Attr("data-on:ld-workspace-access-remove", remove),
	)
}

func WorkspaceAssetPage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection, roleLabel string) g.Node {
	activeSection = normalizeWorkspaceAssetSection(activeSection)
	lineage := assetLineage(workspace.ID, asset, assets, edges)
	extraHead := []g.Node{
		h.Script(h.Type("module"), h.Src(staticAsset("/static/data-grid.js"))),
	}
	if activeSection == "details" && assetDetailUsesCodeBlock(asset) {
		extraHead = append(extraHead, h.Script(h.Type("module"), h.Src(staticAsset("/static/code-block.js"))))
	}
	if activeSection == "lineage" {
		extraHead = append(extraHead,
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/asset-lineage-graph.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/asset-lineage-graph.js"))),
		)
	}
	return workspaceDocument(asset.Title, catalog, "workspaces", roleLabel, workspaceAssetSignals(workspace, asset, assets, edges, lineage, activeSection),
		h.Section(h.Class(metricMainClass), h.Aria("label", "Workspace asset detail"),
			assetBreadcrumbHeader(workspace, asset),
			h.Div(h.Class(metricContentColumnClass),
				assetDetailTabs(workspace.ID, asset.ID, activeSection, lineage.Count),
				h.Div(h.Class(assetDetailBodyClass(activeSection)),
					g.If(activeSection == "details",
						assetDetailsSection(workspace, asset, assets, edges),
					),
					g.If(activeSection == "lineage", assetLineageSection(lineage)),
				),
			),
		),
		extraHead...,
	)
}

func ConnectionAssetPage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection, roleLabel string) g.Node {
	activeSection = normalizeWorkspaceAssetSection(activeSection)
	lineage := assetLineage(workspace.ID, asset, assets, edges)
	extraHead := []g.Node{
		h.Script(h.Type("module"), h.Src(staticAsset("/static/data-grid.js"))),
	}
	if activeSection == "lineage" {
		extraHead = append(extraHead,
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/asset-lineage-graph.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/asset-lineage-graph.js"))),
		)
	}
	return workspaceDocument(asset.Title, catalog, "connections", roleLabel, workspaceAssetSignals(workspace, asset, assets, edges, lineage, activeSection),
		h.Section(h.Class(metricMainClass), h.Aria("label", "Connection asset detail"),
			connectionBreadcrumbHeader(asset),
			h.Div(h.Class(metricContentColumnClass),
				connectionAssetDetailTabs(asset.ID, activeSection, lineage.Count),
				h.Div(h.Class(assetDetailBodyClass(activeSection)),
					g.If(activeSection == "details",
						assetDetailsSection(workspace, asset, assets, edges),
					),
					g.If(activeSection == "lineage", assetLineageSection(lineage)),
				),
			),
		),
		extraHead...,
	)
}

func ConnectionSourceAssetPage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, connection workspaceview.AssetView, source workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, activeSection, roleLabel string) g.Node {
	activeSection = normalizeWorkspaceAssetSection(activeSection)
	lineage := assetLineage(workspace.ID, source, assets, edges)
	extraHead := []g.Node{
		h.Script(h.Type("module"), h.Src(staticAsset("/static/data-grid.js"))),
	}
	if activeSection == "lineage" {
		extraHead = append(extraHead,
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/asset-lineage-graph.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/asset-lineage-graph.js"))),
		)
	}
	return workspaceDocument(source.Title, catalog, "connections", roleLabel, workspaceAssetSignals(workspace, source, assets, edges, lineage, activeSection),
		h.Section(h.Class(metricMainClass), h.Aria("label", "Connection source detail"),
			connectionSourceBreadcrumbHeader(connection, source),
			h.Div(h.Class(metricContentColumnClass),
				connectionSourceAssetDetailTabs(connection.ID, source.ID, activeSection, lineage.Count),
				h.Div(h.Class(assetDetailBodyClass(activeSection)),
					g.If(activeSection == "details",
						assetDetailsSection(workspace, source, assets, edges),
					),
					g.If(activeSection == "lineage", assetLineageSection(lineage)),
				),
			),
		),
		extraHead...,
	)
}

func assetBreadcrumbHeader(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView) g.Node {
	return h.Header(h.Class("grid min-w-0 grid-cols-workspace-header items-center gap-2 border-b border-outline-muted px-4 py-2.5"),
		h.Nav(h.Class("min-w-0"), h.Aria("label", "Breadcrumb"),
			h.Ol(h.Class("flex min-w-0 flex-wrap items-center gap-1.5 text-body-sm font-medium leading-snug"),
				breadcrumbLink("Workspaces", "/workspaces"),
				breadcrumbSeparator(),
				breadcrumbLink(workspace.Title, "/workspaces/"+workspace.ID),
				breadcrumbSeparator(),
				assetBreadcrumbCurrent(asset),
			),
		),
		h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"), assetActions(workspace.ID, asset)),
	)
}

func connectionBreadcrumbHeader(asset workspaceview.AssetView) g.Node {
	return h.Header(h.Class("grid min-w-0 grid-cols-workspace-header items-center gap-2 border-b border-outline-muted px-4 py-2.5"),
		h.Nav(h.Class("min-w-0"), h.Aria("label", "Breadcrumb"),
			h.Ol(h.Class("flex min-w-0 flex-wrap items-center gap-1.5 text-body-sm font-medium leading-snug"),
				breadcrumbLink("Connections", "/connections"),
				breadcrumbSeparator(),
				assetBreadcrumbCurrent(asset),
			),
		),
		h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"), connectionAssetActions()),
	)
}

func connectionSourceBreadcrumbHeader(connection, source workspaceview.AssetView) g.Node {
	return h.Header(h.Class("grid min-w-0 grid-cols-workspace-header items-center gap-2 border-b border-outline-muted px-4 py-2.5"),
		h.Nav(h.Class("min-w-0"), h.Aria("label", "Breadcrumb"),
			h.Ol(h.Class("flex min-w-0 flex-wrap items-center gap-1.5 text-body-sm font-medium leading-snug"),
				breadcrumbLink("Connections", "/connections"),
				breadcrumbSeparator(),
				breadcrumbLink(assetTitle(connection), assetnav.ConnectionAssetSectionHref(connection.ID, "details")),
				breadcrumbSeparator(),
				breadcrumbLink("Sources", "/connections?type=source"),
				breadcrumbSeparator(),
				assetBreadcrumbCurrent(source),
			),
		),
		h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"), connectionSourceAssetActions()),
	)
}

func breadcrumbLink(label, href string) g.Node {
	return h.Li(h.Class("min-w-0"),
		h.A(h.Href(href), h.Class("block min-w-0 truncate text-fg-muted no-underline hover:text-fg-default focus-visible:text-fg-default focus-visible:outline-0"),
			g.Text(label),
		),
	)
}

func breadcrumbSeparator() g.Node {
	return h.Li(h.Class("shrink-0 text-fg-muted"), h.Aria("hidden", "true"), g.Text("/"))
}

func assetBreadcrumbCurrent(asset workspaceview.AssetView) g.Node {
	return h.Li(h.Class("min-w-0"),
		h.H1(h.Class("m-0 inline-flex min-w-0 items-center gap-2 text-title-sm font-semibold leading-snug text-fg-default"),
			assetTypeInlineIcon(asset.Type),
			h.Span(h.Class("min-w-0 truncate"), g.Text(assetTitle(asset))),
		),
	)
}

func WorkspacePermissionsPage(catalog dashboard.Catalog, workspace workspaceview.WorkspaceView, bindings []workspaceview.RoleBindingView, roles []workspaceview.RoleView, csrfToken, roleLabel string) g.Node {
	return workspaceDocument("Workspace permissions", catalog, "settings", roleLabel, nil,
		h.Section(h.Class(catalogMainClass), h.Aria("label", "Workspace permissions"),
			workspaceHeader("Workspace", workspace.Title, "Assign workspace roles. BI assets remain authored in Git.", nil),
			h.Div(h.Class("grid max-w-workspace-detail grid-cols-workspace-detail gap-4 max-lg:grid-cols-1"),
				h.Section(h.Class(workspacePanelClass+" content-start p-4"),
					h.H2(h.Class("m-0 text-body-md font-semibold text-fg-default"), g.Text("Assign role")),
					h.Form(h.Method("post"), h.Action("/workspaces/"+workspace.ID+"/permissions"), h.Class("mt-4 grid gap-3"),
						g.If(csrfToken != "", h.Input(h.Type("hidden"), h.Name("gorilla.csrf.Token"), h.Value(csrfToken))),
						formInput("Email", "email", "person@example.com", ""),
						formInput("Display name", "displayName", "Optional", ""),
						h.Label(h.Class("grid gap-1 text-caption font-medium uppercase text-fg-muted"),
							g.Text("Role"),
							h.Select(h.Name("role"), h.Class("min-h-control-md rounded-small border border-outline-variant bg-control px-2 text-body-sm font-medium text-fg-default"),
								g.Map(roles, func(role workspaceview.RoleView) g.Node {
									return h.Option(h.Value(role.Name), g.Text(role.Name))
								}),
							),
						),
						h.Button(h.Type("submit"), h.Class(primaryLinkButtonClass+" justify-self-start"), lucide.UserPlus(buttonIconAttrs()...), h.Span(g.Text("Assign"))),
					),
				),
				h.Section(h.Class(workspacePanelClass+" content-start p-4"),
					h.H2(h.Class("m-0 text-body-md font-semibold text-fg-default"), g.Text("Current access")),
					g.If(len(bindings) == 0, emptyState("No role bindings yet.")),
					h.Div(h.Class("mt-3 grid"),
						g.Map(bindings, func(binding workspaceview.RoleBindingView) g.Node {
							return roleBindingRow(workspace.ID, binding, csrfToken)
						}),
					),
				),
			),
		),
	)
}

func workspaceDocument(title string, catalog dashboard.Catalog, active, roleLabel string, signals map[string]any, content g.Node, extraHead ...g.Node) g.Node {
	if signals == nil {
		signals = map[string]any{}
	}
	head := []g.Node{
		h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
		inspectorScript(),
		h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
	}
	head = append(head, extraHead...)
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
			h.Main(h.Class(appRootClass),
				ds.Signals(signals),
				h.Div(h.Class(appShellClass),
					sidebar(sidebarConfigForWorkspace(catalog, active, roleLabel)),
					content,
				),
				inspectorElement(),
			),
		},
	})
}

func workspaceCard(workspace workspaceview.WorkspaceView) g.Node {
	description := workspace.Description
	if strings.TrimSpace(description) == "" {
		description = "Published workspace assets."
	}
	return h.Article(h.Class(cardClass),
		h.Div(h.Class("min-w-0"),
			h.P(h.Class(eyebrowClass), g.Text("Workspace")),
			h.H2(h.Class(cardTitleClass), g.Text(workspace.Title)),
			h.P(h.Class(cardDescriptionClass), g.Text(description)),
		),
		h.Footer(h.Class(cardFooterClass),
			h.Span(g.Text(activeDeploymentLabel(workspace))),
			h.A(h.Class(primaryLinkButtonClass), h.Href("/workspaces/"+workspace.ID),
				lucide.ExternalLink(buttonIconAttrs()...),
				h.Span(g.Text("Open")),
			),
		),
	)
}

func activeDeploymentLabel(workspace workspaceview.WorkspaceView) string {
	if workspace.ActiveDeploymentID == "" {
		return "Local catalog"
	}
	return "Published deployment"
}

func assetToolbar(workspaceID, activeType, query string, assets []workspaceview.AssetView) g.Node {
	types := []string{"", "model_table", "semantic_model", "dashboard"}
	return h.Div(h.Class("grid min-w-0 gap-3 border-b border-outline-variant bg-app px-3 pt-3"), g.Attr("data-workspace-asset-toolbar", ""),
		h.Form(h.Method("get"), h.Action("/workspaces/"+workspaceID), h.Class("flex min-w-0 max-w-workspace-search items-center gap-2"),
			h.Input(h.Type("search"), h.Name("q"), h.Value(query), h.Placeholder("Search workspace assets..."), h.Class("min-h-control-md w-full rounded-small border border-outline-variant bg-control px-3 text-body-sm font-medium text-fg-default placeholder:text-fg-muted")),
			g.If(activeType != "", h.Input(h.Type("hidden"), h.Name("type"), h.Value(activeType))),
			h.Button(h.Type("submit"), h.Class(metricActionButtonClass), h.Title("Search"), h.Aria("label", "Search"), lucide.Search(metricActionIconAttrs()...)),
		),
		h.Nav(h.Class("flex min-w-0 flex-wrap gap-x-6"), h.Aria("label", "Asset type filters"),
			g.Map(types, func(typ string) g.Node {
				label := "All"
				if typ != "" {
					label = assetTypeLabel(typ)
				}
				return assetTabLink(workspaceID, typ, activeType, query, label)
			}),
		),
	)
}

func connectionToolbar(activeType, query string) g.Node {
	types := []string{"", "connection", "source"}
	return h.Div(h.Class("grid min-w-0 gap-3 border-b border-outline-variant bg-app px-3 pt-3"), g.Attr("data-connection-toolbar", ""),
		h.Form(h.Method("get"), h.Action("/connections"), h.Class("flex min-w-0 max-w-workspace-search items-center gap-2"),
			h.Input(h.Type("search"), h.Name("q"), h.Value(query), h.Placeholder("Search connections and sources..."), h.Class("min-h-control-md w-full rounded-small border border-outline-variant bg-control px-3 text-body-sm font-medium text-fg-default placeholder:text-fg-muted")),
			g.If(activeType != "", h.Input(h.Type("hidden"), h.Name("type"), h.Value(activeType))),
			h.Button(h.Type("submit"), h.Class(metricActionButtonClass), h.Title("Search"), h.Aria("label", "Search"), lucide.Search(metricActionIconAttrs()...)),
		),
		h.Nav(h.Class("flex min-w-0 flex-wrap gap-x-6"), h.Aria("label", "Connection asset type filters"),
			g.Map(types, func(typ string) g.Node {
				label := "All"
				if typ != "" {
					label = assetTypeLabel(typ)
				}
				return connectionTabLink(typ, activeType, query, label)
			}),
		),
	)
}

func connectionTabLink(typ, activeType, query, label string) g.Node {
	className := "relative -mb-px inline-flex min-h-control-xl items-center whitespace-nowrap border-b-2 px-1 text-body-sm font-medium no-underline transition-colors duration-micro ease-hover"
	if typ == activeType {
		className += " border-fg-accent font-semibold text-fg-default"
	} else {
		className += " border-transparent text-fg-muted hover:border-outline-muted hover:text-fg-default"
	}
	return h.A(h.Class(className), h.Href(connectionAssetListHref(typ, query)), g.If(typ == activeType, h.Aria("current", "page")), g.Text(label))
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

func hasAssetType(assets []workspaceview.AssetView, typ string) bool {
	for _, asset := range assets {
		if asset.Type == typ {
			return true
		}
	}
	return false
}

func assetTabLink(workspaceID, typ, activeType, query, label string) g.Node {
	className := "relative -mb-px inline-flex min-h-control-xl items-center whitespace-nowrap border-b-2 px-1 text-body-sm font-medium no-underline transition-colors duration-micro ease-hover"
	if typ == activeType {
		className += " border-fg-accent font-semibold text-fg-default"
	} else {
		className += " border-transparent text-fg-muted hover:border-outline-muted hover:text-fg-default"
	}
	return h.A(h.Class(className), h.Href(workspaceAssetHref(workspaceID, typ, query)), g.If(typ == activeType, h.Aria("current", "page")), g.Text(label))
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
	case "details", "lineage":
		return true
	default:
		return false
	}
}

func normalizeWorkspaceAssetSection(section string) string {
	if ValidWorkspaceAssetSection(section) {
		return section
	}
	return "details"
}

func assetTable(workspaceID string, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) g.Node {
	assetIndex := map[string]workspaceview.AssetView{}
	for _, asset := range assets {
		assetIndex[asset.ID] = asset
	}
	return h.Div(h.Class("min-w-0 overflow-x-auto"),
		h.Table(h.Class("w-full border-collapse text-left"),
			h.THead(h.Class("border-b border-outline-muted bg-panel-muted"),
				h.Tr(
					assetHeaderCell("Name", ""),
					assetHeaderCell("Type", "w-40"),
					assetHeaderCell("Key", "w-56 max-md:hidden"),
					assetHeaderCell("Parent", "w-48 max-lg:hidden"),
					assetHeaderCell("Actions", "w-24 text-right"),
				),
			),
			h.TBody(
				g.Map(assets, func(asset workspaceview.AssetView) g.Node {
					return assetRow(workspaceID, asset, assetIndex, edges)
				}),
			),
		),
	)
}

func assetHeaderCell(label, className string) g.Node {
	classes := strings.TrimSpace("px-3 py-2 text-caption font-medium uppercase text-fg-muted " + className)
	return h.Th(h.Class(classes), g.Attr("scope", "col"), g.Text(label))
}

func assetCell(className string, children ...g.Node) g.Node {
	classes := strings.TrimSpace("px-3 py-2 align-middle " + className)
	nodes := append([]g.Node{h.Class(classes)}, children...)
	return h.Td(nodes...)
}

func assetRow(workspaceID string, asset workspaceview.AssetView, assetIndex map[string]workspaceview.AssetView, edges []workspaceview.AssetEdgeView) g.Node {
	detailHref := assetnav.CanonicalAssetSectionHref(workspaceID, asset, "details", edges)
	openHref := detailHref
	if asset.Href != "" {
		openHref = asset.Href
	}
	return h.Tr(h.Class(assetRowClass),
		assetCell("min-w-0",
			h.Div(h.Class("flex min-w-0 items-center gap-3"),
				assetTypeIcon(asset.Type),
				h.Div(h.Class("min-w-0"),
					h.A(h.Class("block truncate text-body-sm font-semibold text-fg-default no-underline hover:underline"), h.Href(detailHref), g.Text(assetTitle(asset))),
					g.If(asset.Description != "", h.P(h.Class("m-0 mt-1 truncate text-caption font-normal text-fg-muted"), g.Text(asset.Description))),
				),
			),
		),
		assetCell("w-40 text-body-sm font-medium text-fg-muted", h.Span(g.Text(assetTypeLabel(asset.Type)))),
		assetCell("w-56 max-md:hidden", h.Code(h.Class("block truncate text-caption font-medium text-fg-muted"), g.Text(asset.Key))),
		assetCell("w-48 max-lg:hidden", assetParentTableLink(workspaceID, asset, assetIndex, edges)),
		assetCell("w-24",
			h.Div(h.Class("inline-flex w-full justify-end gap-2"),
				h.A(h.Class(metricActionButtonClass), h.Href(detailHref), h.Title("View details"), h.Aria("label", "View details"), lucide.FileText(metricActionIconAttrs()...)),
				h.A(h.Class(metricActionButtonClass), h.Href(openHref), h.Title("Open asset"), h.Aria("label", "Open asset"), lucide.ExternalLink(metricActionIconAttrs()...)),
			),
		),
	)
}

func assetParentTableLink(workspaceID string, asset workspaceview.AssetView, assetIndex map[string]workspaceview.AssetView, edges []workspaceview.AssetEdgeView) g.Node {
	if asset.Type == "source" {
		if connection, ok := assetIndex[assetnav.SourceConnectionID(asset.ID, edges)]; ok && connection.Type == "connection" {
			return h.A(
				h.Class("block truncate text-body-sm font-medium text-fg-accent no-underline hover:underline"),
				h.Href(assetnav.ConnectionAssetSectionHref(connection.ID, "details")),
				g.Text(assetTitle(connection)),
			)
		}
	}
	parent, ok := assetIndex[asset.ParentID]
	if !ok {
		return h.Span(h.Class("text-caption font-medium text-fg-muted"), g.Text(emptyDash("")))
	}
	return h.A(
		h.Class("block truncate text-body-sm font-medium text-fg-accent no-underline hover:underline"),
		h.Href(assetnav.WorkspaceAssetSectionHref(workspaceID, parent.ID, "details")),
		g.Text(assetTitle(parent)),
	)
}

func assetActions(workspaceID string, asset workspaceview.AssetView) g.Node {
	return h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"),
		h.A(h.Class(metricActionButtonClass), h.Href("/workspaces/"+workspaceID), h.Title("Back to workspace"), h.Aria("label", "Back to workspace"), lucide.ArrowLeft(metricActionIconAttrs()...)),
		g.If(asset.Href != "", h.A(h.Class(metricActionButtonClass), h.Href(asset.Href), h.Title("Open asset"), h.Aria("label", "Open asset"), lucide.ExternalLink(metricActionIconAttrs()...))),
	)
}

func connectionAssetActions() g.Node {
	return h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"),
		h.A(h.Class(metricActionButtonClass), h.Href("/connections"), h.Title("Back to connections"), h.Aria("label", "Back to connections"), lucide.ArrowLeft(metricActionIconAttrs()...)),
	)
}

func connectionSourceAssetActions() g.Node {
	return h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"),
		h.A(h.Class(metricActionButtonClass), h.Href("/connections?type=source"), h.Title("Back to sources"), h.Aria("label", "Back to sources"), lucide.ArrowLeft(metricActionIconAttrs()...)),
	)
}

func assetDetailTabs(workspaceID, assetID, activeSection string, relatedCount int) g.Node {
	return h.Nav(h.Class("flex min-w-0 gap-6 border-b border-outline-variant bg-app px-3"), h.Aria("label", "Workspace asset sections"),
		assetDetailTabLink(assetnav.WorkspaceAssetSectionHref(workspaceID, assetID, "details"), activeSection == "details", "Details", nil),
		assetDetailTabLink(assetnav.WorkspaceAssetSectionHref(workspaceID, assetID, "lineage"), activeSection == "lineage", "Lineage", metricTabCount(relatedCount)),
	)
}

func connectionAssetDetailTabs(assetID, activeSection string, relatedCount int) g.Node {
	return h.Nav(h.Class("flex min-w-0 gap-6 border-b border-outline-variant bg-app px-3"), h.Aria("label", "Connection asset sections"),
		assetDetailTabLink(assetnav.ConnectionAssetSectionHref(assetID, "details"), activeSection == "details", "Details", nil),
		assetDetailTabLink(assetnav.ConnectionAssetSectionHref(assetID, "lineage"), activeSection == "lineage", "Lineage", metricTabCount(relatedCount)),
	)
}

func connectionSourceAssetDetailTabs(connectionID, sourceID, activeSection string, relatedCount int) g.Node {
	return h.Nav(h.Class("flex min-w-0 gap-6 border-b border-outline-variant bg-app px-3"), h.Aria("label", "Connection source sections"),
		assetDetailTabLink(assetnav.ConnectionSourceAssetSectionHref(connectionID, sourceID, "details"), activeSection == "details", "Details", nil),
		assetDetailTabLink(assetnav.ConnectionSourceAssetSectionHref(connectionID, sourceID, "lineage"), activeSection == "lineage", "Lineage", metricTabCount(relatedCount)),
	)
}

func assetDetailBodyClass(activeSection string) string {
	if activeSection == "lineage" {
		return "min-h-0 overflow-auto"
	}
	return "min-h-0 overflow-auto px-4 py-4"
}

func assetDetailTabLink(href string, active bool, label string, meta g.Node) g.Node {
	className := "relative -mb-px inline-flex min-h-control-xl items-center gap-2 whitespace-nowrap border-b-2 px-1 text-body-sm font-medium no-underline transition-colors duration-micro ease-hover"
	if active {
		className += " border-fg-accent font-semibold text-fg-default"
	} else {
		className += " border-transparent text-fg-muted hover:border-outline-muted hover:text-fg-default"
	}
	return h.A(h.Class(className), h.Href(href), g.If(active, h.Aria("current", "page")), h.Span(g.Text(label)), meta)
}

type assetLineageModel struct {
	Count  int
	Graph  assetLineageGraph
	Uses   metricGrid
	UsedBy metricGrid
}

type assetLineageGraph struct {
	Nodes []assetLineageNode `json:"nodes"`
	Edges []assetLineageEdge `json:"edges"`
}

type assetLineageNode struct {
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

type assetLineageEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Kind   string `json:"kind"`
}

func assetLineageSection(lineage assetLineageModel) g.Node {
	return h.Section(h.ID("lineage"), h.Class("grid content-start"), h.Aria("label", "Asset lineage"),
		g.El("ld-asset-lineage-graph", h.Class("block h-min-model-graph min-h-0 border-b border-outline-muted bg-panel"), g.Attr("data-attr:graph", "$assetLineageGraph")),
		h.Div(h.Class("grid content-start gap-5 px-4 py-4"),
			definitionSignalGrid("Uses", "assetLineageUsesGrid"),
			definitionSignalGrid("Used by", "assetLineageUsedByGrid"),
		),
	)
}

func workspaceAssetSignals(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView, lineage assetLineageModel, activeSection string) map[string]any {
	signals := map[string]any{}
	if activeSection == "details" {
		for key, grid := range workspaceAssetDetailGridSignals(workspace, asset, assets, edges) {
			signals[key] = grid
		}
	}
	if activeSection == "lineage" {
		signals["assetLineageGraph"] = lineage.Graph
		signals["assetLineageUsesGrid"] = lineage.Uses
		signals["assetLineageUsedByGrid"] = lineage.UsedBy
	}
	return signals
}

func workspaceAssetDetailGridSignals(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) map[string]metricGrid {
	signals := map[string]metricGrid{}
	for _, section := range assetDetailModelForAsset(workspace, asset, assets, edges).Sections {
		if section.Signal == "" {
			continue
		}
		signals[section.Signal] = section.Grid
	}
	return signals
}

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

func assetDetailsSection(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) g.Node {
	model := assetDetailModelForAsset(workspace, asset, assets, edges)
	return h.Section(h.ID("details"), h.Class("grid content-start gap-6"), h.Aria("label", "Asset details"),
		definitionStats("Overview", model.Overview),
		g.Map(model.Sections, assetDetailSectionNode),
	)
}

func assetDetailSectionNode(section assetDetailSection) g.Node {
	if section.Code != "" {
		return definitionCodeBlock(section.Title, section.Lang, section.Code)
	}
	if section.Signal != "" {
		return definitionSignalGrid(section.Title, section.Signal)
	}
	return definitionFacts(section.Title, section.Facts)
}

func assetDetailUsesCodeBlock(asset workspaceview.AssetView) bool {
	return asset.Type == "model_table" && modelTableSQL(asset.Meta) != ""
}

func assetDetailModelForAsset(workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView, edges []workspaceview.AssetEdgeView) assetDetailModel {
	model := assetDetailModel{
		Overview: commonAssetOverviewFacts(asset, assets, shouldShowParentFact(asset.Type)),
	}
	switch asset.Type {
	case "semantic_model":
		semanticModelDetailModel(&model, workspace, asset, assets)
	case "model_table":
		modelTableDetailModel(&model, workspace, asset, assets)
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
		model.Overview = append(model.Overview, metaFacts(asset.Meta)...)
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

func semanticModelDetailModel(model *assetDetailModel, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView) {
	meta := asset.Meta
	modelTableMeta := metaMap(meta, "Tables", "tables", "Models", "models")
	modelTables := sortedMapKeys(modelTableMeta)
	measures := sortedMapKeys(metaMap(meta, "Measures", "measures"))
	relationships := metaSlice(meta, "Relationships", "relationships")

	model.Overview = append(model.Overview,
		definitionFact{Label: "Model tables", Value: fmt.Sprint(len(modelTables))},
		definitionFact{Label: "Measures", Value: fmt.Sprint(len(measures))},
		definitionFact{Label: "Relationships", Value: fmt.Sprint(len(relationships))},
	)
	model.Sections = append(model.Sections,
		assetDetailSection{Title: fmt.Sprintf("Model tables (%d)", len(modelTables)), Signal: "assetDetailsSemanticModelTablesGrid", Grid: semanticModelTablesGrid(workspace.ID, asset, assets, meta)},
		assetDetailSection{Title: fmt.Sprintf("Measures (%d)", len(measures)), Signal: "assetDetailsSemanticMeasuresGrid", Grid: semanticMeasuresGrid(workspace.ID, asset, assets, meta)},
		assetDetailSection{Title: fmt.Sprintf("Relationships (%d)", len(relationships)), Signal: "assetDetailsSemanticRelationshipsGrid", Grid: semanticRelationshipsGrid(workspace.ID, asset, assets, meta)},
	)
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

func semanticModelTablesGrid(workspaceID string, parent workspaceview.AssetView, assets []workspaceview.AssetView, meta map[string]any) metricGrid {
	tables := metaMap(meta, "Tables", "tables", "Models", "models")
	measureCounts := semanticMeasureCountsByTable(metaMap(meta, "Measures", "measures"))
	rows := make([]map[string]any, 0, len(tables))
	for _, name := range sortedMapKeys(tables) {
		table := asMap(tables[name])
		child := semanticAssetByName(parent.Key, "model_table", name, assets)
		rows = append(rows, map[string]any{
			"name":        name,
			"nameHref":    childHref(workspaceID, child),
			"primary_key": emptyDash(metaString(table, "PrimaryKey", "primary_key")),
			"fields":      len(metaMap(table, "Dimensions", "dimensions", "Fields", "fields")),
			"measures":    measureCounts[name],
			"description": emptyDash(metaString(table, "Description", "description")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "180px"},
			{ID: "primary_key", Header: "Primary key", Kind: "code", Width: "150px"},
			{ID: "fields", Header: "Fields", Width: "100px"},
			{ID: "measures", Header: "Measures", Width: "110px"},
			{ID: "description", Header: "Description"},
		},
		Rows:     rows,
		Empty:    "No model tables are defined for this semantic model.",
		MinWidth: "860px",
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

func modelTableDetailModel(model *assetDetailModel, workspace workspaceview.WorkspaceView, asset workspaceview.AssetView, assets []workspaceview.AssetView) {
	modelKey, tableName := modelTableKeyParts(asset)
	fields := modelTableFields(asset.Meta)
	sources := modelTableSourceNames(asset.Meta)
	mode := "Unspecified"
	if modelTableSQL(asset.Meta) != "" {
		mode = "Transform"
	} else if metaString(asset.Meta, "Source", "source") != "" {
		mode = "Direct source"
	}
	semanticModel := assetByTypeKey("semantic_model", modelKey, assets)
	model.Overview = append(model.Overview,
		definitionFact{Label: "Semantic model", Value: assetTitle(semanticModel)},
		definitionFact{Label: "Primary key", Value: metaString(asset.Meta, "PrimaryKey", "primary_key"), Code: true},
		definitionFact{Label: "Grain", Value: metaString(asset.Meta, "Grain", "grain"), Code: true},
		definitionFact{Label: "Fields", Value: fmt.Sprint(len(fields))},
		definitionFact{Label: "Input sources", Value: fmt.Sprint(len(sources))},
		definitionFact{Label: "Mode", Value: mode},
	)
	model.Sections = append(model.Sections,
		assetDetailSection{Title: fmt.Sprintf("Fields (%d)", len(fields)), Signal: "assetDetailsModelTableFieldsGrid", Grid: modelTableFieldsGrid(workspace.ID, modelKey, tableName, fields, metaMap(asset.Meta, "Schema", "schema"), assets)},
	)
	if sql := modelTableSQL(asset.Meta); sql != "" {
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
	fields := metaMap(asset.Meta, "Fields", "fields")
	schema := metaMap(asset.Meta, "Schema", "schema")
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
		definitionFact{Label: "Semantic model", Value: metaString(asset.Meta, "SemanticModel", "semantic_model")},
		definitionFact{Label: "Tags", Value: strings.Join(stringSlice(metaValue(asset.Meta, "Tags", "tags")), ", ")},
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
			"field":      emptyDash(metaString(filter.Meta, "Dimension", "dimension", "Field", "field")),
			"type":       emptyDash(metaString(filter.Meta, "Type", "type", "Kind", "kind")),
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
		query := metaMap(visual.Meta, "Query", "query")
		rows = append(rows, map[string]any{
			"visual":     assetTitle(visual),
			"visualHref": assetnav.WorkspaceAssetSectionHref(parent.WorkspaceID, visual.ID, "details"),
			"key":        assetChildName(parent, visual),
			"type":       emptyDash(firstNonEmpty(metaString(visual.Meta, "Shape", "shape"), metaString(visual.Meta, "Type", "type"), metaString(visual.Meta, "Kind", "kind"))),
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
			"baseTable": emptyDash(metaString(metaMap(table.Meta, "Query", "query"), "Table", "table")),
			"rows":      emptyDash(strings.Join(stringSlice(metaValue(table.Meta, "Rows", "rows")), ", ")),
			"measures":  emptyDash(strings.Join(stringSlice(metaValue(table.Meta, "Measures", "measures")), ", ")),
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
		{Label: "Kind", Value: metaString(asset.Meta, "Kind", "kind")},
		{Label: "Scope", Value: metaString(asset.Meta, "Scope", "scope")},
		{Label: "Root", Value: metaString(asset.Meta, "Root", "root")},
		{Label: "Path", Value: metaString(asset.Meta, "Path", "path")},
		{Label: "Credentials", Value: boolLabel(metaBool(asset.Meta, "credentials_configured"))},
		{Label: "Options", Value: compactJSON(metaValue(asset.Meta, "Options", "options"))},
	}
}

func sourceFacts(asset workspaceview.AssetView) []definitionFact {
	return []definitionFact{
		{Label: "Connection", Value: metaString(asset.Meta, "Connection", "connection")},
		{Label: "Format", Value: metaString(asset.Meta, "Format", "format")},
		{Label: "Path", Value: metaString(asset.Meta, "Path", "path")},
		{Label: "Object", Value: metaString(asset.Meta, "Object", "object")},
		{Label: "Options", Value: compactJSON(metaValue(asset.Meta, "Options", "options"))},
	}
}

func metricLeafFacts(asset workspaceview.AssetView) []definitionFact {
	facts := []definitionFact{}
	for _, key := range []string{"Expression", "expression", "Expr", "expr", "Where", "where", "OrderExpr", "order_expr", "Unit", "unit", "Format", "format"} {
		if value := metaString(asset.Meta, key); strings.TrimSpace(value) != "" {
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

func definitionFacts(title string, facts []definitionFact) g.Node {
	filtered := make([]definitionFact, 0, len(facts))
	for _, fact := range facts {
		if strings.TrimSpace(fact.Value) == "" {
			continue
		}
		filtered = append(filtered, fact)
	}
	return h.Section(h.Class("grid min-w-0 content-start gap-3 border-b border-outline-muted pb-5 last:border-b-0"), h.Aria("label", title),
		h.H2(h.Class("m-0 text-body-sm font-semibold text-fg-default"), g.Text(title)),
		g.If(len(filtered) == 0, emptyState("No details are available.")),
		g.If(len(filtered) > 0, h.Div(h.Class("grid min-w-0 grid-cols-[repeat(auto-fit,minmax(10rem,1fr))] gap-x-5 gap-y-3"),
			g.Map(filtered, func(fact definitionFact) g.Node {
				return h.Div(h.Class("grid min-w-0 gap-1"),
					h.Span(h.Class("text-caption font-medium uppercase leading-none text-fg-muted"), g.Text(fact.Label)),
					g.If(fact.Code, h.Code(h.Class("min-w-0 truncate font-mono text-body-sm font-medium text-fg-default"), g.Text(fact.Value))),
					g.If(!fact.Code, h.Span(h.Class("min-w-0 truncate text-body-sm font-medium text-fg-default"), g.Text(fact.Value))),
				)
			}),
		)),
	)
}

func definitionCodeBlock(title, lang, code string) g.Node {
	return h.Section(h.Class("grid min-w-0 content-start gap-3 border-b border-outline-muted pb-5 last:border-b-0"), h.Aria("label", title),
		h.H2(h.Class("m-0 text-body-sm font-semibold text-fg-default"), g.Text(title)),
		g.El("ld-code-block",
			g.Attr("language", firstNonEmpty(lang, "text")),
			g.Attr("code", code),
		),
	)
}

func definitionStats(title string, facts []definitionFact) g.Node {
	filtered := make([]definitionFact, 0, len(facts))
	for _, fact := range facts {
		if strings.TrimSpace(fact.Value) == "" {
			continue
		}
		filtered = append(filtered, fact)
	}
	return h.Section(h.Class("grid min-w-0 content-start gap-3 border-b border-outline-muted pb-5 last:border-b-0"), h.Aria("label", title),
		h.H2(h.Class("m-0 text-body-sm font-semibold text-fg-default"), g.Text(title)),
		g.If(len(filtered) == 0, emptyState("No details are available.")),
		g.If(len(filtered) > 0, h.Div(h.Class("grid min-w-0 grid-cols-[repeat(auto-fit,minmax(8rem,1fr))] gap-x-6 gap-y-4"),
			g.Map(filtered, func(fact definitionFact) g.Node {
				return h.Div(h.Class(definitionStatItemClass(fact)),
					h.Span(h.Class("text-caption font-medium uppercase leading-none text-fg-muted"), g.Text(fact.Label)),
					g.If(fact.Code, h.Code(h.Class(definitionStatValueClass(fact, true)), g.Text(fact.Value))),
					g.If(!fact.Code, h.Span(h.Class(definitionStatValueClass(fact, false)), g.Text(fact.Value))),
				)
			}),
		)),
	)
}

func definitionStatItemClass(fact definitionFact) string {
	if fact.Wide {
		return "grid min-w-0 content-start gap-1 sm:col-span-2 xl:col-span-3"
	}
	return "grid min-w-0 content-start gap-1"
}

func definitionStatValueClass(fact definitionFact, code bool) string {
	if fact.Wide {
		if code {
			return "min-w-0 whitespace-pre-wrap font-mono text-body-sm font-normal leading-snug text-fg-default"
		}
		return "min-w-0 text-body-sm font-normal leading-snug text-fg-default"
	}
	if code {
		return "min-w-0 truncate font-mono text-body-sm font-medium leading-normal text-fg-default"
	}
	return "min-w-0 truncate text-body-sm font-medium leading-normal text-fg-default"
}

func definitionSignalGrid(title, signal string) g.Node {
	return h.Section(h.Class("grid min-w-0 content-start gap-3 border-b border-outline-muted pb-5 last:border-b-0"), h.Aria("label", title),
		h.H2(h.Class("m-0 text-body-sm font-semibold text-fg-default"), g.Text(title)),
		g.El("ld-data-grid", g.Attr("data-attr:grid", "$"+signal)),
	)
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

func roleBindingRow(workspaceID string, binding workspaceview.RoleBindingView, csrfToken string) g.Node {
	return h.Div(h.Class("grid grid-cols-role-row items-center gap-3 border-b border-outline-muted py-2 last:border-b-0"),
		h.Div(h.Class("min-w-0"),
			h.P(h.Class("m-0 truncate text-body-sm font-semibold text-fg-default"), g.Text(displayLabel(binding.DisplayName, binding.Email))),
			h.P(h.Class("m-0 mt-0.5 truncate text-caption font-normal text-fg-muted"), g.Text(binding.Email)),
		),
		h.Span(h.Class(tagClass), g.Text(binding.Role)),
		h.Form(h.Method("post"), h.Action("/workspaces/"+workspaceID+"/permissions/remove"), h.Class("justify-self-end"),
			g.If(csrfToken != "", h.Input(h.Type("hidden"), h.Name("gorilla.csrf.Token"), h.Value(csrfToken))),
			h.Input(h.Type("hidden"), h.Name("principalId"), h.Value(binding.PrincipalID)),
			h.Button(h.Type("submit"), h.Class(metricActionButtonClass), h.Title("Remove access"), h.Aria("label", "Remove access"), lucide.Trash2(metricActionIconAttrs()...)),
		),
	)
}

func formInput(label, name, placeholder, value string) g.Node {
	return h.Label(h.Class("grid gap-1 text-caption font-medium uppercase text-fg-muted"),
		g.Text(label),
		h.Input(h.Type("text"), h.Name(name), h.Value(value), h.Placeholder(placeholder), h.Class("min-h-control-md rounded-small border border-outline-variant bg-control px-2 text-body-sm font-medium text-fg-default placeholder:text-fg-muted")),
	)
}

func emptyState(message string) g.Node {
	return h.Div(h.Class("rounded-small border border-dashed border-outline-muted bg-panel-muted px-3 py-4 text-body-sm font-medium text-fg-muted"), g.Text(message))
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
