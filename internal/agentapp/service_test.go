package agentapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/semantic"
	"github.com/Yacobolo/libredash/pkg/agent"
)

func TestReadOnlyToolsExposeWorkspaceFactsAndBoundRows(t *testing.T) {
	service := NewService(fakeAgentMetrics{}, nil, Config{APIKey: "key", Model: "model"})
	tools := service.toolDefinitions(Scope{WorkspaceID: "test", PrincipalID: "principal"})

	list := runTool(t, tools, "list_dashboards", `{}`)
	if !strings.Contains(list, "executive-sales") {
		t.Fatalf("list_dashboards output = %s", list)
	}
	describe := runTool(t, tools, "describe_model", `{"model_id":"test"}`)
	if !strings.Contains(describe, "executive-sales") || !strings.Contains(describe, "Test Model") {
		t.Fatalf("describe_model output = %s", describe)
	}
	table := runTool(t, tools, "query_table", `{"dashboard_id":"executive-sales","page_id":"overview","table_id":"orders","count":500}`)
	var tableOut dashboard.Table
	if err := json.Unmarshal([]byte(table), &tableOut); err != nil {
		t.Fatalf("decode table output: %v", err)
	}
	if len(tableOut.Blocks["a"].Rows) != 50 {
		t.Fatalf("query_table rows were not capped to 50: %s", table)
	}
}

func TestReadOnlyToolPayloadShapesStayStable(t *testing.T) {
	service := NewService(fakeAgentMetrics{}, nil, Config{APIKey: "key", Model: "model"})
	tools := service.toolDefinitions(Scope{WorkspaceID: "test", PrincipalID: "principal"})

	var dashboards struct {
		Dashboards []dashboard.CatalogDashboard `json:"dashboards"`
	}
	if err := json.Unmarshal([]byte(runTool(t, tools, "list_dashboards", `{}`)), &dashboards); err != nil {
		t.Fatalf("decode dashboards: %v", err)
	}
	if len(dashboards.Dashboards) != 1 || dashboards.Dashboards[0].ID != "executive-sales" {
		t.Fatalf("dashboards payload = %#v", dashboards)
	}

	var models struct {
		Models []dashboard.CatalogModel `json:"models"`
	}
	if err := json.Unmarshal([]byte(runTool(t, tools, "list_semantic_models", `{}`)), &models); err != nil {
		t.Fatalf("decode semantic models: %v", err)
	}
	if len(models.Models) != 1 || models.Models[0].ID != "test" {
		t.Fatalf("semantic models payload = %#v", models)
	}

	var model struct {
		ID         string `json:"id"`
		Dashboards []struct {
			ID            string `json:"id"`
			SemanticModel string `json:"semantic_model"`
			Pages         int    `json:"pages"`
		} `json:"dashboards"`
		Counts *struct {
			Sources       int `json:"sources"`
			ModelTables   int `json:"model_tables"`
			Fields        int `json:"fields"`
			Measures      int `json:"measures"`
			Relationships int `json:"relationships"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(runTool(t, tools, "describe_model", `{"model_id":"test"}`)), &model); err != nil {
		t.Fatalf("decode model: %v", err)
	}
	if model.ID != "test" || len(model.Dashboards) != 1 || model.Dashboards[0].ID != "executive-sales" || model.Dashboards[0].SemanticModel != "test" || model.Counts == nil {
		t.Fatalf("model payload = %#v", model)
	}
}

func TestDescribeDashboardReturnsCompactManifest(t *testing.T) {
	service := NewService(largeDashboardMetrics{}, nil, Config{APIKey: "key", Model: "model"})
	tools := service.toolDefinitions(Scope{WorkspaceID: "test", PrincipalID: "principal"})

	output := runTool(t, tools, "describe_dashboard", `{"dashboard_id":"executive-sales"}`)
	if len(output) > 16*1024 {
		t.Fatalf("describe_dashboard output = %d bytes, want compact manifest under 16KiB", len(output))
	}
	if strings.Contains(output, largeDashboardPayloadMarker) {
		t.Fatalf("describe_dashboard leaked full visual/table definitions: %s", output[:min(len(output), 512)])
	}

	var got struct {
		ID     string `json:"id"`
		Counts struct {
			Pages   int `json:"pages"`
			Visuals int `json:"visuals"`
			Tables  int `json:"tables"`
		} `json:"counts"`
		Pages []struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			Components []struct {
				ID    string `json:"id"`
				Kind  string `json:"kind"`
				Ref   string `json:"ref"`
				Title string `json:"title"`
			} `json:"components"`
		} `json:"pages"`
		DetailTools map[string]string `json:"detail_tools"`
	}
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("decode describe_dashboard output: %v\n%s", err, output)
	}
	if got.ID != "executive-sales" || got.Counts.Pages != 24 || got.Counts.Visuals != 48 || got.Counts.Tables != 24 {
		t.Fatalf("manifest counts = %#v", got)
	}
	if len(got.Pages) != 24 || len(got.Pages[0].Components) != 3 {
		t.Fatalf("pages/components = %#v", got.Pages[:min(len(got.Pages), 2)])
	}
	if got.Pages[0].Components[0].Kind != "visual" || got.Pages[0].Components[0].Ref == "" || got.Pages[0].Components[0].Title == "" {
		t.Fatalf("visual component summary = %#v", got.Pages[0].Components[0])
	}
	if got.DetailTools["page_data"] != "query_dashboard_page" || got.DetailTools["model"] != "describe_model" {
		t.Fatalf("detail tools = %#v", got.DetailTools)
	}
}

func TestServicePromptPersistsRunEventsMessagesAndTranscript(t *testing.T) {
	ctx := context.Background()
	store := openAgentAppStore(t, ctx)
	defer store.Close()
	principal := createAgentAppPrincipal(t, ctx, store, "viewer@example.com")

	var calls atomic.Int64
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode model request: %v", err)
		}
		switch call {
		case 1:
			writeJSON(t, w, openAIChatResponse{Choices: []openAIChoice{{
				Message: openAIMessage{Role: "assistant", ToolCalls: []openAIToolCall{{
					ID:   "call_dashboards",
					Type: "function",
					Function: openAIFunctionCall{
						Name:      "list_dashboards",
						Arguments: `{}`,
					},
				}}},
				FinishReason: "tool_calls",
			}}, Usage: openAIUsage{PromptTokens: 20, CompletionTokens: 5, TotalTokens: 25}})
		case 2:
			if len(req.Messages) == 0 || req.Messages[len(req.Messages)-1].Role != "tool" {
				t.Fatalf("second request did not include tool result: %#v", req.Messages)
			}
			writeJSON(t, w, openAIChatResponse{Choices: []openAIChoice{{
				Message:      openAIMessage{Role: "assistant", Content: "You have Executive Sales available."},
				FinishReason: "stop",
			}}, Usage: openAIUsage{PromptTokens: 30, CompletionTokens: 9, TotalTokens: 39}})
		default:
			t.Fatalf("unexpected model call %d", call)
		}
	}))
	defer modelServer.Close()

	service := NewService(fakeAgentMetrics{}, store, Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	conversation, err := service.CreateConversation(ctx, Scope{WorkspaceID: "test", PrincipalID: principal.ID}, "Dashboards")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	result, err := service.Prompt(ctx, PromptInput{
		Scope:          Scope{WorkspaceID: "test", PrincipalID: principal.ID},
		ConversationID: conversation.ID,
		Input:          "What dashboards can I use?",
		CorrelationID:  "corr_1",
	})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if !strings.Contains(result.Content, "Executive Sales") || result.StopReason != agent.StopReasonCompleted {
		t.Fatalf("result = %#v", result)
	}
	messages, err := store.ListAgentMessages(ctx, "test", principal.ID, conversation.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want user/assistant/tool/assistant: %#v", len(messages), messages)
	}
	runs, err := store.ListAgentRuns(ctx, "test", principal.ID, conversation.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != result.RunID || runs[0].TotalTokens != 64 {
		t.Fatalf("runs = %#v result=%#v", runs, result)
	}
	events, err := store.ListAgentEvents(ctx, "test", principal.ID, result.RunID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) == 0 || events[0].Seq != 1 {
		t.Fatalf("events = %#v", events)
	}
	updated, err := store.GetAgentConversation(ctx, "test", principal.ID, conversation.ID)
	if err != nil {
		t.Fatalf("get updated conversation: %v", err)
	}
	if !strings.Contains(updated.TranscriptJson, "Executive Sales") {
		t.Fatalf("transcript was not updated: %s", updated.TranscriptJson)
	}
}

func TestServiceGenerateConversationTitleUsesNoToolsAndSavesCleanTitle(t *testing.T) {
	ctx := context.Background()
	store := openAgentAppStore(t, ctx)
	defer store.Close()
	principal := createAgentAppPrincipal(t, ctx, store, "viewer@example.com")
	var got openAIChatRequest
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode model request: %v", err)
		}
		writeJSON(t, w, openAIChatResponse{Choices: []openAIChoice{{
			Message:      openAIMessage{Role: "assistant", Content: "<think>private</think>\n\"Available dashboards.\""},
			FinishReason: "stop",
		}}, Usage: openAIUsage{PromptTokens: 12, CompletionTokens: 3, TotalTokens: 15}})
	}))
	defer modelServer.Close()

	scope := Scope{WorkspaceID: "test", PrincipalID: principal.ID}
	service := NewService(fakeAgentMetrics{}, store, Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	conversation, err := service.CreateConversation(ctx, scope, "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    scope.WorkspaceID,
		PrincipalID:    scope.PrincipalID,
		ConversationID: conversation.ID,
		Role:           platform.AgentMessageRoleUser,
		ContentText:    "What dashboards can I use?",
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	updated, err := service.GenerateConversationTitle(ctx, scope, conversation.ID)
	if err != nil {
		t.Fatalf("generate title: %v", err)
	}
	if updated.Title != "Available dashboards" {
		t.Fatalf("title = %q", updated.Title)
	}
	if got.Model != "fake-model" || got.MaxTokens != titleReserveOutputTokens {
		t.Fatalf("title model/max = %s/%d", got.Model, got.MaxTokens)
	}
	if got.Thinking != nil {
		t.Fatalf("non-deepseek title request should not include thinking config: %#v", got.Thinking)
	}
	if len(got.Tools) != 0 || got.ToolChoice != "" {
		t.Fatalf("title request should not include tools: %#v choice=%q", got.Tools, got.ToolChoice)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[1].Role != "user" {
		t.Fatalf("title messages = %#v", got.Messages)
	}
	if !strings.Contains(got.Messages[1].Content, "What dashboards can I use?") {
		t.Fatalf("title prompt did not include first user prompt: %#v", got.Messages)
	}
}

func TestServiceGenerateConversationTitleFallsBackWhenModelReturnsEmptyContent(t *testing.T) {
	ctx := context.Background()
	store := openAgentAppStore(t, ctx)
	defer store.Close()
	principal := createAgentAppPrincipal(t, ctx, store, "viewer@example.com")
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, openAIChatResponse{Choices: []openAIChoice{{
			Message:      openAIMessage{Role: "assistant", Content: ""},
			FinishReason: "length",
		}}, Usage: openAIUsage{PromptTokens: 12, CompletionTokens: 64, TotalTokens: 76}})
	}))
	defer modelServer.Close()

	scope := Scope{WorkspaceID: "test", PrincipalID: principal.ID}
	service := NewService(fakeAgentMetrics{}, store, Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	conversation, err := service.CreateConversation(ctx, scope, "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    scope.WorkspaceID,
		PrincipalID:    scope.PrincipalID,
		ConversationID: conversation.ID,
		Role:           platform.AgentMessageRoleUser,
		ContentText:    "how are you?",
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	updated, err := service.GenerateConversationTitle(ctx, scope, conversation.ID)
	if err != nil {
		t.Fatalf("generate title: %v", err)
	}
	if updated.Title != "How are you" {
		t.Fatalf("title = %q", updated.Title)
	}
}

func TestServiceGenerateConversationTitleIsBestEffortAndSkipsUnsafeCases(t *testing.T) {
	ctx := context.Background()
	store := openAgentAppStore(t, ctx)
	defer store.Close()
	principal := createAgentAppPrincipal(t, ctx, store, "viewer@example.com")
	scope := Scope{WorkspaceID: "test", PrincipalID: principal.ID}
	var calls atomic.Int64
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "provider down", http.StatusBadGateway)
	}))
	defer modelServer.Close()
	service := NewService(fakeAgentMetrics{}, store, Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})

	titled, err := service.CreateConversation(ctx, scope, "Manual title")
	if err != nil {
		t.Fatalf("create titled conversation: %v", err)
	}
	if updated, err := service.GenerateConversationTitle(ctx, scope, titled.ID); err != nil || updated.Title != "Manual title" {
		t.Fatalf("manual title changed or errored: updated=%#v err=%v", updated, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("model called for manual title")
	}

	multi, err := service.CreateConversation(ctx, scope, "")
	if err != nil {
		t.Fatalf("create multi conversation: %v", err)
	}
	for _, text := range []string{"hello", "again"} {
		if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
			WorkspaceID:    scope.WorkspaceID,
			PrincipalID:    scope.PrincipalID,
			ConversationID: multi.ID,
			Role:           platform.AgentMessageRoleUser,
			ContentText:    text,
		}); err != nil {
			t.Fatalf("append user message: %v", err)
		}
	}
	if updated, err := service.GenerateConversationTitle(ctx, scope, multi.ID); err != nil || updated.Title != platform.AgentConversationDefaultTitle {
		t.Fatalf("multi-user title changed or errored: updated=%#v err=%v", updated, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("model called for multi-user conversation")
	}

	failing, err := service.CreateConversation(ctx, scope, "")
	if err != nil {
		t.Fatalf("create failing conversation: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    scope.WorkspaceID,
		PrincipalID:    scope.PrincipalID,
		ConversationID: failing.ID,
		Role:           platform.AgentMessageRoleUser,
		ContentText:    "list semantic models",
	}); err != nil {
		t.Fatalf("append failing user message: %v", err)
	}
	if updated, err := service.GenerateConversationTitle(ctx, scope, failing.ID); err != nil || updated.Title != "List semantic models" {
		t.Fatalf("failing provider title changed or errored: updated=%#v err=%v", updated, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("model calls = %d, want 1 for failing provider", calls.Load())
	}
}

func TestServiceConversationTranscriptDerivesDisplayItems(t *testing.T) {
	ctx := context.Background()
	store := openAgentAppStore(t, ctx)
	defer store.Close()
	principal := createAgentAppPrincipal(t, ctx, store, "viewer@example.com")
	service := NewService(fakeAgentMetrics{}, store, Config{APIKey: "key", Model: "fake-model"})
	scope := Scope{WorkspaceID: "test", PrincipalID: principal.ID}
	conversation, err := service.CreateConversation(ctx, scope, "Transcript")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           platform.AgentMessageRoleUser,
		ContentText:    "hello",
	}); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           platform.AgentMessageRoleSummary,
		ContentText:    "internal summary",
	}); err != nil {
		t.Fatalf("append summary: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           platform.AgentMessageRoleAssistant,
		ContentText:    "Let me look that up.",
		ContentJSON:    `{"tool_calls":[{"id":"call_1","name":"list_dashboards","arguments":{"limit":2}}]}`,
	}); err != nil {
		t.Fatalf("append assistant tool call: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           platform.AgentMessageRoleTool,
		ContentText:    `{"summary":"Found 2 dashboards"}`,
		ToolCallID:     "call_1",
		ToolName:       "list_dashboards",
	}); err != nil {
		t.Fatalf("append tool: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           platform.AgentMessageRoleAssistant,
		ContentText:    "I found **two** dashboards.",
	}); err != nil {
		t.Fatalf("append assistant answer: %v", err)
	}

	transcript, err := service.ConversationTranscript(ctx, scope, conversation.ID)
	if err != nil {
		t.Fatalf("conversation transcript: %v", err)
	}
	if len(transcript) != 4 {
		t.Fatalf("transcript len = %d, want user/assistant/tool/assistant: %#v", len(transcript), transcript)
	}
	if transcript[0].Kind != "user" || transcript[0].Text != "hello" {
		t.Fatalf("user item = %#v", transcript[0])
	}
	if transcript[1].Kind != "assistant" || transcript[1].Markdown != "Let me look that up." {
		t.Fatalf("assistant preamble item = %#v", transcript[1])
	}
	if transcript[2].Kind != "tool" || transcript[2].ToolCallID != "call_1" || transcript[2].Status != "complete" || transcript[2].Summary != "Found 2 dashboards" || transcript[2].ResultSummary != "Found 2 dashboards" {
		t.Fatalf("tool item = %#v", transcript[2])
	}
	if strings.Contains(transcript[2].InputJSON, `"id"`) || strings.Contains(transcript[2].InputJSON, `"type"`) || !strings.Contains(transcript[2].InputJSON, `"name": "list_dashboards"`) || !strings.Contains(transcript[2].InputJSON, `"arguments": "{\"limit\":2}"`) {
		t.Fatalf("tool input preview = %q", transcript[2].InputJSON)
	}
	if !strings.Contains(transcript[2].ResultJSON, `"summary": "Found 2 dashboards"`) {
		t.Fatalf("tool result preview = %q", transcript[2].ResultJSON)
	}
	if transcript[3].Kind != "assistant" || !strings.Contains(transcript[3].Markdown, "two") {
		t.Fatalf("assistant item = %#v", transcript[3])
	}
	for _, item := range transcript {
		if strings.Contains(item.Text+item.Markdown+item.Summary, "internal summary") {
			t.Fatalf("summary leaked into transcript: %#v", transcript)
		}
	}
}

func TestServiceRejectsConcurrentConversationTurns(t *testing.T) {
	ctx := context.Background()
	store := openAgentAppStore(t, ctx)
	defer store.Close()
	principal := createAgentAppPrincipal(t, ctx, store, "viewer@example.com")
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writeJSON(t, w, openAIChatResponse{Choices: []openAIChoice{{
			Message:      openAIMessage{Role: "assistant", Content: "ok"},
			FinishReason: "stop",
		}}})
	}))
	defer modelServer.Close()

	service := NewService(fakeAgentMetrics{}, store, Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	conversation, err := service.CreateConversation(ctx, Scope{WorkspaceID: "test", PrincipalID: principal.ID}, "Dashboards")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := service.Prompt(ctx, PromptInput{
				Scope:          Scope{WorkspaceID: "test", PrincipalID: principal.ID},
				ConversationID: conversation.ID,
				Input:          "hello",
			})
			errs <- err
		}()
	}
	first := <-errs
	second := <-errs
	if !IsBusy(first) && !IsBusy(second) {
		t.Fatalf("errors = %v / %v, want one busy error", first, second)
	}
}

type fakeAgentMetrics struct{}

func (fakeAgentMetrics) Catalog() dashboard.Catalog {
	return dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{ID: "test", Title: "Test Workspace"},
		Models: []dashboard.CatalogModel{
			{ID: "test", Title: "Test Model", Description: "Fixture model"},
		},
		Dashboards: []dashboard.CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales", Description: "Sales dashboard", SemanticModel: "test", PageCount: 1},
		},
	}
}

func (fakeAgentMetrics) Report(id string) (semantic.Dashboard, *semantic.Model, bool) {
	if id != "executive-sales" {
		return semantic.Dashboard{}, nil, false
	}
	return semantic.Dashboard{
		ID:            "executive-sales",
		Title:         "Executive Sales",
		Description:   "Sales dashboard",
		SemanticModel: "test",
		Visuals: map[string]semantic.Visual{
			"orders": {Title: "Orders", Query: semantic.VisualQuery{Measures: []semantic.FieldRef{{Field: "order_count"}}}},
		},
		Tables: map[string]semantic.TableVisual{
			"orders": {Title: "Orders", Query: semantic.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}},
		},
		Pages: []dashboard.Page{{ID: "overview", Title: "Overview", Visuals: []dashboard.PageVisual{{ID: "orders", Visual: "orders"}, {ID: "orders-table", Table: "orders"}}}},
	}, fakeSemanticModel(), true
}

func fakeSemanticModel() *semantic.Model {
	return &semantic.Model{
		Name:      "test",
		Title:     "Test Model",
		BaseTable: "orders",
		Sources: map[string]semantic.Source{
			"orders": {Path: "orders.csv"},
		},
		Tables: map[string]semantic.ModelTable{
			"orders": {
				Kind:       "fact",
				Source:     "orders",
				PrimaryKey: "order_id",
				Dimensions: map[string]semantic.MetricDimension{
					"order_id": {Expr: "order_id"},
				},
			},
		},
		Measures: map[string]semantic.MetricMeasure{
			"order_count": {Table: "orders", Grain: "order_id", Expression: "COUNT(DISTINCT orders.order_id)"},
		},
	}
}

func (fakeAgentMetrics) Pages(id string) []dashboard.Page {
	report, _, ok := fakeAgentMetrics{}.Report(id)
	if !ok {
		return nil
	}
	return report.Pages
}

func (fakeAgentMetrics) DefaultFilters(string) dashboard.Filters {
	return dashboard.Filters{}.WithDefaults()
}

func (fakeAgentMetrics) NormalizeTableRequest(_ string, request dashboard.TableRequest) dashboard.TableRequest {
	return request.WithDefaults()
}

func (fakeAgentMetrics) QueryDashboardPage(_ context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return dashboard.Patch{
		Filters: filters.WithDefaults(),
		Visuals: map[string]dashboard.Visual{
			"orders": {ID: "orders", Title: "Orders", Data: []dashboard.Datum{{"label": "delivered", "value": 10}}},
		},
	}, nil
}

func (fakeAgentMetrics) QueryTablePage(_ context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	rows := make([]map[string]any, 0, request.Count)
	for i := 0; i < request.Count; i++ {
		rows = append(rows, map[string]any{"order_id": "order_" + string(rune('A'+i%26))})
	}
	return dashboard.Table{
		Title:         "Orders",
		AvailableRows: len(rows),
		Blocks:        map[string]dashboard.TableBlock{"a": {Rows: rows}},
	}, nil
}

const largeDashboardPayloadMarker = "payload that must not appear in compact dashboard manifest"

type largeDashboardMetrics struct {
	fakeAgentMetrics
}

func (largeDashboardMetrics) Report(id string) (semantic.Dashboard, *semantic.Model, bool) {
	if id != "executive-sales" {
		return semantic.Dashboard{}, nil, false
	}
	report, model, _ := fakeAgentMetrics{}.Report(id)
	report.Pages = make([]dashboard.Page, 0, 24)
	report.Visuals = map[string]semantic.Visual{}
	report.Tables = map[string]semantic.TableVisual{}
	for pageIndex := 1; pageIndex <= 24; pageIndex++ {
		chartID := fmt.Sprintf("chart_%02d", pageIndex)
		kpiID := fmt.Sprintf("kpi_%02d", pageIndex)
		tableID := fmt.Sprintf("table_%02d", pageIndex)
		report.Visuals[chartID] = semantic.Visual{
			Title:           fmt.Sprintf("Chart %02d", pageIndex),
			Type:            "bar",
			Query:           semantic.VisualQuery{Measures: []semantic.FieldRef{{Field: "order_count"}}},
			RendererOptions: map[string]any{"large": largeDashboardPayloadMarker + strings.Repeat("x", 4096)},
		}
		report.Visuals[kpiID] = semantic.Visual{
			Title:   fmt.Sprintf("KPI %02d", pageIndex),
			Kind:    "kpi",
			Query:   semantic.VisualQuery{Measures: []semantic.FieldRef{{Field: "order_count"}}},
			Options: map[string]any{"large": largeDashboardPayloadMarker + strings.Repeat("y", 4096)},
		}
		report.Tables[tableID] = semantic.TableVisual{
			Title: fmt.Sprintf("Table %02d", pageIndex),
			Query: semantic.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}},
			Columns: []dashboard.TableColumn{{
				Key:   largeDashboardPayloadMarker + strings.Repeat("z", 4096),
				Label: "Large Column",
			}},
		}
		report.Pages = append(report.Pages, dashboard.Page{
			ID:    fmt.Sprintf("page_%02d", pageIndex),
			Title: fmt.Sprintf("Page %02d", pageIndex),
			Visuals: []dashboard.PageVisual{
				{ID: chartID, Visual: chartID},
				{ID: kpiID, Visual: kpiID},
				{ID: tableID, Table: tableID},
			},
		})
	}
	return report, model, true
}

func (largeDashboardMetrics) Pages(id string) []dashboard.Page {
	report, _, ok := largeDashboardMetrics{}.Report(id)
	if !ok {
		return nil
	}
	return report.Pages
}

func runTool(t *testing.T, tools []agent.ToolDefinition, name, args string) string {
	t.Helper()
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		result, err := tool.Handler.Run(context.Background(), agent.ToolCall{ID: "call_1", Name: name, Arguments: []byte(args)})
		if err != nil {
			t.Fatalf("%s returned error: %v", name, err)
		}
		bytes, err := json.Marshal(result.Content)
		if err != nil {
			t.Fatalf("marshal %s result: %v", name, err)
		}
		return string(bytes)
	}
	t.Fatalf("tool %q not found", name)
	return ""
}

func openAgentAppStore(t *testing.T, ctx context.Context) *platform.Store {
	t.Helper()
	store, err := platform.Open(ctx, t.TempDir()+"/libredash.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.EnsureWorkspace(ctx, platform.WorkspaceInput{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	return store
}

func createAgentAppPrincipal(t *testing.T, ctx context.Context, store *platform.Store, email string) platformdb.Principal {
	t.Helper()
	principal, err := store.UpsertPrincipal(ctx, platform.PrincipalInput{Email: email, DisplayName: email})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	if err := store.BindRole(ctx, "test", principal.ID, "viewer"); err != nil {
		t.Fatalf("bind role: %v", err)
	}
	return principal
}
