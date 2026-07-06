package ui

import (
	"github.com/Yacobolo/libredash/internal/dashboard"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/pkg/pagestream"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func ChatPage(catalog dashboard.Catalog, workspaceID, csrfToken, roleLabel, view string, signal ChatSignal) g.Node {
	chatUpdatesURL := updatesURL(uisignals.RouteChat, "workspace", workspaceID, "view", view, "conversation", signal.ActiveConversationID)
	chatBasePath := "/chat"
	return pagestream.RenderPage(pagestream.PageSpec{
		Title: "LibreDash Chat",
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
			g.El("ld-app-shell",
				g.El("ld-chat-page",
					g.Attr("slot", "page"),
					g.Attr("workspace-id", workspaceID),
					g.Attr("view", view),
					g.Attr("data-indicator", "agentTurnPending"),
					g.Attr("data-on:ld-chat-submit", "$agent.composer.value = evt.detail.input; "+postAction(chatBasePath+"/turns")),
				),
			),
			inspectorElement(),
		},
	})
}

func ChatBootstrapSignals(catalog dashboard.Catalog, workspaceID, roleLabel, view string, signal ChatSignal) map[string]any {
	envelope := uisignals.ChatInitialEnvelope(catalog, workspaceID, roleLabel, view, signal)
	envelope.Runtime = uisignals.RouteRuntimeSignal{Kind: uisignals.RouteChat, WorkspaceID: workspaceID}
	return map[string]any{
		"chrome":  envelope.Chrome,
		"page":    envelope.Page,
		"runtime": envelope.Runtime,
		"agent":   envelope.Agent,
		"visuals": envelope.Visuals,
		"tables":  envelope.Tables,
	}
}
