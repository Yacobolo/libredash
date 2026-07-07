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
				"/workspaces/{workspace}/updates",
				"/chat/updates",
				"/data/updates",
				"/admin/storage/updates",
				"/admin/queries/updates",
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
		"--project dashboards/libredash.yaml",
		"--auto-approve",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/agent_e2e.sh missing current deploy/agent argument %q", want)
		}
	}
}

func TestEnvExampleDoesNotEnablePlaceholderIdentityProviders(t *testing.T) {
	envExample := readRepoFile(t, filepath.Join("..", ".."), ".env.example")
	for _, name := range []string{
		"LIBREDASH_AZURE_CLIENT_ID",
		"LIBREDASH_AZURE_CLIENT_SECRET",
		"LIBREDASH_AZURE_CALLBACK_URL",
		"LIBREDASH_AZURE_TENANT",
		"LIBREDASH_OIDC_PROVIDER_ID",
		"LIBREDASH_OIDC_ISSUER_URL",
		"LIBREDASH_OIDC_CLIENT_ID",
		"LIBREDASH_OIDC_CLIENT_SECRET",
		"LIBREDASH_OIDC_CALLBACK_URL",
		"LIBREDASH_OIDC_SCOPES",
	} {
		if regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `=`).MatchString(envExample) {
			t.Fatalf(".env.example enables optional provider variable %s by default", name)
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
