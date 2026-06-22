package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/platform"
)

func TestChatPageRequiresAuthAndRendersComponents(t *testing.T) {
	store := testStore(t)
	auth := NewAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentapp.NewService(fakeMetrics{}, store, agentapp.Config{APIKey: "key", Model: "fake-model"}), DefaultWorkspaceID: "test"})

	unauthReq := httptest.NewRequest(http.MethodGet, "/chat", nil)
	unauthRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusFound {
		t.Fatalf("unauth status = %d, want redirect", unauthRec.Code)
	}

	ctx := context.Background()
	principal, err := store.UpsertPrincipal(ctx, platform.PrincipalInput{Email: "viewer@example.com", DisplayName: "Viewer"})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	if err := store.BindRole(ctx, "test", principal.ID, "viewer"); err != nil {
		t.Fatalf("bind role: %v", err)
	}
	token, err := store.CreateAPIToken(ctx, principal.ID, "chat-page")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`/static/chat.js`,
		`data-signals=`,
		`<ld-chat-thread`,
		`<ld-chat-composer`,
		`data-attr:events="$agent.events"`,
		`data-on:ld-chat-submit`,
		`/chat/turns`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("chat page missing %q:\n%s", want, body)
		}
	}
}

func TestChatPageDisabledState(t *testing.T) {
	store := testStore(t)
	auth := NewAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentapp.NewService(fakeMetrics{}, store, agentapp.Config{}), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<ld-chat-thread`) || !strings.Contains(body, `Agent is not configured`) {
		t.Fatalf("disabled chat page did not render usable disabled state:\n%s", body)
	}
}

func TestChatTurnStreamsDatastarSignalsAndPersistsEvents(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal, token := chatPrincipalAndToken(t, ctx, store)
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

	signals := map[string]any{"agent": map[string]any{"activeConversationId": "", "composer": map[string]any{"value": "What dashboards can I use?"}}}
	req := chatSignalsRequest(http.MethodPost, "/chat/turns", token, signals)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"event: datastar-patch-signals", "message_delta", "message_appended", "Executive Sales", "agent_end"} {
		if !strings.Contains(body, want) {
			t.Fatalf("turn response missing %q:\n%s", want, body)
		}
	}
	conversations, err := store.ListAgentConversations(ctx, "test", principal.ID)
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("conversations = %#v", conversations)
	}
}

func TestChatConversationSelectStreamsEventsAndRejectsOtherPrincipal(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	owner, token := chatPrincipalAndToken(t, ctx, store)
	other, err := store.UpsertPrincipal(ctx, platform.PrincipalInput{Email: "other@example.com", DisplayName: "Other"})
	if err != nil {
		t.Fatalf("upsert other: %v", err)
	}
	if err := store.BindRole(ctx, "test", other.ID, "viewer"); err != nil {
		t.Fatalf("bind other: %v", err)
	}
	service := agentapp.NewService(fakeMetrics{}, store, agentapp.Config{APIKey: "key", Model: "fake-model"})
	owned, err := service.CreateConversation(ctx, agentapp.Scope{WorkspaceID: "test", PrincipalID: owner.ID}, "Owned")
	if err != nil {
		t.Fatalf("create owned: %v", err)
	}
	if _, err := store.AppendAgentMessage(ctx, platform.AgentMessageInput{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: owned.ID,
		Role:           platform.AgentMessageRoleUser,
		ContentText:    "hello",
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}
	hidden, err := service.CreateConversation(ctx, agentapp.Scope{WorkspaceID: "test", PrincipalID: other.ID}, "Hidden")
	if err != nil {
		t.Fatalf("create hidden: %v", err)
	}
	auth := NewAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: service, DefaultWorkspaceID: "test"})

	req := chatSignalsRequest(http.MethodPost, "/chat/conversations/select", token, map[string]any{"agent": map[string]any{"activeConversationId": owned.ID}})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "message_appended") || !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("select owned status=%d body=%s", rec.Code, rec.Body.String())
	}

	hiddenReq := chatSignalsRequest(http.MethodPost, "/chat/conversations/select", token, map[string]any{"agent": map[string]any{"activeConversationId": hidden.ID}})
	hiddenRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(hiddenRec, hiddenReq)
	if hiddenRec.Code != http.StatusNotFound {
		t.Fatalf("select hidden status=%d body=%s", hiddenRec.Code, hiddenRec.Body.String())
	}
}

func chatPrincipalAndToken(t *testing.T, ctx context.Context, store *platform.Store) (platformdbPrincipal, string) {
	t.Helper()
	principal, err := store.UpsertPrincipal(ctx, platform.PrincipalInput{Email: "viewer@example.com", DisplayName: "Viewer"})
	if err != nil {
		t.Fatalf("upsert principal: %v", err)
	}
	if err := store.BindRole(ctx, "test", principal.ID, "viewer"); err != nil {
		t.Fatalf("bind role: %v", err)
	}
	token, err := store.CreateAPIToken(ctx, principal.ID, "chat-test")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return platformdbPrincipal{ID: principal.ID}, token
}

type platformdbPrincipal struct {
	ID string
}

func chatSignalsRequest(method, path, token string, signals map[string]any) *http.Request {
	bytes, _ := json.Marshal(signals)
	req := httptest.NewRequest(method, path, strings.NewReader(string(bytes)))
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}
