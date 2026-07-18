package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	content "github.com/Yacobolo/libredash/docs"
	docsearch "github.com/Yacobolo/libredash/internal/site/search/sqlite"
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
	sectionID          string
	groupID            string
	source             string
	navigationTitle    string
	generated          bool
}

type siteCatalogDocument struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	NavigationTitle string `json:"navigationTitle"`
	Summary         string `json:"summary"`
	Source          string `json:"source"`
	Breadcrumb      string `json:"breadcrumb"`
	ChartID         string `json:"chartID"`
	Generated       bool   `json:"generated"`
}

type siteCatalogGroup struct {
	ID        string                `json:"id"`
	Title     string                `json:"title"`
	Summary   string                `json:"summary"`
	Href      string                `json:"href"`
	Documents []siteCatalogDocument `json:"documents"`
}

type siteCatalogSection struct {
	ID        string                `json:"id"`
	Title     string                `json:"title"`
	Summary   string                `json:"summary"`
	Href      string                `json:"href"`
	Documents []siteCatalogDocument `json:"documents"`
	Groups    []siteCatalogGroup    `json:"groups"`
}

type siteDocumentationCatalog struct {
	Sections []siteCatalogSection `json:"sections"`
}

type loadedDocumentation struct {
	catalog   siteDocumentationCatalog
	documents []siteDocument
	bySlug    map[string]siteDocument
}

var documentation = loadDocumentation()
var siteCatalog = documentation.catalog
var siteDocuments = documentation.documents
var siteDocumentsBySlug = documentation.bySlug
var documentationSearchIndex = loadDocumentationSearchIndex()
var visualDocuments = documentsInCatalogGroup("reference", "visuals", true)

func loadDocumentationSearchIndex() *docsearch.Index {
	index, err := docsearch.Open(content.Files, docsearch.Filename)
	if err != nil {
		panic(fmt.Sprintf("open documentation search index: %v", err))
	}
	slugs, err := index.Slugs(context.Background())
	if err != nil {
		panic(fmt.Sprintf("read documentation search index: %v", err))
	}
	if len(slugs) != len(siteDocuments) {
		panic(fmt.Sprintf("documentation search index contains %d documents, catalog contains %d", len(slugs), len(siteDocuments)))
	}
	for _, slug := range slugs {
		if _, exists := siteDocumentsBySlug[slug]; !exists {
			panic(fmt.Sprintf("documentation search index contains unknown slug %q", slug))
		}
	}
	return index
}

func loadDocumentation() loadedDocumentation {
	catalogContents, err := content.Files.ReadFile("catalog.json")
	if err != nil {
		panic(fmt.Sprintf("read documentation catalog: %v", err))
	}
	var catalog siteDocumentationCatalog
	if err := json.Unmarshal(catalogContents, &catalog); err != nil {
		panic(fmt.Sprintf("decode documentation catalog: %v", err))
	}
	loaded := loadedDocumentation{catalog: catalog, bySlug: make(map[string]siteDocument)}
	for _, section := range catalog.Sections {
		for _, document := range section.Documents {
			loaded.add(section, siteCatalogGroup{}, document)
		}
		for _, group := range section.Groups {
			for _, document := range group.Documents {
				loaded.add(section, group, document)
			}
		}
	}
	return loaded
}

func (loaded *loadedDocumentation) add(section siteCatalogSection, group siteCatalogGroup, document siteCatalogDocument) {
	markdown, err := content.Files.ReadFile(document.Source)
	if err != nil {
		panic(fmt.Sprintf("read documentation source %q: %v", document.Source, err))
	}
	if _, exists := loaded.bySlug[document.Slug]; exists {
		panic(fmt.Sprintf("duplicate documentation slug %q", document.Slug))
	}
	rootTitle, rootHref := section.Title, section.Href
	if group.ID != "" {
		rootTitle, rootHref = group.Title, group.Href
	}
	if rootHref == "/docs/"+document.Slug {
		rootTitle, rootHref = "Documentation", "/docs"
	}
	entry := siteDocument{
		slug:               document.Slug,
		title:              document.Title,
		breadcrumb:         firstNonEmpty(document.Breadcrumb, document.Title),
		breadcrumbRoot:     rootTitle,
		breadcrumbRootHref: rootHref,
		chartID:            document.ChartID,
		summary:            document.Summary,
		markdown:           string(markdown),
		sectionID:          section.ID,
		groupID:            group.ID,
		source:             document.Source,
		navigationTitle:    document.NavigationTitle,
		generated:          document.Generated,
	}
	loaded.documents = append(loaded.documents, entry)
	loaded.bySlug[entry.slug] = entry
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func documentsInCatalogGroup(sectionID, groupID string, skipFirst bool) []siteDocument {
	for _, section := range siteCatalog.Sections {
		if section.ID != sectionID {
			continue
		}
		for _, group := range section.Groups {
			if group.ID != groupID {
				continue
			}
			documents := make([]siteDocument, 0, len(group.Documents))
			for index, document := range group.Documents {
				if skipFirst && index == 0 {
					continue
				}
				documents = append(documents, siteDocumentsBySlug[document.Slug])
			}
			return documents
		}
	}
	return nil
}

func allSiteDocuments() []siteDocument {
	return siteDocuments
}

func siteDocumentBySlug(slug string) (siteDocument, bool) {
	document, ok := siteDocumentsBySlug[strings.Trim(slug, "/")]
	return document, ok
}

func searchSiteDocuments(query string) []siteDocument {
	matches, err := documentationSearchIndex.Search(context.Background(), query, len(siteDocuments))
	if err != nil {
		panic(fmt.Sprintf("search documentation: %v", err))
	}
	results := make([]siteDocument, 0, len(matches))
	for _, match := range matches {
		document, exists := siteDocumentsBySlug[match.Slug]
		if !exists {
			panic(fmt.Sprintf("documentation search returned unknown slug %q", match.Slug))
		}
		results = append(results, document)
	}
	return results
}

func siteConfigurationSchema(name string) ([]byte, bool) {
	if strings.Contains(name, "/") || !strings.HasSuffix(name, ".schema.json") {
		return nil, false
	}
	schema, err := content.Files.ReadFile("reference/config/schemas/" + name)
	return schema, err == nil
}

func siteOpenAPISpecification() []byte {
	specification, err := content.Files.ReadFile("api/openapi.yaml")
	if err != nil {
		panic(fmt.Sprintf("read generated OpenAPI specification: %v", err))
	}
	return specification
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
		siteDocsArticleFooter(document),
	)
}

func siteDocsArticleFooter(document siteDocument) g.Node {
	sourceLabel, sourceHref := documentationSourceLink(document)
	return h.Footer(h.Class("site-docs-article-footer"),
		h.Section(h.Class("site-docs-page-meta"), g.Attr("aria-labelledby", "site-docs-about-this-page"),
			h.H2(h.ID("site-docs-about-this-page"), g.Text("About this page")),
			h.Ul(
				h.Li(h.A(h.Href(documentationIssueLink(document)), g.Attr("rel", "external"), g.Text("Report content issue"))),
				h.Li(h.A(h.Href(documentationMarkdownLink(document)), g.Attr("rel", "external"), g.Text("See this page as Markdown"))),
				h.Li(h.A(h.Href(sourceHref), g.Attr("rel", "external"), g.Text(sourceLabel))),
			),
		),
	)
}

func documentationIssueLink(document siteDocument) string {
	query := url.Values{}
	query.Set("title", "Docs: "+document.title)
	query.Set("labels", "documentation")
	query.Set("body", "Page: /docs/"+document.slug+"\n\nDescribe the content issue or suggested improvement.")
	return "https://github.com/Yacobolo/libredash/issues/new?" + query.Encode()
}

func documentationMarkdownLink(document siteDocument) string {
	return "https://raw.githubusercontent.com/Yacobolo/libredash/main/docs/" + document.source
}

func documentationSourceLink(document siteDocument) (string, string) {
	const repository = "https://github.com/Yacobolo/libredash/"
	if !strings.HasPrefix(document.markdown, "<!-- Code generated") {
		return "Edit this page on GitHub", repository + "edit/main/docs/" + document.source
	}
	switch {
	case document.source == "configuration.md":
		return "View source contract on GitHub", repository + "blob/main/internal/configspec/spec.go"
	case strings.HasPrefix(document.source, "reference/config/"):
		return "View source contract on GitHub", repository + "blob/main/internal/configschema/contracts/contracts.cue"
	case strings.HasPrefix(document.source, "reference/cli/"):
		return "View source contract on GitHub", repository + "tree/main/internal/cli"
	case strings.HasPrefix(document.source, "api/"):
		return "View source contract on GitHub", repository + "tree/main/api/typespec"
	default:
		return "View source on GitHub", repository + "blob/main/docs/" + document.source
	}
}
