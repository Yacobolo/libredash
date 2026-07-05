package pagestream

import (
	"bytes"
	"strings"
	"testing"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func TestRenderPageIncludesSignalsUpdatesInitMainAttrsAndBody(t *testing.T) {
	var body bytes.Buffer
	err := RenderPage(PageSpec{
		Title:            "Test Page",
		HTMLAttrs:        []g.Node{g.Attr("data-color-mode", "auto")},
		Head:             []g.Node{h.Meta(g.Attr("name", "test-head"))},
		MainAttrs:        []g.Node{h.ID("root"), h.Class("app-shell")},
		Signals:          map[string]any{"runtime": map[string]any{}, "page": map[string]any{"title": "Test"}},
		BeforeStreamInit: []string{"$ready = true"},
		UpdatesURL:       "/updates?route=test",
		Body:             []g.Node{h.Div(h.ID("content"), g.Text("Hello"))},
	}).Render(&body)
	if err != nil {
		t.Fatalf("render document: %v", err)
	}
	html := body.String()
	for _, want := range []string{
		"<title>Test Page</title>",
		`data-color-mode="auto"`,
		`<main id="root" class="app-shell"`,
		`data-signals=`,
		`updatesUrl`,
		`/updates?route=test`,
		`data-init="$ready = true; @get($runtime.updatesUrl, {openWhenHidden: true})"`,
		`<div id="content">Hello</div>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered document missing %q:\n%s", want, html)
		}
	}
}

func TestRenderPageRequiresUpdatesURL(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RenderPage did not panic for missing UpdatesURL")
		}
	}()
	_ = RenderPage(PageSpec{Signals: map[string]any{}})
}

func TestRenderPageRequiresCanonicalUpdatesPath(t *testing.T) {
	for _, updatesURL := range []string{"/data/updates", "/updates-old", "https://example.com/updates"} {
		t.Run(updatesURL, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("RenderPage did not panic for %q", updatesURL)
				}
			}()
			_ = RenderPage(PageSpec{Signals: map[string]any{}, UpdatesURL: updatesURL})
		})
	}
}

func TestRenderPageRejectsMismatchedRuntimeUpdatesURL(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RenderPage did not panic for mismatched runtime.updatesUrl")
		}
	}()
	_ = RenderPage(PageSpec{
		Signals:    map[string]any{"runtime": map[string]any{"updatesUrl": "/updates?route=wrong"}},
		UpdatesURL: "/updates?route=right",
	})
}

func TestRenderPageSeedsTypedRuntimeUpdatesURL(t *testing.T) {
	type runtimeSignal struct {
		Kind       string `json:"kind,omitempty"`
		UpdatesURL string `json:"updatesUrl,omitempty"`
	}
	var body bytes.Buffer
	err := RenderPage(PageSpec{
		Title:      "Typed Runtime",
		Signals:    map[string]any{"runtime": runtimeSignal{Kind: "test"}},
		UpdatesURL: "/updates?route=test",
	}).Render(&body)
	if err != nil {
		t.Fatalf("render page: %v", err)
	}
	html := body.String()
	if !strings.Contains(html, "updatesUrl") || !strings.Contains(html, "/updates?route=test") {
		t.Fatalf("rendered page did not seed typed runtime updates URL:\n%s", html)
	}
}

func TestRenderInitSkipsEmptyExpressions(t *testing.T) {
	if got, want := initExpression("", "$a = 1", "", "@get('/updates')"), "$a = 1; @get('/updates')"; got != want {
		t.Fatalf("init expression = %q, want %q", got, want)
	}
}
