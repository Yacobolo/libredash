package http

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
		"<title>LibreDash — BI dashboards as code</title>",
		`data-color-mode="auto"`,
		`/updates`,
		`data-init="@get(&#39;/updates&#39;, {openWhenHidden: true})"`,
		`/shared/app.css`,
		`/static/site.css`,
		`/static/site-page.js`,
		`<meta name="view-transition" content="same-origin">`,
		`<ld-topology-background class="site-hero-background" aria-hidden="true"></ld-topology-background>`,
		`<section id="main-content" class="site-hero">`,
		`<div class="site-hero-proof">`,
		`<ld-site-feature-icon name="database" aria-hidden="true"></ld-site-feature-icon>`,
		`<ld-site-feature-icon name="chart" aria-hidden="true"></ld-site-feature-icon>`,
		`<ld-site-feature-icon name="git-branch" aria-hidden="true"></ld-site-feature-icon>`,
		`<section id="demo" class="site-product-proof">`,
		`<section class="site-cta">`,
		`<footer class="site-footer" role="contentinfo">`,
		`<header class="site-header">`,
		`<ld-site-theme-toggle></ld-site-theme-toggle>`,
		"<ld-site-chart-demo>",
		`data-on:click="@post(&#39;/demo&#39;, {payload: {demo: {metric: &#39;revenue&#39;}}})"`,
		`data-on:click="@post(&#39;/demo&#39;, {payload: {demo: {metric: &#39;orders&#39;}}})"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("home page missing %q:\n%s", want, body)
		}
	}
}

func TestSiteChartsRendersPageStreamShowcase(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/charts")
	if err != nil {
		t.Fatalf("get charts page: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("charts status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	body := readBody(t, response)
	for _, want := range []string{
		"<title>LibreDash chart showcase</title>",
		`data-init="@get(&#39;/updates?view=charts&#39;, {openWhenHidden: true})"`,
		"<ld-site-chart-showcase>",
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
		"<title>Get started with LibreDash</title>",
		`<ld-site-docs-drawer-toggle></ld-site-docs-drawer-toggle>`,
		`<nav class="site-docs-breadcrumb" aria-label="Breadcrumb"><ol><li><a href="/docs">Documentation</a></li><li><span aria-current="page">Getting started</span></li></ol></nav>`,
		`<button class="site-docs-drawer-backdrop" type="button" aria-label="Close documentation menu" aria-hidden="true" tabindex="-1" data-site-docs-drawer-close="true"></button>`,
		`<ld-site-markdown-copy`,
		`<article id="main-content" class="site-docs-article">`,
		`<aside class="site-docs-sidebar" id="site-docs-sidebar">`,
		`<a class="site-docs-link site-docs-link-current" href="/docs/getting-started" aria-current="page">Get started with LibreDash</a>`,
		`<details class="site-docs-nav-group" data-site-docs-group="configuration">`,
		`<a class="site-docs-link" href="/docs/configuration">Environment</a>`,
		`<a class="site-docs-link" href="/docs/enterprise-auth">Enterprise auth</a>`,
		`<a class="site-docs-link" href="/docs/storage-architecture">Storage architecture</a>`,
		`<details class="site-docs-nav-group site-docs-nav-group-active" data-site-docs-group="documentation" open="true">`,
		`<summary>Documentation</summary>`,
		`<details class="site-docs-nav-group" data-site-docs-group="charts">`,
		`<summary>Charts</summary>`,
		`<ul class="site-docs-nav-tree">`,
		`<a class="site-docs-link" href="/docs/charts/overview">Overview</a>`,
		"<h1>Get started with LibreDash</h1>",
		"<h2>Bootstrap the workspace</h2>",
		"task bootstrap",
		"task dev",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("getting started page missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "LibreDash Docs") {
		t.Errorf("getting started page retains the redundant docs header:\n%s", body)
	}
	if strings.Contains(body, "Guides and reference") {
		t.Errorf("getting started page retains the redundant sidebar heading:\n%s", body)
	}
	if strings.Contains(body, `<footer class="site-footer" role="contentinfo">`) {
		t.Errorf("getting started page retains the marketing footer:\n%s", body)
	}
}

func TestSiteDocsIndexListsEveryArticle(t *testing.T) {
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
		"<title>LibreDash documentation</title>",
		"<h1>Documentation</h1>",
		`href="/docs/getting-started"`,
		`href="/docs/configuration"`,
		`href="/docs/enterprise-auth"`,
		`href="/docs/storage-architecture"`,
		`href="/docs/charts/overview"`,
		`href="/docs/charts/line"`,
		`href="/docs/charts/kpi"`,
		`href="/docs/api"`,
		`href="/docs/api/workspaces"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("docs index missing %q:\n%s", want, body)
		}
	}
}

func TestSiteChartsDocumentationParentPathIsNotAnArticle(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/charts")
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

func TestSiteChartDocumentationArticleRendersConfiguration(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/docs/charts/line")
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
		`data-init="@get(&#39;/updates?view=charts&#39;, {openWhenHidden: true})"`,
		`<nav class="site-docs-breadcrumb" aria-label="Breadcrumb"><ol><li><a href="/docs/charts/overview">Charts</a></li><li><span aria-current="page">Line chart</span></li></ol></nav>`,
		"<h1>Line chart</h1>",
		`<ld-site-doc-chart chart-id="line"></ld-site-doc-chart>`,
		"<h2>Configuration</h2>",
		"type: line",
		`href="/docs/charts/line"`,
		`href="/docs/charts/kpi"`,
		`<details class="site-docs-nav-group site-docs-nav-group-active" data-site-docs-group="charts" open="true">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("line chart documentation missing %q:\n%s", want, body)
		}
	}
}

func TestSiteEveryChartTypeHasDocumentation(t *testing.T) {
	if got, want := len(visualDocuments), 23; got != want {
		t.Fatalf("documented chart types = %d, want %d", got, want)
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
		if !strings.Contains(body, "<h2>Configuration</h2>") {
			t.Errorf("%s documentation has no configuration block", document.slug)
		}
		if !strings.Contains(body, `<ld-site-doc-chart chart-id="`+document.chartID+`"></ld-site-doc-chart>`) {
			t.Errorf("%s documentation has no live chart component", document.slug)
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

func TestSiteUpdatesSendInitialDemoSignal(t *testing.T) {
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

	line := readSSEUntil(t, response, `"metric":"revenue"`)
	for _, want := range []string{`event: datastar-patch-signals`, `"demo"`, `"title":"Monthly revenue"`} {
		if !strings.Contains(line, want) {
			t.Errorf("initial updates missing %q:\n%s", want, line)
		}
	}
}

func TestSiteChartShowcaseUpdatesIncludeEveryChartType(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/updates?view=charts")
	if err != nil {
		t.Fatalf("get chart showcase updates: %v", err)
	}
	defer response.Body.Close()

	line := readSSEUntil(t, response, `"charts"`)
	for _, want := range []string{`"type":"line"`, `"type":"sunburst"`, `"type":"kpi"`, `"tables"`, `"kind":"matrix_table"`, `"kind":"pivot_table"`, `"title":"Orders conditional formatting"`} {
		if !strings.Contains(line, want) {
			t.Errorf("chart showcase updates missing %q:\n%s", want, line)
		}
	}
}

func TestSiteDemoCommandPatchesRequestedMetric(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Post(server.URL+"/demo", "application/json", strings.NewReader(`{"demo":{"metric":"orders"}}`))
	if err != nil {
		t.Fatalf("post demo: %v", err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("demo content type = %q, want text/event-stream", got)
	}
	body := readBody(t, response)
	for _, want := range []string{`"metric":"orders"`, `"title":"Monthly orders"`} {
		if !strings.Contains(body, want) {
			t.Errorf("demo response missing %q:\n%s", want, body)
		}
	}
}

func TestSiteDemoCommandRejectsUnknownMetric(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	response, err := server.Client().Post(server.URL+"/demo", "application/json", strings.NewReader(`{"demo":{"metric":"unknown"}}`))
	if err != nil {
		t.Fatalf("post demo: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("demo status = %d, want %d", response.StatusCode, http.StatusBadRequest)
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
