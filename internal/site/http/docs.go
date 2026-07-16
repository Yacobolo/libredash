package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	content "github.com/Yacobolo/libredash/docs"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

var markdownRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))

type siteDocument struct {
	slug               string
	title              string
	breadcrumb         string
	breadcrumbRoot     string
	breadcrumbRootHref string
	chartID            string
	summary            string
	markdown           string
}

var siteDocuments = []siteDocument{
	{
		slug:       "getting-started",
		title:      "Get started with LibreDash",
		breadcrumb: "Getting started",
		summary:    "Set up a local LibreDash workspace and make your first dashboard changes.",
		markdown:   content.GettingStarted,
	},
	{
		slug:       "configuration",
		title:      "Configuration reference",
		breadcrumb: "Configuration",
		summary:    "Review the process-wide environment settings that configure LibreDash.",
		markdown:   content.Configuration,
	},
	{
		slug:       "enterprise-auth",
		title:      "Enterprise auth",
		breadcrumb: "Enterprise auth",
		summary:    "Configure OIDC, SCIM, and scoped credentials for enterprise deployments.",
		markdown:   content.EnterpriseAuth,
	},
	{
		slug:       "storage-architecture",
		title:      "Storage architecture",
		breadcrumb: "Storage architecture",
		summary:    "Understand how LibreDash uses DuckLake and DuckDB for durable analytical storage.",
		markdown:   content.StorageArchitecture,
	},
}

var chartDocuments = loadChartDocuments()

var chartOverviewDocument = chartDocuments.overview

var visualDocuments = chartDocuments.documents

type chartDocumentation struct {
	section   string
	overview  siteDocument
	documents []siteDocument
}

type chartDocumentationCatalog struct {
	Section   string                  `json:"section"`
	Overview  chartDocumentMetadata   `json:"overview"`
	Documents []chartDocumentMetadata `json:"documents"`
}

type chartDocumentMetadata struct {
	Source     string `json:"source"`
	Title      string `json:"title"`
	Breadcrumb string `json:"breadcrumb"`
}

func loadChartDocuments() chartDocumentation {
	catalogContents, err := content.Visuals.ReadFile("visuals/catalog.json")
	if err != nil {
		panic(fmt.Sprintf("read chart documentation catalog: %v", err))
	}
	var catalog chartDocumentationCatalog
	if err := json.Unmarshal(catalogContents, &catalog); err != nil {
		panic(fmt.Sprintf("decode chart documentation catalog: %v", err))
	}
	if catalog.Section == "" || catalog.Overview.Source == "" || catalog.Overview.Title == "" {
		panic("chart documentation catalog is missing its section or overview")
	}

	overviewBreadcrumb := catalog.Overview.Breadcrumb
	if overviewBreadcrumb == "" {
		overviewBreadcrumb = catalog.Section
	}
	collection := chartDocumentation{
		section: catalog.Section,
		overview: siteDocument{
			slug:               "charts/overview",
			title:              catalog.Overview.Title,
			breadcrumb:         overviewBreadcrumb,
			breadcrumbRoot:     catalog.Section,
			breadcrumbRootHref: "/docs/charts/overview",
			summary:            "Configure every supported LibreDash chart visual from dashboard YAML.",
			markdown:           visualMarkdown(catalog.Overview.Source),
		},
		documents: make([]siteDocument, 0, len(catalog.Documents)),
	}
	for _, document := range catalog.Documents {
		if document.Source == "" || document.Title == "" {
			panic("chart documentation catalog contains an incomplete chart document")
		}
		breadcrumb := document.Breadcrumb
		if breadcrumb == "" {
			breadcrumb = document.Title
		}
		collection.documents = append(collection.documents, siteDocument{
			slug:               "charts/" + document.Source,
			title:              document.Title,
			breadcrumb:         breadcrumb,
			breadcrumbRoot:     catalog.Section,
			breadcrumbRootHref: "/docs/charts/overview",
			chartID:            document.Source,
			summary:            "Configuration and query shape for the " + document.Title + " visual.",
			markdown:           visualMarkdown(document.Source),
		})
	}
	return collection
}

var apiReferenceDocuments = loadAPIReferenceDocuments()

var configurationReferenceDocuments = loadConfigurationReferenceDocuments()

var cliReferenceDocuments = loadCLIReferenceDocuments()

var cliGuideDocuments = loadCLIGuideDocuments()

type apiReferenceCatalog struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Documents   []struct {
		Slug    string `json:"slug"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"documents"`
}

func loadAPIReferenceDocuments() []siteDocument {
	catalogContents, err := content.API.ReadFile("api/catalog.json")
	if err != nil {
		panic(fmt.Sprintf("read API documentation catalog: %v", err))
	}
	var catalog apiReferenceCatalog
	if err := json.Unmarshal(catalogContents, &catalog); err != nil {
		panic(fmt.Sprintf("decode API documentation catalog: %v", err))
	}

	index := mustReadAPIDocument("index")
	documents := make([]siteDocument, 0, len(catalog.Documents)+1)
	documents = append(documents, siteDocument{
		slug:               "api",
		title:              "API reference",
		breadcrumb:         "API reference",
		breadcrumbRoot:     "API reference",
		breadcrumbRootHref: "/docs/api",
		summary:            catalog.Description,
		markdown:           index,
	})
	for _, document := range catalog.Documents {
		documents = append(documents, siteDocument{
			slug:               "api/" + document.Slug,
			title:              document.Title,
			breadcrumb:         document.Title,
			breadcrumbRoot:     "API reference",
			breadcrumbRootHref: "/docs/api",
			summary:            document.Summary,
			markdown:           mustReadAPIDocument(document.Slug),
		})
	}
	return documents
}

func mustReadAPIDocument(name string) string {
	markdown, err := content.API.ReadFile("api/" + name + ".md")
	if err != nil {
		panic(fmt.Sprintf("read API documentation %q: %v", name, err))
	}
	return string(markdown)
}

type configurationReferenceCatalog struct {
	Title     string `json:"title"`
	Documents []struct {
		Slug    string `json:"slug"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
	} `json:"documents"`
}

func loadConfigurationReferenceDocuments() []siteDocument {
	catalogContents, err := content.Config.ReadFile("reference/config/catalog.json")
	if err != nil {
		panic(fmt.Sprintf("read configuration reference catalog: %v", err))
	}
	var catalog configurationReferenceCatalog
	if err := json.Unmarshal(catalogContents, &catalog); err != nil {
		panic(fmt.Sprintf("decode configuration reference catalog: %v", err))
	}
	documents := make([]siteDocument, 0, len(catalog.Documents))
	for _, document := range catalog.Documents {
		markdown, err := content.Config.ReadFile("reference/config/" + document.Slug + ".md")
		if err != nil {
			panic(fmt.Sprintf("read configuration reference %q: %v", document.Slug, err))
		}
		documents = append(documents, siteDocument{
			slug:               "config/" + document.Slug,
			title:              document.Title,
			breadcrumb:         document.Title,
			breadcrumbRoot:     "Configuration",
			breadcrumbRootHref: "/docs/configuration",
			summary:            document.Summary,
			markdown:           string(markdown),
		})
	}
	return documents
}

func siteConfigurationSchema(name string) ([]byte, bool) {
	if strings.Contains(name, "/") || !strings.HasSuffix(name, ".schema.json") {
		return nil, false
	}
	schema, err := content.Config.ReadFile("reference/config/schemas/" + name)
	return schema, err == nil
}

func loadCLIReferenceDocuments() []siteDocument {
	catalogContents, err := content.CLI.ReadFile("reference/cli/catalog.json")
	if err != nil {
		panic(fmt.Sprintf("read CLI reference catalog: %v", err))
	}
	var catalog configurationReferenceCatalog
	if err := json.Unmarshal(catalogContents, &catalog); err != nil {
		panic(fmt.Sprintf("decode CLI reference catalog: %v", err))
	}
	documents := make([]siteDocument, 0, len(catalog.Documents))
	for _, document := range catalog.Documents {
		markdown, err := content.CLI.ReadFile("reference/cli/" + document.Slug + ".md")
		if err != nil {
			panic(fmt.Sprintf("read CLI reference %q: %v", document.Slug, err))
		}
		documents = append(documents, siteDocument{slug: "cli/" + document.Slug, title: document.Title, breadcrumb: document.Title, breadcrumbRoot: "CLI", breadcrumbRootHref: "/docs/cli", summary: document.Summary, markdown: string(markdown)})
	}
	return documents
}

func loadCLIGuideDocuments() []siteDocument {
	guides := []struct{ slug, title, summary, source string }{
		{"cli", "CLI overview", "Install, validate, plan, and publish with the LibreDash CLI.", "overview"},
		{"cli/authentication", "Install and authenticate", "Configure a CLI target and use tokens safely.", "authentication"},
		{"cli/targets", "Targets and environments", "Keep local, staging, and production operations explicit.", "targets"},
		{"cli/validate-publish", "Validate, plan, and publish", "Use the standard dashboard-as-code delivery workflow.", "validate-publish"},
		{"cli/automation", "Automation and CI", "Run LibreDash safely in continuous integration.", "automation"},
		{"cli/troubleshooting", "Troubleshooting", "Diagnose local validation and remote operation failures.", "troubleshooting"},
	}
	documents := make([]siteDocument, 0, len(guides))
	for _, guide := range guides {
		markdown, err := content.CLIGuides.ReadFile("guides/cli/" + guide.source + ".md")
		if err != nil {
			panic(fmt.Sprintf("read CLI guide %q: %v", guide.source, err))
		}
		documents = append(documents, siteDocument{slug: guide.slug, title: guide.title, breadcrumb: guide.title, breadcrumbRoot: "CLI", breadcrumbRootHref: "/docs/cli", summary: guide.summary, markdown: string(markdown)})
	}
	return documents
}

func siteOpenAPISpecification() []byte {
	specification, err := content.API.ReadFile("api/openapi.yaml")
	if err != nil {
		panic(fmt.Sprintf("read generated OpenAPI specification: %v", err))
	}
	return specification
}

func visualMarkdown(name string) string {
	markdown, err := content.Visuals.ReadFile("visuals/" + name + ".md")
	if err != nil {
		panic(fmt.Sprintf("read visual documentation %q: %v", name, err))
	}
	return string(markdown)
}

func allSiteDocuments() []siteDocument {
	documents := make([]siteDocument, 0, len(siteDocuments)+1+len(visualDocuments)+len(apiReferenceDocuments)+len(configurationReferenceDocuments)+len(cliReferenceDocuments)+len(cliGuideDocuments))
	documents = append(documents, siteDocuments...)
	documents = append(documents, chartOverviewDocument)
	documents = append(documents, visualDocuments...)
	documents = append(documents, apiReferenceDocuments...)
	documents = append(documents, configurationReferenceDocuments...)
	documents = append(documents, cliReferenceDocuments...)
	documents = append(documents, cliGuideDocuments...)
	return documents
}

func siteDocumentBySlug(slug string) (siteDocument, bool) {
	for _, document := range allSiteDocuments() {
		if document.slug == slug {
			return document, true
		}
	}
	return siteDocument{}, false
}

const docsChartShortcode = "{{< chart >}}"

const docsChartPlaceholder = "LIBREDASH_DOCS_CHART_PLACEHOLDER"

func siteDocsArticle(document siteDocument) g.Node {
	source := document.markdown
	if strings.Contains(source, docsChartShortcode) {
		if document.chartID == "" {
			panic(fmt.Sprintf("chart shortcode requires a chart document: %s", document.slug))
		}
		source = strings.ReplaceAll(source, docsChartShortcode, docsChartPlaceholder)
	}

	var rendered bytes.Buffer
	if err := markdownRenderer.Convert([]byte(source), &rendered); err != nil {
		panic(fmt.Sprintf("render documentation Markdown: %v", err))
	}
	renderedHTML := rendered.String()
	if strings.Contains(source, docsChartPlaceholder) {
		placeholder := "<p>" + docsChartPlaceholder + "</p>\n"
		component := fmt.Sprintf("<ld-site-doc-chart chart-id=\"%s\"></ld-site-doc-chart>\n", document.chartID)
		if !strings.Contains(renderedHTML, placeholder) {
			panic(fmt.Sprintf("render chart shortcode for documentation: %s", document.slug))
		}
		renderedHTML = strings.ReplaceAll(renderedHTML, placeholder, component)
	}

	return h.Article(
		h.ID("main-content"),
		h.Class("site-docs-article"),
		h.Div(h.Class("site-docs-article-actions"), g.El("ld-site-markdown-copy", g.Attr("markdown", document.markdown))),
		g.Raw(renderedHTML),
	)
}
