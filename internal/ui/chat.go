package ui

import (
	"github.com/Yacobolo/libredash/internal/dashboard"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/pkg/pagestream"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func ChatPage(catalog dashboard.Catalog, workspaceID, csrfToken, roleLabel, view string, signal ChatSignal) g.Node {
	envelope := uisignals.ChatInitialEnvelope(catalog, workspaceID, csrfToken, roleLabel, view, signal)
	chatUpdatesURL := updatesURL(uisignals.RouteChat)
	envelope.Runtime = runtimeSignal(uisignals.RouteChat, chatUpdatesURL)
	envelope.Runtime.WorkspaceID = workspaceID
	chatBasePath := "/chat"
	return pagestream.RenderPage(pagestream.PageSpec{
		Title: "LibreDash Chat",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/chat-page.js"))),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		),
		MainAttrs: []g.Node{h.Class(appRootClass)},
		Signals: map[string]any{
			"chrome":    envelope.Chrome,
			"page":      envelope.Page,
			"runtime":   envelope.Runtime,
			"csrfToken": envelope.CSRFToken,
			"agent":     envelope.Agent,
			"visuals":   envelope.Visuals,
			"tables":    envelope.Tables,
		},
		UpdatesURL: chatUpdatesURL,
		Body: []g.Node{
			g.El("ld-app-shell",
				g.Attr("chrome", jsonString(envelope.Chrome)),
				g.Attr("data-attr:chrome", "$chrome"),
				g.El("ld-chat-page",
					g.Attr("slot", "page"),
					g.Attr("page", jsonString(envelope.Page)),
					g.Attr("agent", jsonString(envelope.Agent)),
					g.Attr("data-indicator", "agentTurnPending"),
					g.Attr("data-attr:page", "$page"),
					g.Attr("data-attr:agent", "$agent"),
					g.Attr("data-attr:visuals", "$visuals"),
					g.Attr("data-attr:tables", "$tables"),
					g.Attr("data-attr:pending", "$agentTurnPending || $agent.status.running"),
					g.Attr("data-attr:composerdisabled", "$agentTurnPending || $agent.status.running || $agent.composer.disabled"),
					g.Attr("data-on:ld-chat-submit", "$agent.composer.value = evt.detail.input; "+postAction(chatBasePath+"/turns")),
				),
			),
			inspectorElement(),
		},
	})
}
