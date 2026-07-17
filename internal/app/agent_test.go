package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/agent"
	agentconfig "github.com/Yacobolo/libredash/internal/agent/config"
)

func TestAgentAPIReportsDisabledWhenProviderMissing(t *testing.T) {
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agent.NewService(fakeMetrics{}, testAgentRepository(store), agent.Config{}), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations", nil)
	req.Header.Set("Authorization", "Bearer dev")
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

	createReq := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/agent/conversations", token, `{"title":"Global ask"}`)
	createRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", createRec.Code, createRec.Body.String())
	}

	listReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations", token, "")
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
	backgroundCtx, cancelBackground := context.WithCancel(context.Background())
	server.StartBackgroundJobs(backgroundCtx)
	t.Cleanup(func() {
		cancelBackground()
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.StopBackgroundJobs(stopCtx)
	})

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

	turnReq := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/agent/conversations/"+conversationID+"/runs", token, `{"input":"What dashboards can I use?","correlationId":"corr_1"}`)
	turnReq.Header.Set("Idempotency-Key", "agent-run-1")
	turnRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(turnRec, turnReq)
	if turnRec.Code != http.StatusAccepted {
		t.Fatalf("turn status = %d body=%s", turnRec.Code, turnRec.Body.String())
	}
	var turn map[string]any
	if err := json.Unmarshal(turnRec.Body.Bytes(), &turn); err != nil {
		t.Fatalf("decode turn: %v", err)
	}
	runID := turn["id"].(string)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations/"+conversationID+"/runs/"+runID, token, "")
		runRec := httptest.NewRecorder()
		server.Routes().ServeHTTP(runRec, runReq)
		if strings.Contains(runRec.Body.String(), `"status":"completed"`) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

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

func TestAdminAgentConfigurationIsNotPublicAPI(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/v1/admin/agent/config", nil)
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d want 404 body=%s", method, rec.Code, rec.Body.String())
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
	updateReq.Header.Set("If-Match", "*")
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
	if eventsRec.Code != http.StatusOK || !strings.Contains(eventsRec.Body.String(), `"event":"model_request"`) {
		t.Fatalf("nested events status=%d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	if _, err := testAgentRepository(store).FinishRun(ctx, agent.RunFinish{
		WorkspaceID: "test", PrincipalID: principal.ID, ConversationID: conversation.ID, RunID: run.ID, Status: agent.RunStatusCompleted,
	}); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	sseReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/agent/conversations/"+conversation.ID+"/runs/"+run.ID+"/events", token, "")
	sseReq.Header.Set("Accept", "text/event-stream")
	sseRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(sseRec, sseReq)
	if sseRec.Code != http.StatusOK || !strings.HasPrefix(sseRec.Header().Get("Content-Type"), "text/event-stream") || !strings.Contains(sseRec.Body.String(), "event: model_request") {
		t.Fatalf("SSE events status=%d headers=%v body=%s", sseRec.Code, sseRec.Header(), sseRec.Body.String())
	}

	archiveReq := authedJSONRequest(http.MethodDelete, "/api/v1/workspaces/test/agent/conversations/"+conversation.ID, token, "")
	archiveRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusNoContent || archiveRec.Body.Len() != 0 {
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
		go func(index int) {
			defer wg.Done()
			req := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/agent/conversations/"+conversation.ID+"/runs", token, `{"input":"hello"}`)
			req.Header.Set("Idempotency-Key", fmt.Sprintf("concurrent-%d", index))
			rec := httptest.NewRecorder()
			server.Routes().ServeHTTP(rec, req)
			statuses <- rec.Code
		}(i)
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
	createReq.Header.Set("Idempotency-Key", "refresh-agent-test")
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
	retryReq := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/refresh-runs", token, `{"retryOf":"`+created.ID+`"}`)
	retryReq.Header.Set("Idempotency-Key", "refresh-agent-retry")
	retryRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(retryRec, retryReq)
	if retryRec.Code != http.StatusAccepted || !strings.Contains(retryRec.Body.String(), `"retryOf":"`+created.ID+`"`) || retryRec.Header().Get("Location") == "" {
		t.Fatalf("retry status=%d location=%q body=%s", retryRec.Code, retryRec.Header().Get("Location"), retryRec.Body.String())
	}
	var retried struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(retryRec.Body.Bytes(), &retried); err != nil || retried.ID == "" {
		t.Fatalf("decode retry: %v body=%s", err, retryRec.Body.String())
	}
	select {
	case <-metrics.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for retry refresh")
	}
	deadline := time.Now().Add(time.Second)
	retryFinished := false
	for time.Now().Before(deadline) {
		statusReq := authedJSONRequest(http.MethodGet, "/api/v1/workspaces/test/refresh-runs/"+retried.ID, token, "")
		statusRec := httptest.NewRecorder()
		server.Routes().ServeHTTP(statusRec, statusReq)
		if strings.Contains(statusRec.Body.String(), `"status":"succeeded"`) {
			retryFinished = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !retryFinished {
		t.Fatal("retry refresh did not reach succeeded")
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
	if method == http.MethodPost {
		req.Header.Set("Idempotency-Key", "test-"+strings.ReplaceAll(path, "/", "-"))
	}
	if method == http.MethodPatch {
		req.Header.Set("If-Match", "*")
	}
	return req
}

func writeRawJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
