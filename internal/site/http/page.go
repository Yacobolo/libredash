package http

import (
	"strings"

	"github.com/Yacobolo/libredash/pkg/pagestream"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func sitePage() g.Node {
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:      "LibreDash — BI dashboards as code",
		HTMLAttrs:  siteHTMLAttrs(),
		Head:       siteHead(),
		MainAttrs:  []g.Node{h.Class("site-page")},
		UpdatesURL: "/updates",
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(false),
			h.Section(h.ID("main-content"), h.Class("site-hero"),
				g.El("ld-topology-background", h.Class("site-hero-background"), g.Attr("aria-hidden", "true")),
				h.Div(h.Class("site-hero-content"),
					h.P(h.Class("site-eyebrow"), g.Text("Business intelligence, defined in code")),
					h.H1(g.Text("Build dashboards your data team can trust.")),
					h.P(h.Class("site-lede"), g.Text("LibreDash turns versioned semantic models and dashboard definitions into a fast, interactive BI workspace.")),
					h.Div(h.Class("site-actions"), h.A(h.Class("site-button site-button-primary"), h.Href("/docs/getting-started"), g.Text("Get started")), h.A(h.Class("site-button"), h.Href("#demo"), g.Text("Explore the demo"))),
				),
				h.Div(h.Class("site-hero-proof"),
					siteProofItem("database", "Semantic layer", "Define measures, dimensions, and relationships once."),
					siteProofItem("chart", "Interactive visuals", "Render focused components from streamed signal updates."),
					siteProofItem("git-branch", "Versioned workspace", "Keep dashboards in the same workflow as your product."),
				),
			),
			h.Div(h.Class("site-shell"),
				h.Section(h.ID("demo"), h.Class("site-product-proof"),
					h.Div(h.Class("site-product-copy"),
						h.P(h.Class("site-eyebrow"), g.Text("A real LibreDash surface")),
						h.H2(g.Text("Start with the model. End with a dashboard.")),
						h.P(h.Class("site-lede"), g.Text("LibreDash keeps the semantic model, dashboard definition, and runtime visual in one small, reviewable system.")),
						h.A(h.Class("site-button"), h.Href("/charts"), g.Text("Browse every visual")),
					),
					h.Div(h.Class("site-demo"),
						h.Div(h.Class("site-section-heading"), h.P(h.Class("site-eyebrow"), g.Text("Live visual")), h.H3(g.Text("A server-owned chart payload")), h.P(g.Text("Switch the metric to stream a new chart shape into the component."))),
						h.Div(h.Class("site-demo-controls"),
							h.Button(h.Type("button"), g.Attr("data-on:click", "@post('/demo', {payload: {demo: {metric: 'revenue'}}})"), g.Text("Revenue")),
							h.Button(h.Type("button"), g.Attr("data-on:click", "@post('/demo', {payload: {demo: {metric: 'orders'}}})"), g.Text("Orders")),
						),
						g.El("ld-site-chart-demo"),
					),
				),
				h.Section(h.Class("site-principles-section"),
					h.Div(h.Class("site-principles-heading"), h.P(h.Class("site-eyebrow"), g.Text("Designed for data teams")), h.H2(g.Text("A deliberately small BI stack.")), h.P(g.Text("The essentials for making trustworthy dashboards without recreating an entire data platform."))),
					h.Div(h.Class("site-principles"),
						sitePrinciple("blocks", "Semantic first", "Model measures, dimensions, and relationships once, then reuse them everywhere."),
						sitePrinciple("server", "Server-owned data", "Keep queries, filters, and payloads in one predictable runtime."),
						sitePrinciple("chart", "Visual breadth", "Use a shared contract for charts, KPIs, tables, matrices, and pivots."),
						sitePrinciple("git-branch", "Versioned by default", "Keep dashboards and models in the same reviewable workflow as the rest of your product."),
						sitePrinciple("boxes", "Composable surfaces", "Bring focused web components together without a frontend framework layer."),
						sitePrinciple("radio", "Interactive by default", "Stream small state updates into focused web components."),
					),
				),
				h.Section(h.Class("site-cta"),
					h.P(h.Class("site-eyebrow"), g.Text("Open source BI")),
					h.H2(g.Text("Bring your dashboards into the codebase.")),
					h.P(g.Text("Explore the components, contracts, and visual types behind LibreDash.")),
					h.Div(h.Class("site-actions"), h.A(h.Class("site-button site-button-primary"), h.Href("/docs/getting-started"), g.Text("Get started")), h.A(h.Class("site-button"), h.Href("/charts"), g.Text("See the visual gallery"))),
				),
			),
			siteFooter(),
		},
	})
}

func chartsPage() g.Node {
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:      "LibreDash chart showcase",
		HTMLAttrs:  siteHTMLAttrs(),
		Head:       siteHead(),
		MainAttrs:  []g.Node{h.Class("site-page")},
		UpdatesURL: "/updates?view=charts",
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(false),
			h.Div(h.Class("site-shell site-showcase-shell"),
				h.Section(h.ID("main-content"), h.Class("site-showcase-intro"),
					h.P(h.Class("site-eyebrow"), g.Text("LibreDash visual system")),
					h.H1(g.Text("Every chart type, using one contract.")),
					h.P(h.Class("site-lede"), g.Text("Each visual below is a real LibreDash component rendered from the same renderer-neutral chart payload shape.")),
				),
				g.El("ld-site-chart-showcase"),
			),
			siteFooter(),
		},
	})
}

func docsIndexPage() g.Node {
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:      "LibreDash documentation",
		HTMLAttrs:  siteHTMLAttrs(),
		Head:       siteHead(),
		MainAttrs:  []g.Node{h.Class("site-page")},
		UpdatesURL: "/updates",
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(true),
			siteDocsLayout(nil, siteDocsIndex()),
		},
	})
}

func docsArticlePage(document siteDocument) g.Node {
	updatesURL := "/updates"
	if document.chartID != "" {
		updatesURL = "/updates?view=charts"
	}
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:      document.title,
		HTMLAttrs:  siteHTMLAttrs(),
		Head:       siteHead(),
		MainAttrs:  []g.Node{h.Class("site-page")},
		UpdatesURL: updatesURL,
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(true),
			siteDocsLayout(&document, siteDocsArticle(document)),
		},
	})
}

func siteHead() []g.Node {
	return []g.Node{
		h.Meta(h.Name("view-transition"), h.Content("same-origin")),
		h.Link(h.Rel("stylesheet"), h.Href("/shared/app.css")),
		h.Link(h.Rel("stylesheet"), h.Href("/static/site.css")),
		h.Script(h.Src("/shared/theme.js")),
		h.Script(h.Type("module"), h.Src("/static/site-page.js")),
	}
}

func siteHeader(hasDocsDrawer bool) g.Node {
	actions := []g.Node{
		h.Div(h.Class("site-nav-links"), h.A(h.Href("/docs"), g.Text("Docs")), h.A(h.Href("/#demo"), g.Text("Demo")), h.A(h.Href("/charts"), g.Text("Charts"))),
	}
	if hasDocsDrawer {
		actions = append(actions, g.El("ld-site-docs-drawer-toggle"))
	}
	actions = append(actions, g.El("ld-site-theme-toggle"))
	actions = append(actions, g.El("ld-site-mobile-menu"))

	return h.Header(h.Class("site-header"),
		h.Nav(h.Class("site-nav"),
			h.A(h.Class("site-brand"), h.Href("/"), g.Text("LibreDash")),
			h.Div(h.Class("site-nav-actions"), g.Group(actions)),
		),
	)
}

func siteHTMLAttrs() []g.Node {
	return []g.Node{
		g.Attr("data-color-mode", "auto"),
		g.Attr("data-light-theme", "light"),
		g.Attr("data-dark-theme", "dark"),
	}
}

func sitePrinciple(icon, title, body string) g.Node {
	return h.Article(h.Class("site-principle"), siteFeatureIcon(icon), h.H3(g.Text(title)), h.P(g.Text(body)))
}

func siteProofItem(icon, title, body string) g.Node {
	return h.Article(h.Class("site-proof-item"), siteFeatureIcon(icon), h.H3(g.Text(title)), h.P(g.Text(body)))
}

func siteFeatureIcon(name string) g.Node {
	return g.El("ld-site-feature-icon", g.Attr("name", name), g.Attr("aria-hidden", "true"))
}

func siteFooter() g.Node {
	return h.Footer(h.Class("site-footer"), g.Attr("role", "contentinfo"),
		h.Div(h.Class("site-footer-content"),
			h.Div(h.Class("site-footer-brand-block"),
				h.A(h.Class("site-brand"), h.Href("/"), g.Text("LibreDash")),
				h.P(g.Text("A small, code-native BI workspace for teams that value trustworthy dashboards.")),
			),
			siteFooterGroup("Explore", []siteFooterLink{
				{label: "Documentation", href: "/docs"},
				{label: "Getting started", href: "/docs/getting-started"},
				{label: "Live demo", href: "/#demo"},
				{label: "Visual gallery", href: "/charts"},
			}),
			siteFooterGroup("Project", []siteFooterLink{
				{label: "GitHub", href: "https://github.com/Yacobolo/libredash"},
				{label: "Issues", href: "https://github.com/Yacobolo/libredash/issues"},
			}),
		),
		h.Div(h.Class("site-footer-bottom"), h.P(g.Text("LibreDash — dashboards as code."))),
	)
}

type siteFooterLink struct {
	label string
	href  string
}

func siteFooterGroup(title string, links []siteFooterLink) g.Node {
	items := make([]g.Node, 0, len(links))
	for _, link := range links {
		items = append(items, h.A(h.Href(link.href), g.Text(link.label)))
	}
	return h.Nav(g.Attr("aria-label", title), h.H2(g.Text(title)), g.Group(items))
}

func siteDocsLayout(document *siteDocument, content ...g.Node) g.Node {
	return h.Div(h.Class("site-docs-layout"),
		siteDocsSidebar(document),
		h.Button(h.Class("site-docs-drawer-backdrop"), h.Type("button"), g.Attr("aria-label", "Close documentation menu"), g.Attr("aria-hidden", "true"), g.Attr("tabindex", "-1"), g.Attr("data-site-docs-drawer-close", "true")),
		h.Div(h.Class("site-docs-content"),
			h.Div(h.Class("site-docs-reading-layout"),
				h.Div(h.Class("site-guide-shell"),
					siteDocsArticleHeader(document),
					g.Group(content),
				),
				g.El("ld-site-article-toc"),
			),
		),
	)
}

func siteDocsArticleHeader(document *siteDocument) g.Node {
	rootLabel := "Documentation"
	rootHref := "/docs"
	if document != nil && document.breadcrumbRoot != "" {
		rootLabel = document.breadcrumbRoot
		rootHref = document.breadcrumbRootHref
	}
	breadcrumb := []g.Node{h.Li(h.A(h.Href(rootHref), g.Text(rootLabel)))}
	if document != nil {
		if document.breadcrumb == rootLabel {
			breadcrumb[0] = h.Li(h.Span(g.Attr("aria-current", "page"), g.Text(rootLabel)))
		} else {
			breadcrumb = append(breadcrumb, h.Li(h.Span(g.Attr("aria-current", "page"), g.Text(document.breadcrumb))))
		}
	} else {
		breadcrumb[0] = h.Li(h.Span(g.Attr("aria-current", "page"), g.Text("Documentation")))
	}

	return h.Header(h.Class("site-docs-article-header"),
		h.Nav(h.Class("site-docs-breadcrumb"), g.Attr("aria-label", "Breadcrumb"), h.Ol(g.Group(breadcrumb))),
	)
}

func siteDocsIndex() g.Node {
	documents := allSiteDocuments()
	items := make([]g.Node, 0, len(documents))
	for _, document := range documents {
		items = append(items, h.Li(
			h.A(h.Href("/docs/"+document.slug), h.H2(g.Text(document.title)), h.P(g.Text(document.summary))),
		))
	}
	return h.Article(h.ID("main-content"), h.Class("site-docs-article site-docs-index"),
		h.H1(g.Text("Documentation")),
		h.P(g.Text("Guides and references for building dashboards with LibreDash.")),
		h.Nav(g.Attr("aria-label", "Documentation articles"), h.Ul(h.Class("site-docs-index-list"), g.Group(items))),
	)
}

func siteDocsSidebar(current *siteDocument) g.Node {
	documentationLinks := make([]g.Node, 0, len(siteDocuments)+1)
	documentationActive := false
	configurationActive := current != nil && (current.slug == "configuration" || strings.HasPrefix(current.slug, "config/"))
	chartActive := current != nil && current.slug == chartOverviewDocument.slug
	for _, document := range siteDocuments {
		if document.slug == "configuration" {
			continue
		}
		isCurrent := current != nil && current.slug == document.slug
		if isCurrent {
			documentationActive = true
		}
		documentationLinks = append(documentationLinks, h.Li(siteDocsLink("/docs/"+document.slug, document.title, isCurrent)))
	}
	visualLinks := []g.Node{
		h.Li(siteDocsLink("/docs/"+chartOverviewDocument.slug, "Overview", current != nil && current.slug == chartOverviewDocument.slug)),
	}
	for _, document := range visualDocuments {
		isCurrent := current != nil && current.slug == document.slug
		if isCurrent {
			chartActive = true
		}
		visualLinks = append(visualLinks, h.Li(siteDocsLink("/docs/"+document.slug, document.title, isCurrent)))
	}
	configurationLinks := []g.Node{
		h.Li(siteDocsLink("/docs/configuration", "Environment", current != nil && current.slug == "configuration")),
	}
	for _, document := range configurationReferenceDocuments {
		isCurrent := current != nil && current.slug == document.slug
		configurationLinks = append(configurationLinks, h.Li(siteDocsLink("/docs/"+document.slug, document.title, isCurrent)))
	}
	if configurationActive {
		documentationActive = true
	}
	documentationLinks = append(documentationLinks, h.Li(siteDocsNavGroup("configuration", "Configuration", configurationActive, configurationLinks)))
	if chartActive {
		documentationActive = true
	}
	documentationLinks = append(documentationLinks, h.Li(siteDocsNavGroup("charts", chartDocuments.section, chartActive, visualLinks)))
	cliGuideActive := current != nil && (current.slug == "cli" || current.slug == "cli/authentication" || current.slug == "cli/targets" || current.slug == "cli/validate-publish" || current.slug == "cli/automation" || current.slug == "cli/troubleshooting")
	cliReferenceActive := current != nil && strings.HasPrefix(current.slug, "cli/") && !cliGuideActive
	cliActive := cliGuideActive || cliReferenceActive
	cliLinks := make([]g.Node, 0, len(cliGuideDocuments)+1)
	for _, document := range cliGuideDocuments {
		label := document.title
		if document.slug == "cli" {
			label = "Overview"
		}
		cliLinks = append(cliLinks, h.Li(siteDocsLink("/docs/"+document.slug, label, current != nil && current.slug == document.slug)))
	}
	commandLinks := make([]g.Node, 0, len(cliReferenceDocuments))
	for _, document := range cliReferenceDocuments {
		commandLinks = append(commandLinks, h.Li(siteDocsLink("/docs/"+document.slug, document.title, current != nil && current.slug == document.slug)))
	}
	cliLinks = append(cliLinks, h.Li(siteDocsNavGroup("cli-commands", "Command reference", cliReferenceActive, commandLinks)))
	if cliActive {
		documentationActive = true
	}
	documentationLinks = append(documentationLinks, h.Li(siteDocsNavGroup("cli", "CLI reference", cliActive, cliLinks)))
	apiLinks := []g.Node{
		h.Li(siteDocsLink("/docs/api", "Overview", current != nil && current.slug == "api")),
	}
	apiActive := current != nil && current.slug == "api"
	for _, document := range apiReferenceDocuments[1:] {
		isCurrent := current != nil && current.slug == document.slug
		if isCurrent {
			apiActive = true
		}
		apiLinks = append(apiLinks, h.Li(siteDocsLink("/docs/"+document.slug, document.title, isCurrent)))
	}
	return h.Aside(h.Class("site-docs-sidebar"), h.ID("site-docs-sidebar"),
		h.Div(h.Class("site-docs-drawer-actions"),
			g.El("ld-site-docs-drawer-toggle", g.Attr("placement", "drawer")),
		),
		h.Nav(g.Attr("aria-label", "Documentation"),
			siteDocsNavGroup("documentation", "Documentation", documentationActive, documentationLinks),
			siteDocsNavGroup("api-reference", "API reference", apiActive, apiLinks),
		),
	)
}

func siteDocsNavGroup(group, label string, active bool, links []g.Node) g.Node {
	class := "site-docs-nav-group"
	if active {
		class += " site-docs-nav-group-active"
	}
	attributes := []g.Node{h.Class(class), g.Attr("data-site-docs-group", group)}
	if active {
		attributes = append(attributes, g.Attr("open", "true"))
	}
	return g.El("details", g.Group(attributes),
		g.El("summary", g.Text(label)),
		h.Ul(h.Class("site-docs-nav-tree"), g.Group(links)),
	)
}

func siteDocsLink(href, label string, current bool) g.Node {
	class := "site-docs-link"
	if current {
		class += " site-docs-link-current"
	}
	attrs := []g.Node{h.Class(class), h.Href(href)}
	if current {
		attrs = append(attrs, g.Attr("aria-current", "page"))
	}
	return h.A(g.Group(attrs), g.Text(label))
}
