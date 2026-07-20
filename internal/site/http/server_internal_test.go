package http

import (
	"bufio"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/analytics/connectors"
)

func TestHomepageFeaturedIntegrationsExistInTheConnectorRegistry(t *testing.T) {
	for _, group := range siteStackGroups {
		for _, integration := range group.integrations {
			if integration.format {
				if _, ok := connectors.LookupFormat(integration.registryKey); !ok {
					t.Errorf("featured format %q (%s) is not registered", integration.label, integration.registryKey)
				}
				continue
			}
			if _, ok := connectors.LookupConnection(integration.registryKey); !ok {
				t.Errorf("featured connection %q (%s) is not registered", integration.label, integration.registryKey)
			}
		}
	}
}

func TestHomepageFeaturesSupportedDatabasesAndFormats(t *testing.T) {
	want := map[string][]string{
		"Databases": {"postgres", "mysql", "sqlite"},
		"Formats":   {"csv", "json", "parquet", "excel", "vortex", "delta", "iceberg", "lance", "ducklake"},
	}
	for _, group := range siteStackGroups {
		expected, ok := want[group.title]
		if !ok {
			continue
		}
		got := make([]string, 0, len(group.integrations))
		for _, integration := range group.integrations {
			got = append(got, integration.registryKey)
		}
		if strings.Join(got, ",") != strings.Join(expected, ",") {
			t.Errorf("%s integrations = %v, want %v", group.title, got, expected)
		}
	}
}

func TestSiteUnknownRouteReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/not-a-real-route")
	if err != nil {
		t.Fatalf("get unknown route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown route status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
	body := readBody(t, response)
	if !strings.Contains(body, "Page not found") || !strings.Contains(body, `href="/docs"`) {
		t.Fatalf("unknown route did not render the site 404 page:\n%s", body)
	}
}

func TestSiteArticlePublishesSearchAndSocialMetadata(t *testing.T) {
	baseURL, err := url.Parse("https://docs.leapview.dev")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	server := httptest.NewServer(NewHandlerWithOptions(Options{BaseURL: baseURL}))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/introduction")
	if err != nil {
		t.Fatalf("get article: %v", err)
	}
	defer response.Body.Close()
	body := readBody(t, response)
	document, ok := siteDocumentBySlug("introduction")
	if !ok {
		t.Fatal("introduction document not found")
	}
	for _, want := range []string{
		`<meta name="description" content="` + document.summary + `">`,
		`<link rel="canonical" href="https://docs.leapview.dev/docs/introduction">`,
		`<meta property="og:title" content="` + document.title + `">`,
		`<meta property="og:type" content="article">`,
		`<meta property="og:url" content="https://docs.leapview.dev/docs/introduction">`,
		`<meta name="twitter:card" content="summary">`,
		`<link rel="icon" href="/static/favicon.svg" type="image/svg+xml">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("article metadata missing %q:\n%s", want, body)
		}
	}
}

func TestSitePublishesSitemapAndRobots(t *testing.T) {
	baseURL, err := url.Parse("https://docs.leapview.dev")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	server := httptest.NewServer(NewHandlerWithOptions(Options{BaseURL: baseURL}))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/sitemap.xml")
	if err != nil {
		t.Fatalf("get sitemap: %v", err)
	}
	sitemap := readBody(t, response)
	response.Body.Close()
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/xml") {
		t.Fatalf("sitemap content type = %q", got)
	}
	for _, want := range []string{
		"https://docs.leapview.dev/",
		"https://docs.leapview.dev/visuals",
		"https://docs.leapview.dev/docs",
		"https://docs.leapview.dev/docs/introduction",
	} {
		if !strings.Contains(sitemap, "<loc>"+want+"</loc>") {
			t.Errorf("sitemap missing %q:\n%s", want, sitemap)
		}
	}
	if strings.Contains(sitemap, "/docs/search") {
		t.Fatalf("sitemap contains search page:\n%s", sitemap)
	}

	response, err = server.Client().Get(server.URL + "/robots.txt")
	if err != nil {
		t.Fatalf("get robots: %v", err)
	}
	robots := readBody(t, response)
	response.Body.Close()
	for _, want := range []string{
		"User-agent: *",
		"Disallow: /docs/search",
		"Sitemap: https://docs.leapview.dev/sitemap.xml",
	} {
		if !strings.Contains(robots, want) {
			t.Errorf("robots.txt missing %q:\n%s", want, robots)
		}
	}
}

func TestSiteProductionHeadersAndHealthEndpoints(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	for _, path := range []string{"/healthz", "/readyz"} {
		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		body := readBody(t, response)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || strings.TrimSpace(body) != "ok" {
			t.Errorf("%s = %d %q, want 200 ok", path, response.StatusCode, body)
		}
	}

	response, err := server.Client().Get(server.URL + "/docs/introduction")
	if err != nil {
		t.Fatalf("get article: %v", err)
	}
	response.Body.Close()
	for header, want := range map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Cache-Control":           "no-cache",
	} {
		if got := response.Header.Get(header); !strings.Contains(got, want) {
			t.Errorf("%s = %q, want containing %q", header, got, want)
		}
	}
	if got := response.Header.Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self' 'unsafe-eval'") {
		t.Errorf("Content-Security-Policy = %q, want Datastar expression evaluation allowance", got)
	}

	response, err = server.Client().Get(server.URL + "/static/site-page.js")
	if err != nil {
		t.Fatalf("get site asset: %v", err)
	}
	response.Body.Close()
	if got := response.Header.Get("Cache-Control"); got != "public, max-age=0, must-revalidate" {
		t.Fatalf("site asset cache control = %q", got)
	}
}

func TestSiteAssetsDoNotDependOnWorkingDirectory(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Chdir(filepath.Dir(workingDirectory))

	server := httptest.NewServer(NewHandler())
	defer server.Close()
	for _, path := range []string{"/static/favicon.svg", "/static/logo-lab.html", "/static/site.css", "/static/site-page.js", "/shared/app.css", "/shared/theme.js", "/shared/files/inter-latin-wght-normal.woff2", "/static/vendor/datastar-1.0.2.js", "/static/vendor/github-mark.svg"} {
		response, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d, want %d", path, response.StatusCode, http.StatusOK)
		}
	}
}

func TestLogoLabIsSelfContainedAndComparesFourDirections(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/static/logo-lab.html")
	if err != nil {
		t.Fatalf("get logo lab: %v", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read logo lab: %v", err)
	}
	page := string(contents)
	for _, want := range []string{
		`<title>LeapView Logo Lab</title>`,
		`data-logo="aperture"`,
		`data-logo="aperture-ring"`,
		`data-logo="aperture-ring" data-selected="true"`,
		`Selected direction`,
		`data-logo="mountain"`,
		`data-logo="telescope"`,
		`aria-label="Aperture logo study"`,
		`aria-label="Aperture ring logo study"`,
		`aria-label="Mountain logo study"`,
		`aria-label="Telescope logo study"`,
		`m14.31 8 5.74 9.94`,
		`m8 3 4 8 5-5 5 15H2L8 3z`,
		`m10.065 12.493-6.18 1.318`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("logo lab does not contain %q", want)
		}
	}
	for _, unwanted := range []string{`<script`, `src=`, `href="/`} {
		if strings.Contains(page, unwanted) {
			t.Errorf("logo lab contains external dependency %q", unwanted)
		}
	}
}

func TestSiteAssetsSupportGzipCompression(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	request, err := http.NewRequest(http.MethodGet, server.URL+"/static/site-page.js", nil)
	if err != nil {
		t.Fatalf("create asset request: %v", err)
	}
	request.Header.Set("Accept-Encoding", "gzip")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("get compressed asset: %v", err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("asset content encoding = %q, want gzip", got)
	}
	compressed, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatalf("open compressed asset: %v", err)
	}
	defer compressed.Close()
	body, err := io.ReadAll(compressed)
	if err != nil {
		t.Fatalf("read compressed asset: %v", err)
	}
	if !strings.Contains(string(body), "customElements") {
		t.Fatalf("compressed asset does not contain site bundle")
	}
}

func TestSiteHomeRendersPageStreamDocument(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatalf("get home page: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("home status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	body := readBody(t, response)
	for _, want := range []string{
		"<title>LeapView — agent-native BI and analytics as code</title>",
		`data-color-mode="auto"`,
		`/updates`,
		`data-init="@get(&#39;/updates&#39;, {openWhenHidden: true})"`,
		`/shared/app.css`,
		`<link rel="preload" href="/shared/files/inter-latin-wght-normal.woff2" as="font" type="font/woff2" crossorigin="anonymous">`,
		`/static/site.css`,
		`/static/site-page.js`,
		`<meta name="view-transition" content="same-origin">`,
		`<lv-site-flow-background class="site-hero-background" aria-hidden="true"></lv-site-flow-background>`,
		`<section id="main-content" class="site-hero">`,
		`<div class="site-hero-layout">`,
		`<img class="site-product-screenshot site-product-screenshot-light" src="/static/product-dashboard-light.png"`,
		`<img class="site-product-screenshot site-product-screenshot-dark" src="/static/product-dashboard-dark.png"`,
		`<aside class="site-agent-preview" aria-label="Verified AI agent answer">`,
		`Why did revenue fall in October?`,
		`Verified against the sales semantic model`,
		`<div class="site-proof-strip">`,
		`<svg class="site-stack-edges site-stack-edges-desktop"`,
		`<li class="site-stack-stage site-stack-node site-stack-product-node">`,
		`<lv-brand-mark large="" aria-hidden="true"></lv-brand-mark>`,
		`<lv-site-feature-icon name="dashboard" aria-hidden="true"></lv-site-feature-icon>`,
		`<lv-site-feature-icon name="git-branch" aria-hidden="true"></lv-site-feature-icon>`,
		`<section id="product" class="site-workflow">`,
		`<article class="site-workflow-artifact">`,
		`apiVersion: leapview.dev/v1`,
		`<ol class="site-stack-flow" aria-label="How LeapView connects to your data stack">`,
		`<section class="site-interfaces-section">`,
		`One model. Two ways to explore.`,
		`<article class="site-interface-card">`,
		`<lv-site-feature-icon name="agent" aria-hidden="true"></lv-site-feature-icon>`,
		`<a class="site-interface-link" href="/docs/guides/integrate/agent">Explore agent integrations</a>`,
		`<span class="site-stack-integration-label">PostgreSQL</span>`,
		`<section class="site-trust-section">`,
		`Governed from question to answer.`,
		`<section class="site-cta">`,
		`<footer class="site-footer" role="contentinfo">`,
		`<header class="site-header">`,
		`<lv-site-theme-toggle></lv-site-theme-toggle>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("home page missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "site-capabilities-section") {
		t.Error("home page still renders the redundant capabilities section")
	}
}

func TestSiteVisualsRendersPageStreamShowcase(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/visuals")
	if err != nil {
		t.Fatalf("get charts page: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("charts status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	body := readBody(t, response)
	for _, want := range []string{
		"<title>LeapView visual showcase</title>",
		`data-init="@get(&#39;/updates?view=visuals&#39;, {openWhenHidden: true})"`,
		"<lv-site-visual-showcase>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("charts page missing %q:\n%s", want, body)
		}
	}
}

func TestSiteGettingStartedRendersGuide(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/getting-started")
	if err != nil {
		t.Fatalf("get getting started page: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("getting started status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	body := readBody(t, response)
	for _, want := range []string{
		"<title>Get started with LeapView</title>",
		`<lv-site-docs-drawer-toggle></lv-site-docs-drawer-toggle>`,
		`<nav class="site-docs-breadcrumb" aria-label="Breadcrumb"><ol><li><a href="/docs/introduction">Start here</a></li><li><span aria-current="page">Getting started</span></li></ol></nav>`,
		`<button class="site-docs-drawer-backdrop" type="button" aria-label="Close documentation menu" aria-hidden="true" tabindex="-1" data-site-docs-drawer-close="true"></button>`,
		`<lv-site-markdown-copy`,
		`<article id="main-content" class="site-docs-article">`,
		`<aside class="site-docs-sidebar" id="site-docs-sidebar">`,
		`<a class="site-docs-link site-docs-link-current" href="/docs/getting-started" title="Get started with LeapView" aria-current="page">Get started with LeapView</a>`,
		`<details class="site-docs-nav-group" data-site-docs-group="reference-configuration">`,
		`<a class="site-docs-link" href="/docs/configuration" title="Environment variable reference">Environment variable reference</a>`,
		`<a class="site-docs-link" href="/docs/enterprise-auth" title="Overview">Overview</a>`,
		`<a class="site-docs-link" href="/docs/storage-architecture" title="Storage architecture">Storage architecture</a>`,
		`<details class="site-docs-nav-group site-docs-nav-group-active" data-site-docs-group="start" open="true">`,
		`<summary title="Start here"><span class="site-docs-nav-label">Start here</span></summary>`,
		`<details class="site-docs-nav-group" data-site-docs-group="reference-visuals">`,
		`<summary title="Visuals"><span class="site-docs-nav-label">Visuals</span></summary>`,
		`<ul class="site-docs-nav-tree">`,
		`<a class="site-docs-link" href="/docs/visuals/overview" title="Overview">Overview</a>`,
		"<h1>Get started with LeapView</h1>",
		"<h2>Bootstrap the workspace</h2>",
		"task bootstrap",
		"task dev",
		"leapview.yaml",
		"workspaces/",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("getting started page missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "LeapView Docs") {
		t.Errorf("getting started page retains the redundant docs header:\n%s", body)
	}
	if strings.Contains(body, "Guides and reference") {
		t.Errorf("getting started page retains the redundant sidebar heading:\n%s", body)
	}
	if strings.Contains(body, "catalog.yaml") {
		t.Errorf("getting started page retains the obsolete catalog layout:\n%s", body)
	}
	if strings.Contains(body, `<footer class="site-footer" role="contentinfo">`) {
		t.Errorf("getting started page retains the marketing footer:\n%s", body)
	}
}

func TestSiteCLIGuideUsesDeployCommand(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/cli/validate-deploy")
	if err != nil {
		t.Fatalf("get deploy guide: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("deploy guide status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body := readBody(t, response)
	for _, want := range []string{"Validate, plan, and deploy", "leapview deploy --project dashboards/leapview.yaml"} {
		if !strings.Contains(body, want) {
			t.Errorf("deploy guide missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "leapview publish") {
		t.Errorf("deploy guide contains nonexistent publish command:\n%s", body)
	}
}

func TestSiteDocsIndexListsEverySection(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs")
	if err != nil {
		t.Fatalf("get docs index: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("docs index status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	body := readBody(t, response)
	for _, want := range []string{
		"<title>LeapView documentation</title>",
		"<h1>Documentation</h1>",
		`href="/docs/introduction"`,
		`href="/docs/concepts"`,
		`href="/docs/guides/build"`,
		`href="/docs/data-ingestion"`,
		`href="/docs/guides/operate"`,
		`href="/docs/enterprise-auth"`,
		`href="/docs/integrate"`,
		`href="/docs/reference"`,
		`href="/docs/architecture"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("docs index missing %q:\n%s", want, body)
		}
	}
}

func TestSiteDocumentationCatalogRendersJourneySections(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs")
	if err != nil {
		t.Fatalf("get docs index: %v", err)
	}
	defer response.Body.Close()
	body := readBody(t, response)
	for _, want := range []string{"Start here", "Core concepts", "Build dashboards", "Manage data", "Deploy and operate", "Security and administration", "Integrate", "Reference", "Architecture and contributing"} {
		if !strings.Contains(body, want) {
			t.Errorf("docs index missing journey section %q", want)
		}
	}
}

func TestSiteDocumentationSupportsNestedArticleSlugs(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/concepts/query-lifecycle")
	if err != nil {
		t.Fatalf("get nested documentation article: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("nested documentation status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body := readBody(t, response)
	for _, want := range []string{"<h1>Query and interaction lifecycle</h1>", "Core concepts", "Datastar signal flow", "About this page", "Edit this page"} {
		if !strings.Contains(body, want) {
			t.Errorf("nested documentation missing %q", want)
		}
	}
	for _, want := range []string{`class="site-docs-pagination"`, `aria-label="Documentation pagination"`} {
		if !strings.Contains(body, want) {
			t.Errorf("nested documentation missing %q", want)
		}
	}
}

func TestSiteDocumentationRendersSequentialPagination(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	tests := []struct {
		name          string
		slug          string
		previousHref  string
		previousTitle string
		nextHref      string
		nextTitle     string
	}{
		{
			name:      "first article",
			slug:      "introduction",
			nextHref:  "/docs/installation",
			nextTitle: "Installation",
		},
		{
			name:          "middle article",
			slug:          "getting-started",
			previousHref:  "/docs/installation",
			previousTitle: "Installation",
			nextHref:      "/docs/first-dashboard",
			nextTitle:     "Build your first dashboard",
		},
		{
			name:          "section boundary",
			slug:          "project-structure",
			previousHref:  "/docs/first-dashboard",
			previousTitle: "Build your first dashboard",
			nextHref:      "/docs/concepts",
			nextTitle:     "Core concepts",
		},
		{
			name:          "last article",
			slug:          "contributing/documentation",
			previousHref:  "/docs/contributing/configuration",
			previousTitle: "Add a configuration resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := server.Client().Get(server.URL + "/docs/" + tt.slug)
			if err != nil {
				t.Fatalf("get documentation article: %v", err)
			}
			defer response.Body.Close()
			body := readBody(t, response)
			if !strings.Contains(body, `aria-label="Documentation pagination"`) {
				t.Fatalf("documentation article missing pagination:\n%s", body)
			}
			assertPaginationLink(t, body, "previous", tt.previousHref, tt.previousTitle)
			assertPaginationLink(t, body, "next", tt.nextHref, tt.nextTitle)
		})
	}
}

func assertPaginationLink(t *testing.T, body, direction, href, title string) {
	t.Helper()
	class := `site-docs-pagination-` + direction
	if href == "" {
		if strings.Contains(body, class) {
			t.Errorf("documentation article unexpectedly renders %s pagination", direction)
		}
		return
	}
	label, rel := "Next", "next"
	if direction == "previous" {
		label, rel = "Previous", "prev"
	}
	for _, want := range []string{
		`class="site-docs-pagination-card ` + class + `"`,
		`href="` + href + `"`,
		`rel="` + rel + `"`,
		`aria-label="` + label + ` page: ` + title + `"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%s pagination missing %q:\n%s", direction, want, body)
		}
	}
}

func TestSiteDocumentationSearchFindsCatalogContent(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/search?q=semantic+relationships")
	if err != nil {
		t.Fatalf("search documentation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("search status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body := readBody(t, response)
	for _, want := range []string{"Search documentation", `value="semantic relationships"`, `href="/docs/concepts/semantic-models"`, "Semantic models"} {
		if !strings.Contains(body, want) {
			t.Errorf("search result missing %q", want)
		}
	}
}

func TestDocumentationNavigationUsesExplicitOverviewLabelsAndDeveloperGroups(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/architecture/runtime")
	if err != nil {
		t.Fatalf("get architecture documentation: %v", err)
	}
	defer response.Body.Close()
	body := readBody(t, response)
	for _, want := range []string{
		`data-site-docs-group="architecture-architecture"`,
		`data-site-docs-group="architecture-contributing"`,
		`href="/docs/architecture" title="Overview">Overview</a>`,
		`href="/docs/data-ingestion" title="Overview">Overview</a>`,
		`href="/docs/config" title="Overview">Overview</a>`,
		`title="Projects, workspaces, and environments">Projects, workspaces, and environments</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("architecture navigation missing %q", want)
		}
	}
}

func TestSiteDocumentationActiveSearchStreamsRankedResults(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	signals := `{"docsSearch":{"query":"semantic relationships"}}`
	response, err := server.Client().Get(server.URL + "/docs/search/active?datastar=" + url.QueryEscape(signals))
	if err != nil {
		t.Fatalf("active search documentation: %v", err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("active search content type = %q, want text/event-stream", got)
	}
	body := readBody(t, response)
	for _, want := range []string{`"docsSearch"`, `"resultQuery":"semantic relationships"`, `"title":"Semantic models"`, `"href":"/docs/concepts/semantic-models"`} {
		if !strings.Contains(body, want) {
			t.Errorf("active search result missing %q:\n%s", want, body)
		}
	}
}

func TestDocumentationMarkdownTablesExposeRowHeaders(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/configuration")
	if err != nil {
		t.Fatalf("get configuration documentation: %v", err)
	}
	defer response.Body.Close()
	body := readBody(t, response)
	if !strings.Contains(body, `<th scope="row">`) {
		t.Fatalf("configuration table does not expose body row headers")
	}
}

func TestEveryCatalogDocumentHasAReachableRoute(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()
	for _, document := range allSiteDocuments() {
		response, err := server.Client().Get(server.URL + "/docs/" + document.slug)
		if err != nil {
			t.Fatalf("get %s: %v", document.slug, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d, want %d", document.slug, response.StatusCode, http.StatusOK)
		}
	}
}

func TestSiteServesDeploymentScopedMCPGuide(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/guides/integrate/mcp")
	if err != nil {
		t.Fatalf("get MCP guide: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("MCP guide status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body := readBody(t, response)
	for _, want := range []string{
		"Connect an MCP host",
		"https://bi.example.com/mcp",
		"https://leapview.dev",
		"remote MCP custom connector guide",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("MCP guide missing %q:\n%s", want, body)
		}
	}
}

func TestSiteChartsDocumentationParentPathIsNotAnArticle(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/visuals")
	if err != nil {
		t.Fatalf("get chart documentation parent path: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("chart documentation parent status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

func TestSiteAPIReferenceIsGeneratedFromOpenAPI(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/api/workspaces")
	if err != nil {
		t.Fatalf("get API reference: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("API reference status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body := readBody(t, response)
	for _, want := range []string{"<title>Workspaces</title>", "<h1>Workspaces</h1>", "Get the active asset graph", "<code>GET /api/v1/workspaces"} {
		if !strings.Contains(body, want) {
			t.Errorf("API reference missing %q:\n%s", want, body)
		}
	}

	specification, err := server.Client().Get(server.URL + "/docs/openapi.yaml")
	if err != nil {
		t.Fatalf("get OpenAPI specification: %v", err)
	}
	defer specification.Body.Close()
	if got := specification.Header.Get("Content-Type"); !strings.Contains(got, "application/yaml") {
		t.Errorf("OpenAPI content type = %q, want application/yaml", got)
	}
	if !strings.Contains(readBody(t, specification), "openapi: 3.0.0") {
		t.Error("served OpenAPI specification is not the generated contract")
	}
}

func TestSiteServesMachineDocumentationArtifacts(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	tests := []struct {
		path        string
		contentType string
		contains    []string
	}{
		{path: "/llms.txt", contentType: "text/plain", contains: []string{"# LeapView", "/mcp", "/docs/cli/manifest.json", "/docs/api/operations.json"}},
		{path: "/docs/cli/manifest.json", contentType: "application/json", contains: []string{`"schemaVersion": 1`, `"id": "deploy"`, `"effect": "write"`}},
		{path: "/docs/api/operations.json", contentType: "application/json", contains: []string{`"schemaVersion": 1`, `"operationId": "listWorkspaces"`}},
		{path: "/docs/api/operations/listWorkspaces.json", contentType: "application/json", contains: []string{`"operationId": "listWorkspaces"`, `"method": "GET"`, `"schemas": {`, `"WorkspaceListResponse": {`}},
		{path: "/docs/api/operations/listWorkspaces.md", contentType: "text/markdown", contains: []string{"# List workspaces", "`GET /api/v1/workspaces`", "USE_WORKSPACE"}},
		{path: "/docs/cli/commands/deploy.json", contentType: "application/json", contains: []string{`"id": "deploy"`, `"usage": "leapview deploy`}},
		{path: "/docs/cli/commands/deploy.md", contentType: "text/markdown", contains: []string{"# leapview deploy", "## Usage"}},
		{path: "/docs/cli/commands/semantic-models-query.md", contentType: "text/markdown", contains: []string{"# leapview semantic-models query", "## Usage", "## Behavior", "--body-json"}},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response, err := server.Client().Get(server.URL + test.path)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.StatusCode)
			}
			if got := response.Header.Get("Content-Type"); !strings.Contains(got, test.contentType) {
				t.Errorf("content type = %q, want %q", got, test.contentType)
			}
			body := readBody(t, response)
			for _, want := range test.contains {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q:\n%s", want, body)
				}
			}
		})
	}
}

func TestSiteCLIReferenceGroupsSubcommandsAndRedirectsLeafPages(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	article, err := server.Client().Get(server.URL + "/docs/cli/semantic-models")
	if err != nil {
		t.Fatal(err)
	}
	defer article.Body.Close()
	if article.StatusCode != http.StatusOK {
		t.Fatalf("semantic models status = %d, want 200", article.StatusCode)
	}
	body := readBody(t, article)
	for _, want := range []string{
		`<h1>leapview semantic-models</h1>`,
		`<h2 id="subcommands">Subcommands</h2>`,
		`<h3 id="query">query</h3>`,
		`<h4>Usage</h4>`,
		`leapview semantic-models query &lt;model&gt; &lt;dataset&gt;`,
		`href="/docs/cli/commands/semantic-models-query.json"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("grouped CLI article missing %q:\n%s", want, body)
		}
	}

	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	legacy, err := client.Get(server.URL + "/docs/cli/semantic-models-query")
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Body.Close()
	if legacy.StatusCode != http.StatusPermanentRedirect {
		t.Fatalf("legacy leaf status = %d, want %d", legacy.StatusCode, http.StatusPermanentRedirect)
	}
	if got, want := legacy.Header.Get("Location"), "/docs/cli/semantic-models#query"; got != want {
		t.Errorf("legacy leaf location = %q, want %q", got, want)
	}
}

func TestSiteDocumentationMCPTools(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	initialize := postMCP(t, server.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	for _, want := range []string{`"protocolVersion":"2025-11-25"`, `"name":"leapview-docs"`} {
		if !strings.Contains(initialize, want) {
			t.Errorf("initialize response missing %q:\n%s", want, initialize)
		}
	}

	tools := postMCP(t, server.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	for _, want := range []string{`"name":"docs_catalog"`, `"name":"docs_search"`, `"name":"docs_read"`} {
		if !strings.Contains(tools, want) {
			t.Errorf("tools response missing %q:\n%s", want, tools)
		}
	}

	search := postMCP(t, server.URL, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"docs_search","arguments":{"query":"line chart","limit":2}}}`)
	if !strings.Contains(search, "visuals/line") {
		t.Errorf("search response does not contain line chart:\n%s", search)
	}

	read := postMCP(t, server.URL, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"docs_read","arguments":{"id":"api:listWorkspaces","format":"json"}}}`)
	if !strings.Contains(read, "listWorkspaces") || !strings.Contains(read, "/api/v1/workspaces") {
		t.Errorf("read response does not contain operation slice:\n%s", read)
	}

	crossOrigin, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":5,"method":"tools/list","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	crossOrigin.Header.Set("Content-Type", "application/json")
	crossOrigin.Header.Set("Origin", "https://untrusted.example")
	response, err := http.DefaultClient.Do(crossOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin MCP status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

func postMCP(t *testing.T, baseURL, body string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("MCP status = %d, body = %s", response.StatusCode, readBody(t, response))
	}
	return readBody(t, response)
}

func TestSiteChartDocumentationArticleRendersConfiguration(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/visuals/line")
	if err != nil {
		t.Fatalf("get line chart documentation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("line chart documentation status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	body := readBody(t, response)
	for _, want := range []string{
		"<title>Line chart</title>",
		`data-init="@get(&#39;/updates?view=visual-docs&amp;document=visuals%2Fline&#39;, {openWhenHidden: true})"`,
		`<nav class="site-docs-breadcrumb" aria-label="Breadcrumb"><ol><li><a href="/docs/visuals/overview">Visuals</a></li><li><span aria-current="page">Line chart</span></li></ol></nav>`,
		"<h1>Line chart</h1>",
		`<h2 id="site-visual-api-reference">API reference</h2>`,
		`<table aria-labelledby="site-visual-api-reference">`,
		`<th scope="col">Field</th><th scope="col">Type</th><th scope="col">Default</th><th scope="col">Allowed values</th><th scope="col">Description</th>`,
		`<code>options.step</code>`,
		`<code>string | boolean</code>`,
		`<lv-site-visual-example example-id="revenue_line"></lv-site-visual-example>`,
		`<lv-site-visual-example example-id="revenue_line_status"></lv-site-visual-example>`,
		`<lv-site-visual-example example-id="revenue_line_step"></lv-site-visual-example>`,
		`<div class="site-visual-key-fields" aria-label="Key fields" data-key-fields="[&#34;query.dimensions&#34;,&#34;query.measures&#34;]">`,
		`<button type="button" class="site-visual-key-field" data-visual-key-field="options.step" aria-label="Highlight options.step in YAML"><code>options.step</code></button>`,
		"<h2>Basic</h2>",
		"type: line",
		"visual-example=revenue_line_step",
		`href="/docs/visuals/line"`,
		`href="/docs/visuals/kpi"`,
		`<details class="site-docs-nav-group site-docs-nav-group-active" data-site-docs-group="reference-visuals" open="true">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("line chart documentation missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `class="site-visual-api-summary"`) || strings.Contains(body, `class="site-visual-field-reference"`) {
		t.Error("API reference is rendered inside a visual-specific container instead of the article's Markdown flow")
	}
	stepped := strings.Index(body, "<h2>Stepped line</h2>")
	api := strings.Index(body, `<h2 id="site-visual-api-reference">API reference</h2>`)
	about := strings.Index(body, `<h2 id="site-docs-about-this-page">About this page</h2>`)
	if stepped < 0 || api < stepped || about < api {
		t.Errorf("article order = stepped %d, API %d, about %d; want examples, API reference, footer", stepped, api, about)
	}
}

func TestSiteEveryVisualTypeHasDocumentation(t *testing.T) {
	if got, want := len(visualDocuments), 26; got != want {
		t.Fatalf("documented visual types = %d, want %d", got, want)
	}

	server := httptest.NewServer(NewHandler())
	defer server.Close()
	for _, document := range visualDocuments {
		response, err := server.Client().Get(server.URL + "/docs/" + document.slug)
		if err != nil {
			t.Fatalf("get %s documentation: %v", document.slug, err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			t.Errorf("%s documentation status = %d, want %d", document.slug, response.StatusCode, http.StatusOK)
			continue
		}
		body := readBody(t, response)
		if !strings.Contains(body, "visual-example=") {
			t.Errorf("%s documentation has no executable configuration block", document.slug)
		}
		if !strings.Contains(body, `<lv-site-visual-example example-id="`) {
			t.Errorf("%s documentation has no live visual example", document.slug)
		}
	}
}

func TestSiteServesGeneratedConfigurationReferenceAndSchema(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	article, err := server.Client().Get(server.URL + "/docs/config/project")
	if err != nil {
		t.Fatalf("get generated project configuration reference: %v", err)
	}
	defer article.Body.Close()
	if article.StatusCode != http.StatusOK {
		t.Fatalf("project configuration status = %d, want %d", article.StatusCode, http.StatusOK)
	}
	body := readBody(t, article)
	for _, want := range []string{"<h1>Project configuration</h1>", "<h2>Example</h2>", "<h2>Fields</h2>", "/docs/schemas/project.schema.json", `href="/docs/config/project"`} {
		if !strings.Contains(body, want) {
			t.Errorf("project configuration reference missing %q:\n%s", want, body)
		}
	}

	schema, err := server.Client().Get(server.URL + "/docs/schemas/project.schema.json")
	if err != nil {
		t.Fatalf("get project configuration schema: %v", err)
	}
	defer schema.Body.Close()
	if schema.StatusCode != http.StatusOK {
		t.Fatalf("project schema status = %d, want %d", schema.StatusCode, http.StatusOK)
	}
	if got := schema.Header.Get("Content-Type"); !strings.Contains(got, "application/schema+json") {
		t.Errorf("project schema content type = %q", got)
	}
	if !strings.Contains(readBody(t, schema), `"kind": {`) {
		t.Error("project schema does not contain the generated contract")
	}
}

func TestSiteGettingStartedRedirectsToDocumentation(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := client.Get(server.URL + "/getting-started")
	if err != nil {
		t.Fatalf("get legacy getting started route: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPermanentRedirect {
		t.Fatalf("legacy route status = %d, want %d", response.StatusCode, http.StatusPermanentRedirect)
	}
	if got := response.Header.Get("Location"); got != "/docs/getting-started" {
		t.Errorf("legacy route location = %q, want %q", got, "/docs/getting-started")
	}
}

func TestSiteUpdatesSendReadySignal(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/updates")
	if err != nil {
		t.Fatalf("get updates: %v", err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("updates content type = %q, want text/event-stream", got)
	}

	line := readSSEUntil(t, response, `"ready":true`)
	for _, want := range []string{`event: datastar-patch-signals`, `"site"`, `"ready":true`} {
		if !strings.Contains(line, want) {
			t.Errorf("initial updates missing %q:\n%s", want, line)
		}
	}
}

func TestSiteUpdatesCloseAfterInitialPatch(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	client := server.Client()
	client.Timeout = time.Second
	response, err := client.Get(server.URL + "/updates")
	if err != nil {
		t.Fatalf("get updates: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read bounded updates response: %v", err)
	}
	if !strings.Contains(string(body), `"ready":true`) {
		t.Fatalf("bounded updates response does not contain initial patch:\n%s", body)
	}
}

func TestSiteVisualShowcaseUpdatesIncludeEveryVisualType(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/updates?view=visuals")
	if err != nil {
		t.Fatalf("get chart showcase updates: %v", err)
	}
	defer response.Body.Close()

	line := readSSEUntil(t, response, `"visuals"`)
	for _, want := range []string{`"type":"line"`, `"type":"sunburst"`, `"type":"kpi"`, `"type":"table"`, `"type":"matrix"`, `"type":"pivot"`} {
		if !strings.Contains(line, want) {
			t.Errorf("chart showcase updates missing %q:\n%s", want, line)
		}
	}
}

func TestSiteVisualDocumentationUpdatesAreScopedToTheArticle(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/updates?view=visual-docs&document=visuals%2Fline")
	if err != nil {
		t.Fatalf("get visual documentation updates: %v", err)
	}
	defer response.Body.Close()
	line := readSSEUntil(t, response, `"visuals"`)
	for _, want := range []string{`"id":"revenue_line"`, `"id":"revenue_line_status"`, `"id":"revenue_line_step"`, `"step":"middle"`} {
		if !strings.Contains(line, want) {
			t.Errorf("line documentation updates missing %q:\n%s", want, line)
		}
	}
	for _, unwanted := range []string{`"type":"area"`, `"type":"kpi"`, `"type":"table"`} {
		if strings.Contains(line, unwanted) {
			t.Errorf("line documentation updates unexpectedly include %q:\n%s", unwanted, line)
		}
	}

	missing, err := server.Client().Get(server.URL + "/updates?view=visual-docs&document=visuals%2Fmissing")
	if err != nil {
		t.Fatalf("get missing visual documentation updates: %v", err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing visual documentation status = %d, want %d", missing.StatusCode, http.StatusNotFound)
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(bytes)
}

func readSSEUntil(t *testing.T, response *http.Response, marker string) string {
	t.Helper()
	var builder strings.Builder
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		builder.WriteString(line)
		builder.WriteByte('\n')
		if strings.Contains(line, marker) {
			return builder.String()
		}
		if line == "" {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read updates: %v", err)
	}
	t.Fatalf("updates ended before %q:\n%s", marker, builder.String())
	return ""
}
