package ui

import (
	"fmt"
	"html"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
)

func TestWorkspaceAssetDetailsRenderSharedShapeForSemanticModel(t *testing.T) {
	workspace, catalog, assets, edges := testWorkspaceAssetFixtures()
	asset := testAssetByID(t, assets, "model")

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

func TestAssetLineageProjectsRecursiveDependenciesAndContext(t *testing.T) {
	workspace, _, assets, edges := testWorkspaceAssetFixtures()
	asset := testAssetByID(t, assets, "page-item")

	lineage := assetLineage(workspace.ID, asset, assets, edges)

	assertLineageNodeKindsExact(t, lineage.Graph, []string{
		"connection",
		"measure",
		"model_table",
		"dashboard",
		"semantic_model",
		"source",
	})
	assertLineageEdgeKinds(t, lineage.Graph, []string{
		"lineage_connection_source",
		"lineage_source_model_table",
		"lineage_model_table_semantic_model",
		"lineage_semantic_model_measure",
		"lineage_measure_dashboard",
	})
	assertLineageEdgesMoveLeftToRight(t, lineage.Graph)
	assertLineageSelectedNode(t, lineage.Graph, "dashboard")
	assertGridRelations(t, lineage.Uses, []string{
		"Reads source",
		"Uses connection",
		"Uses field",
		"Uses filter",
		"Uses measure",
		"Uses model table",
		"Uses semantic table",
		"Uses table",
		"Uses visual",
	})
	assertGridRelations(t, lineage.UsedBy, nil)
	if gridHasRelation(lineage.Uses, "Contains") || gridHasRelation(lineage.UsedBy, "Contains") {
		t.Fatalf("dependency grids included contains edges: uses=%#v usedBy=%#v", lineage.Uses.Rows, lineage.UsedBy.Rows)
	}
	if lineage.Count != 10 {
		t.Fatalf("lineage count included context edges, got %d", lineage.Count)
	}
}

func TestAssetLineageProjectsRecursiveConsumers(t *testing.T) {
	workspace, _, assets, edges := testWorkspaceAssetFixtures()
	asset := testAssetByID(t, assets, "semantic-table")

	lineage := assetLineage(workspace.ID, asset, assets, edges)

	assertLineageNodeKindsExact(t, lineage.Graph, []string{
		"connection",
		"dashboard",
		"measure",
		"model_table",
		"semantic_model",
		"source",
	})
	assertLineageEdgesMoveLeftToRight(t, lineage.Graph)
	assertLineageSelectedNode(t, lineage.Graph, "semantic_model")
	assertGridRelations(t, lineage.Uses, []string{
		"Reads source",
		"Uses connection",
		"Uses model table",
	})
	assertGridRelations(t, lineage.UsedBy, []string{
		"Uses semantic table",
		"Uses table",
		"Uses visual",
	})
	if gridHasRelation(lineage.Uses, "Contains") || gridHasRelation(lineage.UsedBy, "Contains") {
		t.Fatalf("consumer/dependency grids included contains edges: uses=%#v usedBy=%#v", lineage.Uses.Rows, lineage.UsedBy.Rows)
	}
}

func TestAssetLineageDashboardDerivesMeasureConsumers(t *testing.T) {
	workspace, _, assets, edges := testWorkspaceAssetFixtures()
	asset := testAssetByID(t, assets, "dashboard")

	lineage := assetLineage(workspace.ID, asset, assets, edges)

	assertLineageNodeKindsExact(t, lineage.Graph, []string{
		"connection",
		"dashboard",
		"measure",
		"model_table",
		"semantic_model",
		"source",
	})
	assertLineageEdgeKinds(t, lineage.Graph, []string{"lineage_measure_dashboard"})
	assertLineageEdgesMoveLeftToRight(t, lineage.Graph)
	for _, edge := range lineage.Graph.Edges {
		if edge.Source == "model" && edge.Target == "dashboard" {
			t.Fatalf("semantic model dashboard shortcut should be suppressed when measure usage exists: %#v", lineage.Graph.Edges)
		}
	}
	node := assertLineageNode(t, lineage.Graph, "dashboard")
	if node.VisibleUpstream != 1 || node.VisibleDownstream != 0 {
		t.Fatalf("dashboard visible counts = upstream %d downstream %d, want 1/0: %#v", node.VisibleUpstream, node.VisibleDownstream, node)
	}
	if node.UsesCount != 1 || node.UsedByCount != 0 {
		t.Fatalf("dashboard full-fidelity counts = uses %d usedBy %d, want 1/0: %#v", node.UsesCount, node.UsedByCount, node)
	}
	if node.ContainedCount != 4 || node.ContainedSummary != "1 filter, 1 page, 1 table, 1 visual" {
		t.Fatalf("dashboard contained summary = %d %q, want 4 dashboard children: %#v", node.ContainedCount, node.ContainedSummary, node)
	}
}

func TestAssetLineageSemanticModelDerivesMeasureDashboardPath(t *testing.T) {
	workspace, _, assets, edges := testWorkspaceAssetFixtures()
	asset := testAssetByID(t, assets, "model")

	lineage := assetLineage(workspace.ID, asset, assets, edges)

	assertLineageNodeKindsExact(t, lineage.Graph, []string{
		"connection",
		"dashboard",
		"measure",
		"model_table",
		"semantic_model",
		"source",
	})
	assertLineageEdgeKinds(t, lineage.Graph, []string{"lineage_measure_dashboard"})
	assertLineageEdgesMoveLeftToRight(t, lineage.Graph)
	for _, edge := range lineage.Graph.Edges {
		if edge.Source == "model" && edge.Target == "dashboard" {
			t.Fatalf("semantic model dashboard shortcut should be suppressed when measure usage exists: %#v", lineage.Graph.Edges)
		}
	}
	node := assertLineageNode(t, lineage.Graph, "model")
	if node.VisibleUpstream != 1 || node.VisibleDownstream != 1 {
		t.Fatalf("semantic model visible counts = upstream %d downstream %d, want 1/1: %#v", node.VisibleUpstream, node.VisibleDownstream, node)
	}
	if node.UsesCount != 0 || node.UsedByCount != 1 {
		t.Fatalf("semantic model full-fidelity counts = uses %d usedBy %d, want 0/1: %#v", node.UsesCount, node.UsedByCount, node)
	}
	if node.ContainedCount != 3 || node.ContainedSummary != "1 measure, 1 relationship, 1 semantic table" {
		t.Fatalf("semantic model contained summary = %d %q, want 3 semantic children: %#v", node.ContainedCount, node.ContainedSummary, node)
	}
}

func TestAssetLineageFallsBackToContainsWhenNoDependenciesExist(t *testing.T) {
	workspace, _, assets, edges := testWorkspaceAssetFixtures()
	asset := testAssetByID(t, assets, "catalog")

	lineage := assetLineage(workspace.ID, asset, assets, edges)

	assertLineageEdgeKinds(t, lineage.Graph, []string{"contains"})
	assertGridRelations(t, lineage.Uses, []string{"Contains"})
	assertGridRelations(t, lineage.UsedBy, nil)
	if lineage.Count != 5 {
		t.Fatalf("contains fallback should count direct hierarchy context, got %d", lineage.Count)
	}
}

func TestLineageProjectionPolicy(t *testing.T) {
	layerTests := []struct {
		typ     string
		want    int
		visible bool
	}{
		{typ: "connection", want: 0, visible: true},
		{typ: "source", want: 1, visible: true},
		{typ: "model_table", want: 2, visible: true},
		{typ: "semantic_model", want: 3, visible: true},
		{typ: "measure", want: 4, visible: true},
		{typ: "dashboard", want: 5, visible: true},
		{typ: "field", want: -1, visible: false},
	}
	for _, tt := range layerTests {
		t.Run(tt.typ, func(t *testing.T) {
			if got := lineageVisualLayer(tt.typ); got != tt.want {
				t.Fatalf("lineageVisualLayer(%q) = %d, want %d", tt.typ, got, tt.want)
			}
			if got := isLineageVisibleGraphAsset(tt.typ); got != tt.visible {
				t.Fatalf("isLineageVisibleGraphAsset(%q) = %v, want %v", tt.typ, got, tt.visible)
			}
		})
	}

	edgeTests := []struct {
		name       string
		sourceType string
		targetType string
		fallback   string
		wantKind   string
		wantLabel  string
	}{
		{
			name:       "connection source",
			sourceType: "connection",
			targetType: "source",
			fallback:   "uses_connection",
			wantKind:   "lineage_connection_source",
			wantLabel:  "Provides source",
		},
		{
			name:       "source model table",
			sourceType: "source",
			targetType: "model_table",
			fallback:   "reads_source",
			wantKind:   "lineage_source_model_table",
			wantLabel:  "Feeds model table",
		},
		{
			name:       "model table semantic model",
			sourceType: "model_table",
			targetType: "semantic_model",
			fallback:   "uses_model_table",
			wantKind:   "lineage_model_table_semantic_model",
			wantLabel:  "Feeds semantic model",
		},
		{
			name:       "semantic model measure",
			sourceType: "semantic_model",
			targetType: "measure",
			fallback:   "uses_semantic_table",
			wantKind:   "lineage_semantic_model_measure",
			wantLabel:  "Defines measure",
		},
		{
			name:       "measure dashboard",
			sourceType: "measure",
			targetType: "dashboard",
			fallback:   "uses_measure",
			wantKind:   "lineage_measure_dashboard",
			wantLabel:  "Used in dashboard",
		},
		{
			name:       "semantic model dashboard",
			sourceType: "semantic_model",
			targetType: "dashboard",
			fallback:   "uses_semantic_model",
			wantKind:   "lineage_semantic_model_dashboard",
			wantLabel:  "Powers dashboard",
		},
		{
			name:       "fallback",
			sourceType: "field",
			targetType: "dashboard",
			fallback:   "filters_field",
			wantKind:   "filters_field",
			wantLabel:  "Filters field",
		},
	}
	for _, tt := range edgeTests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lineageCollapsedEdgeKind(tt.sourceType, tt.targetType, tt.fallback); got != tt.wantKind {
				t.Fatalf("lineageCollapsedEdgeKind(%q, %q, %q) = %q, want %q", tt.sourceType, tt.targetType, tt.fallback, got, tt.wantKind)
			}
			if got := lineageCollapsedEdgeLabel(tt.sourceType, tt.targetType, tt.fallback); got != tt.wantLabel {
				t.Fatalf("lineageCollapsedEdgeLabel(%q, %q, %q) = %q, want %q", tt.sourceType, tt.targetType, tt.fallback, got, tt.wantLabel)
			}
		})
	}
}

func TestCollapsedAssetLineageSuppressesSemanticDashboardShortcutByPair(t *testing.T) {
	workspaceID := "libredash"
	assets := map[string]api.AssetResponse{
		"model-a": {
			ID:          "model-a",
			WorkspaceID: workspaceID,
			Type:        "semantic_model",
			Key:         "model_a",
			Title:       "Model A",
		},
		"measure-a": {
			ID:          "measure-a",
			WorkspaceID: workspaceID,
			Type:        "measure",
			Key:         "model_a.revenue",
			ParentID:    "model-a",
			Title:       "Revenue",
		},
		"dashboard-a": {
			ID:          "dashboard-a",
			WorkspaceID: workspaceID,
			Type:        "dashboard",
			Key:         "dashboard-a",
			Title:       "Dashboard A",
		},
		"model-b": {
			ID:          "model-b",
			WorkspaceID: workspaceID,
			Type:        "semantic_model",
			Key:         "model_b",
			Title:       "Model B",
		},
		"dashboard-b": {
			ID:          "dashboard-b",
			WorkspaceID: workspaceID,
			Type:        "dashboard",
			Key:         "dashboard-b",
			Title:       "Dashboard B",
		},
	}
	graph := assetLineageGraph{
		Nodes: []assetLineageNode{
			{ID: "model-a"},
			{ID: "measure-a"},
			{ID: "dashboard-a"},
			{ID: "model-b"},
			{ID: "dashboard-b"},
		},
		Edges: []assetLineageEdge{
			{ID: "dashboard-a-measure-a", Source: "dashboard-a", Target: "measure-a", Kind: "uses_measure"},
			{ID: "dashboard-a-model-a", Source: "dashboard-a", Target: "model-a", Kind: "uses_semantic_model"},
			{ID: "dashboard-b-model-b", Source: "dashboard-b", Target: "model-b", Kind: "uses_semantic_model"},
		},
	}

	lineage := collapsedAssetLineageGraph(workspaceID, assets["dashboard-b"], graph, assets)

	assertLineageHasEdge(t, lineage, "measure-a", "dashboard-a", "lineage_measure_dashboard")
	assertLineageMissingEdge(t, lineage, "model-a", "dashboard-a", "lineage_semantic_model_dashboard")
	assertLineageHasEdge(t, lineage, "model-b", "dashboard-b", "lineage_semantic_model_dashboard")
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
		"LibreDash Workspace",
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
	visibleAssets := []api.AssetResponse{
		testAssetByID(t, assets, "model"),
		testAssetByID(t, assets, "measure"),
		testAssetByID(t, assets, "dashboard"),
	}

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

func testAssetByID(t *testing.T, assets []api.AssetResponse, id string) api.AssetResponse {
	t.Helper()
	for _, asset := range assets {
		if asset.ID == id {
			return asset
		}
	}
	t.Fatalf("asset %q not found", id)
	return api.AssetResponse{}
}

func assertLineageNodeKinds(t *testing.T, graph assetLineageGraph, expected []string) {
	t.Helper()
	got := map[string]struct{}{}
	for _, node := range graph.Nodes {
		got[node.Kind] = struct{}{}
	}
	for _, kind := range expected {
		if _, ok := got[kind]; !ok {
			t.Fatalf("lineage graph missing node kind %q: %#v", kind, graph.Nodes)
		}
	}
}

func assertLineageNodeKindsExact(t *testing.T, graph assetLineageGraph, expected []string) {
	t.Helper()
	got := map[string]int{}
	for _, node := range graph.Nodes {
		got[node.Kind]++
	}
	if len(got) != len(expected) {
		t.Fatalf("lineage graph node kinds = %#v, want exactly %#v; nodes=%#v", got, expected, graph.Nodes)
	}
	for _, kind := range expected {
		if got[kind] == 0 {
			t.Fatalf("lineage graph missing node kind %q: %#v", kind, graph.Nodes)
		}
	}
}

func assertLineageEdgeKinds(t *testing.T, graph assetLineageGraph, expected []string) {
	t.Helper()
	got := map[string]struct{}{}
	for _, edge := range graph.Edges {
		got[edge.Kind] = struct{}{}
	}
	for _, kind := range expected {
		if _, ok := got[kind]; !ok {
			t.Fatalf("lineage graph missing edge kind %q: %#v", kind, graph.Edges)
		}
	}
}

func assertLineageEdgesMoveLeftToRight(t *testing.T, graph assetLineageGraph) {
	t.Helper()
	ranks := map[string]int{}
	for _, node := range graph.Nodes {
		ranks[node.ID] = node.Rank
	}
	for _, edge := range graph.Edges {
		if ranks[edge.Source] >= ranks[edge.Target] {
			t.Fatalf("lineage edge does not move left-to-right: edge=%#v ranks=%#v nodes=%#v", edge, ranks, graph.Nodes)
		}
	}
}

func assertLineageSelectedNode(t *testing.T, graph assetLineageGraph, wantKind string) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.Selected {
			if node.Kind != wantKind {
				t.Fatalf("selected lineage node kind = %q, want %q: %#v", node.Kind, wantKind, graph.Nodes)
			}
			return
		}
	}
	t.Fatalf("lineage graph has no selected node: %#v", graph.Nodes)
}

func assertLineageNode(t *testing.T, graph assetLineageGraph, id string) assetLineageNode {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("lineage graph missing node %q: %#v", id, graph.Nodes)
	return assetLineageNode{}
}

func assertLineageHasEdge(t *testing.T, graph assetLineageGraph, source, target, kind string) {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.Source == source && edge.Target == target && edge.Kind == kind {
			return
		}
	}
	t.Fatalf("lineage graph missing edge %s -> %s (%s): %#v", source, target, kind, graph.Edges)
}

func assertLineageMissingEdge(t *testing.T, graph assetLineageGraph, source, target, kind string) {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.Source == source && edge.Target == target && edge.Kind == kind {
			t.Fatalf("lineage graph included unwanted edge %s -> %s (%s): %#v", source, target, kind, graph.Edges)
		}
	}
}

func assertGridRelations(t *testing.T, grid metricGrid, expected []string) {
	t.Helper()
	if len(expected) == 0 {
		if len(grid.Rows) != 0 {
			t.Fatalf("expected no relations, got rows %#v", grid.Rows)
		}
		return
	}
	got := map[string]struct{}{}
	for _, row := range grid.Rows {
		got[fmt.Sprint(row["relation"])] = struct{}{}
	}
	for _, relation := range expected {
		if _, ok := got[relation]; !ok {
			t.Fatalf("grid missing relation %q: %#v", relation, grid.Rows)
		}
	}
}

func gridHasRelation(grid metricGrid, relation string) bool {
	for _, row := range grid.Rows {
		if fmt.Sprint(row["relation"]) == relation {
			return true
		}
	}
	return false
}

func testWorkspaceAssetFixtures() (api.WorkspaceResponse, dashboard.Catalog, []api.AssetResponse, []api.AssetEdgeResponse) {
	workspace := api.WorkspaceResponse{ID: "libredash", Title: "LibreDash Workspace", Description: "Local BI workspace."}
	catalog := dashboard.Catalog{Workspace: dashboard.CatalogWorkspace{ID: workspace.ID, Title: workspace.Title, Description: workspace.Description}}
	assets := []api.AssetResponse{
		{ID: "catalog", WorkspaceID: workspace.ID, Type: "catalog", Key: workspace.ID, Title: workspace.Title, Description: workspace.Description},
		{
			ID:          "model",
			WorkspaceID: workspace.ID,
			Type:        "semantic_model",
			Key:         "olist",
			ParentID:    "catalog",
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
		{ID: "connection", WorkspaceID: workspace.ID, Type: "connection", Key: "olist.olist", ParentID: "catalog", Title: "Olist connection", Meta: map[string]any{"Kind": "local", "credentials_configured": false}},
		{ID: "source", WorkspaceID: workspace.ID, Type: "source", Key: "olist.orders", ParentID: "catalog", Title: "orders", Meta: map[string]any{"Connection": "olist", "Format": "csv", "Path": "orders.csv"}},
		{ID: "table-model", WorkspaceID: workspace.ID, Type: "model_table", Key: "olist.orders", ParentID: "catalog", Title: "orders", Meta: map[string]any{"Source": "orders", "PrimaryKey": "order_id"}},
		{ID: "semantic-table", WorkspaceID: workspace.ID, Type: "semantic_table", Key: "olist.orders", ParentID: "model", Title: "Orders semantic table", Meta: map[string]any{"Table": "orders"}},
		{ID: "field", WorkspaceID: workspace.ID, Type: "field", Key: "olist.orders.state", ParentID: "semantic-table", Title: "State", Meta: map[string]any{"Expr": "state"}},
		{ID: "measure", WorkspaceID: workspace.ID, Type: "measure", Key: "olist.revenue", ParentID: "model", Title: "Revenue", Meta: map[string]any{"Table": "orders", "Expression": "SUM(orders.revenue)", "Format": "currency"}},
		{ID: "relationship", WorkspaceID: workspace.ID, Type: "relationship", Key: "olist.orders_customers", ParentID: "model", Title: "Orders to customers", Meta: map[string]any{"From": "orders.customer_id", "To": "customers.customer_id"}},
		{ID: "dashboard", WorkspaceID: workspace.ID, Type: "dashboard", Key: "executive-sales", ParentID: "catalog", Title: "Executive Sales Dashboard", Description: "Sales overview.", Href: "/dashboards/executive-sales", Meta: map[string]any{"SemanticModel": "olist", "Tags": []any{"sales"}}},
		{ID: "page", WorkspaceID: workspace.ID, Type: "page", Key: "executive-sales.overview", ParentID: "dashboard", Title: "Overview"},
		{ID: "page-item", WorkspaceID: workspace.ID, Type: "page_item", Key: "executive-sales.overview.revenue", ParentID: "page", Title: "Revenue tile"},
		{ID: "filter", WorkspaceID: workspace.ID, Type: "filter", Key: "executive-sales.state", ParentID: "dashboard", Title: "State", Meta: map[string]any{"Field": "orders.state", "Type": "multi_select"}},
		{ID: "visual", WorkspaceID: workspace.ID, Type: "visual", Key: "executive-sales.revenue", ParentID: "dashboard", Title: "Revenue by month", Meta: map[string]any{"Type": "line"}},
		{ID: "table", WorkspaceID: workspace.ID, Type: "table", Key: "executive-sales.orders", ParentID: "dashboard", Title: "Orders", Meta: map[string]any{"Table": "orders"}},
	}
	edges := []api.AssetEdgeResponse{
		{ID: "catalog-model", FromAssetID: "catalog", ToAssetID: "model", Type: "contains"},
		{ID: "catalog-connection", FromAssetID: "catalog", ToAssetID: "connection", Type: "contains"},
		{ID: "catalog-source", FromAssetID: "catalog", ToAssetID: "source", Type: "contains"},
		{ID: "catalog-model-table", FromAssetID: "catalog", ToAssetID: "table-model", Type: "contains"},
		{ID: "catalog-dashboard", FromAssetID: "catalog", ToAssetID: "dashboard", Type: "contains"},
		{ID: "model-semantic-table", FromAssetID: "model", ToAssetID: "semantic-table", Type: "contains"},
		{ID: "model-measure", FromAssetID: "model", ToAssetID: "measure", Type: "contains"},
		{ID: "model-relationship", FromAssetID: "model", ToAssetID: "relationship", Type: "contains"},
		{ID: "semantic-table-field", FromAssetID: "semantic-table", ToAssetID: "field", Type: "contains"},
		{ID: "table-source", FromAssetID: "table-model", ToAssetID: "source", Type: "reads_source"},
		{ID: "source-connection", FromAssetID: "source", ToAssetID: "connection", Type: "uses_connection"},
		{ID: "semantic-table-model-table", FromAssetID: "semantic-table", ToAssetID: "table-model", Type: "uses_model_table"},
		{ID: "measure-semantic-table", FromAssetID: "measure", ToAssetID: "semantic-table", Type: "uses_semantic_table"},
		{ID: "measure-field", FromAssetID: "measure", ToAssetID: "field", Type: "uses_field"},
		{ID: "dashboard-model", FromAssetID: "dashboard", ToAssetID: "model", Type: "uses_semantic_model"},
		{ID: "dashboard-page", FromAssetID: "dashboard", ToAssetID: "page", Type: "contains"},
		{ID: "dashboard-filter", FromAssetID: "dashboard", ToAssetID: "filter", Type: "contains"},
		{ID: "dashboard-visual", FromAssetID: "dashboard", ToAssetID: "visual", Type: "contains"},
		{ID: "dashboard-table", FromAssetID: "dashboard", ToAssetID: "table", Type: "contains"},
		{ID: "page-item-edge", FromAssetID: "page", ToAssetID: "page-item", Type: "contains"},
		{ID: "page-item-visual", FromAssetID: "page-item", ToAssetID: "visual", Type: "uses_visual"},
		{ID: "page-item-table", FromAssetID: "page-item", ToAssetID: "table", Type: "uses_table"},
		{ID: "page-item-filter", FromAssetID: "page-item", ToAssetID: "filter", Type: "uses_filter"},
		{ID: "visual-measure", FromAssetID: "visual", ToAssetID: "measure", Type: "uses_measure"},
		{ID: "visual-field", FromAssetID: "visual", ToAssetID: "field", Type: "uses_field"},
		{ID: "table-semantic-table", FromAssetID: "table", ToAssetID: "semantic-table", Type: "uses_semantic_table"},
		{ID: "table-field", FromAssetID: "table", ToAssetID: "field", Type: "uses_field"},
		{ID: "filter-field", FromAssetID: "filter", ToAssetID: "field", Type: "filters_field"},
	}
	return workspace, catalog, assets, edges
}
