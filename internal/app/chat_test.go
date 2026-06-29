package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/api"
	dashboardstream "github.com/Yacobolo/libredash/internal/dashboard/stream"
	"github.com/Yacobolo/libredash/internal/platform"
)

func TestChatPageRequiresAuthAndRendersComponents(t *testing.T) {
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", Model: "fake-model"}), DefaultWorkspaceID: "test"})

	unauthReq := httptest.NewRequest(http.MethodGet, "/chat", nil)
	unauthRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusFound {
		t.Fatalf("unauth status = %d, want redirect", unauthRec.Code)
	}

	ctx := context.Background()
	principal := testPrincipal(t, ctx, store, "viewer@example.com", "Viewer", "viewer")
	token := testAPIToken(t, ctx, store, principal.ID, "chat-page")

	req := httptest.NewRequest(http.MethodGet, "/chat/new", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`/static/app-shell.js`,
		`/static/chat-page.js`,
		`data-signals=`,
		`<ld-app-shell`,
		`<ld-chat-page`,
		`&#34;compact&#34;:true`,
		`&#34;collapsible&#34;:false`,
		`&#34;numbered&#34;:false`,
		`data-attr:page="JSON.stringify($page)"`,
		`data-attr:agent="JSON.stringify($agent)"`,
		`data-attr:pending="$agentTurnPending || $agent.status.running"`,
		`data-attr:composerdisabled="$agentTurnPending || $agent.status.running || $agent.composer.disabled"`,
		`data-indicator="agentTurnPending"`,
		`data-on:ld-chat-submit`,
		`/chat/turns`,
		`/chat/updates`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("chat page missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `aria-label="Agent conversations"`) {
		t.Fatalf("chat page should render the conversation web component instead of the static rail:\n%s", body)
	}
	for _, legacy := range []string{`<ld-sub-sidebar`, `<ld-chat-thread`, `<ld-chat-composer`, `data-attr:transcript`, `JSON.stringify($page.sidebar)`} {
		if strings.Contains(body, legacy) {
			t.Fatalf("chat page rendered product internals below the route root (%q):\n%s", legacy, body)
		}
	}
	if strings.Contains(body, `<ld-chat-conversation-sidebar`) {
		t.Fatalf("chat page still rendered chat-specific conversation sidebar:\n%s", body)
	}
	if strings.Contains(body, `data-on:ld-sub-sidebar-select`) || strings.Contains(body, `/chat/conversations/select`) {
		t.Fatalf("chat page should use conversation URLs instead of select POST:\n%s", body)
	}
	if strings.Contains(body, `data-attr:events`) || strings.Contains(body, `$agent.events`) {
		t.Fatalf("chat page should not feed raw events to the chat thread:\n%s", body)
	}
}

func TestChatPageDisabledState(t *testing.T) {
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{}), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<ld-chat-page`) || !strings.Contains(body, `Agent is not configured`) {
		t.Fatalf("disabled chat page did not render usable disabled state:\n%s", body)
	}
}

func TestChatRootRedirectsToNewWhenNoConversations(t *testing.T) {
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", Model: "fake-model"}), DefaultWorkspaceID: "test"})
	ctx := context.Background()
	_, token := chatPrincipalAndToken(t, ctx, store)

	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/chat/new" {
		t.Fatalf("status=%d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}

func TestChatRootRedirectsToLatestConversation(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	service := agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: service, DefaultWorkspaceID: "test"})
	principal, token := chatPrincipalAndToken(t, ctx, store)
	scope := agentapp.Scope{WorkspaceID: "test", PrincipalID: principal.ID}
	old, err := service.CreateConversation(ctx, scope, "Old")
	if err != nil {
		t.Fatalf("create old: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	latest, err := service.CreateConversation(ctx, scope, "Latest")
	if err != nil {
		t.Fatalf("create latest: %v", err)
	}
	if old.ID == latest.ID {
		t.Fatal("conversation IDs unexpectedly equal")
	}

	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/chat/"+latest.ID {
		t.Fatalf("status=%d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}

func TestChatNewRendersDraftWithoutCreatingConversation(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", Model: "fake-model"}), DefaultWorkspaceID: "test"})
	principal, token := chatPrincipalAndToken(t, ctx, store)

	req := httptest.NewRequest(http.MethodGet, "/chat/new", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`&#34;href&#34;:&#34;/chat/new&#34;`, `&#34;title&#34;:&#34;New chat&#34;`, `&#34;activeId&#34;:&#34;&#34;`} {
		if !strings.Contains(body, want) {
			t.Fatalf("draft chat page missing %q:\n%s", want, body)
		}
	}
	conversations, err := testAgentRepository(store).ListConversations(ctx, "test", principal.ID)
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 0 {
		t.Fatalf("draft route created conversations: %#v", conversations)
	}
}

func TestChatSignalConversationListUsesCallerContext(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	service := agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: service, DefaultWorkspaceID: "test"})
	principal, _ := chatPrincipalAndToken(t, ctx, store)
	scope := agentapp.Scope{WorkspaceID: "test", PrincipalID: principal.ID}
	if _, err := service.CreateConversation(ctx, scope, "Visible only with live context"); err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	signal := server.chatSignalWith(canceled, scope, "", nil, "", false)
	if len(signal.Conversations) != 0 {
		t.Fatalf("canceled context should prevent conversation loading, got %#v", signal.Conversations)
	}
}

func TestChatConversationRouteLoadsOwnedEventsAndRejectsOtherPrincipal(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	owner, token := chatPrincipalAndToken(t, ctx, store)
	other := testPrincipal(t, ctx, store, "other@example.com", "Other", "viewer")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	service := agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: service, DefaultWorkspaceID: "test"})
	owned, err := service.CreateConversation(ctx, agentapp.Scope{WorkspaceID: "test", PrincipalID: owner.ID}, "Owned")
	if err != nil {
		t.Fatalf("create owned: %v", err)
	}
	if _, err := testAgentRepository(store).AppendMessage(ctx, agentapp.MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: owned.ID,
		Role:           agentapp.MessageRoleUser,
		ContentText:    "hello",
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}
	hidden, err := service.CreateConversation(ctx, agentapp.Scope{WorkspaceID: "test", PrincipalID: other.ID}, "Hidden")
	if err != nil {
		t.Fatalf("create hidden: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/chat/"+owned.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `&#34;kind&#34;:&#34;user&#34;`) || !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("owned route status=%d body=%s", rec.Code, rec.Body.String())
	}

	hiddenReq := httptest.NewRequest(http.MethodGet, "/chat/"+hidden.ID, nil)
	hiddenReq.Header.Set("Authorization", "Bearer "+token)
	hiddenRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(hiddenRec, hiddenReq)
	if hiddenRec.Code != http.StatusNotFound {
		t.Fatalf("hidden route status=%d body=%s", hiddenRec.Code, hiddenRec.Body.String())
	}
}

func TestChatConversationRouteQueuesMissingTitleRepair(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	owner, token := chatPrincipalAndToken(t, ctx, store)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"Greeting"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`)
	}))
	defer modelServer.Close()
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	service := agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: service, DefaultWorkspaceID: "test"})
	scope := agentapp.Scope{WorkspaceID: "test", PrincipalID: owner.ID}
	conversation, err := service.CreateConversation(ctx, scope, "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := testAgentRepository(store).AppendMessage(ctx, agentapp.MessageInput{
		WorkspaceID:    "test",
		PrincipalID:    owner.ID,
		ConversationID: conversation.ID,
		Role:           agentapp.MessageRoleUser,
		ContentText:    "how are you?",
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/chat/"+conversation.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "client-test"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `&#34;titlePending&#34;:true`) {
		t.Fatalf("default one-turn conversation should render title pending:\n%s", rec.Body.String())
	}
	waitForAgentConversationTitle(t, store, "test", owner.ID, conversation.ID, "Greeting")
}

func TestChatConversationRouteSkipsTitleRepairForManualAndMultiUserTitles(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	owner, token := chatPrincipalAndToken(t, ctx, store)
	var calls atomic.Int64
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"Should not be used"},"finish_reason":"stop"}]}`)
	}))
	defer modelServer.Close()
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	service := agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: service, DefaultWorkspaceID: "test"})
	scope := agentapp.Scope{WorkspaceID: "test", PrincipalID: owner.ID}
	manual, err := service.CreateConversation(ctx, scope, "Manual title")
	if err != nil {
		t.Fatalf("create manual: %v", err)
	}
	multi, err := service.CreateConversation(ctx, scope, "")
	if err != nil {
		t.Fatalf("create multi: %v", err)
	}
	for _, text := range []string{"hello", "again"} {
		if _, err := testAgentRepository(store).AppendMessage(ctx, agentapp.MessageInput{
			WorkspaceID:    "test",
			PrincipalID:    owner.ID,
			ConversationID: multi.ID,
			Role:           agentapp.MessageRoleUser,
			ContentText:    text,
		}); err != nil {
			t.Fatalf("append message: %v", err)
		}
	}
	for _, conversationID := range []string{manual.ID, multi.ID} {
		req := httptest.NewRequest(http.MethodGet, "/chat/"+conversationID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		server.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `&#34;titlePending&#34;:true`) {
			t.Fatalf("conversation %s should not render title pending:\n%s", conversationID, rec.Body.String())
		}
	}
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("title model calls = %d, want 0", calls.Load())
	}
}

func TestChatTurnStreamsDatastarSignalsAndPersistsEvents(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal, token := chatPrincipalAndToken(t, ctx, store)
	var calls atomic.Int64
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"Let me look that up.","tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_dashboards","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
			return
		case 2:
			writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"Executive Sales is available."},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}}`)
			return
		default:
			writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"Available dashboards"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`)
		}
	}))
	defer modelServer.Close()
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	agentService := agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentService, DefaultWorkspaceID: "test"})

	signals := map[string]any{
		"csrfToken":        "test-token",
		"agentTurnPending": false,
		"agent": map[string]any{
			"activeConversationId": "",
			"composer":             map[string]any{"value": "What dashboards can I use?", "disabled": false, "placeholder": "Ask"},
			"conversations": []map[string]any{
				{"id": "agentconv_existing", "title": "New conversation", "titlePending": ""},
			},
			"status":     map[string]any{"enabled": true, "running": false},
			"transcript": []map[string]any{},
		},
	}
	req := chatSignalsRequest(http.MethodPost, "/chat/turns", token, signals)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"event: datastar-patch-signals", `"transcript"`, `"kind":"tool"`, `"status":"running"`, `"status":"complete"`, "Executive Sales"} {
		if !strings.Contains(body, want) {
			t.Fatalf("turn response missing %q:\n%s", want, body)
		}
	}
	if strings.Index(body, "Let me look that up.") == -1 || strings.Index(body, "List Dashboards") == -1 || strings.Index(body, "Let me look that up.") > strings.Index(body, "List Dashboards") {
		t.Fatalf("assistant preamble should appear before tool row:\n%s", body)
	}
	for _, unwanted := range []string{`"events"`, "message_delta", "message_appended", "agent_end"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("turn response leaked raw event %q:\n%s", unwanted, body)
		}
	}
	conversations, err := testAgentRepository(store).ListConversations(ctx, "test", principal.ID)
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("conversations = %#v", conversations)
	}
	firstPatch := firstDatastarPatchSignals(body)
	if !strings.Contains(firstPatch, `"conversations":[]`) {
		t.Fatalf("first streaming patch should preserve the existing conversation rail:\n%s", firstPatch)
	}
	if strings.Contains(firstPatch, `"title":"New conversation"`) {
		t.Fatalf("first streaming patch refreshed the draft conversation into the rail:\n%s", firstPatch)
	}
	for _, want := range []string{"window.history.replaceState", "/chat/" + conversations[0].ID} {
		if !strings.Contains(body, want) {
			t.Fatalf("draft turn response missing URL replacement %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "pending:user") {
		t.Fatalf("turn response emitted an optimistic pending message:\n%s", body)
	}
	if !strings.Contains(body, `"titlePending":true`) {
		t.Fatalf("draft turn response should mark generated title pending:\n%s", body)
	}
	if !strings.Contains(body, `"running":false`) {
		t.Fatalf("draft turn should finish normal composer running state before title generation:\n%s", body)
	}
}

func TestChatTurnWithActiveConversationDoesNotReplaceURL(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	owner, token := chatPrincipalAndToken(t, ctx, store)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeRawJSON(t, w, `{"choices":[{"message":{"role":"assistant","content":"Still here."},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`)
	}))
	defer modelServer.Close()
	service := agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", Model: "fake-model"})
	owned, err := service.CreateConversation(ctx, agentapp.Scope{WorkspaceID: "test", PrincipalID: owner.ID}, "Owned")
	if err != nil {
		t.Fatalf("create owned: %v", err)
	}
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", BaseURL: modelServer.URL, Model: "fake-model"}), DefaultWorkspaceID: "test"})

	req := chatSignalsRequest(http.MethodPost, "/chat/turns", token, map[string]any{"agent": map[string]any{
		"activeConversationId": owned.ID,
		"composer":             map[string]any{"value": "Continue"},
	}})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "window.history.replaceState") {
		t.Fatalf("active conversation turn should not replace URL:\n%s", rec.Body.String())
	}
}

func TestChatUpdatesStreamsConversationPatches(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal, token := chatPrincipalAndToken(t, ctx, store)
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	service := agentapp.NewService(fakeMetrics{}, testAgentRepository(store), agentapp.Config{APIKey: "key", Model: "fake-model"})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, Agent: service, DefaultWorkspaceID: "test"})
	scope := agentapp.Scope{WorkspaceID: "test", PrincipalID: principal.ID}
	key := chatStreamID(scope, "client-test")

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/chat/updates", nil).WithContext(reqCtx)
	req.Header.Set("Authorization", "Bearer "+token)
	req.AddCookie(&http.Cookie{Name: "ld_client_id", Value: "client-test"})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Routes().ServeHTTP(rec, req)
	}()
	waitForBrokerSubscription(t, server, key)
	server.broker.Publish(key, dashboardstream.Patch{"agent": map[string]any{"conversations": []api.AgentConversationResponse{{ID: "agentconv_title", Title: "Available dashboards"}}}})
	time.Sleep(25 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	for _, want := range []string{"event: datastar-patch-signals", `"conversations"`, "Available dashboards"} {
		if !strings.Contains(body, want) {
			t.Fatalf("chat updates stream missing %q:\n%s", want, body)
		}
	}
}

func chatPrincipalAndToken(t *testing.T, ctx context.Context, store *platform.Store) (testPrincipalRef, string) {
	t.Helper()
	principal := testPrincipal(t, ctx, store, "viewer@example.com", "Viewer", "viewer")
	token := testAPIToken(t, ctx, store, principal.ID, "chat-test")
	return testPrincipalRef{ID: principal.ID}, token
}

func waitForBrokerSubscription(t *testing.T, server *Server, key string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		count := server.broker.SubscriberCount(key)
		if count > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for broker subscription %q", key)
}

func waitForAgentConversationTitle(t *testing.T, store *platform.Store, workspaceID, principalID, conversationID, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var got string
	for time.Now().Before(deadline) {
		conversation, err := testAgentRepository(store).GetConversation(context.Background(), workspaceID, principalID, conversationID)
		if err != nil {
			t.Fatalf("get conversation: %v", err)
		}
		got = conversation.Title
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("conversation title = %q, want %q", got, want)
}

type testPrincipalRef struct {
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

func firstDatastarPatchSignals(body string) string {
	const marker = "event: datastar-patch-signals"
	start := strings.Index(body, marker)
	if start == -1 {
		return ""
	}
	rest := body[start:]
	if end := strings.Index(rest, "\n\n"); end != -1 {
		return rest[:end]
	}
	return rest
}
