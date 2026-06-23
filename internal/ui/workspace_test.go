package ui

import (
	"html"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
)

func TestWorkspaceAssetDetailsRenderSharedShapeForSemanticModel(t *testing.T) {
	workspace, catalog, assets, edges := testWorkspaceAssetFixtures()
	asset := assets[0]

	var out strings.Builder
	err := WorkspaceAssetPage(catalog, workspace, asset, assets, edges, "details", "Owner").Render(&out)
	if err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(out.String())

	for _, want := range []string{
		"Breadcrumb",
		"Workspaces",
		"LibreDash Workspace",
		"Olist Commerce",
		"Details",
		"Lineage",
		"Overview",
		"Default connection",
		"Connections (1)",
		"Sources (1)",
		"Model tables (1)",
		"Fields (1)",
		"Measures (1)",
		"Relationships (1)",
		`data-attr:grid="$assetDetailsSemanticConnectionsGrid"`,
		`data-attr:grid="$assetDetailsSemanticSourcesGrid"`,
		`data-attr:grid="$assetDetailsSemanticModelTablesGrid"`,
		`data-attr:grid="$assetDetailsSemanticFieldsGrid"`,
		`data-attr:grid="$assetDetailsSemanticMeasuresGrid"`,
		`data-attr:grid="$assetDetailsSemanticRelationshipsGrid"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("semantic model details did not render %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Published from Git/YAML") {
		t.Fatalf("semantic model details rendered hardcoded publication source:\n%s", rendered)
	}
}

func TestWorkspaceAssetDetailSignalsUseSharedGridShape(t *testing.T) {
	workspace, _, assets, edges := testWorkspaceAssetFixtures()
	byType := map[string]api.AssetResponse{}
	for _, asset := range assets {
		if _, ok := byType[asset.Type]; !ok {
			byType[asset.Type] = asset
		}
	}

	semanticSignals := workspaceAssetSignals(workspace, byType["semantic_model"], assets, edges, assetLineage(workspace.ID, byType["semantic_model"], assets, edges), "details")
	for _, key := range []string{
		"assetDetailsSemanticConnectionsGrid",
		"assetDetailsSemanticSourcesGrid",
		"assetDetailsSemanticModelTablesGrid",
		"assetDetailsSemanticFieldsGrid",
		"assetDetailsSemanticMeasuresGrid",
		"assetDetailsSemanticRelationshipsGrid",
	} {
		if _, ok := semanticSignals[key]; !ok {
			t.Fatalf("semantic model details did not seed %s: %#v", key, semanticSignals)
		}
	}

	dashboardSignals := workspaceAssetSignals(workspace, byType["dashboard"], assets, edges, assetLineage(workspace.ID, byType["dashboard"], assets, edges), "details")
	for _, key := range []string{"assetDetailsPagesGrid", "assetDetailsFiltersGrid", "assetDetailsVisualsGrid", "assetDetailsTablesGrid"} {
		if _, ok := dashboardSignals[key]; !ok {
			t.Fatalf("dashboard details did not seed %s: %#v", key, dashboardSignals)
		}
	}

	lineageSignals := workspaceAssetSignals(workspace, byType["dashboard"], assets, edges, assetLineage(workspace.ID, byType["dashboard"], assets, edges), "lineage")
	for _, key := range []string{"assetLineageGraph", "assetLineageUsesGrid", "assetLineageUsedByGrid"} {
		if _, ok := lineageSignals[key]; !ok {
			t.Fatalf("lineage did not seed %s: %#v", key, lineageSignals)
		}
	}
}

func TestWorkspaceAssetDetailsRenderSharedShapeForLeafAsset(t *testing.T) {
	workspace, catalog, assets, edges := testWorkspaceAssetFixtures()
	var connection api.AssetResponse
	for _, asset := range assets {
		if asset.Type == "connection" {
			connection = asset
			break
		}
	}

	var out strings.Builder
	err := WorkspaceAssetPage(catalog, workspace, connection, assets, edges, "details", "Owner").Render(&out)
	if err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(out.String())

	for _, want := range []string{
		"Breadcrumb",
		"Olist connection",
		"Overview",
		"Type",
		"Connection",
		"Parent",
		"Olist Commerce",
		"Kind",
		"Credentials",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("connection details did not render %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Published from Git/YAML") {
		t.Fatalf("connection details rendered hardcoded publication source:\n%s", rendered)
	}
}

func TestWorkspaceAssetRowsUseDetailLinksForModelAndMetricAssets(t *testing.T) {
	workspace, _, assets, _ := testWorkspaceAssetFixtures()
	byType := map[string]api.AssetResponse{}
	byID := map[string]api.AssetResponse{}
	for _, asset := range assets {
		byID[asset.ID] = asset
		if _, ok := byType[asset.Type]; !ok {
			byType[asset.Type] = asset
		}
	}

	for _, typ := range []string{"semantic_model", "dashboard"} {
		var out strings.Builder
		if err := assetRow(workspace.ID, byType[typ], byID).Render(&out); err != nil {
			t.Fatal(err)
		}
		rendered := html.UnescapeString(out.String())
		if strings.Contains(rendered, `/models`) || strings.Contains(rendered, `/metrics`) {
			t.Fatalf("%s asset row rendered removed legacy link:\n%s", typ, rendered)
		}
		if !strings.Contains(rendered, "/workspaces/libredash/assets/"+byType[typ].ID+"/details") {
			t.Fatalf("%s asset row did not link to canonical details:\n%s", typ, rendered)
		}
	}
}

func TestWorkspaceAssetRowsRenderTokenBackedIconColors(t *testing.T) {
	workspace, catalog, assets, _ := testWorkspaceAssetFixtures()
	visibleAssets := []api.AssetResponse{assets[0], assets[5], assets[6]}

	var out strings.Builder
	err := WorkspacePage(catalog, workspace, visibleAssets, "", "", "Owner", testWorkspaceAccess(workspace, true), "csrf").Render(&out)
	if err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(out.String())

	for _, want := range []string{
		`<th class="px-3 py-2 text-caption font-medium uppercase text-fg-muted" scope="col">Name</th>`,
		`<th class="px-3 py-2 text-caption font-medium uppercase text-fg-muted w-40" scope="col">Type</th>`,
		`<th class="px-3 py-2 text-caption font-medium uppercase text-fg-muted w-56 max-md:hidden" scope="col">Key</th>`,
		`<th class="px-3 py-2 text-caption font-medium uppercase text-fg-muted w-48 max-lg:hidden" scope="col">Parent</th>`,
		"background-color: var(--ld-asset-semantic-model-bg); border-color: var(--ld-asset-semantic-model-border); color: var(--ld-asset-semantic-model-accent)",
		"background-color: var(--ld-asset-measure-bg); border-color: var(--ld-asset-measure-border); color: var(--ld-asset-measure-accent)",
		"background-color: var(--ld-asset-dashboard-bg); border-color: var(--ld-asset-dashboard-border); color: var(--ld-asset-dashboard-accent)",
		`href="/workspaces/libredash/assets/model/details">Olist Commerce</a>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("workspace asset rows did not render token-backed icon style %q:\n%s", want, rendered)
		}
	}
}

func TestWorkspaceAccessControlRendersForManagers(t *testing.T) {
	workspace, catalog, assets, _ := testWorkspaceAssetFixtures()

	var out strings.Builder
	err := WorkspacePage(catalog, workspace, []api.AssetResponse{assets[0]}, "", "", "Owner", testWorkspaceAccess(workspace, true), "csrf").Render(&out)
	if err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(out.String())

	for _, want := range []string{
		`/static/workspace-access-control.js?v=dev`,
		`<ld-workspace-access-control data-attr:access="$workspaceAccess"`,
		`data-attr:search="$workspaceAccess.search"`,
		`data-on:ld-workspace-access-search__debounce.200ms=`,
		`data-on:ld-workspace-access-upsert=`,
		`data-on:ld-workspace-access-remove=`,
		`workspaceAccess`,
		`command`,
		`search`,
		`csrfToken`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("workspace access control did not render %q:\n%s", want, rendered)
		}
	}
	for _, notWant := range []string{
		`workspaceAccessCommand`,
		`workspaceAccessSearch`,
	} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("workspace access control rendered root-level access signal %q:\n%s", notWant, rendered)
		}
	}
}

func TestWorkspaceAccessControlDoesNotRenderForViewers(t *testing.T) {
	workspace, catalog, assets, _ := testWorkspaceAssetFixtures()

	var out strings.Builder
	err := WorkspacePage(catalog, workspace, []api.AssetResponse{assets[0]}, "", "", "Viewer", testWorkspaceAccess(workspace, false), "").Render(&out)
	if err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(out.String())

	for _, notWant := range []string{
		`/static/workspace-access-control.js?v=dev`,
		`<ld-workspace-access-control`,
		`data-on:ld-workspace-access-upsert=`,
	} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("workspace access control rendered for viewer %q:\n%s", notWant, rendered)
		}
	}
}

func testWorkspaceAccess(workspace api.WorkspaceResponse, canManage bool) api.WorkspaceAccessResponse {
	return api.WorkspaceAccessResponse{
		Workspace: workspace,
		Roles: []api.RoleResponse{
			{Name: "viewer"},
			{Name: "editor"},
			{Name: "admin"},
		},
		Bindings: []api.RoleBindingResponse{
			{PrincipalID: "principal_1", WorkspaceID: workspace.ID, Email: "owner@example.com", DisplayName: "Owner", Role: "owner"},
		},
		CanManage: canManage,
	}
}

func testWorkspaceAssetFixtures() (api.WorkspaceResponse, dashboard.Catalog, []api.AssetResponse, []api.AssetEdgeResponse) {
	workspace := api.WorkspaceResponse{ID: "libredash", Title: "LibreDash Workspace", Description: "Local BI workspace."}
	catalog := dashboard.Catalog{Workspace: dashboard.CatalogWorkspace{ID: workspace.ID, Title: workspace.Title, Description: workspace.Description}}
	assets := []api.AssetResponse{
		{
			ID:          "model",
			WorkspaceID: workspace.ID,
			Type:        "semantic_model",
			Key:         "olist",
			Title:       "Olist Commerce",
			Description: "Brazilian ecommerce model.",
			Meta: map[string]any{
				"DefaultConnection": "olist",
				"Connections": map[string]any{
					"olist": map[string]any{"Kind": "local", "credentials_configured": false, "Defaults": map[string]any{"Options": map[string]any{"header": true}}},
				},
				"Sources": map[string]any{
					"orders": map[string]any{"Connection": "olist", "Format": "csv", "Path": "orders.csv"},
				},
				"Tables": map[string]any{
					"orders": map[string]any{
						"Source":      "orders",
						"PrimaryKey":  "order_id",
						"Description": "One row per order.",
						"Dimensions": map[string]any{
							"state": map[string]any{"Expr": "state"},
						},
					},
				},
				"Measures": map[string]any{
					"revenue": map[string]any{"Table": "orders", "Expression": "SUM(orders.revenue)", "Format": "currency"},
				},
				"Relationships": []any{map[string]any{"ID": "orders_customers", "From": "orders.customer_id", "To": "customers.customer_id", "Cardinality": "many_to_one", "Active": true}},
			},
		},
		{ID: "connection", WorkspaceID: workspace.ID, Type: "connection", Key: "olist.olist", ParentID: "model", Title: "Olist connection", Meta: map[string]any{"Kind": "local", "credentials_configured": false}},
		{ID: "source", WorkspaceID: workspace.ID, Type: "source", Key: "olist.orders", ParentID: "model", Title: "orders", Meta: map[string]any{"Connection": "olist", "Format": "csv", "Path": "orders.csv"}},
		{ID: "table-model", WorkspaceID: workspace.ID, Type: "model_table", Key: "olist.orders", ParentID: "model", Title: "orders", Meta: map[string]any{"Source": "orders", "PrimaryKey": "order_id"}},
		{ID: "field", WorkspaceID: workspace.ID, Type: "field", Key: "olist.orders.state", ParentID: "table-model", Title: "State", Meta: map[string]any{"Expr": "state"}},
		{ID: "measure", WorkspaceID: workspace.ID, Type: "measure", Key: "olist.revenue", ParentID: "model", Title: "Revenue", Meta: map[string]any{"Table": "orders", "Expression": "SUM(orders.revenue)", "Format": "currency"}},
		{ID: "dashboard", WorkspaceID: workspace.ID, Type: "dashboard", Key: "executive-sales", Title: "Executive Sales Dashboard", Description: "Sales overview.", Href: "/dashboards/executive-sales", Meta: map[string]any{"SemanticModel": "olist", "Tags": []any{"sales"}}},
		{ID: "page", WorkspaceID: workspace.ID, Type: "page", Key: "executive-sales.overview", ParentID: "dashboard", Title: "Overview"},
		{ID: "filter", WorkspaceID: workspace.ID, Type: "filter", Key: "executive-sales.state", ParentID: "dashboard", Title: "State", Meta: map[string]any{"Field": "orders.state", "Type": "multi_select"}},
		{ID: "visual", WorkspaceID: workspace.ID, Type: "visual", Key: "executive-sales.revenue", ParentID: "dashboard", Title: "Revenue by month", Meta: map[string]any{"Type": "line"}},
		{ID: "table", WorkspaceID: workspace.ID, Type: "table", Key: "executive-sales.orders", ParentID: "dashboard", Title: "Orders", Meta: map[string]any{"Table": "orders"}},
	}
	edges := []api.AssetEdgeResponse{
		{ID: "model-table", FromAssetID: "model", ToAssetID: "table-model", Type: "contains"},
		{ID: "model-measure", FromAssetID: "model", ToAssetID: "measure", Type: "contains"},
		{ID: "table-field", FromAssetID: "table-model", ToAssetID: "field", Type: "contains"},
		{ID: "table-source", FromAssetID: "table-model", ToAssetID: "source", Type: "reads_source"},
		{ID: "source-connection", FromAssetID: "source", ToAssetID: "connection", Type: "uses_connection"},
		{ID: "dashboard-model", FromAssetID: "dashboard", ToAssetID: "model", Type: "uses_semantic_model"},
		{ID: "dashboard-table", FromAssetID: "dashboard", ToAssetID: "table-model", Type: "uses_model_table"},
		{ID: "dashboard-measure", FromAssetID: "dashboard", ToAssetID: "measure", Type: "uses_measure"},
	}
	return workspace, catalog, assets, edges
}
