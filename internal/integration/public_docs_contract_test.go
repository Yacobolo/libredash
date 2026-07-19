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
		"sales workspace",
		"--project dashboards/libredash.yaml",
		"--auto-approve",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/agent_e2e.sh missing current deploy/agent argument %q", want)
		}
	}
	if regexp.MustCompile(`agent ask[^\n]*--workspace`).MatchString(script) {
		t.Fatal("scripts/agent_e2e.sh still passes the removed agent --workspace flag")
	}
}

func TestPublicDocsAdvertiseOnlyProjectDeploy(t *testing.T) {
	root := filepath.Join("..", "..")
	files := []string{
		"README.md",
		filepath.Join("docs", "data-ingestion.md"),
		filepath.Join("deploy", "hetzner", "README.md"),
	}
	for _, name := range files {
		body := readRepoFile(t, root, name)
		for _, forbidden := range []string{
			"libredash publish",
			"libredash publishes",
			"libredash data deploy",
			"`data deploy`",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s still advertises removed command %q", name, forbidden)
			}
		}
		if !strings.Contains(body, "libredash deploy") {
			t.Fatalf("%s does not document the project deploy command", name)
		}
	}

	ingestion := readRepoFile(t, root, filepath.Join("docs", "data-ingestion.md"))
	for _, want := range []string{
		"libredash data plan",
		"libredash data sync",
		"--revision",
	} {
		if !strings.Contains(ingestion, want) {
			t.Fatalf("docs/data-ingestion.md missing current command surface %q", want)
		}
	}
}

func TestDeveloperWorkflowsUseProjectDeploy(t *testing.T) {
	root := filepath.Join("..", "..")
	files := map[string][]string{
		"Taskfile.yml": {"go run ./cmd/libredash deploy", `olist=$REVISION`},
		filepath.Join("scripts", "dev-server.sh"): {"go run ./cmd/libredash deploy", `$connection=$revision`},
		filepath.Join("scripts", "agent_e2e.sh"):  {`"$BIN" deploy`, `olist=$REVISION`},
	}
	for name, required := range files {
		body := readRepoFile(t, root, name)
		for _, forbidden := range []string{"data deploy", "libredash publish", `"$BIN" publish`} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s still invokes removed command surface %q", name, forbidden)
			}
		}
		for _, want := range required {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing project deploy contract %q", name, want)
			}
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
