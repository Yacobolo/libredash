package ui

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	g "maragu.dev/gomponents"
	ds "maragu.dev/gomponents-datastar"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

func postAction(path string) string {
	return "@post('" + path + "', {headers: {'X-CSRF-Token': $csrfToken}})"
}

func postActionWithCSRFSignal(path, signal string) string {
	return "@post('" + path + "', {headers: {'X-CSRF-Token': " + signal + "}})"
}

func staticAsset(path string) string {
	return path + "?v=dev"
}

const appRootClass = "min-h-svh bg-app text-fg-default"

func inspectorScript() g.Node {
	return h.Script(h.Type("module"), h.Src(staticAsset("/static/datastar-inspector.js")))
}

func inspectorElement() g.Node {
	return g.El("datastar-inspector")
}

func pageHead(extra ...g.Node) []g.Node {
	nodes := []g.Node{
		h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
		h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
		h.Script(h.Src(staticAsset("/static/theme.js"))),
	}
	return append(nodes, extra...)
}

func LoginPage() g.Node {
	favicon := "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='10' fill='%230969da'/%3E%3Ctext x='32' y='39' text-anchor='middle' font-family='Arial,sans-serif' font-size='20' font-weight='700' fill='white'%3ELD%3C/text%3E%3C/svg%3E"
	page := uisignals.LoginPageSignal{
		Kind:                uisignals.RouteLogin,
		Title:               "LibreDash",
		ProviderLabel:       "Sign in with Azure Active Directory",
		BackgroundModuleSrc: staticAsset("/static/topology-background.js"),
	}
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Login",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: []g.Node{
			h.Link(h.Rel("preconnect"), h.Href("https://cdn.jsdelivr.net")),
			h.Link(h.Rel("icon"), h.Href(favicon)),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			h.Script(h.Src(staticAsset("/static/theme.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/login-page.js"))),
			loginBackgroundLoaderScript(),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		},
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				ds.Signals(map[string]any{
					"page":    page,
					"runtime": uisignals.RouteRuntimeSignal{Kind: uisignals.RouteLogin},
					"status":  dashboard.Status{},
				}),
				ds.Init("document.dispatchEvent(new CustomEvent('libredash-login-background-init'))", ds.ModifierDelay, ds.Duration(900*time.Millisecond)),
				g.El("ld-login-page",
					g.Attr("page", jsonString(page)),
					g.Attr("data-attr:page", "JSON.stringify($page)"),
				),
				inspectorElement(),
			),
		},
	})
}

func CatalogPage(catalog dashboard.Catalog) g.Node {
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForCatalog(catalog)}
	page := catalogPageSignal(catalog)
	signals := map[string]any{
		"chrome":  chrome,
		"page":    page,
		"runtime": uisignals.RouteRuntimeSignal{Kind: uisignals.RouteCatalog},
		"status":  dashboard.Status{},
	}
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Dashboards",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/catalog-page.js"))),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		),
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				ds.Signals(signals),
				g.El("ld-app-shell",
					g.Attr("chrome", jsonString(chrome)),
					g.Attr("data-attr:chrome", "JSON.stringify($chrome)"),
					g.El("ld-catalog-page",
						g.Attr("slot", "page"),
						g.Attr("page", jsonString(page)),
						g.Attr("data-attr:page", "JSON.stringify($page)"),
					),
				),
				inspectorElement(),
			),
		},
	})
}

type metricGrid = uisignals.MetricGridSignal
type metricGridColumn = uisignals.MetricGridColumnSignal
type metricGridBadge = uisignals.MetricGridBadgeSignal

func catalogPageSignal(catalog dashboard.Catalog) uisignals.CatalogPageSignal {
	dashboards := make([]uisignals.CatalogDashboardSignal, 0, len(catalog.Dashboards))
	for _, report := range catalog.Dashboards {
		dashboards = append(dashboards, uisignals.CatalogDashboardSignal{
			ID:            report.ID,
			Title:         report.Title,
			Description:   report.Description,
			SemanticModel: report.SemanticModel,
			PageCount:     report.PageCount,
			Tags:          report.Tags,
			Href:          "/dashboards/" + report.ID,
		})
	}
	return uisignals.CatalogPageSignal{
		Kind:        uisignals.RouteCatalog,
		Title:       "Dashboards",
		Description: "Reports backed by semantic models.",
		Dashboards:  dashboards,
	}
}

func metricGridBadgeValue(value, tone string) any {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return metricGridBadge{Label: value, Tone: tone}
}

func loginBackgroundLoaderScript() g.Node {
	return h.Script(g.Raw(`(()=>{const schedule=(task)=>{const run=()=>{"requestIdleCallback"in window?requestIdleCallback(task,{timeout:1600}):setTimeout(task,600)};document.readyState==="complete"?run():window.addEventListener("load",run,{once:true})};document.addEventListener("libredash-login-background-init",()=>schedule(()=>{const el=document.querySelector("[data-login-background]");if(!el)return;const state=el.dataset.backgroundState;if(state==="loading"||state==="loaded")return;const src=el.dataset.moduleSrc;if(!src)return;el.dataset.backgroundState="loading";import(src).then(()=>{el.dataset.backgroundState="loaded"}).catch((error)=>{el.dataset.backgroundState="error";console.error("LibreDash login background failed to load",error)})}))})();`))
}

func displayLabel(label, fallback string) string {
	if strings.TrimSpace(label) != "" {
		return label
	}
	return fallback
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func jsonString(value any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
