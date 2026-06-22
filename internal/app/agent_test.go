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

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/platform"
)

func TestAgentAPIReportsDisabledWhenProviderMissing(t *testing.T) {
	store := testStore(t)
	auth := NewAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentapp.NewService(fakeMetrics{}, store, agentapp.Config{}), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/test/agent/conversations", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentAPIConversationTurnPersistsMessagesAndEvents(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal, err := store.UpsertPrincipal(ctx, platform.PrincipalInput{Email: "viewer@example.com", DisplayName: "Viewer"})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	if err := store.BindRole(ctx, "test", principal.ID, "viewer"); err != nil {
		t.Fatalf("bind role: %v", err)
	}
	token, err := store.CreateAPIToken(ctx, principal.ID, "agent-test")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	var calls atomic.Int64
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_dashboards","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
			return
		}
		writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"Executive Sales is available."},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}}`)
	}))
	defer modelServer.Close()
	auth := NewAuth(store, "test", AuthConfig{APITokenOnly: true})
	agentService := agentapp.NewService(fakeMetrics{}, store, agentapp.Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentService, DefaultWorkspaceID: "test"})

	createReq := authedJSONRequest(http.MethodPost, "/api/workspaces/test/agent/conversations", token, `{"title":"Ask"}`)
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

	turnReq := authedJSONRequest(http.MethodPost, "/api/workspaces/test/agent/conversations/"+conversationID+"/turns", token, `{"input":"What dashboards can I use?","correlationId":"corr_1"}`)
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

	messagesReq := authedJSONRequest(http.MethodGet, "/api/workspaces/test/agent/conversations/"+conversationID+"/messages", token, "")
	messagesRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(messagesRec, messagesReq)
	if messagesRec.Code != http.StatusOK || !strings.Contains(messagesRec.Body.String(), "Executive Sales") {
		t.Fatalf("messages status=%d body=%s", messagesRec.Code, messagesRec.Body.String())
	}
	eventsReq := authedJSONRequest(http.MethodGet, "/api/workspaces/test/agent/runs/"+runID+"/events", token, "")
	eventsRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK || !strings.Contains(eventsRec.Body.String(), "model_response") {
		t.Fatalf("events status=%d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
}

func TestAgentAPIRejectsConcurrentTurnsForConversation(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal, err := store.UpsertPrincipal(ctx, platform.PrincipalInput{Email: "viewer@example.com", DisplayName: "Viewer"})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	if err := store.BindRole(ctx, "test", principal.ID, "viewer"); err != nil {
		t.Fatalf("bind role: %v", err)
	}
	token, err := store.CreateAPIToken(ctx, principal.ID, "agent-test")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer modelServer.Close()
	auth := NewAuth(store, "test", AuthConfig{APITokenOnly: true})
	agentService := agentapp.NewService(fakeMetrics{}, store, agentapp.Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentService, DefaultWorkspaceID: "test"})
	conversation, err := agentService.CreateConversation(ctx, agentapp.Scope{WorkspaceID: "test", PrincipalID: principal.ID}, "Ask")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := authedJSONRequest(http.MethodPost, "/api/workspaces/test/agent/conversations/"+conversation.ID+"/turns", token, `{"input":"hello"}`)
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
