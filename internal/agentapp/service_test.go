package agentapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
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
	messages, err := store.ListMessages(ctx, "test", principal.ID, conversation.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want user/assistant/tool/assistant: %#v", len(messages), messages)
	}
	runs, err := store.ListRuns(ctx, "test", principal.ID, conversation.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != result.RunID {
		t.Fatalf("runs = %#v result=%#v", runs, result)
	}
	events, err := store.ListEvents(ctx, "test", principal.ID, result.RunID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) == 0 || events[0].Seq != 1 {
		t.Fatalf("events = %#v", events)
	}
	updated, err := store.GetConversation(ctx, "test", principal.ID, conversation.ID)
	if err != nil {
		t.Fatalf("get updated conversation: %v", err)
	}
	if !strings.Contains(updated.TranscriptJSON, "Executive Sales") {
		t.Fatalf("transcript was not updated: %s", updated.TranscriptJSON)
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
	if _, err := store.AppendMessage(ctx, MessageInput{
		WorkspaceID:    scope.WorkspaceID,
		PrincipalID:    scope.PrincipalID,
		ConversationID: conversation.ID,
		Role:           MessageRoleUser,
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
	if _, err := store.AppendMessage(ctx, MessageInput{
		WorkspaceID:    scope.WorkspaceID,
		PrincipalID:    scope.PrincipalID,
		ConversationID: conversation.ID,
		Role:           MessageRoleUser,
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
		if _, err := store.AppendMessage(ctx, MessageInput{
			WorkspaceID:    scope.WorkspaceID,
			PrincipalID:    scope.PrincipalID,
			ConversationID: multi.ID,
			Role:           MessageRoleUser,
			ContentText:    text,
		}); err != nil {
			t.Fatalf("append user message: %v", err)
		}
	}
	if updated, err := service.GenerateConversationTitle(ctx, scope, multi.ID); err != nil || updated.Title != ConversationDefaultTitle {
		t.Fatalf("multi-user title changed or errored: updated=%#v err=%v", updated, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("model called for multi-user conversation")
	}

	failing, err := service.CreateConversation(ctx, scope, "")
	if err != nil {
		t.Fatalf("create failing conversation: %v", err)
	}
	if _, err := store.AppendMessage(ctx, MessageInput{
		WorkspaceID:    scope.WorkspaceID,
		PrincipalID:    scope.PrincipalID,
		ConversationID: failing.ID,
		Role:           MessageRoleUser,
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
	if _, err := store.AppendMessage(ctx, MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           MessageRoleUser,
		ContentText:    "hello",
	}); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if _, err := store.AppendMessage(ctx, MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           MessageRoleSummary,
		ContentText:    "internal summary",
	}); err != nil {
		t.Fatalf("append summary: %v", err)
	}
	if _, err := store.AppendMessage(ctx, MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           MessageRoleAssistant,
		ContentText:    "Let me look that up.",
		ContentJSON:    `{"tool_calls":[{"id":"call_1","name":"list_dashboards","arguments":{"limit":2}}]}`,
	}); err != nil {
		t.Fatalf("append assistant tool call: %v", err)
	}
	if _, err := store.AppendMessage(ctx, MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           MessageRoleTool,
		ContentText:    `{"summary":"Found 2 dashboards"}`,
		ToolCallID:     "call_1",
		ToolName:       "list_dashboards",
	}); err != nil {
		t.Fatalf("append tool: %v", err)
	}
	if _, err := store.AppendMessage(ctx, MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		Role:           MessageRoleAssistant,
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

func (fakeAgentMetrics) Report(id string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	if id != "executive-sales" {
		return reportdef.Dashboard{}, nil, false
	}
	return reportdef.Dashboard{
		ID:            "executive-sales",
		Title:         "Executive Sales",
		Description:   "Sales dashboard",
		SemanticModel: "test",
		Visuals: map[string]reportdef.Visual{
			"orders": {Title: "Orders", Query: reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "order_count"}}}},
		},
		Tables: map[string]reportdef.TableVisual{
			"orders": {Title: "Orders", Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}},
		},
		Pages: []dashboard.Page{{ID: "overview", Title: "Overview", Visuals: []dashboard.PageVisual{{ID: "orders", Visual: "orders"}, {ID: "orders-table", Table: "orders"}}}},
	}, fakeSemanticModel(), true
}

func fakeSemanticModel() *semanticmodel.Model {
	return &semanticmodel.Model{
		Name:      "test",
		Title:     "Test Model",
		BaseTable: "orders",
		Sources: map[string]semanticmodel.Source{
			"orders": {Path: "orders.csv"},
		},
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Kind:       "fact",
				Source:     "orders",
				PrimaryKey: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Expr: "order_id"},
				},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{
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

func (largeDashboardMetrics) Report(id string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	if id != "executive-sales" {
		return reportdef.Dashboard{}, nil, false
	}
	report, model, _ := fakeAgentMetrics{}.Report(id)
	report.Pages = make([]dashboard.Page, 0, 24)
	report.Visuals = map[string]reportdef.Visual{}
	report.Tables = map[string]reportdef.TableVisual{}
	for pageIndex := 1; pageIndex <= 24; pageIndex++ {
		chartID := fmt.Sprintf("chart_%02d", pageIndex)
		kpiID := fmt.Sprintf("kpi_%02d", pageIndex)
		tableID := fmt.Sprintf("table_%02d", pageIndex)
		report.Visuals[chartID] = reportdef.Visual{
			Title:           fmt.Sprintf("Chart %02d", pageIndex),
			Type:            "bar",
			Query:           reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "order_count"}}},
			RendererOptions: map[string]any{"large": largeDashboardPayloadMarker + strings.Repeat("x", 4096)},
		}
		report.Visuals[kpiID] = reportdef.Visual{
			Title:   fmt.Sprintf("KPI %02d", pageIndex),
			Kind:    "kpi",
			Query:   reportdef.VisualQuery{Measures: []reportdef.FieldRef{{Field: "order_count"}}},
			Options: map[string]any{"large": largeDashboardPayloadMarker + strings.Repeat("y", 4096)},
		}
		report.Tables[tableID] = reportdef.TableVisual{
			Title: fmt.Sprintf("Table %02d", pageIndex),
			Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}},
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

func openAgentAppStore(t *testing.T, _ context.Context) *testAgentStore {
	t.Helper()
	return newTestAgentStore()
}

func createAgentAppPrincipal(t *testing.T, _ context.Context, store *testAgentStore, email string) testPrincipal {
	t.Helper()
	id := "principal_" + strings.NewReplacer("@", "_", ".", "_").Replace(email)
	store.upsertPrincipal(id)
	return testPrincipal{ID: id}
}

type testPrincipal struct {
	ID string
}

type testAgentStore struct {
	mu              sync.Mutex
	nextID          int
	principals      map[string]struct{}
	conversations   map[string]Conversation
	messages        map[string][]Message
	runs            map[string]Run
	runConversation map[string]string
	events          map[string][]Event
}

func newTestAgentStore() *testAgentStore {
	return &testAgentStore{
		principals:      map[string]struct{}{},
		conversations:   map[string]Conversation{},
		messages:        map[string][]Message{},
		runs:            map[string]Run{},
		runConversation: map[string]string{},
		events:          map[string][]Event{},
	}
}

func (s *testAgentStore) Close() error { return nil }

func (s *testAgentStore) upsertPrincipal(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.principals[id] = struct{}{}
}

func (s *testAgentStore) CreateConversation(_ context.Context, input ConversationInput) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = ConversationDefaultTitle
	}
	conversation := Conversation{
		ID:             s.id("agentconv"),
		WorkspaceID:    input.WorkspaceID,
		PrincipalID:    input.PrincipalID,
		Title:          title,
		Status:         ConversationStatusActive,
		MetadataJSON:   firstNonEmpty(input.MetadataJSON, "{}"),
		TranscriptJSON: "[]",
		CreatedAt:      testNow(),
		UpdatedAt:      testNow(),
	}
	s.conversations[conversation.ID] = conversation
	return conversation, nil
}

func (s *testAgentStore) ListConversations(_ context.Context, workspaceID, principalID string) ([]Conversation, error) {
	return s.ListConversationsPage(context.Background(), workspaceID, principalID, Page{})
}

func (s *testAgentStore) ListConversationsPage(_ context.Context, workspaceID, principalID string, page Page) ([]Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Conversation
	for _, conversation := range s.conversations {
		if conversation.WorkspaceID == workspaceID && conversation.PrincipalID == principalID && conversation.Status == ConversationStatusActive {
			out = append(out, conversation)
		}
	}
	return pageTestRows(out, page, func(row Conversation) string { return row.ID }), nil
}

func (s *testAgentStore) GetConversation(_ context.Context, workspaceID, principalID, conversationID string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conversationLocked(workspaceID, principalID, conversationID)
}

func (s *testAgentStore) UpdateConversation(_ context.Context, input ConversationUpdate) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversation, err := s.conversationLocked(input.WorkspaceID, input.PrincipalID, input.ConversationID)
	if err != nil {
		return Conversation{}, err
	}
	if conversation.Status != ConversationStatusActive {
		return Conversation{}, sql.ErrNoRows
	}
	conversation.Title = strings.TrimSpace(input.Title)
	conversation.UpdatedAt = testNow()
	s.conversations[conversation.ID] = conversation
	return conversation, nil
}

func (s *testAgentStore) ArchiveConversation(_ context.Context, workspaceID, principalID, conversationID string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversation, err := s.conversationLocked(workspaceID, principalID, conversationID)
	if err != nil {
		return Conversation{}, err
	}
	conversation.Status = ConversationStatusArchived
	conversation.ArchivedAt = testNow()
	conversation.UpdatedAt = testNow()
	s.conversations[conversation.ID] = conversation
	return conversation, nil
}

func (s *testAgentStore) UpdateDefaultConversationTitle(_ context.Context, workspaceID, principalID, conversationID, title string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversation, err := s.conversationLocked(workspaceID, principalID, conversationID)
	if err != nil {
		return Conversation{}, err
	}
	if conversation.Title != ConversationDefaultTitle || conversation.Status != ConversationStatusActive {
		return Conversation{}, sql.ErrNoRows
	}
	conversation.Title = title
	conversation.UpdatedAt = testNow()
	s.conversations[conversation.ID] = conversation
	return conversation, nil
}

func (s *testAgentStore) UpdateConversationTranscript(_ context.Context, workspaceID, principalID, conversationID, transcriptJSON string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversation, err := s.conversationLocked(workspaceID, principalID, conversationID)
	if err != nil {
		return Conversation{}, err
	}
	conversation.TranscriptJSON = transcriptJSON
	conversation.UpdatedAt = testNow()
	s.conversations[conversation.ID] = conversation
	return conversation, nil
}

func (s *testAgentStore) AppendMessage(_ context.Context, input MessageInput) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conversationLocked(input.WorkspaceID, input.PrincipalID, input.ConversationID); err != nil {
		return Message{}, err
	}
	message := Message{
		ID:             s.id("agentmsg"),
		ConversationID: input.ConversationID,
		RunID:          input.RunID,
		Seq:            int64(len(s.messages[input.ConversationID]) + 1),
		Role:           input.Role,
		ContentText:    input.ContentText,
		ContentJSON:    firstNonEmpty(input.ContentJSON, "{}"),
		ToolCallID:     input.ToolCallID,
		ToolName:       input.ToolName,
		IsError:        input.IsError,
		CreatedAt:      testNow(),
	}
	s.messages[input.ConversationID] = append(s.messages[input.ConversationID], message)
	return message, nil
}

func (s *testAgentStore) ListMessages(_ context.Context, workspaceID, principalID, conversationID string) ([]Message, error) {
	return s.ListMessagesPage(context.Background(), workspaceID, principalID, conversationID, Page{})
}

func (s *testAgentStore) ListMessagesPage(_ context.Context, workspaceID, principalID, conversationID string, page Page) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conversationLocked(workspaceID, principalID, conversationID); err != nil {
		return nil, err
	}
	return pageTestRows(s.messages[conversationID], page, func(row Message) string { return row.ID }), nil
}

func (s *testAgentStore) CreateRun(_ context.Context, input RunInput) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conversationLocked(input.WorkspaceID, input.PrincipalID, input.ConversationID); err != nil {
		return Run{}, err
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		runID = s.id("agentrun")
	}
	run := Run{ID: runID, ConversationID: input.ConversationID, Status: RunStatusRunning, Model: input.Model, StartedAt: testNow(), CreatedAt: testNow()}
	s.runs[run.ID] = run
	s.runConversation[run.ID] = input.ConversationID
	return run, nil
}

func (s *testAgentStore) FinishRun(_ context.Context, input RunFinish) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conversationLocked(input.WorkspaceID, input.PrincipalID, input.ConversationID); err != nil {
		return Run{}, err
	}
	run, ok := s.runs[input.RunID]
	if !ok {
		return Run{}, sql.ErrNoRows
	}
	run.Status = input.Status
	run.StopReason = input.StopReason
	run.InputTokens = input.InputTokens
	run.OutputTokens = input.OutputTokens
	run.TotalTokens = input.TotalTokens
	run.Error = input.Error
	run.FinishedAt = testNow()
	s.runs[run.ID] = run
	return run, nil
}

func (s *testAgentStore) ListRuns(_ context.Context, workspaceID, principalID, conversationID string) ([]Run, error) {
	return s.ListRunsPage(context.Background(), workspaceID, principalID, conversationID, Page{})
}

func (s *testAgentStore) ListRunsPage(_ context.Context, workspaceID, principalID, conversationID string, page Page) ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conversationLocked(workspaceID, principalID, conversationID); err != nil {
		return nil, err
	}
	var out []Run
	for runID, candidate := range s.runs {
		if s.runConversation[runID] == conversationID {
			out = append(out, candidate)
		}
	}
	return pageTestRows(out, page, func(row Run) string { return row.ID }), nil
}

func (s *testAgentStore) GetRun(_ context.Context, workspaceID, principalID, conversationID, runID string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conversationLocked(workspaceID, principalID, conversationID); err != nil {
		return Run{}, err
	}
	run, ok := s.runs[runID]
	if !ok || s.runConversation[runID] != conversationID {
		return Run{}, sql.ErrNoRows
	}
	return run, nil
}

func (s *testAgentStore) GetRunByID(_ context.Context, workspaceID, principalID, runID string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversationID, ok := s.runConversation[runID]
	if !ok {
		return Run{}, sql.ErrNoRows
	}
	if _, err := s.conversationLocked(workspaceID, principalID, conversationID); err != nil {
		return Run{}, err
	}
	return s.runs[runID], nil
}

func (s *testAgentStore) AppendEvent(_ context.Context, input EventInput) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversationID, ok := s.runConversation[input.RunID]
	if !ok {
		return Event{}, sql.ErrNoRows
	}
	if _, err := s.conversationLocked(input.WorkspaceID, input.PrincipalID, conversationID); err != nil {
		return Event{}, err
	}
	event := Event{
		ID:          s.id("agentevt"),
		RunID:       input.RunID,
		Seq:         input.Sequence,
		EventType:   input.EventType,
		Severity:    firstNonEmpty(input.Severity, "info"),
		PayloadJSON: firstNonEmpty(input.PayloadJSON, "{}"),
		CreatedAt:   testNow(),
	}
	s.events[input.RunID] = append(s.events[input.RunID], event)
	return event, nil
}

func (s *testAgentStore) ListEvents(_ context.Context, workspaceID, principalID, runID string) ([]Event, error) {
	return s.ListEventsPage(context.Background(), workspaceID, principalID, runID, Page{})
}

func (s *testAgentStore) ListEventsPage(_ context.Context, workspaceID, principalID, runID string, page Page) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversationID, ok := s.runConversation[runID]
	if !ok {
		return nil, sql.ErrNoRows
	}
	if _, err := s.conversationLocked(workspaceID, principalID, conversationID); err != nil {
		return nil, err
	}
	return pageTestRows(s.events[runID], page, func(row Event) string { return row.ID }), nil
}

func (s *testAgentStore) conversationLocked(workspaceID, principalID, conversationID string) (Conversation, error) {
	conversation, ok := s.conversations[conversationID]
	if !ok || conversation.WorkspaceID != workspaceID || conversation.PrincipalID != principalID {
		return Conversation{}, sql.ErrNoRows
	}
	return conversation, nil
}

func (s *testAgentStore) id(prefix string) string {
	s.nextID++
	return fmt.Sprintf("%s_%d", prefix, s.nextID)
}

func testNow() string {
	return "2026-01-01 00:00:00"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func pageTestRows[T any](rows []T, page Page, id func(T) string) []T {
	limit := page.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	start := 0
	if page.After != "" {
		start = len(rows)
		for i, row := range rows {
			if id(row) == page.After {
				start = i + 1
				break
			}
		}
	}
	if start >= len(rows) {
		return []T{}
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	return append([]T(nil), rows[start:end]...)
}

func testConversation(id, workspaceID, principalID, title, status, metadataJSON, transcriptJSON, createdAt, updatedAt string) Conversation {
	return Conversation{
		ID:             id,
		WorkspaceID:    workspaceID,
		PrincipalID:    principalID,
		Title:          title,
		Status:         status,
		MetadataJSON:   metadataJSON,
		TranscriptJSON: transcriptJSON,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}
