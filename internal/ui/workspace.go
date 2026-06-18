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
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

const (
	workspaceMainClass  = "grid min-w-0 min-h-svh content-start gap-3 bg-report-workspace px-4 py-4 max-sm:min-h-0 max-sm:p-3"
	workspacePanelClass = "grid min-w-0 rounded-default border border-outline-muted bg-surface"
	assetRowClass       = "grid min-w-0 grid-cols-asset-row items-center gap-3 border-b border-outline-muted px-3 py-2 last:border-b-0 hover:bg-control-hover"
)

func WorkspacesPage(catalog dashboard.Catalog, workspaces []api.WorkspaceResponse, roleLabel string) g.Node {
	return workspaceDocument("LibreDash Workspaces", catalog, "workspaces", roleLabel,
		h.Section(h.Class(catalogMainClass), h.Aria("label", "LibreDash workspaces"),
			workspaceHeader("", "Workspaces", "View published BI workspaces. Authoring lives in Git.", nil),
			h.Div(h.Class("grid grid-cols-catalog-grid items-start justify-start gap-4"),
				g.Map(workspaces, workspaceCard),
			),
		),
	)
}

func WorkspacePage(catalog dashboard.Catalog, workspace api.WorkspaceResponse, assets []api.AssetResponse, activeType, query, roleLabel string) g.Node {
	return workspaceDocument(workspace.Title, catalog, "workspaces", roleLabel,
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
	definitionFields := assetDefinitionFields(asset.Meta)
	lineage := assetLineage(workspace.ID, asset, assets, edges)
	extraHead := []g.Node{}
	if activeSection == "lineage" {
		extraHead = append(extraHead,
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/asset-lineage-graph.css"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/asset-lineage-graph.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/data-grid.js"))),
		)
	}
	return workspaceDocument(asset.Title, catalog, "workspaces", roleLabel,
		h.Section(h.Class(metricMainClass), h.Aria("label", "Workspace asset detail"),
			workspaceHeader(
				assetTypeLabel(asset.Type),
				assetTitle(asset),
				asset.Description,
				assetActions(workspace.ID, asset),
			),
			g.El("ld-detail-rail", h.Class(metricWorkspaceClass), g.Attr("data-detail-rail", ""),
				h.Div(h.Class(metricContentColumnClass),
					assetDetailTabs(workspace.ID, asset.ID, activeSection, lineage.Count),
					h.Div(h.Class("min-h-0 overflow-auto px-4 py-4"),
						g.If(activeSection == "definition",
							h.Section(h.ID("definition"), h.Class("grid content-start"), h.Aria("label", "Asset definition"),
								g.If(len(definitionFields) == 0, emptyState("No YAML-derived definition metadata is available for this asset.")),
								g.Group(definitionFields),
							),
						),
						g.If(activeSection == "lineage", assetLineageSection(lineage)),
					),
				),
				assetDetailSidebar(asset),
			),
		),
		extraHead...,
	)
}

func WorkspacePermissionsPage(catalog dashboard.Catalog, workspace api.WorkspaceResponse, bindings []api.RoleBindingResponse, roles []api.RoleResponse, csrfToken, roleLabel string) g.Node {
	return workspaceDocument("Workspace permissions", catalog, "settings", roleLabel,
		h.Section(h.Class(catalogMainClass), h.Aria("label", "Workspace permissions"),
			workspaceHeader("Workspace", workspace.Title, "Assign workspace roles. BI assets remain authored in Git.", nil),
			h.Div(h.Class("grid max-w-workspace-detail grid-cols-workspace-detail gap-4 max-lg:grid-cols-1"),
				h.Section(h.Class(workspacePanelClass+" content-start p-4"),
					h.H2(h.Class("m-0 text-body-md font-850 text-fg-default"), g.Text("Assign role")),
					h.Form(h.Method("post"), h.Action("/workspaces/"+workspace.ID+"/permissions"), h.Class("mt-4 grid gap-3"),
						g.If(csrfToken != "", h.Input(h.Type("hidden"), h.Name("gorilla.csrf.Token"), h.Value(csrfToken))),
						formInput("Email", "email", "person@example.com", ""),
						formInput("Display name", "displayName", "Optional", ""),
						h.Label(h.Class("grid gap-1 text-caption font-850 uppercase text-fg-muted"),
							g.Text("Role"),
							h.Select(h.Name("role"), h.Class("min-h-control-md rounded-small border border-outline-variant bg-container px-2 text-body-sm font-720 text-fg-default"),
								g.Map(roles, func(role api.RoleResponse) g.Node {
									return h.Option(h.Value(role.Name), g.Text(role.Name))
								}),
							),
						),
						h.Button(h.Type("submit"), h.Class(primaryLinkButtonClass+" justify-self-start"), lucide.UserPlus(buttonIconAttrs()...), h.Span(g.Text("Assign"))),
					),
				),
				h.Section(h.Class(workspacePanelClass+" content-start p-4"),
					h.H2(h.Class("m-0 text-body-md font-850 text-fg-default"), g.Text("Current access")),
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

func workspaceDocument(title string, catalog dashboard.Catalog, active, roleLabel string, content g.Node, extraHead ...g.Node) g.Node {
	head := []g.Node{
		h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
		h.Script(h.Type("module"), h.Src(staticAsset("/static/detail-rail.js"))),
		inspectorScript(),
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
	return h.Div(h.Class("grid min-w-0 gap-3 border-b border-outline-variant bg-report-workspace px-3 pt-3"), g.Attr("data-workspace-asset-toolbar", ""),
		h.Form(h.Method("get"), h.Action("/workspaces/"+workspaceID), h.Class("flex min-w-0 max-w-workspace-search items-center gap-2"),
			h.Input(h.Type("search"), h.Name("q"), h.Value(query), h.Placeholder("Search workspace assets..."), h.Class("min-h-control-md w-full rounded-small border border-outline-variant bg-container px-3 text-body-sm font-720 text-fg-default placeholder:text-fg-muted")),
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
	className := "relative -mb-px inline-flex min-h-control-xl items-center whitespace-nowrap border-b-2 px-1 text-body-sm font-850 no-underline transition-colors duration-fast"
	if typ == activeType {
		className += " border-fg-accent text-fg-default"
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
	case "definition", "lineage":
		return true
	default:
		return false
	}
}

func normalizeWorkspaceAssetSection(section string) string {
	if ValidWorkspaceAssetSection(section) {
		return section
	}
	return "definition"
}

func assetRow(workspaceID string, asset api.AssetResponse) g.Node {
	detailHref := workspaceAssetSectionHref(workspaceID, asset.ID, "definition")
	openHref := detailHref
	if asset.Href != "" {
		openHref = asset.Href
	}
	return h.Article(h.Class(assetRowClass),
		assetTypeIcon(asset.Type),
		h.Div(h.Class("min-w-0"),
			h.P(h.Class("m-0 text-caption font-900 uppercase text-fg-muted"), g.Text(assetTypeLabel(asset.Type))),
			h.A(h.Class("mt-0.5 block truncate text-body-sm font-850 text-fg-default no-underline hover:underline"), h.Href(detailHref), g.Text(assetTitle(asset))),
			g.If(asset.Description != "", h.P(h.Class("m-0 mt-1 truncate text-caption font-650 text-fg-muted"), g.Text(asset.Description))),
		),
		h.Code(h.Class("truncate text-caption font-720 text-fg-muted"), g.Text(asset.Key)),
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
	return h.Nav(h.Class("flex min-w-0 gap-6 border-b border-outline-variant bg-report-workspace px-3"), h.Aria("label", "Workspace asset sections"),
		assetDetailTabLink(workspaceAssetSectionHref(workspaceID, assetID, "definition"), activeSection == "definition", "Definition", nil),
		assetDetailTabLink(workspaceAssetSectionHref(workspaceID, assetID, "lineage"), activeSection == "lineage", "Lineage", metricTabCount(relatedCount)),
	)
}

func assetDetailTabLink(href string, active bool, label string, meta g.Node) g.Node {
	className := "relative -mb-px inline-flex min-h-control-xl items-center gap-2 whitespace-nowrap border-b-2 px-1 text-body-sm font-850 no-underline transition-colors duration-fast"
	if active {
		className += " border-fg-accent text-fg-default"
	} else {
		className += " border-transparent text-fg-muted hover:border-outline-muted hover:text-fg-default"
	}
	return h.A(h.Class(className), h.Href(href), g.If(active, h.Aria("current", "page")), h.Span(g.Text(label)), meta)
}

func assetDetailSidebar(asset api.AssetResponse) g.Node {
	return h.Aside(h.Class(metricInfoSidebarClass), h.Aria("label", "Asset details"), g.Attr("data-metric-info-sidebar", ""),
		h.Div(h.Class("flex min-h-control-xl items-center justify-between gap-2 border-b border-outline-muted px-4 py-2"), g.Attr("data-metric-info-header", ""),
			h.H2(h.Class("m-0 flex min-w-0 items-center gap-2 truncate text-body-sm font-850 text-fg-default"), assetTypeIcon(asset.Type), h.Span(g.Text("Details"))),
		),
		h.Div(h.Class("grid content-start overflow-auto"), g.Attr("data-metric-info-body", ""),
			h.Div(h.Class("grid content-start"),
				metricInfoItem("Type", h.Span(g.Text(assetTypeLabel(asset.Type)))),
				metricInfoItem("Key", h.Code(h.Class("truncate font-mono"), g.Text(asset.Key))),
				g.If(asset.Description != "", metricInfoItem("Description", h.Span(g.Text(asset.Description)))),
			),
		),
	)
}

type assetLineageModel struct {
	Count int
	Graph assetLineageGraph
	Grid  metricGrid
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
	return h.Section(h.ID("lineage"), h.Class("grid content-start gap-3 pt-8"), h.Aria("label", "Asset lineage"),
		h.Div(h.Class("flex min-h-control-xl items-center gap-2 border-b border-outline-variant"),
			h.H2(h.Class("m-0 text-body-sm font-850 text-fg-default"), g.Text("Lineage")),
			metricTabCount(lineage.Count),
		),
		g.El("ld-asset-lineage-graph", h.Class("block h-metric-usage min-h-0 rounded-default border border-outline-muted bg-surface"), g.Attr("data-graph", jsonString(lineage.Graph))),
		g.El("ld-data-grid", g.Attr("data-grid", jsonString(lineage.Grid))),
	)
}

func assetLineage(workspaceID string, selected api.AssetResponse, assets []api.AssetResponse, edges []api.AssetEdgeResponse) assetLineageModel {
	byID := map[string]api.AssetResponse{}
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	graph := assetLineageGraph{
		Nodes: []assetLineageNode{lineageNode(workspaceID, selected, "selected", true)},
	}
	rows := []map[string]any{}
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
		direction := metricGridBadge{Label: "Outgoing", Tone: "accent"}
		if edge.ToAssetID == selected.ID {
			direction = metricGridBadge{Label: "Incoming", Tone: "muted"}
		}
		rows = append(rows, map[string]any{
			"direction": direction,
			"relation":  labelFromKey(edge.Type),
			"from":      assetTitle(from),
			"fromHref":  lineageAssetHref(workspaceID, from),
			"to":        assetTitle(to),
			"toHref":    lineageAssetHref(workspaceID, to),
			"type":      assetTypeLabel(lineagePeerType(selected.ID, edge, from, to)),
		})
	}
	return assetLineageModel{
		Count: len(rows),
		Graph: graph,
		Grid: metricGrid{
			Columns: []metricGridColumn{
				{ID: "direction", Header: "Direction", Kind: "badge", Width: "120px"},
				{ID: "relation", Header: "Relationship", Width: "190px"},
				{ID: "from", Header: "From", Kind: "link", HrefKey: "fromHref", Width: "240px"},
				{ID: "to", Header: "To", Kind: "link", HrefKey: "toHref", Width: "240px"},
				{ID: "type", Header: "Asset type", Width: "150px"},
			},
			Rows:     rows,
			Empty:    "No direct lineage for this asset.",
			MinWidth: "940px",
		},
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
	return workspaceAssetSectionHref(workspaceID, asset.ID, "definition")
}

func workspaceAssetSectionHref(workspaceID, assetID, section string) string {
	return "/workspaces/" + workspaceID + "/assets/" + assetID + "/" + section
}

func assetDefinitionFields(meta map[string]any) []g.Node {
	if len(meta) == 0 {
		return nil
	}
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	nodes := make([]g.Node, 0, len(keys))
	for _, key := range keys {
		if assetDefinitionDetailKey(key) {
			continue
		}
		nodes = append(nodes, assetDefinitionField(labelFromKey(key), assetDefinitionValue(meta[key])))
	}
	return nodes
}

func assetDefinitionDetailKey(key string) bool {
	switch strings.ToLower(key) {
	case "description", "id", "name", "title":
		return true
	default:
		return false
	}
}

func assetDefinitionValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		if data, err := json.MarshalIndent(typed, "", "  "); err == nil {
			return string(data)
		}
		return fmt.Sprint(value)
	}
}

func assetDefinitionField(label, value string) g.Node {
	if strings.TrimSpace(value) == "" {
		value = "-"
	}
	return h.Div(h.Class("grid min-w-0 gap-2 border-b border-outline-muted py-4 last:border-b-0"),
		h.Span(h.Class("text-caption font-900 uppercase leading-none text-fg-muted"), g.Text(label)),
		h.Code(h.Class("min-w-0 max-w-full overflow-auto whitespace-pre-wrap break-words text-caption font-720 leading-snug text-fg-default"), g.Text(value)),
	)
}

func roleBindingRow(workspaceID string, binding api.RoleBindingResponse, csrfToken string) g.Node {
	return h.Div(h.Class("grid grid-cols-role-row items-center gap-3 border-b border-outline-muted py-2 last:border-b-0"),
		h.Div(h.Class("min-w-0"),
			h.P(h.Class("m-0 truncate text-body-sm font-850 text-fg-default"), g.Text(displayLabel(binding.DisplayName, binding.Email))),
			h.P(h.Class("m-0 mt-0.5 truncate text-caption font-650 text-fg-muted"), g.Text(binding.Email)),
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
	return h.Label(h.Class("grid gap-1 text-caption font-850 uppercase text-fg-muted"),
		g.Text(label),
		h.Input(h.Type("text"), h.Name(name), h.Value(value), h.Placeholder(placeholder), h.Class("min-h-control-md rounded-small border border-outline-variant bg-container px-2 text-body-sm font-720 text-fg-default placeholder:text-fg-muted")),
	)
}

func emptyState(message string) g.Node {
	return h.Div(h.Class("rounded-small border border-dashed border-outline-muted bg-container-low px-3 py-4 text-body-sm font-720 text-fg-muted"), g.Text(message))
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
