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
		"Cache tables (1)",
		"Datasets (1)",
		"Relationships (1)",
		`data-attr:grid="$assetDetailsSemanticConnectionsGrid"`,
		`data-attr:grid="$assetDetailsSemanticSourcesGrid"`,
		`data-attr:grid="$assetDetailsSemanticCacheTablesGrid"`,
		`data-attr:grid="$assetDetailsSemanticDatasetsGrid"`,
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
		"assetDetailsSemanticCacheTablesGrid",
		"assetDetailsSemanticDatasetsGrid",
		"assetDetailsSemanticRelationshipsGrid",
	} {
		if _, ok := semanticSignals[key]; !ok {
			t.Fatalf("semantic model details did not seed %s: %#v", key, semanticSignals)
		}
	}

	metricSignals := workspaceAssetSignals(workspace, byType["metric_view"], assets, edges, assetLineage(workspace.ID, byType["metric_view"], assets, edges), "details")
	for _, key := range []string{"assetDetailsMeasuresGrid", "assetDetailsDimensionsGrid"} {
		if _, ok := metricSignals[key]; !ok {
			t.Fatalf("metric view details did not seed %s: %#v", key, metricSignals)
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

func TestWorkspaceAssetDetailsRenderSharedShapeForMetricView(t *testing.T) {
	workspace, catalog, assets, edges := testWorkspaceAssetFixtures()
	var metric api.AssetResponse
	for _, asset := range assets {
		if asset.Type == "metric_view" {
			metric = asset
			break
		}
	}

	var out strings.Builder
	err := WorkspaceAssetPage(catalog, workspace, metric, assets, edges, "details", "Owner").Render(&out)
	if err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(out.String())

	for _, want := range []string{
		"Breadcrumb",
		"Orders Metrics",
		"Overview",
		"Dataset",
		"orders",
		"Timeseries",
		"purchase_timestamp",
		"Measures (1)",
		"Dimensions (1)",
		`data-attr:grid="$assetDetailsMeasuresGrid"`,
		`data-attr:grid="$assetDetailsDimensionsGrid"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("metric view details did not render %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `/metrics`) || strings.Contains(rendered, `/models`) {
		t.Fatalf("metric view details rendered removed legacy link:\n%s", rendered)
	}
}

func TestWorkspaceAssetRowsUseDetailLinksForModelAndMetricAssets(t *testing.T) {
	workspace, _, assets, _ := testWorkspaceAssetFixtures()
	byType := map[string]api.AssetResponse{}
	for _, asset := range assets {
		if _, ok := byType[asset.Type]; !ok {
			byType[asset.Type] = asset
		}
	}

	for _, typ := range []string{"semantic_model", "metric_view"} {
		var out strings.Builder
		if err := assetRow(workspace.ID, byType[typ]).Render(&out); err != nil {
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
				"Cache": map[string]any{
					"Tables": map[string]any{"orders_enriched": map[string]any{"Description": "One row per order.", "SQL": "select * from raw.orders"}},
				},
				"Datasets": map[string]any{
					"orders": map[string]any{"Source": "orders_enriched"},
				},
				"Relationships": []any{map[string]any{"ID": "orders_customers", "From": "raw.orders.customer_id", "To": "raw.customers.customer_id", "Cardinality": "many_to_one", "Active": true}},
			},
		},
		{ID: "connection", WorkspaceID: workspace.ID, Type: "connection", Key: "olist.olist", ParentID: "model", Title: "Olist connection", Meta: map[string]any{"Kind": "local", "credentials_configured": false}},
		{ID: "source", WorkspaceID: workspace.ID, Type: "source", Key: "olist.orders", ParentID: "model", Title: "orders", Meta: map[string]any{"Connection": "olist", "Format": "csv", "Path": "orders.csv"}},
		{ID: "cache", WorkspaceID: workspace.ID, Type: "cache_table", Key: "olist.orders_enriched", ParentID: "model", Title: "orders_enriched"},
		{ID: "dataset", WorkspaceID: workspace.ID, Type: "dataset", Key: "olist.orders", ParentID: "model", Title: "orders"},
		{ID: "metric", WorkspaceID: workspace.ID, Type: "metric_view", Key: "orders", ParentID: "model", Title: "Orders Metrics", Description: "Order metrics.", Meta: map[string]any{"Dataset": "orders", "Timeseries": "purchase_timestamp"}},
		{ID: "measure", WorkspaceID: workspace.ID, Type: "measure", Key: "orders.revenue", ParentID: "metric", Title: "Revenue", Meta: map[string]any{"Expression": "SUM(revenue)", "Format": "currency"}},
		{ID: "dimension", WorkspaceID: workspace.ID, Type: "dimension", Key: "orders.state", ParentID: "metric", Title: "State", Meta: map[string]any{"Expr": "customer_state"}},
		{ID: "dashboard", WorkspaceID: workspace.ID, Type: "dashboard", Key: "executive-sales", Title: "Executive Sales Dashboard", Description: "Sales overview.", Href: "/dashboards/executive-sales", Meta: map[string]any{"MetricViews": []any{"orders"}, "Tags": []any{"sales"}}},
		{ID: "page", WorkspaceID: workspace.ID, Type: "page", Key: "executive-sales.overview", ParentID: "dashboard", Title: "Overview"},
		{ID: "filter", WorkspaceID: workspace.ID, Type: "filter", Key: "executive-sales.state", ParentID: "dashboard", Title: "State", Meta: map[string]any{"MetricView": "orders", "Dimension": "state", "Type": "multi_select"}},
		{ID: "visual", WorkspaceID: workspace.ID, Type: "visual", Key: "executive-sales.revenue", ParentID: "dashboard", Title: "Revenue by month", Meta: map[string]any{"MetricView": "orders", "Type": "line"}},
		{ID: "table", WorkspaceID: workspace.ID, Type: "table", Key: "executive-sales.orders", ParentID: "dashboard", Title: "Orders", Meta: map[string]any{"MetricView": "orders"}},
	}
	edges := []api.AssetEdgeResponse{
		{ID: "model-metric", FromAssetID: "model", ToAssetID: "metric", Type: "contains"},
		{ID: "metric-dataset", FromAssetID: "metric", ToAssetID: "dataset", Type: "uses_dataset"},
		{ID: "dataset-cache", FromAssetID: "dataset", ToAssetID: "cache", Type: "uses_cache_table"},
		{ID: "cache-source", FromAssetID: "cache", ToAssetID: "source", Type: "reads_source"},
		{ID: "source-connection", FromAssetID: "source", ToAssetID: "connection", Type: "uses_connection"},
		{ID: "dashboard-metric", FromAssetID: "dashboard", ToAssetID: "metric", Type: "uses_metric_view"},
	}
	return workspace, catalog, assets, edges
}
