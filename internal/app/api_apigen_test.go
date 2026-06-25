package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
)

func TestAPIGenUsesTypeSpecV030(t *testing.T) {
	root := projectRoot(t)
	manifest, err := os.ReadFile(filepath.Join(root, "api", "apigen.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestText := string(manifest)
	if !strings.Contains(manifestText, "typespec_dir: typespec") {
		t.Fatalf("manifest should use TypeSpec source, got:\n%s", manifestText)
	}
	if strings.Contains(manifestText, "cue_dir:") {
		t.Fatalf("manifest should not use cue_dir after APIGen v0.3.0 migration")
	}

	taskfile, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatalf("read Taskfile.yml: %v", err)
	}
	taskText := string(taskfile)
	for _, want := range []string{
		"github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.3.0 typespec-compile",
		"github.com/Yacobolo/toolbelt/apigen/cmd/apigen@v0.3.0 all",
	} {
		if !strings.Contains(taskText, want) {
			t.Fatalf("Taskfile.yml missing generation command %q", want)
		}
	}
	for _, forbidden := range []string{"cue-compile", "apigen@v0.2.0"} {
		if strings.Contains(taskText, forbidden) {
			t.Fatalf("Taskfile.yml should not contain %q after APIGen v0.3.0 migration", forbidden)
		}
	}
}

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

func TestAPIGenOperationExtensions(t *testing.T) {
	contracts := apigenapi.GetAPIGenOperationContracts()
	for operationID, contract := range contracts {
		if _, ok := contract.Extensions["x-agent"]; ok {
			t.Fatalf("%s should not have x-agent metadata in the TypeSpec migration", operationID)
		}
		authz, ok := contract.Extensions["x-authz"].(map[string]any)
		if !ok {
			t.Fatalf("%s missing generated x-authz extension: %#v", operationID, contract.Extensions["x-authz"])
		}
		if got := authz["mode"]; got != "permission" {
			t.Fatalf("%s x-authz mode = %#v, want permission", operationID, got)
		}
		if got := authz["permission"]; got != apigenOperationPermissions[operationID] {
			t.Fatalf("%s x-authz permission = %#v, want %q", operationID, got, apigenOperationPermissions[operationID])
		}
		if operationID != "uploadDeploymentArtifact" {
			if _, ok := contract.Extensions["x-libredash-dispatch"]; ok {
				t.Fatalf("%s should not have raw-body dispatch extension", operationID)
			}
		}
	}

	upload, ok := contracts["uploadDeploymentArtifact"]
	if !ok {
		t.Fatal("uploadDeploymentArtifact contract missing")
	}
	if got := upload.Extensions["x-libredash-dispatch"]; got != "raw-body" {
		t.Fatalf("uploadDeploymentArtifact x-libredash-dispatch = %#v, want raw-body", got)
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("could not find project root from %s", dir)
		}
		dir = next
	}
}
