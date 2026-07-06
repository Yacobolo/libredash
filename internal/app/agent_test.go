package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agent"
	agentconfig "github.com/Yacobolo/libredash/internal/agent/config"
)

func TestAgentAPIReportsDisabledWhenProviderMissing(t *testing.T) {
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{}), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 body=%s", rec.Code, rec.Body.String())
	}
}

func TestGlobalAgentAPIListsPrincipalConversations(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "viewer@example.com", "Viewer", "viewer")
	token := testAPIToken(t, ctx, store, principal.ID, "agent-global")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	agentService := agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{APIKey: "key", Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentService, DefaultWorkspaceID: "test"})

	createReq := authedJSONRequest(http.MethodPost, "/api/v1/agent/conversations", token, `{"title":"Global ask"}`)
	createRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", createRec.Code, createRec.Body.String())
	}

	listReq := authedJSONRequest(http.MethodGet, "/api/v1/agent/conversations", token, "")
	listRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "Global ask") {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
}

func TestAgentAPIConversationTurnPersistsMessagesAndEvents(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "viewer@example.com", "Viewer", "viewer")
	token := testAPIToken(t, ctx, store, principal.ID, "agent-test")
	if err := store.UpsertSetting(ctx, agentconfig.SystemPromptSettingKey, "Stored admin system prompt."); err != nil {
		t.Fatalf("seed system prompt: %v", err)
	}
	var calls atomic.Int64
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode model request: %v", err)
		}
		if len(req.Messages) == 0 || req.Messages[0].Role != "system" || req.Messages[0].Content != "Stored admin system prompt." {
			t.Fatalf("model request system prompt = %#v", req.Messages)
		}
		if calls.Add(1) == 1 {
			writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_dashboards","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
			return
		}
		writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"Executive Sales is available."},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}}`)
	}))
	defer modelServer.Close()
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	agentService := agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentService, DefaultWorkspaceID: "test"})

	createReq := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/agent/conversations", token, `{"title":"Ask"}`)
	createRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	conversationID := created["id"].(string)

	turnReq := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/agent/conversations/"+conversationID+"/turns", token, `{"input":"What dashboards can I use?","correlationId":"corr_1"}`)
	turnRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(turnRec, turnReq)
	if turnRec.Code != http.StatusOK {
		t.Fatalf("turn status = %d body=%s", turnRec.Code, turnRec.Body.String())
	}
	var turn map[string]any
	if err := json.Unmarshal(turnRec.Body.Bytes(), &turn); err != nil {
		t.Fatalf("decode turn: %v", err)
	}
	if !strings.Contains(turn["content"].(string), "Executive Sales") {
		t.Fatalf("turn response = %#v", turn)
	}
	runID := turn["runId"].(string)

	messagesReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations/"+conversationID+"/messages", token, "")
	messagesRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(messagesRec, messagesReq)
	if messagesRec.Code != http.StatusOK || !strings.Contains(messagesRec.Body.String(), "Executive Sales") {
		t.Fatalf("messages status=%d body=%s", messagesRec.Code, messagesRec.Body.String())
	}
	eventsReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations/"+conversationID+"/runs/"+runID+"/events", token, "")
	eventsRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK || !strings.Contains(eventsRec.Body.String(), "model_response") {
		t.Fatalf("events status=%d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
}

func TestAdminAgentAPIReturnsAndUpdatesSystemPrompt(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	owner := testPrincipal(t, ctx, store, "owner@example.com", "Owner", access.RoleOwner)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	agentService := agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{APIKey: "key", Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentService, DefaultWorkspaceID: "test"})

	getReq := authedJSONRequest(http.MethodGet, "/api/v1/admin/agent/config", token, "")
	getRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"systemPrompt":"You are LibreDash`) || !strings.Contains(getRec.Body.String(), `"tools"`) || !strings.Contains(getRec.Body.String(), `"query_visual"`) {
		t.Fatalf("get body missing prompt or tools: %s", getRec.Body.String())
	}

	putReq := authedJSONRequest(http.MethodPatch, "/api/v1/admin/agent/config", token, `{"systemPrompt":"  Updated platform prompt.  "}`)
	putRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", putRec.Code, putRec.Body.String())
	}
	if !strings.Contains(putRec.Body.String(), `"systemPrompt":"Updated platform prompt."`) {
		t.Fatalf("put body did not return trimmed prompt: %s", putRec.Body.String())
	}

	getUpdatedReq := authedJSONRequest(http.MethodGet, "/api/v1/admin/agent/config", token, "")
	getUpdatedRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(getUpdatedRec, getUpdatedReq)
	if getUpdatedRec.Code != http.StatusOK || !strings.Contains(getUpdatedRec.Body.String(), `"systemPrompt":"Updated platform prompt."`) {
		t.Fatalf("updated get status=%d body=%s", getUpdatedRec.Code, getUpdatedRec.Body.String())
	}
}

func TestAdminAgentAPIRejectsEmptySystemPrompt(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	owner := testPrincipal(t, ctx, store, "owner-empty@example.com", "Owner", access.RoleOwner)
	token := testAPIToken(t, ctx, store, owner.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{APIKey: "key", Model: "fake-model"}), DefaultWorkspaceID: "test"})

	req := authedJSONRequest(http.MethodPatch, "/api/v1/admin/agent/config", token, `{"systemPrompt":"   "}`)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminAgentAPIRequiresGrantPrivileges(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	viewer := testPrincipal(t, ctx, store, "viewer-admin-agent@example.com", "Viewer", access.RoleViewer)
	token := testAPIToken(t, ctx, store, viewer.ID, "test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{APIKey: "key", Model: "fake-model"}), DefaultWorkspaceID: "test"})

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/admin/agent/config"},
		{method: http.MethodPatch, path: "/api/v1/admin/agent/config", body: `{"systemPrompt":"Updated"}`},
	} {
		req := authedJSONRequest(tc.method, tc.path, token, tc.body)
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s status=%d want 403 body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestAgentAPISupportsConversationAndRunReads(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "viewer@example.com", "Viewer", "viewer")
	token := testAPIToken(t, ctx, store, principal.ID, "agent-test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	agentService := agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{APIKey: "key", Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentService, DefaultWorkspaceID: "test"})
	scope := agent.Scope{WorkspaceID: "test", PrincipalID: principal.ID}
	conversation, err := agentService.CreateConversation(ctx, scope, "Original")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	run, err := testAgentRepository(store).CreateRun(ctx, agent.RunInput{
		WorkspaceID:    "test",
		PrincipalID:    principal.ID,
		ConversationID: conversation.ID,
		RunID:          "run_test",
		Model:          "fake-model",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := testAgentRepository(store).AppendEvent(ctx, agent.EventInput{
		WorkspaceID: "test",
		PrincipalID: principal.ID,
		RunID:       run.ID,
		Sequence:    1,
		EventType:   "model_request",
		PayloadJSON: `{"ok":true}`,
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	updateReq := authedJSONRequest(http.MethodPatch, "/api/v1/workspaces/test/agent/conversations/"+conversation.ID, token, `{"title":"Updated"}`)
	updateRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK || !strings.Contains(updateRec.Body.String(), `"title":"Updated"`) {
		t.Fatalf("update status=%d body=%s", updateRec.Code, updateRec.Body.String())
	}

	runsReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations/"+conversation.ID+"/runs?limit=1", token, "")
	runsRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(runsRec, runsReq)
	if runsRec.Code != http.StatusOK || !strings.Contains(runsRec.Body.String(), `"id":"run_test"`) {
		t.Fatalf("runs status=%d body=%s", runsRec.Code, runsRec.Body.String())
	}

	runReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations/"+conversation.ID+"/runs/"+run.ID, token, "")
	runRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK || !strings.Contains(runRec.Body.String(), `"conversationId":"`+conversation.ID+`"`) {
		t.Fatalf("run status=%d body=%s", runRec.Code, runRec.Body.String())
	}

	eventsReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations/"+conversation.ID+"/runs/"+run.ID+"/events?limit=1", token, "")
	eventsRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK || !strings.Contains(eventsRec.Body.String(), `"eventType":"model_request"`) {
		t.Fatalf("nested events status=%d body=%s", eventsRec.Code, eventsRec.Body.String())
	}

	archiveReq := authedJSONRequest(http.MethodDelete, "/api/v1/workspaces/test/agent/conversations/"+conversation.ID, token, "")
	archiveRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusOK || !strings.Contains(archiveRec.Body.String(), `"status":"archived"`) {
		t.Fatalf("archive status=%d body=%s", archiveRec.Code, archiveRec.Body.String())
	}
	listReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations", token, "")
	listRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || strings.Contains(listRec.Body.String(), conversation.ID) {
		t.Fatalf("archived conversation listed status=%d body=%s", listRec.Code, listRec.Body.String())
	}
}

func TestAgentAPIRejectsConcurrentTurnsForConversation(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "viewer@example.com", "Viewer", "viewer")
	token := testAPIToken(t, ctx, store, principal.ID, "agent-test")
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer modelServer.Close()
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	agentService := agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentService, DefaultWorkspaceID: "test"})
	conversation, err := agentService.CreateConversation(ctx, agent.Scope{WorkspaceID: "test", PrincipalID: principal.ID}, "Ask")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/agent/conversations/"+conversation.ID+"/turns", token, `{"input":"hello"}`)
			rec := httptest.NewRecorder()
			server.Routes().ServeHTTP(rec, req)
			statuses <- rec.Code
		}()
	}
	wg.Wait()
	close(statuses)
	sawConflict := false
	for status := range statuses {
		if status == http.StatusConflict {
			sawConflict = true
		}
	}
	if !sawConflict {
		t.Fatal("concurrent turns did not return a 409 conflict")
	}
}

func TestMaterializationRunAPIPersistsAsyncRefreshStatus(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "editor@example.com", "Editor", "editor")
	if _, err := store.SQLDB().ExecContext(ctx, `
		INSERT INTO serving_states (id, workspace_id, status, digest, manifest_json, created_by)
		VALUES ('dep_1', 'test', 'active', 'sha256:test', '{}', ?)
	`, principal.ID); err != nil {
		t.Fatalf("seed deployment: %v", err)
	}
	token := testAPIToken(t, ctx, store, principal.ID, "materialization-test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	metrics := &materializationAPIMetrics{done: make(chan string, 1)}
	server := NewWithOptions(metrics, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	createReq := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/refresh-runs", token, `{"modelId":"model.orders","servingStateId":"dep_1"}`)
	createRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID                   string `json:"id"`
		ModelID              string `json:"modelId"`
		ServingStateID       string `json:"servingStateId"`
		Status               string `json:"status"`
		PrincipalID          string `json:"principalId"`
		PrincipalDisplayName string `json:"principalDisplayName"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.Status != "queued" || created.ModelID != "model.orders" || created.ServingStateID != "dep_1" {
		t.Fatalf("created run = %#v", created)
	}
	if created.PrincipalID != principal.ID || created.PrincipalDisplayName != "Editor" {
		t.Fatalf("created attribution = %#v, want Editor principal", created)
	}

	select {
	case modelID := <-metrics.done:
		if modelID != "model.orders" {
			t.Fatalf("refreshed model = %q", modelID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async refresh")
	}

	getReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/refresh-runs/"+created.ID, token, "")
	getRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), `"status":"succeeded"`) || !strings.Contains(getRec.Body.String(), `"principalDisplayName":"Editor"`) {
		t.Fatalf("get status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	listReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/refresh-runs?limit=1", token, "")
	listRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), created.ID) || !strings.Contains(listRec.Body.String(), `"principalDisplayName":"Editor"`) {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
}

type materializationAPIMetrics struct {
	fakeMetrics
	done chan string
}

func (m *materializationAPIMetrics) RefreshMaterializations(_ context.Context, modelID string) error {
	m.done <- modelID
	return nil
}

func authedJSONRequest(method, path, token, body string) *http.Request {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Accept", "application/json")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func writeRawJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
