package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/pkg/agent"
)

func TestAPIGenAgentToolsExposeTaggedReadOperationsOnly(t *testing.T) {
	server := NewWithOptions(manyRowsMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	names := map[string]agent.ToolDefinition{}
	for _, tool := range tools {
		names[tool.Name] = tool
	}

	for _, want := range []string{
		"describe_dashboard",
		"describe_model",
		"get_deployment",
		"get_materialization_run",
		"list_dashboards",
		"list_deployments",
		"list_materialization_runs",
		"list_semantic_models",
		"list_workspace_asset_edges",
		"list_workspace_assets",
		"list_workspaces",
		"query_dashboard_page",
		"query_table",
	} {
		if _, ok := names[want]; !ok {
			t.Fatalf("missing APIGen agent tool %q in %#v", want, toolNames(tools))
		}
	}
	for _, forbidden := range []string{
		"activate_deployment",
		"create_agent_turn",
		"create_deployment",
		"create_role_binding",
		"revoke_current_api_token",
		"upload_deployment_artifact",
	} {
		if _, ok := names[forbidden]; ok {
			t.Fatalf("risky operation exposed as agent tool %q", forbidden)
		}
	}

	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(names["list_workspace_assets"].InputSchema, &schema); err != nil {
		t.Fatalf("decode list_workspace_assets schema: %v", err)
	}
	if _, ok := schema.Properties["workspace"]; ok {
		t.Fatalf("workspace should be injected from agent scope, not model arguments: %s", names["list_workspace_assets"].InputSchema)
	}
	for _, want := range []string{"type", "q", "limit", "pageToken"} {
		if _, ok := schema.Properties[want]; !ok {
			t.Fatalf("schema missing query parameter %q: %s", want, names["list_workspace_assets"].InputSchema)
		}
	}
}

func TestAPIGenAgentToolsExposeTypeSpecArgumentNamesAndBodyFields(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	names := map[string]agent.ToolDefinition{}
	for _, tool := range tools {
		names[tool.Name] = tool
	}
	for toolName, wantProps := range map[string][]string{
		"describe_dashboard":   {"dashboard"},
		"describe_model":       {"model"},
		"query_dashboard_page": {"dashboard", "page", "filters"},
		"query_table":          {"dashboard", "table", "count", "filters", "pageId"},
	} {
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(names[toolName].InputSchema, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", toolName, err)
		}
		for _, want := range wantProps {
			if _, ok := schema.Properties[want]; !ok {
				t.Fatalf("%s schema missing %q: %s", toolName, want, names[toolName].InputSchema)
			}
		}
		for _, forbidden := range []string{"dashboard_id", "model_id", "page_id", "table_id"} {
			if _, ok := schema.Properties[forbidden]; ok {
				t.Fatalf("%s schema exposes rewritten arg %q: %s", toolName, forbidden, names[toolName].InputSchema)
			}
		}
	}
}

func TestAPIGenAgentToolDispatchesThroughGeneratedOperation(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var listAssets agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "list_workspace_assets" {
			listAssets = tool
			break
		}
	}
	if listAssets.Handler == nil {
		t.Fatal("list_workspace_assets tool missing")
	}

	result, err := listAssets.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "list_workspace_assets",
		Arguments: json.RawMessage(`{"type":"dashboard","limit":1}`),
	})
	if err != nil {
		t.Fatalf("run tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	body, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal result content: %v", err)
	}
	var decoded struct {
		Items []struct {
			ID          string `json:"id"`
			WorkspaceID string `json:"workspaceId"`
			Type        string `json:"type"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode result: %v\n%s", err, body)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].ID != "dashboard:executive-sales" || decoded.Items[0].WorkspaceID != "test" || decoded.Items[0].Type != "dashboard" {
		t.Fatalf("tool result = %#v", decoded.Items)
	}
}

func TestAPIGenAgentToolDispatchesJSONBodyOperation(t *testing.T) {
	server := NewWithOptions(manyRowsMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var queryTable agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "query_table" {
			queryTable = tool
			break
		}
	}
	if queryTable.Handler == nil {
		t.Fatal("query_table tool missing")
	}
	result, err := queryTable.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "query_table",
		Arguments: json.RawMessage(`{"dashboard":"executive-sales","pageId":"overview","table":"orders","count":500}`),
	})
	if err != nil {
		t.Fatalf("run tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	body, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal result content: %v", err)
	}
	var table struct {
		AvailableRows int `json:"availableRows"`
		Blocks        map[string]struct {
			Rows []map[string]any `json:"rows"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(body, &table); err != nil {
		t.Fatalf("decode table result: %v\n%s", err, body)
	}
	if table.AvailableRows != 50 || len(table.Blocks["a"].Rows) != 50 {
		t.Fatalf("table result was not capped to 50: %#v", table)
	}
}

func TestAPIGenAgentToolEnforcesCredentialPermissionAllowlistAndWorkspace(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "agent-token@example.com", "Agent Token", access.RoleOwner)
	agentOnlyToken := access.APIToken{WorkspaceID: "test", Permissions: []string{access.PermissionAgentUse}}
	assetToken := access.APIToken{WorkspaceID: "test", Permissions: []string{access.PermissionAgentUse, access.PermissionAssetRead}}
	foreignToken := access.APIToken{WorkspaceID: "other", Permissions: []string{access.PermissionAssetRead}}
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, AccessRepo: testAccessRepository(store), DefaultWorkspaceID: "test"})

	run := func(token access.APIToken) agent.ToolResult {
		scope := agentapp.Scope{
			WorkspaceID: "test",
			PrincipalID: principal.ID,
			Credential: agentapp.CredentialScope{
				WorkspaceID: token.WorkspaceID,
				Permissions: append([]string(nil), token.Permissions...),
				Restricted:  token.Permissions != nil,
			},
		}
		tools := server.agentAPIGenToolDefinitions(scope)
		for _, tool := range tools {
			if tool.Name == "list_dashboards" {
				result, err := tool.Handler.Run(ctx, agent.ToolCall{ID: "call_1", Name: "list_dashboards", Arguments: json.RawMessage(`{}`)})
				if err != nil {
					t.Fatalf("run list_dashboards: %v", err)
				}
				return result
			}
		}
		t.Fatal("list_dashboards tool missing")
		return agent.ToolResult{}
	}

	if result := run(agentOnlyToken); !result.IsError {
		t.Fatalf("agent-only token unexpectedly called asset tool: %#v", result.Content)
	}
	if result := run(foreignToken); !result.IsError {
		t.Fatalf("foreign workspace token unexpectedly called asset tool: %#v", result.Content)
	}
	if result := run(assetToken); result.IsError {
		t.Fatalf("asset token was rejected: %#v", result.Content)
	}
}

func toolNames(tools []agent.ToolDefinition) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
