package ui

import (
	"html"
	"os"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/access/http/mcpoauth"
	"github.com/Yacobolo/leapview/internal/dashboard"
	g "maragu.dev/gomponents"
)

func TestProductDocumentsUseLeapViewBrandAndFavicon(t *testing.T) {
	tests := []struct {
		name      string
		document  g.Node
		wantTitle string
	}{
		{name: "login", document: LoginPage(), wantTitle: "LeapView Login"},
		{name: "catalog", document: CatalogPage(dashboard.Catalog{}), wantTitle: "LeapView Dashboards"},
		{name: "OAuth consent", document: OAuthConsentPage(mcpoauth.Consent{ClientName: "Agent", Resource: "https://example.test"}, nil, "csrf"), wantTitle: "Authorize MCP access · LeapView"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			if err := test.document.Render(&output); err != nil {
				t.Fatalf("render product document: %v", err)
			}
			rendered := html.UnescapeString(output.String())
			if !strings.Contains(rendered, "<title>"+test.wantTitle+"</title>") {
				t.Errorf("document does not contain LeapView title %q", test.wantTitle)
			}
			if !strings.Contains(rendered, `<link rel="icon" href="/static/favicon.svg?v=dev" type="image/svg+xml">`) {
				t.Errorf("document does not contain the LeapView favicon")
			}
		})
	}
}

func TestLoginBootstrapUsesLeapViewName(t *testing.T) {
	page := LoginBootstrapSignals()["page"]
	rendered := html.UnescapeString(jsonString(page))
	if !strings.Contains(rendered, `"title":"LeapView"`) {
		t.Fatalf("login bootstrap does not use LeapView: %s", rendered)
	}
}

func TestProductFaviconUsesApertureRing(t *testing.T) {
	contents, err := os.ReadFile("../../static/favicon.svg")
	if err != nil {
		t.Fatalf("read product favicon: %v", err)
	}
	icon := string(contents)
	for _, want := range []string{`<circle cx="12" cy="12" r="10"`, `d="m14.31 8 5.74 9.94"`} {
		if !strings.Contains(icon, want) {
			t.Errorf("product favicon does not contain %q", want)
		}
	}
}
