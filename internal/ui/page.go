package ui

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/staticasset"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/pkg/pagestream"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func postAction(path string) string {
	return "@post('" + path + "', {headers: window.LibreDashCommand.headers()})"
}

func patchAction(path string) string {
	return "@patch('" + path + "', {headers: window.LibreDashCommand.headers()})"
}

func staticAsset(path string) string {
	return staticasset.URL(path)
}

const appRootClass = "min-h-svh bg-app text-fg-default"

func updatesURL(route uisignals.RouteKind, pairs ...string) string {
	values := url.Values{}
	values.Set("route", string(route))
	for i := 0; i+1 < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) == "" {
			continue
		}
		values.Set(pairs[i], pairs[i+1])
	}
	return "/updates?" + values.Encode()
}

func runtimeSignal(kind uisignals.RouteKind) uisignals.RouteRuntimeSignal {
	return uisignals.RouteRuntimeSignal{
		Kind: kind,
	}
}

func inspectorScript() g.Node {
	return h.Script(h.Type("module"), h.Src(staticAsset("/static/datastar-inspector.js")))
}

func inspectorElement() g.Node {
	return g.El("datastar-inspector")
}

func pageHead(extra ...g.Node) []g.Node {
	nodes := []g.Node{
		h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
		h.Script(h.Src(staticAsset("/static/theme.js"))),
		h.Script(h.Type("module"), h.Src(staticAsset("/static/command.js"))),
	}
	return append(nodes, extra...)
}

func csrfMeta(token string) g.Node {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return h.Meta(h.Name("csrf-token"), h.Content(token))
}

type LoginPageOptions struct {
	LocalAuth          bool
	SSOAuth            bool
	MustChangePassword bool
	ProviderLabel      string
	CSRFToken          string
}

func LoginPage(options ...LoginPageOptions) g.Node {
	opts := LoginPageOptions{SSOAuth: true, ProviderLabel: "Sign in with Azure Active Directory"}
	if len(options) > 0 {
		opts = options[0]
		if strings.TrimSpace(opts.ProviderLabel) == "" {
			opts.ProviderLabel = "Sign in with Azure Active Directory"
		}
	}
	favicon := "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='10' fill='%230969da'/%3E%3Ctext x='32' y='39' text-anchor='middle' font-family='Arial,sans-serif' font-size='20' font-weight='700' fill='white'%3ELD%3C/text%3E%3C/svg%3E"
	loginUpdatesURL := updatesURL(uisignals.RouteLogin)
	return pagestream.RenderPage(pagestream.PageSpec{
		Title: "LibreDash Login",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: []g.Node{
			csrfMeta(opts.CSRFToken),
			h.Link(h.Rel("icon"), h.Href(favicon)),
			h.Link(h.Rel("stylesheet"), h.Href(staticAsset("/static/app.css"))),
			h.Script(h.Src(staticAsset("/static/theme.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/login-page.js"))),
			loginBackgroundLoaderScript(),
			inspectorScript(),
		},
		MainAttrs:  []g.Node{h.Class(appRootClass)},
		UpdatesURL: loginUpdatesURL,
		Body: []g.Node{
			g.El("ld-login-page"),
			inspectorElement(),
		},
	})
}

func LoginBootstrapSignals() map[string]any {
	return map[string]any{
		"page": uisignals.LoginPageSignal{
			Kind:                uisignals.RouteLogin,
			Title:               "LibreDash",
			ProviderLabel:       "Sign in with Azure Active Directory",
			LocalAuth:           false,
			SSOAuth:             true,
			MustChangePassword:  false,
			BackgroundModuleSrc: staticAsset("/static/topology-background.js"),
		},
		"status": dashboard.Status{},
	}
}

func LoginBootstrapSignalsForOptions(opts LoginPageOptions) map[string]any {
	signals := LoginBootstrapSignals()
	page := signals["page"].(uisignals.LoginPageSignal)
	page.LocalAuth = opts.LocalAuth
	page.SSOAuth = opts.SSOAuth
	page.MustChangePassword = opts.MustChangePassword
	if strings.TrimSpace(opts.ProviderLabel) != "" {
		page.ProviderLabel = opts.ProviderLabel
	}
	signals["page"] = page
	return signals
}

func CatalogPage(catalog dashboard.Catalog, chromeOptions ...ChromeOption) g.Node {
	return catalogPageDocument(catalog, catalogPageSignal(catalog), chromeOptions...)
}

func CatalogPageForCatalogs(catalogs []dashboard.Catalog, chromeOptions ...ChromeOption) g.Node {
	if len(catalogs) == 0 {
		return CatalogPage(dashboard.Catalog{}, chromeOptions...)
	}
	dashboards := []uisignals.CatalogDashboardSignal{}
	for _, catalog := range catalogs {
		for _, report := range catalog.Dashboards {
			dashboards = append(dashboards, uisignals.CatalogDashboardSignal{
				ID:            catalog.Workspace.ID + "." + report.ID,
				Title:         report.Title,
				Description:   report.Description,
				SemanticModel: report.SemanticModel,
				PageCount:     report.PageCount,
				Tags:          append([]string{}, report.Tags...),
				Href:          "/workspaces/" + catalog.Workspace.ID + "/dashboards/" + report.ID,
			})
		}
	}
	page := catalogPageSignal(catalogs[0])
	page.Dashboards = dashboards
	return catalogPageDocument(catalogs[0], page, chromeOptions...)
}

func catalogPageDocument(catalog dashboard.Catalog, page uisignals.CatalogPageSignal, chromeOptions ...ChromeOption) g.Node {
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForCatalog(catalog)}
	applyChromeOptions(&chrome, chromeOptions)
	catalogUpdatesURL := updatesURL(uisignals.RouteCatalog)
	return pagestream.RenderPage(pagestream.PageSpec{
		Title: "LibreDash Dashboards",
		HTMLAttrs: []g.Node{
			g.Attr("data-color-mode", "auto"),
			g.Attr("data-light-theme", "light"),
			g.Attr("data-dark-theme", "dark"),
		},
		Head: pageHead(
			h.Script(h.Type("module"), h.Src(staticAsset("/static/app-shell.js"))),
			h.Script(h.Type("module"), h.Src(staticAsset("/static/catalog-page.js"))),
			inspectorScript(),
		),
		MainAttrs:  []g.Node{h.Class(appRootClass)},
		UpdatesURL: catalogUpdatesURL,
		Body: []g.Node{
			g.El("ld-app-shell",
				g.El("ld-catalog-page",
					g.Attr("slot", "page"),
				),
			),
			inspectorElement(),
		},
	})
}

func CatalogBootstrapSignals(catalog dashboard.Catalog, chromeOptions ...ChromeOption) map[string]any {
	return CatalogBootstrapSignalsForPage(catalog, catalogPageSignal(catalog), chromeOptions...)
}

func CatalogBootstrapSignalsForCatalogs(catalogs []dashboard.Catalog, chromeOptions ...ChromeOption) map[string]any {
	if len(catalogs) == 0 {
		return CatalogBootstrapSignals(dashboard.Catalog{}, chromeOptions...)
	}
	dashboards := []uisignals.CatalogDashboardSignal{}
	for _, catalog := range catalogs {
		for _, report := range catalog.Dashboards {
			dashboards = append(dashboards, uisignals.CatalogDashboardSignal{
				ID:            catalog.Workspace.ID + "." + report.ID,
				Title:         report.Title,
				Description:   report.Description,
				SemanticModel: report.SemanticModel,
				PageCount:     report.PageCount,
				Tags:          append([]string{}, report.Tags...),
				Href:          "/workspaces/" + catalog.Workspace.ID + "/dashboards/" + report.ID,
			})
		}
	}
	page := catalogPageSignal(catalogs[0])
	page.Dashboards = dashboards
	return CatalogBootstrapSignalsForPage(catalogs[0], page, chromeOptions...)
}

func CatalogBootstrapSignalsForPage(catalog dashboard.Catalog, page uisignals.CatalogPageSignal, chromeOptions ...ChromeOption) map[string]any {
	chrome := uisignals.ChromeSignal{Sidebar: uisignals.SidebarConfigForCatalog(catalog)}
	applyChromeOptions(&chrome, chromeOptions)
	return map[string]any{
		"chrome": chrome,
		"page":   page,
		"status": dashboard.Status{},
	}
}

type recordTable = uisignals.RecordTableSignal
type recordTableColumn = uisignals.RecordTableColumnSignal
type recordTableBadge = uisignals.RecordTableBadgeSignal

type ChromeOption func(*uisignals.ChromeSignal)

func WithChatSidebar(signal ChatSignal) ChromeOption {
	return func(chrome *uisignals.ChromeSignal) {
		uisignals.AttachChatSidebar(&chrome.Sidebar, signal)
	}
}

func applyChromeOptions(chrome *uisignals.ChromeSignal, options []ChromeOption) {
	for _, option := range options {
		if option != nil {
			option(chrome)
		}
	}
}

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
			Href:          "/workspaces/" + catalog.Workspace.ID + "/dashboards/" + report.ID,
		})
	}
	return uisignals.CatalogPageSignal{
		Kind:        uisignals.RouteCatalog,
		Title:       "Dashboards",
		Description: "Reports backed by semantic models.",
		Dashboards:  dashboards,
	}
}

func recordTableBadgeValue(value, tone string) any {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return recordTableBadge{Label: value, Tone: tone}
}

func loginBackgroundLoaderScript() g.Node {
	return h.Script(h.Type("module"), h.Src(staticAsset("/static/login-background-loader.js")))
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
