package ui

import (
	"encoding/json"

	"github.com/Yacobolo/libredash/internal/dashboard"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

func DataExplorerPage(catalog dashboard.Catalog, page uisignals.DataExplorerPageSignal, explorer uisignals.DataExplorerSignal, roleLabel, csrfToken string, chromeOptions ...ChromeOption) g.Node {
	catalog = catalogWithoutWorkspaceContext(catalog)
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForWorkspace(catalog, "data", roleLabel)}
	applyChromeOptions(&chrome, chromeOptions)
	signals := map[string]any{
		"chrome":              chrome,
		"page":                page,
		"dataExplorer":        explorer,
		"dataExplorerCommand": explorer.Command,
		"csrfToken":           csrfToken,
		"runtime":             uisignals.RouteRuntimeSignal{Kind: uisignals.RouteData},
		"status":              dashboard.Status{},
	}
	return c.HTML5(c.HTML5Props{
		Title:    page.Title,
		Language: "en",
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
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				ds.Signals(signals),
				ds.Init("@get('/data/updates', {openWhenHidden: true})"),
				g.El("ld-app-shell",
					g.Attr("chrome", jsonString(chrome)),
					g.Attr("data-attr:chrome", "JSON.stringify($chrome)"),
					g.El("ld-data-explorer",
						g.Attr("slot", "page"),
						g.Attr("page", mustJSONString(page)),
						g.Attr("data-attr:page", "JSON.stringify($page)"),
						g.Attr("dataexplorer", mustJSONString(explorer)),
						g.Attr("data-attr:dataexplorer", "JSON.stringify($dataExplorer)"),
						g.Attr("data-on:ld-data-explorer-command", "$dataExplorerCommand = evt.detail; "+postAction("/data/command")),
					),
				),
				inspectorElement(),
			),
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
