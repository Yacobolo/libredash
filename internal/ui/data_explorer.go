package ui

import (
	"encoding/json"

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
	explorerUpdatesURL := updatesURL(uisignals.RouteData)
	signals := map[string]any{
		"chrome":              chrome,
		"page":                page,
		"dataExplorer":        explorer,
		"dataExplorerCommand": explorer.Command,
		"csrfToken":           csrfToken,
		"runtime":             runtimeSignal(uisignals.RouteData, explorerUpdatesURL),
		"status":              dashboard.Status{},
	}
	return pagestream.RenderPage(pagestream.PageSpec{
		Title: page.Title,
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/data-explorer.js"))),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		),
		MainAttrs:  []g.Node{h.Class(appRootClass)},
		Signals:    signals,
		UpdatesURL: explorerUpdatesURL,
		Body: []g.Node{
			g.El("ld-app-shell",
				g.Attr("chrome", jsonString(chrome)),
				g.Attr("data-attr:chrome", "$chrome"),
				g.El("ld-data-explorer",
					g.Attr("slot", "page"),
					g.Attr("page", mustJSONString(page)),
					g.Attr("data-attr:page", "$page"),
					g.Attr("dataexplorer", mustJSONString(explorer)),
					g.Attr("data-attr:dataexplorer", "$dataExplorer"),
					g.Attr("data-on:ld-data-explorer-command", "$dataExplorerCommand = evt.detail; "+postAction("/data/command")),
				),
			),
			inspectorElement(),
		},
	})
}

func mustJSONString(value any) string {
	out, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(out)
}
