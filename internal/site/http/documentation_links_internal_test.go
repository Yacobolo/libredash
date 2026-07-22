package http

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestDocumentationLinkCrawlerReportsBrokenRoutesAndFragments(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /docs/start", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<a href="/docs/missing">Missing</a><a href="/docs/target#absent">Absent section</a><a href="/docs/alias">Redirected section</a>`))
	})
	mux.HandleFunc("GET /docs/target", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<h1 id="present">Target</h1>`))
	})
	mux.HandleFunc("GET /docs/alias", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/target#present", http.StatusPermanentRedirect)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	issues := crawlInternalDocumentationLinks(server.Client(), baseURL, "/docs/start")
	if len(issues) != 2 {
		t.Fatalf("issues = %v, want exactly the broken route and fragment", issues)
	}
	for _, want := range []string{
		`/docs/start links to /docs/missing: status 404`,
		`/docs/start links to /docs/target#absent: fragment "absent" does not exist`,
	} {
		if !slices.Contains(issues, want) {
			t.Errorf("issues = %v, want %q", issues, want)
		}
	}
}

func TestEveryRenderedDocumentationLinkResolves(t *testing.T) {
	server := httptest.NewServer(NewHandler())
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	for _, issue := range crawlInternalDocumentationLinks(server.Client(), baseURL, "/docs") {
		t.Error(issue)
	}
}

type documentationPage struct {
	finalURL  *url.URL
	fragments map[string]struct{}
	hrefs     []string
	status    int
	err       error
}

type documentationLink struct {
	source string
	target *url.URL
}

func crawlInternalDocumentationLinks(client *http.Client, baseURL *url.URL, entrypoint string) []string {
	start, err := baseURL.Parse(entrypoint)
	if err != nil {
		return []string{fmt.Sprintf("parse documentation entrypoint %q: %v", entrypoint, err)}
	}
	queue := []documentationLink{{target: start}}
	pages := map[string]documentationPage{}
	checkedLinks := map[string]struct{}{}
	crawledPages := map[string]struct{}{}
	issues := make([]string, 0)

	for len(queue) > 0 {
		link := queue[0]
		queue = queue[1:]
		if !isInternalDocumentationURL(baseURL, link.target) {
			continue
		}

		displayTarget := documentationURLPath(link.target)
		checkKey := link.source + "\x00" + displayTarget
		if _, exists := checkedLinks[checkKey]; exists {
			continue
		}
		checkedLinks[checkKey] = struct{}{}

		requestURL := *link.target
		requestURL.Fragment = ""
		pageKey := requestURL.String()
		page, exists := pages[pageKey]
		if !exists {
			page = fetchDocumentationPage(client, &requestURL)
			pages[pageKey] = page
		}
		if page.err != nil {
			issues = append(issues, documentationLinkIssue(link.source, displayTarget, page.err.Error()))
			continue
		}
		if page.status != http.StatusOK {
			issues = append(issues, documentationLinkIssue(link.source, displayTarget, fmt.Sprintf("status %d", page.status)))
			continue
		}

		fragment := link.target.Fragment
		if page.finalURL != nil && page.finalURL.Fragment != "" {
			fragment = page.finalURL.Fragment
		}
		if fragment != "" {
			if _, exists := page.fragments[fragment]; !exists {
				issues = append(issues, documentationLinkIssue(link.source, displayTarget, fmt.Sprintf("fragment %q does not exist", fragment)))
			}
		}

		finalPageKey := pageKey
		if page.finalURL != nil {
			finalURL := *page.finalURL
			finalURL.Fragment = ""
			finalPageKey = finalURL.String()
		}
		if _, crawled := crawledPages[finalPageKey]; crawled {
			continue
		}
		crawledPages[finalPageKey] = struct{}{}
		source := documentationURLPath(page.finalURL)
		for _, href := range page.hrefs {
			target, parseErr := page.finalURL.Parse(href)
			if parseErr != nil {
				issues = append(issues, documentationLinkIssue(source, href, "invalid URL: "+parseErr.Error()))
				continue
			}
			if isInternalDocumentationURL(baseURL, target) {
				queue = append(queue, documentationLink{source: source, target: target})
			}
		}
	}

	sort.Strings(issues)
	return issues
}

func fetchDocumentationPage(client *http.Client, target *url.URL) documentationPage {
	response, err := client.Get(target.String())
	if err != nil {
		return documentationPage{err: err}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return documentationPage{err: err}
	}
	page := documentationPage{
		finalURL:  response.Request.URL,
		fragments: map[string]struct{}{},
		status:    response.StatusCode,
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType != "text/html" || response.StatusCode != http.StatusOK {
		return page
	}
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		page.err = fmt.Errorf("parse HTML: %w", err)
		return page
	}
	collectDocumentationLinks(document, &page)
	return page
}

func collectDocumentationLinks(node *html.Node, page *documentationPage) {
	if node.Type == html.ElementNode {
		for _, attribute := range node.Attr {
			switch {
			case attribute.Key == "id" && attribute.Val != "":
				page.fragments[attribute.Val] = struct{}{}
			case node.Data == "a" && attribute.Key == "name" && attribute.Val != "":
				page.fragments[attribute.Val] = struct{}{}
			case node.Data == "a" && attribute.Key == "href":
				page.hrefs = append(page.hrefs, attribute.Val)
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectDocumentationLinks(child, page)
	}
}

func isInternalDocumentationURL(baseURL, target *url.URL) bool {
	return target.Scheme == baseURL.Scheme && target.Host == baseURL.Host && (target.Path == "/docs" || strings.HasPrefix(target.Path, "/docs/"))
}

func documentationURLPath(target *url.URL) string {
	if target == nil {
		return ""
	}
	path := target.EscapedPath()
	if target.RawQuery != "" {
		path += "?" + target.RawQuery
	}
	if target.Fragment != "" {
		path += "#" + target.EscapedFragment()
	}
	return path
}

func documentationLinkIssue(source, target, problem string) string {
	if source == "" {
		return target + ": " + problem
	}
	return source + " links to " + target + ": " + problem
}
