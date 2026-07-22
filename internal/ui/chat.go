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
				g.El("lv-chat-page",
					g.Attr("slot", "page"),
					g.Attr("workspace-id", workspaceID),
					g.Attr("view", view),
					g.Attr("data-indicator", "agentTurnPending"),
					g.Attr("data-on:lv-chat-submit", "$agent.composer.value = evt.detail.input; "+uiactions.Post(chatBasePath+"/turns")),
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
		"chrome":  envelope.Chrome,
		"page":    envelope.Page,
		"runtime": envelope.Runtime,
		"agent":   envelope.Agent,
		"visuals": envelope.Visuals,
	}
}
