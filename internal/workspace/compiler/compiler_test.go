package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestCompileOlistWorkspace(t *testing.T) {
	compiled, err := Compile(filepath.Join("..", "..", "..", "dashboards", "catalog.yaml"), Options{
		WorkspaceID:  "libredash",
		DeploymentID: "dep_test",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v, want Olist workspace to compile", err)
	}
	if compiled.Definition == nil {
		t.Fatal("Compile() returned nil workspace definition")
	}
	if compiled.Workspace.ID != "libredash" {
		t.Fatalf("workspace id = %q, want libredash", compiled.Workspace.ID)
	}
	if len(compiled.Workspace.Graph.Assets) == 0 {
		t.Fatal("expected compiled asset graph")
	}
}

func TestCompileRejectsBadSemanticReferences(t *testing.T) {
	catalogPath := writeCompilerWorkspace(t, `
id: sales
title: Sales
semantic_model: olist
filters:
  missing:
    type: multi_select
    label: Missing
    url_param: missing
    operator: in
    field: orders.missing
visuals:
  revenue:
    kind: kpi
    query:
      measures:
        revenue:
tables: {}
pages:
  - id: overview
    title: Overview
    visuals: []
`)

	_, err := Compile(catalogPath, Options{WorkspaceID: "libredash", DeploymentID: "dep_test"})
	if err == nil {
		t.Fatal("Compile() error = nil, want bad semantic reference failure")
	}
	if !strings.Contains(err.Error(), "unknown dimension") {
		t.Fatalf("Compile() error = %v, want unknown dimension failure", err)
	}
}

func TestCompileRejectsLegacyVocabulary(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantText string
	}{
		{
			name: "metric_views",
			files: map[string]string{
				"catalog.yaml": `
workspace:
  id: libredash
  title: LibreDash Workspace
metric_views: []
semantic_models:
  - id: olist
    title: Olist
    path: model.yaml
dashboards:
  - id: sales
    title: Sales
    path: dashboard.yaml
`,
				"model.yaml":     validCompilerModelYAML(),
				"dashboard.yaml": validCompilerDashboardYAML(),
			},
			wantText: "metric views",
		},
		{
			name: "dataset",
			files: map[string]string{
				"catalog.yaml":   validCompilerCatalogYAML(),
				"model.yaml":     validCompilerModelYAML() + "\ndatasets: {}\n",
				"dashboard.yaml": validCompilerDashboardYAML(),
			},
			wantText: "datasets",
		},
		{
			name: "cache_table",
			files: map[string]string{
				"catalog.yaml":   validCompilerCatalogYAML(),
				"model.yaml":     validCompilerModelYAML() + "\ncache_tables: {}\n",
				"dashboard.yaml": validCompilerDashboardYAML(),
			},
			wantText: "cache_tables",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				writeCompilerFixture(t, filepath.Join(dir, name), content)
			}
			_, err := Compile(filepath.Join(dir, "catalog.yaml"), Options{WorkspaceID: "libredash", DeploymentID: "dep_test"})
			if err == nil {
				t.Fatal("Compile() error = nil, want legacy vocabulary rejection")
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("Compile() error = %v, want text %q", err, tt.wantText)
			}
		})
	}
}

func TestCompileLineageAssetTypes(t *testing.T) {
	compiled, err := Compile(filepath.Join("..", "..", "..", "dashboards", "catalog.yaml"), Options{
		WorkspaceID:  "libredash",
		DeploymentID: "dep_test",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v, want Olist workspace to compile", err)
	}

	types := map[workspace.AssetType]bool{}
	for _, asset := range compiled.Workspace.Graph.Assets {
		types[asset.Type] = true
	}
	for _, want := range []workspace.AssetType{
		workspace.AssetTypeSemanticModel,
		workspace.AssetTypeModelTable,
		workspace.AssetTypeSemanticTable,
		workspace.AssetTypeRelationship,
		workspace.AssetTypeMeasure,
		workspace.AssetTypeSource,
		workspace.AssetTypeConnection,
		workspace.AssetTypeDashboard,
		workspace.AssetTypePage,
		workspace.AssetTypePageItem,
		workspace.AssetTypeVisual,
		workspace.AssetTypeFilter,
		workspace.AssetTypeTable,
	} {
		if !types[want] {
			t.Fatalf("lineage asset type %q missing: %#v", want, types)
		}
	}
	for _, notWant := range []workspace.AssetType{"metric_view", "dataset", "cache_table"} {
		if types[notWant] {
			t.Fatalf("legacy asset type %q should not be present: %#v", notWant, types)
		}
	}

	edgeTypes := map[workspace.AssetEdgeType]bool{}
	for _, edge := range compiled.Workspace.Graph.Edges {
		edgeTypes[edge.Type] = true
	}
	for _, notWant := range []workspace.AssetEdgeType{"uses_metric_view", "uses_dataset", "uses_cache_table"} {
		if edgeTypes[notWant] {
			t.Fatalf("legacy edge type %q should not be present: %#v", notWant, edgeTypes)
		}
	}
}

func TestCompileAssetGraphIdentityAndPayloadInvariants(t *testing.T) {
	catalogPath := writeCompilerWorkspace(t, validCompilerDashboardYAML())
	first, err := Compile(catalogPath, Options{WorkspaceID: "libredash", DeploymentID: "dep_a"})
	if err != nil {
		t.Fatalf("Compile(dep_a) error = %v", err)
	}
	second, err := Compile(catalogPath, Options{WorkspaceID: "libredash", DeploymentID: "dep_b"})
	if err != nil {
		t.Fatalf("Compile(dep_b) error = %v", err)
	}

	firstAssets := assetsByID(first.Workspace.Graph)
	secondAssets := assetsByID(second.Workspace.Graph)
	if len(firstAssets) == 0 || len(firstAssets) != len(secondAssets) {
		t.Fatalf("asset counts = %d and %d", len(firstAssets), len(secondAssets))
	}
	for id, firstAsset := range firstAssets {
		secondAsset, ok := secondAssets[id]
		if !ok {
			t.Fatalf("asset %q missing from second graph", id)
		}
		if !strings.Contains(id, ":") || strings.HasPrefix(id, "asset_") {
			t.Fatalf("asset id %q is not a logical id", id)
		}
		if firstAsset.ContentHash != secondAsset.ContentHash {
			t.Fatalf("asset %q content hash changed across deployments", id)
		}
		if firstAsset.SnapshotID == secondAsset.SnapshotID {
			t.Fatalf("asset %q snapshot id did not change across deployments", id)
		}
		if firstAsset.ParentID != "" && !strings.Contains(string(firstAsset.ParentID), ":") {
			t.Fatalf("asset %q parent id %q is not logical", id, firstAsset.ParentID)
		}
		if firstAsset.PayloadSchema == "" {
			t.Fatalf("asset %q has empty payload schema", id)
		}
		if want := workspace.PayloadSchemaForAssetType(firstAsset.Type); want == "" || firstAsset.PayloadSchema != want {
			t.Fatalf("asset %q payload schema = %q, want %q", id, firstAsset.PayloadSchema, want)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(firstAsset.PayloadJSON), &payload); err != nil {
			t.Fatalf("asset %q payload is invalid JSON: %v", id, err)
		}
		if strings.Contains(strings.ToLower(firstAsset.PayloadJSON), `"auth"`) {
			t.Fatalf("asset %q payload leaked auth: %s", id, firstAsset.PayloadJSON)
		}
	}
	for _, edge := range first.Workspace.Graph.Edges {
		if !strings.Contains(string(edge.FromAssetID), ":") || !strings.Contains(string(edge.ToAssetID), ":") {
			t.Fatalf("edge endpoints are not logical ids: %#v", edge)
		}
	}

	changedPath := writeCompilerWorkspace(t, strings.Replace(validCompilerDashboardYAML(), "title: Sales", "title: Sales Updated", 1))
	changed, err := Compile(changedPath, Options{WorkspaceID: "libredash", DeploymentID: "dep_a"})
	if err != nil {
		t.Fatalf("Compile(changed) error = %v", err)
	}
	if assetsByID(changed.Workspace.Graph)["dashboard:sales"].ContentHash == firstAssets["dashboard:sales"].ContentHash {
		t.Fatal("dashboard content hash did not change after authored title changed")
	}
}

func TestCompileConnectionPayloadRedactsAuth(t *testing.T) {
	t.Setenv("LIBREDASH_TEST_S3_KEY", "env-key")
	t.Setenv("LIBREDASH_TEST_S3_SECRET", "env-secret")
	dir := t.TempDir()
	writeCompilerFixture(t, filepath.Join(dir, "catalog.yaml"), validCompilerCatalogYAML())
	writeCompilerFixture(t, filepath.Join(dir, "model.yaml"), `
name: olist
title: Olist
connections:
  prod_lake:
    kind: s3
    scope: s3://analytics-prod/
    auth:
      access_key_id: ${LIBREDASH_TEST_S3_KEY}
      secret_access_key: ${LIBREDASH_TEST_S3_SECRET}
sources:
  orders:
    connection: prod_lake
    path: orders.parquet
models:
  orders:
    source: orders
    primary_key: order_id
    fields:
      revenue: {label: Revenue}
semantic_models:
  olist:
    base_table: orders
    tables:
      - orders
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue), format: currency}
`)
	writeCompilerFixture(t, filepath.Join(dir, "dashboard.yaml"), validCompilerDashboardYAML())

	compiled, err := Compile(filepath.Join(dir, "catalog.yaml"), Options{WorkspaceID: "libredash", DeploymentID: "dep_auth"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	asset := assetsByID(compiled.Workspace.Graph)["connection:olist.prod_lake"]
	if asset.ID == "" {
		t.Fatal("connection asset missing")
	}
	if asset.PayloadSchema != "connection.v1" {
		t.Fatalf("connection payload schema = %q, want connection.v1", asset.PayloadSchema)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(asset.PayloadJSON), &payload); err != nil {
		t.Fatalf("connection payload JSON invalid: %v", err)
	}
	if payload["credentials_configured"] != true {
		t.Fatalf("credentials_configured = %#v, want true in %s", payload["credentials_configured"], asset.PayloadJSON)
	}
	for _, leaked := range []string{`"auth"`, "access_key_id", "secret_access_key", "env-key", "env-secret"} {
		if strings.Contains(strings.ToLower(asset.PayloadJSON), strings.ToLower(leaked)) {
			t.Fatalf("connection payload leaked %q: %s", leaked, asset.PayloadJSON)
		}
	}
}

func writeCompilerWorkspace(t *testing.T, dashboardYAML string) string {
	t.Helper()
	dir := t.TempDir()
	writeCompilerFixture(t, filepath.Join(dir, "catalog.yaml"), validCompilerCatalogYAML())
	writeCompilerFixture(t, filepath.Join(dir, "model.yaml"), validCompilerModelYAML())
	writeCompilerFixture(t, filepath.Join(dir, "dashboard.yaml"), dashboardYAML)
	return filepath.Join(dir, "catalog.yaml")
}

func assetsByID(graph workspace.AssetGraph) map[string]workspace.Asset {
	out := make(map[string]workspace.Asset, len(graph.Assets))
	for _, asset := range graph.Assets {
		out[string(asset.ID)] = asset
	}
	return out
}

func validCompilerCatalogYAML() string {
	return `
workspace:
  id: libredash
  title: LibreDash Workspace
semantic_models:
  - id: olist
    title: Olist
    path: model.yaml
dashboards:
  - id: sales
    title: Sales
    path: dashboard.yaml
`
}

func validCompilerModelYAML() string {
	return `
name: olist
title: Olist
connections:
  olist:
    kind: local
sources:
  orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    source: orders
    primary_key: order_id
    fields:
      status: {label: Status}
      revenue: {label: Revenue}
semantic_models:
  olist:
    base_table: orders
    tables:
      - orders
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue), format: currency}
`
}

func validCompilerDashboardYAML() string {
	return `
id: sales
title: Sales
semantic_model: olist
filters: {}
visuals:
  revenue:
    kind: kpi
    query:
      measures:
        revenue:
tables: {}
pages:
  - id: overview
    title: Overview
    visuals: []
`
}

func writeCompilerFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
