package app

import (
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
)

func TestAPIGenRoutesCoverHeadlessAPINotUITransports(t *testing.T) {
	spec, err := apigenapi.GetEmbeddedOpenAPISpec()
	if err != nil {
		t.Fatalf("embedded openapi: %v", err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("openapi paths missing: %#v", spec["paths"])
	}

	for _, path := range []string{
		"/api/v1/me",
		"/api/v1/me/permissions",
		"/api/v1/me/api-tokens",
		"/api/v1/me/api-tokens/{token}",
		"/api/v1/me/sessions",
		"/api/v1/me/sessions/{session}",
		"/api/v1/principals",
		"/api/v1/principals/{principal}",
		"/api/v1/workspaces",
		"/api/v1/workspaces/{workspace}/assets",
		"/api/v1/workspaces/{workspace}/asset-edges",
		"/api/v1/workspaces/{workspace}/deployments",
		"/api/v1/workspaces/{workspace}/deployments/{deployment}",
		"/api/v1/workspaces/{workspace}/deployments/{deployment}/artifact",
		"/api/v1/workspaces/{workspace}/deployments/{deployment}/validate",
		"/api/v1/workspaces/{workspace}/deployments/{deployment}/activate",
		"/api/v1/workspaces/{workspace}/materialization-runs",
		"/api/v1/workspaces/{workspace}/materialization-runs/{run}",
		"/api/v1/workspaces/{workspace}/agent/conversations",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/messages",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/turns",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}",
		"/api/v1/workspaces/{workspace}/agent/conversations/{conversation}/runs/{run}/events",
		"/api/v1/workspaces/{workspace}/roles",
		"/api/v1/workspaces/{workspace}/groups",
		"/api/v1/workspaces/{workspace}/groups/{group}",
		"/api/v1/workspaces/{workspace}/groups/{group}/members",
		"/api/v1/workspaces/{workspace}/groups/{group}/members/{principal}",
		"/api/v1/workspaces/{workspace}/role-bindings",
		"/api/v1/workspaces/{workspace}/role-bindings/{binding}",
		"/api/v1/workspaces/{workspace}/audit-events",
	} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("generated OpenAPI missing path %s", path)
		}
	}

	for _, path := range []string{"/api/workspaces", "/api/deployments", "/api/v1/workspaces/{workspace}/deployments/{deployment}/rollback", "/updates", "/commands/select", "/chat/updates", "/dashboards/{dashboard}"} {
		if _, ok := paths[path]; ok {
			t.Fatalf("generated OpenAPI should not include UI transport path %s", path)
		}
	}
}

func TestAPIGenOperationAuthCoverage(t *testing.T) {
	contracts := apigenapi.GetAPIGenOperationContracts()
	if len(contracts) == 0 {
		t.Fatal("no generated operation contracts")
	}
	for operationID, contract := range contracts {
		if contract.AuthzMode != "permission" || !contract.Protected {
			t.Fatalf("%s auth contract = mode %q protected %t, want permission/protected", operationID, contract.AuthzMode, contract.Protected)
		}
		if _, ok := apigenOperationPermissions[operationID]; !ok {
			t.Fatalf("%s missing app permission mapping", operationID)
		}
	}
	for operationID := range apigenOperationPermissions {
		if _, ok := contracts[operationID]; !ok {
			t.Fatalf("%s has app permission mapping but no generated contract", operationID)
		}
	}
	if got := apigenOperationPermissions["uploadDeploymentArtifact"]; got != access.PermissionDeploymentCreate {
		t.Fatalf("uploadDeploymentArtifact permission = %q, want %q", got, access.PermissionDeploymentCreate)
	}
}
