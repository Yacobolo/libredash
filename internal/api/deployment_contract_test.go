package api_test

import (
	"strings"
	"testing"
)

func TestProjectDeploymentAPIContract(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	paths := openAPIMap(t, spec, "paths")
	base := "/api/v1/projects/{project}/deployments"

	list := openAPIOperation(t, paths, base, "get")
	if list["operationId"] != "listDeployments" {
		t.Fatalf("list deployment operation = %#v", list)
	}
	create := openAPIOperation(t, paths, base, "post")
	if create["operationId"] != "createDeployment" || !operationHasParameter(create, "path", "project") || !operationHasParameter(create, "header", "Idempotency-Key") {
		t.Fatalf("create deployment operation = %#v", create)
	}
	if _, ok := openAPIMap(t, create, "responses")["202"]; !ok {
		t.Fatal("create deployment must return 202")
	}
	if privilege := openAPIMap(t, create, "x-authz")["privilege"]; privilege != "ACTIVATE_DEPLOYMENT" {
		t.Fatalf("deployment privilege = %#v", privilege)
	}
	for suffix, operationID := range map[string]string{
		"": "getDeployment", "/events": "listDeploymentEvents", "/cancel": "cancelDeployment", "/rollback": "rollbackDeployment",
	} {
		method := "get"
		if suffix == "/cancel" || suffix == "/rollback" {
			method = "post"
		}
		operation := openAPIOperation(t, paths, base+"/{deployment}"+suffix, method)
		if operation["operationId"] != operationID {
			t.Fatalf("%s operation = %#v", operationID, operation)
		}
	}
	if _, exists := paths[base+"/{deployment}/activate"]; exists {
		t.Fatal("separate deployment activation route remains public")
	}

	schemas := openAPIMap(t, openAPIMap(t, spec, "components"), "schemas")
	response := openAPISchema(t, schemas, "DeploymentResponse")
	for _, field := range []string{"id", "projectId", "releaseId", "environment", "status", "targets", "connections", "createdAt"} {
		_ = schemaProperty(t, response, field)
	}
	assertEnum(t, openAPISchema(t, schemas, "DeploymentStatus"), "queued", "running", "active", "failed", "cancelled", "superseded")

	for path := range paths {
		if strings.Contains(path, "/rollouts") || strings.Contains(path, "/deployment-candidates") {
			t.Fatalf("legacy deployment route remains: %s", path)
		}
	}
}
