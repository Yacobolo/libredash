package cli

import (
	"testing"

	cligen "github.com/Yacobolo/libredash/internal/cli/gen"
)

func TestAPIGenOperationURLUsesGeneratedContracts(t *testing.T) {
	u, err := apiOperationURL("https://libredash.example/", "rollbackDeployment", map[string]string{"project": "sales project", "deployment": "deploy 1"}, nil)
	if err != nil {
		t.Fatalf("operation URL: %v", err)
	}
	if u != "https://libredash.example/api/v1/projects/sales%20project/deployments/deploy%201/rollback" {
		t.Fatalf("url = %q", u)
	}

	u, err = apiOperationURL("https://libredash.example", "finalizeRelease", map[string]string{"project": "demo project", "release": "release 1"}, nil)
	if err != nil {
		t.Fatalf("operation URL: %v", err)
	}
	if u != "https://libredash.example/api/v1/projects/demo%20project/releases/release%201/finalize" {
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

func TestGeneratedVisualCLIUsesUnionCollectionMetadata(t *testing.T) {
	for _, spec := range cligen.APIGeneratedCommandSpecs {
		if spec.OperationID != "queryDashboardVisualData" {
			continue
		}
		if spec.Output.Mode != "collection" || spec.Pagination == nil || spec.Pagination.ItemsField != "data" {
			t.Fatalf("visual CLI metadata = output %#v pagination %#v, want data collection", spec.Output, spec.Pagination)
		}
		return
	}
	t.Fatal("queryDashboardVisualData CLI metadata missing")
}
