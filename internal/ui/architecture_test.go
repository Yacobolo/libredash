package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoUIRenderersDoNotMountProductInternals(t *testing.T) {
	files := []string{
		"page.go",
		"workspace.go",
		"admin.go",
		"chat.go",
		filepath.Join("..", "dashboard", "ui", "page.go"),
	}
	for _, file := range files {
		body := readArchitectureFile(t, file)
		for _, forbidden := range []string{
			`g.El("ld-data-grid"`,
			`g.El("ld-code-block"`,
			`g.El("ld-report-canvas"`,
			`g.El("ld-filter-panel"`,
			`g.El("ld-filter-card"`,
			`g.El("ld-kpi-card"`,
			`g.El("ld-echart"`,
			`g.El("ld-data-table"`,
			`g.El("ld-chat-thread"`,
			`g.El("ld-chat-composer"`,
			`g.El("ld-workspace-access-control"`,
			`g.El("ld-asset-lineage-graph"`,
			`data-workspace-asset-toolbar`,
			`data-connection-toolbar`,
			`data-filter-dock`,
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s mounts product UI internals %q", file, forbidden)
			}
		}
	}
}

func TestLitRouteRootsDoNotOwnRoutingOrBackendFetches(t *testing.T) {
	files := []string{
		filepath.Join("..", "..", "web", "components", "app", "catalog-page.ts"),
		filepath.Join("..", "..", "web", "components", "dashboard", "dashboard-page.ts"),
		filepath.Join("..", "..", "web", "components", "workspace", "workspace-page.ts"),
		filepath.Join("..", "..", "web", "components", "chat", "chat-page.ts"),
		filepath.Join("..", "..", "web", "components", "admin", "admin-page.ts"),
		filepath.Join("..", "..", "web", "components", "login", "login-page.ts"),
	}
	for _, file := range files {
		body := readArchitectureFile(t, file)
		for _, forbidden := range []string{
			"fetch(",
			"XMLHttpRequest",
			"@lit-labs/router",
			"vaadin-router",
			"navigo",
			"page.js",
			"history.pushState",
			"history.replaceState",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s owns routing or backend fetching via %q", file, forbidden)
			}
		}
	}
}

func readArchitectureFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}
