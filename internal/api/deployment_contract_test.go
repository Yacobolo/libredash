package api_test

import (
	"strings"
	"testing"
)

func TestProjectDeploymentAPIContract(t *testing.T) {
	spec := managedDataOpenAPISpec(t)
	paths := openAPIMap(t, spec, "paths")
	base := "/api/v1/projects/{project}/deployments"

	create := openAPIOperation(t, paths, base, "post")
	if create["operationId"] != "createProjectDeployment" || !operationHasParameter(create, "path", "project") || !operationHasParameter(create, "header", "Idempotency-Key") {
		t.Fatalf("create deployment operation = %#v", create)
	}
	if _, ok := openAPIMap(t, create, "responses")["201"]; !ok {
		t.Fatal("create deployment must return 201")
	}

	get := openAPIOperation(t, paths, base+"/{deployment}", "get")
	if get["operationId"] != "getProjectDeployment" {
		t.Fatalf("get deployment operationId = %#v", get["operationId"])
	}
	activate := openAPIOperation(t, paths, base+"/{deployment}/activate", "post")
	if activate["operationId"] != "activateProjectDeployment" || !operationHasParameter(activate, "header", "Idempotency-Key") {
		t.Fatalf("activate deployment operation = %#v", activate)
	}
	if _, ok := openAPIMap(t, activate, "responses")["200"]; !ok {
		t.Fatal("activate deployment must return 200")
	}
	if privilege := openAPIMap(t, activate, "x-authz")["privilege"]; privilege != "ACTIVATE_DEPLOYMENT" {
		t.Fatalf("activate deployment privilege = %#v, want ACTIVATE_DEPLOYMENT", privilege)
	}

	schemas := openAPIMap(t, openAPIMap(t, spec, "components"), "schemas")
	response := openAPISchema(t, schemas, "ProjectDeploymentResponse")
	for _, field := range []string{"id", "project", "environment", "requestDigest", "status", "targets", "connections", "createdAt"} {
		_ = schemaProperty(t, response, field)
	}
	target := openAPISchema(t, schemas, "ProjectDeploymentTargetResponse")
	for _, field := range []string{"workspace", "candidateId", "status"} {
		_ = schemaProperty(t, target, field)
	}
	assertEnum(t, openAPISchema(t, schemas, "ProjectDeploymentStatus"), "pending", "active", "failed", "superseded")
	assertEnum(t, openAPISchema(t, schemas, "ProjectDeploymentTargetStatus"), "pending", "active", "failed")

	for path := range paths {
		if strings.Contains(path, "/data-connections/") && strings.Contains(path, "/rollouts") {
			t.Fatalf("connection-scoped rollout route remains: %s", path)
		}
	}
	if _, exists := paths["/api/v1/workspaces/{workspace}/publishes/{publish}/activate"]; exists {
		t.Fatal("single-publish activation route remains public")
	}
	encoded := string(mustJSON(t, spec))
	for _, removed := range []string{"ACTIVATE_PUBLISH", "ACTIVATE_DATA"} {
		if strings.Contains(encoded, removed) {
			t.Fatalf("removed privilege %s remains in generated API", removed)
		}
	}
}
