package cli

import (
	"net/url"
	"testing"

	cligen "github.com/Yacobolo/libredash/internal/cli/gen"
)

func TestAPIGenOperationURLUsesGeneratedContracts(t *testing.T) {
	u, err := apiOperationURL("https://libredash.example/", "rollbackDeployment", map[string]string{"workspace": "demo", "deployment": "dep 1"}, nil)
	if err != nil {
		t.Fatalf("operation URL: %v", err)
	}
	if u != "https://libredash.example/api/v1/workspaces/demo/deployments/dep%201/activate" {
		t.Fatalf("url = %q", u)
	}

	query := url.Values{"limit": []string{"10"}}
	u, err = apiOperationURL("https://libredash.example", "listDeployments", map[string]string{"workspace": "demo"}, query)
	if err != nil {
		t.Fatalf("operation URL: %v", err)
	}
	if u != "https://libredash.example/api/v1/workspaces/demo/deployments?limit=10" {
		t.Fatalf("url = %q", u)
	}
}

func TestGeneratedCLIRegistryContainsCompatibilityCommands(t *testing.T) {
	commands := map[string]bool{}
	for _, spec := range cligen.APIGeneratedCommandSpecs {
		commands[spec.OperationID] = true
	}
	for _, operationID := range []string{"listDeployments", "listAgentConversations"} {
		if !commands[operationID] {
			t.Fatalf("generated CLI registry missing %s", operationID)
		}
	}
}
