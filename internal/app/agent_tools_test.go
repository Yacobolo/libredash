package app

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/agenttools"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/queryaudit"
	"github.com/Yacobolo/libredash/internal/workspace"
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
		"describe_dashboard_visual",
		"describe_model",
		"get_publish",
		"get_refresh_run",
		"asset_lineage",
		"describe_asset",
		"list_dashboard_components",
		"list_dashboard_filter_options",
		"list_assets",
		"list_dashboards",
		"list_publishes",
		"list_refresh_runs",
		"list_semantic_datasets",
		"list_semantic_fields",
		"list_semantic_models",
		"list_workspace_asset_edges",
		"list_workspaces",
		"preview_semantic_dataset",
		"query_dashboard_page",
		"query_dashboard_table_data",
		"query_dashboard_visual_data",
		"query_semantic_dataset",
		"search_workspace",
		"query_table",
		"explain_semantic_preview",
		"explain_semantic_query",
	} {
		if _, ok := names[want]; !ok {
			t.Fatalf("missing APIGen agent tool %q in %#v", want, toolNames(tools))
		}
	}
	for _, forbidden := range []string{
		"activate_publish",
		"create_agent_turn",
		"create_publish",
		"create_role_binding",
		"revoke_current_api_token",
		"upload_publish_artifact",
	} {
		if _, ok := names[forbidden]; ok {
			t.Fatalf("risky operation exposed as agent tool %q", forbidden)
		}
	}

	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(names["list_assets"].InputSchema, &schema); err != nil {
		t.Fatalf("decode list_assets schema: %v", err)
	}
	if _, ok := schema.Properties["workspace"]; !ok {
		t.Fatalf("workspace should be available as an optional tool target: %s", names["list_assets"].InputSchema)
	}
	for _, want := range []string{"type", "q", "limit", "pageToken"} {
		if _, ok := schema.Properties[want]; !ok {
			t.Fatalf("schema missing query parameter %q: %s", want, names["list_assets"].InputSchema)
		}
	}
	if err := json.Unmarshal(names["search_workspace"].InputSchema, &schema); err != nil {
		t.Fatalf("decode search_workspace schema: %v", err)
	}
	if _, ok := schema.Properties["workspace"]; !ok {
		t.Fatalf("workspace should be available as an optional tool target: %s", names["search_workspace"].InputSchema)
	}
	for _, want := range []string{"q", "types", "limit", "pageToken"} {
		if _, ok := schema.Properties[want]; !ok {
			t.Fatalf("search_workspace schema missing query parameter %q: %s", want, names["search_workspace"].InputSchema)
		}
	}
}

func TestAgentVisualToolIsCustomAgentOnlyTool(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentVisualToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal", DevAuthBypass: true})
	if len(tools) != 1 || tools[0].Name != agenttools.QueryVisualToolName || tools[0].Handler == nil {
		t.Fatalf("visual tools = %#v", tools)
	}
	schemaText := string(tools[0].InputSchema)
	for _, forbidden := range []string{`"$ref"`, `"$defs"`, `"definitions"`} {
		if strings.Contains(schemaText, forbidden) {
			t.Fatalf("query_visual schema contains non-portable keyword %s: %s", forbidden, schemaText)
		}
	}
	for _, tool := range server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"}) {
		if tool.Name == agenttools.QueryVisualToolName {
			t.Fatalf("query_visual should not be exposed through APIGen tools")
		}
	}
}

func TestAgentAPIGenQueryAuditSurface(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})
	var queryTool agent.ToolDefinition
	for _, tool := range server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal", DevAuthBypass: true}) {
		if tool.Name == "query_semantic_dataset" {
			queryTool = tool
			break
		}
	}
	if queryTool.Handler == nil {
		t.Fatal("query_semantic_dataset tool not found")
	}

	result, err := queryTool.Handler.Run(context.Background(), agent.ToolCall{
		ID:   "call_agent_query",
		Name: "query_semantic_dataset",
		Arguments: json.RawMessage(`{
			"model":"test",
			"dataset":"orders",
			"dimensions":[{"field":"orders.status","alias":"status"}],
			"measures":[{"field":"order_count"}],
			"limit":1
		}`),
	})
	if err != nil {
		t.Fatalf("run query_semantic_dataset: %v", err)
	}
	if result.IsError {
		t.Fatalf("query_semantic_dataset returned error: %#v", result.Content)
	}

	events := queryEventsForTest(t, server, queryaudit.Filter{WorkspaceID: "test", Surface: dataquery.SurfaceAgent})
	if len(events) != 1 {
		t.Fatalf("agent query events = %d, want 1: %#v", len(events), events)
	}
	if events[0].Operation != dataquery.OperationAgentQuery || events[0].ObjectType != "agent_tool" || events[0].RequestID != "call_agent_query" {
		t.Fatalf("agent query event = %#v", events[0])
	}
}

func TestAgentVisualToolReturnsChartPatchFromSemanticData(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tool := server.agentVisualToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal", DevAuthBypass: true})[0]
	result, err := tool.Handler.Run(context.Background(), agent.ToolCall{
		ID:   "call_1",
		Name: "query_visual",
		Arguments: json.RawMessage(`{
			"kind":"chart",
			"model":"test",
			"dataset":"orders",
			"title":"Orders by status",
			"type":"bar",
			"dimensions":[{"field":"orders.status"}],
			"measures":[{"field":"order_count"}],
			"limit":10
		}`),
	})
	if err != nil {
		t.Fatalf("run query_visual: %v", err)
	}
	if result.IsError {
		t.Fatalf("query_visual returned error: %#v", result.Content)
	}
	compact, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(compact), "delivered") || strings.Contains(string(compact), `"patch"`) || strings.Contains(string(compact), `"data"`) {
		t.Fatalf("model-visible chart result should be compact: %s", compact)
	}
	var compactResult struct {
		OK      bool   `json:"ok"`
		Kind    string `json:"kind"`
		ID      string `json:"id"`
		Summary string `json:"summary"`
		Signal  string `json:"signal"`
	}
	if err := json.Unmarshal(compact, &compactResult); err != nil {
		t.Fatalf("decode compact result: %v body=%s", err, compact)
	}
	if compactResult.ID != "agent_chart_call_1" {
		t.Fatalf("chart artifact id = %q, want call-scoped id", compactResult.ID)
	}
	body, err := json.Marshal(result.DisplayContent)
	if err != nil {
		t.Fatalf("marshal display result: %v", err)
	}
	var decoded struct {
		Kind  string `json:"kind"`
		ID    string `json:"id"`
		Patch struct {
			Visuals map[string]dashboard.Visual `json:"visuals"`
		} `json:"patch"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode result: %v body=%s", err, body)
	}
	if !compactResult.OK || compactResult.Kind != "chart" || compactResult.ID != decoded.ID || compactResult.Signal != "visuals."+decoded.ID {
		t.Fatalf("compact result = %#v decoded=%#v", compactResult, decoded)
	}
	visual := decoded.Patch.Visuals[decoded.ID]
	if decoded.Kind != "chart" || visual.ID != decoded.ID || visual.Title != "Orders by status" || visual.Type != "bar" || len(visual.Data) != 2 {
		t.Fatalf("chart result = %#v visual=%#v", decoded, visual)
	}
	if visual.Data[0]["label"] == nil || visual.Data[0]["value"] == nil {
		t.Fatalf("chart data does not use dashboard datum shape: %#v", visual.Data)
	}
	if len(visual.Interaction.Mappings) != 0 || len(visual.Selection) != 0 {
		t.Fatalf("chart should not include interactivity: %#v", visual)
	}
}

func TestAgentVisualToolReturnsTablePatchFromSemanticData(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tool := server.agentVisualToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal", DevAuthBypass: true})[0]
	result, err := tool.Handler.Run(context.Background(), agent.ToolCall{
		ID:   "call_1",
		Name: "query_visual",
		Arguments: json.RawMessage(`{
			"kind":"table",
			"model":"test",
			"dataset":"orders",
			"title":"Orders",
			"fields":[{"field":"orders.order_id"},{"field":"orders.status"}],
			"limit":10
		}`),
	})
	if err != nil {
		t.Fatalf("run query_visual: %v", err)
	}
	if result.IsError {
		t.Fatalf("query_visual returned error: %#v", result.Content)
	}
	compact, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(compact), "delivered") || strings.Contains(string(compact), `"patch"`) || strings.Contains(string(compact), `"rows"`) {
		t.Fatalf("model-visible table result should be compact: %s", compact)
	}
	var compactResult struct {
		OK     bool   `json:"ok"`
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Signal string `json:"signal"`
	}
	if err := json.Unmarshal(compact, &compactResult); err != nil {
		t.Fatalf("decode compact result: %v body=%s", err, compact)
	}
	if compactResult.ID != "agent_table_call_1" {
		t.Fatalf("table artifact id = %q, want call-scoped id", compactResult.ID)
	}
	body, err := json.Marshal(result.DisplayContent)
	if err != nil {
		t.Fatalf("marshal display result: %v", err)
	}
	var decoded struct {
		Kind  string `json:"kind"`
		ID    string `json:"id"`
		Patch struct {
			Tables map[string]dashboard.Table `json:"tables"`
		} `json:"patch"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode result: %v body=%s", err, body)
	}
	if !compactResult.OK || compactResult.Kind != "table" || compactResult.ID != decoded.ID || compactResult.Signal != "tables."+decoded.ID {
		t.Fatalf("compact result = %#v decoded=%#v", compactResult, decoded)
	}
	table := decoded.Patch.Tables[decoded.ID]
	if decoded.Kind != "table" || table.Title != "Orders" || table.Kind != "data_table" || len(table.Columns) != 2 || len(table.Blocks["a"].Rows) != 2 {
		t.Fatalf("table result = %#v table=%#v", decoded, table)
	}
	if len(table.Interaction.Mappings) != 0 || len(table.Selection) != 0 {
		t.Fatalf("table should not include interactivity: %#v", table)
	}
}

func TestAgentVisualToolReturnsAggregateTableFromRowsAndMeasures(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tool := server.agentVisualToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal", DevAuthBypass: true})[0]
	result, err := tool.Handler.Run(context.Background(), agent.ToolCall{
		ID:   "call_1",
		Name: "query_visual",
		Arguments: json.RawMessage(`{
			"kind":"table",
			"model":"test",
			"dataset":"orders",
			"title":"Orders by status",
			"rows":[{"field":"orders.status"}],
			"measures":[{"field":"order_count"}],
			"limit":10
		}`),
	})
	if err != nil {
		t.Fatalf("run query_visual: %v", err)
	}
	if result.IsError {
		t.Fatalf("query_visual returned error: %#v", result.Content)
	}
	body, err := json.Marshal(result.DisplayContent)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var decoded struct {
		ID    string `json:"id"`
		Patch struct {
			Tables map[string]dashboard.Table `json:"tables"`
		} `json:"patch"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode result: %v body=%s", err, body)
	}
	table := decoded.Patch.Tables[decoded.ID]
	if len(table.Columns) != 2 || table.Columns[0].Key != "status" || table.Columns[1].Key != "order_count" {
		t.Fatalf("aggregate table columns = %#v", table.Columns)
	}
	if got := table.Blocks["a"].Rows[0]["order_count"]; got == nil {
		t.Fatalf("aggregate table rows missing measure: %#v", table.Blocks["a"].Rows)
	}
}

func TestAgentVisualToolUsesToolCallScopedArtifactIDs(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tool := server.agentVisualToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal", DevAuthBypass: true})[0]
	args := json.RawMessage(`{
		"kind":"chart",
		"model":"test",
		"dataset":"orders",
		"title":"Orders by status",
		"type":"bar",
		"dimensions":[{"field":"orders.status"}],
		"measures":[{"field":"order_count"}],
		"limit":10
	}`)
	first, err := tool.Handler.Run(context.Background(), agent.ToolCall{ID: "call_first", Name: "query_visual", Arguments: args})
	if err != nil {
		t.Fatalf("run first query_visual: %v", err)
	}
	second, err := tool.Handler.Run(context.Background(), agent.ToolCall{ID: "call_second", Name: "query_visual", Arguments: args})
	if err != nil {
		t.Fatalf("run second query_visual: %v", err)
	}
	firstBody, _ := json.Marshal(first.Content)
	secondBody, _ := json.Marshal(second.Content)
	var firstCompact, secondCompact struct {
		ID     string `json:"id"`
		Signal string `json:"signal"`
	}
	if err := json.Unmarshal(firstBody, &firstCompact); err != nil {
		t.Fatalf("decode first compact: %v", err)
	}
	if err := json.Unmarshal(secondBody, &secondCompact); err != nil {
		t.Fatalf("decode second compact: %v", err)
	}
	if firstCompact.ID == secondCompact.ID || firstCompact.Signal == secondCompact.Signal {
		t.Fatalf("identical requests reused artifact identity: first=%#v second=%#v", firstCompact, secondCompact)
	}
	if firstCompact.ID != "agent_chart_call_first" || secondCompact.ID != "agent_chart_call_second" {
		t.Fatalf("unexpected call-scoped IDs: first=%#v second=%#v", firstCompact, secondCompact)
	}
}

func TestAgentVisualToolRejectsInlineDataAndFilters(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	for _, args := range []string{
		`{"kind":"chart","model":"test","dataset":"orders","data":[{"label":"x","value":1}],"measures":[{"field":"order_count"}]}`,
		`{"kind":"chart","model":"test","dataset":"orders","filters":{"controls":{}},"measures":[{"field":"order_count"}]}`,
		`{"kind":"table","model":"test","dataset":"orders","interaction":{"row_selection":{}},"fields":[{"field":"orders.order_id"}]}`,
	} {
		result := server.runAgentVisualTool(context.Background(), agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal", DevAuthBypass: true}, agent.ToolCall{ID: "call_1", Name: "query_visual", Arguments: json.RawMessage(args)})
		if !result.IsError {
			t.Fatalf("query_visual accepted forbidden input %s: %#v", args, result.Content)
		}
	}
}

func TestAPIGenAgentSearchToolInjectsDefaultLimit(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var search agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "search_workspace" {
			search = tool
			break
		}
	}
	if search.Handler == nil {
		t.Fatal("search_workspace tool missing")
	}
	result, err := search.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "search_workspace",
		Arguments: json.RawMessage(`{"q":"orders"}`),
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
		Count      int              `json:"count"`
		Items      []map[string]any `json:"items"`
		HasMore    bool             `json:"hasMore"`
		NextCursor string           `json:"nextCursor"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode result: %v body=%s", err, body)
	}
	if decoded.Count != len(decoded.Items) || len(decoded.Items) == 0 || len(decoded.Items) > 10 {
		t.Fatalf("search result count = %d, want 1..10: %#v", len(decoded.Items), decoded.Items)
	}
	if decoded.HasMore || decoded.NextCursor != "" {
		t.Fatalf("search should not expose empty cursor metadata: %#v", decoded)
	}
	for _, item := range decoded.Items {
		for _, forbidden := range []string{"dashboardId", "pageId", "visualId", "tableId", "filterId", "modelId", "datasetId", "fieldId", "assetId"} {
			if _, ok := item[forbidden]; ok {
				t.Fatalf("search item kept metadata field %q: %#v", forbidden, item)
			}
		}
		if _, ok := item["name"]; !ok {
			t.Fatalf("search item missing concise name: %#v", item)
		}
		if _, ok := item["description"]; !ok {
			t.Fatalf("search item missing concise description: %#v", item)
		}
		if _, ok := item["type"]; !ok {
			t.Fatalf("search item missing concise type: %#v", item)
		}
	}
}

func TestAPIGenAgentListWorkspacesUsesDeclarativeOutputShape(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var listWorkspaces agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "list_workspaces" {
			listWorkspaces = tool
			break
		}
	}
	if listWorkspaces.Handler == nil {
		t.Fatal("list_workspaces tool missing")
	}
	result, err := listWorkspaces.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "list_workspaces",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("run list_workspaces: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	body, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal result content: %v", err)
	}
	var decoded struct {
		Count      int              `json:"count"`
		Items      []map[string]any `json:"items"`
		HasMore    bool             `json:"hasMore"`
		NextCursor string           `json:"nextCursor"`
		Page       any              `json:"page"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode result: %v body=%s", err, body)
	}
	if decoded.Count != len(decoded.Items) || decoded.Count == 0 || decoded.HasMore || decoded.NextCursor != "" || decoded.Page != nil {
		t.Fatalf("workspace shaped result metadata = %#v body=%s", decoded, body)
	}
	for _, item := range decoded.Items {
		if _, ok := item["id"]; !ok {
			t.Fatalf("workspace item missing id: %#v", item)
		}
		if _, ok := item["title"]; !ok {
			t.Fatalf("workspace item missing title: %#v", item)
		}
		if _, ok := item["description"]; !ok {
			t.Fatalf("workspace item missing description: %#v", item)
		}
		for _, forbidden := range []string{"activeServingStateId", "createdAt", "updatedAt"} {
			if _, ok := item[forbidden]; ok {
				t.Fatalf("workspace item kept noisy metadata field %q: %#v", forbidden, item)
			}
		}
	}
}

func TestAPIGenAgentOutputShapeKeepsNonEmptyCursor(t *testing.T) {
	shaped := shapeAPIGenAgentToolContent(map[string]any{
		"items": []any{
			map[string]any{"id": "one", "title": "One", "description": "First", "createdAt": "ignored"},
		},
		"page": map[string]any{"nextCursor": "cursor-1"},
	}, agenttools.Output{
		ItemsPath:  "items",
		Fields:     []string{"id", "title"},
		CursorPath: "page.nextCursor",
		Count:      true,
	})
	decoded, ok := shaped.(map[string]any)
	if !ok {
		t.Fatalf("shaped content type = %T", shaped)
	}
	if decoded["count"] != 1 || decoded["hasMore"] != true || decoded["nextCursor"] != "cursor-1" {
		t.Fatalf("shaped cursor metadata = %#v", decoded)
	}
	if _, ok := decoded["page"]; ok {
		t.Fatalf("shaped output kept raw page metadata: %#v", decoded)
	}
}

func TestAPIGenAgentOutputShapeProjectsRootsCollectionsAndMaps(t *testing.T) {
	shaped := shapeAPIGenAgentToolContent(map[string]any{
		"title":         "Orders",
		"availableRows": 7,
		"style":         map[string]any{"density": "compact"},
		"blocks": map[string]any{
			"a": map[string]any{
				"rows": []any{
					map[string]any{"order_id": "o1", "status": "delivered", "internal": true},
					map[string]any{"order_id": "o2", "status": "shipped", "internal": false},
				},
			},
		},
		"visuals": map[string]any{
			"orders": map[string]any{
				"id":              "orders",
				"title":           "Orders",
				"type":            "bar",
				"rendererOptions": map[string]any{"hidden": true},
				"data": []any{
					map[string]any{"label": "delivered", "value": 1, "extra": "ignored"},
				},
			},
		},
	}, agenttools.Output{
		RootFields: []string{"title", "availableRows"},
		Collections: []agenttools.OutputCollection{{
			Path:   "blocks.a.rows",
			As:     "rows",
			Fields: []string{"order_id", "status"},
			Count:  true,
		}},
		Maps: []agenttools.OutputMap{{
			Path:   "visuals",
			As:     "visuals",
			Fields: []string{"id", "title", "type"},
			Collection: agenttools.OutputCollection{
				Path:  "data",
				As:    "data",
				Count: true,
			},
		}},
	})
	decoded, ok := shaped.(map[string]any)
	if !ok {
		t.Fatalf("shaped content type = %T", shaped)
	}
	if decoded["title"] != "Orders" || decoded["availableRows"] != 7 || decoded["count"] != 2 || decoded["hasMore"] != true {
		t.Fatalf("root/count metadata = %#v", decoded)
	}
	rows, ok := decoded["rows"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("rows = %#v", decoded["rows"])
	}
	firstRow := rows[0].(map[string]any)
	if firstRow["order_id"] != "o1" || firstRow["status"] != "delivered" {
		t.Fatalf("projected row = %#v", firstRow)
	}
	if _, ok := firstRow["internal"]; ok {
		t.Fatalf("row kept noisy field: %#v", firstRow)
	}
	visuals := decoded["visuals"].(map[string]any)
	orders := visuals["orders"].(map[string]any)
	if orders["id"] != "orders" || orders["title"] != "Orders" || orders["type"] != "bar" || orders["count"] != 1 {
		t.Fatalf("projected visual = %#v", orders)
	}
	if _, ok := orders["rendererOptions"]; ok {
		t.Fatalf("visual kept renderer options: %#v", orders)
	}
}

func TestAPIGenAgentOutputShapeOmitsMissingPathsAndFallbackRawWithoutMetadata(t *testing.T) {
	raw := map[string]any{"body": "raw"}
	if shaped := shapeAPIGenAgentToolContent(raw, agenttools.Output{}); !reflect.DeepEqual(shaped, raw) {
		t.Fatalf("empty output metadata should preserve raw content: %#v", shaped)
	}
	shaped := shapeAPIGenAgentToolContent(raw, agenttools.Output{
		RootFields:  []string{"missing"},
		Collections: []agenttools.OutputCollection{{Path: "items", As: "items", Count: true}},
	})
	decoded, ok := shaped.(map[string]any)
	if !ok {
		t.Fatalf("shaped missing-path content type = %T", shaped)
	}
	if len(decoded) != 0 {
		t.Fatalf("missing paths should be omitted without falling back to raw: %#v", decoded)
	}
}

func TestAPIGenAgentToolsExposeTypeSpecArgumentNamesAndBodyFields(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	names := map[string]agent.ToolDefinition{}
	for _, tool := range tools {
		names[tool.Name] = tool
		schemaText := string(tool.InputSchema)
		for _, forbidden := range []string{`"example"`, `"format"`, `"$ref"`, `"$defs"`, `"oneOf"`, `"anyOf"`, `"allOf"`} {
			if strings.Contains(schemaText, forbidden) {
				t.Fatalf("%s schema contains non-portable keyword %s: %s", tool.Name, forbidden, schemaText)
			}
		}
	}
	for toolName, wantProps := range map[string][]string{
		"describe_dashboard":            {"dashboard"},
		"describe_dashboard_visual":     {"dashboard", "page", "visual"},
		"describe_asset":                {"assetId"},
		"asset_lineage":                 {"assetId"},
		"describe_model":                {"model"},
		"list_dashboard_components":     {"dashboard", "page", "limit", "pageToken"},
		"list_dashboard_filter_options": {"dashboard", "page", "filter", "filters", "limit", "pageToken"},
		"query_dashboard_page":          {"dashboard", "page", "filters"},
		"query_dashboard_table_data":    {"dashboard", "page", "table", "count", "filters"},
		"query_dashboard_visual_data":   {"dashboard", "page", "visual", "filters"},
		"query_semantic_dataset":        {"model", "dataset", "dimensions", "measures", "filters", "sort", "limit", "pageToken"},
		"query_table":                   {"dashboard", "table", "count", "filters", "pageId"},
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

func TestAPIGenAgentOperationsDeclareOutputMetadata(t *testing.T) {
	for _, operation := range agenttools.APIGenOperations() {
		if outputMetadataEmpty(operation.Extension.Output) {
			t.Fatalf("agent operation %s (%s) does not declare x-agent.output metadata", operation.Contract.OperationID, operation.Extension.Name)
		}
	}
}

func TestAPIGenAgentToolDispatchesThroughGeneratedOperation(t *testing.T) {
	catalog := testAgentAssetCatalogFromProvider(t, manyEdgesMetrics{})
	server := NewWithOptions(manyEdgesMetrics{}, Options{
		AssetCatalog:       fakeAssetCatalogReader{catalog: catalog},
		DefaultWorkspaceID: "test",
	})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var listAssets agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "list_assets" {
			listAssets = tool
			break
		}
	}
	if listAssets.Handler == nil {
		t.Fatal("list_assets tool missing")
	}

	result, err := listAssets.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "list_assets",
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
		Count int `json:"count"`
		Items []struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode result: %v\n%s", err, body)
	}
	if decoded.Count != 1 || len(decoded.Items) != 1 || decoded.Items[0].ID != "dashboard:dashboard-0" || decoded.Items[0].Type != "dashboard" || decoded.Items[0].Title == "" {
		t.Fatalf("tool result = %#v", decoded)
	}
}

func TestAPIGenAgentDescribeDashboardVisualUsesDeclarativeOutputShape(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var describeVisual agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "describe_dashboard_visual" {
			describeVisual = tool
			break
		}
	}
	if describeVisual.Handler == nil {
		t.Fatal("describe_dashboard_visual tool missing")
	}
	result, err := describeVisual.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "describe_dashboard_visual",
		Arguments: json.RawMessage(`{"dashboard":"executive-sales","page":"overview","visual":"orders"}`),
	})
	if err != nil {
		t.Fatalf("run describe_dashboard_visual: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	body, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal result content: %v", err)
	}
	var visual map[string]any
	if err := json.Unmarshal(body, &visual); err != nil {
		t.Fatalf("decode visual result: %v body=%s", err, body)
	}
	for _, want := range []string{"id", "title", "kind", "type", "query"} {
		if _, ok := visual[want]; !ok {
			t.Fatalf("visual result missing %q: %#v", want, visual)
		}
	}
	for _, forbidden := range []string{"componentId", "renderer", "rendererOptions", "options", "interaction", "placement", "x", "y", "width", "height"} {
		if _, ok := visual[forbidden]; ok {
			t.Fatalf("visual result kept noisy field %q: %#v", forbidden, visual)
		}
	}
}

func TestAPIGenAgentAssetDescribeAndLineageToolsUseTypeSpecContracts(t *testing.T) {
	catalog := testAgentAssetCatalog(t)
	server := NewWithOptions(fakeMetrics{}, Options{
		AssetCatalog:       fakeAssetCatalogReader{catalog: catalog},
		DefaultWorkspaceID: "test",
	})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	names := map[string]agent.ToolDefinition{}
	for _, tool := range tools {
		names[tool.Name] = tool
	}
	for _, want := range []string{"describe_asset", "asset_lineage"} {
		if names[want].Handler == nil {
			t.Fatalf("missing asset tool %q in %#v", want, toolNames(tools))
		}
	}

	result, err := names["describe_asset"].Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_2",
		Name:      "describe_asset",
		Arguments: json.RawMessage(`{"assetId":"visual:executive-sales.revenue"}`),
	})
	if err != nil {
		t.Fatalf("run describe_asset: %v", err)
	}
	body, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal describe_asset: %v", err)
	}
	var described struct {
		ID            string `json:"id"`
		Type          string `json:"type"`
		Title         string `json:"title"`
		Description   string `json:"description"`
		PayloadSchema string `json:"payloadSchema"`
	}
	if err := json.Unmarshal(body, &described); err != nil {
		t.Fatalf("decode describe_asset: %v body=%s", err, body)
	}
	if described.ID != "visual:executive-sales.revenue" || described.Type != "visual" || described.Title != "Revenue" || described.PayloadSchema != "visual.v1" {
		t.Fatalf("describe_asset result = %#v", described)
	}
	var describedMap map[string]any
	if err := json.Unmarshal(body, &describedMap); err != nil {
		t.Fatalf("decode describe_asset map: %v", err)
	}
	for _, forbidden := range []string{"snapshotId", "workspaceId", "servingStateId", "payload", "key", "sourceFile"} {
		if _, ok := describedMap[forbidden]; ok {
			t.Fatalf("describe_asset kept noisy field %q: %#v", forbidden, describedMap)
		}
	}

	result, err = names["asset_lineage"].Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_3",
		Name:      "asset_lineage",
		Arguments: json.RawMessage(`{"assetId":"visual:executive-sales.revenue"}`),
	})
	if err != nil {
		t.Fatalf("run asset_lineage: %v", err)
	}
	body, err = json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal asset_lineage: %v", err)
	}
	var lineage struct {
		AssetID    string   `json:"assetId"`
		Upstream   []string `json:"upstream"`
		Downstream []string `json:"downstream"`
	}
	if err := json.Unmarshal(body, &lineage); err != nil {
		t.Fatalf("decode asset_lineage: %v body=%s", err, body)
	}
	if lineage.AssetID != "visual:executive-sales.revenue" || !stringSliceHas(lineage.Upstream, "dashboard:executive-sales") || !stringSliceHas(lineage.Downstream, "measure:olist.revenue") {
		t.Fatalf("asset_lineage result = %#v", lineage)
	}
}

func TestAPIGenAgentQueryDashboardPageUsesDeclarativeOutputShape(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var queryPage agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "query_dashboard_page" {
			queryPage = tool
			break
		}
	}
	if queryPage.Handler == nil {
		t.Fatal("query_dashboard_page tool missing")
	}
	result, err := queryPage.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "query_dashboard_page",
		Arguments: json.RawMessage(`{"dashboard":"executive-sales","page":"overview"}`),
	})
	if err != nil {
		t.Fatalf("run query_dashboard_page: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	body, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("marshal result content: %v", err)
	}
	var page map[string]any
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("decode page result: %v body=%s", err, body)
	}
	visuals, ok := page["visuals"].(map[string]any)
	if !ok || len(visuals) == 0 {
		t.Fatalf("page result missing compact visuals map: %#v", page)
	}
	orders := visuals["orders"].(map[string]any)
	if orders["title"] != "Orders" || orders["count"] != float64(1) {
		t.Fatalf("compact visual = %#v", orders)
	}
	for _, forbidden := range []string{"filters", "filterOptions", "status"} {
		if _, ok := page[forbidden]; ok {
			t.Fatalf("page result kept noisy field %q: %#v", forbidden, page)
		}
	}
	for _, forbidden := range []string{"rendererOptions", "options", "interaction", "selection", "version"} {
		if _, ok := orders[forbidden]; ok {
			t.Fatalf("compact visual kept noisy field %q: %#v", forbidden, orders)
		}
	}
}

func TestAPIGenAgentListToolInjectsDefaultLimit(t *testing.T) {
	catalog := testAgentAssetCatalogFromProvider(t, manyEdgesMetrics{})
	server := NewWithOptions(manyEdgesMetrics{}, Options{
		AssetCatalog:       fakeAssetCatalogReader{catalog: catalog},
		DefaultWorkspaceID: "test",
	})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var listEdges agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "list_workspace_asset_edges" {
			listEdges = tool
			break
		}
	}
	if listEdges.Handler == nil {
		t.Fatal("list_workspace_asset_edges tool missing")
	}
	result, err := listEdges.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "list_workspace_asset_edges",
		Arguments: json.RawMessage(`{}`),
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
		Count      int              `json:"count"`
		Items      []map[string]any `json:"items"`
		HasMore    bool             `json:"hasMore"`
		NextCursor string           `json:"nextCursor"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode result: %v body=%s", err, body)
	}
	if decoded.Count != 25 || len(decoded.Items) != 25 || !decoded.HasMore || decoded.NextCursor == "" {
		t.Fatalf("default-limited edge result = %#v", decoded)
	}
	for _, item := range decoded.Items {
		for _, want := range []string{"fromAssetId", "toAssetId", "type"} {
			if _, ok := item[want]; !ok {
				t.Fatalf("edge item missing %q: %#v", want, item)
			}
		}
		if _, ok := item["servingStateId"]; ok {
			t.Fatalf("edge item kept noisy metadata: %#v", item)
		}
	}

	result, err = listEdges.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_2",
		Name:      "list_workspace_asset_edges",
		Arguments: json.RawMessage(`{"limit":3}`),
	})
	if err != nil {
		t.Fatalf("run explicit limit tool: %v", err)
	}
	body, _ = json.Marshal(result.Content)
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode explicit result: %v body=%s", err, body)
	}
	if decoded.Count != 3 || len(decoded.Items) != 3 {
		t.Fatalf("explicit-limited edge result = %#v", decoded)
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
		AvailableRows int              `json:"availableRows"`
		Count         int              `json:"count"`
		HasMore       bool             `json:"hasMore"`
		Columns       []map[string]any `json:"columns"`
		Rows          []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(body, &table); err != nil {
		t.Fatalf("decode table result: %v\n%s", err, body)
	}
	if table.AvailableRows != 50 || table.Count != 50 || len(table.Rows) != 50 || len(table.Columns) == 0 {
		t.Fatalf("table result was not capped to 50: %#v", table)
	}
	var tableMap map[string]any
	if err := json.Unmarshal(body, &tableMap); err != nil {
		t.Fatalf("decode table map: %v", err)
	}
	for _, forbidden := range []string{"blocks", "style", "interaction", "selection", "chunkSize", "rowHeight", "resetVersion", "loadingBlock"} {
		if _, ok := tableMap[forbidden]; ok {
			t.Fatalf("table result kept noisy field %q: %#v", forbidden, tableMap)
		}
	}
}

func TestAPIGenAgentToolFetchesSingleDashboardVisualData(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var queryVisual agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "query_dashboard_visual_data" {
			queryVisual = tool
			break
		}
	}
	if queryVisual.Handler == nil {
		t.Fatal("query_dashboard_visual_data tool missing")
	}
	result, err := queryVisual.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "query_dashboard_visual_data",
		Arguments: json.RawMessage(`{"dashboard":"executive-sales","page":"overview","visual":"orders"}`),
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
	var visual struct {
		Title string           `json:"title"`
		Count int              `json:"count"`
		Data  []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &visual); err != nil {
		t.Fatalf("decode visual result: %v body=%s", err, body)
	}
	if visual.Title != "Orders" || visual.Count != 1 || len(visual.Data) != 1 {
		t.Fatalf("visual result = %#v", visual)
	}
	var visualMap map[string]any
	if err := json.Unmarshal(body, &visualMap); err != nil {
		t.Fatalf("decode visual map: %v", err)
	}
	for _, forbidden := range []string{"renderer", "rendererOptions", "options", "interaction", "selection", "version"} {
		if _, ok := visualMap[forbidden]; ok {
			t.Fatalf("visual result kept noisy field %q: %#v", forbidden, visualMap)
		}
	}
}

func TestAPIGenAgentSemanticQueryToolInjectsBodyDefaultLimit(t *testing.T) {
	server := NewWithOptions(manySemanticRowsMetrics{}, Options{DefaultWorkspaceID: "test"})
	tools := server.agentAPIGenToolDefinitions(agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"})
	var querySemantic agent.ToolDefinition
	for _, tool := range tools {
		if tool.Name == "query_semantic_dataset" {
			querySemantic = tool
			break
		}
	}
	if querySemantic.Handler == nil {
		t.Fatal("query_semantic_dataset tool missing")
	}
	result, err := querySemantic.Handler.Run(context.Background(), agent.ToolCall{
		ID:        "call_1",
		Name:      "query_semantic_dataset",
		Arguments: json.RawMessage(`{"model":"test","dataset":"orders","dimensions":[{"field":"orders.status","alias":"status"}],"measures":[{"field":"order_count"}],"sort":[{"field":"status","direction":"asc"}]}`),
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
		Columns    []string         `json:"columns"`
		Items      []map[string]any `json:"items"`
		Count      int              `json:"count"`
		HasMore    bool             `json:"hasMore"`
		NextCursor string           `json:"nextCursor"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode semantic result: %v body=%s", err, body)
	}
	if len(decoded.Columns) == 0 || len(decoded.Items) != 25 || decoded.Count != 25 || !decoded.HasMore || decoded.NextCursor == "" {
		t.Fatalf("semantic default-limited result = %#v", decoded)
	}
	var decodedMap map[string]any
	if err := json.Unmarshal(body, &decodedMap); err != nil {
		t.Fatalf("decode semantic map: %v", err)
	}
	if _, ok := decodedMap["page"]; ok {
		t.Fatalf("semantic result kept raw page metadata: %#v", decodedMap)
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

func TestRuntimeAgentToolsMatchPolicyRegistry(t *testing.T) {
	server := NewWithOptions(manyRowsMetrics{}, Options{DefaultWorkspaceID: "test"})
	scope := agentapp.Scope{WorkspaceID: "test", PrincipalID: "principal"}
	var runtimeTools []agent.ToolDefinition
	runtimeTools = append(runtimeTools, server.agentVisualToolDefinitions(scope)...)
	runtimeTools = append(runtimeTools, server.agentAPIGenToolDefinitions(scope)...)
	if got, want := sortedToolNames(runtimeTools), agenttools.ToolNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime tools = %#v, policy registry = %#v", got, want)
	}
}

func toolNames(tools []agent.ToolDefinition) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func sortedToolNames(tools []agent.ToolDefinition) []string {
	names := toolNames(tools)
	sort.Strings(names)
	return names
}

func outputMetadataEmpty(output agenttools.Output) bool {
	return output.ItemsPath == "" &&
		len(output.Fields) == 0 &&
		output.CursorPath == "" &&
		!output.Count &&
		len(output.RootFields) == 0 &&
		len(output.Collections) == 0 &&
		len(output.Maps) == 0
}

type fakeAssetCatalogReader struct {
	catalog workspace.AssetCatalog
	ok      bool
	err     error
}

func (f fakeAssetCatalogReader) ActiveAssetCatalog(_ context.Context, _ workspace.WorkspaceID, _ string) (workspace.AssetCatalog, bool, error) {
	if f.err != nil {
		return workspace.AssetCatalog{}, false, f.err
	}
	ok := f.ok
	if !ok && (len(f.catalog.Assets) > 0 || len(f.catalog.Edges) > 0) {
		ok = true
	}
	return f.catalog, ok, nil
}

func testAgentAssetCatalog(t *testing.T) workspace.AssetCatalog {
	t.Helper()
	workspaceID := workspace.WorkspaceID("test")
	servingStateID := workspace.ServingStateID("deploy_a")
	dashboard, err := workspace.NewAsset(workspaceID, servingStateID, workspace.AssetTypeDashboard, "executive-sales", "", "Executive Sales", "", "dashboard.v1", map[string]any{"semantic_model": "olist"})
	if err != nil {
		t.Fatalf("dashboard asset: %v", err)
	}
	measure, err := workspace.NewAsset(workspaceID, servingStateID, workspace.AssetTypeMeasure, "olist.revenue", "", "Revenue", "", "measure.v1", map[string]any{"table": "orders"})
	if err != nil {
		t.Fatalf("measure asset: %v", err)
	}
	visual, err := workspace.NewAsset(workspaceID, servingStateID, workspace.AssetTypeVisual, "executive-sales.revenue", dashboard.ID, "Revenue", "", "visual.v1", map[string]any{"query_kind": "aggregate"})
	if err != nil {
		t.Fatalf("visual asset: %v", err)
	}
	graph := workspace.AssetGraph{
		Assets: []workspace.Asset{dashboard, measure, visual},
		Edges: []workspace.AssetEdge{
			workspace.NewAssetEdge(workspaceID, servingStateID, dashboard.ID, visual.ID, workspace.AssetEdgeContains),
			workspace.NewAssetEdge(workspaceID, servingStateID, visual.ID, measure.ID, workspace.AssetEdgeUsesMeasure),
		},
	}
	catalog, err := workspace.DecodeAssetCatalog(graph)
	if err != nil {
		t.Fatalf("decode asset catalog: %v", err)
	}
	return catalog
}

func testAgentAssetCatalogFromProvider(t *testing.T, provider workspaceAssetGraphProvider) workspace.AssetCatalog {
	t.Helper()
	assets, edges, ok := provider.WorkspaceAssets("test", "deploy_a")
	if !ok {
		t.Fatal("workspace assets unavailable")
	}
	catalog, err := workspace.DecodeAssetCatalog(workspace.AssetGraph{Assets: assets, Edges: edges})
	if err != nil {
		t.Fatalf("decode asset catalog: %v", err)
	}
	return catalog
}

func stringSliceHas(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type manyEdgesMetrics struct {
	fakeMetrics
}

type manySemanticRowsMetrics struct {
	fakeMetrics
}

func (manySemanticRowsMetrics) QuerySemantic(_ context.Context, _ string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	rows := make(reportdef.QueryRows, 0, request.Limit)
	for i := 0; i < request.Limit; i++ {
		rows = append(rows, reportdef.QueryRow{"status": "s" + strconv.Itoa(i), "order_count": i})
	}
	return rows, nil
}

func (m manySemanticRowsMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	if request.Kind != dataquery.KindSemanticAggregate {
		return m.fakeMetrics.ExecuteDataQuery(ctx, request)
	}
	rows, err := m.QuerySemantic(ctx, request.ModelID, reportdef.AggregateQuery{Limit: request.Limit})
	if err != nil {
		return dataquery.Result{}, err
	}
	out := make([]dataquery.Row, 0, len(rows))
	for _, row := range rows {
		out = append(out, dataquery.Row(row))
	}
	return dataquery.Result{Columns: dataquery.ColumnsFromNames([]string{"status", "order_count"}), Rows: out}, nil
}

func (manyEdgesMetrics) WorkspaceAssets(workspaceID, servingStateID string) ([]workspace.Asset, []workspace.AssetEdge, bool) {
	root, err := workspace.NewAsset(workspace.WorkspaceID(workspaceID), workspace.ServingStateID(servingStateID), workspace.AssetTypeCatalog, "catalog", "", "Catalog", "", "catalog.v1", map[string]any{})
	if err != nil {
		return nil, nil, false
	}
	assets := []workspace.Asset{root}
	edges := make([]workspace.AssetEdge, 0, 30)
	for i := 0; i < 30; i++ {
		key := "dashboard-" + strconv.Itoa(i)
		child, err := workspace.NewAsset(workspace.WorkspaceID(workspaceID), workspace.ServingStateID(servingStateID), workspace.AssetTypeDashboard, key, root.ID, "Dashboard", "", "dashboard.v1", map[string]any{"index": i})
		if err != nil {
			return nil, nil, false
		}
		assets = append(assets, child)
		edges = append(edges, workspace.NewAssetEdge(workspace.WorkspaceID(workspaceID), workspace.ServingStateID(servingStateID), root.ID, child.ID, workspace.AssetEdgeContains))
	}
	return assets, edges, true
}
