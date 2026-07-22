package ui

import (
	"github.com/Yacobolo/leapview/internal/brand"
	"github.com/Yacobolo/leapview/internal/dashboard"
	uiactions "github.com/Yacobolo/leapview/internal/ui/actions"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
	"github.com/Yacobolo/leapview/pkg/pagestream"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func ChatPage(catalog dashboard.Catalog, workspaceID, csrfToken, roleLabel, view string, state ChatViewState) g.Node {
	chatUpdatesURL := updatesURL(uisignals.RouteChat, "workspace", workspaceID, "view", view, "conversation", state.Agent.ActiveConversationID)
	chatBasePath := "/chats"
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:             brand.Name + " Chat",
		DatastarScriptURL: datastarScriptURL(),
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			csrfMeta(csrfToken),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/chat-page.js"))),
			inspectorScript(),
		),
		MainAttrs:  []g.Node{h.Class(appRootClass)},
		UpdatesURL: chatUpdatesURL,
		Body: []g.Node{
			g.El("lv-app-shell",
				g.Attr("data-on:lv-chat-reference-search__debounce.200ms", "$agentReferenceSearch.query = evt.detail.query; $agentReferenceSearch.requestId = evt.detail.requestId; "+uiactions.Get(chatBasePath+"/references/search", "agentReferenceSearch", "agentContext")),
				g.El("lv-chat-page",
					g.Attr("slot", "page"),
					g.Attr("workspace-id", workspaceID),
					g.Attr("view", view),
					g.Attr("data-indicator", "agentTurnPending"),
					g.Attr("data-on:lv-chat-submit", "$agent.composer.value = evt.detail.input; $agentContext.references = evt.detail.references; "+uiactions.Post(chatBasePath+"/turns", "agent", "agentContext")),
				),
			),
			inspectorElement(),
		},
	})
}

func ChatBootstrapSignals(catalog dashboard.Catalog, workspaceID, roleLabel, view string, state ChatViewState) map[string]any {
	envelope := uisignals.ChatInitialEnvelope(catalog, workspaceID, roleLabel, view, state)
	envelope.Runtime = uisignals.RouteRuntimeSignal{Kind: uisignals.RouteChat, WorkspaceID: uisignals.Optional(workspaceID)}
	return map[string]any{
		"chrome":               envelope.Chrome,
		"page":                 envelope.Page,
		"runtime":              envelope.Runtime,
		"agent":                envelope.Agent,
		"agentContext":         envelope.AgentContext,
		"agentReferenceSearch": envelope.AgentReferenceSearch,
		"visuals":              envelope.Visuals,
	}
}

// ChatSignalPatch keeps the chat body and the sidebar history synchronized.
// The sidebar renders from the chrome signal, so conversation changes must be
// streamed to both roots in the same patch.
func ChatSignalPatch(state ChatViewState) pagestream.SignalPatch {
	patch := ChatConversationsPatch(state.Agent.Conversations, state.Agent.ActiveConversationID)
	patch["agent"] = state.Agent
	patch["visuals"] = state.Visuals
	return patch
}

// ChatConversationsPatch updates conversation state without replacing the
// rest of the agent or chrome signal trees.
func ChatConversationsPatch(conversations []ChatConversationSummary, activeConversationID string) pagestream.SignalPatch {
	agent := ChatSignal{
		ActiveConversationID: activeConversationID,
		Conversations:        conversations,
	}
	return pagestream.SignalPatch{
		"agent": map[string]any{
			"conversations": conversations,
		},
		"chrome": map[string]any{
			"sidebar": map[string]any{
				"history": map[string]any{
					"items": uisignals.ChatHistoryItems(agent),
				},
			},
		},
	}
}
