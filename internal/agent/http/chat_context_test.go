package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/internal/ui"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
)

func TestChatReferenceSearchUsesGlobalScopeAndEchoesRequestIdentity(t *testing.T) {
	results := make([]uisignals.AgentReferenceSignal, 30)
	for index := range results {
		results[index] = uisignals.AgentReferenceSignal{
			Reference: uisignals.AgentReferenceKeySignal{WorkspaceID: "sales", Type: "field", ID: "field-" + string(rune('a'+index))},
			Name:      "Field", Workspace: uisignals.AgentReferenceWorkspaceSignal{ID: "sales", Name: "Sales"},
			Locations: []uisignals.AgentReferenceLocationSignal{}, Context: []string{},
		}
	}
	searchedContext := agent.TurnContext{Surface: "invalid"}
	searchedLimit := 0
	handler := NewHandler(Options{
		SearchReferences: func(_ *http.Request, context agent.TurnContext, _ string, limit int) ([]uisignals.AgentReferenceSignal, error) {
			searchedContext = context
			searchedLimit = limit
			return results, nil
		},
	})
	signals, err := json.Marshal(map[string]any{
		"agentReferenceSearch": map[string]any{
			"query": "field", "requestId": 7,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/?datastar="+url.QueryEscape(string(signals)), nil)
	request.Header.Set("Accept", "text/event-stream")
	response := httptest.NewRecorder()

	handler.ChatReferenceSearch(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if searchedContext.WorkspaceID != "" {
		t.Fatalf("searched workspace = %q, want global scope", searchedContext.WorkspaceID)
	}
	if searchedLimit != maxChatReferenceSearchResults {
		t.Fatalf("searched limit = %d, want %d", searchedLimit, maxChatReferenceSearchResults)
	}
	if got := strings.Count(response.Body.String(), `"type":"field"`); got != 24 {
		t.Fatalf("result count = %d, want 24:\n%s", got, response.Body.String())
	}
	for _, want := range []string{`"query":"field"`, `"requestId":7`, `"workspaceId":"sales"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("search response missing %s:\n%s", want, response.Body.String())
		}
	}
	for _, deadField := range []string{`"dashboardId":"executive-sales"`, `"pageId":"overview"`} {
		if strings.Contains(response.Body.String(), deadField) {
			t.Fatalf("search response echoed redundant context %s:\n%s", deadField, response.Body.String())
		}
	}
}

func TestChatSignalPatchKeepsEmbeddedArtifactsSeparateFromDashboardVisuals(t *testing.T) {
	state := ui.ChatViewState{
		Agent:   ui.ChatSignal{Conversations: []ui.ChatConversationSummary{}, Transcript: []ui.ChatTranscriptItemSignal{}},
		Visuals: map[string]uisignals.DashboardVisual{},
	}
	embedded := chatSignalPatch(state, true)
	if _, ok := embedded["agentVisuals"]; !ok {
		t.Fatalf("embedded patch = %#v, want agentVisuals", embedded)
	}
	if _, ok := embedded["visuals"]; ok {
		t.Fatalf("embedded patch = %#v, must not replace dashboard visuals", embedded)
	}
	standalone := chatSignalPatch(state, false)
	if _, ok := standalone["visuals"]; !ok {
		t.Fatalf("standalone patch = %#v, want visuals", standalone)
	}
}
