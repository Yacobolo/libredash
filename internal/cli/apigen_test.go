package cli

import (
	"testing"

	cligen "github.com/Yacobolo/libredash/internal/cli/gen"
)

func TestAPIGenOperationURLUsesGeneratedContracts(t *testing.T) {
	u, err := apiOperationURL("https://libredash.example/", "activateProjectDeployment", map[string]string{"project": "sales project", "deployment": "deploy 1"}, nil)
	if err != nil {
		t.Fatalf("operation URL: %v", err)
	}
	if u != "https://libredash.example/api/v1/projects/sales%20project/deployments/deploy%201/activate" {
		t.Fatalf("url = %q", u)
	}

	u, err = apiOperationURL("https://libredash.example", "validateDeploymentCandidate", map[string]string{"project": "demo project", "workspace": "demo", "candidate": "candidate 1"}, nil)
	if err != nil {
		t.Fatalf("operation URL: %v", err)
	}
	if u != "https://libredash.example/api/v1/projects/demo%20project/workspaces/demo/deployment-candidates/candidate%201/validate" {
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

func TestGeneratedCLIRegistryContainsCoreCommands(t *testing.T) {
	commands := map[string]bool{}
	for _, spec := range cligen.APIGeneratedCommandSpecs {
		commands[spec.OperationID] = true
	}
	for _, operationID := range []string{"listAgentConversations", "listDashboards", "getDashboard", "queryDashboardPage", "queryDashboardTable", "listSemanticModels", "getSemanticModel"} {
		if !commands[operationID] {
			t.Fatalf("generated CLI registry missing %s", operationID)
		}
	}
}
