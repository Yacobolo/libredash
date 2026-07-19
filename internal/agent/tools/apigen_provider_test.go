package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	agentcore "github.com/Yacobolo/libredash/pkg/agent"
)

func TestGlobalAPIGenDefinitionsRequireWorkspaceForWorkspaceRoutes(t *testing.T) {
	var authorizedScope Scope
	var dispatchedPath string
	provider := APIGenProvider{
		Authorize: func(_ context.Context, scope Scope, _ string) (agentcore.ToolResult, bool) {
			authorizedScope = scope
			return agentcore.ToolResult{}, true
		},
		Dispatch: func(_ Scope, _ string, request *http.Request) (*http.Response, bool) {
			dispatchedPath = request.URL.Path
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       http.NoBody,
			}, true
		},
	}

	var definition agentcore.ToolDefinition
	for _, candidate := range provider.Definitions(Scope{PrincipalID: "principal-1"}) {
		if candidate.Name == "list_dashboards" {
			definition = candidate
			break
		}
	}
	if definition.Name == "" {
		t.Fatal("list_dashboards definition not found")
	}
	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("decode input schema: %v", err)
	}
	if _, ok := schema.Properties["workspace"]; !ok || !containsString(schema.Required, "workspace") {
		t.Fatalf("global input schema = %s, want required workspace", definition.InputSchema)
	}

	result, err := definition.Handler.Run(context.Background(), agentcore.ToolCall{ID: "call-1", Arguments: json.RawMessage(`{"workspace":"sales"}`)})
	if err != nil {
		t.Fatalf("run tool: %v", err)
	}
	if result.IsError && !strings.Contains(dispatchedPath, "/api/v1/workspaces/sales/") {
		t.Fatalf("tool result = %#v", result)
	}
	if authorizedScope.WorkspaceID != "sales" {
		t.Fatalf("authorized workspace = %q, want sales", authorizedScope.WorkspaceID)
	}
	if dispatchedPath != "/api/v1/workspaces/sales/dashboards" {
		t.Fatalf("dispatched path = %q", dispatchedPath)
	}
}

func TestGlobalVisualDefinitionRequiresWorkspace(t *testing.T) {
	definition := (VisualProvider{}).Definitions(Scope{PrincipalID: "principal-1"})[0]
	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("decode input schema: %v", err)
	}
	if _, ok := schema.Properties["workspace"]; !ok || !containsString(schema.Required, "workspace") {
		t.Fatalf("global visual schema = %s, want required workspace", definition.InputSchema)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
