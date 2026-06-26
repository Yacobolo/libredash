package ui

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	lucide "github.com/eduardolat/gomponents-lucide"
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

const (
	appRootClass             = "min-h-svh bg-app text-fg-default"
	appShellClass            = "grid min-h-svh grid-cols-app-shell bg-app max-sm:grid-cols-1"
	reportShellClass         = "grid min-h-svh grid-cols-report-shell bg-app max-sm:grid-cols-1"
	appMainClass             = "grid min-w-0 min-h-svh grid-rows-app-main bg-app"
	catalogMainClass         = appMainClass + " gap-3 px-4 py-4 max-sm:min-h-0 max-sm:p-3"
	reportMainClass          = appMainClass + " h-svh min-h-0 overflow-hidden"
	metricMainClass          = "grid h-svh min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-app"
	cardClass                = "grid min-h-min-card max-w-card grid-rows-card rounded-default border border-outline-variant bg-panel p-4 shadow-resting-sm"
	cardTitleClass           = "m-0 mt-1 text-body-md leading-snug font-semibold text-fg-default"
	cardDescriptionClass     = "m-0 mt-2 text-body-sm leading-relaxed font-normal text-fg-muted"
	cardFooterClass          = "mt-4 flex items-center justify-between gap-3 border-t border-outline-muted pt-3 text-caption font-medium text-fg-muted"
	eyebrowClass             = "m-0 mb-1 text-caption leading-tight font-medium uppercase text-fg-muted"
	visualCardClass          = "h-full min-h-0 w-full overflow-hidden rounded-default border border-outline-variant bg-panel"
	actionButtonClass        = "inline-flex size-action min-h-action items-center justify-center rounded-default border border-outline-variant bg-transparent p-0 text-fg-default hover:bg-control-hover focus-visible:bg-control-hover focus-visible:outline-0 disabled:cursor-not-allowed disabled:border-control-border-disabled disabled:bg-control-disabled disabled:text-control-fg-disabled"
	metricActionButtonClass  = "inline-flex size-8 items-center justify-center rounded-small border border-transparent bg-transparent p-0 text-fg-muted no-underline transition-colors duration-micro ease-hover hover:border-outline-muted hover:bg-control-hover hover:text-fg-default focus-visible:border-outline-accent focus-visible:bg-control-hover focus-visible:text-fg-default focus-visible:outline-0"
	primaryLinkButtonClass   = "inline-flex min-h-control-xs items-center justify-center gap-1.5 rounded-small bg-button-primary px-2.5 text-caption font-semibold text-on-primary no-underline hover:bg-button-primary-hover focus-visible:bg-button-primary-hover focus-visible:outline-0"
	tagClass                 = "rounded-full border border-outline-muted bg-panel-muted px-2 py-0.5 text-caption font-medium uppercase text-fg-muted"
	metricContentColumnClass = "grid min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
)

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
		h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
	}
	return append(nodes, extra...)
}

func LoginPage() g.Node {
	favicon := "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='10' fill='%230969da'/%3E%3Ctext x='32' y='39' text-anchor='middle' font-family='Arial,sans-serif' font-size='20' font-weight='700' fill='white'%3ELD%3C/text%3E%3C/svg%3E"
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
			h.Script(h.Type("module"), h.Src(staticAsset("/static/theme.js"))),
			loginBackgroundLoaderScript(),
			inspectorScript(),
			h.Script(h.Type("module"), h.Src("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.2/bundles/datastar.js")),
		},
		Body: []g.Node{
			h.Main(h.Class("relative grid h-svh min-h-svh place-items-center overflow-hidden bg-app p-6 text-fg-default"), h.Aria("label", "LibreDash login"),
				g.El("ld-topology-background",
					h.Class("absolute inset-0 block bg-app"),
					g.Attr("data-login-background", ""),
					g.Attr("data-module-src", staticAsset("/static/topology-background.js")),
					ds.Init("document.dispatchEvent(new CustomEvent('libredash-login-background-init'))", ds.ModifierDelay, ds.Duration(900*time.Millisecond)),
				),
				h.Div(h.Class("pointer-events-none absolute inset-0 z-overlay bg-app/80"), h.Aria("hidden", "true")),
				h.Button(h.Type("button"), h.Class("absolute right-4 top-4 z-modal inline-grid size-8 appearance-none place-items-center rounded-default border border-outline-variant bg-control text-fg-muted shadow-resting-sm hover:bg-control-hover hover:text-fg-default focus-visible:border-outline-accent focus-visible:bg-control-hover focus-visible:text-fg-default focus-visible:outline-0 max-sm:right-3 max-sm:top-3"), g.Attr("data-theme-toggle", ""),
					lucide.Monitor(loginThemeIconAttrs("system")...),
					lucide.Sun(loginThemeIconAttrs("light")...),
					lucide.Moon(loginThemeIconAttrs("dark")...),
				),
				h.Section(h.Class("relative z-modal grid w-full max-w-login-panel justify-items-center gap-5 rounded-default border border-outline-variant bg-panel p-6 text-center shadow-resting-md max-sm:px-5 max-sm:py-6"),
					h.Div(h.Class("grid justify-items-center"), h.Aria("hidden", "true"),
						h.H1(h.Class("m-0 text-title-md font-semibold leading-snug text-fg-default"), g.Text("LibreDash")),
					),
					h.Button(h.Type("button"), h.Class("inline-grid min-h-control-xl w-full appearance-none grid-cols-login-button items-center gap-3 rounded-default border border-outline-variant bg-control px-4 text-body-md font-medium text-fg-default shadow-resting-sm hover:border-outline-accent hover:bg-control-hover focus-visible:border-outline-accent focus-visible:bg-control-hover focus-visible:outline-0"),
						h.Span(h.Class("grid size-5 grid-cols-2 grid-rows-2 gap-px"), h.Aria("hidden", "true"),
							h.Span(h.Class("block bg-danger")),
							h.Span(h.Class("block bg-success")),
							h.Span(h.Class("block bg-accent")),
							h.Span(h.Class("block bg-warning")),
						),
						h.Span(g.Text("Sign in with Azure Active Directory")),
					),
				),
				h.Div(h.Class("absolute"), inspectorElement()),
			),
		},
	})
}

func CatalogPage(catalog dashboard.Catalog) g.Node {
	return c.HTML5(c.HTML5Props{
		Title:    "LibreDash Dashboards",
		Language: "en",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/sidebar.js"))),
			inspectorScript(),
		),
		Body: []g.Node{
			h.Main(h.Class(appRootClass),
				h.Div(h.Class(appShellClass),
					sidebar(sidebarConfigForCatalog(catalog)),
					h.Section(h.Class(catalogMainClass), h.Aria("label", "LibreDash dashboard catalog"),
						workspaceHeader(
							"",
							"Dashboards",
							"Reports backed by semantic models.",
							nil,
						),
						h.Div(h.Class("grid grid-cols-catalog-grid items-start justify-start gap-4"),
							g.Map(catalog.Dashboards, dashboardCard),
						),
					),
				),
				inspectorElement(),
			),
		},
	})
}

func dashboardCard(report dashboard.CatalogDashboard) g.Node {
	eyebrow := report.SemanticModel
	if eyebrow == "" {
		eyebrow = "Dashboard"
	}
	return h.Article(h.Class(cardClass),
		h.Div(h.Class("min-w-0"),
			h.P(h.Class(eyebrowClass), g.Text(eyebrow)),
			h.H2(h.Class(cardTitleClass), g.Text(report.Title)),
			h.P(h.Class(cardDescriptionClass), g.Text(report.Description)),
		),
		h.Div(h.Class("mt-4 flex flex-wrap gap-2"),
			g.Map(report.Tags, func(tag string) g.Node {
				return h.Span(h.Class(tagClass), g.Text(tag))
			}),
		),
		h.Footer(h.Class(cardFooterClass),
			h.Span(g.Textf("%d pages", report.PageCount)),
			h.A(h.Class(primaryLinkButtonClass), h.Href("/dashboards/"+report.ID),
				lucide.ExternalLink(buttonIconAttrs()...),
				h.Span(g.Text("Open")),
			),
		),
	)
}

func metricTabCount(count int) g.Node {
	return h.Span(h.Class("inline-flex min-w-4 items-center justify-center rounded-full bg-panel-muted px-1.5 py-px text-caption font-medium leading-none text-fg-muted"), g.Text(strconv.Itoa(count)))
}

type metricGrid struct {
	Columns  []metricGridColumn `json:"columns"`
	Rows     []map[string]any   `json:"rows"`
	Empty    string             `json:"empty"`
	MinWidth string             `json:"minWidth,omitempty"`
}

type metricGridColumn struct {
	ID      string `json:"id"`
	Header  string `json:"header"`
	Kind    string `json:"kind,omitempty"`
	Align   string `json:"align,omitempty"`
	HrefKey string `json:"hrefKey,omitempty"`
	Width   string `json:"width,omitempty"`
}

type metricGridBadge struct {
	Label string `json:"label"`
	Tone  string `json:"tone,omitempty"`
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

func sidebar(config map[string]any) g.Node {
	return g.El("ld-sidebar", h.Class("border-r border-outline-variant max-sm:border-b max-sm:border-r-0"), g.Attr("config", jsonString(config)))
}

func sidebarConfigForCatalog(catalog dashboard.Catalog) map[string]any {
	modelID := ""
	modelTitle := ""
	if len(catalog.Models) > 0 {
		modelID = catalog.Models[0].ID
		modelTitle = catalog.Models[0].Title
	}
	return sidebarConfig(catalog, "dashboards", "", workspaceDisplayTitle(catalog), "Dashboards", "Discovery", modelID, modelTitle, false)
}

func sidebarConfigForWorkspace(catalog dashboard.Catalog, active, roleLabel string) map[string]any {
	config := sidebarConfig(catalog, active, "", workspaceDisplayTitle(catalog), "Workspace", "Published assets", "", "", false)
	if roleLabel != "" {
		config["userRole"] = roleLabel
	}
	return config
}

func sidebarConfig(catalog dashboard.Catalog, active, dashboardID, workspaceTitle, dashboardTitle, pageTitle, modelID, modelTitle string, compact bool) map[string]any {
	return map[string]any{
		"workspaceTitle": workspaceTitle,
		"active":         active,
		"dashboardId":    dashboardID,
		"dashboardTitle": dashboardTitle,
		"pageTitle":      pageTitle,
		"modelId":        modelID,
		"modelTitle":     modelTitle,
		"compact":        compact,
		"groups":         sidebarGroups(catalog),
	}
}

func sidebarGroups(catalog dashboard.Catalog) []map[string]any {
	workspaceID := catalog.Workspace.ID
	if strings.TrimSpace(workspaceID) == "" {
		workspaceID = "libredash"
	}
	return []map[string]any{
		{
			"label": "Navigation",
			"items": []map[string]any{
				{"id": "dashboards", "label": "Dashboards", "href": "/", "icon": "dashboard", "meta": "Reports"},
				{"id": "chat", "label": "Chats", "href": "/chat", "icon": "chat", "meta": "Agent interface"},
				{"id": "workspaces", "label": "Workspaces", "href": "/workspaces", "icon": "catalog", "meta": "Published assets"},
				{"id": "connections", "label": "Connections", "href": "/connections", "icon": "data", "meta": "Data access"},
				{"id": "admin", "label": "Admin", "href": "/admin", "icon": "settings", "meta": "Read-only administration"},
			},
		},
	}
}

func workspaceDisplayTitle(catalog dashboard.Catalog) string {
	if strings.TrimSpace(catalog.Workspace.Title) != "" {
		return catalog.Workspace.Title
	}
	return "LibreDash Workspace"
}

func workspaceHeader(eyebrow, title, detail string, actions g.Node) g.Node {
	return h.Header(h.Class("grid min-w-0 grid-cols-workspace-header items-center gap-2 border-b border-outline-muted px-4 py-2.5"),
		h.Div(h.Class("min-w-0"),
			g.If(eyebrow != "", h.P(h.Class(eyebrowClass), g.Text(eyebrow))),
			h.H1(h.Class("m-0 truncate text-title-sm font-semibold leading-snug text-fg-default"), g.Text(title)),
			g.If(detail != "", h.P(h.Class("m-0 mt-1 truncate text-body-sm font-normal leading-snug text-fg-muted"), g.Text(detail))),
		),
		h.Div(h.Class("inline-flex min-w-0 items-center justify-end gap-2"), actions),
	)
}

func jsonString(value any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func iconAttrs() g.Node {
	return g.Attr("aria-hidden", "true")
}

func buttonIconAttrs() []g.Node {
	return []g.Node{g.Attr("aria-hidden", "true"), h.Class("size-4 shrink-0"), h.Style("stroke-width: 1.75")}
}

func metricActionIconAttrs() []g.Node {
	return []g.Node{g.Attr("aria-hidden", "true"), h.Class("size-4 shrink-0"), h.Style("stroke-width: 1.75")}
}

func loginThemeIconAttrs(mode string) []g.Node {
	return []g.Node{g.Attr("aria-hidden", "true"), g.Attr("data-theme-icon", mode), h.Class("hidden size-4 shrink-0"), h.Style("stroke-width: 1.75")}
}
