package http

import (
	"net/url"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/pkg/pagestream"
	siteassets "github.com/Yacobolo/libredash/site"
	g "maragu.dev/gomponents"
	dsattr "maragu.dev/gomponents-datastar"
	h "maragu.dev/gomponents/html"
)

type sitePageMetadata struct {
	title       string
	description string
	canonical   string
	contentType string
	robots      string
}

type siteStackIntegration struct {
	label       string
	registryKey string
	icon        string
	format      bool
}

type siteStackGroupSpec struct {
	title        string
	integrations []siteStackIntegration
}

var siteStackGroups = []siteStackGroupSpec{
	{
		title: "Databases",
		integrations: []siteStackIntegration{
			{label: "PostgreSQL", registryKey: "postgres", icon: "postgresql"},
			{label: "MySQL", registryKey: "mysql", icon: "mysql"},
			{label: "SQLite", registryKey: "sqlite", icon: "sqlite"},
		},
	},
	{
		title: "Object storage",
		integrations: []siteStackIntegration{
			{label: "Amazon S3", registryKey: "s3", icon: "amazons3"},
			{label: "Azure Blob", registryKey: "azure_blob", icon: "microsoftazure"},
			{label: "Google Cloud Storage", registryKey: "gcs", icon: "googlecloudstorage"},
			{label: "Cloudflare R2", registryKey: "r2", icon: "cloudflare"},
			{label: "Hetzner Object Storage", registryKey: "s3", icon: "hetzner"},
		},
	},
	{
		title: "Formats",
		integrations: []siteStackIntegration{
			{label: "CSV", registryKey: "csv", icon: "csv", format: true},
			{label: "JSON", registryKey: "json", icon: "json", format: true},
			{label: "Parquet", registryKey: "parquet", icon: "apacheparquet", format: true},
			{label: "Excel", registryKey: "excel", icon: "excel", format: true},
			{label: "Vortex", registryKey: "vortex", icon: "vortex", format: true},
			{label: "Delta Lake", registryKey: "delta", icon: "deltalake", format: true},
			{label: "Apache Iceberg", registryKey: "iceberg", icon: "apacheiceberg", format: true},
			{label: "Lance", registryKey: "lance", icon: "lance", format: true},
			{label: "DuckLake", registryKey: "ducklake", icon: "ducklake"},
		},
	},
}

const siteDatastarScriptURL = "/static/vendor/datastar-1.0.2.js"
const siteBrandName = "LeapView"

func sitePage(metadata sitePageMetadata) g.Node {
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:             metadata.title,
		HTMLAttrs:         siteHTMLAttrs(),
		Head:              siteHead(metadata),
		MainAttrs:         []g.Node{h.Class("site-page")},
		DatastarScriptURL: siteDatastarScriptURL,
		UpdatesURL:        "/updates",
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(false),
			h.Section(h.ID("main-content"), h.Class("site-hero"),
				g.El("ld-site-flow-background", h.Class("site-hero-background"), g.Attr("aria-hidden", "true")),
				h.Div(h.Class("site-hero-layout"),
					h.Div(h.Class("site-hero-content"),
						h.P(h.Class("site-eyebrow"), g.Text("Open-source analytics as code")),
						h.H1(g.Text("The agent-native BI platform.")),
						h.P(h.Class("site-lede"), g.Text("Build dashboards as code, keep analytics in version control, and explore data with AI agents.")),
						siteHomepageActions(),
					),
					g.El("figure", h.Class("site-product-frame"),
						h.Div(h.Class("site-product-frame-bar"),
							h.Span(h.Class("site-product-frame-dots"), g.Attr("aria-hidden", "true"), h.I(), h.I(), h.I()),
							h.Span(g.Text("Visual Showcase · Overview")),
						),
						h.Img(
							h.Class("site-product-screenshot site-product-screenshot-light"),
							h.Src("/static/product-dashboard-light.png"),
							h.Alt(siteBrandName+" Visual Showcase overview with KPIs, line, donut, and bar charts, and an analytical table"),
							g.Attr("width", "1440"),
							g.Attr("height", "900"),
							g.Attr("fetchpriority", "high"),
						),
						h.Img(
							h.Class("site-product-screenshot site-product-screenshot-dark"),
							h.Src("/static/product-dashboard-dark.png"),
							h.Alt(siteBrandName+" Visual Showcase overview with KPIs, line, donut, and bar charts, and an analytical table"),
							g.Attr("width", "1440"),
							g.Attr("height", "900"),
						),
					),
				),
				h.Div(h.Class("site-proof-strip"),
					siteProofItem("blocks", "Open source", "MIT licensed"),
					siteProofItem("git-branch", "Version controlled", "Review every change"),
					siteProofItem("dashboard", "Dashboards + agents", "Two native interfaces"),
					siteProofItem("server", "Self-hosted", "Run it yourself"),
				),
			),
			h.Div(h.Class("site-shell"),
				h.Section(h.Class("site-interfaces-section"),
					h.Div(h.Class("site-interfaces-heading"),
						h.P(h.Class("site-eyebrow"), g.Text("Agent-native BI")),
						h.H2(g.Text("Dashboards and agents, together.")),
						h.P(g.Text("Use dashboards for repeatable analysis and AI agents for questions you did not plan for.")),
					),
					h.Div(h.Class("site-interfaces-grid"),
						siteInterfaceCard("dashboard", "Dashboards", "Build repeatable views for teams, reviews, and recurring decisions.", []string{"Charts and KPIs", "Filters and drill-downs", "Analytical tables"}, "Explore dashboard guides", "/docs/guides/build"),
						siteInterfaceCard("agent", "AI agents", "Ask open-ended questions and investigate data through conversation.", []string{"Natural-language questions", "Visual, verifiable answers", "The same metrics and permissions"}, "Explore agent integrations", "/docs/guides/integrate/agent"),
					),
					h.Div(h.Class("site-interface-core"),
						siteFeatureIcon("blocks"),
						h.Div(
							h.H3(g.Text("One analytics layer")),
							h.P(g.Text("Dashboards and AI agents use the same version-controlled definitions.")),
						),
						h.Ul(
							h.Li(g.Text("Same metrics")),
							h.Li(g.Text("Same permissions")),
							h.Li(g.Text("Same data")),
						),
					),
				),
				h.Section(h.ID("product"), h.Class("site-workflow"),
					h.Div(h.Class("site-section-intro"),
						h.P(h.Class("site-eyebrow"), g.Text("Analytics as code")),
						h.H2(g.Text("Ship analytics like software.")),
						h.P(g.Text("Build in code. Review in Git. Deploy with confidence.")),
					),
					h.Ol(h.Class("site-workflow-flow"), g.Attr("aria-label", "Analytics delivery workflow"),
						siteWorkflowCard("blocks", "01", "Build in code", "Create analytics in one versioned project."),
						siteWorkflowCard("git-branch", "02", "Review in Git", "Validate and approve every change before release."),
						siteWorkflowCard("server", "03", "Deploy with confidence", "Publish dashboards and AI agents together."),
					),
				),
				h.Section(h.Class("site-stack-section"),
					h.Div(h.Class("site-stack-heading"),
						h.P(h.Class("site-eyebrow"), g.Text("Works with your stack")),
						h.H2(g.Text("Keep your data stack. Add "+siteBrandName+".")),
						h.P(g.Text("Connect databases and object storage directly, or query open lakehouse formats where they already live.")),
					),
					h.Ol(h.Class("site-stack-flow"), g.Attr("aria-label", "How "+siteBrandName+" connects to your data stack"),
						siteDataStackStage(),
						siteStackProductNode(),
					),
				),
				h.Section(h.Class("site-cta"),
					h.P(h.Class("site-eyebrow"), g.Text("Open-source BI")),
					h.H2(g.Text("Put your analytics in version control.")),
					h.P(g.Text("Build your first dashboard and explore it with an AI agent.")),
					siteHomepageActions(),
				),
			),
			siteFooter(),
		},
	})
}

func chartsPage(metadata sitePageMetadata) g.Node {
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:             metadata.title,
		HTMLAttrs:         siteHTMLAttrs(),
		Head:              siteHead(metadata),
		MainAttrs:         []g.Node{h.Class("site-page")},
		DatastarScriptURL: siteDatastarScriptURL,
		UpdatesURL:        "/updates?view=charts",
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(false),
			h.Div(h.Class("site-shell site-showcase-shell"),
				h.Section(h.ID("main-content"), h.Class("site-showcase-intro"),
					h.P(h.Class("site-eyebrow"), g.Text(siteBrandName+" visual system")),
					h.H1(g.Text("Every chart type, using one contract.")),
					h.P(h.Class("site-lede"), g.Text("Each visual below is a real "+siteBrandName+" component rendered from the same renderer-neutral chart payload shape.")),
				),
				g.El("ld-site-chart-showcase"),
			),
			siteFooter(),
		},
	})
}

func docsIndexPage(metadata sitePageMetadata) g.Node {
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:             metadata.title,
		HTMLAttrs:         siteHTMLAttrs(),
		Head:              siteHead(metadata),
		MainAttrs:         []g.Node{h.Class("site-page")},
		DatastarScriptURL: siteDatastarScriptURL,
		UpdatesURL:        "/updates",
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(true),
			siteDocsLayout(nil, siteDocsIndex()),
		},
	})
}

func docsSearchPage(query string, metadata sitePageMetadata) g.Node {
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:             metadata.title,
		HTMLAttrs:         siteHTMLAttrs(),
		Head:              siteHead(metadata),
		MainAttrs:         []g.Node{h.Class("site-page")},
		DatastarScriptURL: siteDatastarScriptURL,
		UpdatesURL:        "/updates",
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(true),
			siteDocsLayout(nil, siteDocsSearch(query)),
		},
	})
}

func docsArticlePage(document siteDocument, metadata sitePageMetadata) g.Node {
	updatesURL := "/updates"
	if _, ok := visualExamplesForDocument(document.slug); ok {
		updatesURL = "/updates?view=visual-docs&document=" + url.QueryEscape(document.slug)
	}
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:             metadata.title,
		HTMLAttrs:         siteHTMLAttrs(),
		Head:              siteHead(metadata),
		MainAttrs:         []g.Node{h.Class("site-page")},
		DatastarScriptURL: siteDatastarScriptURL,
		UpdatesURL:        updatesURL,
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(true),
			siteDocsLayout(&document, siteDocsArticle(document)),
		},
	})
}

func notFoundPage(metadata sitePageMetadata) g.Node {
	return pagestream.RenderPage(pagestream.PageSpec{
		Title:             metadata.title,
		HTMLAttrs:         siteHTMLAttrs(),
		Head:              siteHead(metadata),
		MainAttrs:         []g.Node{h.Class("site-page")},
		DatastarScriptURL: siteDatastarScriptURL,
		UpdatesURL:        "/updates",
		Body: []g.Node{
			h.A(h.Class("skip-link"), h.Href("#main-content"), g.Text("Skip to content")),
			siteHeader(false),
			h.Div(h.Class("site-shell"),
				h.Section(h.ID("main-content"), h.Class("site-showcase-intro"),
					h.P(h.Class("site-eyebrow"), g.Text("404")),
					h.H1(g.Text("Page not found")),
					h.P(h.Class("site-lede"), g.Text("The page may have moved, or the address may be incomplete.")),
					h.Div(h.Class("site-actions"),
						h.A(h.Class("site-button site-button-primary"), h.Href("/docs"), g.Text("Browse documentation")),
						h.A(h.Class("site-button"), h.Href("/"), g.Text("Go to "+siteBrandName)),
					),
				),
			),
			siteFooter(),
		},
	})
}

func siteHead(metadata sitePageMetadata) []g.Node {
	nodes := []g.Node{
		h.Meta(h.Name("view-transition"), h.Content("same-origin")),
		h.Meta(h.Name("description"), h.Content(metadata.description)),
		h.Link(h.Rel("canonical"), h.Href(metadata.canonical)),
		h.Meta(g.Attr("property", "og:site_name"), h.Content(siteBrandName)),
		h.Meta(g.Attr("property", "og:title"), h.Content(metadata.title)),
		h.Meta(g.Attr("property", "og:description"), h.Content(metadata.description)),
		h.Meta(g.Attr("property", "og:type"), h.Content(metadata.contentType)),
		h.Meta(g.Attr("property", "og:url"), h.Content(metadata.canonical)),
		h.Meta(h.Name("twitter:card"), h.Content("summary")),
		h.Meta(h.Name("twitter:title"), h.Content(metadata.title)),
		h.Meta(h.Name("twitter:description"), h.Content(metadata.description)),
		h.Link(h.Rel("icon"), h.Href("/static/favicon.svg"), h.Type("image/svg+xml")),
		h.Link(h.Rel("preload"), h.Href("/shared/files/inter-latin-wght-normal.woff2"), g.Attr("as", "font"), h.Type("font/woff2"), g.Attr("crossorigin", "anonymous")),
		h.Link(h.Rel("stylesheet"), h.Href("/shared/app.css")),
		h.Link(h.Rel("stylesheet"), h.Href("/static/site.css")),
		h.Script(h.Src("/shared/theme.js")),
		h.Script(h.Type("module"), h.Src("/static/site-page.js")),
	}
	if metadata.robots != "" {
		nodes = append(nodes, h.Meta(h.Name("robots"), h.Content(metadata.robots)))
	}
	return nodes
}

func siteHeader(isDocs bool) g.Node {
	var actions []g.Node
	if isDocs {
		actions = append(actions, h.Div(h.Class("site-nav-links site-nav-links-docs"), siteActiveSearch()))
	} else {
		actions = append(actions, h.Div(h.Class("site-nav-links"),
			h.A(h.Href("/docs"), g.Text("Docs")),
			h.A(h.Href("/charts"), g.Text("Charts")),
		))
	}
	actions = append(actions, g.El("ld-site-theme-toggle"))
	if !isDocs {
		actions = append(actions, g.El("ld-site-mobile-menu"))
	}

	return h.Header(h.Class("site-header"),
		h.Nav(h.Class("site-nav"),
			siteBrandLink(),
			h.Div(h.Class("site-nav-actions"), g.Group(actions)),
		),
	)
}

func siteActiveSearch() g.Node {
	return g.El("ld-site-search",
		h.A(h.Class("site-search-fallback"), h.Href("/docs/search"), g.Text("Search")),
		h.Input(
			h.Class("site-search-active-input"),
			g.Attr("slot", "input"),
			h.Type("search"),
			h.Name("q"),
			g.Attr("aria-label", "Search documentation"),
			h.Placeholder("Search concepts, guides, commands, and APIs"),
			g.Attr("autocomplete", "off"),
			dsattr.Bind("docsSearch.query"),
			dsattr.On("input", "@get('/docs/search/active', {filterSignals: {include: /^docsSearch\\./}})", dsattr.ModifierDebounce, dsattr.Duration(200*time.Millisecond)),
			dsattr.Indicator("docsSearch.loading"),
		),
	)
}

func siteBrandLink() g.Node {
	return h.A(h.Class("site-brand"), h.Href("/"),
		g.El("ld-site-brand-mark", g.Attr("aria-hidden", "true")),
		h.Span(g.Text(siteBrandName)),
	)
}

func siteHTMLAttrs() []g.Node {
	return []g.Node{
		g.Attr("data-color-mode", "auto"),
		g.Attr("data-light-theme", "light"),
		g.Attr("data-dark-theme", "dark"),
	}
}

func siteWorkflowCard(icon, number, title, body string) g.Node {
	return h.Li(h.Class("site-workflow-card"),
		h.Div(h.Class("site-workflow-card-top"),
			siteFeatureIcon(icon),
			h.Span(h.Class("site-workflow-number"), g.Attr("aria-hidden", "true"), g.Text(number)),
		),
		h.H3(g.Text(title)),
		h.P(g.Text(body)),
		h.Span(h.Class("site-workflow-arrow"), g.Attr("aria-hidden", "true")),
	)
}

func siteDataStackStage() g.Node {
	groups := make([]g.Node, 0, len(siteStackGroups))
	for _, group := range siteStackGroups {
		groups = append(groups, siteStackGroup(group))
	}
	return h.Li(h.Class("site-stack-stage site-stack-stage-data"),
		h.Div(h.Class("site-stack-groups"), g.Group(groups)),
		siteStackEdges(
			"site-stack-edges-desktop",
			"0 0 100 300",
			"site-stack-arrow-desktop",
			[]string{
				"M0 50 C44 50 42 150 66 150",
				"M0 150 H66",
				"M0 250 C44 250 42 150 66 150",
				"M66 150 H96",
			},
			[]string{
				"M0 50 C44 50 42 150 66 150 H96",
				"M0 150 H96",
				"M0 250 C44 250 42 150 66 150 H96",
			},
		),
		siteStackEdges(
			"site-stack-edges-mobile",
			"0 0 100 100",
			"site-stack-arrow-mobile",
			[]string{"M50 0 V96"},
			[]string{"M50 0 V96"},
		),
	)
}

func siteStackProductNode() g.Node {
	return h.Li(h.Class("site-stack-stage site-stack-node site-stack-product-node"),
		h.H3(h.Class("site-stack-product-brand"),
			g.El("ld-site-brand-mark", g.Attr("large", ""), g.Attr("aria-hidden", "true")),
			h.Span(g.Text(siteBrandName)),
		),
		h.Div(h.Class("site-stack-client-surface"),
			h.H4(g.Text("Interfaces")),
			h.Ul(h.Class("site-stack-client-interfaces"), g.Attr("aria-label", siteBrandName+" interfaces"),
				siteStackClientInterface("Web app", siteStackFeatureIcon("square-mouse-pointer")),
				siteStackClientInterface("CLI", siteStackFeatureIcon("terminal")),
				siteStackClientInterface("REST API", siteStackFeatureIcon("code-xml")),
				siteStackClientInterface("MCP", siteStackMCPMark()),
			),
		),
	)
}

func siteStackClientInterface(label string, icon g.Node) g.Node {
	return h.Li(
		h.Class("site-stack-integration site-stack-client-interface"),
		g.Attr("aria-label", label),
		g.Attr("data-label", label),
		g.Attr("tabindex", "0"),
		icon,
	)
}

func siteStackFeatureIcon(name string) g.Node {
	return g.El("ld-site-feature-icon", g.Attr("name", name), g.Attr("plain", ""), g.Attr("aria-hidden", "true"))
}

func siteStackMCPMark() g.Node {
	mark, err := siteassets.MCPMark()
	if err != nil {
		panic(err)
	}
	return h.Span(
		h.Class("site-stack-mcp-mark"),
		g.Attr("aria-hidden", "true"),
		g.Raw(mark),
	)
}

func siteStackEdges(className, viewBox, markerID string, paths, flowPaths []string) g.Node {
	edges := make([]g.Node, 0, len(paths))
	for index, path := range paths {
		attributes := []g.Node{h.Class("site-stack-edge"), g.Attr("d", path)}
		if index == len(paths)-1 {
			attributes = append(attributes, g.Attr("marker-end", "url(#"+markerID+")"))
		}
		edges = append(edges, g.El("path", attributes...))
	}
	flowMarkers := make([]g.Node, 0, len(flowPaths))
	flowMarkerClasses := []string{"site-stack-flow-marker-1", "site-stack-flow-marker-2", "site-stack-flow-marker-3"}
	for index, path := range flowPaths {
		flowMarkers = append(flowMarkers,
			g.El("path",
				h.Class("site-stack-flow-marker "+flowMarkerClasses[index%len(flowMarkerClasses)]),
				g.Attr("d", path),
				g.Attr("data-flow-path", path),
			),
		)
	}
	return g.El("svg",
		h.Class("site-stack-edges "+className),
		g.Attr("viewBox", viewBox),
		g.Attr("preserveAspectRatio", "none"),
		g.Attr("aria-hidden", "true"),
		g.Attr("focusable", "false"),
		g.El("defs",
			g.El("marker",
				g.Attr("id", markerID),
				g.Attr("viewBox", "0 0 8 8"),
				g.Attr("refX", "7"),
				g.Attr("refY", "4"),
				g.Attr("markerWidth", "7"),
				g.Attr("markerHeight", "7"),
				g.Attr("orient", "auto"),
				g.El("path", h.Class("site-stack-arrowhead"), g.Attr("d", "M0 0 L8 4 L0 8 Z")),
			),
		),
		g.Group(edges),
		g.Group(flowMarkers),
	)
}

func siteStackGroup(group siteStackGroupSpec) g.Node {
	return h.Section(h.Class("site-stack-group"),
		h.H4(g.Text(group.title)),
		siteStackLogoList(group.integrations),
	)
}

func siteStackLogoList(integrations []siteStackIntegration) g.Node {
	items := make([]g.Node, 0, len(integrations))
	for _, integration := range integrations {
		items = append(items, h.Li(
			h.Class("site-stack-integration"),
			g.Attr("aria-label", integration.label),
			g.Attr("data-label", integration.label),
			g.Attr("data-registry-key", integration.registryKey),
			g.Attr("tabindex", "0"),
			siteStackLogo(integration.icon),
		))
	}
	return h.Ul(h.Class("site-stack-logos"), g.Group(items))
}

func siteStackLogo(icon string) g.Node {
	logo, err := siteassets.IntegrationLogo(icon)
	if err != nil {
		panic(err)
	}
	return h.Span(
		h.Class("site-stack-logo"),
		g.Attr("aria-hidden", "true"),
		g.Raw(logo),
	)
}

func siteInterfaceCard(icon, title, body string, points []string, linkLabel, href string) g.Node {
	items := make([]g.Node, 0, len(points))
	for _, point := range points {
		items = append(items, h.Li(g.Text(point)))
	}
	return h.Article(h.Class("site-interface-card"),
		h.Div(h.Class("site-interface-card-header"), siteFeatureIcon(icon), h.H3(g.Text(title))),
		h.P(g.Text(body)),
		h.Ul(h.Class("site-interface-points"), g.Group(items)),
		h.A(h.Class("site-interface-link"), h.Href(href), g.Text(linkLabel)),
	)
}

func siteProofItem(icon, title, body string) g.Node {
	return h.Article(h.Class("site-proof-item"), siteFeatureIcon(icon), h.H3(g.Text(title)), h.P(g.Text(body)))
}

func siteHomepageActions() g.Node {
	return h.Div(h.Class("site-actions"),
		h.A(h.Class("site-button site-button-primary"), h.Href("/docs/getting-started"), g.Text("Get started")),
		h.A(h.Class("site-button"), h.Href("https://github.com/Yacobolo/libredash"),
			h.Span(h.Class("site-github-mark"), g.Attr("aria-hidden", "true")),
			g.Text("View on GitHub"),
		),
	)
}

func siteFeatureIcon(name string) g.Node {
	return g.El("ld-site-feature-icon", g.Attr("name", name), g.Attr("aria-hidden", "true"))
}

func siteFooter() g.Node {
	return h.Footer(h.Class("site-footer"), g.Attr("role", "contentinfo"),
		h.Div(h.Class("site-footer-content"),
			h.Div(h.Class("site-footer-brand-block"),
				siteBrandLink(),
				h.P(g.Text("Open-source analytics as code for dashboards and AI agents.")),
			),
			siteFooterGroup("Explore", []siteFooterLink{
				{label: "Documentation", href: "/docs"},
				{label: "Getting started", href: "/docs/getting-started"},
				{label: "Visual gallery", href: "/charts"},
			}),
			siteFooterGroup("Project", []siteFooterLink{
				{label: "GitHub", href: "https://github.com/Yacobolo/libredash"},
				{label: "Issues", href: "https://github.com/Yacobolo/libredash/issues"},
			}),
		),
		h.Div(h.Class("site-footer-bottom"), h.P(g.Text(siteBrandName+" — open-source analytics as code."))),
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
		g.El("ld-site-docs-drawer-toggle"),
		h.Nav(h.Class("site-docs-breadcrumb"), g.Attr("aria-label", "Breadcrumb"), h.Ol(g.Group(breadcrumb))),
	)
}

func siteDocsIndex() g.Node {
	items := make([]g.Node, 0, len(siteCatalog.Sections))
	for _, section := range siteCatalog.Sections {
		items = append(items, h.Li(
			h.A(h.Href(section.Href), h.H2(g.Text(section.Title)), h.P(g.Text(section.Summary))),
		))
	}
	return h.Article(h.ID("main-content"), h.Class("site-docs-article site-docs-index"),
		h.H1(g.Text("Documentation")),
		h.P(g.Text("Follow a task-oriented path or open the generated reference for an exact contract.")),
		docsSearchForm(""),
		h.Nav(g.Attr("aria-label", "Documentation sections"), h.Ul(h.Class("site-docs-index-list"), g.Group(items))),
	)
}

func siteDocsSidebar(current *siteDocument) g.Node {
	sections := make([]g.Node, 0, len(siteCatalog.Sections))
	for _, section := range siteCatalog.Sections {
		sectionActive := current != nil && current.sectionID == section.ID
		links := make([]g.Node, 0, len(section.Documents)+len(section.Groups))
		for _, document := range section.Documents {
			isCurrent := current != nil && current.slug == document.Slug
			links = append(links, h.Li(siteDocsLink("/docs/"+document.Slug, siteDocsNavigationLabel(section.Title, document.Title, document.NavigationTitle), isCurrent)))
		}
		for _, group := range section.Groups {
			groupActive := sectionActive && current.groupID == group.ID
			groupLinks := make([]g.Node, 0, len(group.Documents))
			for _, document := range group.Documents {
				isCurrent := current != nil && current.slug == document.Slug
				groupLinks = append(groupLinks, h.Li(siteDocsLink("/docs/"+document.Slug, siteDocsNavigationLabel(group.Title, document.Title, document.NavigationTitle), isCurrent)))
			}
			links = append(links, h.Li(siteDocsNavGroup(section.ID+"-"+group.ID, group.Title, groupActive, groupLinks)))
		}
		sections = append(sections, siteDocsNavGroup(section.ID, section.Title, sectionActive, links))
	}
	return h.Aside(h.Class("site-docs-sidebar"), h.ID("site-docs-sidebar"),
		h.Div(h.Class("site-docs-drawer-actions"),
			g.El("ld-site-docs-drawer-toggle", g.Attr("placement", "drawer")),
		),
		h.Nav(g.Attr("aria-label", "Documentation"), g.Group(sections)),
	)
}

func siteDocsNavigationLabel(parent, document, navigationTitle string) string {
	if strings.TrimSpace(navigationTitle) != "" {
		return navigationTitle
	}
	if strings.EqualFold(strings.TrimSpace(parent), strings.TrimSpace(document)) {
		return "Overview"
	}
	return document
}

func siteDocsSearch(query string) g.Node {
	results := searchSiteDocuments(query)
	items := make([]g.Node, 0, len(results))
	for index, document := range results {
		if index == 50 {
			break
		}
		items = append(items, h.Li(h.A(h.Href("/docs/"+document.slug), h.H2(g.Text(document.title)), h.P(g.Text(document.summary)))))
	}
	content := []g.Node{h.H1(g.Text("Search documentation")), docsSearchForm(query)}
	if query != "" {
		content = append(content, h.P(g.Textf("%d results for %q", len(results), query)), h.Ul(h.Class("site-docs-index-list site-docs-search-results"), g.Group(items)))
	}
	return h.Article(h.ID("main-content"), h.Class("site-docs-article site-docs-index"), g.Group(content))
}

func docsSearchForm(query string) g.Node {
	return h.Form(h.Class("site-docs-search"), h.Action("/docs/search"), h.Method("get"),
		h.Label(h.For("docs-search-query"), g.Text("Search documentation")),
		h.Div(h.Class("site-docs-search-controls"),
			h.Input(h.ID("docs-search-query"), h.Name("q"), h.Type("search"), h.Value(query), h.Placeholder("Search concepts, guides, commands, and APIs")),
			h.Button(h.Type("submit"), g.Text("Search")),
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
		g.El("summary", g.Attr("title", label), h.Span(h.Class("site-docs-nav-label"), g.Text(label))),
		h.Ul(h.Class("site-docs-nav-tree"), g.Group(links)),
	)
}

func siteDocsLink(href, label string, current bool) g.Node {
	class := "site-docs-link"
	if current {
		class += " site-docs-link-current"
	}
	attrs := []g.Node{h.Class(class), h.Href(href), g.Attr("title", label)}
	if current {
		attrs = append(attrs, g.Attr("aria-current", "page"))
	}
	return h.A(g.Group(attrs), g.Text(label))
}
