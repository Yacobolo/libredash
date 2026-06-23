package deploy

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSemanticModelDesignBundleAssets(t *testing.T) {
	catalogPath := writeSemanticDesignBundleWorkspace(t)
	var bundle bytes.Buffer

	_, _, err := PackCatalog(catalogPath, &bundle)
	if err != nil {
		t.Fatalf("PackCatalog() error = %v, want semantic-model-first bundle", err)
	}

	path := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	validation, err := ValidateArtifact(path, "libredash", "dep_red")
	if err != nil {
		t.Fatalf("ValidateArtifact() error = %v, want semantic-model-first artifact", err)
	}
	defer os.RemoveAll(validation.RootDir)

	types := map[string]bool{}
	for _, asset := range validation.Assets {
		types[asset.Type] = true
	}
	for _, want := range []string{"semantic_model", "model_table", "measure", "source", "connection", "dashboard"} {
		if !types[want] {
			t.Fatalf("asset type %q missing from semantic-model-first artifact: %#v", want, types)
		}
	}
	for _, notWant := range []string{"metric_view", "dataset", "cache_table"} {
		if types[notWant] {
			t.Fatalf("legacy user-facing asset type %q should not be required: %#v", notWant, types)
		}
	}
}

func TestSemanticModelDesignDashboardLineageEdges(t *testing.T) {
	catalogPath := writeSemanticDesignBundleWorkspace(t)
	var bundle bytes.Buffer

	if _, _, err := PackCatalog(catalogPath, &bundle); err != nil {
		t.Fatalf("PackCatalog() error = %v, want semantic-model-first bundle", err)
	}
	path := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := os.WriteFile(path, bundle.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	validation, err := ValidateArtifact(path, "libredash", "dep_red")
	if err != nil {
		t.Fatalf("ValidateArtifact() error = %v, want semantic-model-first artifact", err)
	}
	defer os.RemoveAll(validation.RootDir)

	edgeTypes := map[string]bool{}
	for _, edge := range validation.Edges {
		edgeTypes[edge.Type] = true
	}
	for _, want := range []string{"uses_semantic_model", "uses_model_table", "reads_source", "uses_connection", "uses_measure"} {
		if !edgeTypes[want] {
			t.Fatalf("lineage edge %q missing from semantic-model-first artifact: %#v", want, edgeTypes)
		}
	}
	if edgeTypes["uses_metric_view"] || edgeTypes["uses_dataset"] || edgeTypes["uses_cache_table"] {
		t.Fatalf("legacy lineage edges should not be present: %#v", edgeTypes)
	}
}

func writeSemanticDesignBundleWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeDeployFixture(t, filepath.Join(dir, "catalog.yaml"), `
workspace:
  id: libredash
  title: LibreDash Workspace
semantic_models:
  - id: olist
    title: Olist Commerce
    path: model.yaml
dashboards:
  - id: sales
    title: Sales
    path: dashboard.yaml
`)
	writeDeployFixture(t, filepath.Join(dir, "model.yaml"), `
name: olist
title: Olist Commerce
connections:
  olist:
    kind: local
sources:
  olist_orders:
    connection: olist
    path: orders.csv
    format: csv
models:
  orders:
    source: olist_orders
semantic_models:
  olist:
    base_table: orders
    tables:
      orders:
        model: orders
        primary_key: order_id
        fields:
          status: {expr: status}
    measures:
      defaults: {table: orders, grain: order_id}
      revenue: {expr: SUM(orders.revenue), format: currency}
`)
	writeDeployFixture(t, filepath.Join(dir, "dashboard.yaml"), `
id: sales
title: Sales
semantic_model: olist
filters: {}
visuals:
  revenue:
    title: Revenue
    type: bar
    query:
      dimensions:
        status: orders.status
      measures:
        revenue:
tables: {}
pages:
  - id: overview
    title: Overview
    visuals: []
`)
	return filepath.Join(dir, "catalog.yaml")
}

func writeDeployFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
