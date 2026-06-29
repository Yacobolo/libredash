package ui

import (
	"github.com/Yacobolo/libredash/internal/dashboard"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

func ChatPage(catalog dashboard.Catalog, csrfToken, roleLabel string, signal ChatSignal) g.Node {
	envelope := uisignals.ChatInitialEnvelope(catalog, csrfToken, roleLabel, signal)
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Chat",
		Language: "en",
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
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				ds.Signals(map[string]any{
					"chrome":    envelope.Chrome,
					"page":      envelope.Page,
					"runtime":   envelope.Runtime,
					"csrfToken": envelope.CSRFToken,
					"agent":     envelope.Agent,
				}),
				g.If(signal.Status.Enabled, ds.Init("@get('/chat/updates', {openWhenHidden: true})")),
				g.El("ld-app-shell",
					g.Attr("chrome", jsonString(envelope.Chrome)),
					g.Attr("data-attr:chrome", "JSON.stringify($chrome)"),
					g.El("ld-chat-page",
						g.Attr("slot", "page"),
						g.Attr("page", jsonString(envelope.Page)),
						g.Attr("agent", jsonString(envelope.Agent)),
						g.Attr("data-indicator", "agentTurnPending"),
						g.Attr("data-attr:page", "JSON.stringify($page)"),
						g.Attr("data-attr:agent", "JSON.stringify($agent)"),
						g.Attr("data-attr:pending", "$agentTurnPending || $agent.status.running"),
						g.Attr("data-attr:composerdisabled", "$agentTurnPending || $agent.status.running || $agent.composer.disabled"),
						g.Attr("data-on:ld-chat-submit", "$agent.composer.value = evt.detail.input; "+postAction("/chat/turns")),
					),
				),
				inspectorElement(),
			),
		},
	})
}
