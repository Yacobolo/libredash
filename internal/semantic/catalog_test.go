package semantic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorkspaceCatalog(t *testing.T) {
	workspace, err := LoadWorkspace(filepath.Join("..", "..", "dashboards", "catalog.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	if len(workspace.Catalog.SemanticModels) != 1 {
		t.Fatalf("model catalog count = %d, want 1", len(workspace.Catalog.SemanticModels))
	}
	if len(workspace.Catalog.Dashboards) != 1 {
		t.Fatalf("dashboard catalog count = %d, want 1", len(workspace.Catalog.Dashboards))
	}
	if got := workspace.Catalog.Workspace.Title; got != "LibreDash Workspace" {
		t.Fatalf("workspace title = %q, want LibreDash Workspace", got)
	}
	if _, ok := workspace.Models["olist"]; !ok {
		t.Fatal("workspace missing olist model")
	}
	if _, ok := workspace.Dashboards["executive-sales"]; !ok {
		t.Fatal("workspace missing executive-sales dashboard")
	}
}

func TestLoadWorkspaceRejectsLegacyMetricViewsKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.yaml")
	content := `workspace:
  id: test
  title: Test
semantic_models: []
metrics_views: []
dashboards: []
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadWorkspace(path)
	if err == nil || !strings.Contains(err.Error(), "legacy metric views") {
		t.Fatalf("LoadWorkspace error = %v, want legacy metrics_views rejection", err)
	}
}

func TestCatalogValidateRejectsDuplicateIDs(t *testing.T) {
	baseDir := filepath.Join("..", "..", "dashboards")
	catalog := Catalog{
		SemanticModels: []CatalogModel{
			{ID: "olist", Title: "Olist", Path: "olist/model.yaml"},
			{ID: "olist", Title: "Olist Copy", Path: "olist/model.yaml"},
		},
		Dashboards: []CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales", Path: "olist/executive-sales.yaml"},
		},
	}

	assertCatalogValidateError(t, catalog, baseDir, "duplicate semantic model")
}

func TestCatalogValidateRejectsMissingPath(t *testing.T) {
	baseDir := filepath.Join("..", "..", "dashboards")
	catalog := Catalog{
		SemanticModels: []CatalogModel{
			{ID: "olist", Title: "Olist", Path: "olist/missing.yaml"},
		},
		Dashboards: []CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales", Path: "olist/executive-sales.yaml"},
		},
	}

	assertCatalogValidateError(t, catalog, baseDir, "missing.yaml")
}
