package ui

import (
	"testing"

	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
)

func TestChatSignalPatchStreamsSidebarHistoryState(t *testing.T) {
	pending := true
	state := ChatViewState{Agent: ChatSignal{
		ActiveConversationID: "agentconv_1",
		Conversations: []ChatConversationSummary{{
			ID:           "agentconv_1",
			Title:        "New conversation",
			TitlePending: &pending,
		}},
	}}

	patch := ChatSignalPatch(state)
	assertChatHistoryPatch(t, patch, true)
}

func TestChatConversationsPatchStreamsCompletedSidebarHistoryState(t *testing.T) {
	pending := false
	patch := ChatConversationsPatch([]ChatConversationSummary{{
		ID:           "agentconv_1",
		Title:        "Quick check-in",
		TitlePending: &pending,
	}}, "agentconv_1")

	assertChatHistoryPatch(t, patch, false)
}

func assertChatHistoryPatch(t *testing.T, patch map[string]any, wantPending bool) {
	t.Helper()
	chrome, ok := patch["chrome"].(map[string]any)
	if !ok {
		t.Fatalf("chat patch chrome = %#v, want object", patch["chrome"])
	}
	sidebar, ok := chrome["sidebar"].(map[string]any)
	if !ok {
		t.Fatalf("chat patch sidebar = %#v, want object", chrome["sidebar"])
	}
	history, ok := sidebar["history"].(map[string]any)
	if !ok {
		t.Fatalf("chat patch history = %#v, want object", sidebar["history"])
	}
	items, ok := history["items"].([]uisignals.SidebarHistoryItemSignal)
	if !ok || len(items) != 1 {
		t.Fatalf("chat patch history items = %#v, want one typed item", history["items"])
	}
	if items[0].Pending == nil || *items[0].Pending != wantPending || !items[0].Active {
		t.Fatalf("chat patch history item = %#v, want pending=%t active=true", items[0], wantPending)
	}
}
