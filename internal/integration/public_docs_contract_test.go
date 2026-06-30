package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestPublicDocsAndScriptsDoNotAdvertiseRemovedCaCSurfaces(t *testing.T) {
	root := filepath.Join("..", "..")
	files := []string{
		"README.md",
		"ui-spec.md",
		filepath.Join("scripts", "agent_e2e.sh"),
	}
	for _, name := range files {
		name := name
		t.Run(name, func(t *testing.T) {
			body := readRepoFile(t, root, name)
			for _, forbidden := range []string{
				"dashboards/catalog.yaml",
				"GET /dashboards",
				"`/dashboards/",
				"('/dashboards/",
				"\"/dashboards/",
				"GET /updates",
				"`/updates",
				"('/updates",
				"\"/updates",
			} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("%s still advertises removed surface %q", name, forbidden)
				}
			}
			if regexp.MustCompile(`(?m)^\s*/chat(?:\s|$)`).MatchString(body) {
				t.Fatalf("%s still advertises unscoped /chat route", name)
			}
		})
	}

	script := readRepoFile(t, root, filepath.Join("scripts", "agent_e2e.sh"))
	for _, want := range []string{
		"--workspace sales",
		"--catalog dashboards/libredash.yaml",
		"--auto-approve",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/agent_e2e.sh missing current deploy/agent argument %q", want)
		}
	}
}

func readRepoFile(t *testing.T, root, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}
