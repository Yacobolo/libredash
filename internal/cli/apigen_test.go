package cli

import (
	"net/url"
	"testing"

	cligen "github.com/Yacobolo/libredash/internal/cli/gen"
)

func TestAPIGenOperationURLUsesGeneratedContracts(t *testing.T) {
	u, err := apiOperationURL("https://libredash.example/", "activatePublish", map[string]string{"workspace": "demo", "publish": "state 1"}, nil)
	if err != nil {
		t.Fatalf("operation URL: %v", err)
	}
	if u != "https://libredash.example/api/v1/workspaces/demo/publishes/state%201/activate" {
		t.Fatalf("url = %q", u)
	}

	query := url.Values{"limit": []string{"10"}}
	u, err = apiOperationURL("https://libredash.example", "listPublishes", map[string]string{"workspace": "demo"}, query)
	if err != nil {
		t.Fatalf("operation URL: %v", err)
	}
	if u != "https://libredash.example/api/v1/workspaces/demo/publishes?limit=10" {
		t.Fatalf("url = %q", u)
	}

	u, err = apiOperationURL("https://libredash.example", "queryDashboardPage", map[string]string{"workspace": "demo", "dashboard": "sales dash", "page": "overview"}, nil)
	if err != nil {
		t.Fatalf("operation URL: %v", err)
	}
	if u != "https://libredash.example/api/v1/workspaces/demo/dashboards/sales%20dash/pages/overview/query" {
		t.Fatalf("url = %q", u)
	}
}

func TestGeneratedCLIRegistryContainsCompatibilityCommands(t *testing.T) {
	commands := map[string]bool{}
	for _, spec := range cligen.APIGeneratedCommandSpecs {
		commands[spec.OperationID] = true
	}
	for _, operationID := range []string{"listPublishes", "listAgentConversations", "listDashboards", "getDashboard", "queryDashboardPage", "queryDashboardTable", "listSemanticModels", "getSemanticModel"} {
		if !commands[operationID] {
			t.Fatalf("generated CLI registry missing %s", operationID)
		}
	}
}
