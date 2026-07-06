package ui

import (
	"github.com/Yacobolo/libredash/internal/dashboard"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/pkg/pagestream"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func DataExplorerPage(catalog dashboard.Catalog, page uisignals.DataExplorerPageSignal, explorer uisignals.DataExplorerSignal, roleLabel, csrfToken string, chromeOptions ...ChromeOption) g.Node {
	catalog = catalogWithoutWorkspaceContext(catalog)
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForWorkspace(catalog, "data", roleLabel)}
	applyChromeOptions(&chrome, chromeOptions)
	explorerUpdatesURL := updatesURL(uisignals.RouteData, "workspace", explorer.Command.WorkspaceID, "object", explorer.Command.ObjectKey)
	_ = chrome
	return pagestream.RenderPage(pagestream.PageSpec{
		Title: page.Title,
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			csrfMeta(csrfToken),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/data-explorer.js"))),
			inspectorScript(),
		),
		MainAttrs:  []g.Node{h.Class(appRootClass)},
		UpdatesURL: explorerUpdatesURL,
		Body: []g.Node{
			g.El("ld-app-shell",
				g.El("ld-data-explorer",
					g.Attr("slot", "page"),
					g.Attr("data-on:ld-data-explorer-command", "$dataExplorerCommand = evt.detail; "+postAction("/data/command")),
				),
			),
			inspectorElement(),
		},
	})
}

func DataExplorerBootstrapSignals(catalog dashboard.Catalog, page uisignals.DataExplorerPageSignal, explorer uisignals.DataExplorerSignal, roleLabel string, chromeOptions ...ChromeOption) map[string]any {
	catalog = catalogWithoutWorkspaceContext(catalog)
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForWorkspace(catalog, "data", roleLabel)}
	applyChromeOptions(&chrome, chromeOptions)
	return map[string]any{
		"chrome":              chrome,
		"page":                page,
		"dataExplorer":        explorer,
		"dataExplorerCommand": explorer.Command,
		"status":              dashboard.Status{},
	}
}
