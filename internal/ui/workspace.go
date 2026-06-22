package ui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

const (
	workspaceMainClass  = "grid min-w-0 min-h-svh content-start gap-3 bg-app px-4 py-4 max-sm:min-h-0 max-sm:p-3"
	workspacePanelClass = "grid min-w-0 rounded-default border border-outline-muted bg-panel"
	assetRowClass       = "grid min-w-0 grid-cols-asset-row items-center gap-3 border-b border-outline-muted px-3 py-2 last:border-b-0 hover:bg-control-hover"
)

func WorkspacesPage(catalog dashboard.Catalog, workspaces []api.WorkspaceResponse, roleLabel string) g.Node {
	return workspaceDocument("LibreDash Workspaces", catalog, "workspaces", roleLabel, nil,
		h.Section(h.Class(catalogMainClass), h.Aria("label", "LibreDash workspaces"),
			workspaceHeader("", "Workspaces", "View published BI workspaces. Authoring lives in Git.", nil),
			h.Div(h.Class("grid grid-cols-catalog-grid items-start justify-start gap-4"),
				g.Map(workspaces, workspaceCard),
			),
		),
	)
}

func WorkspacePage(catalog dashboard.Catalog, workspace api.WorkspaceResponse, assets []api.AssetResponse, activeType, query, roleLabel string) g.Node {
	return workspaceDocument(workspace.Title, catalog, "workspaces", roleLabel, nil,
		h.Section(h.Class(workspaceMainClass), h.Aria("label", "Workspace assets"),
			workspaceHeader(
				"Workspace",
				workspace.Title,
				workspace.Description,
				h.A(h.Class(metricActionButtonClass), h.Href("/workspaces/"+workspace.ID+"/permissions"), h.Title("Workspace permissions"), h.Aria("label", "Workspace permissions"), lucide.Shield(metricActionIconAttrs()...)),
			),
			assetToolbar(workspace.ID, activeType, query),
			h.Div(h.Class(workspacePanelClass),
				g.If(len(assets) == 0, emptyState("No assets match this view.")),
				g.Map(assets, func(asset api.AssetResponse) g.Node {
					return assetRow(workspace.ID, asset)
				}),
			),
		),
	)
}

func WorkspaceAssetPage(catalog dashboard.Catalog, workspace api.WorkspaceResponse, asset api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse, activeSection, roleLabel string) g.Node {
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

func assetBreadcrumbHeader(workspace api.WorkspaceResponse, asset api.AssetResponse) g.Node {
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

func assetBreadcrumbCurrent(asset api.AssetResponse) g.Node {
	icon := assetIconByType[asset.Type]
	if icon == nil {
		icon = lucide.Component
	}
	return h.Li(h.Class("min-w-0"),
		h.H1(h.Class("m-0 inline-flex min-w-0 items-center gap-2 text-title-sm font-semibold leading-snug text-fg-default"),
			h.Span(h.Class("inline-flex size-5 shrink-0 items-center justify-center text-icon-muted"), h.Aria("hidden", "true"),
				icon(assetIconAttrs()...),
			),
			h.Span(h.Class("min-w-0 truncate"), g.Text(assetTitle(asset))),
		),
	)
}

func WorkspacePermissionsPage(catalog dashboard.Catalog, workspace api.WorkspaceResponse, bindings []api.RoleBindingResponse, roles []api.RoleResponse, csrfToken, roleLabel string) g.Node {
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
								g.Map(roles, func(role api.RoleResponse) g.Node {
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
						g.Map(bindings, func(binding api.RoleBindingResponse) g.Node {
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

func workspaceCard(workspace api.WorkspaceResponse) g.Node {
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

func activeDeploymentLabel(workspace api.WorkspaceResponse) string {
	if workspace.ActiveDeploymentID == "" {
		return "Local catalog"
	}
	return "Published deployment"
}

func assetToolbar(workspaceID, activeType, query string) g.Node {
	types := []string{"", "dashboard", "semantic_model", "metric_view"}
	if activeType == "connection" {
		types = append(types, "connection")
	}
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

func assetRow(workspaceID string, asset api.AssetResponse) g.Node {
	detailHref := workspaceAssetSectionHref(workspaceID, asset.ID, "details")
	openHref := detailHref
	if asset.Href != "" {
		openHref = asset.Href
	}
	return h.Article(h.Class(assetRowClass),
		assetTypeIcon(asset.Type),
		h.Div(h.Class("min-w-0"),
			h.P(h.Class("m-0 text-caption font-medium uppercase text-fg-muted"), g.Text(assetTypeLabel(asset.Type))),
			h.A(h.Class("mt-0.5 block truncate text-body-sm font-semibold text-fg-default no-underline hover:underline"), h.Href(detailHref), g.Text(assetTitle(asset))),
			g.If(asset.Description != "", h.P(h.Class("m-0 mt-1 truncate text-caption font-normal text-fg-muted"), g.Text(asset.Description))),
		),
		h.Code(h.Class("truncate text-caption font-medium text-fg-muted"), g.Text(asset.Key)),
		h.Div(h.Class("inline-flex justify-end gap-2"),
			h.A(h.Class(metricActionButtonClass), h.Href(detailHref), h.Title("View details"), h.Aria("label", "View details"), lucide.FileText(metricActionIconAttrs()...)),
			h.A(h.Class(metricActionButtonClass), h.Href(openHref), h.Title("Open asset"), h.Aria("label", "Open asset"), lucide.ExternalLink(metricActionIconAttrs()...)),
		),
	)
}

func assetActions(workspaceID string, asset api.AssetResponse) g.Node {
	return h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"),
		h.A(h.Class(metricActionButtonClass), h.Href("/workspaces/"+workspaceID), h.Title("Back to workspace"), h.Aria("label", "Back to workspace"), lucide.ArrowLeft(metricActionIconAttrs()...)),
		g.If(asset.Href != "", h.A(h.Class(metricActionButtonClass), h.Href(asset.Href), h.Title("Open asset"), h.Aria("label", "Open asset"), lucide.ExternalLink(metricActionIconAttrs()...))),
	)
}

func assetDetailTabs(workspaceID, assetID, activeSection string, relatedCount int) g.Node {
	return h.Nav(h.Class("flex min-w-0 gap-6 border-b border-outline-variant bg-app px-3"), h.Aria("label", "Workspace asset sections"),
		assetDetailTabLink(workspaceAssetSectionHref(workspaceID, assetID, "details"), activeSection == "details", "Details", nil),
		assetDetailTabLink(workspaceAssetSectionHref(workspaceID, assetID, "lineage"), activeSection == "lineage", "Lineage", metricTabCount(relatedCount)),
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
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Meta     string `json:"meta,omitempty"`
	Href     string `json:"href,omitempty"`
	Side     string `json:"side"`
	Selected bool   `json:"selected,omitempty"`
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

func workspaceAssetSignals(workspace api.WorkspaceResponse, asset api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse, lineage assetLineageModel, activeSection string) map[string]any {
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

func workspaceAssetDetailGridSignals(workspace api.WorkspaceResponse, asset api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse) map[string]metricGrid {
	signals := map[string]metricGrid{}
	for _, section := range assetDetailModelForAsset(workspace, asset, assets, edges).Sections {
		if section.Signal == "" {
			continue
		}
		signals[section.Signal] = section.Grid
	}
	return signals
}

func assetLineage(workspaceID string, selected api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse) assetLineageModel {
	if selected.Type == "dashboard" {
		return dashboardAssetLineage(workspaceID, selected, assets, edges)
	}
	byID := map[string]api.AssetResponse{}
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	graph := assetLineageGraph{
		Nodes: []assetLineageNode{lineageNode(workspaceID, selected, "selected", true)},
	}
	usesRows := []map[string]any{}
	usedByRows := []map[string]any{}
	seenNodes := map[string]struct{}{selected.ID: {}}
	relations := make([]api.AssetEdgeResponse, 0)
	for _, edge := range edges {
		if edge.FromAssetID == selected.ID || edge.ToAssetID == selected.ID {
			relations = append(relations, edge)
		}
	}
	sort.Slice(relations, func(i, j int) bool {
		left := relationSortKey(selected.ID, relations[i], byID)
		right := relationSortKey(selected.ID, relations[j], byID)
		return left < right
	})
	for _, edge := range relations {
		from, fromOK := byID[edge.FromAssetID]
		to, toOK := byID[edge.ToAssetID]
		if !fromOK || !toOK {
			continue
		}
		if _, ok := seenNodes[from.ID]; !ok {
			graph.Nodes = append(graph.Nodes, lineageNode(workspaceID, from, "upstream", false))
			seenNodes[from.ID] = struct{}{}
		}
		if _, ok := seenNodes[to.ID]; !ok {
			graph.Nodes = append(graph.Nodes, lineageNode(workspaceID, to, "downstream", false))
			seenNodes[to.ID] = struct{}{}
		}
		graph.Edges = append(graph.Edges, assetLineageEdge{
			ID:     edge.ID,
			Source: edge.FromAssetID,
			Target: edge.ToAssetID,
			Label:  labelFromKey(edge.Type),
			Kind:   edge.Type,
		})
		if edge.ToAssetID == selected.ID {
			usesRows = append(usesRows, lineageTableRow(workspaceID, edge, from))
		} else {
			usedByRows = append(usedByRows, lineageTableRow(workspaceID, edge, to))
		}
	}
	sortLineageRows(usesRows)
	sortLineageRows(usedByRows)
	return assetLineageModel{
		Count:  len(usesRows) + len(usedByRows),
		Graph:  graph,
		Uses:   lineageTable(usesRows, "This asset does not reference other assets."),
		UsedBy: lineageTable(usedByRows, "No assets reference this asset."),
	}
}

type dashboardLineageRow struct {
	Depth int
	Row   map[string]any
}

func dashboardAssetLineage(workspaceID string, selected api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse) assetLineageModel {
	byID := assetsByID(assets)
	outgoing := edgesByFromAsset(edges)
	incoming := edgesByToAsset(edges)
	graph := assetLineageGraph{
		Nodes: []assetLineageNode{lineageNode(workspaceID, selected, "selected", true)},
	}
	seenNodes := map[string]struct{}{selected.ID: {}}
	seenEdges := map[string]struct{}{}
	seenRows := map[string]struct{}{}
	rows := []dashboardLineageRow{}

	addNode := func(asset api.AssetResponse) {
		if asset.ID == "" {
			return
		}
		if _, ok := seenNodes[asset.ID]; ok {
			return
		}
		graph.Nodes = append(graph.Nodes, lineageNode(workspaceID, asset, "upstream", false))
		seenNodes[asset.ID] = struct{}{}
	}
	addEdge := func(edge api.AssetEdgeResponse) {
		if edge.FromAssetID == "" || edge.ToAssetID == "" {
			return
		}
		key := edge.FromAssetID + "|" + edge.ToAssetID + "|" + edge.Type
		if _, ok := seenEdges[key]; ok {
			return
		}
		id := edge.ID
		if id == "" {
			id = key
		}
		graph.Edges = append(graph.Edges, assetLineageEdge{
			ID:     id,
			Source: edge.FromAssetID,
			Target: edge.ToAssetID,
			Label:  labelFromKey(edge.Type),
			Kind:   edge.Type,
		})
		seenEdges[key] = struct{}{}
	}
	addRow := func(depth int, relation string, asset api.AssetResponse) {
		if asset.ID == "" {
			return
		}
		if _, ok := seenRows[asset.ID]; ok {
			return
		}
		rows = append(rows, dashboardLineageRow{
			Depth: depth,
			Row:   lineageTableRowForAsset(workspaceID, relation, asset),
		})
		seenRows[asset.ID] = struct{}{}
	}
	addAsset := func(depth int, relation string, asset api.AssetResponse) {
		addNode(asset)
		addRow(depth, relation, asset)
	}

	for _, metricEdge := range outgoing[selected.ID] {
		if metricEdge.Type != "uses_metric_view" {
			continue
		}
		metricView, ok := byID[metricEdge.ToAssetID]
		if !ok || metricView.Type != "metric_view" {
			continue
		}
		addAsset(1, labelFromKey(metricEdge.Type), metricView)
		addEdge(metricEdge)

		if semanticModel, semanticEdge, ok := semanticModelForMetricView(metricView, byID, incoming); ok {
			addAsset(2, "Semantic model", semanticModel)
			addEdge(semanticEdge)
		}

		for _, datasetEdge := range outgoing[metricView.ID] {
			if datasetEdge.Type != "uses_dataset" {
				continue
			}
			dataset, ok := byID[datasetEdge.ToAssetID]
			if !ok || dataset.Type != "dataset" {
				continue
			}
			addAsset(3, labelFromKey(datasetEdge.Type), dataset)
			addEdge(datasetEdge)

			for _, cacheEdge := range outgoing[dataset.ID] {
				if cacheEdge.Type != "uses_cache_table" {
					continue
				}
				cacheTable, ok := byID[cacheEdge.ToAssetID]
				if !ok || cacheTable.Type != "cache_table" {
					continue
				}
				addAsset(4, labelFromKey(cacheEdge.Type), cacheTable)
				addEdge(cacheEdge)

				for _, sourceEdge := range outgoing[cacheTable.ID] {
					if sourceEdge.Type != "reads_source" {
						continue
					}
					source, ok := byID[sourceEdge.ToAssetID]
					if !ok || source.Type != "source" {
						continue
					}
					addAsset(5, labelFromKey(sourceEdge.Type), source)
					addEdge(sourceEdge)

					for _, connectionEdge := range outgoing[source.ID] {
						if connectionEdge.Type != "uses_connection" {
							continue
						}
						connection, ok := byID[connectionEdge.ToAssetID]
						if !ok || connection.Type != "connection" {
							continue
						}
						addAsset(6, labelFromKey(connectionEdge.Type), connection)
						addEdge(connectionEdge)
					}
				}
			}
		}
	}

	sortDashboardLineageRows(rows)
	useRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		useRows = append(useRows, row.Row)
	}
	usedByRows := dashboardUsedByRows(workspaceID, selected, byID, incoming)
	sortLineageRows(usedByRows)
	return assetLineageModel{
		Count:  len(useRows) + len(usedByRows),
		Graph:  graph,
		Uses:   lineageTable(useRows, "This dashboard does not reference data assets."),
		UsedBy: lineageTable(usedByRows, "No assets reference this dashboard."),
	}
}

func edgesByFromAsset(edges []api.AssetEdgeResponse) map[string][]api.AssetEdgeResponse {
	out := map[string][]api.AssetEdgeResponse{}
	for _, edge := range edges {
		out[edge.FromAssetID] = append(out[edge.FromAssetID], edge)
	}
	return out
}

func edgesByToAsset(edges []api.AssetEdgeResponse) map[string][]api.AssetEdgeResponse {
	out := map[string][]api.AssetEdgeResponse{}
	for _, edge := range edges {
		out[edge.ToAssetID] = append(out[edge.ToAssetID], edge)
	}
	return out
}

func semanticModelForMetricView(metricView api.AssetResponse, assets map[string]api.AssetResponse, incoming map[string][]api.AssetEdgeResponse) (api.AssetResponse, api.AssetEdgeResponse, bool) {
	for _, edge := range incoming[metricView.ID] {
		if edge.Type != "contains" {
			continue
		}
		asset, ok := assets[edge.FromAssetID]
		if ok && asset.Type == "semantic_model" {
			return asset, edge, true
		}
	}
	if asset, ok := assets[metricView.ParentID]; ok && asset.Type == "semantic_model" {
		return asset, api.AssetEdgeResponse{FromAssetID: asset.ID, ToAssetID: metricView.ID, Type: "contains"}, true
	}
	return api.AssetResponse{}, api.AssetEdgeResponse{}, false
}

func dashboardUsedByRows(workspaceID string, selected api.AssetResponse, assets map[string]api.AssetResponse, incoming map[string][]api.AssetEdgeResponse) []map[string]any {
	rows := []map[string]any{}
	for _, edge := range incoming[selected.ID] {
		asset, ok := assets[edge.FromAssetID]
		if !ok || asset.Type == "catalog" {
			continue
		}
		rows = append(rows, lineageTableRow(workspaceID, edge, asset))
	}
	return rows
}

func lineageTableRowForAsset(workspaceID, relation string, asset api.AssetResponse) map[string]any {
	return map[string]any{
		"relation":  relation,
		"asset":     assetTitle(asset),
		"assetHref": lineageAssetHref(workspaceID, asset),
		"type":      assetTypeLabel(asset.Type),
		"key":       asset.Key,
	}
}

func sortDashboardLineageRows(rows []dashboardLineageRow) {
	sort.Slice(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		if left.Depth != right.Depth {
			return left.Depth < right.Depth
		}
		return fmt.Sprint(left.Row["type"], left.Row["asset"], left.Row["key"]) < fmt.Sprint(right.Row["type"], right.Row["asset"], right.Row["key"])
	})
}

func sortLineageRows(rows []map[string]any) {
	sort.Slice(rows, func(i, j int) bool {
		return fmt.Sprint(rows[i]["relation"], rows[i]["type"], rows[i]["asset"]) < fmt.Sprint(rows[j]["relation"], rows[j]["type"], rows[j]["asset"])
	})
}

func lineageTableRow(workspaceID string, edge api.AssetEdgeResponse, peer api.AssetResponse) map[string]any {
	return map[string]any{
		"relation":  labelFromKey(edge.Type),
		"asset":     assetTitle(peer),
		"assetHref": lineageAssetHref(workspaceID, peer),
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

func lineageNode(workspaceID string, asset api.AssetResponse, side string, selected bool) assetLineageNode {
	return assetLineageNode{
		ID:       asset.ID,
		Label:    assetTitle(asset),
		Kind:     asset.Type,
		Meta:     asset.Key,
		Href:     lineageAssetHref(workspaceID, asset),
		Side:     side,
		Selected: selected,
	}
}

func relationSortKey(selectedID string, edge api.AssetEdgeResponse, assets map[string]api.AssetResponse) string {
	direction := "out"
	peerID := edge.ToAssetID
	if edge.ToAssetID == selectedID {
		direction = "in"
		peerID = edge.FromAssetID
	}
	peer := assets[peerID]
	return direction + ":" + edge.Type + ":" + peer.Type + ":" + assetTitle(peer)
}

func lineagePeerType(selectedID string, edge api.AssetEdgeResponse, from, to api.AssetResponse) string {
	if edge.ToAssetID == selectedID {
		return from.Type
	}
	return to.Type
}

func lineageAssetHref(workspaceID string, asset api.AssetResponse) string {
	return workspaceAssetSectionHref(workspaceID, asset.ID, "details")
}

func workspaceAssetSectionHref(workspaceID, assetID, section string) string {
	return "/workspaces/" + workspaceID + "/assets/" + assetID + "/" + section
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
}

func assetDetailsSection(workspace api.WorkspaceResponse, asset api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse) g.Node {
	model := assetDetailModelForAsset(workspace, asset, assets, edges)
	return h.Section(h.ID("details"), h.Class("grid content-start gap-6"), h.Aria("label", "Asset details"),
		definitionStats("Overview", model.Overview),
		g.Map(model.Sections, assetDetailSectionNode),
	)
}

func assetDetailSectionNode(section assetDetailSection) g.Node {
	if section.Signal != "" {
		return definitionSignalGrid(section.Title, section.Signal)
	}
	return definitionFacts(section.Title, section.Facts)
}

func assetDetailModelForAsset(workspace api.WorkspaceResponse, asset api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse) assetDetailModel {
	model := assetDetailModel{
		Overview: commonAssetOverviewFacts(asset, assets, shouldShowParentFact(asset.Type)),
	}
	switch asset.Type {
	case "semantic_model":
		semanticModelDetailModel(&model, workspace, asset, assets, edges)
	case "metric_view":
		metricViewDetailModel(&model, asset, assets)
	case "dashboard":
		dashboardDetailModel(&model, asset, assets)
	case "connection":
		model.Overview = append(model.Overview, connectionFacts(asset)...)
	case "source":
		model.Overview = append(model.Overview, sourceFacts(asset)...)
	case "measure":
		model.Overview = append(model.Overview, metricLeafFacts(asset)...)
	case "dimension":
		model.Overview = append(model.Overview, metricLeafFacts(asset)...)
	default:
		model.Overview = append(model.Overview, metaFacts(asset.Meta)...)
	}
	return model
}

func commonAssetOverviewFacts(asset api.AssetResponse, assets []api.AssetResponse, includeParent bool) []definitionFact {
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
	case "catalog", "dashboard", "metric_view", "semantic_model":
		return false
	default:
		return true
	}
}

func semanticModelDetailModel(model *assetDetailModel, workspace api.WorkspaceResponse, asset api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse) {
	meta := asset.Meta
	connections := sortedMapKeys(metaMap(meta, "Connections", "connections"))
	sources := sortedMapKeys(metaMap(meta, "Sources", "sources"))
	cacheTables := sortedMapKeys(metaMap(metaMap(meta, "Cache", "cache"), "Tables", "tables"))
	datasets := sortedMapKeys(metaMap(meta, "Datasets", "datasets"))
	relationships := metaSlice(meta, "Relationships", "relationships")

	model.Overview = append(model.Overview,
		definitionFact{Label: "Default connection", Value: metaString(meta, "DefaultConnection", "default_connection")},
		definitionFact{Label: "Connections", Value: fmt.Sprint(len(connections))},
		definitionFact{Label: "Sources", Value: fmt.Sprint(len(sources))},
		definitionFact{Label: "Cache tables", Value: fmt.Sprint(len(cacheTables))},
		definitionFact{Label: "Datasets", Value: fmt.Sprint(len(datasets))},
		definitionFact{Label: "Relationships", Value: fmt.Sprint(len(relationships))},
	)
	model.Sections = append(model.Sections,
		assetDetailSection{Title: fmt.Sprintf("Connections (%d)", len(connections)), Signal: "assetDetailsSemanticConnectionsGrid", Grid: semanticConnectionsGrid(workspace.ID, asset, assets, meta)},
		assetDetailSection{Title: fmt.Sprintf("Sources (%d)", len(sources)), Signal: "assetDetailsSemanticSourcesGrid", Grid: semanticSourcesGrid(workspace.ID, asset, assets, meta)},
		assetDetailSection{Title: fmt.Sprintf("Cache tables (%d)", len(cacheTables)), Signal: "assetDetailsSemanticCacheTablesGrid", Grid: semanticCacheGrid(workspace.ID, asset, assets, edges, meta)},
		assetDetailSection{Title: fmt.Sprintf("Datasets (%d)", len(datasets)), Signal: "assetDetailsSemanticDatasetsGrid", Grid: semanticDatasetsGrid(workspace.ID, asset, assets, meta)},
		assetDetailSection{Title: fmt.Sprintf("Relationships (%d)", len(relationships)), Signal: "assetDetailsSemanticRelationshipsGrid", Grid: semanticRelationshipsGrid(meta)},
	)
}

func assetParentTitle(parentID string, assets []api.AssetResponse) string {
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

func semanticModelDetails(asset api.AssetResponse) []g.Node {
	meta := asset.Meta
	connections := sortedMapKeys(metaMap(meta, "Connections", "connections"))
	sources := sortedMapKeys(metaMap(meta, "Sources", "sources"))
	cacheTables := sortedMapKeys(metaMap(metaMap(meta, "Cache", "cache"), "Tables", "tables"))
	datasets := sortedMapKeys(metaMap(meta, "Datasets", "datasets"))
	relationships := metaSlice(meta, "Relationships", "relationships")

	return []g.Node{
		definitionStats("Overview", []definitionFact{
			{Label: "Default connection", Value: metaString(meta, "DefaultConnection", "default_connection")},
			{Label: "Connections", Value: fmt.Sprint(len(connections))},
			{Label: "Sources", Value: fmt.Sprint(len(sources))},
			{Label: "Cache tables", Value: fmt.Sprint(len(cacheTables))},
			{Label: "Datasets", Value: fmt.Sprint(len(datasets))},
			{Label: "Relationships", Value: fmt.Sprint(len(relationships))},
		}),
	}
}

func semanticConnectionsGrid(workspaceID string, parent api.AssetResponse, assets []api.AssetResponse, meta map[string]any) metricGrid {
	connections := metaMap(meta, "Connections", "connections")
	rows := make([]map[string]any, 0, len(connections))
	for _, name := range sortedMapKeys(connections) {
		connection := asMap(connections[name])
		child := childAssetByName(parent.ID, "connection", name, assets)
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

func semanticSourcesGrid(workspaceID string, parent api.AssetResponse, assets []api.AssetResponse, meta map[string]any) metricGrid {
	sources := metaMap(meta, "Sources", "sources")
	rows := make([]map[string]any, 0, len(sources))
	for _, name := range sortedMapKeys(sources) {
		source := asMap(sources[name])
		child := childAssetByName(parent.ID, "source", name, assets)
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

func semanticCacheGrid(workspaceID string, parent api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse, meta map[string]any) metricGrid {
	tables := metaMap(metaMap(meta, "Cache", "cache"), "Tables", "tables")
	rows := make([]map[string]any, 0, len(tables))
	for _, name := range sortedMapKeys(tables) {
		table := asMap(tables[name])
		child := childAssetByName(parent.ID, "cache_table", name, assets)
		rows = append(rows, map[string]any{
			"name":        name,
			"nameHref":    childHref(workspaceID, child),
			"description": emptyDash(metaString(table, "Description", "description")),
			"reads":       strings.Join(dependentAssetNames(child.ID, "reads_source", assets, edges), ", "),
			"sql":         sqlPreview(metaString(table, "SQL", "sql")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "180px"},
			{ID: "description", Header: "Description", Width: "320px"},
			{ID: "reads", Header: "Reads", Kind: "expression", Width: "220px"},
			{ID: "sql", Header: "SQL preview", Kind: "expression"},
		},
		Rows:     rows,
		Empty:    "No cache tables are defined for this semantic model.",
		MinWidth: "980px",
	}
}

func semanticDatasetsGrid(workspaceID string, parent api.AssetResponse, assets []api.AssetResponse, meta map[string]any) metricGrid {
	datasets := metaMap(meta, "Datasets", "datasets")
	rows := make([]map[string]any, 0, len(datasets))
	for _, name := range sortedMapKeys(datasets) {
		dataset := asMap(datasets[name])
		child := childAssetByName(parent.ID, "dataset", name, assets)
		sourceName := metaString(dataset, "Source", "source")
		source := childAssetByName(parent.ID, "cache_table", sourceName, assets)
		rows = append(rows, map[string]any{
			"name":       name,
			"nameHref":   childHref(workspaceID, child),
			"source":     emptyDash(sourceName),
			"sourceHref": childHref(workspaceID, source),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "220px"},
			{ID: "source", Header: "Source cache table", Kind: "link", HrefKey: "sourceHref"},
		},
		Rows:     rows,
		Empty:    "No datasets are defined for this semantic model.",
		MinWidth: "520px",
	}
}

func semanticRelationshipsGrid(meta map[string]any) metricGrid {
	relationships := metaSlice(meta, "Relationships", "relationships")
	rows := make([]map[string]any, 0, len(relationships))
	for _, item := range relationships {
		relationship := asMap(item)
		rows = append(rows, map[string]any{
			"id":          metaString(relationship, "ID", "id"),
			"from":        metaString(relationship, "From", "from"),
			"to":          metaString(relationship, "To", "to"),
			"cardinality": metricGridBadgeValue(metaString(relationship, "Cardinality", "cardinality"), "muted"),
			"active":      metricGridBadgeValue(boolLabel(metaBool(relationship, "Active", "active")), "success"),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "id", Header: "ID", Kind: "code", Width: "190px"},
			{ID: "from", Header: "From", Kind: "code", Width: "240px"},
			{ID: "to", Header: "To", Kind: "code", Width: "240px"},
			{ID: "cardinality", Header: "Cardinality", Kind: "badge", Width: "140px"},
			{ID: "active", Header: "Active", Kind: "badge", Width: "90px"},
		},
		Rows:     rows,
		Empty:    "No relationships are defined for this semantic model.",
		MinWidth: "900px",
	}
}

func metricViewDetailModel(model *assetDetailModel, asset api.AssetResponse, assets []api.AssetResponse) {
	measures := childrenByType(asset.ID, "measure", assets)
	dimensions := childrenByType(asset.ID, "dimension", assets)
	semanticModel := assetParentTitle(asset.ParentID, assets)
	if semanticModel == "" {
		semanticModel = metaString(asset.Meta, "SemanticModel", "semantic_model")
	}
	model.Overview = append(model.Overview,
		definitionFact{Label: "Semantic model", Value: semanticModel},
		definitionFact{Label: "Dataset", Value: metaString(asset.Meta, "Dataset", "dataset")},
		definitionFact{Label: "Timeseries", Value: metaString(asset.Meta, "Timeseries", "timeseries")},
	)
	model.Sections = append(model.Sections,
		assetDetailSection{Title: fmt.Sprintf("Measures (%d)", len(measures)), Signal: "assetDetailsMeasuresGrid", Grid: metricViewMeasuresGrid(asset, measures)},
		assetDetailSection{Title: fmt.Sprintf("Dimensions (%d)", len(dimensions)), Signal: "assetDetailsDimensionsGrid", Grid: metricViewDimensionsGrid(asset, dimensions)},
	)
}

func metricViewMeasuresGrid(parent api.AssetResponse, measures []api.AssetResponse) metricGrid {
	sort.Slice(measures, func(i, j int) bool {
		return metricChildName(parent, measures[i]) < metricChildName(parent, measures[j])
	})
	rows := make([]map[string]any, 0, len(measures))
	for _, measure := range measures {
		name := metricChildName(parent, measure)
		rows = append(rows, map[string]any{
			"name":       name,
			"nameHref":   workspaceAssetSectionHref(parent.WorkspaceID, measure.ID, "details"),
			"label":      displayLabel(assetTitle(measure), name),
			"expression": metaString(measure.Meta, "Expression", "expression"),
			"unit":       metricGridBadgeValue(metaString(measure.Meta, "Unit", "unit"), "success"),
			"format":     metricGridBadgeValue(metaString(measure.Meta, "Format", "format"), "accent"),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "150px"},
			{ID: "label", Header: "Label", Width: "160px"},
			{ID: "expression", Header: "Expression", Kind: "expression"},
			{ID: "unit", Header: "Unit", Kind: "badge", Width: "82px"},
			{ID: "format", Header: "Format", Kind: "badge", Width: "88px"},
		},
		Rows:     rows,
		Empty:    "No measures are defined for this metric view.",
		MinWidth: "100%",
	}
}

func metricViewDimensionsGrid(parent api.AssetResponse, dimensions []api.AssetResponse) metricGrid {
	sort.Slice(dimensions, func(i, j int) bool {
		return metricChildName(parent, dimensions[i]) < metricChildName(parent, dimensions[j])
	})
	rows := make([]map[string]any, 0, len(dimensions))
	for _, dimension := range dimensions {
		name := metricChildName(parent, dimension)
		rows = append(rows, map[string]any{
			"name":       name,
			"nameHref":   workspaceAssetSectionHref(parent.WorkspaceID, dimension.ID, "details"),
			"label":      displayLabel(assetTitle(dimension), name),
			"expression": metaString(dimension.Meta, "Expr", "expr", "Expression", "expression"),
			"filter":     emptyDash(metaString(dimension.Meta, "Where", "where")),
			"order":      emptyDash(metaString(dimension.Meta, "OrderExpr", "order_expr")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "name", Header: "Name", Kind: "link", HrefKey: "nameHref", Width: "170px"},
			{ID: "label", Header: "Label", Width: "180px"},
			{ID: "expression", Header: "Expression", Kind: "expression", Width: "260px"},
			{ID: "filter", Header: "Filter", Kind: "expression", Width: "220px"},
			{ID: "order", Header: "Order", Kind: "expression", Width: "190px"},
		},
		Rows:     rows,
		Empty:    "No dimensions are defined for this metric view.",
		MinWidth: "1020px",
	}
}

func dashboardDetailModel(model *assetDetailModel, asset api.AssetResponse, assets []api.AssetResponse) {
	pages := childrenByType(asset.ID, "page", assets)
	filters := childrenByType(asset.ID, "filter", assets)
	visuals := childrenByType(asset.ID, "visual", assets)
	tables := childrenByType(asset.ID, "table", assets)
	model.Overview = append(model.Overview,
		definitionFact{Label: "Metric views", Value: strings.Join(stringSlice(metaValue(asset.Meta, "MetricViews", "metrics_views")), ", ")},
		definitionFact{Label: "Tags", Value: strings.Join(stringSlice(metaValue(asset.Meta, "Tags", "tags")), ", ")},
	)
	model.Sections = append(model.Sections,
		assetDetailSection{Title: fmt.Sprintf("Pages (%d)", len(pages)), Signal: "assetDetailsPagesGrid", Grid: dashboardPagesGrid(asset, pages)},
		assetDetailSection{Title: fmt.Sprintf("Filters (%d)", len(filters)), Signal: "assetDetailsFiltersGrid", Grid: dashboardFiltersGrid(asset, filters)},
		assetDetailSection{Title: fmt.Sprintf("Visuals (%d)", len(visuals)), Signal: "assetDetailsVisualsGrid", Grid: dashboardVisualsGrid(asset, visuals)},
		assetDetailSection{Title: fmt.Sprintf("Tables (%d)", len(tables)), Signal: "assetDetailsTablesGrid", Grid: dashboardTablesGrid(asset, tables)},
	)
}

func dashboardPagesGrid(parent api.AssetResponse, pages []api.AssetResponse) metricGrid {
	rows := make([]map[string]any, 0, len(pages))
	for _, page := range pages {
		key := assetChildName(parent, page)
		rows = append(rows, map[string]any{
			"page":        assetTitle(page),
			"pageHref":    workspaceAssetSectionHref(parent.WorkspaceID, page.ID, "details"),
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

func dashboardFiltersGrid(parent api.AssetResponse, filters []api.AssetResponse) metricGrid {
	sortAssetChildren(parent, filters)
	rows := make([]map[string]any, 0, len(filters))
	for _, filter := range filters {
		rows = append(rows, map[string]any{
			"filter":     assetTitle(filter),
			"filterHref": workspaceAssetSectionHref(parent.WorkspaceID, filter.ID, "details"),
			"key":        assetChildName(parent, filter),
			"metricView": emptyDash(metaString(filter.Meta, "MetricView", "metric_view", "metrics_view")),
			"dimension":  emptyDash(metaString(filter.Meta, "Dimension", "dimension")),
			"type":       emptyDash(metaString(filter.Meta, "Type", "type", "Kind", "kind")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "filter", Header: "Filter", Kind: "link", HrefKey: "filterHref", Width: "190px"},
			{ID: "key", Header: "Key", Kind: "code", Width: "160px"},
			{ID: "metricView", Header: "Metric view", Kind: "code", Width: "150px"},
			{ID: "dimension", Header: "Dimension", Kind: "code", Width: "180px"},
			{ID: "type", Header: "Type", Width: "120px"},
		},
		Rows:     rows,
		Empty:    "No filters are defined for this dashboard.",
		MinWidth: "820px",
	}
}

func dashboardVisualsGrid(parent api.AssetResponse, visuals []api.AssetResponse) metricGrid {
	sortAssetChildren(parent, visuals)
	rows := make([]map[string]any, 0, len(visuals))
	for _, visual := range visuals {
		query := metaMap(visual.Meta, "Query", "query")
		rows = append(rows, map[string]any{
			"visual":     assetTitle(visual),
			"visualHref": workspaceAssetSectionHref(parent.WorkspaceID, visual.ID, "details"),
			"key":        assetChildName(parent, visual),
			"metricView": emptyDash(metaString(visual.Meta, "MetricView", "metric_view", "metrics_view")),
			"type":       emptyDash(firstNonEmpty(metaString(visual.Meta, "Shape", "shape"), metaString(visual.Meta, "Type", "type"), metaString(visual.Meta, "Kind", "kind"))),
			"measures":   emptyDash(strings.Join(stringSlice(metaValue(query, "Measures", "measures")), ", ")),
			"dimensions": emptyDash(strings.Join(stringSlice(metaValue(query, "Dimensions", "dimensions")), ", ")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "visual", Header: "Visual", Kind: "link", HrefKey: "visualHref", Width: "230px"},
			{ID: "key", Header: "Key", Kind: "code", Width: "180px"},
			{ID: "metricView", Header: "Metric view", Kind: "code", Width: "140px"},
			{ID: "type", Header: "Type", Width: "120px"},
			{ID: "measures", Header: "Measures", Kind: "expression", Width: "220px"},
			{ID: "dimensions", Header: "Dimensions", Kind: "expression"},
		},
		Rows:     rows,
		Empty:    "No visuals are defined for this dashboard.",
		MinWidth: "1040px",
	}
}

func dashboardTablesGrid(parent api.AssetResponse, tables []api.AssetResponse) metricGrid {
	sortAssetChildren(parent, tables)
	rows := make([]map[string]any, 0, len(tables))
	for _, table := range tables {
		rows = append(rows, map[string]any{
			"table":      assetTitle(table),
			"tableHref":  workspaceAssetSectionHref(parent.WorkspaceID, table.ID, "details"),
			"key":        assetChildName(parent, table),
			"metricView": emptyDash(metaString(table.Meta, "MetricView", "metric_view", "metrics_view")),
			"rows":       emptyDash(strings.Join(stringSlice(metaValue(table.Meta, "Rows", "rows")), ", ")),
			"measures":   emptyDash(strings.Join(stringSlice(metaValue(table.Meta, "Measures", "measures")), ", ")),
		})
	}
	return metricGrid{
		Columns: []metricGridColumn{
			{ID: "table", Header: "Table", Kind: "link", HrefKey: "tableHref", Width: "220px"},
			{ID: "key", Header: "Key", Kind: "code", Width: "170px"},
			{ID: "metricView", Header: "Metric view", Kind: "code", Width: "140px"},
			{ID: "rows", Header: "Rows", Kind: "expression", Width: "280px"},
			{ID: "measures", Header: "Measures", Kind: "expression"},
		},
		Rows:     rows,
		Empty:    "No tables are defined for this dashboard.",
		MinWidth: "920px",
	}
}

func connectionFacts(asset api.AssetResponse) []definitionFact {
	return []definitionFact{
		{Label: "Kind", Value: metaString(asset.Meta, "Kind", "kind")},
		{Label: "Scope", Value: metaString(asset.Meta, "Scope", "scope")},
		{Label: "Root", Value: metaString(asset.Meta, "Root", "root")},
		{Label: "Path", Value: metaString(asset.Meta, "Path", "path")},
		{Label: "Credentials", Value: boolLabel(metaBool(asset.Meta, "credentials_configured"))},
		{Label: "Options", Value: compactJSON(metaValue(asset.Meta, "Options", "options"))},
	}
}

func sourceFacts(asset api.AssetResponse) []definitionFact {
	return []definitionFact{
		{Label: "Connection", Value: metaString(asset.Meta, "Connection", "connection")},
		{Label: "Format", Value: metaString(asset.Meta, "Format", "format")},
		{Label: "Path", Value: metaString(asset.Meta, "Path", "path")},
		{Label: "Object", Value: metaString(asset.Meta, "Object", "object")},
		{Label: "Options", Value: compactJSON(metaValue(asset.Meta, "Options", "options"))},
	}
}

func metricLeafFacts(asset api.AssetResponse) []definitionFact {
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

func childAssetGrid(workspaceID string, assets []api.AssetResponse, empty string) metricGrid {
	sort.Slice(assets, func(i, j int) bool {
		return assetTitle(assets[i]) < assetTitle(assets[j])
	})
	rows := make([]map[string]any, 0, len(assets))
	for _, asset := range assets {
		rows = append(rows, map[string]any{
			"name":        assetTitle(asset),
			"nameHref":    workspaceAssetSectionHref(workspaceID, asset.ID, "details"),
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

func childDependencyGrid(workspaceID, assetID string, assets []api.AssetResponse, edges []api.AssetEdgeResponse) metricGrid {
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
			"assetHref": workspaceAssetSectionHref(workspaceID, peer.ID, "details"),
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sqlPreview(sql string) string {
	sql = strings.Join(strings.Fields(sql), " ")
	if len(sql) > 160 {
		return sql[:157] + "..."
	}
	return sql
}

func childAssetByName(parentID, typ, name string, assets []api.AssetResponse) api.AssetResponse {
	for _, asset := range assets {
		if asset.ParentID != parentID || asset.Type != typ {
			continue
		}
		if asset.Title == name || asset.Key == name || strings.HasSuffix(asset.Key, "."+name) {
			return asset
		}
	}
	return api.AssetResponse{}
}

func childrenByType(parentID, typ string, assets []api.AssetResponse) []api.AssetResponse {
	out := []api.AssetResponse{}
	for _, asset := range assets {
		if asset.ParentID == parentID && asset.Type == typ {
			out = append(out, asset)
		}
	}
	return out
}

func metricChildName(parent, child api.AssetResponse) string {
	return assetChildName(parent, child)
}

func assetChildName(parent, child api.AssetResponse) string {
	prefix := parent.Key + "."
	if strings.HasPrefix(child.Key, prefix) {
		return strings.TrimPrefix(child.Key, prefix)
	}
	if child.Key != "" {
		return child.Key
	}
	return assetTitle(child)
}

func sortAssetChildren(parent api.AssetResponse, children []api.AssetResponse) {
	sort.Slice(children, func(i, j int) bool {
		left := assetChildName(parent, children[i])
		right := assetChildName(parent, children[j])
		if left == right {
			return assetTitle(children[i]) < assetTitle(children[j])
		}
		return left < right
	})
}

func childHref(workspaceID string, asset api.AssetResponse) string {
	if asset.ID == "" {
		return ""
	}
	return workspaceAssetSectionHref(workspaceID, asset.ID, "details")
}

func assetsByID(assets []api.AssetResponse) map[string]api.AssetResponse {
	byID := map[string]api.AssetResponse{}
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	return byID
}

func dependentAssetNames(assetID, edgeType string, assets []api.AssetResponse, edges []api.AssetEdgeResponse) []string {
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

func roleBindingRow(workspaceID string, binding api.RoleBindingResponse, csrfToken string) g.Node {
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

func assetTitle(asset api.AssetResponse) string {
	return displayLabel(asset.Title, asset.Key)
}

func assetTypeLabel(typ string) string {
	switch typ {
	case "semantic_model":
		return "Semantic model"
	case "metric_view":
		return "Metric view"
	case "cache_table":
		return "Cache table"
	default:
		return strings.Title(strings.ReplaceAll(typ, "_", " "))
	}
}

func labelFromKey(key string) string {
	return strings.Title(strings.ReplaceAll(key, "_", " "))
}
