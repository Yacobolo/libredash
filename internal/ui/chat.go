package ui

import (
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

func ChatPage(catalog dashboard.Catalog, csrfToken, roleLabel string, signal api.AgentChatSignal) g.Node {
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Chat",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/chat.js"))),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		),
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				ds.Signals(map[string]any{
					"csrfToken": csrfToken,
					"agent":     signal,
				}),
				g.If(signal.Status.Enabled, ds.Init("@get('/chat/updates', {openWhenHidden: true})")),
				h.Div(h.Class(reportShellClass),
					sidebar(sidebarConfigForChat(catalog, roleLabel)),
					g.El("ld-sub-sidebar",
						h.Class("block min-h-0 border-r border-outline-variant bg-app max-md:hidden"),
						g.Attr("data-attr:config", chatSubSidebarConfigExpression()),
					),
					h.Section(h.Class("grid h-svh min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-app"), h.Aria("label", "LibreDash chats"),
						workspaceHeader("Agent", "Chats", "Ask read-only questions about dashboards, semantic models, measures, and fields.", nil),
						h.Div(h.Class("grid min-h-0 min-w-0 overflow-hidden bg-app"),
							h.Div(h.Class("grid min-h-0 min-w-0 grid-rows-[minmax(0,1fr)_auto] overflow-hidden bg-app"),
								g.El("ld-chat-thread",
									h.Class("block min-h-0 min-w-0 overflow-hidden"),
									g.Attr("data-attr:transcript", "$agent.transcript"),
									g.Attr("data-attr:status", "$agent.status"),
									g.Attr("data-attr:conversation-id", "$agent.activeConversationId"),
									g.Text(signal.Status.Error),
								),
								g.El("ld-chat-composer",
									h.Class("block border-t border-outline-variant bg-app"),
									g.Attr("data-indicator", "agentTurnPending"),
									g.Attr("data-attr:value", "$agent.composer.value"),
									g.Attr("data-attr:disabled", "$agentTurnPending || $agent.status.running || $agent.composer.disabled"),
									g.Attr("data-attr:pending", "$agentTurnPending || $agent.status.running"),
									g.Attr("data-attr:placeholder", "$agent.composer.placeholder"),
									g.Attr("data-on:ld-chat-submit", "$agent.composer.value = evt.detail.input; "+postAction("/chat/turns")),
								),
							),
						),
					),
				),
				inspectorElement(),
			),
		},
	})
}

func sidebarConfigForChat(catalog dashboard.Catalog, roleLabel string) map[string]any {
	config := sidebarConfigForWorkspace(catalog, "chat", roleLabel)
	config["compact"] = true
	return config
}

func chatSubSidebarConfigExpression() string {
	return `JSON.stringify({
label: 'Chats',
railLabel: 'Chats',
ariaLabel: 'Chat conversations',
storageKey: 'libredash-chat-conversations-collapsed',
activeId: $agent.activeConversationId,
emptyText: 'No conversations yet.',
disabled: ($agent.status && $agent.status.running) || false,
collapsible: false,
numbered: false,
items: [
{id: 'new', title: 'New chat', href: '/chat/new', active: !$agent.activeConversationId},
...($agent.conversations || []).map((conversation) => ({
id: conversation.id,
title: conversation.title || 'Conversation',
href: '/chat/' + encodeURIComponent(conversation.id),
active: conversation.id === $agent.activeConversationId,
pending: conversation.titlePending || false
}))
]
})`
}
