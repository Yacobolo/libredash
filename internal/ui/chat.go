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
				h.Div(h.Class(appShellClass),
					sidebar(sidebarConfigForWorkspace(catalog, "chat", roleLabel)),
					h.Section(h.Class("grid h-svh min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-app"), h.Aria("label", "LibreDash chat"),
						workspaceHeader("Agent", "Chat", "Ask read-only questions about dashboards, metric views, and semantic models.", nil),
						h.Div(h.Class("grid min-h-0 min-w-0 grid-cols-[260px_minmax(0,1fr)] overflow-hidden max-md:grid-cols-1"),
							chatConversationRail(signal.Conversations, signal.ActiveConversationID),
							h.Div(h.Class("grid min-h-0 min-w-0 grid-rows-[minmax(0,1fr)_auto] overflow-hidden bg-app"),
								g.El("ld-chat-thread",
									h.Class("block min-h-0 min-w-0 overflow-hidden"),
									g.Attr("data-attr:events", "$agent.events"),
									g.Attr("data-attr:status", "$agent.status"),
									g.Attr("data-attr:conversation-id", "$agent.activeConversationId"),
									g.Text(signal.Status.Error),
								),
								g.El("ld-chat-composer",
									h.Class("block border-t border-outline-variant bg-app"),
									g.Attr("data-attr:value", "$agent.composer.value"),
									g.Attr("data-attr:disabled", "$agent.status.running || $agent.composer.disabled"),
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

func chatConversationRail(conversations []api.AgentConversationResponse, activeID string) g.Node {
	return h.Aside(h.Class("grid min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] border-r border-outline-variant bg-app max-md:hidden"), h.Aria("label", "Agent conversations"),
		h.Div(h.Class("border-b border-outline-variant px-3 py-3"),
			h.H2(h.Class("m-0 text-body-sm font-semibold text-fg-default"), g.Text("Conversations")),
		),
		h.Div(h.Class("min-h-0 overflow-auto p-2"),
			g.If(len(conversations) == 0, h.P(h.Class("m-0 px-2 py-2 text-body-sm text-fg-muted"), g.Text("No conversations yet."))),
			g.Map(conversations, func(conversation api.AgentConversationResponse) g.Node {
				className := "mb-1 grid w-full min-w-0 rounded-default border px-2 py-2 text-left text-body-sm text-fg-default"
				if conversation.ID == activeID {
					className += " border-outline-accent bg-accent-muted"
				} else {
					className += " border-transparent bg-transparent hover:border-outline-muted hover:bg-control-hover"
				}
				return h.Button(
					h.Type("button"),
					h.Class(className),
					g.Attr("data-on:click", "$agent.activeConversationId = '"+conversation.ID+"'; "+postAction("/chat/conversations/select")),
					h.Span(h.Class("truncate font-medium"), g.Text(conversation.Title)),
					h.Span(h.Class("truncate text-caption text-fg-muted"), g.Text(conversation.UpdatedAt)),
				)
			}),
		),
	)
}
